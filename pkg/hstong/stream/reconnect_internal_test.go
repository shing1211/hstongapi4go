// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package stream

import (
	"context"
	"errors"
	"testing"

	"github.com/shing1211/hstongapi4go/internal/errs"
	"github.com/shing1211/hstongapi4go/pkg/types"
)

// errReconnectCause is the error onReconnect reports wrapping ErrReconnected. A
// test uses the same cause for every subscription so the notice it asserts on
// is the same value throughout.
func newReconnectCause() error { return errors.New("push connection reset by the Gateway") }

// TestOnReconnectAfterCloseTouchesNothing covers the closed guard. A push
// reconnect can land after the stream client has shut down; re-subscribing then
// would send HTTP requests for a client the caller has already torn down, and
// would report onto channels that Close has already closed.
func TestOnReconnectAfterCloseTouchesNothing(t *testing.T) {
	t.Run("after Close", func(t *testing.T) {
		srv := newHookServer(t, nil, nil)
		tp := &scriptedPusher{}
		s := newTestClient(t, srv.url, &pipeDialer{}, WithTradeManager(tp))

		marketSub, err := s.Subscribe(context.Background(), types.TopicBasicQot, hkSecurity())
		if err != nil {
			t.Fatalf("Subscribe: %v", err)
		}
		tradeSub, err := s.SubscribeTrade(context.Background())
		if err != nil {
			t.Fatalf("SubscribeTrade: %v", err)
		}
		if got := srv.count(subscribePath); got != 1 {
			t.Fatalf("/hq/Subscribe called %d times, want 1 before the reconnect", got)
		}
		if subs, unsubs := tp.counts(); subs != 1 || unsubs != 0 {
			t.Fatalf("trade pusher subscribe/unsubscribe = %d/%d, want 1/0", subs, unsubs)
		}

		if err := s.Close(); err != nil {
			t.Fatalf("Close: %v", err)
		}
		s.onReconnect(newReconnectCause())

		if got := srv.count(subscribePath); got != 1 {
			t.Errorf("/hq/Subscribe called %d times after Close, want 1: no resubscribe may be issued", got)
		}
		if subs, unsubs := tp.counts(); subs != 1 || unsubs != 0 {
			t.Errorf("trade pusher called after Close: subscribe=%d unsubscribe=%d, want 1/0", subs, unsubs)
		}
		for name, sub := range map[string]*Subscription{"market": marketSub, "trade": tradeSub} {
			if got := drainErrors(sub); len(got) != 0 {
				t.Errorf("%s subscription received %d errors after Close, want 0: %v", name, len(got), got)
			}
		}
	})

	t.Run("closed with subscriptions still registered", func(t *testing.T) {
		// shutdown empties the subscription map in the same locked transition
		// that sets the closed flag, so after a real Close the map alone already
		// makes onReconnect a no-op. Set the flag directly to pin the guard's
		// own contract: however the client came to be closed, a closed client
		// must re-subscribe nothing and report nothing, even if it still holds
		// live subscriptions. Without the guard this subtest issues two HTTP
		// resubscribes and fills both error channels.
		srv := newHookServer(t, nil, nil)
		tp := &scriptedPusher{}
		s := newTestClient(t, srv.url, &pipeDialer{}, WithTradeManager(tp))

		marketSub, err := s.Subscribe(context.Background(), types.TopicBasicQot, hkSecurity())
		if err != nil {
			t.Fatalf("Subscribe: %v", err)
		}
		tradeSub, err := s.SubscribeTrade(context.Background())
		if err != nil {
			t.Fatalf("SubscribeTrade: %v", err)
		}

		s.mu.Lock()
		s.closed = true
		s.mu.Unlock()
		if got := subscriptionCount(s); got != 2 {
			t.Fatalf("registered %d subscriptions, want 2", got)
		}

		s.onReconnect(newReconnectCause())

		if got := srv.count(subscribePath); got != 1 {
			t.Errorf("/hq/Subscribe called %d times on a closed client, want 1 (no resubscribe)", got)
		}
		if subs, unsubs := tp.counts(); subs != 1 || unsubs != 0 {
			t.Errorf("trade pusher called on a closed client: subscribe=%d unsubscribe=%d, want 1/0", subs, unsubs)
		}
		for name, sub := range map[string]*Subscription{"market": marketSub, "trade": tradeSub} {
			if got := drainErrors(sub); len(got) != 0 {
				t.Errorf("%s subscription received %d errors on a closed client, want 0: %v", name, len(got), got)
			}
		}
	})
}

