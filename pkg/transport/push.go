// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package transport

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/shing1211/hstongapi4go/internal/push"
	"github.com/shing1211/hstongapi4go/pkg/domain"
	"github.com/shing1211/hstongapi4go/pkg/types"
)

// Defaults applied by NewPushAdapter when no option overrides them.
const (
	// DefaultPushAddr is the local Gateway TCP push address.
	DefaultPushAddr = push.DefaultAddr
	// DefaultPushMinReconnect is the initial reconnect delay.
	DefaultPushMinReconnect = push.DefaultMinReconnect
	// DefaultPushMaxReconnect is the ceiling on the reconnect delay.
	DefaultPushMaxReconnect = push.DefaultMaxReconnect
	// DefaultPushErrorBuffer is the depth of this package's error stream. It is
	// bounded and drop-oldest: a lost diagnosis is recoverable, and an unbounded
	// queue on a flapping connection is a memory leak.
	DefaultPushErrorBuffer = 64
)

// DialFunc dials the Gateway push address. It has the signature of
// (*net.Dialer).DialContext and is substitutable so a caller - or a test - can
// route the connection somewhere other than loopback.
type DialFunc func(ctx context.Context, network, address string) (net.Conn, error)

// PushConfig is the resolved configuration of a PushAdapter. Callers build it
// through the With... options passed to NewPushAdapter rather than by writing a
// literal.
//
// It carries no field for key material. WithVerification takes the platform
// public key as a parameter and keeps it in an unexported field, so no code
// outside this package - and no struct literal anywhere - can hold a copy of it
// by assignment. That is the whole reason it is not a field: a public key is
// reference data and not a secret (docs/adr/0005-key-model-and-push-verification.md),
// but a struct field is a place for a private key to be put by the next change,
// and gosec and TestPushAdapterDeclaresNoCredentialStorage exist to stop exactly
// that. There is no way to put one here.
type PushConfig struct {
	// Addr is the Gateway push address in "host:port" form. It defaults to
	// DefaultPushAddr.
	Addr string
	// Dial dials Addr. A nil value restores a fresh net.Dialer.
	Dial DialFunc
	// MinReconnect is the initial reconnect delay, and MaxReconnect its ceiling.
	// Non-positive values restore the defaults.
	MinReconnect time.Duration
	MaxReconnect time.Duration
	// ReadDeadline bounds a single frame read. It defaults to 0, which disables
	// the deadline: the Gateway documents no heartbeat cadence on this stream, so
	// a non-zero default could tear down a healthy connection during a quiet
	// market. See R3 in docs/threat-model.md.
	ReadDeadline time.Duration

	verifyKey      []byte
	verifyRequired bool
}

// PushOption mutates a PushConfig. Options are applied in order by
// NewPushAdapter, so the last one that sets a field wins.
type PushOption func(*PushConfig)

// WithPushAddr sets the Gateway push address in "host:port" form. An empty value
// restores DefaultPushAddr.
func WithPushAddr(addr string) PushOption {
	return func(c *PushConfig) {
		if addr == "" {
			c.Addr = DefaultPushAddr
			return
		}
		c.Addr = addr
	}
}

// WithPushDialer replaces the dialer used to establish the push connection. A nil
// value restores the default.
func WithPushDialer(d DialFunc) PushOption {
	return func(c *PushConfig) { c.Dial = d }
}

// WithPushReconnect sets the reconnect backoff bounds. Non-positive values
// restore the defaults and a max below min is raised to min.
func WithPushReconnect(min, max time.Duration) PushOption {
	return func(c *PushConfig) {
		if min > 0 {
			c.MinReconnect = min
		}
		if max > 0 {
			c.MaxReconnect = max
		}
	}
}

// WithPushReadDeadline bounds how long a single frame read may block, so a
// silently dead peer is detected instead of parking the read goroutine until the
// OS gives up. A non-positive value disables the deadline, which is the default;
// see PushConfig.ReadDeadline.
func WithPushReadDeadline(d time.Duration) PushOption {
	return func(c *PushConfig) {
		if d > 0 {
			c.ReadDeadline = d
			return
		}
		c.ReadDeadline = 0
	}
}

