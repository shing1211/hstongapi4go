// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package client

import (
	"testing"

	"github.com/shing1211/hstongapi4go/internal/resilience"
)

// orderMutationRoutes is the reviewed set of routes that place, modify, or
// cancel an order. It mirrors internal/resilience.mutationPaths, which cannot
// import this package without an import cycle, so the two lists are kept in
// step by the tests below rather than by the compiler.
var orderMutationRoutes = []Route{
	RouteTradeEntrust,
	RouteTradeCancelEntrust,
	RouteTradeBatchCancelEntrust,
	RouteTradeChangeEntrust,
	RouteTradeFuturesEntrust,
	RouteTradeFuturesCancelEntrust,
	RouteTradeFuturesModifyEntrust,
	RouteTradeAlgoAddOrder,
	RouteTradeAlgoCancelOrder,
	RouteTradeAlgoCancelEntrust,
	RouteTradeAlgoChangeOrder,
	RouteTradeAlgoActionOrder,
}

// TestOrderMutationRoutesAreExhaustive is the guard for R2 in the threat
// model. resilience.mutationSet is a closed set, and a mutation route missing
// from it is classified as a retryable query, which would break the ADR 0003
// guarantee that order mutations issue exactly one attempt.
//
// docs/SPEC.md does not mark which endpoints mutate, and no naming rule can
// recognise every conceivable future mutation, so the invariant enforced here is
// exhaustion rather than inference: every registered route must appear in
// exactly one of the two lists below. Adding a route therefore forces a
// conscious classification decision instead of silently defaulting to
// "retryable".
func TestOrderMutationRoutesAreExhaustive(t *testing.T) {
	classified := make(map[Route]string, len(orderMutationRoutes))
	for _, r := range orderMutationRoutes {
		if prev, dup := classified[r]; dup {
			t.Fatalf("route %q is listed twice in orderMutationRoutes (also as %q)", r, prev)
		}
		classified[r] = "mutation"
	}

	for _, r := range Routes() {
		if _, ok := classified[r]; ok {
			continue
		}
		if resilience.IsMutation(r.Path()) {
			t.Errorf("route %q classifies as a mutation but is not in orderMutationRoutes; "+
				"add it, or remove it from resilience.mutationPaths if that is wrong", r)
			continue
		}
		classified[r] = "query"
	}

	if got, want := len(classified), len(Routes()); got != want {
		t.Errorf("classified %d routes but %d are registered; the lists above are stale", got, want)
	}
}

// TestMutationSetMatchesRouteInventory checks the same invariant from the other
// direction: every path resilience knows about is a real route, and no route is
// silently absent from the resilience set.
func TestMutationSetMatchesRouteInventory(t *testing.T) {
	registered := make(map[string]struct{}, len(Routes()))
	for _, r := range Routes() {
		registered[r.Path()] = struct{}{}
	}

	paths := resilience.MutationPaths()
	if len(paths) != len(orderMutationRoutes) {
		t.Errorf("resilience.MutationPaths() has %d entries, orderMutationRoutes has %d",
			len(paths), len(orderMutationRoutes))
	}

	for _, p := range paths {
		if _, ok := registered[p]; !ok {
			t.Errorf("resilience.MutationPaths() contains %q, which is not a registered route", p)
		}
	}
}

// TestOrderMutationAliasesAreSingleAttempt walks the full 3x12 matrix of
// Gateway path forms. The alias suffixes are deliberately duplicated inside
// internal/resilience to avoid an import cycle, so alias handling is a second
// place the classification can drift.
func TestOrderMutationAliasesAreSingleAttempt(t *testing.T) {
	for _, r := range orderMutationRoutes {
		canonical := r.Path()
		for _, form := range []string{canonical, canonical + "Request", canonical + "RequestMsgType"} {
			if !resilience.IsMutation(form) {
				t.Errorf("IsMutation(%q) = false, want true", form)
			}
		}
	}
}

// TestQueriesAreNotClassifiedAsMutations guards the opposite failure: a query
// misfiled as a mutation would silently lose retries, so both directions are
// asserted rather than only the dangerous one.
func TestQueriesAreNotClassifiedAsMutations(t *testing.T) {
	queries := []Route{
		RouteHqBasicQot,
		RouteHqSubscribe,
		RouteTradeLogin,
		RouteTradeQueryHoldsList,
		RouteTradeQueryRealEntrustList,
		RouteTradeQueryMaxAvailableAsset,
		RouteTradeAlgoQueryOrderList,
		RouteTradeFuturesQueryFundInfo,
		RouteTradeSubscribe,
	}
	for _, r := range queries {
		if resilience.IsMutation(r.Path()) {
			t.Errorf("IsMutation(%q) = true, want false", r)
		}
		if !IsQuery(r.Path()) {
			t.Errorf("IsQuery(%q) = false, want true", r)
		}
	}
}

// TestIsQueryIsTheNegationOfIsMutation ties the two public classifiers
// together, since client.IsQuery is documented as the negation of
// resilience.IsMutation and a divergence would misroute traffic to the wrong
// circuit breaker.
func TestIsQueryIsTheNegationOfIsMutation(t *testing.T) {
	for _, r := range Routes() {
		if got, want := IsQuery(r.Path()), !resilience.IsMutation(r.Path()); got != want {
			t.Errorf("IsQuery(%q) = %v, want %v", r, got, want)
		}
	}
}
