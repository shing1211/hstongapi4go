// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package push

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"google.golang.org/protobuf/proto"

	pbmsg "github.com/shing1211/hstongapi4go/gen/common/msg"
	hqnotify "github.com/shing1211/hstongapi4go/gen/hq/notify"
	tradenotify "github.com/shing1211/hstongapi4go/gen/trade/notify"
	"github.com/shing1211/hstongapi4go/pkg/types"
)

// Defaults applied by New when no Option overrides them.
const (
	// DefaultAddr is the local Gateway TCP push address.
	DefaultAddr = "127.0.0.1:11112"
	// DefaultMinReconnect is the initial reconnect delay.
	DefaultMinReconnect = 500 * time.Millisecond
	// DefaultMaxReconnect is the ceiling on the reconnect delay.
	DefaultMaxReconnect = 30 * time.Second
	// dispatchBuffer is the per-notify-type handler queue depth. When a handler
	// falls behind, the read loop drops the oldest queued notification for that
	// type rather than blocking.
	dispatchBuffer = 64
	// errorBuffer is the error-channel queue depth, using the same drop-oldest
	// policy.
	errorBuffer = 16
)

// Sentinel errors returned by Client.
var (
	// ErrClosed is returned by Connect, Run, and SubscribeTypes after Close.
	ErrClosed = errors.New("push: client is closed")
)

// DialFunc dials the push address. It has the signature of
// (*net.Dialer).DialContext and may be replaced with WithDialer in tests.
type DialFunc func(ctx context.Context, network, address string) (net.Conn, error)

// Options holds the resolved configuration of a Client. Callers normally build
// it through With... options passed to New rather than constructing it
// directly.
type Options struct {
	// Addr is the Gateway push address in "host:port" form. It defaults to
	// DefaultAddr.
	Addr string
	// Dial dials Addr. It defaults to a fresh *net.Dialer's DialContext.
	Dial DialFunc
	// MinReconnect is the initial reconnect delay. It defaults to
	// DefaultMinReconnect.
	MinReconnect time.Duration
	// MaxReconnect is the reconnect delay ceiling. It defaults to
	// DefaultMaxReconnect and is never allowed below MinReconnect.
	MaxReconnect time.Duration

	// VerifyPublicKey, when non-empty, enables verification of each push
	// frame's SHA1WithRSA bodySHA1 signature against the raw body. It accepts
	// a PEM PUBLIC KEY block, a base64 SPKI string, or raw SPKI DER (see
	// ParsePublicKey). Verification is off by default (ADR 0005).
	VerifyPublicKey []byte
	// VerifyRequired controls the failure policy when VerifyPublicKey is set:
	// true drops a frame whose signature is missing or invalid, false reports
	// the error but still delivers the frame. It has no effect when
	// VerifyPublicKey is empty.
	VerifyRequired bool

	// onReconnect are the hooks invoked after a successful reconnect.
	onReconnect []func(error)
}

// Option mutates an Options value. Options are applied in order by New, so the
// last one that sets a field wins.
type Option func(*Options)

// WithAddr sets the Gateway push address in "host:port" form. An empty value
// restores DefaultAddr.
func WithAddr(addr string) Option {
	return func(o *Options) {
		if addr == "" {
			o.Addr = DefaultAddr
			return
		}
		o.Addr = addr
	}
}

// WithDialer replaces the dialer used to establish the push connection. It is
// primarily useful in tests, for example to route reconnections to a listener
// on a different port. A nil dialer restores the default.
func WithDialer(d DialFunc) Option {
	return func(o *Options) {
		if d == nil {
			o.Dial = nil
			return
		}
		o.Dial = d
	}
}

// WithReconnect sets the reconnect backoff bounds. The delay starts at min,
// doubles after each failed attempt, is capped at max, and gains up to one
// delay's worth of jitter. Non-positive values restore the defaults; a max
// below min is raised to min.
func WithReconnect(min, max time.Duration) Option {
	return func(o *Options) {
		if min > 0 {
			o.MinReconnect = min
		}
		if max > 0 {
			o.MaxReconnect = max
		}
	}
}

