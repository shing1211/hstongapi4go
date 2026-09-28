// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package services

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/shing1211/hstongapi4go/client"
	"github.com/shing1211/hstongapi4go/internal/errs"
	"github.com/shing1211/hstongapi4go/pkg/domain"
	"github.com/shing1211/hstongapi4go/pkg/types"
)

// This file is the suite for the two trade-push subscription endpoints:
// SubscribeOrders and UnsubscribeOrders on /trade/TradeSubscribe and
// /trade/TradeUnsubscribe, SPEC 2.6 rows 32 and 33.
//
// It is its own file, and not a section of the ADR 0003 file, because these two
// endpoints are the first in this package whose interesting property is a
// classification that is *not* the one their own GoDoc claims. The released
// pkg/hstong/trade/push.go says of both that they "issue exactly one HTTP
// request and are never retried". They do not: neither path is in
// internal/resilience's closed mutation set, so client.IsQuery classifies both as
// query class and a retry policy re-sends them. That is a real and measurable
// property, it is the correct classification (ADR 0003 is about order mutations,
// and a subscription is not one), and it is the kind of thing a future reader
// "fixes" by adding two paths to a set this task was told not to touch. So the
// behaviour is pinned by a measurement rather than left to a comment, and the
// disagreement between the doc and the code is recorded in trading.go's own
// GoDoc and in the run tracker.
//
// Four levels are used, each because nothing cheaper can see the thing it checks:
//
//   - the fake sequencedExecutor says what the service did with an outcome, and
//     what it put in params;
//   - the real-HTTP wireRecorder says what internal/transport built out of that
//     outcome, and - the only level that can - what the request body actually was,
//     since a struct assertion cannot see a key that is absent;
//   - the released pkg/hstong/trade/push.go is read as text, so the two op
//     constants cannot drift from the layer this one takes parity against without
//     a test failing;
//   - and a retry policy with BaseBackoff 0 makes the request counts a property of
//     the retry classification rather than of elapsed time. Nothing here sleeps,
//     and nothing here points at 127.0.0.1:11111.
//
// testsupport_test.go is used unchanged: newSequencedExecutor, sequencedReply,
// requireCalls and tradingExpectCall for the fake level; wireRecorder,
// newWireExecutor, gatewaySuccess and gatewayFailure for the real one; and
// errRejects / assertInvalidParam for the typed-error shape. Every double
// declared here is tradePush-prefixed, because a duplicate declaration in a
// package whose tests are all in-package is a compile error that takes every other
// run in pkg/services down with it.

// ---------------------------------------------------------------------------
// Fixtures and doubles
// ---------------------------------------------------------------------------

// tradePushReleasedPath is the released layer's file, relative to this package's
// directory. It is read as text rather than imported, because pkg/hstong is the
// deprecated v0.x surface and internal/layering forbids pkg/services importing
// it. The claim being made is between two *spellings*, and a text read is the
// honest way to make it without creating the very dependency the parity rules
// are about.
const tradePushReleasedPath = "../../pkg/hstong/trade/push.go"

// tradePushOpRE matches an op constant declaration in the released push.go as
// text: a `name = "value"` line inside a const block. It is deliberately a
// regexp over lines rather than a go/ast parse, so it is an independent reader of
// the same file rather than a second copy of the same technique.
var tradePushOpRE = regexp.MustCompile(`^\s*(opTrade\w+)\s*=\s*"([^"]+)"`)

// tradePushCall pairs one push method with the op label, route and caller the
// tests drive, so a table row cannot name one and invoke the other. The pairing
// is the point: a "swap the two routes" defect is invisible to a suite that checks
// the route and the invocation in separate places, and this is the shape that
// catches it.
type tradePushCall struct {
	name      string
	constName string
	op        string
	route     client.Route
	invoke    func(ctx context.Context, svc *TradingService, accountID domain.AccountID) error
}

// tradePushCalls is the two methods, in the order the released file declares them.
func tradePushCalls() []tradePushCall {
	return []tradePushCall{
		{
			name:      "SubscribeOrders",
			constName: "opTradeSubscribe",
			op:        opTradeSubscribe,
			route:     client.RouteTradeSubscribe,
			invoke: func(ctx context.Context, s *TradingService, accountID domain.AccountID) error {
				return s.SubscribeOrders(ctx, accountID)
			},
		},
		{
			name:      "UnsubscribeOrders",
			constName: "opTradeUnsubscribe",
			op:        opTradeUnsubscribe,
			route:     client.RouteTradeUnsubscribe,
			invoke: func(ctx context.Context, s *TradingService, accountID domain.AccountID) error {
				return s.UnsubscribeOrders(ctx, accountID)
			},
		},
	}
}

