// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package domain

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
	"testing"
)

// This file is the regression net for the futures domain layer: the five row
// payloads the Gateway sends, the six mappers that turn a row into a model, the
// five value wrappers every field is read through, and the tick derivation.
//
// Its counterpart is pkg/services/futures_test.go, which covers the futures
// service layer: the operation labels, the request bodies, and the four reply
// envelopes. The line between them is the one the layering draws. A row and the
// mapper that reads it are this package's; an envelope that holds rows, and any
// request body, are the service's, because an envelope is what the service
// decodes and a request is what it sends.
//
// The split changes what each side can assert, and the difference is worth
// stating. Where the old single-package test decoded the mock's *wrapped* reply
// and mapped the row out of it, the row tests here decode the bare row the
// Gateway sends and the envelope tests in pkg/services decode the wrapper — so
// each decode is now asserted where the type is declared, and neither side
// depends on the other's declaration to reach a fixture.
//
// The fixtures are the mock Gateway's own futures payloads, copied verbatim from
// test/mockgateway/fixtures.go. Copying rather than importing them is
// deliberate: importing would make this package's wire types and the mock's
// payloads move together, so a rename of a key in one would silently pass. Here
// a mismatch between the two is the failure.
//
// pkg/services/futures_test.go carries the same rows inside the envelopes it
// decodes. The two copies cannot be shared — a helper or a const in one
// package's _test.go file is invisible to the other — and a drift between them
// is caught on both sides, because each copy's own test asserts the decoded
// values.

// ---------------------------------------------------------------------------
// Fixtures - the mock Gateway's futures payloads, one row each
// ---------------------------------------------------------------------------

const futuresProductInfoBody = `{"prodCode":"HSI","instCode":"FUT","lotSize":50,"decInPrice":0,"contractSize":"50","priceDecimalPoint":"1","expiryDate":"2026-09-29","isSupportT1":0}`

const futuresMaxBuySellBody = `{"positionStatus":0,"maxBuyAmount":10,"maxSellAmount":5,"initialMargin":"50000.00","ccy":"HKD"}`

const futuresFundInfoBody = `{"assetBalance":"1000000.00","enableBalance":"500000.00","marginCall":"0.00","incomeBalance":"1200.00","cashBal":"300000.00","iMargin":"100000.00","mMargin":"50000.00","marginLevel":"2.50","maxMargin":"200000.00","creditLimit":"0.00","ctrlLevel":"1","marginClass":"1","aeId":"AE001","cashBalHKD":"250000.00","cashBalUSD":"6410.00","cycRateUSDtoHSD":"7.80","marginStatus":"1","statusPercent":"40","closeProfit":"500.00","cashBalCNH":"0.00"}`

const futuresHoldBody = `{"stockName":"Hang Seng Index Futures Sep26","stockCode":"HSI2609.HK","lastDayQty":"1","lastDayPrice":"24800","depQty":"1","dayLongQty":"0","dayLongPrice":"0","dayShortQty":"0","dayShortPrice":"0","dayNetQty":"0","dayNetPrice":"0","currentQty":"1","costPrice":"25000","lastPrice":"25100","preClosePrice":"24900","profitLoss":"500.00","ccyRate":"1.0","contractValue":"50","profitLossBaseCcy":"500.00","ccy":"HKD","dataType":"10010","closeProfit":"0.00","closeProfitHKD":"0.00"}`

const futuresOrderBody = `{"stockCode":"HSI2609.HK","stockName":"Hang Seng Index Futures Sep26","businessPrice":"25100","entrustBs":"1","entrustPrice":"25000","entrustAmount":"1","businessAmount":"1","date":"20260921","businessTime":"10:00:00","entrustTime":"09:59:59","queryParamStr":"1","statusDesc":"Filled","status":"8","entrustId":"F20260921001","canBeCanceled":0,"entrustType":"0","entrustTypeNum":"0","isValid":1,"canBeUpdated":0,"validType":"0","validTypeDesc":"Day","orderOptions":0,"validTime":""}`

// ---------------------------------------------------------------------------
// Assertion helpers
// ---------------------------------------------------------------------------

// futuresRequireMoney, futuresRequireQuantity, futuresRequirePrice and
// futuresRequireRate compare a mapped value against the literal the mock sent.
// The currency is asserted alongside the amount, because a futures amount
// attributed to the wrong currency is a wrong amount that still adds up.
//
// The same four helpers are declared in pkg/services/futures_test.go for the
// envelope tests. They cannot be shared: a helper in either package's _test.go
// file is invisible to the other, and exporting one from the non-test source to
// share it would put test scaffolding on the public surface.
func futuresRequireMoney(t *testing.T, label string, got Money, want, wantCurrency string) {
	t.Helper()
	if got.String() != want {
		t.Errorf("%s = %s, want the literal %q the Gateway sent", label, got, want)
	}
	if got.Currency() != wantCurrency {
		t.Errorf("%s currency = %q, want %q", label, got.Currency(), wantCurrency)
	}
}