// WithPushVerification enables opt-in verification of each push frame's 128-byte
// bodySHA1 field, a SHA1WithRSA signature over the raw (uncompressed) body. The
// key may be a PEM "PUBLIC KEY" block, a base64-encoded SPKI string (the form of
// the bundled pkg/types platform keys), or raw SPKI DER; see
// push.ParsePublicKey.
//
// Verification is off by default: the Gateway runs on loopback and is the trusted
// transport (docs/adr/0005-key-model-and-push-verification.md). When required is
// true a frame whose signature is missing or invalid is dropped and reported;
// when false it is reported and still delivered, so a caller can observe the
// failure without losing data.
//
// A key that cannot be parsed is reported by Connect rather than by
// NewPushAdapter, which mirrors internal/push's own split: construction never
// fails, and the key parse is a connect-time configuration check. The key is
// retained only for the adapter's lifetime and is never written anywhere.
func WithPushVerification(pubKeyPEMorSPKI []byte, required bool) PushOption {
	return func(c *PushConfig) {
		c.verifyKey = pubKeyPEMorSPKI
		c.verifyRequired = required
	}
}

// PushAdapter is the TCP half of the push orchestration: it owns the Gateway
// push connection and turns each decoded push frame into a *domain.PushUpdate.
//
// It is the first use of internal/push in this package, which ADR 0010 rule 4
// permits and this package's doc has always reserved. It satisfies
// services.PushTransport structurally: Go interfaces are structural, so the
// adapter never has to import the package that declares the interface, and that
// is what keeps the boundary a real one rather than an import cycle - the
// arrangement client.Client already uses for services.Executor.
//
// *PushAdapter is safe for concurrent use. Close is idempotent and, as with the
// push client underneath it, the error stream is bounded and drop-oldest.
type PushAdapter struct {
	client *push.Client
	cfg    PushConfig

	// hookMu guards hooks, which the orchestrator fills after construction while
	// the push client fills nothing: the client's own hooks are an option, so the
	// adapter installs one fan-out hook at construction and keeps the list here.
	// A copy is taken before fanning out so a hook that registers another hook
	// cannot deadlock.
	hookMu sync.RWMutex
	hooks  []func(error)

	// The adapter owns its error stream rather than handing out the push client's,
	// because one translation has to happen on the way through and the push client
	// reports its own errors before any caller can see them: a verification verdict
	// is raised inside the client's read loop, so an adapter that only wrapped the
	// channel it returned would never get to look at it.
	errc      chan error
	errcMu    sync.Mutex
	done      chan struct{}
	wg        sync.WaitGroup
	closeOnce sync.Once
}

// NewPushAdapter returns an adapter over a *push.Client configured by opts. It
// never dials and never returns a nil adapter: call Connect, then Run.
//
// Configuration problems that can only be detected later - a verification key
// that will not parse - are reported by Connect. The push client never fails at
// construction, and this keeps that property, so a caller has one place to check
// for a misconfiguration rather than two.
func NewPushAdapter(opts ...PushOption) (*PushAdapter, error) {
	cfg := PushConfig{
		Addr:         DefaultPushAddr,
		MinReconnect: DefaultPushMinReconnect,
		MaxReconnect: DefaultPushMaxReconnect,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(&cfg)
		}
	}
	// MinReconnect and MaxReconnect need no post-option normalisation: WithPushReconnect
	// already ignores a non-positive bound and raises a max below its min. The defaults
	// above are the only other way in, and they are already ordered.
	if cfg.MaxReconnect < cfg.MinReconnect {
		cfg.MaxReconnect = cfg.MinReconnect
	}
	a := &PushAdapter{
		cfg:  cfg,
		errc: make(chan error, DefaultPushErrorBuffer),
		done: make(chan struct{}),
	}
	a.client = push.New(append(pushOptions(cfg), push.WithOnReconnect(a.fireReconnect))...)
	a.wg.Add(1)
	go a.translateErrors()
	return a, nil
}

// pushOptions translates the adapter's public configuration into the push
// client's own options, which is the only place the two vocabularies meet.
func pushOptions(cfg PushConfig) []push.Option {
	opts := []push.Option{
		push.WithAddr(cfg.Addr),
		push.WithReconnect(cfg.MinReconnect, cfg.MaxReconnect),
		push.WithReadDeadline(cfg.ReadDeadline),
	}
	if cfg.Dial != nil {
		opts = append(opts, push.WithDialer(push.DialFunc(cfg.Dial)))
	}
	if len(cfg.verifyKey) > 0 {
		opts = append(opts, push.WithVerification(cfg.verifyKey, cfg.verifyRequired))
	}
	return opts
}

// Addr returns the resolved push address.
func (a *PushAdapter) Addr() string { return a.cfg.Addr }

