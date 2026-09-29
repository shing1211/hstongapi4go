// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package services

import (
	"encoding/json"
	"math/big"
	"strconv"
	"strings"
	"testing"

	"github.com/shing1211/hstongapi4go/client"
	"github.com/shing1211/hstongapi4go/pkg/domain"
	"github.com/shing1211/hstongapi4go/pkg/types"
)

// This file is the regression net for the defect the trade request path already
// shipped: fmt.Sprintf("%.3f", …) hardcoded a three-decimal scale the Gateway never
// promised, so a 0.0005 tick became 0.001 — a different and wrong price, with no
// error anywhere. P1 removed 23 such conversions across the SDK; these tests are
// the reason one cannot come back on this path.
//
// Two value sets from the frozen testsupport_test.go, because there are two
// different failure modes and a value that discriminates one is often blind to
// the other:
//
//   - float64HostilePrices / float64HostileQuantities carry more significant
//     digits than binary64 can hold, or underflow it to zero. A float64 wire
//     field cannot pass them; only a verbatim decimal field can.
//   - scaleHostilePrices are float64-stable, so a float64 bug would not show —
//     but a fixed-scale formatter still destroys them, and that is exactly the
//     bug that shipped. All four survive a float64 round-trip untouched, which is
//     why they need their own table.
//
// The trade request path is where the honest promise lives, unlike the HQ
// response path the market tests cover: every numeric in these requests is a
// quoted string built from a decimal, and nothing between the caller and the body
// can lose a digit unless a float64 field or a %.Nf conversion reappears.
//
// Two levels, both required. Level 1 type-asserts the recorded params and
// compares the string field to the literal with ==. Level 2 drives the real client
// over a wireRecorder and asserts the recorded request body contains the quoted
// literal — which is the assertion that also catches a changed field type, since
// the envelope's params member is a json.RawMessage and a float64 would render
// unquoted.

// ---------------------------------------------------------------------------
// Level 1: the recorded request struct
// ---------------------------------------------------------------------------

// TestMaxAvailableAssetCarriesHostilePricesVerbatim is the headline round trip,
// on the cleanest host in the package: MaxAvailableAsset has no validation gate
// at all, so price.String() goes straight into the request with nothing able to
// reject it on the way.
func TestMaxAvailableAssetCarriesHostilePricesVerbatim(t *testing.T) {
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
					exec := newSequencedExecutor(t, sequencedReply{reply: tradingMaxAvailableBody()})
					_, err := NewTradingService(exec).MaxAvailableAsset(t.Context(), tradingAccountID(),
						tradingHKSymbol(), domain.MustNewPrice(tc.in, "0"), types.EntrustTypeLimit)
					if err != nil {
						t.Fatalf("MaxAvailableAsset with price %s: %v", tc.in, err)
					}

					got := tradingParamsAs[maxAvailableAssetWireRequest](t, exec).EntrustPrice
					tradingAssertVerbatim(t, "EntrustPrice", got, tc)
				})
			}
		})
	}
}

// TestEntrustCarriesHostilePricesWithNoGrid is the same guarantee on the
// mutation path, where a validation gate does stand in the way.
//
// The resolver reports no opinion, so the step check is skipped and the price is
// checked for being non-negative and then left alone — the decision working as
// designed rather than a workaround: nothing about the value is softened on the
// way through.
//
// REWRITTEN by the TickSchedule work. This test used to get its pass by
// declaring a zero tick on the Price itself, which design-tick-model.md §2.1
// prescribed for read paths. That route is closed: the instrument's grid now
// governs the step, and a 25-significant-digit value is off every real HK grid,
// so declaring a zero tick no longer exempts an order. Reporting no opinion
// through the resolver is the honest way to say "this instrument's grid is
// unknown", and it keeps the test measuring what it was written to measure —
// that the digits reach the wire verbatim.
func TestEntrustCarriesHostilePricesWithNoGrid(t *testing.T) {
	// No opinion, non-empty tick on both sides of the ok flag: a resolver that has
	// genuinely not been told anything.
	noGrid := &fixedSchedule{ok: false}

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
					order := tradingHKOrder()
					order.Price = domain.MustNewPrice(tc.in, "0")

					exec := newSequencedExecutor(t, sequencedReply{reply: tradingEntrustBody("E-3001")})
					svc := NewTradingService(exec, WithTickSchedule(noGrid))
					if _, err := svc.Entrust(t.Context(), tradingAccountID(), order); err != nil {
						t.Fatalf("Entrust with a no-grid price of %s: %v", tc.in, err)
					}
					tradingExpectCall(t, exec, opEntrust, client.RouteTradeEntrust)

					got := tradingParamsAs[entrustWireRequest](t, exec).EntrustPrice
					tradingAssertVerbatim(t, "EntrustPrice", got, tc)
				})
			}
		})
	}
}

