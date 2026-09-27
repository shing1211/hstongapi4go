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
// # Validation
//
// Every Manager method validates its request before any HTTP request is sent and
// returns a typed "1016" invalid-parameter error, matchable with errs.CodeOf,
// when a field is empty, malformed, or outside a documented closed set. The
// closed sets are exchangeType — K (Hong Kong), P (US), v (Shenzhen Connect),
// t (Shanghai Connect), which is case-sensitive — and entrustBs — 1 (open long),
// 2 (close long), 3 (close short), 4 (open short). All four directions are
// valid; 3 and 4 are the short-selling directions.
//
// A value outside either set is rejected locally rather than forwarded, because
// exchangeType names the book an order, cancel, or change resolves against and a
// direction the SDK does not recognise is a request it knows it does not mean. In
// particular a mutation that fails validation never reaches the Gateway, and is
// never retried (see the no-auto-retry section below).
//
// Positions is the one request whose exchangeType is optional: an absent market
// asks the Gateway for every market, so it is accepted, while a supplied one is
// checked against the same closed set as everywhere else. Every other request
// type in this package requires the field. The response types — OrderVo,
// CondOrderVo, HoldsVo — carry Gateway-supplied exchangeType and entrustBs
// values and are deliberately not validated; validating inbound data would turn
// a vendor-side surprise into a decode error, which is a different policy from
// validating outbound requests.
//
// The order type and session type remain emptiness-only, because their
// documented code sets are not published as closed; see types.EntrustType.
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
