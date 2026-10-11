// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package domain

import (
	"github.com/shing1211/hstongapi4go/pkg/types"
)

// TickScheduleResolver resolves the lot size and minimum price increment for one
// instrument.
//
// It exists because the tick is a property of the instrument, not of the order.
// An SDK that validates a price against a grid it invented accepts orders the
// exchange rejects, and — on a read path — rounds values the Gateway actually
// sent. See design-tick-model.md §1 in
// docs/runs/2026-09-26-vnext-parity-wire/ for the defect this type addresses.
//
// The name avoids TickSchedule, which is the concrete table type in symbol.go.
// The distinction is deliberate: TickSchedule is data, this is a lookup.
//
// # Contract
//
// ScheduleFor reports ok false when it has no opinion about the symbol. That is
// the honest answer for an instrument it does not cover, and it must not be
// reported as a tick of "0.001" or any other default: a fabricated grid is the
// defect, not the absence of one. A caller receiving ok false should skip the
// tick-specific check and let the exchange decide, rather than substitute a
// number of its own.
type TickScheduleResolver interface {
	// ScheduleFor returns the lot size and tick for sym. ok is false when the
	// resolver does not cover sym, in which case lot and tick are zero and empty
	// and must be ignored.
	ScheduleFor(sym Symbol) (lot uint64, tick string, ok bool)
}

// PricedTickScheduleResolver is a TickScheduleResolver that can additionally
// answer the tick as a function of the order price.
//
// HKEX sets an HK stock's minimum price increment by price band, so for those
// instruments the tick is a property of (instrument, price), not of the
// instrument alone. Order validation prefers this method when the resolver
// implements it; a plain TickScheduleResolver keeps the price-independent
// answer and remains accepted, so adopting the priced form is additive.
//
// The ok contract matches ScheduleFor: false means "no opinion" and the caller
// must skip the check rather than substitute a default.
type PricedTickScheduleResolver interface {
	TickScheduleResolver

	// ScheduleForPrice returns the minimum price increment for sym at the given
	// order price. ok is false when the resolver has no opinion about that
	// (symbol, price) pair, in which case tick is empty and must be ignored.
	ScheduleForPrice(sym Symbol, price Price) (tick string, ok bool)
}

// ScheduleFor implements TickScheduleResolver, so the default HK table can be
// injected anywhere the interface is taken.
//
// The lookup is keyed on the symbol's DataType, which is a data-type-level
// default rather than a per-instrument one. For an HK stock the real answer is
// priced — HKEX sets the increment by band, see hk_price_bands.go — and
// ScheduleFor's 0.001 is only correct in the lowest band. It survives as the
// price-independent answer for lot checks and other callers that need a single
// number; order price validation must use ScheduleForPrice, which this type
// also implements.
func (t TickSchedule) ScheduleFor(sym Symbol) (lot uint64, tick string, ok bool) {
	// The default tables are HK-only, so a symbol from another market is not
	// answered from them even if its DataType happens to collide.
	if sym.Market != MarketHK && sym.Market != "" {
		return 0, "", false
	}
	schedule := DefaultHKTickSchedule(sym.DataType)
	if schedule.TickSize == "" {
		return 0, "", false
	}
	return schedule.Lot, schedule.TickSize, true
}

// ScheduleForPrice implements PricedTickScheduleResolver on the default table.
//
// The market guard matches ScheduleFor. An HK stock is answered from the HKEX
// price-band table keyed on the order price; every other covered type falls
// back to ScheduleFor's DataType answer, because HKEX does not band those
// instruments. ok false means no opinion for that (symbol, price) pair — an HK
// stock priced above the table's ceiling, or a type the table does not cover.
func (t TickSchedule) ScheduleForPrice(sym Symbol, price Price) (tick string, ok bool) {
	if sym.Market != MarketHK && sym.Market != "" {
		return "", false
	}
	if sym.DataType == types.DataTypeHKStock {
		return hkStockTickAtPrice(price)
	}
	_, tick, ok = t.ScheduleFor(sym)
	return tick, ok
}