// TestEntrustCarriesHostilePricesOnAPriceOptionalOrder is the second host, and it
// exercises a different gate: the price is not looked at at all.
//
// isPriceOptional is true for nine of the eighteen order types, so an order
// carrying one of them skips validatePriceForHK entirely. The 0.0005 row here is
// therefore not merely accepted but never checked, which is worth stating because
// the loud-failure row further down is the opposite: the same 0.0005, inside a
// Price that declares the real tick, is refused.
func TestEntrustCarriesHostilePricesOnAPriceOptionalOrder(t *testing.T) {
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
					order := tradingHKOrder()
					order.OrderType = types.EntrustTypeStopProfitLimit // "31": price-optional and eligible
					order.Price = domain.MustNewPrice(tc.in, "0.001")
					order.ValidDays = 7
					order.CondValue = "380.000"

					exec := newSequencedExecutor(t, sequencedReply{reply: tradingEntrustBody("E-3002")})
					if _, err := NewTradingService(exec).Entrust(t.Context(), tradingAccountID(), order); err != nil {
						t.Fatalf("Entrust on a price-optional order with price %s: %v", tc.in, err)
					}
					got := tradingParamsAs[entrustWireRequest](t, exec).EntrustPrice
					tradingAssertVerbatim(t, "EntrustPrice", got, tc)
				})
			}
		})
	}
}

// TestEntrustCarriesTheHostileQuantityVerbatim is the quantity counterpart.
//
// The hostile value is a 27-digit integer that binary64 would render as a
// different number, and it is a legal order quantity besides: it is an integer and
// it is an exact multiple of the HK stock lot of 100. So the whole quantity
// validation chain passes it and the digits reach the request unchanged.
func TestEntrustCarriesTheHostileQuantityVerbatim(t *testing.T) {
	for _, tc := range float64HostileQuantities {
		t.Run(tc.name, func(t *testing.T) {
			order := tradingHKOrder()
			order.Quantity = domain.MustNewQuantity(tc.in)

			// The fixture is legal on both counts the helper checks, and saying so
			// here means a failure below is a round-trip failure and not a
			// validation one.
			if err := validateOrderForHK(order, domain.TickSchedule{}); err != nil {
				t.Fatalf("the hostile quantity %s is not a legal HK stock order: %v", tc.in, err)
			}

			exec := newSequencedExecutor(t, sequencedReply{reply: tradingEntrustBody("E-3003")})
			if _, err := NewTradingService(exec).Entrust(t.Context(), tradingAccountID(), order); err != nil {
				t.Fatalf("Entrust with quantity %s: %v", tc.in, err)
			}
			got := tradingParamsAs[entrustWireRequest](t, exec).EntrustAmount
			tradingAssertVerbatim(t, "EntrustAmount", got, tc)
		})
	}
}

