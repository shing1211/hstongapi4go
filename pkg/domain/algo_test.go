// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package domain

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/shing1211/hstongapi4go/pkg/types"
)

// This file is the regression net for the algo domain layer: the four
// dictionaries the algo surface declares of its own, the two strategy
// dictionaries, the row payload a master-order query returns, the model it maps
// to, and the four value wrappers every decimal on the path is read through.
//
// Its counterpart is pkg/services/algo_test.go, which covers the service layer:
// the operation labels, the seven request bodies, the three reply envelopes, and
// the type-collision assertions that need a service to drive. The line between
// them is the one the layering draws — a row and the mapper that reads it are this
// package's; an envelope that holds rows, and every request body, are the
// service's.
//
// The fixtures are the mock Gateway's own algo payloads, copied verbatim from
// test/mockgateway/fixtures.go. Copying rather than importing is deliberate:
// importing would make this package's wire types and the mock's payloads move
// together, so a rename of a key in one would silently pass. Here a mismatch
// between the two is the failure.

// ---------------------------------------------------------------------------
// Fixtures - the mock Gateway's algo payloads
// ---------------------------------------------------------------------------

// algoMasterOrderBody is one master-order row, the exact payload the mock answers
// /trade/AlgoQueryOrderList with.
//
// Note what it does and does not carry: there is no showQty (it is an ICE_BERG
// member and the strategy here is VWAP), and the strategyParam object carries a
// quoted string for every decimal and a bare number for interval. That split is
// the point — a wire type that typed interval as a string would fail to decode
// this fixture, and one that typed maxVolume as a number would decode it and then
// round the digits away.
const algoMasterOrderBody = `{"orderId":"MA20260921001","stockCode":"00700.HK","exchangeType":"K","tradeDate":"20260921","entrustType":"1","entrustPrice":"388.00","entrustAmount":"1000","cumQty":"200","leavesQty":"800","status":"1","entrustBs":"1","targetStrategy":"1","strategyStatus":"1","strategyParam":{"origStartTime":"093000","origEndTime":"160000","maxVolume":"100","minAmount":"10000","sensitivity":"1","qtyPercent":"10","interval":60},"roundLot":"100","sendingTime":"2026-09-21 09:30:00","transactionTime":"2026-09-21 09:30:01","avgPx":"388.10"}`

// algoMasterOrderBareBody is a master-order row with every decimal absent. It is
// the partial-reply fixture: a read must report zeros for these fields rather than
// panic, because a partial reply is a read and a read must not crash the caller's
// process. It is written by hand rather than derived from the full fixture so that
// a change to the full fixture cannot quietly remove the field under test.
const algoMasterOrderBareBody = `{"orderId":"MA2","stockCode":"AAPL.US","exchangeType":"P","tradeDate":"20260921","entrustType":"2","entrustPrice":"","entrustAmount":"","cumQty":"","leavesQty":"","status":"","entrustBs":"","targetStrategy":"","strategyStatus":"","strategyParam":{},"roundLot":"","sendingTime":"","transactionTime":"","avgPx":""}`

// ---------------------------------------------------------------------------
// Assertion helpers
// ---------------------------------------------------------------------------

// algoRequirePrice, algoRequireQuantity, algoRequireMoney and algoRequireRate
// compare a mapped value against the literal the mock sent.
//
// The same four helpers are declared in pkg/services/algo_test.go for the envelope
// tests. They cannot be shared: a helper in either package's _test.go file is
// invisible to the other, and exporting one from the non-test source to share it
// would put test scaffolding on the public surface.
func algoRequirePrice(t *testing.T, label string, got Price, want string) {
	t.Helper()
	if got.String() != want {
		t.Errorf("%s = %s, want the literal %q the Gateway sent", label, got, want)
	}
}

func algoRequireQuantity(t *testing.T, label string, got Quantity, want string) {
	t.Helper()
	if got.String() != want {
		t.Errorf("%s = %s, want the literal %q the Gateway sent", label, got, want)
	}
}

func algoRequireMoney(t *testing.T, label string, got Money, want, wantCurrency string) {
	t.Helper()
	if got.String() != want {
		t.Errorf("%s = %s, want the literal %q the Gateway sent", label, got, want)
	}
	if got.Currency() != wantCurrency {
		t.Errorf("%s currency = %q, want %q: the algo wire carries no currency field, so an "+
			"amount attributed to the wrong one is a wrong amount that still adds up",
			label, got.Currency(), wantCurrency)
	}
}

func algoRequireRate(t *testing.T, label string, got Rate, want string) {
	t.Helper()
	if got.String() != want {
		t.Errorf("%s = %s, want the literal %q the Gateway sent", label, got, want)
	}
}

// ---------------------------------------------------------------------------
// The type-collision net, first because it is the defining risk
// ---------------------------------------------------------------------------