// WithOnReconnect registers a hook invoked after each successful reconnect with
// the error that ended the previous connection. Hooks are invoked from the Run
// goroutine, so they must not block for long. Multiple hooks may be registered;
// they run in registration order.
func WithOnReconnect(fn func(error)) Option {
	return func(o *Options) {
		if fn != nil {
			o.onReconnect = append(o.onReconnect, fn)
		}
	}
}

// WithVerification enables opt-in verification of each push frame's 128-byte
// bodySHA1 field, a SHA1WithRSA signature over the raw (uncompressed) body. The
// key may be a PEM "PUBLIC KEY" block, a base64-encoded SPKI string (the form
// of the bundled pkg/types platform keys), or raw SPKI DER; see ParsePublicKey.
//
// Verification is off by default: the Gateway runs on loopback and is the
// trusted transport (docs/adr/0005-key-model-and-push-verification.md). When
// required is true a frame whose signature is missing or invalid is dropped and
// the error is reported on Errors; when false the frame is still delivered so
// callers can observe the failure without losing data.
//
// A key that cannot be parsed is reported by Connect rather than by New, which
// never fails.
func WithVerification(pubKeyPEMorSPKI []byte, required bool) Option {
	return func(o *Options) {
		o.VerifyPublicKey = pubKeyPEMorSPKI
		o.VerifyRequired = required
	}
}

// Notification is one decoded push message. Payload is the concrete generated
// protobuf pointer selected by Type (for example *hqnotify.BasicQotNotify), or
// nil when the frame carried no Any payload. It is never a wire buffer.
type Notification struct {
	// Type is the PBNotify notifyMsgType discriminator.
	Type types.NotifyMsgType
	// ID is the PBNotify notifyId (for market pushes this is the security
	// code).
	ID string
	// Time is the PBNotify notifyTime as sent on the wire, in milliseconds
	// since the Unix epoch. It is 0 when unset.
	Time uint64
	// Payload is the decoded, typed notification body.
	Payload any
}

// dispatcher owns the queue and goroutine that invokes one registered handler.
type dispatcher struct {
	ch   chan *Notification
	stop chan struct{}
	once sync.Once
}

// close stops the dispatcher goroutine exactly once.
func (d *dispatcher) close() {
	d.once.Do(func() { close(d.stop) })
}

// Client maintains a TCP push connection to the Gateway, decodes push frames,
// and dispatches typed notifications to registered handlers. It reconnects
// with exponential backoff and invokes the OnReconnect hooks after each
// successful reconnect so callers can restore their subscriptions.
//
// A Client is safe for concurrent use. Connect dials, Run reads and
// reconnects, and Close releases the connection, stops the read loop, and stops
// every dispatcher goroutine. Connect and Run are typically driven from one
// goroutine each.
type Client struct {
	opts Options

	closed    atomic.Bool
	closeOnce sync.Once
	done      chan struct{}

	mu       sync.Mutex
	conn     net.Conn
	handlers map[types.NotifyMsgType]*dispatcher

	errc   chan error
	errcMu sync.Mutex

	onReconnect []func(error)

	verifier *Verifier
	initErr  error
}

// New returns a Client configured by opts. It never returns nil and never
// dials; call Connect to establish the connection. Invalid durations are
// normalized to their defaults.
func New(opts ...Option) *Client {
	o := Options{
		Addr:         DefaultAddr,
		MinReconnect: DefaultMinReconnect,
		MaxReconnect: DefaultMaxReconnect,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(&o)
		}
	}
	if o.Dial == nil {
		d := &net.Dialer{}
		o.Dial = d.DialContext
	}
	if o.Addr == "" {
		o.Addr = DefaultAddr
	}
	if o.MinReconnect <= 0 {
		o.MinReconnect = DefaultMinReconnect
	}
	if o.MaxReconnect < o.MinReconnect {
		o.MaxReconnect = o.MinReconnect
	}

	var verifier *Verifier
	var initErr error
	if len(o.VerifyPublicKey) > 0 {
		key, err := ParsePublicKey(o.VerifyPublicKey)
		if err != nil {
			initErr = err
		} else {
			verifier = NewVerifier(key, o.VerifyRequired)
		}
	}

	return &Client{
		opts:        o,
		done:        make(chan struct{}),
		handlers:    make(map[types.NotifyMsgType]*dispatcher),
		errc:        make(chan error, errorBuffer),
		onReconnect: o.onReconnect,
		verifier:    verifier,
		initErr:     initErr,
	}
}

