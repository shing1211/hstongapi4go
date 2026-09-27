// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package services

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"testing"

	"github.com/shing1211/hstongapi4go/client"
	"github.com/shing1211/hstongapi4go/internal/errs"
	"github.com/shing1211/hstongapi4go/pkg/domain"
	"github.com/shing1211/hstongapi4go/pkg/types"
)

// This file is the ADR 0003 proof for pkg/services: the four order mutations must
// issue exactly one attempt, ever, at every retry-policy configuration.
//
// Three things make this non-obvious at this layer, and each of them is a way the
// assertion below could pass for the wrong reason.
//
//  1. The guarantee lives in client, not here. client.Client.execute derives the
//     retry class from the route path and consults resilience.IsMutation, which
//     issues one attempt for that class regardless of the policy. A fake executor
//     has no retry logic at all, so "the fake recorded one call" is trivially
//     true and says nothing about retries. That check is kept — as a guard against
//     a service growing a loop of its own — but it is not the proof.
//
//  2. The proof therefore needs a real *client.Client over an httptest server, and
//     a retryable rejection so the policy has every reason to fire. "1011 service
//     busy" is the code errs.Retryable reports as retryable. BaseBackoff is 0, so
//     the request count is a property of the classification and not of elapsed
//     time — the discipline internal/push needed after its reconnect tests made a
//     coverage figure a coin flip. Nothing here points at 127.0.0.1:11111.
//
//  3. A single request is also what a client with no retry policy installed
//     produces, and it is also what a policy that was never applied produces. So
//     the control at the end drives a read-only route under the identical client
//     options and the identical rejection, and shows it taking all five attempts.
//     Without that, D2 would pass against a policy that did nothing, and a test
//     that cannot fail is worse than no test because it reads as coverage.
//
// The route list is not duplicated here. client/route_mutation_test.go holds the
// exhaustive classification check over all twelve mutation routes, and
// internal/resilience.mutationPaths is the closed set it verifies. This file
// asserts the behaviour for the four routes ADR 0003 names for Trade; the other
// belongs to the other test.

// tradingMutationAttempt drives one mutation with the given arguments, for the
// three layers below.
type tradingMutationAttempt struct {
	name   string
	op     string
	route  client.Route
	invoke func(ctx context.Context, svc *TradingService) error
}

// tradingMutationAttempts is the four routes ADR 0003 §Decision "Trade" names.
//
// Every invocation uses valid arguments. That is load-bearing for D2: Entrust
// validates before it calls the executor, so an invalid fixture would record zero
// requests — which would look identical to "exactly one attempt" if the assertion
// were rec.total() == 1 rather than rec.count(route) == 1 paired with an explicit
// check that a call happened at all. The assertions below do both.
func tradingMutationAttempts() []tradingMutationAttempt {
	account := tradingAccountID()
	entrustID := tradingEntrustID()

	return []tradingMutationAttempt{
		{
			name:  "Entrust",
			op:    opEntrust,
			route: client.RouteTradeEntrust,
			invoke: func(ctx context.Context, svc *TradingService) error {
				_, err := svc.Entrust(ctx, account, tradingHKOrder())
				return err
			},
		},
		{
			name:  "CancelEntrust",
			op:    opCancelEntrust,
			route: client.RouteTradeCancelEntrust,
			invoke: func(ctx context.Context, svc *TradingService) error {
				return svc.CancelEntrust(ctx, account, entrustID)
			},
		},
		{
			name:  "BatchCancelEntrust",
			op:    opBatchCancelEntrust,
			route: client.RouteTradeBatchCancelEntrust,
			invoke: func(ctx context.Context, svc *TradingService) error {
				return svc.BatchCancelEntrust(ctx, account, []domain.EntrustID{entrustID, "E-1002"})
			},
		},
		{
			name:  "ChangeEntrust",
			op:    opChangeEntrust,
			route: client.RouteTradeChangeEntrust,
			invoke: func(ctx context.Context, svc *TradingService) error {
				return svc.ChangeEntrust(ctx, account, entrustID,
					domain.MustNewPrice("388.05", "0.001"), domain.MustNewQuantity("200"))
			},
		},
	}
}

