// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

// This is an external test package on purpose. client imports
// internal/resilience, so an in-package test could not import client to check
// the duplicated route list against it. An external test package may import a
// package that depends on the package under test, which is exactly the join this
// check needs.
//
// Closes security-register item R2.
package resilience_test

import (
	"testing"

	"github.com/shing1211/hstongapi4go/client"
	"github.com/shing1211/hstongapi4go/internal/resilience"
)

// expectedMutations is the closed set of order-mutation endpoints, transcribed
// from the vendor's own endpoint list. It is a hand-maintained duplicate of
// resilience.MutationPaths on purpose: the whole point of this file is to fail
// when the two disagree, and a test that recomputed its expectation from the
// code under test would only prove the code equals itself.
//
// Twelve, across three surfaces:
//   - cash:      entrust, cancel, batch-cancel, change
//   - futures:   entrust, cancel, modify
//   - algo:      add, cancel, cancel-entrust, change, action
var expectedMutations = []string{
	"/trade/AlgoActionOrder",
	"/trade/AlgoAddOrder",
	"/trade/AlgoCancelEntrust",
	"/trade/AlgoCancelOrder",
	"/trade/AlgoChangeOrder",
	"/trade/FuturesCancelEntrust",
	"/trade/FuturesEntrust",
	"/trade/FuturesModifyEntrust",
	"/trade/TradeBatchCancelEntrust",
	"/trade/TradeCancelEntrust",
	"/trade/TradeChangeEntrust",
	"/trade/TradeEntrust",
}

// nonMutations are trade reads that sit next to the mutations in the same
// packages. They are here to catch the opposite defect: a set that is too wide
// silently stops retrying a query, which is a liveness bug that looks like
// "retries aren't happening" rather than like a safety problem.
var nonMutations = []string{
	"/trade/TradeQueryRealEntrustList",
	"/trade/TradeQueryRealDeliverList",
	"/trade/TradeQueryHistoryEntrustList",
	"/trade/TradeQueryMaxAvailableAsset",
	"/trade/FuturesQueryRealEntrustList",
	"/trade/FuturesQueryRealDeliverList",
	"/trade/FuturesQueryHistoryEntrustList",
	"/trade/FuturesQueryFundInfo",
	"/trade/AlgoQueryOrderList",
	"/trade/AlgoQueryEntrustIDList",
}

// TestMutationSetCoversEveryMutationRoute is the R2 guard.
//
// The failure it prevents is a money-moving one. A mutation route missing from
// the set is classified ClassQuery, which makes it retryable, and ADR 0003
// requires that order mutations make exactly one attempt. A missing entry
// therefore does not degrade a read path - it introduces a duplicate order on a
// retryable mutation, and nothing else in the build would notice.
func TestMutationSetCoversEveryMutationRoute(t *testing.T) {
	got := resilience.MutationPaths()

	if len(got) != len(expectedMutations) {
		t.Errorf("mutation set has %d entries, expected %d.\n  got:      %v\n  expected: %v",
			len(got), len(expectedMutations), got, expectedMutations)
	}

	have := make(map[string]bool, len(got))
	for _, p := range got {
		have[p] = true
	}
	for _, want := range expectedMutations {
		if !have[want] {
			t.Errorf("mutation route %s is missing from resilience.MutationPaths(). "+
				"It will be classified as a retryable query, so a mutation on this "+
				"route can be retried - which ADR 0003 forbids", want)
		}
	}
	for _, p := range got {
		found := false
		for _, want := range expectedMutations {
			if p == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("resilience.MutationPaths() contains %s, which is not one of the "+
				"documented mutations. An over-wide set stops a query being retried", p)
		}
	}
}

// TestEveryMutationPathIsACanonicalRoute catches a stale or misspelled entry. A
// path that no route declares can never match a real request, so it is dead
// weight in the set and a sign the list was edited by hand without checking.
func TestEveryMutationPathIsACanonicalRoute(t *testing.T) {
	canonical := make(map[string]bool)
	for _, r := range client.Routes() {
		canonical[r.Path()] = true
	}
	for _, p := range resilience.MutationPaths() {
		if !canonical[p] {
			t.Errorf("mutation path %s is not a canonical client.Route. It can never "+
				"match a request, so it does not classify anything", p)
		}
	}
}

// TestQueryRoutesAreNotMutations is the other direction, and the reason this file
// asserts more than membership.
func TestQueryRoutesAreNotMutations(t *testing.T) {
	for _, p := range nonMutations {
		if resilience.IsMutation(p) {
			t.Errorf("%s is classified as a mutation, so its retries are suppressed. "+
				"That is a liveness bug, not a safety one, and it is easy to cause by "+
				"copying a neighbouring entry", p)
		}
		if got := resilience.ClassForPath(p); got == resilience.ClassMutation {
			t.Errorf("ClassForPath(%s) = ClassMutation, want ClassQuery", p)
		}
	}
}

// TestAliasFormsOfMutationsStayMutations closes the bypass. The Gateway accepts
// two alias forms for every route, and a caller can send either. If
// classification only matched the canonical path, a mutation would become
// retryable simply by changing the suffix - which is not a bug anyone would find
// by reading the retry policy.
//
// The suffixes are taken from client.NormalizePath rather than hardcoded. A
// first draft guessed "MsgType" and "Request" and failed, and the failure was
// informative: the real pair is "RequestMsgType" and "Request". Deriving them
// means a change to client's alias handling breaks this test loudly instead of
// quietly testing a suffix nobody sends.
func TestAliasFormsOfMutationsStayMutations(t *testing.T) {
	// Discover the real alias suffixes: a path that NormalizePath rewrites back
	// to a canonical route is an alias form, whatever it is spelled.
	suffixes := aliasSuffixes(t)

	for _, p := range expectedMutations {
		for _, suffix := range suffixes {
			alias := p + suffix
			if got := client.NormalizePath(alias); got != p {
				t.Errorf("discovered suffix %q but NormalizePath(%s) = %s, want %s; "+
					"the alias forms this test exercises are not the ones the Gateway "+
					"accepts", suffix, alias, got, p)
			}
			if !resilience.IsMutation(alias) {
				t.Errorf("alias form %s is not classified as a mutation, so it would be "+
					"retried. A caller can bypass the no-auto-retry guarantee just by "+
					"sending the alias", alias)
			}
			if got := resilience.ClassForPath(alias); got != resilience.ClassMutation {
				t.Errorf("ClassForPath(%s) = %v, want ClassMutation", alias, got)
			}
		}
	}
}

// aliasSuffixes returns every suffix S such that canonical+S normalises back to
// canonical, for a known mutation route.
func aliasSuffixes(t *testing.T) []string {
	t.Helper()
	const probe = "/trade/TradeEntrust"
	var out []string
	for _, candidate := range []string{"Request", "RequestMsgType", "MsgType", "Req", "RequestMessage"} {
		if client.NormalizePath(probe+candidate) == probe {
			out = append(out, candidate)
		}
	}
	// Exactly two are documented, and a third appearing later would mean a new
	// bypass this test is not covering.
	if len(out) < 2 {
		t.Fatalf("discovered only %d alias suffix(es) %v for %s; the Gateway is "+
			"documented to accept two, so either the alias handling changed or this "+
			"probe is stale", len(out), out, probe)
	}
	return out
}
