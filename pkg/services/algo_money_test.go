// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package services

import (
	"context"
	"encoding/json"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/shing1211/hstongapi4go/client"
	"github.com/shing1211/hstongapi4go/pkg/domain"
)

// This file is the money regression net for the algo service, on the **request**
// side — and the request side is the point, because an algo order's quantity is a
// write. A price that loses a digit on the way to the Gateway is not a wrong
// display; it is a wrong order.
//
// It is ported from the futures mutation money table (C5/C6) rather than written
// fresh, for the reason that table exists: the value sets are frozen, shared and
// guarded, and re-deriving them here would create a second table that eventually
// guards nothing. The sets and their two guards — float64HostilePrices,
// float64HostileQuantities, scaleHostilePrices, requireFloat64Hostile and
// requireScaleHostile — are declared once in testsupport_test.go and used from
// there; the guards are re-asserted below so an edit to another file's copy cannot
// silently disarm this one.
//
// Two levels, and the split is the point:
//
//   - **The typed path** (TestAlgoMutationMoneyIsReachedThroughTheTypedPath) reads
//     the request struct the method handed the executor. This pins that the mapping
//     boundary did not touch the digits.
//   - **The wire** (TestAlgoMutationMoneyCrossesVerbatim) reads the recorded HTTP
//     request body. This pins that the transport did not touch them either — a
//     float64 wire *type* fails the second, a %.3f in the mapper fails the first,
//     and only running both says which layer a regression landed in.
//
// C5's lesson is carried as a structure: the hosts are a table with one row per
// (method, field) pair, and the count is asserted, so a body that gained a money
// field without a row here cannot pass. A rule asserted on one body is half a rule,
// and the four order mutations plus the operate body are five different bodies.

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

// algoMutationMoneyHost is one money-bearing field on one mutation body.
//
// invoke takes the literal and performs the whole call, so the two levels of the
// test cannot drift apart: both drive the same closure with the same value, one
// reading the request struct back and one reading the recorded bytes. A design
// where the value were recorded in one call and read in another would let a row
// assert a field the method does not actually send.
type algoMutationMoneyHost struct {
	name string
	// method is the mutation whose request carries the field. It must name a real
	// row of algoMutationCases, which the test asserts.
	method string
	// route is the endpoint the field must reach.
	route client.Route
	// invoke sets the field to literal and performs the call.
	invoke func(t *testing.T, ctx context.Context, svc *AlgoService, literal string) error
	// field is the JSON key the value must appear under on the wire.
	field string
	// nested is true for a field inside the strategyParam object, which is a
	// different shape from the envelope's params and is read separately.
	nested bool
	// quantity is true for a field the 27-digit integer set is aimed at.
	quantity bool
	// inRange is true for a field with a documented bounded domain, which the
	// hostile price rows cannot all be fed to. It selects the quantity set and says
	// why in the comment on the row.
	inRange bool
}

