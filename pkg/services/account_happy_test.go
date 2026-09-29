// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package services

import (
	"context"
	"encoding/json"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/shing1211/hstongapi4go/client"
	"github.com/shing1211/hstongapi4go/pkg/domain"
	"github.com/shing1211/hstongapi4go/pkg/types"
)

// This file covers the five uncovered methods in account.go: MarginFundInfo,
// HoldsList, RealFundJourList, HistoryFundJourList, and RateQueryList.
//
// It is the happy-path and wire-shape half. The failure half is in
// account_errors_test.go and the money half in account_money_test.go, both of
// which build on the fixtures declared here.
//
// Two properties of this file shape it more than anything else.
//
// Every fixture populates every field. domain.AccountBalanceFromDTO,
// PositionFromDTO, FundJournalEntryFromDTO, and InterestRateFromDTO each panic on
// an unparseable decimal — finding F2 in
// docs/runs/2026-09-26-vnext-parity-wire/plan-services-tests.md §9 — and an
// absent quoted field decodes to "", so there is no recover() anywhere in this
// file. accountAssertJSONTagsMatchFieldNames is what makes that maintainable: it
// fails the *test* with a readable message when a fixture drifts from the DTO,
// instead of letting the drift become a panic in a decimal constructor.
//
// RateQueryList's result order is nondeterministic, so nothing here asserts it.
// The wire response is map[string]map[string]string and the method appends to a
// slice while ranging over it (account.go:280-285), so Go's randomised map
// iteration reaches the caller directly. accountSortedRates is the only way this
// file looks at that result; the comment on it says why.

// accountFixtureID is the account every fixture asks for.
func accountFixtureID() domain.AccountID { return domain.AccountID("ACC-B4-0001") }

// ---------------------------------------------------------------------------
// Fixtures — MarginFundInfo
// ---------------------------------------------------------------------------

// accountMarginField is one populated field of a MarginFundInfo reply. The keys
// are written out by hand rather than read from the struct's tags, so a renamed,
// added, or removed tag is caught by the reflection guard rather than being
// silently satisfied by the fixture that mirrors the struct.
type accountMarginField struct {
	key   string
	value string
}

// accountMarginFields are the 28 fields of a MarginFundInfo reply, in
// MarginFundInfoWire declaration order. Every value is a distinct canonical
// decimal string, so a mapper that read the wrong field — or copied one source
// into two destinations — is visible rather than plausible.
//
// accountStatus is the one non-decimal field: the mapper copies it straight
// through, which is why it carries a word rather than a number. Every other
// value must be a valid decimal, because that is what the mapper parses.
var accountMarginFields = []accountMarginField{
	{key: "holdsBalance", value: "1000.001"},
	{key: "assetBalance", value: "2000.002"},
	{key: "enableBalance", value: "3000.003"},
	{key: "marketValue", value: "4000.004"},
	{key: "cashOnHold", value: "5000.005"},
	{key: "creditValue", value: "6000.006"},
	{key: "creditLine", value: "7000.007"},
	{key: "fetchBalance", value: "8000.008"},
	{key: "frozenBalance", value: "9000.009"},
	{key: "accountStatus", value: "NORMAL"},
	{key: "spentRatio", value: "0.35"},
	{key: "currentCreditLimit", value: "11000.011"},
	{key: "maxCreditLimit", value: "12000.012"},
	{key: "unitCreditLimit", value: "13000.013"},
	{key: "unitMaxCreditLimit", value: "14000.014"},
	{key: "buyPower", value: "15000.015"},
	{key: "buyPowerCredit", value: "16000.016"},
	{key: "buyPowerHk", value: "17000.017"},
	{key: "buyPowerUs", value: "18000.018"},
	{key: "buyPowerCn", value: "19000.019"},
	{key: "unitedBuyPowerHk", value: "21000.021"},
	{key: "unitedBuyPowerUs", value: "22000.022"},
	{key: "unitedBuyPowerCn", value: "23000.023"},
	{key: "thirdBuyPowerHk", value: "24000.024"},
	{key: "thirdBuyPowerUs", value: "25000.025"},
	{key: "thirdBuyPowerCn", value: "26000.026"},
	{key: "buyPowerShortMarket", value: "27000.027"},
	{key: "buyPowerMoney", value: "28000.028"},
}

// accountMarginValues returns the 28 fixture values in field order, which is the
// order the assertions walk domain.AccountBalance in.
func accountMarginValues() []string {
	out := make([]string, len(accountMarginFields))
	for i, f := range accountMarginFields {
		out[i] = f.value
	}
	return out
}

// marginFundInfoBody is a MarginFundInfo reply with all 28 fields populated.
func marginFundInfoBody() json.RawMessage {
	return marginFundInfoBodyWith(accountMarginFields[0].value)
}

// marginFundInfoBodyWith is marginFundInfoBody with holdsBalance replaced, so the
// money file can push a hostile literal through the same fixture instead of
// hand-writing a second one that could drift from this one.
func marginFundInfoBodyWith(holdsBalance string) json.RawMessage {
	var b strings.Builder
	b.WriteByte('{')
	for i, f := range accountMarginFields {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(`"` + f.key + `":`)
		if f.key == "holdsBalance" {
			b.WriteString(`"` + holdsBalance + `"`)
		} else {
			b.WriteString(`"` + f.value + `"`)
		}
	}
	b.WriteByte('}')
	return json.RawMessage(b.String())
}

// ---------------------------------------------------------------------------
// Fixtures — HoldsList
// ---------------------------------------------------------------------------

// accountHoldsRow is one populated position row. All fifteen values are valid
// decimals or plain text, and every value is distinct across the two fixture
// rows, so a row mix-up is visible.
type accountHoldsRow struct {
	stockName, enableAmount, currentAmount, stockCode string
	costPrice, lastPrice                              string
	incomeBalance, marketValue                        string
	marketValueRate, incomeRatio                      string
	dayCostPrice                                      string
	dayInComeAmount, dayOutComeAmount                 string
	keepCostPrice, exchangeType                       string
}

// accountHoldsRows are the two positions every HoldsList fixture returns.
var accountHoldsRows = []accountHoldsRow{
	{
		stockName: "TENCENT", enableAmount: "1000", currentAmount: "2000",
		stockCode: "00700", costPrice: "380.123", lastPrice: "387.05",
		incomeBalance: "1386.5", marketValue: "774100",
		marketValueRate: "0.0168", incomeRatio: "0.0036",
		dayCostPrice:    "381.234",
		dayInComeAmount: "100.001", dayOutComeAmount: "50.002",
		keepCostPrice: "380.5", exchangeType: "K",
	},
	{
		stockName: "APPLE", enableAmount: "300", currentAmount: "450",
		stockCode: "AAPL", costPrice: "189.5", lastPrice: "195.25",
		incomeBalance: "2595", marketValue: "87862.5",
		marketValueRate: "0.03034", incomeRatio: "0.0295",
		dayCostPrice:    "190.005",
		dayInComeAmount: "75.003", dayOutComeAmount: "25.004",
		keepCostPrice: "188.75", exchangeType: "P",
	},
}

// accountHoldsWireKeys are the fifteen JSON keys of a HoldsVo row, hand written
// in declaration order, for the same reason as accountMarginFields.
var accountHoldsWireKeys = []string{
	"stockName", "enableAmount", "currentAmount", "stockCode",
	"costPrice", "lastPrice", "incomeBalance", "marketValue",
	"marketValueRate", "incomeRatio", "dayCostPrice",
	"dayInComeAmount", "dayOutComeAmount", "keepCostPrice", "exchangeType",
}

// accountHoldsFieldValues returns one row's fifteen values in declaration order.
func accountHoldsFieldValues(r accountHoldsRow) []string {
	return []string{
		r.stockName, r.enableAmount, r.currentAmount, r.stockCode,
		r.costPrice, r.lastPrice, r.incomeBalance, r.marketValue,
		r.marketValueRate, r.incomeRatio, r.dayCostPrice,
		r.dayInComeAmount, r.dayOutComeAmount, r.keepCostPrice, r.exchangeType,
	}
}

// holdsListBody is a HoldsList reply carrying both fixture rows.
func holdsListBody() json.RawMessage {
	rows := make([][]string, len(accountHoldsRows))
	for i, r := range accountHoldsRows {
		rows[i] = accountHoldsFieldValues(r)
	}
	return holdsListBodyFromValues(rows)
}