// Connect dials the Gateway push address. It is idempotent while already
// connected, and it is where a verification key that will not parse is reported.
func (a *PushAdapter) Connect(ctx context.Context) error { return a.client.Connect(ctx) }

// Run reads frames until ctx is cancelled or Close is called, reconnecting with
// backoff and invoking the OnReconnect hooks after each reconnect.
func (a *PushAdapter) Run(ctx context.Context) error { return a.client.Run(ctx) }

// Close releases the connection, stops the read loop, and closes the error
// stream. It is idempotent.
func (a *PushAdapter) Close() error {
	a.closeOnce.Do(func() {
		close(a.done)
		_ = a.client.Close()
		// The push client's error stream closes with the client, which ends the
		// translation goroutine; joining it before closing this package's stream is
		// what makes "no send on a closed channel" impossible rather than unlikely.
		a.wg.Wait()
		a.errcMu.Lock()
		close(a.errc)
		a.errcMu.Unlock()
	})
	return nil
}

// Errors returns the adapter's error stream: transport read errors, decode
// failures, and verification verdicts, all with their errors.Is and errors.As
// chains intact and, for a verification verdict, one extra nameable sentinel.
//
// It is bounded and drop-oldest, and closed by Close.
func (a *PushAdapter) Errors() <-chan error { return a.errc }

// translateErrors forwards the push client's error stream onto this package's,
// applying the one translation the boundary needs.
//
// It is a goroutine rather than a wrapper because the error has to be rewritten on
// the way past, and the push client raises its errors before any caller - including
// this adapter - can observe them.
func (a *PushAdapter) translateErrors() {
	defer a.wg.Done()
	upstream := a.client.Errors()
	for {
		select {
		case err, ok := <-upstream:
			if !ok {
				return
			}
			a.emit(err)
		case <-a.done:
			return
		}
	}
}

// emit places err on this package's stream, dropping the oldest when it is full.
// It never blocks, so a slow reader cannot stall the push read loop.
func (a *PushAdapter) emit(err error) {
	if err == nil {
		return
	}
	err = translatePushError(err)

	a.errcMu.Lock()
	defer a.errcMu.Unlock()
	select {
	case a.errc <- err:
		return
	default:
	}
	select {
	case <-a.errc:
	default:
	}
	select {
	case a.errc <- err:
	default:
	}
}

// translatePushError pairs the nameable domain sentinel with a signature-mismatch
// verdict, and returns everything else untouched.
//
// The verdict has to gain a second name because the one internal/push raises is
// unnameable outside this module, and a v1.0 API may not put one of its types where
// a caller has to spell it. Only that one condition is rewritten: a read error, a
// decode failure and a missing signature all keep the push package's own value and
// its text, so forwarding is verbatim for everything the caller already matched on.
func translatePushError(err error) error {
	if err == nil || !errors.Is(err, push.ErrSignatureMismatch) {
		return err
	}
	return &unverifiedError{cause: err}
}

// OnReconnect registers a hook invoked after each successful reconnect, with the
// error that ended the previous connection. Hooks run on the Run goroutine and
// must not block for long.
//
// The push client reads its hooks as a construction option, so a hook registered
// after construction would otherwise have nowhere to go. Rather than widen
// internal/push's option-only hook list for a case it does not have, the adapter
// installs one fan-out hook at construction and keeps this list itself.
func (a *PushAdapter) OnReconnect(fn func(error)) {
	if fn == nil {
		return
	}
	a.hookMu.Lock()
	a.hooks = append(a.hooks, fn)
	a.hookMu.Unlock()
}

// fireReconnect invokes every registered hook with cause. It snapshots the list
// first so a hook that registers another hook cannot deadlock on hookMu.
func (a *PushAdapter) fireReconnect(cause error) {
	a.hookMu.RLock()
	hooks := make([]func(error), len(a.hooks))
	copy(hooks, a.hooks)
	a.hookMu.RUnlock()
	for _, fn := range hooks {
		fn(cause)
	}
}

// VerificationEnabled reports whether a platform public key was configured.
func (a *PushAdapter) VerificationEnabled() bool { return a.client.VerificationEnabled() }

// VerificationRequired reports whether a verification failure must reject a
// frame. It is false when verification is disabled.
func (a *PushAdapter) VerificationRequired() bool { return a.client.VerificationRequired() }

