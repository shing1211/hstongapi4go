// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package services

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/shing1211/hstongapi4go/client"
	"github.com/shing1211/hstongapi4go/gen/hq/dto"
	"github.com/shing1211/hstongapi4go/pkg/types"
)

// This file drives every market method end to end for the first time. Before
// it, not one of the eleven had been called at all: the converters in market.go
// were at 100% because P1's regression tests called them directly, while the
// service methods wrapping them were at 0%.
//
// Each case asserts four things: the op and route the method reached the
// Executor with, the request it built, the reply it mapped, and — for a
// representative subset — the bytes that reach the Gateway. The wire assertions
// are not redundant with the request assertions: the request is a Go struct, so
// a field whose JSON tag or type changed would still type-assert cleanly and
// only the body would show it.
//
// Every reply fixture carries a `security` echo and populates every numeric
// field. Populating them all is not decoration: the generated DTO numerics are
// float64, so an absent field silently decodes to 0 and a test that asserted
// against a half-empty fixture would be asserting against its own gaps.

func TestBasicQotHappyPath(t *testing.T) {
	exec := newSequencedExecutor(t, sequencedReply{reply: basicQotBody()})
	svc := NewMarketService(exec)

	resp, err := svc.BasicQot(t.Context(), BasicQotRequest{
		Security: []*Security{
			marketSecurity(),
			{DataType: types.DataTypeUSStock, Code: "AAPL"},
		},
		NeedDelayFlag: "1",
		MktTmType:     1,
	})
	if err != nil {
		t.Fatalf("BasicQot: %v", err)
	}

	call := exec.lastCall(t)
	if call.op != opBasicQot || call.route != client.RouteHqBasicQot {
		t.Errorf("reached (%q, %q), want (%q, %q)", call.op, call.route, opBasicQot, client.RouteHqBasicQot)
	}
	params, ok := call.params.(basicQotRequest)
	if !ok {
		t.Fatalf("params is %T, want basicQotRequest", call.params)
	}
	if len(params.Security) != 2 {
		t.Fatalf("request carries %d securities, want 2", len(params.Security))
	}
	if params.Security[0].DataType != int32(types.DataTypeHKStock) || params.Security[0].Code != "00700.HK" {
		t.Errorf("request security[0] = %+v, want {10000 00700.HK}", params.Security[0])
	}
	if params.Security[1].DataType != int32(types.DataTypeUSStock) || params.Security[1].Code != "AAPL" {
		t.Errorf("request security[1] = %+v, want {20000 AAPL}", params.Security[1])
	}
	if params.NeedDelayFlag != "1" || params.MktTmType != 1 {
		t.Errorf("request flags = (%q, %d), want (\"1\", 1)", params.NeedDelayFlag, params.MktTmType)
	}
	requireCalls(t, exec, 1)

	if len(resp.BasicQot) != 2 {
		t.Fatalf("BasicQot returned %d quotes, want 2", len(resp.BasicQot))
	}
	first := resp.BasicQot[0]
	if got := first.LastPrice.String(); got != "387.05" {
		t.Errorf("quote[0].LastPrice = %q, want 387.05", got)
	}
	if got := first.Turnover.String(); got != "464460" {
		t.Errorf("quote[0].Turnover = %q, want 464460", got)
	}
	if got := first.Volume.String(); got != "1200" {
		t.Errorf("quote[0].Volume = %q, want 1200", got)
	}
	if first.TradeTime != "09:30:01" || first.MktTmType != 1 || first.StockStatus != "0" {
		t.Errorf("quote[0] passthrough fields = (%q, %d, %q), want (\"09:30:01\", 1, \"0\")",
			first.TradeTime, first.MktTmType, first.StockStatus)
	}
	if !resp.BasicQot[1].IsSuspended {
		t.Error("quote[1].IsSuspended = false, want the wire's true")
	}
	if got := resp.BasicQot[1].LastClosePrice.String(); got != "381" {
		t.Errorf("quote[1].LastClosePrice = %q, want 381", got)
	}
}

