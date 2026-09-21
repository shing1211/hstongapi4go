// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

// Package trade provides the public managers for the HStong (华盛) Quant
// OpenAPI Gateway trading surface: account funds and positions, and the order
// lifecycle (place, cancel, batch cancel, change, and the real and historical
// order/deliver/condition-order queries).
//
// # Codec
//
// Every endpoint in this package is encoded and decoded with the JSON codec
// returned by (*client.Client).JSON, against hand-written structs that carry
// explicit json:"..." tags. The official protobuf package contains no HTTP
// request or response messages for the trade surface, so no proto types are
// used here (see docs/adr/0002-hybrid-codec.md).
//
// # Money and quantities
//
// Money, prices, and quantities are carried as string on both requests and
// responses, never as float64 or float32. The Gateway encodes them as JSON
// strings, and keeping them as strings preserves the exact decimal text that
// was sent (see docs/DESIGN.md §7). A caller that needs to compute parses them
// explicitly, choosing its own numeric precision.
//
// # Session
//
// Trade calls require a logged-in trading session. A Manager accepts an
// optional Session through WithSession; *hstong.SessionManager satisfies the
// interface. When a session is attached, every call first ensures the session
// is live, and a failure that errs.ReLoginRequired reports triggers one
// best-effort re-login before the original error is returned. A Manager with no
// session sends each call directly, which is useful for a caller that owns
// session management itself or for offline tests.
//
// # No auto-retry
//
// The order mutations — Entrust, CancelEntrust, BatchCancelEntrust, and
// ChangeEntrust — issue exactly one HTTP request per call and are never retried
// at any layer. An ambiguous failure (for example a timeout after the request
// was sent) is returned to the caller to reconcile with a real/history query
// before resubmitting (see docs/adr/0003-no-auto-retry-orders.md).
//
// # Pagination
//
// The cursor list endpoints (fund journeys, entrusts, delivers) are queried one
// page at a time; Paginate walks them, honoring the documented page-size cap and
// stop condition.
package trade
