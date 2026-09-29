// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package services

import (
	"context"
	"testing"

	"github.com/shing1211/hstongapi4go/pkg/domain"
	"github.com/shing1211/hstongapi4go/pkg/types"
)

// fixedSchedule is a TickScheduleResolver that answers from a fixed table, so a
// test can say what the instrument's grid is without depending on the default
// table the production code ships.
type fixedSchedule struct {
	lot  uint64
	tick string
	ok   bool
	// calls records every symbol asked about, so a test can prove the resolver
	// was consulted rather than bypassed.
	calls []domain.Symbol
}

func (f *fixedSchedule) ScheduleFor(sym domain.Symbol) (uint64, string, bool) {
	f.calls = append(f.calls, sym)
	return f.lot, f.tick, f.ok
}

func hkStockSymbol() domain.Symbol {
	return domain.Symbol{
		Market:   domain.MarketHK,
		Code:     "00700",
		DataType: types.DataTypeHKStock,
		FullCode: "00700.HK",
	}
}

// TestTheInstrumentTickGovernsNotTheCallers is the regression test for the whole
// TickSchedule change.
//
// Before it, validatePriceForHK asked the schedule only whether a tick existed
// and then called Price.Validate(true), which tests a price against the tick the
// caller attached to it. A caller could therefore satisfy the check by choosing a
// tick that divides their price, whatever the instrument's real grid was — the
// check could not fail for a reason the caller had not already accepted. The
// schedule's answer was consulted and then thrown away.
//
// The row below is that defect, written down: a price four decimal places deep,
// on an instrument whose tick is three. It is accepted by the old code and must
// be refused by the current one.
func TestTheInstrumentTickGovernsNotTheCallers(t *testing.T) {
	stock := hkStockSymbol()
	const fourDecimals = "388.0501" // not a multiple of the HK stock tick 0.001

	// The caller declares a tick that divides the price, which is what used to
	// make the check pass.
	price := domain.MustNewPrice(fourDecimals, "0.0001")

	err := validatePriceForHK(price, domain.TickSchedule{}, stock)
	if err == nil {
		t.Fatal("a price declaring its own 0.0001 tick was accepted for an HK stock whose " +
			"tick is 0.001: the check is still running against the caller's number")
	}
	assertInvalidParam(t, err, opEntrust)
}

// TestTheDeclaredTickNoLongerWidensTheGrid is the complement: a price that is on
// the instrument's grid must be accepted whatever tick it declares.
//
// Without this, "reject a finer declared tick" could be satisfied by rejecting
// every price, which the invalid-param path would not distinguish from the right
// answer.
func TestTheDeclaredTickNoLongerWidensTheGrid(t *testing.T) {
	stock := hkStockSymbol()

	// Declares 0.01, a coarser grid than the stock's 0.001, but 388.05 is on both.
	coarser := domain.MustNewPrice("388.05", "0.01")
	if err := validatePriceForHK(coarser, domain.TickSchedule{}, stock); err != nil {
		t.Errorf("a price on the stock grid was rejected because it declared 0.01: %v. "+
			"The instrument's tick decides the step; the declared tick is not consulted", err)
	}
}

// TestAnInjectedScheduleIsActuallyConsulted proves the injection point is live
// rather than decorative: a resolver that reports a grid the default table does
// not have must change the outcome.
func TestAnInjectedScheduleIsActuallyConsulted(t *testing.T) {
	stock := hkStockSymbol()

	// Four decimals: refused under the stock's 0.001, accepted under 0.0001.
	price := domain.MustNewPrice("388.0501", "0.0001")

	fine := &fixedSchedule{lot: 100, tick: "0.0001", ok: true}
	if err := validatePriceForHK(price, fine, stock); err != nil {
		t.Errorf("price rejected under an injected 0.0001 grid: %v. The resolver was not "+
			"consulted, or its tick was discarded", err)
	}
	if len(fine.calls) != 1 {
		t.Errorf("resolver consulted %d times, want 1", len(fine.calls))
	}

	coarse := &fixedSchedule{lot: 1, tick: "0.01", ok: true}
	if err := validatePriceForHK(price, coarse, stock); err == nil {
		t.Error("price accepted under an injected 0.01 grid, want rejected: 388.0501 is not " +
			"a multiple of 0.01")
	}
}

