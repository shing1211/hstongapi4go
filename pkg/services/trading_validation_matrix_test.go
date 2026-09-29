// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package services

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/shing1211/hstongapi4go/pkg/domain"
	"github.com/shing1211/hstongapi4go/pkg/types"
)

// This file covers the validation surface of trading.go: the nine unexported
// helpers and the thirteen methods that consult them before spending a round
// trip.
//
// The shape follows pkg/hstong/algo/validation_matrix_test.go for a reason B1
// measured: every helper here returns on the first failure, so a suite that
// starts from one fixture and mutates a single sampled field can only ever reach
// the branches it happens to name. Each helper therefore gets a known-valid
// fixture, a reject table that changes exactly one field, and an accept table, so
// a rejection row can never be attributed to a defect in the fixture.
//
// One constraint shapes several tests below. Every rejection in this file is the
// same shape — StatusInvalidParam, CategoryAPI, Op == "trade/TradeEntrust" — so
// the typed surface cannot say which of two checks fired. Ordering between two
// validators is therefore not asserted at all. What is asserted, per row, is that
// the row's single fault is caught: the other four checks are given valid values,
// so deleting the check under test turns that row into a nil error and fails it.
// Where a *differential* pair can be built — a value accepted by one path and
// refused by the other, with nothing else changed — the pair is used instead,
// because that does distinguish the two checks. Both techniques are sound; the
// ordinal one is not, and it is not used.

// ---------------------------------------------------------------------------
// validateQuantityForHK
// ---------------------------------------------------------------------------

// TestValidateQuantityForHKMatrix walks every entry in the HK tick schedule,
// because the helper delegates its whole decision to that table and the table is
// the real subject. A dtype whose schedule has no lot is "unknown", not
// "rejected", and the two are asserted as different outcomes rather than as one.
func TestValidateQuantityForHKMatrix(t *testing.T) {
	for _, tc := range []struct {
		name    string
		dtype   types.DataType
		lot     uint64
		tick    string
		qty     string
		wantErr bool
		why     string
	}{
		{"HK stock exact lot", types.DataTypeHKStock, 100, "0.001", "100", false, "a multiple of 100"},
		{"HK stock zero", types.DataTypeHKStock, 100, "0.001", "0", false, "zero is a legal multiple; pinned as current behaviour"},
		{"HK stock not a multiple", types.DataTypeHKStock, 100, "0.001", "150", true, "150 mod 100 is 50"},
		{"HK stock not an integer", types.DataTypeHKStock, 100, "0.001", "100.5", true, "the integer check runs first"},
		{"HK stock negative multiple", types.DataTypeHKStock, 100, "0.001", "-100", false, "no check rejects a negative quantity; recorded, not claimed correct"},

		{"HK ETF two lots", types.DataTypeHKETF, 100, "0.001", "200", false, "a multiple of 100"},
		{"HK ETF half a lot", types.DataTypeHKETF, 100, "0.001", "50", true, "50 mod 100 is 50"},

		{"HK warrant odd lot", types.DataTypeHKWarrant, 1, "0.001", "7", false, "lot 1 makes every integer legal"},
		{"HK CBBC odd lot", types.DataTypeHKCBBC, 1, "0.001", "3", false, "lot 1"},

		{"HK bond single unit", types.DataTypeHKBond, 1, "0.0001", "1", false, "lot 1"},

		{"HK index has no lot", types.DataTypeHKIndex, 0, "", "150", false, "a fractional index quantity is legal"},
		{"HK index fraction", types.DataTypeHKIndex, 0, "", "0.5", false, "the lot==0 skip also skips the integer check"},

		{"unknown dtype has no lot", types.DataType(-1), 0, "", "0.5", false, "an unknown dtype is unknown, not refused"},
		{"zero dtype has no lot", types.DataType(0), 0, "", "1e-330", false, "and so is a zero one"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// The table row is cross-checked against domain.DefaultHKTickSchedule
			// so a change to the schedule is reported as a schedule change rather
			// than as a validation change.
			schedule := domain.DefaultHKTickSchedule(tc.dtype)
			if schedule.Lot != tc.lot || schedule.TickSize != tc.tick {
				t.Fatalf("fixture is stale: DefaultHKTickSchedule(%d) is lot %d tick %q, "+
					"this row claims lot %d tick %q", tc.dtype, schedule.Lot, schedule.TickSize, tc.lot, tc.tick)
			}

			err := validateQuantityForHK(domain.MustNewQuantity(tc.qty),
				domain.TickSchedule{}, domain.Symbol{Market: domain.MarketHK, DataType: tc.dtype})
			if tc.wantErr {
				assertInvalidParam(t, err, opEntrust)
				return
			}
			if err != nil {
				t.Errorf("validateQuantityForHK(%s, %d) = %v, want nil (%s)", tc.qty, tc.dtype, err, tc.why)
			}
		})
	}
}