func futuresRequireQuantity(t *testing.T, label string, got Quantity, want string) {
	t.Helper()
	if got.String() != want {
		t.Errorf("%s = %s, want the literal %q the Gateway sent", label, got, want)
	}
}

func futuresRequirePrice(t *testing.T, label string, got Price, want string) {
	t.Helper()
	if got.String() != want {
		t.Errorf("%s = %s, want the literal %q the Gateway sent", label, got, want)
	}
}

func futuresRequireRate(t *testing.T, label string, got Rate, want string) {
	t.Helper()
	if got.String() != want {
		t.Errorf("%s = %s, want the literal %q the Gateway sent", label, got, want)
	}
}

// ---------------------------------------------------------------------------
// Hostile value sets
// ---------------------------------------------------------------------------

// moneyCase is one hostile value: in is the literal a test feeds in, and want is
// the exact string that must reach the domain value.
//
// These four sets and the four helpers below are the money regression net the
// cash layer keeps in pkg/services/testsupport_test.go, restated here because
// the futures mappers this file tests live in this package. They are restated
// rather than imported for the same reason the wire fixtures are: a shared table
// that a future edit can move independently of the mappers it guards is a table
// that eventually guards nothing.
type moneyCase struct {
	name string
	in   string
	want string
}

// float64HostilePrices are values a float64 round-trip changes: they carry more
// significant digits than binary64 can hold, or they underflow it to zero. A
// float64 wire field or a parse-then-format conversion cannot pass them; only a
// verbatim decimal field can.
//
// The last row is the interesting one: 1e-330 is a positive decimal that is
// zero to every float64. MustNewPrice expands it to the full positional form, so
// the value survives, while a float64 in the path would silently turn a real
// price into nothing.
var float64HostilePrices = []moneyCase{
	{
		name: "19 significant digits",
		in:   "123.4567890123456789",
		want: "123.4567890123456789",
	},
	{
		name: "25 significant digits",
		in:   "0.1234567890123456789012345",
		want: "0.1234567890123456789012345",
	},
	{
		name: "underflows binary64",
		in:   "1e-330",
		// The positional expansion, which is what a decimal String() yields: a
		// leading zero, a point, 329 zeros, and the significant 1. Written as a
		// construction because the literal is 332 characters long and a
		// transcription error in it would be invisible in review.
		want: underflowPositional,
	},
}

// float64HostileQuantities is the quantity counterpart: a 27-digit integer that
// a float64 renders as a different number, and that is nevertheless a legal
// futures order quantity.
var float64HostileQuantities = []moneyCase{
	{
		name: "27-digit integer",
		in:   "999999999999999999999999900",
		want: "999999999999999999999999900",
	},
}

// scaleHostilePrices are float64-stable values that a fixed-scale formatter
// still destroys. This is the set the shipped bug lived in: the removed
// fmt.Sprintf("%.3f", …) conversion turned a 0.0005 tick into a different and
// wrong price, with no error anywhere. All four are float64-stable, which is
// exactly why they discriminate a re-introduced %.3f and would not have shown
// up in the table above.
//
// The literal the bug produced is deliberately not repeated here: restating it
// would add one more hardcoded tick literal to the repository, and P5 exists to
// take the 40 that are already there.
var scaleHostilePrices = []moneyCase{
	{
		name: "the shipped bug",
		in:   "0.0005",
		want: "0.0005",
	},
	{
		name: "sub-micro",
		in:   "0.0000001",
		want: "0.0000001",
	},
	{
		name: "trailing zeros",
		in:   "0.1",
		want: "0.1",
	},
	{
		name: "one third",
		in:   "0.3333333333333333",
		want: "0.3333333333333333",
	},
}

// underflowPositional is the positional decimal form of 1e-330, the
// representation every domain constructor here renders. It is computed rather
// than typed out for the reason given on the row above.
var underflowPositional = "0." + strings.Repeat("0", 329) + "1"

// legacyRounded is the removed fmt.Sprintf("%.3f", …) conversion, kept so the
// tests below can show what the old code did to a value.
func legacyRounded(f float64) string { return fmt.Sprintf("%.3f", f) }

// requireFloat64Hostile asserts that in is genuinely hostile to a float64
// round-trip: parsing it and reformatting the shortest way changes the digits.
// The guard is the point — a future edit that softens a literal into a benign
// one fails here instead of quietly removing the regression net.
func requireFloat64Hostile(t *testing.T, in string) {
	t.Helper()
	f, err := strconv.ParseFloat(in, 64)
	if err != nil {
		t.Fatalf("test bug: strconv.ParseFloat(%q) failed: %v", in, err)
	}
	if got := strconv.FormatFloat(f, 'f', -1, 64); got == in {
		t.Fatalf("test bug: %q survives a float64 round-trip (%q), so it is no longer "+
			"float64-hostile and proves nothing about a float64 in the path", in, got)
	}
}