// TestAlgoDictionariesAreNotTheirTradeNamesakes is the assertion that catches the
// mistake this file exists to prevent: an algo dictionary quietly collapsed onto
// its trade namesake.
//
// The failure mode is a type that **compiles**. `type AlgoEntrustType =
// types.EntrustType` is a legal Go declaration, every call site keeps compiling,
// and the value sets become interchangeable at runtime — so no compiler error and
// no vet warning stands between a refactor and a wrong order. A test has to
// assert the property deliberately, which is what this one does, in four
// independent directions:
//
//  1. **Type identity.** Each algo dictionary must be a *defined* type whose
//     reflect identity is this package's. An alias would report the aliased type's
//     name and package, so reflect.TypeOf names it types.EntrustType and this
//     fails. This is the direction that catches the alias, and it is why the
//     assertion is about the type rather than about the values.
//  2. **Value-set disjointness where the sets claim the same codes.** The algo and
//     trade order-type sets both contain "1" and "2" and mean different things by
//     each, so a code that is valid on one surface must be refused on the other.
//     Asserting the sets differ is a weaker version of the same fact; asserting
//     that the *named constants* differ is the one a collapse breaks.
//  3. **Semantic inversion.** The algo status set inverts the trade status set on
//     the two most common codes: trade 0 is 未报 and algo 0 is 已报; trade 2 is
//     已报 and algo 2 is 全部成交. A collapse would make a caller read a cash
//     order's status and apply it to a master order and get the opposite answer,
//     so the inversion is asserted by name rather than by set comparison.
//  4. **Forward-compatibility asymmetry.** types.EntrustType documents itself as
//     non-exhaustive with unknown values forwarded, and AlgoEntrustType documents
//     itself as a closed two-code set. A collapse would merge those promises, so
//     the assertion is that a code the trade set forwards (EntrustTypeDarkPool
//     "6") is *not* a valid algo code, and that Valid says so.
//
// The assertions read the *source text* of pkg/types/enums.go for the trade
// constants rather than restating their values, so this file cannot drift out of
// agreement with the package it is checking: a change to a trade code moves the
// value this asserts against without touching the test.
func TestAlgoDictionariesAreNotTheirTradeNamesakes(t *testing.T) {
	// 1. Type identity. A defined type in this package reports this package's
	// path and the algo name; an alias reports the aliased type's, which is the
	// whole reason this direction exists.
	domainPkg := reflect.TypeOf(AlgoEntrustType("")).PkgPath()
	if !strings.HasSuffix(domainPkg, "/pkg/domain") {
		t.Fatalf("AlgoEntrustType is declared in %q, want pkg/domain; the package path is "+
			"derived from this file's own location so it cannot be faked", domainPkg)
	}
	for _, tc := range []struct {
		name    string
		got     any
		want    string
		pkgPath string
	}{
		{"AlgoEntrustType", AlgoEntrustType(""), "AlgoEntrustType", domainPkg},
		{"AlgoStatus", AlgoStatus(""), "AlgoStatus", domainPkg},
		{"AlgoSessionType", AlgoSessionType(""), "AlgoSessionType", domainPkg},
		{"AlgoSensitivity", AlgoSensitivity(""), "AlgoSensitivity", domainPkg},
		{"AlgoStrategy", AlgoStrategy(""), "AlgoStrategy", domainPkg},
		{"AlgoStrategyStatus", AlgoStrategyStatus(""), "AlgoStrategyStatus", domainPkg},
		{"AlgoAction", AlgoAction(""), "AlgoAction", domainPkg},
	} {
		t.Run("type identity "+tc.name, func(t *testing.T) {
			rt := reflect.TypeOf(tc.got)
			if rt.Name() != tc.want {
				t.Errorf("%s resolves to the reflect type %q, not %q. A type alias "+
					"(type %s = types.X) compiles and reports the *aliased* type here, which "+
					"is exactly the mistake this assertion exists to catch: the value sets "+
					"become interchangeable and the compiler says nothing",
					tc.name, rt.Name(), tc.want, tc.want)
			}
			if rt.PkgPath() != tc.pkgPath {
				t.Errorf("%s is declared in %q, want %q", tc.name, rt.PkgPath(), tc.pkgPath)
			}
			if rt.Kind() != reflect.String {
				t.Errorf("%s has kind %v, want string: the wire codes are strings and a "+
					"numeric dictionary would be a wire change on a guess", tc.name, rt.Kind())
			}
		})
	}

	// 2. The collision table. This is the documentation the file exists for, written
	// as an assertion so a change to either dictionary makes it fail.
	//
	// Seven codes are shared between the two surfaces, and every one of them means
	// something different on each. That is the hazard stated as data: a caller who
	// read a cash order's status and applied it to a master order gets the opposite
	// answer, and a caller who reached for types.EntrustTypeLimit sends "3", a code
	// the algo surface does not accept at all.
	//
	// Each row pairs the algo constant with the *term* the reference uses for it
	// (限价单, 已报, …) and the trade constant that holds the same code. The term is
	// matched as a substring of the algo constant's own doc comment, so a reworded
	// English gloss does not churn the table while a changed Chinese term — the
	// part the vendor's documentation is actually written in — does. The trade
	// constant is resolved through the parsed source rather than restated, so a
	// change to a trade code moves the assertion without touching this file.
	enums := algoTradeEnumSource(t)
	for _, tc := range []struct {
		algoConst  string
		algoCode   string
		algoTerm   string // the Chinese term the reference uses for this code
		tradeConst string
		tradeTerm  string
	}{
		{"AlgoEntrustTypeLimit", "1", "限价单", "EntrustTypeAuction", "竞价"},
		{"AlgoEntrustTypeMarket", "2", "市价单", "EntrustTypeEnhancedLimit", "增强限价盘"},
		{"AlgoStatusRegistered", "0", "已报", "EntrustStatusNoRegister", "未报"},
		{"AlgoStatusPartFilled", "1", "部分成交", "EntrustStatusWaitToRegister", "待报"},
		{"AlgoStatusFilled", "2", "全部成交", "EntrustStatusRegistered", "已报"},
		{"AlgoStatusCancelled", "4", "已撤", "EntrustStatusPartFilledWaitCancel", "部成待撤"},
		{"AlgoStatusRejected", "8", "废单", "EntrustStatusFilled", "已成"},
	} {
		t.Run(fmt.Sprintf("%s=%q is %s here and types.%s=%q is %s on trade",
			tc.algoConst, tc.algoCode, tc.algoTerm, tc.tradeConst, tc.algoCode, tc.tradeTerm),
			func(t *testing.T) {
				gotCode, gotDoc := algoCodeAndMeaning(t, tc.algoConst)
				if gotCode != tc.algoCode {
					t.Errorf("%s = %q, but this row records the collision on code %q. The "+
						"algo constant has moved, so the disagreement this table documents "+
						"has changed shape and the row is stale", tc.algoConst, gotCode, tc.algoCode)
				}
				if !strings.Contains(gotDoc, tc.algoTerm) {
					t.Errorf("%s is documented as %q, which does not mention %q. The table is "+
						"a record of the two dictionaries disagreeing about what this code "+
						"means, and a term that has moved is as stale as a code that has",
						tc.algoConst, gotDoc, tc.algoTerm)
				}
				tradeCode, ok := enums.values[tc.tradeConst]
				if !ok {
					t.Fatalf("test bug: pkg/types declares no constant %q", tc.tradeConst)
				}
				if tradeCode != tc.algoCode {
					t.Errorf("types.%s = %q, but this row records the collision on code %q. "+
						"The trade constant has moved, so the code that is shared between the "+
						"two surfaces is no longer shared and this row documents nothing",
						tc.tradeConst, tradeCode, tc.algoCode)
				}
				// The trade term is the one pkg/types' own comment carries, read
				// from the same parsed source, so the collision is asserted in both
				// directions rather than in one.
				tradeDoc := enums.docs[tc.tradeConst]
				if !strings.Contains(tradeDoc, tc.tradeTerm) {
					t.Errorf("types.%s is documented as %q, which does not mention %q, so the "+
						"collision this row claims is not the one the two surfaces have",
						tc.tradeConst, tradeDoc, tc.tradeTerm)
				}
			})
	}

	// 3. The forward-compatibility asymmetry. types.EntrustType promises that
	// unknown values are forwarded; AlgoEntrustType makes no such promise and
	// refuses them. A collapse would merge the two promises, and the surviving
	// promise would be the wrong one on the algo surface: an unlisted algo order
	// type is a request the SDK already knows it does not mean, on a mutation.
	//
	// Only trade codes *outside* the algo set can be asserted here. The two inside
	// it are the collision rows above, and asserting they are invalid would be
	// asserting the opposite of the truth.
	for _, trade := range []struct {
		name string
		v    types.EntrustType
	}{
		{"dark pool", types.EntrustTypeDarkPool},
		{"auction limit", types.EntrustTypeAuctionLimit},
		{"special limit", types.EntrustTypeSpecialLimit},
		{"iceberg market", types.EntrustTypeIcebergMarket},
		{"trailing stop market", types.EntrustTypeTrailingStopMarket},
		{"stop loss point", types.EntrustTypeStopLossLimit},
	} {
		t.Run("a trade-only order type is not an algo one: "+trade.name, func(t *testing.T) {
			if string(trade.v) == "1" || string(trade.v) == "2" {
				t.Fatalf("test bug: %q is a code the algo set also uses, so this row asserts "+
					"the opposite of the truth; the two shared codes are the collision rows "+
					"above", trade.v)
			}
			if AlgoEntrustType(trade.v).Valid() {
				t.Errorf("AlgoEntrustType(%q) is valid, but %q is a trade-surface order type "+
					"with no algo meaning. types.EntrustType is documented non-exhaustive with "+
					"unknown values forwarded; AlgoEntrustType is a closed two-code set, and "+
					"the two promises cannot both hold of one type", trade.v, trade.name)
			}
		})
	}

	// 4. The forward-compatibility asymmetry. types.EntrustType promises that
	// unknown values are forwarded; AlgoEntrustType makes no such promise and
	// refuses them. A collapse would merge the two promises, and the surviving
	// promise would be the wrong one on the algo surface: an unlisted algo order
	// type is a request the SDK already knows it does not mean, on a mutation.
	//
	// Only trade codes *outside* the algo set can be asserted here. The two inside
	// it — 1 and 2 — are the collision rows above, and asserting they are invalid
	// would be asserting the opposite of the truth.
	for _, trade := range []struct {
		name string
		v    types.EntrustType
	}{
		{"dark pool", types.EntrustTypeDarkPool},
		{"auction limit", types.EntrustTypeAuctionLimit},
		{"special limit", types.EntrustTypeSpecialLimit},
		{"iceberg market", types.EntrustTypeIcebergMarket},
		{"trailing stop market", types.EntrustTypeTrailingStopMarket},
		{"stop loss point", types.EntrustTypeStopLossLimit},
	} {
		t.Run("a trade-only order type is not an algo one: "+trade.name, func(t *testing.T) {
			if string(trade.v) == "1" || string(trade.v) == "2" {
				t.Fatalf("test bug: %q is a code the algo set also uses, so this row asserts "+
					"the opposite of the truth; the two shared codes are the collision rows "+
					"above", trade.v)
			}
			if AlgoEntrustType(trade.v).Valid() {
				t.Errorf("AlgoEntrustType(%q) is valid, but %q is a trade-surface order type "+
					"with no algo meaning. types.EntrustType is documented non-exhaustive with "+
					"unknown values forwarded; AlgoEntrustType is a closed two-code set, and "+
					"the two promises cannot both hold of one type", trade.v, trade.name)
			}
		})
	}

	// The trade sessionType is wider than the algo one, and the extra codes are the
	// conditional-order codes. A collapse onto the trade vocabulary would let a
	// caller send 3, 5 or 7 to an algorithm master order, which is not a
	// conditional order.
	for _, code := range []string{"3", "5", "7"} {
		t.Run("trade sessionType "+code+" is not an algo one", func(t *testing.T) {
			if AlgoSessionType(code).Valid() {
				t.Errorf("AlgoSessionType(%q) is valid, but the trade surface's sessionType "+
					"reserves %q for a conditional order and an algorithm master order is not "+
					"one. The Python SessionType class carries five codes and both vendors' algo "+
					"documentation lists two; the algo set is the two", code, code)
			}
		})
	}

	// Sensitivity has no namesake and is asserted here anyway: the point of the
	// whole file is that a caller can tell which vocabulary a code belongs to, and
	// an algo-only dictionary that is a bare string fails that just as surely as a
	// collapsed one.
	for _, tc := range []struct {
		name string
		v    AlgoSensitivity
		want bool
	}{
		{"neutral", AlgoSensitivityNeutral, true},
		{"aggressive", AlgoSensitivityAggressive, true},
		{"passive", AlgoSensitivityPassive, true},
		{"an undocumented code", AlgoSensitivity("4"), false},
		{"zero", AlgoSensitivity("0"), false},
		{"empty", AlgoSensitivity(""), false},
	} {
		t.Run("sensitivity "+tc.name, func(t *testing.T) {
			if got := tc.v.Valid(); got != tc.want {
				t.Errorf("AlgoSensitivity(%q).Valid() = %v, want %v", tc.v, got, tc.want)
			}
		})
	}

	// And the four-value action set, which the vendors and the released layer all
	// state identically, so it is closed and the assertion belongs here.
	for _, tc := range []struct {
		name string
		v    AlgoAction
		want bool
	}{
		{"start", AlgoActionStart, true},
		{"stop", AlgoActionStop, true},
		{"suspend", AlgoActionSuspend, true},
		{"resume", AlgoActionResume, true},
		{"a fifth code", AlgoAction("5"), false},
		{"zero", AlgoAction("0"), false},
		{"empty", AlgoAction(""), false},
	} {
		t.Run("action "+tc.name, func(t *testing.T) {
			if got := tc.v.Valid(); got != tc.want {
				t.Errorf("AlgoAction(%q).Valid() = %v, want %v", tc.v, got, tc.want)
			}
		})
	}

	// AlgoStrategyStatus shares Action's codes and is still a different type. The
	// code equality is asserted alongside the type distinction, because "they are
	// the same numbers" is exactly what makes collapsing them tempting.
	if AlgoStrategyStatusStart != AlgoStrategyStatus("1") || AlgoStrategyStatusResume != AlgoStrategyStatus("4") {
		t.Error("AlgoStrategyStatus does not carry the four codes the Java AlgoOrder " +
			"comment documents (1:START 2:STOP 3:SUSPEND 4:RESUME)")
	}
	if reflect.TypeOf(AlgoStrategyStatus("")).Name() == reflect.TypeOf(AlgoAction("")).Name() {
		t.Error("AlgoStrategyStatus and AlgoAction are the same type; they answer different " +
			"questions — what was asked on an action, what is running on a strategy status")
	}
}