// TestBasicQotPassesThroughANullQuote pins the loop's behaviour on a JSON null
// element. The Gateway would not normally send one, but the converter has a
// documented nil branch, and a method that panicked on it would take the
// caller's goroutine down for a single malformed row rather than the one field.
func TestBasicQotPassesThroughANullQuote(t *testing.T) {
	body := json.RawMessage(`{"basicQot":[null,{"lastPrice":381.25,"volume":10,"turnover":3812.5}]}`)
	exec := newSequencedExecutor(t, sequencedReply{reply: body})

	resp, err := NewMarketService(exec).BasicQot(t.Context(), BasicQotRequest{
		Security: []*Security{marketSecurity()},
	})
	if err != nil {
		t.Fatalf("BasicQot: %v", err)
	}
	if len(resp.BasicQot) != 2 {
		t.Fatalf("BasicQot returned %d quotes, want 2", len(resp.BasicQot))
	}
	if resp.BasicQot[0] != nil {
		t.Errorf("quote[0] = %+v, want nil for a JSON null element", resp.BasicQot[0])
	}
	if got := resp.BasicQot[1].LastPrice.String(); got != "381.25" {
		t.Errorf("quote[1].LastPrice = %q, want 381.25", got)
	}
}

// TestOrderBookHappyPath uses the reply fixture market_test.go already owns, so
// the shape of a book reply is proven on the one method that had a working
// fixture before the other five methods were written against it.
func TestOrderBookHappyPath(t *testing.T) {
	exec := newSequencedExecutor(t, sequencedReply{reply: orderBookBody(`,"spreadLevel":0.0005`)})

	resp, err := NewMarketService(exec).OrderBook(t.Context(), OrderBookRequest{
		Security:      marketSecurity(),
		MktTmType:     1,
		DepthBookType: 2,
	})
	if err != nil {
		t.Fatalf("OrderBook: %v", err)
	}

	call := exec.lastCall(t)
	if call.op != opOrderBook || call.route != client.RouteHqOrderBook {
		t.Errorf("reached (%q, %q), want (%q, %q)", call.op, call.route, opOrderBook, client.RouteHqOrderBook)
	}
	params, ok := call.params.(orderBookRequest)
	if !ok {
		t.Fatalf("params is %T, want orderBookRequest", call.params)
	}
	if params.Security == nil || params.Security.Code != "00700.HK" || params.Security.DataType != 10000 {
		t.Errorf("request security = %+v, want {10000 00700.HK}", params.Security)
	}
	if params.MktTmType != 1 || params.DepthBookType != 2 {
		t.Errorf("request (mktTmType, depthBookType) = (%d, %d), want (1, 2)", params.MktTmType, params.DepthBookType)
	}
	requireCalls(t, exec, 1)

	if resp.Security == nil || resp.Security.Code != "00700.HK" {
		t.Errorf("response security = %+v, want the echo", resp.Security)
	}
	if len(resp.Ask) != 1 || len(resp.Bid) != 0 {
		t.Fatalf("book = %d asks / %d bids, want 1 / 0", len(resp.Ask), len(resp.Bid))
	}
	if got := resp.Ask[0].Price.String(); got != "0.0005" {
		t.Errorf("Ask[0].Price = %q, want 0.0005", got)
	}
	if got := resp.Ask[0].Quantity.String(); got != "1200" {
		t.Errorf("Ask[0].Quantity = %q, want 1200", got)
	}
	if got := resp.TickSize.String(); got != "0.0005" {
		t.Errorf("TickSize = %q, want 0.0005", got)
	}
}