// holdsListBodyFromValues renders a HoldsList reply from positional field values,
// one slice per row, in accountHoldsWireKeys order.
//
// It is factored out of holdsListBody so account_money_test.go can push a hostile
// literal into one cell without hand-writing a second fixture that could drift
// from this one.
func holdsListBodyFromValues(rows [][]string) json.RawMessage {
	var b strings.Builder
	b.WriteString(`{"holdsList":[`)
	for i, values := range rows {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteByte('{')
		for j, key := range accountHoldsWireKeys {
			if j > 0 {
				b.WriteByte(',')
			}
			b.WriteString(`"` + key + `":"` + values[j] + `"`)
		}
		b.WriteByte('}')
	}
	b.WriteString(`]}`)
	return json.RawMessage(b.String())
}

// ---------------------------------------------------------------------------
// Fixtures — the fund-journey pages
// ---------------------------------------------------------------------------

// accountFundJourFields are the six JSON keys of a FundJourVo.
var accountFundJourFields = []string{
	"businessBalance", "type", "typeDesc", "fundJourParentType", "time", "queryParamStr",
}

// accountFundJourValues are the six values accountFundJourRow writes, in the same
// order. time is populated on every row even though the spec calls it
// historical-only: the code parses it unconditionally, so a fixture that left it
// out would be relying on a field being optional when the code does not treat it
// that way.
var accountFundJourValues = []string{"1386.5", "CASH", "deposit", "CASH_IN", "20260926 09:30:01", "cursor-A"}

// accountFundJourRow is one populated FundJourVo carrying the given amount and
// cursor. Every other field comes from accountFundJourValues.
func accountFundJourRow(businessBalance, cursor string) string {
	values := append([]string(nil), accountFundJourValues...)
	values[0] = businessBalance
	values[5] = cursor

	var b strings.Builder
	b.WriteByte('{')
	for i, key := range accountFundJourFields {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(`"` + key + `":"` + values[i] + `"`)
	}
	b.WriteByte('}')
	return b.String()
}

// fundJourPageBody is a fund-journey reply whose data member is the given rows,
// written verbatim so an empty page is expressible as a real `[]`.
func fundJourPageBody(rows ...string) json.RawMessage {
	return json.RawMessage(`{"data":[` + strings.Join(rows, ",") + `]}`)
}

// ---------------------------------------------------------------------------
// Fixtures — RateQueryList
// ---------------------------------------------------------------------------

// accountRateFixtures is the rate map every RateQueryList fixture returns: three
// sources and one source with no targets at all.
//
// The empty "GBP" object is deliberate. It pins that a source with no targets
// contributes nothing, so the result has 5 rows rather than 6 — which is the
// nested loop's whole behaviour.
const accountRateFixtures = `{"HKD":{"USD":"7.8123456789012345","CNY":"0.912345678901234"},` +
	`"CNY":{"HKD":"1.0967","USD":"0.13891"},` +
	`"USD":{"HKD":"0.12801"},` +
	`"GBP":{}}`

// accountRatePair is one expected (source, target, rate) triple.
type accountRatePair struct {
	source, target, rate string
}

// accountWantRates is accountRateFixtures read by hand, in the sorted order
// accountSortedRates produces. "GBP" is absent by design: the empty source
// contributes no row, and TestRateQueryListReturnsEveryPairAsAKeyedSet asserts
// its absence separately so a row appearing for it is a named failure.
var accountWantRates = []accountRatePair{
	{"CNY", "HKD", "1.0967"},
	{"CNY", "USD", "0.13891"},
	{"HKD", "CNY", "0.912345678901234"},
	{"HKD", "USD", "7.8123456789012345"},
	{"USD", "HKD", "0.12801"},
}

// accountSortedRates returns the rates ordered by source then target, so a
// result can be compared positionally after the fact.
//
// This is the only correct way to assert on RateQueryList's result. The method
// ranges over a map twice and appends (account.go:280-285), so Go's randomised
// iteration order reaches the caller unchanged: an assertion written against the
// fixture's own key order would fail on most runs, and one written as a set would
// pass a method that dropped or duplicated a row. Sorting pins the contents
// exactly while discarding the order a caller cannot rely on.
func accountSortedRates(rates []*domain.InterestRate) []*domain.InterestRate {
	out := append([]*domain.InterestRate(nil), rates...)
	sort.Slice(out, func(i, j int) bool {
		if out[i].SourceCurrency != out[j].SourceCurrency {
			return out[i].SourceCurrency < out[j].SourceCurrency
		}
		return out[i].TargetCurrency < out[j].TargetCurrency
	})
	return out
}

// ---------------------------------------------------------------------------
// Assertion helpers
// ---------------------------------------------------------------------------

// accountAssertJSONTagsMatchFieldNames is the guard that makes "populate every
// field" checkable rather than a matter of vigilance.
//
// It asserts that each DTO's json tag is the lower-camel spelling of the domain
// struct's field name at the *same index*. That single property is what lets the
// assertions below pair fixture values with domain fields positionally instead of
// through 28 hand-written expectations that could drift from the fixture.
//
// A field added, removed, reordered, or renamed on either side fails here with a
// message naming the pair — instead of becoming a panic inside a decimal
// constructor on the first run that exercised it.
func accountAssertJSONTagsMatchFieldNames(t *testing.T, dto, domainValue any) {
	t.Helper()
	dtoType, domType := reflect.TypeOf(dto), reflect.TypeOf(domainValue)

	if dtoType.NumField() != domType.NumField() {
		t.Fatalf("%s has %d fields and %s has %d; the fixture and the assertions are "+
			"indexed by position and cannot be paired",
			dtoType.Name(), dtoType.NumField(), domType.Name(), domType.NumField())
	}
	for i := 0; i < dtoType.NumField(); i++ {
		tag, _, _ := strings.Cut(dtoType.Field(i).Tag.Get("json"), ",")
		name := domType.Field(i).Name
		if want := lowerFirst(name); tag != want {
			t.Errorf("%s field %d: json tag %q, want %q (the lower-camel spelling of %s.%s). "+
				"The fixture indexes these positionally, so the mismatch is not cosmetic",
				dtoType.Name(), i, tag, want, domType.Name(), name)
		}
	}
}

// lowerFirst is the lower-camel spelling of a Go field name.
func lowerFirst(s string) string {
	if s == "" {
		return s
	}
	return strings.ToLower(s[:1]) + s[1:]
}

// accountAssertFixtureCoversEveryKey fails when a hand-written key list and a
// fixture's own JSON object disagree, or when either disagrees with the DTO.
func accountAssertFixtureCoversEveryKey(t *testing.T, dto any, keys []string, raw json.RawMessage) {
	t.Helper()
	dtoType := reflect.TypeOf(dto)

	var want []string
	for i := 0; i < dtoType.NumField(); i++ {
		tag, _, _ := strings.Cut(dtoType.Field(i).Tag.Get("json"), ",")
		if tag != "" && tag != "-" {
			want = append(want, tag)
		}
	}
	if !reflect.DeepEqual(want, keys) {
		t.Errorf("the hand-written key list = %v, want the %d json tags of %s in order: %v",
			keys, len(want), dtoType.Name(), want)
	}

	var got map[string]json.RawMessage
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("test bug: the fixture is not a JSON object: %v", err)
	}
	if len(got) != len(want) {
		t.Errorf("the fixture has %d keys, want %d: %v", len(got), len(want), accountSortedKeys(got))
	}
	for _, k := range want {
		if _, ok := got[k]; !ok {
			t.Errorf("the fixture is missing the wire key %q, which %s declares. An absent "+
				"key decodes to \"\" and panics in the decimal constructor", k, dtoType.Name())
		}
	}
}

// accountSortedKeys renders a decoded JSON object's keys for a failure message.
func accountSortedKeys(m map[string]json.RawMessage) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// accountAssertAllDistinct fails when a fixture reuses a value, which is what
// would let a mapper that read the wrong field pass.
func accountAssertAllDistinct(t *testing.T, label string, values []string) {
	t.Helper()
	seen := make(map[string]int, len(values))
	for i, v := range values {
		if prev, dup := seen[v]; dup {
			t.Fatalf("the %s fixture reuses %q at indexes %d and %d, so a field read "+
				"from the wrong source would be invisible", label, v, prev, i)
		}
		seen[v] = i
	}
}