// TestValidateQuantityForHKDoesNotRejectNegativeOrders states the missing sign
// check on its own, because it is the one row in the matrix where the helper's
// behaviour is a defect rather than a decision.
//
// domain.Quantity.ValidateInteger asks whether the value has no fractional part;
// ValidateLot asks whether it divides evenly. Neither asks whether it is
// positive, and a negative multiple of 100 satisfies both, so
// validateOrderForHK accepts a short order expressed as a negative quantity. The
// Gateway will reject it, but the fail-closed check this package advertises does
// not. Recorded here as current behaviour; fixing it is a production change.
func TestValidateQuantityForHKDoesNotRejectNegativeOrders(t *testing.T) {
	for _, qty := range []string{"-100", "-1", "-0.5"} {
		t.Run(qty, func(t *testing.T) {
			if err := validateQuantityForHK(domain.MustNewQuantity(qty),
				domain.TickSchedule{},
				domain.Symbol{Market: domain.MarketHK, DataType: types.DataTypeHKStock}); err != nil {
				// If a later change adds the sign check, this row starts failing and
				// the comment above becomes out of date, which is the point.
				t.Logf("validateQuantityForHK(%s) now rejects a negative quantity: "+
					"the sign check has been added, so the comment on this test is stale", qty)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// validatePriceForHK
// ---------------------------------------------------------------------------

// TestValidatePriceForHKMatrix covers both branches and the three ways through.
//
// The dtype with no schedule skips the check entirely, so a negative price is
// accepted there. That is pinned, because it is the one place where the helper
// accepts something a reader would expect it to refuse, and a reader who does not
// know that will conclude the check is not running at all.
func TestValidatePriceForHKMatrix(t *testing.T) {
	for _, tc := range []struct {
		name    string
		dtype   types.DataType
		price   string
		tick    string
		wantErr bool
		why     string
	}{
		{"stock on tick", types.DataTypeHKStock, "123.456", "0.001", false, "an exact multiple of 0.001"},
		{"stock off tick", types.DataTypeHKStock, "123.4565", "0.001", true, "0.0005 remainder"},
		{"stock negative", types.DataTypeHKStock, "-1", "0.001", true, "the negative check runs before the step check"},
		{"stock negative zero", types.DataTypeHKStock, "-0.001", "0.001", true, "and it is a sign test, not a magnitude test"},
		{"stock zero price", types.DataTypeHKStock, "0", "0.001", false, "zero is not negative and divides evenly"},

		{"read-path zero tick no longer exempts an order", types.DataTypeHKStock, "0.0005", "0", true,
			"REWRITTEN by the TickSchedule work. This row used to pass: the caller's zero tick " +
				"meant Price.Validate skipped the step test, so an off-grid order slipped through on " +
				"the strength of a tick the caller had declared. The instrument's grid now decides, " +
				"and 0.0005 is not a multiple of the stock tick 0.001. See the note above this table."},

		{"bond on its tick", types.DataTypeHKBond, "92.2567", "0.0001", false, "an exact multiple of 0.0001"},
		{"bond off its tick", types.DataTypeHKBond, "92.25675", "0.0001", true, "0.00005 remainder"},

		{"index has no schedule, so nothing is checked", types.DataTypeHKIndex, "-1", "0.001", false,
			"a negative price is accepted on a dtype with no tick schedule"},
		{"index accepts an off-schedule price", types.DataTypeHKIndex, "123.4567", "0.001", false,
			"the schedule's TickSize is empty, so the helper returns before the check"},
		{"unknown dtype is not checked", types.DataType(-1), "-1", "0.001", false, "no schedule, no check"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := validatePriceForHK(domain.MustNewPrice(tc.price, tc.tick),
				domain.TickSchedule{},
				domain.Symbol{Market: domain.MarketHK, DataType: tc.dtype})
			if tc.wantErr {
				assertInvalidParam(t, err, opEntrust)
				return
			}
			if err != nil {
				t.Errorf("validatePriceForHK(%q, %d) = %v, want nil (%s)", tc.price, tc.dtype, err, tc.why)
			}
		})
	}
}

// TestValidatePriceForHKSplitsGateFromStep separates the two roles the tick plays,
// because they are different and only one of them is the schedule's.
//
// validatePriceForHK asks the schedule whether to run a check at all, and the
// step test itself runs against the tick that check was given. Before the
// TickSchedule work, that tick was the one the caller handed the Price
// constructor, so the HK stock schedule could not make an off-tick price fail if
// the caller declared a finer one: the gate was the schedule, the step was the
// caller's, and a caller could always satisfy the second by choosing it.
//
// That was the defect. The step now runs against the instrument's tick, so
// TestValidatePriceForHKBelowRecordsTheOldBehaviour keeps the previous contract
// as a dated record while this test states the current one.
//
// This matters to a reader of the money tests: the loud-failure behaviour for a
// faithful 0.0005 is real, and it is real because the Price carries tick 0.001,
// not because the schedule insisted on it.
func TestValidatePriceForHKSplitsGateFromStep(t *testing.T) {
	const offStockTick = "388.0501" // four decimal places, finer than the HK stock tick
	stock := domain.Symbol{Market: domain.MarketHK, DataType: types.DataTypeHKStock}

	// The instrument's tick governs, so a caller cannot widen the grid by
	// declaring a finer one. This is the row that fails against the pre-rewrite
	// code, which is what makes it the regression fence for the whole change.
	declaredFiner := domain.MustNewPrice(offStockTick, "0.0001")
	assertInvalidParam(t, validatePriceForHK(declaredFiner, domain.TickSchedule{}, stock), opEntrust)

	// A price on the instrument's grid passes whichever tick it declares, because
	// the declared tick is no longer consulted for the step.
	declaredCoarser := domain.MustNewPrice("388.05", "0.01")
	if err := validatePriceForHK(declaredCoarser, domain.TickSchedule{}, stock); err != nil {
		t.Errorf("a price on the stock grid was rejected because it declared 0.01: %v. "+
			"The instrument's tick decides the step, not the price's own.", err)
	}

	refused := domain.MustNewPrice(offStockTick, "0.001")
	assertInvalidParam(t, validatePriceForHK(refused, domain.TickSchedule{}, stock), opEntrust)
}

// TestPriceValidateStillChecksAgainstItsOwnTick pins the old gate/step split at
// the type it lived on, so the change above is legible rather than merely
// different.
//
// Price.Validate(step=true) still tests a price against the tick the caller
// attached to it. That is correct for what Validate means, and validatePriceForHK
// simply stopped using it for the step: a caller can always satisfy a check
// against a number they chose. If this test ever fails, Validate's contract moved
// and the note in TestValidatePriceForHKSplitsGateFromStep is no longer
// describing the code.
func TestPriceValidateStillChecksAgainstItsOwnTick(t *testing.T) {
	const offStockTick = "388.0501"

	if err := domain.MustNewPrice(offStockTick, "0.0001").Validate(true); err != nil {
		t.Errorf("Validate(true) against the price's own 0.0001 tick = %v, want nil: a price is "+
			"always a multiple of its own tick, which is why the step cannot be taken from here", err)
	}
	if err := domain.MustNewPrice(offStockTick, "0.001").Validate(true); err == nil {
		t.Error("Validate(true) against a 0.001 tick = nil, want an error: 0.0501 is not a " +
			"multiple of 0.001")
	}
}

// ---------------------------------------------------------------------------
// validateSessionWindow
// ---------------------------------------------------------------------------

// tradingSessionWindow is an alias for the anonymous struct value type of
// hkSessionWindows, declared so the tests below can build a replacement value
// and restore the original one. It is an alias rather than a defined type
// because a defined type would not be identical to the map's value type and the
// assignment would not compile — which is the property that makes this safe.
type tradingSessionWindow = struct {
	preMarketStart  string
	preMarketEnd    string
	regularStart    string
	regularEnd      string
	afterHoursStart string
	afterHoursEnd   string
}

// tradingReplaceSessionWindow installs a new HK window and restores the original
// one when the test ends.
//
// hkSessionWindows is a package global, so the restore is not optional and the
// containing test must not be parallel. Nothing in this package calls
// t.Parallel, and Go runs a package's tests sequentially by default, so the
// mutated window is never observed by another test. Both facts are load-bearing
// and are re-checked in TestSessionWindowMutationIsRestored.
func tradingReplaceSessionWindow(t *testing.T, w tradingSessionWindow) {
	t.Helper()
	original, ok := hkSessionWindows[string(domain.MarketHK)]
	if !ok {
		t.Fatalf("hkSessionWindows has no %q key to snapshot; the delete test below has "+
			"already run and not restored it", domain.MarketHK)
	}
	t.Cleanup(func() { hkSessionWindows[string(domain.MarketHK)] = original })
	hkSessionWindows[string(domain.MarketHK)] = w
}

// tradingDeleteSessionWindow removes the HK entry and restores it on cleanup. The
// only statement in the helper that no argument can reach.
func tradingDeleteSessionWindow(t *testing.T) {
	t.Helper()
	original, ok := hkSessionWindows[string(domain.MarketHK)]
	if !ok {
		t.Fatal("hkSessionWindows has no \"HK\" key to snapshot")
	}
	t.Cleanup(func() { hkSessionWindows[string(domain.MarketHK)] = original })
	delete(hkSessionWindows, string(domain.MarketHK))
}

// TestValidateSessionWindowAgreesWithTheWindowTable is the clock-dependent half,
// and it never asserts a hardcoded verdict. It computes the expectation from the
// same window the helper reads, so the assertion is a relationship that holds at
// any hour.
//
// It is not a duplicate of TestValidateSessionWindowAgreesWithHKClock in
// trading_test.go, which asserts the same relationship for the same three
// session types. This one additionally pins the four clock-independent exits —
// the empty session type, "0", an out-of-set type, and a non-HK market — and
// asserts the invariant that makes the whole helper's coverage clock-independent:
// the three windows are disjoint, so at most one of the three session types can
// be inside its window and at least two must therefore be refused, at every
// minute of every day.
func TestValidateSessionWindowAgreesWithTheWindowTable(t *testing.T) {
	window := hkSessionWindows[string(domain.MarketHK)]
	current := time.Now().In(hkZone).Format("15:04")

	t.Run("computed verdict for each session type", func(t *testing.T) {
		for _, tc := range []struct {
			sessionType string
			bounds      [2]string
		}{
			{"1", [2]string{window.preMarketStart, window.preMarketEnd}},
			{"2", [2]string{window.regularStart, window.regularEnd}},
			{"3", [2]string{window.afterHoursStart, window.afterHoursEnd}},
		} {
			before := time.Now().In(hkZone).Format("15:04")
			err := validateSessionWindow(domain.MarketHK, tc.sessionType)
			after := time.Now().In(hkZone).Format("15:04")
			if before != after {
				t.Skipf("minute boundary straddled the call (%s -> %s); the verdict is "+
					"legitimately ambiguous, so the row is skipped rather than failed", before, after)
			}
			if before != current {
				current = before
			}

			wantInside := before >= tc.bounds[0] && before <= tc.bounds[1]
			if gotInside := err == nil; gotInside != wantInside {
				t.Errorf("session %q = %v at %s HKT, want nil == %t for window %s-%s",
					tc.sessionType, err, before, wantInside, tc.bounds[0], tc.bounds[1])
			}
			if wantInside {
				continue
			}
			// A rejection is only a rejection if it is the typed one.
			assertInvalidParam(t, err, opEntrust)
		}
	})

	t.Run("the three windows are disjoint, so at least two session types are refused", func(t *testing.T) {
		refused := 0
		for _, sessionType := range []string{"1", "2", "3"} {
			if validateSessionWindow(domain.MarketHK, sessionType) != nil {
				refused++
			}
		}
		if refused < 2 {
			t.Errorf("%d of 3 session types were refused at %s HKT, want at least 2: the "+
				"windows %s-%s, %s-%s, %s-%s are disjoint, so at most one can contain "+
				"any minute. If this fires, the window table has been changed to overlap "+
				"and this test's premise needs restating.",
				refused, current,
				window.preMarketStart, window.preMarketEnd,
				window.regularStart, window.regularEnd,
				window.afterHoursStart, window.afterHoursEnd)
		}
	})

	t.Run("clock-independent exits", func(t *testing.T) {
		// An empty session type returns before the clock is consulted.
		if err := validateSessionWindow(domain.MarketHK, ""); err != nil {
			t.Errorf("session \"\" = %v, want nil: the empty type short-circuits before the map", err)
		}
		// "0" reaches the switch and returns nil without a window comparison.
		if err := validateSessionWindow(domain.MarketHK, "0"); err != nil {
			t.Errorf("session \"0\" = %v, want nil: the case exists and always returns nil", err)
		}
		// Anything outside the closed set falls out of the switch. This is the
		// only clock-independent way to reach the helper's trailing return, and it
		// is why the coverage of this function does not move with the hour.
		for _, sessionType := range []string{"7", "4", "unknown", " 1", "1 "} {
			if err := validateSessionWindow(domain.MarketHK, sessionType); err != nil {
				t.Errorf("out-of-set session type %q = %v, want nil", sessionType, err)
			}
		}
		// A non-HK market returns before the map is read at all, so the fixed
		// +08:00 zone cannot shift a window that is not Hong Kong wall-clock.
		for _, market := range []domain.Market{domain.MarketUS, domain.MarketShenzhenConnect, domain.MarketShanghaiConnect, ""} {
			for _, sessionType := range []string{"", "1", "2", "3"} {
				if err := validateSessionWindow(market, sessionType); err != nil {
					t.Errorf("validateSessionWindow(%q, %q) = %v, want nil", market, sessionType, err)
				}
			}
		}
	})
}

// TestValidateSessionWindowRejectsEveryWindowOnDemand is what makes the coverage
// number independent of the hour.
//
// The three rejection arms inside the switch are each reachable only while the
// clock is outside that one window, and the three windows together cover 09:00
// to 16:00 Hong Kong time. There is therefore no minute of the day at which all
// three arms are reachable at once — so a suite that merely drives "1", "2" and
// "3" covers two of the three arms at best, and which two depends on the hour.
// That is exactly the AGENTS.md failure mode, "a coverage figure that moves between
// runs of the same commit".
//
// So each arm is forced instead. The window bounds are moved to a sentinel pair
// that no "15:04" string can be inside — "25:00" is later than every minute of
// the day, and the comparison is lexicographic on equal-length strings — and the
// accept side is forced by a window that spans the whole day. Both directions are
// asserted, so the test does not merely reach the arm; it pins that the arm is
// governed by the window and by nothing else.
func TestValidateSessionWindowRejectsEveryWindowOnDemand(t *testing.T) {
	const (
		unreachableStart = "25:00"
		unreachableEnd   = "25:00"
		wholeDayStart    = "00:00"
		wholeDayEnd      = "23:59"
	)

	shipped := hkSessionWindows[string(domain.MarketHK)]

	for _, tc := range []struct {
		name        string
		sessionType string
		// patch replaces one window's bounds and leaves the other two as shipped,
		// so the row cannot be satisfied by a neighbouring arm.
		patch func(w *tradingSessionWindow)
	}{
		{
			name: "pre-market", sessionType: "1",
			patch: func(w *tradingSessionWindow) { w.preMarketStart, w.preMarketEnd = unreachableStart, unreachableEnd },
		},
		{
			name: "regular", sessionType: "2",
			patch: func(w *tradingSessionWindow) { w.regularStart, w.regularEnd = unreachableStart, unreachableEnd },
		},
		{
			name: "after-hours", sessionType: "3",
			patch: func(w *tradingSessionWindow) { w.afterHoursStart, w.afterHoursEnd = unreachableStart, unreachableEnd },
		},
	} {
		t.Run(tc.name+" refused", func(t *testing.T) {
			w := shipped
			tc.patch(&w)
			tradingReplaceSessionWindow(t, w)

			assertInvalidParam(t, validateSessionWindow(domain.MarketHK, tc.sessionType), opEntrust)
		})

		t.Run(tc.name+" accepted when the window covers the day", func(t *testing.T) {
			w := shipped
			tc.patch(&w)
			// Move the same pair to the other side of the comparison so this row
			// differs from the one above in exactly one thing.
			switch tc.name {
			case "pre-market":
				w.preMarketStart, w.preMarketEnd = wholeDayStart, wholeDayEnd
			case "regular":
				w.regularStart, w.regularEnd = wholeDayStart, wholeDayEnd
			case "after-hours":
				w.afterHoursStart, w.afterHoursEnd = wholeDayStart, wholeDayEnd
			}
			tradingReplaceSessionWindow(t, w)

			if err := validateSessionWindow(domain.MarketHK, tc.sessionType); err != nil {
				t.Errorf("session %q inside a whole-day window = %v, want nil: the rejection "+
					"above is governed by the window bounds and by nothing else", tc.sessionType, err)
			}
		})
	}
}

// TestValidateSessionWindowWithoutAnEntryCoversTheLookupMiss is the one statement
// in this file that no argument can reach, so it is reached by removing the input
// instead.
//
// The line above the lookup pins market == MarketHK and the map's only key is
// "HK", so ok is always true and the !ok arm is dead as written. Deleting the key
// inside the test, with a t.Cleanup that puts it back, is the only way in — and it
// is why this test exists at all rather than a comment.
//
// The assertion is a differential pair, not a single verdict: with the key gone
// all three session types are accepted unconditionally, and with the key present
// at least two of them are refused (the windows are disjoint, see
// TestValidateSessionWindowAgreesWithTheWindowTable). Deleting the key therefore
// changes an observable outcome, which is what shows the arm was taken.
func TestValidateSessionWindowWithoutAnEntryCoversTheLookupMiss(t *testing.T) {
	if _, ok := hkSessionWindows[string(domain.MarketHK)]; !ok {
		t.Fatal(`hkSessionWindows has no "HK" key to start from`)
	}
	refusedWhilePresent := 0
	for _, sessionType := range []string{"1", "2", "3"} {
		if validateSessionWindow(domain.MarketHK, sessionType) != nil {
			refusedWhilePresent++
		}
	}

	tradingDeleteSessionWindow(t)

	for _, sessionType := range []string{"1", "2", "3"} {
		if err := validateSessionWindow(domain.MarketHK, sessionType); err != nil {
			t.Errorf("session %q with no window entry = %v, want nil: a missing entry is "+
				"treated as no window rather than as a rejection", sessionType, err)
		}
	}
	// A missing entry must not turn a closed-set type into a rejection either.
	for _, sessionType := range []string{"", "0", "7"} {
		if err := validateSessionWindow(domain.MarketHK, sessionType); err != nil {
			t.Errorf("session %q with no window entry = %v, want nil", sessionType, err)
		}
	}

	if refusedWhilePresent < 2 {
		t.Fatalf("only %d of 3 session types were refused with the entry present, so the pair "+
			"below proves nothing: the lookup-miss arm and the rejection arm would look alike",
			refusedWhilePresent)
	}
	t.Logf("with the entry present, %d of 3 session types were refused; with it deleted, 0 are. "+
		"The difference is the lookup-miss return.", refusedWhilePresent)
}

// TestSessionWindowMutationIsRestored is the guard on the two tests above, because
// they mutate a package global and a test that leaves it changed would turn every
// later session-window assertion into a false result. The guard is a fresh
// snapshot compared against the values the production file declares, so a leaked
// mutation is reported by name rather than as a mysterious failure downstream.
func TestSessionWindowMutationIsRestored(t *testing.T) {
	got, ok := hkSessionWindows[string(domain.MarketHK)]
	if !ok {
		t.Fatal(`hkSessionWindows has no "HK" key: a test mutated the global and did not restore it`)
	}
	want := tradingSessionWindow{
		preMarketStart:  "09:00",
		preMarketEnd:    "09:30",
		regularStart:    "09:30",
		regularEnd:      "12:00",
		afterHoursStart: "12:00",
		afterHoursEnd:   "16:00",
	}
	if got != want {
		t.Errorf("hkSessionWindows[\"HK\"] = %+v, want %+v: a test above leaked a mutated window",
			got, want)
	}
	if len(hkSessionWindows) != 1 {
		t.Errorf("len(hkSessionWindows) = %d, want 1: a key was added or left behind", len(hkSessionWindows))
	}
}

// ---------------------------------------------------------------------------
// validateOrderTypeEligibility
// ---------------------------------------------------------------------------

// tradingHKEligibleOrderTypes is the closed set knownHKOrderTypes accepts, named
// rather than read from the map, so the test fails if a type is added to one side
// and not the other. This is the same exhaustion-by-count discipline
// client/route_mutation_test.go applies to the mutation routes: types.EntrustType
// has no exported enumeration, so a count plus a named list is the only way to
// notice drift in either direction.
var tradingHKEligibleOrderTypes = []types.EntrustType{
	types.EntrustTypeAuctionLimit,
	types.EntrustTypeAuction,
	types.EntrustTypeEnhancedLimit,
	types.EntrustTypeLimit,
	types.EntrustTypeSpecialLimit,
	types.EntrustTypeDarkPool,
	types.EntrustTypeOddLot,
	types.EntrustTypeStopProfitLimit,
	types.EntrustTypeStopLossLimit,
	types.EntrustTypeTrailingStopLimit,
}

// TestValidateOrderTypeEligibilityMatrix covers the non-HK skip, the closed set on
// both sides, and the rejections.
func TestValidateOrderTypeEligibilityMatrix(t *testing.T) {
	t.Run("closed set size", func(t *testing.T) {
		if len(knownHKOrderTypes) != 10 {
			t.Fatalf("len(knownHKOrderTypes) = %d, want 10: the set has drifted from the ten "+
				"types named below", len(knownHKOrderTypes))
		}
		if len(tradingHKEligibleOrderTypes) != len(knownHKOrderTypes) {
			t.Fatalf("the test names %d types but knownHKOrderTypes holds %d",
				len(tradingHKEligibleOrderTypes), len(knownHKOrderTypes))
		}
	})

	t.Run("accept the named set", func(t *testing.T) {
		for _, orderType := range tradingHKEligibleOrderTypes {
			t.Run(string(orderType), func(t *testing.T) {
				if _, ok := knownHKOrderTypes[orderType]; !ok {
					t.Fatalf("knownHKOrderTypes has no entry for the type the test names as "+
						"eligible: %q", orderType)
				}
				if err := validateOrderTypeEligibility(orderType, domain.MarketHK); err != nil {
					t.Errorf("validateOrderTypeEligibility(%q, HK) = %v, want nil", orderType, err)
				}
			})
		}
	})

	t.Run("reject", func(t *testing.T) {
		for _, tc := range []struct {
			name      string
			orderType types.EntrustType
		}{
			{"empty", ""},
			{"market order", types.EntrustTypeMarket},
			{"iceberg limit", types.EntrustTypeIcebergLimit},
			{"iceberg market", types.EntrustTypeIcebergMarket},
			{"hidden limit", types.EntrustTypeHiddenLimit},
			{"hidden market", types.EntrustTypeHiddenMarket},
			// The three market-order stop variants are conditional and
			// price-optional but are not on the HK eligible list, so an order built
			// from one of them is refused at eligibility and never reaches the
			// conditional block. Asserted so the asymmetry is deliberate: the
			// limit-order counterparts ("31", "33", "35") are eligible.
			{"stop-profit market", types.EntrustTypeStopProfitMarket},
			{"stop-loss market", types.EntrustTypeStopLossMarket},
			{"trailing-stop market", types.EntrustTypeTrailingStopMarket},
			{"unknown code", types.EntrustType("99")},
		} {
			t.Run(tc.name, func(t *testing.T) {
				if _, ok := knownHKOrderTypes[tc.orderType]; ok {
					t.Fatalf("knownHKOrderTypes contains %q, which the test names as rejected", tc.orderType)
				}
				assertInvalidParam(t, validateOrderTypeEligibility(tc.orderType, domain.MarketHK), opEntrust)
			})
		}
	})

	t.Run("a non-HK market skips the check entirely", func(t *testing.T) {
		for _, market := range []domain.Market{domain.MarketUS, domain.MarketShenzhenConnect, domain.MarketShanghaiConnect, ""} {
			for _, orderType := range []types.EntrustType{"", "99", types.EntrustTypeMarket, types.EntrustTypeHiddenLimit} {
				if err := validateOrderTypeEligibility(orderType, market); err != nil {
					t.Errorf("validateOrderTypeEligibility(%q, %q) = %v, want nil", orderType, market, err)
				}
			}
		}
	})
}

// ---------------------------------------------------------------------------
// validateTimeInForce
// ---------------------------------------------------------------------------

// TestValidateTimeInForceMatrix covers the closed set on both sides.
//
// The case row is the point of the accept table: the lookup is a map keyed by the
// exact string, so "day" is not "DAY". A case-insensitive implementation would be
// a wire change — the value that reaches the Gateway would differ from the value
// the caller wrote.
func TestValidateTimeInForceMatrix(t *testing.T) {
	for _, tc := range []struct {
		name    string
		tif     string
		wantErr bool
	}{
		{"unset", "", false},
		{"day", "DAY", false},
		{"immediate or cancel", "IOC", false},
		{"fill or kill", "FOK", false},
		{"good till date", "GTD", false},

		{"lowercase", "day", true},
		{"mixed case", "Day", true},
		{"padded", " DAY", true},
		{"unknown", "XGTD", true},
		{"lowercase gt day", "gt d", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := validateTimeInForce(tc.tif)
			if tc.wantErr {
				assertInvalidParam(t, err, opEntrust)
				return
			}
			if err != nil {
				t.Errorf("validateTimeInForce(%q) = %v, want nil", tc.tif, err)
			}
		})
	}
}

// TestValidateTimeInForceIsCaseSensitive states the property rather than the
// table, because the table above can be edited to forget it: a case-insensitive
// lookup would still pass every row except the lowercase one, and that row is a
// single case in a single list.
func TestValidateTimeInForceIsCaseSensitive(t *testing.T) {
	for _, canonical := range []string{"DAY", "IOC", "FOK", "GTD"} {
		lowered := strings.ToLower(canonical)
		if err := validateTimeInForce(canonical); err != nil {
			t.Fatalf("validateTimeInForce(%q) = %v, want nil", canonical, err)
		}
		assertInvalidParam(t, validateTimeInForce(lowered), opEntrust)
	}
}

// ---------------------------------------------------------------------------
// validateShortSellingFlag
// ---------------------------------------------------------------------------

// TestValidateShortSellingFlagMatrix covers the two dtypes the helper governs,
// the four legal sides on each, and the skip for every other dtype.
func TestValidateShortSellingFlagMatrix(t *testing.T) {
	for _, dtype := range []types.DataType{types.DataTypeHKStock, types.DataTypeHKETF} {
		dtype := dtype
		t.Run(dtype.String()+"/accept", func(t *testing.T) {
			for _, side := range []types.EntrustBS{
				types.EntrustBuy, types.EntrustSell, types.EntrustOpenShort, types.EntrustCloseShort,
			} {
				if err := validateShortSellingFlag(side, dtype); err != nil {
					t.Errorf("validateShortSellingFlag(%q, %s) = %v, want nil", side, dtype, err)
				}
			}
		})
		t.Run(dtype.String()+"/reject", func(t *testing.T) {
			for _, side := range []types.EntrustBS{"9", "", "0", "5", "01"} {
				assertInvalidParam(t, validateShortSellingFlag(side, dtype), opEntrust)
			}
		})
	}

	t.Run("every other dtype skips the check", func(t *testing.T) {
		for _, dtype := range []types.DataType{
			types.DataTypeHKIndex, types.DataTypeHKWarrant, types.DataTypeHKCBBC,
			types.DataTypeHKBond, types.DataTypeHKSector, types.DataTypeUSStock,
			types.DataType(-1), types.DataType(0),
		} {
			for _, side := range []types.EntrustBS{"9", "", "5", "01", types.EntrustBuy} {
				if err := validateShortSellingFlag(side, dtype); err != nil {
					t.Errorf("validateShortSellingFlag(%q, %s) = %v, want nil: only an HK stock "+
						"or ETF has a restricted side set", side, dtype, err)
				}
			}
		}
	})
}

// TestValidateShortSellingFlagIsADtypeGatedDifferential is the accept/reject pair
// that shows the dtype argument is load-bearing, rather than showing the branch
// was reached. The only thing that changes between the two calls is the data type.
func TestValidateShortSellingFlagIsADtypeGatedDifferential(t *testing.T) {
	const bad = types.EntrustBS("9")

	assertInvalidParam(t, validateShortSellingFlag(bad, types.DataTypeHKStock), opEntrust)
	assertInvalidParam(t, validateShortSellingFlag(bad, types.DataTypeHKETF), opEntrust)
	if err := validateShortSellingFlag(bad, types.DataTypeHKBond); err != nil {
		t.Errorf("side %q on a bond = %v, want nil: a bond is outside the restricted dtypes", bad, err)
	}
	if err := validateShortSellingFlag(bad, types.DataTypeUSStock); err != nil {
		t.Errorf("side %q on a US stock = %v, want nil", bad, err)
	}
}

// ---------------------------------------------------------------------------
// isPriceOptional and isConditionalOrder
// ---------------------------------------------------------------------------

// tradingOrderTypeFacts is the whole classification of every types.EntrustType
// constant, in one table: whether the price is optional, whether the order is
// conditional, and whether the type is eligible for HK. One table rather than
// three because the three classifiers overlap in a way that is only visible
// together — the six conditional types are all price-optional, and three of those
// six are not HK-eligible.
var tradingOrderTypeFacts = []struct {
	orderType  types.EntrustType
	constant   string
	priceOpt   bool
	conditiona bool
	hkEligible bool
}{
	{types.EntrustTypeAuctionLimit, "EntrustTypeAuctionLimit", false, false, true},
	{types.EntrustTypeAuction, "EntrustTypeAuction", false, false, true},
	{types.EntrustTypeEnhancedLimit, "EntrustTypeEnhancedLimit", false, false, true},
	{types.EntrustTypeLimit, "EntrustTypeLimit", false, false, true},
	{types.EntrustTypeSpecialLimit, "EntrustTypeSpecialLimit", false, false, true},
	{types.EntrustTypeMarket, "EntrustTypeMarket", true, false, false},
	{types.EntrustTypeDarkPool, "EntrustTypeDarkPool", false, false, true},
	{types.EntrustTypeOddLot, "EntrustTypeOddLot", false, false, true},
	{types.EntrustTypeIcebergMarket, "EntrustTypeIcebergMarket", true, false, false},
	{types.EntrustTypeIcebergLimit, "EntrustTypeIcebergLimit", false, false, false},
	{types.EntrustTypeHiddenMarket, "EntrustTypeHiddenMarket", true, false, false},
	{types.EntrustTypeHiddenLimit, "EntrustTypeHiddenLimit", false, false, false},
	{types.EntrustTypeStopProfitLimit, "EntrustTypeStopProfitLimit", true, true, true},
	{types.EntrustTypeStopProfitMarket, "EntrustTypeStopProfitMarket", true, true, false},
	{types.EntrustTypeStopLossLimit, "EntrustTypeStopLossLimit", true, true, true},
	{types.EntrustTypeStopLossMarket, "EntrustTypeStopLossMarket", true, true, false},
	{types.EntrustTypeTrailingStopLimit, "EntrustTypeTrailingStopLimit", true, true, true},
	{types.EntrustTypeTrailingStopMarket, "EntrustTypeTrailingStopMarket", true, true, false},
}

// TestOrderTypeClassificationTables drives both switch-based classifiers over
// every types.EntrustType constant.
//
// The table is exhaustive in the sense that matters: types.EntrustType is a
// string with no exported enumeration, so a new constant added to types would
// silently fall into the default arm of both classifiers. The count is pinned so
// that drift is a test failure. (The plan called this set "17"; it is 18.)
func TestOrderTypeClassificationTables(t *testing.T) {
	if len(tradingOrderTypeFacts) != 18 {
		t.Fatalf("tradingOrderTypeFacts holds %d rows, want 18: types.EntrustType has gained "+
			"or lost a constant and this table must be extended", len(tradingOrderTypeFacts))
	}

	var priceOptional, conditional, eligible int
	seen := make(map[types.EntrustType]string, len(tradingOrderTypeFacts))
	for _, tc := range tradingOrderTypeFacts {
		if prev, dup := seen[tc.orderType]; dup {
			t.Fatalf("order type %q appears twice in the table (also as %s)", tc.orderType, prev)
		}
		seen[tc.orderType] = tc.constant

		t.Run(tc.constant, func(t *testing.T) {
			if got := isPriceOptional(tc.orderType); got != tc.priceOpt {
				t.Errorf("isPriceOptional(%q) = %t, want %t", tc.orderType, got, tc.priceOpt)
			}
			if got := isConditionalOrder(tc.orderType); got != tc.conditiona {
				t.Errorf("isConditionalOrder(%q) = %t, want %t", tc.orderType, got, tc.conditiona)
			}
			// The eligibility classifier is driven through the helper so the two
			// switch tables and the map are checked against one another.
			err := validateOrderTypeEligibility(tc.orderType, domain.MarketHK)
			if got := err == nil; got != tc.hkEligible {
				t.Errorf("validateOrderTypeEligibility(%q, HK) accepted = %t, want %t",
					tc.orderType, got, tc.hkEligible)
			}
			if !tc.hkEligible {
				assertInvalidParam(t, err, opEntrust)
			}
		})

		priceOptional += boolToInt(tc.priceOpt)
		conditional += boolToInt(tc.conditiona)
		eligible += boolToInt(tc.hkEligible)
	}

	if priceOptional != 9 {
		t.Errorf("price-optional types = %d, want 9", priceOptional)
	}
	if conditional != 6 {
		t.Errorf("conditional types = %d, want 6", conditional)
	}
	if eligible != 10 {
		t.Errorf("HK-eligible types = %d, want 10", eligible)
	}
}

// TestEveryConditionalOrderIsPriceOptional is the invariant the table above
// encodes but does not by itself assert: a conditional order with a required
// price would be checked for a tick the caller has not supplied, and an order
// type that is both conditional and price-optional is exactly the shape the
// validDays/condValue block exists for. A future sixth-and-a-half entry that
// breaks it fails here.
func TestEveryConditionalOrderIsPriceOptional(t *testing.T) {
	for _, tc := range tradingOrderTypeFacts {
		if !tc.conditiona {
			continue
		}
		if !isPriceOptional(tc.orderType) {
			t.Errorf("%q is conditional but not price-optional, so its price would be tick-"+
				"checked against a value the caller never had to supply", tc.constant)
		}
	}
}

// TestEveryHKEligibleConditionalOrderExists states the other half of the
// asymmetry: of the six conditional types, only the three limit-order variants
// are HK-eligible. An HK caller can therefore build a stop-profit *limit* order
// and not a stop-profit *market* order, and that is a Gateway-side distinction
// the SDK encodes as a local rejection.
func TestEveryHKEligibleConditionalOrderExists(t *testing.T) {
	var eligible, ineligible []string
	for _, tc := range tradingOrderTypeFacts {
		if !tc.conditiona {
			continue
		}
		if tc.hkEligible {
			eligible = append(eligible, tc.constant)
		} else {
			ineligible = append(ineligible, tc.constant)
		}
	}
	if len(eligible) != 3 || len(ineligible) != 3 {
		t.Errorf("of six conditional types, %d are HK-eligible (%v) and %d are not (%v); "+
			"this test states a measured split, and the measurement no longer holds",
			len(eligible), eligible, len(ineligible), ineligible)
	}
}

// boolToInt keeps the tallies above readable without a conversion at each site.
func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// ---------------------------------------------------------------------------
// validateOrderForHK
// ---------------------------------------------------------------------------

// TestValidateOrderForHKAcceptsTheFixture is the accept side of the whole matrix,
// and it is what makes every reject row below attributable. If this failed, every
// rejection row would be suspect.
func TestValidateOrderForHKAcceptsTheFixture(t *testing.T) {
	if err := validateOrderForHK(tradingHKOrder(), domain.TickSchedule{}); err != nil {
		t.Fatalf("the HK fixture is invalid: %v. Every reject row in this file is "+
			"attributed to its single mutated field only if this passes", err)
	}
	if err := validateOrderForHK(tradingUSOrder(), domain.TickSchedule{}); err != nil {
		t.Fatalf("the US fixture is invalid: %v", err)
	}
}

// TestValidateOrderForHKRejectsOneFaultAtATime is the reject table.
//
// Each row starts from the valid fixture and changes exactly one thing, so the
// other five checks still pass and the row can only fail if the check it names
// exists. Delete any single check from validateOrderForHK and exactly the rows
// that name it turn into a nil error.
//
// What these rows deliberately do not assert is which check fired when two are
// faulty at once, because every rejection here is the same typed error and the
// typed surface cannot tell them apart. TestValidateOrderForHKDoesNotPinOrder
// says so where a reader will look for the ordering.
func TestValidateOrderForHKRejectsOneFaultAtATime(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(o *domain.Order)
		why    string
	}{
		{
			name:   "quantity is not a lot multiple",
			mutate: func(o *domain.Order) { o.Quantity = domain.MustNewQuantity("150") },
			why:    "150 mod 100 for a stock",
		},
		{
			name:   "quantity is not an integer",
			mutate: func(o *domain.Order) { o.Quantity = domain.MustNewQuantity("100.5") },
			why:    "the integer check precedes the lot check",
		},
		{
			name:   "price is negative",
			mutate: func(o *domain.Order) { o.Price = domain.MustNewPrice("-1", "0.001") },
			why:    "the sign check precedes the step check",
		},
		{
			// The loud-failure case from design-tick-model.md §1: a faithful
			// sub-tick price inside a Price that declares the real tick now fails,
			// rather than being rounded into a different price.
			name:   "price is off the tick",
			mutate: func(o *domain.Order) { o.Price = domain.MustNewPrice("0.0005", "0.001") },
			why:    "0.0005 is not a multiple of 0.001",
		},
		{
			name: "order type is not eligible for HK",
			mutate: func(o *domain.Order) {
				o.OrderType = types.EntrustTypeMarket // price-optional, so the price still passes
				o.Price = domain.MustNewPrice("387.05", "0.001")
			},
			why: `"5" is not in knownHKOrderTypes`,
		},
		{
			name:   "order type is empty",
			mutate: func(o *domain.Order) { o.OrderType = "" },
			why:    "the empty type is neither price-optional nor eligible",
		},
		{
			name:   "time in force is not in the closed set",
			mutate: func(o *domain.Order) { o.TimeInForce = "XGTD" },
			why:    "the set is exact-match and case-sensitive",
		},
		{
			name:   "side is not one of the four",
			mutate: func(o *domain.Order) { o.Side = "9" },
			why:    "an HK stock may only carry 1, 2, 3 or 4",
		},
		{
			name: "conditional order with validDays below the range",
			mutate: func(o *domain.Order) {
				o.OrderType = types.EntrustTypeStopProfitLimit
				o.Price = domain.MustNewPrice("387.05", "0.001")
				o.ValidDays = 0
				o.CondValue = "380.000"
			},
			why: "the range is 1..100 and the fixture's zero is below it",
		},
		{
			name: "conditional order with validDays above the range",
			mutate: func(o *domain.Order) {
				o.OrderType = types.EntrustTypeStopProfitLimit
				o.Price = domain.MustNewPrice("387.05", "0.001")
				o.ValidDays = maxValidDays + 1
				o.CondValue = "380.000"
			},
			why: "one past the ceiling",
		},
		{
			name: "conditional order with no condValue",
			mutate: func(o *domain.Order) {
				o.OrderType = types.EntrustTypeStopProfitLimit
				o.Price = domain.MustNewPrice("387.05", "0.001")
				o.ValidDays = 7
				o.CondValue = ""
			},
			why: "condValue is required for a conditional order",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			order := tradingHKOrder()
			tc.mutate(&order)
			assertInvalidParam(t, validateOrderForHK(order, domain.TickSchedule{}), opEntrust)
		})
	}
}

// TestValidateOrderForHKAcceptsTheVariantsThatLookLikeRejections is the accept
// table for the same helper, covering every row a naive reading would expect to
// be refused.
func TestValidateOrderForHKAcceptsTheVariantsThatLookLikeRejections(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(o *domain.Order)
		why    string
	}{
		{
			name:   "zero quantity is a legal multiple",
			mutate: func(o *domain.Order) { o.Quantity = domain.MustNewQuantity("0") },
			why:    "0 mod 100 is 0",
		},
		{
			name:   "an out-of-set session type is not a window failure",
			mutate: func(o *domain.Order) { o.SessionType = "7" },
			why:    "only 1, 2 and 3 are compared against the window",
		},
		{
			name:   "session type 0 is always inside",
			mutate: func(o *domain.Order) { o.SessionType = "0" },
			why:    `the switch has an explicit case for "0"`,
		},
		{
			name:   "an empty session type skips the clock",
			mutate: func(o *domain.Order) { o.SessionType = "" },
			why:    "the short circuit is above the map lookup",
		},
		{
			name: "a conditional order with a validDays/CondValue pair is accepted",
			mutate: func(o *domain.Order) {
				o.OrderType = types.EntrustTypeStopLossLimit // "33"
				o.Price = domain.MustNewPrice("0", "0.001")
				o.ValidDays = 1
				o.CondValue = "380.000"
			},
			why: "the price check is guarded by isPriceOptional and the conditional block is satisfied",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			order := tradingHKOrder()
			tc.mutate(&order)
			if err := validateOrderForHK(order, domain.TickSchedule{}); err != nil {
				t.Errorf("validateOrderForHK = %v, want nil (%s)", err, tc.why)
			}
		})
	}
}