// TestOrderBookHappyPathWithTwoSidedLevels covers the shape the one-sided
// fixture cannot: a book with levels on both sides, so the ask and bid loops
// are both non-empty and a swapped or shared slice would show up.
func TestOrderBookHappyPathWithTwoSidedLevels(t *testing.T) {
	body := json.RawMessage(`{` + marketSecurityEcho + `,` +
		`"orderBookAskList":[{"level":1,"price":388.2,"volume":1200},{"level":2,"price":388.5,"volume":600}],` +
		`"orderBookBidList":[{"level":1,"price":388.15,"volume":800},{"level":2,"price":388.0,"volume":400}],` +
		`"spreadLevel":0.01}`)
	exec := newSequencedExecutor(t, sequencedReply{reply: body})

	resp, err := NewMarketService(exec).OrderBook(t.Context(), OrderBookRequest{
		Security: marketSecurity(),
	})
	if err != nil {
		t.Fatalf("OrderBook: %v", err)
	}
	if len(resp.Ask) != 2 || len(resp.Bid) != 2 {
		t.Fatalf("book = %d asks / %d bids, want 2 / 2", len(resp.Ask), len(resp.Bid))
	}
	for i, want := range []struct {
		price string
		qty   string
	}{
		{"388.2", "1200"},
		{"388.5", "600"},
	} {
		if got := resp.Ask[i].Price.String(); got != want.price {
			t.Errorf("Ask[%d].Price = %q, want %q", i, got, want.price)
		}
		if got := resp.Ask[i].Quantity.String(); got != want.qty {
			t.Errorf("Ask[%d].Quantity = %q, want %q", i, got, want.qty)
		}
	}
	if got := resp.Bid[0].Price.String(); got != "388.15" {
		t.Errorf("Bid[0].Price = %q, want 388.15", got)
	}
	if got := resp.Bid[1].Quantity.String(); got != "400" {
		t.Errorf("Bid[1].Quantity = %q, want 400", got)
	}
	if resp.Ask[0].Level != 1 || resp.Bid[1].Level != 2 {
		t.Errorf("levels = (%d, %d), want (1, 2)", resp.Ask[0].Level, resp.Bid[1].Level)
	}
	if got := resp.TickSize.String(); got != "0.01" {
		t.Errorf("TickSize = %q, want 0.01", got)
	}
}

func TestKLHappyPath(t *testing.T) {
	exec := newSequencedExecutor(t, sequencedReply{reply: klBody()})

	resp, err := NewMarketService(exec).KL(t.Context(), KLRequest{
		Security:    marketSecurity(),
		StartDate:   20260101,
		Direction:   1,
		ExRightFlag: 2,
		CycType:     3,
		Limit:       30,
	})
	if err != nil {
		t.Fatalf("KL: %v", err)
	}

	call := exec.lastCall(t)
	if call.op != opKL || call.route != client.RouteHqKL {
		t.Errorf("reached (%q, %q), want (%q, %q)", call.op, call.route, opKL, client.RouteHqKL)
	}
	params, ok := call.params.(klRequest)
	if !ok {
		t.Fatalf("params is %T, want klRequest", call.params)
	}
	if params.StartDate != 20260101 || params.Direction != 1 || params.ExRightFlag != 2 ||
		params.CycType != 3 || params.Limit != 30 {
		t.Errorf("request = %+v, want the five fields passed through unchanged", params)
	}
	requireCalls(t, exec, 1)

	if resp.Security == nil || resp.Security.DataType != types.DataTypeHKStock {
		t.Errorf("response security = %+v, want the echo", resp.Security)
	}
	if len(resp.Kline) != 2 {
		t.Fatalf("KL returned %d candles, want 2", len(resp.Kline))
	}
	if resp.Kline[0].Date != "20260123" || resp.Kline[1].Date != "20260126" {
		t.Errorf("candle dates = (%q, %q), want (20260123, 20260126)",
			resp.Kline[0].Date, resp.Kline[1].Date)
	}
	if got := resp.Kline[1].ClosePrice.String(); got != "382.5" {
		t.Errorf("Kline[1].ClosePrice = %q, want 382.5", got)
	}
	if got := resp.Kline[0].Turnover.String(); got != "4644600" {
		t.Errorf("Kline[0].Turnover = %q, want 4644600", got)
	}
	if resp.Kline[0].Timestamp != 1769126400 || resp.Kline[0].Time != "16:00" {
		t.Errorf("Kline[0] passthrough = (%d, %q), want (1769126400, 16:00)",
			resp.Kline[0].Timestamp, resp.Kline[0].Time)
	}
}

