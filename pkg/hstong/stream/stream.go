// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package stream

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/shing1211/hstongapi4go/client"
	"github.com/shing1211/hstongapi4go/gen/hq/dto"
	hqnotify "github.com/shing1211/hstongapi4go/gen/hq/notify"
	tradenotify "github.com/shing1211/hstongapi4go/gen/trade/notify"
	"github.com/shing1211/hstongapi4go/internal/errs"
	"github.com/shing1211/hstongapi4go/internal/push"
	"github.com/shing1211/hstongapi4go/pkg/hstong/market"
	"github.com/shing1211/hstongapi4go/pkg/types"
)

// Sentinel errors returned by the stream client.
var (
	// ErrClosed is returned by Connect and Subscribe after Close.
	ErrClosed = errors.New("stream: client is closed")
	// ErrNoClient is returned when the stream client was built without a
	// non-nil *client.Client.
	ErrNoClient = errors.New("stream: no underlying client")
	// ErrReconnected is pushed on a Subscription's Errors channel after the
	// push connection is re-established and its subscriptions restored. The
	// error that ended the previous connection is wrapped and available with
	// errors.Unwrap.
	ErrReconnected = errors.New("stream: push connection re-established")
)

// defaultBuffer is the default Updates/Errors channel depth. It is overridden
// by WithBuffer.
const defaultBuffer = 64

// resubscribeTimeout bounds a single automatic re-subscribe call after a
// reconnect; the run context may already be cancelled, so it is derived from a
// background context.
const resubscribeTimeout = 10 * time.Second

// marketNotifyTypes are the market push message types the stream client
// registers decoders for. Trade and futures delivery subscriptions (P06/P07)
// add their own.
var marketNotifyTypes = []types.NotifyMsgType{
	types.BasicQotNotifyMsgType,
	types.TickerNotifyMsgType,
	types.BrokerQueueNotifyMsgType,
	types.OrderBookNotifyMsgType,
}

// options is the resolved stream configuration.
type options struct {
	pushAddr     string
	dial         push.DialFunc
	minReconnect time.Duration
	maxReconnect time.Duration
	buffer       int
	trade        TradePusher
}

// Option mutates the stream configuration. Options are applied in order by New.
type Option func(*options)

// WithPushAddr overrides the Gateway TCP push address. The default is the
// address carried by the *client.Client (client.WithPushAddr); an empty value
// leaves that default in place.
func WithPushAddr(addr string) Option {
	return func(o *options) { o.pushAddr = addr }
}

// WithDialer replaces the dialer used for the push connection. It is primarily
// useful in tests that need to redirect reconnects to a listener on another
// port.
func WithDialer(d push.DialFunc) Option {
	return func(o *options) { o.dial = d }
}

// WithReconnect sets the push reconnect backoff bounds. Non-positive values
// leave the push package defaults in place.
func WithReconnect(min, max time.Duration) Option {
	return func(o *options) {
		if min > 0 {
			o.minReconnect = min
		}
		if max > 0 {
			o.maxReconnect = max
		}
	}
}

// WithBuffer sets the depth of every Subscription's Updates and Errors channel.
// A non-positive value restores the default of 64.
func WithBuffer(n int) Option {
	return func(o *options) {
		if n > 0 {
			o.buffer = n
		}
	}
}

