// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package e2e

import (
	"context"
	"testing"
	"time"

	"github.com/shing1211/hstongapi4go/client"
	"github.com/shing1211/hstongapi4go/gen/hq/dto"
	tradenotify "github.com/shing1211/hstongapi4go/gen/trade/notify"
	"github.com/shing1211/hstongapi4go/pkg/domain"
	"github.com/shing1211/hstongapi4go/pkg/services"
	"github.com/shing1211/hstongapi4go/pkg/transport"
	"github.com/shing1211/hstongapi4go/pkg/types"
	"github.com/shing1211/hstongapi4go/test/mockgateway"
)

// vnextStack is the v-next composition under test, wired to a real *client.Client
// and a real push adapter against the in-repo mock Gateway.
//
// Verification stays off on both halves, which is not an omission: the mock does
// not sign push frames (ADR 0005, and test/mockgateway/doc.go says so), and the
// Gateway owns the trade password.
func newVNextStack(t *testing.T) (*mockgateway.Server, *services.Stack) {
	t.Helper()

	srv := mockgateway.New(mockgateway.WithPushAddr("127.0.0.1:0"))
	srv.StartT(t)

	cli, err := client.New(
		client.WithBaseURL(srv.HTTPBaseURL()),
		client.WithPushAddr(srv.PushAddr()),
		client.WithTradePassword("123456"),
	)
	if err != nil {
		t.Fatalf("client.New: %v", err)
	}

	adapter, err := transport.NewPushAdapter(transport.WithPushAddr(srv.PushAddr()))
	if err != nil {
		_ = cli.Close()
		t.Fatalf("transport.NewPushAdapter: %v", err)
	}

	stack := services.NewStack(cli, services.WithPushTransport(adapter))
	// The Stack owns both the push transport and the client, so Close is the only
	// teardown this test needs. Registering cli.Close separately would mask a
	// Stack that failed to close it, which is one of the things being checked.
	t.Cleanup(func() { _ = stack.Close() })
	return srv, stack
}

// TestE2E_VNextStackDrivesTheRealGateway walks the whole wired path: the REST
// half over real HTTP with the real wire codec, then the push half over real TCP
// with real protobuf frames, then a single Close that must release both.
//
// This is the first exercise of the v-next layer against a real wire. Everything
// before it drove pkg/services with a fake Executor, which proves the services'
// logic but cannot catch a composition that is wired to the wrong address, sends
// the wrong route, or never connects.
func TestE2E_VNextStackDrivesTheRealGateway(t *testing.T) {
	srv, stack := newVNextStack(t)
	ctx := context.Background()

	// --- REST half ---------------------------------------------------------
	//
	// A real request through Stack.Market proves the Stack handed its services a
	// live Executor: the call decodes a fixture served over a real socket, which a
	// fake could not.
	tk, err := stack.Market.Ticker(ctx, services.TickerRequest{
		Security: &services.Security{DataType: types.DataTypeHKStock, Code: "00700.HK"},
		Limit:    10,
	})
	if err != nil {
		t.Fatalf("Stack.Market.Ticker over the real Gateway: %v", err)
	}
	if len(tk.Ticker) == 0 {
		t.Fatal("Ticker returned no ticks; the Stack's request path reached no fixture")
	}

	// --- push half ---------------------------------------------------------
	runCtx, cancelRun := context.WithCancel(ctx)
	defer cancelRun()

	if err := stack.Push.Connect(runCtx); err != nil {
		t.Fatalf("Stack.Push.Connect: %v", err)
	}
	runDone := make(chan error, 1)
	go func() { runDone <- stack.Push.Run(runCtx) }()
	t.Cleanup(func() {
		cancelRun()
		select {
		case <-runDone:
		case <-time.After(3 * time.Second):
			t.Error("Stack.Push.Run did not return within 3s of its context being cancelled")
		}
	})

	quoteSub, err := stack.Push.Subscribe(ctx, types.TopicBasicQot,
		&services.Security{DataType: types.DataTypeHKStock, Code: "00700.HK"})
	if err != nil {
		t.Fatalf("Stack.Push.Subscribe: %v", err)
	}
	waitForClients(t, srv, 1)

	if err := srv.EmitQuote(
		&dto.Security{DataType: int32(types.DataTypeHKStock), Code: "00700.HK"},
		&dto.BasicQot{LastPrice: 388.0},
	); err != nil {
		t.Fatalf("EmitQuote: %v", err)
	}

	// The v-next envelope, not the released layer's Event: Type, ID, Time and a
	// non-nil Event. A quote frame proves the *domain.PushUpdate seam end to end,
	// which is the thing C11 introduced and nothing outside pkg/services has
	// exercised yet.
	var quote domain.QuoteEvent
	awaitUpdate(t, quoteSub.Updates(), "quote", func(u domain.PushUpdate) bool {
		quote, _ = u.Event.(domain.QuoteEvent)
		return quote.Symbol.Code == "00700.HK"
	})

	// --- the trade half, which D1 got wrong ---------------------------
	//
	// D1's NewStack snippet passed *TradingService where a PushOption belonged, so
	// the trade HTTP half would have been dropped had the arity not stopped it
	// compiling. pkg/services pins that with a fake; only a real request against
	// the mock's recorded /trade/TradeSubscribe call proves the wiring survived
	// composition.
	ordSub, err := stack.Push.SubscribeOrders(ctx, domain.AccountID("ACC-E2E"))
	if err != nil {
		t.Fatalf("Stack.Push.SubscribeOrders: %v", err)
	}
	if !srv.TradeSubscribed() {
		t.Fatal("the mock did not record /trade/TradeSubscribe: the trade half is not wired")
	}

	if err := srv.EmitTradeDeliver(&tradenotify.TradeStockDeliverNotify{
		StockCode: "00700.HK",
		EntrustNo: "E2E-1",
	}); err != nil {
		t.Fatalf("EmitTradeDeliver: %v", err)
	}
	var trade domain.TradeEvent
	awaitUpdate(t, ordSub.Updates(), "trade delivery", func(u domain.PushUpdate) bool {
		trade, _ = u.Event.(domain.TradeEvent)
		return trade.OrderID == "E2E-1" || trade.EntrustID == "E2E-1"
	})

	// --- one Close releases both halves ---------------------------------
	if err := stack.Close(); err != nil {
		t.Fatalf("Stack.Close: %v", err)
	}
	// Close is idempotent, and a second call must not panic or re-close: the
	// orchestrator closes the push transport it owns, so doing it twice would be
	// the double-close Stack's own doc rules out.
	if err := stack.Close(); err != nil {
		t.Fatalf("the second Stack.Close: %v; it must return the first result", err)
	}
	if _, err := stack.Push.Subscribe(ctx, types.TopicBasicQot,
		&services.Security{DataType: types.DataTypeHKStock, Code: "00700.HK"}); err == nil {
		t.Error("Subscribe after Close succeeded; the Stack kept serving after release")
	}
}

