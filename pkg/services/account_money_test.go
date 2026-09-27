// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package services

import (
	"encoding/json"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/shing1211/hstongapi4go/client"
	"github.com/shing1211/hstongapi4go/pkg/domain"
	"github.com/shing1211/hstongapi4go/pkg/transport"
)

// This file is the money and quantity regression net for account.go, and it rests
// on a fact that is true of this group and of neither of the two before it.
//
// Every numeric on every account wire type is a quoted string: the five DTOs in
// pkg/domain/account.go (MarginFundInfoWire, HoldsVoWire, FundJourVoWire, and the
// rate map's values) declare string, and the account request structs carry no
// numeric money at all — only queryCount, a page size, and two date strings. So
// unlike the HQ response path market_money_test.go covers, where the generated
// DTOs are float64 and encoding/json has already destroyed the extra digits
// before this layer sees them, here nothing between the Gateway and the domain
// value can lose a digit unless a future change introduces a float64 field or a
// %.Nf conversion.
//
// That makes the account group a byte-for-byte promise rather than a
// binary64-ceiling one, and these tests hold it to that. All five methods are
// covered: a hostile amount on MarginFundInfo, a hostile price, quantity, and
// rate on a HoldsList row, a hostile amount on both fund-journey walks, and a
// hostile rate in the nested rate map.
//
// Two value sets from the frozen testsupport_test.go, because there are two
// different failure modes and a value that discriminates one is often blind to
// the other:
//
//   - float64HostilePrices / float64HostileQuantities carry more significant
//     digits than binary64 can hold, or underflow it to zero. A float64 field
//     cannot pass them; only a verbatim decimal field can.
//   - scaleHostilePrices are float64-stable, so a float64 bug would not show —
//     but a fixed-scale formatter still destroys them, and that is exactly the
//     bug this repository shipped. All four survive a float64 round-trip
//     untouched, which is why they need their own table.
//
// The reference value is 1e-330: positive as a decimal and zero to every float64.
// A float64 in the path turns a real amount into nothing, silently, and the
// domain constructors would then see "0" — a plausible balance. Here the value
// survives as a 332-character positional decimal, and TestTheUnderflowingValueIs
// PositiveNotZero pins that contrast directly.

// accountMoneySet is one of the frozen value sets, with the guard that states
// which failure mode it is chosen to detect.
type accountMoneySet struct {
	label string
	cases []moneyCase
	guard func(*testing.T, string)
}

// accountMoneySets are both sets, applied to every host below rather than to
// whichever one a given host happened to get.
func accountMoneySets() []accountMoneySet {
	return []accountMoneySet{
		{"float64-hostile", float64HostilePrices, requireFloat64Hostile},
		{"scale-hostile", scaleHostilePrices, requireScaleHostile},
	}
}

