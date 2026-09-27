// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package services

import (
	"context"
	"errors"
	"testing"

	"github.com/shing1211/hstongapi4go/client"
	"github.com/shing1211/hstongapi4go/internal/errs"
	"github.com/shing1211/hstongapi4go/pkg/types"
)

// This file covers the failure side of the market surface: the branch every
// method takes once Executor.Do returns an error.
//
// It exists because that branch was the package's largest hole. A2 measured the
// identical shape one layer down: twenty endpoints in pkg/hstong/trade, a fully
// covered happy path, and a `return ..., err` statement no test had ever
// reached, with the symptom that four mutations all sat at exactly 83.3%. Here
// nine of the eleven methods carried the same unexercised statement.
//
// Two levels, both required. Level 1 drives a sequencedExecutor returning a
// sentinel: it proves the error is propagated unchanged, that the zero value is
// returned beside it, and that exactly one Do was recorded. Level 2 drives a
// real *client.Client over a wireRecorder answering with a real ok:false
// envelope, and asserts the typed *errs.Error the transport actually builds,
// which is something a fake can never show because a fake never builds a typed
// error at all. A service that laundered a Gateway rejection into an opaque one
// would pass every Level 1 row and fail here.
//
// No row sleeps. The one place a retry policy appears (the control at the end)
// installs BaseBackoff 0, so the request counts are a property of the retry
// classification rather than of elapsed time, which is the discipline
// internal/push needed after its reconnect tests made a coverage figure a coin
// flip.

var errMarketExecutorDown = errors.New("market: scripted executor failure")

// marketErrorCase is one row of the table: the op and route the method must
// reach, and the call itself. Each row asserts the zero value beside the error
// inside its own invoke, against the concrete response type, which is the only
// way a generic zero check is meaningful here.
type marketErrorCase struct {
	name   string
	op     string
	route  client.Route
	invoke func(t *testing.T, ctx context.Context, svc *MarketService) error
}

// marketErrorCases is the per-method table, one row per method in market.go.
func marketErrorCases() []marketErrorCase {
	return []marketErrorCase{
		{
			name:  "BasicQot",
			op:    opBasicQot,
			route: client.RouteHqBasicQot,
			invoke: func(t *testing.T, ctx context.Context, svc *MarketService) error {
				resp, err := svc.BasicQot(ctx, BasicQotRequest{Security: []*Security{marketSecurity()}})
				requireZero(t, resp)
				return err
			},
		},
		{
			name:  "OrderBook",
			op:    opOrderBook,
			route: client.RouteHqOrderBook,
			invoke: func(t *testing.T, ctx context.Context, svc *MarketService) error {
				resp, err := svc.OrderBook(ctx, OrderBookRequest{Security: marketSecurity()})
				requireZero(t, resp)
				return err
			},
		},
		{
			name:  "KL",
			op:    opKL,
			route: client.RouteHqKL,
			invoke: func(t *testing.T, ctx context.Context, svc *MarketService) error {
				resp, err := svc.KL(ctx, KLRequest{Security: marketSecurity()})
				requireZero(t, resp)
				return err
			},
		},
		{
			name:  "TimeShare",
			op:    opTimeShare,
			route: client.RouteHqTimeShare,
			invoke: func(t *testing.T, ctx context.Context, svc *MarketService) error {
				resp, err := svc.TimeShare(ctx, TimeShareRequest{Security: marketSecurity()})
				requireZero(t, resp)
				return err
			},
		},
		{
			name:  "Ticker",
			op:    opTicker,
			route: client.RouteHqTicker,
			invoke: func(t *testing.T, ctx context.Context, svc *MarketService) error {
				resp, err := svc.Ticker(ctx, TickerRequest{Security: marketSecurity(), Limit: 10})
				requireZero(t, resp)
				return err
			},
		},
		{
			name:  "Broker",
			op:    opBroker,
			route: client.RouteHqBroker,
			invoke: func(t *testing.T, ctx context.Context, svc *MarketService) error {
				resp, err := svc.Broker(ctx, BrokerRequest{Security: marketSecurity()})
				requireZero(t, resp)
				return err
			},
		},
		{
			name:  "UsOptionChainCode",
			op:    opUsOptionChainCode,
			route: client.RouteHqUsOptionChainCode,
			invoke: func(t *testing.T, ctx context.Context, svc *MarketService) error {
				resp, err := svc.UsOptionChainCode(ctx, UsOptionChainCodeRequest{SecurityCode: "AAPL"})
				requireZero(t, resp)
				return err
			},
		},
		{
			name:  "UsOptionChainExpireDate",
			op:    opUsOptionChainExpireDate,
			route: client.RouteHqUsOptionChainExpireDate,
			invoke: func(t *testing.T, ctx context.Context, svc *MarketService) error {
				resp, err := svc.UsOptionChainExpireDate(ctx, UsOptionChainExpireDateRequest{SecurityCode: "AAPL"})
				requireZero(t, resp)
				return err
			},
		},
		{
			name:  "UsOverNightTradeCodes",
			op:    opUsOverNightTradeCodes,
			route: client.RouteHqUsOverNightTradeCodes,
			invoke: func(t *testing.T, ctx context.Context, svc *MarketService) error {
				resp, err := svc.UsOverNightTradeCodes(ctx, UsOverNightTradeCodesRequest{})
				requireZero(t, resp)
				return err
			},
		},
		{
			// Subscribe and Unsubscribe return s.client.Do(...) directly, so they
			// have no separate error block and these two rows buy no coverage.
			// They stay in the table because the error is their only return
			// value: a row is the only way to tell a failed subscription from a
			// successful one, and dropping them because the number would not move
			// would trade a real assertion for a metric.
			name:  "Subscribe",
			op:    opSubscribe,
			route: client.RouteHqSubscribe,
			invoke: func(_ *testing.T, ctx context.Context, svc *MarketService) error {
				return svc.Subscribe(ctx, types.TopicBasicQot, marketSecurity())
			},
		},
		{
			name:  "Unsubscribe",
			op:    opUnsubscribe,
			route: client.RouteHqUnsubscribe,
			invoke: func(_ *testing.T, ctx context.Context, svc *MarketService) error {
				return svc.Unsubscribe(ctx, types.TopicBasicQot, marketSecurity())
			},
		},
	}
}

