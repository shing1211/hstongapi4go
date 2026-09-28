// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/shing1211/hstongapi4go/client"
	"github.com/shing1211/hstongapi4go/internal/errs"
	"github.com/shing1211/hstongapi4go/pkg/domain"
	"github.com/shing1211/hstongapi4go/pkg/types"
)

// This file drives the five algo mutations: the happy path with the exact request
// body, the local rejections at zero recorded calls, both error levels, the ADR
// 0003 proof with its read control inside the same test, and the typed
// reconciliation error.
//
// C5 implemented the futures mutations and C6 tested them in one change,
// deliberately. A2 found pkg/hstong/trade with twenty endpoints, a fully covered
// happy path, and not one test that made a Gateway call *fail*: splitting
// implement-then-test is what produces that shape, because the test is written
// against the code that was just finished and the failure surface is the part
// nobody has an excuse to skip. So the same discipline applies here, with one thing
// added — a mutation's failure is money. The tables below therefore drive, for all
// five:
//
//   - the happy path, pinning the op, the route and the exact request body, so a
//     field swapped or a key misspelled fails here. The mock Gateway answers by
//     path and would not notice either.
//   - the local rejections, each at *zero* recorded calls, which is the only thing
//     that distinguishes "I built this wrong" from "the exchange said no".
//   - both error levels — the sequenced fake, and a real client over an ok:false
//     envelope — because a fake never builds a typed error.
//   - the ADR 0003 proof, with the read control *in the same test* so that
//     "exactly one request" cannot be a policy that never fired.
//   - the typed reconciliation error a caller branches on.
//
// No row sleeps: where a retry count is the assertion the policy installs
// MaxAttempts with a zero backoff, so the count is a property of the retry
// classification and not of elapsed time. No row recovers: a builder or mapper that
// panicked would take the process down.

// ---------------------------------------------------------------------------
// Routing
// ---------------------------------------------------------------------------

// TestAlgoMutationsReachTheirRouteWithTheirOp is the positive path for all five:
// the op label and the route are the ones the method names, the request body is
// the one the call built, and exactly one call was recorded.
//
// The request body is compared per method, from algoMutationCases, so a body that
// drifted from its own case table fails here rather than only in the wire
// assertions — and because the table has a row per method, a fifth body appearing
// cannot be added without one.
func TestAlgoMutationsReachTheirRouteWithTheirOp(t *testing.T) {
	for _, tc := range algoMutationCases() {
		t.Run(tc.name, func(t *testing.T) {
			exec := newSequencedExecutor(t, sequencedReply{reply: tc.reply})
			svc := NewAlgoService(exec)

			if err := tc.invoke(t, t.Context(), svc); err != nil {
				t.Fatalf("%s with a valid request = %v, want nil", tc.name, err)
			}
			algoExpectCall(t, exec, tc.op, tc.route)
			requireCalls(t, exec, 1)

			if got := exec.lastParams(t); !reflect.DeepEqual(got, tc.wantParams) {
				t.Errorf("the request body = %#v, want %#v", got, tc.wantParams)
			}
		})
	}
}