// algoMutationMoneyHosts is the table: every money-bearing field on every algo
// mutation body.
//
// The count is asserted by the test below, so a body that gained a money field
// without a row here shows up as a failure rather than as a silently untested
// field. Eight rows, across two bodies: the add carries four economic terms plus
// three nested, the change carries two, and the two cancels and the operate body
// carry none.
func algoMutationMoneyHosts() []algoMutationMoneyHost {
	return []algoMutationMoneyHost{
		{
			name: "AddOrder/entrustPrice", method: "AddOrder",
			route: client.RouteTradeAlgoAddOrder,
			invoke: func(t *testing.T, ctx context.Context, svc *AlgoService, literal string) error {
				t.Helper()
				r := algoFixtureAdd()
				r.Price = domain.MustNewPrice(literal, "0")
				_, err := svc.AddOrder(ctx, algoFixtureAccount(), r)
				return err
			},
			field: "entrustPrice",
		},
		{
			name: "AddOrder/entrustAmount", method: "AddOrder",
			route: client.RouteTradeAlgoAddOrder, quantity: true,
			invoke: func(t *testing.T, ctx context.Context, svc *AlgoService, literal string) error {
				t.Helper()
				r := algoFixtureAdd()
				r.Quantity = domain.MustNewQuantity(literal)
				_, err := svc.AddOrder(ctx, algoFixtureAccount(), r)
				return err
			},
			field: "entrustAmount",
		},
		{
			name: "AddOrder/strategyParam.maxVolume", method: "AddOrder",
			route: client.RouteTradeAlgoAddOrder, quantity: true, nested: true,
			invoke: func(t *testing.T, ctx context.Context, svc *AlgoService, literal string) error {
				t.Helper()
				r := algoFixtureAdd()
				r.StrategyParam.MaxVolume = domain.MustNewQuantity(literal)
				_, err := svc.AddOrder(ctx, algoFixtureAccount(), r)
				return err
			},
			field: "maxVolume",
		},
		{
			name: "AddOrder/strategyParam.minAmount", method: "AddOrder",
			route: client.RouteTradeAlgoAddOrder, nested: true,
			invoke: func(t *testing.T, ctx context.Context, svc *AlgoService, literal string) error {
				t.Helper()
				r := algoFixtureAdd()
				// The currency is never transmitted — the wire field is a bare
				// decimal — so it is local metadata only, and the assertion is on
				// the digits.
				r.StrategyParam.MinAmount = domain.MustNewMoney(literal, "HKD", 3)
				_, err := svc.AddOrder(ctx, algoFixtureAccount(), r)
				return err
			},
			field: "minAmount",
		},
		{
			name: "AddOrder/strategyParam.showQty", method: "AddOrder",
			route: client.RouteTradeAlgoAddOrder, quantity: true, nested: true,
			invoke: func(t *testing.T, ctx context.Context, svc *AlgoService, literal string) error {
				t.Helper()
				r := algoFixtureAdd()
				r.StrategyParam.ShowQty = domain.MustNewQuantity(literal)
				_, err := svc.AddOrder(ctx, algoFixtureAccount(), r)
				return err
			},
			field: "showQty",
		},
		{
			// The participation percentage is the one money-ish field here with a
			// *documented bounded domain*: 可设置1-99, stated identically by the
			// reference and both vendors. The 19- and 25-digit price rows are above
			// 99 and the fractional ones are not percentages, so this row is driven
			// with the quantity set — a 27-digit integer is a whole number with more
			// digits than a float64 holds, which is the property this row is for. The
			// range itself is the subject of the validation tables, not this one, and
			// narrowing the set is stated rather than quietly done.
			name: "AddOrder/strategyParam.qtyPercent", method: "AddOrder",
			route: client.RouteTradeAlgoAddOrder, nested: true,
			quantity: true, inRange: true,
			invoke: func(t *testing.T, ctx context.Context, svc *AlgoService, literal string) error {
				t.Helper()
				r := algoFixtureAdd()
				r.StrategyParam.QtyPercent = domain.MustNewRate(literal)
				// The field's own validator is expected to *refuse* a value outside
				// 1-99, so the row drives the typed path and asserts what the method
				// did rather than assuming a request went out. The wire rows for
				// this field are therefore not asserted here; the range tests in the
				// mutations file own them.
				_, err := svc.AddOrder(ctx, algoFixtureAccount(), r)
				return err
			},
			field: "qtyPercent",
		},
		{
			name: "ChangeOrder/entrustPrice", method: "ChangeOrder",
			route: client.RouteTradeAlgoChangeOrder,
			invoke: func(t *testing.T, ctx context.Context, svc *AlgoService, literal string) error {
				t.Helper()
				r := algoFixtureChange()
				r.Price = domain.MustNewPrice(literal, "0")
				_, err := svc.ChangeOrder(ctx, algoFixtureAccount(), r)
				return err
			},
			field: "entrustPrice",
		},
		{
			name: "ChangeOrder/entrustAmount", method: "ChangeOrder",
			route: client.RouteTradeAlgoChangeOrder, quantity: true,
			invoke: func(t *testing.T, ctx context.Context, svc *AlgoService, literal string) error {
				t.Helper()
				r := algoFixtureChange()
				r.Quantity = domain.MustNewQuantity(literal)
				_, err := svc.ChangeOrder(ctx, algoFixtureAccount(), r)
				return err
			},
			field: "entrustAmount",
		},
	}
}

// TestAlgoMoneyValueSetsAreStillHostile is the guard on the guard. Each row asserts
// the property that makes it worth having, so a future edit that softened a literal
// into a benign one deletes the regression net below without any test noticing.
//
// The same rows are asserted in pkg/domain/algo_test.go for the reply side and in
// testsupport_test.go for the cash side; it is repeated here so this file's hosts
// are not silently disarmed by an edit to either of those.
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