// Addr returns the resolved push address.
func (c *Client) Addr() string { return c.opts.Addr }

// Errors returns the channel on which transport and decode errors are
// surfaced. Sends are non-blocking with a drop-oldest policy, so a slow reader
// never stalls the read loop. The channel is closed by Close.
func (c *Client) Errors() <-chan error { return c.errc }

// Connect dials the Gateway and stores the connection. It returns ErrClosed
// after Close, and is idempotent while already connected. The supplied context
// bounds only the dial; connection lifetime is governed by Run and Close.
func (c *Client) Connect(ctx context.Context) error {
	if c.closed.Load() {
		return ErrClosed
	}
	if c.initErr != nil {
		return c.initErr
	}
	c.mu.Lock()
	if c.conn != nil {
		c.mu.Unlock()
		return nil
	}
	c.mu.Unlock()

	conn, err := c.dial(ctx)
	if err != nil {
		return err
	}
	c.mu.Lock()
	if c.closed.Load() {
		c.mu.Unlock()
		_ = conn.Close()
		return ErrClosed
	}
	if c.conn != nil {
		c.mu.Unlock()
		_ = conn.Close()
		return nil
	}
	c.conn = conn
	c.mu.Unlock()
	return nil
}

// Run reads frames until ctx is cancelled or Close is called, dispatching
// decoded notifications and reconnecting with backoff after a read failure.
// After each successful reconnect it invokes the OnReconnect hooks with the
// error that ended the previous connection. Run returns ctx.Err() on
// cancellation, ErrClosed after Close, or nil if the client was closed while
// the read loop was unwinding. Run must be called after Connect, and it is
// normally run in its own goroutine.
func (c *Client) Run(ctx context.Context) error {
	if c.initErr != nil {
		return c.initErr
	}
	runDone := make(chan struct{})
	defer close(runDone)
	go func() {
		select {
		case <-ctx.Done():
			c.closeConn()
		case <-c.done:
		case <-runDone:
		}
	}()

	backoff := c.opts.MinReconnect
	for {
		if c.closed.Load() {
			return ErrClosed
		}
		if err := ctx.Err(); err != nil {
			return err
		}

		conn := c.currentConn()
		if conn == nil {
			nc, err := c.dial(ctx)
			if err != nil {
				c.report(err)
				if !sleepCtx(ctx, backoff) {
					return c.exitCause(ctx)
				}
				backoff = nextBackoff(backoff, c.opts.MaxReconnect)
				continue
			}
			c.setConn(nc)
			backoff = c.opts.MinReconnect
			continue
		}

		err := c.readLoop(conn)
		c.closeConn()
		if c.closed.Load() {
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		c.report(err)

		nc, derr := c.dial(ctx)
		for derr != nil {
			if !sleepCtx(ctx, backoff) {
				return c.exitCause(ctx)
			}
			backoff = nextBackoff(backoff, c.opts.MaxReconnect)
			if c.closed.Load() {
				return nil
			}
			nc, derr = c.dial(ctx)
		}
		c.setConn(nc)
		backoff = c.opts.MinReconnect
		c.fireOnReconnect(err)
	}
}

// exitCause returns the reason Run is stopping, preferring ErrClosed over ctx.
func (c *Client) exitCause(ctx context.Context) error {
	if c.closed.Load() {
		return ErrClosed
	}
	return ctx.Err()
}

// readLoop reads and dispatches frames from conn until an error occurs.
func (c *Client) readLoop(conn net.Conn) error {
	for {
		h, body, err := ReadFrame(conn)
		if err != nil {
			return err
		}
		switch h.MsgType {
		case MsgHeartbeat, MsgRequest, MsgResponse:
			continue
		case MsgPush:
			if verr := c.verifyFrame(h, body); verr != nil {
				c.report(verr)
				if c.opts.VerifyRequired {
					continue
				}
			}
			n, err := decodeNotification(body)
			if err != nil {
				c.report(err)
				continue
			}
			c.dispatch(n)
		default:
			continue
		}
	}
}

// SubscribeTypes registers h as the handler for notifications of type t. A nil
// handler deregisters. Registering a second handler for the same type replaces
// the first and stops its dispatcher. Handlers are invoked one at a time per
// type from a dedicated goroutine; the read loop hands work off through a
// buffered queue and drops the oldest notification when the queue is full.
func (c *Client) SubscribeTypes(t types.NotifyMsgType, h func(*Notification)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed.Load() {
		return
	}
	if old, ok := c.handlers[t]; ok {
		old.close()
		delete(c.handlers, t)
	}
	if h == nil {
		return
	}
	d := &dispatcher{ch: make(chan *Notification, dispatchBuffer), stop: make(chan struct{})}
	c.handlers[t] = d
	go func() {
		for {
			select {
			case <-d.stop:
				return
			case n := <-d.ch:
				h(n)
			}
		}
	}()
}

// UnsubscribeTypes deregisters the handler for t, if any, and stops its
// dispatcher goroutine.
func (c *Client) UnsubscribeTypes(t types.NotifyMsgType) {
	c.SubscribeTypes(t, nil)
}

// dispatch enqueues n for the handler registered for n.Type. It never blocks:
// when the handler queue is full the oldest entry is discarded.
func (c *Client) dispatch(n *Notification) {
	if n == nil {
		return
	}
	c.mu.Lock()
	d := c.handlers[n.Type]
	c.mu.Unlock()
	if d == nil {
		return
	}
	select {
	case d.ch <- n:
	default:
		select {
		case <-d.ch:
		default:
		}
		select {
		case d.ch <- n:
		default:
		}
	}
}

// report surfaces err on the error channel, dropping the oldest error when the
// channel is full. It never blocks, and it is a no-op after Close.
func (c *Client) report(err error) {
	if err == nil {
		return
	}
	c.errcMu.Lock()
	defer c.errcMu.Unlock()
	if c.closed.Load() {
		return
	}
	select {
	case c.errc <- err:
	default:
		select {
		case <-c.errc:
		default:
		}
		select {
		case c.errc <- err:
		default:
		}
	}
}

// fireOnReconnect invokes every registered reconnect hook with cause.
func (c *Client) fireOnReconnect(cause error) {
	for _, fn := range c.onReconnect {
		fn(cause)
	}
}

// dial opens a connection using the configured dialer.
func (c *Client) dial(ctx context.Context) (net.Conn, error) {
	return c.opts.Dial(ctx, "tcp", c.opts.Addr)
}

// currentConn returns the active connection, or nil.
func (c *Client) currentConn() net.Conn {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.conn
}

// setConn stores conn as the active connection.
func (c *Client) setConn(conn net.Conn) {
	c.mu.Lock()
	c.conn = conn
	c.mu.Unlock()
}

// closeConn closes and clears the active connection, if any.
func (c *Client) closeConn() {
	c.mu.Lock()
	conn := c.conn
	c.conn = nil
	c.mu.Unlock()
	if conn != nil {
		_ = conn.Close()
	}
}

// Close stops the read loop, closes the connection and the error channel, and
// stops every dispatcher goroutine. It is idempotent and never panics. After
// Close, Connect and Run return ErrClosed and SubscribeTypes is a no-op.
func (c *Client) Close() error {
	c.closeOnce.Do(func() {
		c.closed.Store(true)
		close(c.done)
		c.mu.Lock()
		conn := c.conn
		c.conn = nil
		dispatchers := make([]*dispatcher, 0, len(c.handlers))
		for _, d := range c.handlers {
			dispatchers = append(dispatchers, d)
		}
		c.handlers = make(map[types.NotifyMsgType]*dispatcher)
		c.mu.Unlock()

		if conn != nil {
			_ = conn.Close()
		}
		for _, d := range dispatchers {
			d.close()
		}
		c.errcMu.Lock()
		close(c.errc)
		c.errcMu.Unlock()
	})
	return nil
}

// decodeNotification unmarshals a push body into a PBNotify and decodes its Any
// payload by notifyMsgType. A frame with no payload yields a Notification with
// a nil Payload.
func decodeNotification(body []byte) (*Notification, error) {
	var pb pbmsg.PBNotify
	if err := proto.Unmarshal(body, &pb); err != nil {
		return nil, fmt.Errorf("push: decode PBNotify: %w", err)
	}
	t := types.NotifyMsgType(pb.GetNotifyMsgType())
	n := &Notification{Type: t, ID: pb.GetNotifyId(), Time: pb.GetNotifyTime()}
	if p := pb.GetPayload(); p != nil {
		decoded, err := Decode(t, p.GetValue())
		if err != nil {
			return nil, err
		}
		n.Payload = decoded
	}
	return n, nil
}

// Decode unmarshals a PBNotify Any payload into the concrete generated message
// selected by notifyMsgType and returns it as an any. The Any's type_url is
// deliberately ignored: the vendored protos declare no proto package, so this
// package dispatches on the wire enum instead of the global registry (see the
// package doc).
//
// The recognized types are:
//
//	TradeStockDeliverMsgType (1), FuturesTradeStockDeliverMsgType (2),
//	TrsStockDeliverMsgType (0) -> *tradenotify.TradeStockDeliverNotify
//	OrderBookNotifyMsgType (20001)  -> *hqnotify.OrderBookFullNotify
//	BrokerQueueNotifyMsgType (20002) -> *hqnotify.BrokerNotify
//	BasicQotNotifyMsgType (20003)   -> *hqnotify.BasicQotNotify
//	TickerNotifyMsgType (20004)     -> *hqnotify.TickerNotify
//
// An empty payload unmarshals to an empty message of the selected type; an
// unknown notifyMsgType is an error. Decode returns a pointer to a freshly
// allocated message and the caller owns it.
func Decode(notifyMsgType types.NotifyMsgType, payload []byte) (any, error) {
	var m proto.Message
	switch notifyMsgType {
	case types.TrsStockDeliverMsgType,
		types.TradeStockDeliverMsgType,
		types.FuturesTradeStockDeliverMsgType:
		m = &tradenotify.TradeStockDeliverNotify{}
	case types.OrderBookNotifyMsgType:
		m = &hqnotify.OrderBookFullNotify{}
	case types.BrokerQueueNotifyMsgType:
		m = &hqnotify.BrokerNotify{}
	case types.BasicQotNotifyMsgType:
		m = &hqnotify.BasicQotNotify{}
	case types.TickerNotifyMsgType:
		m = &hqnotify.TickerNotify{}
	default:
		return nil, fmt.Errorf("push: unknown notifyMsgType %d", int32(notifyMsgType))
	}
	if err := proto.Unmarshal(payload, m); err != nil {
		return nil, fmt.Errorf("push: decode %T: %w", m, err)
	}
	return m, nil
}

// sleepCtx sleeps for d, returning false if ctx is cancelled first.
func sleepCtx(ctx context.Context, d time.Duration) bool {
	if d <= 0 {
		return ctx.Err() == nil
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return true
	case <-ctx.Done():
		return false
	}
}

// nextBackoff doubles cur up to max and adds jitter in [0, cur). The jitter
// spreads reconnects from a fleet of clients.
func nextBackoff(cur, max time.Duration) time.Duration {
	next := cur * 2
	if next > max {
		next = max
	}
	if next <= 0 {
		return cur
	}
	jitter := time.Duration(rand.Int63n(int64(next))) // #nosec G404 -- reconnect backoff jitter only; not a security-sensitive random value
	return next/2 + jitter/2
}
