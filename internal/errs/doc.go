// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

// Package errs defines the SDK's typed error taxonomy for the HStong Gateway.
//
// Every failure surfaced by internal/transport is an *Error carrying the
// Gateway status code (types.StatusCode), a coarse Category used by the
// resilience layer, a human-readable message, the operation (endpoint) that
// failed, and the wrapped cause. Callers match errors with errors.Is and
// errors.As and the CodeOf/CategoryOf/Retryable/ReLoginRequired/IsDeprecated
// helpers; library code never matches on error strings.
//
// Retry policy is deliberately conservative: mutations are never retried
// (see docs/adr/0003-no-auto-retry-orders.md), so only transient
// connection/timeout conditions and the documented retryable gateway codes are
// reported by Retryable. ReLoginRequired reports the codes that invalidate a
// session and therefore must trigger a single-flight re-login.
package errs
