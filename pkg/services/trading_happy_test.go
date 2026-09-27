// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package services

import (
	"encoding/json"
	"strconv"
	"testing"

	"github.com/shing1211/hstongapi4go/client"
	"github.com/shing1211/hstongapi4go/pkg/domain"
	"github.com/shing1211/hstongapi4go/pkg/types"
)

// This file holds the trading.go fixtures and the thirteen happy paths.
//
// The fixtures are here rather than in the frozen testsupport_test.go because
// that file was frozen before B3 started, and every name in it is trading-
// prefixed so a duplicate declaration cannot take B2's or B4's run down with
// mine. Nothing redeclares orderBookBody, legacyRounded, mustFloat, or entry.
//
// Every reply fixture populates every decimal-bearing field, without exception.
// Thirteen of the twenty-nine service methods delegate to a domain.*FromWire
// mapper, and each of those calls domain.MustNew*, which panics on "". A
// partially populated fixture therefore does not fail an assertion, it takes the
// test process down — and a recover() wrapper would convert that crash into a
// pass while pinning the defect, so there is no recover() anywhere in this file.

// ---------------------------------------------------------------------------
// Fixtures
// ---------------------------------------------------------------------------

// tradingAccountID is a non-zero account, which is the precondition for all
// thirteen methods.
func tradingAccountID() domain.AccountID { return domain.AccountID("HK88888") }

// tradingEntrustID is a non-zero entrust id.
func tradingEntrustID() domain.EntrustID { return domain.EntrustID("E-1001") }

// tradingHKSymbol is the instrument every HK fixture trades.
func tradingHKSymbol() domain.Symbol {
	return domain.NewSymbol(domain.MarketHK, "00700", types.DataTypeHKStock)
}

// tradingUSSymbol is the instrument every US fixture trades.
func tradingUSSymbol() domain.Symbol {
	return domain.NewSymbol(domain.MarketUS, "AAPL", types.DataTypeUSStock)
}

// tradingConnectSymbol is a Shenzhen Connect symbol. It exists so the exchange
// mapping's unmapped case is asserted rather than assumed: only HK and US have
// cases, and a symbol in any other market falls through to the empty
// exchange type.
func tradingConnectSymbol() domain.Symbol {
	return domain.NewSymbol(domain.MarketShenzhenConnect, "300750", types.DataTypeHKStock)
}

// tradingHKOrder is a valid HK limit order.
//
// Every branch of validateOrderForHK is satisfied deliberately:
//   - Quantity 100 is an integer and a multiple of the HK stock lot of 100;
//   - Price 387.05 is positive and a multiple of the 0.001 tick;
//   - OrderType "3" (limit) is in knownHKOrderTypes and is neither
//     price-optional nor conditional, so the price is checked and the
//     validDays/condValue block does not run;
//   - SessionType is empty, which returns before the clock is consulted — this
//     is what makes the fixture deterministic at any hour;
//   - TimeInForce "DAY" is a member of the closed set;
//   - Side is EntrustBuy, one of the four sides an HK stock may carry.
func tradingHKOrder() domain.Order {
	return domain.Order{
		Symbol:      tradingHKSymbol(),
		Side:        types.EntrustBuy,
		OrderType:   types.EntrustTypeLimit,
		Quantity:    domain.MustNewQuantity("100"),
		Price:       domain.MustNewPrice("387.05", "0.001"),
		TimeInForce: "DAY",
		SessionType: "",
	}
}

// tradingUSOrder is a US order. validateOrderForHK returns nil on the first
// line for it, so nothing about it is validated at all.
func tradingUSOrder() domain.Order {
	return domain.Order{
		Symbol:      tradingUSSymbol(),
		Side:        types.EntrustSell,
		OrderType:   types.EntrustTypeLimit,
		Quantity:    domain.MustNewQuantity("10"),
		Price:       domain.MustNewPrice("190.25", "0.01"),
		TimeInForce: "DAY",
		SessionType: "",
	}
}

// tradingHKHistoryFilter is a populated HistoryFilter, so the date and paging
// fields are exercised rather than defaulted.
func tradingHKHistoryFilter() HistoryFilter {
	return HistoryFilter{
		ExchangeType: types.ExchangeHK,
		StartDate:    "20260101",
		EndDate:      "20260131",
		StockCode:    "00700",
		PageNo:       2,
		PageSize:     100,
	}
}

// tradingEntrustBody is an Entrust reply.
func tradingEntrustBody(entrustID string) json.RawMessage {
	return json.RawMessage(`{"entrustId":"` + entrustID + `"}`)
}

// tradingFareVo is a fully populated FareWire. FareFromWire calls MustNewMoney on
// ten of its fields, so a missing one panics rather than decoding to zero.
const tradingFareVo = `"fareVo":{"fare0":"10.000","fare1":"0.500","fare2":"1.000",` +
	`"fare3":"0.100","fare4":"0.000","fare5":"0.000","fare6":"0.000","fare7":"0.000",` +
	`"fare8":"0.000","fare9":"0.200","farex":"0.050","faret":"11.850"}`

// tradingOrderListBody is the reply both order-list families decode: RealEntrustList
// and HistoryEntrustList read it through EntrustFromWire, RealDeliverList and
// HistoryDeliverList through FillFromWire, and the two mappers between them
// require every price, quantity, money and fare field to be non-empty.
func tradingOrderListBody() json.RawMessage {
	return json.RawMessage(`{"data":[` +
		`{"stockCode":"00700","stockName":"Tencent","businessPrice":"387.100",` +
		`"entrustBs":"1","entrustPrice":"387.050","businessBalance":"38710.000",` +
		`"entrustAmount":"100","businessAmount":"100","date":"20260126",` +
		`"businessTime":"10:00:01","entrustTime":"09:59:58","queryParamStr":"",` +
		`"statusDesc":"filled","status":"8","entrustId":"E-1001",` +
		`"unBusinessAmount":"0","canBeCanceled":0,"entrustType":"3",` +
		`"opponentSeat":"9999","entrustTypeNum":"3","remarkType":"0",` +
		`"remark":"manual","exchangeType":"K","canBeUpdated":1,"exchange":"HK",` +
		tradingFareVo + `},` +
		`{"stockCode":"00005","stockName":"HSBC","businessPrice":"92.250",` +
		`"entrustBs":"2","entrustPrice":"92.500","businessBalance":"9225.000",` +
		`"entrustAmount":"5000","businessAmount":"2500","date":"20260126",` +
		`"businessTime":"14:31:02","entrustTime":"14:30:11","queryParamStr":"",` +
		`"statusDesc":"part filled","status":"7","entrustId":"E-1002",` +
		`"unBusinessAmount":"2500","canBeCanceled":1,"entrustType":"3",` +
		`"opponentSeat":"8888","entrustTypeNum":"3","remarkType":"0",` +
		`"remark":"","exchangeType":"K","canBeUpdated":1,"exchange":"HK",` +
		tradingFareVo + `}]}`)
}

// tradingCondOrderBody is a conditional-order page carrying two rows plus the
// page envelope. CondOrderFromWire only requires entrustAmount, but every other
// field is populated so the mapped values can be asserted against something
// real rather than against a gap in the fixture.
func tradingCondOrderBody() json.RawMessage {
	return json.RawMessage(`{"data":[` +
		`{"condOrderId":"C-1","dataType":"10000","stockCode":"00700",` +
		`"stockName":"Tencent","stockNameTc":"","stockNameEn":"Tencent",` +
		`"exchangeType":"K","entrustType":"31","sessionType":"","status":"1",` +
		`"canBeCancel":"1","canBeModify":"0","entrustBs":"1","entrustAmount":"100",` +
		`"createTime":"20260126 09:00:00","startTime":"20260126 09:00:00",` +
		`"endTime":"20260127 09:00:00","errorCode":"","errorMsg":"",` +
		`"condValue":"380.000","condPrice":"","condTrackType":"1"},` +
		`{"condOrderId":"C-2","dataType":"10000","stockCode":"00005",` +
		`"stockName":"HSBC","stockNameTc":"","stockNameEn":"HSBC",` +
		`"exchangeType":"K","entrustType":"33","sessionType":"","status":"3",` +
		`"canBeCancel":"0","canBeModify":"1","entrustBs":"2","entrustAmount":"5000",` +
		`"createTime":"20260125 14:00:00","startTime":"20260125 14:00:00",` +
		`"endTime":"20260126 14:00:00","errorCode":"1016","errorMsg":"invalid",` +
		`"condValue":"90.000","condPrice":"","condTrackType":"2"}` +
		`],"curPageNo":1,"curPageSize":2,"totalPages":3}`)
}

