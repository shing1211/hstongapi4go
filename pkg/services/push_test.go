// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package services

import (
	"context"
	"errors"
	"io"
	"net"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/shing1211/hstongapi4go/internal/errs"
	"github.com/shing1211/hstongapi4go/pkg/domain"
	"github.com/shing1211/hstongapi4go/pkg/types"
)

// The tests here drive PushOrchestration against fakes for both halves.
//
// Nothing sleeps, and nothing waits on a clock to get somewhere. Every
// asynchronous edge is crossed by an explicit handshake:
//
//   - the fake transport's handlers are invoked on the test's own goroutine, so
//     dispatch is synchronous and needs no gate at all;
//   - the fake transport's reconnect hooks are the same, so a reconnect and its
//     re-subscribe are complete before the next assertion;
//   - the one place a goroutine is unavoidable - a re-subscribe that must be
//     interrupted mid-flight - is joined with a WaitGroup, and the moment to act
//     is taken from a channel the fake signals on entry rather than from a delay;
//   - the clock is injected, so Stale and LastSeen are asserted without waiting.
//
// No test calls recover(). A panic inside the orchestrator's own goroutine ends
// the process and fails everything, which is a cleaner outcome than a test that
// installs a recover and hides it.

// ---------------------------------------------------------------------------
// Fakes
// ---------------------------------------------------------------------------

// fakePushTransport is a PushTransport a test can drive. It implements the seam
// with no import beyond pkg/domain and pkg/types, which is only possible because
// the seam's payload type is a public one: a signature naming internal/push could
// not be implemented here at all.
type fakePushTransport struct {
	mu       sync.Mutex
	handlers map[types.NotifyMsgType]func(*domain.PushUpdate)
	hooks    []func(error)
	dials    int
	closed   bool

	connectErr error
	runErr     error
	connected  bool

	errc          chan error
	runGate       chan struct{}
	runCalled     chan struct{}
	streamOnce    sync.Once
	closeGateOnce sync.Once
}

// newFakePushTransport returns a transport with a recorded handler registry and
// a Run that parks until Close releases it.
func newFakePushTransport() *fakePushTransport {
	return &fakePushTransport{
		handlers:  make(map[types.NotifyMsgType]func(*domain.PushUpdate)),
		errc:      make(chan error, 64),
		runGate:   make(chan struct{}),
		runCalled: make(chan struct{}, 4),
	}
}

// Connect records the dial and returns the scripted error. It is idempotent while
// already connected, which is the contract PushOrchestration.Connect documents and
// the one the real push client implements - so a test that calls it twice is
// measuring the orchestrator's pass-through rather than the transport's behaviour.
func (f *fakePushTransport) Connect(context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.closed {
		return errors.New("fake: closed")
	}
	if f.connected {
		return nil
	}
	f.connected = true
	f.dials++
	return f.connectErr
}

// Run signals that it was called and then parks until Close, so a test can hold
// a client in its running state without a goroutine of its own.
func (f *fakePushTransport) Run(ctx context.Context) error {
	select {
	case f.runCalled <- struct{}{}:
	default:
	}
	select {
	case <-f.runGate:
	case <-ctx.Done():
		return ctx.Err()
	}
	return f.runErr
}

// Close releases Run and closes the error stream, exactly as the real transport
// does. It is idempotent, and it shares the stream's close with closeErrStream so
// a test that closes the stream itself does not panic here.
func (f *fakePushTransport) Close() error {
	f.closeGateOnce.Do(func() { close(f.runGate) })
	f.closeErrStream()
	f.mu.Lock()
	f.closed = true
	f.mu.Unlock()
	return nil
}

// closeErrStream closes the error stream exactly once, from either the test or
// Close. Closing it is the handshake a test needs: the orchestrator's drain
// goroutine exits when it sees the close, so joining the drain goroutine after
// this is a barrier over every error already sent.
func (f *fakePushTransport) closeErrStream() {
	f.streamOnce.Do(func() { close(f.errc) })
}

// Errors returns the recorded error stream.
func (f *fakePushTransport) Errors() <-chan error { return f.errc }

// OnReconnect records the hook.
func (f *fakePushTransport) OnReconnect(fn func(error)) {
	if fn == nil {
		return
	}
	f.mu.Lock()
	f.hooks = append(f.hooks, fn)
	f.mu.Unlock()
}

// SubscribeTypes records the handler, replacing any previous one for the type.
func (f *fakePushTransport) SubscribeTypes(t types.NotifyMsgType, h func(*domain.PushUpdate)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if h == nil {
		delete(f.handlers, t)
		return
	}
	f.handlers[t] = h
}

// handler returns the installed handler for t, or nil.
func (f *fakePushTransport) handler(t types.NotifyMsgType) func(*domain.PushUpdate) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.handlers[t]
}

// installed reports how many handlers are registered.
func (f *fakePushTransport) installed() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.handlers)
}

// fireReconnect invokes every recorded hook, synchronously on the caller's
// goroutine. It returns how many hooks ran, so a test can assert the orchestrator
// registered exactly one.
func (f *fakePushTransport) fireReconnect(cause error) int {
	f.mu.Lock()
	hooks := make([]func(error), len(f.hooks))
	copy(hooks, f.hooks)
	f.mu.Unlock()
	for _, fn := range hooks {
		fn(cause)
	}
	return len(hooks)
}

// raise puts err on the transport's error stream, as the real transport does for
// a read error, a decode failure or a verification verdict.
func (f *fakePushTransport) raise(err error) {
	f.errc <- err
}

// fakeTopicCall is one recorded /hq/Subscribe or /hq/Unsubscribe.
type fakeTopicCall struct {
	topic types.TopicID
	// codes are the instrument codes, copied out of the caller's pointers.
	codes []string
}

// fakeTopicSubscriber is the HTTP half. It records every call, can fail a chosen
// call index, and can hold a call open so a test can act while it is in flight.
type fakeTopicSubscriber struct {
	mu           sync.Mutex
	subscribes   []fakeTopicCall
	unsubscribes []fakeTopicCall
	subErr       map[int]error
	unsubErr     map[int]error

	// gate, when non-nil, is closed by a test to release every blocked Subscribe.
	gate chan struct{}
	// entered receives once per Subscribe call that reached the blocking step.
	entered chan struct{}
	// ctxErr records the context error each Subscribe saw on release.
	ctxErr []error
}

// newFakeTopicSubscriber returns an HTTP half that accepts everything.
func newFakeTopicSubscriber() *fakeTopicSubscriber {
	return &fakeTopicSubscriber{
		subErr:   make(map[int]error),
		unsubErr: make(map[int]error),
		entered:  make(chan struct{}, 16),
	}
}

// Subscribe records the call and answers the scripted outcome for its index.
//
// The entered signal fires only while the fake is holding the call, so it is a
// handshake about *this* call rather than a token left over from an earlier one -
// which is what makes "act while the request is in flight" reproducible.
func (f *fakeTopicSubscriber) Subscribe(ctx context.Context, topic types.TopicID, sec ...*Security) error {
	f.mu.Lock()
	f.subscribes = append(f.subscribes, recordTopicCall(topic, sec))
	call := len(f.subscribes)
	err := f.subErr[call]
	gate := f.gate
	f.mu.Unlock()

	if gate != nil {
		select {
		case f.entered <- struct{}{}:
		default:
		}
		select {
		case <-gate:
		case <-ctx.Done():
			f.mu.Lock()
			f.ctxErr = append(f.ctxErr, ctx.Err())
			f.mu.Unlock()
			return ctx.Err()
		}
	}
	return err
}

// Unsubscribe records the call and answers the scripted outcome for its index.
func (f *fakeTopicSubscriber) Unsubscribe(_ context.Context, topic types.TopicID, sec ...*Security) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.unsubscribes = append(f.unsubscribes, recordTopicCall(topic, sec))
	return f.unsubErr[len(f.unsubscribes)]
}

// counts reports how many subscribe and unsubscribe calls were recorded.
func (f *fakeTopicSubscriber) counts() (subs, unsubs int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.subscribes), len(f.unsubscribes)
}

// lastSubscribeCodes reports the instrument codes of the most recent Subscribe.
func (f *fakeTopicSubscriber) lastSubscribeCodes() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.subscribes) == 0 {
		return nil
	}
	return f.subscribes[len(f.subscribes)-1].codes
}

// holdSubscribe makes every subsequent Subscribe block until gate is closed or its
// context is cancelled, and signals entered so a test can act mid-call.
func (f *fakeTopicSubscriber) holdSubscribe(gate chan struct{}) {
	f.mu.Lock()
	f.gate = gate
	f.mu.Unlock()
}

// releaseSubscribe lets held calls through.
func (f *fakeTopicSubscriber) releaseSubscribe() {
	f.mu.Lock()
	gate := f.gate
	f.gate = nil
	f.mu.Unlock()
	if gate != nil {
		close(gate)
	}
}

// sawContextError reports the context error the first held Subscribe observed.
func (f *fakeTopicSubscriber) sawContextError() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.ctxErr) == 0 {
		return nil
	}
	return f.ctxErr[0]
}

// recordTopicCall copies a call out of a caller-supplied instrument list, so a
// recorded call cannot change under the test's feet.
func recordTopicCall(topic types.TopicID, sec []*Security) fakeTopicCall {
	call := fakeTopicCall{topic: topic, codes: make([]string, 0, len(sec))}
	for _, s := range sec {
		call.codes = append(call.codes, s.Code)
	}
	return call
}

// fakeOrderSubscriber is the session-wide trade half.
type fakeOrderSubscriber struct {
	mu       sync.Mutex
	subs     int
	unsubs   int
	subErr   map[int]error
	unsubErr map[int]error
	gate     chan struct{}
	entered  chan struct{}
	ctxErr   []error
}

// newFakeOrderSubscriber returns a trade half that accepts everything.
func newFakeOrderSubscriber() *fakeOrderSubscriber {
	return &fakeOrderSubscriber{
		subErr:   make(map[int]error),
		unsubErr: make(map[int]error),
		entered:  make(chan struct{}, 16),
	}
}

// SubscribeOrders records the call and answers the scripted outcome for its index.
func (f *fakeOrderSubscriber) SubscribeOrders(ctx context.Context, _ domain.AccountID) error {
	f.mu.Lock()
	f.subs++
	call := f.subs
	err := f.subErr[call]
	gate := f.gate
	f.mu.Unlock()

	if gate != nil {
		select {
		case f.entered <- struct{}{}:
		default:
		}
		select {
		case <-gate:
		case <-ctx.Done():
			f.mu.Lock()
			f.ctxErr = append(f.ctxErr, ctx.Err())
			f.mu.Unlock()
			return ctx.Err()
		}
	}
	return err
}

// UnsubscribeOrders records the call and answers the scripted outcome for its index.
func (f *fakeOrderSubscriber) UnsubscribeOrders(_ context.Context, _ domain.AccountID) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.unsubs++
	return f.unsubErr[f.unsubs]
}

// counts reports the subscribe and unsubscribe totals.
func (f *fakeOrderSubscriber) counts() (subs, unsubs int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.subs, f.unsubs
}

// holdSubscribe makes every subsequent SubscribeOrders block until gate closes or
// the context is cancelled.
func (f *fakeOrderSubscriber) holdSubscribe(gate chan struct{}) {
	f.mu.Lock()
	f.gate = gate
	f.mu.Unlock()
}

// releaseSubscribe lets held calls through.
func (f *fakeOrderSubscriber) releaseSubscribe() {
	f.mu.Lock()
	gate := f.gate
	f.gate = nil
	f.mu.Unlock()
	if gate != nil {
		close(gate)
	}
}

// sawContextError reports the context error the first held SubscribeOrders saw.
func (f *fakeOrderSubscriber) sawContextError() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.ctxErr) == 0 {
		return nil
	}
	return f.ctxErr[0]
}

// ---------------------------------------------------------------------------
// Shared helpers
// ---------------------------------------------------------------------------

// pushTestAccount is the account every trade subscription test uses.
func pushTestAccount() domain.AccountID { return domain.AccountID("HST-1") }

// newTestOrchestration builds an orchestration over a fake transport and the given
// HTTP halves, with an injected clock and cleanup registered.
//
// A nil market or trade half is left nil on purpose: several tests want the
// orchestrator in the shape where that half was never configured.
func newTestOrchestration(t *testing.T, tr *fakePushTransport, opts ...PushOption) *PushOrchestration {
	t.Helper()
	market := newFakeTopicSubscriber()
	trade := newFakeOrderSubscriber()
	return newTestOrchestrationWith(t, market, trade, tr, opts...)
}

// newTestOrchestrationWith is newTestOrchestration with both HTTP halves supplied,
// for the tests that assert on the requests those halves received.
func newTestOrchestrationWith(t *testing.T, market TopicSubscriber, trade OrderPushSubscriber, tr *fakePushTransport, opts ...PushOption) *PushOrchestration {
	t.Helper()
	all := append([]PushOption{WithTradeSubscriber(trade)}, opts...)
	o := NewPushOrchestration(market, tr, all...)
	// Cleanup runs last-registered-first, so this runs before any gate a test
	// closed and after nothing else matters: Close is idempotent and releases the
	// transport.
	t.Cleanup(func() { _ = o.Close() })
	return o
}

// waitBound is the ceiling a test waits on an event. Reaching it is always a
// failure, never a way to reach a code path.
const waitBound = 30 * time.Second

// fakeClock is the injected clock the freshness assertions read.
type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

// newFakeClock returns a clock fixed at a recognisable instant.
func newFakeClock() *fakeClock {
	return &fakeClock{now: time.Date(2026, 1, 26, 9, 30, 0, 0, time.UTC)}
}

// Now reports the clock's current instant.
func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