// unverifiedError pairs the nameable domain sentinel with the push package's own
// so both stay matchable across the internal boundary.
//
// One %w cannot carry two targets and a multi-%w wrap has Unwrap() []error, which
// errors.Unwrap does not understand. An Is method that answers for the shared
// sentinel and defers to the cause for everything else keeps errors.Unwrap
// returning the real push error - errors.Is(err, push.ErrSignatureMismatch) and
// errors.Is(err, domain.ErrSignatureMismatch) are both true, and so is
// errors.Is(err, services.ErrSignatureMismatch), which is the same value as the
// domain one.
type unverifiedError struct{ cause error }

// Error renders the cause's own text, so log output is unchanged by the wrapping.
func (e *unverifiedError) Error() string { return e.cause.Error() }

// Is reports whether target is the shared sentinel or matches the cause.
func (e *unverifiedError) Is(target error) bool {
	return target == domain.ErrSignatureMismatch || errors.Is(e.cause, target)
}

// Unwrap returns the push package's own error.
func (e *unverifiedError) Unwrap() error { return e.cause }

// Report surfaces err on this package's error stream, with the same non-blocking
// drop-oldest policy the transport errors use.
//
// It exists for the errors this adapter produces and the push client has no read
// loop position for: a frame whose payload could not be normalised into a domain
// event. Without it that failure would either be swallowed - the defect class this
// repository exists to remove - or raised on a second channel a caller would have to
// know about. A nil err is a no-op and a report after Close is dropped.
func (a *PushAdapter) Report(err error) { a.emit(err) }

// notifyTime converts a wire notifyTime (milliseconds since the Unix epoch) into
// a UTC time.Time. A zero value maps to the zero time rather than to 1970.
//
// The conversion lives here rather than in internal/push because ADR 0010 rule 2
// puts wire-to-domain conversion in the transport layer, and because
// push.Notification.Time is deliberately left as the wire value it is.
func notifyTime(ms uint64) time.Time {
	if ms == 0 {
		return time.Time{}
	}
	return time.UnixMilli(int64(ms)).UTC() // #nosec G115 -- Gateway notifyTime; a wrapped value yields an implausible timestamp, not an unsafe one
}

// SubscribeTypes registers h as the handler for push notifications of type t.
//
// This is the seam: a nil handler deregisters, and a second handler for the same
// type replaces the first, so the caller owns the handler set and is expected to
// install one handler per payload type and fan out from there.
//
// The conversion it performs is the whole reason this package exists on the push
// path. A decoded frame arrives as an internal/push Notification whose payload is
// a generated protobuf pointer; h receives a *domain.PushUpdate whose Event is a
// typed domain event. A frame whose payload cannot be normalised is reported on
// the error stream and not delivered, because delivering a nil event would break
// the contract domain.PushUpdate documents.
//
// The notification is never nil here - the push client's dispatch drops a nil
// before it reaches a handler - so the only guard is the handler, and it is
// reachable: a nil h still installs this closure, which then discards every frame
// of that type.
//
// No sentinel is raised here beyond the verification translation: the push client
// reports its own transport, verification and decode errors on its own stream, and
// this method only adds the error that exists solely because the two halves have
// to meet.
func (a *PushAdapter) SubscribeTypes(t types.NotifyMsgType, h func(*domain.PushUpdate)) {
	a.client.SubscribeTypes(t, func(n *push.Notification) { a.convert(n, h) })
}

// convert turns one decoded push notification into the domain update the seam
// delivers, and is the whole of what the seam does.
//
// It is a named method rather than an inline closure so that its failure branch is a
// thing a test can reach: the push client drives Decode from the same notifyMsgType
// it puts on the Notification, so a payload that does not match its declared type
// cannot come off the wire and the branch is unreachable from a frame. Reporting it
// still matters, because the alternative is a swallowed normalisation failure - the
// defect class this repository exists to remove - so it is exercised directly here.
func (a *PushAdapter) convert(n *push.Notification, h func(*domain.PushUpdate)) {
	if h == nil {
		return
	}
	event, err := push.NormalizeNotification(n)
	if err != nil {
		a.Report(err)
		return
	}
	h(&domain.PushUpdate{
		Type:  n.Type,
		ID:    n.ID,
		Time:  notifyTime(n.Time),
		Event: *event,
	})
}

// String renders the adapter's resolved address, so a log line names the
// connection it is about rather than the struct that owns it.
func (a *PushAdapter) String() string {
	return fmt.Sprintf("transport.PushAdapter{addr: %s}", a.cfg.Addr)
}