// tradingMaxAvailableBody populates all twenty fields of MaxAvailableWire.
// MaxAvailableFromWire calls MustNewQuantity or MustNewMoney on nineteen of
// them, so this fixture is the only shape in which the call can complete. It is
// wrapped in the "data" object the response envelope strips off before the
// service sees it, because maxAvailableAssetWireResponse carries the wire struct
// one level down.
func tradingMaxAvailableBody() json.RawMessage {
	return json.RawMessage(`{"data":{"positionStatus":"ok",` +
		`"position":"1000","longOpenAvailable":"900","longCloseAvailable":"800",` +
		`"cashAvailableToOpen":"700","marginAvailableToOpen":"600",` +
		`"cashAndMarginAvailableToOpen":"500","cashAvailableAmount":"12345.678",` +
		`"marginAvailableAmount":"22345.678","cashAndMarginAvailableAmount":"34345.678",` +
		`"shortOpenAvailable":"400","shortCloseAvailable":"300",` +
		`"shortCloseCashAvailable":"200","shortOpenPool":"100",` +
		`"shortBuyPower":"98765.432","contractSize":"1",` +
		`"unitedBuyPowerStatus":"normal","creditLimit":"500000.000",` +
		`"optionLongMarginAmount":"1500.250","optionShortMarginAmount":"2500.750"}}`)
}

// tradingMarginFullInfoBody populates the eight scalars of MarginFullInfoWire
// and carries a two-element rate array, so MarginInfoFromWire's loop is exercised
// rather than skipped.
func tradingMarginFullInfoBody() json.RawMessage {
	return json.RawMessage(`{"marginAllow":"1","marginInitRatio":"0.1250",` +
		`"marginKeepRatio":"0.2000","shortAllow":"1","shortInitMarginRatio":"0.1500",` +
		`"shortInterestRate":"0.0800","shortKeepMarginRatio":"0.2500",` +
		`"shortLastAvailableQty":"10000","rate":[` +
		`{"currencyCode":"HKD","currencyDesc":"Hong Kong Dollar",` +
		`"interestRateWithinMortgage":"0.0650"},` +
		`{"currencyCode":"USD","currencyDesc":"US Dollar",` +
		`"interestRateWithinMortgage":"0.0725"}]}`)
}

// tradingSupportBody is a BeforeAndAfterSupport reply. The Gateway's field is the
// single-character string "1" (supported) or "0" (not), and SessionSupportFromWire
// maps it onto two booleans.
func tradingSupportBody(support string) json.RawMessage {
	return json.RawMessage(`{"data":"` + support + `"}`)
}

// ---------------------------------------------------------------------------
// Request-shape helpers
// ---------------------------------------------------------------------------

// tradingParamsAs is a type assertion on the recorded params with a readable
// failure: the request-shape assertions are worthless if they silently succeed
// against the wrong type, so a mismatch names both what it wanted and what it
// got.
func tradingParamsAs[T any](t *testing.T, exec *sequencedExecutor) T {
	t.Helper()
	raw := exec.lastParams(t)
	req, ok := raw.(T)
	if !ok {
		t.Fatalf("recorded params = %T, want the request struct the method builds", raw)
	}
	return req
}

// tradingExpectCall asserts the op and route of the single recorded Do, which is
// the per-method half of the ADR 0010 dependency-inversion contract. Every
// happy path here repeats it, so a service that reached for a different endpoint
// fails at the row that changed rather than in a later test.
func tradingExpectCall(t *testing.T, exec *sequencedExecutor, wantOp string, wantRoute client.Route) {
	t.Helper()
	call := exec.lastCall(t)
	if call.op != wantOp {
		t.Errorf("op = %q, want %q", call.op, wantOp)
	}
	if call.route != wantRoute {
		t.Errorf("route = %q, want %q", call.route, wantRoute)
	}
	requireCalls(t, exec, 1)
}

// tradingItoa is strconv.Itoa spelled through the same function the service uses,
// so the expected string in the paging matrix is produced by the code under test's
// own conversion rather than by a second spelling of it. A change to that
// conversion therefore shows up as a mismatch here instead of silently
// invalidating the expectation.
func tradingItoa(t *testing.T, n int) string {
	t.Helper()
	return strconv.Itoa(n)
}

// ---------------------------------------------------------------------------
// Happy paths
// ---------------------------------------------------------------------------

// TestEntrustHKPlacesTheOrder is the mutation's happy path, and the one fixture
// the ADR 0003 proof runs on, so it is asserted in full: op, route, and every
// field of the wire request.
func TestEntrustHKPlacesTheOrder(t *testing.T) {
	exec := newSequencedExecutor(t, sequencedReply{reply: tradingEntrustBody("E-2001")})
	svc := NewTradingService(exec)

	res, err := svc.Entrust(t.Context(), tradingAccountID(), tradingHKOrder())
	if err != nil {
		t.Fatalf("Entrust: %v", err)
	}
	tradingExpectCall(t, exec, opEntrust, client.RouteTradeEntrust)

	if got, want := res.EntrustID.String(), "E-2001"; got != want {
		t.Errorf("EntrustID = %q, want %q", got, want)
	}
	// The status is a local decision, not a Gateway field: the order has been
	// accepted for registration the moment the call returns without error.
	if res.Status != types.EntrustStatusWaitToRegister {
		t.Errorf("Status = %q, want %q", res.Status, types.EntrustStatusWaitToRegister)
	}
	if res.OrderID != "" {
		t.Errorf("OrderID = %q, want empty: the entrust reply carries no order id", res.OrderID)
	}

	req := tradingParamsAs[entrustWireRequest](t, exec)
	if req.ExchangeType != types.ExchangeHK {
		t.Errorf("ExchangeType = %q, want %q", req.ExchangeType, types.ExchangeHK)
	}
	if req.StockCode != "00700" {
		t.Errorf("StockCode = %q, want 00700", req.StockCode)
	}
	if req.EntrustAmount != "100" {
		t.Errorf("EntrustAmount = %q, want 100", req.EntrustAmount)
	}
	if req.EntrustPrice != "387.05" {
		t.Errorf("EntrustPrice = %q, want 387.05", req.EntrustPrice)
	}
	if req.EntrustBS != types.EntrustBuy {
		t.Errorf("EntrustBS = %q, want %q", req.EntrustBS, types.EntrustBuy)
	}
	if req.EntrustType != types.EntrustTypeLimit {
		t.Errorf("EntrustType = %q, want %q", req.EntrustType, types.EntrustTypeLimit)
	}
	// TimeInForce is validated by validateTimeInForce and then dropped:
	// entrustWireRequest has no field for it, so the Gateway never learns which
	// time-in-force the caller asked for. Recorded rather than asserted, because
	// proving the absence would mean asserting on a struct shape that does not
	// have the member.
	//
	// ValidDays is stringified unconditionally, so a non-conditional order still
	// carries the literal "0" on the wire — the field is not omitted because the
	// number is zero, only because a zero int would be.
	if req.ValidDays != "0" {
		t.Errorf("ValidDays = %q, want \"0\": strconv.Itoa is applied unconditionally", req.ValidDays)
	}
	if req.SessionType != "" || req.CondValue != "" || req.CondTrackType != "" {
		t.Errorf("conditional fields = %q/%q/%q, want all empty for a limit order",
			req.SessionType, req.CondValue, req.CondTrackType)
	}
	if req.IceBergDisplaySize != "0" {
		t.Errorf("IceBergDisplaySize = %q, want \"0\": an unset iceberg quantity is still stringified",
			req.IceBergDisplaySize)
	}
}