// Client is the public push API. It owns a push.Client, performs HTTP
// subscribe/unsubscribe through a market.Manager, and fans decoded events out
// to Subscriptions. It is safe for concurrent use.
//
// A Client must be connected with Connect before subscribing. Close releases
// the push connection and stops every goroutine; it does not close the
// caller-owned *client.Client.
type Client struct {
	client *client.Client
	market *market.Manager
	trade  TradePusher
	opts   options
	push   *push.Client

	mu        sync.Mutex
	subs      map[*Subscription]struct{}
	tradeSubs int
	started   bool
	closed    bool
	closeOnce sync.Once
	done      chan struct{}

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// New returns a stream Client over c. c must be non-nil and is not owned by the
// stream client: the caller remains responsible for closing it. The push
// address defaults to c.PushAddr and may be overridden with WithPushAddr.
func New(c *client.Client, opts ...Option) *Client {
	o := options{buffer: defaultBuffer}
	for _, opt := range opts {
		if opt != nil {
			opt(&o)
		}
	}
	if o.pushAddr == "" && c != nil {
		o.pushAddr = c.PushAddr()
	}
	if o.pushAddr == "" {
		o.pushAddr = push.DefaultAddr
	}
	if o.buffer <= 0 {
		o.buffer = defaultBuffer
	}

	s := &Client{
		client: c,
		trade:  o.trade,
		opts:   o,
		subs:   make(map[*Subscription]struct{}),
		done:   make(chan struct{}),
	}
	if c != nil {
		s.market = market.New(c)
	}

	pushOpts := []push.Option{
		push.WithAddr(o.pushAddr),
		push.WithReconnect(o.minReconnect, o.maxReconnect),
		push.WithOnReconnect(s.onReconnect),
	}
	if o.dial != nil {
		pushOpts = append(pushOpts, push.WithDialer(o.dial))
	}
	if c != nil && c.VerifyPush() && c.PlatformPublicKey() != "" {
		pushOpts = append(pushOpts, push.WithVerification([]byte(c.PlatformPublicKey()), true))
	}
	s.push = push.New(pushOpts...)
	for _, t := range marketNotifyTypes {
		s.push.SubscribeTypes(t, s.deliver)
	}
	for _, t := range tradeNotifyTypes {
		s.push.SubscribeTypes(t, s.deliver)
	}
	return s
}

// Connect dials the Gateway push channel and starts the read/reconnect loop.
// The connection lives until Close or until ctx is cancelled, whichever comes
// first. Connect is idempotent while already connected and returns ErrClosed
// after Close. The context bounds the dial and the connection lifetime; it is
// not used for the later HTTP subscribe calls.
func (s *Client) Connect(ctx context.Context) error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return ErrClosed
	}
	if s.started {
		s.mu.Unlock()
		return nil
	}
	if s.client == nil {
		s.mu.Unlock()
		return ErrNoClient
	}
	s.ctx, s.cancel = context.WithCancel(ctx)
	runCtx := s.ctx
	s.mu.Unlock()

	if err := s.push.Connect(runCtx); err != nil {
		if s.cancel != nil {
			s.cancel()
		}
		return err
	}

	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return ErrClosed
	}
	s.started = true
	s.wg.Add(3)
	s.mu.Unlock()

	go func() {
		defer s.wg.Done()
		_ = s.push.Run(runCtx)
	}()
	go func() {
		defer s.wg.Done()
		select {
		case <-runCtx.Done():
			_ = s.shutdown()
		case <-s.done:
		}
	}()
	go func() {
		defer s.wg.Done()
		s.forwardPushErrors(runCtx)
	}()
	return nil
}

// forwardPushErrors drains the push client's error channel and surfaces
// signature-verification failures on every active subscription. Other push
// errors are left untouched: reconnect notices are emitted by onReconnect, and
// the stream API has never surfaced transport read errors, so forwarding them
// here would be a behaviour change. The loop ends when ctx is cancelled or the
// push error channel is closed by Close.
func (s *Client) forwardPushErrors(ctx context.Context) {
	for {
		select {
		case err, ok := <-s.push.Errors():
			if !ok {
				return
			}
			if isVerificationError(err) {
				s.broadcastError(err)
			}
		case <-ctx.Done():
			return
		}
	}
}

// isVerificationError reports whether err is one of the push verifier's typed
// failures.
func isVerificationError(err error) bool {
	return errors.Is(err, push.ErrSignatureMismatch) ||
		errors.Is(err, push.ErrMissingSignature) ||
		errors.Is(err, push.ErrNoPublicKey)
}

// broadcastError enqueues err on every active subscription's Errors channel.
func (s *Client) broadcastError(err error) {
	s.mu.Lock()
	subs := make([]*Subscription, 0, len(s.subs))
	for sub := range s.subs {
		subs = append(subs, sub)
	}
	s.mu.Unlock()
	for _, sub := range subs {
		sub.sendErr(err)
	}
}