// requireScaleHostile asserts that in is destroyed by a three-decimal
// formatter, which is the specific conversion the bug used. It deliberately does
// not require the value to be float64-stable: what matters here is only that
// %.3f changes it, because a scale-hostile value that is also float64-hostile
// would be indistinguishable from the table above.
func requireScaleHostile(t *testing.T, in string) {
	t.Helper()
	f, err := strconv.ParseFloat(in, 64)
	if err != nil {
		t.Fatalf("test bug: strconv.ParseFloat(%q) failed: %v", in, err)
	}
	if got := legacyRounded(f); got == in {
		t.Fatalf("test bug: %%.3f renders %q as %q, so it is no longer scale-hostile "+
			"and would not catch a re-introduced conversion", in, got)
	}
}

// futuresAssertVerbatim is the assertion the money tables below are read
// through: the mapped value must equal the literal the Gateway sent, digit for
// digit, and the failure message names both conversions that would destroy it.
func futuresAssertVerbatim(t *testing.T, field, got string, tc moneyCase) {
	t.Helper()
	if got == tc.want {
		return
	}
	t.Errorf("%s = %q, want %q (the literal the Gateway sent).\n"+
		"  input literal:          %s\n"+
		"  a float64 round trip:   %s\n"+
		"  the removed %%.3f form: %s",
		field, got, tc.want, tc.in, futuresFloatCeiling(tc.in), futuresLegacy(t, tc.in))
}

// futuresFloatCeiling renders what binary64 could have produced, for the failure
// message. A value that cannot be parsed is reported as such rather than
// panicking inside a failure path.
func futuresFloatCeiling(in string) string {
	f, ok := futuresParseFloat(in)
	if !ok {
		return "(not representable as a float64)"
	}
	return futuresFormatFloat(f)
}

// futuresLegacy is the removed fmt.Sprintf("%.3f", …) conversion, spelled out so a
// failure message can show what the old code did to the value.
func futuresLegacy(t *testing.T, in string) string {
	t.Helper()
	f, ok := futuresParseFloat(in)
	if !ok {
		return "(not representable as a float64)"
	}
	return legacyRounded(f)
}

// futuresParseFloat and futuresFormatFloat wrap strconv so a failure message can
// render a value a float64 could not hold.
func futuresParseFloat(in string) (float64, bool) {
	f, err := strconv.ParseFloat(in, 64)
	if err != nil {
		return 0, false
	}
	return f, true
}

func futuresFormatFloat(f float64) string { return strconv.FormatFloat(f, 'f', -1, 64) }

// ---------------------------------------------------------------------------
// Mappers over the mock's rows
// ---------------------------------------------------------------------------

// TestFuturesProductFromDTOTheMockFixture maps the product row field by field.
// decInPrice and priceDecimalPoint are both asserted, because the tick rule
// depends on which of the two is authoritative.
func TestFuturesProductFromDTOTheMockFixture(t *testing.T) {
	var row FuturesProductInfoWire
	if err := json.Unmarshal([]byte(futuresProductInfoBody), &row); err != nil {
		t.Fatalf("the mock's product row does not decode: %v", err)
	}

	got := FuturesProductFromDTO(&row)
	if got == nil {
		t.Fatal("FuturesProductFromDTO returned nil for a present row")
	}
	if got.ProdCode != "HSI" || got.InstCode != "FUT" {
		t.Errorf("prodCode/instCode = %q/%q, want HSI/FUT", got.ProdCode, got.InstCode)
	}
	if got.LotSize != 50 {
		t.Errorf("lotSize = %d, want 50", got.LotSize)
	}
	if got.DecInPrice != 0 {
		t.Errorf("decInPrice = %d, want 0 (a whole-unit grid is a real value, not unset)", got.DecInPrice)
	}
	futuresRequireQuantity(t, "contractSize", got.ContractSize, "50")
	if got.PriceDecimalPoint != "1" {
		t.Errorf("priceDecimalPoint = %q, want the literal %q", got.PriceDecimalPoint, "1")
	}
	if got.ExpiryDate != "2026-09-29" {
		t.Errorf("expiryDate = %q, want 2026-09-29", got.ExpiryDate)
	}
	if got.IsSupportT1 {
		t.Error("isSupportT1 = true, want false for the wire's 0")
	}
}

// TestFuturesProductCarriesTheTickFromDecInPrice is the hard rule of this task,
// asserted: the tick is derived from the integer power and the Gateway's string
// rendering is not the source.
//
// The row is deliberately hostile — a priceDecimalPoint that is not a number at
// all. A mapper that preferred the string, or parsed it, would either panic or
// report a tick that disagrees with decInPrice; this one reports the integer
// and carries the string through untouched.
func TestFuturesProductCarriesTheTickFromDecInPrice(t *testing.T) {
	row := FuturesProductInfoWire{
		ProdCode:          "ES",
		DecInPrice:        3,
		ContractSize:      "50",
		PriceDecimalPoint: "not-a-number",
	}
	got := FuturesProductFromDTO(&row)
	if got.DecInPrice != 3 {
		t.Errorf("decInPrice = %d, want 3", got.DecInPrice)
	}
	if got.PriceDecimalPoint != "not-a-number" {
		t.Errorf("priceDecimalPoint = %q, want it carried verbatim", got.PriceDecimalPoint)
	}
	if tick := futuresTickString(got.DecInPrice); tick != "0.001" {
		t.Errorf("the tick derived from decInPrice 3 = %q, want 0.001", tick)
	}
}