// TestAlgoMoneySetsAreTheAccountSets pins algo's value sets to the ones
// accountMoneyTest already uses, rather than letting this file re-declare a
// parallel copy that could quietly drift.
//
// The two were written independently, so this is a real check rather than a
// restatement: a future edit that strengthens an account row -- adding a
// subnormal, a 27-digit integer, a value a fixed-scale formatter would re-plot
// -- must be picked up here too, because an algo host driven by a stale set
// would pass while testing nothing. A divergence is reported with both labels so
// the failing row is identifiable without a bisect.
func TestAlgoMoneySetsAreTheAccountSets(t *testing.T) {
	account := accountMoneySets()
	algo := algoMoneySets()
	// The two set types are structurally identical but nominally distinct, so the
	// labels are collected by two walks rather than through a shared helper.
	algoLabels, accountLabels := make([]string, 0, len(algo)), make([]string, 0, len(account))
	for _, s := range algo {
		algoLabels = append(algoLabels, s.label)
	}
	for _, s := range account {
		accountLabels = append(accountLabels, s.label)
	}
	if len(account) != len(algo) {
		t.Fatalf("algo drives %d value sets, account drives %d: %v vs %v",
			len(algo), len(account), algoLabels, accountLabels)
	}
	for i := range account {
		if account[i].label != algo[i].label {
			t.Errorf("set %d: algo calls it %q, account calls it %q; the order and the "+
				"labels must match so a host cannot be reported against the wrong set",
				i, algo[i].label, account[i].label)
			continue
		}
		if !reflect.DeepEqual(algo[i].cases, account[i].cases) {
			t.Errorf("set %q: algo and account disagree on the cases:\n algo:   %v\n account: %v",
				account[i].label, algo[i].cases, account[i].cases)
		}
	}
}

// algoAssertVerbatim is the central money assertion: the domain value reports the
// exact digits the caller supplied, and the failure message names the two
// conversions that would have destroyed them.
func algoAssertVerbatim(t *testing.T, field, got string, tc moneyCase) {
	t.Helper()
	if got == tc.want {
		return
	}
	t.Errorf("%s = %q, want %q (the literal the caller supplied).\n"+
		"  input literal:          %s\n"+
		"  a float64 round trip:   %s\n"+
		"  the removed %%.3f form: %s",
		field, got, tc.want, tc.in,
		strconv.FormatFloat(algoMustFloat(t, tc.in), 'f', -1, 64),
		legacyRounded(algoMustFloat(t, tc.in)))
}

// algoAssertQuotedInRequest is the mutation-side wire assertion: the recorded
// *request* envelope carries the value as a quoted JSON string and does not also
// carry it unquoted.
//
// It is the assertion that catches a wire field retyped from string to float64, and
// it is the one that fails if a conversion reappears anywhere between the caller's
// domain.Price and the envelope — a float64 round trip, or the removed
// fmt.Sprintf("%.3f", …) that turned a 0.0005 tick into 0.001 with no error
// anywhere. On a *mutation* that is a lossy write rather than a wrong display,
// which is why it is asserted here and not only on the read side.
func algoAssertQuotedInRequest(t *testing.T, body, field string, tc moneyCase) {
	t.Helper()
	quoted := `"` + field + `":"` + tc.want + `"`
	if !strings.Contains(body, quoted) {
		t.Errorf("the recorded request does not contain %s.\n"+
			"  body:              %s\n"+
			"  wanted substring:  %s\n"+
			"  a float64 round trip would render %s\n"+
			"  the removed %%.3f form would render %s",
			quoted, body, quoted,
			strconv.FormatFloat(algoMustFloat(t, tc.in), 'f', -1, 64),
			legacyRounded(algoMustFloat(t, tc.in)))
		return
	}
	if unquoted := `"` + field + `":` + tc.want; strings.Contains(body, unquoted) {
		t.Errorf("the unquoted form %s is also present, so the field is a JSON number on "+
			"the wire: a float64 in the request path would render this way, and a "+
			"mutation's price would be a lossy write rather than a quoted decimal",
			unquoted)
	}
}

