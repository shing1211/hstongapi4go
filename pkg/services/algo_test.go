// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package services

import (
	"context"
	"encoding/json"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/shing1211/hstongapi4go/client"
	"github.com/shing1211/hstongapi4go/pkg/domain"
	"github.com/shing1211/hstongapi4go/pkg/types"
	"github.com/shing1211/hstongapi4go/test/mockgateway"
)

// This file is the scaffold half of the algo service layer's regression net: the
// seven operation labels, the constructor, the seven request bodies, the three
// reply envelopes, and the fixtures every other algo test file drives.
//
// The endpoint behaviour is in three files beside this one, split by the shape of
// what they prove rather than by the size of the table:
//
//   - algo_reads_test.go — the two reads, their local rejections, and their error
//     arms at both levels.
//   - algo_mutations_test.go — the five mutations, their local rejections, the
//     ADR 0003 proof with its read control, and the typed reconciliation error.
//   - algo_money_test.go — the money hosts, on the request side because an algo
//     order's quantity is a *write*.
//
// The algo domain layer — the four dictionaries, the row payload, the model and the
// mappers — is tested in pkg/domain/algo_test.go, beside the code it covers. What
// stays here is what this layer owns: a request is built here, an envelope is
// decoded here, and the type-collision assertions need a service to drive because
// the failure they guard against is one that compiles.
//
// The fixtures are the mock Gateway's own algo payloads, copied verbatim from
// test/mockgateway/fixtures.go, for the reason the futures file gives: importing
// them would make the SDK's wire types and the mock's payloads move together, so a
// rename of a key in one would silently pass. Here a mismatch between the two is
// the failure.
//
// The mock Gateway answers all seven algo routes by path and never inspects a
// request body, so a call-level test would pass with every field name misspelled.
// A struct-level assertion on the marshalled body is the only check in this
// repository that can fail when a JSON key is wrong, and TestAlgoRequestWireKeys
// is that check for the 26 request keys.
//
// Two lessons from C5 are carried explicitly, because both cost a survivor:
//
//   - **A rule asserted on one body is half a rule.** AlgoActionOrder is a
//     structurally different body from the four order mutations — no price, no
//     quantity, no strategyParam, and an `action` key none of the others has — so
//     every request-shape assertion below is driven *per body* through the case
//     tables and names the body it is asserting. A rule proved on AddOrder says
//     nothing about ActionOrder. C5's one surviving mutation was exactly that
//     shape: dropping omitempty from the *modify* body passed because every
//     assertion about a dropped date went through the entrust.
//   - **The driver must be able to lie.** The mutation driver is reported in the
//     final section of algo_mutations_test.go, and it reports NOOP on a zero-match
//     and takes its baseline hash from outside itself. C5's first driver was
//     PowerShell, where a backtick is a literal backtick rather than an escape: it
//     silently deleted a validation, measured every verdict against the
//     already-damaged file, and reported nothing wrong — its own hash comparison
//     could not see the damage because the damage predated its baseline.
//
// No row sleeps: where a retry count is the assertion, the policy installs
// MaxAttempts with a zero backoff, so the count is a property of the retry
// classification and not of elapsed time — the discipline internal/push needed
// after its reconnect tests made a coverage figure a coin flip. No row recovers: a
// builder or mapper that panicked would take the process down, and the
// partial-reply rows assert the opposite.

// ---------------------------------------------------------------------------
// Fixtures - the mock Gateway's algo payloads
// ---------------------------------------------------------------------------

// algoMasterOrderBody is one master-order row, copied verbatim from the mock's
// /trade/AlgoQueryOrderList fixture.
const algoMasterOrderBody = `{"orderId":"MA20260921001","stockCode":"00700.HK","exchangeType":"K","tradeDate":"20260921","entrustType":"1","entrustPrice":"388.00","entrustAmount":"1000","cumQty":"200","leavesQty":"800","status":"1","entrustBs":"1","targetStrategy":"1","strategyStatus":"1","strategyParam":{"origStartTime":"093000","origEndTime":"160000","maxVolume":"100","minAmount":"10000","sensitivity":"1","qtyPercent":"10","interval":60},"roundLot":"100","sendingTime":"2026-09-21 09:30:00","transactionTime":"2026-09-21 09:30:01","avgPx":"388.10"}`

// algoOrderListResponseBody is that row inside the envelope the endpoint returns it
// in.
const algoOrderListResponseBody = `{"algoOrderList":[` + algoMasterOrderBody + `]}`

// algoOrderListEmptyBody is the same envelope with nothing in it. It is why the
// read returns a slice rather than a page: the reply carries no page state, so a
// page built from it would be four zero-valued fields.
const algoOrderListEmptyBody = `{"algoOrderList":[]}`

// algoEntrustIDListResponseBody is the mock's /trade/AlgoQueryEntrustIdList
// payload. Note the ids are CH…, children, and not MA… — which is the wire evidence
// for the return-type distinction CancelEntrust makes.
const algoEntrustIDListResponseBody = `{"entrustId":["CH20260921001","CH20260921002"]}`

// algoEntrustIDListEmptyBody is the same envelope with nothing in it. "No children"
// is a real answer to a reconciliation query and must not read as a failed call.
const algoEntrustIDListEmptyBody = `{"entrustId":[]}`

// algoMutationBody is the {"data":"<id>"} body all five mutations return.
const algoMutationBody = `{"data":"MA20260921001"}`

// algoMutationBodyWith is the same body with a caller-chosen identifier, for the
// rows that assert the reply's value is what the method hands back.
func algoMutationBodyWith(id string) string {
	return `{"data":"` + id + `"}`
}

// ---------------------------------------------------------------------------
// Assertion helpers
// ---------------------------------------------------------------------------

// algoWireKeys marshals a wire request and returns its decoded body, so a test can
// assert on the key set rather than on a substring. "absent" and "spelled
// differently" are indistinguishable to strings.Contains, and the absent direction
// is the one that matters for an omitempty.
func algoWireKeys(t *testing.T, body any) map[string]json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("test bug: marshalling %T: %v", body, err)
	}
	var out map[string]json.RawMessage
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("test bug: the marshalled %T is not a JSON object: %v", body, err)
	}
	return out
}

// algoRequireKeys asserts the body carries exactly want, so a key added by mistake
// fails here as loudly as one dropped by mistake. The Gateway drops what it does not
// recognise, so a stray field is a silent no-op rather than a rejection.
func algoRequireKeys(t *testing.T, label string, got map[string]json.RawMessage, want ...string) {
	t.Helper()
	gotKeys := make([]string, 0, len(got))
	for k := range got {
		gotKeys = append(gotKeys, k)
	}
	sort.Strings(gotKeys)
	wantKeys := append([]string(nil), want...)
	sort.Strings(wantKeys)
	if !reflect.DeepEqual(gotKeys, wantKeys) {
		t.Errorf("%s carries %v, want exactly %v", label, gotKeys, wantKeys)
	}
}

// algoRequireString asserts a decoded wire value, quoted or not.
func algoRequireString(t *testing.T, params map[string]json.RawMessage, key, want string) {
	t.Helper()
	raw, ok := params[key]
	if !ok {
		t.Fatalf("the body carries no %q key; it carries %v", key, algoBodyKeys(params))
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		var n json.Number
		if err := json.Unmarshal(raw, &n); err != nil {
			t.Fatalf("the %q value is neither a string nor a number: %s", key, raw)
		}
		s = n.String()
	}
	if s != want {
		t.Errorf("%s on the wire = %q, want %q", key, s, want)
	}
}

