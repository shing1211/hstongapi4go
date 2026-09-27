// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package services

import (
	"encoding/json"
	"strconv"
	"strings"
	"testing"

	"github.com/shing1211/hstongapi4go/client"
	"github.com/shing1211/hstongapi4go/pkg/domain"
)

// This file is the regression net for the defect this package already shipped:
// fmt.Sprintf("%.3f", …) hardcoded a three-decimal scale the Gateway never
// promised, so a 0.0005 tick became 0.001 — a different and wrong price, with
// no error anywhere. P1 removed 23 such conversions; these tests are the reason
// they cannot come back silently.
//
// Two value sets, because there are two different failure modes and a value that
// discriminates one is often blind to the other.
//
//   - float64HostilePrices / float64HostileQuantities carry more significant
//     digits than binary64 can hold, or underflow it. A float64 wire field
//     cannot pass them; only a verbatim decimal field can.
//   - scaleHostilePrices are float64-stable, so a float64 bug would not show —
//     but a fixed-scale formatter still destroys them, and that is exactly the
//     bug that shipped. All four survive a float64 round-trip untouched, which
//     is why they need their own table.
//
// The guard tests below assert that property on every row. Without them a future
// edit that softened a literal into a benign one would remove the net without
// failing anything, which is the same class of quiet regression as the bug
// itself.
//
// One honest limit is stated rather than papered over. Only one field in this
// file can carry a value verbatim: OrderBookResponse.TickSize, which P1 typed
// json.Number. Every other market numeric is a generated float64, so
// encoding/json has already destroyed the extra digits before floatToString sees
// them. Those rows therefore assert the binary64 ceiling, and TestSpreadLevelIs
// TheOnlyVerbatimCapableMarketField puts the two side by side on the same wire
// value so the difference is impossible to mistake for sloppiness.

// binary64Ceiling is what the float64 DTO fields can honestly promise: the
// shortest decimal string that parses back to the same float64. It is the
// ceiling the generated wire types impose, not a defect of this package.
func binary64Ceiling(t *testing.T, in string) string {
	t.Helper()
	f, err := strconv.ParseFloat(in, 64)
	if err != nil {
		t.Fatalf("test bug: %q is not a JSON number this file can put on the wire: %v", in, err)
	}
	return strconv.FormatFloat(f, 'f', -1, 64)
}

// orderBookLevelBody is an OrderBook reply with one ask level carrying the given
// price literal, written unquoted exactly as the Gateway sends it.
func orderBookLevelBody(price string) json.RawMessage {
	return json.RawMessage(`{"security":{"dataType":10000,"code":"00700.HK"},` +
		`"orderBookAskList":[{"level":1,"price":` + price + `,"volume":1200}],` +
		`"orderBookBidList":[],"spreadLevel":0.001}`)
}

// spreadLevelBody is an OrderBook reply whose spreadLevel is the given literal,
// written exactly as supplied so the caller controls the quoting.
func spreadLevelBody(spread string) json.RawMessage {
	return json.RawMessage(`{"security":{"dataType":10000,"code":"00700.HK"},` +
		`"orderBookAskList":[],"orderBookBidList":[],"spreadLevel":` + spread + `}`)
}