// TestFuturesCapacityFromDTOTheMockFixture maps the max buy/sell payload. The
// two counts are the only numerics in any futures reply, so this is the row
// that proves the int64 treatment.
func TestFuturesCapacityFromDTOTheMockFixture(t *testing.T) {
	var reply FuturesMaxBuySellAmountWire
	if err := json.Unmarshal([]byte(futuresMaxBuySellBody), &reply); err != nil {
		t.Fatalf("the mock's capacity reply does not decode: %v", err)
	}

	got := FuturesCapacityFromDTO(&reply)
	if got == nil {
		t.Fatal("FuturesCapacityFromDTO returned nil for a present reply")
	}
	if got.PositionStatus != 0 {
		t.Errorf("positionStatus = %d, want 0 (flat)", got.PositionStatus)
	}
	futuresRequireQuantity(t, "maxBuyAmount", got.MaxBuyAmount, "10")
	futuresRequireQuantity(t, "maxSellAmount", got.MaxSellAmount, "5")
	futuresRequireMoney(t, "initialMargin", got.InitialMargin, "50000", "HKD")
	if got.Ccy != "HKD" {
		t.Errorf("ccy = %q, want the literal the Gateway sent", got.Ccy)
	}
}

// TestFuturesCapacityCountsSurviveInt64 proves the counts are read as integers
// and not as floats. The largest int64 is one more than binary64 can hold, so
// this value is the one that discriminates the two treatments: a float64 path
// renders 9223372036854775807 as 9223372036854775808.
func TestFuturesCapacityCountsSurviveInt64(t *testing.T) {
	got := FuturesCapacityFromDTO(&FuturesMaxBuySellAmountWire{
		MaxBuyAmount:  math.MaxInt64,
		MaxSellAmount: 0,
		InitialMargin: "0",
		Ccy:           "HKD",
	})
	if s := got.MaxBuyAmount.String(); s != "9223372036854775807" {
		t.Errorf("maxBuyAmount = %s, want 9223372036854775807", s)
	}
	futuresRequireQuantity(t, "maxSellAmount", got.MaxSellAmount, "0")
}

// TestFuturesAccountFromDTOTheMockFixture maps the funds snapshot row field by
// field, including the currency attribution: three fields name their currency
// and the rest are attributed the base currency.
func TestFuturesAccountFromDTOTheMockFixture(t *testing.T) {
	var reply FuturesFundInfoWire
	if err := json.Unmarshal([]byte(futuresFundInfoBody), &reply); err != nil {
		t.Fatalf("the mock's fund info does not decode: %v", err)
	}

	got := FuturesAccountFromDTO(&reply)
	if got == nil {
		t.Fatal("FuturesAccountFromDTO returned nil for a present snapshot")
	}
	for _, c := range []struct {
		label string
		got   Money
		want  string
	}{
		{"assetBalance", got.AssetBalance, "1000000"},
		{"enableBalance", got.EnableBalance, "500000"},
		{"marginCall", got.MarginCall, "0"},
		{"incomeBalance", got.IncomeBalance, "1200"},
		{"cashBal", got.CashBal, "300000"},
		{"iMargin", got.IMargin, "100000"},
		{"mMargin", got.MMargin, "50000"},
		{"maxMargin", got.MaxMargin, "200000"},
		{"creditLimit", got.CreditLimit, "0"},
		{"closeProfit", got.CloseProfit, "500"},
	} {
		futuresRequireMoney(t, c.label, c.got, c.want, futuresBaseCurrency)
	}
	// The three fields whose names state a currency, and the one that does.
	futuresRequireMoney(t, "cashBalHKD", got.CashBalHKD, "250000", futuresCurrencyHKD)
	futuresRequireMoney(t, "cashBalUSD", got.CashBalUSD, "6410", futuresCurrencyUSD)
	futuresRequireMoney(t, "cashBalCNH", got.CashBalCNH, "0", futuresCurrencyCNH)
	// Ratios are not money: a margin level that could be added to a balance is
	// a Rate, so the mistake is a type error rather than a plausible number.
	futuresRequireRate(t, "marginLevel", got.MarginLevel, "2.5")
	futuresRequireRate(t, "statusPercent", got.StatusPercent, "40")
	futuresRequireRate(t, "cycRateUSDtoHSD", got.CycRateUSDtoHSD, "7.8")
	if got.AEID != "AE001" || got.CtrlLevel != "1" || got.MarginClass != "1" || got.MarginStatus != "1" {
		t.Errorf("codes = %q/%q/%q/%q, want AE001/1/1/1",
			got.AEID, got.CtrlLevel, got.MarginClass, got.MarginStatus)
	}
}