func TestTimeShareHappyPath(t *testing.T) {
	exec := newSequencedExecutor(t, sequencedReply{reply: timeShareBody()})

	resp, err := NewMarketService(exec).TimeShare(t.Context(), TimeShareRequest{
		Security:  marketSecurity(),
		MktTmType: 1,
	})
	if err != nil {
		t.Fatalf("TimeShare: %v", err)
	}

	call := exec.lastCall(t)
	if call.op != opTimeShare || call.route != client.RouteHqTimeShare {
		t.Errorf("reached (%q, %q), want (%q, %q)", call.op, call.route, opTimeShare, client.RouteHqTimeShare)
	}
	params, ok := call.params.(timeShareRequest)
	if !ok {
		t.Fatalf("params is %T, want timeShareRequest", call.params)
	}
	if params.Security == nil || params.Security.Code != "00700.HK" || params.MktTmType != 1 {
		t.Errorf("request = %+v, want the security and mktTmType", params)
	}
	requireCalls(t, exec, 1)

	if len(resp.TimeShare) != 2 {
		t.Fatalf("TimeShare returned %d points, want 2", len(resp.TimeShare))
	}
	if resp.TimeShare[0].Time != "09:30" || resp.TimeShare[1].Time != "09:31" {
		t.Errorf("point times = (%q, %q), want (09:30, 09:31)",
			resp.TimeShare[0].Time, resp.TimeShare[1].Time)
	}
	if got := resp.TimeShare[1].AvgPrice.String(); got != "380.875" {
		t.Errorf("TimeShare[1].AvgPrice = %q, want 380.875", got)
	}
	if got := resp.TimeShare[0].Turnover.String(); got != "380500" {
		t.Errorf("TimeShare[0].Turnover = %q, want 380500", got)
	}
}

func TestTickerHappyPath(t *testing.T) {
	exec := newSequencedExecutor(t, sequencedReply{reply: tickerBody()})

	resp, err := NewMarketService(exec).Ticker(t.Context(), TickerRequest{
		Security:  marketSecurity(),
		Limit:     20,
		MktTmType: 1,
	})
	if err != nil {
		t.Fatalf("Ticker: %v", err)
	}

	call := exec.lastCall(t)
	if call.op != opTicker || call.route != client.RouteHqTicker {
		t.Errorf("reached (%q, %q), want (%q, %q)", call.op, call.route, opTicker, client.RouteHqTicker)
	}
	params, ok := call.params.(tickerRequest)
	if !ok {
		t.Fatalf("params is %T, want tickerRequest", call.params)
	}
	if params.Limit != 20 || params.MktTmType != 1 || params.Security.Code != "00700.HK" {
		t.Errorf("request = %+v, want limit 20, mktTmType 1 and the security", params)
	}
	requireCalls(t, exec, 1)

	if len(resp.Ticker) != 2 {
		t.Fatalf("Ticker returned %d ticks, want 2", len(resp.Ticker))
	}
	if got := resp.Ticker[0].Price.String(); got != "387.05" {
		t.Errorf("Ticker[0].Price = %q, want 387.05", got)
	}
	if resp.Ticker[0].Side != 1 || resp.Ticker[1].Side != 2 || resp.Ticker[1].Type != 2 {
		t.Errorf("tick sides/types = (%d/%d, %d/%d), want (1/1, 2/2)",
			resp.Ticker[0].Side, resp.Ticker[0].Type, resp.Ticker[1].Side, resp.Ticker[1].Type)
	}
	if got := resp.Ticker[1].Turnover.String(); got != "77400" {
		t.Errorf("Ticker[1].Turnover = %q, want 77400", got)
	}
	if resp.Ticker[1].Timestamp != 1769391002 || resp.Ticker[1].MktTmType != 1 {
		t.Errorf("Ticker[1] passthrough = (%d, %d), want (1769391002, 1)",
			resp.Ticker[1].Timestamp, resp.Ticker[1].MktTmType)
	}
}