// TestOnReconnectResubscribesMarketTopicsOnly covers the resubscribe fan-out. A
// market subscription is restored over the market HTTP endpoint and the
// session-wide trade subscription through the trade manager instead. A trade
// subscription carries no market topic, so sending it to /hq/Subscribe would be
// a request the Gateway can only reject, and the rejection would be reported to
// the caller as a resubscribe failure on a subscription that never needed one.
func TestOnReconnectResubscribesMarketTopicsOnly(t *testing.T) {
	srv := newHookServer(t, nil, nil)
	tp := &scriptedPusher{}
	s := newTestClient(t, srv.url, &pipeDialer{}, WithTradeManager(tp))

	ctx := context.Background()
	marketSub, err := s.Subscribe(ctx, types.TopicBasicQot, hkSecurity())
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	tradeSub, err := s.SubscribeTrade(ctx)
	if err != nil {
		t.Fatalf("SubscribeTrade: %v", err)
	}
	if subs, _ := tp.counts(); subs != 1 {
		t.Fatalf("trade SubscribeOrders called %d times before the reconnect, want 1", subs)
	}

	cause := newReconnectCause()
	s.onReconnect(cause)

	if got := srv.count(subscribePath); got != 2 {
		t.Errorf("/hq/Subscribe called %d times, want 2 (one initial, one market resubscribe)", got)
	}
	if subs, unsubs := tp.counts(); subs != 2 || unsubs != 0 {
		t.Errorf("trade pusher subscribe/unsubscribe = %d/%d, want 2/0", subs, unsubs)
	}
	for name, sub := range map[string]*Subscription{"market": marketSub, "trade": tradeSub} {
		assertReconnectNotice(t, nextError(t, sub), cause)
		if got := drainErrors(sub); len(got) != 0 {
			t.Errorf("%s subscription received %d further errors, want 0: %v", name, len(got), got)
		}
	}
}

