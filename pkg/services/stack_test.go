// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package services

import (
	"context"
	"errors"
	"testing"

	"github.com/shing1211/hstongapi4go/client"
	"github.com/shing1211/hstongapi4go/pkg/domain"
)

// orderingExecutor asserts, from inside its own Close, that the push transport has
// already been closed.
//
// That is the direct proof of Stack.Close's documented order. Asserting after
// Close returns would only prove that both were closed, which is a weaker claim
// than the one the doc makes; asserting from inside Close fails if the order is
// ever reversed, at the moment the inversion is still observable.
type orderingExecutor struct {
	*sequencedExecutor
	transport *fakePushTransport
	t         *testing.T
}

func (o *orderingExecutor) Close() error {
	if !o.transport.isClosed() {
		o.t.Error("Stack.Close released the request path before the push transport: " +
			"a live TCP connection was left pointing at a closed HTTP client")
	}
	return o.sequencedExecutor.Close()
}

// TestNewStackBuildsEveryService pins that construction fills all six service
// fields and, without a transport, leaves Push nil.
//
// The nil-Push half is not a degenerate case: a caller that only wants REST must
// not be handed a TCP client it never asked for, so "no push" has to be the
// default rather than an error state.
func TestNewStackBuildsEveryService(t *testing.T) {
	exec := newSequencedExecutor(t)
	s := NewStack(exec)

	for _, c := range []struct {
		name string
		got  any
	}{
		{"Market", s.Market},
		{"Account", s.Account},
		{"Trading", s.Trading},
		{"Algo", s.Algo},
		{"Futures", s.Futures},
		{"Session", s.Session},
	} {
		if isNilValue(c.got) {
			t.Errorf("NewStack left %s nil; every service field is set by construction", c.name)
		}
	}

	if s.Push != nil {
		t.Errorf("Push = %v, want nil: no push transport was given", s.Push)
	}
}

// isNilValue reports whether a non-nil interface holds a nil pointer, which is the
// only way a *constructed* service can still be absent.
func isNilValue(v any) bool {
	switch t := v.(type) {
	case *MarketService:
		return t == nil
	case *AccountService:
		return t == nil
	case *TradingService:
		return t == nil
	case *AlgoService:
		return t == nil
	case *FuturesService:
		return t == nil
	case *SessionService:
		return t == nil
	}
	return v == nil
}

// TestPushIsBuiltOnlyWhenATransportIsGiven is the other half of the opt-in: given a
// transport, Push exists.
func TestPushIsBuiltOnlyWhenATransportIsGiven(t *testing.T) {
	s := NewStack(newSequencedExecutor(t), WithPushTransport(newFakePushTransport()))

	if s.Push == nil {
		t.Fatal("Push = nil after WithPushTransport; the opt-in produced nothing")
	}
}

// TestTheTradeHalfIsWiredThroughTheOption is the regression test for the defect
// the D1 design note shipped with.
//
// The note's NewStack called NewPushOrchestration with three required arguments and
// passed *TradingService where a PushOption belonged. It did not compile, which was
// the lucky outcome: had the arity lined up, the trade HTTP half would have been
// dropped and SubscribeOrders would have had no route to /trade/TradeSubscribe
// while still appearing to work. The trade half arrives as WithTradeSubscriber, an
// option that is invisible in the parameter list, so it is pinned here by its
// observable effect - a real request on the released route - rather than by
// asserting on a field.
func TestTheTradeHalfIsWiredThroughTheOption(t *testing.T) {
	// Two replies, not one: the deferred Cancel below issues the unsubscribe, and
	// a fixture shorter than the code under test fails the test rather than
	// quietly passing on a stale reply.
	exec := newSequencedExecutor(t, sequencedReply{reply: &struct{}{}}, sequencedReply{reply: &struct{}{}})
	s := NewStack(exec, WithPushTransport(newFakePushTransport()))

	sub, err := s.Push.SubscribeOrders(context.Background(), domain.AccountID("ACC-STACK"))
	if err != nil {
		t.Fatalf("SubscribeOrders: %v; the trade half is not wired", err)
	}
	defer func() { _ = sub.Cancel(context.Background()) }()

	requireCalls(t, exec, 1)
	if got := exec.callAt(t, 0).route; got != client.RouteTradeSubscribe {
		t.Errorf("the order subscription used %q, want %q: WithTradeSubscriber was not "+
			"passed, so SubscribeOrders has no HTTP half", got, client.RouteTradeSubscribe)
	}
}