// TestD1TheServiceIssuesOneDo is layer one: the service itself asks the executor
// once, and hands a retryable error straight back.
//
// The scripted error is a genuine *errs.Error with code 1011, and the test first
// checks that errs.Retryable agrees it is retryable — otherwise "one attempt"
// would be true for the boring reason that nothing wanted a second one. This
// layer cannot prove the client's behaviour; it proves the service adds no retry
// of its own on top of it.
func TestD1TheServiceIssuesOneDo(t *testing.T) {
	for _, tc := range tradingMutationAttempts() {
		t.Run(tc.name, func(t *testing.T) {
			retryable := errs.New(types.StatusServiceBusy, tc.op, "service busy, retry later")
			if !errs.Retryable(retryable) {
				t.Fatalf("test bug: %q is not retryable, so a single call would be expected "+
					"even without ADR 0003", types.StatusServiceBusy)
			}

			exec := newSequencedExecutor(t,
				sequencedReply{err: retryable},
				sequencedReply{err: retryable},
			)
			svc := NewTradingService(exec)

			err := tc.invoke(t.Context(), svc)
			if !errors.Is(err, retryable) {
				t.Fatalf("%s error = %v, want the scripted retryable error", tc.name, err)
			}
			if got := errs.CategoryOf(err); got != errs.CategoryRateLimit {
				t.Errorf("CategoryOf = %q, want %q", got, errs.CategoryRateLimit)
			}
			requireCalls(t, exec, 1)
		})
	}
}

// TestD2AMutationIsSentExactlyOnceUnderARetryPolicy is the ADR 0003 proof.
//
// A real client, five attempts allowed, zero backoff, and a retryable rejection
// on every path. One attempt is the only outcome consistent with the guarantee,
// and it is the outcome a regression would change: misclassify the route as a
// query, or drop the mutation check, and all five go out.
func TestD2AMutationIsSentExactlyOnceUnderARetryPolicy(t *testing.T) {
	const maxAttempts = 5
	busy := gatewayFailure(types.StatusServiceBusy, "service busy, retry later")

	// Every path answers busy, not just the one under test, so a service that
	// quietly issued a second request somewhere else would show up in total().
	paths := make(map[string]string, len(tradingMutationAttempts()))
	for _, tc := range tradingMutationAttempts() {
		paths[string(tc.route)] = busy
	}

	for _, tc := range tradingMutationAttempts() {
		t.Run(tc.name, func(t *testing.T) {
			rec := newWireRecorder(paths)
			svc := NewTradingService(newWireExecutor(t, rec,
				client.WithRetryPolicy(client.RetryPolicy{MaxAttempts: maxAttempts, BaseBackoff: 0})))

			err := tc.invoke(t.Context(), svc)
			if err == nil {
				t.Fatalf("%s = nil error, want the %q rejection", tc.name, types.StatusServiceBusy)
			}
			// The typed error is asserted, not just the count: a caller reconciling
			// an ambiguous order needs to know the exchange was busy, not that the
			// SDK gave up.
			errRejects(t, err, types.StatusServiceBusy, tc.op)
			if got := errs.CategoryOf(err); got != errs.CategoryRateLimit {
				t.Errorf("CategoryOf = %q, want %q", got, errs.CategoryRateLimit)
			}

			if got := rec.count(string(tc.route)); got != 1 {
				t.Errorf("requests to %s = %d, want exactly 1: ADR 0003 permits one attempt for "+
					"an order mutation at any policy configuration, and this one allowed %d",
					tc.route, got, maxAttempts)
			}
			if got := rec.total(); got != 1 {
				t.Errorf("total requests = %d, want 1: the mutation must not have issued a "+
					"second request on any other path", got)
			}
		})
	}
}