func TestBrokerHappyPath(t *testing.T) {
	exec := newSequencedExecutor(t, sequencedReply{reply: brokerBody()})

	resp, err := NewMarketService(exec).Broker(t.Context(), BrokerRequest{Security: marketSecurity()})
	if err != nil {
		t.Fatalf("Broker: %v", err)
	}

	call := exec.lastCall(t)
	if call.op != opBroker || call.route != client.RouteHqBroker {
		t.Errorf("reached (%q, %q), want (%q, %q)", call.op, call.route, opBroker, client.RouteHqBroker)
	}
	params, ok := call.params.(brokerRequest)
	if !ok {
		t.Fatalf("params is %T, want brokerRequest", call.params)
	}
	if params.Security == nil || params.Security.Code != "00700.HK" {
		t.Errorf("request security = %+v, want the echo code", params.Security)
	}
	requireCalls(t, exec, 1)

	if len(resp.Ask) != 2 || len(resp.Bid) != 2 {
		t.Fatalf("queues = %d asks / %d bids, want 2 / 2", len(resp.Ask), len(resp.Bid))
	}
	for i, want := range []struct {
		item, name string
		level      int32
		typ        int32
	}{
		{"1", "broker-a", 1, 0},
		{"2", "broker-b", 2, 0},
	} {
		if resp.Ask[i].Item != want.item || resp.Ask[i].Name != want.name ||
			resp.Ask[i].Level != want.level || resp.Ask[i].Type != want.typ {
			t.Errorf("Ask[%d] = %+v, want {level:%d item:%q type:%d name:%q}",
				i, resp.Ask[i], want.level, want.item, want.typ, want.name)
		}
	}
	if resp.Bid[0].Item != "3" || resp.Bid[1].Item != "4" {
		t.Errorf("Bid items = (%q, %q), want (3, 4)", resp.Bid[0].Item, resp.Bid[1].Item)
	}
	if resp.Bid[1].Name != "broker-d" || resp.Bid[1].Type != 1 {
		t.Errorf("Bid[1] = %+v, want {level:2 item:4 type:1 name:broker-d}", resp.Bid[1])
	}
}

func TestUsOptionChainCodeHappyPath(t *testing.T) {
	exec := newSequencedExecutor(t, sequencedReply{reply: usOptionChainBody()})

	resp, err := NewMarketService(exec).UsOptionChainCode(t.Context(), UsOptionChainCodeRequest{
		SecurityCode: "AAPL",
		ExpireDate:   "20260619",
		FlagInOut:    1,
		OptionType:   "C",
	})
	if err != nil {
		t.Fatalf("UsOptionChainCode: %v", err)
	}

	call := exec.lastCall(t)
	if call.op != opUsOptionChainCode || call.route != client.RouteHqUsOptionChainCode {
		t.Errorf("reached (%q, %q), want (%q, %q)", call.op, call.route,
			opUsOptionChainCode, client.RouteHqUsOptionChainCode)
	}
	params, ok := call.params.(usOptionChainCodeRequest)
	if !ok {
		t.Fatalf("params is %T, want usOptionChainCodeRequest", call.params)
	}
	if params.SecurityCode != "AAPL" || params.ExpireDate != "20260619" ||
		params.FlagInOut != 1 || params.OptionType != "C" {
		t.Errorf("request = %+v, want the four fields passed through unchanged", params)
	}
	requireCalls(t, exec, 1)

	if len(resp.OptionCode) != 2 {
		t.Fatalf("UsOptionChainCode returned %d codes, want 2", len(resp.OptionCode))
	}
	if resp.OptionCode[0] != "AAPL260619C00150000" || resp.OptionCode[1] != "AAPL260619C00200000" {
		t.Errorf("option codes = %v, want the two in the reply", resp.OptionCode)
	}
}