// TestAlgoStrategyIsNotValueBounded pins the one algo field the SDK refuses to
// bound, so a future "consistency" edit that adds a Valid to it fails here rather
// than refusing a code the vendor's own documentation endorses.
//
// The reason is on domain.AlgoStrategy: the reference documents code 1005 under
// two names, so a closed set would reject a code the vendor documents. The test
// asserts both halves — the five documented codes are declared, and an
// undocumented one is carried — because a file that declared only the documented
// codes and refused everything else would pass a weaker version of this assertion.
func TestAlgoStrategyIsNotValueBounded(t *testing.T) {
	if AlgoStrategyVWAP != "1" || AlgoStrategyTWAP != "1001" || AlgoStrategyIceberg != "1002" ||
		AlgoStrategyTPOV != "1003" || AlgoStrategyPOV != "1005" {
		t.Errorf("the documented strategy codes moved: VWAP=%q TWAP=%q ICE_BERG=%q TPOV=%q POV=%q, "+
			"want 1/1001/1002/1003/1005. The released layer and both vendors agree on all five",
			AlgoStrategyVWAP, AlgoStrategyTWAP, AlgoStrategyIceberg, AlgoStrategyTPOV, AlgoStrategyPOV)
	}
	// The set is deliberately open, and this is the assertion that says so. A
	// strategy outside the documented five is representable and carried, because
	// the SDK forwards it unchanged; an empty string is the only value refused.
	if unknown := AlgoStrategy("9999"); unknown == "" {
		t.Error("an unknown strategy code must be representable, because the SDK forwards " +
			"it unchanged; an empty string is the only value it refuses")
	}
}