// TestAccountMoneyValueSetsAreStillHostile is the guard on the guard.
//
// Each row's guard asserts the property that makes it worth having: a
// float64-hostile value changes digits under a float64 round-trip, a
// scale-hostile one is destroyed by %.3f. Without this, a future edit that
// softened a literal into a benign one would delete the regression net below
// without any test noticing — the same class of quiet regression as the bug
// itself. TestMoneyValueSetsStayHostile in market_money_test.go asserts the same
// rows; it is repeated here so this file's hosts are not silently disarmed by an
// edit to that one.
func TestAccountMoneyValueSetsAreStillHostile(t *testing.T) {
	if len(float64HostilePrices) == 0 || len(float64HostileQuantities) == 0 || len(scaleHostilePrices) == 0 {
		t.Fatal("a money value set is empty, so the round-trip tests in this file would pass " +
			"vacuously")
	}
	for _, set := range accountMoneySets() {
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

// accountAssertVerbatim is the central assertion of this file: the domain value
// reports the exact digits the Gateway sent, and the failure message names the
// two conversions that would have destroyed them.
func accountAssertVerbatim(t *testing.T, field, got string, tc moneyCase) {
	t.Helper()
	if got == tc.want {
		return
	}
	t.Errorf("%s = %q, want %q (the literal the Gateway sent).\n"+
		"  input literal:          %s\n"+
		"  a float64 round trip:   %s\n"+
		"  the removed %%.3f form: %s",
		field, got, tc.want, tc.in, accountFloatCeiling(tc.in), accountLegacy(t, tc.in))
}

// accountAssertQuotedOnWire is the real-stack half: the recorded reply body
// carries the value as a quoted JSON string, which is what a string field produces
// and what a float64 field cannot.
//
// It looks for tc.in, not tc.want, because the two differ: the Gateway sends the
// literal (1e-330) and the domain constructor expands it to the positional form
// (0.000…1). Asserting the expanded form here would be asserting the wrong layer.
//
// It is stated separately from accountAssertVerbatim because it catches a
// different regression. A field retyped from string to float64 would still decode
// and might still produce a plausible number, but it would arrive unquoted, and
// this assertion fails on the quoting alone.
func accountAssertQuotedOnWire(t *testing.T, body, field string, tc moneyCase) {
	t.Helper()
	quoted := `"` + field + `":"` + tc.in + `"`
	if !strings.Contains(body, quoted) {
		t.Errorf("the recorded reply does not contain %s.\n"+
			"  body:                %s\n"+
			"  wanted substring:    %s",
			quoted, body, quoted)
		return
	}
	if unquoted := `"` + field + `":` + tc.in; strings.Contains(body, unquoted) {
		t.Errorf("the unquoted form %s is also present, so the field is a JSON number on the "+
			"wire: a float64 in the response path would render this way", unquoted)
	}
}

// accountFloatCeiling renders what binary64 could have produced, for the failure
// message. A value that cannot be parsed is reported as such rather than
// panicking inside a failure path.
func accountFloatCeiling(in string) string {
	f, ok := accountParseFloat(in)
	if !ok {
		return "(not representable as a float64)"
	}
	return accountFormatFloat(f)
}

// accountLegacy is the removed fmt.Sprintf("%.3f", …) conversion, spelled out so a
// failure message can show what the old code did to the value.
func accountLegacy(t *testing.T, in string) string {
	t.Helper()
	f, ok := accountParseFloat(in)
	if !ok {
		return "(not representable as a float64)"
	}
	return legacyRounded(f)
}

// accountParseFloat and accountFormatFloat wrap strconv so a failure message in
// this file can render a value a float64 could not hold.
func accountParseFloat(in string) (float64, bool) {
	f, err := strconv.ParseFloat(in, 64)
	if err != nil {
		return 0, false
	}
	return f, true
}

func accountFormatFloat(f float64) string { return strconv.FormatFloat(f, 'f', -1, 64) }

// ---------------------------------------------------------------------------
// MarginFundInfo
// ---------------------------------------------------------------------------

// TestMarginFundInfoCarriesHostileAmountsVerbatim is the widest money host in the
// group: 26 Money fields plus a Rate, every one of them a quoted string on the
// wire.
//
// Only holdsBalance carries the hostile literal — the point is that one is enough
// to prove the field is verbatim-capable, and putting it in all 26 would make the
// failure message 26 lines long without proving anything more.
func TestMarginFundInfoCarriesHostileAmountsVerbatim(t *testing.T) {
	for _, set := range accountMoneySets() {
		t.Run(set.label, func(t *testing.T) {
			for _, tc := range set.cases {
				t.Run(tc.name, func(t *testing.T) {
					exec := newSequencedExecutor(t, sequencedReply{
						reply: marginFundInfoBodyWith(tc.in),
					})
					bal, err := NewAccountService(exec).MarginFundInfo(t.Context(),
						accountFixtureID(), MarginFundInfoRequest{})
					if err != nil {
						t.Fatalf("MarginFundInfo with holdsBalance %s: %v", tc.in, err)
					}
					accountAssertVerbatim(t, "AccountBalance.HoldsBalance", bal.HoldsBalance.String(), tc)
				})
			}
		})
	}

	// The hostile quantity is a 27-digit integer, legal as a decimal and
	// unrepresentable as a float64. It belongs here because a balance is the
	// largest thing a float64 would silently round.
	for _, tc := range float64HostileQuantities {
		t.Run("quantity/"+tc.name, func(t *testing.T) {
			exec := newSequencedExecutor(t, sequencedReply{reply: marginFundInfoBodyWith(tc.in)})
			bal, err := NewAccountService(exec).MarginFundInfo(t.Context(),
				accountFixtureID(), MarginFundInfoRequest{})
			if err != nil {
				t.Fatalf("MarginFundInfo with holdsBalance %s: %v", tc.in, err)
			}
			accountAssertVerbatim(t, "AccountBalance.HoldsBalance", bal.HoldsBalance.String(), tc)
		})
	}
}

// TestMarginFundInfoSpentRatioIsVerbatimToo covers the one Rate on the balance,
// which is the field a float64 would damage most visibly: a ratio is small, so
// binary64 keeps it positive but changes its digits, and the caller has no way to
// tell.
func TestMarginFundInfoSpentRatioIsVerbatimToo(t *testing.T) {
	// spentRatio is index 10 of accountMarginFields, between accountStatus and
	// currentCreditLimit. The index is asserted rather than assumed.
	const spentRatioIndex = 10
	if accountMarginFields[spentRatioIndex].key != "spentRatio" {
		t.Fatalf("accountMarginFields[%d] is %q, want spentRatio: the fixture and this test "+
			"have drifted apart", spentRatioIndex, accountMarginFields[spentRatioIndex].key)
	}

	for _, set := range accountMoneySets() {
		t.Run(set.label, func(t *testing.T) {
			for _, tc := range set.cases {
				t.Run(tc.name, func(t *testing.T) {
					body := marginFundInfoBodyWith(accountMarginFields[0].value)
					exec := newSequencedExecutor(t, sequencedReply{
						reply: accountReplaceField(t, body, "spentRatio", tc.in),
					})
					bal, err := NewAccountService(exec).MarginFundInfo(t.Context(),
						accountFixtureID(), MarginFundInfoRequest{})
					if err != nil {
						t.Fatalf("MarginFundInfo with spentRatio %s: %v", tc.in, err)
					}
					accountAssertVerbatim(t, "AccountBalance.SpentRatio", bal.SpentRatio.String(), tc)
				})
			}
		})
	}
}

// accountReplaceField rewrites one string field of a flat JSON object, so a money
// test can target a field other than the one marginFundInfoBodyWith replaces
// without hand-writing a second 28-field fixture.
func accountReplaceField(t *testing.T, raw json.RawMessage, key, value string) json.RawMessage {
	t.Helper()
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatalf("test bug: the fixture is not a flat JSON object: %v", err)
	}
	if _, ok := fields[key]; !ok {
		t.Fatalf("test bug: the fixture has no %q key; it has %v", key, accountSortedKeys(fields))
	}
	fields[key] = json.RawMessage(strconv.Quote(value))
	out, err := json.Marshal(fields)
	if err != nil {
		t.Fatalf("test bug: json.Marshal of the rewritten fixture failed: %v", err)
	}
	return out
}

// ---------------------------------------------------------------------------
// HoldsList
// ---------------------------------------------------------------------------

// TestHoldsListCarriesHostilePricesQuantitiesAndRatesVerbatim is the four-type row:
// a position carries a Money, a Price, a Quantity, and a Rate, and all four come
// off the wire as quoted strings.
//
// The quantity uses the 27-digit hostile value, which is a legal order quantity
// (an integer, and a multiple of the HK lot of 100) and something no float64 can
// hold — the market money file reaches the same conclusion from the other side,
// where the DTO field is an int64 and the value cannot be put on the wire at all.
func TestHoldsListCarriesHostilePricesQuantitiesAndRatesVerbatim(t *testing.T) {
	// Field indexes into accountHoldsWireKeys, each asserted by name so the test
	// cannot silently start driving a different field if the fixture is reordered.
	fields := []struct {
		key   string
		index int
	}{
		{"costPrice", 4},
		{"lastPrice", 5},
		{"dayCostPrice", 10},
		{"keepCostPrice", 13},
	}
	for _, f := range fields {
		if accountHoldsWireKeys[f.index] != f.key {
			t.Fatalf("accountHoldsWireKeys[%d] = %q, want %q: the fixture and this test have "+
				"drifted apart", f.index, accountHoldsWireKeys[f.index], f.key)
		}
	}

	for _, set := range accountMoneySets() {
		t.Run("price/"+set.label, func(t *testing.T) {
			for _, tc := range set.cases {
				t.Run(tc.name, func(t *testing.T) {
					for _, f := range fields {
						t.Run(f.key, func(t *testing.T) {
							rows := accountHoldsRowsAsValues()
							rows[0][f.index] = tc.in

							exec := newSequencedExecutor(t, sequencedReply{reply: holdsListBodyFromValues(rows)})
							positions, err := NewAccountService(exec).HoldsList(t.Context(),
								accountFixtureID(), HoldsFilter{})
							if err != nil {
								t.Fatalf("HoldsList with %s %s: %v", f.key, tc.in, err)
							}
							if len(positions) != 2 {
								t.Fatalf("positions = %d, want 2", len(positions))
							}
							accountAssertPositionPrice(t, positions[0], f.key, tc)
						})
					}
				})
			}
		})
	}

	t.Run("quantity", func(t *testing.T) {
		for _, tc := range float64HostileQuantities {
			t.Run(tc.name, func(t *testing.T) {
				for _, key := range []string{"enableAmount", "currentAmount"} {
					t.Run(key, func(t *testing.T) {
						index := accountHoldsIndexOf(t, key)
						rows := accountHoldsRowsAsValues()
						rows[0][index] = tc.in

						exec := newSequencedExecutor(t, sequencedReply{reply: holdsListBodyFromValues(rows)})
						positions, err := NewAccountService(exec).HoldsList(t.Context(),
							accountFixtureID(), HoldsFilter{})
						if err != nil {
							t.Fatalf("HoldsList with %s %s: %v", key, tc.in, err)
						}
						got := accountPositionQuantity(t, positions[0], key).String()
						accountAssertVerbatim(t, "Position[0]."+key, got, tc)
					})
				}
			})
		}
	})

	t.Run("rate", func(t *testing.T) {
		for _, set := range accountMoneySets() {
			t.Run(set.label, func(t *testing.T) {
				for _, tc := range set.cases {
					for _, key := range []string{"marketValueRate", "incomeRatio"} {
						t.Run(tc.name+"/"+key, func(t *testing.T) {
							index := accountHoldsIndexOf(t, key)
							rows := accountHoldsRowsAsValues()
							rows[1][index] = tc.in

							exec := newSequencedExecutor(t, sequencedReply{reply: holdsListBodyFromValues(rows)})
							positions, err := NewAccountService(exec).HoldsList(t.Context(),
								accountFixtureID(), HoldsFilter{})
							if err != nil {
								t.Fatalf("HoldsList with %s %s: %v", key, tc.in, err)
							}
							got := accountPositionRate(t, positions[1], key).String()
							accountAssertVerbatim(t, "Position[1]."+key, got, tc)
						})
					}
				}
			})
		}
	})

	t.Run("money", func(t *testing.T) {
		for _, set := range accountMoneySets() {
			t.Run(set.label, func(t *testing.T) {
				for _, tc := range set.cases {
					for _, key := range []string{"incomeBalance", "marketValue", "dayInComeAmount", "dayOutComeAmount"} {
						t.Run(tc.name+"/"+key, func(t *testing.T) {
							index := accountHoldsIndexOf(t, key)
							rows := accountHoldsRowsAsValues()
							rows[0][index] = tc.in

							exec := newSequencedExecutor(t, sequencedReply{reply: holdsListBodyFromValues(rows)})
							positions, err := NewAccountService(exec).HoldsList(t.Context(),
								accountFixtureID(), HoldsFilter{})
							if err != nil {
								t.Fatalf("HoldsList with %s %s: %v", key, tc.in, err)
							}
							got := accountPositionMoney(t, positions[0], key).String()
							accountAssertVerbatim(t, "Position[0]."+key, got, tc)
						})
					}
				}
			})
		}
	})
}

// accountHoldsIndexOf resolves a wire key to its index in accountHoldsWireKeys.
func accountHoldsIndexOf(t *testing.T, key string) int {
	t.Helper()
	for i, k := range accountHoldsWireKeys {
		if k == key {
			return i
		}
	}
	t.Fatalf("accountHoldsWireKeys has no %q key; it has %v", key, accountHoldsWireKeys)
	return -1
}

// accountHoldsRowsAsValues copies the fixture rows into positional slices, so a
// caller can edit one cell without mutating the shared fixture.
func accountHoldsRowsAsValues() [][]string {
	out := make([][]string, len(accountHoldsRows))
	for i, r := range accountHoldsRows {
		out[i] = append([]string(nil), accountHoldsFieldValues(r)...)
	}
	return out
}

// accountPositionField reads one field of a Position by Go name, so the money file
// does not need its own accessors for four types. The wire key is Pascal-cased to
// reach it, which accountAssertJSONTagsMatchFieldNames has already proved is the
// same field.
func accountPositionField(t *testing.T, pos *domain.Position, key string) reflect.Value {
	t.Helper()
	v := reflect.ValueOf(*pos).FieldByName(upperFirst(key))
	if !v.IsValid() {
		t.Fatalf("domain.Position has no field for the wire key %q; this test's expectation "+
			"is stale", key)
	}
	return v
}

func accountPositionPrice(t *testing.T, pos *domain.Position, key string) domain.Price {
	t.Helper()
	v := accountPositionField(t, pos, key)
	if v.Type() != reflect.TypeOf(domain.Price{}) {
		t.Fatalf("domain.Position.%s is a %s, not a domain.Price", upperFirst(key), v.Type())
	}
	return v.Interface().(domain.Price)
}

func accountPositionQuantity(t *testing.T, pos *domain.Position, key string) domain.Quantity {
	t.Helper()
	v := accountPositionField(t, pos, key)
	if v.Type() != reflect.TypeOf(domain.Quantity{}) {
		t.Fatalf("domain.Position.%s is a %s, not a domain.Quantity", upperFirst(key), v.Type())
	}
	return v.Interface().(domain.Quantity)
}

func accountPositionRate(t *testing.T, pos *domain.Position, key string) domain.Rate {
	t.Helper()
	v := accountPositionField(t, pos, key)
	if v.Type() != reflect.TypeOf(domain.Rate{}) {
		t.Fatalf("domain.Position.%s is a %s, not a domain.Rate", upperFirst(key), v.Type())
	}
	return v.Interface().(domain.Rate)
}

func accountPositionMoney(t *testing.T, pos *domain.Position, key string) domain.Money {
	t.Helper()
	v := accountPositionField(t, pos, key)
	if v.Type() != reflect.TypeOf(domain.Money{}) {
		t.Fatalf("domain.Position.%s is a %s, not a domain.Money", upperFirst(key), v.Type())
	}
	return v.Interface().(domain.Money)
}

// accountAssertPositionPrice checks one of the four prices of a position by wire
// key, going through the positional assertion the happy file already owns so the
// two files cannot disagree about which field is which.
func accountAssertPositionPrice(t *testing.T, pos *domain.Position, key string, tc moneyCase) {
	t.Helper()
	got := accountPositionPrice(t, pos, key).String()
	accountAssertVerbatim(t, "Position[0]."+key, got, tc)
}

// upperFirst is the Pascal-cased spelling of a Go field name.
func upperFirst(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// ---------------------------------------------------------------------------
// The fund-journey walks
// ---------------------------------------------------------------------------

// TestFundJourCarriesHostileAmountsVerbatim covers the one Money on a
// fund-journey row, through both walks.
//
// A walk is the harder host of the two: the value has to survive a cursor, a page
// boundary, and a re-fetch, and a hostile literal that underflowed to 0 on page
// one would look like a legitimate zero row on page two.
func TestFundJourCarriesHostileAmountsVerbatim(t *testing.T) {
	for _, set := range accountMoneySets() {
		t.Run(set.label, func(t *testing.T) {
			for _, tc := range set.cases {
				t.Run(tc.name, func(t *testing.T) {
					for _, req := range accountJourRequests() {
						t.Run(req.name, func(t *testing.T) {
							// Two pages, so the value is seen on both a first and a
							// subsequent fetch of the walk.
							exec := newSequencedExecutor(t,
								sequencedReply{reply: fundJourPageBody(
									accountFundJourRow(tc.in, "cursor-A"))},
								sequencedReply{reply: fundJourPageBody(
									accountFundJourRow(tc.in, ""))},
							)
							entries, err := req.invoke(t, t.Context(), NewAccountService(exec),
								FundJourFilter{}, transport.Pagination{PageSize: 1})
							if err != nil {
								t.Fatalf("%s with businessBalance %s: %v", req.name, tc.in, err)
							}
							if len(entries) != 2 {
								t.Fatalf("entries = %d, want 2 across the walk", len(entries))
							}
							for i, e := range entries {
								accountAssertVerbatim(t,
									"entries["+strconv.Itoa(i)+"].BusinessBalance", e.BusinessBalance.String(), tc)
							}
						})
					}
				})
			}
		})
	}
}

// ---------------------------------------------------------------------------
// RateQueryList
// ---------------------------------------------------------------------------

// TestRateQueryListCarriesHostileRatesVerbatim is the nested-map host: the rate is
// two levels down, so a converter introduced on the inner value would be easy to
// miss and the set comparison makes it visible.
//
// The assertion sorts (see accountSortedRates for why) and looks the cell up by its
// key pair rather than by position.
func TestRateQueryListCarriesHostileRatesVerbatim(t *testing.T) {
	for _, set := range accountMoneySets() {
		t.Run(set.label, func(t *testing.T) {
			for _, tc := range set.cases {
				t.Run(tc.name, func(t *testing.T) {
					// One hostile cell and three ordinary ones, so a converter that
					// only damaged the value it was introduced for is still caught and
					// the surrounding pairs prove the map was walked, not short-circuited.
					body := `{"HKD":{"USD":"7.8123456789012345","CNY":"` + tc.in + `"},` +
						`"CNY":{"HKD":"1.0967","USD":"0.13891"},` +
						`"USD":{"HKD":"0.12801"},"GBP":{}}`
					exec := newSequencedExecutor(t, sequencedReply{reply: json.RawMessage(body)})

					rates, err := NewAccountService(exec).RateQueryList(t.Context())
					if err != nil {
						t.Fatalf("RateQueryList with HKD/CNY %s: %v", tc.in, err)
					}
					if len(rates) != 5 {
						t.Fatalf("rates = %d, want 5", len(rates))
					}
					got, found := accountRateFor(rates, "HKD", "CNY")
					if !found {
						t.Fatalf("the result has no HKD/CNY pair; it has %v",
							accountRateKeys(accountSortedRates(rates)))
					}
					accountAssertVerbatim(t, "InterestRate{HKD,CNY}.Rate", got, tc)
				})
			}
		})
	}
}

// accountRateFor finds one pair in a rate result without relying on its order.
func accountRateFor(rates []*domain.InterestRate, source, target string) (string, bool) {
	for _, r := range rates {
		if r.SourceCurrency == source && r.TargetCurrency == target {
			return r.Rate.String(), true
		}
	}
	return "", false
}

// accountRateKeys renders a rate result for a failure message.
func accountRateKeys(rates []*domain.InterestRate) []string {
	out := make([]string, len(rates))
	for i, r := range rates {
		out[i] = r.SourceCurrency + "/" + r.TargetCurrency
	}
	return out
}

// ---------------------------------------------------------------------------
// The real stack
// ---------------------------------------------------------------------------

// TestAccountHostileValuesSurviveTheRealStack is the same guarantee proved end to
// end: a real *client.Client, a real HTTP round trip, a real JSON decode, and a
// value that a float64 anywhere in that path would destroy.
//
// It is the row that says "reached the wire" and "came off the wire" literally. A
// quoted-string assertion on the reply body is included, so a field retyped from
// string to float64 fails on the quoting even if the decoded number happened to
// look right.
func TestAccountHostileValuesSurviveTheRealStack(t *testing.T) {
	for _, set := range accountMoneySets() {
		t.Run(set.label, func(t *testing.T) {
			for _, tc := range set.cases {
				t.Run(tc.name, func(t *testing.T) {
					t.Run("MarginFundInfo", func(t *testing.T) {
						reply := string(marginFundInfoBodyWith(tc.in))
						rec := newWireRecorder(map[string]string{
							string(client.RouteTradeQueryMarginFundInfo): gatewaySuccess(reply),
						})
						bal, err := NewAccountService(newWireExecutor(t, rec)).MarginFundInfo(
							t.Context(), accountFixtureID(), MarginFundInfoRequest{})
						if err != nil {
							t.Fatalf("MarginFundInfo over the wire with %s: %v", tc.in, err)
						}
						accountAssertVerbatim(t, "AccountBalance.HoldsBalance", bal.HoldsBalance.String(), tc)
						accountAssertQuotedOnWire(t, reply, "holdsBalance", tc)
						if got := rec.count(string(client.RouteTradeQueryMarginFundInfo)); got != 1 {
							t.Errorf("requests = %d, want exactly 1", got)
						}
					})

					t.Run("HoldsList", func(t *testing.T) {
						rows := accountHoldsRowsAsValues()
						rows[0][accountHoldsIndexOf(t, "lastPrice")] = tc.in
						reply := string(holdsListBodyFromValues(rows))

						rec := newWireRecorder(map[string]string{
							string(client.RouteTradeQueryHoldsList): gatewaySuccess(reply),
						})
						positions, err := NewAccountService(newWireExecutor(t, rec)).HoldsList(
							t.Context(), accountFixtureID(), HoldsFilter{})
						if err != nil {
							t.Fatalf("HoldsList over the wire with %s: %v", tc.in, err)
						}
						if len(positions) != 2 {
							t.Fatalf("positions = %d, want 2", len(positions))
						}
						got := accountPositionPrice(t, positions[0], "lastPrice").String()
						accountAssertVerbatim(t, "Position[0].LastPrice", got, tc)
						accountAssertQuotedOnWire(t, reply, "lastPrice", tc)
					})

					t.Run("RealFundJourList", func(t *testing.T) {
						reply := string(fundJourPageBody(accountFundJourRow(tc.in, "")))
						rec := newWireRecorder(map[string]string{
							string(client.RouteTradeQueryRealFundJourList): gatewaySuccess(reply),
						})
						entries, err := NewAccountService(newWireExecutor(t, rec)).RealFundJourList(
							t.Context(), accountFixtureID(), FundJourFilter{}, transport.Pagination{})
						if err != nil {
							t.Fatalf("RealFundJourList over the wire with %s: %v", tc.in, err)
						}
						if len(entries) != 1 {
							t.Fatalf("entries = %d, want 1", len(entries))
						}
						accountAssertVerbatim(t, "entries[0].BusinessBalance", entries[0].BusinessBalance.String(), tc)
						accountAssertQuotedOnWire(t, reply, "businessBalance", tc)
					})

					t.Run("RateQueryList", func(t *testing.T) {
						reply := `{"HKD":{"CNY":"` + tc.in + `"}}`
						rec := newWireRecorder(map[string]string{
							string(client.RouteHsRateQueryList): gatewaySuccess(reply),
						})
						rates, err := NewAccountService(newWireExecutor(t, rec)).RateQueryList(t.Context())
						if err != nil {
							t.Fatalf("RateQueryList over the wire with %s: %v", tc.in, err)
						}
						got, found := accountRateFor(rates, "HKD", "CNY")
						if !found {
							t.Fatalf("the result has no HKD/CNY pair")
						}
						accountAssertVerbatim(t, "InterestRate{HKD,CNY}.Rate", got, tc)
						accountAssertQuotedOnWire(t, reply, "CNY", tc)
					})
				})
			}
		})
	}
}

// TestTheUnderflowingValueIsPositiveNotZero states the reference case on its own,
// because it is the value that makes the whole table necessary.
//
// 1e-330 is a positive decimal that is exactly zero to every float64. Sent as
// holdsBalance it arrives as a 332-character positional amount; a float64 in the
// path would report 0 — a plausible balance, not an error, and not even a wrong
// sign. The domain constructor panics on a parse failure, so nothing downstream
// could have caught it either.
func TestTheUnderflowingValueIsPositiveNotZero(t *testing.T) {
	const underflow = "1e-330"
	requireFloat64Hostile(t, underflow)

	rec := newWireRecorder(map[string]string{
		string(client.RouteTradeQueryMarginFundInfo): gatewaySuccess(
			string(marginFundInfoBodyWith(underflow))),
	})
	bal, err := NewAccountService(newWireExecutor(t, rec)).MarginFundInfo(t.Context(),
		accountFixtureID(), MarginFundInfoRequest{})
	if err != nil {
		t.Fatalf("MarginFundInfo with an underflowing holdsBalance: %v", err)
	}

	got := bal.HoldsBalance.String()
	if got == "0" {
		t.Fatalf("HoldsBalance = 0 for a wire value of %s: a float64 is in the path and the "+
			"amount has been silently destroyed", underflow)
	}
	if len(got) != len(underflowPositional) {
		t.Errorf("HoldsBalance = %q (%d characters), want the full positional form (%d characters)",
			got, len(got), len(underflowPositional))
	}
	if got != underflowPositional {
		t.Errorf("HoldsBalance = %q, want the positional expansion of %s", got, underflow)
	}
	if !strings.HasPrefix(got, "0.0000") {
		t.Errorf("HoldsBalance = %q, want the positional form starting 0.0000", got)
	}
	// A positive decimal that reports as positive is the other half: IsPositive
	// excludes zero explicitly, so a float64 collapse would also have flipped this.
	if !bal.HoldsBalance.IsPositive() {
		t.Errorf("HoldsBalance.IsPositive() = false for %s, want true: the value is positive as "+
			"a decimal, so only a float64 collapse could make it zero", underflow)
	}
	if bal.HoldsBalance.IsZero() {
		t.Error("HoldsBalance.IsZero() = true, which would mean the underflowed value survived " +
			"as a zero rather than as the decimal the Gateway sent")
	}
}

// TestAccountDomainTypesCarryNoFloat is the static half.
//
// docs/DESIGN.md §7 and hard rule 3 say money and quantities are never float64.
// scripts/check_money.py enforces that by *name* and by *wire key*, which is the
// right heuristic for the generated DTOs but is blind to a decimal field given an
// innocuous name. This walks the four domain types the account methods return and
// asserts no field is a float at all, whatever it is called.
func TestAccountDomainTypesCarryNoFloat(t *testing.T) {
	for _, v := range []any{
		domain.AccountBalance{},
		domain.Position{},
		domain.FundJournalEntry{},
		domain.InterestRate{},
	} {
		t.Run(reflect.TypeOf(v).Name(), func(t *testing.T) {
			rt := reflect.TypeOf(v)
			for i := 0; i < rt.NumField(); i++ {
				f := rt.Field(i)
				switch f.Type.Kind() {
				case reflect.Float32, reflect.Float64:
					t.Errorf("%s.%s is a %s: money, prices, quantities, and rates are decimal "+
						"values in this layer (docs/DESIGN.md §7)", rt.Name(), f.Name, f.Type)
				}
			}
			if rt.NumField() == 0 {
				t.Errorf("%s has no fields, so this test would pass vacuously", rt.Name())
			}
		})
	}
}

// TestAccountDomainTypesCarryDecimal is the positive half of the same rule, and it
// is what would fail first if a field were retyped from decimal to string: a plain
// string would hold the digits but lose the currency and the scale, so a caller
// could no longer format the value safely.
//
// Every field must be one of the four decimal types or one of an explicitly listed
// set of descriptive strings. The list is written out rather than counted, so a
// numeric retyped as a string is named in the failure instead of being absorbed
// by a moved threshold.
func TestAccountDomainTypesCarryDecimal(t *testing.T) {
	descriptive := map[string][]string{
		"AccountBalance":   {"AccountStatus"},
		"Position":         {"StockName", "StockCode", "ExchangeType"},
		"FundJournalEntry": {"Type", "TypeDesc", "FundJourParentType", "Time", "QueryParamStr"},
		"InterestRate":     {"SourceCurrency", "TargetCurrency"},
	}
	decimalTypes := map[reflect.Type]bool{
		reflect.TypeOf(domain.Money{}):    true,
		reflect.TypeOf(domain.Price{}):    true,
		reflect.TypeOf(domain.Quantity{}): true,
		reflect.TypeOf(domain.Rate{}):     true,
	}

	for _, v := range []any{
		domain.AccountBalance{},
		domain.Position{},
		domain.FundJournalEntry{},
		domain.InterestRate{},
	} {
		rt := reflect.TypeOf(v)
		wantPlain := descriptive[rt.Name()]
		if len(wantPlain) == 0 {
			t.Fatalf("test bug: %s has no listed descriptive fields, so the string check "+
				"below would be vacuous", rt.Name())
		}
		t.Run(rt.Name(), func(t *testing.T) {
			listed := make(map[string]bool, len(wantPlain))
			for _, n := range wantPlain {
				listed[n] = true
			}
			decimals, plain := 0, 0
			for i := 0; i < rt.NumField(); i++ {
				f := rt.Field(i)
				if decimalTypes[f.Type] {
					decimals++
					continue
				}
				if f.Type.Kind() == reflect.String && listed[f.Name] {
					listed[f.Name] = false
					plain++
					continue
				}
				t.Errorf("%s.%s is a %s, want one of domain.Money/Price/Quantity/Rate or one of "+
					"the descriptive strings %v: anything else carries no currency and no scale",
					rt.Name(), f.Name, f.Type, wantPlain)
			}
			if decimals == 0 {
				t.Errorf("%s has no decimal field at all, so this test would pass vacuously", rt.Name())
			}
			for name, unused := range listed {
				if unused {
					t.Errorf("%s no longer has a string field named %q, so this test's list of "+
						"descriptive fields is stale", rt.Name(), name)
				}
			}
			if plain != len(wantPlain) {
				t.Errorf("%s has %d descriptive string fields, want %d", rt.Name(), plain, len(wantPlain))
			}
		})
	}
}