// TestValidateOrderForHKPriceIsCheckedOnlyWhenItIsNotOptional is the differential
// pair that shows the isPriceOptional guard really gates the price check, rather
// than merely being reached. Two orders, identical except for the type and the
// price; one is refused and one is not, and the only difference is whether the
// price participates.
func TestValidateOrderForHKPriceIsCheckedOnlyWhenItIsNotOptional(t *testing.T) {
	// A limit order (price required) carrying a negative price.
	checked := tradingHKOrder()
	checked.Price = domain.MustNewPrice("-1", "0.001")
	assertInvalidParam(t, validateOrderForHK(checked, domain.TickSchedule{}), opEntrust)

	// The same negative price on a price-optional, HK-eligible conditional order.
	// "33" is stop-loss limit: conditional, price-optional, and eligible.
	skipped := tradingHKOrder()
	skipped.OrderType = types.EntrustTypeStopLossLimit
	skipped.Price = domain.MustNewPrice("-1", "0.001")
	skipped.ValidDays = 7
	skipped.CondValue = "380.000"
	if err := validateOrderForHK(skipped, domain.TickSchedule{}); err != nil {
		t.Errorf("a negative price on a price-optional order = %v, want nil: the price is "+
			"never handed to validatePriceForHK when isPriceOptional is true", err)
	}
}