// TestChangeEntrustCarriesHostileValuesVerbatim is the fourth host, and it is the
// narrowest: ChangeEntrust hardcodes DataTypeHKStock for both validators, so only
// a zero-tick price survives, and the 27-digit quantity needs its trailing "00" to
// stay a multiple of the lot of 100.
func TestChangeEntrustCarriesHostileValuesVerbatim(t *testing.T) {
	for _, tc := range float64HostilePrices {
		t.Run("price/"+tc.name, func(t *testing.T) {
			exec := newSequencedExecutor(t, sequencedReply{reply: json.RawMessage(`{"data":"1"}`)})
			err := NewTradingService(exec, WithTickSchedule(&fixedSchedule{ok: false})).ChangeEntrust(t.Context(), tradingAccountID(), tradingEntrustID(),
				domain.MustNewPrice(tc.in, "0"), domain.MustNewQuantity("100"))
			if err != nil {
				t.Fatalf("ChangeEntrust with a zero-tick price of %s: %v", tc.in, err)
			}
			got := tradingParamsAs[changeEntrustWireRequest](t, exec).EntrustPrice
			tradingAssertVerbatim(t, "EntrustPrice", got, tc)
		})
	}
	for _, tc := range scaleHostilePrices {
		t.Run("price/"+tc.name, func(t *testing.T) {
			exec := newSequencedExecutor(t, sequencedReply{reply: json.RawMessage(`{"data":"1"}`)})
			err := NewTradingService(exec, WithTickSchedule(&fixedSchedule{ok: false})).ChangeEntrust(t.Context(), tradingAccountID(), tradingEntrustID(),
				domain.MustNewPrice(tc.in, "0"), domain.MustNewQuantity("100"))
			if err != nil {
				t.Fatalf("ChangeEntrust with a zero-tick price of %s: %v", tc.in, err)
			}
			got := tradingParamsAs[changeEntrustWireRequest](t, exec).EntrustPrice
			tradingAssertVerbatim(t, "EntrustPrice", got, tc)
		})
	}
	for _, tc := range float64HostileQuantities {
		t.Run("quantity/"+tc.name, func(t *testing.T) {
			exec := newSequencedExecutor(t, sequencedReply{reply: json.RawMessage(`{"data":"1"}`)})
			err := NewTradingService(exec, WithTickSchedule(&fixedSchedule{ok: false})).ChangeEntrust(t.Context(), tradingAccountID(), tradingEntrustID(),
				domain.MustNewPrice("387.05", "0.001"), domain.MustNewQuantity(tc.in))
			if err != nil {
				t.Fatalf("ChangeEntrust with quantity %s: %v", tc.in, err)
			}
			got := tradingParamsAs[changeEntrustWireRequest](t, exec).EntrustAmount
			tradingAssertVerbatim(t, "EntrustAmount", got, tc)
		})
	}
}

// TestCancelEntrustCarriesTheIDVerbatim keeps the fourth wire field that holds an
// identifier rather than a decimal, on the same path, so a field that stops being
// stringified would be caught by the same suite.
func TestCancelEntrustCarriesTheIDVerbatim(t *testing.T) {
	const id = "E-99999999999999999999999999999999"
	exec := newSequencedExecutor(t, sequencedReply{reply: json.RawMessage(`{"data":"1"}`)})
	if err := NewTradingService(exec).CancelEntrust(t.Context(), tradingAccountID(), domain.EntrustID(id)); err != nil {
		t.Fatalf("CancelEntrust: %v", err)
	}
	if got := tradingParamsAs[cancelEntrustWireRequest](t, exec).EntrustID; got != id {
		t.Errorf("EntrustID = %q, want %q verbatim", got, id)
	}
}

// ---------------------------------------------------------------------------
// Level 2: the recorded HTTP body
// ---------------------------------------------------------------------------