// algoAssertNotRenderedAs is the negative half: the two renderings the two
// historical defects produce must both be absent from the request body.
//
// It is separated from the positive assertion because the positive one could pass
// by accident if a future set were short enough that the two renderings coincided,
// and these two sets are chosen so that they never do.
func algoAssertNotRenderedAs(t *testing.T, body, field string, tc moneyCase) {
	t.Helper()
	f := algoMustFloat(t, tc.in)
	for label, rendered := range map[string]string{
		"a float64 round trip":  strconv.FormatFloat(f, 'f', -1, 64),
		"the removed %.3f form": legacyRounded(f),
	} {
		if rendered == tc.want {
			continue
		}
		if strings.Contains(body, `"`+field+`":"`+rendered+`"`) ||
			strings.Contains(body, `"`+field+`":`+rendered) {
			t.Errorf("the request carries %s = %s, which is %s of the input %q rather than "+
				"the exact decimal; a mutation writes this field, so a lost digit is a "+
				"wrong order", field, rendered, label, tc.in)
		}
	}
}

// algoMustFloat parses in for a failure message and fails the test if it cannot, so
// a value a float could not represent reports that rather than panicking inside a
// failure path.
func algoMustFloat(t *testing.T, in string) float64 {
	t.Helper()
	f, err := strconv.ParseFloat(in, 64)
	if err != nil {
		t.Fatalf("test bug: %q does not parse as a float: %v", in, err)
	}
	return f
}

// TestAlgoMutationMoneyCrossesVerbatim drives every mutation money host over every
// value set it is aimed at and asserts the exact decimal reaches the wire.
//
// The value sets are the frozen ones from testsupport_test.go: three
// float64-hostile prices (19 significant digits, 25 significant digits, and
// 1e-330, which is positive as a decimal and zero to every float64), four
// scale-hostile prices (including 0.0005, the literal the shipped bug turned into
// 0.001), and one float64-hostile quantity (a 27-digit integer binary64 cannot
// represent). The guard on the guards — TestAlgoMoneyValueSetsAreStillHostile —
// asserts each is still hostile, so a future edit that softened a literal cannot
// quietly delete this net.
func TestAlgoMutationMoneyCrossesVerbatim(t *testing.T) {
	hosts := algoMutationMoneyHosts()
	if len(hosts) != 8 {
		t.Fatalf("the money host table has %d hosts, want the eight money-bearing fields "+
			"the two order-bearing algo bodies carry", len(hosts))
	}
	for _, host := range hosts {
		// The qtyPercent row has a documented bounded domain, so it is driven only
		// on the typed path: its validator is expected to refuse a 27-digit integer,
		// and asserting a request went out would be asserting the opposite of what
		// the code does. The range itself is covered by the validation tables.
		if host.inRange {
			continue
		}
		t.Run(host.name, func(t *testing.T) {
			if host.method == "" || host.route == "" || host.field == "" {
				t.Fatal("test bug: the host has no method, route or field, so the row proves nothing")
			}
			// Every host must be a real mutation from the case table, so a row
			// naming a method that does not exist fails here rather than silently
			// never being driven.
			_ = algoMutationCaseNamed(t, host.method)
			for _, set := range algoMoneyCasesFor(host.quantity) {
				t.Run(set.label, func(t *testing.T) {
					for _, tc := range set.cases {
						t.Run(tc.name, func(t *testing.T) {
							// The canonical form the wire must carry is the
							// decimal's own String(), which for these inputs is
							// tc.want. Assert that first, so a test bug in the set is
							// visible before the method runs.
							algoAssertCanonical(t, host, tc)

							rec := newWireRecorder(map[string]string{
								string(host.route): gatewaySuccess(algoMutationBody),
							})
							if err := host.invoke(t, t.Context(),
								NewAlgoService(newWireExecutor(t, rec)), tc.in); err != nil {
								t.Fatalf("%s carrying %q = %v, want nil", host.name, tc.in, err)
							}
							if got := rec.count(string(host.route)); got != 1 {
								t.Fatalf("requests to %s = %d, want exactly 1", host.route, got)
							}

							body := rec.lastBody(t, string(host.route))
							algoAssertQuotedInRequest(t, body, host.field, tc)
							algoAssertNotRenderedAs(t, body, host.field, tc)
						})
					}
				})
			}
		})
	}
}