// TestValidateOrderForHKSkipsEverythingForANonHKMarket is the passthrough row. The
// order is invalid in every way the HK path would object to, and it is still
// accepted.
func TestValidateOrderForHKSkipsEverythingForANonHKMarket(t *testing.T) {
	for _, market := range []domain.Market{domain.MarketUS, domain.MarketShenzhenConnect, domain.MarketShanghaiConnect, ""} {
		order := tradingHKOrder()
		order.Symbol.Market = market
		order.Quantity = domain.MustNewQuantity("150")
		order.Price = domain.MustNewPrice("-1", "0.001")
		order.OrderType = types.EntrustTypeHiddenLimit
		order.TimeInForce = "XGTD"
		order.Side = "9"
		order.SessionType = "1"
		if err := validateOrderForHK(order, domain.TickSchedule{}); err != nil {
			t.Errorf("validateOrderForHK for market %q = %v, want nil: the helper returns on "+
				"its first line for any market other than HK", market, err)
		}
	}
}

// TestValidateOrderForHKValidDaysBoundaries pins the two ends of the conditional
// range from both sides, because a range check is only pinned by its boundaries.
func TestValidateOrderForHKValidDaysBoundaries(t *testing.T) {
	if maxValidDays != 100 {
		t.Fatalf("maxValidDays = %d, want 100: the unexported ceiling is the contract the "+
			"message and the check both refer to", maxValidDays)
	}
	for _, tc := range []struct {
		days    int
		wantErr bool
		why     string
	}{
		{days: -1, wantErr: true, why: "negative"},
		{days: 0, wantErr: true, why: "below one"},
		{days: 1, wantErr: false, why: "the lower bound"},
		{days: 50, wantErr: false, why: "interior"},
		{days: maxValidDays - 1, wantErr: false, why: "one below the ceiling"},
		{days: maxValidDays, wantErr: false, why: "the ceiling"},
		{days: maxValidDays + 1, wantErr: true, why: "one past the ceiling"},
	} {
		t.Run(strconv.Itoa(tc.days), func(t *testing.T) {
			order := tradingHKOrder()
			order.OrderType = types.EntrustTypeStopProfitLimit
			order.Price = domain.MustNewPrice("387.05", "0.001")
			order.ValidDays = tc.days
			order.CondValue = "380.000"

			err := validateOrderForHK(order, domain.TickSchedule{})
			if tc.wantErr {
				assertInvalidParam(t, err, opEntrust)
				return
			}
			if err != nil {
				t.Errorf("validDays = %d (%s) = %v, want nil", tc.days, tc.why, err)
			}
		})
	}

	// A non-conditional order ignores validDays entirely, so the fixture's zero is
	// not a fault. Asserted so a future move of the range check outside the
	// isConditionalOrder guard would be visible.
	order := tradingHKOrder()
	order.ValidDays = 9999
	if err := validateOrderForHK(order, domain.TickSchedule{}); err != nil {
		t.Errorf("a limit order with validDays 9999 = %v, want nil: the range check only "+
			"applies to conditional orders", err)
	}
}