func TestUsOptionChainExpireDateHappyPath(t *testing.T) {
	exec := newSequencedExecutor(t, sequencedReply{reply: usOptionChainExpireBody()})

	resp, err := NewMarketService(exec).UsOptionChainExpireDate(t.Context(),
		UsOptionChainExpireDateRequest{SecurityCode: "AAPL"})
	if err != nil {
		t.Fatalf("UsOptionChainExpireDate: %v", err)
	}

	call := exec.lastCall(t)
	if call.op != opUsOptionChainExpireDate || call.route != client.RouteHqUsOptionChainExpireDate {
		t.Errorf("reached (%q, %q), want (%q, %q)", call.op, call.route,
			opUsOptionChainExpireDate, client.RouteHqUsOptionChainExpireDate)
	}
	params, ok := call.params.(usOptionChainExpireDateRequest)
	if !ok {
		t.Fatalf("params is %T, want usOptionChainExpireDateRequest", call.params)
	}
	if params.SecurityCode != "AAPL" {
		t.Errorf("request securityCode = %q, want AAPL", params.SecurityCode)
	}
	requireCalls(t, exec, 1)

	if len(resp.ExpireDate) != 2 {
		t.Fatalf("UsOptionChainExpireDate returned %d dates, want 2", len(resp.ExpireDate))
	}
	if resp.ExpireDate[0] != "20260619" || resp.ExpireDate[1] != "20260918" {
		t.Errorf("expire dates = %v, want [20260619 20260918]", resp.ExpireDate)
	}
}

func TestUsOverNightTradeCodesHappyPath(t *testing.T) {
	exec := newSequencedExecutor(t, sequencedReply{reply: usOverNightBody()})

	resp, err := NewMarketService(exec).UsOverNightTradeCodes(t.Context(), UsOverNightTradeCodesRequest{})
	if err != nil {
		t.Fatalf("UsOverNightTradeCodes: %v", err)
	}

	call := exec.lastCall(t)
	if call.op != opUsOverNightTradeCodes || call.route != client.RouteHqUsOverNightTradeCodes {
		t.Errorf("reached (%q, %q), want (%q, %q)", call.op, call.route,
			opUsOverNightTradeCodes, client.RouteHqUsOverNightTradeCodes)
	}
	// The request type is empty by design — the endpoint takes no parameters —
	// so the assertion is that the method forwards the value it was given rather
	// than inventing one.
	if _, ok := call.params.(UsOverNightTradeCodesRequest); !ok {
		t.Errorf("params is %T, want UsOverNightTradeCodesRequest", call.params)
	}
	requireCalls(t, exec, 1)

	if len(resp.SecurityCodes) != 3 {
		t.Fatalf("UsOverNightTradeCodes returned %d codes, want 3", len(resp.SecurityCodes))
	}
	if resp.SecurityCodes[0] != "AAPL" || resp.SecurityCodes[2] != "TSLA" {
		t.Errorf("security codes = %v, want the three in the reply", resp.SecurityCodes)
	}
}