// tradePushReadReleasedOps reads the op constants out of the released
// pkg/hstong/trade/push.go, keyed by constant name.
func tradePushReadReleasedOps(t *testing.T) map[string]string {
	t.Helper()
	raw, err := os.ReadFile(tradePushReleasedPath)
	if err != nil {
		t.Fatalf("reading the released %s: %v", tradePushReleasedPath, err)
	}
	ops := map[string]string{}
	for _, line := range strings.Split(string(raw), "\n") {
		if m := tradePushOpRE.FindStringSubmatch(strings.TrimSuffix(line, "\r")); m != nil {
			ops[m[1]] = m[2]
		}
	}
	return ops
}

// tradePushAck is the reply the Gateway sends for an acknowledged subscription.
// The released layer reads nothing out of it, so only the ok flag is set, and the
// two endpoints' replies are not distinguished here because neither is decoded.
func tradePushAck() sequencedReply {
	return sequencedReply{reply: json.RawMessage(`{"success":true}`)}
}

// ---------------------------------------------------------------------------
// Parity with the released layer
// ---------------------------------------------------------------------------

// TestTradePushOpConstantsMatchTheReleasedLayer is the parity anchor, stated as a
// read of the released file rather than as a restatement of this package's own
// constants. A literal compared with a literal proves nothing; a constant read out
// of pkg/hstong/trade/push.go and compared with the one the service uses does.
//
// It also checks the collision the two const blocks could have had and did not:
// pkg/services declares opEntrust and opCancelEntrust for the cash order
// endpoints and opTradeLogin and opTradeLogout for the session, and a shared label
// would make two different endpoints' errors indistinguishable.
func TestTradePushOpConstantsMatchTheReleasedLayer(t *testing.T) {
	released := tradePushReadReleasedOps(t)
	if len(released) == 0 {
		t.Fatalf("no op constants were read out of the released %s; the pattern no "+
			"longer matches that file and this test would pass vacuously", tradePushReleasedPath)
	}

	for _, tc := range tradePushCalls() {
		want, ok := released[tc.constName]
		if !ok {
			t.Errorf("%s is not declared in the released %s; the two layers would report "+
				"different labels for the same endpoint", tc.constName, tradePushReleasedPath)
			continue
		}
		if tc.op != want {
			t.Errorf("%s = %q, want the released layer's %q", tc.constName, tc.op, want)
		}
		// The label is the Gateway path without its leading slash - the convention
		// every label in this package follows - so it cannot disagree with the route
		// the method sends to. That is what makes the label usable in a log line.
		if got := tc.route.Path(); "/"+tc.op != got {
			t.Errorf("%s = %q, but the method sends to %q; the label must be the route so "+
				"an error points at the failing endpoint", tc.constName, tc.op, got)
		}
	}
}

// TestTradePushOpLabelsAreDistinct is the collision check, stated once for every
// label this package declares. The neighbours are named explicitly so the failure
// message says which pair collided, because "duplicate map key" would not.
//
// A test in the same file as the const block is the only place a future
// contributor adding a seventeenth label will be caught: nothing in Go stops two
// constants from carrying the same string, and the consequence - one endpoint's
// error rendered as another's - is invisible until someone reads a log.
func TestTradePushOpLabelsAreDistinct(t *testing.T) {
	labels := map[string]string{
		"opTradeSubscribe":     opTradeSubscribe,
		"opTradeUnsubscribe":   opTradeUnsubscribe,
		"opEntrust":            opEntrust,
		"opCancelEntrust":      opCancelEntrust,
		"opBatchCancelEntrust": opBatchCancelEntrust,
		"opChangeEntrust":      opChangeEntrust,
		"opTradeLogin":         opTradeLogin,
		"opTradeLogout":        opTradeLogout,
	}
	seen := map[string]string{}
	for name, value := range labels {
		if other, dup := seen[value]; dup {
			t.Errorf("the op labels %s and %s are both %q; an error from one endpoint "+
				"would be indistinguishable from the other's", other, name, value)
			continue
		}
		seen[value] = name
	}
	if len(seen) != len(labels) {
		t.Errorf("%d distinct labels out of %d declared; the map above has a duplicate key",
			len(seen), len(labels))
	}
}