// accountParamsAs type-asserts the params of the most recent recorded call.
func accountParamsAs[T any](t *testing.T, exec *sequencedExecutor) T {
	t.Helper()
	return accountParamsAt[T](t, exec, exec.callCount()-1)
}

// accountParamsAt is accountParamsAs for an arbitrary recorded call index, which
// is what a multi-page walk needs: lastParams would report page two twice.
func accountParamsAt[T any](t *testing.T, exec *sequencedExecutor, i int) T {
	t.Helper()
	raw := exec.callAt(t, i).params
	typed, ok := raw.(T)
	if !ok {
		t.Fatalf("recorded params for call %d = %T, want %T", i, raw, *new(T))
	}
	return typed
}

// accountExpectCall asserts the op and route of the most recent recorded call.
// docs/SPEC.md owns the route table and this repository treats it as canonical,
// so a method that drifted onto another endpoint fails here.
func accountExpectCall(t *testing.T, exec *sequencedExecutor, wantOp string, wantRoute client.Route) {
	t.Helper()
	got := exec.lastCall(t)
	if got.op != wantOp {
		t.Errorf("op = %q, want %q", got.op, wantOp)
	}
	if got.route != wantRoute {
		t.Errorf("route = %q, want %q", got.route, wantRoute)
	}
}

// accountRecordedParams decodes the params member of the recorded request
// envelope for path.
//
// It decodes rather than substring-matches, because the assertions in this file
// are about a key being *absent* and a substring search cannot tell an absent key
// from one spelled differently.
func accountRecordedParams(t *testing.T, rec *wireRecorder, path string) map[string]json.RawMessage {
	t.Helper()
	var envelope struct {
		Params json.RawMessage `json:"params"`
	}
	if err := json.Unmarshal([]byte(rec.lastBody(t, path)), &envelope); err != nil {
		t.Fatalf("the recorded request body is not the expected envelope: %v", err)
	}
	var params map[string]json.RawMessage
	if err := json.Unmarshal(envelope.Params, &params); err != nil {
		t.Fatalf("the envelope's params member is not a JSON object: %v", err)
	}
	return params
}

// accountStringField reads one string field out of a decoded params member.
func accountStringField(t *testing.T, params map[string]json.RawMessage, key string) string {
	t.Helper()
	raw, ok := params[key]
	if !ok {
		t.Fatalf("the request carries no %q key; it carries %v", key, accountSortedKeys(params))
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		t.Fatalf("the %q field is not a JSON string (%s): %v", key, raw, err)
	}
	return s
}

// accountIntField reads one integer field out of a decoded params member.
func accountIntField(t *testing.T, params map[string]json.RawMessage, key string) int {
	t.Helper()
	raw, ok := params[key]
	if !ok {
		t.Fatalf("the request carries no %q key; it carries %v", key, accountSortedKeys(params))
	}
	var n int
	if err := json.Unmarshal(raw, &n); err != nil {
		t.Fatalf("the %q field is not a JSON integer (%s): %v", key, raw, err)
	}
	return n
}

// accountAssertEveryField compares each exported field of v against want, keyed
// by field name, through the decimal accessor its type provides.
func accountAssertEveryField(t *testing.T, v reflect.Value, label string, want map[string]string) {
	t.Helper()
	rt := v.Type()
	seen := make(map[string]bool, rt.NumField())
	for i := 0; i < rt.NumField(); i++ {
		name := rt.Field(i).Name
		seen[name] = true

		got, ok := accountFieldString(v.Field(i))
		if !ok {
			t.Errorf("%s.%s has type %s, which is neither one of the decimal types nor a "+
				"string; this file must be taught how to compare it", label, name, rt.Field(i).Type)
			continue
		}
		w, ok := want[name]
		switch {
		case !ok:
			t.Errorf("%s.%s = %q, which the fixture does not set: the fixture and the "+
				"domain struct have drifted apart", label, name, got)
		case got != w:
			t.Errorf("%s.%s = %q, want %q: the value landed in the wrong domain field, or "+
				"the mapper read the wrong wire key", label, name, got, w)
		}
	}
	for name := range want {
		if !seen[name] {
			t.Errorf("the fixture sets %q, which %s has no field for", name, label)
		}
	}
}

// accountFieldString renders a decimal-bearing or string field as the exact
// digits a caller would see, and reports whether the type is one this file
// supports. An unrecognised type is a test bug, never a value to skip.
func accountFieldString(v reflect.Value) (string, bool) {
	switch x := v.Interface().(type) {
	case domain.Money:
		return x.String(), true
	case domain.Price:
		return x.String(), true
	case domain.Quantity:
		return x.String(), true
	case domain.Rate:
		return x.String(), true
	case string:
		return x, true
	default:
		return "", false
	}
}

// accountExpectationsFor builds the field-name-keyed expectation map for a domain
// struct from a fixture's positional values, using the order accountAssertJSON
// TagsMatchFieldNames has already proved to be shared.
func accountExpectationsFor(domainValue any, values []string) map[string]string {
	rt := reflect.TypeOf(domainValue)
	out := make(map[string]string, rt.NumField())
	for i := 0; i < rt.NumField(); i++ {
		out[rt.Field(i).Name] = values[i]
	}
	return out
}

// accountMoneyField reads one Money field by name, for the currency assertions.
func accountMoneyField(t *testing.T, bal *domain.AccountBalance, name string) domain.Money {
	t.Helper()
	v := reflect.ValueOf(*bal).FieldByName(name)
	if !v.IsValid() {
		t.Fatalf("AccountBalance has no field %q; this test's expectation is stale", name)
	}
	money, ok := v.Interface().(domain.Money)
	if !ok {
		t.Fatalf("AccountBalance.%s is not a domain.Money (%s)", name, v.Type())
	}
	return money
}

// accountAssertEveryMoneyIsScaled asserts every Money field of the balance
// carries a non-empty currency and the HKD scale the mapper declares.
//
// A zero-value Money — the result of an unpopulated field — has an empty currency
// and scale 0, so this is also the check that notices a field that never received
// a value at all.
func accountAssertEveryMoneyIsScaled(t *testing.T, bal *domain.AccountBalance, wantMoneyFields int) {
	t.Helper()
	v := reflect.ValueOf(*bal)
	rt := v.Type()
	seen := 0
	for i := 0; i < rt.NumField(); i++ {
		if rt.Field(i).Type != reflect.TypeOf(domain.Money{}) {
			continue
		}
		seen++
		money := v.Field(i).Interface().(domain.Money)
		if money.Currency() == "" {
			t.Errorf("AccountBalance.%s has no currency, so the field never received a value",
				rt.Field(i).Name)
		}
		if money.Scale() != accountMarginScale {
			t.Errorf("AccountBalance.%s scale = %d, want %d", rt.Field(i).Name, money.Scale(), accountMarginScale)
		}
	}
	if seen != wantMoneyFields {
		t.Errorf("AccountBalance has %d Money fields, want %d; this test's currency and scale "+
			"coverage has to move with the struct", seen, wantMoneyFields)
	}
}

// accountAssertPositionTicksAgree asserts the four prices of one position all
// carry the same non-zero tick.
//
// It deliberately does not assert *which* tick: finding F9 in the run plan records
// the 0.001 literal and the spreadLevel/TickSize naming as unresolved, so pinning
// the value would pin a decision this repository has not made. What must hold
// under any resolution is that the four prices of a position agree — a per-field
// tick would silently round one of them differently — and that none is zero,
// because a zero tick disables the step check wherever the price is validated.
func accountAssertPositionTicksAgree(t *testing.T, pos *domain.Position, index, wantPriceFields int) {
	t.Helper()
	v := reflect.ValueOf(*pos)
	rt := v.Type()
	first, firstField := "", ""
	count := 0
	for i := 0; i < rt.NumField(); i++ {
		if rt.Field(i).Type != reflect.TypeOf(domain.Price{}) {
			continue
		}
		count++
		field := rt.Field(i).Name
		tick := v.Field(i).Interface().(domain.Price).Tick().String()
		if firstField == "" {
			first, firstField = tick, field
			// Inverted by P5. This asserted the tick was non-zero, which was
			// true only because every read-path Price was built with the literal
			// "0.001" - a grid the Gateway never promised, and the wrong one for
			// every US instrument and most HK stocks. A read-path price now
			// carries the zero tick, which is the type's way of saying "I
			// observed this price; I do not know this instrument's tick
			// schedule" (design-tick-model.md 2.1). Restoring "0.001" in
			// pkg/domain/account.go fails this line.
			if tick != "0" {
				t.Errorf("Position[%d].%s has tick %s, want the zero tick: a read-path price "+
					"carries no grid, and a non-zero one would let Round() corrupt a value "+
					"the Gateway sent", index, field, tick)
			}
			continue
		}
		if tick != first {
			t.Errorf("Position[%d].%s tick = %s, want %s (the tick %s carries): the prices of "+
				"one position must agree or they round differently", index, field, tick, first, firstField)
		}
	}
	if count != wantPriceFields {
		t.Errorf("Position has %d Price fields, want %d; this test's tick coverage has to "+
			"move with the struct", count, wantPriceFields)
	}
}