// algoAssertCanonical asserts that the expected digits are what the domain
// constructor renders, before the method is asked for them. It separates a test bug
// in a value set from a bug in the code, which is the difference between a failure
// that names the fixture and one that names the mapper.
func algoAssertCanonical(t *testing.T, host algoMutationMoneyHost, tc moneyCase) {
	t.Helper()
	var got string
	switch {
	case host.quantity:
		got = domain.MustNewQuantity(tc.want).String()
	case host.nested && host.field == "minAmount":
		got = domain.MustNewMoney(tc.want, "HKD", 3).String()
	case host.nested && host.field == "qtyPercent":
		got = domain.MustNewRate(tc.want).String()
	default:
		got = domain.MustNewPrice(tc.want, "0").String()
	}
	if got != tc.want {
		t.Fatalf("test bug: the constructor renders %q as %q, so the expected wire literal "+
			"is not tc.want and this row would fail for a reason that has nothing to do "+
			"with the mapping boundary", tc.want, got)
	}
}

// TestAlgoMutationMoneyIsReachedThroughTheTypedPath is the Level 1 counterpart: the
// same literals, asserted on the request struct the method handed the executor
// rather than on the marshalled bytes.
//
// The two levels are not redundant. The struct assertion pins that the mapping
// boundary did not touch the digits, and the wire assertion pins that the transport
// did not touch them either — a float64 wire *type* would fail the second and a
// %.3f in the mapper would fail the first, and only running both says which layer a
// regression landed in.
func TestAlgoMutationMoneyIsReachedThroughTheTypedPath(t *testing.T) {
	for _, host := range algoMutationMoneyHosts() {
		t.Run(host.name, func(t *testing.T) {
			_ = algoMutationCaseNamed(t, host.method)
			for _, set := range algoMoneyCasesFor(host.quantity) {
				t.Run(set.label, func(t *testing.T) {
					for _, tc := range set.cases {
						t.Run(tc.name, func(t *testing.T) {
							exec := newSequencedExecutor(t, sequencedReply{
								reply: json.RawMessage(algoMutationBody),
							})
							err := host.invoke(t, t.Context(), NewAlgoService(exec), tc.in)
							if host.inRange {
								// The field's own range check is expected to
								// refuse this value, so the assertion is on the
								// refusal and on the fact that nothing was sent.
								// A request here would mean the range check was
								// removed, which is a real defect the row catches.
								if err == nil {
									t.Fatalf("%s carrying %q = nil error, want the range "+
										"check to refuse it: the documented domain is 1-99 "+
										"and a %d-digit integer is outside it",
										host.name, tc.in, len(tc.in))
								}
								requireCalls(t, exec, 0)
								return
							}
							if err != nil {
								t.Fatalf("%s carrying %q = %v, want nil", host.name, tc.in, err)
							}
							requireCalls(t, exec, 1)

							algoAssertVerbatim(t, host.name,
								algoRecordedMoneyField(t, exec.lastParams(t), host), tc)
						})
					}
				})
			}
		})
	}
}

// algoRecordedMoneyField reads one money field out of whichever of the two
// order-bearing request structs the executor recorded. An unrecognised struct is a
// test bug and fails here rather than returning empty and being asserted against,
// so a third body appearing in the table cannot pass by being skipped.
func algoRecordedMoneyField(t *testing.T, params any, host algoMutationMoneyHost) string {
	t.Helper()
	// The nested members are read out of the shared strategyParam struct, which
	// both bodies carry, so a host on either body resolves here.
	readParam := func(p algoStrategyParamWire) string {
		switch host.field {
		case "maxVolume":
			return p.MaxVolume
		case "minAmount":
			return p.MinAmount
		case "showQty":
			return p.ShowQty
		case "qtyPercent":
			return p.QtyPercent
		}
		return ""
	}
	switch body := params.(type) {
	case algoAddOrderWireRequest:
		if host.nested {
			return readParam(body.StrategyParam)
		}
		switch host.field {
		case "entrustPrice":
			return body.EntrustPrice
		case "entrustAmount":
			return body.EntrustAmount
		}
	case algoChangeOrderWireRequest:
		if host.nested {
			return readParam(body.StrategyParam)
		}
		switch host.field {
		case "entrustPrice":
			return body.EntrustPrice
		case "entrustAmount":
			return body.EntrustAmount
		}
	}
	t.Fatalf("the recorded params are %T, which carries no %q field; the money host table "+
		"and the wire structs have drifted apart", params, host.field)
	return ""
}