// TestEntrustCarriesTheConditionalFields asserts the other side of the request
// builder: an order type that is both price-optional and conditional, so the
// session, iceberg, validDays, and condValue fields are all populated.
func TestEntrustCarriesTheConditionalFields(t *testing.T) {
	order := tradingHKOrder()
	order.OrderType = types.EntrustTypeStopProfitLimit // "31"
	order.Price = domain.MustNewPrice("0", "0.001")    // price-optional, so never checked
	order.SessionType = "0"                            // deterministic at any hour
	order.ValidDays = 7
	order.CondValue = "380.000"
	order.CondTrackType = "1"
	order.IcebergQty = domain.MustNewQuantity("20")
	order.ClientType = 3
	order.Exchange = "HK"

	exec := newSequencedExecutor(t, sequencedReply{reply: tradingEntrustBody("E-2002")})
	if _, err := NewTradingService(exec).Entrust(t.Context(), tradingAccountID(), order); err != nil {
		t.Fatalf("Entrust: %v", err)
	}
	tradingExpectCall(t, exec, opEntrust, client.RouteTradeEntrust)

	req := tradingParamsAs[entrustWireRequest](t, exec)
	if req.SessionType != "0" || req.ValidDays != "7" ||
		req.CondValue != "380.000" || req.CondTrackType != "1" {
		t.Errorf("conditional fields = %q/%q/%q/%q, want 0/7/380.000/1",
			req.SessionType, req.ValidDays, req.CondValue, req.CondTrackType)
	}
	if req.IceBergDisplaySize != "20" {
		t.Errorf("IceBergDisplaySize = %q, want 20", req.IceBergDisplaySize)
	}
	if req.ClientType != 3 {
		t.Errorf("ClientType = %d, want 3", req.ClientType)
	}
	// entrustWireRequest has an Exchange field and domain.Order has one too, but
	// the builder never copies it across, so a caller who set order.Exchange sees
	// it vanish. Recorded rather than asserted as right; it is a dropped field,
	// and only a production change can put it back.
	if req.Exchange != "" {
		t.Errorf("Exchange = %q, want empty: Entrust never copies order.Exchange into the request", req.Exchange)
	}
	if req.EntrustPrice != "0" {
		t.Errorf("EntrustPrice = %q, want \"0\": a price-optional order still stringifies its price",
			req.EntrustPrice)
	}
}

// TestEntrustUSSkipsEveryHKCheck is the non-HK passthrough at the method level.
// A US order is not lot-checked, not tick-checked, and not window-checked, so a
// deliberately off-lot, off-tick order is accepted and reaches the wire.
func TestEntrustUSSkipsEveryHKCheck(t *testing.T) {
	order := tradingUSOrder()
	order.Quantity = domain.MustNewQuantity("7")       // not a multiple of anything
	order.Price = domain.MustNewPrice("190.2567", "0") // no tick
	order.TimeInForce = "XGTD"                         // not a valid TIF
	order.Side = "9"                                   // not a valid side
	order.SessionType = "1"                            // HK window does not apply

	exec := newSequencedExecutor(t, sequencedReply{reply: tradingEntrustBody("E-2003")})
	if _, err := NewTradingService(exec).Entrust(t.Context(), tradingAccountID(), order); err != nil {
		t.Fatalf("Entrust for a US order = %v, want nil: validateOrderForHK returns on the "+
			"first line for any non-HK market", err)
	}
	tradingExpectCall(t, exec, opEntrust, client.RouteTradeEntrust)

	req := tradingParamsAs[entrustWireRequest](t, exec)
	if req.ExchangeType != types.ExchangeUS {
		t.Errorf("ExchangeType = %q, want %q", req.ExchangeType, types.ExchangeUS)
	}
	if req.StockCode != "AAPL" {
		t.Errorf("StockCode = %q, want AAPL", req.StockCode)
	}
	if req.EntrustAmount != "7" || req.EntrustPrice != "190.2567" {
		t.Errorf("amount/price = %q/%q, want 7/190.2567", req.EntrustAmount, req.EntrustPrice)
	}
	if req.EntrustBS != "9" || req.SessionType != "1" {
		t.Errorf("an unchecked side and session type still reach the wire: %q / %q", req.EntrustBS, req.SessionType)
	}
}

// TestEntrustMapsOnlyHKAndUSExchanges pins the exchange mapping's unmapped case.
//
// The switch in Entrust has cases for MarketHK and MarketUS and nothing else, so a
// Shenzhen Connect symbol passes every check (validateOrderForHK returns nil for
// any non-HK market) and is sent with an empty exchange type. The Gateway rejects
// it, but the SDK raises nothing locally. This is recorded as current behaviour so
// a later widening of the switch to the four documented exchanges is a visible
// change rather than a silent one.
func TestEntrustMapsOnlyHKAndUSExchanges(t *testing.T) {
	order := tradingHKOrder()
	order.Symbol = tradingConnectSymbol()

	exec := newSequencedExecutor(t, sequencedReply{reply: tradingEntrustBody("E-2004")})
	if _, err := NewTradingService(exec).Entrust(t.Context(), tradingAccountID(), order); err != nil {
		t.Fatalf("Entrust for a Shenzhen Connect symbol = %v, want nil", err)
	}
	req := tradingParamsAs[entrustWireRequest](t, exec)
	if req.ExchangeType != "" {
		t.Errorf("ExchangeType = %q, want empty: the switch maps only HK and US", req.ExchangeType)
	}
	if req.StockCode != "300750" {
		t.Errorf("StockCode = %q, want 300750", req.StockCode)
	}
}

// TestCancelEntrustSendsTheID pins the cancel request shape, including the
// hardcoded HK exchange type.
func TestCancelEntrustSendsTheID(t *testing.T) {
	exec := newSequencedExecutor(t, sequencedReply{reply: json.RawMessage(`{"data":"1"}`)})
	if err := NewTradingService(exec).CancelEntrust(t.Context(), tradingAccountID(), tradingEntrustID()); err != nil {
		t.Fatalf("CancelEntrust: %v", err)
	}
	tradingExpectCall(t, exec, opCancelEntrust, client.RouteTradeCancelEntrust)

	req := tradingParamsAs[cancelEntrustWireRequest](t, exec)
	if req.EntrustID != "E-1001" {
		t.Errorf("EntrustID = %q, want E-1001", req.EntrustID)
	}
	// Hardcoded, not derived from the entrust. CancelEntrust takes no market
	// argument, so a US order can only be cancelled as an HK one. Recorded rather
	// than fixed: it is a behaviour change, and the SDK's own exchange validation
	// (A8) deliberately does not reach this layer.
	if req.ExchangeType != types.ExchangeHK {
		t.Errorf("ExchangeType = %q, want %q (hardcoded by the service)", req.ExchangeType, types.ExchangeHK)
	}
}