// TestMarketExecutorFailurePropagates is Level 1: the executor's error reaches
// the caller unchanged, beside the zero value, after exactly one attempt.
//
// The zero-value assertion lives inside each invoke and is checked against the
// concrete response type. A method that returned a half-decoded payload beside
// its error would let a caller mistake a failure for an empty success, which is
// the failure mode the row exists to prevent.
func TestMarketExecutorFailurePropagates(t *testing.T) {
	for _, tc := range marketErrorCases() {
		t.Run(tc.name, func(t *testing.T) {
			exec := newSequencedExecutor(t, sequencedReply{err: errMarketExecutorDown})
			svc := NewMarketService(exec)

			if err := tc.invoke(t, t.Context(), svc); !errors.Is(err, errMarketExecutorDown) {
				t.Fatalf("%s error = %v, want the executor's own error", tc.name, err)
			}
			requireCalls(t, exec, 1)
		})
	}
}

// TestMarketGatewayRejectionArrivesTyped is Level 2: the same eleven rows driven
// through a real client over a real HTTP server, with the Gateway answering
// "1007 : duplicate submission".
//
// The assertions are the ones a fake cannot make. internal/transport parses the
// documented "<code> : <text>" form back into a *errs.Error carrying the code
// and its category, and the service must hand that value to the caller with its
// own op attached. A wrapper that replaced it with a bare error, or that lost
// the op, would pass Level 1 and fail here.
func TestMarketGatewayRejectionArrivesTyped(t *testing.T) {
	const code = types.StatusDuplicateSubmit
	const text = "duplicate submission"

	for _, tc := range marketErrorCases() {
		t.Run(tc.name, func(t *testing.T) {
			rec := newWireRecorder(map[string]string{
				string(tc.route): gatewayFailure(code, text),
			})
			svc := NewMarketService(newWireExecutor(t, rec))

			err := tc.invoke(t, t.Context(), svc)
			if err == nil {
				t.Fatalf("%s = nil error, want the Gateway rejection %q", tc.name, code)
			}
			errRejects(t, err, code, tc.op)
			if got := errs.CategoryOf(err); got != errs.CategoryTrading {
				t.Errorf("CategoryOf = %q, want %q: a 1007 is a trading-state failure the "+
					"caller must reconcile before resubmitting", got, errs.CategoryTrading)
			}
			if got, ok := errs.CodeOf(err); !ok || got != code {
				t.Errorf("CodeOf = (%q, %v), want (%q, true)", got, ok, code)
			}
			if got := rec.count(string(tc.route)); got != 1 {
				t.Fatalf("requests to %s = %d, want exactly 1", tc.route, got)
			}
			if got := rec.total(); got != 1 {
				t.Fatalf("total requests = %d, want exactly 1", got)
			}
		})
	}
}