// TestOnReconnectReportsResubscribeFailures covers both failure reports. A
// resubscribe the Gateway refused must be reported on the subscription it
// belongs to and on no other, because a caller reading Errors() has to be able
// to tell which of its subscriptions the Gateway would not restore.
func TestOnReconnectReportsResubscribeFailures(t *testing.T) {
	t.Run("market resubscribe refused", func(t *testing.T) {
		// The first /hq/Subscribe succeeds so the subscription is established;
		// every later one fails, which is the reconnect.
		srv := newHookServer(t, func(_ string, call int) string {
			if call == 1 {
				return okEnvelope
			}
			return failEnvelope
		}, nil)
		tp := &scriptedPusher{}
		s := newTestClient(t, srv.url, &pipeDialer{}, WithTradeManager(tp))

		ctx := context.Background()
		marketSub, err := s.Subscribe(ctx, types.TopicBasicQot, hkSecurity())
		if err != nil {
			t.Fatalf("Subscribe: %v", err)
		}
		tradeSub, err := s.SubscribeTrade(ctx)
		if err != nil {
			t.Fatalf("SubscribeTrade: %v", err)
		}

		cause := newReconnectCause()
		s.onReconnect(cause)

		assertReconnectNotice(t, nextError(t, marketSub), cause)
		failure := nextError(t, marketSub)
		if code, ok := errs.CodeOf(failure); !ok || code != types.StatusServiceBusy {
			t.Errorf("market resubscribe failure = %v (code %q, ok=%v), want StatusServiceBusy", failure, code, ok)
		}
		if got := drainErrors(marketSub); len(got) != 0 {
			t.Errorf("market subscription received %d further errors, want 0: %v", len(got), got)
		}

		// The trade resubscribe in the same pass succeeded, so the trade
		// subscription must report the notice and nothing else.
		assertReconnectNotice(t, nextError(t, tradeSub), cause)
		if got := drainErrors(tradeSub); len(got) != 0 {
			t.Errorf("trade subscription received %d further errors, want 0: %v", len(got), got)
		}
	})

	t.Run("trade resubscribe refused", func(t *testing.T) {
		resubErr := errors.New("trade subscribe refused by the session")
		tp := &scriptedPusher{subErr: map[int]error{2: resubErr}}
		srv := newHookServer(t, nil, nil)
		s := newTestClient(t, srv.url, &pipeDialer{}, WithTradeManager(tp))

		ctx := context.Background()
		marketSub, err := s.Subscribe(ctx, types.TopicBasicQot, hkSecurity())
		if err != nil {
			t.Fatalf("Subscribe: %v", err)
		}
		tradeSub, err := s.SubscribeTrade(ctx)
		if err != nil {
			t.Fatalf("SubscribeTrade: %v", err)
		}

		cause := newReconnectCause()
		s.onReconnect(cause)

		assertReconnectNotice(t, nextError(t, tradeSub), cause)
		failure := nextError(t, tradeSub)
		if !errors.Is(failure, resubErr) {
			t.Errorf("trade resubscribe failure = %v, want it to wrap %v", failure, resubErr)
		}
		if got := drainErrors(tradeSub); len(got) != 0 {
			t.Errorf("trade subscription received %d further errors, want 0: %v", len(got), got)
		}

		// The market resubscribe in the same pass succeeded, so the market
		// subscription must report the notice and nothing else.
		assertReconnectNotice(t, nextError(t, marketSub), cause)
		if got := drainErrors(marketSub); len(got) != 0 {
			t.Errorf("market subscription received %d further errors, want 0: %v", len(got), got)
		}
		if subs, _ := tp.counts(); subs != 2 {
			t.Errorf("trade SubscribeOrders called %d times, want 2 (one initial, one resubscribe)", subs)
		}
	})
}

// TestSubscribeFailureModes covers the four ways Subscribe refuses a caller.
// Every one of them must be reported before a Subscription is handed back, so a
// caller is never told to Cancel a subscription it never received, and no
// request may reach the Gateway for input the stream client rejects itself.
func TestSubscribeFailureModes(t *testing.T) {
	t.Run("no underlying client", func(t *testing.T) {
		s := New(nil)
		t.Cleanup(func() { _ = s.Close() })

		if _, err := s.Subscribe(context.Background(), types.TopicBasicQot, hkSecurity()); !errors.Is(err, ErrNoClient) {
			t.Errorf("Subscribe without a client = %v, want ErrNoClient", err)
		}
	})

	t.Run("after Close", func(t *testing.T) {
		srv := newHookServer(t, nil, nil)
		s := newTestClient(t, srv.url, &pipeDialer{})

		if err := s.Close(); err != nil {
			t.Fatalf("Close: %v", err)
		}
		if _, err := s.Subscribe(context.Background(), types.TopicBasicQot, hkSecurity()); !errors.Is(err, ErrClosed) {
			t.Errorf("Subscribe after Close = %v, want ErrClosed", err)
		}
		if got := srv.count(subscribePath); got != 0 {
			t.Errorf("sent %d subscribe requests after Close, want 0", got)
		}
	})

	t.Run("the Gateway refuses the subscribe", func(t *testing.T) {
		srv := newHookServer(t, func(string, int) string { return failEnvelope }, nil)
		s := newTestClient(t, srv.url, &pipeDialer{})

		sub, err := s.Subscribe(context.Background(), types.TopicBasicQot, hkSecurity())
		if err == nil {
			t.Fatal("Subscribe = nil error, want the Gateway failure")
		}
		if sub != nil {
			t.Error("a subscription was returned alongside the error")
		}
		if code, ok := errs.CodeOf(err); !ok || code != types.StatusServiceBusy {
			t.Errorf("Subscribe error = %v (code %q, ok=%v), want StatusServiceBusy", err, code, ok)
		}
		if got := subscriptionCount(s); got != 0 {
			t.Errorf("registered %d subscriptions after a failed subscribe, want 0", got)
		}
	})

	t.Run("closed while the subscribe is in flight", func(t *testing.T) {
		// The server hook runs before the reply is written, so the client cannot
		// leave its HTTP call until Close has completed. That turns the window
		// between the subscribe and the registration into a handshake.
		var s *Client
		srv := newHookServer(t, nil, func(string, int) { _ = s.Close() })
		s = newTestClient(t, srv.url, &pipeDialer{})

		sub, err := s.Subscribe(context.Background(), types.TopicBasicQot, hkSecurity())
		if !errors.Is(err, ErrClosed) {
			t.Errorf("Subscribe closed in flight = %v, want ErrClosed", err)
		}
		if sub != nil {
			t.Error("a subscription was returned after the client was closed")
		}
		if got := subscriptionCount(s); got != 0 {
			t.Errorf("registered %d subscriptions on a closed client, want 0", got)
		}
	})
}