// TestD3AReadOnlyQueryDoesRetry is the control, and it is not optional.
//
// Same client options, same retryable rejection, same package, one call: the
// read-only entrust-list query. If the read route takes all five attempts, then
// the policy really is installed and really does fire on a retryable rejection,
// and the single request D2 observed for each mutation is a decision rather than
// a default.
//
// It also pins the error's typed shape on the way out, so the control is not
// merely a request counter: the caller must be told the exchange was busy and
// which endpoint was busy.
func TestD3AReadOnlyQueryDoesRetry(t *testing.T) {
	const maxAttempts = 5

	rec := newWireRecorder(map[string]string{
		string(client.RouteTradeQueryRealEntrustList): gatewayFailure(
			types.StatusServiceBusy, "service busy, retry later"),
	})
	svc := NewTradingService(newWireExecutor(t, rec,
		client.WithRetryPolicy(client.RetryPolicy{MaxAttempts: maxAttempts, BaseBackoff: 0})))

	entrusts, err := svc.RealEntrustList(t.Context(), tradingAccountID(),
		EntrustFilter{ExchangeType: types.ExchangeHK})
	if err == nil {
		t.Fatalf("RealEntrustList = nil error and %d rows, want the %q rejection",
			len(entrusts), types.StatusServiceBusy)
	}
	requireZero(t, entrusts)
	errRejects(t, err, types.StatusServiceBusy, opRealEntrustList)
	if got := errs.CategoryOf(err); got != errs.CategoryRateLimit {
		t.Errorf("CategoryOf = %q, want %q", got, errs.CategoryRateLimit)
	}
	if got, ok := errs.CodeOf(err); !ok || got != types.StatusServiceBusy {
		t.Errorf("CodeOf = (%q, %v), want (%q, true)", got, ok, types.StatusServiceBusy)
	}

	if got := rec.count(string(client.RouteTradeQueryRealEntrustList)); got != maxAttempts {
		t.Fatalf("requests to %s = %d, want %d. The control must show the retry policy really "+
			"is applied, or the single-request rows in TestD2AMutationIsSentExactlyOnceUnder"+
			"ARetryPolicy prove nothing.", client.RouteTradeQueryRealEntrustList, got, maxAttempts)
	}
	if got := rec.total(); got != maxAttempts {
		t.Errorf("total requests = %d, want %d", got, maxAttempts)
	}
}