// ---------------------------------------------------------------------------
// Routing and the two request bodies
// ---------------------------------------------------------------------------

// TestTradePushRoutesAndBodies is the positive path for both endpoints at the
// real-HTTP level: one request per call, to the route the method names.
func TestTradePushRoutesAndBodies(t *testing.T) {
	rec := newWireRecorder(map[string]string{
		string(client.RouteTradeSubscribe):   gatewaySuccess(`{"success":true}`),
		string(client.RouteTradeUnsubscribe): gatewaySuccess(`{"success":true}`),
	})
	svc := NewTradingService(newWireExecutor(t, rec))
	account := tradingAccountID()

	for _, tc := range tradePushCalls() {
		if err := tc.invoke(t.Context(), svc, account); err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
	}
	if got := rec.total(); got != 2 {
		t.Errorf("requests = %d, want 2: one per endpoint", got)
	}
	for _, tc := range tradePushCalls() {
		if got := rec.count(string(tc.route)); got != 1 {
			t.Errorf("requests to %s = %d, want exactly 1", tc.route, got)
		}
	}
}

// TestTradePushParamsAreNeverNil states the struct{}{} rule as a test rather than a
// comment, at the only level that can see the difference: the recorded body.
//
// A nil params is dropped from the envelope by internal/transport, because
// request.Params carries omitempty, so a call that passed nil would send an
// envelope with no params key at all. That is a different request from the one
// both vendor SDKs send and from the one the released layer sends, and it is
// invisible at the fake level and to a struct assertion.
//
// The positive half of the assertion - the key is present, and its value is the
// empty object - is what makes the negative half mean something: "nothing
// unexpected is in the body" would also be satisfied by a body carrying nothing
// at all.
func TestTradePushParamsAreNeverNil(t *testing.T) {
	rec := newWireRecorder(map[string]string{
		string(client.RouteTradeSubscribe):   gatewaySuccess(`{"success":true}`),
		string(client.RouteTradeUnsubscribe): gatewaySuccess(`{"success":true}`),
	})
	svc := NewTradingService(newWireExecutor(t, rec))
	account := tradingAccountID()

	for _, tc := range tradePushCalls() {
		if err := tc.invoke(t.Context(), svc, account); err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		body := rec.lastBody(t, string(tc.route))
		if !strings.Contains(body, `"params"`) {
			t.Errorf("the %s body has no params key at all, so the params were nil rather "+
				"than an empty object:\n%s", tc.route, body)
			continue
		}
		if !strings.Contains(body, `"params":{}`) {
			t.Errorf("the %s body does not carry an empty params object:\n%s", tc.route, body)
		}
	}

	// The same property at the fake level, where the value handed to the executor is
	// visible rather than its rendering. A nil here is what produces the absent key
	// above, so the two checks are one property observed from both sides.
	exec := newSequencedExecutor(t, tradePushAck(), tradePushAck())
	fake := NewTradingService(exec)
	for i, tc := range tradePushCalls() {
		if err := tc.invoke(t.Context(), fake, account); err != nil {
			t.Fatalf("%s over the fake: %v", tc.name, err)
		}
		// The op and route are asserted here rather than through tradingExpectCall,
		// which pins a single Do and so cannot be called twice against one executor.
		call := exec.callAt(t, i)
		if call.op != tc.op {
			t.Errorf("call %d op = %q, want %q", i, call.op, tc.op)
		}
		if call.route != tc.route {
			t.Errorf("call %d route = %q, want %q", i, call.route, tc.route)
		}
		if params := call.params; params == nil {
			t.Fatalf("call %d passed nil params: internal/transport drops a nil params from "+
				"the envelope, so the key would be absent rather than {}", i)
		}
	}
	requireCalls(t, exec, 2)
}