// TestBatchCancelEntrustSendsEveryID covers the two-id loop and the
// zero-id case.
func TestBatchCancelEntrustSendsEveryID(t *testing.T) {
	t.Run("two ids", func(t *testing.T) {
		exec := newSequencedExecutor(t, sequencedReply{reply: json.RawMessage(
			`{"successEntrustId":["E-1","E-2"],"failCancelEntrust":[]}`)})
		err := NewTradingService(exec).BatchCancelEntrust(t.Context(), tradingAccountID(),
			[]domain.EntrustID{"E-1", "E-2"})
		if err != nil {
			t.Fatalf("BatchCancelEntrust: %v", err)
		}
		tradingExpectCall(t, exec, opBatchCancelEntrust, client.RouteTradeBatchCancelEntrust)

		req := tradingParamsAs[batchCancelEntrustWireRequest](t, exec)
		if len(req.EntrustIDs) != 2 || req.EntrustIDs[0] != "E-1" || req.EntrustIDs[1] != "E-2" {
			t.Errorf("EntrustIDs = %v, want [E-1 E-2]: the order of the caller's slice is preserved", req.EntrustIDs)
		}
		if req.ExchangeType != types.ExchangeHK {
			t.Errorf("ExchangeType = %q, want %q (hardcoded by the service)", req.ExchangeType, types.ExchangeHK)
		}
	})

	// An empty list is not a validation error: the service sends the request with
	// the id array omitted, and the Gateway decides. Pinned because a caller
	// cancelling nothing should not be told the SDK refused.
	t.Run("empty list is sent, not refused", func(t *testing.T) {
		exec := newSequencedExecutor(t, sequencedReply{reply: json.RawMessage(`{}`)})
		if err := NewTradingService(exec).BatchCancelEntrust(t.Context(), tradingAccountID(), nil); err != nil {
			t.Fatalf("BatchCancelEntrust with no ids = %v, want nil", err)
		}
		requireCalls(t, exec, 1)
		req := tradingParamsAs[batchCancelEntrustWireRequest](t, exec)
		if len(req.EntrustIDs) != 0 {
			t.Errorf("EntrustIDs = %v, want empty", req.EntrustIDs)
		}
	})
}

// TestChangeEntrustSendsTheReplacement pins the change request and the fact that
// both validators are hardcoded to DataTypeHKStock, so the tick and lot applied
// are the stock ones whatever the entrust was.
func TestChangeEntrustSendsTheReplacement(t *testing.T) {
	exec := newSequencedExecutor(t, sequencedReply{reply: json.RawMessage(`{"data":"1"}`)})
	err := NewTradingService(exec).ChangeEntrust(t.Context(), tradingAccountID(), tradingEntrustID(),
		domain.MustNewPrice("388.05", "0.001"), domain.MustNewQuantity("200"))
	if err != nil {
		t.Fatalf("ChangeEntrust: %v", err)
	}
	tradingExpectCall(t, exec, opChangeEntrust, client.RouteTradeChangeEntrust)

	req := tradingParamsAs[changeEntrustWireRequest](t, exec)
	if req.EntrustID != "E-1001" {
		t.Errorf("EntrustID = %q, want E-1001", req.EntrustID)
	}
	if req.EntrustPrice != "388.05" {
		t.Errorf("EntrustPrice = %q, want 388.05", req.EntrustPrice)
	}
	if req.EntrustAmount != "200" {
		t.Errorf("EntrustAmount = %q, want 200", req.EntrustAmount)
	}
	if req.ExchangeType != types.ExchangeHK {
		t.Errorf("ExchangeType = %q, want %q (hardcoded by the service)", req.ExchangeType, types.ExchangeHK)
	}
}

// TestChangeEntrustValidatesAsAnHKStock pins what ChangeEntrust's two hardcoded
// DataTypeHKStock arguments actually do, and it is not what the name suggests for
// the price.
//
// The lot does come from the schedule: an odd quantity is refused, because
// DataTypeHKStock's lot is 100 regardless of what the entrust really was.
//
// The tick does not. validatePriceForHK uses the schedule's TickSize only to
// decide whether to run a step check at all, and the step check itself
// (domain.Price.Validate) uses the tick the caller gave the Price constructor. So
// a caller who builds MustNewPrice("388.0501", "0.0001") — four decimal places,
// finer than any HK tick — is accepted for a stock, because the price it is
// carrying says its own tick is 0.0001 and 388.0501 is an exact multiple of it.
//
// That is recorded here rather than fixed: the loud-failure behaviour
// docs/runs/…/design-tick-model.md §1 promises depends on the caller passing the
// real tick, and nothing in this layer can tell whether they did.
func TestChangeEntrustValidatesAsAnHKStock(t *testing.T) {
	t.Run("odd lot is refused", func(t *testing.T) {
		exec := newSequencedExecutor(t, sequencedReply{reply: json.RawMessage(`{"data":"1"}`)})
		err := NewTradingService(exec).ChangeEntrust(t.Context(), tradingAccountID(), tradingEntrustID(),
			domain.MustNewPrice("388.05", "0.001"), domain.MustNewQuantity("7"))
		assertInvalidParam(t, err, opEntrust)
		requireCalls(t, exec, 0)
	})

	t.Run("a caller-supplied finer tick is honoured, not the schedule's", func(t *testing.T) {
		exec := newSequencedExecutor(t, sequencedReply{reply: json.RawMessage(`{"data":"1"}`)})
		err := NewTradingService(exec).ChangeEntrust(t.Context(), tradingAccountID(), tradingEntrustID(),
			domain.MustNewPrice("388.0501", "0.0001"), domain.MustNewQuantity("100"))
		if err != nil {
			t.Fatalf("ChangeEntrust with a 0.0001-tick price = %v, want nil: the step check "+
				"runs against the price's own tick, not the schedule's", err)
		}
		req := tradingParamsAs[changeEntrustWireRequest](t, exec)
		if req.EntrustPrice != "388.0501" {
			t.Errorf("EntrustPrice = %q, want 388.0501 verbatim", req.EntrustPrice)
		}
	})

	// The price rejection reports the entrust op, not the change op, because both
	// validators hardcode opEntrust. A caller inspecting Op to work out which
	// call failed is therefore told the wrong endpoint. Asserted as it is; see
	// the report.
	t.Run("a price rejection reports the entrust op", func(t *testing.T) {
		exec := newSequencedExecutor(t, sequencedReply{reply: json.RawMessage(`{"data":"1"}`)})
		err := NewTradingService(exec).ChangeEntrust(t.Context(), tradingAccountID(), tradingEntrustID(),
			domain.MustNewPrice("388.0501", "0.001"), domain.MustNewQuantity("100"))
		e := assertInvalidParam(t, err, opEntrust)
		if e.Op == opChangeEntrust {
			t.Error("Op = opChangeEntrust, want opEntrust: both validators hardcode opEntrust, " +
				"and this row exists to make that fact visible rather than to claim it is right")
		}
		requireCalls(t, exec, 0)
	})
}

// TestMaxAvailableAssetReturnsTheMappedReply is the cleanest money host in the
// package: MaxAvailableAsset has no validation gate at all, so price.String()
// goes straight into the request.
func TestMaxAvailableAssetReturnsTheMappedReply(t *testing.T) {
	exec := newSequencedExecutor(t, sequencedReply{reply: tradingMaxAvailableBody()})
	got, err := NewTradingService(exec).MaxAvailableAsset(t.Context(), tradingAccountID(),
		tradingHKSymbol(), domain.MustNewPrice("387.05", "0.001"), types.EntrustTypeLimit)
	if err != nil {
		t.Fatalf("MaxAvailableAsset: %v", err)
	}
	tradingExpectCall(t, exec, opMaxAvailableAsset, client.RouteTradeQueryMaxAvailableAsset)

	req := tradingParamsAs[maxAvailableAssetWireRequest](t, exec)
	if req.ExchangeType != types.ExchangeHK {
		t.Errorf("ExchangeType = %q, want %q", req.ExchangeType, types.ExchangeHK)
	}
	if req.StockCode != "00700" {
		t.Errorf("StockCode = %q, want 00700", req.StockCode)
	}
	if req.EntrustPrice != "387.05" {
		t.Errorf("EntrustPrice = %q, want 387.05", req.EntrustPrice)
	}
	if req.EntrustType != types.EntrustTypeLimit {
		t.Errorf("EntrustType = %q, want %q", req.EntrustType, types.EntrustTypeLimit)
	}

	if got.PositionStatus != "ok" {
		t.Errorf("PositionStatus = %q, want ok", got.PositionStatus)
	}
	if s := got.Position.String(); s != "1000" {
		t.Errorf("Position = %q, want 1000", s)
	}
	if s := got.LongOpenAvailable.String(); s != "900" {
		t.Errorf("LongOpenAvailable = %q, want 900", s)
	}
	if s := got.CashAndMarginAvailableToOpen.String(); s != "500" {
		t.Errorf("CashAndMarginAvailableToOpen = %q, want 500", s)
	}
	if s := got.ShortBuyPower.String(); s != "98765.432" {
		t.Errorf("ShortBuyPower = %q, want 98765.432: the money scale must not be re-formatted", s)
	}
	if s := got.CreditLimit.String(); s != "500000" {
		t.Errorf("CreditLimit = %q, want 500000: MustNewMoney normalises the scale on construction", s)
	}
	if s := got.CashAvailableAmount.Currency(); s != "HKD" {
		t.Errorf("CashAvailableAmount currency = %q, want HKD", s)
	}
	if s := got.OptionShortMarginAmount.String(); s != "2500.75" {
		t.Errorf("OptionShortMarginAmount = %q, want 2500.75", s)
	}
	if s := got.ContractSize.String(); s != "1" {
		t.Errorf("ContractSize = %q, want 1", s)
	}
	if got.UnitedBuyPowerStatus != "normal" {
		t.Errorf("UnitedBuyPowerStatus = %q, want normal", got.UnitedBuyPowerStatus)
	}
}