// TestHostilePricesSurviveTheRealStack is the assertion that also catches a
// changed field type.
//
// The envelope's params member is a json.RawMessage (internal/transport), so a
// float64 field would render unquoted and the substring below would not match at
// all. A string field holding a fixed-scale formatted number would render quoted
// but with different digits. Both failures are caught here, and neither is
// catchable from the recorded params alone.
func TestHostilePricesSurviveTheRealStack(t *testing.T) {
	all := make([]moneyCase, 0, len(float64HostilePrices)+len(scaleHostilePrices))
	all = append(all, float64HostilePrices...)
	all = append(all, scaleHostilePrices...)

	for _, tc := range all {
		t.Run("MaxAvailableAsset/"+tc.name, func(t *testing.T) {
			rec := newWireRecorder(map[string]string{
				string(client.RouteTradeQueryMaxAvailableAsset): gatewaySuccess(string(tradingMaxAvailableBody())),
			})
			svc := NewTradingService(newWireExecutor(t, rec), WithTickSchedule(&fixedSchedule{ok: false}))

			_, err := svc.MaxAvailableAsset(t.Context(), tradingAccountID(), tradingHKSymbol(),
				domain.MustNewPrice(tc.in, "0"), types.EntrustTypeLimit)
			if err != nil {
				t.Fatalf("MaxAvailableAsset over the wire with %s: %v", tc.in, err)
			}

			body := rec.lastBody(t, string(client.RouteTradeQueryMaxAvailableAsset))
			tradingAssertQuotedOnWire(t, body, "entrustPrice", tc)
			if got := rec.count(string(client.RouteTradeQueryMaxAvailableAsset)); got != 1 {
				t.Errorf("requests = %d, want exactly 1", got)
			}
		})

		t.Run("Entrust/"+tc.name, func(t *testing.T) {
			order := tradingHKOrder()
			order.Price = domain.MustNewPrice(tc.in, "0")

			rec := newWireRecorder(map[string]string{
				string(client.RouteTradeEntrust): gatewaySuccess(`{"entrustId":"E-4001"}`),
			})
			svc := NewTradingService(newWireExecutor(t, rec), WithTickSchedule(&fixedSchedule{ok: false}))

			if _, err := svc.Entrust(t.Context(), tradingAccountID(), order); err != nil {
				t.Fatalf("Entrust over the wire with %s: %v", tc.in, err)
			}
			body := rec.lastBody(t, string(client.RouteTradeEntrust))
			tradingAssertQuotedOnWire(t, body, "entrustPrice", tc)
			// The other numeric on the same request must survive too, so a change
			// that hardened the price field and not the quantity is still caught.
			if !strings.Contains(body, `"entrustAmount":"100"`) {
				t.Errorf("body does not carry the quantity verbatim:\n%s", body)
			}
		})
	}
}

// TestHostileQuantitySurvivesTheRealStack is the quantity row on the wire, where
// a 27-digit integer would become 1000000000000000000000000000 in a float64 and
// would render unquoted in the first place.
func TestHostileQuantitySurvivesTheRealStack(t *testing.T) {
	for _, tc := range float64HostileQuantities {
		t.Run(tc.name, func(t *testing.T) {
			order := tradingHKOrder()
			order.Quantity = domain.MustNewQuantity(tc.in)

			rec := newWireRecorder(map[string]string{
				string(client.RouteTradeEntrust): gatewaySuccess(`{"entrustId":"E-4002"}`),
			})
			if _, err := NewTradingService(newWireExecutor(t, rec)).Entrust(
				t.Context(), tradingAccountID(), order); err != nil {
				t.Fatalf("Entrust over the wire: %v", err)
			}

			body := rec.lastBody(t, string(client.RouteTradeEntrust))
			tradingAssertQuotedOnWire(t, body, "entrustAmount", tc)
		})
	}
}