// TestTradePushAccountIDNeverReachesTheWire states the account rule as an assertion
// on the rendered request body, at the real-HTTP level so the check is on bytes the
// Gateway would actually receive.
//
// It is checked twice and the two are not the same test. At the fake level the
// params value is marshalled here, so a wire struct with an AccountID field would
// be caught even before it is compiled into a body. At the HTTP level the whole
// envelope is searched, which also catches an account identifier placed outside
// params - in a path, an op label, or anywhere else the transport could put one.
func TestTradePushAccountIDNeverReachesTheWire(t *testing.T) {
	account := tradingAccountID()
	rec := newWireRecorder(map[string]string{
		string(client.RouteTradeSubscribe):   gatewaySuccess(`{"success":true}`),
		string(client.RouteTradeUnsubscribe): gatewaySuccess(`{"success":true}`),
	})
	svc := NewTradingService(newWireExecutor(t, rec))

	for _, tc := range tradePushCalls() {
		if err := tc.invoke(t.Context(), svc, account); err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
	}
	for _, tc := range tradePushCalls() {
		if body := rec.lastBody(t, string(tc.route)); strings.Contains(body, string(account)) {
			t.Errorf("the %s body carries the account id:\n%s", tc.route, body)
		}
	}

	exec := newSequencedExecutor(t, tradePushAck(), tradePushAck())
	fake := NewTradingService(exec)
	for i, tc := range tradePushCalls() {
		if err := tc.invoke(t.Context(), fake, account); err != nil {
			t.Fatalf("%s over the fake: %v", tc.name, err)
		}
		rendered, err := json.Marshal(exec.callAt(t, i).params)
		if err != nil {
			t.Fatalf("rendering call %d params: %v", i, err)
		}
		if strings.Contains(string(rendered), string(account)) {
			t.Errorf("call %d put the account id in the wire struct: %s", i, rendered)
		}
	}
}

// ---------------------------------------------------------------------------
// Local rejections, at zero requests
// ---------------------------------------------------------------------------

// TestTradePushLocalRejectionCostsNoRequest is the fail-closed property, and the
// fixture is the assertion: newSequencedExecutor is built with *no* scripted reply,
// so a request that escaped the validation would hit the fake's own t.Fatalf
// rather than be quietly absorbed. A rejection that cost a request therefore fails
// this test instead of passing it, which is the direction a zero-argument
// validation test can be fooled in and the one that matters here.
//
// Both methods are driven and each is checked against its own op, because a
// SubscribeOrders that reported its rejection under opTradeUnsubscribe would pass
// a per-method test that only asserted the status code.
func TestTradePushLocalRejectionCostsNoRequest(t *testing.T) {
	for _, tc := range tradePushCalls() {
		t.Run(tc.name, func(t *testing.T) {
			exec := newSequencedExecutor(t)
			svc := NewTradingService(exec)

			// The zero account is a real domain.AccountID that happens to be empty -
			// which is what IsZero reports on - rather than a nil or an omitted
			// argument.
			assertInvalidParam(t, tc.invoke(t.Context(), svc, ""), tc.op)
			requireCalls(t, exec, 0)
		})
	}
}

// TestTradePushAcceptsTheMinimumValidAccount is the accept counterpart, because a
// method that refused every account would pass the row above and a caller would
// never be able to subscribe at all. A one-character account is the smallest
// non-zero value, and this package asks for nothing more.
func TestTradePushAcceptsTheMinimumValidAccount(t *testing.T) {
	exec := newSequencedExecutor(t, tradePushAck(), tradePushAck())
	svc := NewTradingService(exec)

	for _, tc := range tradePushCalls() {
		if err := tc.invoke(t.Context(), svc, "a"); err != nil {
			t.Errorf("%s with a one-character account: %v", tc.name, err)
		}
	}
	requireCalls(t, exec, 2)
}

// ---------------------------------------------------------------------------
// The reply is discarded
// ---------------------------------------------------------------------------