// advance moves the clock forward.
func (c *fakeClock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

// pushTestTime converts a millisecond offset into a UTC instant, so a test can
// name a wire notifyTime without a clock read.
func pushTestTime(ms int64) time.Time { return time.UnixMilli(ms).UTC() }

// syntheticMarketEvent builds a domain event of the type msgType names, carrying
// code as its instrument. It exists so a routing assertion is about the routing
// and not about which payload a decoder produced.
func syntheticMarketEvent(msgType types.NotifyMsgType, code string) domain.PushEvent {
	sym := domain.NewSymbol(domain.MarketFromCode(code), code, types.DataTypeHKStock)
	switch msgType {
	case types.TickerNotifyMsgType:
		return domain.TickerEvent{Symbol: sym, Timestamp: "20260126 09:30:01"}
	case types.OrderBookNotifyMsgType:
		return domain.OrderBookEvent{Symbol: sym, Timestamp: "20260126 09:30:01"}
	case types.BrokerQueueNotifyMsgType:
		return domain.BrokerEvent{Symbol: sym, Timestamp: "20260126 09:30:01"}
	default:
		return domain.QuoteEvent{Symbol: sym, Timestamp: "20260126 09:30:01"}
	}
}

// syntheticTradeEvent builds a fill carrying stockCode as its instrument.
func syntheticTradeEvent(stockCode string) domain.PushEvent {
	sym := domain.NewSymbol(domain.MarketFromCode(stockCode), stockCode, types.DataTypeHKStock)
	return domain.TradeEvent{Symbol: sym, CounterID: "M1", Timestamp: "20260126 09:30:01"}
}

// nextClientError reads one error off a client's stream, failing the test when none
// arrives.
//
// It is the blocking form, for the errors the transport's own goroutine forwards.
func nextClientError(t *testing.T, o *PushOrchestration) error {
	t.Helper()
	select {
	case err, ok := <-o.Errors():
		if !ok {
			t.Fatal("the client error stream closed, want an error")
		}
		return err
	case <-time.After(waitBound):
		t.Fatal("timed out waiting for a client error")
		return nil
	}
}

// requireClientErrorNow reads one error that the calling goroutine's own action put
// on the stream, and fails immediately if it is not there.
//
// Every error this file's orchestrator reports in response to a synchronous action -
// a fired reconnect hook, a delivered frame, an overflow - is placed on the stream
// before that action returns, so a receive that has to wait for one is waiting for
// something that will never come. Failing on the spot says which, and says it in
// milliseconds instead of after the harness bound.
//
// nextClientError is the other form and is the right one for an error the transport's
// own goroutine forwards; using this one there would be a race.
func requireClientErrorNow(t *testing.T, o *PushOrchestration, what string) error {
	t.Helper()
	select {
	case err, ok := <-o.Errors():
		if !ok {
			t.Fatalf("the client error stream closed while waiting for %s", what)
		}
		return err
	default:
		t.Fatalf("no error was reported for %s: the client stream is empty immediately after the "+
			"action that must raise one", what)
		return nil
	}
}

// drainClientErrors returns the errors already queued, without blocking, so a
// test can assert that nothing *else* was reported.
func drainClientErrors(o *PushOrchestration) []error {
	var out []error
	for {
		select {
		case err, ok := <-o.Errors():
			if !ok {
				return out
			}
			out = append(out, err)
		default:
			return out
		}
	}
}

// waitClosed asserts that a subscription's Updates channel closes, which is the
// only way a caller learns its subscription is gone.
//
// It is the blocking form, for a closure another goroutine performs.
func waitClosed(t *testing.T, updates <-chan domain.PushUpdate) {
	t.Helper()
	for {
		select {
		case _, open := <-updates:
			if !open {
				return
			}
		case <-time.After(waitBound):
			t.Fatal("timed out waiting for Updates to close")
			return
		}
	}
}

// requireClosedNow asserts that a subscription's Updates channel is already closed,
// for a closure the calling goroutine's own action performed.
//
// A subscription the orchestrator kills during a reconnect is shut down before the
// reconnect hook returns, so a receive that has to wait is waiting for something that
// will never come. Failing on the spot names the condition and says so in
// milliseconds; the blocking form is the right one for a closure Close performs on
// its own goroutine.
func requireClosedNow(t *testing.T, updates <-chan domain.PushUpdate, what string) {
	t.Helper()
	// Drain first: a killed subscription may still hold undelivered updates, and the
	// closure is what the assertion is about.
	for {
		select {
		case _, open := <-updates:
			if !open {
				return
			}
			continue
		default:
		}
		t.Fatalf("Updates is still open after %s: the subscription was not cancelled", what)
	}
}

// ---------------------------------------------------------------------------
// T2/T3 - subscribing, coalescing, cancelling
// ---------------------------------------------------------------------------

// TestSubscribeCoalescesByTopicAndInstrumentSet is the reference-count rule.
//
// Two subscribers of the same topic and the same instruments - passed in opposite
// orders, because Subscribe takes a variadic list and two callers may name them
// differently - must produce one /hq/Subscribe. Cancelling either one must produce
// no /hq/Unsubscribe, because the other is still using the stream. Cancelling the
// last must produce exactly one.
//
// The order-insensitivity is the load-bearing half: a key built by appending in
// call order would give two keys, two requests, and the first cancel would tear
// down a stream the second caller is still reading - silently, because nothing
// reports an unsubscribe another subscriber still wanted.
func TestSubscribeCoalescesByTopicAndInstrumentSet(t *testing.T) {
	tr := newFakePushTransport()
	market := newFakeTopicSubscriber()
	o := newTestOrchestrationWith(t, market, newFakeOrderSubscriber(), tr)

	a := &Security{DataType: types.DataTypeHKStock, Code: "00700.HK"}
	b := &Security{DataType: types.DataTypeUSStock, Code: "AAPL.US"}

	first, err := o.Subscribe(t.Context(), types.TopicBasicQot, a, b)
	if err != nil {
		t.Fatalf("first Subscribe: %v", err)
	}
	second, err := o.Subscribe(t.Context(), types.TopicBasicQot, b, a)
	if err != nil {
		t.Fatalf("second Subscribe: %v", err)
	}

	if subs, unsubs := market.counts(); subs != 1 || unsubs != 0 {
		t.Fatalf("after two subscribers of one key: subscribe=%d unsubscribe=%d, want 1/0", subs, unsubs)
	}
	if got := market.lastSubscribeCodes(); len(got) != 2 {
		t.Errorf("the /hq/Subscribe carried %v, want both instruments", got)
	}

	if err := first.Cancel(t.Context()); err != nil {
		t.Fatalf("first Cancel: %v", err)
	}
	if subs, unsubs := market.counts(); subs != 1 || unsubs != 0 {
		t.Errorf("after cancelling one of two: subscribe=%d unsubscribe=%d, want 1/0: the other "+
			"subscriber is still using the stream", subs, unsubs)
	}

	if err := second.Cancel(t.Context()); err != nil {
		t.Fatalf("second Cancel: %v", err)
	}
	if subs, unsubs := market.counts(); subs != 1 || unsubs != 1 {
		t.Errorf("after cancelling the last: subscribe=%d unsubscribe=%d, want 1/1", subs, unsubs)
	}

	// Cancelling again is a no-op rather than a second request.
	if err := second.Cancel(t.Context()); err != nil {
		t.Errorf("second Cancel again = %v, want nil: Cancel is idempotent", err)
	}
	if _, unsubs := market.counts(); unsubs != 1 {
		t.Errorf("unsubscribe count = %d after a repeated Cancel, want 1", unsubs)
	}
}

// TestSubscribeKeepsDistinctKeysApart is the negative half of the coalescing rule:
// a different topic, or the same topic with a different instrument set, is a
// different key and must get its own request.
func TestSubscribeKeepsDistinctKeysApart(t *testing.T) {
	tr := newFakePushTransport()
	market := newFakeTopicSubscriber()
	o := newTestOrchestrationWith(t, market, newFakeOrderSubscriber(), tr)

	a := &Security{DataType: types.DataTypeHKStock, Code: "00700.HK"}
	b := &Security{DataType: types.DataTypeUSStock, Code: "AAPL.US"}

	if _, err := o.Subscribe(t.Context(), types.TopicBasicQot, a); err != nil {
		t.Fatalf("Subscribe(topic, a): %v", err)
	}
	if _, err := o.Subscribe(t.Context(), types.TopicBasicQot, b); err != nil {
		t.Fatalf("Subscribe(topic, b): %v", err)
	}
	if _, err := o.Subscribe(t.Context(), types.TopicTicker, a); err != nil {
		t.Fatalf("Subscribe(other topic, a): %v", err)
	}
	if subs, _ := market.counts(); subs != 3 {
		t.Errorf("subscribe count = %d, want 3: the key is (topic, instrument set)", subs)
	}
}

// TestSubscribeStoresCopiesOfTheCallerInstruments pins the copy rule.
//
// This is the one place a caller-supplied pointer is read, so the registry must
// hold copies: a caller that mutates its own Security after Subscribe must not
// change what the registry keys on, or its next Subscribe of what it thinks is the
// same instrument would open a second stream.
func TestSubscribeStoresCopiesOfTheCallerInstruments(t *testing.T) {
	tr := newFakePushTransport()
	market := newFakeTopicSubscriber()
	o := newTestOrchestrationWith(t, market, newFakeOrderSubscriber(), tr)

	caller := &Security{DataType: types.DataTypeHKStock, Code: "00700.HK"}
	sub, err := o.Subscribe(t.Context(), types.TopicBasicQot, caller)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	caller.Code = "MUTATED"

	if got := sub.Securities(); len(got) != 1 || got[0].Code != "00700.HK" {
		t.Fatalf("Securities() = %v, want the code as subscribed", got)
	}
	// Mutating the returned slice must not reach the subscription either.
	securities := sub.Securities()
	securities[0].Code = "ALSO MUTATED"
	if got := sub.Securities(); got[0].Code != "00700.HK" {
		t.Errorf("Securities() = %v after the caller mutated the copy, want the stored value", got)
	}

	// And the mutated caller's new code must open a second stream rather than
	// silently joining the first.
	if _, err := o.Subscribe(t.Context(), types.TopicBasicQot, caller); err != nil {
		t.Fatalf("Subscribe(mutated): %v", err)
	}
	if subs, _ := market.counts(); subs != 2 {
		t.Errorf("subscribe count = %d, want 2: the registry keyed on the mutated caller pointer", subs)
	}
}

// TestSubscribeRejectsBadInputWithoutARequest covers every local rejection, and
// asserts each costs zero HTTP.
//
// The unmapped-topic row is the one §5.5 settles: a guess would hand the caller
// events of a type it mis-parses with every accessor, and nothing would report it,
// so the topic is refused locally with a message naming every payload type that
// *is* deliverable.
func TestSubscribeRejectsBadInputWithoutARequest(t *testing.T) {
	cases := []struct {
		name   string
		topic  types.TopicID
		secs   []*Security
		wantOp string
		substr string
	}{
		{
			name:   "a topic with no payload",
			topic:  types.TopicID(999),
			secs:   []*Security{{DataType: types.DataTypeHKStock, Code: "00700.HK"}},
			wantOp: opPushSubscribe,
			// The message must name every deliverable payload type, so a reader
			// cannot infer from the list that only some are possible.
			substr: "BasicQotNotifyMsgType, BrokerQueueNotifyMsgType, OrderBookNotifyMsgType, TickerNotifyMsgType",
		},
		{
			name:   "no instruments",
			topic:  types.TopicBasicQot,
			secs:   nil,
			wantOp: opPushSubscribe,
			substr: "at least one security",
		},
		{
			name:   "a nil instrument",
			topic:  types.TopicBasicQot,
			secs:   []*Security{nil},
			wantOp: opPushSubscribe,
			substr: "must not be nil",
		},
		{
			name:   "an instrument with no code",
			topic:  types.TopicBasicQot,
			secs:   []*Security{{DataType: types.DataTypeHKStock}},
			wantOp: opPushSubscribe,
			substr: "must carry a code",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tr := newFakePushTransport()
			market := newFakeTopicSubscriber()
			o := newTestOrchestrationWith(t, market, newFakeOrderSubscriber(), tr)

			sub, err := o.Subscribe(t.Context(), tc.topic, tc.secs...)
			if err == nil {
				t.Fatal("Subscribe = nil error, want a local rejection")
			}
			if sub != nil {
				t.Error("a subscription was handed back alongside the error")
			}
			e := assertInvalidParam(t, err, tc.wantOp)
			if !strings.Contains(e.Error(), tc.substr) {
				t.Errorf("error = %q, want it to say %q", e.Error(), tc.substr)
			}
			if subs, _ := market.counts(); subs != 0 {
				t.Errorf("%d request(s) reached the Gateway for an input this package rejected itself", subs)
			}
		})
	}
}

// TestSubscribeReportsAGatewayRefusalWithoutRegistering covers the one failure the
// package does not decide itself.
func TestSubscribeReportsAGatewayRefusalWithoutRegistering(t *testing.T) {
	tr := newFakePushTransport()
	market := newFakeTopicSubscriber()
	market.subErr[1] = errs.New(types.StatusServiceBusy, opSubscribe, "busy")
	o := newTestOrchestrationWith(t, market, newFakeOrderSubscriber(), tr)

	sub, err := o.Subscribe(t.Context(), types.TopicBasicQot, &Security{DataType: types.DataTypeHKStock, Code: "00700.HK"})
	if err == nil {
		t.Fatal("Subscribe = nil error, want the Gateway refusal")
	}
	if sub != nil {
		t.Error("a subscription was handed back alongside the error")
	}
	if code, ok := errs.CodeOf(err); !ok || code != types.StatusServiceBusy {
		t.Errorf("error = %v (code %q, ok=%v), want StatusServiceBusy", err, code, ok)
	}

	// Nothing is registered, so the key is still free and a later subscriber is
	// still the first one - and must therefore still issue the request.
	if _, err := o.Subscribe(t.Context(), types.TopicBasicQot, &Security{DataType: types.DataTypeHKStock, Code: "00700.HK"}); err != nil {
		t.Fatalf("second Subscribe = %v, want nil: the refused first call left the key free", err)
	}
	if subs, _ := market.counts(); subs != 2 {
		t.Errorf("subscribe count = %d, want 2: a refused subscribe must not count as established", subs)
	}
}

// TestSubscribeAfterCloseRefusesWithoutARequest pins the lifecycle guard.
func TestSubscribeAfterCloseRefusesWithoutARequest(t *testing.T) {
	tr := newFakePushTransport()
	market := newFakeTopicSubscriber()
	o := newTestOrchestrationWith(t, market, newFakeOrderSubscriber(), tr)
	if err := o.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	sub, err := o.Subscribe(t.Context(), types.TopicBasicQot, &Security{DataType: types.DataTypeHKStock, Code: "00700.HK"})
	if !errors.Is(err, ErrPushClosed) {
		t.Errorf("Subscribe after Close = %v, want ErrPushClosed", err)
	}
	if sub != nil {
		t.Error("a subscription was handed back after Close")
	}
	if subs, _ := market.counts(); subs != 0 {
		t.Errorf("%d request(s) reached the Gateway after Close", subs)
	}
	if ord, err := o.SubscribeOrders(t.Context(), pushTestAccount()); !errors.Is(err, ErrPushClosed) || ord != nil {
		t.Errorf("SubscribeOrders after Close = (%v, %v), want (nil, ErrPushClosed)", ord, err)
	}
	if err := o.Connect(t.Context()); !errors.Is(err, ErrPushClosed) {
		t.Errorf("Connect after Close = %v, want ErrPushClosed", err)
	}
	if err := o.Run(t.Context()); !errors.Is(err, ErrPushClosed) {
		t.Errorf("Run after Close = %v, want ErrPushClosed", err)
	}
}

