// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package domain

import (
	"testing"

	"github.com/shing1211/hstongapi4go/pkg/types"
)

// TestValidateOn covers the predicate the order path depends on.
//
// It is here rather than in pkg/services because this is a property of the type:
// ValidateOn must behave like Validate's step check for every tick except the one
// it takes as an argument. Measured from pkg/domain alone the function is at 0% —
// it is only exercised through the service layer — so a package-local test is what
// keeps the two definitions from drifting apart unnoticed.
func TestValidateOn(t *testing.T) {
	tests := []struct {
		name    string
		price   string
		tick    string
		wantErr bool
		why     string
	}{
		// The discriminating rows: the same value against two different ticks.
		{"on the grid", "388.05", "0.001", false, "388.050 / 0.001 = 388050, an integer"},
		{"off the grid", "388.0501", "0.001", true, "the 0.0001 remainder"},
		{"off a finer grid", "388.0501", "0.0001", false, "and a multiple of this one"},
		{"on a coarser grid", "388.05", "0.01", false, "388.05 / 0.01 = 3885"},

		// Unknown tick: skip rather than invent a grid. This is the distinction
		// that separates ValidateOn from Validate, where the caller's own zero tick
		// used to exempt a whole order.
		{"empty tick skips", "388.0501", "", false, "unknown, not zero"},
		{"zero tick skips", "388.0501", "0", false, "a zero grid would reject every value"},
		{"negative tick skips", "388.0501", "-0.001", false, "and would be nonsense as a grid"},

		// The sign check always runs, whatever the tick says.
		{"negative price with empty tick", "-1", "", true, "sign is checked before the tick is read"},
		{"negative price on grid", "-1", "0.001", true, "a signed value is never a multiple"},
		{"zero price on grid", "0", "0.001", false, "zero divides evenly and is not negative"},

		// A tick the caller cannot have meant. Reported rather than ignored: a
		// silent skip here would be the same failure as an empty string.
		{"unparseable tick", "1", "nonsense", true, "an invalid tick is a caller error"},
		{"empty price with empty tick", "0", "", false, "the early return must not skip the sign test"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := MustNewPrice(tc.price, "0").ValidateOn(tc.tick)
			if tc.wantErr && err == nil {
				t.Errorf("ValidateOn(%q, %q) = nil, want an error: %s", tc.price, tc.tick, tc.why)
			}
			if !tc.wantErr && err != nil {
				t.Errorf("ValidateOn(%q, %q) = %v, want nil: %s", tc.price, tc.tick, err, tc.why)
			}
		})
	}
}

// TestValidateOnRejectsExactlyWhereValidateDoes is the property the two methods
// are supposed to share, stated as a comparison rather than as two tables.
//
// ValidateOn(t) and Validate(true) on a Price built with tick t must agree. If
// they ever disagree, one of them is wrong, and the direction of the difference
// names which.
func TestValidateOnRejectsExactlyWhereValidateDoes(t *testing.T) {
	values := []string{"0", "0.001", "388.05", "388.0501", "388.05015", "0.0005", "-1", "123.456"}
	ticks := []string{"0.001", "0.0001", "0.01"}

	for _, v := range values {
		for _, tick := range ticks {
			own := MustNewPrice(v, tick).Validate(true)
			supplied := MustNewPrice(v, "0").ValidateOn(tick)

			if (own == nil) != (supplied == nil) {
				t.Errorf("price %q against tick %q: Validate(true) on a Price carrying that "+
					"tick = %v, ValidateOn with it supplied = %v. The two must agree, because "+
					"ValidateOn is the same step check with the tick taken as an argument",
					v, tick, own, supplied)
			}
		}
	}
}

// TestTheDefaultScheduleAnswersOnlyForHK pins the default resolver's contract at
// the type that owns it, for the same reason as TestValidateOn: it is 0% when
// pkg/domain is measured on its own.
func TestTheDefaultScheduleAnswersOnlyForHK(t *testing.T) {
	schedule := TickSchedule{}

	tests := []struct {
		name      string
		sym       Symbol
		wantLot   uint64
		wantTick  string
		wantKnown bool
	}{
		{"HK stock", Symbol{Market: MarketHK, DataType: types.DataTypeHKStock}, 100, "0.001", true},
		{"HK ETF", Symbol{Market: MarketHK, DataType: types.DataTypeHKETF}, 100, "0.001", true},
		{"HK bond", Symbol{Market: MarketHK, DataType: types.DataTypeHKBond}, 1, "0.0001", true},
		{"HK index has no entry", Symbol{Market: MarketHK, DataType: types.DataTypeHKIndex}, 0, "", false},
		{"US with an HK DataType", Symbol{Market: MarketUS, DataType: types.DataTypeHKStock}, 0, "", false},
		{"Shenzhen Connect", Symbol{Market: MarketShenzhenConnect, DataType: types.DataTypeHKStock}, 0, "", false},
		{"no market is treated as HK", Symbol{DataType: types.DataTypeHKStock}, 100, "0.001", true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			lot, tick, ok := schedule.ScheduleFor(tc.sym)
			if ok != tc.wantKnown || lot != tc.wantLot || tick != tc.wantTick {
				t.Errorf("ScheduleFor(%+v) = (%d, %q, %v), want (%d, %q, %v)", tc.sym,
					lot, tick, ok, tc.wantLot, tc.wantTick, tc.wantKnown)
			}
		})
	}
}