// Subscribe performs the HTTP /hq/Subscribe call for topicID and registers a
// local subscription that receives matching push events. It rejects an unknown
// topic or an empty (or nil-containing) security list with a typed
// StatusInvalidParam error before any request is sent. The returned
// Subscription is owned by the caller and must be released with Cancel.
func (s *Client) Subscribe(ctx context.Context, topicID types.TopicID, securities ...*dto.Security) (*Subscription, error) {
	if s.client == nil {
		return nil, ErrNoClient
	}
	notifyType, ok := notifyTypeForTopic(topicID)
	if !ok {
		return nil, errs.New(types.StatusInvalidParam, "stream.Subscribe",
			fmt.Sprintf("unknown topicId %d", int(topicID)))
	}
	if len(securities) == 0 {
		return nil, errs.New(types.StatusInvalidParam, "stream.Subscribe", "security list must not be empty")
	}
	s.mu.Lock()
	closed := s.closed
	s.mu.Unlock()
	if closed {
		return nil, ErrClosed
	}

	if err := s.market.Subscribe(ctx, topicID, securities...); err != nil {
		return nil, err
	}

	sub := &Subscription{
		client:     s,
		topicID:    topicID,
		securities: append([]*dto.Security(nil), securities...),
		accepts:    []types.NotifyMsgType{notifyType},
		updates:    make(chan Event, s.opts.buffer),
		errs:       make(chan error, s.opts.buffer),
	}

	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil, ErrClosed
	}
	s.subs[sub] = struct{}{}
	s.mu.Unlock()
	return sub, nil
}

// Close cancels the connection, stops the read loop and every dispatcher, and
// closes every Subscription channel. It is idempotent and waits for all
// package goroutines to exit. It does not close the caller-owned
// *client.Client.
func (s *Client) Close() error {
	err := s.shutdown()
	s.wg.Wait()
	return err
}

// shutdown performs the one-time teardown. It must be safe to call from the
// connection-lifetime watcher goroutine as well as from Close.
func (s *Client) shutdown() error {
	s.closeOnce.Do(func() {
		s.mu.Lock()
		s.closed = true
		cancel := s.cancel
		subs := make([]*Subscription, 0, len(s.subs))
		for sub := range s.subs {
			subs = append(subs, sub)
		}
		s.subs = make(map[*Subscription]struct{})
		s.tradeSubs = 0
		s.mu.Unlock()

		if cancel != nil {
			cancel()
		}
		_ = s.push.Close()
		close(s.done)
		for _, sub := range subs {
			sub.close()
		}
	})
	return nil
}

// deliver fans a decoded push notification out to every active subscription
// whose topic maps to the notification's type. Sends are non-blocking.
func (s *Client) deliver(n *push.Notification) {
	if n == nil {
		return
	}
	s.mu.Lock()
	subs := make([]*Subscription, 0, len(s.subs))
	for sub := range s.subs {
		subs = append(subs, sub)
	}
	s.mu.Unlock()
	if len(subs) == 0 {
		return
	}

	ev := Event{Type: n.Type, ID: n.ID, Time: notifyTime(n.Time), Payload: n.Payload}
	for _, sub := range subs {
		if sub.acceptsType(n.Type) {
			sub.sendUpdate(ev)
		}
	}
}

// onReconnect restores every active subscription after a push reconnect. It
// pushes an ErrReconnected notice on each Errors channel, then re-issues the
// HTTP subscribe, reporting a failure on the same channel.
func (s *Client) onReconnect(cause error) {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	subs := make([]*Subscription, 0, len(s.subs))
	for sub := range s.subs {
		subs = append(subs, sub)
	}
	trade := s.trade
	tradeActive := s.tradeSubs > 0
	s.mu.Unlock()

	for _, sub := range subs {
		sub.sendErr(fmt.Errorf("%w: %v", ErrReconnected, cause))
	}
	for _, sub := range subs {
		if sub.trade {
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), resubscribeTimeout)
		err := s.market.Subscribe(ctx, sub.topicID, sub.securities...)
		cancel()
		if err != nil {
			sub.sendErr(fmt.Errorf("stream: resubscribe %s: %w", sub.topicID, err))
		}
	}
	if tradeActive && trade != nil {
		ctx, cancel := context.WithTimeout(context.Background(), resubscribeTimeout)
		err := trade.SubscribeOrders(ctx)
		cancel()
		if err != nil {
			for _, sub := range subs {
				if sub.trade {
					sub.sendErr(fmt.Errorf("stream: resubscribe trade push: %w", err))
				}
			}
		}
	}
}