// ---------------------------------------------------------------------------
// Handler installation
// ---------------------------------------------------------------------------

// TestTheHandlerSetIsTheImageOfTheMappingPlusTheDeliveries pins §6.1: the TCP
// handler set is a fixed set installed once, derived rather than declared.
//
// Seven registrations is the arithmetic the design states: the four market payload
// types the eleven topics collapse onto, plus the three session-wide deliveries
// that have no topic at all.
func TestTheHandlerSetIsTheImageOfTheMappingPlusTheDeliveries(t *testing.T) {
	tr := newFakePushTransport()
	newTestOrchestration(t, tr)

	if got := tr.installed(); got != 7 {
		t.Errorf("installed handlers = %d, want 7 (4 market payloads + 3 deliveries): %v",
			got, installedTypes(tr))
	}
	for _, msgType := range tradeNotifyTypes {
		if tr.handler(msgType) == nil {
			t.Errorf("no handler for the delivery type %s", msgType.String())
		}
	}
	if len(marketNotifyTypes()) != 4 {
		t.Errorf("the image of NotifyTypeForTopic has %d payload types, want 4", len(marketNotifyTypes()))
	}
}

// installedTypes names the types a fake transport has handlers for.
func installedTypes(tr *fakePushTransport) []string {
	tr.mu.Lock()
	defer tr.mu.Unlock()
	out := make([]string, 0, len(tr.handlers))
	for t := range tr.handlers {
		out = append(out, t.String())
	}
	return out
}

// ---------------------------------------------------------------------------
// T11 - routing
// ---------------------------------------------------------------------------

// TestRoutingMatchesBothCandidateKeys covers §11.3's dual index.
//
// The repository cannot say whether the Gateway's notifyId is the same string as
// the security code sent to /hq/Subscribe, so both are indexed, both are tried, and
// neither is discarded. Each row here is a way the two spellings could disagree
// while both still meaning the same instrument.
//
// The last two rows are the load-bearing ones: each can only be delivered by ONE of
// the two candidates, and each has that candidate spelled differently from the code
// that was subscribed. A dual index that picked one key and discarded the other
// would fail them, and so would a dual index that stopped normalising the keys -
// which is why they are not folded into the rows above.
func TestRoutingMatchesBothCandidateKeys(t *testing.T) {
	cases := []struct {
		name       string
		subscribed string
		notifyID   string
		eventCode  string
	}{
		{name: "both spellings agree", subscribed: "00700.HK", notifyID: "00700.HK", eventCode: "00700.HK"},
		{name: "notifyId is the bare code", subscribed: "00700.HK", notifyID: "00700", eventCode: "00700.HK"},
		{name: "notifyId is suffixed, the event code is bare", subscribed: "00700", notifyID: "00700.HK", eventCode: "00700"},
		{name: "only the notifyId matches", subscribed: "00700.HK", notifyID: "00700.HK", eventCode: "SOMETHING.ELSE"},
		{name: "only the payload code matches", subscribed: "00700.HK", notifyID: "00001.HK", eventCode: "00700.HK"},
		{name: "case differs", subscribed: "00700.HK", notifyID: "00700.hk", eventCode: "00700.HK"},
		{
			name:       "only the notifyId can match, and it is spelled differently",
			subscribed: "00700",
			notifyID:   "00700.HK",
			eventCode:  "SOMETHING.ELSE",
		},
		{
			name:       "only the payload code can match, and it is spelled differently",
			subscribed: "00700",
			notifyID:   "SOMETHING.ELSE",
			eventCode:  "00700.HK",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tr := newFakePushTransport()
			market := newFakeTopicSubscriber()
			o := newTestOrchestrationWith(t, market, newFakeOrderSubscriber(), tr)

			sub, err := o.Subscribe(t.Context(), types.TopicBasicQot,
				&Security{DataType: types.DataTypeHKStock, Code: tc.subscribed})
			if err != nil {
				t.Fatalf("Subscribe: %v", err)
			}
			tr.handler(types.BasicQotNotifyMsgType)(&domain.PushUpdate{
				Type:  types.BasicQotNotifyMsgType,
				ID:    tc.notifyID,
				Time:  pushTestTime(1_700_000_000_000),
				Event: syntheticMarketEvent(types.BasicQotNotifyMsgType, tc.eventCode),
			})

			select {
			case _, open := <-sub.Updates():
				if !open {
					t.Fatal("Updates closed instead of delivering")
				}
			default:
				t.Fatalf("nothing delivered: subscribe=%q notifyId=%q eventCode=%q", tc.subscribed, tc.notifyID, tc.eventCode)
			}
			if got := o.Unrouted(); got != 0 {
				t.Errorf("Unrouted = %d, want 0 for an update that was delivered", got)
			}
		})
	}
}

// TestRoutingDeliversToEveryMatchingSubscriber covers fan-out: two subscriptions
// of the same instrument on the same topic each get the frame.
func TestRoutingDeliversToEveryMatchingSubscriber(t *testing.T) {
	tr := newFakePushTransport()
	market := newFakeTopicSubscriber()
	o := newTestOrchestrationWith(t, market, newFakeOrderSubscriber(), tr)

	sec := &Security{DataType: types.DataTypeHKStock, Code: "00700.HK"}
	first, err := o.Subscribe(t.Context(), types.TopicBasicQot, sec)
	if err != nil {
		t.Fatalf("first Subscribe: %v", err)
	}
	other := &Security{DataType: types.DataTypeHKStock, Code: "00001.HK"}
	second, err := o.Subscribe(t.Context(), types.TopicBasicQot, other)
	if err != nil {
		t.Fatalf("second Subscribe: %v", err)
	}

	handler := tr.handler(types.BasicQotNotifyMsgType)
	handler(&domain.PushUpdate{Type: types.BasicQotNotifyMsgType, ID: "00700.HK", Time: pushTestTime(1_700_000_000_000),
		Event: syntheticMarketEvent(types.BasicQotNotifyMsgType, "00700.HK")})
	handler(&domain.PushUpdate{Type: types.BasicQotNotifyMsgType, ID: "00001.HK", Time: pushTestTime(1_700_000_001_000),
		Event: syntheticMarketEvent(types.BasicQotNotifyMsgType, "00001.HK")})

	for name, sub := range map[string]*Subscription{"00700.HK": first, "00001.HK": second} {
		got := 0
		for {
			select {
			case _, open := <-sub.Updates():
				if !open {
					t.Fatalf("%s: Updates closed", name)
				}
				got++
				continue
			default:
			}
			break
		}
		if got != 1 {
			t.Errorf("%s received %d update(s), want 1", name, got)
		}
	}
}

// TestAnUnroutedUpdateIsLoud covers §11.3's report and T11.
//
// An update nobody receives while the caller believes they subscribed is the worst
// outcome this layer has, so it is counted, and the first update per distinct id
// raises ErrUnrouted on the client stream. The "per distinct id" is what keeps a
// Gateway that keeps pushing the same stale id from flooding the stream - and it
// is asserted both ways: the first raises, the second does not, a new id does.
func TestAnUnroutedUpdateIsLoud(t *testing.T) {
	tr := newFakePushTransport()
	market := newFakeTopicSubscriber()
	o := newTestOrchestrationWith(t, market, newFakeOrderSubscriber(), tr)

	handler := tr.handler(types.BasicQotNotifyMsgType)
	push := func(id string) {
		handler(&domain.PushUpdate{
			Type:  types.BasicQotNotifyMsgType,
			ID:    id,
			Time:  pushTestTime(1_700_000_000_000),
			Event: syntheticMarketEvent(types.BasicQotNotifyMsgType, "NO-SUCH-CODE"),
		})
	}

	push("AAA")
	err := nextClientError(t, o)
	if !errors.Is(err, ErrUnrouted) {
		t.Errorf("first unrouted update = %v, want ErrUnrouted", err)
	}
	if !strings.Contains(err.Error(), "AAA") {
		t.Errorf("error = %q, want it to name the id that matched nothing", err)
	}
	// The message has to tell a caller the question it should ask, because the
	// repository cannot answer it: the Gateway's notifyId may differ from the code
	// sent to /hq/Subscribe.
	if !strings.Contains(err.Error(), "securityCode") {
		t.Errorf("error = %q, want it to name both candidate keys", err)
	}

	push("AAA")
	if got := drainClientErrors(o); len(got) != 0 {
		t.Errorf("a repeat of the same id produced %d further error(s), want 0: one alert per distinct id", len(got))
	}
	if got := o.Unrouted(); got != 2 {
		t.Errorf("Unrouted = %d, want 2: every unrouted update is counted even when only the first is reported", got)
	}

	push("BBB")
	if err := nextClientError(t, o); !errors.Is(err, ErrUnrouted) {
		t.Errorf("a new unrouted id = %v, want ErrUnrouted", err)
	}
	if got := o.Unrouted(); got != 3 {
		t.Errorf("Unrouted = %d, want 3", got)
	}
}

// TestUnroutedReportCapKeepsTheSeenSetBounded covers the cap.
//
// Past the cap every unrouted update is reported again rather than silently
// dropped, because an unbounded map keyed by wire data is exactly the memory
// growth a bounded error stream exists to prevent. The count still rises, so
// nothing is lost from the caller's point of view.
func TestUnroutedReportCapKeepsTheSeenSetBounded(t *testing.T) {
	tr := newFakePushTransport()
	market := newFakeTopicSubscriber()
	o := newTestOrchestrationWith(t, market, newFakeOrderSubscriber(), tr)

	handler := tr.handler(types.BasicQotNotifyMsgType)
	for i := 0; i < unroutedReportCap+4; i++ {
		handler(&domain.PushUpdate{
			Type:  types.BasicQotNotifyMsgType,
			ID:    "CODE-" + strconv.Itoa(i),
			Time:  pushTestTime(1_700_000_000_000),
			Event: syntheticMarketEvent(types.BasicQotNotifyMsgType, "NO-SUCH-CODE"),
		})
	}
	drainClientErrors(o)

	handler(&domain.PushUpdate{
		Type:  types.BasicQotNotifyMsgType,
		ID:    "PAST-THE-CAP",
		Time:  pushTestTime(1_700_000_000_000),
		Event: syntheticMarketEvent(types.BasicQotNotifyMsgType, "NO-SUCH-CODE"),
	})
	if err := nextClientError(t, o); !errors.Is(err, ErrUnrouted) {
		t.Errorf("an update past the report cap = %v, want ErrUnrouted: the cap must not turn a report into silence", err)
	}

	o.mu.Lock()
	seen := len(o.seenIDs)
	o.mu.Unlock()
	if seen > unroutedReportCap {
		t.Errorf("the seen-id set holds %d entries, want at most %d", seen, unroutedReportCap)
	}
}

// TestACancelledSubscriptionRoutesNothing is the negative control on the routing
// index: removing a subscription must remove it from every bucket, or a cancelled
// stream keeps receiving frames.
func TestACancelledSubscriptionRoutesNothing(t *testing.T) {
	tr := newFakePushTransport()
	market := newFakeTopicSubscriber()
	o := newTestOrchestrationWith(t, market, newFakeOrderSubscriber(), tr)

	sec := &Security{DataType: types.DataTypeHKStock, Code: "00700.HK"}
	sub, err := o.Subscribe(t.Context(), types.TopicBasicQot, sec)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	if err := sub.Cancel(t.Context()); err != nil {
		t.Fatalf("Cancel: %v", err)
	}

	tr.handler(types.BasicQotNotifyMsgType)(&domain.PushUpdate{
		Type:  types.BasicQotNotifyMsgType,
		ID:    "00700.HK",
		Time:  pushTestTime(1_700_000_000_000),
		Event: syntheticMarketEvent(types.BasicQotNotifyMsgType, "00700.HK"),
	})
	if got := o.Unrouted(); got != 1 {
		t.Errorf("Unrouted = %d after the only subscriber cancelled, want 1", got)
	}
	// And the close is what the caller sees, not a silent stop.
	waitClosed(t, sub.Updates())
}

// ---------------------------------------------------------------------------
// T6 - market backpressure
// ---------------------------------------------------------------------------

// TestMarketBackpressureDropsOldestAndCounts covers §8 and T6.
//
// A slow subscriber's buffer is filled until it overflows; the drop must be the
// oldest entry (a quote supersedes its predecessor) and it must be counted, because
// a silent drop is the released layer's defect. The load-bearing second half is that
// the slow subscriber's overflow does not affect the fast one on the same payload
// type: a block-instead-of-drop would stall every other subscriber of that type.
func TestMarketBackpressureDropsOldestAndCounts(t *testing.T) {
	tr := newFakePushTransport()
	market := newFakeTopicSubscriber()
	o := newTestOrchestrationWith(t, market, newFakeOrderSubscriber(), tr, WithBuffer(4))

	slow, err := o.Subscribe(t.Context(), types.TopicBasicQot,
		&Security{DataType: types.DataTypeHKStock, Code: "SLOW.HK"})
	if err != nil {
		t.Fatalf("Subscribe(slow): %v", err)
	}
	fast, err := o.Subscribe(t.Context(), types.TopicBasicQot,
		&Security{DataType: types.DataTypeHKStock, Code: "FAST.HK"})
	if err != nil {
		t.Fatalf("Subscribe(fast): %v", err)
	}

	handler := tr.handler(types.BasicQotNotifyMsgType)
	for i := 1; i <= 10; i++ {
		// Each subscriber gets its own frame: routing is per instrument, so one
		// subscriber's overflow is only meaningful next to a *different*
		// subscriber of the same payload type that is keeping up.
		handler(&domain.PushUpdate{
			Type:  types.BasicQotNotifyMsgType,
			ID:    "SLOW.HK",
			Time:  pushTestTime(1_700_000_000_000 + int64(i)),
			Event: syntheticMarketEvent(types.BasicQotNotifyMsgType, "SLOW.HK"),
		})
		handler(&domain.PushUpdate{
			Type:  types.BasicQotNotifyMsgType,
			ID:    "FAST.HK",
			Time:  pushTestTime(1_700_000_000_000 + int64(i)),
			Event: syntheticMarketEvent(types.BasicQotNotifyMsgType, "FAST.HK"),
		})
		// The fast subscriber drains as it goes, so its buffer is never full.
		select {
		case _, open := <-fast.Updates():
			if !open {
				t.Fatal("the fast subscription closed")
			}
		default:
			t.Fatalf("the fast subscriber missed update %d: one subscriber's overflow stalled another", i)
		}
	}

	if got := slow.Dropped(); got == 0 {
		t.Errorf("Dropped = 0 after 10 frames into a 4-entry buffer, want a positive count: a " +
			"drop-oldest queue is only defensible while it is loud")
	}
	if got := fast.Dropped(); got != 0 {
		t.Errorf("the fast subscriber Dropped = %d, want 0: it drained as it went", got)
	}

	// Drop-oldest, not drop-newest: what survived is the newest frames.
	var times []int64
	for len(slow.Updates()) > 0 {
		u := <-slow.Updates()
		times = append(times, u.Time.UnixMilli())
	}
	if len(times) != 4 {
		t.Fatalf("the slow subscriber holds %d frame(s), want the 4-entry buffer full", len(times))
	}
	if times[0] >= times[len(times)-1] {
		t.Errorf("surviving timestamps %v are not increasing, so the oldest were not the ones dropped", times)
	}
	if want := int64(1_700_000_000_010); times[len(times)-1] != want {
		t.Errorf("newest surviving frame = %d, want %d: the newest frame must be the one enqueued", times[len(times)-1], want)
	}
}