// TestValidateOrderForHKDoesNotPinOrder states the one thing this file cannot
// assert, in the place a reader will look for it.
//
// validateOrderForHK calls six helpers in a fixed order and returns on the first
// failure. All six produce the same *errs.Error: StatusInvalidParam,
// CategoryAPI, Op "trade/TradeEntrust". Nothing on the error says which helper
// rejected, so an order that is faulty in two ways cannot be observed to fail on
// a particular one, and this file does not pretend otherwise.
//
// The rows in TestValidateOrderForHKRejectsOneFaultAtATime are what does the work:
// each supplies valid values for the other five checks, so a row can only fail if
// the check it names is both present and reached. That is a stronger claim than
// "the branch was executed", and it does not need the error to identify itself.
func TestValidateOrderForHKDoesNotPinOrder(t *testing.T) {
	both := tradingHKOrder()
	both.Quantity = domain.MustNewQuantity("150") // fails validateQuantityForHK
	both.TimeInForce = "XGTD"                     // fails validateTimeInForce
	assertInvalidParam(t, validateOrderForHK(both, domain.TickSchedule{}), opEntrust)

	// The same two faults with only one of them present, which is what makes the
	// pair above attributable at all.
	onlyQuantity := tradingHKOrder()
	onlyQuantity.Quantity = domain.MustNewQuantity("150")
	assertInvalidParam(t, validateOrderForHK(onlyQuantity, domain.TickSchedule{}), opEntrust)

	onlyTIF := tradingHKOrder()
	onlyTIF.TimeInForce = "XGTD"
	assertInvalidParam(t, validateOrderForHK(onlyTIF, domain.TickSchedule{}), opEntrust)

	// Neither present: the same call is accepted, so the two rejections above are
	// not an artefact of the fixture.
	clean := tradingHKOrder()
	clean.Quantity = domain.MustNewQuantity("100")
	clean.TimeInForce = "DAY"
	if err := validateOrderForHK(clean, domain.TickSchedule{}); err != nil {
		t.Errorf("the clean order = %v, want nil: the rejections above are the two named "+
			"faults and not something about the fixture", err)
	}
}