// TestFuturesCurrencyAttribution pins the one rule this package has to state
// rather than read: the Gateway labels some futures amounts and not others. A
// labelled amount takes the labelled currency, an unlabelled one takes the base
// currency, and an absent ccy takes the base currency rather than producing a
// Money that panics when it meets a real one.
func TestFuturesCurrencyAttribution(t *testing.T) {
	for _, tc := range []struct {
		name         string
		ccy          string
		wantCurrency string
	}{
		{"labelled HKD", "HKD", "HKD"},
		{"labelled USD", "USD", "USD"},
		{"labelled CNH", "CNH", "CNH"},
		{"absent falls back to the base", "", futuresBaseCurrency},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := FuturesCapacityFromDTO(&FuturesMaxBuySellAmountWire{
				InitialMargin: "1",
				Ccy:           tc.ccy,
			})
			futuresRequireMoney(t, "initialMargin", got.InitialMargin, "1", tc.wantCurrency)
		})
	}
}

// TestFuturesPositionFromDTOTheMockFixture maps the position row field by field.
// The two-part reply this row arrives in is assembled by the service layer's
// futuresHoldResultFromDTO and is tested there; what is tested here is the row.
func TestFuturesPositionFromDTOTheMockFixture(t *testing.T) {
	var row FuturesHoldWire
	if err := json.Unmarshal([]byte(futuresHoldBody), &row); err != nil {
		t.Fatalf("the mock's hold row does not decode: %v", err)
	}

	got := FuturesPositionFromDTO(&row)
	if got == nil {
		t.Fatal("FuturesPositionFromDTO returned nil for a present row")
	}
	if got.StockCode != "HSI2609.HK" || got.StockName != "Hang Seng Index Futures Sep26" {
		t.Errorf("stockCode/stockName = %q/%q", got.StockCode, got.StockName)
	}
	if got.DataType != "10010" {
		t.Errorf("dataType = %q, want 10010 (the field that says the book without a request field)", got.DataType)
	}
	if got.Ccy != "HKD" {
		t.Errorf("ccy = %q, want HKD", got.Ccy)
	}
	for _, c := range []struct {
		label string
		got   Quantity
		want  string
	}{
		{"lastDayQty", got.LastDayQty, "1"},
		{"depQty", got.DepQty, "1"},
		{"dayLongQty", got.DayLongQty, "0"},
		{"dayShortQty", got.DayShortQty, "0"},
		{"dayNetQty", got.DayNetQty, "0"},
		{"currentQty", got.CurrentQty, "1"},
		{"contractValue", got.ContractValue, "50"},
	} {
		futuresRequireQuantity(t, c.label, c.got, c.want)
	}
	for _, c := range []struct {
		label string
		got   Price
		want  string
	}{
		{"lastDayPrice", got.LastDayPrice, "24800"},
		{"dayLongPrice", got.DayLongPrice, "0"},
		{"dayShortPrice", got.DayShortPrice, "0"},
		{"dayNetPrice", got.DayNetPrice, "0"},
		{"costPrice", got.CostPrice, "25000"},
		{"lastPrice", got.LastPrice, "25100"},
		{"preClosePrice", got.PreClosePrice, "24900"},
	} {
		futuresRequirePrice(t, c.label, c.got, c.want)
	}
	futuresRequireRate(t, "ccyRate", got.CcyRate, "1")
	futuresRequireMoney(t, "profitLoss", got.ProfitLoss, "500", futuresBaseCurrency)
	futuresRequireMoney(t, "profitLossBaseCcy", got.ProfitLossBaseCcy, "500", futuresBaseCurrency)
	futuresRequireMoney(t, "closeProfit", got.CloseProfit, "0", futuresBaseCurrency)
	futuresRequireMoney(t, "closeProfitHKD", got.CloseProfitHKD, "0", futuresCurrencyHKD)
}