// TestMarketBackpressureNeverBlocksDispatch asserts the dispatch path itself does
// not block, by measuring that a full buffer returns control immediately.
//
// The measurement is not a duration: it is the fact that the handler returns and
// the assertion runs at all, which a blocking implementation could not do without a
// reader on the other side.
func TestMarketBackpressureNeverBlocksDispatch(t *testing.T) {
	tr := newFakePushTransport()
	market := newFakeTopicSubscriber()
	o := newTestOrchestrationWith(t, market, newFakeOrderSubscriber(), tr, WithBuffer(1))

	if _, err := o.Subscribe(t.Context(), types.TopicBasicQot,
		&Security{DataType: types.DataTypeHKStock, Code: "00700.HK"}); err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	handler := tr.handler(types.BasicQotNotifyMsgType)
	done := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 64; i++ {
			handler(&domain.PushUpdate{
				Type:  types.BasicQotNotifyMsgType,
				ID:    "00700.HK",
				Time:  pushTestTime(1_700_000_000_000 + int64(i)),
				Event: syntheticMarketEvent(types.BasicQotNotifyMsgType, "00700.HK"),
			})
		}
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(waitBound):
		t.Fatal("dispatch blocked on a full subscription buffer")
	}
	wg.Wait()
}

// ---------------------------------------------------------------------------
// T7 - trade delivery
// ---------------------------------------------------------------------------

// TestTradeOverflowIsReportedNotSilent covers §8.3 and T7.
//
// A fill is a per-event fact with no later notification, so an overflow cannot be
// the same quiet event as a dropped quote. The channel still does not block -
// blocking would stall the transport's dispatcher for every trade subscriber in the
// process - so the guarantee is "no silent loss": the drop is counted on the
// subscription and ErrTradeBacklog goes on the client stream.
//
// The report is raised once and the count keeps rising, because the error stream is
// itself bounded: an alert per dropped fill would crowd out the reports it exists
// to deliver.
func TestTradeOverflowIsReportedNotSilent(t *testing.T) {
	tr := newFakePushTransport()
	market := newFakeTopicSubscriber()
	trade := newFakeOrderSubscriber()
	o := newTestOrchestrationWith(t, market, trade, tr, WithBuffer(2))

	sub, err := o.SubscribeOrders(t.Context(), pushTestAccount())
	if err != nil {
		t.Fatalf("SubscribeOrders: %v", err)
	}

	handler := tr.handler(types.TradeStockDeliverMsgType)
	for i := 0; i < 6; i++ {
		handler(&domain.PushUpdate{
			Type:  types.TradeStockDeliverMsgType,
			ID:    "00700.HK",
			Time:  pushTestTime(1_700_000_000_000 + int64(i)),
			Event: syntheticTradeEvent("00700.HK"),
		})
	}

	if got := sub.Dropped(); got == 0 {
		t.Error("Dropped = 0 after 6 fills into a 2-entry buffer, want a positive count")
	}
	err = requireClientErrorNow(t, o, "the trade-delivery overflow")
	if !errors.Is(err, ErrTradeBacklog) {
		t.Errorf("the overflow = %v, want ErrTradeBacklog", err)
	}
	if !strings.Contains(err.Error(), "discarded") {
		t.Errorf("error = %q, want it to say a fill was discarded rather than imply otherwise", err)
	}
	if rest := drainClientErrors(o); len(rest) != 0 {
		t.Errorf("%d further error(s) after the first overflow, want 0", len(rest))
	}

	// A caller that drains is never asked to make the trade-off at all.
	drained := 0
	for len(sub.Updates()) > 0 {
		<-sub.Updates()
		drained++
	}
	if drained != 2 {
		t.Errorf("the subscription holds %d frame(s), want the 2-entry buffer", drained)
	}
	if got := sub.Dropped(); got != 4 {
		t.Errorf("Dropped = %d, want 4 (6 delivered, 2 kept)", got)
	}
}

// TestTradeDeliveryReachesEveryOrderSubscription covers the fan-out and asserts
// that an ordinary fill costs nothing: the backlog counter stays at zero.
func TestTradeDeliveryReachesEveryOrderSubscription(t *testing.T) {
	tr := newFakePushTransport()
	trade := newFakeOrderSubscriber()
	o := newTestOrchestrationWith(t, newFakeTopicSubscriber(), trade, tr)

	first, err := o.SubscribeOrders(t.Context(), pushTestAccount())
	if err != nil {
		t.Fatalf("first SubscribeOrders: %v", err)
	}
	second, err := o.SubscribeOrders(t.Context(), domain.AccountID("HST-2"))
	if err != nil {
		t.Fatalf("second SubscribeOrders: %v", err)
	}

	tr.handler(types.TradeStockDeliverMsgType)(&domain.PushUpdate{
		Type:  types.TradeStockDeliverMsgType,
		ID:    "00700.HK",
		Time:  pushTestTime(1_700_000_000_000),
		Event: syntheticTradeEvent("00700.HK"),
	})

	for name, sub := range map[string]*OrderSubscription{"first": first, "second": second} {
		select {
		case got, open := <-sub.Updates():
			if !open {
				t.Fatalf("%s: Updates closed", name)
			}
			if _, ok := got.Event.(domain.TradeEvent); !ok {
				t.Errorf("%s: event = %T, want domain.TradeEvent", name, got.Event)
			}
		default:
			t.Fatalf("%s received nothing", name)
		}
		if got := sub.Dropped(); got != 0 {
			t.Errorf("%s Dropped = %d, want 0 for a delivery that fit", name, got)
		}
	}
	if got := drainClientErrors(o); len(got) != 0 {
		t.Errorf("a delivery that fit produced %d error(s), want 0", len(got))
	}
}

// TestOrderSubscriptionCountsAcrossAccounts covers §10's third decision: the
// session-wide trade subscription is one HTTP call for the first and one for the
// last, across accounts, because the Gateway's scope is the session.
//
// It must not share the market registry's per-(topic, instruments) count: two
// accounts are two callers and one Gateway subscription.
func TestOrderSubscriptionCountsAcrossAccounts(t *testing.T) {
	tr := newFakePushTransport()
	trade := newFakeOrderSubscriber()
	o := newTestOrchestrationWith(t, newFakeTopicSubscriber(), trade, tr)

	first, err := o.SubscribeOrders(t.Context(), domain.AccountID("HST-1"))
	if err != nil {
		t.Fatalf("first SubscribeOrders: %v", err)
	}
	second, err := o.SubscribeOrders(t.Context(), domain.AccountID("HST-2"))
	if err != nil {
		t.Fatalf("second SubscribeOrders: %v", err)
	}
	if subs, unsubs := trade.counts(); subs != 1 || unsubs != 0 {
		t.Fatalf("two accounts: subscribe=%d unsubscribe=%d, want 1/0", subs, unsubs)
	}

	if err := first.Cancel(t.Context()); err != nil {
		t.Fatalf("first Cancel: %v", err)
	}
	if subs, unsubs := trade.counts(); subs != 1 || unsubs != 0 {
		t.Errorf("after one of two: subscribe=%d unsubscribe=%d, want 1/0", subs, unsubs)
	}
	if err := second.Cancel(t.Context()); err != nil {
		t.Fatalf("second Cancel: %v", err)
	}
	if subs, unsubs := trade.counts(); subs != 1 || unsubs != 1 {
		t.Errorf("after the last: subscribe=%d unsubscribe=%d, want 1/1", subs, unsubs)
	}
}

// TestSubscribeOrdersRefusesWithoutATradeHalf pins every local rejection of the trade
// entry, and asserts each costs zero HTTP.
//
// The rows are ordered so each one reaches its own guard: an orchestration with no
// transport is refused before the trade half is even consulted, and one with no trade
// half is refused for that reason rather than for the account - otherwise a zero
// account would be reported against the wrong cause.
func TestSubscribeOrdersRefusesWithoutATradeHalf(t *testing.T) {
	t.Run("no transport at all", func(t *testing.T) {
		o := NewPushOrchestration(newFakeTopicSubscriber(), nil, WithTradeSubscriber(newFakeOrderSubscriber()))
		t.Cleanup(func() { _ = o.Close() })
		sub, err := o.SubscribeOrders(t.Context(), pushTestAccount())
		if !errors.Is(err, ErrNoPushTransport) {
			t.Errorf("SubscribeOrders with no transport = %v, want ErrNoPushTransport", err)
		}
		if sub != nil {
			t.Error("a subscription was handed back alongside the error")
		}
	})

	t.Run("no trade half", func(t *testing.T) {
		o := NewPushOrchestration(newFakeTopicSubscriber(), newFakePushTransport())
		t.Cleanup(func() { _ = o.Close() })
		sub, err := o.SubscribeOrders(t.Context(), pushTestAccount())
		if err == nil {
			t.Fatal("SubscribeOrders with no trade half = nil error, want a local rejection")
		}
		if sub != nil {
			t.Error("a subscription was handed back alongside the error")
		}
		assertInvalidParam(t, err, opPushSubscribeOrders)
		if !strings.Contains(err.Error(), "WithTradeSubscriber") {
			t.Errorf("error = %q, want it to name the option that supplies the trade half", err.Error())
		}
	})

	t.Run("a zero account", func(t *testing.T) {
		trade := newFakeOrderSubscriber()
		o := NewPushOrchestration(newFakeTopicSubscriber(), newFakePushTransport(), WithTradeSubscriber(trade))
		t.Cleanup(func() { _ = o.Close() })
		sub, err := o.SubscribeOrders(t.Context(), domain.AccountID(""))
		if err == nil {
			t.Fatal("SubscribeOrders with a zero account = nil error, want a local rejection")
		}
		if sub != nil {
			t.Error("a subscription was handed back alongside the error")
		}
		assertInvalidParam(t, err, opPushSubscribeOrders)
		if subs, _ := trade.counts(); subs != 0 {
			t.Errorf("%d trade request(s) reached the Gateway for a rejected account", subs)
		}
	})

	t.Run("the session refuses the subscribe", func(t *testing.T) {
		trade := newFakeOrderSubscriber()
		trade.subErr[1] = errors.New("the session will not enable order push")
		o := NewPushOrchestration(newFakeTopicSubscriber(), newFakePushTransport(), WithTradeSubscriber(trade))
		t.Cleanup(func() { _ = o.Close() })

		sub, err := o.SubscribeOrders(t.Context(), pushTestAccount())
		if err == nil {
			t.Fatal("SubscribeOrders = nil error, want the session's refusal")
		}
		if sub != nil {
			t.Error("a subscription was handed back alongside the error")
		}
		if !strings.Contains(err.Error(), "will not enable order push") {
			t.Errorf("error = %q, want it to carry the session's reason", err.Error())
		}
		o.mu.Lock()
		live := len(o.orders)
		active := o.tradeOn
		o.mu.Unlock()
		if live != 0 || active {
			t.Errorf("the refused subscribe registered %d subscription(s) with tradeOn=%v, want 0/false: "+
				"a refused enable must not count as established", live, active)
		}
	})
}

// ---------------------------------------------------------------------------
// T4 - reconnect, on the services side
// ---------------------------------------------------------------------------

// TestAReconnectWithNoSubscriptionIsObservable covers §7.1's first hole and T4.
//
// The released layer delivers its reconnect notice by iterating its subscriptions,
// so a reconnect with none is invisible: a caller that connects, is disconnected,
// reconnects, and *then* subscribes has learned nothing. This emits exactly one
// notice regardless, with zero subscriptions.
//
// The cause is asserted through the chain rather than the message, and specifically
// through errors.Unwrap, because that is what distinguishes a notice that carries
// its cause as matchable chain from one that renders it as text: a multi-%w wrap
// has Unwrap() []error, on which errors.Unwrap returns nil, silently falsifying the
// mechanism the doc comment promises.
func TestAReconnectWithNoSubscriptionIsObservable(t *testing.T) {
	tr := newFakePushTransport()
	o := newTestOrchestration(t, tr)

	if hooks := tr.fireReconnect(io.EOF); hooks != 1 {
		t.Fatalf("the orchestrator registered %d reconnect hook(s), want exactly 1", hooks)
	}

	notice := requireClientErrorNow(t, o, "the reconnect with no subscription")
	if !errors.Is(notice, ErrReconnected) {
		t.Errorf("notice = %v, want ErrReconnected", notice)
	}
	if got := errors.Unwrap(notice); got != io.EOF {
		t.Errorf("errors.Unwrap(notice) = %v, want the reconnect cause io.EOF", got)
	}
	if !errors.Is(notice, io.EOF) {
		t.Error("errors.Is(notice, io.EOF) = false, want the cause to be matchable")
	}
	if want := ErrReconnected.Error() + ": " + io.EOF.Error(); notice.Error() != want {
		t.Errorf("notice = %q, want %q", notice, want)
	}
	if rest := drainClientErrors(o); len(rest) != 0 {
		t.Errorf("one reconnect produced %d further error(s), want exactly one notice", len(rest))
	}
	if got := o.Unrouted(); got != 0 {
		t.Errorf("Unrouted = %d, want 0", got)
	}
}