// TestMaxAvailableAssetExchangeMapping covers the same three-way switch Entrust
// has, on a method that is a pure query, so the unmapped case is visible without
// any order being placed.
func TestMaxAvailableAssetExchangeMapping(t *testing.T) {
	for _, tc := range []struct {
		name   string
		symbol domain.Symbol
		want   types.ExchangeType
	}{
		{"HK", tradingHKSymbol(), types.ExchangeHK},
		{"US", tradingUSSymbol(), types.ExchangeUS},
		{"unmapped market", tradingConnectSymbol(), ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			exec := newSequencedExecutor(t, sequencedReply{reply: tradingMaxAvailableBody()})
			if _, err := NewTradingService(exec).MaxAvailableAsset(t.Context(), tradingAccountID(),
				tc.symbol, domain.MustNewPrice("1", "0"), types.EntrustTypeLimit); err != nil {
				t.Fatalf("MaxAvailableAsset: %v", err)
			}
			if got := tradingParamsAs[maxAvailableAssetWireRequest](t, exec).ExchangeType; got != tc.want {
				t.Errorf("ExchangeType = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestRealEntrustListMapsTwoRows is the entrust-list happy path.
func TestRealEntrustListMapsTwoRows(t *testing.T) {
	exec := newSequencedExecutor(t, sequencedReply{reply: tradingOrderListBody()})
	got, err := NewTradingService(exec).RealEntrustList(t.Context(), tradingAccountID(),
		EntrustFilter{ExchangeType: types.ExchangeHK, EntrustIDs: []domain.EntrustID{"E-1001"}})
	if err != nil {
		t.Fatalf("RealEntrustList: %v", err)
	}
	tradingExpectCall(t, exec, opRealEntrustList, client.RouteTradeQueryRealEntrustList)

	req := tradingParamsAs[realEntrustListWireRequest](t, exec)
	if req.ExchangeType != types.ExchangeHK {
		t.Errorf("ExchangeType = %q, want %q", req.ExchangeType, types.ExchangeHK)
	}
	if req.QueryCount != defaultPageSize {
		t.Errorf("QueryCount = %d, want %d: the list queries ask for one page's worth", req.QueryCount, defaultPageSize)
	}
	if req.QueryParamStr != "" {
		t.Errorf("QueryParamStr = %q, want empty", req.QueryParamStr)
	}
	if len(req.EntrustIDs) != 1 || req.EntrustIDs[0] != "E-1001" {
		t.Errorf("EntrustIDs = %v, want [E-1001]", req.EntrustIDs)
	}

	if len(got) != 2 {
		t.Fatalf("returned %d entrusts, want 2", len(got))
	}
	first := got[0]
	if first.EntrustID.String() != "E-1001" {
		t.Errorf("EntrustID = %q, want E-1001", first.EntrustID)
	}
	if first.OrderID.String() != "E-1001" {
		t.Errorf("OrderID = %q, want E-1001: it is derived from the entrust id", first.OrderID)
	}
	if first.Symbol.Market != domain.MarketHK || first.Symbol.Code != "00700" {
		t.Errorf("Symbol = %+v, want HK/00700: the market comes from the filter's exchange type", first.Symbol)
	}
	if s := first.Price.String(); s != "387.05" {
		t.Errorf("Price = %q, want 387.05", s)
	}
	if s := first.Quantity.String(); s != "100" {
		t.Errorf("Quantity = %q, want 100", s)
	}
	if s := first.FilledQty.String(); s != "100" {
		t.Errorf("FilledQty = %q, want 100", s)
	}
	if s := first.AvgPrice.String(); s != "387.1" {
		t.Errorf("AvgPrice = %q, want 387.1", s)
	}
	if first.Status != types.EntrustStatusFilled {
		t.Errorf("Status = %q, want %q", first.Status, types.EntrustStatusFilled)
	}
	if first.CanCancel || !first.CanModify {
		t.Errorf("CanCancel/CanModify = %t/%t, want false/true", first.CanCancel, first.CanModify)
	}
	if first.Date != "20260126" || first.Time != "09:59:58" {
		t.Errorf("Date/Time = %q/%q, want 20260126/09:59:58 (entrust time, not business time)", first.Date, first.Time)
	}
	if first.Remark != "manual" || first.OpponentSeat != "9999" {
		t.Errorf("Remark/OpponentSeat = %q/%q", first.Remark, first.OpponentSeat)
	}
	if first.Fare == nil {
		t.Fatal("Fare = nil, want the mapped fare block")
	}
	if s := first.Fare.Commission.String(); s != "10" {
		t.Errorf("Fare.Commission = %q, want 10", s)
	}
	if s := first.Fare.Total.String(); s != "11.85" {
		t.Errorf("Fare.Total = %q, want 11.85", s)
	}
	if got[1].StatusDesc != "part filled" || got[1].CanCancel != true {
		t.Errorf("second row status/canCancel = %q/%t, want part filled/true",
			got[1].StatusDesc, got[1].CanCancel)
	}
}

// TestRealEntrustListWithoutAnIDFilter pins the id-array default: a nil filter
// list produces an empty slice, which omitempty then drops from the wire.
func TestRealEntrustListWithoutAnIDFilter(t *testing.T) {
	exec := newSequencedExecutor(t, sequencedReply{reply: tradingOrderListBody()})
	if _, err := NewTradingService(exec).RealEntrustList(t.Context(), tradingAccountID(),
		EntrustFilter{ExchangeType: types.ExchangeHK}); err != nil {
		t.Fatalf("RealEntrustList: %v", err)
	}
	req := tradingParamsAs[realEntrustListWireRequest](t, exec)
	if len(req.EntrustIDs) != 0 {
		t.Errorf("EntrustIDs = %v, want empty", req.EntrustIDs)
	}
}

// TestRealDeliverListMapsTwoRows is the fill-list happy path, on the same reply
// shape but through FillFromWire.
func TestRealDeliverListMapsTwoRows(t *testing.T) {
	exec := newSequencedExecutor(t, sequencedReply{reply: tradingOrderListBody()})
	got, err := NewTradingService(exec).RealDeliverList(t.Context(), tradingAccountID(),
		DeliverFilter{ExchangeType: types.ExchangeHK})
	if err != nil {
		t.Fatalf("RealDeliverList: %v", err)
	}
	tradingExpectCall(t, exec, opRealDeliverList, client.RouteTradeQueryRealDeliverList)

	req := tradingParamsAs[realDeliverListWireRequest](t, exec)
	if req.ExchangeType != types.ExchangeHK || req.QueryCount != defaultPageSize || req.QueryParamStr != "" {
		t.Errorf("request = %+v, want HK/50/empty", req)
	}

	if len(got) != 2 {
		t.Fatalf("returned %d fills, want 2", len(got))
	}
	first := got[0]
	// FillFromWire takes the price from businessPrice, not entrustPrice, and the
	// time from businessTime, not entrustTime. Asserting both against the values
	// the fixture makes different is the only way this is pinned.
	if s := first.Price.String(); s != "387.1" {
		t.Errorf("Price = %q, want 387.1 (businessPrice)", s)
	}
	if s := first.Quantity.String(); s != "100" {
		t.Errorf("Quantity = %q, want 100 (entrustAmount)", s)
	}
	if s := first.FilledQty.String(); s != "100" {
		t.Errorf("FilledQty = %q, want 100 (businessAmount)", s)
	}
	if s := first.Turnover.String(); s != "38710" {
		t.Errorf("Turnover = %q, want 38710 (businessBalance, HKD at scale 3)", s)
	}
	if first.Time != "10:00:01" {
		t.Errorf("Time = %q, want 10:00:01 (businessTime)", first.Time)
	}
	if first.CounterID != "9999" {
		t.Errorf("CounterID = %q, want 9999", first.CounterID)
	}
	if first.Fare == nil || first.Fare.Total.String() != "11.85" {
		t.Errorf("Fare = %+v, want the mapped fare block", first.Fare)
	}
	if got[1].Status != types.EntrustStatusPartFilled {
		t.Errorf("second row status = %q, want %q", got[1].Status, types.EntrustStatusPartFilled)
	}
}

// TestRealCondOrderListMapsTwoRows is the conditional-order page happy path.
func TestRealCondOrderListMapsTwoRows(t *testing.T) {
	exec := newSequencedExecutor(t, sequencedReply{reply: tradingCondOrderBody()})
	got, err := NewTradingService(exec).RealCondOrderList(t.Context(), tradingAccountID(),
		CondOrderFilter{ExchangeType: types.ExchangeHK, StockCode: "00700", PageNo: 2, PageSize: 100})
	if err != nil {
		t.Fatalf("RealCondOrderList: %v", err)
	}
	tradingExpectCall(t, exec, opRealCondOrderList, client.RouteTradeQueryRealCondOrderList)

	req := tradingParamsAs[condOrderListWireRequest](t, exec)
	if req.ExchangeType != types.ExchangeHK || req.StockCode != "00700" {
		t.Errorf("request exchange/code = %q/%q, want K/00700", req.ExchangeType, req.StockCode)
	}
	if req.PageNo != 2 || req.PageSize != 100 {
		t.Errorf("PageNo/PageSize = %d/%d, want 2/100 (in range, so passed through)", req.PageNo, req.PageSize)
	}

	if len(got) != 2 {
		t.Fatalf("returned %d conditional orders, want 2", len(got))
	}
	first := got[0]
	if first.CondOrderID != "C-1" {
		t.Errorf("CondOrderID = %q, want C-1", first.CondOrderID)
	}
	if s := first.Quantity.String(); s != "100" {
		t.Errorf("Quantity = %q, want 100", s)
	}
	// StatusDesc is derived from the numeric status, not read from the wire.
	if first.StatusDesc != "pending trigger" {
		t.Errorf("StatusDesc = %q, want \"pending trigger\": it is derived from status \"1\"", first.StatusDesc)
	}
	if !first.CanCancel || first.CanModify {
		t.Errorf("CanCancel/CanModify = %t/%t, want true/false", first.CanCancel, first.CanModify)
	}
	if first.CondValue != "380.000" || first.CondTrackType != "1" {
		t.Errorf("CondValue/CondTrackType = %q/%q", first.CondValue, first.CondTrackType)
	}
	if first.Symbol.Market != domain.MarketHK || first.Symbol.Code != "00700" {
		t.Errorf("Symbol = %+v, want HK/00700: the market comes from the row's own exchange type", first.Symbol)
	}
	if got[1].StatusDesc != "paused" {
		t.Errorf("second row StatusDesc = %q, want paused", got[1].StatusDesc)
	}
	if got[1].ErrorCode != "1016" {
		t.Errorf("second row ErrorCode = %q, want 1016", got[1].ErrorCode)
	}
}

// TestPagingClampMatrix is the page arithmetic both paged methods share. It is a
// matrix rather than a set of named cases because the three clamps are four
// boundaries each, and any one of them reverted would leave the other two passing.
//
// RealCondOrderList sends ints and HistoryCondOrderList sends the same numbers as
// strings, so both spellings are asserted from one table.
func TestPagingClampMatrix(t *testing.T) {
	cases := []struct {
		name         string
		pageNo       int
		pageSize     int
		wantPageNo   int
		wantPageSize int
	}{
		{"zero page no becomes one", 0, 100, 1, 100},
		{"negative page no becomes one", -1, 100, 1, 100},
		{"negative page no far below zero", -1000, 100, 1, 100},
		{"page no passes through", 7, 100, 7, 100},
		{"zero page size becomes the default", 1, 0, 1, defaultPageSize},
		{"negative page size becomes the default", 1, -3, 1, defaultPageSize},
		{"page size at the ceiling passes through", 1, maxPageSize, 1, maxPageSize},
		{"page size one past the ceiling is clamped", 1, maxPageSize + 1, 1, maxPageSize},
		{"page size far past the ceiling is clamped", 1, 100000, 1, maxPageSize},
		{"both out of range", 0, 0, 1, defaultPageSize},
		{"both negative", -1, -1, 1, defaultPageSize},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Run("RealCondOrderList", func(t *testing.T) {
				exec := newSequencedExecutor(t, sequencedReply{reply: tradingCondOrderBody()})
				_, err := NewTradingService(exec).RealCondOrderList(t.Context(), tradingAccountID(),
					CondOrderFilter{ExchangeType: types.ExchangeHK, PageNo: tc.pageNo, PageSize: tc.pageSize})
				if err != nil {
					t.Fatalf("RealCondOrderList: %v", err)
				}
				req := tradingParamsAs[condOrderListWireRequest](t, exec)
				if req.PageNo != tc.wantPageNo {
					t.Errorf("PageNo = %d, want %d (asked for %d)", req.PageNo, tc.wantPageNo, tc.pageNo)
				}
				if req.PageSize != tc.wantPageSize {
					t.Errorf("PageSize = %d, want %d (asked for %d)", req.PageSize, tc.wantPageSize, tc.pageSize)
				}
			})

			t.Run("HistoryCondOrderList", func(t *testing.T) {
				exec := newSequencedExecutor(t, sequencedReply{reply: tradingCondOrderBody()})
				_, err := NewTradingService(exec).HistoryCondOrderList(t.Context(), tradingAccountID(),
					HistoryFilter{ExchangeType: types.ExchangeHK, PageNo: tc.pageNo, PageSize: tc.pageSize})
				if err != nil {
					t.Fatalf("HistoryCondOrderList: %v", err)
				}
				req := tradingParamsAs[historyCondOrderListWireRequest](t, exec)
				if req.PageNo != tradingItoa(t, tc.wantPageNo) {
					t.Errorf("PageNo = %q, want %q (asked for %d)", req.PageNo, tradingItoa(t, tc.wantPageNo), tc.pageNo)
				}
				if req.PageSize != tradingItoa(t, tc.wantPageSize) {
					t.Errorf("PageSize = %q, want %q (asked for %d)", req.PageSize, tradingItoa(t, tc.wantPageSize), tc.pageSize)
				}
			})
		})
	}
}

// TestHistoryCondOrderListBuildsADateRange pins the one place this file's paging
// and date handling differ from the intraday method: page numbers are strings
// here, and the bare dates are widened to whole days.
func TestHistoryCondOrderListBuildsADateRange(t *testing.T) {
	exec := newSequencedExecutor(t, sequencedReply{reply: tradingCondOrderBody()})
	filter := tradingHKHistoryFilter()
	if _, err := NewTradingService(exec).HistoryCondOrderList(t.Context(), tradingAccountID(), filter); err != nil {
		t.Fatalf("HistoryCondOrderList: %v", err)
	}
	tradingExpectCall(t, exec, opHistoryCondOrderList, client.RouteTradeQueryHistoryCondOrderList)

	req := tradingParamsAs[historyCondOrderListWireRequest](t, exec)
	if req.StartTime != "20260101 00:00:00" {
		t.Errorf("StartTime = %q, want %q: the start date is widened to the first second of the day",
			req.StartTime, "20260101 00:00:00")
	}
	if req.EndTime != "20260131 23:59:59" {
		t.Errorf("EndTime = %q, want %q: the end date is widened to the last second of the day",
			req.EndTime, "20260131 23:59:59")
	}
	if req.PageNo != "2" || req.PageSize != "100" {
		t.Errorf("PageNo/PageSize = %q/%q, want \"2\"/\"100\": this endpoint takes them as strings",
			req.PageNo, req.PageSize)
	}
	if req.StockCode != "00700" {
		t.Errorf("StockCode = %q, want 00700", req.StockCode)
	}
}

// TestHistoryCondOrderListWidenEmptyDates records what an undated filter
// produces. The service concatenates unconditionally, so an empty StartDate
// becomes the bare suffix rather than being omitted, and the Gateway sees a
// time-range filter it cannot interpret. Pinned because it is a caller-visible
// consequence of a missing check rather than of a crash.
func TestHistoryCondOrderListWidenEmptyDates(t *testing.T) {
	exec := newSequencedExecutor(t, sequencedReply{reply: tradingCondOrderBody()})
	if _, err := NewTradingService(exec).HistoryCondOrderList(t.Context(), tradingAccountID(),
		HistoryFilter{ExchangeType: types.ExchangeHK}); err != nil {
		t.Fatalf("HistoryCondOrderList: %v", err)
	}
	req := tradingParamsAs[historyCondOrderListWireRequest](t, exec)
	if req.StartTime != " 00:00:00" || req.EndTime != " 23:59:59" {
		t.Errorf("StartTime/EndTime = %q/%q, want the widened empty dates verbatim: "+
			"no check requires the caller to supply one", req.StartTime, req.EndTime)
	}
	if req.StockCode != "" {
		t.Errorf("StockCode = %q, want empty", req.StockCode)
	}
}

// TestHistoryEntrustListPassesTheDatesThrough asserts the intraday history
// methods forward the filter's dates untouched — no widening, no default.
func TestHistoryEntrustListPassesTheDatesThrough(t *testing.T) {
	filter := tradingHKHistoryFilter()

	t.Run("entrust", func(t *testing.T) {
		exec := newSequencedExecutor(t, sequencedReply{reply: tradingOrderListBody()})
		got, err := NewTradingService(exec).HistoryEntrustList(t.Context(), tradingAccountID(), filter)
		if err != nil {
			t.Fatalf("HistoryEntrustList: %v", err)
		}
		tradingExpectCall(t, exec, opHistoryEntrustList, client.RouteTradeQueryHistoryEntrustList)

		req := tradingParamsAs[historyEntrustListWireRequest](t, exec)
		if req.StartDate != "20260101" || req.EndDate != "20260131" {
			t.Errorf("StartDate/EndDate = %q/%q, want 20260101/20260131", req.StartDate, req.EndDate)
		}
		if req.QueryCount != defaultPageSize || req.QueryParamStr != "" {
			t.Errorf("QueryCount/QueryParamStr = %d/%q, want 50/empty", req.QueryCount, req.QueryParamStr)
		}
		// StockCode is on the filter but the entrust-list endpoint does not carry
		// it. Asserted so a later addition is a visible change.
		if len(got) != 2 {
			t.Errorf("returned %d entrusts, want 2", len(got))
		}
	})

	t.Run("deliver", func(t *testing.T) {
		exec := newSequencedExecutor(t, sequencedReply{reply: tradingOrderListBody()})
		got, err := NewTradingService(exec).HistoryDeliverList(t.Context(), tradingAccountID(), filter)
		if err != nil {
			t.Fatalf("HistoryDeliverList: %v", err)
		}
		tradingExpectCall(t, exec, opHistoryDeliverList, client.RouteTradeQueryHistoryDeliverList)

		req := tradingParamsAs[historyDeliverListWireRequest](t, exec)
		if req.StartDate != "20260101" || req.EndDate != "20260131" {
			t.Errorf("StartDate/EndDate = %q/%q, want 20260101/20260131", req.StartDate, req.EndDate)
		}
		if req.QueryCount != defaultPageSize {
			t.Errorf("QueryCount = %d, want %d", req.QueryCount, defaultPageSize)
		}
		if len(got) != 2 {
			t.Errorf("returned %d fills, want 2", len(got))
		}
		if s := got[1].Turnover.String(); s != "9225" {
			t.Errorf("second fill Turnover = %q, want 9225", s)
		}
	})
}

// TestMarginFullInfoSendsTheLiteralDataType pins the two constants this method
// hardcodes. They are the whole request, so a change to either is a wire change
// the Gateway may or may not tolerate, and nothing else in the package would
// notice.
func TestMarginFullInfoSendsTheLiteralDataType(t *testing.T) {
	exec := newSequencedExecutor(t, sequencedReply{reply: tradingMarginFullInfoBody()})
	got, err := NewTradingService(exec).MarginFullInfo(t.Context(), tradingAccountID())
	if err != nil {
		t.Fatalf("MarginFullInfo: %v", err)
	}
	tradingExpectCall(t, exec, opMarginFullInfo, client.RouteTradeQueryMarginFullInfo)

	req := tradingParamsAs[marginFullInfoWireRequest](t, exec)
	if req.DataType != "10000" {
		t.Errorf("DataType = %q, want the literal \"10000\"", req.DataType)
	}
	if req.StockCode != "" {
		t.Errorf("StockCode = %q, want empty: the endpoint is account-wide", req.StockCode)
	}

	if !got.MarginAllow || !got.ShortAllow {
		t.Errorf("MarginAllow/ShortAllow = %t/%t, want true/true", got.MarginAllow, got.ShortAllow)
	}
	if s := got.MarginInitRatio.String(); s != "0.125" {
		t.Errorf("MarginInitRatio = %q, want 0.125", s)
	}
	if s := got.MarginKeepRatio.String(); s != "0.2" {
		t.Errorf("MarginKeepRatio = %q, want 0.2", s)
	}
	if s := got.ShortInterestRate.String(); s != "0.08" {
		t.Errorf("ShortInterestRate = %q, want 0.08", s)
	}
	if s := got.ShortLastAvailableQty.String(); s != "10000" {
		t.Errorf("ShortLastAvailableQty = %q, want 10000", s)
	}
	if len(got.Rates) != 2 {
		t.Fatalf("Rates = %d entries, want 2: a two-element rate array must survive the mapper", len(got.Rates))
	}
	if got.Rates[0].CurrencyCode != "HKD" || got.Rates[0].CurrencyDesc != "Hong Kong Dollar" {
		t.Errorf("first rate = %+v, want HKD/Hong Kong Dollar", got.Rates[0])
	}
	if s := got.Rates[0].InterestRateWithinMortgage.String(); s != "0.065" {
		t.Errorf("first rate value = %q, want 0.065", s)
	}
	if got.Rates[1].CurrencyCode != "USD" {
		t.Errorf("second rate currency = %q, want USD", got.Rates[1].CurrencyCode)
	}
	if s := got.Rates[1].InterestRateWithinMortgage.String(); s != "0.0725" {
		t.Errorf("second rate value = %q, want 0.0725", s)
	}
}

// TestMarginFullInfoFalseFlagsAndEmptyRateArray covers the boolean decoding from
// the string "0" and the mapper's loop over a zero-length rate array, which is
// the shape a margin-disabled account actually returns.
func TestMarginFullInfoFalseFlagsAndEmptyRateArray(t *testing.T) {
	exec := newSequencedExecutor(t, sequencedReply{reply: json.RawMessage(
		`{"marginAllow":"0","marginInitRatio":"0.0000","marginKeepRatio":"0.0000",` +
			`"shortAllow":"0","shortInitMarginRatio":"0.0000","shortInterestRate":"0.0000",` +
			`"shortKeepMarginRatio":"0.0000","shortLastAvailableQty":"0","rate":[]}`)})
	got, err := NewTradingService(exec).MarginFullInfo(t.Context(), tradingAccountID())
	if err != nil {
		t.Fatalf("MarginFullInfo: %v", err)
	}
	if got.MarginAllow || got.ShortAllow {
		t.Errorf("MarginAllow/ShortAllow = %t/%t, want false/false for the string \"0\"",
			got.MarginAllow, got.ShortAllow)
	}
	if len(got.Rates) != 0 {
		t.Errorf("Rates = %d entries, want 0: the mapper makes a zero-length slice, not nil", len(got.Rates))
	}
	if got.Rates == nil {
		t.Error("Rates = nil, want an empty slice: make([]*MarginInterestRate, 0) is not nil")
	}
}

// TestBeforeAndAfterSupportMapsBothAnswers drives the single-character answer on
// both sides, because SessionSupportFromWire maps "1" to two booleans at once and
// a future change that split them would be invisible otherwise.
func TestBeforeAndAfterSupportMapsBothAnswers(t *testing.T) {
	for _, tc := range []struct {
		name        string
		support     string
		wantPre     bool
		wantAfter   bool
		description string
	}{
		{"supported", "1", true, true, "1"},
		{"not supported", "0", false, false, "0"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			exec := newSequencedExecutor(t, sequencedReply{reply: tradingSupportBody(tc.support)})
			got, err := NewTradingService(exec).BeforeAndAfterSupport(t.Context(), tradingHKSymbol())
			if err != nil {
				t.Fatalf("BeforeAndAfterSupport: %v", err)
			}
			tradingExpectCall(t, exec, opBeforeAndAfterSupport, client.RouteTradeQueryBeforeAndAfterSupport)

			req := tradingParamsAs[beforeAndAfterSupportWireRequest](t, exec)
			if req.StockCode != "00700" {
				t.Errorf("StockCode = %q, want 00700", req.StockCode)
			}
			if req.ExchangeType != types.ExchangeHK {
				t.Errorf("ExchangeType = %q, want %q", req.ExchangeType, types.ExchangeHK)
			}
			if got.PreMarket != tc.wantPre || got.AfterHours != tc.wantAfter {
				t.Errorf("PreMarket/AfterHours = %t/%t, want %t/%t for %q",
					got.PreMarket, got.AfterHours, tc.wantPre, tc.wantAfter, tc.description)
			}
			// The caller's symbol is echoed back untouched, including the fields
			// this endpoint has no opinion about.
			if got.Symbol != tradingHKSymbol() {
				t.Errorf("Symbol = %+v, want the caller's symbol verbatim", got.Symbol)
			}
		})
	}

	// Anything that is not exactly "1" reads as unsupported, including an empty
	// data field. Pinned because the comparison is equality against one string,
	// not a parse: a Gateway that sent "true" or "Y" would be read as "no".
	t.Run("anything but 1 is unsupported", func(t *testing.T) {
		for _, support := range []string{"", "2", "true", "Y", " 1"} {
			exec := newSequencedExecutor(t, sequencedReply{reply: tradingSupportBody(support)})
			got, err := NewTradingService(exec).BeforeAndAfterSupport(t.Context(), tradingUSSymbol())
			if err != nil {
				t.Fatalf("BeforeAndAfterSupport for %q: %v", support, err)
			}
			if got.PreMarket || got.AfterHours {
				t.Errorf("support %q read as supported: the comparison is equality against \"1\"", support)
			}
		}
	})
}

// TestBeforeAndAfterSupportExchangeMapping covers the three-way switch, including
// the US and unmapped cases, on a method that neither validates nor mutates
// anything.
func TestBeforeAndAfterSupportExchangeMapping(t *testing.T) {
	for _, tc := range []struct {
		name   string
		symbol domain.Symbol
		want   types.ExchangeType
	}{
		{"HK", tradingHKSymbol(), types.ExchangeHK},
		{"US", tradingUSSymbol(), types.ExchangeUS},
		{"unmapped market", tradingConnectSymbol(), ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			exec := newSequencedExecutor(t, sequencedReply{reply: tradingSupportBody("1")})
			if _, err := NewTradingService(exec).BeforeAndAfterSupport(t.Context(), tc.symbol); err != nil {
				t.Fatalf("BeforeAndAfterSupport: %v", err)
			}
			if got := tradingParamsAs[beforeAndAfterSupportWireRequest](t, exec).ExchangeType; got != tc.want {
				t.Errorf("ExchangeType = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestEmptyReplyIsSuccessNotAnError is the boundary the error-arm table sits
// next to, on the four methods whose decoders are given nothing and cannot
// therefore panic.
//
// A Gateway that answers ok with no data is a successful call that decoded
// nothing. Reading it as an error would fail a call that worked; reading the
// resulting zeros as a position would report a real account as empty. Both
// directions are costly, and the transport's contract is that out is left
// untouched in this case.
//
// Two methods are deliberately absent, and the omission is the finding rather
// than a gap: MaxAvailableAsset (trading.go:279) and MarginFullInfo
// (trading.go:472) hand a zero-valued wire struct straight to a *FromWire mapper,
// and every one of those mappers calls domain.MustNew* on each field, which
// panics on "". A successful reply with no data therefore takes the caller's
// goroutine down. There is no test for that here, and there deliberately is not
// one: a recover() wrapper would pass while pinning the panic, which is the
// outcome AGENTS.md and the plan's §6.5 both forbid. It is reported instead.
func TestEmptyReplyIsSuccessNotAnError(t *testing.T) {
	empty := json.RawMessage(`{}`)

	t.Run("order lists", func(t *testing.T) {
		exec := newSequencedExecutor(t,
			sequencedReply{reply: empty},
			sequencedReply{reply: empty},
		)
		svc := NewTradingService(exec)
		entrusts, err := svc.RealEntrustList(t.Context(), tradingAccountID(),
			EntrustFilter{ExchangeType: types.ExchangeHK})
		if err != nil {
			t.Fatalf("RealEntrustList on an empty reply = %v, want nil", err)
		}
		if len(entrusts) != 0 {
			t.Errorf("RealEntrustList returned %d rows, want 0", len(entrusts))
		}
		condOrders, err := svc.RealCondOrderList(t.Context(), tradingAccountID(),
			CondOrderFilter{ExchangeType: types.ExchangeHK})
		if err != nil {
			t.Fatalf("RealCondOrderList on an empty reply = %v, want nil", err)
		}
		if len(condOrders) != 0 {
			t.Errorf("RealCondOrderList returned %d rows, want 0", len(condOrders))
		}
		// Empty, not nil: the methods make the slice at the mapped length, so a
		// caller ranging over it gets zero iterations and a caller comparing it to
		// nil sees a difference from a nil return. Pinned so the choice is a
		// decision someone made rather than an accident.
		if entrusts == nil {
			t.Error("RealEntrustList returned a nil slice on an empty reply, want an empty one")
		}
		requireCalls(t, exec, 2)
	})

	t.Run("BeforeAndAfterSupport", func(t *testing.T) {
		exec := newSequencedExecutor(t, sequencedReply{reply: empty})
		got, err := NewTradingService(exec).BeforeAndAfterSupport(t.Context(), tradingHKSymbol())
		if err != nil {
			t.Fatalf("BeforeAndAfterSupport on an empty reply = %v, want nil", err)
		}
		if got.PreMarket || got.AfterHours {
			t.Error("an absent support field read as supported")
		}
	})

	t.Run("Entrust", func(t *testing.T) {
		exec := newSequencedExecutor(t, sequencedReply{reply: empty})
		got, err := NewTradingService(exec).Entrust(t.Context(), tradingAccountID(), tradingHKOrder())
		if err != nil {
			t.Fatalf("Entrust on an empty reply = %v, want nil", err)
		}
		if got.EntrustID != "" {
			t.Errorf("EntrustID = %q, want empty: a successful call with no entrust id in the reply", got.EntrustID)
		}
		if got.Status != types.EntrustStatusWaitToRegister {
			t.Errorf("Status = %q, want %q even with no id in the reply", got.Status, types.EntrustStatusWaitToRegister)
		}
	})
}
