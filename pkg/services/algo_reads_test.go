// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package services

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/shing1211/hstongapi4go/client"
	"github.com/shing1211/hstongapi4go/internal/errs"
	"github.com/shing1211/hstongapi4go/pkg/domain"
	"github.com/shing1211/hstongapi4go/pkg/types"
)

// This file drives the two algo reads: the positive path with the exact request
// body, the local rejections at zero recorded calls, and the error arms at both
// levels.
//
// Four levels, each reaching something the level above cannot:
//
//   - Local rejection: a zero accountID or a bad input is refused before the
//     executor is consulted, so the assertion is *zero* recorded calls. That is
//     what distinguishes a local rejection from a Gateway rejection wearing the
//     same code, and the fixture is a sequencedExecutor with **no scripted
//     replies** — a call would hit t.Fatalf rather than pass vacuously, which is
//     what C4 and C5 did and is the reason a zero-call assertion means something.
//   - Level 1 is the sequencedExecutor returning a sentinel: the error reaches the
//     caller unchanged, the zero value comes back beside it, one call is recorded.
//   - Level 2 is a real *client.Client over a server answering a real ok:false
//     envelope, asserting the typed *errs.Error the transport actually builds. A
//     fake never builds a typed error, so a service laundering a Gateway rejection
//     into an opaque one passes every Level 1 row and fails here.
//   - The retry control: a single HTTP request is also what a client with no policy
//     installed produces, so the one-request assertions in the mutation file would
//     prove nothing without it. The read rows here drive the same live policy, and
//     TestAlgoReadsRetryUnderARetryPolicy is the named half that reports the
//     contrast.
//
// The two reads are the reconciliation path for an ambiguous mutation
// (docs/adr/0003-no-auto-retry-orders.md), which is why the "no children" and "no
// masters" cases are asserted as non-nil empty slices rather than as nil: for a
// reconciliation query an empty answer is a real answer.

// ---------------------------------------------------------------------------
// Routing
// ---------------------------------------------------------------------------

