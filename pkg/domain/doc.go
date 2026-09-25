// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

// Package domain holds the pure domain model for the v-next layer.
//
// It contains:
//
//   - decimal-backed financial types: Money, Price, Quantity, Rate
//   - typed identifiers: AccountID, OrderID, EntrustID, ContractID
//   - HK market models: Symbol, LotSize, TickSchedule, MarketSession
//   - order state machine and lifecycle events
//   - push event types for the real-time stream
//
// Rules (per ADR 0010):
//
//   - domain has zero imports from transport, internal, or client packages.
//   - domain types receive injected dependencies (logger, clock, tracer) via
//     constructor options, not via global state.
//   - wire-to-domain conversion lives in this package's mappers, which accept
//     generated DTOs. ADR 0010 intended that conversion to happen in the
//     transport layer; see the Deviations table in
//     docs/runs/2026-09-23-hstong-enterprise-sdk/ARCHITECTURE.md. The domain
//     value types themselves still do not hold string/json.Number wire
//     representations.
//   - decimal.Decimal is used for all financial arithmetic (ADR 0008); float64
//     is never used for monetary values.
//
// The domain layer is stable once declared; no per-endpoint wire knowledge
// lives here.
package domain
