// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

// Package services provides the use-case layer for the v-next SDK.
//
// It composes domain models with transport adapters to expose operations
// such as market data queries, account lookups, order submission, and push
// event orchestration.
//
// Service interfaces are declared in this package and implemented by
// pkg/transport.  Callers depend only on the service interfaces.
//
// Rules (per ADR 0010):
//
//   - services may depend only on pkg/domain and service interface types.
//   - transport implementations are injected, not instantiated directly.
//   - HK-market validation (lot sizes, tick schedules, session windows,
//     order-type eligibility, TIF, short-selling flags) lives here, sourced
//     from domain market models.
//   - mutation calls are never retried automatically (ADR 0003).
//   - All blocking operations accept context.Context as the first argument.
package services