// TestAlgoMoneyCrossesVerbatimThroughTheRealStack is the same guarantee proved end
// to end for one representative field per value type: a real *client.Client, a real
// HTTP round trip, a real JSON encode, and a value that a float64 anywhere in that
// path would destroy.
//
// The three rows are one price, one quantity and one nested quantity, so the test
// says "the real stack" without repeating the whole table. The read-side
// equivalent in the domain file carries the other direction (a value coming *off*
// the wire); this one carries a value going *onto* it, which is the direction a
// mutation takes and the one where a lost digit is a wrong order.
func TestAlgoMoneyCrossesVerbatimThroughTheRealStack(t *testing.T) {
	for _, tc := range []struct {
		name    string
		host    string
		method  string
		lit     moneyCase
		route   client.Route
		invoke  func(t *testing.T, ctx context.Context, svc *AlgoService) error
		wantKey string
	}{
		{
			name: "AddOrder/entrustPrice with a value no float64 holds", method: "AddOrder",
			route: client.RouteTradeAlgoAddOrder,
			lit:   float64HostilePrices[0],
			invoke: func(t *testing.T, ctx context.Context, svc *AlgoService) error {
				t.Helper()
				r := algoFixtureAdd()
				r.Price = domain.MustNewPrice(float64HostilePrices[0].in, "0")
				_, err := svc.AddOrder(ctx, algoFixtureAccount(), r)
				return err
			},
			wantKey: "entrustPrice",
		},
		{
			name: "AddOrder/entrustAmount with the 27-digit quantity", method: "AddOrder",
			route: client.RouteTradeAlgoAddOrder,
			lit:   float64HostileQuantities[0],
			invoke: func(t *testing.T, ctx context.Context, svc *AlgoService) error {
				t.Helper()
				r := algoFixtureAdd()
				r.Quantity = domain.MustNewQuantity(float64HostileQuantities[0].in)
				_, err := svc.AddOrder(ctx, algoFixtureAccount(), r)
				return err
			},
			wantKey: "entrustAmount",
		}, {
			// The shipped bug's own literal: 0.0005 rendered through the removed
			// %.3f became 0.001, a different and wrong price, with no error anywhere.
			name: "AddOrder/entrustPrice with the shipped bug's literal", method: "AddOrder",
			route: client.RouteTradeAlgoAddOrder,
			lit:   scaleHostilePrices[0],
			invoke: func(t *testing.T, ctx context.Context, svc *AlgoService) error {
				t.Helper()
				r := algoFixtureAdd()
				r.Price = domain.MustNewPrice(scaleHostilePrices[0].in, "0")
				_, err := svc.AddOrder(ctx, algoFixtureAccount(), r)
				return err
			},
			wantKey: "entrustPrice",
		},
		{
			// A nested member, which lives under a different key on a different
			// object — so it is read through the nested reader rather than the
			// envelope's, and a body that dropped strategyParam would fail here.
			name: "AddOrder/strategyParam.maxVolume with the 27-digit quantity", method: "AddOrder",
			route: client.RouteTradeAlgoAddOrder,
			lit:   float64HostileQuantities[0],
			invoke: func(t *testing.T, ctx context.Context, svc *AlgoService) error {
				t.Helper()
				r := algoFixtureAdd()
				r.StrategyParam.MaxVolume = domain.MustNewQuantity(float64HostileQuantities[0].in)
				_, err := svc.AddOrder(ctx, algoFixtureAccount(), r)
				return err
			},
			wantKey: "maxVolume",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_ = algoMutationCaseNamed(t, tc.method)
			rec := newWireRecorder(map[string]string{
				string(tc.route): gatewaySuccess(algoMutationBody),
			})
			if err := tc.invoke(t, t.Context(), NewAlgoService(newWireExecutor(t, rec))); err != nil {
				t.Fatalf("%s = %v, want nil", tc.name, err)
			}
			if got := rec.count(string(tc.route)); got != 1 {
				t.Fatalf("requests = %d, want exactly 1", got)
			}
			body := rec.lastBody(t, string(tc.route))
			algoAssertQuotedInRequest(t, body, tc.wantKey, tc.lit)
			algoAssertNotRenderedAs(t, body, tc.wantKey, tc.lit)
		})
	}
}

