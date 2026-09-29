// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

// Package future provides typed access to the eleven HStong (华盛) futures
// trading endpoints of the local Quant OpenAPI Gateway and a decoder for the
// futures trade-delivery push.
//
// The futures surface is split into funds/positions queries, order mutations,
// and order/fill queries. Every request and response body is a hand-written Go
// struct with explicit json tags, decoded by the client's JSON codec
// (docs/adr/0002-hybrid-codec.md); money and quantities are carried as strings,
// or as documented integer counts, and never as binary floats.
//
// # Mutations
//
// Entrust, CancelEntrust, and ModifyEntrust are order mutations. Following
// docs/adr/0003-no-auto-retry-orders.md each method issues exactly one attempt
// and never retries: the caller owns reconciliation after an ambiguous failure.
//
// # Push
//
// Futures trade deliveries arrive over the TCP push channel with the
// PBNotify message type types.FuturesTradeStockDeliverMsgType (value 2) and a
// payload that reuses the generated TradeStockDeliverNotify message. FromDeliverNotify
// maps that payload into the typed DeliverNotification view; the package does
// not own the transport.
//
// A Manager is safe for concurrent use by multiple goroutines.
//
// # Relationship to pkg/services
//
// This package remains the supported default and is not scheduled for removal.
// The v-next layer in pkg/services covers the same endpoints; see
// docs/MIGRATION.md for a side-by-side migration, and docs/adr/0011-v01x-compatibility.md
// for the compatibility guarantee. Migration is opt-in and incremental — the two
// surfaces coexist and nothing here changes behaviour.
//
// The entrustBs value set differs by endpoint, and this is a futures-only
// restriction. The vendor documents POST /trade/FuturesEntrust as
// "entrustBs": "string 1:买入,2:卖出" - two values - while the cash
// POST /trade/TradeEntrust separately offers 3 and 4 as explicit input values.
// Futures has no open/close field, so the Gateway infers that from the position.
// This package therefore accepts only 1 and 2 on a futures mutation, and refuses
// 3 and 4 locally with a typed error rather than sending a request the Gateway
// will reject. It accepted 1-4 until v1.0.1, when the vendor's per-endpoint
// documentation resolved what had been an open question on both surfaces
// (tracker C1b). See docs/SPEC.md §7.4.
//
// pkg/services reached feature parity on 2026-09-26, which is the condition
// ADR 0011 sets for announcing a deprecation. The machine-readable
// // Deprecated: marker landed with v1.0.0, on this package's Manager type
// rather than here, so `go doc` shows it on the thing callers name.
//
// It is also quieter than a deprecation usually is. staticcheck's SA1019 fires
// only where a deprecated identifier is written out, so the constructor call
// most callers make produces no warning at all; only code that names the type
// explicitly is nagged. Nothing here is scheduled for removal - this surface
// still works, is still the default, and the marker is advisory. The
// machine-readable marker was deliberately deferred to v1.0.0 so that a v0.1.x
// patch release would not make every existing consumer's build emit warnings
// they did not ask for.
package future