// TestE2E_VNextStackWithoutPushMakesNoConnection pins the opt-in from the outside:
// a Stack built with no push transport must never open the TCP socket, and must
// still serve the REST half.
func TestE2E_VNextStackWithoutPushMakesNoConnection(t *testing.T) {
	srv := mockgateway.New(mockgateway.WithPushAddr("127.0.0.1:0"))
	srv.StartT(t)

	cli, err := client.New(
		client.WithBaseURL(srv.HTTPBaseURL()),
		client.WithPushAddr(srv.PushAddr()),
	)
	if err != nil {
		t.Fatalf("client.New: %v", err)
	}
	stack := services.NewStack(cli)
	t.Cleanup(func() { _ = stack.Close() })

	if stack.Push != nil {
		t.Error("Push is non-nil with no transport given; the opt-in is not opt-in")
	}
	if got := srv.ClientCount(); got != 0 {
		t.Errorf("the mock saw %d push connections, want 0: a REST-only Stack must not dial", got)
	}

	// The REST half still works, which is the point of the split: the six services
	// do not depend on the push half existing.
	if _, err := stack.Market.Ticker(context.Background(), services.TickerRequest{
		Security: &services.Security{DataType: types.DataTypeHKStock, Code: "00700.HK"},
		Limit:    10,
	}); err != nil {
		t.Fatalf("a Stack with no push must still serve REST: %v", err)
	}
}

// awaitUpdate reads from ch until accept returns true for a delivered update, or
// fails the test on timeout.
//
// It never sleeps and never polls: the channel is the synchronisation. A delivered
// update whose payload is not the one being waited for is a real failure and is
// reported with what arrived, because "timed out" alone would hide a
// misrouted frame behind an empty assertion.
func awaitUpdate(t *testing.T, ch <-chan domain.PushUpdate, what string, accept func(domain.PushUpdate) bool) {
	t.Helper()
	timeout := time.After(3 * time.Second)
	for {
		select {
		case u, ok := <-ch:
			if !ok {
				t.Fatalf("the %s updates channel closed before the expected update arrived", what)
			}
			if u.Event == nil {
				t.Fatalf("a delivered %s update has a nil Event; the seam guarantees one", what)
			}
			if accept(u) {
				return
			}
			t.Fatalf("a %s update arrived but did not match: Type=%v ID=%q SecurityCode=%q event=%T",
				what, u.Type, u.ID, u.SecurityCode(), u.Event)
		case <-timeout:
			t.Fatalf("timed out after 3s waiting for the %s update", what)
		}
	}
}