// TestAlgoEveryMoneyBearingMutationKeyIsQuoted is the whole-request assertion: every
// money-bearing key on every order-bearing body must be a quoted string on the
// wire, whatever the value.
//
// It is the aggregate the per-field table cannot be: a per-field assertion would
// pass if a *seventh* money field were added to a body and typed as a float, because
// no row would target it. This one walks the structs and looks at every
// money-shaped key, so a new one is covered by construction.
// TestAlgoReplyMoneyIsVerbatimThroughTheRealStack is the reply-side counterpart
// to the request table above, and it is not a restatement of the domain package's
// TestAlgoReplyMoneyIsVerbatim.
//
// That domain test builds the wire DTO and hands it to the mapper, so it starts
// *after* the JSON envelope and the codec. This one starts at an HTTP response
// body, which is the only place the two remaining hops are visible: the envelope
// decode into AlgoMasterOrderWire, and the codec's handling of the values on the
// way out. Both are string-typed today, so neither can lose a digit — but that is
// exactly the property worth pinning, because the loss is silent and a later
// change to a numeric JSON type (or a custom UnmarshalJSON on the wire struct)
// would reintroduce it with no compiler error and no test failing here.
//
// The nested strategyParam members are the rows that matter most: they sit one
// level deeper, so a body that flattened or dropped the object would still let
// the top-level rows pass.
func TestAlgoReplyMoneyIsVerbatimThroughTheRealStack(t *testing.T) {
	// The literal is chosen per row for the same reason the request table varies
	// it: a quantity is driven with the 27-digit integer, a price with the
	// value that underflows a float64, and the scale-hostile row carries the
	// literal the removed %.3f formatter would have re-plotted.
	priceLit := scaleHostilePrices[0]
	qtyLit := float64HostileQuantities[0]
	underflowLit := float64HostilePrices[0]

	for _, tc := range []struct {
		name    string
		lit     moneyCase
		nested  bool
		key     string
		rewrite func(t *testing.T, key, value string) json.RawMessage
		read    func(o *domain.AlgoMasterOrder) string
	}{
		{
			name: "entrustPrice", lit: underflowLit, key: "entrustPrice",
			rewrite: algoReplaceMasterOrderField,
			read:    func(o *domain.AlgoMasterOrder) string { return o.EntrustPrice.String() },
		},
		{
			name: "entrustAmount", lit: qtyLit, key: "entrustAmount",
			rewrite: algoReplaceMasterOrderField,
			read:    func(o *domain.AlgoMasterOrder) string { return o.EntrustAmount.String() },
		},
		{
			name: "avgPx", lit: underflowLit, key: "avgPx",
			rewrite: algoReplaceMasterOrderField,
			read:    func(o *domain.AlgoMasterOrder) string { return o.AvgPx.String() },
		},
		{
			name: "roundLot", lit: qtyLit, key: "roundLot",
			rewrite: algoReplaceMasterOrderField,
			read:    func(o *domain.AlgoMasterOrder) string { return o.RoundLot.String() },
		},
		{
			name: "strategyParam.maxVolume", lit: qtyLit, nested: true, key: "maxVolume",
			rewrite: algoReplaceStrategyParamField,
			read:    func(o *domain.AlgoMasterOrder) string { return o.StrategyParam.MaxVolume.String() },
		},
		{
			name: "strategyParam.minAmount", lit: priceLit, nested: true, key: "minAmount",
			rewrite: algoReplaceStrategyParamField,
			read:    func(o *domain.AlgoMasterOrder) string { return o.StrategyParam.MinAmount.String() },
		},
		{
			// showQty is absent from the mock's fixture, so this row also proves
			// an empty-string member on a nested object arrives as a zero value
			// rather than as an error, which is the absent-member rule applied at
			// depth.
			name: "strategyParam.showQty", lit: qtyLit, nested: true, key: "showQty",
			rewrite: algoReplaceStrategyParamField,
			read:    func(o *domain.AlgoMasterOrder) string { return o.StrategyParam.ShowQty.String() },
		},
		{
			name: "strategyParam.qtyPercent", lit: qtyLit, nested: true, key: "qtyPercent",
			rewrite: algoReplaceStrategyParamField,
			read:    func(o *domain.AlgoMasterOrder) string { return o.StrategyParam.QtyPercent.String() },
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := tc.rewrite(t, tc.key, tc.lit.in)
			rec := newWireRecorder(map[string]string{
				string(client.RouteTradeAlgoQueryOrderList): gatewaySuccess(string(body)),
			})

			orders, err := NewAlgoService(newWireExecutor(t, rec)).
				QueryOrderList(t.Context(), algoFixtureAccount(),
					AlgoOrderQuery{Page: algoFixturePage()})
			if err != nil {
				t.Fatalf("QueryOrderList = %v, want nil", err)
			}
			if len(orders) != 1 {
				t.Fatalf("QueryOrderList returned %d orders, want the fixture's single row, "+
					"so this row would assert against the wrong order", len(orders))
			}
			got := tc.read(orders[0])
			if got == tc.lit.want {
				return
			}
			t.Errorf("%s = %q, want %q (the literal the Gateway sent).\n"+
				"  input literal:          %s\n"+
				"  a float64 round trip:   %s\n"+
				"  the removed %%.3f form: %s",
				tc.name, got, tc.lit.want, tc.lit.in,
				strconv.FormatFloat(algoMustFloat(t, tc.lit.in), 'f', -1, 64),
				legacyRounded(algoMustFloat(t, tc.lit.in)))
		})
	}
}