// TestTheReconnectCauseDistinguishesAHangupFromADeadFeed is the operational
// question a caller actually has to answer, and it is only visible through the chain.
func TestTheReconnectCauseDistinguishesAHangupFromADeadFeed(t *testing.T) {
	cases := []struct {
		name    string
		cause   error
		timeout bool
	}{
		{name: "the Gateway hung up", cause: io.EOF},
		{name: "the read deadline expired", cause: &net.OpError{Op: "read", Net: "tcp", Err: errTimeoutSentinel}, timeout: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tr := newFakePushTransport()
			o := newTestOrchestration(t, tr)
			tr.fireReconnect(tc.cause)

			notice := nextClientError(t, o)
			if !errors.Is(notice, ErrReconnected) {
				t.Fatalf("notice = %v, want ErrReconnected", notice)
			}
			var netErr *net.OpError
			if errors.As(notice, &netErr) != tc.timeout {
				t.Errorf("errors.As(notice, *net.OpError) = %v, want %v", tc.timeout, tc.timeout)
			}
			if got := errors.Unwrap(notice); got != tc.cause {
				t.Errorf("errors.Unwrap = %v, want the cause %v", got, tc.cause)
			}
		})
	}
}

// TestTheReconnectNoticeShape is the unit-level property A4 established for the
// released layer, pinned here for this package's copy of the mechanism.
//
// A nil cause yields the bare sentinel rather than a wrapped nil: the cause arrives
// from the transport, and rendering a missing cause must not be able to panic in a
// public API.
func TestTheReconnectNoticeShape(t *testing.T) {
	t.Run("a nil cause yields the bare sentinel", func(t *testing.T) {
		notice := newReconnectNotice(nil)
		if notice != ErrReconnected {
			t.Errorf("newReconnectNotice(nil) = %v, want the bare sentinel", notice)
		}
		if got := errors.Unwrap(notice); got != nil {
			t.Errorf("errors.Unwrap = %v, want nil", got)
		}
		if got, want := notice.Error(), ErrReconnected.Error(); got != want {
			t.Errorf("Error() = %q, want %q", got, want)
		}
	})

	t.Run("nothing else matches", func(t *testing.T) {
		notice := newReconnectNotice(io.ErrUnexpectedEOF)
		if errors.Is(notice, io.EOF) {
			t.Error("errors.Is(notice, io.EOF) = true, want false: io.EOF is a sibling, not the cause")
		}
		if errors.Is(notice, errors.New("unrelated")) {
			t.Error("errors.Is(notice, unrelated) = true, want false")
		}
		if got := errors.Unwrap(notice); got != io.ErrUnexpectedEOF {
			t.Errorf("errors.Unwrap = %v, want io.ErrUnexpectedEOF", got)
		}
	})
}

// TestReconnectRestoresEveryEstablishedSubscription covers §7.3's first decision:
// the re-subscribe is automatic and comes from the orchestrator's own registry,
// never from a caller-visible notification.
//
// A caller-driven re-subscribe is a notification nobody is watching - the whole
// point of the feature is that it happens without the caller doing anything - so
// the test fires the transport's reconnect hook and asserts the request was issued
// with no subscriber involvement.
func TestReconnectRestoresEveryEstablishedSubscription(t *testing.T) {
	tr := newFakePushTransport()
	market := newFakeTopicSubscriber()
	trade := newFakeOrderSubscriber()
	o := newTestOrchestrationWith(t, market, trade, tr)

	if _, err := o.Subscribe(t.Context(), types.TopicBasicQot,
		&Security{DataType: types.DataTypeHKStock, Code: "00700.HK"}); err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	if _, err := o.Subscribe(t.Context(), types.TopicTicker,
		&Security{DataType: types.DataTypeUSStock, Code: "AAPL.US"}); err != nil {
		t.Fatalf("Subscribe(ticker): %v", err)
	}
	if _, err := o.SubscribeOrders(t.Context(), pushTestAccount()); err != nil {
		t.Fatalf("SubscribeOrders: %v", err)
	}

	tr.fireReconnect(io.EOF)

	// Two keys, each established once and re-subscribed once, plus the session-wide
	// trade subscription's own pair.
	if subs, _ := market.counts(); subs != 4 {
		t.Errorf("/hq/Subscribe count = %d, want 4 (two keys established, two re-subscribed)", subs)
	}
	if subs, _ := trade.counts(); subs != 2 {
		t.Errorf("trade SubscribeOrders count = %d, want 2 (one initial, one on the reconnect)", subs)
	}

	// The notice is on the client stream, not on any subscription, so a caller with
	// no subscription still learns of it - that is asserted in the test above.
	_ = nextClientError(t, o)
}

// TestAFailedResubscribeKillsTheSubscription covers §7.3's second decision and T5.
//
// The released layer only reports a failed re-subscribe, leaving a subscription that
// is registered locally and will never deliver again. For a market topic the next
// observation is the *absence* of news, which during a quiet market is
// indistinguishable from a working feed - so the subscription is cancelled, its
// channel is closed, and a terminal error naming the topic and the Gateway's reason
// goes on the client stream.
//
// A loud dead subscription beats a silent one.
func TestAFailedResubscribeKillsTheSubscription(t *testing.T) {
	tr := newFakePushTransport()
	market := newFakeTopicSubscriber()
	o := newTestOrchestrationWith(t, market, newFakeOrderSubscriber(), tr)

	sub, err := o.Subscribe(t.Context(), types.TopicBasicQot,
		&Security{DataType: types.DataTypeHKStock, Code: "00700.HK"})
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	// The initial subscribe succeeded; every later one is the reconnect.
	market.subErr[2] = errs.New(types.StatusServiceBusy, opSubscribe, "the Gateway is busy")

	tr.fireReconnect(io.EOF)

	requireClosedNow(t, sub.Updates(), "the refused re-subscribe")

	var terminal error
	for _, err := range drainAll(o) {
		if errors.Is(err, ErrSubscriptionLost) {
			terminal = err
		}
	}
	if terminal == nil {
		t.Fatal("no ErrSubscriptionLost on the client stream, want a terminal error naming the topic")
	}
	for _, want := range []string{"11", "00700.HK", "busy"} {
		if !strings.Contains(terminal.Error(), want) {
			t.Errorf("terminal error = %q, want it to name %q", terminal.Error(), want)
		}
	}
	if got := o.Unrouted(); got != 0 {
		t.Errorf("Unrouted = %d, want 0", got)
	}
}

// drainAll collects every error currently queued and waits briefly for a late one,
// so an assertion about "the" error is not racing the goroutine that reports it.
// It bounds the wait and never blocks on a clock to reach a code path: the only
// thing it is for is letting an already-scheduled report land.
func drainAll(o *PushOrchestration) []error {
	var out []error
	for {
		select {
		case err, ok := <-o.Errors():
			if !ok {
				return out
			}
			out = append(out, err)
		default:
			return out
		}
	}
}

// TestAFailedResubscribeRemovesOnlyTheDeadKey covers the blast radius of a kill:
// another key that re-subscribed successfully must survive.
func TestAFailedResubscribeRemovesOnlyTheDeadKey(t *testing.T) {
	tr := newFakePushTransport()
	market := newFakeTopicSubscriber()
	o := newTestOrchestrationWith(t, market, newFakeOrderSubscriber(), tr)

	doomed, err := o.Subscribe(t.Context(), types.TopicBasicQot,
		&Security{DataType: types.DataTypeHKStock, Code: "00700.HK"})
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	survivor, err := o.Subscribe(t.Context(), types.TopicTicker,
		&Security{DataType: types.DataTypeUSStock, Code: "AAPL.US"})
	if err != nil {
		t.Fatalf("Subscribe(ticker): %v", err)
	}
	// Calls 1 and 2 are the two initial subscribes. The reconnect walks the keys in
	// canonical order, so call 3 is the basic-quote re-subscribe and call 4 is the
	// ticker's; only the first is refused.
	market.subErr[3] = errors.New("the Gateway refused the reconnect")

	tr.fireReconnect(io.EOF)

	waitClosed(t, doomed.Updates())
	if survivor.isClosed() {
		t.Error("the surviving subscription was closed: one failed re-subscribe killed every key")
	}

	tr.handler(types.TickerNotifyMsgType)(&domain.PushUpdate{
		Type:  types.TickerNotifyMsgType,
		ID:    "AAPL.US",
		Time:  pushTestTime(1_700_000_000_000),
		Event: syntheticMarketEvent(types.TickerNotifyMsgType, "AAPL.US"),
	})
	select {
	case _, open := <-survivor.Updates():
		if !open {
			t.Fatal("the surviving subscription closed on delivery")
		}
	default:
		t.Error("the surviving subscription received nothing after the reconnect")
	}
}

// TestAFailedTradeResubscribeKillsTheOrderSubscriptions covers the session-wide
// half of the same rule.
func TestAFailedTradeResubscribeKillsTheOrderSubscriptions(t *testing.T) {
	tr := newFakePushTransport()
	trade := newFakeOrderSubscriber()
	o := newTestOrchestrationWith(t, newFakeTopicSubscriber(), trade, tr)

	sub, err := o.SubscribeOrders(t.Context(), pushTestAccount())
	if err != nil {
		t.Fatalf("SubscribeOrders: %v", err)
	}
	trade.subErr[2] = errors.New("the session refused the reconnect")

	tr.fireReconnect(io.EOF)

	requireClosedNow(t, sub.Updates(), "the refused re-subscribe")
	var sawLoss bool
	for _, err := range drainAll(o) {
		if errors.Is(err, ErrSubscriptionLost) {
			sawLoss = true
		}
	}
	if !sawLoss {
		t.Error("no ErrSubscriptionLost for the refused trade re-subscribe")
	}
}

// TestCloseDuringAReconnectCancelsTheInFlightResubscribe covers §7.3's first
// decision and T12.
//
// The released layer derives its re-subscribe from context.Background() and comments
// that the run context "may already be cancelled". That is right for the released
// layer and wrong here: the run context is exactly what a caller cancels to stop
// reading, so honouring it would mean a Close racing a reconnect leaves an in-flight
// /hq/Subscribe against a Gateway the caller has already released.
//
// The re-subscribe is driven from a goroutine and joined with a WaitGroup; the
// moment to act is taken from the channel the fake signals on entry, not from a
// delay.
func TestCloseDuringAReconnectCancelsTheInFlightResubscribe(t *testing.T) {
	tr := newFakePushTransport()
	market := newFakeTopicSubscriber()
	o := newTestOrchestrationWith(t, market, newFakeOrderSubscriber(), tr)

	if _, err := o.Subscribe(t.Context(), types.TopicBasicQot,
		&Security{DataType: types.DataTypeHKStock, Code: "00700.HK"}); err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	gate := make(chan struct{})
	market.holdSubscribe(gate)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		tr.fireReconnect(io.EOF)
	}()

	// The re-subscribe is now inside the HTTP half, holding its context open.
	select {
	case <-market.entered:
	case <-time.After(waitBound):
		t.Fatal("the re-subscribe never reached the HTTP half")
	}

	if err := o.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	market.releaseSubscribe()
	wg.Wait()

	if got := market.sawContextError(); !errors.Is(got, context.Canceled) {
		t.Errorf("the in-flight re-subscribe saw %v, want context.Canceled: Close must cancel it "+
			"rather than leave a request against a released Gateway", got)
	}
}

// TestCloseDuringAReconnectCancelsTheInFlightTradeResubscribe is the session-wide
// half of the previous test.
//
// It is a separate case rather than an extra row because the trade re-subscribe
// takes a different path: it goes through the trade half rather than the market
// half, and it is counted separately from the market registry, so a guard that only
// covered the market path would leave the trade path's context parent unguarded.
func TestCloseDuringAReconnectCancelsTheInFlightTradeResubscribe(t *testing.T) {
	tr := newFakePushTransport()
	trade := newFakeOrderSubscriber()
	o := newTestOrchestrationWith(t, newFakeTopicSubscriber(), trade, tr)

	if _, err := o.SubscribeOrders(t.Context(), pushTestAccount()); err != nil {
		t.Fatalf("SubscribeOrders: %v", err)
	}

	gate := make(chan struct{})
	trade.holdSubscribe(gate)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		tr.fireReconnect(io.EOF)
	}()

	select {
	case <-trade.entered:
	case <-time.After(waitBound):
		t.Fatal("the trade re-subscribe never reached the HTTP half")
	}
	if err := o.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	trade.releaseSubscribe()
	wg.Wait()

	if got := trade.sawContextError(); !errors.Is(got, context.Canceled) {
		t.Errorf("the in-flight trade re-subscribe saw %v, want context.Canceled", got)
	}
}

// TestCloseIsIdempotentAndClosesEveryStream covers the rest of T12.
func TestCloseIsIdempotentAndClosesEveryStream(t *testing.T) {
	tr := newFakePushTransport()
	market := newFakeTopicSubscriber()
	o := newTestOrchestrationWith(t, market, newFakeOrderSubscriber(), tr)

	sub, err := o.Subscribe(t.Context(), types.TopicBasicQot,
		&Security{DataType: types.DataTypeHKStock, Code: "00700.HK"})
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	order, err := o.SubscribeOrders(t.Context(), pushTestAccount())
	if err != nil {
		t.Fatalf("SubscribeOrders: %v", err)
	}

	for i := 0; i < 3; i++ {
		if err := o.Close(); err != nil {
			t.Fatalf("Close %d: %v", i, err)
		}
	}
	waitClosed(t, sub.Updates())
	waitClosed(t, order.Updates())
	if _, open := <-o.Errors(); open {
		t.Error("the client error stream is still open after Close")
	}

	// The cleanup registered by the helper calls Close again; nothing may panic.
	if err := o.Close(); err != nil {
		t.Errorf("Close after the stream closed: %v", err)
	}
}

// TestARunReturnsWhenTheTransportStops covers the Run pass-through, which is what
// makes Close leak-free: the caller's goroutine has to come back.
func TestARunReturnsWhenTheTransportStops(t *testing.T) {
	tr := newFakePushTransport()
	o := newTestOrchestration(t, tr)

	var wg sync.WaitGroup
	wg.Add(1)
	runErr := make(chan error, 1)
	go func() {
		defer wg.Done()
		runErr <- o.Run(t.Context())
	}()

	select {
	case <-tr.runCalled:
	case <-time.After(waitBound):
		t.Fatal("Run was never reached")
	}
	if err := o.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	wg.Wait()
	if err := <-runErr; err != nil {
		t.Errorf("Run = %v, want nil after a Close", err)
	}
}

// ---------------------------------------------------------------------------
// Forwarding
// ---------------------------------------------------------------------------