// TestAlgoActionString covers the one exported method that renders a code back to
// a human name. Both directions matter: a recognised code must render its
// documented name, and an unrecognised one must render something that says what
// the code was rather than an empty string, because a log line built from it is
// the only record of what was sent.
func TestAlgoActionString(t *testing.T) {
	for _, tc := range []struct {
		in   AlgoAction
		want string
	}{
		{AlgoActionStart, "START"},
		{AlgoActionStop, "STOP"},
		{AlgoActionSuspend, "SUSPEND"},
		{AlgoActionResume, "RESUME"},
		{algoActionUndocumented, "AlgoAction(9)"},
		{algoActionEmpty, "AlgoAction()"},
	} {
		t.Run(string(tc.in), func(t *testing.T) {
			if got := tc.in.String(); got != tc.want {
				t.Errorf("AlgoAction(%q).String() = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// algoActionUndocumented and algoActionEmpty are named so the table above reads as
// cases rather than as literals inline.
const (
	algoActionUndocumented = AlgoAction("9")
	algoActionEmpty        = AlgoAction("")
)

// ---------------------------------------------------------------------------
// Mappers
// ---------------------------------------------------------------------------

// TestAlgoMasterOrderFromDTOTheMockFixture is the positive path: the mock's own
// payload decodes into a model whose every field is asserted, including the four
// decimal types and the four algo dictionaries.
//
// It asserts each field separately rather than comparing the whole struct, because
// a whole-struct comparison reports "not equal" and leaves the reader to work out
// which of seventeen fields moved.
func TestAlgoMasterOrderFromDTOTheMockFixture(t *testing.T) {
	var wire AlgoMasterOrderWire
	if err := json.Unmarshal([]byte(algoMasterOrderBody), &wire); err != nil {
		t.Fatalf("decoding the mock's master-order row: %v", err)
	}
	order := AlgoMasterOrderFromDTO(&wire)
	if order == nil {
		t.Fatal("AlgoMasterOrderFromDTO returned nil for a populated row")
	}

	if order.OrderID != OrderID("MA20260921001") {
		t.Errorf("OrderID = %q, want MA20260921001", order.OrderID)
	}
	if order.StockCode != "00700.HK" {
		t.Errorf("StockCode = %q, want 00700.HK", order.StockCode)
	}
	if order.ExchangeType != types.ExchangeHK {
		t.Errorf("ExchangeType = %q, want %q", order.ExchangeType, types.ExchangeHK)
	}
	if order.TradeDate != "20260921" {
		t.Errorf("TradeDate = %q, want 20260921", order.TradeDate)
	}
	if order.EntrustType != AlgoEntrustTypeLimit {
		t.Errorf("EntrustType = %q, want %q: the algo dictionary's limit order, not "+
			"types.EntrustTypeLimit which is a different code entirely",
			order.EntrustType, AlgoEntrustTypeLimit)
	}
	// The wire carries "388.00" and "388.10"; the canonical decimal rendering of
	// those is "388" and "388.1". The trailing zeros are a property of how the
	// Gateway chose to pad the field, not information, and every decimal type in
	// this package normalises them — the same is true of the futures layer, where
	// the mock's "1000000.00" is asserted as "1000000". What must survive
	// byte for byte is the *value*, and the hostile-value table below is what pins
	// that: a rendering that dropped a significant digit would fail there.
	algoRequirePrice(t, "EntrustPrice", order.EntrustPrice, "388")
	algoRequireQuantity(t, "EntrustAmount", order.EntrustAmount, "1000")
	algoRequireQuantity(t, "CumQty", order.CumQty, "200")
	algoRequireQuantity(t, "LeavesQty", order.LeavesQty, "800")
	// The status is the algo set, so 1 means 部分成交 here and 待报 on the cash
	// surface. That inversion is the reason the type is separate.
	if order.Status != AlgoStatusPartFilled {
		t.Errorf("Status = %q, want %q: the algo set documents 1 as 部分成交, and "+
			"types.EntrustStatusWaitToRegister also holds 1 with the opposite meaning",
			order.Status, AlgoStatusPartFilled)
	}
	if order.EntrustBS != types.EntrustBuy {
		t.Errorf("EntrustBS = %q, want %q", order.EntrustBS, types.EntrustBuy)
	}
	if order.TargetStrategy != AlgoStrategyVWAP {
		t.Errorf("TargetStrategy = %q, want %q", order.TargetStrategy, AlgoStrategyVWAP)
	}
	if order.StrategyStatus != AlgoStrategyStatusStart {
		t.Errorf("StrategyStatus = %q, want %q", order.StrategyStatus, AlgoStrategyStatusStart)
	}
	algoRequireQuantity(t, "RoundLot", order.RoundLot, "100")
	if order.SendingTime != "2026-09-21 09:30:00" {
		t.Errorf("SendingTime = %q", order.SendingTime)
	}
	if order.TransactionTime != "2026-09-21 09:30:01" {
		t.Errorf("TransactionTime = %q", order.TransactionTime)
	}
	algoRequirePrice(t, "AvgPx", order.AvgPx, "388.1")

	// The nested tuning object, field by field: the four decimals and the
	// aggressiveness, plus the two times and the interval.
	algoRequireQuantity(t, "StrategyParam.MaxVolume", order.StrategyParam.MaxVolume, "100")
	algoRequireMoney(t, "StrategyParam.MinAmount", order.StrategyParam.MinAmount, "10000", "HKD")
	if order.StrategyParam.Sensitivity != AlgoSensitivityNeutral {
		t.Errorf("StrategyParam.Sensitivity = %q, want %q", order.StrategyParam.Sensitivity, AlgoSensitivityNeutral)
	}
	// showQty is absent from the fixture, so it is the zero quantity — an absent
	// field, not a missing one.
	algoRequireQuantity(t, "StrategyParam.ShowQty", order.StrategyParam.ShowQty, "0")
	algoRequireRate(t, "StrategyParam.QtyPercent", order.StrategyParam.QtyPercent, "10")
	if order.StrategyParam.OrigStartTime != "093000" || order.StrategyParam.OrigEndTime != "160000" {
		t.Errorf("the strategy window = %q-%q, want 093000-160000",
			order.StrategyParam.OrigStartTime, order.StrategyParam.OrigEndTime)
	}
	if order.StrategyParam.Interval != 60 {
		t.Errorf("StrategyParam.Interval = %d, want 60: the wire carries it as a bare number "+
			"while every other member is a quoted string, and a wire type that typed it as a "+
			"string would fail to decode this fixture", order.StrategyParam.Interval)
	}
}

// TestAlgoMasterOrderFromDTOOnAnAbsentRow is the fail-safe half of the mapping
// contract, and it is the reason the four value wrappers exist.
//
// A partial reply is a read, not a corrupt input, and a read must not crash the
// caller's process. MustNewMoney, MustNewPrice, MustNewQuantity and MustNewRate all
// panic on a string they cannot parse, and the empty string is one — so without the
// wrappers an absent field would take the process down. The fixture is written by
// hand for this reason: deriving it from the full fixture would let an edit to the
// full fixture remove the field under test.
//
// There is no recover() anywhere in this file. A fixture that needed one to pass
// would be asserting the wrong thing — that a panic is survivable — and the
// wrappers are what make it unnecessary.
func TestAlgoMasterOrderFromDTOOnAnAbsentRow(t *testing.T) {
	var wire AlgoMasterOrderWire
	if err := json.Unmarshal([]byte(algoMasterOrderBareBody), &wire); err != nil {
		t.Fatalf("decoding the bare master-order row: %v", err)
	}
	order := AlgoMasterOrderFromDTO(&wire)
	if order == nil {
		t.Fatal("AlgoMasterOrderFromDTO returned nil for a row with absent decimals")
	}
	algoRequirePrice(t, "EntrustPrice", order.EntrustPrice, "0")
	algoRequireQuantity(t, "EntrustAmount", order.EntrustAmount, "0")
	algoRequireQuantity(t, "CumQty", order.CumQty, "0")
	algoRequireQuantity(t, "LeavesQty", order.LeavesQty, "0")
	algoRequireQuantity(t, "RoundLot", order.RoundLot, "0")
	algoRequirePrice(t, "AvgPx", order.AvgPx, "0")
	algoRequireQuantity(t, "StrategyParam.MaxVolume", order.StrategyParam.MaxVolume, "0")
	algoRequireMoney(t, "StrategyParam.MinAmount", order.StrategyParam.MinAmount, "0", algoBaseCurrency)
	algoRequireQuantity(t, "StrategyParam.ShowQty", order.StrategyParam.ShowQty, "0")
	algoRequireRate(t, "StrategyParam.QtyPercent", order.StrategyParam.QtyPercent, "0")
	if order.StrategyParam.Interval != 0 {
		t.Errorf("StrategyParam.Interval = %d, want 0", order.StrategyParam.Interval)
	}
	// The codes survive verbatim, including the empty ones, because they are
	// dictionary values rather than decimals and are not validated inbound.
	if order.EntrustType != AlgoEntrustTypeMarket {
		t.Errorf("EntrustType = %q, want the verbatim %q", order.EntrustType, AlgoEntrustTypeMarket)
	}
	if order.EntrustBS != types.EntrustBS("") {
		t.Errorf("EntrustBS = %q, want the verbatim empty code", order.EntrustBS)
	}
}

// TestAlgoMappersRejectNil is the other half of the mapping contract: a nil row
// yields a nil model and a nil tuning yields the zero tuning, so a caller
// iterating a reply whose list is absent never has to check before the call.
//
// It is asserted for every mapper in this file, including the one that takes a
// pointer and returns a value, because a mapper that handled nil for its siblings
// and not for itself would be a nil dereference on exactly the reply shape that
// produces it.
func TestAlgoMappersRejectNil(t *testing.T) {
	if got := AlgoMasterOrderFromDTO(nil); got != nil {
		t.Errorf("AlgoMasterOrderFromDTO(nil) = %#v, want nil: a mapper here is safe to call "+
			"over a reply whose list is absent", got)
	}
	if got := AlgoStrategyParamFromDTO(nil); got != (AlgoStrategyParam{}) {
		t.Errorf("AlgoStrategyParamFromDTO(nil) = %#v, want the zero tuning: the field is "+
			"nested inside a row, so there is no nil to propagate and a value keeps the "+
			"caller's field non-nilable", got)
	}
}

// TestAlgoUndocumentedReplyCodesAreCarriedNotRejected pins the decision on
// AlgoMasterOrderFromDTO not to validate inbound dictionaries.
//
// The reasoning is on AlgoMasterOrder: turning a vendor-side surprise into a decode
// error for the caller is a different policy from validating an outbound request,
// so a code outside the published set is carried through in its named type and the
// caller switches on what it recognises. The test asserts the *carry*, and that a
// downstream switch simply falls through — because a mapper that rejected the code
// would have taken the process down instead.
func TestAlgoUndocumentedReplyCodesAreCarriedNotRejected(t *testing.T) {
	var wire AlgoMasterOrderWire
	if err := json.Unmarshal([]byte(algoMasterOrderBody), &wire); err != nil {
		t.Fatalf("decoding the mock row: %v", err)
	}
	wire.Status = "7"
	wire.StrategyStatus = "9"
	wire.EntrustType = "3"
	wire.TargetStrategy = "9999"

	order := AlgoMasterOrderFromDTO(&wire)
	if order.Status != AlgoStatus("7") {
		t.Errorf("Status = %q, want the verbatim 7: 7 is a gap in the published set, and a "+
			"reply is not validated inbound", order.Status)
	}
	if order.StrategyStatus != AlgoStrategyStatus("9") {
		t.Errorf("StrategyStatus = %q, want the verbatim 9", order.StrategyStatus)
	}
	if order.EntrustType != AlgoEntrustType("3") {
		t.Errorf("EntrustType = %q, want the verbatim 3: 3 is types.EntrustTypeLimit, so a "+
			"caller must not read it as an algo order type, and the mapper must not "+
			"rewrite a reply to make it look like one", order.EntrustType)
	}
	if order.TargetStrategy != AlgoStrategy("9999") {
		t.Errorf("TargetStrategy = %q, want the verbatim 9999: an unknown strategy is "+
			"forwarded in both directions, which is why AlgoStrategy has no Valid",
			order.TargetStrategy)
	}
	// The carry is useful only if a caller can act on it, so the switch shape is
	// exercised: a recognised code takes its branch and an unrecognised one falls
	// through rather than panicking.
	ran := false
	switch order.Status {
	case AlgoStatusRegistered, AlgoStatusFilled, AlgoStatusCancelled:
		ran = true
	}
	if ran {
		t.Error("an undocumented status matched a documented branch, so the carried code is " +
			"not what the fixture set")
	}
}

// TestAlgoPricesCarryTheZeroTick is the static half of the tick decision.
//
// It asserts the constant is "0" and that every price the mappers build carries
// it, so a future edit that starts measuring algo prices against a grid this SDK
// invented fails here. The check is written as a round trip through Price.Tick
// rather than as a comparison against a literal in this file, so the assertion and
// the constant cannot drift apart by being edited in one place.
func TestAlgoPricesCarryTheZeroTick(t *testing.T) {
	if algoZeroTick != "0" {
		t.Fatalf("algoZeroTick = %q, want \"0\": the tick note's rule for a price whose "+
			"instrument schedule is not known. I observed this price, I do not know this "+
			"product's tick", algoZeroTick)
	}
	var wire AlgoMasterOrderWire
	if err := json.Unmarshal([]byte(algoMasterOrderBody), &wire); err != nil {
		t.Fatalf("decoding the mock row: %v", err)
	}
	order := AlgoMasterOrderFromDTO(&wire)
	for _, p := range []struct {
		label string
		price Price
	}{
		{"EntrustPrice", order.EntrustPrice},
		{"AvgPx", order.AvgPx},
	} {
		if !p.price.Tick().IsZero() {
			t.Errorf("%s carries tick %s, want the zero tick: no algo row documents a tick "+
				"schedule, so a non-zero tick would measure this price against a grid this "+
				"SDK made up", p.label, p.price.Tick())
		}
		if got := p.price.Round().String(); got != p.price.String() {
			t.Errorf("%s.Round() changed %s to %s, so the tick is not the zero tick and a "+
				"caller's price would be rounded on the way through", p.label, p.price, got)
		}
	}
}

// ---------------------------------------------------------------------------
// Money
// ---------------------------------------------------------------------------

// The three frozen value sets and their two guards — moneyCase,
// float64HostilePrices, float64HostileQuantities, scaleHostilePrices,
// requireFloat64Hostile and requireScaleHostile — are **not** restated here.
// pkg/domain/futures_test.go declares them in this same package, so redeclaring
// them is a compile error that would take every other test in the package down
// with it; they are the same declarations, not a per-file copy, and that is
// strictly better than the restatement the cross-package files in this repository
// are forced into. TestAlgoMoneyValueSetsAreStillHostile below re-asserts the
// guards anyway, so an edit to the futures file cannot silently disarm this
// file's hosts.

// algoMoneySet is one of the frozen value sets, with the guard that states which
// failure mode it is chosen to detect.
type algoMoneySet struct {
	label string
	cases []moneyCase
	guard func(*testing.T, string)
}

// algoMoneySets returns both sets, applied to every host below rather than to
// whichever one a given host happened to get. A value that discriminates a float64
// is often blind to a fixed-scale formatter and vice versa, so applying only one
// set to a host would leave half the regression net unbuilt there.
func algoMoneySets() []algoMoneySet {
	return []algoMoneySet{
		{"float64-hostile", float64HostilePrices, requireFloat64Hostile},
		{"scale-hostile", scaleHostilePrices, requireScaleHostile},
	}
}

// algoMoneyCasesFor returns the value sets a host is driven with. A quantity host
// gets the 27-digit integer, which is its float64-hostile analogue; 1e-330 is a
// price-shaped value and a quantity is not.
func algoMoneyCasesFor(quantity bool) []algoMoneySet {
	if quantity {
		return []algoMoneySet{
			{"float64-hostile-quantity", float64HostileQuantities, requireFloat64Hostile},
		}
	}
	return algoMoneySets()
}

// TestAlgoMoneyValueSetsAreStillHostile is the guard on the guard. Each row
// asserts the property that makes it worth having, so a future edit that softened a
// literal into a benign one deletes the regression net below without any test
// noticing. The same rows are asserted in pkg/services/algo_test.go for the request
// side, and again in testsupport_test.go for the cash side; it is repeated in all
// three so no file's hosts are silently disarmed by an edit to another.
func TestAlgoMoneyValueSetsAreStillHostile(t *testing.T) {
	if len(float64HostilePrices) == 0 || len(float64HostileQuantities) == 0 || len(scaleHostilePrices) == 0 {
		t.Fatal("a money value set is empty, so the round-trip tests in this file would pass " +
			"vacuously")
	}
	for _, set := range algoMoneySets() {
		t.Run(set.label, func(t *testing.T) {
			for _, tc := range set.cases {
				t.Run(tc.name, func(t *testing.T) {
					set.guard(t, tc.in)
					if tc.want == "" {
						t.Fatalf("row %q has no want value", tc.name)
					}
				})
			}
		})
	}
	t.Run("float64-hostile-quantities", func(t *testing.T) {
		for _, tc := range float64HostileQuantities {
			t.Run(tc.name, func(t *testing.T) {
				requireFloat64Hostile(t, tc.in)
			})
		}
	})
}

// algoMoneyHost is one decimal-bearing field on the master-order row.
//
// It is a table rather than a loop over a field list because the fields are not
// interchangeable: a price host and a quantity host take different value sets, and
// the currency of a money host is part of what is asserted. read returns the mapped
// value's digits so a row cannot assert a field the mapper does not produce.
type algoMoneyHost struct {
	name string
	// build returns a master-order row with the given literal in the targeted
	// field. It is a function so the money sets can drive it without a fixture per
	// field, and so an assertion about a field the row does not carry fails here
	// rather than silently reading a different field's value.
	build func(t *testing.T, literal string) *AlgoMasterOrderWire
	// read returns the mapped value's digits.
	read func(t *testing.T, order *AlgoMasterOrder) string
	// currency is non-empty for a money host, and is asserted alongside the amount
	// because an amount attributed to the wrong currency is a wrong amount that
	// still adds up.
	currency string
	// quantity is true for a field the 27-digit integer set is aimed at.
	quantity bool
	// nested is true for a field inside strategyParam rather than on the row.
	nested bool
}

// algoMoneyHosts is the table. It covers all four decimal types the algo mappers
// produce — Price, Quantity, Money, Rate — and every money-bearing field on the row
// and in the nested tuning object, so no field is money-tested only by proxy
// through another. The count is asserted, so a field added to the row without a row
// here fails.
func algoMoneyHosts() []algoMoneyHost {
	build := func(field string) func(t *testing.T, literal string) *AlgoMasterOrderWire {
		return func(t *testing.T, literal string) *AlgoMasterOrderWire {
			t.Helper()
			var wire AlgoMasterOrderWire
			if err := json.Unmarshal([]byte(algoMasterOrderBody), &wire); err != nil {
				t.Fatalf("test bug: decoding the mock row: %v", err)
			}
			if nested := strings.HasPrefix(field, "strategyParam."); nested {
				member := strings.TrimPrefix(field, "strategyParam.")
				if !algoStrategyParamWireHas(member) {
					t.Fatalf("test bug: strategyParam has no %q member", member)
				}
				algoSetStrategyParamMember(t, &wire, member, literal)
			} else if !algoRowHas(field) {
				t.Fatalf("test bug: the master-order row has no %q field; the money host "+
					"table and the wire struct have drifted apart", field)
			} else {
				algoSetRowMember(t, &wire, field, literal)
			}
			return &wire
		}
	}

	return []algoMoneyHost{
		{
			name: "entrustPrice", build: build("entrustPrice"),
			read: func(t *testing.T, o *AlgoMasterOrder) string {
				t.Helper()
				return o.EntrustPrice.String()
			},
		},
		{
			name: "entrustAmount", build: build("entrustAmount"), quantity: true,
			read: func(t *testing.T, o *AlgoMasterOrder) string {
				t.Helper()
				return o.EntrustAmount.String()
			},
		},
		{
			name: "cumQty", build: build("cumQty"), quantity: true,
			read: func(t *testing.T, o *AlgoMasterOrder) string {
				t.Helper()
				return o.CumQty.String()
			},
		},
		{
			name: "leavesQty", build: build("leavesQty"), quantity: true,
			read: func(t *testing.T, o *AlgoMasterOrder) string {
				t.Helper()
				return o.LeavesQty.String()
			},
		},
		{
			name: "avgPx", build: build("avgPx"),
			read: func(t *testing.T, o *AlgoMasterOrder) string {
				t.Helper()
				return o.AvgPx.String()
			},
		},
		{
			name: "roundLot", build: build("roundLot"), quantity: true,
			read: func(t *testing.T, o *AlgoMasterOrder) string {
				t.Helper()
				return o.RoundLot.String()
			},
		},
		{
			name: "strategyParam.maxVolume", build: build("strategyParam.maxVolume"),
			quantity: true, nested: true,
			read: func(t *testing.T, o *AlgoMasterOrder) string {
				t.Helper()
				return o.StrategyParam.MaxVolume.String()
			},
		},
		{
			name: "strategyParam.minAmount", build: build("strategyParam.minAmount"),
			currency: "HKD", nested: true,
			read: func(t *testing.T, o *AlgoMasterOrder) string {
				t.Helper()
				return o.StrategyParam.MinAmount.String()
			},
		},
		{
			name: "strategyParam.showQty", build: build("strategyParam.showQty"),
			quantity: true, nested: true,
			read: func(t *testing.T, o *AlgoMasterOrder) string {
				t.Helper()
				return o.StrategyParam.ShowQty.String()
			},
		},
		{
			name: "strategyParam.qtyPercent", build: build("strategyParam.qtyPercent"),
			nested: true,
			read: func(t *testing.T, o *AlgoMasterOrder) string {
				t.Helper()
				return o.StrategyParam.QtyPercent.String()
			},
		},
	}
}

// TestAlgoReplyMoneyIsVerbatim drives every money host over every value set it is
// aimed at, and asserts the exact decimal the Gateway sent survives the decode and
// the mapping.
//
// The assertion is on the mapped value's digits, not on the struct, so a mapping
// that rounded, scaled or dropped a sign fails here. The per-field table is what
// makes it more than an aggregate: an aggregate assertion over "some price on the
// row" would pass with two fields swapped.
func TestAlgoReplyMoneyIsVerbatim(t *testing.T) {
	hosts := algoMoneyHosts()
	if len(hosts) != 10 {
		t.Fatalf("the money host table has %d hosts, want the ten money-bearing fields the "+
			"master-order row and its nested tuning carry", len(hosts))
	}
	for _, host := range hosts {
		t.Run(host.name, func(t *testing.T) {
			for _, set := range algoMoneyCasesFor(host.quantity) {
				t.Run(set.label, func(t *testing.T) {
					for _, tc := range set.cases {
						t.Run(tc.name, func(t *testing.T) {
							// The canonical form the mapper must produce is the
							// decimal's own String(), which for these inputs is
							// tc.want. Assert that first, so a test bug in the set
							// is visible before the mapper runs.
							algoAssertCanonical(t, host, tc)
							order := AlgoMasterOrderFromDTO(host.build(t, tc.in))
							if order == nil {
								t.Fatalf("AlgoMasterOrderFromDTO returned nil for a row "+
									"carrying %q", tc.in)
							}
							algoAssertVerbatim(t, host.name, host.read(t, order), tc)
						})
					}
				})
			}
		})
	}
}

// algoAssertCanonical asserts that the expected digits are what the domain
// constructor renders, before the mapper is asked for them. It separates a test
// bug in a value set from a bug in the code, which is the difference between a
// failure that names the fixture and one that names the mapper.
func algoAssertCanonical(t *testing.T, host algoMoneyHost, tc moneyCase) {
	t.Helper()
	var got string
	switch {
	case host.currency != "":
		got = MustNewMoney(tc.want, host.currency, algoMoneyScale).String()
	case host.quantity:
		got = MustNewQuantity(tc.want).String()
	case host.nested && strings.Contains(host.name, "qtyPercent"):
		got = MustNewRate(tc.want).String()
	default:
		got = MustNewPrice(tc.want, algoZeroTick).String()
	}
	if got != tc.want {
		t.Fatalf("test bug: the constructor renders %q as %q, so the expected digits are "+
			"not tc.want and this row would fail for a reason that has nothing to do with "+
			"the mapper", tc.want, got)
	}
}

// algoAssertVerbatim is the central money assertion: the mapped value reports the
// exact digits the Gateway sent, and the failure message names the two conversions
// that would have destroyed them.
func algoAssertVerbatim(t *testing.T, field, got string, tc moneyCase) {
	t.Helper()
	if got == tc.want {
		return
	}
	t.Errorf("%s = %q, want %q (the literal the Gateway sent).\n"+
		"  input literal:          %s\n"+
		"  a float64 round trip:   %s\n"+
		"  the removed %%.3f form: %s",
		field, got, tc.want, tc.in,
		strconv.FormatFloat(algoMustFloat(t, tc.in), 'f', -1, 64),
		legacyRounded(algoMustFloat(t, tc.in)))
}

// TestAlgoTheUnderflowingValueIsPositiveNotZero is the one row that states the
// property the 1e-330 case exists for: the value survives as a *positive* decimal,
// so a caller reading it is not handed a zero for a real price.
//
// It is separate from the verbatim table because the verbatim assertion would
// pass on a value that had been zeroed and re-rendered as "0.000…1" by some
// accident, and the sign is the thing a caller acts on.
func TestAlgoTheUnderflowingValueIsPositiveNotZero(t *testing.T) {
	for _, tc := range float64HostilePrices {
		if tc.in != "1e-330" {
			continue
		}
		t.Run("price", func(t *testing.T) {
			price := MustNewPrice(tc.in, algoZeroTick)
			if price.IsZero() {
				t.Fatalf("MustNewPrice(%q) is zero: the value is positive as a decimal and "+
					"zero to every float64, so a zero here means a real price was turned "+
					"into nothing", tc.in)
			}
			if price.IsNegative() {
				t.Errorf("MustNewPrice(%q) is negative, want positive", tc.in)
			}
			if got := price.String(); got != tc.want {
				t.Errorf("String() = %q, want the positional expansion %q", got, tc.want)
			}
			if err := price.Validate(false); err != nil {
				t.Errorf("Validate(false) = %v, want nil: a positive decimal is a valid price", err)
			}
		})
		t.Run("quantity", func(t *testing.T) {
			qty := MustNewQuantity(tc.in)
			if !qty.IsPositive() {
				t.Errorf("MustNewQuantity(%q) is not positive: the value is positive as a "+
					"decimal and zero to every float64", tc.in)
			}
		})
	}
}

// TestAlgoDomainTypesCarryNoFloat is the static half of hard rule 3 for the algo
// domain layer: no field of the wire row, the model, or the nested tuning is a
// float, whatever it is called.
//
// scripts/check_money.py guards by field name and by wire key, which is the right
// heuristic for the generated DTOs and blind to a decimal field given an innocuous
// name. This walks the three structs and asserts no field is a float at all, and
// that every decimal-bearing one is one of the four domain decimal types — which is
// the positive form of the same rule, and is what catches a field left as a bare
// string where the model is supposed to be typed.
func TestAlgoDomainTypesCarryNoFloat(t *testing.T) {
	for _, v := range []any{
		AlgoMasterOrderWire{},
		AlgoStrategyParamWire{},
		AlgoMasterOrder{},
		AlgoStrategyParam{},
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
						"wire as a quoted decimal string and is read into a domain decimal "+
						"type (docs/DESIGN.md §7, hard rule 3)", rt.Name(), f.Name, f.Type)
				}
			}
			if fields == 0 {
				t.Errorf("%s has no fields, so this test would pass vacuously", rt.Name())
			}
		})
	}

	// The positive form, on the model: each decimal-bearing field must be one of
	// the four domain types. A field left as a bare string would decode and print
	// plausibly while being un-addable and un-checkable.
	for _, tc := range []struct {
		field string
		typ   reflect.Type
		want  reflect.Type
	}{
		{"EntrustPrice", reflect.TypeOf(Price{}), reflect.TypeOf(Price{})},
		{"AvgPx", reflect.TypeOf(Price{}), reflect.TypeOf(Price{})},
		{"EntrustAmount", reflect.TypeOf(Quantity{}), reflect.TypeOf(Quantity{})},
		{"CumQty", reflect.TypeOf(Quantity{}), reflect.TypeOf(Quantity{})},
		{"LeavesQty", reflect.TypeOf(Quantity{}), reflect.TypeOf(Quantity{})},
		{"RoundLot", reflect.TypeOf(Quantity{}), reflect.TypeOf(Quantity{})},
		{"StrategyParam", reflect.TypeOf(AlgoStrategyParam{}), reflect.TypeOf(AlgoStrategyParam{})},
	} {
		t.Run("model field type "+tc.field, func(t *testing.T) {
			f, ok := reflect.TypeOf(AlgoMasterOrder{}).FieldByName(tc.field)
			if !ok {
				t.Fatalf("AlgoMasterOrder has no field %q", tc.field)
			}
			if f.Type != tc.want {
				t.Errorf("AlgoMasterOrder.%s is a %s, want the domain type %s", tc.field, f.Type, tc.want)
			}
		})
	}
	for _, tc := range []struct {
		field string
		want  reflect.Type
	}{
		{"MaxVolume", reflect.TypeOf(Quantity{})},
		{"MinAmount", reflect.TypeOf(Money{})},
		{"ShowQty", reflect.TypeOf(Quantity{})},
		{"QtyPercent", reflect.TypeOf(Rate{})},
	} {
		t.Run("tuning field type "+tc.field, func(t *testing.T) {
			f, ok := reflect.TypeOf(AlgoStrategyParam{}).FieldByName(tc.field)
			if !ok {
				t.Fatalf("AlgoStrategyParam has no field %q", tc.field)
			}
			if f.Type != tc.want {
				t.Errorf("AlgoStrategyParam.%s is a %s, want the domain type %s", tc.field, f.Type, tc.want)
			}
		})
	}
}

// TestAlgoDomainDeclaresNoEnvelope re-asserts, for this file's additions, the rule
// pkg/domain/futures_test.go's TestDomainDeclaresNoEnvelope asserts for the whole
// package: an envelope is an exported struct carrying a row slice together with
// the Gateway's page counters, and it belongs to pkg/services because it is the
// layer that decodes it.
//
// The algo surface is the one place this rule could plausibly be broken, because
// the master-order reply *is* a list and a reader might reach for a
// AlgoOrderPage in this package. The local half of the assertion is here so a
// failure names algo rather than an unrelated file; the package-wide half is
// TestDomainDeclaresNoEnvelope, which scans every file.
func TestAlgoDomainDeclaresNoEnvelope(t *testing.T) {
	source, err := os.ReadFile("algo.go")
	if err != nil {
		t.Fatalf("reading algo.go: %v", err)
	}
	counters := []string{"curPageNo", "curPageSize", "totalPageNo", "totalPages", "lastPage"}
	for _, decl := range exportedStructDecls(string(source)) {
		sliceOfRows := strings.Contains(decl.body, "[]") &&
			(strings.Contains(decl.body, "Wire") || strings.Contains(decl.body, "data"))
		carriesCounter := false
		for _, counter := range counters {
			if strings.Contains(decl.body, counter) {
				carriesCounter = true
			}
		}
		if sliceOfRows && carriesCounter {
			t.Errorf("algo.go declares exported type %s, which carries a row slice together "+
				"with the Gateway's page counters. That is an envelope, and an envelope belongs "+
				"to the service layer that decodes it", decl.name)
		}
	}
	// The named half, which the shape check above cannot report. The algo envelope
	// must be declared by the service layer and the row by this one; a name in the
	// wrong package is a decoder with no decoder, which is the mistake in its
	// smallest form.
	if strings.Contains(string(source), "AlgoOrderListWireResponse") ||
		strings.Contains(string(source), "AlgoEntrustIDListWireResponse") {
		t.Error("algo.go declares a reply envelope. The master-order list and the child-ID " +
			"list are envelopes — the layer that decodes them is pkg/services — and a type " +
			"here with no decoder in its own package is the mistake C3b removed")
	}
	if !strings.Contains(string(source), "AlgoMasterOrderWire") {
		t.Error("algo.go does not declare AlgoMasterOrderWire. The row is this package's: it " +
			"is what AlgoMasterOrderFromDTO takes, and an envelope holding it belongs to " +
			"pkg/services")
	}
}

// ---------------------------------------------------------------------------
// Helpers for the money host table
// ---------------------------------------------------------------------------

// algoRowHas reports whether the master-order row carries a named JSON field. It
// is a shape check on the struct's tags, so a money host naming a field the row
// does not have fails here with a message that names the drift.
func algoRowHas(field string) bool {
	rt := reflect.TypeOf(AlgoMasterOrderWire{})
	for i := 0; i < rt.NumField(); i++ {
		if key, _, _ := strings.Cut(rt.Field(i).Tag.Get("json"), ","); key == field {
			return true
		}
	}
	return false
}

// algoStrategyParamWireHas reports whether the nested tuning object carries a named
// JSON field, for the same reason algoRowHas exists.
func algoStrategyParamWireHas(field string) bool {
	rt := reflect.TypeOf(AlgoStrategyParamWire{})
	for i := 0; i < rt.NumField(); i++ {
		if key, _, _ := strings.Cut(rt.Field(i).Tag.Get("json"), ","); key == field {
			return true
		}
	}
	return false
}

// algoSetRowMember rewrites one quoted-string field of a master-order row and
// returns the row, failing the test if the field is not a string or is absent.
//
// It is a switch rather than a reflection assignment so that a field's type is
// part of what is asserted: a wire field that had been retyped away from string
// cannot be rewritten this way, so the table cannot silently stop testing it.
func algoSetRowMember(t *testing.T, wire *AlgoMasterOrderWire, field, value string) {
	t.Helper()
	switch field {
	case "entrustPrice":
		wire.EntrustPrice = value
	case "entrustAmount":
		wire.EntrustAmount = value
	case "cumQty":
		wire.CumQty = value
	case "leavesQty":
		wire.LeavesQty = value
	case "avgPx":
		wire.AvgPx = value
	case "roundLot":
		wire.RoundLot = value
	default:
		t.Fatalf("test bug: algoSetRowMember has no case for %q; the host table and this "+
			"switch have drifted apart", field)
	}
}

// algoSetStrategyParamMember rewrites one quoted-string field of the nested tuning
// object, for the reason algoSetRowMember states.
func algoSetStrategyParamMember(t *testing.T, wire *AlgoMasterOrderWire, field, value string) {
	t.Helper()
	switch field {
	case "maxVolume":
		wire.StrategyParam.MaxVolume = value
	case "minAmount":
		wire.StrategyParam.MinAmount = value
	case "showQty":
		wire.StrategyParam.ShowQty = value
	case "qtyPercent":
		wire.StrategyParam.QtyPercent = value
	default:
		t.Fatalf("test bug: algoSetStrategyParamMember has no case for %q; the host table "+
			"and this switch have drifted apart", field)
	}
}

// algoMustFloat parses in for a failure message and fails the test if it cannot,
// so a value a float could not represent reports that rather than panicking inside
// a failure path.
func algoMustFloat(t *testing.T, in string) float64 {
	t.Helper()
	f, err := strconv.ParseFloat(in, 64)
	if err != nil {
		t.Fatalf("test bug: %q does not parse as a float: %v", in, err)
	}
	return f
}

// algoCodeAndMeaning returns the literal and the documented meaning of a constant
// declared in this package's own algo.go.
//
// It reads the source text rather than referring to the constant, for the same
// reason algoTradeEnumSource reads pkg/types/enums.go: the collision table is a
// record of what each surface calls each code, and a record that cannot drift is
// one that does not have to be maintained by hand. The meaning comes from the
// constant's own doc comment — the line *above* the declaration, since that is
// where GoDoc lives for a const in a grouped block — so a reworded comment moves
// the assertion too.
//
// It walks backwards from the declaration to find that comment, and fails if there
// is none: a dictionary constant with no stated meaning is exactly the gap this
// file exists to close, and a reader that silently returned "" would let it pass.
func algoCodeAndMeaning(t *testing.T, name string) (code, doc string) {
	t.Helper()
	source, err := os.ReadFile("algo.go")
	if err != nil {
		t.Fatalf("reading algo.go: %v", err)
	}
	lines := strings.Split(string(source), "\n")
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, name+" ") {
			continue
		}
		rest := trimmed[len(name)+1:]
		open := strings.Index(rest, "=")
		if open < 0 {
			t.Fatalf("the constant %q is declared without a value, so this row cannot "+
				"resolve it; the reader has drifted", name)
		}
		rest = strings.TrimSpace(rest[open+1:])
		if len(rest) < 2 || !strings.HasPrefix(rest, `"`) {
			t.Fatalf("the constant %q does not carry a quoted literal (%q), so this row "+
				"cannot resolve it", name, rest)
		}
		end := strings.Index(rest[1:], `"`)
		if end < 0 {
			t.Fatalf("the constant %q has an unterminated literal", name)
		}
		code = rest[1 : 1+end]
		// Walk back over the declaration's own type prefix and the blank line to
		// the doc comment immediately above.
		for j := i - 1; j >= 0; j-- {
			above := strings.TrimSpace(lines[j])
			if above == "" {
				continue
			}
			if comment := strings.Index(above, "//"); comment >= 0 {
				doc = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(above[comment:]), "//"))
				break
			}
			// A non-comment line means there is no doc comment for this constant.
			break
		}
		if doc == "" {
			t.Errorf("the constant %q carries no doc comment stating what its code means, so "+
				"the collision table has nothing to compare against — and an undocumented "+
				"dictionary constant is the gap this file exists to close", name)
		}
		return code, doc
	}
	t.Fatalf("algo.go declares no constant %q; the collision table and this file have "+
		"drifted apart, so every row in the table passes vacuously", name)
	return "", ""
}