// TestFuturesOrderAndFillFromDTOTheMockFixture maps one wire row into both
// domain types, because the Gateway documents the same object for an order list
// and a fill list and the two mappers must agree on the money while disagreeing
// on which field is the fill price.
func TestFuturesOrderAndFillFromDTOTheMockFixture(t *testing.T) {
	var row FuturesOrderWire
	if err := json.Unmarshal([]byte(futuresOrderBody), &row); err != nil {
		t.Fatalf("the mock's order row does not decode: %v", err)
	}

	order := FuturesOrderFromDTO(&row)
	if order == nil {
		t.Fatal("FuturesOrderFromDTO returned nil for a present row")
	}
	if order.StockCode != "HSI2609.HK" {
		t.Errorf("stockCode = %q, want HSI2609.HK", order.StockCode)
	}
	if order.EntrustID != EntrustID("F20260921001") {
		t.Errorf("entrustID = %q, want F20260921001", order.EntrustID)
	}
	if order.Side != "1" {
		t.Errorf("side = %q, want the wire's 1 as a types.EntrustBS", order.Side)
	}
	if order.Status != "8" {
		t.Errorf("status = %q, want the wire's 8 as a types.EntrustStatus", order.Status)
	}
	if order.StatusDesc != "Filled" {
		t.Errorf("statusDesc = %q, want Filled", order.StatusDesc)
	}
	// businessPrice is the fill price and entrustPrice the order price; the two
	// mappers name them the other way round, so this is the row that proves the
	// difference rather than a copy.
	futuresRequirePrice(t, "order.OrderPrice", order.OrderPrice, "25000")
	futuresRequirePrice(t, "order.FillPrice", order.FillPrice, "25100")
	futuresRequireQuantity(t, "order.Quantity", order.Quantity, "1")
	futuresRequireQuantity(t, "order.FilledQty", order.FilledQty, "1")
	if order.OrderType != "0" || order.OrderTypeNum != "0" {
		t.Errorf("orderType/orderTypeNum = %q/%q, want 0/0", order.OrderType, order.OrderTypeNum)
	}
	if !order.IsValid {
		t.Error("isValid = false, want true for the wire's 1")
	}
	if order.CanBeCanceled || order.CanBeUpdated {
		t.Error("canBeCanceled/canBeUpdated are true, want false for the wire's 0s")
	}
	if order.OrderOptions != 0 {
		t.Errorf("orderOptions = %d, want 0", order.OrderOptions)
	}
	if order.Date != "20260921" || order.EntrustTime != "09:59:59" || order.BusinessTime != "10:00:00" {
		t.Errorf("date/times = %q/%q/%q", order.Date, order.EntrustTime, order.BusinessTime)
	}
	if order.ValidType != "0" || order.ValidTypeDesc != "Day" {
		t.Errorf("validType/desc = %q/%q, want 0/Day", order.ValidType, order.ValidTypeDesc)
	}
	if order.QueryParamStr != "1" {
		t.Errorf("queryParamStr = %q, want 1 carried as informational", order.QueryParamStr)
	}

	fill := FuturesFillFromDTO(&row)
	if fill == nil {
		t.Fatal("FuturesFillFromDTO returned nil for a present row")
	}
	futuresRequirePrice(t, "fill.Price", fill.Price, "25100")
	futuresRequirePrice(t, "fill.OrderPrice", fill.OrderPrice, "25000")
	futuresRequireQuantity(t, "fill.Quantity", fill.Quantity, "1")
	futuresRequireQuantity(t, "fill.FilledQty", fill.FilledQty, "1")
	if fill.EntrustID != order.EntrustID || fill.Status != order.Status {
		t.Errorf("the two mappers disagree on the order identity: %q/%q vs %q/%q",
			fill.EntrustID, fill.Status, order.EntrustID, order.Status)
	}
}

// ---------------------------------------------------------------------------
// Nil and empty rows
// ---------------------------------------------------------------------------

// TestFuturesMappersRejectNil is the nil guard every row mapper carries. The
// endpoint methods range over a reply's rows, so a nil row has to be reachable
// and safe.
//
// The three envelope-consuming assembly helpers carry the same guard and are
// tested in pkg/services/futures_test.go, beside them.
func TestFuturesMappersRejectNil(t *testing.T) {
	if got := FuturesProductFromDTO(nil); got != nil {
		t.Errorf("FuturesProductFromDTO(nil) = %v, want nil", got)
	}
	if got := FuturesCapacityFromDTO(nil); got != nil {
		t.Errorf("FuturesCapacityFromDTO(nil) = %v, want nil", got)
	}
	if got := FuturesAccountFromDTO(nil); got != nil {
		t.Errorf("FuturesAccountFromDTO(nil) = %v, want nil", got)
	}
	if got := FuturesPositionFromDTO(nil); got != nil {
		t.Errorf("FuturesPositionFromDTO(nil) = %v, want nil", got)
	}
	if got := FuturesOrderFromDTO(nil); got != nil {
		t.Errorf("FuturesOrderFromDTO(nil) = %v, want nil", got)
	}
	if got := FuturesFillFromDTO(nil); got != nil {
		t.Errorf("FuturesFillFromDTO(nil) = %v, want nil", got)
	}
}