// TestEveryTransportErrorReachesTheClientStream covers §7.1's second hole.
//
// The released layer forwards *verification* errors only, on the stated ground that
// the stream API has never surfaced transport read errors. A client-level stream has
// no such compatibility constraint against a channel reconnect notices already
// consume, so every error is forwarded verbatim - including one raised before any
// subscription exists, which is the other half of the hole: a caller with no
// subscription must still learn that a frame was rejected.
func TestEveryTransportErrorReachesTheClientStream(t *testing.T) {
	tr := newFakePushTransport()
	o := newTestOrchestration(t, tr)

	verdict := errors.New("frame rejected by the verifier")
	tr.raise(verdict)
	got := nextClientError(t, o)
	if got != verdict {
		t.Errorf("forwarded error = %v, want the transport's own value %v: the stream must be "+
			"verbatim, not filtered to some categories", got, verdict)
	}
	if got := o.Unrouted(); got != 0 {
		t.Errorf("Unrouted = %d, want 0: no subscription existed and none was needed", got)
	}
}

// TestTheClientErrorStreamIsBoundedAndCountsItsLosses covers §8.3's third rule.
//
// A lost diagnosis is recoverable; an unbounded queue on a flapping connection is a
// memory leak. So the stream drops oldest and counts, and the count is public.
//
// The barrier is the transport stream's close: the drain goroutine exits when it
// sees it, so joining the drain goroutine afterwards is a barrier over every error
// already sent. Nothing here races the goroutine that forwards, and nothing waits
// on a clock.
func TestTheClientErrorStreamIsBoundedAndCountsItsLosses(t *testing.T) {
	tr := newFakePushTransport()
	o := newTestOrchestration(t, tr)

	// Three times the client's own capacity, so the stream is certain to overflow
	// however the two goroutines interleave.
	total := 3 * cap(o.errc)
	for i := 0; i < total; i++ {
		tr.errc <- errors.New("flapping")
	}
	tr.closeErrStream()
	o.wg.Wait()

	if got := o.ErrorsDropped(); got == 0 {
		t.Errorf("ErrorsDropped = 0 after %d errors into a %d-entry stream, want a positive count: "+
			"a bounded queue is only defensible while it is loud", total, cap(o.errc))
	}
}

// ---------------------------------------------------------------------------
// T9 - freshness
// ---------------------------------------------------------------------------

// TestStaleUsesTheInjectedClockAndItsOwnThreshold covers §9.2 and T9.
//
// Two properties. The threshold is the caller's, because a five-minute silence means
// something very different for a liquid Hong Kong stock and an illiquid warrant and
// the SDK has no business guessing. And a non-positive threshold restores the
// five-minute default rather than accepting zero, which would mark every
// subscription stale on the first check - the rule internal/push's own
// NewFreshnessMonitor states, adopted because it is right.
//
// Nothing here waits: the clock is injected and moved.
func TestStaleUsesTheInjectedClockAndItsOwnThreshold(t *testing.T) {
	tr := newFakePushTransport()
	clock := newFakeClock()
	market := newFakeTopicSubscriber()
	o := newTestOrchestrationWith(t, market, newFakeOrderSubscriber(), tr)
	o.now = clock.Now

	sub, err := o.Subscribe(t.Context(), types.TopicBasicQot,
		&Security{DataType: types.DataTypeHKStock, Code: "00700.HK"})
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	if sub.Stale(0) {
		t.Error("Stale(0) = true before any frame arrived, want false: nothing has been late yet")
	}

	tr.handler(types.BasicQotNotifyMsgType)(&domain.PushUpdate{
		Type:  types.BasicQotNotifyMsgType,
		ID:    "00700.HK",
		Time:  pushTestTime(1_700_000_000_000),
		Event: syntheticMarketEvent(types.BasicQotNotifyMsgType, "00700.HK"),
	})
	if got := sub.LastSeen(); !got.Equal(clock.Now()) {
		t.Errorf("LastSeen = %v, want the injected clock's %v", got, clock.Now())
	}
	if got, want := sub.LastUpdate(), "20260126 09:30:01"; got != want {
		t.Errorf("LastUpdate = %q, want the payload's own timestamp %q", got, want)
	}

	clock.advance(6 * time.Minute)
	if !sub.Stale(time.Minute) {
		t.Error("Stale(1m) = false after 6 minutes of silence, want true")
	}
	if sub.Stale(time.Hour) {
		t.Error("Stale(1h) = true after 6 minutes of silence, want false: the threshold is the caller's")
	}
	// The non-positive case restores the five-minute default, so six minutes is
	// stale and one minute is not.
	if !sub.Stale(0) {
		t.Error("Stale(0) = false after 6 minutes, want true: a non-positive threshold restores the default")
	}
	if !sub.Stale(-time.Second) {
		t.Error("Stale(-1s) = false, want the same default as Stale(0)")
	}
	clock.advance(-5 * time.Minute)
	if sub.Stale(0) {
		t.Error("Stale(0) = true after one minute of silence, want false: the default is five minutes")
	}
}

// TestStaleIsFalseForACancelledSubscription is the other half of the rule: a caller
// that stops reading must see the channel close, not a growing staleness.
func TestStaleIsFalseForACancelledSubscription(t *testing.T) {
	tr := newFakePushTransport()
	clock := newFakeClock()
	market := newFakeTopicSubscriber()
	o := newTestOrchestrationWith(t, market, newFakeOrderSubscriber(), tr)
	o.now = clock.Now

	sub, err := o.Subscribe(t.Context(), types.TopicBasicQot,
		&Security{DataType: types.DataTypeHKStock, Code: "00700.HK"})
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	tr.handler(types.BasicQotNotifyMsgType)(&domain.PushUpdate{
		Type:  types.BasicQotNotifyMsgType,
		ID:    "00700.HK",
		Time:  pushTestTime(1_700_000_000_000),
		Event: syntheticMarketEvent(types.BasicQotNotifyMsgType, "00700.HK"),
	})
	clock.advance(time.Hour)
	if err := sub.Cancel(t.Context()); err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	if sub.Stale(0) {
		t.Error("Stale = true after Cancel: a cancelled subscription is gone, not stale")
	}
}

// TestOrderSubscriptionLastSeenUsesTheSameClock covers the trade half, so the two
// subscription types cannot drift.
func TestOrderSubscriptionLastSeenUsesTheSameClock(t *testing.T) {
	tr := newFakePushTransport()
	clock := newFakeClock()
	o := newTestOrchestrationWith(t, newFakeTopicSubscriber(), newFakeOrderSubscriber(), tr)
	o.now = clock.Now

	sub, err := o.SubscribeOrders(t.Context(), pushTestAccount())
	if err != nil {
		t.Fatalf("SubscribeOrders: %v", err)
	}
	clock.advance(90 * time.Second)
	tr.handler(types.TradeStockDeliverMsgType)(&domain.PushUpdate{
		Type:  types.TradeStockDeliverMsgType,
		ID:    "00700.HK",
		Time:  pushTestTime(1_700_000_000_000),
		Event: syntheticTradeEvent("00700.HK"),
	})
	if got := sub.LastSeen(); !got.Equal(clock.Now()) {
		t.Errorf("LastSeen = %v, want %v", got, clock.Now())
	}
	if got := sub.AccountID(); got != pushTestAccount() {
		t.Errorf("AccountID = %v, want %v", got, pushTestAccount())
	}
}

// ---------------------------------------------------------------------------
// T10 - out-of-order frames
// ---------------------------------------------------------------------------

// TestAnOutOfOrderFrameIsCountedAndStillDelivered covers §9.3 and T10.
//
// Two conditions are easy to conflate and are kept apart. Freshness is "nothing has
// arrived for a while". Out-of-order is "a frame arrived stamped older than one
// already accepted" - the Gateway's clock moved, or a reconnect boundary was
// crossed. The second is detected, counted, reported once, and the frame is still
// delivered: dropping it would be a silent loss, and it would be wrong across a
// reconnect boundary where an earlier timestamp is expected.
//
// Strict gap detection is impossible on this protocol - the envelope carries no
// per-event sequence number - so this is a monotonic-clock observation, not a dedup.
func TestAnOutOfOrderFrameIsCountedAndStillDelivered(t *testing.T) {
	tr := newFakePushTransport()
	market := newFakeTopicSubscriber()
	o := newTestOrchestrationWith(t, market, newFakeOrderSubscriber(), tr, WithBuffer(8))

	sub, err := o.Subscribe(t.Context(), types.TopicBasicQot,
		&Security{DataType: types.DataTypeHKStock, Code: "00700.HK"})
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	handler := tr.handler(types.BasicQotNotifyMsgType)

	deliver := func(ms int64) {
		handler(&domain.PushUpdate{
			Type:  types.BasicQotNotifyMsgType,
			ID:    "00700.HK",
			Time:  pushTestTime(ms),
			Event: syntheticMarketEvent(types.BasicQotNotifyMsgType, "00700.HK"),
		})
	}

	deliver(1_700_000_100_000)
	deliver(1_700_000_200_000)
	deliver(1_700_000_150_000) // older than the previous one

	if got := sub.OutOfOrder(); got != 1 {
		t.Fatalf("OutOfOrder = %d, want 1", got)
	}
	if got := len(sub.Updates()); got != 3 {
		t.Errorf("the subscription holds %d frame(s), want 3: an out-of-order frame must still be delivered", got)
	}
	// And the delivered frame is the out-of-order one, last in the queue: the
	// out-of-order check must not reorder or suppress anything.
	var stamps []int64
	for len(sub.Updates()) > 0 {
		stamps = append(stamps, (<-sub.Updates()).Time.UnixMilli())
	}
	want := []int64{1_700_000_100_000, 1_700_000_200_000, 1_700_000_150_000}
	if len(stamps) != len(want) {
		t.Fatalf("drained %d frame(s), want %d", len(stamps), len(want))
	}
	for i := range want {
		if stamps[i] != want[i] {
			t.Errorf("delivered[%d] is stamped %d, want %d: the out-of-order frame must be delivered "+
				"in arrival order, not dropped and not reordered", i, stamps[i], want[i])
		}
	}

	err = requireClientErrorNow(t, o, "the first out-of-order frame")
	if !errors.Is(err, ErrUpdateOutOfOrder) {
		t.Errorf("the first out-of-order frame = %v, want ErrUpdateOutOfOrder", err)
	}

	// Later occurrences are counted silently: a permanently skewed clock must
	// produce one alert, not a flood that crowds out the drop reports.
	deliver(1_700_000_120_000)
	deliver(1_700_000_110_000)
	if got := sub.OutOfOrder(); got != 3 {
		t.Errorf("OutOfOrder = %d, want 3", got)
	}
	if rest := drainClientErrors(o); len(rest) != 0 {
		t.Errorf("%d further out-of-order error(s), want 0 after the first", len(rest))
	}
}

// TestOutOfOrderIsPerSubscription covers the keying: the subscription unit is
// (topic, security), so another instrument's clock is not this one's.
func TestOutOfOrderIsPerSubscription(t *testing.T) {
	tr := newFakePushTransport()
	market := newFakeTopicSubscriber()
	o := newTestOrchestrationWith(t, market, newFakeOrderSubscriber(), tr)

	first, err := o.Subscribe(t.Context(), types.TopicBasicQot,
		&Security{DataType: types.DataTypeHKStock, Code: "00700.HK"})
	if err != nil {
		t.Fatalf("Subscribe(00700.HK): %v", err)
	}
	second, err := o.Subscribe(t.Context(), types.TopicBasicQot,
		&Security{DataType: types.DataTypeHKStock, Code: "00001.HK"})
	if err != nil {
		t.Fatalf("Subscribe(00001.HK): %v", err)
	}

	handler := tr.handler(types.BasicQotNotifyMsgType)
	handler(&domain.PushUpdate{Type: types.BasicQotNotifyMsgType, ID: "00700.HK",
		Time: pushTestTime(1_700_000_200_000), Event: syntheticMarketEvent(types.BasicQotNotifyMsgType, "00700.HK")})
	handler(&domain.PushUpdate{Type: types.BasicQotNotifyMsgType, ID: "00001.HK",
		Time: pushTestTime(1_700_000_100_000), Event: syntheticMarketEvent(types.BasicQotNotifyMsgType, "00001.HK")})

	if got := first.OutOfOrder(); got != 0 {
		t.Errorf("00700.HK OutOfOrder = %d, want 0: the other instrument's older frame is not this one's", got)
	}
	if got := second.OutOfOrder(); got != 0 {
		t.Errorf("00001.HK OutOfOrder = %d, want 0", got)
	}
}

// ---------------------------------------------------------------------------
// Keys
// ---------------------------------------------------------------------------