// TestSubscribeTradeFailureModes mirrors TestSubscribeFailureModes for the
// trade surface. Order push is session-wide, so a subscribe the Gateway refuses
// must leave the trade subscription count at zero: a later successful subscribe
// must still be the first one, and must therefore still issue the HTTP call.
func TestSubscribeTradeFailureModes(t *testing.T) {
	t.Run("no underlying client", func(t *testing.T) {
		s := New(nil)
		t.Cleanup(func() { _ = s.Close() })

		if _, err := s.SubscribeTrade(context.Background()); !errors.Is(err, ErrNoClient) {
			t.Errorf("SubscribeTrade without a client = %v, want ErrNoClient", err)
		}
	})

	t.Run("after Close", func(t *testing.T) {
		srv := newHookServer(t, nil, nil)
		tp := &scriptedPusher{}
		s := newTestClient(t, srv.url, &pipeDialer{}, WithTradeManager(tp))

		if err := s.Close(); err != nil {
			t.Fatalf("Close: %v", err)
		}
		if _, err := s.SubscribeTrade(context.Background()); !errors.Is(err, ErrClosed) {
			t.Errorf("SubscribeTrade after Close = %v, want ErrClosed", err)
		}
		if subs, _ := tp.counts(); subs != 0 {
			t.Errorf("trade SubscribeOrders called %d times after Close, want 0", subs)
		}
	})

	t.Run("the session refuses the subscribe", func(t *testing.T) {
		subErr := errors.New("trade subscribe refused")
		tp := &scriptedPusher{subErr: map[int]error{1: subErr}}
		srv := newHookServer(t, nil, nil)
		s := newTestClient(t, srv.url, &pipeDialer{}, WithTradeManager(tp))

		sub, err := s.SubscribeTrade(context.Background())
		if !errors.Is(err, subErr) {
			t.Fatalf("SubscribeTrade = %v, want it to wrap %v", err, subErr)
		}
		if sub != nil {
			t.Error("a subscription was returned alongside the error")
		}
		if got := subscriptionCount(s); got != 0 {
			t.Errorf("registered %d subscriptions after a refused subscribe, want 0", got)
		}
		if s.tradeSubs != 0 {
			t.Errorf("tradeSubs = %d after a refused subscribe, want 0", s.tradeSubs)
		}
	})

	t.Run("closed while the subscribe is in flight", func(t *testing.T) {
		// The pusher hook runs on SubscribeTrade's own goroutine, so Close has
		// completed by the time the registration lock is taken.
		var s *Client
		tp := &scriptedPusher{}
		srv := newHookServer(t, nil, nil)
		tp.onSub = func(int) { _ = s.Close() }
		s = newTestClient(t, srv.url, &pipeDialer{}, WithTradeManager(tp))

		sub, err := s.SubscribeTrade(context.Background())
		if !errors.Is(err, ErrClosed) {
			t.Errorf("SubscribeTrade closed in flight = %v, want ErrClosed", err)
		}
		if sub != nil {
			t.Error("a subscription was returned after the client was closed")
		}
		if got := subscriptionCount(s); got != 0 {
			t.Errorf("registered %d subscriptions on a closed client, want 0", got)
		}
		if s.tradeSubs != 0 {
			t.Errorf("tradeSubs = %d on a closed client, want 0", s.tradeSubs)
		}
	})
}