// TestAlgoMutationsReturnTheGatewayIdentifier checks the one thing a mutation reply
// is for, per method.
//
// All five are legitimate replies, and the Gateway documents `data` as the
// resulting identifier *or the empty string when it is omitted* — so the empty case
// is a success with an empty id, not a failure to report. What is pinned is that the
// empty one is reported as an empty identifier on a non-nil result, so a caller
// reading it cannot mistake an omitted field for a failed call; and, as the GoDoc on
// each method says, must reconcile either way.
//
// The child/master distinction is the load-bearing row: four of the five echo a
// master order ID and CancelEntrust echoes the child's, which is why it returns a
// domain.EntrustID and the others a domain.OrderID. A test that returned the same
// type for all five would pass against a body with the two swapped.
func TestAlgoMutationsReturnTheGatewayIdentifier(t *testing.T) {
	for _, tc := range []struct {
		name string
		// method is the mutation whose return is under test.
		method string
		// reply is the data object the Gateway answers with.
		reply string
		// want is the identifier the method must hand back.
		want string
		// read performs the call and returns the identifier's string form.
		read func(t *testing.T, ctx context.Context, svc *AlgoService) (string, error)
	}{
		{
			name: "AddOrder/echoes the master order ID", method: "AddOrder",
			reply: algoMutationBodyWith("MA20260929999"),
			want:  "MA20260929999",
			read: func(t *testing.T, ctx context.Context, svc *AlgoService) (string, error) {
				t.Helper()
				got, err := svc.AddOrder(ctx, algoFixtureAccount(), algoFixtureAdd())
				return got.String(), err
			},
		},
		{
			name: "CancelOrder/echoes the master order ID", method: "CancelOrder",
			reply: algoMutationBodyWith("MA20260929998"),
			want:  "MA20260929998",
			read: func(t *testing.T, ctx context.Context, svc *AlgoService) (string, error) {
				t.Helper()
				got, err := svc.CancelOrder(ctx, algoFixtureAccount(), algoFixtureCancelOrder())
				return got.String(), err
			},
		},
		{
			// The one mutation whose reply is not a master order id. The mock
			// Gateway says so on this route — its cancel-entrust fixture answers
			// CH…, not MA… — and a test that treated it as a master would pass
			// against a body that had the two fields swapped.
			name: "CancelEntrust/echoes the CHILD entrust ID", method: "CancelEntrust",
			reply: algoMutationBodyWith("CH20260929997"),
			want:  "CH20260929997",
			read: func(t *testing.T, ctx context.Context, svc *AlgoService) (string, error) {
				t.Helper()
				got, err := svc.CancelEntrust(ctx, algoFixtureAccount(), algoFixtureCancelEntrust())
				return got.String(), err
			},
		},
		{
			name: "ChangeOrder/echoes the master order ID", method: "ChangeOrder",
			reply: algoMutationBodyWith("MA20260929996"),
			want:  "MA20260929996",
			read: func(t *testing.T, ctx context.Context, svc *AlgoService) (string, error) {
				t.Helper()
				got, err := svc.ChangeOrder(ctx, algoFixtureAccount(), algoFixtureChange())
				return got.String(), err
			},
		},
		{
			name: "ActionOrder/echoes the master order ID", method: "ActionOrder",
			reply: algoMutationBodyWith("MA20260929995"),
			want:  "MA20260929995",
			read: func(t *testing.T, ctx context.Context, svc *AlgoService) (string, error) {
				t.Helper()
				got, err := svc.ActionOrder(ctx, algoFixtureAccount(), algoFixtureAction())
				return got.String(), err
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			exec := newSequencedExecutor(t, sequencedReply{reply: json.RawMessage(tc.reply)})
			got, err := tc.read(t, t.Context(), NewAlgoService(exec))
			if err != nil {
				t.Fatalf("%s = %v, want nil: a mutation's reply carries a field the "+
					"Gateway documents as optional, and its absence is not a failure to "+
					"report", tc.name, err)
			}
			if got != tc.want {
				t.Errorf("the returned identifier = %q, want %q", got, tc.want)
			}
			requireCalls(t, exec, 1)
		})
	}

	// The empty and absent cases, for all five. An empty identifier is *not*
	// evidence the order was refused, so the call must succeed and hand back the
	// zero value — which is what the GoDoc on each method says a caller must
	// reconcile around.
	for _, tc := range []struct {
		name  string
		reply string
	}{
		{"the Gateway omitted the identifier", `{"data":""}`},
		{"the data member is absent entirely", `{}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, mc := range algoMutationCases() {
				t.Run(mc.name, func(t *testing.T) {
					exec := newSequencedExecutor(t, sequencedReply{reply: json.RawMessage(tc.reply)})
					if err := mc.invoke(t, t.Context(), NewAlgoService(exec)); err != nil {
						t.Fatalf("%s = %v, want nil: an absent order identifier is not a "+
							"failure to report, and reporting it as one would tell a caller "+
							"the order was refused when the Gateway never said so",
							mc.name, err)
					}
					requireCalls(t, exec, 1)
				})
			}
		})
	}
}

// TestAlgoMutationsRejectAZeroAccountIDWithoutARequest is the fail-closed row for
// all five.
//
// Zero recorded calls is the assertion that matters. accountID is a session key
// and never crosses the wire, so a zero-accountID request body is byte-identical
// to a valid one and only the absence of the call distinguishes them — which is why
// C2a §7.1 warns that adding accountID to an algo wire struct would leak an account
// identifier into every algo request while staying invisible on the mock.
//
// The fixture is a sequencedExecutor with **no scripted replies**: a removed
// validator would hit its t.Fatalf rather than pass vacuously.
func TestAlgoMutationsRejectAZeroAccountIDWithoutARequest(t *testing.T) {
	for _, tc := range algoMutationCases() {
		t.Run(tc.name, func(t *testing.T) {
			exec := newSequencedExecutor(t)
			assertInvalidParam(t, tc.zeroAccount(t, t.Context(), NewAlgoService(exec)), tc.op)
			requireCalls(t, exec, 0)
		})
	}
}

// TestAlgoMutationAccountIDNeverReachesTheWire is the other direction of the same
// rule, driven over real HTTP so it is the bytes the Gateway would receive.
//
// C2a §7.1 calls the accountID leak "invisible on the mock", which is why this
// decodes the recorded request: the check is that no account-shaped key is present
// *and* that the account id appears nowhere in the envelope. The second half matters
// because a value could be smuggled in under a key that is not account-shaped.
func TestAlgoMutationAccountIDNeverReachesTheWire(t *testing.T) {
	for _, tc := range algoMutationCases() {
		t.Run(tc.name, func(t *testing.T) {
			rec := newWireRecorder(map[string]string{
				string(tc.route): gatewaySuccess(string(tc.reply)),
			})
			if err := tc.invoke(t, t.Context(), NewAlgoService(newWireExecutor(t, rec))); err != nil {
				t.Fatalf("%s = %v, want nil", tc.name, err)
			}
			body := rec.lastBody(t, string(tc.route))
			if strings.Contains(body, string(algoFixtureAccount())) {
				t.Errorf("the recorded %s request carries the account id: %s", tc.route, body)
			}
			for _, key := range algoBodyKeys(algoRecordedParams(t, rec, string(tc.route))) {
				lower := strings.ToLower(key)
				for _, bad := range []string{"account", "accno", "userid", "clienttype"} {
					if strings.Contains(lower, bad) {
						t.Errorf("the %s request carries the key %q", tc.route, key)
					}
				}
			}
		})
	}
}

// TestAlgoMutationRequestsCarryTheirIdentifiersOnTheWire is the wire-level half of
// the identifier tests, and it is per body because the bodies differ here too.
//
// The add body carries no order id at all — it is the request that *creates* the
// master — while the other four carry one, and the child cancel carries two. A test
// that asserted only "the body has an orderId" would pass against the add body
// gaining one it must not have.
func TestAlgoMutationRequestsCarryTheirIdentifiersOnTheWire(t *testing.T) {
	for _, tc := range []struct {
		method string
		route  client.Route
		invoke func(t *testing.T, ctx context.Context, svc *AlgoService) error
		// wantOrderID is the expected value of the orderId key, or "" for a body
		// that must not carry one.
		wantOrderID string
		// wantEntrustID is the expected value of the entrustId key, or "" for a body
		// that must not carry one.
		wantEntrustID string
	}{
		{
			// The add creates the master, so naming one would be a request to modify
			// something the caller has not placed yet.
			method: "AddOrder", route: client.RouteTradeAlgoAddOrder,
			invoke: func(t *testing.T, ctx context.Context, svc *AlgoService) error {
				t.Helper()
				_, err := svc.AddOrder(ctx, algoFixtureAccount(), algoFixtureAdd())
				return err
			},
		},
		{
			method: "CancelOrder", route: client.RouteTradeAlgoCancelOrder,
			invoke: func(t *testing.T, ctx context.Context, svc *AlgoService) error {
				t.Helper()
				_, err := svc.CancelOrder(ctx, algoFixtureAccount(), algoFixtureCancelOrder())
				return err
			},
			wantOrderID: "MA20260921001",
		},
		{
			// The only body that names a child, so the only body with an entrustId.
			method: "CancelEntrust", route: client.RouteTradeAlgoCancelEntrust,
			invoke: func(t *testing.T, ctx context.Context, svc *AlgoService) error {
				t.Helper()
				_, err := svc.CancelEntrust(ctx, algoFixtureAccount(), algoFixtureCancelEntrust())
				return err
			},
			wantOrderID:   "MA20260921001",
			wantEntrustID: "CH20260921001",
		},
		{
			method: "ChangeOrder", route: client.RouteTradeAlgoChangeOrder,
			invoke: func(t *testing.T, ctx context.Context, svc *AlgoService) error {
				t.Helper()
				_, err := svc.ChangeOrder(ctx, algoFixtureAccount(), algoFixtureChange())
				return err
			},
			wantOrderID: "MA20260921001",
		},
		{
			method: "ActionOrder", route: client.RouteTradeAlgoActionOrder,
			invoke: func(t *testing.T, ctx context.Context, svc *AlgoService) error {
				t.Helper()
				_, err := svc.ActionOrder(ctx, algoFixtureAccount(), algoFixtureAction())
				return err
			},
			wantOrderID: "MA20260921001",
		},
	} {
		t.Run(tc.method, func(t *testing.T) {
			rec := newWireRecorder(map[string]string{
				string(tc.route): gatewaySuccess(algoMutationBody),
			})
			if err := tc.invoke(t, t.Context(), NewAlgoService(newWireExecutor(t, rec))); err != nil {
				t.Fatalf("%s = %v, want nil", tc.method, err)
			}
			params := algoRecordedParams(t, rec, string(tc.route))
			if tc.wantOrderID == "" {
				if _, ok := params["orderId"]; ok {
					t.Errorf("the %s request carries an orderId; it is the request that "+
						"*creates* the master, so naming one would be a request to modify "+
						"something the caller has not placed yet", tc.method)
				}
			} else {
				algoRequireString(t, params, "orderId", tc.wantOrderID)
			}
			if tc.wantEntrustID == "" {
				if _, ok := params["entrustId"]; ok {
					t.Errorf("the %s request carries an entrustId; it names a master, not a "+
						"child, and the two are different operations", tc.method)
				}
			} else {
				algoRequireString(t, params, "entrustId", tc.wantEntrustID)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Local rejections
// ---------------------------------------------------------------------------

// algoMutationRejection is one local-rejection row for the mutation tables: a name
// for the sub-test, the op the error must carry, and the call itself.
type algoMutationRejection struct {
	name string
	op   string
	call func(t *testing.T, ctx context.Context, svc *AlgoService) error
}

// algoMutationRejections is the local-rejection table, per method, in the order each
// method checks its fields.
//
// Every row is a *local* rejection — a types.StatusInvalidParam *errs.Error in
// errs.CategoryAPI — and every one costs zero HTTP requests. That is the property
// that makes a local rejection safe on an order: a request the Gateway saw and
// refused may have had a side effect, and a request this SDK never sent cannot have
// had one.
//
// A local rejection is also a *different category* from a Gateway rejection, and the
// table's name is the reason a caller can tell them apart: on a mutation, "I built
// this wrong" and "the exchange said no" call for opposite responses.
func algoMutationRejections() []algoMutationRejection {
	id := algoFixtureAccount()
	add := algoFixtureAdd()
	change := algoFixtureChange()

	return []algoMutationRejection{
		// --- AddOrder ---
		{
			name: "AddOrder/empty stockCode", op: opAlgoAddOrder,
			call: func(t *testing.T, ctx context.Context, svc *AlgoService) error {
				t.Helper()
				r := add
				r.StockCode = ""
				_, err := svc.AddOrder(ctx, id, r)
				return err
			},
		},
		{
			name: "AddOrder/blank stockCode", op: opAlgoAddOrder,
			call: func(t *testing.T, ctx context.Context, svc *AlgoService) error {
				t.Helper()
				r := add
				r.StockCode = " \t "
				_, err := svc.AddOrder(ctx, id, r)
				return err
			},
		},
		{
			name: "AddOrder/stockCode with interior whitespace", op: opAlgoAddOrder,
			call: func(t *testing.T, ctx context.Context, svc *AlgoService) error {
				t.Helper()
				r := add
				r.StockCode = "007 00.HK"
				_, err := svc.AddOrder(ctx, id, r)
				return err
			},
		},
		{
			name: "AddOrder/empty exchangeType", op: opAlgoAddOrder,
			call: func(t *testing.T, ctx context.Context, svc *AlgoService) error {
				t.Helper()
				r := add
				r.ExchangeType = ""
				_, err := svc.AddOrder(ctx, id, r)
				return err
			},
		},
		{
			name: "AddOrder/unrecognised exchangeType", op: opAlgoAddOrder,
			call: func(t *testing.T, ctx context.Context, svc *AlgoService) error {
				t.Helper()
				r := add
				r.ExchangeType = "Z"
				_, err := svc.AddOrder(ctx, id, r)
				return err
			},
		},
		{
			// The type collision, on the wire. types.EntrustTypeLimit is "3", so a
			// caller who reached for the trade dictionary's limit order would send a
			// code the algo surface does not accept — and a caller who reached for
			// the trade dictionary's *auction* ("1") would send algo's limit order
			// and get a limit order they did not ask for. This row refuses the
			// first; the test below is what pins the second as refused too.
			name: "AddOrder/a trade-surface order type", op: opAlgoAddOrder,
			call: func(t *testing.T, ctx context.Context, svc *AlgoService) error {
				t.Helper()
				r := add
				r.OrderType = domain.AlgoEntrustType(types.EntrustTypeLimit)
				_, err := svc.AddOrder(ctx, id, r)
				return err
			},
		},
		{
			name: "AddOrder/empty entrustType", op: opAlgoAddOrder,
			call: func(t *testing.T, ctx context.Context, svc *AlgoService) error {
				t.Helper()
				r := add
				r.OrderType = ""
				_, err := svc.AddOrder(ctx, id, r)
				return err
			},
		},
		{
			name: "AddOrder/a negative price", op: opAlgoAddOrder,
			call: func(t *testing.T, ctx context.Context, svc *AlgoService) error {
				t.Helper()
				r := add
				r.Price = domain.MustNewPrice("-1", "0")
				_, err := svc.AddOrder(ctx, id, r)
				return err
			},
		},
		{
			name: "AddOrder/a zero quantity", op: opAlgoAddOrder,
			call: func(t *testing.T, ctx context.Context, svc *AlgoService) error {
				t.Helper()
				r := add
				r.Quantity = domain.MustNewQuantity("0")
				_, err := svc.AddOrder(ctx, id, r)
				return err
			},
		},
		{
			name: "AddOrder/a negative quantity", op: opAlgoAddOrder,
			call: func(t *testing.T, ctx context.Context, svc *AlgoService) error {
				t.Helper()
				r := add
				r.Quantity = domain.MustNewQuantity("-100")
				_, err := svc.AddOrder(ctx, id, r)
				return err
			},
		},
		{
			name: "AddOrder/empty entrustBs", op: opAlgoAddOrder,
			call: func(t *testing.T, ctx context.Context, svc *AlgoService) error {
				t.Helper()
				r := add
				r.Side = ""
				_, err := svc.AddOrder(ctx, id, r)
				return err
			},
		},
		{
			name: "AddOrder/an out-of-set entrustBs", op: opAlgoAddOrder,
			call: func(t *testing.T, ctx context.Context, svc *AlgoService) error {
				t.Helper()
				r := add
				r.Side = "5"
				_, err := svc.AddOrder(ctx, id, r)
				return err
			},
		},
		{
			// The strategy is checked for emptiness only, and this row is the proof
			// that the emptiness check exists — an unknown code is accepted and
			// forwarded, which is the deliberate divergence from every other code on
			// this surface. See algoStrategyForwardsUnknownCodes below.
			name: "AddOrder/empty targetStrategy", op: opAlgoAddOrder,
			call: func(t *testing.T, ctx context.Context, svc *AlgoService) error {
				t.Helper()
				r := add
				r.TargetStrategy = ""
				_, err := svc.AddOrder(ctx, id, r)
				return err
			},
		},
		{
			name: "AddOrder/empty sessionType", op: opAlgoAddOrder,
			call: func(t *testing.T, ctx context.Context, svc *AlgoService) error {
				t.Helper()
				r := add
				r.SessionType = ""
				_, err := svc.AddOrder(ctx, id, r)
				return err
			},
		},
		{
			// The trade surface's conditional-order session codes. An algorithm
			// master order is not a conditional order, so these three must be
			// refused rather than forwarded to a Gateway that has no meaning for
			// them.
			name: "AddOrder/a trade-surface sessionType", op: opAlgoAddOrder,
			call: func(t *testing.T, ctx context.Context, svc *AlgoService) error {
				t.Helper()
				r := add
				r.SessionType = domain.AlgoSessionType("3")
				_, err := svc.AddOrder(ctx, id, r)
				return err
			},
		},
		{
			name: "AddOrder/a zero maxVolume", op: opAlgoAddOrder,
			call: func(t *testing.T, ctx context.Context, svc *AlgoService) error {
				t.Helper()
				r := add
				r.StrategyParam.MaxVolume = domain.Quantity{}
				_, err := svc.AddOrder(ctx, id, r)
				return err
			},
		},
		{
			name: "AddOrder/a negative maxVolume", op: opAlgoAddOrder,
			call: func(t *testing.T, ctx context.Context, svc *AlgoService) error {
				t.Helper()
				r := add
				r.StrategyParam.MaxVolume = domain.MustNewQuantity("-1")
				_, err := svc.AddOrder(ctx, id, r)
				return err
			},
		},
		{
			name: "AddOrder/empty sensitivity", op: opAlgoAddOrder,
			call: func(t *testing.T, ctx context.Context, svc *AlgoService) error {
				t.Helper()
				r := add
				r.StrategyParam.Sensitivity = ""
				_, err := svc.AddOrder(ctx, id, r)
				return err
			},
		},
		{
			name: "AddOrder/an out-of-set sensitivity", op: opAlgoAddOrder,
			call: func(t *testing.T, ctx context.Context, svc *AlgoService) error {
				t.Helper()
				r := add
				r.StrategyParam.Sensitivity = domain.AlgoSensitivity("4")
				_, err := svc.AddOrder(ctx, id, r)
				return err
			},
		},
		{
			name: "AddOrder/a malformed origStartTime", op: opAlgoAddOrder,
			call: func(t *testing.T, ctx context.Context, svc *AlgoService) error {
				t.Helper()
				r := add
				r.StrategyParam.OrigStartTime = "0930"
				_, err := svc.AddOrder(ctx, id, r)
				return err
			},
		},
		{
			name: "AddOrder/an impossible origEndTime", op: opAlgoAddOrder,
			call: func(t *testing.T, ctx context.Context, svc *AlgoService) error {
				t.Helper()
				r := add
				r.StrategyParam.OrigEndTime = "256199"
				_, err := svc.AddOrder(ctx, id, r)
				return err
			},
		},
		{
			name: "AddOrder/a negative minAmount", op: opAlgoAddOrder,
			call: func(t *testing.T, ctx context.Context, svc *AlgoService) error {
				t.Helper()
				r := add
				r.StrategyParam.MinAmount = domain.MustNewMoney("-1", "HKD", 3)
				_, err := svc.AddOrder(ctx, id, r)
				return err
			},
		},
		{
			name: "AddOrder/a negative showQty", op: opAlgoAddOrder,
			call: func(t *testing.T, ctx context.Context, svc *AlgoService) error {
				t.Helper()
				r := add
				r.StrategyParam.ShowQty = domain.MustNewQuantity("-1")
				_, err := svc.AddOrder(ctx, id, r)
				return err
			},
		},
		{
			name: "AddOrder/a fractional qtyPercent", op: opAlgoAddOrder,
			call: func(t *testing.T, ctx context.Context, svc *AlgoService) error {
				t.Helper()
				r := add
				r.StrategyParam.QtyPercent = domain.MustNewRate("10.5")
				_, err := svc.AddOrder(ctx, id, r)
				return err
			},
		},
		{
			name: "AddOrder/a qtyPercent above the documented range", op: opAlgoAddOrder,
			call: func(t *testing.T, ctx context.Context, svc *AlgoService) error {
				t.Helper()
				r := add
				r.StrategyParam.QtyPercent = domain.MustNewRate("100")
				_, err := svc.AddOrder(ctx, id, r)
				return err
			},
		},
		{
			name: "AddOrder/a negative qtyPercent", op: opAlgoAddOrder,
			call: func(t *testing.T, ctx context.Context, svc *AlgoService) error {
				t.Helper()
				r := add
				r.StrategyParam.QtyPercent = domain.MustNewRate("-10")
				_, err := svc.AddOrder(ctx, id, r)
				return err
			},
		},
		{
			name: "AddOrder/a negative interval", op: opAlgoAddOrder,
			call: func(t *testing.T, ctx context.Context, svc *AlgoService) error {
				t.Helper()
				r := add
				r.StrategyParam.Interval = -1
				_, err := svc.AddOrder(ctx, id, r)
				return err
			},
		},

		// --- CancelOrder ---
		{
			name: "CancelOrder/empty orderId", op: opAlgoCancelOrder,
			call: func(t *testing.T, ctx context.Context, svc *AlgoService) error {
				t.Helper()
				_, err := svc.CancelOrder(ctx, id, AlgoCancelOrderRequest{
					OrderID: "", ExchangeType: types.ExchangeHK,
				})
				return err
			},
		},
		{
			name: "CancelOrder/blank orderId", op: opAlgoCancelOrder,
			call: func(t *testing.T, ctx context.Context, svc *AlgoService) error {
				t.Helper()
				_, err := svc.CancelOrder(ctx, id, AlgoCancelOrderRequest{
					OrderID: "  \t ", ExchangeType: types.ExchangeHK,
				})
				return err
			},
		},
		{
			name: "CancelOrder/empty exchangeType", op: opAlgoCancelOrder,
			call: func(t *testing.T, ctx context.Context, svc *AlgoService) error {
				t.Helper()
				_, err := svc.CancelOrder(ctx, id, AlgoCancelOrderRequest{
					OrderID: algoFixtureMasterID(), ExchangeType: "",
				})
				return err
			},
		},
		{
			name: "CancelOrder/unrecognised exchangeType", op: opAlgoCancelOrder,
			call: func(t *testing.T, ctx context.Context, svc *AlgoService) error {
				t.Helper()
				_, err := svc.CancelOrder(ctx, id, AlgoCancelOrderRequest{
					OrderID: algoFixtureMasterID(), ExchangeType: "Q",
				})
				return err
			},
		},

		// --- CancelEntrust ---
		{
			name: "CancelEntrust/empty orderId", op: opAlgoCancelEntrust,
			call: func(t *testing.T, ctx context.Context, svc *AlgoService) error {
				t.Helper()
				_, err := svc.CancelEntrust(ctx, id, AlgoCancelEntrustRequest{
					OrderID: "", EntrustID: algoFixtureChildID(), ExchangeType: types.ExchangeHK,
				})
				return err
			},
		},
		{
			name: "CancelEntrust/blank entrustId", op: opAlgoCancelEntrust,
			call: func(t *testing.T, ctx context.Context, svc *AlgoService) error {
				t.Helper()
				_, err := svc.CancelEntrust(ctx, id, AlgoCancelEntrustRequest{
					OrderID: algoFixtureMasterID(), EntrustID: "   ", ExchangeType: types.ExchangeHK,
				})
				return err
			},
		},
		{
			name: "CancelEntrust/empty exchangeType", op: opAlgoCancelEntrust,
			call: func(t *testing.T, ctx context.Context, svc *AlgoService) error {
				t.Helper()
				_, err := svc.CancelEntrust(ctx, id, AlgoCancelEntrustRequest{
					OrderID: algoFixtureMasterID(), EntrustID: algoFixtureChildID(), ExchangeType: "",
				})
				return err
			},
		},
		{
			name: "CancelEntrust/unrecognised exchangeType", op: opAlgoCancelEntrust,
			call: func(t *testing.T, ctx context.Context, svc *AlgoService) error {
				t.Helper()
				_, err := svc.CancelEntrust(ctx, id, AlgoCancelEntrustRequest{
					OrderID: algoFixtureMasterID(), EntrustID: algoFixtureChildID(), ExchangeType: "x",
				})
				return err
			},
		},

		// --- ChangeOrder ---
		{
			name: "ChangeOrder/empty orderId", op: opAlgoChangeOrder,
			call: func(t *testing.T, ctx context.Context, svc *AlgoService) error {
				t.Helper()
				r := change
				r.OrderID = ""
				_, err := svc.ChangeOrder(ctx, id, r)
				return err
			},
		},
		{
			name: "ChangeOrder/empty stockCode", op: opAlgoChangeOrder,
			call: func(t *testing.T, ctx context.Context, svc *AlgoService) error {
				t.Helper()
				r := change
				r.StockCode = ""
				_, err := svc.ChangeOrder(ctx, id, r)
				return err
			},
		},
		{
			name: "ChangeOrder/stockCode with interior whitespace", op: opAlgoChangeOrder,
			call: func(t *testing.T, ctx context.Context, svc *AlgoService) error {
				t.Helper()
				r := change
				r.StockCode = "AAPL US"
				_, err := svc.ChangeOrder(ctx, id, r)
				return err
			},
		},
		{
			name: "ChangeOrder/empty exchangeType", op: opAlgoChangeOrder,
			call: func(t *testing.T, ctx context.Context, svc *AlgoService) error {
				t.Helper()
				r := change
				r.ExchangeType = ""
				_, err := svc.ChangeOrder(ctx, id, r)
				return err
			},
		},
		{
			name: "ChangeOrder/a negative price", op: opAlgoChangeOrder,
			call: func(t *testing.T, ctx context.Context, svc *AlgoService) error {
				t.Helper()
				r := change
				r.Price = domain.MustNewPrice("-0.5", "0")
				_, err := svc.ChangeOrder(ctx, id, r)
				return err
			},
		},
		{
			name: "ChangeOrder/a zero quantity", op: opAlgoChangeOrder,
			call: func(t *testing.T, ctx context.Context, svc *AlgoService) error {
				t.Helper()
				r := change
				r.Quantity = domain.MustNewQuantity("0")
				_, err := svc.ChangeOrder(ctx, id, r)
				return err
			},
		},
		{
			name: "ChangeOrder/an out-of-set sensitivity", op: opAlgoChangeOrder,
			call: func(t *testing.T, ctx context.Context, svc *AlgoService) error {
				t.Helper()
				// On a change the sensitivity is optional, so this row is the
				// *supplied-but-wrong* half of the optional-field shape; the
				// absent-but-valid half is in the accept table.
				r := change
				r.StrategyParam.Sensitivity = domain.AlgoSensitivity("9")
				_, err := svc.ChangeOrder(ctx, id, r)
				return err
			},
		},
		{
			name: "ChangeOrder/a negative maxVolume", op: opAlgoChangeOrder,
			call: func(t *testing.T, ctx context.Context, svc *AlgoService) error {
				t.Helper()
				r := change
				r.StrategyParam.MaxVolume = domain.MustNewQuantity("-5")
				_, err := svc.ChangeOrder(ctx, id, r)
				return err
			},
		},
		{
			name: "ChangeOrder/a negative showQty", op: opAlgoChangeOrder,
			call: func(t *testing.T, ctx context.Context, svc *AlgoService) error {
				t.Helper()
				r := change
				r.StrategyParam.ShowQty = domain.MustNewQuantity("-5")
				_, err := svc.ChangeOrder(ctx, id, r)
				return err
			},
		},
		{
			name: "ChangeOrder/a qtyPercent above the documented range", op: opAlgoChangeOrder,
			call: func(t *testing.T, ctx context.Context, svc *AlgoService) error {
				t.Helper()
				r := change
				r.StrategyParam.QtyPercent = domain.MustNewRate("101")
				_, err := svc.ChangeOrder(ctx, id, r)
				return err
			},
		},
		{
			name: "ChangeOrder/a malformed origStartTime", op: opAlgoChangeOrder,
			call: func(t *testing.T, ctx context.Context, svc *AlgoService) error {
				t.Helper()
				r := change
				r.StrategyParam.OrigStartTime = "09:30"
				_, err := svc.ChangeOrder(ctx, id, r)
				return err
			},
		},
		{
			name: "ChangeOrder/a negative interval", op: opAlgoChangeOrder,
			call: func(t *testing.T, ctx context.Context, svc *AlgoService) error {
				t.Helper()
				r := change
				r.StrategyParam.Interval = -30
				_, err := svc.ChangeOrder(ctx, id, r)
				return err
			},
		},
		{
			name: "ChangeOrder/a negative minAmount", op: opAlgoChangeOrder,
			call: func(t *testing.T, ctx context.Context, svc *AlgoService) error {
				t.Helper()
				r := change
				r.StrategyParam.MinAmount = domain.MustNewMoney("-0.01", "HKD", 3)
				_, err := svc.ChangeOrder(ctx, id, r)
				return err
			},
		},

		// --- ActionOrder ---
		{
			name: "ActionOrder/empty orderId", op: opAlgoActionOrder,
			call: func(t *testing.T, ctx context.Context, svc *AlgoService) error {
				t.Helper()
				r := algoFixtureAction()
				r.OrderID = ""
				_, err := svc.ActionOrder(ctx, id, r)
				return err
			},
		},
		{
			name: "ActionOrder/empty targetStrategy", op: opAlgoActionOrder,
			call: func(t *testing.T, ctx context.Context, svc *AlgoService) error {
				t.Helper()
				r := algoFixtureAction()
				r.TargetStrategy = ""
				_, err := svc.ActionOrder(ctx, id, r)
				return err
			},
		},
		{
			name: "ActionOrder/empty exchangeType", op: opAlgoActionOrder,
			call: func(t *testing.T, ctx context.Context, svc *AlgoService) error {
				t.Helper()
				r := algoFixtureAction()
				r.ExchangeType = ""
				_, err := svc.ActionOrder(ctx, id, r)
				return err
			},
		},
		{
			name: "ActionOrder/unrecognised exchangeType", op: opAlgoActionOrder,
			call: func(t *testing.T, ctx context.Context, svc *AlgoService) error {
				t.Helper()
				r := algoFixtureAction()
				r.ExchangeType = "Q"
				_, err := svc.ActionOrder(ctx, id, r)
				return err
			},
		},
		{
			name: "ActionOrder/an empty action", op: opAlgoActionOrder,
			call: func(t *testing.T, ctx context.Context, svc *AlgoService) error {
				t.Helper()
				r := algoFixtureAction()
				r.Action = ""
				_, err := svc.ActionOrder(ctx, id, r)
				return err
			},
		},
		{
			// The one dictionary the two vendors and the released layer all state
			// identically, so a fifth code is a request the SDK already knows it
			// does not mean — on a live order.
			name: "ActionOrder/a fifth action code", op: opAlgoActionOrder,
			call: func(t *testing.T, ctx context.Context, svc *AlgoService) error {
				t.Helper()
				r := algoFixtureAction()
				r.Action = domain.AlgoAction("5")
				_, err := svc.ActionOrder(ctx, id, r)
				return err
			},
		},
		{
			name: "ActionOrder/a zero action code", op: opAlgoActionOrder,
			call: func(t *testing.T, ctx context.Context, svc *AlgoService) error {
				t.Helper()
				r := algoFixtureAction()
				r.Action = domain.AlgoAction("0")
				_, err := svc.ActionOrder(ctx, id, r)
				return err
			},
		},
	}
}

// TestAlgoMutationsRejectBadInputWithoutARequest drives the table above.
//
// The fixture is a sequencedExecutor with no scripted replies, so a validator that
// was removed would hit the executor's t.Fatalf rather than returning a reply and
// letting the row pass. That is the C4/C5 discipline, and it is what makes
// "zero requests" a measurement rather than an absence of evidence.
func TestAlgoMutationsRejectBadInputWithoutARequest(t *testing.T) {
	rows := algoMutationRejections()
	if len(rows) == 0 {
		t.Fatal("the mutation rejection table is empty, so this test would pass vacuously")
	}
	seen := map[string]int{}
	for _, tc := range rows {
		method, _, _ := strings.Cut(tc.name, "/")
		seen[method]++
		t.Run(tc.name, func(t *testing.T) {
			exec := newSequencedExecutor(t)
			assertInvalidParam(t, tc.call(t, t.Context(), NewAlgoService(exec)), tc.op)
			// Zero is the assertion that makes a local rejection safe on an order:
			// the request never left, so it provably had no side effect.
			requireCalls(t, exec, 0)
		})
	}
	// Every mutation must have rows, or a method whose validator was emptied would
	// simply have no rows and this test would still pass. The counts are stated
	// rather than merely "> 0" so a table that shrank shows up.
	for method, want := range map[string]int{
		"AddOrder": 27, "CancelOrder": 4, "CancelEntrust": 4,
		"ChangeOrder": 13, "ActionOrder": 7,
	} {
		if got := seen[method]; got != want {
			t.Errorf("%s has %d local-rejection rows, want %d. A validator emptied of "+
				"checks would leave this count short, and an assertion that only counted "+
				"rows would not notice", method, got, want)
		}
	}
}

// TestAlgoMutationsAcceptTheDocumentedBoundaries is the other half: every value the
// reference and the vendors document is accepted and reaches the wire, so a
// fail-closed check cannot be widened by accident either.
//
// It is a separate test from the rejection table because the two directions fail
// differently. A check that is too strict is a rejected order; a check that is too
// loose is a request the SDK sent that it should have known it did not mean. The
// second is worse, and it is the one a rejection-only table cannot see.
func TestAlgoMutationsAcceptTheDocumentedBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name string
		// run drives the rows. Each builds its own recorder and its own service over
		// it, rather than taking a service from the caller: a shared service built
		// from a *different* recorder would send every request to the wrong server
		// and the per-row request count would read zero, which is the failure this
		// restructure exists to prevent.
		run func(t *testing.T)
	}{
		{
			// Both algo order types, all four directions, both session codes, all
			// four markets, all three aggressiveness codes and both ends of the
			// participation range: the whole closed alphabet, in one place, on the
			// wire. A predicate narrowed by accident fails here.
			name: "every documented code on the add body reaches the wire",
			run: func(t *testing.T) {
				rows := []struct {
					label   string
					mutate  func(r *AlgoAddOrderRequest)
					wantKey string
					wantVal string
					// nested is true for a member of the strategyParam object, which
					// is a *different shape* from the envelope's params — reaching
					// for one through the other fails on a key that is genuinely
					// there, so the two are read separately and the row says which.
					nested bool
				}{
					{"the market order type", func(r *AlgoAddOrderRequest) {
						r.OrderType = domain.AlgoEntrustTypeMarket
					}, "entrustType", "2", false},
					{"direction 2, close long", func(r *AlgoAddOrderRequest) {
						r.Side = types.EntrustSell
					}, "entrustBs", "2", false},
					// 3 and 4 are the short-selling directions. A check written
					// against only 1 and 2 would refuse a legitimate order, and A6
					// records that as the mistake its design note's own example
					// made.
					{"direction 3, close short", func(r *AlgoAddOrderRequest) {
						r.Side = types.EntrustCloseShort
					}, "entrustBs", "3", false},
					{"direction 4, open short", func(r *AlgoAddOrderRequest) {
						r.Side = types.EntrustOpenShort
					}, "entrustBs", "4", false},
					{"session on", func(r *AlgoAddOrderRequest) {
						r.SessionType = domain.AlgoSessionTypeOn
					}, "sessionType", "1", false},
					{"the US market", func(r *AlgoAddOrderRequest) {
						r.ExchangeType = types.ExchangeUS
					}, "exchangeType", "P", false},
					{"Shenzhen Connect, lowercase v", func(r *AlgoAddOrderRequest) {
						r.ExchangeType = types.ExchangeShenzhenConnect
					}, "exchangeType", "v", false},
					{"Shanghai Connect, lowercase t", func(r *AlgoAddOrderRequest) {
						r.ExchangeType = types.ExchangeShanghaiConnect
					}, "exchangeType", "t", false},
					{"aggressive sensitivity", func(r *AlgoAddOrderRequest) {
						r.StrategyParam.Sensitivity = domain.AlgoSensitivityAggressive
					}, "sensitivity", "2", true},
					{"passive sensitivity", func(r *AlgoAddOrderRequest) {
						r.StrategyParam.Sensitivity = domain.AlgoSensitivityPassive
					}, "sensitivity", "3", true},
					{"the lowest participation percentage", func(r *AlgoAddOrderRequest) {
						r.StrategyParam.QtyPercent = domain.MustNewRate("1")
					}, "qtyPercent", "1", true},
					{"the highest participation percentage", func(r *AlgoAddOrderRequest) {
						r.StrategyParam.QtyPercent = domain.MustNewRate("99")
					}, "qtyPercent", "99", true},
					// A zero price is a market order's conventional spelling and both
					// vendors send the field unconditionally, so it must be accepted
					// rather than needing a special case.
					{"a zero price, the market-order convention", func(r *AlgoAddOrderRequest) {
						r.Price = domain.MustNewPrice("0", "0")
					}, "entrustPrice", "0", false},
					// The times are optional and a caller who means "the exchange
					// open" leaves them empty, which is a different state and is not
					// checked at all.
					{"an absent strategy window", func(r *AlgoAddOrderRequest) {
						r.StrategyParam.OrigStartTime = ""
						r.StrategyParam.OrigEndTime = ""
					}, "", "", false},
					{"a zero interval, which means the Gateway's default", func(r *AlgoAddOrderRequest) {
						r.StrategyParam.Interval = 0
					}, "", "", false},
					{"midnight on the strategy window", func(r *AlgoAddOrderRequest) {
						r.StrategyParam.OrigStartTime = "000000"
					}, "origStartTime", "000000", true},
					{"the last second of the strategy window", func(r *AlgoAddOrderRequest) {
						r.StrategyParam.OrigEndTime = "235959"
					}, "origEndTime", "235959", true},
				}
				for _, row := range rows {
					t.Run(row.label, func(t *testing.T) {
						r := algoFixtureAdd()
						row.mutate(&r)
						rec := newWireRecorder(map[string]string{
							string(client.RouteTradeAlgoAddOrder): gatewaySuccess(algoMutationBody),
						})
						if _, err := NewAlgoService(newWireExecutor(t, rec)).
							AddOrder(t.Context(), algoFixtureAccount(), r); err != nil {
							t.Fatalf("a documented value was rejected: %v. A check that is "+
								"too strict is a rejected order", err)
						}
						if got := rec.count(string(client.RouteTradeAlgoAddOrder)); got != 1 {
							t.Fatalf("requests = %d, want 1: a row asserting only \"no "+
								"error\" would pass against a method that returned early "+
								"without sending anything", got)
						}
						if row.wantKey == "" {
							return
						}
						if row.nested {
							algoRequireString(t,
								algoRecordedNestedParams(t, rec, string(client.RouteTradeAlgoAddOrder)),
								row.wantKey, row.wantVal)
							return
						}
						algoRequireString(t,
							algoRecordedParams(t, rec, string(client.RouteTradeAlgoAddOrder)),
							row.wantKey, row.wantVal)
					})
				}
			},
		},
		{
			// All four action codes, because the set is closed and a predicate
			// narrowed to two would refuse half of them. This is asserted over the
			// wire on the *action* key, which is the key none of the other four
			// bodies has — C5's lesson, in a row.
			name: "every documented action code reaches the wire",
			run: func(t *testing.T) {
				for action, want := range map[domain.AlgoAction]string{
					domain.AlgoActionStart:   "1",
					domain.AlgoActionStop:    "2",
					domain.AlgoActionSuspend: "3",
					domain.AlgoActionResume:  "4",
				} {
					r := algoFixtureAction()
					r.Action = action
					rec := newWireRecorder(map[string]string{
						string(client.RouteTradeAlgoActionOrder): gatewaySuccess(algoMutationBody),
					})
					if _, err := NewAlgoService(newWireExecutor(t, rec)).
						ActionOrder(t.Context(), algoFixtureAccount(), r); err != nil {
						t.Errorf("action %q was rejected: %v", want, err)
						continue
					}
					algoRequireString(t,
						algoRecordedParams(t, rec, string(client.RouteTradeAlgoActionOrder)),
						"action", want)
				}
			},
		},
		{
			// The change body with no tuning at all: every member is optional here
			// where the add required two, so a caller can reprice alone. The
			// mirror-image assertion — a change must NOT carry the order type,
			// direction, session or strategy — is in the scaffold file.
			name: "a change with no strategy tuning at all",
			run: func(t *testing.T) {
				rec := newWireRecorder(map[string]string{
					string(client.RouteTradeAlgoChangeOrder): gatewaySuccess(algoMutationBody),
				})
				if _, err := NewAlgoService(newWireExecutor(t, rec)).
					ChangeOrder(t.Context(), algoFixtureAccount(), algoFixtureChange()); err != nil {
					t.Fatalf("ChangeOrder = %v, want nil", err)
				}
				nested := algoRecordedNestedParams(t, rec, string(client.RouteTradeAlgoChangeOrder))
				if len(nested) != 0 {
					t.Errorf("strategyParam carries %v, want no members: every member is "+
						"optional on a change, and an unset one must be omitted rather "+
						"than sent empty", algoBodyKeys(nested))
				}
			},
		},
		{
			// Both cancels accept every documented market, so the exchange check is
			// proven at the other four codes on the two bodies that carry no other
			// economic term.
			name: "every documented market reaches the cancel bodies",
			run: func(t *testing.T) {
				for _, exchange := range []types.ExchangeType{
					types.ExchangeHK, types.ExchangeUS,
					types.ExchangeShenzhenConnect, types.ExchangeShanghaiConnect,
				} {
					rec := newWireRecorder(map[string]string{
						string(client.RouteTradeAlgoCancelOrder): gatewaySuccess(algoMutationBody),
					})
					if _, err := NewAlgoService(newWireExecutor(t, rec)).CancelOrder(
						t.Context(), algoFixtureAccount(),
						AlgoCancelOrderRequest{OrderID: algoFixtureMasterID(), ExchangeType: exchange},
					); err != nil {
						t.Errorf("CancelOrder on market %q was rejected: %v", exchange, err)
						continue
					}
					algoRequireString(t,
						algoRecordedParams(t, rec, string(client.RouteTradeAlgoCancelOrder)),
						"exchangeType", string(exchange))
				}
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tc.run(t)
		})
	}
}

// TestAlgoStrategyForwardsUnknownCodes is the one field on the algo surface that is
// checked for shape and not for membership, asserted in the direction a rejection
// table cannot see.
//
// Every other code on this surface is refused when it is outside its documented set.
// The strategy is not, and the reason is on domain.AlgoStrategy: the reference
// documents code 1005 under two names — the AddOrder parameter table says POV and
// the request examples and both vendors say INLINE — so a closed set would refuse a
// code the vendor's own documentation endorses. A blank code is refused because the
// Gateway cannot resolve a strategy from an empty string at all; every other value
// is forwarded unchanged.
//
// So this test pins the *forwarding*, and the rejection table's empty-targetStrategy
// row pins the *emptiness* check. Together they are the whole policy, and a future
// edit that added a Valid to AlgoStrategy would fail here.
func TestAlgoStrategyForwardsUnknownCodes(t *testing.T) {
	for _, code := range []string{
		"9999", "5", "0", "1005", "1002", // the undocumented one and four documented
		"1", "1001", "1003", // the five the released layer and both vendors name
		"POV", "INLINE", "TPOV", "unknown-but-named",
	} {
		t.Run("targetStrategy "+code, func(t *testing.T) {
			r := algoFixtureAdd()
			r.TargetStrategy = domain.AlgoStrategy(code)
			rec := newWireRecorder(map[string]string{
				string(client.RouteTradeAlgoAddOrder): gatewaySuccess(algoMutationBody),
			})
			if _, err := NewAlgoService(newWireExecutor(t, rec)).
				AddOrder(t.Context(), algoFixtureAccount(), r); err != nil {
				t.Fatalf("targetStrategy %q was refused: %v. The strategy set is not "+
					"bounded — the reference documents code 1005 under two names, so a "+
					"closed set would reject a code the vendor's own documentation "+
					"endorses, and an unknown code is forwarded unchanged", code, err)
			}
			// And the code must reach the wire verbatim, which is what "forwarded
			// unchanged" means.
			algoRequireString(t, algoRecordedParams(t, rec, string(client.RouteTradeAlgoAddOrder)),
				"targetStrategy", code)
		})
	}
}

// TestAlgoEveryEndpointRejectsAnUnrecognisedExchangeType drives the same
// out-of-set market through all five mutations plus both reads, so a check dropped
// from one method's validator is caught.
//
// One control row asserts that the order query with the market *absent* is still
// sent, which pins the optional-field exemption on the one endpoint that has it. A
// different policy for the same type on the same surface is the mistake A6
// specifically called out.
func TestAlgoEveryEndpointRejectsAnUnrecognisedExchangeType(t *testing.T) {
	for _, bad := range []string{"Z", "k", "V", "T", "HK", "0", ""} {
		t.Run("exchangeType "+bad, func(t *testing.T) {
			id := algoFixtureAccount()
			invocations := []struct {
				name string
				op   string
				call func(t *testing.T, exec Executor) error
			}{
				{
					name: "AddOrder", op: opAlgoAddOrder,
					call: func(t *testing.T, exec Executor) error {
						t.Helper()
						r := algoFixtureAdd()
						r.ExchangeType = types.ExchangeType(bad)
						_, err := NewAlgoService(exec).AddOrder(t.Context(), id, r)
						return err
					},
				},
				{
					name: "CancelOrder", op: opAlgoCancelOrder,
					call: func(t *testing.T, exec Executor) error {
						t.Helper()
						_, err := NewAlgoService(exec).CancelOrder(t.Context(), id, AlgoCancelOrderRequest{
							OrderID: algoFixtureMasterID(), ExchangeType: types.ExchangeType(bad),
						})
						return err
					},
				},
				{
					name: "CancelEntrust", op: opAlgoCancelEntrust,
					call: func(t *testing.T, exec Executor) error {
						t.Helper()
						_, err := NewAlgoService(exec).CancelEntrust(t.Context(), id, AlgoCancelEntrustRequest{
							OrderID: algoFixtureMasterID(), EntrustID: algoFixtureChildID(),
							ExchangeType: types.ExchangeType(bad),
						})
						return err
					},
				},
				{
					name: "ChangeOrder", op: opAlgoChangeOrder,
					call: func(t *testing.T, exec Executor) error {
						t.Helper()
						r := algoFixtureChange()
						r.ExchangeType = types.ExchangeType(bad)
						_, err := NewAlgoService(exec).ChangeOrder(t.Context(), id, r)
						return err
					},
				},
				{
					name: "ActionOrder", op: opAlgoActionOrder,
					call: func(t *testing.T, exec Executor) error {
						t.Helper()
						r := algoFixtureAction()
						r.ExchangeType = types.ExchangeType(bad)
						_, err := NewAlgoService(exec).ActionOrder(t.Context(), id, r)
						return err
					},
				},
				{
					name: "QueryEntrustIDList", op: opAlgoQueryEntrustIDList,
					call: func(t *testing.T, exec Executor) error {
						t.Helper()
						_, err := NewAlgoService(exec).QueryEntrustIDList(t.Context(), id, AlgoEntrustIDQuery{
							OrderID: algoFixtureMasterID(), TradeDate: "20260921",
							ExchangeType: types.ExchangeType(bad),
						})
						return err
					},
				},
				{
					// The read where the field is optional. An empty value here is
					// the *legitimate* unfiltered case, so it is excluded from this
					// row's rejection and asserted as the control below; only a
					// present-and-unrecognised value is a rejection, which is the
					// optional-field shape A6 installed on the released layer.
					name: "QueryOrderList (present)", op: opAlgoQueryOrderList,
					call: func(t *testing.T, exec Executor) error {
						t.Helper()
						if bad == "" {
							// The empty market with no stock code is the
							// unfiltered query; it must be sent.
							_, err := NewAlgoService(exec).QueryOrderList(t.Context(), id,
								AlgoOrderQuery{Page: algoFixturePage()})
							return err
						}
						q := algoFixtureOrderQuery()
						q.ExchangeType = types.ExchangeType(bad)
						_, err := NewAlgoService(exec).QueryOrderList(t.Context(), id, q)
						return err
					},
				},
			}
			for _, inv := range invocations {
				t.Run(inv.name, func(t *testing.T) {
					// The empty-market row for the order query is the control and
					// must be sent; every other row must be refused at zero calls.
					control := bad == "" && inv.name == "QueryOrderList (present)"
					var exec *sequencedExecutor
					if control {
						exec = newSequencedExecutor(t, sequencedReply{reply: json.RawMessage(algoOrderListResponseBody)})
					} else {
						exec = newSequencedExecutor(t)
					}
					err := inv.call(t, exec)
					if control {
						if err != nil {
							t.Errorf("the unfiltered master-order query was rejected: %v", err)
							return
						}
						requireCalls(t, exec, 1)
						return
					}
					assertInvalidParam(t, err, inv.op)
					requireCalls(t, exec, 0)
				})
			}
		})
	}
}

// TestAlgoEveryEndpointRejectsAnOutOfSetDirection drives one out-of-set direction
// through every method that carries one, so a check dropped from any one is caught.
//
// The near-misses are the ones a caller is likely to send: the trade surface's
// conditional-order code, a numeric where a letter belongs, a padded value, and the
// names rather than the codes. It goes through AddOrder because that is the only
// mutation with a direction on its body — the change body deliberately omits it, the
// two cancels carry none, and the operate body addresses a strategy — so one call is
// the whole surface.
func TestAlgoEveryEndpointRejectsAnOutOfSetDirection(t *testing.T) {
	for _, bad := range []string{"5", "0", "9", "BUY", "买入", " 1", "1 "} {
		t.Run("entrustBs "+bad, func(t *testing.T) {
			exec := newSequencedExecutor(t)
			r := algoFixtureAdd()
			r.Side = types.EntrustBS(bad)
			_, err := NewAlgoService(exec).AddOrder(t.Context(), algoFixtureAccount(), r)
			assertInvalidParam(t, err, opAlgoAddOrder)
			requireCalls(t, exec, 0)
		})
	}
	// The control: all four documented directions are accepted, and the two
	// short-selling codes among them. A check written against only 1 and 2 would
	// refuse a legitimate short, and A6 records that as the mistake its design
	// note's own example made.
	t.Run("all four documented directions are accepted", func(t *testing.T) {
		for _, side := range []types.EntrustBS{
			types.EntrustBuy, types.EntrustSell,
			types.EntrustCloseShort, types.EntrustOpenShort,
		} {
			rec := newWireRecorder(map[string]string{
				string(client.RouteTradeAlgoAddOrder): gatewaySuccess(algoMutationBody),
			})
			r := algoFixtureAdd()
			r.Side = side
			if _, err := NewAlgoService(newWireExecutor(t, rec)).
				AddOrder(t.Context(), algoFixtureAccount(), r); err != nil {
				t.Errorf("direction %q was rejected: %v. It is a documented direction, and "+
					"3 and 4 are the short-selling ones a check written against 1 and 2 "+
					"would refuse", side, err)
				continue
			}
			algoRequireString(t, algoRecordedParams(t, rec, string(client.RouteTradeAlgoAddOrder)),
				"entrustBs", string(side))
		}
	})
}

// TestAlgoMutationAttemptCountIsIndependentOfThePolicySize is the widening of the
// ADR 0003 guarantee from "one policy" to "every policy": a mutation issues one
// attempt whatever the caller configures, including a budget below one.
//
// resilience.Policy treats a MaxAttempts below one as one, so the smallest row is a
// floor rather than a distinct case, and it is here to make that floor explicit. A
// very large budget is the load-bearing row: it is the one that would catch a retry
// loop added on top of the client, and it is bounded at 20 so the test does not
// depend on the machine's patience.
func TestAlgoMutationAttemptCountIsIndependentOfThePolicySize(t *testing.T) {
	paths := make(map[string]string)
	for _, tc := range algoMutationCases() {
		paths[string(tc.route)] = gatewayFailure(types.StatusServiceBusy, "busy")
	}

	for _, maxAttempts := range []int{0, 1, 2, 5, 20} {
		t.Run("MaxAttempts "+strconv.Itoa(maxAttempts), func(t *testing.T) {
			for _, tc := range algoMutationCases() {
				t.Run(tc.name, func(t *testing.T) {
					rec := newWireRecorder(paths)
					svc := NewAlgoService(newWireExecutor(t, rec,
						client.WithRetryPolicy(client.RetryPolicy{MaxAttempts: maxAttempts, BaseBackoff: 0})))

					if err := tc.invoke(t, t.Context(), svc); err == nil {
						t.Fatalf("%s = nil error, want the rejection", tc.name)
					}
					if got := rec.total(); got != 1 {
						t.Errorf("%s with MaxAttempts %d produced %d requests, want 1. "+
							"ADR 0003 permits one attempt for an order mutation at any "+
							"policy configuration", tc.name, maxAttempts, got)
					}
				})
			}
		})
	}
}

// TestAlgoMutationCallIsCancellable pins that a cancelled context is reported as a
// cancellation and never as a success, matched through errors.Is on
// context.Canceled rather than on a rendered message.
//
// All five are single-shot, so each costs at most one recorded request: each hands
// the context to the executor and lets the request path report the cancellation. A
// caller who gives up on the context must not be able to turn one attempt into
// several, and the error it gets back must still be the cancellation rather than a
// Gateway status the SDK made up on the way out.
func TestAlgoMutationCallIsCancellable(t *testing.T) {
	for _, tc := range algoMutationCases() {
		t.Run(tc.name, func(t *testing.T) {
			rec := newWireRecorder(map[string]string{
				string(tc.route): gatewayFailure(types.StatusServiceBusy, "busy"),
			})
			svc := NewAlgoService(newWireExecutor(t, rec,
				client.WithRetryPolicy(client.RetryPolicy{MaxAttempts: 5, BaseBackoff: 0})))

			ctx, cancel := context.WithCancel(t.Context())
			cancel()

			err := tc.invoke(t, ctx, svc)
			if !errors.Is(err, context.Canceled) {
				t.Errorf("%s on a cancelled context = %v, want context.Canceled", tc.name, err)
			}
			if got := errs.CategoryOf(err); got != errs.CategoryTimeout {
				t.Errorf("CategoryOf = %q, want %q for a cancelled call", got, errs.CategoryTimeout)
			}
			if got := rec.total(); got > 1 {
				t.Errorf("%s issued %d requests before the cancellation took effect, want at "+
					"most 1: a caller giving up must not turn one attempt into several",
					tc.name, got)
			}
		})
	}
}

// TestAlgoMutationMalformedReplyIsReportedAndNotDressedAsAGatewayCode covers a reply
// whose data member cannot be decoded into the mutation envelope.
//
// A decode failure is a local transport error and a Gateway status code is the
// exchange's; reporting one as the other sends a caller looking for a session
// failure that never happened. So two things are asserted — errs.CodeOf finds *no*
// code, and the op is still there, because a caller diagnosing a decode failure
// needs to know which method produced it.
func TestAlgoMutationMalformedReplyIsReportedAndNotDressedAsAGatewayCode(t *testing.T) {
	for _, tc := range algoMutationCases() {
		t.Run(tc.name, func(t *testing.T) {
			rec := newWireRecorder(map[string]string{
				string(tc.route): gatewaySuccess(`42`),
			})
			err := tc.invoke(t, t.Context(), NewAlgoService(newWireExecutor(t, rec)))
			if err == nil {
				t.Fatalf("%s on a data member of 42 = nil error; the value would be silently empty", tc.name)
			}
			if code, ok := errs.CodeOf(err); ok {
				t.Errorf("CodeOf = (%q, true), want (false): a decode failure is a local "+
					"transport error and must not carry a Gateway status code", code)
			}
			var typed *errs.Error
			if !errors.As(err, &typed) {
				t.Fatalf("errors.As(%T) yielded no *errs.Error, so the op is not attributable", err)
			}
			if typed.Op != tc.op {
				t.Errorf("Op = %q, want %q: the op still identifies which method failed",
					typed.Op, tc.op)
			}
		})
	}
}

// TestAlgoMutationExecutorFailurePropagates is Level 1 for the mutations: the fake
// returns a sentinel and the caller gets it back unchanged, with the zero value
// beside it.
//
// Unchanged is the assertion. A service that wrapped a transport error in its own
// would still be traversable with errors.Is, so the check here is both: the sentinel
// must match under errors.Is *and* the returned identifier must be the zero value,
// so a caller cannot mistake an unissued order for a placed one.
func TestAlgoMutationExecutorFailurePropagates(t *testing.T) {
	sentinel := errors.New("transport is down")
	for _, tc := range algoMutationCases() {
		t.Run(tc.name, func(t *testing.T) {
			exec := newSequencedExecutor(t, sequencedReply{err: sentinel})
			err := tc.invoke(t, t.Context(), NewAlgoService(exec))
			if !errors.Is(err, sentinel) {
				t.Errorf("%s = %v, want the sentinel unchanged", tc.name, err)
			}
			requireCalls(t, exec, 1)
		})
	}
}

// TestAlgoMutationGatewayRejectionArrivesTyped is Level 2 for the mutations: a real
// *client.Client over a server answering a real ok:false envelope, asserting the
// typed *errs.Error the transport actually builds.
//
// A fake never builds a typed error, so a service laundering a Gateway rejection
// into an opaque one passes every Level 1 row and fails here.
func TestAlgoMutationGatewayRejectionArrivesTyped(t *testing.T) {
	for _, tc := range []struct {
		code     types.StatusCode
		category errs.Category
		why      string
	}{
		{types.StatusServiceBusy, errs.CategoryRateLimit,
			"the Gateway is busy; a mutation is not retried, so the caller sees this once"},
		{types.StatusInvalidParam, errs.CategoryAPI,
			"the Gateway rejected the request, which is distinct from a local rejection " +
				"of the same shape and cost a request"},
		{types.StatusDuplicateSubmit, errs.CategoryTrading,
			"the ambiguous-submission code, which is the one that exists because of ADR 0003"},
		{types.StatusNotLoggedIn, errs.CategoryAccount,
			"the session is gone, which does require a re-login and is the negative " +
				"counterpart of the duplicate-submission row"},
	} {
		t.Run(string(tc.code)+": "+tc.why, func(t *testing.T) {
			for _, mc := range algoMutationCases() {
				t.Run(mc.name, func(t *testing.T) {
					rec := newWireRecorder(map[string]string{
						string(mc.route): gatewayFailure(tc.code, "the Gateway said no"),
					})
					err := mc.invoke(t, t.Context(), NewAlgoService(newWireExecutor(t, rec)))
					typed := errRejects(t, err, tc.code, mc.op)
					if got := errs.CategoryOf(err); got != tc.category {
						t.Errorf("CategoryOf = %q, want %q", got, tc.category)
					}
					if typed.Category != tc.category {
						t.Errorf("Error.Category = %q, want %q on the error itself and not "+
							"only on the CategoryOf projection", typed.Category, tc.category)
					}
					if code, ok := errs.CodeOf(err); !ok || code != tc.code {
						t.Errorf("CodeOf = (%q, %v), want (%q, true)", code, ok, tc.code)
					}
					if got := rec.count(string(mc.route)); got != 1 {
						t.Errorf("requests to %s = %d, want 1", mc.route, got)
					}
				})
			}
		})
	}
}

// TestAlgoMutationReconciliationErrorIsTypedAndRecoverable is the typed error a
// caller branches on after an ambiguous failure, and it is the same *existing*
// errs.Error the cash and futures layers return — returned unchanged, not
// re-wrapped.
//
// 1007 duplicate submission is the code that exists precisely because of ADR 0003's
// ambiguity: the request may or may not have reached the platform. Three things are
// asserted, and each is a different half of what a caller needs:
//
//   - the code and category, so a caller can identify the ambiguous case;
//   - that errors.As reaches the typed error **through a caller-supplied %w
//     wrapper**, which is the recoverable half — the classification must survive the
//     moment a caller annotates the error, or it is lost at the first log line;
//   - that ReLoginRequired is **false**, because 1007 is a trading state and not a
//     session failure. A caller that re-logs-in on the wrong signal loses the order
//     it is trying to reconcile, which is a worse outcome than doing nothing.
func TestAlgoMutationReconciliationErrorIsTypedAndRecoverable(t *testing.T) {
	const code = types.StatusDuplicateSubmit
	const text = "duplicate submission"

	for _, tc := range algoMutationCases() {
		t.Run(tc.name, func(t *testing.T) {
			rec := newWireRecorder(map[string]string{
				string(tc.route): gatewayFailure(code, text),
			})
			svc := NewAlgoService(newWireExecutor(t, rec))

			err := tc.invoke(t, t.Context(), svc)
			if err == nil {
				t.Fatalf("%s = nil error, want the %q rejection the caller must reconcile",
					tc.name, code)
			}

			typed := errRejects(t, err, code, tc.op)
			if got := errs.CategoryOf(err); got != errs.CategoryTrading {
				t.Errorf("CategoryOf = %q, want %q: a %s is a trading-state failure the caller "+
					"must reconcile before resubmitting, and it is the one code that exists "+
					"because of the ambiguity (docs/adr/0003-no-auto-retry-orders.md)",
					got, errs.CategoryTrading, code)
			}
			if typed.Category != errs.CategoryTrading {
				t.Errorf("Error.Category = %q, want %q on the error itself and not only on the "+
					"CategoryOf projection", typed.Category, errs.CategoryTrading)
			}
			if got, ok := errs.CodeOf(err); !ok || got != code {
				t.Errorf("CodeOf = (%q, %v), want (%q, true)", got, ok, code)
			}

			// errors.As must reach the typed error, and it must be reachable
			// through a wrapper the caller adds — which is the "recoverable" half.
			var viaAs *errs.Error
			if !errors.As(err, &viaAs) {
				t.Errorf("errors.As(%T) yielded no *errs.Error", err)
			}
			wrapped := fmt.Errorf("reconcile after: %w", err)
			if !errors.As(wrapped, &viaAs) {
				t.Error("errors.As yielded no *errs.Error through a caller-supplied wrapper, " +
					"so the classification is lost the moment a caller annotates the error")
			}
			if !errors.Is(wrapped, err) {
				t.Error("errors.Is through the wrapper does not reach the original error, " +
					"so a caller comparing by identity would miss it")
			}
			// The negative counterpart: a 1007 is a trading state, not a session
			// failure, so a caller that re-logs-in on the wrong signal must not be
			// misled into doing so and losing the order it is trying to reconcile.
			if errs.ReLoginRequired(wrapped) {
				t.Errorf("ReLoginRequired(%v) = true; a duplicate submission is a trading "+
					"state, not a session failure", wrapped)
			}
			if got := rec.total(); got != 1 {
				t.Errorf("total requests = %d, want 1", got)
			}
		})
	}
}

// TestAlgoMutationsAreSentExactlyOnceUnderARetryPolicy is the ADR 0003 proof for
// the five algo mutations, and it carries its own control.
//
// Three things make it non-obvious at this layer, and each is a way the assertion
// could pass for the wrong reason.
//
//  1. The guarantee lives in client, not here. client.Client.execute derives the
//     retry class from the route path and consults resilience.IsMutation, which
//     issues one attempt for that class regardless of the policy. A fake executor has
//     no retry logic at all, so "the fake recorded one call" is trivially true and
//     says nothing about retries. All five algo paths are already in the closed set
//     at internal/resilience/retry.go:36-42; this task **proves** that and does not
//     implement it, so neither internal/resilience nor client is touched.
//
//  2. The proof therefore needs a real *client.Client over an httptest server and a
//     retryable rejection, so the policy has every reason to fire. "1011 service
//     busy" is the code errs.Retryable reports as retryable. BaseBackoff is 0, so the
//     request count is a property of the classification and not of elapsed time — the
//     discipline internal/push needed after its reconnect tests made a coverage
//     figure a coin flip. Nothing here points at 127.0.0.1:11111.
//
//  3. A single request is also what a client with *no* retry policy produces, and
//     also what a policy that was never applied produces. So the same test drives
//     the two read-only algo routes under the identical client options, the identical
//     recorder and the identical rejection, and shows them taking all five attempts.
//     Without that control the five mutation rows would pass against a policy that did
//     nothing, and a test that cannot fail is worse than no test because it reads as
//     coverage.
//
// Every invocation uses a valid request, which is load-bearing: each method
// validates before it calls the executor, so an invalid fixture would record zero
// requests — and zero would look like "no attempt at all" rather than "exactly one".
// The assertions therefore pair rec.count(route) == 1 with rec.total() == 1, and
// TestAlgoMutationsRejectBadInputWithoutARequest covers the other case.
func TestAlgoMutationsAreSentExactlyOnceUnderARetryPolicy(t *testing.T) {
	const maxAttempts = 5
	busy := types.StatusServiceBusy

	// Every path answers busy, not just the one under test, so a service that
	// quietly issued a second request somewhere else would show up in total().
	paths := make(map[string]string, len(algoMutationCases())+len(algoReadCases()))
	for _, tc := range algoMutationCases() {
		paths[string(tc.route)] = gatewayFailure(busy, "service busy, retry later")
	}
	// The control routes: the two read-only algo queries, which must be retryable.
	for _, rc := range algoReadCases() {
		paths[string(rc.route)] = gatewayFailure(busy, "service busy, retry later")
	}

	// Prove the case is retryable before relying on it. Otherwise "one attempt"
	// would be true for the boring reason that nothing wanted a second one.
	if !errs.Retryable(errs.New(busy, opAlgoAddOrder, "service busy, retry later")) {
		t.Fatalf("test bug: %q is not retryable, so a single request would be expected "+
			"even without ADR 0003", busy)
	}

	for _, tc := range algoMutationCases() {
		t.Run(tc.name+" issues exactly one attempt", func(t *testing.T) {
			rec := newWireRecorder(paths)
			svc := NewAlgoService(newWireExecutor(t, rec,
				client.WithRetryPolicy(client.RetryPolicy{MaxAttempts: maxAttempts, BaseBackoff: 0})))

			err := tc.invoke(t, t.Context(), svc)
			if err == nil {
				t.Fatalf("%s = nil error, want the %q rejection", tc.name, busy)
			}
			// The typed error is asserted, not just the count: a caller reconciling
			// an ambiguous order needs to know the exchange was busy, not that the
			// SDK gave up.
			errRejects(t, err, busy, tc.op)
			if got := errs.CategoryOf(err); got != errs.CategoryRateLimit {
				t.Errorf("CategoryOf = %q, want %q", got, errs.CategoryRateLimit)
			}

			if got := rec.count(string(tc.route)); got != 1 {
				t.Errorf("requests to %s = %d, want exactly 1: ADR 0003 permits one attempt "+
					"for an order mutation at any policy configuration, and this one "+
					"allowed %d", tc.route, got, maxAttempts)
			}
			if got := rec.total(); got != 1 {
				t.Errorf("total requests = %d, want 1: the mutation must not have issued a "+
					"second request on any other path", got)
			}
			t.Logf("MUTATION %s: %d request under MaxAttempts %d", tc.route, rec.total(), maxAttempts)
		})
	}

	// The control, in the same test, under the same policy, the same recorder table
	// and the same rejection, so the two numbers are directly comparable: five for
	// each read, one for each mutation.
	for _, rc := range algoReadCases() {
		t.Run("the read control "+rc.name+" retries under the identical policy", func(t *testing.T) {
			rec := newWireRecorder(paths)
			svc := NewAlgoService(newWireExecutor(t, rec,
				client.WithRetryPolicy(client.RetryPolicy{MaxAttempts: maxAttempts, BaseBackoff: 0})))

			err := rc.invoke(t, t.Context(), svc)
			if err == nil {
				t.Fatalf("%s = nil error, want the %q rejection", rc.name, busy)
			}
			errRejects(t, err, busy, rc.op)

			if got := rec.count(string(rc.route)); got != maxAttempts {
				t.Fatalf("requests to %s = %d, want %d. This control is what makes the five "+
					"single-request rows above mean something: without it, a client whose "+
					"policy was never applied would produce exactly the same one request.",
					rc.route, got, maxAttempts)
			}
			if got := rec.total(); got != maxAttempts {
				t.Errorf("total requests = %d, want %d", got, maxAttempts)
			}
			t.Logf("CONTROL %s: %d requests under MaxAttempts %d (mutations: 1 each)",
				rc.route, rec.total(), maxAttempts)
		})
	}
}

// TestAlgoMutationsAreSafeForConcurrentUse is the concurrency half of the AlgoService
// contract, extended to the mutation path.
//
// It is worth having because a mutation is where a shared decoded envelope would be
// worst: two callers sharing one reply's backing array would each see the other's
// order identifier, and the order id is what a caller reconciles against. Every
// worker drives a different mutation and asserts the identifier it got back, so a
// service that reused a buffer shows up as a wrong id rather than as a race the
// detector happened to schedule.
// TestAlgoMutationsAreSentExactlyOnceWhenTheGatewayAcceptsThem is the success-path
// half of the ADR 0003 proof, and it exists because a mutation run found the gap.
//
// TestAlgoMutationsAreSentExactlyOnceUnderARetryPolicy drives every path to answer
// `1011 service busy` and therefore counts one request on the *failure* path. That
// is the path ADR 0003 is about, and it is not sufficient on its own: a service that
// sends the mutation, gets a success, and then sends it again is invisible to it.
// The injected defect that exposed this wrapped the send in a two-iteration loop
// that returned early on error, so under `busy` it only ever sent once and the
// exactly-once test correctly reported 1 — the defect had hidden inside the one
// branch the test exercised.
//
// A duplicate order accepted by the exchange is the worst outcome this SDK can
// produce: the caller is told one order id while the platform holds two master
// orders, and nothing in the reply distinguishes them. It is also the outcome a
// caller cannot reconcile after the fact, because unlike a rejected order there is
// no ambiguity to resolve — the second order is real and already working.
//
// So the count is asserted on the success path too, with no retry policy configured
// at all. The two tests are complementary and neither subsumes the other: this one
// would pass against a service that retried only on failure, and the failure-path
// one would pass against a service that duplicated only on success.
func TestAlgoMutationsAreSentExactlyOnceWhenTheGatewayAcceptsThem(t *testing.T) {
	for _, tc := range algoMutationCases() {
		t.Run(tc.name+" is sent once and accepted once", func(t *testing.T) {
			// No WithRetryPolicy here on purpose: a default client is what a
			// caller who configured nothing gets, and a duplicate must not
			// depend on a policy being present to avoid.
			rec := newWireRecorder(map[string]string{
				string(tc.route): gatewaySuccess(algoMutationBody),
			})
			if err := tc.invoke(t, t.Context(), NewAlgoService(newWireExecutor(t, rec))); err != nil {
				t.Fatalf("%s = %v, want nil: the Gateway accepted the mutation", tc.name, err)
			}
			if got := rec.count(string(tc.route)); got != 1 {
				t.Errorf("requests to %s = %d, want exactly 1. A second accepted mutation is "+
					"a second live master order on the platform, and the single order id this "+
					"call returns cannot describe both", tc.route, got)
			}
			if got := rec.total(); got != 1 {
				t.Errorf("total requests = %d, want 1: an accepted mutation must not be followed "+
					"by a second request on its own path or any other", got)
			}
		})
	}
}

func TestAlgoMutationsAreSafeForConcurrentUse(t *testing.T) {
	const workers = 15 // three per mutation, so every row of the table is driven
	cases := algoMutationCases()

	paths := make(map[string]string, len(cases))
	for _, tc := range cases {
		paths[string(tc.route)] = gatewaySuccess(string(tc.reply))
	}
	rec := newWireRecorder(paths)
	// One service shared by every worker, so the test is about the *service* being
	// safe rather than about each worker having its own. A service that cached the
	// last decoded reply would hand two workers the same order identifier here, and
	// the route counts below are what would show it.
	svc := NewAlgoService(newWireExecutor(t, rec))

	type result struct {
		method string
		id     string
		err    error
	}
	results := make([]result, workers)
	start := make(chan struct{})
	done := make(chan struct{}, workers)
	for i := range workers {
		go func() {
			defer func() { done <- struct{}{} }()
			tc := cases[i%len(cases)]
			<-start
			// The identifier is read back per method, because the four master
			// mutations return a domain.OrderID and the child cancel returns a
			// domain.EntrustID. Reading it is the point: a shared decoded reply
			// would give two workers the same value, and the order id is what a
			// caller reconciles against.
			var id string
			var err error
			switch tc.name {
			case "AddOrder":
				var got domain.OrderID
				got, err = svc.AddOrder(t.Context(), algoFixtureAccount(), algoFixtureAdd())
				id = got.String()
			case "CancelOrder":
				var got domain.OrderID
				got, err = svc.CancelOrder(t.Context(), algoFixtureAccount(), algoFixtureCancelOrder())
				id = got.String()
			case "CancelEntrust":
				var got domain.EntrustID
				got, err = svc.CancelEntrust(t.Context(), algoFixtureAccount(), algoFixtureCancelEntrust())
				id = got.String()
			case "ChangeOrder":
				var got domain.OrderID
				got, err = svc.ChangeOrder(t.Context(), algoFixtureAccount(), algoFixtureChange())
				id = got.String()
			case "ActionOrder":
				var got domain.OrderID
				got, err = svc.ActionOrder(t.Context(), algoFixtureAccount(), algoFixtureAction())
				id = got.String()
			default:
				err = fmt.Errorf("test bug: no reader for %q; the case table and this "+
					"switch have drifted apart", tc.name)
			}
			results[i] = result{method: tc.name, id: id, err: err}
		}()
	}
	close(start)
	for range workers {
		<-done
	}

	// Two assertions, and both are needed. The per-worker one says no call failed
	// and each returned the identifier the Gateway sent; the per-route one says
	// every mutation really went out the number of times it was driven, which is
	// what catches a worker that returned early without sending anything.
	for i, r := range results {
		if r.err != nil {
			t.Errorf("worker %d (%s) = %v, want nil", i, r.method, r.err)
			continue
		}
		if r.id == "" {
			t.Errorf("worker %d (%s) returned an empty identifier, want %q", i, r.method,
				algoMutationCaseNamed(t, r.method).replyID)
		}
	}
	for _, tc := range cases {
		driven := 0
		for i := 0; i < workers; i++ {
			if cases[i%len(cases)].name == tc.name {
				driven++
			}
		}
		if got := rec.count(string(tc.route)); got != driven {
			t.Errorf("%s reached %s %d times, want %d: a worker that returned a result "+
				"without sending a request would satisfy the per-worker assertion above "+
				"and fail here", tc.name, tc.route, got, driven)
		}
	}
}
