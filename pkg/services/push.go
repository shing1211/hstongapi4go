// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package services

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/shing1211/hstongapi4go/internal/errs"
	"github.com/shing1211/hstongapi4go/pkg/domain"
	"github.com/shing1211/hstongapi4go/pkg/types"
)

const (
	opPushSubscribe        = "push.Subscribe"
	opPushSubscribeOrders  = "push.SubscribeOrders"
	opPushUnsubscribe      = "push.Unsubscribe"
	opPushResubscribe      = "push.Resubscribe"
	opPushUnsubscribeOrder = "push.UnsubscribeOrders"
)

// Defaults applied by NewPushOrchestration when no option overrides them.
const (
	// DefaultPushBuffer is the depth of a subscription's update channel. It
	// matches internal/push's per-type dispatch queue, so the two layers drop at
	// the same rate when a consumer falls behind.
	DefaultPushBuffer = 64
	// DefaultResubscribeTimeout bounds one /hq/Subscribe re-issue after a
	// reconnect. It is the released layer's value; the difference here is the
	// parent context, not the deadline.
	DefaultResubscribeTimeout = 10 * time.Second
	// DefaultStaleTolerance is the silence after which Stale reports a
	// subscription stale when the caller supplies a non-positive threshold. It
	// matches internal/push.NewFreshnessMonitor's default, whose rule this
	// adopts: accepting zero would mark every subscription stale on the first
	// check.
	DefaultStaleTolerance = 5 * time.Minute
	// unroutedReportCap bounds how many distinct ids the orchestrator remembers
	// for the "report once per distinct id" rule. Past the cap every unrouted
	// update is reported again, because an unbounded map keyed by wire data is
	// the memory growth §8.3's bounded error stream exists to prevent.
	unroutedReportCap = 64
)

// Sentinel errors raised on the PushOrchestration client error stream. Each is
// matched with errors.Is; the underlying cause is reachable with errors.Unwrap.
var (
	// ErrPushClosed is returned by every PushOrchestration method after Close.
	ErrPushClosed = errors.New("services: push orchestration is closed")
	// ErrNoPushTransport reports an orchestration built without a TCP half.
	//
	// It is its own sentinel rather than a re-use of ErrPushClosed because the
	// two are different mistakes with different fixes, and because Subscribe must
	// refuse without it: a market subscription registered with no handler registry
	// can never deliver, and "the caller believes they subscribed and receives
	// nothing" is the single worst outcome this layer has.
	ErrNoPushTransport = errors.New("services: push orchestration has no transport")
	// ErrReconnected reports that the push connection was re-established. It is
	// emitted exactly once per reconnect, unconditionally - including when no
	// subscription exists, because a reconnect a caller cannot observe is a
	// reconnect they will believe was never attempted.
	ErrReconnected = errors.New("services: push connection re-established")
	// ErrUnrouted reports a delivered push update that matched no live
	// subscription. An update nobody receives while the caller believes they
	// subscribed is the worst outcome this layer has, so it is the loudest
	// thing on the stream.
	ErrUnrouted = errors.New("services: push update matched no subscription")
	// ErrTradeBacklog reports that a trade-delivery subscription's buffer
	// overflowed and a fill was discarded. The guarantee is "no silent loss",
	// not "no loss": a caller that never drains is told, rather than being
	// allowed to miss a fill without knowing.
	ErrTradeBacklog = errors.New("services: trade delivery buffer overflowed")
	// ErrSubscriptionLost reports that a subscription was cancelled locally
	// because its re-subscribe after a reconnect was refused. The subscription
	// is dead rather than registered-and-silent: for a market topic the next
	// observation would otherwise be the absence of news, which during a quiet
	// market is indistinguishable from a working feed.
	ErrSubscriptionLost = errors.New("services: push subscription terminated after a failed re-subscribe")
	// ErrUpdateOutOfOrder reports the first push frame whose wire notifyTime is
	// older than one already accepted for the same subscription. Later
	// occurrences are counted on the subscription and not reported, so a
	// permanently skewed Gateway clock produces one alert rather than a flood.
	// The frame itself is always delivered.
	ErrUpdateOutOfOrder = errors.New("services: push frame arrived out of order")
	// ErrSignatureMismatch reports a push frame whose SHA1WithRSA bodySHA1
	// signature did not verify against the configured platform public key.
	//
	// It is an alias of domain.ErrSignatureMismatch rather than a distinct
	// sentinel, so this name and the domain name are the same error value and
	// either matches. It is declared here because a sentinel a caller is
	// expected to branch on has to be nameable by that caller, and the one
	// internal/push raises is not: `internal/` is unimportable outside this
	// module.
	ErrSignatureMismatch = domain.ErrSignatureMismatch
)

// PushTransport is the TCP half of the push orchestration: the Gateway push
// socket, its handler registry, its error stream, and its reconnect signal.
//
// It is declared here, in the dependent package, for the reason ADR 0010 rule 6
// states: pkg/services must say what it needs and pkg/transport must supply it.
// *transport.PushAdapter is the intended implementation.
//
// It names no type from internal/push, and the reason is not tidiness. A type
// under `internal/` is unimportable outside this module, so a signature naming
// one could not be implemented by a third party - not a test fake, not a
// recorded fixture, not a different push engine - and pkg/services would lose
// the ability to be tested without a Gateway. domain.PushUpdate is declared in
// pkg/domain precisely because that is the one package both sides can name.
//
// OnReconnect is the sixth method and the design note's five-method list is
// incomplete here: without a reconnect signal the orchestrator cannot
// re-establish its own subscriptions, and §7.3 requires it to. Its parameter
// type is func(error), so nothing about the internal package leaks into the
// seam; only the count of methods is larger than the note's sketch.
type PushTransport interface {
	// Connect dials the Gateway push address. It is idempotent while already
	// connected.
	Connect(ctx context.Context) error
	// Run reads frames until ctx is cancelled or Close is called, reconnecting
	// with backoff and invoking the OnReconnect hooks after each reconnect.
	Run(ctx context.Context) error
	// Close releases the connection and stops the read loop. It is idempotent.
	Close() error
	// Errors is the transport's error stream. Every error on it is forwarded to
	// the orchestrator's own stream verbatim, chain intact.
	Errors() <-chan error
	// OnReconnect registers a hook invoked after each successful reconnect with
	// the error that ended the previous connection. Hooks run on the transport's
	// Run goroutine, so they must not block for long.
	OnReconnect(func(error))
	// SubscribeTypes registers h as the handler for notifications of type t. A
	// nil handler deregisters, and a second handler for the same type replaces
	// the first.
	SubscribeTypes(t types.NotifyMsgType, h func(*domain.PushUpdate))
}

// TopicSubscriber is the HTTP half of a market push subscription: the
// /hq/Subscribe and /hq/Unsubscribe pair.
//
// *MarketService satisfies it structurally, with no adapter: its Subscribe and
// Unsubscribe already have exactly these signatures.
type TopicSubscriber interface {
	// Subscribe issues POST /hq/Subscribe for one topic and instrument list.
	Subscribe(ctx context.Context, topic types.TopicID, sec ...*Security) error
	// Unsubscribe issues POST /hq/Unsubscribe for one topic and instrument list.
	Unsubscribe(ctx context.Context, topic types.TopicID, sec ...*Security) error
}

// OrderPushSubscriber is the HTTP half of a trade push subscription: the
// session-wide /trade/TradeSubscribe and /trade/TradeUnsubscribe pair.
//
// *TradingService satisfies it structurally. Trade push is session-wide and
// per-account rather than per-topic, which is why it gets its own interface and
// not a second TopicSubscriber: the two halves have different units and
// different counts.
type OrderPushSubscriber interface {
	// SubscribeOrders enables order-status push for the session.
	SubscribeOrders(ctx context.Context, accountID domain.AccountID) error
	// UnsubscribeOrders disables it.
	UnsubscribeOrders(ctx context.Context, accountID domain.AccountID) error
}