// TestNoRequestFieldEverCarriesAnExponent states the property that keeps a hostile
// value from becoming a broken one.
//
// The values reach the wire through decimal.String(), which never emits an
// exponent, so 1e-330 arrives as the 332-character positional form rather than as
// "1e-330". A float64 formatted with %v would have produced the exponent form, and
// the Gateway's decimal parser is not obliged to accept it. The check is on the
// property rather than on a table, because the property holds for every input and
// a table would only prove it for the values somebody thought of.
func TestNoRequestFieldEverCarriesAnExponent(t *testing.T) {
	values := []string{
		"0", "1", "-1", "0.0005", "1e-330", "1e21", "5e-324", "0.1",
		"123.4567890123456789", "999999999999999999999999900", "1e-7",
	}
	for _, in := range values {
		t.Run(in, func(t *testing.T) {
			price := domain.MustNewPrice(in, "0")

			exec := newSequencedExecutor(t, sequencedReply{reply: tradingMaxAvailableBody()})
			if _, err := NewTradingService(exec).MaxAvailableAsset(t.Context(), tradingAccountID(),
				tradingHKSymbol(), price, types.EntrustTypeLimit); err != nil {
				t.Fatalf("MaxAvailableAsset: %v", err)
			}
			got := tradingParamsAs[maxAvailableAssetWireRequest](t, exec).EntrustPrice
			if strings.ContainsAny(got, "eE") {
				t.Errorf("EntrustPrice = %q, which carries an exponent: the Gateway's decimal "+
					"parser is not obliged to accept one", got)
			}
			// And it still denotes the same number, exactly. big.Rat is used rather
			// than the decimal library so the check does not go through the same
			// code the value was rendered with.
			tradingAssertSameNumber(t, "EntrustPrice", in, got)
		})
	}
}

// TestUnderflowingPriceIsPositiveNotZero is the brief's "zero to every float64 yet
// positive as a decimal" case, stated as its own test because it is the row a
// reader is most likely to assume is a typo.
//
// 1e-330 is a real, positive, sub-binary64 price. A float64 anywhere in the path
// turns it into 0, which is a different and much larger trade. The positional
// expansion is what actually goes on the wire, and the test asserts both that the
// digits are the ones the frozen table names and that the value is not zero.
func TestUnderflowingPriceIsPositiveNotZero(t *testing.T) {
	if len(float64HostilePrices) != 3 {
		t.Fatalf("float64HostilePrices holds %d rows, want 3", len(float64HostilePrices))
	}
	underflow := float64HostilePrices[2]
	if underflow.in != "1e-330" {
		t.Fatalf("the underflowing row is %q, want 1e-330: this test states a measured "+
			"divergence and the measurement no longer holds", underflow.in)
	}

	// A float64 would render this as exactly 0.
	if got := strconv.FormatFloat(0, 'f', -1, 64); got != "0" {
		t.Fatalf("test bug: the zero this test contrasts against formatted as %q", got)
	}

	rec := newWireRecorder(map[string]string{
		string(client.RouteTradeQueryMaxAvailableAsset): gatewaySuccess(string(tradingMaxAvailableBody())),
	})
	svc := NewTradingService(newWireExecutor(t, rec))
	if _, err := svc.MaxAvailableAsset(t.Context(), tradingAccountID(), tradingHKSymbol(),
		domain.MustNewPrice(underflow.in, "0"), types.EntrustTypeLimit); err != nil {
		t.Fatalf("MaxAvailableAsset: %v", err)
	}

	body := rec.lastBody(t, string(client.RouteTradeQueryMaxAvailableAsset))
	tradingAssertQuotedOnWire(t, body, "entrustPrice", underflow)

	if !strings.HasPrefix(underflow.want, "0.0000") || strings.Count(underflow.want, "0") != 330 {
		t.Fatalf("the expected expansion %d characters does not look like 0. followed by 329 "+
			"zeros and a 1; the frozen table is not what this test describes", len(underflow.want))
	}
}

// ---------------------------------------------------------------------------
// 5.3.4: the loud failure
// ---------------------------------------------------------------------------

