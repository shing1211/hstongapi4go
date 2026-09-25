// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package transport

import (
	"testing"

	"github.com/shing1211/hstongapi4go/gen/hq/dto"
	"github.com/shing1211/hstongapi4go/gen/hq/notify"
	tradenotify "github.com/shing1211/hstongapi4go/gen/trade/notify"
	"github.com/shing1211/hstongapi4go/pkg/domain"
	"github.com/shing1211/hstongapi4go/pkg/types"
)

func TestMarketFromCode(t *testing.T) {
	tests := []struct {
		code string
		want domain.Market
	}{
		{"00700.HK", domain.MarketHK},
		{"aapl.US", domain.MarketUS},
		{"000001.SZ", domain.MarketShenzhenConnect},
		{"600000.SH", domain.MarketShanghaiConnect},
		{"00700.hk", domain.MarketHK},
		{"NOEXT", ""},
		{"", ""},
	}
	for _, tt := range tests {
		if got := marketFromCode(tt.code); got != tt.want {
			t.Errorf("marketFromCode(%q) = %q, want %q", tt.code, got, tt.want)
		}
	}
}

// TestFloatToStringAvoidsScientificNotation guards the conversion the price and
// money mappers depend on. A JSON number rendered as "1e-07" would panic in
// domain.MustNewPrice, and a trailing ".0" would corrupt a money value.
func TestFloatToStringAvoidsScientificNotation(t *testing.T) {
	tests := []struct {
		in   float64
		want string
	}{
		{0, "0"},
		{100, "100"},
		{100.5, "100.5"},
		{0.0000001, "0.0000001"},
		{-2.25, "-2.25"},
	}
	for _, tt := range tests {
		if got := floatToString(tt.in); got != tt.want {
			t.Errorf("floatToString(%v) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestMapSecurityToSymbol(t *testing.T) {
	s := MapSecurityToSymbol("00700.HK", int32(types.DataTypeHKStock))
	if s.Market != domain.MarketHK {
		t.Errorf("Market = %q, want %q", s.Market, domain.MarketHK)
	}
	if s.DataType != types.DataTypeHKStock {
		t.Errorf("DataType = %v, want %v", s.DataType, types.DataTypeHKStock)
	}
}

func TestMapBasicQotToQuoteEvent(t *testing.T) {
	n := &notify.BasicQotNotify{
		Security: &dto.Security{Code: "00700.HK", DataType: int32(types.DataTypeHKStock)},
		BasicQot: &dto.BasicQot{
			LastPrice:      101.5,
			OpenPrice:      100,
			HighPrice:      102,
			LowPrice:       99.5,
			LastClosePrice: 100,
			Volume:         1234,
			Turnover:       125251,
			MarketTime:     "2026-09-25 10:00",
		},
	}
	ev := MapBasicQotToQuoteEvent(n)
	if ev.Symbol.Code != "00700.HK" || ev.Symbol.Market != domain.MarketHK {
		t.Errorf("Symbol = %+v", ev.Symbol)
	}
	if ev.LastPrice.String() != "101.5" {
		t.Errorf("LastPrice = %q, want 101.5", ev.LastPrice)
	}
	if ev.ClosePrice.String() != "100" {
		t.Errorf("ClosePrice = %q, want 100", ev.ClosePrice)
	}
	if ev.Volume.String() != "1234" {
		t.Errorf("Volume = %q, want 1234", ev.Volume)
	}
	if ev.Turnover.Currency() != "HKD" {
		t.Errorf("Turnover currency = %q, want HKD", ev.Turnover.Currency())
	}
	if ev.Timestamp != "2026-09-25 10:00" {
		t.Errorf("Timestamp = %q", ev.Timestamp)
	}
}

// TestMapBasicQotNilIsZeroValue documents that the generated getters are
// nil-safe, so a malformed notification cannot panic the stream dispatcher.
func TestMapBasicQotNilIsZeroValue(t *testing.T) {
	ev := MapBasicQotToQuoteEvent(nil)
	if !ev.Symbol.IsZero() {
		t.Errorf("Symbol = %+v, want zero", ev.Symbol)
	}
	if !ev.Volume.IsZero() {
		t.Errorf("Volume = %q, want zero", ev.Volume)
	}
	if !ev.Turnover.IsZero() {
		t.Errorf("Turnover = %q, want zero", ev.Turnover)
	}
}

func TestMapTickerToTickerEvent(t *testing.T) {
	ev := MapTickerToTickerEvent(&dto.Ticker{
		Price:     9.87,
		Volume:    500,
		Turnover:  4935,
		Timestamp: 1700,
		Side:      1,
	})
	if ev.Price.String() != "9.87" {
		t.Errorf("Price = %q, want 9.87", ev.Price)
	}
	if ev.Volume.String() != "500" {
		t.Errorf("Volume = %q, want 500", ev.Volume)
	}
	if ev.Side != types.EntrustBS("1") {
		t.Errorf("Side = %q, want \"1\"", ev.Side)
	}
	if ev.Timestamp != "1700" {
		t.Errorf("Timestamp = %q, want \"1700\"", ev.Timestamp)
	}
	// dto.Ticker carries no security, so the mapper cannot resolve a symbol.
	// This is a known limitation rather than a bug in the test: assert it
	// explicitly so a future change to the wire type is noticed here.
	if !ev.Symbol.IsZero() {
		t.Errorf("Symbol = %+v, want zero (dto.Ticker carries no security)", ev.Symbol)
	}
}

func TestMapOrderBookAskLevel(t *testing.T) {
	lvl := MapOrderBookAskLevel(&dto.OrderBook{Price: 101.25, Volume: 300})
	if lvl.Price.String() != "101.25" {
		t.Errorf("Price = %q, want 101.25", lvl.Price)
	}
	if lvl.Quantity.String() != "300" {
		t.Errorf("Quantity = %q, want 300", lvl.Quantity)
	}
}

func TestMapTradeDeliveryToTradeEvent(t *testing.T) {
	ev := MapTradeDeliveryToTradeEvent(&tradenotify.TradeStockDeliverNotify{
		StockCode:          "00700.HK",
		ClientId:           "CLIENT-1",
		EntrustNo:          "ENTRUST-1",
		BusinessPrice:      "101.5",
		BusinessAmount:     "200",
		SumBusinessBalance: "20300",
		EntrustBs:          "1",
		BusinessDate:       "2026-09-25",
		BusinessTime:       "10:00:01",
		MatchNo:            "MATCH-1",
		EntrustStatus:      "8",
	})
	if ev.Symbol.Code != "00700.HK" || ev.Symbol.Market != domain.MarketHK {
		t.Errorf("Symbol = %+v", ev.Symbol)
	}
	if ev.OrderID != domain.OrderID("CLIENT-1") {
		t.Errorf("OrderID = %q", ev.OrderID)
	}
	if ev.EntrustID != domain.EntrustID("ENTRUST-1") {
		t.Errorf("EntrustID = %q", ev.EntrustID)
	}
	if ev.Price.String() != "101.5" {
		t.Errorf("Price = %q, want 101.5", ev.Price)
	}
	if ev.Quantity.String() != "200" {
		t.Errorf("Quantity = %q, want 200", ev.Quantity)
	}
	if ev.Turnover.String() != "20300" {
		t.Errorf("Turnover = %q, want 20300", ev.Turnover)
	}
	if ev.Side != types.EntrustBS("1") {
		t.Errorf("Side = %q", ev.Side)
	}
	if ev.OrderStatus != types.EntrustStatus("8") {
		t.Errorf("OrderStatus = %q, want 8", ev.OrderStatus)
	}
	if ev.Timestamp != "2026-09-25 10:00:01" {
		t.Errorf("Timestamp = %q, want the date and time joined", ev.Timestamp)
	}
	if ev.CounterID != "MATCH-1" {
		t.Errorf("CounterID = %q", ev.CounterID)
	}
}

// TestMapTradeDeliveryNilIsZeroValue pins nil-safety for the trade mapper, which
// the quote mapper gets for free because it formats float fields. Unset
// protobuf string fields are empty, and the domain constructors panic on an
// unparseable value, so this would have panicked a consumer goroutine.
func TestMapTradeDeliveryNilIsZeroValue(t *testing.T) {
	ev := MapTradeDeliveryToTradeEvent(nil)
	if !ev.Symbol.IsZero() {
		t.Errorf("Symbol = %+v, want zero", ev.Symbol)
	}
	if !ev.Price.IsZero() {
		t.Errorf("Price = %q, want zero", ev.Price)
	}
	if !ev.Quantity.IsZero() {
		t.Errorf("Quantity = %q, want zero", ev.Quantity)
	}
	if !ev.Turnover.IsZero() {
		t.Errorf("Turnover = %q, want zero", ev.Turnover)
	}
}

// TestMapTradeDeliveryEmptyStringFieldsIsZeroValue covers the realistic
// malformed case: a delivery message that parsed fine but carries no price,
// quantity, or turnover.
func TestMapTradeDeliveryEmptyStringFieldsIsZeroValue(t *testing.T) {
	ev := MapTradeDeliveryToTradeEvent(&tradenotify.TradeStockDeliverNotify{
		StockCode: "00700.HK",
	})
	if !ev.Price.IsZero() || !ev.Quantity.IsZero() || !ev.Turnover.IsZero() {
		t.Errorf("empty string fields did not map to zero: price=%q qty=%q turnover=%q",
			ev.Price, ev.Quantity, ev.Turnover)
	}
}

func TestDecimalOrZero(t *testing.T) {
	if got := decimalOrZero(""); got != "0" {
		t.Errorf("decimalOrZero(%q) = %q, want \"0\"", "", got)
	}
	if got := decimalOrZero("12.5"); got != "12.5" {
		t.Errorf("decimalOrZero(%q) = %q, want it unchanged", "12.5", got)
	}
}