// TestD3CoversEveryReadOnlyTradingRoute widens the control to every read-only
// route this package owns, so the guarantee is not resting on one endpoint.
//
// The four mutations are the interesting rows above, but a service that grew a
// sixth method on a mutation route would want the same treatment, and a read route
// that had been misfiled into the mutation set would silently lose its retries —
// the failure client/route_mutation_test.go guards by exhaustiveness. This table
// states the same split behaviourally: nine read routes retry, four mutations do
// not.
func TestD3CoversEveryReadOnlyTradingRoute(t *testing.T) {
	const maxAttempts = 5
	busy := gatewayFailure(types.StatusServiceBusy, "service busy, retry later")

	reads := []struct {
		name   string
		op     string
		route  client.Route
		invoke func(ctx context.Context, svc *TradingService) error
	}{
		{"MaxAvailableAsset", opMaxAvailableAsset, client.RouteTradeQueryMaxAvailableAsset,
			func(ctx context.Context, s *TradingService) error {
				_, err := s.MaxAvailableAsset(ctx, tradingAccountID(), tradingHKSymbol(),
					domain.MustNewPrice("387.05", "0.001"), types.EntrustTypeLimit)
				return err
			}},
		{"RealEntrustList", opRealEntrustList, client.RouteTradeQueryRealEntrustList,
			func(ctx context.Context, s *TradingService) error {
				_, err := s.RealEntrustList(ctx, tradingAccountID(), EntrustFilter{ExchangeType: types.ExchangeHK})
				return err
			}},
		{"RealDeliverList", opRealDeliverList, client.RouteTradeQueryRealDeliverList,
			func(ctx context.Context, s *TradingService) error {
				_, err := s.RealDeliverList(ctx, tradingAccountID(), DeliverFilter{ExchangeType: types.ExchangeHK})
				return err
			}},
		{"RealCondOrderList", opRealCondOrderList, client.RouteTradeQueryRealCondOrderList,
			func(ctx context.Context, s *TradingService) error {
				_, err := s.RealCondOrderList(ctx, tradingAccountID(), CondOrderFilter{ExchangeType: types.ExchangeHK})
				return err
			}},
		{"HistoryEntrustList", opHistoryEntrustList, client.RouteTradeQueryHistoryEntrustList,
			func(ctx context.Context, s *TradingService) error {
				_, err := s.HistoryEntrustList(ctx, tradingAccountID(), HistoryFilter{ExchangeType: types.ExchangeHK})
				return err
			}},
		{"HistoryDeliverList", opHistoryDeliverList, client.RouteTradeQueryHistoryDeliverList,
			func(ctx context.Context, s *TradingService) error {
				_, err := s.HistoryDeliverList(ctx, tradingAccountID(), HistoryFilter{ExchangeType: types.ExchangeHK})
				return err
			}},
		{"HistoryCondOrderList", opHistoryCondOrderList, client.RouteTradeQueryHistoryCondOrderList,
			func(ctx context.Context, s *TradingService) error {
				_, err := s.HistoryCondOrderList(ctx, tradingAccountID(), HistoryFilter{ExchangeType: types.ExchangeHK})
				return err
			}},
		{"MarginFullInfo", opMarginFullInfo, client.RouteTradeQueryMarginFullInfo,
			func(ctx context.Context, s *TradingService) error {
				_, err := s.MarginFullInfo(ctx, tradingAccountID())
				return err
			}},
		{"BeforeAndAfterSupport", opBeforeAndAfterSupport, client.RouteTradeQueryBeforeAndAfterSupport,
			func(ctx context.Context, s *TradingService) error {
				_, err := s.BeforeAndAfterSupport(ctx, tradingHKSymbol())
				return err
			}},
	}

	mutations := tradingMutationAttempts()

	// All thirteen routes answer busy, so a stray request to a neighbouring path
	// would be counted rather than silently absorbed by a default reply.
	paths := make(map[string]string, len(reads)+len(mutations))
	for _, tc := range reads {
		paths[string(tc.route)] = busy
	}
	for _, tc := range mutations {
		paths[string(tc.route)] = busy
	}

	for _, tc := range reads {
		t.Run(tc.name+" retries", func(t *testing.T) {
			rec := newWireRecorder(paths)
			svc := NewTradingService(newWireExecutor(t, rec,
				client.WithRetryPolicy(client.RetryPolicy{MaxAttempts: maxAttempts, BaseBackoff: 0})))

			if err := tc.invoke(t.Context(), svc); err == nil {
				t.Fatalf("%s = nil error, want the %q rejection", tc.name, types.StatusServiceBusy)
			} else {
				errRejects(t, err, types.StatusServiceBusy, tc.op)
			}
			if got := rec.total(); got != maxAttempts {
				t.Errorf("total requests = %d, want %d: a read-only route must be retryable, "+
					"or the single-request rows for the mutations prove nothing", got, maxAttempts)
			}
		})
	}

	for _, tc := range mutations {
		t.Run(tc.name+" does not retry", func(t *testing.T) {
			rec := newWireRecorder(paths)
			svc := NewTradingService(newWireExecutor(t, rec,
				client.WithRetryPolicy(client.RetryPolicy{MaxAttempts: maxAttempts, BaseBackoff: 0})))

			if err := tc.invoke(t.Context(), svc); err == nil {
				t.Fatalf("%s = nil error, want the %q rejection", tc.name, types.StatusServiceBusy)
			} else {
				errRejects(t, err, types.StatusServiceBusy, tc.op)
			}
			if got := rec.total(); got != 1 {
				t.Errorf("total requests = %d, want 1: an order mutation is sent once and the "+
					"caller reconciles", got)
			}
		})
	}
}

// TestNoRetryPolicyMeansOneRequestForEveryMethod is the default-configuration
// half. With no policy installed the client issues one attempt for everything, so
// the thirteen methods must all produce exactly one HTTP request — which is the
// baseline the mutation rows above are measured against.
func TestNoRetryPolicyMeansOneRequestForEveryMethod(t *testing.T) {
	rec := newWireRecorder(map[string]string{})
	for _, tc := range tradingErrorCases() {
		rec = newWireRecorder(map[string]string{
			string(tc.route): gatewayFailure(types.StatusServiceBusy, "service busy, retry later"),
		})
		svc := NewTradingService(newWireExecutor(t, rec))
		if err := tc.invoke(t, t.Context(), svc); err == nil {
			t.Fatalf("%s = nil error, want the rejection", tc.name)
		}
		if got := rec.total(); got != 1 {
			t.Errorf("%s produced %d requests with no retry policy, want 1", tc.name, got)
		}
	}
}

