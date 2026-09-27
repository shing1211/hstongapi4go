// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package services

import (
	"errors"
	"testing"
	"time"

	"github.com/shing1211/hstongapi4go/internal/errs"
	"github.com/shing1211/hstongapi4go/pkg/domain"
	"github.com/shing1211/hstongapi4go/pkg/types"
)

// TestHKZoneIsTotal proves the property validateSessionWindow depends on: the
// zone it converts the wall clock into is never nil, so Time.In cannot panic.
//
// The clock is not injectable — validateSessionWindow calls time.Now() itself —
// so a test cannot force the host timezone database to be missing and then
// observe the panic. What it can do is assert the invariant that made the panic
// unreachable: the zone is a value produced by time.FixedZone, which cannot
// return nil, and it carries the +08:00 offset the session-window strings are
// written in. That is the meaningful assertion because the original defect was
// exactly the absence of this invariant — a zone obtained from a fallible
// lookup, with the error dropped.
func TestHKZoneIsTotal(t *testing.T) {
	if hkZone == nil {
		t.Fatal("hkZone is nil: Time.In(nil) panics, which is the defect this fix removes")
	}
	if got, want := hkZone.String(), "HKT"; got != want {
		t.Errorf("hkZone.String() = %q, want %q", got, want)
	}
	if _, offset := time.Now().In(hkZone).Zone(); offset != 8*60*60 {
		t.Errorf("hkZone offset = %d seconds, want %d (HKT is a constant +08:00)", offset, 8*60*60)
	}
}

// TestHKZoneAppliesConstantOffset checks the zone renders Hong Kong wall-clock
// time — a fixed eight hours ahead of UTC — across instants that straddle a DST
// transition in a zone that still has one. Hong Kong has had no DST since 1979,
// so a FixedZone is exact for every instant the SDK can be asked about; an
// offset that shifted with the seasons would mean the zone was wrong.
func TestHKZoneAppliesConstantOffset(t *testing.T) {
	instants := []time.Time{
		time.Date(2026, time.January, 15, 0, 0, 0, 0, time.UTC),
		time.Date(2026, time.March, 29, 0, 0, 0, 0, time.UTC),   // EU DST starts
		time.Date(2026, time.June, 30, 12, 0, 0, 0, time.UTC),   // EU DST mid-season
		time.Date(2026, time.November, 1, 0, 0, 0, 0, time.UTC), // EU DST ends
		time.Date(2026, time.December, 31, 23, 59, 0, 0, time.UTC),
	}
	for _, instant := range instants {
		if got, want := instant.In(hkZone).Format("15:04"), instant.UTC().Add(8*time.Hour).Format("15:04"); got != want {
			t.Errorf("%s in hkZone = %s, want %s", instant.Format(time.RFC3339), got, want)
		}
	}
}

// TestValidateSessionWindowDoesNotPanic calls the entrust-path session check
// directly for every session type and asserts it returns rather than panicking.
// With hkZone non-nil, Time.In has no nil argument to panic on: the
// location-load failure the old code discarded is no longer reachable, by
// construction rather than by an error branch.
func TestValidateSessionWindowDoesNotPanic(t *testing.T) {
	for _, sessionType := range []string{"", "0", "1", "2", "3", "4", "unknown"} {
		err := validateSessionWindow(domain.MarketHK, sessionType)
		if err == nil {
			continue
		}
		var e *errs.Error
		if !errors.As(err, &e) {
			t.Fatalf("validateSessionWindow(HK, %q) returned %T, want nil or *errs.Error", sessionType, err)
		}
		if code, ok := errs.CodeOf(err); !ok || code != types.StatusInvalidParam {
			t.Errorf("validateSessionWindow(HK, %q) code = (%q, %t), want (%q, true)",
				sessionType, code, ok, types.StatusInvalidParam)
		}
		if e.Op != opEntrust {
			t.Errorf("validateSessionWindow(HK, %q) Op = %q, want %q", sessionType, e.Op, opEntrust)
		}
	}
}

// TestValidateSessionWindowAgreesWithHKClock asserts the accept/reject decision
// is the one the session-window table predicts for the current Hong Kong
// wall-clock minute, and that the comparison is made in Hong Kong time at all.
//
// The test deliberately does not assert that a request is accepted "right now".
// hkSessionWindows has no minute at which session types 1, 2 and 3 are all
// simultaneously valid, so such an assertion would pass or fail with the hour it
// runs. It also cannot inject an instant, so the expected verdict is derived
// from the same clock the function reads: the relationship between the two is
// fixed, the hour is not. A minute that flips between the two readings means the
// verdict legitimately differs, so the case is skipped rather than failed.
func TestValidateSessionWindowAgreesWithHKClock(t *testing.T) {
	window := hkSessionWindows[string(domain.MarketHK)]
	inside := map[string][2]string{
		"1": {window.preMarketStart, window.preMarketEnd},
		"2": {window.regularStart, window.regularEnd},
		"3": {window.afterHoursStart, window.afterHoursEnd},
	}

	for sessionType, bounds := range inside {
		before := time.Now().In(hkZone).Format("15:04")
		err := validateSessionWindow(domain.MarketHK, sessionType)
		after := time.Now().In(hkZone).Format("15:04")
		if before != after {
			t.Skipf("minute boundary straddled the call (%s -> %s); verdict is legitimately ambiguous", before, after)
		}

		wantIn := before >= bounds[0] && before <= bounds[1]
		if gotIn := err == nil; gotIn != wantIn {
			t.Errorf("validateSessionWindow(HK, %q) = %v at %s HKT, want nil == %t for window %s-%s",
				sessionType, err, before, wantIn, bounds[0], bounds[1])
		}
	}
}

// TestValidateSessionWindowSkipsNonHKMarkets pins the short-circuits that run
// before the clock is ever consulted, so the fixed zone cannot leak into a
// market whose windows are not Hong Kong wall-clock.
func TestValidateSessionWindowSkipsNonHKMarkets(t *testing.T) {
	for _, market := range []domain.Market{domain.MarketShanghaiConnect, domain.MarketShenzhenConnect, domain.MarketUS} {
		if err := validateSessionWindow(market, "1"); err != nil {
			t.Errorf("validateSessionWindow(%q, \"1\") = %v, want nil", market, err)
		}
	}
}