// TestMoneyValueSetsStayHostile asserts the property that makes each row worth
// having. It is the guard on the guard: a future edit that softened a literal
// would otherwise delete the regression net without any test noticing.
func TestMoneyValueSetsStayHostile(t *testing.T) {
	if len(float64HostilePrices) == 0 || len(float64HostileQuantities) == 0 || len(scaleHostilePrices) == 0 {
		t.Fatal("a money value set is empty, so the round-trip tests below would pass vacuously")
	}
	for _, set := range []struct {
		label string
		cases []moneyCase
		guard func(*testing.T, string)
	}{
		{"float64HostilePrices", float64HostilePrices, requireFloat64Hostile},
		{"float64HostileQuantities", float64HostileQuantities, requireFloat64Hostile},
		{"scaleHostilePrices", scaleHostilePrices, requireScaleHostile},
	} {
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
}

// TestScaleHostileSetIsFloat64Stable is why the two tables cannot be merged.
//
// Every value in the scale-hostile set survives a float64 round-trip untouched,
// so a test that only checked "does the float64 path preserve this?" would pass
// for all four and catch nothing. They earn their place by being destroyed by
// %.3f, which is asserted by requireScaleHostile and by the method-level tests
// below. Asserting the stability here documents the asymmetry in the one place a
// reader will look when wondering why there are two sets.
func TestScaleHostileSetIsFloat64Stable(t *testing.T) {
	for _, tc := range scaleHostilePrices {
		t.Run(tc.name, func(t *testing.T) {
			if got := binary64Ceiling(t, tc.in); got != tc.in {
				t.Errorf("%q is float64-hostile too (%q), so it belongs in the other table "+
					"and this row would not isolate a scale bug", tc.in, got)
			}
			// Both properties at once: unchanged by a float64, destroyed by a
			// three-decimal format. That combination is what makes the row
			// specific to the shipped bug.
			requireScaleHostile(t, tc.in)
		})
	}
	// And the converse for the float64 table, at least for the values that are
	// representable at all: they must differ from their float64 form, or they
	// would be testing the same thing twice.
	for _, tc := range float64HostilePrices {
		t.Run("float64-hostile/"+tc.name, func(t *testing.T) {
			if ceiling := binary64Ceiling(t, tc.in); ceiling == tc.in {
				t.Errorf("%q survives a float64 round-trip, so it is not float64-hostile", tc.in)
			}
		})
	}
}

// TestOrderBookSpreadLevelCarriesHostileValuesVerbatim is the byte-for-byte
// round trip: the digits the Gateway wrote in spreadLevel are the digits the
// domain value reports.
//
// This is the assertion that would fail first if a fixed-scale conversion came
// back, and it only passes because P1 typed the field json.Number. With a
// float64 field, the first row would report 123.45678901234568 and the
// underflowing row would report 0; with a plain string field, the unquoted form
// the Gateway actually sends would not decode at all.
func TestOrderBookSpreadLevelCarriesHostileValuesVerbatim(t *testing.T) {
	for _, set := range []struct {
		label string
		cases []moneyCase
	}{
		{"float64-hostile", float64HostilePrices},
		{"scale-hostile", scaleHostilePrices},
	} {
		t.Run(set.label, func(t *testing.T) {
			for _, tc := range set.cases {
				t.Run(tc.name, func(t *testing.T) {
					exec := newSequencedExecutor(t, sequencedReply{reply: spreadLevelBody(tc.in)})
					resp, err := NewMarketService(exec).OrderBook(t.Context(),
						OrderBookRequest{Security: marketSecurity()})
					if err != nil {
						t.Fatalf("OrderBook with spreadLevel %s: %v", tc.in, err)
					}
					if got := resp.TickSize.String(); got != tc.want {
						t.Errorf("TickSize = %q, want %q (the digits the Gateway sent). "+
							"A fixed-scale conversion of %v would have produced %q",
							got, tc.want, tc.in, legacyRounded(mustFloat(t, tc.in)))
					}
				})
			}
		})
	}
}

// TestOrderBookSpreadLevelSurvivesTheRealStack runs the same values through a
// real client over a real HTTP server, in both the unquoted and the quoted
// spelling, so the guarantee is proved on the whole stack rather than only
// through a fake.
//
// The quoted form is here because P1 documented that json.Number accepts it: a
// Gateway that switched to quoted numerics must not turn into a hard decode
// failure, and a field typed plain string would have made the unquoted case fail
// outright.
func TestOrderBookSpreadLevelSurvivesTheRealStack(t *testing.T) {
	all := make([]moneyCase, 0, len(float64HostilePrices)+len(scaleHostilePrices))
	all = append(all, float64HostilePrices...)
	all = append(all, scaleHostilePrices...)

	for _, tc := range all {
		t.Run(tc.name, func(t *testing.T) {
			for _, spelling := range []struct {
				name    string
				literal string
			}{
				{"unquoted", tc.in},
				{"quoted", `"` + tc.in + `"`},
			} {
				t.Run(spelling.name, func(t *testing.T) {
					rec := newWireRecorder(map[string]string{
						string(client.RouteHqOrderBook): gatewaySuccess(string(spreadLevelBody(spelling.literal))),
					})
					svc := NewMarketService(newWireExecutor(t, rec))

					resp, err := svc.OrderBook(t.Context(), OrderBookRequest{Security: marketSecurity()})
					if err != nil {
						t.Fatalf("OrderBook over the wire with a %s spreadLevel: %v", spelling.name, err)
					}
					if got := resp.TickSize.String(); got != tc.want {
						t.Errorf("%s spreadLevel %s decoded to %q, want %q",
							spelling.name, spelling.literal, got, tc.want)
					}
					if got := rec.count(string(client.RouteHqOrderBook)); got != 1 {
						t.Errorf("requests = %d, want exactly 1", got)
					}
				})
			}
		})
	}
}

// TestConvertedLevelPriceSurvivesTheScaleHostileSet is the test that dies if
// %.3f is reintroduced on a converted price, and it is the direct replacement
// for the assertion market_test.go makes on one value.
//
// market_test.go covers 0.0005 in an order book level. This walks the whole
// scale-hostile set through the same code path, so a mutation is caught by name
// rather than by luck, and each row also prints the value the old conversion
// would have produced so a failure says what went wrong rather than only that
// something did.
func TestConvertedLevelPriceSurvivesTheScaleHostileSet(t *testing.T) {
	for _, tc := range scaleHostilePrices {
		t.Run(tc.name, func(t *testing.T) {
			exec := newSequencedExecutor(t, sequencedReply{reply: orderBookLevelBody(tc.in)})
			resp, err := NewMarketService(exec).OrderBook(t.Context(),
				OrderBookRequest{Security: marketSecurity()})
			if err != nil {
				t.Fatalf("OrderBook with a level price of %s: %v", tc.in, err)
			}
			if len(resp.Ask) != 1 {
				t.Fatalf("Ask levels = %d, want 1", len(resp.Ask))
			}
			got := resp.Ask[0].Price.String()
			if got != tc.want {
				t.Errorf("Ask[0].Price = %q, want %q. The removed conversion "+
					"fmt.Sprintf(\"%%.3f\", %s) produced %q",
					got, tc.want, tc.in, legacyRounded(mustFloat(t, tc.in)))
			}
		})
	}
}

// TestFloat64WireFieldsAdmitOnlyTheBinary64Ceiling states the limit plainly, on
// all four converters that carry a price.
//
// The generated DTO numerics are float64, so encoding/json has already reduced
// these values to binary64 before floatToString sees them. The most this layer
// can honestly promise is the shortest string that parses back to the same
// float64 — and asserting that is what stops a reader from mistaking the
// response path for a verbatim one. Each row additionally asserts the value is
// NOT the literal, so the day a converter grows the ceiling, this test says so
// instead of leaving the illusion in place.
func TestFloat64WireFieldsAdmitOnlyTheBinary64Ceiling(t *testing.T) {
	for _, tc := range float64HostilePrices {
		t.Run(tc.name, func(t *testing.T) {
			ceiling := binary64Ceiling(t, tc.in)

			t.Run("orderBookLevel", func(t *testing.T) {
				exec := newSequencedExecutor(t, sequencedReply{reply: orderBookLevelBody(tc.in)})
				resp, err := NewMarketService(exec).OrderBook(t.Context(),
					OrderBookRequest{Security: marketSecurity()})
				if err != nil {
					t.Fatalf("OrderBook: %v", err)
				}
				if got := resp.Ask[0].Price.String(); got != ceiling {
					t.Errorf("Ask[0].Price = %q, want the binary64 ceiling %q", got, ceiling)
				}
				if ceiling == tc.in {
					t.Errorf("%q survived the float64 DTO field; the ceiling has moved and this "+
						"test's comment is now wrong", tc.in)
				}
			})

			t.Run("kline", func(t *testing.T) {
				body := `{"kline":[{"date":"20260126","closePrice":` + tc.in + `,"volume":1,"turnover":1}]}`
				exec := newSequencedExecutor(t, sequencedReply{reply: json.RawMessage(body)})
				resp, err := NewMarketService(exec).KL(t.Context(), KLRequest{Security: marketSecurity()})
				if err != nil {
					t.Fatalf("KL: %v", err)
				}
				if got := resp.Kline[0].ClosePrice.String(); got != ceiling {
					t.Errorf("KLine[0].ClosePrice = %q, want the binary64 ceiling %q", got, ceiling)
				}
			})

			t.Run("timeShare", func(t *testing.T) {
				body := `{"timeShare":[{"time":"09:30","price":` + tc.in + `,"volume":1,"turnover":1}]}`
				exec := newSequencedExecutor(t, sequencedReply{reply: json.RawMessage(body)})
				resp, err := NewMarketService(exec).TimeShare(t.Context(),
					TimeShareRequest{Security: marketSecurity()})
				if err != nil {
					t.Fatalf("TimeShare: %v", err)
				}
				if got := resp.TimeShare[0].Price.String(); got != ceiling {
					t.Errorf("TimeSharePoint[0].Price = %q, want the binary64 ceiling %q", got, ceiling)
				}
			})

			t.Run("ticker", func(t *testing.T) {
				body := `{"ticker":[{"time":"09:30","price":` + tc.in + `,"volume":1,"turnover":1}]}`
				exec := newSequencedExecutor(t, sequencedReply{reply: json.RawMessage(body)})
				resp, err := NewMarketService(exec).Ticker(t.Context(),
					TickerRequest{Security: marketSecurity(), Limit: 5})
				if err != nil {
					t.Fatalf("Ticker: %v", err)
				}
				if got := resp.Ticker[0].Price.String(); got != ceiling {
					t.Errorf("TickerTick[0].Price = %q, want the binary64 ceiling %q", got, ceiling)
				}
			})

			t.Run("basicQot", func(t *testing.T) {
				body := `{"basicQot":[{"lastPrice":` + tc.in + `,"volume":1,"turnover":1}]}`
				exec := newSequencedExecutor(t, sequencedReply{reply: json.RawMessage(body)})
				resp, err := NewMarketService(exec).BasicQot(t.Context(),
					BasicQotRequest{Security: []*Security{marketSecurity()}})
				if err != nil {
					t.Fatalf("BasicQot: %v", err)
				}
				if got := resp.BasicQot[0].LastPrice.String(); got != ceiling {
					t.Errorf("Quote[0].LastPrice = %q, want the binary64 ceiling %q", got, ceiling)
				}
			})
		})
	}
}

// TestSpreadLevelIsTheOnlyVerbatimCapableMarketField puts the two paths side by
// side on one wire value, so the difference cannot be mistaken for an oversight.
//
// 1e-330 is positive as a decimal and zero to every float64. Sent as
// spreadLevel it arrives as a 332-character positional price; sent as an order
// book level price it arrives as 0. Both are correct for their field type, and
// the pair is the clearest statement of why P1 typed spreadLevel json.Number and
// why the other fields could not be made to match.
func TestSpreadLevelIsTheOnlyVerbatimCapableMarketField(t *testing.T) {
	const underflow = "1e-330"

	book := newSequencedExecutor(t, sequencedReply{reply: orderBookLevelBody(underflow)})
	viaLevel, err := NewMarketService(book).OrderBook(t.Context(), OrderBookRequest{Security: marketSecurity()})
	if err != nil {
		t.Fatalf("OrderBook with an underflowing level price: %v", err)
	}
	if got := viaLevel.Ask[0].Price.String(); got != "0" {
		t.Errorf("Ask[0].Price = %q, want 0: a float64 DTO field cannot represent %s", got, underflow)
	}

	tick := newSequencedExecutor(t, sequencedReply{reply: spreadLevelBody(underflow)})
	viaTick, err := NewMarketService(tick).OrderBook(t.Context(), OrderBookRequest{Security: marketSecurity()})
	if err != nil {
		t.Fatalf("OrderBook with an underflowing spreadLevel: %v", err)
	}
	if got := viaTick.TickSize.String(); got != underflowPositional {
		t.Errorf("TickSize = %q, want the full positional form (%d characters)",
			got, len(underflowPositional))
	}
	if !strings.HasPrefix(viaTick.TickSize.String(), "0.0000") {
		t.Errorf("TickSize = %q, want the positional form starting 0.0000", viaTick.TickSize.String())
	}
}

// TestFloat64VolumeRejectsTheHostileQuantity states the other half of the same
// limit: the DTO volume is an int64, so the 27-digit quantity cannot be put on a
// market reply at all.
//
// A JSON number too large for int64 is a decode error, not a silent truncation,
// and this file records which of the two it is rather than leaving the behaviour
// to be discovered. The hostile quantity therefore has no market host; its host
// is the trade request path, where the wire field is a quoted string.
func TestFloat64VolumeRejectsTheHostileQuantity(t *testing.T) {
	if len(float64HostileQuantities) != 1 {
		t.Fatalf("float64HostileQuantities holds %d rows, want the single 27-digit value "+
			"this test reasons about", len(float64HostileQuantities))
	}
	hostile := float64HostileQuantities[0]

	body := `{"basicQot":[{"lastPrice":1.0,"volume":` + hostile.in + `,"turnover":1}]}`
	exec := newSequencedExecutor(t, sequencedReply{reply: json.RawMessage(body)})
	if _, err := NewMarketService(exec).BasicQot(t.Context(),
		BasicQotRequest{Security: []*Security{marketSecurity()}}); err == nil {
		t.Fatalf("BasicQot with a %s volume = nil error. An int64 DTO field cannot carry it, "+
			"and this test's stated behaviour is a decode failure", hostile.in)
	}

	// The largest int64 the field does carry still round-trips exactly, which is
	// what makes the boundary a property of the type rather than of this code.
	const maxInt64 = "9223372036854775807"
	ok := newSequencedExecutor(t, sequencedReply{
		reply: json.RawMessage(`{"basicQot":[{"lastPrice":1.0,"volume":` + maxInt64 + `,"turnover":1}]}`),
	})
	resp, err := NewMarketService(ok).BasicQot(t.Context(),
		BasicQotRequest{Security: []*Security{marketSecurity()}})
	if err != nil {
		t.Fatalf("BasicQot with a max-int64 volume: %v", err)
	}
	if got := resp.BasicQot[0].Volume.String(); got != maxInt64 {
		t.Errorf("Quote[0].Volume = %q, want %q", got, maxInt64)
	}
}

// TestFloatToStringNeverEmitsAnExponent pins the property that keeps the
// converters from taking the caller's goroutine down.
//
// 'f' with precision -1 never produces an exponent, so a 1e-7 price arrives as
// "0.0000001" and not "1e-07". The exponent form is not merely ugly here: the
// domain constructors are decimal, and a bare exponent string is the kind of
// input they refuse. The check is on the property rather than on a table of
// values, because the property holds for every input and the table would only
// prove it for the ones somebody thought of.
func TestFloatToStringNeverEmitsAnExponent(t *testing.T) {
	for _, f := range []float64{
		0, 1, -1, 0.0005, 1e-7, -2.5e-8, 1e21, 5e-324, 1e-323,
		1.7976931348623157e308, 1.0 / 3.0, 0.1 + 0.2, 380, -0,
	} {
		got := floatToString(f)
		if strings.ContainsAny(got, "eE") {
			t.Errorf("floatToString(%v) = %q, which carries an exponent; the domain "+
				"constructors are decimal and would reject it", f, got)
			continue
		}
		// The value must also parse back to the same float, which is the whole
		// reason the shortest form was chosen over a fixed scale.
		back, err := strconv.ParseFloat(got, 64)
		if err != nil {
			t.Errorf("floatToString(%v) = %q, which does not parse back: %v", f, got, err)
			continue
		}
		if back != f {
			t.Errorf("floatToString(%v) = %q, which parses back to %v", f, got, back)
		}
	}
}

// TestTrailingZeroDivergenceIsOnlyVisibleInTheString records the one row of the
// scale-hostile set a method-level assertion cannot catch, so the table is not
// read as stronger than it is.
//
// %.3f renders 0.1 as "0.100", which is a different string for the same number,
// and requireScaleHostile catches exactly that. But domain.MustNewPrice
// normalises it straight back to 0.1, so by the time a caller sees the value the
// two are indistinguishable. The row still earns its place — it is the only
// evidence that the removed conversion was destructive at the string level at
// all — but its teeth are in the guard and in market_test.go's floatToString
// table, not in the method tests above. Saying so here is cheaper than letting
// the next reader assume four rows fail under the mutation when three do.
func TestTrailingZeroDivergenceIsOnlyVisibleInTheString(t *testing.T) {
	const trailing = "0.1"
	if got := legacyRounded(mustFloat(t, trailing)); got != "0.100" {
		t.Fatalf("legacyRounded(%s) = %q, want 0.100; this test states a measured "+
			"divergence and the measurement no longer holds", trailing, got)
	}
	requireScaleHostile(t, trailing)

	// The divergence is gone by the time the value is a domain Price, which is
	// why the method-level row above passes even under the mutation.
	if got := domain.MustNewPrice("0.100", "0.001").String(); got != trailing {
		t.Fatalf("MustNewPrice(\"0.100\").String() = %q, want %q: the decimal normalises the "+
			"trailing zero, which is the whole point of this test", got, trailing)
	}
}

// mustFloat parses a value this file has already asserted is a JSON number.
func mustFloat(t *testing.T, in string) float64 {
	t.Helper()
	f, err := strconv.ParseFloat(in, 64)
	if err != nil {
		t.Fatalf("test bug: %q does not parse as a float: %v", in, err)
	}
	return f
}