// TestValidateOrderForHKPropagatesASessionRejection is the one row of
// validateOrderForHK whose coverage would otherwise be a function of the hour.
//
// The helper's fourth check returns the session-window error, and the window is
// compared against time.Now(). A rejection is reachable only while the clock is
// outside that session type's window, so a suite that merely sets SessionType to
// "1" covers the check's condition and nothing else — the return at trading.go:516
// stays dark at some hours and lights up at others, which is precisely the
// "coverage figure that moves between runs of the same commit" defect AGENTS.md
// records for internal/push.
//
// So the window is moved instead, with the same technique and the same cleanup
// TestValidateSessionWindowRejectsEveryWindowOnDemand uses. The accept row is
// included so the rejection cannot be a helper that refuses every session type.
func TestValidateOrderForHKPropagatesASessionRejection(t *testing.T) {
	shipped := hkSessionWindows[string(domain.MarketHK)]
	order := func(sessionType string) domain.Order {
		o := tradingHKOrder()
		o.SessionType = sessionType
		return o
	}

	t.Run("rejected when the window has moved", func(t *testing.T) {
		moved := shipped
		moved.preMarketStart, moved.preMarketEnd = "25:00", "25:00"
		tradingReplaceSessionWindow(t, moved)

		// Everything else on the order is valid, so the session check is the only
		// one that can fire.
		assertInvalidParam(t, validateOrderForHK(order("1"), domain.TickSchedule{}), opEntrust)
	})

	t.Run("accepted when the window covers the day", func(t *testing.T) {
		covering := shipped
		covering.preMarketStart, covering.preMarketEnd = "00:00", "23:59"
		tradingReplaceSessionWindow(t, covering)

		if err := validateOrderForHK(order("1"), domain.TickSchedule{}); err != nil {
			t.Errorf("session \"1\" inside a whole-day window = %v, want nil", err)
		}
	})

	t.Run("an out-of-set session type is never a window failure", func(t *testing.T) {
		moved := shipped
		moved.preMarketStart, moved.preMarketEnd = "25:00", "25:00"
		tradingReplaceSessionWindow(t, moved)

		if err := validateOrderForHK(order("7"), domain.TickSchedule{}); err != nil {
			t.Errorf("session \"7\" with an unreachable window = %v, want nil: the switch has no "+
				"case for it and falls through to the helper's trailing return", err)
		}
	})
}