// TestAResolverThatHasNoOpinionSkipsTheCheck covers ok false.
//
// An absent grid must read as unknown, not as a default. Substituting a number
// here is the original defect, so the assertion is that a negative price — which
// every other check rejects — passes when the schedule declines to answer.
func TestAResolverThatHasNoOpinionSkipsTheCheck(t *testing.T) {
	stock := hkStockSymbol()
	silent := &fixedSchedule{ok: false}

	if err := validatePriceForHK(domain.MustNewPrice("-1", "0.001"), silent, stock); err != nil {
		t.Errorf("a negative price was rejected although the schedule has no opinion: %v. "+
			"ok false must skip the check rather than invent a grid", err)
	}
	if err := validatePriceForHK(domain.MustNewPrice("388.0501", "0.0001"), silent, stock); err != nil {
		t.Errorf("an off-grid price was rejected although the schedule has no opinion: %v", err)
	}

	// ok false must be honoured on its own, not inferred from the tick being
	// empty. A resolver that reports no opinion *and* leaves a value behind — a
	// stale cache, a partially populated master — must still be believed, or the
	// ok flag is decorative and a caller can be validated against a grid the
	// resolver disowned.
	//
	// This row is the one that kills replacing `if !ok` with `if tick == ""`, and
	// the value is chosen to make that possible: 388.05015 is NOT a multiple of
	// the disowned 0.0001 tick, so a helper that checks `tick == ""` instead of
	// `!ok` rejects it, while the correct helper skips the check and accepts it. A
	// value that happened to sit on the disowned grid would pass either way and
	// prove nothing — which is exactly the first version of this row's bug.
	const offTheDisownedTick = "388.05015"

	disowns := &fixedSchedule{lot: 100, tick: "0.0001", ok: false}
	if err := validatePriceForHK(domain.MustNewPrice(offTheDisownedTick, "0"), disowns, stock); err != nil {
		t.Errorf("a price was validated against a tick the resolver disowned (ok false, tick %q): "+
			"%v. The ok flag is the contract; a non-empty tick does not override it", disowns.tick, err)
	}

	// The mirror row, to prove the value is genuinely off that grid rather than
	// being accepted for some unrelated reason. With ok true it must be refused.
	if err := validatePriceForHK(domain.MustNewPrice(offTheDisownedTick, "0"),
		&fixedSchedule{lot: 100, tick: "0.0001", ok: true}, stock); err == nil {
		t.Error("a price that is not a multiple of 0.0001 was accepted when the resolver " +
			"claimed that grid: the value above cannot discriminate unless it is off-grid")
	}

	// And the lot side obeys the same rule, or a disowned lot would silently
	// reject a quantity. 150 is not a multiple of 100, so a helper checking the
	// lot despite ok false would refuse it.
	lotDisowns := &fixedSchedule{lot: 100, tick: "0.001", ok: false}
	if err := validateQuantityForHK(domain.MustNewQuantity("150"), lotDisowns, stock); err != nil {
		t.Errorf("quantity 150 was checked against a lot the resolver disowned: %v", err)
	}
}

// TestTheLotCheckStillUsesTheInjectedLot guards the half of the rewire that was
// already working. validateQuantityForHK validated against schedule.Lot before
// this change, so the interface must carry lot through rather than quietly
// defaulting it to zero — a zero lot would make ValidateLot pass for everything.
func TestTheLotCheckStillUsesTheInjectedLot(t *testing.T) {
	stock := hkStockSymbol()

	hundred := &fixedSchedule{lot: 100, tick: "0.001", ok: true}
	if err := validateQuantityForHK(domain.MustNewQuantity("150"), hundred, stock); err == nil {
		t.Error("quantity 150 accepted under lot 100, want rejected")
	}
	if err := validateQuantityForHK(domain.MustNewQuantity("200"), hundred, stock); err != nil {
		t.Errorf("quantity 200 rejected under lot 100: %v", err)
	}

	// A lot of 1 admits odd quantities, so the same 150 is fine there. If the
	// injected lot were being ignored, this row would fail too.
	anyone := &fixedSchedule{lot: 1, tick: "0.001", ok: true}
	if err := validateQuantityForHK(domain.MustNewQuantity("150"), anyone, stock); err != nil {
		t.Errorf("quantity 150 rejected under lot 1: %v. The injected lot is not being used", err)
	}
}