func TestAlgoEveryMoneyBearingMutationKeyIsQuoted(t *testing.T) {
	// The keys a money value can arrive under, from the wire structs' own tags
	// rather than a restatement.
	moneyKeys := map[string]bool{
		"entrustPrice": true, "entrustAmount": true,
		"maxVolume": true, "minAmount": true, "showQty": true, "qtyPercent": true,
	}
	checked := 0
	// Both levels of a body are walked. strategyParam is nested, so a
	// top-level-only walk would read the aggregate as covering the nested money
	// keys and would miss maxVolume, minAmount, showQty and qtyPercent
	// entirely -- the four most likely to be retyped, because they are the ones
	// that look like plain numbers inside an object.
	nestedChecked := 0
	for _, body := range []any{
		algoAddOrderWireRequest{StrategyParam: algoStrategyParamWireFrom(algoFixtureStrategyParam())},
		algoChangeOrderWireRequest{StrategyParam: algoStrategyParamWireFrom(algoFixtureStrategyParam())},
	} {
		// Build a fully populated body so every omitempty key is actually emitted;
		// an absent key cannot be checked for its quoting.
		params := algoWireKeys(t, body)
		checked++
		for _, key := range algoBodyKeys(params) {
			if key == "strategyParam" {
				continue
			}
			if !moneyKeys[key] {
				continue
			}
			if strings.HasPrefix(string(params[key]), `"`) {
				continue
			}
			t.Errorf("the %s body emits %s as %s, want a quoted string. Every algo price, "+
				"quantity and amount crosses the wire as a quoted decimal string "+
				"(docs/DESIGN.md §7, hard rule 3), and a mutation writes this field",
				key, key, params[key])
		}
		nested := map[string]json.RawMessage{}
		if err := json.Unmarshal(params["strategyParam"], &nested); err != nil {
			t.Fatalf("strategyParam decoded as %s, want a nested object: a flat key would "+
				"mean the nested money values are not addressable at all, which is itself "+
				"a wire contract change (%v)", params["strategyParam"], err)
		}
		for _, key := range algoBodyKeys(nested) {
			if !moneyKeys[key] {
				continue
			}
			nestedChecked++
			if strings.HasPrefix(string(nested[key]), `"`) {
				continue
			}
			t.Errorf("strategyParam.%s is %s, want a quoted string: a nested money value is "+
				"no safer than a top-level one (docs/DESIGN.md §7, hard rule 3)",
				key, nested[key])
		}
	}
	if checked != 2 {
		t.Fatalf("checked %d bodies, want the two order-bearing algo bodies", checked)
	}
	// Every one of the four nested money keys must have been reached. Without the
	// count the loop above would also pass if a future edit renamed them, because
	// a renamed key is simply no longer in moneyKeys and is skipped in silence.
	if want := 4 * 2; nestedChecked != want {
		t.Errorf("checked %d nested strategyParam money keys, want %d (maxVolume, "+
			"minAmount, showQty and qtyPercent on both bodies)", nestedChecked, want)
	}
	// And the control: the page counters are *not* money, and are sent as numbers,
	// so the predicate is not simply "everything is a string".
	query := algoWireKeys(t, algoOrderQueryWireRequest{PageNo: 1, PageSize: 30})
	for _, key := range []string{"pageNo", "pageSize"} {
		if strings.HasPrefix(string(query[key]), `"`) {
			t.Errorf("the %q key is quoted on the query body: a page counter is not money "+
				"and both vendor SDKs send it as a number", key)
		}
	}
}