// Subscription is one active market subscription. Updates and Errors are
// buffered with a drop-oldest policy; TopicID reports the topic; Cancel
// performs the HTTP unsubscribe and releases the local channels.
type Subscription struct {
	client     *Client
	topicID    types.TopicID
	securities []*dto.Security
	trade      bool
	accepts    []types.NotifyMsgType

	updates chan Event
	errs    chan error

	mu        sync.Mutex
	closed    bool
	closeOnce sync.Once
}

// Updates returns the channel of decoded push events. The channel is closed by
// Cancel, by Client.Close, or when the stream client shuts down.
func (sub *Subscription) Updates() <-chan Event { return sub.updates }

// Errors returns the channel of subscription-level errors, including the
// ErrReconnected notice emitted after a reconnect and any re-subscribe failure.
// The channel is closed by Cancel, by Client.Close, or when the stream client
// shuts down.
func (sub *Subscription) Errors() <-chan error { return sub.errs }

// TopicID returns the market push topic of the subscription. A trade
// subscription created by SubscribeTrade has no market topic and returns 0.
func (sub *Subscription) TopicID() types.TopicID { return sub.topicID }

// Securities returns a copy of the subscribed instruments. A trade
// subscription created by SubscribeTrade carries none and returns nil. Mutating
// the returned slice does not affect the subscription.
func (sub *Subscription) Securities() []*dto.Security {
	out := make([]*dto.Security, len(sub.securities))
	copy(out, sub.securities)
	return out
}

// acceptsType reports whether the subscription wants events of type t.
func (sub *Subscription) acceptsType(t types.NotifyMsgType) bool {
	for _, a := range sub.accepts {
		if a == t {
			return true
		}
	}
	return false
}

// Cancel deregisters the subscription and closes its Updates and Errors
// channels. For a market subscription it performs the HTTP /hq/Unsubscribe call;
// for a trade subscription created by SubscribeTrade it performs
// /trade/TradeUnsubscribe when a trade manager is configured and this is the
// last active trade subscription. The channels are closed even when the HTTP
// call fails, so the error is returned but does not leave the subscription
// live. Cancel is idempotent.
func (sub *Subscription) Cancel(ctx context.Context) error {
	c := sub.client
	c.mu.Lock()
	_, active := c.subs[sub]
	delete(c.subs, sub)
	closed := c.closed
	trade := c.trade
	lastTrade := false
	if active && sub.trade {
		c.tradeSubs--
		if c.tradeSubs <= 0 {
			c.tradeSubs = 0
			lastTrade = true
		}
	}
	c.mu.Unlock()

	var err error
	if active && !closed {
		switch {
		case sub.trade:
			if lastTrade && trade != nil {
				err = trade.UnsubscribeOrders(ctx)
			}
		case c.market != nil:
			err = c.market.Unsubscribe(ctx, sub.topicID, sub.securities...)
		}
	}
	sub.close()
	return err
}

// sendUpdate enqueues e with a drop-oldest policy; it is a no-op after close.
func (sub *Subscription) sendUpdate(e Event) {
	sub.mu.Lock()
	defer sub.mu.Unlock()
	if sub.closed {
		return
	}
	select {
	case sub.updates <- e:
	default:
		select {
		case <-sub.updates:
		default:
		}
		select {
		case sub.updates <- e:
		default:
		}
	}
}

// sendErr enqueues err with a drop-oldest policy; it is a no-op after close.
func (sub *Subscription) sendErr(err error) {
	if err == nil {
		return
	}
	sub.mu.Lock()
	defer sub.mu.Unlock()
	if sub.closed {
		return
	}
	select {
	case sub.errs <- err:
	default:
		select {
		case <-sub.errs:
		default:
		}
		select {
		case sub.errs <- err:
		default:
		}
	}
}