func TestSubscribeHappyPath(t *testing.T) {
	exec := newSequencedExecutor(t, sequencedReply{reply: map[string]any{}})

	if err := NewMarketService(exec).Subscribe(t.Context(), types.TopicOrderBook,
		marketSecurity(),
		&Security{DataType: types.DataTypeHKIndex, Code: "HSI"},
	); err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	call := exec.lastCall(t)
	if call.op != opSubscribe || call.route != client.RouteHqSubscribe {
		t.Errorf("reached (%q, %q), want (%q, %q)", call.op, call.route, opSubscribe, client.RouteHqSubscribe)
	}
	params, ok := call.params.(subscribeRequest)
	if !ok {
		t.Fatalf("params is %T, want subscribeRequest", call.params)
	}
	if params.TopicID != types.TopicOrderBook {
		t.Errorf("request topicId = %d, want %d", params.TopicID, types.TopicOrderBook)
	}
	if len(params.Security) != 2 {
		t.Fatalf("request carries %d securities, want 2", len(params.Security))
	}
	if params.Security[1].DataType != int32(types.DataTypeHKIndex) || params.Security[1].Code != "HSI" {
		t.Errorf("request security[1] = %+v, want {10001 HSI}", params.Security[1])
	}
	requireCalls(t, exec, 1)
}

func TestUnsubscribeHappyPath(t *testing.T) {
	exec := newSequencedExecutor(t, sequencedReply{reply: map[string]any{}})

	if err := NewMarketService(exec).Unsubscribe(t.Context(), types.TopicTicker, marketSecurity()); err != nil {
		t.Fatalf("Unsubscribe: %v", err)
	}

	call := exec.lastCall(t)
	if call.op != opUnsubscribe || call.route != client.RouteHqUnsubscribe {
		t.Errorf("reached (%q, %q), want (%q, %q)", call.op, call.route,
			opUnsubscribe, client.RouteHqUnsubscribe)
	}
	params, ok := call.params.(subscribeRequest)
	if !ok {
		t.Fatalf("params is %T, want subscribeRequest (Unsubscribe reuses it)", call.params)
	}
	if params.TopicID != types.TopicTicker || len(params.Security) != 1 {
		t.Errorf("request = %+v, want topic 14 and one security", params)
	}
	requireCalls(t, exec, 1)
}