// TestTradePushDiscardsItsReply proves the response is handed to the executor as a
// nil out, which is what the released layer does (push.go passes a literal nil as
// its last argument) and what makes a nil error the whole result.
//
// The proof is a reply that could not survive a decode. `data` is a JSON number, so
// a method that passed a non-nil out would get a typed "decode response data" error
// out of internal/transport, while a method that passed nil returns before the codec
// is consulted at all. Asserting only that a well-formed reply is accepted would not
// distinguish the two, because an empty object decodes into every struct.
//
// The request count is asserted too, so a service that discarded the reply by never
// sending anything would fail on the count rather than pass quietly.
func TestTradePushDiscardsItsReply(t *testing.T) {
	account := tradingAccountID()
	for _, tc := range tradePushCalls() {
		t.Run(tc.name, func(t *testing.T) {
			rec := newWireRecorder(map[string]string{
				string(tc.route): gatewaySuccess(`42`),
			})
			svc := NewTradingService(newWireExecutor(t, rec))

			if err := tc.invoke(t.Context(), svc, account); err != nil {
				t.Fatalf("%s on a reply whose data is a JSON number: %v. The reply must be "+
					"discarded (out == nil), not decoded into a struct.", tc.name, err)
			}
			if got := rec.count(string(tc.route)); got != 1 {
				t.Errorf("requests to %s = %d, want 1: discarding the reply must not mean "+
					"skipping the request", tc.route, got)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Error arms
// ---------------------------------------------------------------------------

// TestTradePushGatewayRejectionArrivesTyped is the arm that matters most for these
// two endpoints specifically, because it is the one the GoDoc promises and the only
// one this package cannot check locally: the Gateway refuses a push route made
// without an authenticated session, and the refusal must reach the caller as a typed
// error carrying this method's own op and a code that says "log in again".
//
// There is no local session gate to test, because pkg/services holds no session store
// and the session lives in SessionService behind an Executor boundary. So the
// Gateway's answer is the only thing there is to assert, and asserting it is what
// tells a caller what to do: re-authenticate, then call again. A caller handed an
// opaque error here could not tell "the Gateway is busy" from "you are not logged
// in", and those need opposite responses.
func TestTradePushGatewayRejectionArrivesTyped(t *testing.T) {
	account := tradingAccountID()
	codes := []struct {
		name        string
		code        types.StatusCode
		wantRetry   bool
		wantReLogin bool
		wantCat     errs.Category
	}{
		{"a not-logged-in rejection", types.StatusNotLoggedIn, false, true, errs.CategoryAccount},
		{"a displaced session", types.StatusKickedOffline, false, true, errs.CategoryAccount},
		{"a rate limit", types.StatusServiceBusy, true, false, errs.CategoryRateLimit},
		{"a failed call", types.StatusCallFailed, false, false, errs.CategoryAPI},
	}

	for _, tc := range tradePushCalls() {
		for _, cc := range codes {
			t.Run(tc.name+"/"+cc.name, func(t *testing.T) {
				rec := newWireRecorder(map[string]string{
					string(tc.route): gatewayFailure(cc.code, "the Gateway said no"),
				})
				svc := NewTradingService(newWireExecutor(t, rec))

				err := tc.invoke(t.Context(), svc, account)
				errRejects(t, err, cc.code, tc.op)
				if got := errs.CategoryOf(err); got != cc.wantCat {
					t.Errorf("CategoryOf = %q, want %q", got, cc.wantCat)
				}
				if got := errs.ReLoginRequired(err); got != cc.wantReLogin {
					t.Errorf("ReLoginRequired = %v, want %v: this is the signal that tells a "+
						"caller to log in again, and it is the only thing standing in for "+
						"the session check this package cannot perform", got, cc.wantReLogin)
				}
				if got := errs.Retryable(err); got != cc.wantRetry {
					t.Errorf("Retryable = %v, want %v", got, cc.wantRetry)
				}
			})
		}
	}
}

// TestTradePushExecutorFailurePropagates is the fake-level counterpart: the
// executor's own error reaches the caller unchanged, after exactly one attempt, so
// a service that wrapped it, swallowed it, or grew a loop of its own fails here.
//
// One attempt is not the retry class - that is measured at the real-HTTP level in
// TestTradePushRequestsRetryAndOrderMutationsDoNot, with a policy installed. This
// row is here because a fake is the only level that can show a service-side loop: a
// real client would hide a second Do behind its own classification, so "the service
// asked once" is not otherwise observable.
//
// The scripted error is a genuine *errs.Error carrying 1011 and the test first
// checks errs.Retryable agrees it is retryable, so "one attempt" is not true for the
// boring reason that nothing wanted a second one.
func TestTradePushExecutorFailurePropagates(t *testing.T) {
	account := tradingAccountID()
	for _, tc := range tradePushCalls() {
		t.Run(tc.name, func(t *testing.T) {
			busy := errs.New(types.StatusServiceBusy, tc.op, "service busy, retry later")
			if !errs.Retryable(busy) {
				t.Fatalf("test bug: %q is not retryable, so a single call would be expected "+
					"even without the guarantee this row is about", types.StatusServiceBusy)
			}
			exec := newSequencedExecutor(t,
				sequencedReply{err: busy},
				sequencedReply{err: busy},
			)
			svc := NewTradingService(exec)

			err := tc.invoke(t.Context(), svc, account)
			if !errors.Is(err, busy) {
				t.Fatalf("%s = %v, want the executor's own error back unchanged", tc.name, err)
			}
			errRejects(t, err, types.StatusServiceBusy, tc.op)
			if got := errs.CategoryOf(err); got != errs.CategoryRateLimit {
				t.Errorf("CategoryOf = %q, want %q", got, errs.CategoryRateLimit)
			}
			requireCalls(t, exec, 1)
		})
	}
}

// ---------------------------------------------------------------------------
// Retry classification
// ---------------------------------------------------------------------------

// TestTradePushRequestsRetryAndOrderMutationsDoNot is the measurement the whole file
// exists for, and it is the reason these two paths are *not* added to
// internal/resilience's mutation set.
//
// Under one MaxAttempts-5 policy with a retryable 1011 on every path, a push route
// takes all five requests and an order mutation takes one. Five is only reachable by
// a policy that is installed and firing, and one is only meaningful beside it: a
// client with no policy installed also sends a mutation once, so without the
// contrast the mutation rows would pass for the wrong reason. This is the same
// control TestD2AMutationIsSentExactlyOnceUnderARetryPolicy and
// TestD3AReadOnlyQueryDoesRetry use, run here for the two routes whose own GoDoc
// claims the opposite.
//
// The classification is right, and the reason is worth stating because it is the
// argument against "fixing" it: ADR 0003 exists because a retry of a failed entrust
// can place an order the caller did not intend. A subscription cannot do that.
// Re-sending a subscribe that never took effect, and unsubscribing a subscription
// that is already gone, are the same no-op twice - which is the idempotence a retry
// needs. Adding these two paths to mutationPaths would make this layer's behaviour
// disagree with the released layer's, which sends both through the same client path
// this one does.
func TestTradePushRequestsRetryAndOrderMutationsDoNot(t *testing.T) {
	const maxAttempts = 5
	account := tradingAccountID()
	busy := gatewayFailure(types.StatusServiceBusy, "service busy, retry later")
	policy := client.WithRetryPolicy(client.RetryPolicy{MaxAttempts: maxAttempts, BaseBackoff: 0})

	// Every path answers busy, not just the ones under test, so a service that
	// quietly issued a request somewhere else would show up in total().
	paths := map[string]string{
		string(client.RouteTradeSubscribe):   busy,
		string(client.RouteTradeUnsubscribe): busy,
	}
	for _, m := range tradingMutationAttempts() {
		paths[string(m.route)] = busy
	}

	for _, tc := range tradePushCalls() {
		t.Run(tc.name+" retries", func(t *testing.T) {
			rec := newWireRecorder(paths)
			svc := NewTradingService(newWireExecutor(t, rec, policy))

			errRejects(t, tc.invoke(t.Context(), svc, account), types.StatusServiceBusy, tc.op)
			if got := rec.count(string(tc.route)); got != maxAttempts {
				t.Errorf("requests to %s = %d, want %d. A push route is not an order "+
					"mutation, so a retry policy applies to it - the classification "+
					"client.IsQuery gives this path. Changing this row to expect 1 would "+
					"be a behaviour change to the released layer, not a fix to this one.",
					tc.route, got, maxAttempts)
			}
			if got := rec.total(); got != maxAttempts {
				t.Errorf("total requests = %d, want %d: the policy must not have spent an "+
					"attempt on any other path", got, maxAttempts)
			}
		})
	}

	t.Run("an order mutation does not", func(t *testing.T) {
		rec := newWireRecorder(paths)
		svc := NewTradingService(newWireExecutor(t, rec, policy))

		for _, m := range tradingMutationAttempts() {
			t.Run(m.name, func(t *testing.T) {
				if err := m.invoke(t.Context(), svc); err == nil {
					t.Fatalf("%s = nil error, want the %q rejection", m.name, types.StatusServiceBusy)
				}
				if got := rec.count(string(m.route)); got != 1 {
					t.Errorf("requests to %s = %d, want exactly 1 under the identical policy "+
						"that sent both push routes %d times. This contrast is the only "+
						"evidence that the two push rows above are measuring the retry class "+
						"rather than a policy that never fired.", m.route, got, maxAttempts)
				}
			})
		}
	})
}