// TestMaxValidDaysAndPageConstants pins the three exported-to-the-package
// constants the request builders depend on, so a change to one is reported here
// rather than as an unexplained value in a paging assertion.
func TestMaxValidDaysAndPageConstants(t *testing.T) {
	if defaultPageSize != 50 {
		t.Errorf("defaultPageSize = %d, want 50", defaultPageSize)
	}
	if maxPageSize != 500 {
		t.Errorf("maxPageSize = %d, want 500", maxPageSize)
	}
	if idempotencyKeyLen != 32 {
		t.Errorf("idempotencyKeyLen = %d, want 32", idempotencyKeyLen)
	}
	// idempotencyKeyLen is declared in trading.go and read by no service method in
	// this file. It is pinned here so that a reader who finds it unused knows the
	// number was noticed rather than missed.
	t.Logf("idempotencyKeyLen is declared in trading.go but no method in the v-next trading " +
		"surface sends an idempotency key; the value is pinned here so the gap is recorded")
}

// ---------------------------------------------------------------------------
// Method-level argument rejection
// ---------------------------------------------------------------------------

// tradingValidationCase is one method's argument rejection, driven with the
// executor present so the "no request was sent" half can be asserted.
type tradingValidationCase struct {
	name string
	op   string
	run  func(context.Context, *TradingService) error
}

// TestTradingMethodsRejectInvalidArgumentsWithoutARequest is the method-level half
// of the matrix: nineteen rows, one per argument a method checks for emptiness,
// plus the two delegated price and quantity checks on ChangeEntrust and the one
// delegated order check on Entrust.
//
// Every row asserts the typed rejection and zero recorded Do calls. The last is
// the half a validation test usually omits, and it is the one with teeth here: a
// mutation that reached the Gateway and was refused there would look identical to
// one refused locally, except that the local one must carry 1016 and the api
// category rather than a Gateway trading code.
func TestTradingMethodsRejectInvalidArgumentsWithoutARequest(t *testing.T) {
	account := tradingAccountID()
	validOrder := tradingHKOrder()
	badLotOrder := tradingHKOrder()
	badLotOrder.Quantity = domain.MustNewQuantity("150")

	cases := []tradingValidationCase{
		{"Entrust/zero account", opEntrust, func(ctx context.Context, s *TradingService) error {
			_, err := s.Entrust(ctx, "", validOrder)
			return err
		}},
		{"Entrust/invalid order", opEntrust, func(ctx context.Context, s *TradingService) error {
			_, err := s.Entrust(ctx, account, badLotOrder)
			return err
		}},

		{"CancelEntrust/zero account", opCancelEntrust, func(ctx context.Context, s *TradingService) error {
			return s.CancelEntrust(ctx, "", tradingEntrustID())
		}},
		{"CancelEntrust/zero entrust id", opCancelEntrust, func(ctx context.Context, s *TradingService) error {
			return s.CancelEntrust(ctx, account, "")
		}},

		{"BatchCancelEntrust/zero account", opBatchCancelEntrust, func(ctx context.Context, s *TradingService) error {
			return s.BatchCancelEntrust(ctx, "", []domain.EntrustID{"E-1"})
		}},

		{"ChangeEntrust/zero account", opChangeEntrust, func(ctx context.Context, s *TradingService) error {
			return s.ChangeEntrust(ctx, "", tradingEntrustID(), domain.MustNewPrice("1", "0.001"), domain.MustNewQuantity("100"))
		}},
		{"ChangeEntrust/zero entrust id", opChangeEntrust, func(ctx context.Context, s *TradingService) error {
			return s.ChangeEntrust(ctx, account, "", domain.MustNewPrice("1", "0.001"), domain.MustNewQuantity("100"))
		}},
		{"ChangeEntrust/price off tick", opEntrust, func(ctx context.Context, s *TradingService) error {
			return s.ChangeEntrust(ctx, account, tradingEntrustID(),
				domain.MustNewPrice("0.0005", "0.001"), domain.MustNewQuantity("100"))
		}},
		{"ChangeEntrust/price negative", opEntrust, func(ctx context.Context, s *TradingService) error {
			return s.ChangeEntrust(ctx, account, tradingEntrustID(),
				domain.MustNewPrice("-1", "0.001"), domain.MustNewQuantity("100"))
		}},
		{"ChangeEntrust/quantity off lot", opEntrust, func(ctx context.Context, s *TradingService) error {
			return s.ChangeEntrust(ctx, account, tradingEntrustID(),
				domain.MustNewPrice("1", "0.001"), domain.MustNewQuantity("150"))
		}},
		{"ChangeEntrust/quantity not an integer", opEntrust, func(ctx context.Context, s *TradingService) error {
			return s.ChangeEntrust(ctx, account, tradingEntrustID(),
				domain.MustNewPrice("1", "0.001"), domain.MustNewQuantity("100.5"))
		}},

		{"MaxAvailableAsset/zero account", opMaxAvailableAsset, func(ctx context.Context, s *TradingService) error {
			_, err := s.MaxAvailableAsset(ctx, "", tradingHKSymbol(), domain.MustNewPrice("1", "0"), types.EntrustTypeLimit)
			return err
		}},
		{"MaxAvailableAsset/zero symbol", opMaxAvailableAsset, func(ctx context.Context, s *TradingService) error {
			_, err := s.MaxAvailableAsset(ctx, account, domain.Symbol{}, domain.MustNewPrice("1", "0"), types.EntrustTypeLimit)
			return err
		}},
		// The symbol gate is on the code alone: a symbol carrying a market and a
		// data type but no code is still zero, and one carrying a code is not.
		{"MaxAvailableAsset/symbol with a market but no code", opMaxAvailableAsset, func(ctx context.Context, s *TradingService) error {
			_, err := s.MaxAvailableAsset(ctx, account,
				domain.Symbol{Market: domain.MarketHK, DataType: types.DataTypeHKStock},
				domain.MustNewPrice("1", "0"), types.EntrustTypeLimit)
			return err
		}},

		{"RealEntrustList/zero account", opRealEntrustList, func(ctx context.Context, s *TradingService) error {
			_, err := s.RealEntrustList(ctx, "", EntrustFilter{ExchangeType: types.ExchangeHK})
			return err
		}},
		{"RealDeliverList/zero account", opRealDeliverList, func(ctx context.Context, s *TradingService) error {
			_, err := s.RealDeliverList(ctx, "", DeliverFilter{ExchangeType: types.ExchangeHK})
			return err
		}},
		{"RealCondOrderList/zero account", opRealCondOrderList, func(ctx context.Context, s *TradingService) error {
			_, err := s.RealCondOrderList(ctx, "", CondOrderFilter{ExchangeType: types.ExchangeHK})
			return err
		}},
		{"HistoryEntrustList/zero account", opHistoryEntrustList, func(ctx context.Context, s *TradingService) error {
			_, err := s.HistoryEntrustList(ctx, "", HistoryFilter{ExchangeType: types.ExchangeHK})
			return err
		}},
		{"HistoryDeliverList/zero account", opHistoryDeliverList, func(ctx context.Context, s *TradingService) error {
			_, err := s.HistoryDeliverList(ctx, "", HistoryFilter{ExchangeType: types.ExchangeHK})
			return err
		}},
		{"HistoryCondOrderList/zero account", opHistoryCondOrderList, func(ctx context.Context, s *TradingService) error {
			_, err := s.HistoryCondOrderList(ctx, "", HistoryFilter{ExchangeType: types.ExchangeHK})
			return err
		}},
		{"MarginFullInfo/zero account", opMarginFullInfo, func(ctx context.Context, s *TradingService) error {
			_, err := s.MarginFullInfo(ctx, "")
			return err
		}},

		{"BeforeAndAfterSupport/zero symbol", opBeforeAndAfterSupport, func(ctx context.Context, s *TradingService) error {
			_, err := s.BeforeAndAfterSupport(ctx, domain.Symbol{})
			return err
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// One harmless reply is scripted so that a method which wrongly reached
			// the executor fails on requireCalls with a clear count rather than on
			// the fake's own exhausted-fixture check.
			exec := newSequencedExecutor(t, sequencedReply{reply: map[string]any{}})
			svc := NewTradingService(exec)

			assertInvalidParam(t, tc.run(t.Context(), svc), tc.op)
			requireCalls(t, exec, 0)
		})
	}
}