// TestFaithfulSubTickPriceFailsLoudlyInsteadOfBeingRounded is as important as the
// round-trip rows, and it asserts the opposite outcome.
//
// design-tick-model.md §1 says the honest state of a faithful 0.0005 inside a
// Price whose tick is 0.001 is a loud rejection. That is a positive claim about
// behaviour, and a positive claim needs a test, or the next person to see a
// validation failure on a legitimate-looking price will "fix" it by rounding —
// which is how the original bug came to exist.
//
// Both halves are asserted. The typed rejection, so a caller can tell the SDK
// refused from the exchange refused. And zero recorded calls, so the half that
// matters most is proven: the value was never rounded into 0.001 and sent.
func TestFaithfulSubTickPriceFailsLoudlyInsteadOfBeingRounded(t *testing.T) {
	t.Run("Entrust", func(t *testing.T) {
		order := tradingHKOrder()
		order.Price = domain.MustNewPrice("0.0005", "0.001")

		exec := newSequencedExecutor(t, sequencedReply{reply: tradingEntrustBody("E-5001")})
		res, err := NewTradingService(exec).Entrust(t.Context(), tradingAccountID(), order)
		assertInvalidParam(t, err, opEntrust)
		requireZero(t, res)
		requireCalls(t, exec, 0)
	})

	t.Run("ChangeEntrust", func(t *testing.T) {
		exec := newSequencedExecutor(t, sequencedReply{reply: json.RawMessage(`{"data":"1"}`)})
		err := NewTradingService(exec).ChangeEntrust(t.Context(), tradingAccountID(), tradingEntrustID(),
			domain.MustNewPrice("0.0005", "0.001"), domain.MustNewQuantity("100"))
		assertInvalidParam(t, err, opEntrust)
		requireCalls(t, exec, 0)
	})

	// The paired accept: the same 0.0005 in a Price that declares a zero tick, and
	// the same 0.0005 on an order type whose price is optional. Both go on the wire
	// as 0.0005, neither as 0.001. Without the pair, the rejection above could be
	// a helper that refuses everything.
	t.Run("the same value is accepted where the price is not checked", func(t *testing.T) {
		for _, tc := range []struct {
			name   string
			mutate func(o *domain.Order)
		}{
			{"no grid known", func(o *domain.Order) { o.Price = domain.MustNewPrice("0.0005", "0") }},
			{"price-optional type", func(o *domain.Order) {
				o.OrderType = types.EntrustTypeStopLossLimit // "33"
				o.Price = domain.MustNewPrice("0.0005", "0.001")
				o.ValidDays = 7
				o.CondValue = "380.000"
			}},
		} {
			t.Run(tc.name, func(t *testing.T) {
				order := tradingHKOrder()
				tc.mutate(&order)

				rec := newWireRecorder(map[string]string{
					string(client.RouteTradeEntrust): gatewaySuccess(`{"entrustId":"E-5002"}`),
				})
				// The "no grid known" row relies on the step check being skipped, which
				// used to be reached by declaring a zero tick on the Price. That route
				// is closed: the instrument's grid now governs. Injecting a resolver
				// with no opinion is the honest way to say the grid is unknown, and it
				// keeps this row measuring the thing it exists to measure — that 0.0005
				// reaches the wire unrounded, which the removed %%.3f conversion broke.
				svc := NewTradingService(newWireExecutor(t, rec), WithTickSchedule(&fixedSchedule{ok: false}))
				if _, err := svc.Entrust(t.Context(), tradingAccountID(), order); err != nil {
					t.Fatalf("Entrust: %v", err)
				}
				body := rec.lastBody(t, string(client.RouteTradeEntrust))
				if !strings.Contains(body, `"entrustPrice":"0.0005"`) {
					t.Errorf("body does not carry 0.0005 verbatim; the shipped bug would have "+
						"put 0.001 here:\n%s", body)
				}
				if strings.Contains(body, `"entrustPrice":"0.001"`) {
					t.Errorf("body carries 0.001, which is the value the removed %%.3f "+
						"conversion produced:\n%s", body)
				}
			})
		}
	})
}