// TestMarketReadRetriesUnderARetryPolicy is the control for the "exactly one
// request" assertions above, and without it those assertions would be worth very
// little.
//
// Level 2 asserts a single HTTP request, and a single request is also what a
// client with no retry policy installed produces, so a client that silently
// ignored its policy would pass every row. This test installs MaxAttempts 5 with
// a zero backoff and a retryable rejection ("1011 service busy", which
// errs.Retryable reports as retryable) and shows the HQ read routes taking all
// five. The policy is therefore demonstrably live, and the one request in the
// table above is a decision rather than a default.
//
// ADR 0003 puts the four trade mutations outside this set entirely, and the
// exhaustive classification check lives in client/route_mutation_test.go rather
// than here. This is the behavioural half for the routes this package owns: every
// /hq route is a read, so all of them are legitimately retryable.
func TestMarketReadRetriesUnderARetryPolicy(t *testing.T) {
	const wantAttempts = 5
	busy := gatewayFailure(types.StatusServiceBusy, "service busy, retry later")

	for _, tc := range marketErrorCases() {
		t.Run(tc.name, func(t *testing.T) {
			rec := newWireRecorder(map[string]string{string(tc.route): busy})
			svc := NewMarketService(newWireExecutor(t, rec,
				client.WithRetryPolicy(client.RetryPolicy{MaxAttempts: wantAttempts})))

			err := tc.invoke(t, t.Context(), svc)
			if err == nil {
				t.Fatalf("%s = nil error, want the %q rejection", tc.name, types.StatusServiceBusy)
			}
			errRejects(t, err, types.StatusServiceBusy, tc.op)
			if got := errs.CategoryOf(err); got != errs.CategoryRateLimit {
				t.Errorf("CategoryOf = %q, want %q", got, errs.CategoryRateLimit)
			}
			if got := rec.total(); got != wantAttempts {
				t.Fatalf("total requests = %d, want %d: the control must show the retry policy "+
					"really is applied, or the single-request rows above prove nothing", got, wantAttempts)
			}
		})
	}
}

// TestMarketEmptyReplyIsSuccessNotFailure pins the boundary the error tables sit
// next to.
//
// The transport treats an ok envelope with absent or null data as a successful
// call that decoded nothing, so a method pointed at an endpoint the Gateway
// answered with no data must return an empty result and no error. Getting this
// wrong in either direction is costly: treating it as an error would fail a call
// that succeeded, and the reverse would report an empty book as a real one.
func TestMarketEmptyReplyIsSuccessNotFailure(t *testing.T) {
	// Three calls, three scripted replies: an exhausted fixture calls
	// t.Fatal rather than silently replaying the last one, which is exactly the
	// failure mode a shared fake has to avoid.
	exec := newSequencedExecutor(t,
		sequencedReply{reply: map[string]any{}},
		sequencedReply{reply: map[string]any{}},
		sequencedReply{reply: map[string]any{}},
	)
	svc := NewMarketService(exec)

	book, err := svc.OrderBook(t.Context(), OrderBookRequest{Security: marketSecurity()})
	if err != nil {
		t.Fatalf("OrderBook on an empty reply = %v, want nil", err)
	}
	if len(book.Ask) != 0 || len(book.Bid) != 0 {
		t.Errorf("book = %d asks / %d bids, want empty", len(book.Ask), len(book.Bid))
	}
	// decimalOrZero is the guard that keeps an absent spreadLevel from taking the
	// caller's goroutine down in domain.MustNewPrice.
	if got := book.TickSize.String(); got != "0" {
		t.Errorf("TickSize on an empty reply = %q, want 0", got)
	}
	if book.Security != nil {
		t.Errorf("Security on an empty reply = %+v, want nil", book.Security)
	}

	quotes, err := svc.BasicQot(t.Context(), BasicQotRequest{Security: []*Security{marketSecurity()}})
	if err != nil {
		t.Fatalf("BasicQot on an empty reply = %v, want nil", err)
	}
	if len(quotes.BasicQot) != 0 {
		t.Errorf("BasicQot returned %d quotes, want 0", len(quotes.BasicQot))
	}

	codes, err := svc.UsOverNightTradeCodes(t.Context(), UsOverNightTradeCodesRequest{})
	if err != nil {
		t.Fatalf("UsOverNightTradeCodes on an empty reply = %v, want nil", err)
	}
	if len(codes.SecurityCodes) != 0 {
		t.Errorf("UsOverNightTradeCodes returned %d codes, want 0", len(codes.SecurityCodes))
	}
	requireCalls(t, exec, 3)
}

// TestMarketCallIsCancellable pins that a cancelled context reaches the executor
// untouched and that a service neither swallows the cancellation nor turns it
// into a success. The error is matched through errors.Is on context.Canceled,
// never on a rendered message.
func TestMarketCallIsCancellable(t *testing.T) {
	exec := newSequencedExecutor(t, sequencedReply{err: context.Canceled})
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	resp, err := NewMarketService(exec).Ticker(ctx, TickerRequest{Security: marketSecurity(), Limit: 5})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Ticker on a cancelled context = %v, want context.Canceled", err)
	}
	requireZero(t, resp)
	if got := errs.CategoryOf(err); got != errs.CategoryTimeout {
		t.Errorf("CategoryOf = %q, want %q for a cancelled call", got, errs.CategoryTimeout)
	}
	requireCalls(t, exec, 1)
}
