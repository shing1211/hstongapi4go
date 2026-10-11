// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package domain

import (
	"github.com/shopspring/decimal"
)

// hkPriceBand is one row of the HKEX minimum-price-increment table: every price
// up to and including upper pays the band's tick.
type hkPriceBand struct {
	upper decimal.Decimal
	tick  string
}

// hkStockPriceBands is the HKEX minimum-price-increment table for HK stock
// cash equities, in force from 2026-08-03.
//
// Source, cited at implementation time rather than recalled: HKEX "Reduction of
// Minimum Spreads — Frequently Asked Questions (Phase 2)", document version
// 2026-05-08, and the December 2024 consultation conclusions, which set the
// Phase 2 launch date of 2026-08-03 (circular CT/104-26 of 2026-07-14).
//
// FAQ (PDF):
// https://www.hkex.com.hk/-/media/HKEX-Market/Services/Trading/Securities/Overview/Trading-Mechanism/Reduction-of-Minimum-Spreads/Reduction-of-Minimum-Spreads-FAQP2_E.pdf
//
// Conclusions (PDF):
// https://www.hkex.com.hk/-/media/HKEX-Market/News/Market-Consultations/2016-Present/June-2024-Review-of-Minimum-Spreads/Conclusions-Dec-2024/cp202406cc.pdf
//
// Bands are inclusive at the upper bound: "Over 0.25 to 10" is (0.25, 10].
// Scope, as published: equities, REITs and equity warrants, which this SDK's
// DataType enum maps to DataTypeHKStock. Exchange-traded funds, debt
// securities, equity tradeable options, inline warrants and structured
// products (CBBCs) are excluded by HKEX and keep the fixed DataType tick in
// DefaultHKTickSchedule.
//
// This table is a snapshot of a rule HKEX may revise. A caller with a live
// instrument master should inject a PricedTickScheduleResolver via
// services.WithTickSchedule rather than trust the snapshot.
var hkStockPriceBands = []hkPriceBand{
	{decimal.RequireFromString("0.25"), "0.001"},
	{decimal.RequireFromString("10"), "0.005"},
	{decimal.RequireFromString("20"), "0.010"},
	{decimal.RequireFromString("50"), "0.020"},
	{decimal.RequireFromString("100"), "0.050"},
	{decimal.RequireFromString("200"), "0.100"},
	{decimal.RequireFromString("500"), "0.200"},
	{decimal.RequireFromString("1000"), "0.500"},
	{decimal.RequireFromString("2000"), "1.000"},
	{decimal.RequireFromString("5000"), "2.000"},
	{decimal.RequireFromString("9995"), "5.000"},
}

// hkStockTickAtPrice answers the HKEX band question for one order price.
//
// The first band's upper bound is 0.25, so every price at or below it — the
// published floor is 0.01, but also zero, a negative, or any sub-floor value —
// resolves to the lowest tick 0.001. That is deliberate under-constraining:
// Price.ValidateOn's sign check still rejects a negative, and a positive
// sub-floor price passes the grid test and is left to the exchange, which is
// the honest failure mode for a price the table does not cover. Fabricating a
// rejection would be the invented-tick defect in the other direction.
//
// A price above the last band's upper bound reports ok false. The table ends at
// 9,995.00 and the SDK declines to invent a coarser grid beyond it; the caller
// must skip the check and let the exchange decide.
func hkStockTickAtPrice(price Price) (tick string, ok bool) {
	p := price.Decimal()
	for _, band := range hkStockPriceBands {
		if p.LessThanOrEqual(band.upper) {
			return band.tick, true
		}
	}
	return "", false
}