// TestAnOffTickOrderNeverReachesTheWire is the same claim one layer down, driven
// over the real client so "reached the wire" means exactly that: no HTTP request
// was made at all.
func TestAnOffTickOrderNeverReachesTheWire(t *testing.T) {
	order := tradingHKOrder()
	order.Price = domain.MustNewPrice("123.4565", "0.001")

	rec := newWireRecorder(map[string]string{
		string(client.RouteTradeEntrust): gatewaySuccess(`{"entrustId":"E-5003"}`),
	})
	res, err := NewTradingService(newWireExecutor(t, rec)).Entrust(t.Context(), tradingAccountID(), order)
	assertInvalidParam(t, err, opEntrust)
	requireZero(t, res)
	if got := rec.total(); got != 0 {
		t.Fatalf("requests = %d, want 0: a rejected order must not be sent", got)
	}
}

// ---------------------------------------------------------------------------
// Assertion helpers
// ---------------------------------------------------------------------------

// tradingAssertVerbatim is the Level 1 assertion: the recorded field is the exact
// string, byte for byte, and the failure message says what the removed conversion
// would have produced so a regression names its own cause.
func tradingAssertVerbatim(t *testing.T, field, got string, tc moneyCase) {
	t.Helper()
	if got == tc.want {
		return
	}
	t.Errorf("%s = %q, want %q (the literal that went in).\n"+
		"  input literal:            %s\n"+
		"  a float64 round trip:     %s\n"+
		"  the removed %%.3f form:   %s",
		field, got, tc.want, tc.in, tradingFloatCeiling(tc.in), tradingLegacy(t, tc.in))
}

// tradingAssertQuotedOnWire is the Level 2 assertion: the recorded request body
// carries the value as a quoted JSON string, which is what a decimal field
// produces and what a float64 field cannot.
func tradingAssertQuotedOnWire(t *testing.T, body, field string, tc moneyCase) {
	t.Helper()
	quoted := `"` + field + `":"` + tc.want + `"`
	if strings.Contains(body, quoted) {
		return
	}
	unquoted := `"` + field + `":` + tc.want
	t.Errorf("recorded request body does not contain %s.\n"+
		"  body:                %s\n"+
		"  wanted substring:    %s\n"+
		"  the value unquoted:  %s\n"+
		"  would also match a %%.3f conversion of the input: %s",
		quoted, body, quoted, unquoted, tradingLegacy(t, tc.in))
	if strings.Contains(body, unquoted) {
		t.Error("  the unquoted form is present, so the field is a JSON number on the wire: " +
			"a float64 in the request path would render this way")
	}
}

// tradingAssertSameNumber checks exact numeric equality through math/big, which is
// independent of the decimal library that rendered the value and of float64.
func tradingAssertSameNumber(t *testing.T, field, in, got string) {
	t.Helper()
	want, okIn := new(big.Rat).SetString(in)
	if !okIn {
		t.Fatalf("test bug: %q is not a number big.Rat can read", in)
	}
	have, okGot := new(big.Rat).SetString(got)
	if !okGot {
		t.Fatalf("%s = %q, which is not a number the wire could carry", field, got)
	}
	if want.Cmp(have) != 0 {
		t.Errorf("%s = %q denotes %s, want the exact value of %q (%s)", field, got, have.RatString(), in, want.RatString())
	}
	if have.Sign() == 0 && want.Sign() != 0 {
		t.Errorf("%s = 0 for a non-zero input %q: a float64 in the path would do this", field, in)
	}
}

// tradingFloatCeiling renders what binary64 could have produced, for the failure
// message. A value that cannot be parsed is reported as such rather than panicking
// inside a failure path.
func tradingFloatCeiling(in string) string {
	f, err := strconv.ParseFloat(in, 64)
	if err != nil {
		return "(not representable as a float64: " + err.Error() + ")"
	}
	return strconv.FormatFloat(f, 'f', -1, 64)
}

// tradingLegacy is the removed fmt.Sprintf("%.3f", …) conversion, spelled out so a
// failure message can show what the old code did to the value.
func tradingLegacy(t *testing.T, in string) string {
	t.Helper()
	f, err := strconv.ParseFloat(in, 64)
	if err != nil {
		return "(not representable as a float64: " + err.Error() + ")"
	}
	return legacyRounded(f)
}
