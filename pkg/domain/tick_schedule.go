// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package domain

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

// ScheduleFor implements TickScheduleResolver, so the default HK table can be
// injected anywhere the interface is taken.
//
// The lookup is keyed on the symbol's DataType, which is a data-type-level
// default rather than a per-instrument one. That is a known and recorded
// limitation: HKEX sets a stock's tick by price band, so no single value is
// correct for every HK stock, and the band table is deliberately absent rather
// than guessed. See the deferred price-band work in next-phase.md and
// design-tick-model.md §2.3.
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