// TestFuturesMappersTolerAnEmptyRow is the divergence from the cash mappers,
// asserted. MustNewMoney, MustNewQuantity, MustNewPrice and MustNewRate all
// panic on the empty string, and a partial futures reply is reachable — a field
// the Gateway omits on a read is a read, not a corrupt input — so every futures
// mapper routes its fields through a wrapper that maps an absent value to an
// explicit zero.
//
// A recover() is deliberately not used here. One would pass while pinning the
// panic it is meant to catch, which is the trap the money tests in
// pkg/services already document.
func TestFuturesMappersTolerAnEmptyRow(t *testing.T) {
	product := FuturesProductFromDTO(&FuturesProductInfoWire{})
	if product.ContractSize.String() != "0" {
		t.Errorf("an absent contractSize = %s, want an explicit zero", product.ContractSize)
	}

	capacity := FuturesCapacityFromDTO(&FuturesMaxBuySellAmountWire{})
	if capacity.MaxBuyAmount.String() != "0" || capacity.MaxSellAmount.String() != "0" {
		t.Errorf("absent counts = %s/%s, want 0/0",
			capacity.MaxBuyAmount, capacity.MaxSellAmount)
	}
	futuresRequireMoney(t, "an absent initialMargin", capacity.InitialMargin, "0", futuresBaseCurrency)

	account := FuturesAccountFromDTO(&FuturesFundInfoWire{})
	for _, c := range []struct {
		label string
		got   Money
	}{
		{"assetBalance", account.AssetBalance},
		{"enableBalance", account.EnableBalance},
		{"cashBalHKD", account.CashBalHKD},
		{"cashBalUSD", account.CashBalUSD},
		{"cashBalCNH", account.CashBalCNH},
	} {
		futuresRequireMoney(t, c.label, c.got, "0", c.got.Currency())
	}
	if account.MarginLevel.String() != "0" || account.StatusPercent.String() != "0" {
		t.Error("an absent ratio did not map to an explicit zero")
	}

	position := FuturesPositionFromDTO(&FuturesHoldWire{})
	futuresRequireQuantity(t, "an absent currentQty", position.CurrentQty, "0")
	futuresRequirePrice(t, "an absent costPrice", position.CostPrice, "0")
	futuresRequireRate(t, "an absent ccyRate", position.CcyRate, "0")

	order := FuturesOrderFromDTO(&FuturesOrderWire{})
	futuresRequirePrice(t, "an absent entrustPrice", order.OrderPrice, "0")
	futuresRequireQuantity(t, "an absent entrustAmount", order.Quantity, "0")
	if order.IsValid || order.CanBeCanceled || order.CanBeUpdated {
		t.Error("absent 0/1 flags mapped to true; an absent flag is false, not unknown")
	}

	fill := FuturesFillFromDTO(&FuturesOrderWire{})
	futuresRequirePrice(t, "an absent businessPrice", fill.Price, "0")
}

// ---------------------------------------------------------------------------
// Money fidelity
// ---------------------------------------------------------------------------

// TestFuturesMoneyIsVerbatim is the money regression net for the futures
// mappers, and it rests on the same fact the cash one does: every futures money
// and quantity on the wire is a quoted string, so nothing between the Gateway
// and the domain value can lose a digit unless a future change introduces a
// float field or a fixed-scale formatter.
//
// futuresAssertVerbatim is this package's established assertion for that
// promise; its failure message already names both conversions that would destroy
// the value.
func TestFuturesMoneyIsVerbatim(t *testing.T) {
	for _, tc := range float64HostilePrices {
		t.Run("price/"+tc.name, func(t *testing.T) {
			requireFloat64Hostile(t, tc.in)
			row := FuturesOrderWire{EntrustPrice: tc.in, BusinessPrice: tc.in}
			order := FuturesOrderFromDTO(&row)
			futuresAssertVerbatim(t, "FuturesOrder.OrderPrice", order.OrderPrice.String(), tc)
			futuresAssertVerbatim(t, "FuturesOrder.FillPrice", order.FillPrice.String(), tc)
			futuresRequirePrice(t, "FuturesPosition.CostPrice",
				futuresPrice(tc.in), tc.want)
		})
	}
	for _, tc := range scaleHostilePrices {
		t.Run("scale-hostile/"+tc.name, func(t *testing.T) {
			requireScaleHostile(t, tc.in)
			row := FuturesHoldWire{CostPrice: tc.in, CurrentQty: tc.in}
			position := FuturesPositionFromDTO(&row)
			futuresAssertVerbatim(t, "FuturesPosition.CostPrice", position.CostPrice.String(), tc)
			futuresAssertVerbatim(t, "FuturesPosition.CurrentQty", position.CurrentQty.String(), tc)
		})
	}
	for _, tc := range float64HostileQuantities {
		t.Run("quantity/"+tc.name, func(t *testing.T) {
			requireFloat64Hostile(t, tc.in)
			row := FuturesOrderWire{EntrustAmount: tc.in, BusinessAmount: tc.in}
			order := FuturesOrderFromDTO(&row)
			futuresAssertVerbatim(t, "FuturesOrder.Quantity", order.Quantity.String(), tc)
			futuresAssertVerbatim(t, "FuturesOrder.FilledQty", order.FilledQty.String(), tc)
		})
	}
	t.Run("money column", func(t *testing.T) {
		for _, tc := range append(append([]moneyCase(nil), float64HostilePrices...), scaleHostilePrices...) {
			row := FuturesFundInfoWire{AssetBalance: tc.in, CashBalUSD: tc.in}
			account := FuturesAccountFromDTO(&row)
			futuresAssertVerbatim(t, "FuturesAccount.AssetBalance", account.AssetBalance.String(), tc)
			futuresAssertVerbatim(t, "FuturesAccount.CashBalUSD", account.CashBalUSD.String(), tc)
		}
	})
	t.Run("ratio column", func(t *testing.T) {
		for _, tc := range scaleHostilePrices {
			row := FuturesFundInfoWire{MarginLevel: tc.in, CycRateUSDtoHSD: tc.in}
			account := FuturesAccountFromDTO(&row)
			futuresAssertVerbatim(t, "FuturesAccount.MarginLevel", account.MarginLevel.String(), tc)
			futuresAssertVerbatim(t, "FuturesAccount.CycRateUSDtoHSD", account.CycRateUSDtoHSD.String(), tc)
		}
	})
}