// TestTheRoutingKeyNormalisation pins the normalisation rule the dual index rests
// on: upper-case, and one trailing market suffix stripped, so both spellings of an
// instrument reach the same entry.
func TestTheRoutingKeyNormalisation(t *testing.T) {
	cases := map[string]string{
		"00700.HK":  "00700",
		"00700.hk":  "00700",
		"00700":     "00700",
		" 00700.HK": "00700",
		"AAPL.US":   "AAPL",
		"000001.SZ": "000001",
		"600000.SH": "600000",
		"":          "",
		".HK":       ".HK",
	}
	for in, want := range cases {
		if got := normaliseRoutingKey(in); got != want {
			t.Errorf("normaliseRoutingKey(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestTheCanonicalKeyIsOrderInsensitiveAndValueBased pins the registry key rule.
func TestTheCanonicalKeyIsOrderInsensitiveAndValueBased(t *testing.T) {
	a := &Security{DataType: types.DataTypeHKStock, Code: "00700.HK"}
	b := &Security{DataType: types.DataTypeUSStock, Code: "AAPL.US"}

	forward := canonicalPushKey(types.TopicBasicQot, []*Security{a, b})
	reverse := canonicalPushKey(types.TopicBasicQot, []*Security{b, a})
	if forward != reverse {
		t.Errorf("the same instruments in two orders produced two keys:\n  %q\n  %q", forward, reverse)
	}

	// Value-based: a distinct caller-owned copy with the same values is the same key.
	copyA := &Security{DataType: types.DataTypeHKStock, Code: "00700.HK"}
	if got := canonicalPushKey(types.TopicBasicQot, []*Security{copyA, b}); got != forward {
		t.Error("the key is pointer-based: two equal values produced two keys")
	}

	// And the two axes really are separate.
	if got := canonicalPushKey(types.TopicTicker, []*Security{a, b}); got == forward {
		t.Error("a different topic produced the same key")
	}
	if got := canonicalPushKey(types.TopicBasicQot, []*Security{a}); got == forward {
		t.Error("a different instrument set produced the same key")
	}
}

// TestSecurityKeysSkipsUnusableInstruments covers the negative arm: there is no
// routing key for an instrument with no code, so a subscription that asked for one
// can never be matched and must not create a bucket under the empty string.
func TestSecurityKeysSkipsUnusableInstruments(t *testing.T) {
	got := securityKeys([]*Security{
		nil,
		{Code: ""},
		{Code: "00700.HK"},
		{Code: "00700.hk"},
		{Code: "AAPL.US"},
	})
	want := []string{"00700", "AAPL"}
	if len(got) != len(want) {
		t.Fatalf("securityKeys = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("securityKeys[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

// TestUpdateRoutingKeysDeduplicates pins the dual index's own behaviour: both
// candidate keys are returned, and a frame whose two spellings normalise to one key
// yields one key rather than being delivered twice.
func TestUpdateRoutingKeysDeduplicates(t *testing.T) {
	same := updateRoutingKeys(&domain.PushUpdate{
		ID:    "00700",
		Event: syntheticMarketEvent(types.BasicQotNotifyMsgType, "00700.HK"),
	})
	if len(same) != 1 || same[0] != "00700" {
		t.Errorf("keys = %v, want one entry 00700 for two spellings of one instrument", same)
	}

	both := updateRoutingKeys(&domain.PushUpdate{
		ID:    "00700",
		Event: syntheticMarketEvent(types.BasicQotNotifyMsgType, "AAPL.US"),
	})
	if len(both) != 2 {
		t.Errorf("keys = %v, want both candidates when they disagree", both)
	}

	neither := updateRoutingKeys(&domain.PushUpdate{
		ID:    "",
		Event: domain.SystemEvent{Code: "X", Level: "warn"},
	})
	if len(neither) != 0 {
		t.Errorf("keys = %v, want none for an event with no instrument", neither)
	}
}

// TestEventTimestampReadsEveryEventThatHasOne covers the type switch behind
// LastUpdate, including the arm that has none.
func TestEventTimestampReadsEveryEventThatHasOne(t *testing.T) {
	ts := "20260126 09:30:01"
	withStamp := []domain.PushEvent{
		domain.QuoteEvent{Timestamp: ts},
		domain.TickerEvent{Timestamp: ts},
		domain.OrderBookEvent{Timestamp: ts},
		domain.BrokerEvent{Timestamp: ts},
		domain.TradeEvent{Timestamp: ts},
	}
	for _, ev := range withStamp {
		got, ok := eventTimestamp(ev)
		if !ok || got != ts {
			t.Errorf("eventTimestamp(%T) = (%q, %v), want (%q, true)", ev, got, ok, ts)
		}
	}
	if got, ok := eventTimestamp(domain.SystemEvent{Code: "X"}); ok || got != "" {
		t.Errorf("eventTimestamp(SystemEvent) = (%q, %v), want (\"\", false)", got, ok)
	}
	if got, ok := eventTimestamp(domain.AccountEvent{}); ok || got != "" {
		t.Errorf("eventTimestamp(AccountEvent) = (%q, %v), want (\"\", false)", got, ok)
	}
}

// TestDeliverablePayloadNamesCoverTheWholeMapping is the naming obligation A6
// states and §5.5 adopts: the error message must name *all* the accepted payloads,
// so a reader cannot infer from the list that only some are possible.
func TestDeliverablePayloadNamesCoverTheWholeMapping(t *testing.T) {
	got := deliverablePayloadNames()
	want := []string{
		"BasicQotNotifyMsgType",
		"BrokerQueueNotifyMsgType",
		"OrderBookNotifyMsgType",
		"TickerNotifyMsgType",
	}
	if len(got) != len(want) {
		t.Fatalf("deliverablePayloadNames = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("deliverablePayloadNames[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

// TestMarketTopicsAndTypesAreDerived pins §6.1's "derived, not declared": both
// lists are computed from the accepted-topic set and the mapping, so a topic mapped
// to a new payload type cannot be mapped but undelivered.
func TestMarketTopicsAndTypesAreDerived(t *testing.T) {
	topics := marketTopics()
	if len(topics) != len(knownTopics) {
		t.Errorf("marketTopics has %d entries, knownTopics has %d", len(topics), len(knownTopics))
	}
	for i := 1; i < len(topics); i++ {
		if topics[i-1] >= topics[i] {
			t.Fatalf("marketTopics is not sorted at %d: %v", i, topics[:i+1])
		}
	}
	types := marketNotifyTypes()
	for i := 1; i < len(types); i++ {
		if types[i-1] >= types[i] {
			t.Fatalf("marketNotifyTypes is not sorted at %d", i)
		}
	}
}

// TestIsTradeNotifyTypeSeparatesTheTwoVocabularies pins §5.6: three of the seven
// payload types have no topic, and the layer's two entry vocabularies are disjoint
// because of it.
func TestIsTradeNotifyTypeSeparatesTheTwoVocabularies(t *testing.T) {
	for _, msgType := range tradeNotifyTypes {
		if !isTradeNotifyType(msgType) {
			t.Errorf("isTradeNotifyType(%s) = false, want true", msgType.String())
		}
	}
	for _, msgType := range marketNotifyTypes() {
		if isTradeNotifyType(msgType) {
			t.Errorf("isTradeNotifyType(%s) = true, want false: a market payload has a topic", msgType.String())
		}
	}
	if isTradeNotifyType(types.NotifyMsgType(4242)) {
		t.Error("isTradeNotifyType(4242) = true, want false")
	}
}

// TestTheOrchestratorToleratesNoTransport covers the degenerate construction: a nil
// TCP half must not panic at construction, and the methods that need it must say so.
func TestTheOrchestratorToleratesNoTransport(t *testing.T) {
	o := NewPushOrchestration(newFakeTopicSubscriber(), nil)
	t.Cleanup(func() { _ = o.Close() })

	if got := o.Unrouted(); got != 0 {
		t.Errorf("Unrouted = %d, want 0", got)
	}
	if err := o.Connect(t.Context()); err == nil {
		t.Error("Connect with no transport = nil error, want one")
	}
	if err := o.Run(t.Context()); err == nil {
		t.Error("Run with no transport = nil error, want one")
	}
	if _, err := o.Subscribe(t.Context(), types.TopicBasicQot,
		&Security{DataType: types.DataTypeHKStock, Code: "00700.HK"}); err == nil {
		t.Error("Subscribe with no transport = nil error, want one")
	}
}

// TestSubscribeWithNoTopicHalfRefuses covers the other degenerate construction.
func TestSubscribeWithNoTopicHalfRefuses(t *testing.T) {
	o := NewPushOrchestration(nil, newFakePushTransport())
	t.Cleanup(func() { _ = o.Close() })

	sub, err := o.Subscribe(t.Context(), types.TopicBasicQot,
		&Security{DataType: types.DataTypeHKStock, Code: "00700.HK"})
	if err == nil {
		t.Fatal("Subscribe with no topic subscriber = nil error, want one")
	}
	if sub != nil {
		t.Error("a subscription was handed back alongside the error")
	}
}

// TestWithBufferAndWithResubscribeTimeoutRestoreTheirDefaults pins the option
// contract: a non-positive value restores the default rather than accepting a
// nonsensical one.
func TestWithBufferAndWithResubscribeTimeoutRestoreTheirDefaults(t *testing.T) {
	o := NewPushOrchestration(newFakeTopicSubscriber(), newFakePushTransport(),
		WithBuffer(0), WithResubscribeTimeout(-time.Second), WithTradeSubscriber(nil))
	t.Cleanup(func() { _ = o.Close() })

	if o.buffer != DefaultPushBuffer {
		t.Errorf("buffer = %d after WithBuffer(0), want %d", o.buffer, DefaultPushBuffer)
	}
	if o.resubTimeout != DefaultResubscribeTimeout {
		t.Errorf("resubTimeout = %v after a negative value, want %v", o.resubTimeout, DefaultResubscribeTimeout)
	}
	if o.trade != nil {
		t.Error("WithTradeSubscriber(nil) left a trade half installed")
	}
	o2 := NewPushOrchestration(newFakeTopicSubscriber(), newFakePushTransport(),
		WithBuffer(8), WithResubscribeTimeout(2*time.Second), nil)
	t.Cleanup(func() { _ = o2.Close() })
	if o2.buffer != 8 {
		t.Errorf("buffer = %d, want 8", o2.buffer)
	}
	if o2.resubTimeout != 2*time.Second {
		t.Errorf("resubTimeout = %v, want 2s", o2.resubTimeout)
	}
}

// TestNotifyTypeForTopicRejectsWhatItDoesNotMap covers the unmapped arm, including
// the zero value: 0 is TrsStockDeliverMsgType, a *valid* type, so a mapping built on
// a map's zero value would deliver order notifications to a quote subscriber.
func TestNotifyTypeForTopicRejectsWhatItDoesNotMap(t *testing.T) {
	for _, topic := range []types.TopicID{0, 1, -1, 999, types.TopicID(11) + 1} {
		if topic == types.TopicBasicQot {
			continue
		}
		msgType, ok := NotifyTypeForTopic(topic)
		if ok {
			t.Errorf("NotifyTypeForTopic(%d) = (%s, true), want false", int(topic), msgType.String())
		}
		if msgType != 0 {
			t.Errorf("NotifyTypeForTopic(%d) returned %s with ok=false, want the zero value: a "+
				"non-zero payload type alongside false would be a caller trap", int(topic), msgType.String())
		}
	}
}

// errTimeoutSentinel is the timeout condition the reconnect-cause test needs; it is
// declared here so the test does not depend on os.ErrDeadlineExceeded's identity
// crossing a package boundary.
var errTimeoutSentinel error = timeoutError{}

type timeoutError struct{}

func (timeoutError) Error() string   { return "i/o timeout" }
func (timeoutError) Timeout() bool   { return true }
func (timeoutError) Temporary() bool { return true }

// ---------------------------------------------------------------------------
// The remaining guards, and the invariants they stand on
// ---------------------------------------------------------------------------

// TestAHandlerDiscardsAnUndeliverableUpdate covers the seam's only defensive guard.
//
// domain.PushUpdate documents Event as never nil for a delivered update, so a nil
// event cannot come off the wire - but the guard costs nothing and the alternative is
// a nil dereference in the routing path, which is the one place a panic would take
// down the read loop for every subscriber of that type.
func TestAHandlerDiscardsAnUndeliverableUpdate(t *testing.T) {
	tr := newFakePushTransport()
	market := newFakeTopicSubscriber()
	o := newTestOrchestrationWith(t, market, newFakeOrderSubscriber(), tr)
	if _, err := o.Subscribe(t.Context(), types.TopicBasicQot,
		&Security{DataType: types.DataTypeHKStock, Code: "00700.HK"}); err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	handler := tr.handler(types.BasicQotNotifyMsgType)

	handler(nil)
	handler(&domain.PushUpdate{Type: types.BasicQotNotifyMsgType, ID: "00700.HK"})

	if got := o.Unrouted(); got != 0 {
		t.Errorf("Unrouted = %d, want 0: an undeliverable update is discarded, not counted as unrouted", got)
	}
	if got := len(tr.errc); got != 0 {
		t.Errorf("the transport stream carries %d error(s), want 0", got)
	}
}

// TestTheDerivedHandlerSetSkipsAnUnmappedTopic covers the one guard in the derived
// handler set.
//
// The set is computed from the accepted-topic set through NotifyTypeForTopic, and an
// accepted topic the mapping does not know must be skipped rather than contribute
// the zero NotifyMsgType - which is TrsStockDeliverMsgType, a real order-status type,
// and would install an order handler on the market half. G1 keeps the two sets in
// step; this covers the branch that would be reached if they ever drifted.
func TestTheDerivedHandlerSetSkipsAnUnmappedTopic(t *testing.T) {
	knownTopics[types.TopicID(4242)] = struct{}{}
	t.Cleanup(func() { delete(knownTopics, types.TopicID(4242)) })

	for _, msgType := range marketNotifyTypes() {
		if msgType == 0 {
			t.Fatalf("marketNotifyTypes includes the zero NotifyMsgType, which is " +
				"TrsStockDeliverMsgType: an unmapped accepted topic contributed it")
		}
		if _, ok := NotifyTypeForTopic(types.TopicID(4242)); ok {
			t.Fatal("the fixture topic 4242 is mapped; the test is not exercising the guard")
		}
	}
	if len(marketNotifyTypes()) != 4 {
		t.Errorf("marketNotifyTypes has %d entries with an unmapped topic accepted, want the 4 "+
			"deliverable payloads", len(marketNotifyTypes()))
	}
}

// TestACloseRacingASubscribeIsRefused covers the closed re-check that sits under the
// subscription lock, in both entry points.
//
// Each method checks for a closed orchestrator once before it takes the lock and once
// after, because Close drains the registry under that same lock: a Subscribe that got
// past the first check and registered after that drain would be left live on a closed
// orchestrator, with a channel nobody will ever close. Only the check under the lock
// rules it out.
//
// It is reached by hand rather than by winning a race, which is why the body is a
// separate function: a test holds no lock, closes the orchestrator, and calls the
// guarded part directly. The end-to-end shape of the same race - a real Close landing
// between the two checks - is a scheduler turn wide and no timing reaches it reliably.
func TestACloseRacingASubscribeIsRefused(t *testing.T) {
	cases := []struct {
		name string
		call func(*testing.T, *PushOrchestration) error
	}{
		{
			name: "Subscribe",
			call: func(t *testing.T, o *PushOrchestration) error {
				_, err := o.subscribeUnderLock(t.Context(), types.TopicBasicQot,
					&Security{DataType: types.DataTypeHKStock, Code: "00700.HK"})
				return err
			},
		},
		{
			name: "SubscribeOrders",
			call: func(t *testing.T, o *PushOrchestration) error {
				_, err := o.subscribeOrdersUnderLock(t.Context(), pushTestAccount())
				return err
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tr := newFakePushTransport()
			market := newFakeTopicSubscriber()
			trade := newFakeOrderSubscriber()
			o := NewPushOrchestration(market, tr, WithTradeSubscriber(trade))
			t.Cleanup(func() { _ = o.Close() })

			if err := o.Close(); err != nil {
				t.Fatalf("Close: %v", err)
			}
			if err := tc.call(t, o); !errors.Is(err, ErrPushClosed) {
				t.Errorf("%s under the lock after Close = %v, want ErrPushClosed", tc.name, err)
			}
			if subs, _ := market.counts(); subs != 0 {
				t.Errorf("%d request(s) reached the Gateway for a refused subscription", subs)
			}
			if subs, _ := trade.counts(); subs != 0 {
				t.Errorf("%d trade request(s) reached the Gateway for a refused subscription", subs)
			}
			if len(o.entries) != 0 || len(o.orders) != 0 {
				t.Errorf("the registry holds %d entrie(s) and %d order subscription(s), want none",
					len(o.entries), len(o.orders))
			}
		})
	}
}

// TestAReconnectAfterCloseTouchesNothing covers the reconnect hook's own closed guard.
//
// A reconnect can land after the orchestrator has shut down; re-subscribing then would
// send HTTP requests for a client the caller has already torn down, and would report
// onto a stream Close has already closed.
func TestAReconnectAfterCloseTouchesNothing(t *testing.T) {
	tr := newFakePushTransport()
	market := newFakeTopicSubscriber()
	trade := newFakeOrderSubscriber()
	o := newTestOrchestrationWith(t, market, trade, tr)

	if _, err := o.Subscribe(t.Context(), types.TopicBasicQot,
		&Security{DataType: types.DataTypeHKStock, Code: "00700.HK"}); err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	if _, err := o.SubscribeOrders(t.Context(), pushTestAccount()); err != nil {
		t.Fatalf("SubscribeOrders: %v", err)
	}
	if err := o.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	o.onReconnect(io.EOF)

	if subs, _ := market.counts(); subs != 1 {
		t.Errorf("/hq/Subscribe called %d time(s) after Close, want 1: no re-subscribe may be issued", subs)
	}
	if subs, _ := trade.counts(); subs != 1 {
		t.Errorf("the trade half was called %d time(s) after Close, want 1: no re-subscribe may be issued", subs)
	}
	if _, open := <-o.Errors(); open {
		t.Error("an error was reported onto a closed stream")
	}
}

// TestTheRegistryAccessorsRefuseAnUnregisteredSubscription covers detachLocked's
// miss.
//
// It is reachable when a subscription's entry has already been dropped - the entry
// goes on its last subscriber leaving, and a caller holding a stale handle can then
// call Cancel. Cancel short-circuits on the shutdown flag first, so the miss is
// exercised directly.
func TestTheRegistryAccessorsRefuseAnUnregisteredSubscription(t *testing.T) {
	tr := newFakePushTransport()
	market := newFakeTopicSubscriber()
	o := newTestOrchestrationWith(t, market, newFakeOrderSubscriber(), tr)

	sub, err := o.Subscribe(t.Context(), types.TopicBasicQot,
		&Security{DataType: types.DataTypeHKStock, Code: "00700.HK"})
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	o.subMu.Lock()
	got := o.detachLocked(sub)
	o.subMu.Unlock()
	if got == nil {
		t.Fatal("detachLocked on a registered subscription = nil, want the entry: the caller needs it " +
			"to know whether to issue an unsubscribe")
	}

	o.subMu.Lock()
	got = o.detachLocked(sub)
	o.subMu.Unlock()
	if got != nil {
		t.Errorf("detachLocked on an already-detached subscription = %+v, want nil: there is no "+
			"entry left to unsubscribe from", got)
	}
	if subs, unsubs := market.counts(); subs != 1 || unsubs != 0 {
		t.Errorf("subscribe/unsubscribe = %d/%d, want 1/0: the miss must not issue a request", subs, unsubs)
	}
}

// TestOneSubscriptionMatchingBothRoutingKeysIsNotDeliveredTwice covers the fan-out
// de-duplication.
//
// A subscription that asked for two instruments is in two buckets, so a frame whose
// notifyId and whose payload code point at those two different instruments matches it
// twice. It must be delivered once: a caller watching two instruments must not see the
// same frame twice because the Gateway named them in two different fields.
func TestOneSubscriptionMatchingBothRoutingKeysIsNotDeliveredTwice(t *testing.T) {
	tr := newFakePushTransport()
	market := newFakeTopicSubscriber()
	o := newTestOrchestrationWith(t, market, newFakeOrderSubscriber(), tr)

	sub, err := o.Subscribe(t.Context(), types.TopicBasicQot,
		&Security{DataType: types.DataTypeHKStock, Code: "00700.HK"},
		&Security{DataType: types.DataTypeHKStock, Code: "00001.HK"})
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	tr.handler(types.BasicQotNotifyMsgType)(&domain.PushUpdate{
		Type:  types.BasicQotNotifyMsgType,
		ID:    "00700.HK",
		Time:  pushTestTime(1_700_000_000_000),
		Event: syntheticMarketEvent(types.BasicQotNotifyMsgType, "00001.HK"),
	})

	if got := len(sub.Updates()); got != 1 {
		t.Errorf("the subscription received %d frame(s), want 1: one subscription reachable through two "+
			"routing keys is still one delivery", got)
	}
}

// TestTheSubscriptionAccessorsReportWhatWasSubscribed covers the two read-only
// accessors, including that the orchestrator's method form agrees with the package
// function it delegates to.
func TestTheSubscriptionAccessorsReportWhatWasSubscribed(t *testing.T) {
	tr := newFakePushTransport()
	market := newFakeTopicSubscriber()
	o := newTestOrchestrationWith(t, market, newFakeOrderSubscriber(), tr)

	sub, err := o.Subscribe(t.Context(), types.TopicOrderBookArcabook,
		&Security{DataType: types.DataTypeHKStock, Code: "00700.HK"})
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	if got := sub.TopicID(); got != types.TopicOrderBookArcabook {
		t.Errorf("TopicID() = %d, want %d", int(got), int(types.TopicOrderBookArcabook))
	}
	if got := sub.NotifyMsgType(); got != types.OrderBookNotifyMsgType {
		t.Errorf("NotifyMsgType() = %s, want OrderBookNotifyMsgType", got.String())
	}

	for _, topic := range []types.TopicID{types.TopicBasicQot, types.TopicBroker, types.TopicID(999)} {
		wantType, wantOK := NotifyTypeForTopic(topic)
		gotType, gotOK := o.NotifyTypeForTopic(topic)
		if gotType != wantType || gotOK != wantOK {
			t.Errorf("the method form for topic %d = (%s, %v), want the package function's (%s, %v)",
				int(topic), gotType.String(), gotOK, wantType.String(), wantOK)
		}
	}
}

// TestConnectDialsAndIsIdempotent covers the success path of Connect, including that
// a second call costs no second dial.
func TestConnectDialsAndIsIdempotent(t *testing.T) {
	tr := newFakePushTransport()
	o := newTestOrchestration(t, tr)
	t.Cleanup(func() { _ = o.Close() })

	if err := o.Connect(t.Context()); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	if err := o.Connect(t.Context()); err != nil {
		t.Fatalf("Connect again: %v", err)
	}
	if got := tr.dials; got != 1 {
		t.Errorf("the transport was dialed %d time(s), want 1: Connect is idempotent", got)
	}
}

// TestReportIgnoresANilError covers the report contract in isolation.
//
// A nil on the client stream would be a value the caller cannot match or log, and
// every call site already has something to report; the guard exists so a future one
// cannot put nil there.
func TestReportIgnoresANilError(t *testing.T) {
	tr := newFakePushTransport()
	o := newTestOrchestration(t, tr)
	o.report(nil)
	if got := drainClientErrors(o); len(got) != 0 {
		t.Errorf("%d error(s) were reported for a nil error", len(got))
	}
	cause := errors.New("a real diagnosis")
	o.report(cause)
	if got := nextClientError(t, o); got != cause {
		t.Errorf("report forwarded %v, want %v", got, cause)
	}
}

// TestTheStreamErrorRendersABareTagWithoutACause covers the nil-cause arm of the
// error shape. It is reachable when a caller raises a package sentinel with nothing
// underneath it, which is exactly the bare-sentinel case newReconnectNotice makes.
func TestTheStreamErrorRendersABareTagWithoutACause(t *testing.T) {
	bare := &streamError{tag: ErrReconnected}
	if got, want := bare.Error(), ErrReconnected.Error(); got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
	if got := errors.Unwrap(bare); got != nil {
		t.Errorf("Unwrap = %v, want nil", got)
	}
	if !errors.Is(bare, ErrReconnected) {
		t.Error("Is(ErrReconnected) = false, want true")
	}
	if errors.Is(bare, io.EOF) {
		t.Error("Is(io.EOF) = true, want false")
	}
}

// TestAFailedUnsubscribeIsReportedAndTheSubscriptionStillDies covers the failure
// path of both Cancel methods.
//
// The released layer's rule: the channels close even when the HTTP call fails, so the
// error comes back but the subscription does not stay live. A local registration that
// outlives a refused unsubscribe is exactly the "caller believes they subscribed and
// receives nothing" outcome this layer exists to avoid.
func TestAFailedUnsubscribeIsReportedAndTheSubscriptionStillDies(t *testing.T) {
	t.Run("market", func(t *testing.T) {
		tr := newFakePushTransport()
		market := newFakeTopicSubscriber()
		o := newTestOrchestrationWith(t, market, newFakeOrderSubscriber(), tr)
		market.unsubErr[1] = errors.New("the Gateway does not know that subscription")

		sub, err := o.Subscribe(t.Context(), types.TopicBasicQot,
			&Security{DataType: types.DataTypeHKStock, Code: "00700.HK"})
		if err != nil {
			t.Fatalf("Subscribe: %v", err)
		}
		if err := sub.Cancel(t.Context()); err == nil {
			t.Fatal("Cancel = nil error, want the Gateway refusal")
		}
		requireClosedNow(t, sub.Updates(), "a Cancel whose unsubscribe was refused")

		reported := requireClientErrorNow(t, o, "the refused unsubscribe")
		if !errors.Is(reported, ErrSubscriptionLost) {
			t.Errorf("reported error = %v, want ErrSubscriptionLost", reported)
		}
		if !strings.Contains(reported.Error(), "does not know that subscription") {
			t.Errorf("reported error = %q, want it to carry the Gateway's reason", reported.Error())
		}
	})

	t.Run("trade", func(t *testing.T) {
		tr := newFakePushTransport()
		trade := newFakeOrderSubscriber()
		o := newTestOrchestrationWith(t, newFakeTopicSubscriber(), trade, tr)
		trade.unsubErr[1] = errors.New("the session will not let go")

		sub, err := o.SubscribeOrders(t.Context(), pushTestAccount())
		if err != nil {
			t.Fatalf("SubscribeOrders: %v", err)
		}
		if err := sub.Cancel(t.Context()); err == nil {
			t.Fatal("Cancel = nil error, want the refusal")
		}
		requireClosedNow(t, sub.Updates(), "a Cancel whose trade unsubscribe was refused")

		reported := requireClientErrorNow(t, o, "the refused trade unsubscribe")
		if !errors.Is(reported, ErrSubscriptionLost) {
			t.Errorf("reported error = %v, want ErrSubscriptionLost", reported)
		}
	})
}

// TestARefusedUnsubscribeAfterAFailedResubscribeIsReported covers the best-effort
// unsubscribe in the kill path.
//
// The Gateway cannot have honoured the subscribe it just refused, so the unsubscribe
// is expected to fail - and its failure is reported rather than swallowed, because a
// Gateway left holding a subscription nobody knows about is worth saying out loud.
func TestARefusedUnsubscribeAfterAFailedResubscribeIsReported(t *testing.T) {
	tr := newFakePushTransport()
	market := newFakeTopicSubscriber()
	o := newTestOrchestrationWith(t, market, newFakeOrderSubscriber(), tr)

	if _, err := o.Subscribe(t.Context(), types.TopicBasicQot,
		&Security{DataType: types.DataTypeHKStock, Code: "00700.HK"}); err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	market.subErr[2] = errors.New("the Gateway refused the reconnect")
	market.unsubErr[1] = errors.New("and will not let go either")

	tr.fireReconnect(io.EOF)

	failures := 0
	for {
		select {
		case err, ok := <-o.Errors():
			if !ok {
				goto done
			}
			if errors.Is(err, ErrSubscriptionLost) {
				failures++
			}
		default:
			goto done
		}
	}
done:
	if failures != 2 {
		t.Errorf("%d ErrSubscriptionLost reported, want 2: the refused re-subscribe and the refused "+
			"unsubscribe that followed it", failures)
	}
	if _, unsubs := market.counts(); unsubs != 1 {
		t.Errorf("unsubscribe count = %d, want 1: the best-effort unsubscribe must still be attempted", unsubs)
	}
}

// TestDeliveryToAGoneSubscriptionIsDiscarded covers the closed guard on both deliver
// paths.
//
// It is reachable whenever a frame is already in flight when the subscription dies -
// a handler invoked from a dispatcher goroutine that was mid-delivery. The guard is
// what keeps that from writing to a channel that is closing.
func TestDeliveryToAGoneSubscriptionIsDiscarded(t *testing.T) {
	tr := newFakePushTransport()
	market := newFakeTopicSubscriber()
	trade := newFakeOrderSubscriber()
	o := newTestOrchestrationWith(t, market, trade, tr)

	sub, err := o.Subscribe(t.Context(), types.TopicBasicQot,
		&Security{DataType: types.DataTypeHKStock, Code: "00700.HK"})
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	order, err := o.SubscribeOrders(t.Context(), pushTestAccount())
	if err != nil {
		t.Fatalf("SubscribeOrders: %v", err)
	}
	if err := sub.Cancel(t.Context()); err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	if err := order.Cancel(t.Context()); err != nil {
		t.Fatalf("Cancel (trade): %v", err)
	}

	// A frame that was already in flight when the subscription died.
	sub.deliver(&domain.PushUpdate{Type: types.BasicQotNotifyMsgType, ID: "00700.HK", Event: domain.SystemEvent{Code: "X"}})
	order.deliver(&domain.PushUpdate{Type: types.TradeStockDeliverMsgType, ID: "00700.HK", Event: domain.SystemEvent{Code: "X"}}, o)

	// Shutting a subscription down twice must also be a no-op, not a second close.
	sub.shutdown()
	order.shutdown()
	if !sub.isClosed() || !order.isClosed() {
		t.Error("a subscription reported itself live after being shut down twice")
	}

	// And a second Cancel reports success without a second request.
	if err := sub.Cancel(t.Context()); err != nil {
		t.Errorf("second Cancel = %v, want nil: Cancel is idempotent", err)
	}
	if err := order.Cancel(t.Context()); err != nil {
		t.Errorf("second Cancel (trade) = %v, want nil", err)
	}
}