// TestANilResolverFallsBackToTheDefaultTable is the no-behaviour-change
// guarantee. A caller who never injects a schedule must get exactly what it got
// before the interface existed, which is what makes this change safe to ship on
// its own.
func TestANilResolverFallsBackToTheDefaultTable(t *testing.T) {
	svc := NewTradingService(nil)

	if got := svc.tickSchedule(); got == nil {
		t.Fatal("tickSchedule() = nil for a service with no injected resolver, want the default")
	}

	stock := hkStockSymbol()
	// The default table says 0.001 for an HK stock, so four decimals is refused
	// exactly as it is with the concrete value passed directly.
	price := domain.MustNewPrice("388.0501", "0.0001")
	viaNil := validatePriceForHK(price, svc.tickSchedule(), stock)
	viaDefault := validatePriceForHK(price, domain.TickSchedule{}, stock)

	if (viaNil == nil) != (viaDefault == nil) {
		t.Errorf("nil-resolver outcome (%v) differs from the default table's (%v): a caller "+
			"that injects nothing must see unchanged behaviour", viaNil, viaDefault)
	}
}

// TestWithTickScheduleIgnoresNil keeps a nil argument from becoming a nil
// dereference on the first order. An option that stored an untyped nil would
// leave a non-nil interface holding no resolver, and tickSchedule's nil check
// would not see it.
func TestWithTickScheduleIgnoresNil(t *testing.T) {
	svc := NewTradingService(nil, WithTickSchedule(nil))

	got := svc.tickSchedule()
	if got == nil {
		t.Fatal("tickSchedule() = nil after WithTickSchedule(nil), want the default table")
	}
	// And it must actually work, not merely be non-nil.
	if err := validatePriceForHK(domain.MustNewPrice("388.0501", "0.0001"), got, hkStockSymbol()); err == nil {
		t.Error("a nil resolver disabled the price check: an off-grid price was accepted")
	}
}

// TestTheDefaultResolverDeclinesNonHKSymbols pins the market guard on the
// default table. Its DataType-keyed switch would otherwise answer for a US symbol
// whose DataType happened to collide with an HK one, which is the same class of
// error as the dtype-vs-instrument confusion the interface exists to remove.
func TestTheDefaultResolverDeclinesNonHKSymbols(t *testing.T) {
	schedule := domain.TickSchedule{}

	us := domain.Symbol{Market: domain.MarketUS, DataType: types.DataTypeHKStock}
	if _, _, ok := schedule.ScheduleFor(us); ok {
		t.Error("the default table answered for a US symbol carrying an HK DataType: " +
			"it is keyed on DataType, so the market must be checked too")
	}

	// An HK index has no entry, so it is declined for the ordinary reason.
	index := domain.Symbol{Market: domain.MarketHK, DataType: types.DataTypeHKIndex}
	if _, _, ok := schedule.ScheduleFor(index); ok {
		t.Error("the default table claimed an HK index, whose TickSize is empty")
	}

	// And the ordinary case still answers.
	if _, tick, ok := schedule.ScheduleFor(hkStockSymbol()); !ok || tick != "0.001" {
		t.Errorf("ScheduleFor(HK stock) = tick %q ok %v, want 0.001 true", tick, ok)
	}
}

// TestEntrustRejectsAnOffGridPriceThroughTheService walks the production entry
// point rather than the helper, so the wiring from Entrust to the resolver is
// covered and not just the helper in isolation.
func TestEntrustRejectsAnOffGridPriceThroughTheService(t *testing.T) {
	svc := NewTradingService(nil)
	ctx := context.Background()

	order := domain.Order{
		Symbol:      hkStockSymbol(),
		Side:        types.EntrustBuy,
		Price:       domain.MustNewPrice("388.0501", "0.0001"),
		Quantity:    domain.MustNewQuantity("200"),
		OrderType:   types.EntrustTypeLimit,
		TimeInForce: "DAY",
	}

	// No client is injected, so a nil Executor would panic if validation passed.
	// Reaching the nil dereference would mean the price check did not run.
	_, err := svc.Entrust(ctx, domain.AccountID("acct"), order)
	if err == nil {
		t.Fatal("Entrust accepted an off-grid price")
	}
	assertInvalidParam(t, err, opEntrust)
}