// TestMarketRequestBodiesReachTheWire drives the real client over a
// wireRecorder, so the request is proved as the bytes the Gateway receives
// rather than as a Go struct.
//
// The envelope is {"timeout_sec":N,"params":{…}}, and params is a
// json.RawMessage, so a field whose wire type changed from a quoted string to a
// bare number would still type-assert in the tests above and only show up here.
// That is the same class of failure P1 fixed, one layer out.
func TestMarketRequestBodiesReachTheWire(t *testing.T) {
	for _, tc := range []struct {
		name   string
		route  client.Route
		want   []string
		invoke func(context.Context, *MarketService) error
	}{
		{
			name:  "BasicQot carries every security in order",
			route: client.RouteHqBasicQot,
			want: []string{
				`"security":[{"dataType":10000,"code":"00700.HK"},{"dataType":20000,"code":"AAPL"}]`,
				`"needDelayFlag":"1"`,
				`"mktTmType":1`,
			},
			invoke: func(ctx context.Context, svc *MarketService) error {
				_, err := svc.BasicQot(ctx, BasicQotRequest{
					Security: []*Security{
						marketSecurity(),
						{DataType: types.DataTypeUSStock, Code: "AAPL"},
					},
					NeedDelayFlag: "1",
					MktTmType:     1,
				})
				return err
			},
		},
		{
			name:  "OrderBook omits depthBookType when it is zero",
			route: client.RouteHqOrderBook,
			want: []string{
				`"security":{"dataType":10000,"code":"00700.HK"}`,
				`"mktTmType":0`,
			},
			invoke: func(ctx context.Context, svc *MarketService) error {
				_, err := svc.OrderBook(ctx, OrderBookRequest{Security: marketSecurity()})
				return err
			},
		},
		{
			name:  "UsOptionChainCode omits the two optional members",
			route: client.RouteHqUsOptionChainCode,
			want:  []string{`{"securityCode":"AAPL","expireDate":""}`},
			invoke: func(ctx context.Context, svc *MarketService) error {
				_, err := svc.UsOptionChainCode(ctx, UsOptionChainCodeRequest{SecurityCode: "AAPL"})
				return err
			},
		},
		{
			name:  "UsOverNightTradeCodes sends an empty params object",
			route: client.RouteHqUsOverNightTradeCodes,
			want:  []string{`"params":{}`},
			invoke: func(ctx context.Context, svc *MarketService) error {
				_, err := svc.UsOverNightTradeCodes(ctx, UsOverNightTradeCodesRequest{})
				return err
			},
		},
		{
			name:  "Subscribe sends the topic and the security list",
			route: client.RouteHqSubscribe,
			want: []string{
				`{"topicId":14,"security":[{"dataType":10000,"code":"00700.HK"}]}`,
			},
			invoke: func(ctx context.Context, svc *MarketService) error {
				return svc.Subscribe(ctx, types.TopicTicker, marketSecurity())
			},
		},
		{
			name:  "Unsubscribe sends the topic and the security list",
			route: client.RouteHqUnsubscribe,
			want: []string{
				`{"topicId":11,"security":[{"dataType":10000,"code":"00700.HK"}]}`,
			},
			invoke: func(ctx context.Context, svc *MarketService) error {
				return svc.Unsubscribe(ctx, types.TopicBasicQot, marketSecurity())
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := newWireRecorder(nil)
			svc := NewMarketService(newWireExecutor(t, rec))

			if err := tc.invoke(t.Context(), svc); err != nil {
				t.Fatalf("%s over the wire: %v", tc.name, err)
			}
			body := rec.lastBody(t, string(tc.route))
			for _, want := range tc.want {
				if !strings.Contains(body, want) {
					t.Errorf("request body = %s\nwant it to contain %s", body, want)
				}
			}
			if got := rec.count(string(tc.route)); got != 1 {
				t.Errorf("requests to %s = %d, want exactly 1", tc.route, got)
			}
		})
	}
}

// TestConvertBrokerEntriesDirect calls the converter directly, for the three
// shapes a method-level test cannot reach: a nil slice, a single entry, and a
// three-entry slice. It is at 100% from the method tests already, so the point is
// the contract rather than the percentage — in particular that a nil input
// yields an empty non-nil slice, which a caller ranging over it cannot
// distinguish from an empty list and which is therefore safe either way.
func TestConvertBrokerEntriesDirect(t *testing.T) {
	if got := convertBrokerEntries(nil); len(got) != 0 {
		t.Errorf("convertBrokerEntries(nil) = %d entries, want 0", len(got))
	}
	if got := convertBrokerEntries([]*dto.Broker{}); len(got) != 0 {
		t.Errorf("convertBrokerEntries(empty) = %d entries, want 0", len(got))
	}

	one := convertBrokerEntries([]*dto.Broker{{Level: 1, Item: "1", Type: 0, Name: "a"}})
	if len(one) != 1 || one[0].Name != "a" {
		t.Fatalf("convertBrokerEntries(one) = %+v, want one entry named a", one)
	}

	three := convertBrokerEntries([]*dto.Broker{
		{Level: 1, Item: "1", Type: 0, Name: "a"},
		{Level: 2, Item: "2", Type: 0, Name: "b"},
		{Level: 3, Item: "3", Type: 1, Name: "c"},
	})
	if len(three) != 3 {
		t.Fatalf("convertBrokerEntries(three) = %d entries, want 3", len(three))
	}
	for i, want := range []string{"a", "b", "c"} {
		if three[i].Name != want {
			t.Errorf("entry %d name = %q, want %q", i, three[i].Name, want)
		}
		if three[i].Level != int32(i+1) {
			t.Errorf("entry %d level = %d, want %d", i, three[i].Level, i+1)
		}
	}
}