// TestFuturesTheUnderflowingValueIsPositiveNotZero is the reference row stated on
// its own: 1e-330 is a positive decimal and zero to every float64, so a float in
// this path would turn a real amount into a plausible balance of nothing.
func TestFuturesTheUnderflowingValueIsPositiveNotZero(t *testing.T) {
	account := FuturesAccountFromDTO(&FuturesFundInfoWire{AssetBalance: "1e-330"})
	futuresAssertVerbatim(t, "FuturesAccount.AssetBalance", account.AssetBalance.String(),
		moneyCase{name: "underflows binary64", in: "1e-330", want: underflowPositional})
	if !account.AssetBalance.IsPositive() {
		t.Error("an underflowing value mapped to a non-positive amount, which is the " +
			"silent failure the decimal types exist to prevent")
	}
}

// ---------------------------------------------------------------------------
// The tick
// ---------------------------------------------------------------------------

// TestFuturesPricesCarryTheZeroTick is the zero-tick rule, asserted as behaviour
// rather than as a constant. A price on a finer grid than an invented 0.001 must
// survive: Price.Validate skips its step check when the tick is zero, so a
// faithful 0.0005 futures price is not rejected by a grid this SDK made up.
//
// The second half is the control: the same price against a 0.001 tick does fail,
// so the assertion is not passing because Validate is inert.
func TestFuturesPricesCarryTheZeroTick(t *testing.T) {
	if futuresZeroTick != "0" {
		t.Fatalf("futuresZeroTick = %q, want \"0\"; this package must not invent a "+
			"futures grid, and the 0.001 literals elsewhere are another task's debt", futuresZeroTick)
	}
	const fine = "0.0005"
	if err := futuresPrice(fine).Validate(true); err != nil {
		t.Errorf("a %s price failed the step check: %v; a futures price must not be "+
			"validated against a grid this SDK invented", fine, err)
	}
	if err := MustNewPrice(fine, "0.001").Validate(true); err == nil {
		t.Error("the control passed: 0.0005 against a 0.001 tick should have failed, " +
			"so the assertion above is not proving the tick is zero")
	}
	if got := futuresPrice(fine).Round(); got.String() != fine {
		t.Errorf("Round on a zero-tick price = %s, want %s unchanged", got, fine)
	}
}

// TestFuturesTickString covers the derivation from the integer power, including
// the two ends: a power of 0 is a real whole-unit grid, and a negative power is
// unknown rather than a number.
func TestFuturesTickString(t *testing.T) {
	for _, tc := range []struct {
		decInPrice int32
		want       string
		why        string
	}{
		{0, "1", "a whole-unit grid, which the mock's HSI product is"},
		{1, "0.1", "one decimal place"},
		{2, "0.01", "two decimal places"},
		{3, "0.001", "three decimal places"},
		{4, "0.0001", "four decimal places"},
		{8, "0.00000001", "no upper bound is enforced here"},
		{-1, futuresZeroTick, "a negative power is undocumented, so the result is unknown"},
	} {
		if got := futuresTickString(tc.decInPrice); got != tc.want {
			t.Errorf("futuresTickString(%d) = %q, want %q (%s)",
				tc.decInPrice, got, tc.want, tc.why)
		}
	}
}

// TestFuturesTickStringAgreesWithTheWireRendering cross-checks the derivation
// against the Gateway's own string rendering on the one product the mock
// documents, and inverts the check: the integer is the source, so a disagreement
// is the Gateway's to explain and must not change the derived value.
func TestFuturesTickStringAgreesWithTheWireRendering(t *testing.T) {
	agree := []struct {
		decInPrice int32
		rendering  string
	}{
		{0, "1"},
		{1, "0.1"},
		{2, "0.01"},
		{3, "0.001"},
	}
	for _, tc := range agree {
		if got := futuresTickString(tc.decInPrice); got != tc.rendering {
			t.Errorf("the derived tick for decInPrice %d is %q while the Gateway "+
				"renders %q; the integer is the source, so this is worth a look",
				tc.decInPrice, got, tc.rendering)
		}
	}
	row := FuturesProductInfoWire{DecInPrice: 2, PriceDecimalPoint: "0.05"}
	if got := futuresTickString(FuturesProductFromDTO(&row).DecInPrice); got != "0.01" {
		t.Errorf("the derived tick = %q, want 0.01; a disagreeing priceDecimalPoint "+
			"must not become the source", got)
	}
}