// TestCloseClosesPushBeforeTheRequestPath pins the documented order from inside the
// executor's Close. See orderingExecutor.
func TestCloseClosesPushBeforeTheRequestPath(t *testing.T) {
	tr := newFakePushTransport()
	base := newSequencedExecutor(t)
	exec := &orderingExecutor{sequencedExecutor: base, transport: tr, t: t}

	s := NewStack(exec, WithPushTransport(tr))
	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	if !tr.isClosed() {
		t.Error("the push transport was not closed; PushOrchestration.Close owns it")
	}
	if got := base.closeCount(); got != 1 {
		t.Errorf("the request path was closed %d times, want 1", got)
	}
}

// TestCloseIsIdempotent pins that the second Close neither repeats the work nor
// changes the answer.
func TestCloseIsIdempotent(t *testing.T) {
	tr := newFakePushTransport()
	exec := newSequencedExecutor(t)
	exec.closeErr = errors.New("close failed")

	s := NewStack(exec, WithPushTransport(tr))

	first := s.Close()
	second := s.Close()

	if first == nil || second == nil {
		t.Fatal("Close returned nil for an executor configured to fail; the error is not surfacing")
	}
	if first.Error() != second.Error() {
		t.Errorf("the second Close returned %q, want the first result %q", second, first)
	}
	if got := exec.closeCount(); got != 1 {
		t.Errorf("the request path was closed %d times across two Close calls, want 1", got)
	}
}

// TestCloseStillClosesPushWhenTheExecutorFails pins that both closes are attempted.
//
// Stopping at the first failure would be the obvious reading of "return the first
// error", and it would leave a socket open behind a failed HTTP close - trading one
// leak for another.
func TestCloseStillClosesPushWhenTheExecutorFails(t *testing.T) {
	tr := newFakePushTransport()
	exec := newSequencedExecutor(t)
	exec.closeErr = errors.New("close failed")

	s := NewStack(exec, WithPushTransport(tr))
	err := s.Close()

	if err == nil || err.Error() != "close failed" {
		t.Fatalf("Close = %v, want the executor's error", err)
	}
	if !tr.isClosed() {
		t.Error("the push transport was left open after the executor failed to close")
	}
}

// TestCloseOnAStackWithNothingToRelease covers the two reachable states where
// Close has no work at all, plus a nil receiver.
//
// A Stack built over a nil executor is constructible - nil satisfies the interface -
// so Close must not dereference it, and a nil *Stack must not panic either. Neither
// is a state the constructor rejects, so neither can be a state Close panics on.
func TestCloseOnAStackWithNothingToRelease(t *testing.T) {
	t.Run("no push and no executor", func(t *testing.T) {
		if err := NewStack(nil).Close(); err != nil {
			t.Errorf("Close = %v, want nil", err)
		}
	})

	t.Run("a nil Stack", func(t *testing.T) {
		var s *Stack
		if err := s.Close(); err != nil {
			t.Errorf("a nil Stack's Close = %v, want nil", err)
		}
	})
}

// TestANilOptionIsSkipped pins that a conditionally-built option slice is safe to
// pass through, which is why the loop guards opt != nil rather than trusting the
// caller.
func TestANilOptionIsSkipped(t *testing.T) {
	s := NewStack(newSequencedExecutor(t), nil, WithPushTransport(newFakePushTransport()))

	if s.Market == nil {
		t.Error("a nil option aborted construction; the remaining options were not applied")
	}
	if s.Push == nil {
		t.Error("a nil option aborted construction before the push transport was applied")
	}
}

// TestWithStackExecutorReplacesWhatCloseReleases pins the option's documented
// effect, including its documented limit: the six services keep the executor they
// were built with.
func TestWithStackExecutorReplacesWhatCloseReleases(t *testing.T) {
	first := newSequencedExecutor(t)
	second := newSequencedExecutor(t)
	tr := newFakePushTransport()

	s := NewStack(first, WithStackExecutor(second), WithPushTransport(tr))
	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	if got := second.closeCount(); got != 1 {
		t.Errorf("the replacement executor was closed %d times, want 1", got)
	}
	if got := first.closeCount(); got != 0 {
		t.Errorf("the original executor was closed %d times, want 0: it was replaced "+
			"before Close ran", got)
	}
	if !tr.isClosed() {
		t.Error("the push transport was not closed")
	}
}