// close closes both channels exactly once. It is safe to call concurrently with
// sendUpdate and sendErr.
func (sub *Subscription) close() {
	sub.closeOnce.Do(func() {
		sub.mu.Lock()
		sub.closed = true
		close(sub.updates)
		close(sub.errs)
		sub.mu.Unlock()
	})
}

// notifyTypeForTopic maps a market push topic to the notifyMsgType of its
// payload. The mapping follows docs/SPEC.md §3.
func notifyTypeForTopic(t types.TopicID) (types.NotifyMsgType, bool) {
	switch t {
	case types.TopicBasicQot, types.TopicQuoteVariant35:
		return types.BasicQotNotifyMsgType, true
	case types.TopicTicker, types.TopicTickVariant27, types.TopicTickVariant28, types.TopicTickVariant37:
		return types.TickerNotifyMsgType, true
	case types.TopicBroker:
		return types.BrokerQueueNotifyMsgType, true
	case types.TopicOrderBook, types.TopicOrderBookArcabook, types.TopicOrderBookTotalView, types.TopicOrderBookVariant36:
		return types.OrderBookNotifyMsgType, true
	default:
		return 0, false
	}
}

// notifyTime converts a wire notifyTime (milliseconds since the Unix epoch) to
// a UTC time.Time. A zero value maps to the zero time rather than 1970.
func notifyTime(ms uint64) time.Time {
	if ms == 0 {
		return time.Time{}
	}
	return time.UnixMilli(int64(ms)).UTC() // #nosec G115 -- Gateway notifyTime; a wrapped value yields an implausible timestamp, not an unsafe one
}

// Event is one decoded market push message delivered on a Subscription's
// Updates channel. Payload is the concrete generated protobuf pointer selected
// by Type; use the typed accessors (BasicQot, Ticker, OrderBook, Broker,
// TradeDeliver) to obtain it safely.
type Event struct {
	// Type is the PBNotify notifyMsgType discriminator.
	Type types.NotifyMsgType
	// ID is the notification id; for market pushes it is the security code.
	ID string
	// Time is the notification time in UTC. It is the zero time when the
	// Gateway sent no notifyTime.
	Time time.Time
	// Payload is the decoded, typed notification body.
	Payload any
}

// BasicQot returns the BasicQotNotify payload of a basic-quote event, and
// reports whether the event carried one.
func (e Event) BasicQot() (*hqnotify.BasicQotNotify, bool) {
	m, ok := e.Payload.(*hqnotify.BasicQotNotify)
	return m, ok
}

// Ticker returns the TickerNotify payload of a tick event, and reports whether
// the event carried one.
func (e Event) Ticker() (*hqnotify.TickerNotify, bool) {
	m, ok := e.Payload.(*hqnotify.TickerNotify)
	return m, ok
}

// OrderBook returns the OrderBookFullNotify payload of an order-book event, and
// reports whether the event carried one.
func (e Event) OrderBook() (*hqnotify.OrderBookFullNotify, bool) {
	m, ok := e.Payload.(*hqnotify.OrderBookFullNotify)
	return m, ok
}

// Broker returns the BrokerNotify payload of a broker-queue event, and reports
// whether the event carried one.
func (e Event) Broker() (*hqnotify.BrokerNotify, bool) {
	m, ok := e.Payload.(*hqnotify.BrokerNotify)
	return m, ok
}

// TradeDeliver returns the TradeStockDeliverNotify payload of a stock
// trade-delivery event, and reports whether the event carried one. It accepts
// notify types TradeStockDeliverMsgType (1) and the TrsStockDeliverMsgType (0)
// variant; use FuturesTradeDeliver for a futures trade delivery (type 2).
func (e Event) TradeDeliver() (*tradenotify.TradeStockDeliverNotify, bool) {
	if e.Type != types.TradeStockDeliverMsgType && e.Type != types.TrsStockDeliverMsgType {
		return nil, false
	}
	m, ok := e.Payload.(*tradenotify.TradeStockDeliverNotify)
	return m, ok
}