// accountMarginScale is the scale domain.AccountBalanceFromDTO declares for every
// Money field.
const accountMarginScale = 3

// accountMarginMoneyFields, accountHoldsPriceFields, and
// accountFundJourValueFields are the per-type field counts the guards above
// assert, so a field added to any of the three domain structs cannot slip past a
// check that only walks the fields the author happened to write down.
const (
	accountMarginMoneyFields = 26
	accountHoldsPriceFields  = 4
)

// ---------------------------------------------------------------------------
// MarginFundInfo
// ---------------------------------------------------------------------------

// TestMarginFundInfoMapsEveryField is the mapper row for the 28-field reply, the
// widest fixture in the package.
//
// The expectation map is built positionally from the fixture's own values, which
// accountAssertJSONTagsMatchFieldNames has already proved aligns with
// domain.AccountBalance. So a mapper that read the wrong source field, or copied
// one source into two destinations, fails here by name.
func TestMarginFundInfoMapsEveryField(t *testing.T) {
	accountAssertAllDistinct(t, "MarginFundInfo", accountMarginValues())
	accountAssertJSONTagsMatchFieldNames(t, domain.MarginFundInfoWire{}, domain.AccountBalance{})
	accountAssertFixtureCoversEveryKey(t, domain.MarginFundInfoWire{},
		accountMarginKeys(), marginFundInfoBody())

	exec := newSequencedExecutor(t, sequencedReply{reply: marginFundInfoBody()})
	bal, err := NewAccountService(exec).MarginFundInfo(t.Context(), accountFixtureID(),
		MarginFundInfoRequest{ExchangeType: types.ExchangeHK})
	if err != nil {
		t.Fatalf("MarginFundInfo: %v", err)
	}
	if bal == nil {
		t.Fatal("MarginFundInfo returned a nil balance beside a nil error")
	}
	accountExpectCall(t, exec, opMarginFundInfo, client.RouteTradeQueryMarginFundInfo)
	requireCalls(t, exec, 1)

	accountAssertEveryField(t, reflect.ValueOf(*bal), "AccountBalance",
		accountExpectationsFor(domain.AccountBalance{}, accountMarginValues()))

	// The three non-HKD currencies are the discriminating part of the mapper: a
	// mutation that attached HKD to a US or CN field would otherwise be invisible,
	// because the amounts are all valid decimals either way.
	for name, want := range map[string]string{
		"BuyPowerUs": "USD", "UnitedBuyPowerUs": "USD", "ThirdBuyPowerUs": "USD",
		"BuyPowerCn": "CNY", "UnitedBuyPowerCn": "CNY", "ThirdBuyPowerCn": "CNY",
		"HoldsBalance": "HKD", "AssetBalance": "HKD", "BuyPowerMoney": "HKD",
	} {
		if got := accountMoneyField(t, bal, name).Currency(); got != want {
			t.Errorf("AccountBalance.%s currency = %q, want %q", name, got, want)
		}
	}
	accountAssertEveryMoneyIsScaled(t, bal, accountMarginMoneyFields)
}

// accountMarginKeys returns the hand-written wire key list in declaration order.
func accountMarginKeys() []string {
	out := make([]string, len(accountMarginFields))
	for i, f := range accountMarginFields {
		out[i] = f.key
	}
	return out
}

// TestMarginFundInfoSendsTheExchangeTypeEvenWhenEmpty pins the wire asymmetry
// account.go:104 creates: marginFundInfoWireRequest has no omitempty, so an empty
// filter still sends the key with an empty value.
//
// holdsListWireRequest one method down *does* carry omitempty, so the two
// endpoints disagree about what an unset market looks like on the wire. That is a
// real difference in the production requests, asserted here so a change to either
// tag is visible rather than silent.
func TestMarginFundInfoSendsTheExchangeTypeEvenWhenEmpty(t *testing.T) {
	for _, tc := range []struct {
		name     string
		exchange types.ExchangeType
		want     string
	}{
		{"unset", "", ""},
		{"hk", types.ExchangeHK, "K"},
		{"us", types.ExchangeUS, "P"},
		{"shenzhen connect", types.ExchangeShenzhenConnect, "v"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := newWireRecorder(map[string]string{
				string(client.RouteTradeQueryMarginFundInfo): gatewaySuccess(string(marginFundInfoBody())),
			})
			svc := NewAccountService(newWireExecutor(t, rec))

			if _, err := svc.MarginFundInfo(t.Context(), accountFixtureID(),
				MarginFundInfoRequest{ExchangeType: tc.exchange}); err != nil {
				t.Fatalf("MarginFundInfo with exchangeType %q: %v", tc.exchange, err)
			}

			params := accountRecordedParams(t, rec, string(client.RouteTradeQueryMarginFundInfo))
			if got := accountStringField(t, params, "exchangeType"); got != tc.want {
				t.Errorf("exchangeType on the wire = %q, want %q", got, tc.want)
			}
			// The key itself is always present: this struct has no omitempty.
			if _, ok := params["exchangeType"]; !ok {
				t.Error("the exchangeType key is absent, but marginFundInfoWireRequest has " +
					"no omitempty, so it must be sent even when the filter is empty")
			}
			if len(params) != 1 {
				t.Errorf("the request carries %v, want exchangeType alone", accountSortedKeys(params))
			}
		})
	}
}

// ---------------------------------------------------------------------------
// HoldsList
// ---------------------------------------------------------------------------

// TestHoldsListMapsTwoPositions is the mapper row for the wrapped position list.
func TestHoldsListMapsTwoPositions(t *testing.T) {
	accountAssertJSONTagsMatchFieldNames(t, domain.HoldsVoWire{}, domain.Position{})

	var decoded struct {
		Rows []json.RawMessage `json:"holdsList"`
	}
	if err := json.Unmarshal(holdsListBody(), &decoded); err != nil {
		t.Fatalf("test bug: holdsListBody is not a JSON object: %v", err)
	}
	if len(decoded.Rows) != len(accountHoldsRows) {
		t.Fatalf("the fixture has %d rows, want %d", len(decoded.Rows), len(accountHoldsRows))
	}
	for i, row := range decoded.Rows {
		accountAssertFixtureCoversEveryKey(t, domain.HoldsVoWire{}, accountHoldsWireKeys, row)
		accountAssertAllDistinct(t, "HoldsList row", accountHoldsFieldValues(accountHoldsRows[i]))
	}

	exec := newSequencedExecutor(t, sequencedReply{reply: holdsListBody()})
	positions, err := NewAccountService(exec).HoldsList(t.Context(), accountFixtureID(),
		HoldsFilter{ExchangeType: types.ExchangeHK})
	if err != nil {
		t.Fatalf("HoldsList: %v", err)
	}
	accountExpectCall(t, exec, opHoldsList, client.RouteTradeQueryHoldsList)
	requireCalls(t, exec, 1)

	if len(positions) != len(accountHoldsRows) {
		t.Fatalf("positions = %d, want %d", len(positions), len(accountHoldsRows))
	}
	for i, pos := range positions {
		accountAssertEveryField(t, reflect.ValueOf(*pos), "Position",
			accountExpectationsFor(domain.Position{}, accountHoldsFieldValues(accountHoldsRows[i])))
		accountAssertPositionTicksAgree(t, pos, i, accountHoldsPriceFields)
	}
}