// TestTheSymbolGateIsOnTheCodeAlone is the accept/reject pair for the two methods
// that gate on a symbol, and it is asymmetric.
//
// domain.Symbol.IsZero reports on the code only. So a symbol carrying a market and
// a data type but no code is refused, while a symbol carrying a code and no market
// is accepted — and then the exchange switch has no case for the empty market, so
// the request goes out with an empty exchange type. A caller who forgot to set the
// market gets no local complaint, which is the same gap TestEntrustMapsOnlyHKAndUS
// Exchanges records for Entrust.
func TestTheSymbolGateIsOnTheCodeAlone(t *testing.T) {
	noCode := domain.Symbol{Market: domain.MarketHK, DataType: types.DataTypeHKStock}
	noMarket := domain.Symbol{Code: "00700", DataType: types.DataTypeHKStock}

	t.Run("no code is refused", func(t *testing.T) {
		exec := newSequencedExecutor(t, sequencedReply{reply: tradingMaxAvailableBody()})
		_, err := NewTradingService(exec).MaxAvailableAsset(t.Context(), tradingAccountID(),
			noCode, domain.MustNewPrice("1", "0"), types.EntrustTypeLimit)
		assertInvalidParam(t, err, opMaxAvailableAsset)
		requireCalls(t, exec, 0)
	})

	t.Run("a code with no market is accepted and sent with no exchange", func(t *testing.T) {
		exec := newSequencedExecutor(t, sequencedReply{reply: tradingSupportBody("1")})
		_, err := NewTradingService(exec).BeforeAndAfterSupport(t.Context(), noMarket)
		if err != nil {
			t.Fatalf("BeforeAndAfterSupport for a code with no market = %v, want nil: "+
				"the symbol gate reads the code only", err)
		}
		if got := tradingParamsAs[beforeAndAfterSupportWireRequest](t, exec).ExchangeType; got != "" {
			t.Errorf("ExchangeType = %q, want empty: the empty market matches no case", got)
		}
	})
}

// TestTradingMethodsAcceptTheMinimumValidArguments is the accept counterpart to
// the table above: a method that refused everything would pass it, and the caller
// would never be able to trade at all.
func TestTradingMethodsAcceptTheMinimumValidArguments(t *testing.T) {
	// The three mutation replies are the bare commonStringResponse shape; the
	// mutations read nothing out of them and return nil.
	str := json.RawMessage(`{"data":"1"}`)
	exec := newSequencedExecutor(t,
		sequencedReply{reply: tradingEntrustBody("E-1")},
		sequencedReply{reply: str},
		sequencedReply{reply: str},
		sequencedReply{reply: str},
		sequencedReply{reply: tradingMaxAvailableBody()},
		sequencedReply{reply: tradingOrderListBody()},
		sequencedReply{reply: tradingOrderListBody()},
		sequencedReply{reply: tradingCondOrderBody()},
		sequencedReply{reply: tradingOrderListBody()},
		sequencedReply{reply: tradingOrderListBody()},
		sequencedReply{reply: tradingCondOrderBody()},
		sequencedReply{reply: tradingMarginFullInfoBody()},
		sequencedReply{reply: tradingSupportBody("1")},
	)
	svc := NewTradingService(exec)
	ctx := t.Context()
	account := tradingAccountID()

	if _, err := svc.Entrust(ctx, account, tradingHKOrder()); err != nil {
		t.Errorf("Entrust: %v", err)
	}
	// A one-character account and a one-character entrust id are the minimum
	// non-zero values; nothing in the service asks for anything more.
	if err := svc.CancelEntrust(ctx, "a", "b"); err != nil {
		t.Errorf("CancelEntrust: %v", err)
	}
	if err := svc.BatchCancelEntrust(ctx, "a", []domain.EntrustID{"b"}); err != nil {
		t.Errorf("BatchCancelEntrust: %v", err)
	}
	if err := svc.ChangeEntrust(ctx, "a", "b", domain.MustNewPrice("1", "0.001"), domain.MustNewQuantity("100")); err != nil {
		t.Errorf("ChangeEntrust: %v", err)
	}
	if _, err := svc.MaxAvailableAsset(ctx, "a", tradingHKSymbol(), domain.MustNewPrice("1", "0"), types.EntrustTypeLimit); err != nil {
		t.Errorf("MaxAvailableAsset: %v", err)
	}
	if _, err := svc.RealEntrustList(ctx, "a", EntrustFilter{}); err != nil {
		t.Errorf("RealEntrustList: %v", err)
	}
	if _, err := svc.RealDeliverList(ctx, "a", DeliverFilter{}); err != nil {
		t.Errorf("RealDeliverList: %v", err)
	}
	if _, err := svc.RealCondOrderList(ctx, "a", CondOrderFilter{}); err != nil {
		t.Errorf("RealCondOrderList: %v", err)
	}
	if _, err := svc.HistoryEntrustList(ctx, "a", HistoryFilter{}); err != nil {
		t.Errorf("HistoryEntrustList: %v", err)
	}
	if _, err := svc.HistoryDeliverList(ctx, "a", HistoryFilter{}); err != nil {
		t.Errorf("HistoryDeliverList: %v", err)
	}
	if _, err := svc.HistoryCondOrderList(ctx, "a", HistoryFilter{}); err != nil {
		t.Errorf("HistoryCondOrderList: %v", err)
	}
	if _, err := svc.MarginFullInfo(ctx, "a"); err != nil {
		t.Errorf("MarginFullInfo: %v", err)
	}
	if _, err := svc.BeforeAndAfterSupport(ctx, domain.Symbol{Code: "X"}); err != nil {
		t.Errorf("BeforeAndAfterSupport: %v", err)
	}
	requireCalls(t, exec, 13)
}