// TestAlgoReadsReachTheirRouteWithTheirOp is the positive path for both: the op
// label and the route are the ones the method names, the request body is the one
// the call built, and exactly one call was recorded.
//
// It is also the control for every rejection table below. A validator that refused
// every input, or one whose check was accidentally inverted, would satisfy the
// rejection tables while making the two methods unusable; the recorded call here is
// the evidence that validation was really skipped rather than merely outvoted.
func TestAlgoReadsReachTheirRouteWithTheirOp(t *testing.T) {
	for _, tc := range algoReadCases() {
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

// TestAlgoReadsMapTheReplies is the reply side: what each method hands back for the
// mock's own payload, asserted field by field for the master order and element by
// element for the child ids.
//
// The master-order rows are the algo dictionaries arriving on a wire, and the
// dictionary assertions are the point: status "1" on this row is 部分成交 and
// 待报 on the cash surface, so a mapper that reached for types.EntrustStatus would
// still decode and would still be wrong.
func TestAlgoReadsMapTheReplies(t *testing.T) {
	t.Run("QueryOrderList", func(t *testing.T) {
		exec := newSequencedExecutor(t, sequencedReply{
			reply: json.RawMessage(algoOrderListResponseBody),
		})
		orders, err := NewAlgoService(exec).QueryOrderList(t.Context(), algoFixtureAccount(), algoFixtureOrderQuery())
		if err != nil {
			t.Fatalf("QueryOrderList = %v, want nil", err)
		}
		requireCalls(t, exec, 1)
		if len(orders) != 1 {
			t.Fatalf("orders = %d, want 1", len(orders))
		}
		order := orders[0]
		if order.OrderID != algoFixtureMasterID() {
			t.Errorf("OrderID = %q, want %q", order.OrderID, algoFixtureMasterID())
		}
		if order.StockCode != "00700.HK" {
			t.Errorf("StockCode = %q, want 00700.HK", order.StockCode)
		}
		if order.ExchangeType != types.ExchangeHK {
			t.Errorf("ExchangeType = %q, want %q", order.ExchangeType, types.ExchangeHK)
		}
		if order.EntrustType != domain.AlgoEntrustTypeLimit {
			t.Errorf("EntrustType = %q, want the algo dictionary's %q", order.EntrustType, domain.AlgoEntrustTypeLimit)
		}
		// Status "1" is 部分成交 on this surface and 待报 on the cash one, so the
		// two mappers would produce opposite answers for the same bytes.
		if order.Status != domain.AlgoStatusPartFilled {
			t.Errorf("Status = %q, want the algo dictionary's %q: the trade set reads this "+
				"same code as %q, the opposite state", order.Status,
				domain.AlgoStatusPartFilled, types.EntrustStatusWaitToRegister)
		}
		if order.EntrustBS != types.EntrustBuy {
			t.Errorf("EntrustBS = %q, want %q: this one *is* the shared dictionary", order.EntrustBS, types.EntrustBuy)
		}
		if order.TargetStrategy != domain.AlgoStrategyVWAP {
			t.Errorf("TargetStrategy = %q, want %q", order.TargetStrategy, domain.AlgoStrategyVWAP)
		}
		if order.StrategyStatus != domain.AlgoStrategyStatusStart {
			t.Errorf("StrategyStatus = %q, want %q", order.StrategyStatus, domain.AlgoStrategyStatusStart)
		}
		if order.StrategyParam.Interval != 60 {
			t.Errorf("StrategyParam.Interval = %d, want 60: the wire carries it as a bare "+
				"number while every other member is a quoted string", order.StrategyParam.Interval)
		}
		if order.StrategyParam.Sensitivity != domain.AlgoSensitivityNeutral {
			t.Errorf("StrategyParam.Sensitivity = %q, want %q", order.StrategyParam.Sensitivity, domain.AlgoSensitivityNeutral)
		}
		if got := order.EntrustPrice.String(); got != "388" {
			t.Errorf("EntrustPrice = %q, want 388: the wire carries \"388.00\" and the "+
				"canonical decimal rendering drops the padding, which is a rendering "+
				"property and not a lost digit", got)
		}
		if got := order.EntrustAmount.String(); got != "1000" {
			t.Errorf("EntrustAmount = %q, want 1000", got)
		}
		if got := order.CumQty.String(); got != "200" {
			t.Errorf("CumQty = %q, want 200", got)
		}
		if got := order.LeavesQty.String(); got != "800" {
			t.Errorf("LeavesQty = %q, want 800", got)
		}
		if got := order.RoundLot.String(); got != "100" {
			t.Errorf("RoundLot = %q, want 100", got)
		}
		if got := order.AvgPx.String(); got != "388.1" {
			t.Errorf("AvgPx = %q, want 388.1", got)
		}
		if order.TradeDate != "20260921" || order.SendingTime != "2026-09-21 09:30:00" ||
			order.TransactionTime != "2026-09-21 09:30:01" {
			t.Errorf("the order's timestamps = %q / %q / %q, want the three the mock sent",
				order.TradeDate, order.SendingTime, order.TransactionTime)
		}
	})

	t.Run("QueryEntrustIDList", func(t *testing.T) {
		exec := newSequencedExecutor(t, sequencedReply{
			reply: json.RawMessage(algoEntrustIDListResponseBody),
		})
		ids, err := NewAlgoService(exec).QueryEntrustIDList(t.Context(), algoFixtureAccount(), algoFixtureEntrustIDQuery())
		if err != nil {
			t.Fatalf("QueryEntrustIDList = %v, want nil", err)
		}
		requireCalls(t, exec, 1)
		want := []domain.EntrustID{domain.EntrustID("CH20260921001"), domain.EntrustID("CH20260921002")}
		if len(ids) != len(want) {
			t.Fatalf("ids = %v, want %v", ids, want)
		}
		for i := range want {
			if ids[i] != want[i] {
				t.Errorf("id[%d] = %q, want %q", i, ids[i], want[i])
			}
		}
	})
}

// TestAlgoReadsReturnNonNilEmptyResults covers the two replies that match nothing.
//
// Both are asserted as non-nil empty slices because for a reconciliation query an
// empty answer is a real answer: "the platform sliced no children" and "the account
// has no algorithm masters" are the results a caller is looking for, and a nil slice
// that reads as "nothing came back" invites a caller to treat a successful empty
// reply as a failure.
func TestAlgoReadsReturnNonNilEmptyResults(t *testing.T) {
	for _, tc := range []struct {
		name  string
		reply json.RawMessage
		read  func(t *testing.T, svc *AlgoService) (int, error)
	}{
		{
			name:  "QueryOrderList",
			reply: json.RawMessage(algoOrderListEmptyBody),
			read: func(t *testing.T, svc *AlgoService) (int, error) {
				t.Helper()
				orders, err := svc.QueryOrderList(t.Context(), algoFixtureAccount(), algoFixtureOrderQuery())
				if orders == nil {
					t.Error("QueryOrderList returned a nil slice for an empty reply, want the " +
						"non-nil empty slice so a caller ranging over it behaves the same")
				}
				return len(orders), err
			},
		},
		{
			name:  "QueryEntrustIDList",
			reply: json.RawMessage(algoEntrustIDListEmptyBody),
			read: func(t *testing.T, svc *AlgoService) (int, error) {
				t.Helper()
				ids, err := svc.QueryEntrustIDList(t.Context(), algoFixtureAccount(), algoFixtureEntrustIDQuery())
				if ids == nil {
					t.Error("QueryEntrustIDList returned a nil slice for an empty reply, want " +
						"the non-nil empty slice: \"no children\" is a real reconciliation answer")
				}
				return len(ids), err
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			exec := newSequencedExecutor(t, sequencedReply{reply: tc.reply})
			n, err := tc.read(t, NewAlgoService(exec))
			if err != nil {
				t.Fatalf("%s on an empty reply = %v, want nil", tc.name, err)
			}
			if n != 0 {
				t.Errorf("%s returned %d rows, want 0", tc.name, n)
			}
			requireCalls(t, exec, 1)
		})
	}
}

// TestAlgoReadsTolerAnAbsentListMember is the partial-reply arm, and it is what the
// four value wrappers in pkg/domain exist for.
//
// A read must not crash the caller's process, so an absent list and an absent
// decimal inside a present row both map to an explicit zero. There is no recover()
// here: a mapper that panicked would take the process down and the test binary with
// it, which is the loud failure this row is written to make impossible.
func TestAlgoReadsTolerAnAbsentListMember(t *testing.T) {
	for _, tc := range []struct {
		name  string
		reply string
		read  func(t *testing.T, svc *AlgoService) error
	}{
		{
			name:  "the list member is absent entirely",
			reply: `{}`,
			read: func(t *testing.T, svc *AlgoService) error {
				t.Helper()
				orders, err := svc.QueryOrderList(t.Context(), algoFixtureAccount(), algoFixtureOrderQuery())
				if err != nil {
					return err
				}
				if orders == nil || len(orders) != 0 {
					t.Errorf("orders = %v, want the non-nil empty slice for an absent list", orders)
				}
				return nil
			},
		},
		{
			name: "every decimal on a present row is absent",
			// The row is the mock's, with every money-bearing field rewritten to an
			// empty string. The codes and times survive, because they are dictionary
			// values and strings rather than decimals.
			reply: `{"algoOrderList":[{"orderId":"MA2","stockCode":"00700.HK",` +
				`"entrustPrice":"","entrustAmount":"","cumQty":"","leavesQty":"",` +
				`"avgPx":"","roundLot":"","strategyParam":{"maxVolume":"","minAmount":"",` +
				`"showQty":"","qtyPercent":""}}]}`,
			read: func(t *testing.T, svc *AlgoService) error {
				t.Helper()
				orders, err := svc.QueryOrderList(t.Context(), algoFixtureAccount(), algoFixtureOrderQuery())
				if err != nil {
					return err
				}
				if len(orders) != 1 {
					t.Fatalf("orders = %d, want 1", len(orders))
				}
				order := orders[0]
				for _, f := range []struct {
					name string
					got  string
				}{
					{"EntrustPrice", order.EntrustPrice.String()},
					{"EntrustAmount", order.EntrustAmount.String()},
					{"CumQty", order.CumQty.String()},
					{"LeavesQty", order.LeavesQty.String()},
					{"AvgPx", order.AvgPx.String()},
					{"RoundLot", order.RoundLot.String()},
					{"MaxVolume", order.StrategyParam.MaxVolume.String()},
					{"MinAmount", order.StrategyParam.MinAmount.String()},
					{"ShowQty", order.StrategyParam.ShowQty.String()},
					{"QtyPercent", order.StrategyParam.QtyPercent.String()},
				} {
					if f.got != "0" {
						t.Errorf("%s = %q for an absent field, want an explicit zero rather "+
							"than a panic: a partial reply is a read, and a read must not "+
							"crash the caller's process", f.name, f.got)
					}
				}
				if order.OrderID != domain.OrderID("MA2") {
					t.Errorf("OrderID = %q, want MA2: the codes survive an absent decimal", order.OrderID)
				}
				return nil
			},
		},
		{
			name:  "the child-id list member is absent entirely",
			reply: `{}`,
			read: func(t *testing.T, svc *AlgoService) error {
				t.Helper()
				ids, err := svc.QueryEntrustIDList(t.Context(), algoFixtureAccount(), algoFixtureEntrustIDQuery())
				if err != nil {
					return err
				}
				if ids == nil || len(ids) != 0 {
					t.Errorf("ids = %v, want the non-nil empty slice for an absent list", ids)
				}
				return nil
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			exec := newSequencedExecutor(t, sequencedReply{reply: json.RawMessage(tc.reply)})
			if err := tc.read(t, NewAlgoService(exec)); err != nil {
				t.Fatalf("%s: %v", tc.name, err)
			}
			requireCalls(t, exec, 1)
		})
	}
}

// TestAlgoReadsNeverSendANilParams is the structural guard on the struct{}{} rule,
// stated for the algo surface.
//
// Neither algo read is parameterless — both take a body — so this is not a
// "struct{}{} rather than nil" row; it is the stronger assertion that **neither
// read may pass a nil params**, because internal/transport drops a nil params from
// the envelope entirely (request.Params carries omitempty) and the key would be
// absent rather than an empty object. A body is required on both algo reads, and
// the recorder rows below prove the key is present on the wire.
func TestAlgoReadsNeverSendANilParams(t *testing.T) {
	for _, tc := range algoReadCases() {
		t.Run(tc.name, func(t *testing.T) {
			exec := newSequencedExecutor(t, sequencedReply{reply: tc.reply})
			if err := tc.invoke(t, t.Context(), NewAlgoService(exec)); err != nil {
				t.Fatalf("%s = %v, want nil", tc.name, err)
			}
			if got := exec.lastParams(t); got == nil {
				t.Fatalf("%s handed the executor a nil params. internal/transport drops a "+
					"nil params from the envelope entirely (request.Params is omitempty), "+
					"so the key would be absent rather than an empty object — a different "+
					"body, and one the wire assertions below would not notice", tc.name)
			}
		})
	}

	// The wire half: every algo read's params member is a decodable JSON object with
	// at least one key. This is what a nil would have failed, observed from outside.
	for _, tc := range algoReadCases() {
		t.Run(tc.name+" on the wire", func(t *testing.T) {
			rec := newWireRecorder(map[string]string{
				string(tc.route): gatewaySuccess(string(tc.reply)),
			})
			if err := tc.invoke(t, t.Context(), NewAlgoService(newWireExecutor(t, rec))); err != nil {
				t.Fatalf("%s = %v, want nil", tc.name, err)
			}
			params := algoRecordedParams(t, rec, string(tc.route))
			if len(params) == 0 {
				t.Errorf("the recorded %s request carries an empty params object; want the "+
					"body's own keys, since a dropped member is a request the Gateway "+
					"resolves against its own default", tc.route)
			}
		})
	}
}

// TestAlgoReadAccountIDNeverReachesTheWire is the other direction of the account
// rule, driven over real HTTP so it is the bytes the Gateway would receive.
//
// C2a §7.1 calls the futures accountID leak "invisible on the mock", which is why
// this decodes the recorded request: the check is that no account-shaped key is
// present *and* that the account id appears nowhere in the envelope. The second
// half matters because a value could be smuggled in under a key that is not
// account-shaped.
func TestAlgoReadAccountIDNeverReachesTheWire(t *testing.T) {
	for _, tc := range algoReadCases() {
		t.Run(tc.name, func(t *testing.T) {
			rec := newWireRecorder(map[string]string{
				string(tc.route): gatewaySuccess(string(tc.reply)),
			})
			if err := tc.invoke(t, t.Context(), NewAlgoService(newWireExecutor(t, rec))); err != nil {
				t.Fatalf("%s = %v, want nil", tc.name, err)
			}
			body := rec.lastBody(t, string(tc.route))
			if strings.Contains(body, string(algoFixtureAccount())) {
				t.Errorf("the recorded %s request carries the account id: %s. The algo "+
					"account is resolved from the authenticated session and no vendor puts "+
					"an account identifier on an algo body, so the argument must never "+
					"reach the wire", tc.route, body)
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

// TestAlgoOrderListQueryCarriesItsMarket is the read-side counterpart of the
// market rule, asserted on the wire for both the filtered and unfiltered cases.
//
// The unfiltered row is the control and it is load-bearing: exchangeType is
// optional on this one endpoint, so a test that only asserted its presence would
// pass against a body that always sent it, and a test that only asserted its
// absence would pass against one that never did. Both halves are needed, and the
// coupling to stockCode is the third.
func TestAlgoOrderListQueryCarriesItsMarket(t *testing.T) {
	for _, tc := range []struct {
		name  string
		query AlgoOrderQuery
		// wantMarket is whether the exchangeType key must be on the wire.
		wantMarket bool
		// wantStock is whether the stockCode key must be on the wire.
		wantStock bool
	}{
		{
			name:       "both filters set",
			query:      algoFixtureOrderQuery(),
			wantMarket: true, wantStock: true,
		},
		{
			name:  "market only",
			query: AlgoOrderQuery{Page: algoFixturePage(), ExchangeType: types.ExchangeUS},
			// The market is present because the caller named one, not because the
			// body always carries it.
			wantMarket: true, wantStock: false,
		},
		{
			name:       "neither filter",
			query:      AlgoOrderQuery{Page: algoFixturePage()},
			wantMarket: false, wantStock: false,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := newWireRecorder(map[string]string{
				string(client.RouteTradeAlgoQueryOrderList): gatewaySuccess(algoOrderListResponseBody),
			})
			if _, err := NewAlgoService(newWireExecutor(t, rec)).
				QueryOrderList(t.Context(), algoFixtureAccount(), tc.query); err != nil {
				t.Fatalf("QueryOrderList = %v, want nil", err)
			}
			params := algoRecordedParams(t, rec, string(client.RouteTradeAlgoQueryOrderList))
			if _, ok := params["exchangeType"]; ok != tc.wantMarket {
				t.Errorf("exchangeType present = %v, want %v. The field is optional on this "+
					"one endpoint, so both directions have to be asserted: a body that always "+
					"sends it and one that never does would each satisfy half the test",
					ok, tc.wantMarket)
			}
			if _, ok := params["stockCode"]; ok != tc.wantStock {
				t.Errorf("stockCode present = %v, want %v", ok, tc.wantStock)
			}
			// The dates and counters are always there; the two filters are the
			// optional half, and their presence is asserted from the same tc that
			// decided the expected key set above, so the two cannot disagree.
			want := []string{"pageNo", "pageSize", "startDate", "endDate"}
			if tc.wantMarket {
				want = append(want, "exchangeType")
			}
			if tc.wantStock {
				want = append(want, "stockCode")
			}
			algoRequireKeys(t, "QueryOrderList body", params, want...)
		})
	}
}

// TestAlgoOrderListPageDefaultsAreSubstituted is the paging row that the
// happy-path fixture cannot reach: every other test builds its page through
// algoFixturePage, so the counters are always in range and the substitution of
// the documented 1/30 defaults is never executed.
//
// Both directions of "out of range" are asserted, because the values are ints
// and the zero value is the common mistake: a caller who leaves a PageRequest
// zeroed -- which the type permits, and which no compiler rejects -- must not
// put pageNo=0 on the wire. A negative value is asserted too, since a caller
// computing a page from a subtraction can produce one, and the Gateway is not
// the place to learn that.
func TestAlgoOrderListPageDefaultsAreSubstituted(t *testing.T) {
	for _, tc := range []struct {
		name             string
		pageNo, pageSize int
		wantNo, wantSize int
	}{
		{"both zeroed", 0, 0, algoDefaultPageNo, algoDefaultPageSize},
		{"pageNo only", 0, 25, algoDefaultPageNo, 25},
		{"pageSize only", 7, 0, 7, algoDefaultPageSize},
		{"both negative", -1, -1, algoDefaultPageNo, algoDefaultPageSize},
		{"both in range are left alone", 3, 50, 3, 50},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := newWireRecorder(map[string]string{
				string(client.RouteTradeAlgoQueryOrderList): gatewaySuccess(algoOrderListResponseBody),
			})
			base := algoFixturePage()
			base.PageNo, base.PageSize = tc.pageNo, tc.pageSize

			if _, err := NewAlgoService(newWireExecutor(t, rec)).
				QueryOrderList(t.Context(), algoFixtureAccount(), AlgoOrderQuery{Page: base}); err != nil {
				t.Fatalf("QueryOrderList = %v, want nil", err)
			}
			params := algoRecordedParams(t, rec, string(client.RouteTradeAlgoQueryOrderList))
			// The counters are numbers, not money, so the assertion is on the raw
			// JSON: comparing against a marshalled number catches both a wrong
			// value and a body that started sending them as strings.
			if got, want := string(params["pageNo"]), strconv.Itoa(tc.wantNo); got != want {
				t.Errorf("pageNo = %s, want %s", got, want)
			}
			if got, want := string(params["pageSize"]), strconv.Itoa(tc.wantSize); got != want {
				t.Errorf("pageSize = %s, want %s", got, want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Local rejections
// ---------------------------------------------------------------------------
// TestAlgoReadsRejectAZeroAccountIDWithoutARequest is the fail-closed row for both.
//
// Zero recorded calls is the assertion that matters. accountID is a session key and
// never crosses the wire, so a zero-accountID request body would be byte-identical
// to a valid one and only the absence of the call distinguishes them.
//
// The fixture is a sequencedExecutor with **no scripted replies**: if the validator
// were removed, the executor would hit its t.Fatalf rather than returning a reply
// and letting the test pass vacuously. That is what C4 and C5 did, and it is why
// the zero-call assertion carries information.
func TestAlgoReadsRejectAZeroAccountIDWithoutARequest(t *testing.T) {
	for _, tc := range algoReadCases() {
		t.Run(tc.name, func(t *testing.T) {
			exec := newSequencedExecutor(t)
			assertInvalidParam(t, tc.zeroAccount(t, t.Context(), NewAlgoService(exec)), tc.op)
			requireCalls(t, exec, 0)
		})
	}
}

// algoReadRejection is one local-rejection row: a name for the sub-test, the op the
// error must carry, and the call itself.
type algoReadRejection struct {
	name string
	op   string
	call func(t *testing.T, ctx context.Context, svc *AlgoService) error
}

// algoReadRejections is the local-rejection table for both reads.
//
// Every row is a *local* rejection — a types.StatusInvalidParam *errs.Error in
// errs.CategoryAPI — and every one costs zero HTTP requests. That is the property
// that makes a local rejection safe: a request the Gateway saw and refused may have
// had a side effect, and a request this SDK never sent cannot have had one. It is
// also why the reject is a different category from a Gateway rejection even though
// both can be a 1016-shaped code: a caller branching on the category is reading
// whether anything happened, not just what the message said.
//
// The rows are ordered as the methods check them, so a failure names the check that
// ran rather than one of its neighbours.
func algoReadRejections() []algoReadRejection {
	id := algoFixtureAccount()
	// base is a valid order query, mutated one field at a time below.
	base := algoFixtureOrderQuery()

	return []algoReadRejection{
		// --- QueryOrderList ---
		{
			name: "QueryOrderList/empty startDate", op: opAlgoQueryOrderList,
			call: func(t *testing.T, ctx context.Context, svc *AlgoService) error {
				t.Helper()
				q := base
				q.Page.StartDate = ""
				_, err := svc.QueryOrderList(ctx, id, q)
				return err
			},
		},
		{
			name: "QueryOrderList/malformed startDate", op: opAlgoQueryOrderList,
			call: func(t *testing.T, ctx context.Context, svc *AlgoService) error {
				t.Helper()
				q := base
				q.Page.StartDate = "2026-09-01"
				_, err := svc.QueryOrderList(ctx, id, q)
				return err
			},
		},
		{
			name: "QueryOrderList/nonexistent startDate", op: opAlgoQueryOrderList,
			call: func(t *testing.T, ctx context.Context, svc *AlgoService) error {
				t.Helper()
				q := base
				q.Page.StartDate = "20260230"
				_, err := svc.QueryOrderList(ctx, id, q)
				return err
			},
		},
		{
			name: "QueryOrderList/empty endDate", op: opAlgoQueryOrderList,
			call: func(t *testing.T, ctx context.Context, svc *AlgoService) error {
				t.Helper()
				q := base
				q.Page.EndDate = ""
				_, err := svc.QueryOrderList(ctx, id, q)
				return err
			},
		},
		{
			name: "QueryOrderList/malformed endDate", op: opAlgoQueryOrderList,
			call: func(t *testing.T, ctx context.Context, svc *AlgoService) error {
				t.Helper()
				q := base
				q.Page.EndDate = "202609"
				_, err := svc.QueryOrderList(ctx, id, q)
				return err
			},
		},
		{
			name: "QueryOrderList/nonexistent endDate", op: opAlgoQueryOrderList,
			call: func(t *testing.T, ctx context.Context, svc *AlgoService) error {
				t.Helper()
				q := base
				q.Page.EndDate = "20261301"
				_, err := svc.QueryOrderList(ctx, id, q)
				return err
			},
		},
		{
			// The coupling the released pkg/hstong/algo also enforces: a security
			// code is not unique across the books, so a code with no market is a
			// filter the SDK cannot check.
			name: "QueryOrderList/stockCode without exchangeType", op: opAlgoQueryOrderList,
			call: func(t *testing.T, ctx context.Context, svc *AlgoService) error {
				t.Helper()
				q := base
				q.ExchangeType = ""
				_, err := svc.QueryOrderList(ctx, id, q)
				return err
			},
		},
		{
			name: "QueryOrderList/unrecognised exchangeType", op: opAlgoQueryOrderList,
			call: func(t *testing.T, ctx context.Context, svc *AlgoService) error {
				t.Helper()
				q := base
				q.ExchangeType = "Z"
				_, err := svc.QueryOrderList(ctx, id, q)
				return err
			},
		},
		{
			name: "QueryOrderList/wrong-case exchangeType", op: opAlgoQueryOrderList,
			call: func(t *testing.T, ctx context.Context, svc *AlgoService) error {
				t.Helper()
				// "v" and "t" are lowercase on the wire, so a caller who
				// upper-cases the whole string produces a code that looks right and is
				// not. This is the near-miss A6's test matrix calls out by name.
				q := base
				q.ExchangeType = "V"
				_, err := svc.QueryOrderList(ctx, id, q)
				return err
			},
		},
		{
			name: "QueryOrderList/blank stockCode", op: opAlgoQueryOrderList,
			call: func(t *testing.T, ctx context.Context, svc *AlgoService) error {
				t.Helper()
				q := base
				q.StockCode = "   "
				_, err := svc.QueryOrderList(ctx, id, q)
				return err
			},
		},
		{
			name: "QueryOrderList/stockCode with interior whitespace", op: opAlgoQueryOrderList,
			call: func(t *testing.T, ctx context.Context, svc *AlgoService) error {
				t.Helper()
				q := base
				q.StockCode = "007 00.HK"
				_, err := svc.QueryOrderList(ctx, id, q)
				return err
			},
		},
		{
			// The control: an unfiltered query with valid dates. Both dates are
			// required on this endpoint — the vendors document them as 必须 and take
			// them positionally — so the thing being varied is the *filters*, not
			// the range. If this row were rejected, the rejection table would be
			// satisfied by a validator that refused everything and the routing tests
			// would be the only thing left to catch it.
			name: "QueryOrderList/accepts no filters", op: opAlgoQueryOrderList,
			call: func(t *testing.T, ctx context.Context, svc *AlgoService) error {
				t.Helper()
				if _, err := svc.QueryOrderList(ctx, id, AlgoOrderQuery{Page: base.Page}); err != nil {
					t.Errorf("an unfiltered master-order query was rejected: %v. A query for "+
						"the whole day is a legitimate request", err)
					return nil
				}
				return nil
			},
		},

		// --- QueryEntrustIDList ---
		{
			name: "QueryEntrustIDList/empty orderId", op: opAlgoQueryEntrustIDList,
			call: func(t *testing.T, ctx context.Context, svc *AlgoService) error {
				t.Helper()
				_, err := svc.QueryEntrustIDList(ctx, id, AlgoEntrustIDQuery{
					OrderID: "", TradeDate: "20260921", ExchangeType: types.ExchangeHK,
				})
				return err
			},
		},
		{
			name: "QueryEntrustIDList/blank orderId", op: opAlgoQueryEntrustIDList,
			call: func(t *testing.T, ctx context.Context, svc *AlgoService) error {
				t.Helper()
				_, err := svc.QueryEntrustIDList(ctx, id, AlgoEntrustIDQuery{
					OrderID: "  \t ", TradeDate: "20260921", ExchangeType: types.ExchangeHK,
				})
				return err
			},
		},
		{
			name: "QueryEntrustIDList/empty tradeDate", op: opAlgoQueryEntrustIDList,
			call: func(t *testing.T, ctx context.Context, svc *AlgoService) error {
				t.Helper()
				_, err := svc.QueryEntrustIDList(ctx, id, AlgoEntrustIDQuery{
					OrderID: algoFixtureMasterID(), TradeDate: "", ExchangeType: types.ExchangeHK,
				})
				return err
			},
		},
		{
			name: "QueryEntrustIDList/malformed tradeDate", op: opAlgoQueryEntrustIDList,
			call: func(t *testing.T, ctx context.Context, svc *AlgoService) error {
				t.Helper()
				_, err := svc.QueryEntrustIDList(ctx, id, AlgoEntrustIDQuery{
					OrderID: algoFixtureMasterID(), TradeDate: "2026-09-21", ExchangeType: types.ExchangeHK,
				})
				return err
			},
		},
		{
			name: "QueryEntrustIDList/nonexistent tradeDate", op: opAlgoQueryEntrustIDList,
			call: func(t *testing.T, ctx context.Context, svc *AlgoService) error {
				t.Helper()
				_, err := svc.QueryEntrustIDList(ctx, id, AlgoEntrustIDQuery{
					OrderID: algoFixtureMasterID(), TradeDate: "20260230", ExchangeType: types.ExchangeHK,
				})
				return err
			},
		},
		{
			name: "QueryEntrustIDList/empty exchangeType", op: opAlgoQueryEntrustIDList,
			call: func(t *testing.T, ctx context.Context, svc *AlgoService) error {
				t.Helper()
				_, err := svc.QueryEntrustIDList(ctx, id, AlgoEntrustIDQuery{
					OrderID: algoFixtureMasterID(), TradeDate: "20260921", ExchangeType: "",
				})
				return err
			},
		},
		{
			name: "QueryEntrustIDList/unrecognised exchangeType", op: opAlgoQueryEntrustIDList,
			call: func(t *testing.T, ctx context.Context, svc *AlgoService) error {
				t.Helper()
				_, err := svc.QueryEntrustIDList(ctx, id, AlgoEntrustIDQuery{
					OrderID: algoFixtureMasterID(), TradeDate: "20260921", ExchangeType: "K2",
				})
				return err
			},
		},
	}
}

// TestAlgoReadsRejectBadInputWithoutARequest drives the table above, and the
// zero-call assertion is the point.
//
// The fixture is a sequencedExecutor with no scripted replies, so a validator that
// was removed would hit the executor's t.Fatalf rather than returning a reply and
// letting the row pass. That is the C4/C5 discipline and it is what makes "zero
// requests" a measurement rather than an absence of evidence.
func TestAlgoReadsRejectBadInputWithoutARequest(t *testing.T) {
	rows := algoReadRejections()
	if len(rows) == 0 {
		t.Fatal("the read rejection table is empty, so this test would pass vacuously")
	}
	for _, tc := range rows {
		t.Run(tc.name, func(t *testing.T) {
			// The control row is expected to succeed, so it needs a scripted reply;
			// the rejection rows deliberately get none, so a removed validator would
			// hit the executor's t.Fatalf rather than pass vacuously. The suffix is
			// the discriminator rather than a bool field, because a bool field is
			// one an edit could flip.
			control := strings.HasPrefix(tc.name, "QueryOrderList/accepts")
			var exec *sequencedExecutor
			if control {
				exec = newSequencedExecutor(t, sequencedReply{reply: json.RawMessage(algoOrderListResponseBody)})
			} else {
				exec = newSequencedExecutor(t)
			}
			err := tc.call(t, t.Context(), NewAlgoService(exec))
			if !control {
				assertInvalidParam(t, err, tc.op)
				// Zero is the assertion that makes a local rejection safe: the
				// request never left, so it cannot have had a side effect.
				requireCalls(t, exec, 0)
				return
			}
			if err != nil {
				return
			}
			// A row asserting only that no error came back would pass against a
			// method that returned early without sending anything, so the call
			// itself is counted.
			requireCalls(t, exec, 1)
		})
	}
}

// TestAlgoReadsAcceptTheDocumentedBoundaries is the other half of the same table:
// every value the reference and the vendors document is accepted and reaches the
// wire, so a fail-closed check cannot be widened by accident either.
//
// It is a separate test from the rejection table because the two directions fail
// differently: a check that is too strict is a rejected order, and a check that is
// too loose is a request the SDK sent that it should have known it did not mean.
func TestAlgoReadsAcceptTheDocumentedBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name  string
		query AlgoEntrustIDQuery
	}{
		{"the first day of a month", AlgoEntrustIDQuery{
			OrderID: algoFixtureMasterID(), TradeDate: "20260901", ExchangeType: types.ExchangeHK,
		}},
		{"a leap day", AlgoEntrustIDQuery{
			OrderID: algoFixtureMasterID(), TradeDate: "20240229", ExchangeType: types.ExchangeUS,
		}},
		// All four markets, because the field is case-sensitive and two of the four
		// codes are lowercase.
		{"Hong Kong", AlgoEntrustIDQuery{OrderID: algoFixtureMasterID(), TradeDate: "20260921", ExchangeType: "K"}},
		{"the US", AlgoEntrustIDQuery{OrderID: algoFixtureMasterID(), TradeDate: "20260921", ExchangeType: "P"}},
		{"Shenzhen Connect, lowercase v", AlgoEntrustIDQuery{OrderID: algoFixtureMasterID(), TradeDate: "20260921", ExchangeType: "v"}},
		{"Shanghai Connect, lowercase t", AlgoEntrustIDQuery{OrderID: algoFixtureMasterID(), TradeDate: "20260921", ExchangeType: "t"}},
		// A code with a market suffix and a plain one: no algo code grammar is
		// documented, so the check stays permissive about characters.
		{"an A-share code with no suffix", AlgoEntrustIDQuery{OrderID: algoFixtureMasterID(), TradeDate: "20260921", ExchangeType: "v"}},
		{"a padded code, which the released layer also accepts", AlgoEntrustIDQuery{
			OrderID: algoFixtureMasterID(), TradeDate: "20260921", ExchangeType: "K",
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := newWireRecorder(map[string]string{
				string(client.RouteTradeAlgoQueryEntrustIdList): gatewaySuccess(algoEntrustIDListResponseBody),
			})
			if _, err := NewAlgoService(newWireExecutor(t, rec)).
				QueryEntrustIDList(t.Context(), algoFixtureAccount(), tc.query); err != nil {
				t.Fatalf("a documented value was rejected: %v", err)
			}
			if got := rec.count(string(client.RouteTradeAlgoQueryEntrustIdList)); got != 1 {
				t.Errorf("requests = %d, want 1: a row that asserts only \"no error\" would "+
					"pass against a method that returned early without sending anything", got)
			}
		})
	}
}

// TestAlgoReadRejectsEveryUnrecognisedExchangeType drives twelve out-of-set markets
// through both reads, because a check dropped from one method's validator would
// satisfy the other.
//
// The near-misses are the ones a caller is likely to send: the wrong case (the two
// Connect codes are lowercase), a neighbouring letter, a numeric where a letter
// belongs, the market's name rather than its code, a trailing space, and a
// non-ASCII digit that looks like one.
func TestAlgoReadRejectsEveryUnrecognisedExchangeType(t *testing.T) {
	for _, code := range []string{
		"Z", "z", "k", "p", "T", "KP", "K ", " K", "1", "KK", "HK", "US", "SZ", "SH",
		"Ｋ",
	} {
		t.Run("exchangeType "+code, func(t *testing.T) {
			// QueryEntrustIDList, where the field is required.
			exec := newSequencedExecutor(t)
			_, err := NewAlgoService(exec).QueryEntrustIDList(t.Context(), algoFixtureAccount(),
				AlgoEntrustIDQuery{OrderID: algoFixtureMasterID(), TradeDate: "20260921", ExchangeType: types.ExchangeType(code)})
			assertInvalidParam(t, err, opAlgoQueryEntrustIDList)
			requireCalls(t, exec, 0)

			// QueryOrderList, where the field is optional but present-and-unrecognised
			// is still refused. Dropping the check there because it is optional would
			// be a second, different policy for the same type on the same surface.
			exec = newSequencedExecutor(t)
			q := algoFixtureOrderQuery()
			q.ExchangeType = types.ExchangeType(code)
			_, err = NewAlgoService(exec).QueryOrderList(t.Context(), algoFixtureAccount(), q)
			assertInvalidParam(t, err, opAlgoQueryOrderList)
			requireCalls(t, exec, 0)
		})
	}

	// The control: the two reads with the market *absent*, which is a legitimate
	// state for the order query and the thing the optional-field exemption is for.
	t.Run("the order query with both filters absent is sent", func(t *testing.T) {
		rec := newWireRecorder(map[string]string{
			string(client.RouteTradeAlgoQueryOrderList): gatewaySuccess(algoOrderListResponseBody),
		})
		if _, err := NewAlgoService(newWireExecutor(t, rec)).
			QueryOrderList(t.Context(), algoFixtureAccount(), AlgoOrderQuery{Page: algoFixturePage()}); err != nil {
			t.Fatalf("an unfiltered master-order query was rejected: %v", err)
		}
		if got := rec.count(string(client.RouteTradeAlgoQueryOrderList)); got != 1 {
			t.Errorf("requests = %d, want 1: without this control, a validator that refused "+
				"the field on every path would satisfy the twelve rows above", got)
		}
	})
}

// TestAlgoLeapDayIsAcceptedAndItsNeighboursAreNot is the calendar half of the date
// validation, kept separate because it is the one check where the shape is right
// and the value is wrong.
//
// A caller that passed 20260230 has a typo the Gateway would refuse, and a caller
// that passed 20240229 must not be refused: the shape is eight digits in both cases
// and only a calendar parse tells them apart.
func TestAlgoLeapDayIsAcceptedAndItsNeighboursAreNot(t *testing.T) {
	for _, tc := range []struct {
		date  string
		valid bool
		why   string
	}{
		{"20240229", true, "a real leap day"},
		{"20260229", false, "2026 is not a leap year"},
		{"20260230", false, "February has 28 days in 2026"},
		{"20260431", false, "April has 30 days"},
		{"20261301", false, "there is no thirteenth month"},
		{"20260001", false, "there is no zeroth month"},
		{"20260100", false, "there is no zeroth day"},
		{"20260132", false, "there is no thirty-second day"},
		{"20260921", true, "an ordinary date"},
	} {
		t.Run(tc.date, func(t *testing.T) {
			// A valid date must reach the executor, so it gets a scripted reply; an
			// invalid one must not reach it, so it gets none and a removed check
			// would hit the executor's t.Fatalf rather than pass vacuously.
			var exec *sequencedExecutor
			if tc.valid {
				exec = newSequencedExecutor(t, sequencedReply{reply: json.RawMessage(algoEntrustIDListResponseBody)})
			} else {
				exec = newSequencedExecutor(t)
			}
			_, err := NewAlgoService(exec).QueryEntrustIDList(t.Context(), algoFixtureAccount(),
				AlgoEntrustIDQuery{
					OrderID: algoFixtureMasterID(), TradeDate: tc.date, ExchangeType: types.ExchangeHK,
				})
			if tc.valid {
				if err != nil {
					t.Fatalf("tradeDate %q was rejected: %v — it is %s", tc.date, err, tc.why)
				}
				requireCalls(t, exec, 1)
				return
			}
			assertInvalidParam(t, err, opAlgoQueryEntrustIDList)
			requireCalls(t, exec, 0)
		})
	}
}

// ---------------------------------------------------------------------------
// Error arms, both levels
// ---------------------------------------------------------------------------

// TestAlgoReadExecutorFailurePropagates is Level 1: the fake returns a sentinel and
// the caller gets it back unchanged, with the zero value beside it.
//
// Unchanged is the assertion. A service that wrapped a transport error in its own
// would still be traversable with errors.Is, so the check here is both: the sentinel
// must match under errors.Is *and* the returned value must be the zero value, so a
// caller cannot mistake a partially decoded payload for a success.
func TestAlgoReadExecutorFailurePropagates(t *testing.T) {
	sentinel := errors.New("transport is down")
	for _, tc := range algoReadCases() {
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

// TestAlgoReadGatewayRejectionArrivesTyped is Level 2: a real *client.Client over a
// server answering a real ok:false envelope, asserting the typed *errs.Error the
// transport actually builds.
//
// A fake never builds a typed error, so a service laundering a Gateway rejection into
// an opaque one passes every Level 1 row and fails here. The category is asserted
// alongside the code, because the code alone would not distinguish a trading
// rejection from a session one — and those two call for opposite responses.
func TestAlgoReadGatewayRejectionArrivesTyped(t *testing.T) {
	for _, tc := range []struct {
		code types.StatusCode
		want string
	}{
		{types.StatusServiceBusy, "the Gateway is busy, which a read may retry"},
		{types.StatusInvalidParam, "the Gateway rejected the request, which is a read's caller error"},
		{types.StatusDuplicateSubmit, "the ambiguous-submission code, which on a read still " +
			"arrives in the trading category"},
	} {
		t.Run(string(tc.code)+": "+tc.want, func(t *testing.T) {
			for _, rc := range algoReadCases() {
				t.Run(rc.name, func(t *testing.T) {
					rec := newWireRecorder(map[string]string{
						string(rc.route): gatewayFailure(tc.code, "the Gateway said no"),
					})
					err := rc.invoke(t, t.Context(), NewAlgoService(newWireExecutor(t, rec)))
					typed := errRejects(t, err, tc.code, rc.op)
					if got := errs.CategoryOf(err); got != typed.Category {
						t.Errorf("CategoryOf = %q, want the same as the error's own %q",
							got, typed.Category)
					}
					// A read has no side effect, so a Gateway rejection here is
					// final rather than ambiguous — but the code must still arrive
					// typed, because a caller branching on 1011 versus 1016 is
					// reading the exchange.
					if errs.ReLoginRequired(err) {
						t.Errorf("ReLoginRequired(%v) = true; none of these is a session "+
							"failure", err)
					}
					if got := rec.count(string(rc.route)); got != 1 {
						t.Errorf("requests to %s = %d, want 1", rc.route, got)
					}
				})
			}
		})
	}
}

// TestAlgoReadMalformedReplyIsReportedAndNotDressedAsAGatewayCode covers the reply
// that is not valid JSON for the field the method expects.
//
// The distinction matters: a decode failure is a local transport error and a Gateway
// status code is the exchange's, and reporting one as the other sends a caller
// looking for a session failure that never happened. So two things are asserted —
// errs.CodeOf finds *no* code, and the op is still there, because a caller
// diagnosing a decode failure needs to know which method produced it.
//
// The category is deliberately **not** asserted as CategoryAPI. A decode failure
// carries no Gateway code, and errs.CategoryOf projects "unknown" for one; forcing
// it into the API category would be asserting a classification the error does not
// have. The futures layer's equivalent test asserts the same two things and says
// why.
func TestAlgoReadMalformedReplyIsReportedAndNotDressedAsAGatewayCode(t *testing.T) {
	for _, tc := range []struct {
		name  string
		reply string
		op    string
		read  func(t *testing.T, svc *AlgoService) error
	}{
		{
			name:  "a string where an object is expected",
			reply: `"not an object"`,
			op:    opAlgoQueryOrderList,
			read: func(t *testing.T, svc *AlgoService) error {
				t.Helper()
				_, err := svc.QueryOrderList(t.Context(), algoFixtureAccount(), algoFixtureOrderQuery())
				return err
			},
		},
		{
			name:  "an array where an object is expected",
			reply: `[1,2,3]`,
			op:    opAlgoQueryEntrustIDList,
			read: func(t *testing.T, svc *AlgoService) error {
				t.Helper()
				_, err := svc.QueryEntrustIDList(t.Context(), algoFixtureAccount(), algoFixtureEntrustIDQuery())
				return err
			},
		},
		{
			name:  "a numeric where a string is expected",
			reply: `{"entrustId":[123]}`,
			op:    opAlgoQueryEntrustIDList,
			read: func(t *testing.T, svc *AlgoService) error {
				t.Helper()
				_, err := svc.QueryEntrustIDList(t.Context(), algoFixtureAccount(), algoFixtureEntrustIDQuery())
				return err
			},
		},
		{
			name:  "a bare number as the data member",
			reply: `42`,
			op:    opAlgoQueryOrderList,
			read: func(t *testing.T, svc *AlgoService) error {
				t.Helper()
				_, err := svc.QueryOrderList(t.Context(), algoFixtureAccount(), algoFixtureOrderQuery())
				return err
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := newWireRecorder(map[string]string{
				string(client.RouteTradeAlgoQueryOrderList):     gatewaySuccess(tc.reply),
				string(client.RouteTradeAlgoQueryEntrustIdList): gatewaySuccess(tc.reply),
			})
			err := tc.read(t, NewAlgoService(newWireExecutor(t, rec)))
			if err == nil {
				t.Fatalf("a malformed reply was accepted: %s", tc.reply)
			}
			if code, ok := errs.CodeOf(err); ok {
				t.Errorf("CodeOf = (%q, true), want (false): a decode failure is a local "+
					"transport error and must not carry a Gateway status code, or a caller "+
					"branching on 1016 or 1011 would match something the Gateway never said",
					code)
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

// TestAlgoReadsRetryUnderARetryPolicy is the named half of the ADR 0003 control.
//
// The two algo reads must take every attempt a live policy offers, because they are
// classified as queries. The mutation file's proof asserts the complement — one
// request each — and this assertion is what makes that one meaningful: a policy that
// was never applied would produce exactly the same single request as ADR 0003 working.
//
// The policy is MaxAttempts 5 with BaseBackoff 0, so the count is a property of the
// retry classification and not of elapsed time — the discipline internal/push needed
// after its reconnect tests made a coverage figure a coin flip. Nothing here points
// at 127.0.0.1:11111.
func TestAlgoReadsRetryUnderARetryPolicy(t *testing.T) {
	const maxAttempts = 5
	busy := types.StatusServiceBusy

	// Prove the case is retryable before relying on it. Otherwise "the reads take
	// five" would be true for the boring reason that nothing wanted a second one.
	if !errs.Retryable(errs.New(busy, opAlgoQueryOrderList, "service busy, retry later")) {
		t.Fatalf("test bug: %q is not retryable, so a read taking one attempt would be "+
			"expected even without ADR 0003", busy)
	}

	for _, rc := range algoReadCases() {
		t.Run(rc.name+" retries", func(t *testing.T) {
			rec := newWireRecorder(map[string]string{
				string(rc.route): gatewayFailure(busy, "service busy, retry later"),
			})
			svc := NewAlgoService(newWireExecutor(t, rec,
				client.WithRetryPolicy(client.RetryPolicy{MaxAttempts: maxAttempts, BaseBackoff: 0})))

			err := rc.invoke(t, t.Context(), svc)
			if err == nil {
				t.Fatalf("%s = nil error, want the %q rejection", rc.name, busy)
			}
			// The typed error is asserted, not just the count: a caller reading a
			// busy Gateway needs to know the exchange was busy, not that the SDK gave
			// up.
			errRejects(t, err, busy, rc.op)

			if got := rec.count(string(rc.route)); got != maxAttempts {
				t.Errorf("requests to %s = %d, want %d. An algo read is a query and must be "+
					"retryable, which is the control that makes the mutation file's "+
					"single-request rows mean something", rc.route, got, maxAttempts)
			}
			t.Logf("READ %s: %d requests under MaxAttempts %d", rc.route, rec.total(), maxAttempts)
		})
	}
}

// TestAlgoReadAttemptCountIsIndependentOfThePolicySize widens the read guarantee from
// "one policy" to "every policy", so a future policy change cannot quietly make a
// read single-shot.
//
// The two ends of the range are the interesting ones: resilience.Policy treats a
// MaxAttempts below one as one, and a very large budget must still be bounded by
// the policy rather than by the test's patience — which is why the top row is 20
// and not 10^9.
func TestAlgoReadAttemptCountIsIndependentOfThePolicySize(t *testing.T) {
	for _, maxAttempts := range []int{1, 2, 5, 20} {
		t.Run("MaxAttempts "+strconv.Itoa(maxAttempts), func(t *testing.T) {
			for _, rc := range algoReadCases() {
				t.Run(rc.name, func(t *testing.T) {
					rec := newWireRecorder(map[string]string{
						string(rc.route): gatewayFailure(types.StatusServiceBusy, "busy"),
					})
					svc := NewAlgoService(newWireExecutor(t, rec,
						client.WithRetryPolicy(client.RetryPolicy{MaxAttempts: maxAttempts, BaseBackoff: 0})))
					if err := rc.invoke(t, t.Context(), svc); err == nil {
						t.Fatalf("%s = nil error, want the rejection", rc.name)
					}
					// At least one, and never more than the budget.
					if got := rec.total(); got < 1 || got > maxAttempts {
						t.Errorf("%s with MaxAttempts %d produced %d requests, want between 1 and %d",
							rc.name, maxAttempts, got, maxAttempts)
					}
				})
			}
		})
	}
}

// TestAlgoReadCallIsCancellable pins that a cancelled context is reported as a
// cancellation and never as a success, matched through errors.Is on
// context.Canceled rather than on a rendered message.
//
// A read is single-shot here, so each costs at most a handful of recorded requests.
// A caller who gives up on the context must not be able to turn a read into a
// different request, and the error it gets back must still be the cancellation
// rather than a Gateway status the SDK made up on the way out.
func TestAlgoReadCallIsCancellable(t *testing.T) {
	for _, rc := range algoReadCases() {
		t.Run(rc.name, func(t *testing.T) {
			rec := newWireRecorder(map[string]string{
				string(rc.route): gatewayFailure(types.StatusServiceBusy, "busy"),
			})
			svc := NewAlgoService(newWireExecutor(t, rec,
				client.WithRetryPolicy(client.RetryPolicy{MaxAttempts: 5, BaseBackoff: 0})))

			ctx, cancel := context.WithCancel(t.Context())
			cancel()

			err := rc.invoke(t, ctx, svc)
			if !errors.Is(err, context.Canceled) {
				t.Errorf("%s on a cancelled context = %v, want context.Canceled", rc.name, err)
			}
			if got := errs.CategoryOf(err); got != errs.CategoryTimeout {
				t.Errorf("CategoryOf = %q, want %q for a cancelled call", got, errs.CategoryTimeout)
			}
		})
	}
}

// TestAlgoReadsAreSafeForConcurrentUse is the concurrency half of the AlgoService
// contract.
//
// It is worth having because a shared decoded envelope would be the one way this
// layer could be wrong without any single call being wrong: two callers sharing one
// reply's backing array would each see the other's rows. Every worker gets its own
// decoded value and every request is counted, so a service that reused a buffer
// would show up as a wrong balance rather than as a race the detector happened to
// schedule.
func TestAlgoReadsAreSafeForConcurrentUse(t *testing.T) {
	const workers = 16
	rec := newWireRecorder(map[string]string{
		string(client.RouteTradeAlgoQueryOrderList): gatewaySuccess(algoOrderListResponseBody),
	})
	svc := NewAlgoService(newWireExecutor(t, rec))

	type result struct {
		orderID string
		err     error
	}
	results := make([]result, workers)
	start := make(chan struct{})
	done := make(chan struct{}, workers)
	for i := range workers {
		go func() {
			defer func() { done <- struct{}{} }()
			<-start
			orders, err := svc.QueryOrderList(t.Context(), algoFixtureAccount(), algoFixtureOrderQuery())
			if err != nil {
				results[i] = result{err: err}
				return
			}
			if len(orders) == 1 {
				results[i] = result{orderID: string(orders[0].OrderID)}
			}
		}()
	}
	close(start)
	for range workers {
		<-done
	}

	for i, r := range results {
		if r.err != nil {
			t.Errorf("worker %d = %v, want nil", i, r.err)
			continue
		}
		if r.orderID != "MA20260921001" {
			t.Errorf("worker %d read %q, want MA20260921001: two callers shared one decoded "+
				"reply", i, r.orderID)
		}
	}
	if got := rec.count(string(client.RouteTradeAlgoQueryOrderList)); got != workers {
		t.Errorf("requests = %d, want %d: a call that returned a result must have gone out", got, workers)
	}
}