// algoEnumTable is what algoTradeEnumSource returns: each declared constant's
// literal, and the doc comment above it. The comment is the second half of the
// collision assertion — a code that is shared but means the same thing on both
// surfaces would be harmless, and the terms are what say it is not.
type algoEnumTable struct {
	values map[string]string
	docs   map[string]string
}

// algoTradeEnumSource parses the declared constants out of pkg/types/enums.go, so
// the type-collision test reads the trade values from the package it is checking
// rather than restating them. A restatement is a second source of truth that a
// change to the trade set would silently invalidate.
func algoTradeEnumSource(t *testing.T) algoEnumTable {
	t.Helper()
	source, err := os.ReadFile(filepath.Join("..", "types", "enums.go"))
	if err != nil {
		t.Fatalf("reading pkg/types/enums.go: %v", err)
	}
	out := algoEnumTable{values: map[string]string{}, docs: map[string]string{}}
	lines := strings.Split(string(source), "\n")
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		open := strings.Index(trimmed, "=")
		if open < 0 {
			continue
		}
		decl := strings.TrimSpace(trimmed[:open])
		space := strings.IndexAny(decl, " \t")
		if space < 0 {
			continue
		}
		name := decl[:space]
		if name == "" || name[0] < 'A' || name[0] > 'Z' {
			continue
		}
		rest := strings.TrimSpace(trimmed[open+1:])
		if len(rest) < 2 || !strings.HasPrefix(rest, `"`) {
			continue
		}
		end := strings.Index(rest[1:], `"`)
		if end < 0 {
			continue
		}
		out.values[name] = rest[1 : 1+end]
		// The doc comment is the line above, for the same reason as in
		// algoCodeAndMeaning.
		for j := i - 1; j >= 0; j-- {
			above := strings.TrimSpace(lines[j])
			if above == "" {
				continue
			}
			if comment := strings.Index(above, "//"); comment >= 0 {
				out.docs[name] = strings.TrimSpace(
					strings.TrimPrefix(strings.TrimSpace(above[comment:]), "//"))
			}
			break
		}
	}
	if len(out.values) == 0 {
		t.Fatal("no constants parsed from pkg/types/enums.go; the reader has drifted and " +
			"every value comparison in the type-collision test would pass vacuously")
	}
	return out
}