func algoBodyKeys(params map[string]json.RawMessage) []string {
	out := make([]string, 0, len(params))
	for k := range params {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// algoExpectCall asserts the recorded call reached wantOp and wantRoute.
func algoExpectCall(t *testing.T, exec *sequencedExecutor, wantOp string, wantRoute client.Route) {
	t.Helper()
	got := exec.lastCall(t)
	if got.op != wantOp {
		t.Errorf("op = %q, want %q", got.op, wantOp)
	}
	if got.route != wantRoute {
		t.Errorf("route = %q, want %q", got.route, wantRoute)
	}
}

// algoRecordedParams decodes the params member of the request envelope the recorder
// saw for path. It decodes rather than substring-matches because "absent" and
// "spelled differently" are indistinguishable to strings.Contains, and the absent
// direction is the one that matters for an omitempty.
func algoRecordedParams(t *testing.T, rec *wireRecorder, path string) map[string]json.RawMessage {
	t.Helper()
	var envelope struct {
		Params json.RawMessage `json:"params"`
	}
	body := rec.lastBody(t, path)
	if err := json.Unmarshal([]byte(body), &envelope); err != nil {
		t.Fatalf("the recorded request body is not the expected envelope: %v", err)
	}
	if len(envelope.Params) == 0 {
		t.Fatalf("the request envelope carries no params member at all: %s", body)
	}
	var params map[string]json.RawMessage
	if err := json.Unmarshal(envelope.Params, &params); err != nil {
		t.Fatalf("the envelope's params member is not a JSON object (%s): %v", envelope.Params, err)
	}
	return params
}

// algoRecordedNestedParams decodes the strategyParam object out of a recorded
// request envelope, for the seven keys that live inside it.
//
// It is separate from algoRecordedParams because the nested object is a *different
// shape* — a sub-object, not a member of the envelope — and a helper that reached
// for it through the outer map would fail on a key that is genuinely there.
func algoRecordedNestedParams(t *testing.T, rec *wireRecorder, path string) map[string]json.RawMessage {
	t.Helper()
	params := algoRecordedParams(t, rec, path)
	raw, ok := params["strategyParam"]
	if !ok {
		t.Fatalf("the body carries no strategyParam key; it carries %v", algoBodyKeys(params))
	}
	var nested map[string]json.RawMessage
	if err := json.Unmarshal(raw, &nested); err != nil {
		t.Fatalf("strategyParam is not a JSON object (%s): %v", raw, err)
	}
	return nested
}

// algoReplaceMasterOrderField rewrites one string field of the first master-order
// row inside a list reply, so a money host can target a field without a fixture per
// field — and without a second hand-written seventeen-key payload drifting, which
// is the failure that shape produces.
func algoReplaceMasterOrderField(t *testing.T, key, value string) json.RawMessage {
	t.Helper()
	rows := algoMasterOrderRows(t)
	if _, ok := rows[0][key]; !ok {
		t.Fatalf("test bug: algoOrderList[0] has no %q key; it has %v", key, algoBodyKeys(rows[0]))
	}
	rows[0][key] = json.RawMessage(strconv.Quote(value))
	return algoReassembleMasterOrderList(t, rows)
}

// algoReplaceStrategyParamField rewrites one string field of the nested
// strategyParam object on the first master-order row.
func algoReplaceStrategyParamField(t *testing.T, key, value string) json.RawMessage {
	t.Helper()
	rows := algoMasterOrderRows(t)
	var param map[string]json.RawMessage
	if err := json.Unmarshal(rows[0]["strategyParam"], &param); err != nil {
		t.Fatalf("test bug: strategyParam is not a JSON object: %v", err)
	}
	param[key] = json.RawMessage(strconv.Quote(value))
	encoded, err := json.Marshal(param)
	if err != nil {
		t.Fatalf("test bug: re-marshalling strategyParam: %v", err)
	}
	rows[0]["strategyParam"] = encoded
	return algoReassembleMasterOrderList(t, rows)
}

// algoMasterOrderRows decodes the algoOrderList array out of the mock's fixture.
func algoMasterOrderRows(t *testing.T) []map[string]json.RawMessage {
	t.Helper()
	var env map[string]json.RawMessage
	if err := json.Unmarshal([]byte(algoOrderListResponseBody), &env); err != nil {
		t.Fatalf("test bug: the fixture is not a JSON object: %v", err)
	}
	var rows []map[string]json.RawMessage
	if err := json.Unmarshal(env["algoOrderList"], &rows); err != nil {
		t.Fatalf("test bug: algoOrderList is not a JSON array: %v", err)
	}
	if len(rows) == 0 {
		t.Fatal("test bug: the algoOrderList array is empty, so there is no first row to rewrite")
	}
	return rows
}

// algoReassembleMasterOrderList puts the rewritten rows back inside the envelope.
func algoReassembleMasterOrderList(t *testing.T, rows []map[string]json.RawMessage) json.RawMessage {
	t.Helper()
	encoded, err := json.Marshal(rows)
	if err != nil {
		t.Fatalf("test bug: re-marshalling the rewritten rows: %v", err)
	}
	return json.RawMessage(`{"algoOrderList":` + string(encoded) + `}`)
}

// ---------------------------------------------------------------------------
// Fixtures - the caller-facing requests
// ---------------------------------------------------------------------------

// algoFixtureAccount is the account every algo fixture names. The algo wire never
// carries it, which is exactly the property the zero-accountID tests assert.
func algoFixtureAccount() domain.AccountID { return domain.AccountID("ACC-C7-ALGO") }

// algoFixtureMasterID and algoFixtureChildID are the two identifiers the algo
// fixtures use. They are deliberately different shapes — MA… for a master, CH… for
// a child — so a test that swapped them would fail on the value and not only on the
// type. They are also the shapes the mock Gateway itself uses, so the fixtures and
// the Gateway's own payloads cannot drift apart.
func algoFixtureMasterID() domain.OrderID  { return domain.OrderID("MA20260921001") }
func algoFixtureChildID() domain.EntrustID { return domain.EntrustID("CH20260921001") }

// algoFixturePage is the page the master-order query asks for. It is fully
// populated so a row cannot pass by reaching the executor and being rejected there,
// and so a defaulted counter is distinguishable from an absent one.
func algoFixturePage() PageRequest {
	return PageRequest{PageNo: 2, PageSize: 50, StartDate: "20260901", EndDate: "20260921"}
}

// algoFixtureStrategyParam is the fully populated tuning object. Every member is
// filled in so a row cannot pass by reaching the executor, and so an omitted member
// is distinguishable from a sent one.
func algoFixtureStrategyParam() AlgoStrategyParam {
	return AlgoStrategyParam{
		OrigStartTime: "093000",
		OrigEndTime:   "160000",
		MaxVolume:     domain.MustNewQuantity("100"),
		MinAmount:     domain.MustNewMoney("10000", "HKD", 3),
		Sensitivity:   domain.AlgoSensitivityNeutral,
		ShowQty:       domain.MustNewQuantity("50"),
		QtyPercent:    domain.MustNewRate("10"),
		Interval:      60,
	}
}

// algoFixtureAdd is a fully populated add, using the algo dictionary's own limit
// order rather than types.EntrustTypeLimit, which is a different code.
func algoFixtureAdd() AlgoAddOrderRequest {
	return AlgoAddOrderRequest{
		StockCode:      "00700.HK",
		ExchangeType:   types.ExchangeHK,
		OrderType:      domain.AlgoEntrustTypeLimit,
		Price:          domain.MustNewPrice("388", "0"),
		Quantity:       domain.MustNewQuantity("1000"),
		Side:           types.EntrustBuy,
		TargetStrategy: domain.AlgoStrategyVWAP,
		SessionType:    domain.AlgoSessionTypeOff,
		StrategyParam:  algoFixtureStrategyParam(),
	}
}

// algoFixtureChange is a fully populated change, and it exercises the two shapes
// the add fixture does not: no strategy tuning at all (every member optional) and
// the US market, so the market field is not always "K".
func algoFixtureChange() AlgoChangeOrderRequest {
	return AlgoChangeOrderRequest{
		OrderID:      algoFixtureMasterID(),
		StockCode:    "AAPL.US",
		ExchangeType: types.ExchangeUS,
		Price:        domain.MustNewPrice("250.5", "0"),
		Quantity:     domain.MustNewQuantity("200"),
	}
}

// algoFixtureCancelOrder is a fully populated master cancel.
func algoFixtureCancelOrder() AlgoCancelOrderRequest {
	return AlgoCancelOrderRequest{OrderID: algoFixtureMasterID(), ExchangeType: types.ExchangeHK}
}

// algoFixtureCancelEntrust is a fully populated child cancel.
func algoFixtureCancelEntrust() AlgoCancelEntrustRequest {
	return AlgoCancelEntrustRequest{
		OrderID:      algoFixtureMasterID(),
		EntrustID:    algoFixtureChildID(),
		ExchangeType: types.ExchangeHK,
	}
}

// algoFixtureAction is a fully populated operate, carrying the action code as a
// literal so the fixture is not written in terms of the code under test.
func algoFixtureAction() AlgoActionRequest {
	return AlgoActionRequest{
		OrderID:        algoFixtureMasterID(),
		Action:         domain.AlgoAction("3"),
		TargetStrategy: domain.AlgoStrategy("1001"),
		ExchangeType:   types.ExchangeHK,
	}
}

// algoFixtureOrderQuery is a fully populated master-order query, with both optional
// filters set so the coupling between them is exercised.
func algoFixtureOrderQuery() AlgoOrderQuery {
	return AlgoOrderQuery{
		Page:         algoFixturePage(),
		ExchangeType: types.ExchangeHK,
		StockCode:    "00700.HK",
	}
}

// algoFixtureEntrustIDQuery is a fully populated child-ID query.
func algoFixtureEntrustIDQuery() AlgoEntrustIDQuery {
	return AlgoEntrustIDQuery{
		OrderID:      algoFixtureMasterID(),
		TradeDate:    "20260921",
		ExchangeType: types.ExchangeHK,
	}
}

// ---------------------------------------------------------------------------
// The case tables
// ---------------------------------------------------------------------------

// algoReadCase is one row of every per-method read table: one row per read
// endpoint, carrying the op and route it must reach, the request it must build, the
// reply that satisfies it, and the call itself.
//
// A new read endpoint cannot be added without a row here, and therefore cannot be
// added without a row in the error arms and the local-rejection table. That coupling
// is the point: A2's twenty dark endpoints were dark precisely because the happy
// path and the failure path had no shared table forcing them to be written together.
type algoReadCase struct {
	name string
	op   string
	// route is the endpoint the method must reach.
	route client.Route
	// wantParams is the request body the method must hand the executor. It is
	// compared with reflect.DeepEqual, so a field added by mistake and a field
	// dropped by mistake both fail.
	wantParams any
	// reply is the data object the sequencedExecutor answers with. It is a
	// json.RawMessage rather than a string because the executor marshals it as a
	// document; a []byte would be marshalled as a base64 JSON string and every row
	// would fail on a decode error.
	reply json.RawMessage
	// invoke performs the call and returns its error. When the call failed it also
	// asserts the zero value beside the error, against the concrete result type,
	// because that is the only way a generic zero check means anything here: a
	// caller must never be handed a partially decoded payload alongside a failure.
	invoke func(t *testing.T, ctx context.Context, svc *AlgoService) error
	// zeroAccount invokes the same method with a zero accountID and an otherwise
	// valid request, so the accountID row cannot pass by failing on some other input
	// first.
	zeroAccount func(t *testing.T, ctx context.Context, svc *AlgoService) error
}

// algoReadCases is the two reads, in SPEC order.
func algoReadCases() []algoReadCase {
	id := algoFixtureAccount()
	return []algoReadCase{
		{
			name: "QueryOrderList", op: opAlgoQueryOrderList,
			route: client.RouteTradeAlgoQueryOrderList,
			wantParams: algoOrderQueryWireRequest{
				PageNo: 2, PageSize: 50,
				StartDate: "20260901", EndDate: "20260921",
				ExchangeType: "K", StockCode: "00700.HK",
			},
			reply: json.RawMessage(algoOrderListResponseBody),
			invoke: func(t *testing.T, ctx context.Context, svc *AlgoService) error {
				t.Helper()
				orders, err := svc.QueryOrderList(ctx, id, algoFixtureOrderQuery())
				if err != nil {
					requireZero(t, orders)
				}
				return err
			},
			zeroAccount: func(t *testing.T, ctx context.Context, svc *AlgoService) error {
				t.Helper()
				orders, err := svc.QueryOrderList(ctx, domain.AccountID(""), algoFixtureOrderQuery())
				requireZero(t, orders)
				return err
			},
		},
		{
			name: "QueryEntrustIDList", op: opAlgoQueryEntrustIDList,
			route: client.RouteTradeAlgoQueryEntrustIdList,
			wantParams: algoEntrustIDQueryWireRequest{
				OrderID: "MA20260921001", TradeDate: "20260921", ExchangeType: "K",
			},
			reply: json.RawMessage(algoEntrustIDListResponseBody),
			invoke: func(t *testing.T, ctx context.Context, svc *AlgoService) error {
				t.Helper()
				ids, err := svc.QueryEntrustIDList(ctx, id, algoFixtureEntrustIDQuery())
				if err != nil {
					requireZero(t, ids)
				}
				return err
			},
			zeroAccount: func(t *testing.T, ctx context.Context, svc *AlgoService) error {
				t.Helper()
				ids, err := svc.QueryEntrustIDList(ctx, domain.AccountID(""), algoFixtureEntrustIDQuery())
				requireZero(t, ids)
				return err
			},
		},
	}
}

// algoMutationCase is one row of every per-method mutation table. It is the read
// table's shape for the read table's reason: a new mutation cannot be added without
// a row here, and therefore cannot be added without a row in the error arms, the
// local rejections and the money hosts.
//
// replyID is the identifier the scripted reply carries and the method must hand
// back, kept separate from reply so a row can assert the returned value against
// what the Gateway sent rather than against a hard-coded literal.
type algoMutationCase struct {
	name string
	op   string
	// route is the endpoint the method must reach.
	route client.Route
	// wantParams is the request body the method must hand the executor, compared
	// with reflect.DeepEqual so a field added or dropped by mistake both fail.
	wantParams any
	// reply is the data object the sequencedExecutor answers with.
	reply json.RawMessage
	// replyID is the identifier inside that reply.
	replyID string
	// invoke performs the call and returns its error, asserting the zero value
	// beside a failure against the concrete result type. That is the only way a
	// generic zero check means anything here: a caller must never be handed a
	// partially decoded payload alongside an error.
	invoke func(t *testing.T, ctx context.Context, svc *AlgoService) error
	// zeroAccount invokes the same method with a zero accountID and an otherwise
	// valid request, so the accountID row cannot pass by failing on some other
	// input first.
	zeroAccount func(t *testing.T, ctx context.Context, svc *AlgoService) error
}

// algoMutationCases is the five algo mutations, in SPEC order.
func algoMutationCases() []algoMutationCase {
	id := algoFixtureAccount()
	return []algoMutationCase{
		{
			name: "AddOrder", op: opAlgoAddOrder,
			route: client.RouteTradeAlgoAddOrder,
			wantParams: algoAddOrderWireRequest{
				ExchangeType: "K", StockCode: "00700.HK",
				EntrustType: "1", EntrustBS: "1",
				EntrustAmount: "1000", EntrustPrice: "388",
				TargetStrategy: "1", SessionType: "0",
				StrategyParam: algoStrategyParamWire{
					OrigStartTime: "093000", OrigEndTime: "160000",
					MaxVolume: "100", MinAmount: "10000",
					Sensitivity: "1", ShowQty: "50", QtyPercent: "10", Interval: 60,
				},
			},
			reply:   json.RawMessage(algoMutationBody),
			replyID: "MA20260921001",
			invoke: func(t *testing.T, ctx context.Context, svc *AlgoService) error {
				t.Helper()
				got, err := svc.AddOrder(ctx, id, algoFixtureAdd())
				if err != nil {
					requireZero(t, got)
				}
				return err
			},
			zeroAccount: func(t *testing.T, ctx context.Context, svc *AlgoService) error {
				t.Helper()
				got, err := svc.AddOrder(ctx, domain.AccountID(""), algoFixtureAdd())
				requireZero(t, got)
				return err
			},
		},
		{
			name: "CancelOrder", op: opAlgoCancelOrder,
			route:      client.RouteTradeAlgoCancelOrder,
			wantParams: algoCancelOrderWireRequest{ExchangeType: "K", OrderID: "MA20260921001"},
			reply:      json.RawMessage(algoMutationBody),
			replyID:    "MA20260921001",
			invoke: func(t *testing.T, ctx context.Context, svc *AlgoService) error {
				t.Helper()
				got, err := svc.CancelOrder(ctx, id, algoFixtureCancelOrder())
				if err != nil {
					requireZero(t, got)
				}
				return err
			},
			zeroAccount: func(t *testing.T, ctx context.Context, svc *AlgoService) error {
				t.Helper()
				got, err := svc.CancelOrder(ctx, domain.AccountID(""), algoFixtureCancelOrder())
				requireZero(t, got)
				return err
			},
		},
		{
			name: "CancelEntrust", op: opAlgoCancelEntrust,
			route: client.RouteTradeAlgoCancelEntrust,
			// The child's id, and a different literal shape from the master's: the
			// mock's own cancel-entrust fixture answers CH…, not MA…, and this row
			// is where the distinction is pinned on the request side too.
			wantParams: algoCancelEntrustWireRequest{
				ExchangeType: "K", OrderID: "MA20260921001", EntrustID: "CH20260921001",
			},
			reply:   json.RawMessage(algoMutationBodyWith("CH20260921001")),
			replyID: "CH20260921001",
			invoke: func(t *testing.T, ctx context.Context, svc *AlgoService) error {
				t.Helper()
				got, err := svc.CancelEntrust(ctx, id, algoFixtureCancelEntrust())
				if err != nil {
					requireZero(t, got)
				}
				return err
			},
			zeroAccount: func(t *testing.T, ctx context.Context, svc *AlgoService) error {
				t.Helper()
				got, err := svc.CancelEntrust(ctx, domain.AccountID(""), algoFixtureCancelEntrust())
				requireZero(t, got)
				return err
			},
		},
		{
			name: "ChangeOrder", op: opAlgoChangeOrder,
			route: client.RouteTradeAlgoChangeOrder,
			wantParams: algoChangeOrderWireRequest{
				OrderID: "MA20260921001", ExchangeType: "P", StockCode: "AAPL.US",
				EntrustAmount: "200", EntrustPrice: "250.5",
			},
			reply:   json.RawMessage(algoMutationBody),
			replyID: "MA20260921001",
			invoke: func(t *testing.T, ctx context.Context, svc *AlgoService) error {
				t.Helper()
				got, err := svc.ChangeOrder(ctx, id, algoFixtureChange())
				if err != nil {
					requireZero(t, got)
				}
				return err
			},
			zeroAccount: func(t *testing.T, ctx context.Context, svc *AlgoService) error {
				t.Helper()
				got, err := svc.ChangeOrder(ctx, domain.AccountID(""), algoFixtureChange())
				requireZero(t, got)
				return err
			},
		},
		{
			// The body that is structurally unlike the other four: no price, no
			// quantity, no strategyParam, and an `action` key none of them has. Its
			// wantParams is asserted in full here and again over the wire, because a
			// rule proved on AddOrder says nothing about it.
			name: "ActionOrder", op: opAlgoActionOrder,
			route: client.RouteTradeAlgoActionOrder,
			wantParams: algoActionOrderWireRequest{
				OrderID: "MA20260921001", ExchangeType: "K",
				Action: "3", TargetStrategy: "1001",
			},
			reply:   json.RawMessage(algoMutationBody),
			replyID: "MA20260921001",
			invoke: func(t *testing.T, ctx context.Context, svc *AlgoService) error {
				t.Helper()
				got, err := svc.ActionOrder(ctx, id, algoFixtureAction())
				if err != nil {
					requireZero(t, got)
				}
				return err
			},
			zeroAccount: func(t *testing.T, ctx context.Context, svc *AlgoService) error {
				t.Helper()
				got, err := svc.ActionOrder(ctx, domain.AccountID(""), algoFixtureAction())
				requireZero(t, got)
				return err
			},
		},
	}
}

// algoMutationCaseNamed returns the row for one method, failing the test if the
// name no longer resolves, so a renamed method cannot silently drop out of the
// single-method tables.
func algoMutationCaseNamed(t *testing.T, name string) algoMutationCase {
	t.Helper()
	for _, tc := range algoMutationCases() {
		if tc.name == name {
			return tc
		}
	}
	t.Fatalf("no mutation row named %q; algoMutationCases has drifted", name)
	return algoMutationCase{}
}

// ---------------------------------------------------------------------------
// Operation labels and the constructor
// ---------------------------------------------------------------------------

// TestAlgoOpConstantsNameGatewayPaths is the cross-check on the seven operation
// labels, in both directions.
//
// An op label is the string that appears in every typed error the service raises,
// so a typo in one does not fail loudly — it misattributes every error to a route
// the caller never called. The two ends of this assertion come from different
// sources on purpose: the labels are the canonical route paths, while the path list
// is the mock Gateway's fixture table, which was written from docs/SPEC.md §2.7.
// Neither side can be wrong alone without this failing.
//
// The reverse direction is the one that would otherwise go unnoticed: an eighth algo
// route appearing in the Gateway with no label here.
func TestAlgoOpConstantsNameGatewayPaths(t *testing.T) {
	ops := []string{
		opAlgoAddOrder,
		opAlgoCancelOrder,
		opAlgoCancelEntrust,
		opAlgoChangeOrder,
		opAlgoActionOrder,
		opAlgoQueryOrderList,
		opAlgoQueryEntrustIDList,
	}
	if len(ops) != 7 {
		t.Fatalf("the service declares %d operation labels, want one per algo endpoint (7)", len(ops))
	}

	paths := make(map[string]struct{})
	for _, p := range mockgateway.DefaultFixtures().Paths() {
		paths[p] = struct{}{}
	}

	labelled := make(map[string]string, len(ops))
	for _, op := range ops {
		if !strings.HasPrefix(op, "trade/Algo") {
			t.Errorf("op label %q is not an algo route path", op)
		}
		if prev, dup := labelled[op]; dup {
			t.Errorf("op label %q is declared for two endpoints (%s and another)", op, prev)
		}
		labelled[op] = op
		if _, ok := paths["/"+op]; !ok {
			t.Errorf("op label %q names /%s, which the Gateway does not answer", op, op)
		}
	}
	for p := range paths {
		if !strings.Contains(p, "/trade/Algo") {
			continue
		}
		if _, ok := labelled[strings.TrimPrefix(p, "/")]; !ok {
			t.Errorf("the Gateway answers %s but no op label names it", p)
		}
	}
}

// TestAlgoOpsAreDistinctFromTheOtherSurfaces guards the naming decision the op
// constants make. This package already declares opEntrust and opCancelEntrust for
// the cash endpoints, and the algo surface has its own routes with the same
// suffixes; a shared label would make an algo error indistinguishable from a cash
// one at the call site, which is the whole reason a caller branches on the op.
func TestAlgoOpsAreDistinctFromTheOtherSurfaces(t *testing.T) {
	algoOps := map[string]bool{
		opAlgoAddOrder: true, opAlgoCancelOrder: true, opAlgoCancelEntrust: true,
		opAlgoChangeOrder: true, opAlgoActionOrder: true,
		opAlgoQueryOrderList: true, opAlgoQueryEntrustIDList: true,
	}
	// The cash and futures labels, restated because they are unexported in the
	// same package and therefore reachable by name here.
	others := map[string]string{
		opEntrust:               "opEntrust (cash)",
		opCancelEntrust:         "opCancelEntrust (cash)",
		opBatchCancelEntrust:    "opBatchCancelEntrust (cash)",
		opChangeEntrust:         "opChangeEntrust (cash)",
		opFuturesEntrust:        "opFuturesEntrust",
		opFuturesCancelEntrust:  "opFuturesCancelEntrust",
		opFuturesModifyEntrust:  "opFuturesModifyEntrust",
		opFuturesQueryFundInfo:  "opFuturesQueryFundInfo",
		opRealEntrustList:       "opRealEntrustList",
		opHistoryEntrustList:    "opHistoryEntrustList",
		opBeforeAndAfterSupport: "opBeforeAndAfterSupport",
	}
	for label, which := range others {
		if algoOps[label] {
			t.Errorf("the algo label %q is also declared as %s. They are different routes "+
				"with different bodies — an algo cancel names a child of a master while a "+
				"cash one names a standalone order — so a shared label would make one "+
				"error indistinguishable from the other at the call site", label, which)
		}
	}
}

// TestNewAlgoService covers the three constructor states the other four services
// share: the client passed positionally, an option that replaces it, and two
// options where the last wins.
func TestNewAlgoService(t *testing.T) {
	first := &sequencedExecutor{}
	second := &sequencedExecutor{}

	svc := NewAlgoService(first)
	if svc.client != Executor(first) {
		t.Errorf("client = %v, want the client passed to the constructor", svc.client)
	}

	svc = NewAlgoService(first, WithAlgoClient(second))
	if svc.client != Executor(second) {
		t.Error("WithAlgoClient did not replace the constructor's client")
	}

	svc = NewAlgoService(nil, WithAlgoClient(first), WithAlgoClient(second))
	if svc.client != Executor(second) {
		t.Error("two options were applied, but the last one did not win")
	}
}

// ---------------------------------------------------------------------------
// Request wire types
// ---------------------------------------------------------------------------

// TestAlgoRequestWireKeys pins the 26 request keys against the two vendor SDKs'
// request literals and the released pkg/hstong/algo, which is the only evidence in
// this repository that can fail when one of them is wrong.
//
// It is driven **per body**, which is C5's lesson stated as a structure rather than
// as a note: the five mutation bodies are five different structs, and an assertion
// that iterated a family of them would prove the rule for the first and silently
// skip the rest. Each row here therefore names the body it is asserting, and
// TestAlgoRequestOmitemptyInBothDirections and the money table are driven the same
// way.
func TestAlgoRequestWireKeys(t *testing.T) {
	param := algoFixtureStrategyParam()
	for _, tc := range []struct {
		name string
		body any
		keys []string
		// wantAbsent is the half that catches a rule proved on one body and not on
		// its mirror image — C5's surviving mutation. It is a field on every row
		// rather than on some of them, so a new body states its absences as
		// explicitly as its keys.
		wantAbsent []string
		check      func(t *testing.T, params map[string]json.RawMessage)
	}{
		{
			// Nine keys, the vendor's own nine, in the Java parameter hierarchy's
			// order. The nested object is one of them: the Java parameter class
			// always declares strategyParam and the Python literal always builds
			// it, so an absent object would be a body neither vendor produces.
			name: "AddOrder",
			body: algoAddOrderWireRequest{
				ExchangeType: "K", StockCode: "00700.HK",
				EntrustType: "1", EntrustBS: "1",
				EntrustAmount: "1000", EntrustPrice: "388",
				TargetStrategy: "1", SessionType: "0",
				StrategyParam: algoStrategyParamWireFrom(param),
			},
			keys: []string{
				"entrustAmount", "entrustBs", "entrustPrice", "entrustType",
				"exchangeType", "sessionType", "stockCode", "strategyParam",
				"targetStrategy",
			},
			check: func(t *testing.T, params map[string]json.RawMessage) {
				t.Helper()
				algoRequireString(t, params, "stockCode", "00700.HK")
				algoRequireString(t, params, "exchangeType", "K")
				// The algo order type, which is "1" for a limit order and "3" on
				// the trade surface. Asserting the literal is what pins the two
				// dictionaries apart on the wire, where a swap would be invisible.
				algoRequireString(t, params, "entrustType", "1")
				algoRequireString(t, params, "entrustBs", "1")
				algoRequireString(t, params, "entrustAmount", "1000")
				algoRequireString(t, params, "entrustPrice", "388")
				algoRequireString(t, params, "targetStrategy", "1")
				algoRequireString(t, params, "sessionType", "0")
			},
		},
		{
			// Six keys and *not* nine: the change body deliberately omits
			// entrustType, entrustBs, sessionType and targetStrategy, which both
			// vendors omit. A symmetry edit that mirrored the add body would add
			// four fields the Gateway does not read, and three of them would be a
			// more powerful operation than this endpoint offers. The nested
			// strategyParam is present-but-empty here because the Java parameter
			// class always declares it and the Python literal always builds it, so
			// this is the vendors' own output for a change with no tuning.
			name: "ChangeOrder",
			body: algoChangeOrderWireRequest{
				OrderID: "MA20260921001", ExchangeType: "P", StockCode: "AAPL.US",
				EntrustAmount: "200", EntrustPrice: "250.5",
			},
			keys: []string{
				"entrustAmount", "entrustPrice", "exchangeType", "orderId", "stockCode",
				"strategyParam",
			},
			check: func(t *testing.T, params map[string]json.RawMessage) {
				t.Helper()
				algoRequireString(t, params, "orderId", "MA20260921001")
				algoRequireString(t, params, "stockCode", "AAPL.US")
				algoRequireString(t, params, "exchangeType", "P")
				algoRequireString(t, params, "entrustAmount", "200")
				algoRequireString(t, params, "entrustPrice", "250.5")
			},
			wantAbsent: []string{
				"entrustType", "entrustBs", "sessionType", "targetStrategy",
			},
		},
		{
			// Two keys and *not* a stock code. A master cancel is addressed by the
			// order id and the market alone, and both vendors agree: the Java class
			// adds nothing to orderId and exchangeType, and the Python literal
			// likewise.
			name:  "CancelOrder",
			body:  algoCancelOrderWireRequest{ExchangeType: "K", OrderID: "MA20260921001"},
			keys:  []string{"exchangeType", "orderId"},
			check: func(t *testing.T, params map[string]json.RawMessage) { t.Helper() },
			wantAbsent: []string{
				"stockCode", "entrustAmount", "entrustPrice", "strategyParam",
				"action", "entrustId", "tradeDate", "pageNo", "sessionType",
			},
		},
		{
			// The cancel body plus the child identifier, and the only algo body
			// that names a child.
			name: "CancelEntrust",
			body: algoCancelEntrustWireRequest{
				ExchangeType: "K", OrderID: "MA20260921001", EntrustID: "CH20260921001",
			},
			keys: []string{"entrustId", "exchangeType", "orderId"},
			check: func(t *testing.T, params map[string]json.RawMessage) {
				t.Helper()
				algoRequireString(t, params, "orderId", "MA20260921001")
				algoRequireString(t, params, "entrustId", "CH20260921001")
			},
			wantAbsent: []string{"stockCode", "entrustAmount", "entrustPrice", "strategyParam"},
		},
		{
			// Four keys, and the mirror-image body: no price, no quantity, no
			// strategyParam, and the `action` key none of the others has. C5's
			// surviving mutation was a rule asserted on one body and not on its
			// mirror image; this row is the structural answer to that, and the
			// wantAbsent list is the half that would have caught it.
			name: "ActionOrder",
			body: algoActionOrderWireRequest{
				OrderID: "MA20260921001", ExchangeType: "K",
				Action: "3", TargetStrategy: "1001",
			},
			keys: []string{"action", "exchangeType", "orderId", "targetStrategy"},
			check: func(t *testing.T, params map[string]json.RawMessage) {
				t.Helper()
				algoRequireString(t, params, "action", "3")
				algoRequireString(t, params, "targetStrategy", "1001")
				algoRequireString(t, params, "orderId", "MA20260921001")
				algoRequireString(t, params, "exchangeType", "K")
			},
			wantAbsent: []string{
				"entrustAmount", "entrustPrice", "strategyParam", "stockCode",
				"entrustId", "entrustType", "entrustBs", "sessionType", "tradeDate",
				"pageNo", "startDate", "endDate", "pageSize", "minAmount", "maxVolume",
				"sensitivity", "interval", "qtyPercent", "showQty", "origStartTime",
				"origEndTime",
			},
		},
		{
			name: "QueryOrderList",
			body: algoOrderQueryWireRequest{
				PageNo: 2, PageSize: 50, StartDate: "20260901", EndDate: "20260921",
				ExchangeType: "K", StockCode: "00700.HK",
			},
			keys: []string{
				"endDate", "exchangeType", "pageNo", "pageSize", "startDate", "stockCode",
			},
			check: func(t *testing.T, params map[string]json.RawMessage) {
				t.Helper()
				// The counters are numbers, not strings: the Java
				// AlgoQueryPageParam types them as a boxed Integer and the Python
				// literal passes an int, and a page counter is not money. The
				// released pkg/hstong/algo sends them quoted, which is the one
				// deliberate divergence from the released body and is recorded on
				// the wire struct.
				var pageNo, pageSize int
				if err := json.Unmarshal(params["pageNo"], &pageNo); err != nil {
					t.Errorf("pageNo is not a JSON number (%s): the vendor types it as a "+
						"boxed Integer, and it is a page counter rather than money",
						params["pageNo"])
				}
				if err := json.Unmarshal(params["pageSize"], &pageSize); err != nil {
					t.Errorf("pageSize is not a JSON number (%s)", params["pageSize"])
				}
				if pageNo != 2 || pageSize != 50 {
					t.Errorf("pageNo/pageSize on the wire = %d/%d, want 2/50", pageNo, pageSize)
				}
			},
		},
		{
			name: "QueryEntrustIDList",
			body: algoEntrustIDQueryWireRequest{
				OrderID: "MA20260921001", TradeDate: "20260921", ExchangeType: "K",
			},
			keys: []string{"exchangeType", "orderId", "tradeDate"},
			check: func(t *testing.T, params map[string]json.RawMessage) {
				t.Helper()
				algoRequireString(t, params, "orderId", "MA20260921001")
				algoRequireString(t, params, "tradeDate", "20260921")
				algoRequireString(t, params, "exchangeType", "K")
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			params := algoWireKeys(t, tc.body)
			algoRequireKeys(t, tc.name+" body", params, tc.keys...)
			tc.check(t, params)
			for _, absent := range tc.wantAbsent {
				if _, ok := params[absent]; ok {
					t.Errorf("the %s body carries a %q key. The Gateway drops what it does "+
						"not recognise, so a stray field is a silent no-op rather than a "+
						"rejection — and on a mutation a field the reference does not list "+
						"is a change to the operation this endpoint performs",
						tc.name, absent)
				}
			}
		})
	}
}

// TestAlgoRequestOmitemptyInBothDirections covers the two directions an omitempty
// mistake can take, per body, because one of them is unreachable for a body whose
// fields are all always-sent and the other is the whole point for the rest.
//
// A key that is dropped when it should be sent is a request the Gateway resolves
// against its own default, on a mutation. A key that is sent empty when it should be
// absent is a request asking the Gateway to match a value that does not exist. Both
// are invisible on the mock, which answers by path.
func TestAlgoRequestOmitemptyInBothDirections(t *testing.T) {
	t.Run("an unset optional strategyParam member is dropped, not sent empty", func(t *testing.T) {
		// A change with no tuning at all. The nested object is present — the
		// vendors always send it — and every member inside it is absent, which is
		// what the vendors' own output looks like for that case.
		params := algoWireKeys(t, algoChangeOrderWireRequest{
			OrderID: "MA20260921001", ExchangeType: "K", StockCode: "00700.HK",
			EntrustAmount: "1", EntrustPrice: "1",
		})
		nested, ok := params["strategyParam"]
		if !ok {
			t.Fatal("the strategyParam key is absent; the Java parameter class always " +
				"declares it and the Python literal always builds it")
		}
		var members map[string]json.RawMessage
		if err := json.Unmarshal(nested, &members); err != nil {
			t.Fatalf("strategyParam is not a JSON object: %v", err)
		}
		if len(members) != 0 {
			t.Errorf("strategyParam carries %v, want no members: an unset member must be "+
				"omitted rather than sent as \"\" or 0, because a zero interval would be a "+
				"request for a zero-second schedule and an empty time is a value no calendar "+
				"has", algoBodyKeys(members))
		}
	})

	t.Run("a zero interval is dropped, because zero means the Gateway's default", func(t *testing.T) {
		// Interval is the one member where zero and absent are *not* the same
		// request: the reference documents a default of 60, so sending 0 would ask
		// the Gateway to schedule children with no gap at all.
		nested := algoWireKeys(t, algoStrategyParamWire{Interval: 0})
		if _, ok := nested["interval"]; ok {
			t.Error("the interval key is present with a zero value; zero means the " +
				"Gateway's documented default of 60, so the key must be omitted")
		}
		nested = algoWireKeys(t, algoStrategyParamWire{Interval: 30})
		if raw, ok := nested["interval"]; !ok || string(raw) != "30" {
			t.Errorf("interval = %s (present %v), want the unquoted number 30", raw, ok)
		}
	})

	t.Run("an omitted query filter is dropped, not sent empty", func(t *testing.T) {
		// The Gateway's own filter is optional: a query with neither market nor
		// security code is a legitimate request for the whole day, and sending ""
		// would ask it to match a market named "" rather than to apply no filter.
		params := algoWireKeys(t, algoOrderQueryWireRequest{
			PageNo: 1, PageSize: 30, StartDate: "20260901", EndDate: "20260921",
		})
		for _, key := range []string{"exchangeType", "stockCode"} {
			if _, ok := params[key]; ok {
				t.Errorf("the %q key is present on an unfiltered query; an absent filter is "+
					"a legitimate wire state and must be omitted", key)
			}
		}
		algoRequireKeys(t, "unfiltered query body", params, "endDate", "pageNo", "pageSize", "startDate")
	})

	t.Run("an empty required field is sent empty, not dropped", func(t *testing.T) {
		// The mirror image. Every key on the add body is always sent by both
		// vendors, so a zero-valued struct must still carry all nine — dropping
		// one would make the request depend on a pre-validation value, and
		// validation refuses a zero-valued add before this can be reached, so the
		// only thing this row proves is that the *struct* is tagged the way the
		// wire is.
		params := algoWireKeys(t, algoAddOrderWireRequest{})
		for _, key := range []string{
			"stockCode", "exchangeType", "entrustType", "entrustBs",
			"entrustAmount", "entrustPrice", "targetStrategy", "sessionType",
			"strategyParam",
		} {
			raw, ok := params[key]
			if !ok {
				t.Errorf("the %q key is absent from an all-zero add body; it is required, so "+
					"it must be present and empty rather than omitted", key)
				continue
			}
			if key == "strategyParam" {
				if string(raw) != "{}" {
					t.Errorf("strategyParam = %s, want an empty object", raw)
				}
				continue
			}
			if string(raw) != `""` {
				t.Errorf("the %q key = %s, want an empty string", key, raw)
			}
		}
	})

	t.Run("a zero page counter is sent zero, not dropped", func(t *testing.T) {
		// The mapping boundary substitutes 1 and 30 before the marshal, so a zero
		// only reaches the body if the substitution was skipped. Dropping the key
		// instead would make the request depend on a pre-substitution value, which
		// is the reason this struct has no omitempty at all.
		params := algoWireKeys(t, algoOrderQueryWireRequest{})
		for _, key := range []string{"pageNo", "pageSize"} {
			raw, ok := params[key]
			if !ok {
				t.Errorf("the %q key is absent; a defaulted page counter is still a "+
					"required field and must be present", key)
				continue
			}
			if string(raw) != "0" {
				t.Errorf("the %q key = %s, want the unquoted number 0", key, raw)
			}
		}
	})
}

// TestAlgoChangeBodyHasNoOrderMutationFields is the named half of the mirror-image
// rule, kept as its own test so a failure says what is wrong rather than reporting
// a key-set difference.
//
// The change endpoint reprices a live master. It does not retype it, reverse it,
// change its session, or rebind its strategy, and both vendors' bodies agree: the
// Java AlgoChangeEntrustParam adds only orderId to the shared parameter base, and
// the Python algo_modify_order literal likewise. A change that could do any of the
// four would be a strictly more powerful operation than this endpoint offers, and on
// a live order with no retry.
func TestAlgoChangeBodyHasNoOrderMutationFields(t *testing.T) {
	// The keys the *add* body carries and the change body must not.
	addOnly := []string{"entrustType", "entrustBs", "sessionType", "targetStrategy"}
	changeBody := algoWireKeys(t, algoChangeOrderWireRequest{
		OrderID: "MA20260921001", ExchangeType: "K", StockCode: "00700.HK",
		EntrustAmount: "1", EntrustPrice: "1",
	})
	for _, key := range addOnly {
		if _, ok := changeBody[key]; ok {
			t.Errorf("the change body carries %q. Both vendors omit it: the order keeps the "+
				"type, direction, session and strategy it was placed with, and a change "+
				"that could alter them would be a more powerful operation than this "+
				"endpoint offers", key)
		}
	}
	// And the direction, as a struct-level check rather than a wire one, because a
	// field could be added to the struct and left untagged.
	rt := reflect.TypeOf(algoChangeOrderWireRequest{})
	changeFields := map[string]bool{}
	for i := 0; i < rt.NumField(); i++ {
		key, _, _ := strings.Cut(rt.Field(i).Tag.Get("json"), ",")
		changeFields[key] = true
	}
	for _, key := range addOnly {
		if changeFields[key] {
			t.Errorf("algoChangeOrderWireRequest declares a %q field, so the key is on the "+
				"struct even though the wire check above did not see it", key)
		}
	}
	// The action body is the other mirror image, and the two are asserted here
	// together because the property is one property with two directions.
	actionBody := algoWireKeys(t, algoActionOrderWireRequest{
		OrderID: "MA20260921001", ExchangeType: "K", Action: "1", TargetStrategy: "1",
	})
	for _, key := range []string{
		"entrustAmount", "entrustPrice", "strategyParam", "stockCode", "entrustId",
		"entrustType", "entrustBs", "sessionType",
	} {
		if _, ok := actionBody[key]; ok {
			t.Errorf("the action body carries %q; an operate request addresses a master "+
				"order's strategy and carries no economic terms at all", key)
		}
	}
	if _, ok := actionBody["action"]; !ok {
		t.Error("the action body carries no \"action\" key, which is the one key that " +
			"distinguishes it from every other algo body")
	}
}

// TestAlgoRequestsCarryNoAccountField is the structural guard for the account rule,
// restated at the struct level where a field would be added.
//
// accountID is a session key on every algo method: it is validated and never placed
// in a wire struct, because neither vendor puts an account identifier on an algo
// body and the released pkg/hstong/algo takes no account argument at all. Adding one
// would leak an account identifier into every algo request while staying invisible on
// the mock, which answers by path — the failure C2a §7.1 calls out by name for
// futures.
func TestAlgoRequestsCarryNoAccountField(t *testing.T) {
	forbidden := []string{"accountId", "accountID", "account", "accNo", "userId", "clientType"}
	for _, body := range []any{
		algoAddOrderWireRequest{},
		algoChangeOrderWireRequest{},
		algoCancelOrderWireRequest{},
		algoCancelEntrustWireRequest{},
		algoActionOrderWireRequest{},
		algoOrderQueryWireRequest{},
		algoEntrustIDQueryWireRequest{},
		algoStrategyParamWire{},
	} {
		rt := reflect.TypeOf(body)
		for i := 0; i < rt.NumField(); i++ {
			key, _, _ := strings.Cut(rt.Field(i).Tag.Get("json"), ",")
			for _, bad := range forbidden {
				if key == bad {
					t.Errorf("%s.%s carries the key %q. No algo request carries an account "+
						"identifier: the account and the book are both resolved from the "+
						"authenticated session, and the accountID argument is a local "+
						"precondition that must never reach the wire", rt.Name(), rt.Field(i).Name, bad)
				}
			}
		}
	}
}

// TestAlgoEveryBodyCarriesAMarket is the other structural guard, and it is the
// exact opposite of the futures one: every algo body *does* carry a market.
//
// Unlike a futures contract — whose code is unique across the Hong Kong and US
// books — a security code is not, and both vendor SDKs put exchangeType on all
// seven bodies. A body that dropped it would be a request the Gateway has to guess
// at, and on a cancel exchangeType names the book the master is resolved against.
func TestAlgoEveryBodyCarriesAMarket(t *testing.T) {
	carriesMarket := map[string]bool{
		"algoAddOrderWireRequest":       true,
		"algoChangeOrderWireRequest":    true,
		"algoCancelOrderWireRequest":    true,
		"algoCancelEntrustWireRequest":  true,
		"algoActionOrderWireRequest":    true,
		"algoOrderQueryWireRequest":     true,
		"algoEntrustIDQueryWireRequest": true,
		"algoStrategyParamWire":         false,
	}
	for _, body := range []any{
		algoAddOrderWireRequest{},
		algoChangeOrderWireRequest{},
		algoCancelOrderWireRequest{},
		algoCancelEntrustWireRequest{},
		algoActionOrderWireRequest{},
		algoOrderQueryWireRequest{},
		algoEntrustIDQueryWireRequest{},
		algoStrategyParamWire{},
	} {
		rt := reflect.TypeOf(body)
		want, known := carriesMarket[rt.Name()]
		if !known {
			t.Fatalf("test bug: %s is not in the market table, so this test would not "+
				"check it; a new body must state whether it carries a market", rt.Name())
		}
		has := false
		for i := 0; i < rt.NumField(); i++ {
			if key, _, _ := strings.Cut(rt.Field(i).Tag.Get("json"), ","); key == "exchangeType" {
				has = true
			}
		}
		if has != want {
			t.Errorf("%s carries exchangeType = %v, want %v. %s", rt.Name(), has, want,
				algoMarketWhy(rt.Name(), want))
		}
	}
}

// algoMarketWhy is the one-line explanation a market-presence failure needs, so the
// message says why the field is there rather than only that it is not.
func algoMarketWhy(name string, want bool) string {
	if want {
		return "A security code is not unique across the books, unlike a futures contract " +
			"code, and both vendors put exchangeType on all seven bodies. On a cancel it " +
			"names the book the Gateway resolves the master against."
	}
	return "The strategy tuning object is nested inside a body that already names its " +
		"market, so it carries none of its own."
}

// TestAlgoRequestBodiesCarryNoFloat is the static half of hard rule 3 for the seven
// request bodies, on the same reasoning as the futures file's equivalent.
//
// scripts/check_money.py guards by field name and by wire key, which is the right
// heuristic for the generated DTOs and blind to a decimal field given an innocuous
// name. This walks the bodies a caller fills in and asserts no field is a float at
// all, whatever it is called: the algo money crosses as a quoted decimal string
// (docs/DESIGN.md §7, hard rule 3), and the string fields on these bodies are
// Gateway *codes* rather than amounts.
func TestAlgoRequestBodiesCarryNoFloat(t *testing.T) {
	for _, v := range []any{
		AlgoAddOrderRequest{},
		AlgoChangeOrderRequest{},
		AlgoCancelOrderRequest{},
		AlgoCancelEntrustRequest{},
		AlgoActionRequest{},
		AlgoOrderQuery{},
		AlgoEntrustIDQuery{},
		AlgoStrategyParam{},
		algoAddOrderWireRequest{},
		algoChangeOrderWireRequest{},
		algoCancelOrderWireRequest{},
		algoCancelEntrustWireRequest{},
		algoActionOrderWireRequest{},
		algoOrderQueryWireRequest{},
		algoEntrustIDQueryWireRequest{},
		algoStrategyParamWire{},
	} {
		rt := reflect.TypeOf(v)
		t.Run(rt.Name(), func(t *testing.T) {
			fields := 0
			for i := 0; i < rt.NumField(); i++ {
				f := rt.Field(i)
				fields++
				switch f.Type.Kind() {
				case reflect.Float32, reflect.Float64:
					t.Errorf("%s.%s is a %s: an algo price, quantity or amount crosses the "+
						"wire as a quoted decimal string, and the caller-facing requests hold "+
						"domain.Price, domain.Quantity, domain.Money and domain.Rate "+
						"(docs/DESIGN.md §7, hard rule 3)", rt.Name(), f.Name, f.Type)
				}
			}
			if fields == 0 {
				t.Errorf("%s has no fields, so this test would pass vacuously", rt.Name())
			}
		})
	}
}

// TestAlgoPageDefaults covers the two substitutions that belong at the mapping
// boundary: a non-positive page number becomes 1 and a non-positive page size
// becomes 30.
//
// 30 and not the futures layer's 20 is the load-bearing part: the Java
// AlgoQueryPageParam comment reads 每页条数 默认30 and the Python
// algo_query_order_list signature defaults page_size to 30, while the released
// pkg/hstong/future documents 20 for its own routes. One constant shared between
// the two surfaces would let a change to either silently move the other, which is
// why the constants are declared separately.
func TestAlgoPageDefaults(t *testing.T) {
	if algoDefaultPageNo != 1 {
		t.Errorf("algoDefaultPageNo = %d, want 1", algoDefaultPageNo)
	}
	if algoDefaultPageSize != 30 {
		t.Errorf("algoDefaultPageSize = %d, want 30. The Java AlgoQueryPageParam comment "+
			"reads 每页条数 默认30 and the Python signature defaults page_size to 30; the "+
			"futures layer's 20 is a different endpoint's documented default and must not "+
			"leak here", algoDefaultPageSize)
	}
	if algoDefaultPageSize == futuresDefaultPageSize {
		t.Error("algoDefaultPageSize and futuresDefaultPageSize are equal, so the two " +
			"surfaces' distinct documented defaults are indistinguishable in this test")
	}
	// The defaults are applied where the body is built, and the zero case is
	// asserted on the wire rather than through a helper, because a helper that
	// defaulted correctly while the body did not would pass either test.
	for _, tc := range []struct {
		name     string
		page     PageRequest
		wantNo   int
		wantSize int
	}{
		{"zero value", PageRequest{}, 1, 30},
		{"negative", PageRequest{PageNo: -3, PageSize: -1}, 1, 30},
		{"explicit", PageRequest{PageNo: 4, PageSize: 99}, 4, 99},
		{"page only", PageRequest{PageNo: 7}, 7, 30},
		{"size only", PageRequest{PageSize: 5}, 1, 5},
		// No upper bound: unlike the futures page, no source in this repository
		// documents a maximum algo page size, and inventing one would refuse a page
		// the Gateway accepts. The 99 above is deliberately below the futures
		// ceiling of 100, and the next row is above it.
		{"a page size above the futures ceiling", PageRequest{PageNo: 1, PageSize: 5000}, 1, 5000},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pageNo, pageSize := tc.page.PageNo, tc.page.PageSize
			if pageNo <= 0 {
				pageNo = algoDefaultPageNo
			}
			if pageSize <= 0 {
				pageSize = algoDefaultPageSize
			}
			if pageNo != tc.wantNo || pageSize != tc.wantSize {
				t.Errorf("page %d/%d, want %d/%d", pageNo, pageSize, tc.wantNo, tc.wantSize)
			}
		})
	}
}