// TestTheAttemptCountIsIndependentOfThePolicySize widens the guarantee from "one
// policy" to "every policy": a mutation issues one attempt whatever the caller
// configures.
//
// One row per attempt budget, including a budget below the mutation's single
// attempt. resilience.Policy treats a MaxAttempts below one as one, so the
// smallest row is a floor rather than a distinct case, and it is here to make that
// floor explicit.
func TestTheAttemptCountIsIndependentOfThePolicySize(t *testing.T) {
	for _, maxAttempts := range []int{0, 1, 2, 5, 20} {
		t.Run(tradingAttemptLabel(maxAttempts), func(t *testing.T) {
			paths := make(map[string]string)
			for _, tc := range tradingMutationAttempts() {
				paths[string(tc.route)] = gatewayFailure(types.StatusServiceBusy, "service busy, retry later")
			}

			for _, tc := range tradingMutationAttempts() {
				rec := newWireRecorder(paths)
				svc := NewTradingService(newWireExecutor(t, rec,
					client.WithRetryPolicy(client.RetryPolicy{MaxAttempts: maxAttempts, BaseBackoff: 0})))

				if err := tc.invoke(t.Context(), svc); err == nil {
					t.Fatalf("%s = nil error, want the rejection", tc.name)
				}
				if got := rec.total(); got != 1 {
					t.Errorf("%s with MaxAttempts %d produced %d requests, want 1",
						tc.name, maxAttempts, got)
				}
			}
		})
	}
}

// TestACancelledMutationStillCountsAsOneAttempt closes the last gap: a caller who
// gives up on the context must not be able to turn a single attempt into several,
// and the error it gets back must still be the cancellation rather than a Gateway
// status the SDK made up on the way out.
func TestACancelledMutationStillCountsAsOneAttempt(t *testing.T) {
	for _, tc := range tradingMutationAttempts() {
		t.Run(tc.name, func(t *testing.T) {
			rec := newWireRecorder(map[string]string{
				string(tc.route): gatewayFailure(types.StatusServiceBusy, "service busy, retry later"),
			})
			svc := NewTradingService(newWireExecutor(t, rec,
				client.WithRetryPolicy(client.RetryPolicy{MaxAttempts: 5, BaseBackoff: 0})))

			ctx, cancel := context.WithCancel(t.Context())
			cancel()

			err := tc.invoke(ctx, svc)
			if !errors.Is(err, context.Canceled) {
				t.Errorf("%s on a cancelled context = %v, want context.Canceled", tc.name, err)
			}
			if got := rec.total(); got > 1 {
				t.Errorf("%s issued %d requests before the cancellation took effect, want at most 1",
					tc.name, got)
			}
		})
	}
}

// TestBatchCancelIsOneRequestForManyIDs is the shape the guarantee has to cover
// most often: one HTTP request carrying a batch is still one attempt, however many
// entrusts it cancels.
func TestBatchCancelIsOneRequestForManyIDs(t *testing.T) {
	ids := make([]domain.EntrustID, 100)
	for i := range ids {
		ids[i] = domain.EntrustID(tradingEntrustID() + domain.EntrustID(tradingAttemptLabel(i)))
	}

	rec := newWireRecorder(map[string]string{
		string(client.RouteTradeBatchCancelEntrust): gatewayFailure(types.StatusServiceBusy, "busy"),
	})
	svc := NewTradingService(newWireExecutor(t, rec,
		client.WithRetryPolicy(client.RetryPolicy{MaxAttempts: 5, BaseBackoff: 0})))

	if err := svc.BatchCancelEntrust(t.Context(), tradingAccountID(), ids); err == nil {
		t.Fatal("BatchCancelEntrust of 100 ids = nil error, want the rejection")
	}
	if got := rec.total(); got != 1 {
		t.Errorf("requests = %d, want 1: a batch of 100 is still one attempt", got)
	}
	body := rec.lastBody(t, string(client.RouteTradeBatchCancelEntrust))
	if !tradingBodyHasIDCount(body, len(ids)) {
		t.Errorf("body does not carry %d ids:\n%s", len(ids), body)
	}
}

// tradingBodyHasIDCount counts the entrustId entries in a recorded body. The batch
// endpoint serialises them as a JSON array under "entrustId", so a count is enough
// and avoids matching on the whole document.
func tradingBodyHasIDCount(body string, want int) bool {
	return strings.Count(body, `"E-1001`) == want
}

// tradingAttemptLabel names a budget for a subtest.
func tradingAttemptLabel(n int) string {
	if n < 0 {
		return "negative"
	}
	return "MaxAttempts " + strconv.Itoa(n)
}