// The two HTTP seams are satisfied structurally by services this package already
// ships, so the orchestrator adds no adapter code and names no client.Route of
// its own.
var (
	_ TopicSubscriber     = (*MarketService)(nil)
	_ OrderPushSubscriber = (*TradingService)(nil)
)

// NotifyTypeForTopic reports the PBNotify notifyMsgType of the payload the
// Gateway pushes for topic t, and false for a TopicID this SDK does not map.
//
// It is the single relation joining the layer's two entry vocabularies: a
// caller holds a types.TopicID and needs to know which payload it will receive.
// The relation is a switch and not a map for a reason that is specific to this
// shape: a missing map key yields the zero value 0, which is
// TrsStockDeliverMsgType - a real, valid, order-status type - so a forgotten
// entry would silently deliver order notifications to a subscriber that asked for
// a quote. A switch has no such state; default returns (0, false) and every call
// site has to say what it does with the second result.
//
// The mapping is the one docs/SPEC.md §4 states, group by group. The three
// delivery types - TrsStockDeliverMsgType, TradeStockDeliverMsgType and
// FuturesTradeStockDeliverMsgType - are deliberately absent: they have no
// TopicID at all, are enabled by the session-wide /trade/TradeSubscribe, and are
// reached through PushOrchestration.SubscribeOrders and by nothing else.
//
// This is exported rather than package-private because its consumer set is wider
// than one internal caller: a caller holding a TopicID needs to know which
// accessor to reach for, and the exhaustive test over the relation needs to
// compare it against the specification. See
// docs/runs/2026-09-26-vnext-parity-wire/design-push-orchestration.md §5.
func NotifyTypeForTopic(t types.TopicID) (types.NotifyMsgType, bool) {
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

// marketTopics lists every topic MarketService accepts, in a stable order.
//
// It reads the package's own accepted-topic set rather than repeating the
// constants, so the set the handlers are derived from and the set /hq/Subscribe
// will accept cannot drift apart by being written twice.
func marketTopics() []types.TopicID {
	out := make([]types.TopicID, 0, len(knownTopics))
	for t := range knownTopics {
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// marketNotifyTypes is the image of NotifyTypeForTopic over the accepted topics:
// the distinct payload types a topic subscription can deliver.
//
// It is derived rather than declared, which is the one improvement over the
// released layer's hardcoded slice. A topic mapped to a fifth payload type cannot
// be "mapped but not delivered", because there is no second list to forget.
func marketNotifyTypes() []types.NotifyMsgType {
	seen := make(map[types.NotifyMsgType]struct{})
	out := make([]types.NotifyMsgType, 0, 4)
	for _, t := range marketTopics() {
		msgType, ok := NotifyTypeForTopic(t)
		if !ok {
			continue
		}
		if _, dup := seen[msgType]; dup {
			continue
		}
		seen[msgType] = struct{}{}
		out = append(out, msgType)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// tradeNotifyTypes are the three order-status push types. They are declared
// rather than derived because there is no relation to derive them from: no
// TopicID maps to them and no other function names them.
var tradeNotifyTypes = []types.NotifyMsgType{
	types.TrsStockDeliverMsgType,
	types.TradeStockDeliverMsgType,
	types.FuturesTradeStockDeliverMsgType,
}

// isTradeNotifyType reports whether t is a session-wide order-status type.
func isTradeNotifyType(t types.NotifyMsgType) bool {
	for _, tt := range tradeNotifyTypes {
		if tt == t {
			return true
		}
	}
	return false
}

// deliverablePayloadNames names every payload type a topic subscription can
// deliver, sorted. The unmapped-topic error message embeds it, so the message
// answers the caller's actual question - "what could I have got instead?" -
// rather than dead-ending, and it names *all* of them so a reader cannot infer
// from the list that only some are accepted.
func deliverablePayloadNames() []string {
	src := marketNotifyTypes()
	out := make([]string, 0, len(src))
	for _, t := range src {
		out = append(out, t.String())
	}
	sort.Strings(out)
	return out
}

// PushOption configures a PushOrchestration. Options are applied in order by
// NewPushOrchestration, so the last one that sets a field wins.
type PushOption func(*PushOrchestration)

// WithBuffer sets the depth of every subscription's update channel. A
// non-positive n restores DefaultPushBuffer.
//
// The channel is never allowed to block the reader. When it is full the oldest
// update is discarded to make room for the newest and the subscription's
// Dropped() counter advances - a quote is state and each frame supersedes its
// predecessor, so a stale sample is worth less than the current one. Trade
// delivery is the exception and is not silent; see ErrTradeBacklog.
func WithBuffer(n int) PushOption {
	return func(o *PushOrchestration) {
		if n > 0 {
			o.buffer = n
			return
		}
		o.buffer = DefaultPushBuffer
	}
}

// WithResubscribeTimeout sets the deadline on one re-subscribe attempt after a
// reconnect. A non-positive d restores DefaultResubscribeTimeout.
//
// The deadline is applied on top of the orchestrator's own re-subscribe context,
// which Close cancels - so a Close racing a reconnect leaves no /hq/Subscribe in
// flight against a Gateway the caller has already released.
func WithResubscribeTimeout(d time.Duration) PushOption {
	return func(o *PushOrchestration) {
		if d > 0 {
			o.resubTimeout = d
			return
		}
		o.resubTimeout = DefaultResubscribeTimeout
	}
}

// WithTradeSubscriber supplies the session-wide trade-push HTTP half, which is
// what SubscribeOrders reaches. It is an option rather than a constructor
// parameter because order push is optional: a caller who never calls
// SubscribeOrders must not have to supply a TradingService.
//
// A nil value clears it, which makes SubscribeOrders fail locally with a typed
// error instead of nil-panicking on a half-configured orchestrator.
func WithTradeSubscriber(tr OrderPushSubscriber) PushOption {
	return func(o *PushOrchestration) {
		o.trade = tr
	}
}

// WithPushClock is deliberately absent. Freshness is measured against the
// orchestrator's own clock field, which defaults to time.Now and is replaced in
// tests; exposing it would add a public option whose only caller is this
// package's own test suite.

// NewPushOrchestration returns an orchestration over sub (the HTTP half) and tr
// (the TCP half), configured by opts. It never dials and never issues a request:
// call Connect, then Run.
//
// The TCP handler set is installed here, once: one handler per deliverable
// payload type, for the image of NotifyTypeForTopic and for the three
// session-wide delivery types. The handler fans out to whichever subscriptions
// are live, so a handler is registered or it is not and re-registering is a
// replace - there is no per-type counting here. Only the HTTP half is
// reference-counted, and it is counted per (topic, security set).
func NewPushOrchestration(sub TopicSubscriber, tr PushTransport, opts ...PushOption) *PushOrchestration {
	o := &PushOrchestration{
		market:       sub,
		transport:    tr,
		buffer:       DefaultPushBuffer,
		resubTimeout: DefaultResubscribeTimeout,
		now:          time.Now,
		entries:      make(map[string]*pushEntry),
		byKey:        make(map[string][]*Subscription),
		seenIDs:      make(map[string]struct{}),
		errc:         make(chan error, DefaultPushBuffer),
		closeCh:      make(chan struct{}),
	}
	o.resubCtx, o.resubCancel = context.WithCancel(context.Background())
	for _, opt := range opts {
		if opt != nil {
			opt(o)
		}
	}
	if tr != nil {
		for _, t := range marketNotifyTypes() {
			o.install(t)
		}
		for _, t := range tradeNotifyTypes {
			o.install(t)
		}
		tr.OnReconnect(o.onReconnect)
		o.wg.Add(1)
		go o.drainTransportErrors()
	}
	return o
}

// install registers this orchestrator's single handler for payload type t.
func (o *PushOrchestration) install(t types.NotifyMsgType) {
	o.transport.SubscribeTypes(t, func(u *domain.PushUpdate) {
		if u == nil || u.Event == nil {
			return
		}
		if isTradeNotifyType(u.Type) {
			o.dispatchTrade(u)
			return
		}
		o.dispatchMarket(u)
	})
}

// pushEntry is one coalesced (topic, security set) key: the unit at which an
// /hq/Subscribe is issued and reference-counted.
type pushEntry struct {
	topic      types.TopicID
	securities []*Security
	subs       []*Subscription
	// subscribed records that the Gateway accepted /hq/Subscribe for this key.
	// Only a key with it true is re-subscribed after a reconnect: the Gateway
	// was never told about the others.
	subscribed bool
}

// PushOrchestration composes the Gateway's two push transports into one
// subscription API: the HTTP /hq/Subscribe side that tells the Gateway what to
// send, and the TCP side that receives it.
//
// The two entry vocabularies are disjoint and are the point of the design. A
// caller names a types.TopicID and a security list for market data, or an account
// for the session-wide order-status push. It may not name a types.NotifyMsgType:
// three of the seven payload types have no topic at all, and a
// NotifyMsgType-parameterised entry point would have to encode "topic plus
// securities, or a login?" - a variant the wire does not express and pkg/types
// does not declare. It would also let a caller ask for an order book without
// ever issuing /hq/Subscribe, and then wait forever for one.
//
// A PushOrchestration is safe for concurrent use. Close is idempotent and
// leak-free, and every subscription's Updates channel is closed by Cancel, by a
// re-subscribe that the Gateway refused, or by Close.
type PushOrchestration struct {
	market    TopicSubscriber
	trade     OrderPushSubscriber
	transport PushTransport

	buffer       int
	resubTimeout time.Duration

	// now measures LastSeen and Stale. It is time.Now unless a test replaces
	// it, which is how the freshness tests avoid sleeping.
	now func() time.Time

	resubCtx    context.Context
	resubCancel context.CancelFunc

	// subMu serialises whole subscription transactions - registry mutation plus
	// the HTTP request that justifies it - so two concurrent subscribers of one
	// key issue one /hq/Subscribe between them. It is never held while mu is
	// not, and it is never held while a dispatch is running, so a slow Gateway
	// cannot stall frame routing.
	subMu sync.Mutex

	mu      sync.Mutex
	entries map[string]*pushEntry
	// byKey indexes live subscriptions by every normalised instrument code they
	// asked for. An update is matched on both its notifyId and the code inside
	// its payload, because the repository cannot say those are the same string
	// and the wire does not promise they are.
	byKey  map[string][]*Subscription
	orders []*OrderSubscription
	// tradeOn and tradeSubs track the session-wide trade subscription, which is
	// counted on its own: one HTTP call for the first subscriber and one for the
	// last, across every account on the connection.
	tradeOn      bool
	tradeSubs    int
	tradeAccount domain.AccountID

	unrouted uint64
	seenIDs  map[string]struct{}
	errsDrop uint64

	errc      chan error
	errcMu    sync.Mutex
	closeCh   chan struct{}
	closed    atomic.Bool
	closeOnce sync.Once
	wg        sync.WaitGroup
}

// Errors returns the client-level error stream. It is live from construction -
// before Connect, and with no subscription - so a reconnect or a verification
// failure is observable whether or not anybody is listening for data.
//
// Every transport error is forwarded here verbatim, with its errors.Is and
// errors.As chain intact, rather than filtered to the verification errors the
// released layer forwards: a client-level stream has no compatibility constraint
// against a channel reconnect notices already consume.
//
// The channel is bounded and drops oldest, and ErrorsDropped reports how many
// diagnoses were lost that way. A lost diagnosis is recoverable; an unbounded
// queue on a flapping connection is a memory leak. It is closed by Close.
func (o *PushOrchestration) Errors() <-chan error { return o.errc }

// ErrorsDropped reports how many errors were discarded because the stream was
// full when they were raised. It is the reason the drop is acceptable, and it is
// asserted to be observable rather than assumed.
func (o *PushOrchestration) ErrorsDropped() uint64 {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.errsDrop
}

// Unrouted reports how many delivered push updates matched no live
// subscription.
//
// A caller who believes they subscribed and receives nothing is the worst
// outcome this layer has, so the count is public, every unrouted update raises
// ErrUnrouted on the client stream, and the first update per distinct id is
// reported at most once so a Gateway that keeps pushing a stale id produces one
// alert rather than a flood.
func (o *PushOrchestration) Unrouted() uint64 {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.unrouted
}

// NotifyTypeForTopic reports the payload type a topic subscription delivers, and
// false for a topic this SDK does not map. It is the package function of the
// same name, reached from a value; see that function for the mapping and the
// reasons it is a switch.
func (o *PushOrchestration) NotifyTypeForTopic(t types.TopicID) (types.NotifyMsgType, bool) {
	return NotifyTypeForTopic(t)
}

// Connect dials the Gateway push address. It is idempotent while already
// connected and returns ErrPushClosed after Close. The context bounds only the
// dial; the connection's lifetime is governed by Run and Close.
func (o *PushOrchestration) Connect(ctx context.Context) error {
	if o.closed.Load() {
		return ErrPushClosed
	}
	if o.transport == nil {
		return ErrNoPushTransport
	}
	return o.transport.Connect(ctx)
}

// Run reads push frames until ctx is cancelled or Close is called, reconnecting
// and re-establishing every subscription after each reconnect. It returns
// ErrPushClosed after Close, ctx.Err() on cancellation, or nil if Close ended
// the read loop. Run is normally started in its own goroutine after Connect.
func (o *PushOrchestration) Run(ctx context.Context) error {
	if o.closed.Load() {
		return ErrPushClosed
	}
	if o.transport == nil {
		return ErrNoPushTransport
	}
	return o.transport.Run(ctx)
}

// Close stops the read loop, cancels any in-flight re-subscribe, closes every
// subscription's Updates channel, and closes the error stream. It is idempotent
// and never panics. After Close, Connect, Run, Subscribe and SubscribeOrders
// return ErrPushClosed and every accessor keeps reporting its last value.
func (o *PushOrchestration) Close() error {
	o.closeOnce.Do(func() {
		o.closed.Store(true)
		close(o.closeCh)
		o.resubCancel()
		if o.transport != nil {
			_ = o.transport.Close()
		}
		o.wg.Wait()
		o.closeAll()
		o.errcMu.Lock()
		close(o.errc)
		o.errcMu.Unlock()
	})
	return nil
}

// drainTransportErrors forwards every transport error verbatim, forever, so the
// client stream carries a reconnect's cause and a bad frame's verdict whether or
// not a subscription exists.
func (o *PushOrchestration) drainTransportErrors() {
	defer o.wg.Done()
	upstream := o.transport.Errors()
	for {
		select {
		case err, ok := <-upstream:
			if !ok {
				return
			}
			o.report(err)
		case <-o.closeCh:
			return
		}
	}
}

// report surfaces err on the client stream, dropping the oldest when the stream
// is full and counting the loss. It never blocks and never sends after Close.
func (o *PushOrchestration) report(err error) {
	if err == nil {
		return
	}
	o.errcMu.Lock()
	defer o.errcMu.Unlock()
	if o.closed.Load() {
		return
	}
	select {
	case o.errc <- err:
		return
	default:
	}
	select {
	case <-o.errc:
		o.mu.Lock()
		o.errsDrop++
		o.mu.Unlock()
	default:
	}
	select {
	case o.errc <- err:
	default:
	}
}

// closeAll closes every live subscription's Updates channel. It runs after the
// drain goroutine has stopped and the transport is closed, so nothing can write
// to a channel it is about to close.
func (o *PushOrchestration) closeAll() {
	o.subMu.Lock()
	markets := make([]*Subscription, 0, len(o.entries))
	for _, e := range o.entries {
		markets = append(markets, e.subs...)
	}
	orders := make([]*OrderSubscription, len(o.orders))
	copy(orders, o.orders)
	o.entries = make(map[string]*pushEntry)
	o.byKey = make(map[string][]*Subscription)
	o.orders = nil
	o.subMu.Unlock()

	for _, sub := range markets {
		sub.shutdown()
	}
	for _, ord := range orders {
		ord.shutdown()
	}
}

// ---------------------------------------------------------------------------
// Subscribing
// ---------------------------------------------------------------------------

// Subscribe registers a market push subscription for one topic and instrument
// list, issuing POST /hq/Subscribe when the key is new.
//
// The key is (topic, canonical security set): the instruments are sorted by
// (Code, DataType) and value-compared, so two callers passing the same
// instruments in a different order share one subscription. It is not the topic
// alone - coalescing on the topic would collapse two subscribers watching
// different instruments into one request carrying one instrument's list, and the
// other would never arrive.
//
// A second subscriber of an existing key issues no request and returns a
// subscription of its own; cancelling either one leaves the Gateway's
// subscription in place until the last is cancelled. That is a deliberate
// divergence from the released layer, which sends one /hq/Subscribe per
// Subscription and one /hq/Unsubscribe per cancel - so the first cancel tears
// down a stream the second caller is still using, silently.
//
// A topic this SDK does not map is rejected locally, before any request, with an
// errs.StatusInvalidParam naming every payload type that *is* deliverable. A
// guess would hand the caller events of a type it will mis-parse with every
// accessor, and nothing anywhere would report it.
func (o *PushOrchestration) Subscribe(ctx context.Context, topic types.TopicID, sec ...*Security) (*Subscription, error) {
	if o.closed.Load() {
		return nil, ErrPushClosed
	}
	if o.transport == nil {
		return nil, ErrNoPushTransport
	}
	if o.market == nil {
		return nil, ErrNoPushTransport
	}
	return o.subscribeUnderLock(ctx, topic, sec...)
}

// subscribeUnderLock is Subscribe's body, with the closed check repeated now that
// the subscription lock is held.
//
// The second check is load-bearing, not belt-and-braces. Close drains the registry
// under this same lock, so a Subscribe that got past the first check has two
// possible orders: it registers before Close's drain, and is drained; or it registers
// after, and is left live on a closed orchestrator with a channel nobody will ever
// close. Only the check under the lock rules out the second. It is split out as its
// own function so that this is a guard a test can reach by hand rather than by
// winning a race against a scheduler turn.
func (o *PushOrchestration) subscribeUnderLock(ctx context.Context, topic types.TopicID, sec ...*Security) (*Subscription, error) {
	msgType, ok := NotifyTypeForTopic(topic)
	if !ok {
		return nil, errs.New(types.StatusInvalidParam, opPushSubscribe, unmappedTopicMessage(topic))
	}
	securities, err := copyAndValidateSecurities(topic, sec)
	if err != nil {
		return nil, err
	}
	key := canonicalPushKey(topic, securities)

	o.subMu.Lock()
	defer o.subMu.Unlock()
	if o.closed.Load() {
		return nil, ErrPushClosed
	}

	entry, present := o.entries[key]
	if !present {
		if err := o.market.Subscribe(ctx, topic, securities...); err != nil {
			return nil, err
		}
		entry = &pushEntry{topic: topic, securities: securities, subscribed: true}
		o.entries[key] = entry
	}
	sub := newSubscription(o, topic, msgType, securities)
	entry.subs = append(entry.subs, sub)
	o.attachLocked(entry, sub)
	return sub, nil
}

// attachLocked adds sub to the routing index under each instrument key.
func (o *PushOrchestration) attachLocked(entry *pushEntry, sub *Subscription) {
	for _, k := range securityKeys(entry.securities) {
		o.byKey[k] = append(o.byKey[k], sub)
	}
}

// detachLocked removes sub from the registry, dropping the entry when its last
// subscriber leaves. It reports the entry when the caller must issue an
// /hq/Unsubscribe, and nil when it must not.
func (o *PushOrchestration) detachLocked(sub *Subscription) *pushEntry {
	key := canonicalPushKey(sub.topic, sub.securities)
	entry, ok := o.entries[key]
	if !ok {
		return nil
	}
	for i, s := range entry.subs {
		if s == sub {
			entry.subs = append(entry.subs[:i], entry.subs[i+1:]...)
			break
		}
	}
	if len(entry.subs) == 0 {
		delete(o.entries, key)
		for _, k := range securityKeys(entry.securities) {
			o.removeFromIndexLocked(k, sub)
		}
		return entry
	}
	for _, k := range securityKeys(entry.securities) {
		o.removeFromIndexLocked(k, sub)
	}
	return nil
}

// removeFromIndexLocked drops sub from one routing bucket.
func (o *PushOrchestration) removeFromIndexLocked(key string, sub *Subscription) {
	bucket := o.byKey[key]
	for i, s := range bucket {
		if s == sub {
			bucket = append(bucket[:i], bucket[i+1:]...)
			break
		}
	}
	if len(bucket) == 0 {
		delete(o.byKey, key)
		return
	}
	o.byKey[key] = bucket
}

// SubscribeOrders enables the session-wide order-status push and returns a
// subscription to the three delivery types.
//
// It issues POST /trade/TradeSubscribe for the first subscriber on the
// connection and POST /trade/TradeUnsubscribe for the last, counting across
// accounts: the Gateway's scope is the session, so two accounts on one
// connection share the one call. That is a different unit from the market
// registry's per-(topic, securities) count and deliberately does not share it.
//
// The returned type has no TopicID and no Securities accessor, and that absence
// is the type-level statement that a trade subscription has no topic: it is
// enabled by a login, not by a topic and a security list.
func (o *PushOrchestration) SubscribeOrders(ctx context.Context, accountID domain.AccountID) (*OrderSubscription, error) {
	if o.closed.Load() {
		return nil, ErrPushClosed
	}
	if o.transport == nil {
		return nil, ErrNoPushTransport
	}
	if o.trade == nil {
		return nil, errs.New(types.StatusInvalidParam, opPushSubscribeOrders,
			"order push needs a trade subscriber; construct the orchestration with services.WithTradeSubscriber")
	}
	return o.subscribeOrdersUnderLock(ctx, accountID)
}

// subscribeOrdersUnderLock is SubscribeOrders' body, with the closed check repeated
// under the subscription lock for the reason subscribeUnderLock gives: without it, a
// subscription registered after Close's drain would be left live on a closed
// orchestrator.
func (o *PushOrchestration) subscribeOrdersUnderLock(ctx context.Context, accountID domain.AccountID) (*OrderSubscription, error) {
	if accountID.IsZero() {
		return nil, errs.New(types.StatusInvalidParam, opPushSubscribeOrders, "accountID must not be empty")
	}

	o.subMu.Lock()
	defer o.subMu.Unlock()
	if o.closed.Load() {
		return nil, ErrPushClosed
	}

	if !o.tradeOn {
		if err := o.trade.SubscribeOrders(ctx, accountID); err != nil {
			return nil, err
		}
		o.tradeOn = true
		o.tradeAccount = accountID
	}
	sub := newOrderSubscription(o, accountID)
	o.orders = append(o.orders, sub)
	o.tradeSubs++
	return sub, nil
}

// ---------------------------------------------------------------------------
// Reconnect
// ---------------------------------------------------------------------------

// onReconnect is the transport's reconnect hook. It raises exactly one
// ErrReconnected per reconnect - unconditionally, including with no subscription
// at all - and then re-establishes every subscription from the orchestrator's own
// registry rather than from a caller-visible notification, because a
// caller-driven re-subscribe is a notification nobody is watching.
//
// The hook runs on the transport's Run goroutine, so the re-subscribe is one
// attempt per reconnect with a deadline and never a retry loop: an unbounded
// retry there would stall the read loop for every other subscriber. The next
// reconnect tries again, which is both simpler and strictly more likely to be
// observed.
func (o *PushOrchestration) onReconnect(cause error) {
	if o.closed.Load() {
		return
	}
	o.report(newReconnectNotice(cause))
	o.resubscribeAll()
}

// resubscribeAll re-issues one /hq/Subscribe per registry key the Gateway had
// accepted, plus the session-wide trade subscription when one is live.
//
// The keys are visited in a stable order rather than map order. Go randomises map
// iteration, so an unordered walk would make which key is attempted first - and
// therefore which subscription a partial Gateway outage kills - differ between
// runs of the same code against the same Gateway. A reproducible order is what
// makes a reconnect debuggable at all.
func (o *PushOrchestration) resubscribeAll() {
	o.subMu.Lock()
	keys := make([]*pushEntry, 0, len(o.entries))
	for _, e := range o.entries {
		if e.subscribed {
			keys = append(keys, e)
		}
	}
	sort.Slice(keys, func(i, j int) bool {
		return canonicalPushKey(keys[i].topic, keys[i].securities) <
			canonicalPushKey(keys[j].topic, keys[j].securities)
	})
	tradeLive := o.tradeOn
	account := o.tradeAccount
	o.subMu.Unlock()

	for _, entry := range keys {
		ctx, cancel := o.resubscribeContext()
		err := o.market.Subscribe(ctx, entry.topic, entry.securities...)
		cancel()
		if err == nil {
			continue
		}
		o.report(&streamError{
			tag:   ErrSubscriptionLost,
			cause: fmt.Errorf("resubscribe topicId %d for %s after reconnect: %w", int(entry.topic), keyList(entry.securities), err),
		})
		o.killEntry(entry)
	}

	if !tradeLive || o.trade == nil {
		return
	}
	ctx, cancel := o.resubscribeContext()
	err := o.trade.SubscribeOrders(ctx, account)
	cancel()
	if err != nil {
		o.report(&streamError{
			tag:   ErrSubscriptionLost,
			cause: fmt.Errorf("resubscribe session-wide order push for account %s: %w", account, err),
		})
		o.killOrders(err)
	}
}

// resubscribeContext derives one bounded context from the orchestrator's own
// re-subscribe parent, which Close cancels. The released layer derives from
// context.Background() and comments that the run context "may already be
// cancelled"; here the run context is exactly the one a caller would cancel to
// stop reading, so honouring it would make a reconnect issue a request against a
// Gateway the caller has walked away from.
func (o *PushOrchestration) resubscribeContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(o.resubCtx, o.resubTimeout)
}

// killEntry terminates every subscription sharing a key whose re-subscribe the
// Gateway refused, and best-effort issues the matching /hq/Unsubscribe when it
// was the last reference.
//
// The unsubscribe is best effort because the Gateway cannot have honoured the
// subscribe it just refused; its failure is reported rather than swallowed, but
// it never keeps the local subscriptions alive - a loud dead subscription beats
// a silent one.
//
// The refusal itself is reported by the caller, which holds the reason; this
// function owns only the local teardown.
func (o *PushOrchestration) killEntry(entry *pushEntry) {
	o.subMu.Lock()
	doomed := make([]*Subscription, 0, len(entry.subs))
	doomed = append(doomed, entry.subs...)
	key := canonicalPushKey(entry.topic, entry.securities)
	for _, sub := range doomed {
		for _, k := range securityKeys(sub.securities) {
			o.removeFromIndexLocked(k, sub)
		}
	}
	delete(o.entries, key)
	topic, securities := entry.topic, entry.securities
	o.subMu.Unlock()

	for _, sub := range doomed {
		sub.shutdown()
	}
	// Every entry in the registry has at least one subscriber - detachLocked drops
	// an entry when its last one leaves - and an entry cannot exist at all without a
	// topic subscriber, since Subscribe refuses without one. So doomed is non-empty
	// and o.market is non-nil here, and neither is re-checked.
	ctx, cancel := o.resubscribeContext()
	defer cancel()
	if err := o.market.Unsubscribe(ctx, topic, securities...); err != nil {
		o.report(&streamError{
			tag:   ErrSubscriptionLost,
			cause: fmt.Errorf("unsubscribe topicId %d for %s after a failed re-subscribe: %w", int(topic), keyList(securities), err),
		})
	}
}

// killOrders terminates every session-wide trade subscription after the Gateway
// refused to re-enable the push.
func (o *PushOrchestration) killOrders(cause error) {
	o.subMu.Lock()
	doomed := make([]*OrderSubscription, len(o.orders))
	copy(doomed, o.orders)
	o.orders = nil
	o.tradeOn = false
	o.tradeSubs = 0
	o.subMu.Unlock()
	for _, ord := range doomed {
		ord.shutdown()
	}
}

// ---------------------------------------------------------------------------
// Dispatch
// ---------------------------------------------------------------------------

// dispatchMarket routes one market update to every subscription that asked for
// its instrument, on both candidate keys.
func (o *PushOrchestration) dispatchMarket(u *domain.PushUpdate) {
	keys := updateRoutingKeys(u)

	o.mu.Lock()
	targets := make([]*Subscription, 0, 2)
	seen := make(map[*Subscription]struct{}, 2)
	for _, k := range keys {
		for _, sub := range o.byKey[k] {
			// A bucket never holds nil - the index is built from live subscriptions
			// only - but one subscription reachable through two keys must not be
			// delivered twice, and that is what the seen set is for.
			if _, dup := seen[sub]; dup {
				continue
			}
			seen[sub] = struct{}{}
			targets = append(targets, sub)
		}
	}
	o.mu.Unlock()

	if len(targets) == 0 {
		o.recordUnrouted(u)
		return
	}
	for _, sub := range targets {
		sub.deliver(u)
	}
}

// dispatchTrade fans one delivery out to every live order subscription.
//
// Overflow is counted and reported rather than dropped silently: a fill is a
// per-event fact with no later notification, so an overflow is an error and not
// a discarded value. The channel still does not block, because blocking would
// stall the transport's dispatcher goroutine for TradeStockDeliverMsgType - that
// is, for every trade subscriber in the process. The guarantee is therefore "no
// silent loss", not "no loss".
func (o *PushOrchestration) dispatchTrade(u *domain.PushUpdate) {
	o.mu.Lock()
	targets := make([]*OrderSubscription, len(o.orders))
	copy(targets, o.orders)
	o.mu.Unlock()

	for _, ord := range targets {
		ord.deliver(u, o)
	}
}

// recordUnrouted counts an update no subscription wanted and reports it once per
// distinct id.
//
// Both keys are tried and neither is discarded: notifyId and the payload's
// security.code may not be the same string, the repository cannot say which the
// Gateway uses, and picking one would turn a mismatch into silent no-data - which
// is the single worst outcome this layer has.
func (o *PushOrchestration) recordUnrouted(u *domain.PushUpdate) {
	o.mu.Lock()
	o.unrouted++
	_, known := o.seenIDs[u.ID]
	if !known && len(o.seenIDs) < unroutedReportCap {
		o.seenIDs[u.ID] = struct{}{}
	}
	o.mu.Unlock()

	if known {
		return
	}
	o.report(&streamError{
		tag: ErrUnrouted,
		cause: fmt.Errorf("notifyMsgType %s id %q securityCode %q matched no live subscription "+
			"(the Gateway's notifyId may differ from the code sent to /hq/Subscribe; both are matched)",
			u.Type.String(), u.ID, u.SecurityCode()),
	})
}

// ---------------------------------------------------------------------------
// Subscription
// ---------------------------------------------------------------------------

// Subscription is one live market push subscription. It has no Close of its own
// beyond Cancel, and every accessor keeps reporting its last value after the
// subscription is cancelled, so a caller holding one can diagnose why it went
// quiet.
type Subscription struct {
	o          *PushOrchestration
	topic      types.TopicID
	msgType    types.NotifyMsgType
	securities []*Security

	updates chan domain.PushUpdate

	mu         sync.Mutex
	closed     bool
	dropped    uint64
	outOfOrder uint64
	lastSeen   time.Time
	lastUpdate string
	lastWire   time.Time
}

// newSubscription builds a subscription with the orchestrator's buffer depth.
func newSubscription(o *PushOrchestration, topic types.TopicID, msgType types.NotifyMsgType, securities []*Security) *Subscription {
	return &Subscription{
		o:          o,
		topic:      topic,
		msgType:    msgType,
		securities: securities,
		updates:    make(chan domain.PushUpdate, o.buffer),
	}
}

// Updates returns the channel of delivered updates. It is closed by Cancel, by a
// re-subscribe the Gateway refused, or by PushOrchestration.Close, and is never
// written to after any of those.
//
// The channel delivers domain.PushUpdate values, never a nil event: a frame
// whose payload cannot be classified arrives as a domain.SystemEvent.
func (sub *Subscription) Updates() <-chan domain.PushUpdate { return sub.updates }

// TopicID reports the market push topic this subscription was created for.
func (sub *Subscription) TopicID() types.TopicID { return sub.topic }

// NotifyMsgType reports the payload type this subscription's frames carry,
// which is NotifyTypeForTopic(TopicID()). It is offered so a caller does not
// have to hold both vocabularies at once; it accepts nothing.
func (sub *Subscription) NotifyMsgType() types.NotifyMsgType { return sub.msgType }

// Securities returns a copy of the subscribed instruments. Mutating the returned
// slice does not affect the subscription, and the registry holds copies too - the
// one place a caller-supplied pointer is read.
func (sub *Subscription) Securities() []*Security {
	// Every stored security is a copy made by copyAndValidateSecurities, which
	// rejects a nil instrument, so none of them can be nil here.
	out := make([]*Security, len(sub.securities))
	for i, s := range sub.securities {
		cp := *s
		out[i] = &cp
	}
	return out
}

// Dropped reports how many updates were discarded because the update channel was
// full when the newest one arrived. Drop-oldest is the right policy for a quote, a
// tick or an order-book level - each supersedes its predecessor - but it is only
// defensible while it is loud, which is what this counter is for.
func (sub *Subscription) Dropped() uint64 {
	sub.mu.Lock()
	defer sub.mu.Unlock()
	return sub.dropped
}

// OutOfOrder reports how many delivered frames carried a wire notifyTime older
// than one already accepted for this subscription.
//
// The frames are counted and still delivered, never dropped: dropping would be a
// silent loss, and it would be wrong across a reconnect boundary where an
// earlier timestamp is expected. Strict gap detection is impossible on this
// protocol - the push envelope carries no per-event sequence number - so this is
// a monotonic-clock observation and not a dedup.
func (sub *Subscription) OutOfOrder() uint64 {
	sub.mu.Lock()
	defer sub.mu.Unlock()
	return sub.outOfOrder
}

// LastSeen reports when this subscription last accepted an update, measured
// against the orchestrator's clock. It is the zero time before the first one.
func (sub *Subscription) LastSeen() time.Time {
	sub.mu.Lock()
	defer sub.mu.Unlock()
	return sub.lastSeen
}

// LastUpdate reports the payload's own Timestamp string as carried by the most
// recently accepted update. It is a different clock from LastSeen: this is the
// Gateway's per-instrument event time, LastSeen is the arrival, and the two
// disagreeing is what OutOfOrder's sibling - a stalled feed - would look like.
func (sub *Subscription) LastUpdate() string {
	sub.mu.Lock()
	defer sub.mu.Unlock()
	return sub.lastUpdate
}

// Stale reports whether this subscription has accepted nothing for longer than
// after. A non-positive after restores DefaultStaleTolerance rather than accepting
// zero, which would mark every subscription stale on the first check.
//
// The threshold is the caller's because a five-minute silence means something
// very different for a liquid Hong Kong stock and an illiquid warrant, and the SDK
// has no business guessing which regime the caller is in. A cancelled
// subscription's channel is closed, so a caller that stops reading sees the
// closure rather than a growing staleness.
func (sub *Subscription) Stale(after time.Duration) bool {
	if after <= 0 {
		after = DefaultStaleTolerance
	}
	sub.mu.Lock()
	lastSeen, closed := sub.lastSeen, sub.closed
	sub.mu.Unlock()
	if closed {
		return false
	}
	if lastSeen.IsZero() {
		// Nothing has arrived yet, so there is no interval to judge; reporting
		// stale before the first frame would flag every subscription the instant
		// it was made.
		return false
	}
	return sub.o.now().Sub(lastSeen) > after
}

// Cancel releases this subscription, issuing POST /hq/Unsubscribe only when it
// was the last holder of its (topic, security set) key.
//
// The local registration is dropped either way, so a Gateway that refuses the
// unsubscribe leaves the caller with an error and no live subscription rather
// than a subscription that will never deliver again. It is idempotent: a second
// call reports ErrPushClosed after the orchestrator is closed and otherwise does
// nothing and returns nil.
func (sub *Subscription) Cancel(ctx context.Context) error {
	o := sub.o

	o.subMu.Lock()
	if sub.isClosed() {
		o.subMu.Unlock()
		return nil
	}
	entry := o.detachLocked(sub)
	o.subMu.Unlock()

	sub.shutdown()

	if entry == nil || !entry.subscribed || o.market == nil {
		return nil
	}
	ctx, cancel := withCancelOnDone(ctx, o.closeCh)
	defer cancel()
	if err := o.market.Unsubscribe(ctx, entry.topic, entry.securities...); err != nil {
		o.report(&streamError{
			tag:   ErrSubscriptionLost,
			cause: fmt.Errorf("unsubscribe topicId %d for %s: %w", int(entry.topic), keyList(entry.securities), err),
		})
		return err
	}
	return nil
}

// isClosed reports whether the subscription's Updates channel is already closed.
func (sub *Subscription) isClosed() bool {
	sub.mu.Lock()
	defer sub.mu.Unlock()
	return sub.closed
}

// shutdown closes the Updates channel exactly once.
func (sub *Subscription) shutdown() {
	sub.mu.Lock()
	defer sub.mu.Unlock()
	if sub.closed {
		return
	}
	sub.closed = true
	close(sub.updates)
}

// deliver accepts one update, or drops the oldest to make room and says so.
//
// The subscription lock is held across the enqueue so that "the channel is full"
// and "the channel was closed" cannot interleave: shutdown takes the same lock
// before closing, so a frame is either enqueued before the close or dropped
// because the subscription is gone, never written to a channel that is closing.
//
// Out-of-order is decided against the last accepted frame's wire notifyTime, not
// against the last delivered one: with drop-oldest the newest queued frame is the
// most recent anyway, and the accepted-time comparison is the one that survives a
// reconnect boundary.
func (sub *Subscription) deliver(u *domain.PushUpdate) {
	sub.mu.Lock()
	if sub.closed {
		sub.mu.Unlock()
		return
	}

	var outOfOrder bool
	if !u.Time.IsZero() {
		if !sub.lastWire.IsZero() && u.Time.Before(sub.lastWire) {
			sub.outOfOrder++
			outOfOrder = sub.outOfOrder == 1
		} else {
			sub.lastWire = u.Time
		}
	}
	sub.lastSeen = sub.o.now()
	if ts, ok := eventTimestamp(u.Event); ok {
		sub.lastUpdate = ts
	}

	select {
	case sub.updates <- *u:
	default:
		select {
		case <-sub.updates:
		default:
		}
		select {
		case sub.updates <- *u:
		default:
		}
		sub.dropped++
	}
	sub.mu.Unlock()

	if outOfOrder {
		sub.o.report(&streamError{
			tag: ErrUpdateOutOfOrder,
			cause: fmt.Errorf("topicId %d accepted a frame stamped %s after one stamped %s; "+
				"the frame was delivered and the count is on Subscription.OutOfOrder",
				int(sub.topic), u.Time.UTC().Format(time.RFC3339Nano), sub.wireStamp()),
		})
	}
}

// wireStamp reads the last accepted wire timestamp for the out-of-order message.
// It is called after the lock is released, so it takes it again rather than racing
// on the field.
func (sub *Subscription) wireStamp() string {
	// The caller is the out-of-order branch of deliver, which only runs when
	// lastWire has been set, so the zero case needs no handling here.
	sub.mu.Lock()
	defer sub.mu.Unlock()
	return sub.lastWire.UTC().Format(time.RFC3339Nano)
}

// ---------------------------------------------------------------------------
// OrderSubscription
// ---------------------------------------------------------------------------

// OrderSubscription is one live session-wide order-status push subscription.
//
// It has no TopicID and no Securities accessor, and their absence is the
// statement: this subscription is enabled by a login, not by a topic and an
// instrument list. A bool field carrying "which notify types, whether the HTTP
// call is needed, whether to re-subscribe" would be a variant wearing a
// boolean's clothes, and a rule asserted on one branch of a variant is half a
// rule.
type OrderSubscription struct {
	o         *PushOrchestration
	accountID domain.AccountID

	updates chan domain.PushUpdate

	mu            sync.Mutex
	closed        bool
	dropped       uint64
	backlogRaised bool
	lastSeen      time.Time
}

// newOrderSubscription builds a trade subscription with the orchestrator's buffer
// depth.
func newOrderSubscription(o *PushOrchestration, accountID domain.AccountID) *OrderSubscription {
	return &OrderSubscription{
		o:         o,
		accountID: accountID,
		updates:   make(chan domain.PushUpdate, o.buffer),
	}
}

// Updates returns the channel of delivered fills and order-status updates. It is
// closed by Cancel, by a re-subscribe the Gateway refused, or by Close.
//
// Unlike a market subscription's channel, an overflow here is reported on the
// client stream as ErrTradeBacklog and counted on Dropped rather than passing
// unnoticed.
func (ord *OrderSubscription) Updates() <-chan domain.PushUpdate { return ord.updates }

// AccountID reports the account this subscription was created for. The Gateway's
// own scope is the session, so this is recorded rather than enforced: it names
// the caller in a re-subscribe failure.
func (ord *OrderSubscription) AccountID() domain.AccountID { return ord.accountID }

// Dropped reports how many fills were discarded because the update channel was
// full. It is expected to stay zero: a caller that drains is never asked to make
// this trade-off, and one that does not is told rather than left guessing.
func (ord *OrderSubscription) Dropped() uint64 {
	ord.mu.Lock()
	defer ord.mu.Unlock()
	return ord.dropped
}

// LastSeen reports when this subscription last accepted an update.
func (ord *OrderSubscription) LastSeen() time.Time {
	ord.mu.Lock()
	defer ord.mu.Unlock()
	return ord.lastSeen
}

// Cancel releases this subscription, issuing POST /trade/TradeUnsubscribe only
// when it was the last order subscription on the connection. It is idempotent.
func (ord *OrderSubscription) Cancel(ctx context.Context) error {
	o := ord.o

	o.subMu.Lock()
	if ord.isClosed() {
		o.subMu.Unlock()
		return nil
	}
	for i, s := range o.orders {
		if s == ord {
			o.orders = append(o.orders[:i], o.orders[i+1:]...)
			break
		}
	}
	o.tradeSubs--
	last := o.tradeSubs <= 0
	account := o.tradeAccount
	if last {
		o.tradeOn = false
	}
	trader := o.trade
	o.subMu.Unlock()

	ord.shutdown()

	if !last || trader == nil {
		return nil
	}
	ctx, cancel := withCancelOnDone(ctx, o.closeCh)
	defer cancel()
	if err := trader.UnsubscribeOrders(ctx, account); err != nil {
		o.report(&streamError{
			tag:   ErrSubscriptionLost,
			cause: fmt.Errorf("unsubscribe session-wide order push for account %s: %w", account, err),
		})
		return err
	}
	return nil
}

// isClosed reports whether the Updates channel is already closed.
func (ord *OrderSubscription) isClosed() bool {
	ord.mu.Lock()
	defer ord.mu.Unlock()
	return ord.closed
}

// shutdown closes the Updates channel exactly once.
func (ord *OrderSubscription) shutdown() {
	ord.mu.Lock()
	defer ord.mu.Unlock()
	if ord.closed {
		return
	}
	ord.closed = true
	close(ord.updates)
}

// deliver accepts one fill, or drops the oldest to make room and raises
// ErrTradeBacklog on the client stream the first time it has to.
//
// Reporting only the first overflow is deliberate: the stream is bounded and
// drop-oldest, so an overflow on every fill would crowd out the very drop
// reports it is meant to deliver. The counter keeps the total, and the caller can
// watch it.
func (ord *OrderSubscription) deliver(u *domain.PushUpdate, o *PushOrchestration) {
	ord.mu.Lock()
	if ord.closed {
		ord.mu.Unlock()
		return
	}
	ord.lastSeen = o.now()

	var overflow bool
	select {
	case ord.updates <- *u:
	default:
		select {
		case <-ord.updates:
		default:
		}
		select {
		case ord.updates <- *u:
		default:
		}
		ord.dropped++
		overflow = !ord.backlogRaised
		ord.backlogRaised = true
	}
	dropped := ord.dropped
	ord.mu.Unlock()

	if overflow {
		o.report(&streamError{
			tag: ErrTradeBacklog,
			cause: fmt.Errorf("order delivery for account %s overflowed its %d-entry buffer; "+
				"a fill was discarded and the running total is %d (Subscription.Dropped is the "+
				"per-subscription count; drain Updates or raise the buffer with services.WithBuffer)",
				ord.accountID, o.buffer, dropped),
		})
	}
}

// ---------------------------------------------------------------------------
// Errors
// ---------------------------------------------------------------------------

// streamError is the shape every error this package raises on the client stream
// takes: a package sentinel in Is and the underlying cause in Unwrap.
//
// It generalises the reconnectNotice mechanism A4 established for the released
// layer, and it keeps that mechanism's two measured properties. Is answers for
// the sentinel, because an error holds one position in a chain and the sentinel
// and the cause cannot both occupy Unwrap. Unwrap hands back the cause, so
// errors.Unwrap reaches it. In particular this is NOT a multi-%w wrap: a value
// with Unwrap() []error leaves errors.Unwrap returning nil, which silently
// falsifies exactly the mechanism callers are told to use.
type streamError struct {
	tag   error
	cause error
}

// Error renders the sentinel and the cause on one line, so a log line carries
// both and the sentinel stays greppable.
func (e *streamError) Error() string {
	if e.cause == nil {
		return e.tag.Error()
	}
	return e.tag.Error() + ": " + e.cause.Error()
}

// Is reports whether target is the sentinel this error was raised under.
func (e *streamError) Is(target error) bool { return target == e.tag }

// Unwrap returns the underlying cause.
func (e *streamError) Unwrap() error { return e.cause }

// newReconnectNotice builds the ErrReconnected notice for one completed
// reconnect.
//
// A nil cause yields the bare sentinel rather than a wrapped nil: the cause
// arrives from the transport, and rendering a missing cause must not be able to
// panic in a public API.
func newReconnectNotice(cause error) error {
	if cause == nil {
		return ErrReconnected
	}
	return &streamError{tag: ErrReconnected, cause: cause}
}

// ---------------------------------------------------------------------------
// Keys
// ---------------------------------------------------------------------------

// marketSuffixes are the market suffixes the Gateway appends to an instrument
// code. They are stripped when a routing key is normalised so that "00700.HK",
// "00700.hk" and "00700" all reach the same subscription, which is what makes it
// safe to match on a key the repository cannot define.
var marketSuffixes = []string{".HK", ".US", ".SZ", ".SH"}

// normaliseRoutingKey upper-cases an instrument code and strips a trailing
// market suffix, so both the suffixed and the bare spelling hash to one entry.
func normaliseRoutingKey(code string) string {
	u := strings.ToUpper(strings.TrimSpace(code))
	for _, suffix := range marketSuffixes {
		if len(u) > len(suffix) && strings.HasSuffix(u, suffix) {
			return strings.TrimSuffix(u, suffix)
		}
	}
	return u
}

// securityKeys returns the distinct normalised routing keys for an instrument
// list. An empty code yields no key: there is nothing to route on, so a
// subscription that asked for one is simply never matched.
func securityKeys(securities []*Security) []string {
	seen := make(map[string]struct{}, len(securities))
	out := make([]string, 0, len(securities))
	for _, s := range securities {
		if s == nil || s.Code == "" {
			continue
		}
		k := normaliseRoutingKey(s.Code)
		if _, dup := seen[k]; dup {
			continue
		}
		seen[k] = struct{}{}
		out = append(out, k)
	}
	return out
}

// updateRoutingKeys returns the candidate routing keys for one delivered update:
// its notifyId and the code inside its payload, both normalised.
//
// Both are returned and neither is discarded, and a caller is expected to treat
// a miss as a *question about the Gateway's key*, not as "no data": that is why
// ErrUnrouted is reported rather than logged and forgotten. FullCode is
// deliberately not a candidate - domain.NewSymbol appends a suffix to a code that
// already carries one, so it reads "00700.HK.HK" for a suffixed code.
func updateRoutingKeys(u *domain.PushUpdate) []string {
	out := make([]string, 0, 2)
	seen := make(map[string]struct{}, 2)
	for _, raw := range []string{u.ID, u.SecurityCode()} {
		if raw == "" {
			continue
		}
		k := normaliseRoutingKey(raw)
		if _, dup := seen[k]; dup {
			continue
		}
		seen[k] = struct{}{}
		out = append(out, k)
	}
	return out
}

// canonicalPushKey serialises a (topic, security set) into the registry's map
// key. The instruments are sorted by (Code, DataType) first, so two callers
// passing the same instruments in a different order produce one key and one
// /hq/Subscribe between them.
func canonicalPushKey(topic types.TopicID, securities []*Security) string {
	parts := make([]string, 0, len(securities))
	for _, s := range securities {
		parts = append(parts, s.Code+"\x00"+strconv.FormatInt(int64(s.DataType), 10))
	}
	sort.Strings(parts)
	return strconv.Itoa(int(topic)) + "\x00" + strings.Join(parts, "\x01")
}

// keyList renders an instrument list for an error message.
//
// Every instrument here reached the registry through copyAndValidateSecurities,
// which rejects a nil security and one with no code, and every caller passes a
// non-empty list, so nothing is filtered and the render cannot produce an empty or
// misleading name.
func keyList(securities []*Security) string {
	codes := make([]string, 0, len(securities))
	for _, s := range securities {
		codes = append(codes, s.Code)
	}
	return strings.Join(codes, ", ")
}

// copyAndValidateSecurities rejects a subscription whose instrument list is empty
// or malformed, before any request, and returns the copies the registry stores.
//
// The copies matter: this is the one place a caller-supplied pointer is read, so
// a caller mutating its own Security after Subscribe must not change what the
// registry keys on. An empty code is rejected here rather than passed down
// because there would be no routing key for it, and a subscription nothing can
// route to is the silent-no-data failure this layer exists to avoid.
func copyAndValidateSecurities(topic types.TopicID, securities []*Security) ([]*Security, error) {
	if len(securities) == 0 {
		return nil, errs.New(types.StatusInvalidParam, opPushSubscribe,
			fmt.Sprintf("topicId %d needs at least one security", int(topic)))
	}
	out := make([]*Security, 0, len(securities))
	for i, s := range securities {
		if s == nil {
			return nil, errs.New(types.StatusInvalidParam, opPushSubscribe,
				fmt.Sprintf("security[%d] must not be nil", i))
		}
		if s.Code == "" {
			return nil, errs.New(types.StatusInvalidParam, opPushSubscribe,
				fmt.Sprintf("security[%d] must carry a code", i))
		}
		cp := *s
		out = append(out, &cp)
	}
	return out, nil
}

// eventTimestamp reads an event's own Timestamp field, reporting whether the
// event type has one.
func eventTimestamp(ev domain.PushEvent) (string, bool) {
	switch e := ev.(type) {
	case domain.QuoteEvent:
		return e.Timestamp, true
	case domain.TickerEvent:
		return e.Timestamp, true
	case domain.OrderBookEvent:
		return e.Timestamp, true
	case domain.BrokerEvent:
		return e.Timestamp, true
	case domain.TradeEvent:
		return e.Timestamp, true
	default:
		return "", false
	}
}

// unmappedTopicMessage explains a topic this SDK does not map, naming every
// payload type that *is* deliverable and the function that holds the mapping.
//
// It names all four rather than a couple, so a reader cannot take the list as
// evidence that only those are possible, and it names NotifyTypeForTopic so the
// error is a complete answer rather than a dead end.
func unmappedTopicMessage(topic types.TopicID) string {
	return fmt.Sprintf(
		"topicId %d has no push payload this SDK can deliver; the deliverable payload types are %s; "+
			"consult services.NotifyTypeForTopic, which holds the docs/SPEC.md section 4 mapping",
		int(topic), strings.Join(deliverablePayloadNames(), ", "))
}

// withCancelOnDone derives a context that is cancelled when ctx is or when done
// closes, so a Cancel racing Close cannot leave an HTTP request in flight against
// a Gateway the caller has released.
func withCancelOnDone(ctx context.Context, done <-chan struct{}) (context.Context, context.CancelFunc) {
	derived, cancel := context.WithCancel(ctx)
	go func() {
		select {
		case <-done:
			cancel()
		case <-derived.Done():
		}
	}()
	return derived, cancel
}