// TestHoldsListWithNoRows pins the boundary: an empty list is a successful call
// that reports no positions, not an error and not a nil slice.
//
// make([]*domain.Position, 0) is non-nil, so a caller ranging over the result
// behaves the same either way, but a caller comparing against nil does not — and
// the difference is a property of the code, so it is pinned rather than assumed.
func TestHoldsListWithNoRows(t *testing.T) {
	exec := newSequencedExecutor(t, sequencedReply{reply: json.RawMessage(`{"holdsList":[]}`)})
	positions, err := NewAccountService(exec).HoldsList(t.Context(), accountFixtureID(), HoldsFilter{})
	if err != nil {
		t.Fatalf("HoldsList on an empty list = %v, want nil", err)
	}
	if len(positions) != 0 {
		t.Errorf("positions = %d, want 0", len(positions))
	}
	if positions == nil {
		t.Error("positions = nil, want the non-nil empty slice the code returns")
	}
	requireCalls(t, exec, 1)
}

// TestHoldsListOmitsAnEmptyExchangeType is the omitempty row in both directions,
// asserted on the real envelope rather than on a marshalled struct.
//
// The absent direction is the one that matters: holdsListWireRequest is tagged
// omitempty, so an unset market must leave no key at all. A substring search
// could not tell that from a differently-spelled key, which is why
// accountRecordedParams decodes the envelope.
func TestHoldsListOmitsAnEmptyExchangeType(t *testing.T) {
	for _, tc := range []struct {
		name     string
		exchange types.ExchangeType
		wantKey  bool
		want     string
	}{
		{"empty is omitted", "", false, ""},
		{"hk is present", types.ExchangeHK, true, "K"},
		{"us is present", types.ExchangeUS, true, "P"},
		{"shenzhen connect is present", types.ExchangeShenzhenConnect, true, "v"},
		{"shanghai connect is present", types.ExchangeShanghaiConnect, true, "t"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := newWireRecorder(map[string]string{
				string(client.RouteTradeQueryHoldsList): gatewaySuccess(string(holdsListBody())),
			})
			svc := NewAccountService(newWireExecutor(t, rec))

			if _, err := svc.HoldsList(t.Context(), accountFixtureID(),
				HoldsFilter{ExchangeType: tc.exchange}); err != nil {
				t.Fatalf("HoldsList with exchangeType %q: %v", tc.exchange, err)
			}

			path := string(client.RouteTradeQueryHoldsList)
			params := accountRecordedParams(t, rec, path)
			raw, present := params["exchangeType"]
			if present != tc.wantKey {
				t.Fatalf("the exchangeType key is present = %v, want %v (params keys: %v)",
					present, tc.wantKey, accountSortedKeys(params))
			}
			if !tc.wantKey {
				if strings.Contains(rec.lastBody(t, path), "exchangeType") {
					t.Error("the key is absent from the decoded params but the raw body still " +
						"mentions exchangeType, so it is being sent somewhere else")
				}
				if len(params) != 0 {
					t.Errorf("the request carries %v, want no fields at all for an empty filter",
						accountSortedKeys(params))
				}
				return
			}
			if got := accountStringField(t, params, "exchangeType"); got != tc.want {
				t.Errorf("exchangeType on the wire = %q, want %q", got, tc.want)
			}
			if string(raw) != `"`+tc.want+`"` {
				t.Errorf("the raw exchangeType value is %s, want a quoted string", raw)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// The cursor walk: RealFundJourList and HistoryFundJourList
// ---------------------------------------------------------------------------

// accountJourRequest is one row of the table the two fund-journey methods share,
// so every walk behaviour is asserted against both rather than against whichever
// one happened to be written first. They differ only in the op, the route, and
// the two extra date fields.
type accountJourRequest struct {
	name     string
	op       string
	route    client.Route
	hasDates bool
	invoke   func(t *testing.T, ctx context.Context, svc *AccountService,
		filter FundJourFilter, page domain.Pagination) ([]*domain.FundJournalEntry, error)
}

// accountJourRequests are the two cursor-walking methods.
func accountJourRequests() []accountJourRequest {
	return []accountJourRequest{
		{
			name: "RealFundJourList", op: opRealFundJourList,
			route: client.RouteTradeQueryRealFundJourList,
			invoke: func(t *testing.T, ctx context.Context, svc *AccountService,
				filter FundJourFilter, page domain.Pagination,
			) ([]*domain.FundJournalEntry, error) {
				t.Helper()
				return svc.RealFundJourList(ctx, accountFixtureID(), filter, page)
			},
		},
		{
			name: "HistoryFundJourList", op: opHistoryFundJourList,
			route:    client.RouteTradeQueryHistoryFundJourList,
			hasDates: true,
			invoke: func(t *testing.T, ctx context.Context, svc *AccountService,
				filter FundJourFilter, page domain.Pagination,
			) ([]*domain.FundJournalEntry, error) {
				t.Helper()
				return svc.HistoryFundJourList(ctx, accountFixtureID(), filter, page)
			},
		},
	}
}

// accountJourCursors reads the queryParamStr each recorded request carried, in
// order, whichever of the two request structs the method built.
func accountJourCursors(t *testing.T, exec *sequencedExecutor) []string {
	t.Helper()
	var out []string
	for i := 0; i < exec.callCount(); i++ {
		switch p := exec.callAt(t, i).params.(type) {
		case realFundJourListWireRequest:
			out = append(out, p.QueryParamStr)
		case historyFundJourListWireRequest:
			out = append(out, p.QueryParamStr)
		default:
			t.Fatalf("recorded params for call %d = %T, want one of the two fund-journey "+
				"request structs", i, p)
		}
	}
	return out
}

// TestFundJourListChainsTheCursor is the two-page walk: the second request must
// carry the queryParamStr of the *last* row of the first page, and the rows of
// both pages come back in the order they were gathered.
func TestFundJourListChainsTheCursor(t *testing.T) {
	for _, req := range accountJourRequests() {
		t.Run(req.name, func(t *testing.T) {
			exec := newSequencedExecutor(t,
				sequencedReply{reply: fundJourPageBody(
					accountFundJourRow("100.001", "cursor-A"),
					accountFundJourRow("200.002", "cursor-B"),
				)},
				sequencedReply{reply: fundJourPageBody(
					accountFundJourRow("300.003", ""),
				)},
			)

			entries, err := req.invoke(t, t.Context(), NewAccountService(exec), FundJourFilter{},
				domain.Pagination{PageSize: 2})
			if err != nil {
				t.Fatalf("%s: %v", req.name, err)
			}
			accountExpectCall(t, exec, req.op, req.route)
			requireCalls(t, exec, 2)
			if got, want := accountJourCursors(t, exec), []string{"", "cursor-B"}; !reflect.DeepEqual(got, want) {
				t.Errorf("queryParamStr per request = %v, want %v: the second page must start "+
					"from the last row of the first", got, want)
			}

			if len(entries) != 3 {
				t.Fatalf("entries = %d, want 3 across two pages", len(entries))
			}
			for i, want := range []string{"100.001", "200.002", "300.003"} {
				if got := entries[i].BusinessBalance.String(); got != want {
					t.Errorf("entries[%d].BusinessBalance = %q, want %q: the walk must preserve "+
						"page order", i, got, want)
				}
			}

			accountAssertJSONTagsMatchFieldNames(t, domain.FundJourVoWire{}, domain.FundJournalEntry{})
			accountAssertEveryField(t, reflect.ValueOf(*entries[0]), "FundJournalEntry",
				accountExpectationsFor(domain.FundJournalEntry{},
					accountFundJourRowValues("100.001", "cursor-A")))
		})
	}
}

// accountFundJourRowValues returns what accountFundJourRow writes for the given
// amount and cursor, so the mapper assertion reads the same values the fixture
// sent.
func accountFundJourRowValues(businessBalance, cursor string) []string {
	out := append([]string(nil), accountFundJourValues...)
	out[0] = businessBalance
	out[5] = cursor
	return out
}

// TestFundJourNextCursorComesFromTheLastRow isolates the cursor rule.
//
// Only the *middle* row of the three carries a cursor the walk has not seen, so a
// method that returned the first row's cursor, or the first non-empty one, would
// fetch the wrong page. The third row's empty cursor is what ends the walk after
// one request, which is asserted too.
func TestFundJourNextCursorComesFromTheLastRow(t *testing.T) {
	for _, req := range accountJourRequests() {
		t.Run(req.name, func(t *testing.T) {
			exec := newSequencedExecutor(t,
				sequencedReply{reply: fundJourPageBody(
					accountFundJourRow("100.001", ""),
					accountFundJourRow("200.002", "cursor-MIDDLE"),
					accountFundJourRow("300.003", ""),
				)},
			)

			entries, err := req.invoke(t, t.Context(), NewAccountService(exec), FundJourFilter{},
				domain.Pagination{PageSize: 3})
			if err != nil {
				t.Fatalf("%s: %v", req.name, err)
			}
			requireCalls(t, exec, 1)
			if got, want := accountJourCursors(t, exec), []string{""}; !reflect.DeepEqual(got, want) {
				t.Errorf("queryParamStr per request = %v, want %v: the walk must stop on the "+
					"last row's empty cursor, not continue on the middle row's", got, want)
			}
			if len(entries) != 3 {
				t.Fatalf("entries = %d, want 3", len(entries))
			}
			if got := entries[1].QueryParamStr; got != "cursor-MIDDLE" {
				t.Errorf("entries[1].QueryParamStr = %q, want cursor-MIDDLE", got)
			}
		})
	}
}

// TestFundJourWalkStopsOnATerminalShortPage pins the short-page rule: a page
// holding fewer rows than the requested size is not itself a stop condition — the
// empty cursor on its last row is — so a second request is still issued.
func TestFundJourWalkStopsOnATerminalShortPage(t *testing.T) {
	for _, req := range accountJourRequests() {
		t.Run(req.name, func(t *testing.T) {
			exec := newSequencedExecutor(t,
				sequencedReply{reply: fundJourPageBody(accountFundJourRow("100.001", "cursor-A"))},
				sequencedReply{reply: fundJourPageBody(accountFundJourRow("200.002", ""))},
			)

			entries, err := req.invoke(t, t.Context(), NewAccountService(exec), FundJourFilter{},
				domain.Pagination{PageSize: 50})
			if err != nil {
				t.Fatalf("%s: %v", req.name, err)
			}
			requireCalls(t, exec, 2)
			if len(entries) != 2 {
				t.Errorf("entries = %d, want 2: a short page is still a page", len(entries))
			}
		})
	}
}

// TestFundJourWalkStopsOnAnEmptyPage pins the other terminal case: a Gateway
// answering with an empty data array ends the walk after one request, and the
// result is an empty walk rather than an error.
func TestFundJourWalkStopsOnAnEmptyPage(t *testing.T) {
	for _, req := range accountJourRequests() {
		t.Run(req.name, func(t *testing.T) {
			exec := newSequencedExecutor(t, sequencedReply{reply: fundJourPageBody()})

			entries, err := req.invoke(t, t.Context(), NewAccountService(exec), FundJourFilter{},
				domain.Pagination{PageSize: 10})
			if err != nil {
				t.Fatalf("%s on an empty page = %v, want nil", req.name, err)
			}
			accountExpectCall(t, exec, req.op, req.route)
			requireCalls(t, exec, 1)
			if len(entries) != 0 {
				t.Errorf("entries = %d, want 0", len(entries))
			}
			// walkFundJourPages accumulates into a nil slice, so a walk that gathered
			// nothing is nil rather than an allocated empty slice. A caller comparing
			// against nil would see the difference, so it is pinned.
			if entries != nil {
				t.Errorf("entries = %+v, want nil for a walk that gathered nothing", entries)
			}
		})
	}
}

// TestFundJourWalkStopsOnAStalledCursor pins the other stop rule, reached through
// the service rather than only through walkFundJourPages: when the last row echoes
// the cursor the request already carried, the walk ends.
func TestFundJourWalkStopsOnAStalledCursor(t *testing.T) {
	for _, req := range accountJourRequests() {
		t.Run(req.name, func(t *testing.T) {
			exec := newSequencedExecutor(t, sequencedReply{
				reply: fundJourPageBody(accountFundJourRow("100.001", "cursor-A")),
			})

			entries, err := req.invoke(t, t.Context(), NewAccountService(exec), FundJourFilter{},
				domain.Pagination{Cursor: "cursor-A", PageSize: 10})
			if err != nil {
				t.Fatalf("%s: %v", req.name, err)
			}
			requireCalls(t, exec, 1)
			if got, want := accountJourCursors(t, exec), []string{"cursor-A"}; !reflect.DeepEqual(got, want) {
				t.Errorf("queryParamStr per request = %v, want %v", got, want)
			}
			if len(entries) != 1 {
				t.Errorf("entries = %d, want the one row of the stalled page", len(entries))
			}
		})
	}
}

// TestFundJourForwardsTheCallersCursor pins the pass-through: a caller-supplied
// cursor is the queryParamStr of the first request, not something the walk
// rewrites.
//
// The single scripted row carries an empty cursor, so the walk ends after one
// request. Were it to carry anything else the walk would fetch a second page and
// exhaust the fixture, which is a failure in its own right.
func TestFundJourForwardsTheCallersCursor(t *testing.T) {
	for _, req := range accountJourRequests() {
		t.Run(req.name, func(t *testing.T) {
			exec := newSequencedExecutor(t, sequencedReply{
				reply: fundJourPageBody(accountFundJourRow("100.001", "")),
			})

			if _, err := req.invoke(t, t.Context(), NewAccountService(exec), FundJourFilter{},
				domain.Pagination{Cursor: "resume-here", PageSize: 5}); err != nil {
				t.Fatalf("%s: %v", req.name, err)
			}
			requireCalls(t, exec, 1)
			if got, want := accountJourCursors(t, exec), []string{"resume-here"}; !reflect.DeepEqual(got, want) {
				t.Errorf("queryParamStr = %v, want %v: the caller's cursor must reach the wire",
					got, want)
			}
		})
	}
}

// TestFundJourPageSizeIsClampedThroughTheWire is the clamp matrix read off the
// real request body, so it is the value the Gateway would see rather than the
// value the code holds.
//
// clampPageSize itself is already covered by account_test.go; what is new here is
// that the clamped value is the one that reaches queryCount, and that the
// non-positive rows still produce a well-formed request rather than a zero.
func TestFundJourPageSizeIsClampedThroughTheWire(t *testing.T) {
	for _, req := range accountJourRequests() {
		t.Run(req.name, func(t *testing.T) {
			for _, tc := range []struct{ in, want int }{
				{0, DefaultPageSize},
				{-5, DefaultPageSize},
				{7, 7},
				{DefaultPageSize, DefaultPageSize},
				{MaxPageSize, MaxPageSize},
				{MaxPageSize + 1, MaxPageSize},
				{100000, MaxPageSize},
			} {
				t.Run(strconv.Itoa(tc.in), func(t *testing.T) {
					rec := newWireRecorder(map[string]string{
						string(req.route): gatewaySuccess(string(fundJourPageBody())),
					})
					svc := NewAccountService(newWireExecutor(t, rec))

					if _, err := req.invoke(t, t.Context(), svc, FundJourFilter{},
						domain.Pagination{PageSize: tc.in}); err != nil {
						t.Fatalf("%s with PageSize %d: %v", req.name, tc.in, err)
					}
					params := accountRecordedParams(t, rec, string(req.route))
					if got := accountIntField(t, params, "queryCount"); got != tc.want {
						t.Errorf("queryCount for PageSize %d = %d, want %d", tc.in, got, tc.want)
					}
					if got := accountStringField(t, params, "queryParamStr"); got != "" {
						t.Errorf("queryParamStr = %q, want the empty string for the first page", got)
					}
					// The dated request carries the two keys the undated one does not,
					// and both are untagged. This is the on-the-wire half of the
					// hasDates flag, so the difference is visible from the body alone.
					_, hasStart := params["startDate"]
					if hasStart != req.hasDates {
						t.Errorf("the request carries startDate = %v, want %v: %s and %s are "+
							"documented as sending different request bodies",
							hasStart, req.hasDates, req.name, "HistoryFundJourList")
					}
				})
			}
		})
	}
}

// TestHistoryFundJourListSendsTheDateRangeOnEveryPage is the only reason
// HistoryFundJourList sends a different request from RealFundJourList, so the two
// keys are pinned on the wire for both pages of a walk.
func TestHistoryFundJourListSendsTheDateRangeOnEveryPage(t *testing.T) {
	rec := newWireRecorder(map[string]string{
		string(client.RouteTradeQueryHistoryFundJourList): gatewaySuccess(
			string(fundJourPageBody(accountFundJourRow("100.001", "cursor-A")))),
	})
	svc := NewAccountService(newWireExecutor(t, rec))

	entries, err := svc.HistoryFundJourList(t.Context(), accountFixtureID(),
		FundJourFilter{ExchangeType: types.ExchangeHK, StartDate: "20260101", EndDate: "20260926"},
		domain.Pagination{PageSize: 1})
	if err != nil {
		t.Fatalf("HistoryFundJourList: %v", err)
	}
	path := string(client.RouteTradeQueryHistoryFundJourList)
	if got := rec.count(path); got != 2 {
		t.Fatalf("requests = %d, want 2: the walk must reach a second page", got)
	}
	// The recorder answers both requests from one table entry, so the second page
	// echoes the first page's cursor, which stalls the walk. Two entries and two
	// requests is therefore the expected end state, and it is what makes the
	// body below the *second* page's.
	if len(entries) != 2 {
		t.Fatalf("entries = %d, want 2: a stalled second page still contributes its row", len(entries))
	}
	// The *last* body is the second page's. Both pages are built from the same
	// filter inside the closure, so asserting the last one asserts the loop
	// invariant; the first page's own body is checked through the recorded struct
	// in TestTheFundJourRequestShapesDiffer.
	params := accountRecordedParams(t, rec, path)
	if got := accountStringField(t, params, "startDate"); got != "20260101" {
		t.Errorf("startDate on the second page = %q, want 20260101", got)
	}
	if got := accountStringField(t, params, "endDate"); got != "20260926" {
		t.Errorf("endDate on the second page = %q, want 20260926", got)
	}
	if got := accountStringField(t, params, "exchangeType"); got != "K" {
		t.Errorf("exchangeType on the second page = %q, want K", got)
	}
	if got := accountIntField(t, params, "queryCount"); got != 1 {
		t.Errorf("queryCount on the second page = %d, want 1: the page size is not consumed "+
			"by the first page", got)
	}
}

// TestTheFundJourRequestShapesDiffer pins the difference between the two request
// structs from the recorded params, so the dates cannot be dropped from
// HistoryFundJourList or invented for RealFundJourList.
func TestTheFundJourRequestShapesDiffer(t *testing.T) {
	filter := FundJourFilter{ExchangeType: types.ExchangeHK, StartDate: "20260101", EndDate: "20260926"}

	realExec := newSequencedExecutor(t, sequencedReply{reply: fundJourPageBody()})
	if _, err := NewAccountService(realExec).RealFundJourList(t.Context(), accountFixtureID(),
		filter, domain.Pagination{PageSize: 5}); err != nil {
		t.Fatalf("RealFundJourList: %v", err)
	}
	real := accountParamsAs[realFundJourListWireRequest](t, realExec)
	if real.ExchangeType != types.ExchangeHK || real.QueryCount != 5 || real.QueryParamStr != "" {
		t.Errorf("realFundJourListWireRequest = %+v, want the exchange, the clamped size, and "+
			"no cursor", real)
	}

	histExec := newSequencedExecutor(t, sequencedReply{reply: fundJourPageBody()})
	if _, err := NewAccountService(histExec).HistoryFundJourList(t.Context(), accountFixtureID(),
		filter, domain.Pagination{PageSize: 5}); err != nil {
		t.Fatalf("HistoryFundJourList: %v", err)
	}
	hist := accountParamsAs[historyFundJourListWireRequest](t, histExec)
	if hist.StartDate != "20260101" || hist.EndDate != "20260926" {
		t.Errorf("historyFundJourListWireRequest dates = (%q, %q), want (20260101, 20260926)",
			hist.StartDate, hist.EndDate)
	}
	if hist.ExchangeType != types.ExchangeHK || hist.QueryCount != 5 || hist.QueryParamStr != "" {
		t.Errorf("historyFundJourListWireRequest = %+v, want the same base fields as the real one",
			hist)
	}

	// Neither date field is tagged omitempty, so an unset range is still sent as
	// two empty strings, which is how the Gateway is told "no range".
	emptyExec := newSequencedExecutor(t, sequencedReply{reply: fundJourPageBody()})
	if _, err := NewAccountService(emptyExec).HistoryFundJourList(t.Context(), accountFixtureID(),
		FundJourFilter{}, domain.Pagination{}); err != nil {
		t.Fatalf("HistoryFundJourList with no filter: %v", err)
	}
	empty := accountParamsAs[historyFundJourListWireRequest](t, emptyExec)
	if empty.StartDate != "" || empty.EndDate != "" {
		t.Errorf("the dates with an empty filter = (%q, %q), want two empty strings",
			empty.StartDate, empty.EndDate)
	}
	if empty.QueryCount != DefaultPageSize {
		t.Errorf("QueryCount with an unset page size = %d, want the default %d",
			empty.QueryCount, DefaultPageSize)
	}
}

// mustMarshalJSON renders a value, failing the test rather than the run.
func mustMarshalJSON(t *testing.T, v any) string {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("test bug: json.Marshal(%T) failed: %v", v, err)
	}
	return string(raw)
}

// ---------------------------------------------------------------------------
// RateQueryList
// ---------------------------------------------------------------------------

// TestRateQueryListReturnsEveryPairAsAKeyedSet is the order-independent row.
//
// The fixture holds three sources, five target pairs, and one source with no
// targets. The assertion sorts before comparing, because the method ranges over a
// map (account.go:280-285) and Go randomises that order — see accountSortedRates
// for the full reason. Asserting positionally here would flake on most runs;
// asserting only the count would let a dropped or duplicated row through.
func TestRateQueryListReturnsEveryPairAsAKeyedSet(t *testing.T) {
	exec := newSequencedExecutor(t, sequencedReply{reply: json.RawMessage(accountRateFixtures)})
	rates, err := NewAccountService(exec).RateQueryList(t.Context())
	if err != nil {
		t.Fatalf("RateQueryList: %v", err)
	}
	accountExpectCall(t, exec, opRateQueryList, client.RouteHsRateQueryList)
	requireCalls(t, exec, 1)

	sorted := accountSortedRates(rates)
	if len(sorted) != len(accountWantRates) {
		t.Fatalf("rates = %d, want %d: three sources contribute 2, 2 and 1 pairs, and the "+
			"empty source contributes none", len(sorted), len(accountWantRates))
	}
	for i, want := range accountWantRates {
		got := sorted[i]
		if got.SourceCurrency != want.source || got.TargetCurrency != want.target {
			t.Fatalf("rates[%d] = %s/%s, want %s/%s (the comparison is over the sorted "+
				"result, so a mismatch is a content error, not an order one)",
				i, got.SourceCurrency, got.TargetCurrency, want.source, want.target)
		}
		if got.Rate.String() != want.rate {
			t.Errorf("rates[%d].Rate (%s/%s) = %q, want %q", i, want.source, want.target,
				got.Rate.String(), want.rate)
		}
	}
	// The empty source must not appear at all, in any position.
	for _, r := range sorted {
		if r.SourceCurrency == "GBP" {
			t.Errorf("a source with no targets produced a rate: %s/%s",
				r.SourceCurrency, r.TargetCurrency)
		}
	}
}

// TestRateQueryListIsOrderIndependent runs the same fixture repeatedly and
// asserts the sorted result is identical every time.
//
// It cannot prove the order is random — nothing can, since a run that happens to
// iterate in order is indistinguishable from a deterministic one — so it does not
// claim to. What it proves is that the *contents* are stable across repeated map
// iterations, which is the half a caller may rely on and the reason the
// assertions above may sort.
func TestRateQueryListIsOrderIndependent(t *testing.T) {
	read := func() []accountRatePair {
		t.Helper()
		exec := newSequencedExecutor(t, sequencedReply{reply: json.RawMessage(accountRateFixtures)})
		rates, err := NewAccountService(exec).RateQueryList(t.Context())
		if err != nil {
			t.Fatalf("RateQueryList: %v", err)
		}
		out := make([]accountRatePair, 0, len(rates))
		for _, r := range accountSortedRates(rates) {
			out = append(out, accountRatePair{r.SourceCurrency, r.TargetCurrency, r.Rate.String()})
		}
		return out
	}

	want := read()
	if !reflect.DeepEqual(want, accountWantRates) {
		t.Fatalf("the first read = %v, want %v", want, accountWantRates)
	}
	for i := 0; i < 32; i++ {
		if got := read(); !reflect.DeepEqual(got, want) {
			t.Fatalf("read %d = %v, want %v: the sorted contents must not vary between runs",
				i, got, want)
		}
	}
}

// TestRateQueryListWithAnEmptyMap pins the boundary: an empty rate map is a
// successful call that reports no rates.
//
// The result is nil rather than an allocated empty slice, because the method
// declares `var rates []*domain.InterestRate` and only ever appends. That is a
// property of the code, so it is pinned rather than assumed.
func TestRateQueryListWithAnEmptyMap(t *testing.T) {
	exec := newSequencedExecutor(t, sequencedReply{reply: json.RawMessage(`{}`)})
	rates, err := NewAccountService(exec).RateQueryList(t.Context())
	if err != nil {
		t.Fatalf("RateQueryList on an empty map = %v, want nil", err)
	}
	accountExpectCall(t, exec, opRateQueryList, client.RouteHsRateQueryList)
	requireCalls(t, exec, 1)
	if len(rates) != 0 {
		t.Errorf("rates = %d, want 0", len(rates))
	}
	if rates != nil {
		t.Errorf("rates = %+v, want nil for an empty map", rates)
	}
}

// TestRateQueryListIsTheOnlyMethodWithNoValidationOrArguments pins the actual
// signature and behaviour of the one method of the five that takes no accountID
// and validates nothing.
//
// It is a test of what the code does, not of what it ought to do: there is no
// zero-accountID arm here, so no row pretends one exists, and the method must
// keep working with nothing but a context.
//
// It also pins the consequence on the wire. rateQueryListWireRequest is tagged
// omitempty and the method always builds it empty — there is no parameter through
// which a rateType could be set — so the key must be absent. The present-when-set
// direction is asserted at the struct level in
// TestTheOmitEmptyRequestTagsAreReal, which is the only place it is reachable
// without a production change.
func TestRateQueryListIsTheOnlyMethodWithNoValidationOrArguments(t *testing.T) {
	rec := newWireRecorder(map[string]string{
		string(client.RouteHsRateQueryList): gatewaySuccess(accountRateFixtures),
	})
	svc := NewAccountService(newWireExecutor(t, rec))

	rates, err := svc.RateQueryList(t.Context())
	if err != nil {
		t.Fatalf("RateQueryList: %v", err)
	}
	if len(rates) != len(accountWantRates) {
		t.Errorf("rates = %d, want %d", len(rates), len(accountWantRates))
	}
	path := string(client.RouteHsRateQueryList)
	if got := rec.count(path); got != 1 {
		t.Errorf("requests = %d, want 1: this method has no validation gate, so it always asks", got)
	}

	params := accountRecordedParams(t, rec, path)
	if _, present := params["rateType"]; present {
		t.Error("the request carries a rateType key, but the method builds an empty " +
			"rateQueryListWireRequest and the field is omitempty, so it must be absent")
	}
	if len(params) != 0 {
		t.Errorf("the request carries %v, want no fields at all", accountSortedKeys(params))
	}
}

// TestTheOmitEmptyRequestTagsAreReal is the struct-level half of the omitempty
// proof: the key is present when the field is set and absent when it is not, for
// both tagged request structs.
//
// Through the public API only the absent direction is reachable for rateType,
// because RateQueryList takes no request. Marshalling the production request
// structs directly closes that half without touching production code, and a future
// edit that drops either omitempty tag fails here.
func TestTheOmitEmptyRequestTagsAreReal(t *testing.T) {
	for _, tc := range []struct {
		name      string
		empty     any
		populated any
		key       string
		want      string
	}{
		{
			name:      "holdsListWireRequest.ExchangeType",
			empty:     holdsListWireRequest{},
			populated: holdsListWireRequest{ExchangeType: types.ExchangeHK},
			key:       "exchangeType", want: `"K"`,
		},
		{
			name:      "rateQueryListWireRequest.RateType",
			empty:     rateQueryListWireRequest{},
			populated: rateQueryListWireRequest{RateType: "MARGIN"},
			key:       "rateType", want: `"MARGIN"`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := mustMarshalJSON(t, tc.empty); strings.Contains(got, tc.key) {
				t.Errorf("marshalling %s = %s, want the key %q omitted entirely", tc.name, got, tc.key)
			}
			want := `"` + tc.key + `":` + tc.want
			if got := mustMarshalJSON(t, tc.populated); !strings.Contains(got, want) {
				t.Errorf("marshalling %s = %s, want it to carry %s", tc.name, got, want)
			}
		})
	}

	// The control: the untagged request fields must always be present, so the two
	// omissions above are a property of the tags and not of a zero-valued struct
	// rendering as {}.
	for _, tc := range []struct {
		name  string
		value any
		key   string
	}{
		{"marginFundInfoWireRequest.ExchangeType", marginFundInfoWireRequest{}, "exchangeType"},
		{"realFundJourListWireRequest.QueryParamStr", realFundJourListWireRequest{}, "queryParamStr"},
		{"realFundJourListWireRequest.QueryCount", realFundJourListWireRequest{}, "queryCount"},
		{"historyFundJourListWireRequest.StartDate", historyFundJourListWireRequest{}, "startDate"},
		{"historyFundJourListWireRequest.EndDate", historyFundJourListWireRequest{}, "endDate"},
	} {
		t.Run("untagged/"+tc.name, func(t *testing.T) {
			if got := mustMarshalJSON(t, tc.value); !strings.Contains(got, `"`+tc.key+`"`) {
				t.Errorf("marshalling %s = %s, want the untagged key %q present", tc.name, got, tc.key)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// The op and route table for all five
// ---------------------------------------------------------------------------

// TestAccountMethodsUseTheirDocumentedOpAndRoute is the routing row for the
// group, and the assertion the plan's step 3 asks for: every method reaches the
// endpoint docs/SPEC.md lists, under the op string its own typed errors carry.
//
// It matters beyond coverage. A caller that branches on Op to decide whether to
// re-login, or on the route for metrics, is misinformed by a method that drifted
// onto another endpoint — and the *error* path carries the op, so a drift would
// also mislabel every rejection.
func TestAccountMethodsUseTheirDocumentedOpAndRoute(t *testing.T) {
	for _, tc := range []struct {
		name  string
		op    string
		route client.Route
		reply json.RawMessage
		run   func(t *testing.T, svc *AccountService)
	}{
		{
			name: "MarginFundInfo", op: opMarginFundInfo,
			route: client.RouteTradeQueryMarginFundInfo, reply: marginFundInfoBody(),
			run: func(t *testing.T, svc *AccountService) {
				if _, err := svc.MarginFundInfo(t.Context(), accountFixtureID(),
					MarginFundInfoRequest{}); err != nil {
					t.Fatalf("MarginFundInfo: %v", err)
				}
			},
		},
		{
			name: "HoldsList", op: opHoldsList,
			route: client.RouteTradeQueryHoldsList, reply: holdsListBody(),
			run: func(t *testing.T, svc *AccountService) {
				if _, err := svc.HoldsList(t.Context(), accountFixtureID(),
					HoldsFilter{}); err != nil {
					t.Fatalf("HoldsList: %v", err)
				}
			},
		},
		{
			name: "RealFundJourList", op: opRealFundJourList,
			route: client.RouteTradeQueryRealFundJourList, reply: fundJourPageBody(),
			run: func(t *testing.T, svc *AccountService) {
				if _, err := svc.RealFundJourList(t.Context(), accountFixtureID(),
					FundJourFilter{}, domain.Pagination{}); err != nil {
					t.Fatalf("RealFundJourList: %v", err)
				}
			},
		},
		{
			name: "HistoryFundJourList", op: opHistoryFundJourList,
			route: client.RouteTradeQueryHistoryFundJourList, reply: fundJourPageBody(),
			run: func(t *testing.T, svc *AccountService) {
				if _, err := svc.HistoryFundJourList(t.Context(), accountFixtureID(),
					FundJourFilter{}, domain.Pagination{}); err != nil {
					t.Fatalf("HistoryFundJourList: %v", err)
				}
			},
		},
		{
			name: "RateQueryList", op: opRateQueryList,
			route: client.RouteHsRateQueryList, reply: json.RawMessage(accountRateFixtures),
			run: func(t *testing.T, svc *AccountService) {
				if _, err := svc.RateQueryList(t.Context()); err != nil {
					t.Fatalf("RateQueryList: %v", err)
				}
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			exec := newSequencedExecutor(t, sequencedReply{reply: tc.reply})
			tc.run(t, NewAccountService(exec))
			accountExpectCall(t, exec, tc.op, tc.route)
			requireCalls(t, exec, 1)
		})
	}
}
