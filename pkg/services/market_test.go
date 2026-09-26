// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package services

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/shing1211/hstongapi4go/gen/hq/dto"
)

// legacyRounded reproduces the conversion this file used before the fix, so the
// tests below can state what the old code did instead of only what the new code
// does. A test that pins the new behaviour alone would still pass if the value
// were being rounded; the one that matters compares against the rounding.
func legacyRounded(f float64) string { return fmt.Sprintf("%.3f", f) }

// orderBookBody is a minimal OrderBook reply. spreadLevel is written per case,
// because the point of these tests is how that one number is decoded.
func orderBookBody(spread string) json.RawMessage {
	return json.RawMessage(`{"security":{"dataType":10000,"code":"00700.HK"},` +
		`"orderBookAskList":[{"level":1,"price":0.0005,"volume":1200}],` +
		`"orderBookBidList":[]` + spread + `}`)
}

// TestOrderBookTickSizeKeepsTheWireDigits is the regression test for the defect
// this change fixes. The wire field used to be a float64 rendered with
// fmt.Sprintf("%.3f", …), which hardcoded a three-decimal scale the Gateway never
// promised: a 0.0005 tick came out as 0.001, a different and wrong price, with no
// error anywhere. Every case below must reach domain.Price untouched.
func TestOrderBookTickSizeKeepsTheWireDigits(t *testing.T) {
	tests := []struct {
		name   string
		spread string
		want   string
	}{
		{
			// The headline case: a sub-milli tick rounded up to 0.001 by the
			// old %.3f conversion.
			name:   "unquoted sub-millisecond tick",
			spread: `,"spreadLevel":0.0005`,
			want:   "0.0005",
		},
		{
			name:   "unquoted millisecond tick",
			spread: `,"spreadLevel":0.001`,
			want:   "0.001",
		},
		{
			name:   "unquoted integral spread",
			spread: `,"spreadLevel":0.2`,
			want:   "0.2",
		},
		{
			// 0.1 has no exact binary representation, so this pins that the
			// shortest round-trip form is what arrives, not a rounded "0.100".
			name:   "unquoted non-representable binary fraction",
			spread: `,"spreadLevel":0.1`,
			want:   "0.1",
		},
		{
			// The quoted form decodes too, so a Gateway that switches to
			// quoted numerics cannot turn into a hard decode failure.
			name:   "quoted sub-millisecond tick",
			spread: `,"spreadLevel":"0.0005"`,
			want:   "0.0005",
		},
		{
			// More significant digits than a float64 can hold: this is the
			// case a float64 wire field could never pass, and the reason the
			// field is json.Number rather than float64.
			name:   "more precision than binary64",
			spread: `,"spreadLevel":123.4567890123456789`,
			want:   "123.4567890123456789",
		},
		{
			// An exponent on the wire must not survive as "1e-4": the domain
			// constructors are decimal and the canonical form is positional.
			name:   "exponent form",
			spread: `,"spreadLevel":1e-4`,
			want:   "0.0001",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := NewMarketService(&fakeExecutor{reply: orderBookBody(tt.spread)})
			resp, err := svc.OrderBook(t.Context(), OrderBookRequest{
				Security: &Security{DataType: 10000, Code: "00700.HK"},
			})
			if err != nil {
				t.Fatalf("OrderBook: %v", err)
			}
			if got := resp.TickSize.String(); got != tt.want {
				t.Errorf("TickSize = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestOrderBookLevelPriceKeepsSubMilliDigit proves the same for the prices inside
// the book, which took the same %.3f path. The ask level carries 0.0005.
func TestOrderBookLevelPriceKeepsSubMilliDigit(t *testing.T) {
	svc := NewMarketService(&fakeExecutor{reply: orderBookBody(`,"spreadLevel":0.001`)})
	resp, err := svc.OrderBook(t.Context(), OrderBookRequest{
		Security: &Security{DataType: 10000, Code: "00700.HK"},
	})
	if err != nil {
		t.Fatalf("OrderBook: %v", err)
	}
	if len(resp.Ask) != 1 {
		t.Fatalf("Ask levels = %d, want 1", len(resp.Ask))
	}
	if got := resp.Ask[0].Price.String(); got != "0.0005" {
		t.Errorf("Ask[0].Price = %q, want 0.0005 (the old 3-decimal conversion gave %q)",
			got, legacyRounded(0.0005))
	}
}

// TestOrderBookTickSizeDefaultsToZeroWhenAbsent covers the case the switch to a
// string-based wire field introduced: a missing or null spreadLevel leaves the
// field empty, and the domain constructors panic on an unparseable value. The old
// float64 field defaulted to 0, so the empty case has to keep meaning zero rather
// than take down the caller's goroutine.
func TestOrderBookTickSizeDefaultsToZeroWhenAbsent(t *testing.T) {
	tests := []struct{ name, spread string }{
		{"key absent", ``},
		{"explicit null", `,"spreadLevel":null`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := NewMarketService(&fakeExecutor{reply: orderBookBody(tt.spread)})
			resp, err := svc.OrderBook(t.Context(), OrderBookRequest{
				Security: &Security{DataType: 10000, Code: "00700.HK"},
			})
			if err != nil {
				t.Fatalf("OrderBook: %v", err)
			}
			if got := resp.TickSize.String(); got != "0" {
				t.Errorf("TickSize = %q, want 0 for a %s spreadLevel", got, tt.name)
			}
		})
	}
}

// TestOrderBookTickSizeRejectsMalformedValue asserts a malformed number still
// fails loudly. Only the empty case is normalised to zero; substituting a number
// for a price the Gateway garbled would be worse than an error.
func TestOrderBookTickSizeRejectsMalformedValue(t *testing.T) {
	svc := NewMarketService(&fakeExecutor{reply: orderBookBody(`,"spreadLevel":"0.00x5"`)})
	if _, err := svc.OrderBook(t.Context(), OrderBookRequest{
		Security: &Security{DataType: 10000, Code: "00700.HK"},
	}); err == nil {
		t.Fatal("OrderBook with a malformed spreadLevel = nil error, want a decode failure")
	}
}

// TestFloatToStringIsPrecisionPreserving pins the conversion every price, money,
// and rate in this file depends on. The 'f' format with precision -1 yields the
// shortest decimal string that parses back to the same float, and never an
// exponent: "1e-07" would panic in domain.MustNewPrice.
func TestFloatToStringIsPrecisionPreserving(t *testing.T) {
	tests := []struct {
		in   float64
		want string
	}{
		{0, "0"},
		{0.0005, "0.0005"},
		{0.001, "0.001"},
		{0.1, "0.1"},
		{0.2, "0.2"},
		{0.3, "0.3"},
		{1.0 / 3.0, "0.3333333333333333"},
		{380, "380"},
		{1234.5678, "1234.5678"},
		{0.0000001, "0.0000001"},
		{4800000000, "4800000000"},
		{-2.25, "-2.25"},
	}
	for _, tt := range tests {
		if got := floatToString(tt.in); got != tt.want {
			t.Errorf("floatToString(%v) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

// TestFloatToStringDivergesFromTheLegacyRounding is the assertion that keeps the
// rest of this file honest: the two conversions must disagree wherever %.3f lost
// something. If they ever agreed everywhere, the replacements would be a
// cosmetic change and the precision argument would not apply.
func TestFloatToStringDivergesFromTheLegacyRounding(t *testing.T) {
	tests := []struct {
		in  float64
		old string
	}{
		{0.0005, "0.001"},
		{0.0001, "0.000"},
		{0.1, "0.100"},
		{1.0 / 3.0, "0.333"},
		{0.0000001, "0.000"},
	}
	for _, tt := range tests {
		if got := legacyRounded(tt.in); got != tt.old {
			t.Errorf("legacyRounded(%v) = %q, want %q (the value the old code produced)", tt.in, got, tt.old)
		}
		if floatToString(tt.in) == tt.old {
			t.Errorf("floatToString(%v) = %q, which is the old rounded value: the fix removed no precision",
				tt.in, floatToString(tt.in))
		}
	}
}

// TestConvertersPreserveQuoteValues covers the DTO converters the BasicQot reply
// goes through. 0.0005 is the value the old %.3f rewrote, 0.1 the value no binary
// float holds exactly, and 4800000000.12345 a turnover that must keep its cents.
func TestConvertersPreserveQuoteValues(t *testing.T) {
	q := convertToQuote(&dto.BasicQot{
		OpenPrice:      0.0005,
		HighPrice:      0.1,
		LowPrice:       0.2,
		LastPrice:      0.3,
		LastClosePrice: 0.0001,
		PriceSpread:    0.0005,
		Volume:         100,
		Turnover:       4800000000.12345,
		TurnoverRate:   0.00005,
		Amplitude:      1.0 / 3.0,
	})
	prices := []struct {
		field string
		got   string
		want  string
	}{
		{"OpenPrice", q.OpenPrice.String(), "0.0005"},
		{"HighPrice", q.HighPrice.String(), "0.1"},
		{"LowPrice", q.LowPrice.String(), "0.2"},
		{"LastPrice", q.LastPrice.String(), "0.3"},
		{"LastClosePrice", q.LastClosePrice.String(), "0.0001"},
		{"PriceSpread", q.PriceSpread.String(), "0.0005"},
		{"Turnover", q.Turnover.String(), "4800000000.12345"},
		{"TurnoverRate", q.TurnoverRate.String(), "0.00005"},
		{"Amplitude", q.Amplitude.String(), "0.3333333333333333"},
	}
	for _, p := range prices {
		if p.got != p.want {
			t.Errorf("Quote.%s = %q, want %q", p.field, p.got, p.want)
		}
	}
	if got := q.Volume.String(); got != "100" {
		t.Errorf("Quote.Volume = %q, want 100", got)
	}
}

// TestConvertersPreserveKLineValues covers the KLine converter, whose five prices
// and turnover took the same %.3f path.
func TestConvertersPreserveKLineValues(t *testing.T) {
	k := convertToKLine(&dto.KLine{
		HighPrice:      0.0005,
		OpenPrice:      0.0001,
		LowPrice:       0.1,
		ClosePrice:     0.2,
		LastClosePrice: 0.3,
		Volume:         7,
		Turnover:       381200.125,
	})
	if got := k.HighPrice.String(); got != "0.0005" {
		t.Errorf("KLine.HighPrice = %q, want 0.0005", got)
	}
	if got := k.OpenPrice.String(); got != "0.0001" {
		t.Errorf("KLine.OpenPrice = %q, want 0.0001", got)
	}
	if got := k.LowPrice.String(); got != "0.1" {
		t.Errorf("KLine.LowPrice = %q, want 0.1", got)
	}
	if got := k.ClosePrice.String(); got != "0.2" {
		t.Errorf("KLine.ClosePrice = %q, want 0.2", got)
	}
	if got := k.LastClosePrice.String(); got != "0.3" {
		t.Errorf("KLine.LastClosePrice = %q, want 0.3", got)
	}
	if got := k.Turnover.String(); got != "381200.125" {
		t.Errorf("KLine.Turnover = %q, want 381200.125", got)
	}
}

// TestConvertersPreserveTimeShareAndTickerValues covers the remaining two
// converters that carried %.3f prices.
func TestConvertersPreserveTimeShareAndTickerValues(t *testing.T) {
	ts := convertToTimeSharePoint(&dto.TimeShare{
		Price:          0.0005,
		LastClosePrice: 0.0001,
		AvgPrice:       0.1,
		Volume:         1000,
		Turnover:       381200.125,
	})
	if got := ts.Price.String(); got != "0.0005" {
		t.Errorf("TimeSharePoint.Price = %q, want 0.0005", got)
	}
	if got := ts.LastClosePrice.String(); got != "0.0001" {
		t.Errorf("TimeSharePoint.LastClosePrice = %q, want 0.0001", got)
	}
	if got := ts.AvgPrice.String(); got != "0.1" {
		t.Errorf("TimeSharePoint.AvgPrice = %q, want 0.1", got)
	}
	if got := ts.Turnover.String(); got != "381200.125" {
		t.Errorf("TimeSharePoint.Turnover = %q, want 381200.125", got)
	}

	tick := convertToTickerTick(&dto.Ticker{Price: 0.0005, Volume: 100, Turnover: 38800.125})
	if got := tick.Price.String(); got != "0.0005" {
		t.Errorf("TickerTick.Price = %q, want 0.0005", got)
	}
	if got := tick.Turnover.String(); got != "38800.125" {
		t.Errorf("TickerTick.Turnover = %q, want 38800.125", got)
	}
}

// TestConvertToOrderBookLevelsPreservesPrice covers the level converter directly,
// including the empty-slice case so the loop bound is pinned.
func TestConvertToOrderBookLevelsPreservesPrice(t *testing.T) {
	if got := convertOrderBookLevels(nil); len(got) != 0 {
		t.Errorf("convertOrderBookLevels(nil) = %d levels, want 0", len(got))
	}
	levels := convertOrderBookLevels([]*dto.OrderBook{
		{Level: 1, Price: 0.0005, Volume: 1200},
		{Level: 2, Price: 388.2, Volume: 800},
	})
	if got := levels[0].Price.String(); got != "0.0005" {
		t.Errorf("level 1 price = %q, want 0.0005", got)
	}
	if got := levels[1].Price.String(); got != "388.2" {
		t.Errorf("level 2 price = %q, want 388.2", got)
	}
	if got := levels[0].Quantity.String(); got != "1200" {
		t.Errorf("level 1 quantity = %q, want 1200", got)
	}
}

// TestConvertersPassThroughNil guards the nil early-returns the converters rely on.
func TestConvertersPassThroughNil(t *testing.T) {
	if got := convertToQuote(nil); got != nil {
		t.Error("convertToQuote(nil) did not return nil")
	}
	if got := convertToKLine(nil); got != nil {
		t.Error("convertToKLine(nil) did not return nil")
	}
	if got := convertToTimeSharePoint(nil); got != nil {
		t.Error("convertToTimeSharePoint(nil) did not return nil")
	}
	if got := convertToTickerTick(nil); got != nil {
		t.Error("convertToTickerTick(nil) did not return nil")
	}
}

// TestDecimalOrZero covers the empty-field guard the string-based wire field needs.
func TestDecimalOrZero(t *testing.T) {
	tests := []struct{ in, want string }{
		{"", "0"},
		{"0.0005", "0.0005"},
		{"0", "0"},
		{"1e-4", "1e-4"},
	}
	for _, tt := range tests {
		if got := decimalOrZero(tt.in); got != tt.want {
			t.Errorf("decimalOrZero(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