// TestAlgoDigitsShaped covers the helper directly, because its two callers guard
// with `field != ""` before calling it and so can never reach the empty row.
//
// The empty case is the interesting one. It is kept as an explicit check inside
// the function rather than left to the callers' guards for two reasons: a third
// caller that forgets the guard would read "" as well-shaped, and the function's
// own name promises a non-empty run, so the contract belongs where the function
// is rather than at every call site. A helper with a narrower contract than its
// name is how an empty date becomes "valid" later.
//
// The non-ASCII rows are the other half. A digit scan written with the wrong
// comparison -- unicode.IsDigit, or a range that stops at '9' with a sign
// allowance -- would admit a code the Gateway would reject, and the caller's
// error message would then name a shape rather than the real fault.
func TestAlgoDigitsShaped(t *testing.T) {
	for _, tc := range []struct {
		name string
		code string
		want bool
	}{
		{"empty", "", false},
		{"single digit", "7", true},
		{"a full date", "20260928", true},
		{"a long run is not length-bounded", "00000000000000000000000000000001", true},
		{"leading zero is a digit here", "00700", true},
		{"letter", "2026AB28", false},
		{"a sign", "+2026", false},
		{"a negative", "-1", false},
		{"a decimal point", "1.0", false},
		{"inner space", "2026 0928", false},
		{"trailing space", "20260928 ", false},
		// These are the rows a non-ASCII-aware scan would get wrong. U+FF10 is
		// FULLWIDTH DIGIT ZERO and unicode.IsDigit reports true for it, so a
		// validator built on that would accept a code the Gateway cannot parse.
		{"fullwidth zero", "２０２６０９２８", false},
		{"arabic-indic zero", "٠", false},
		{"a superscript two is a digit to unicode", "\u00b2", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := algoDigitsShaped(tc.code); got != tc.want {
				t.Errorf("algoDigitsShaped(%q) = %v, want %v", tc.code, got, tc.want)
			}
		})
	}
}
