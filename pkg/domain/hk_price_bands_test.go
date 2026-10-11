// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package domain

import (
	"testing"

	"github.com/shing1211/hstongapi4go/pkg/types"
)

// TestScheduleForPriceUsesTheHKEXBandTable pins the band lookup at both edges
// of every band, plus the floor, the ceiling, and the deliberate
// under-constraining below the published floor.
//
// Every band is inclusive at its upper bound ("Over X to Y" is (X, Y]), so
// each boundary pair is one value inside a band and one value past it. A
// single off-by-one in the comparison order shows up as a failure on the
// second row of a pair.
func TestScheduleForPriceUsesTheHKEXBandTable(t *testing.T) {
	schedule := TickSchedule{}
	stock := Symbol{Market: MarketHK, DataType: types.DataTypeHKStock}

	tests := []struct {
		name     string
		price    string
		wantTick string
		wantOK   bool
		why      string
	}{
		{"lowest band upper is inclusive", "0.25", "0.001", true,
			"from 0.01 to 0.25 pays 0.001"},
		{"just above the lowest band", "0.251", "0.005", true,
			"over 0.25 to 10 pays 0.005"},
		{"second band upper is inclusive", "10", "0.005", true,
			"the Phase 2 merged band ends at 10"},
		{"third band opens exclusively", "10.001", "0.010", true,
			"over 10 to 20 pays 0.010"},
		{"third band upper", "20", "0.010", true, ""},
		{"fourth band", "20.001", "0.020", true, ""},
		{"fourth band upper", "50", "0.020", true, ""},
		{"fifth band", "50.001", "0.050", true, ""},
		{"fifth band upper", "100", "0.050", true, ""},
		{"sixth band", "100.001", "0.100", true, ""},
		{"sixth band upper", "200", "0.100", true, ""},
		{"seventh band", "200.001", "0.200", true, ""},
		{"seventh band upper", "500", "0.200", true, ""},
		{"eighth band", "500.001", "0.500", true, ""},
		{"eighth band upper", "1000", "0.500", true, ""},
		{"ninth band", "1000.001", "1.000", true, ""},
		{"ninth band upper", "2000", "1.000", true, ""},
		{"tenth band", "2000.001", "2.000", true, ""},
		{"tenth band upper", "5000", "2.000", true, ""},
		{"eleventh band", "5000.001", "5.000", true, ""},
		{"top band upper is inclusive", "9995", "5.000", true,
			"the published table ends at 9,995.00"},
		{"above the table", "9995.01", "", false,
			"beyond the ceiling there is no answer, and inventing one is the defect"},
		{"below the published floor", "0.001", "0.001", true,
			"under-constrained on purpose; the exchange decides"},
		{"zero resolves to the lowest band", "0", "0.001", true,
			"the sign check lives in ValidateOn, not here"},
		{"negative resolves to the lowest band", "-1", "0.001", true,
			"band lookup never rejects; ValidateOn rejects the sign"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tick, ok := schedule.ScheduleForPrice(stock, MustNewPrice(tc.price, "0"))
			if ok != tc.wantOK || tick != tc.wantTick {
				t.Errorf("ScheduleForPrice(HK stock, %s) = (%q, %v), want (%q, %v): %s",
					tc.price, tick, ok, tc.wantTick, tc.wantOK, tc.why)
			}
		})
	}
}

// TestBandUpperBoundsSitOnTheirOwnGrid is the property that makes the band
// table usable by the order path at all: every band's inclusive upper bound is
// a multiple of that band's tick, so the boundary price itself is accepted.
// A band whose upper bound were off its own grid would reject the exact price
// the exchange names as the last price on it.
func TestBandUpperBoundsSitOnTheirOwnGrid(t *testing.T) {
	uppers := []string{"0.25", "10", "20", "50", "100", "200", "500", "1000", "2000", "5000", "9995"}

	for _, upper := range uppers {
		tick, ok := TickSchedule{}.ScheduleForPrice(
			Symbol{Market: MarketHK, DataType: types.DataTypeHKStock},
			MustNewPrice(upper, "0"))
		if !ok {
			t.Fatalf("ScheduleForPrice(%s) reported no opinion: the band table must "+
				"cover every published upper bound", upper)
		}
		if err := MustNewPrice(upper, "0").ValidateOn(tick); err != nil {
			t.Errorf("price %s, the inclusive upper bound of its band, fails "+
				"ValidateOn against that band's tick %s: %v", upper, tick, err)
		}
	}
}

// TestScheduleForPriceLeavesNonStockTypesOnTheirDataTypeGrid covers the
// exclusion scope: HKEX bands equities, REITs and equity warrants — which the
// DataType enum maps to HKStock — and leaves ETFs, bonds, CBBCs and the rest
// on fixed increments. Those must keep answering through the DataType table
// rather than silently join the stock bands.
func TestScheduleForPriceLeavesNonStockTypesOnTheirDataTypeGrid(t *testing.T) {
	schedule := TickSchedule{}
	mid := MustNewPrice("388.2", "0")

	tests := []struct {
		name     string
		sym      Symbol
		wantTick string
		wantOK   bool
	}{
		{"HK ETF stays at 0.001", Symbol{Market: MarketHK, DataType: types.DataTypeHKETF}, "0.001", true},
		{"HK warrant stays at 0.001", Symbol{Market: MarketHK, DataType: types.DataTypeHKWarrant}, "0.001", true},
		{"HK CBBC stays at 0.001", Symbol{Market: MarketHK, DataType: types.DataTypeHKCBBC}, "0.001", true},
		{"HK bond stays at 0.0001", Symbol{Market: MarketHK, DataType: types.DataTypeHKBond}, "0.0001", true},
		{"HK index has no opinion", Symbol{Market: MarketHK, DataType: types.DataTypeHKIndex}, "", false},
		{"US with an HK DataType", Symbol{Market: MarketUS, DataType: types.DataTypeHKStock}, "", false},
		{"Shenzhen Connect", Symbol{Market: MarketShenzhenConnect, DataType: types.DataTypeHKStock}, "", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tick, ok := schedule.ScheduleForPrice(tc.sym, mid)
			if ok != tc.wantOK || tick != tc.wantTick {
				t.Errorf("ScheduleForPrice(%+v, %s) = (%q, %v), want (%q, %v)",
					tc.sym, mid, tick, ok, tc.wantTick, tc.wantOK)
			}
		})
	}
}

// TestScheduleForRemainsPriceIndependent guards the other half of the split:
// lot checks and any non-priced caller still get one answer per instrument, and
// for an HK stock that answer is the lowest band's 0.001 — correct only in the
// bottom band, which is why price validation must go through ScheduleForPrice.
func TestScheduleForRemainsPriceIndependent(t *testing.T) {
	schedule := TickSchedule{}
	stock := Symbol{Market: MarketHK, DataType: types.DataTypeHKStock}

	lot, tick, ok := schedule.ScheduleFor(stock)
	if !ok || lot != 100 || tick != "0.001" {
		t.Errorf("ScheduleFor(HK stock) = (%d, %q, %v), want (100, %q, true): the "+
			"price-independent answer is unchanged", lot, tick, ok, "0.001")
	}
}
