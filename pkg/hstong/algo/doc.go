// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

// Package algo provides typed access to the HStong (华盛) Quant OpenAPI
// algorithm-trading (算法交易) surface. It covers the seven Gateway endpoints of
// docs/SPEC.md §2.7: place, cancel (master and child), modify, operate, and the
// two paginated/cursor reads.
//
// # Master and child orders
//
// An algorithm order is submitted once as a master order (母单) and then sliced
// by the platform into child orders (子单) according to the chosen strategy.
// Each slice is an ordinary exchange entrust tied to the master's order ID:
//
//   - AddOrder creates a master order and returns its order ID.
//   - ChangeOrder updates a live master's price, quantity, and strategy tuning.
//   - ActionOrder starts, stops, suspends, or resumes a master (see Action).
//   - CancelOrder cancels the whole master; CancelEntrust cancels one child.
//   - QueryOrderList lists master orders for a date range.
//   - QueryEntrustIDList lists the child entrust IDs belonging to one master on
//     one trade date.
//
// A child is addressed by the pair (orderId, entrustId). Cancelling the master
// does not use the child's entrustId, and cancelling a child does not affect the
// master's remaining schedule.
//
// # Codec
//
// The algorithm HTTP bodies are hand-written Go structs with explicit JSON tags
// and are encoded with the client's encoding/json codec (Client.JSON), exactly
// as ADR 0002 prescribes: the official protobuf package carries no HTTP
// request/response messages, and inventing .proto files for these bodies would
// guarantee drift against the Gateway. Money, prices, and quantities are
// therefore string fields (for example EntrustPrice, EntrustAmount, CumQty,
// AvgPx), never float64.
//
// # Mutations are never retried
//
// AddOrder, CancelOrder, CancelEntrust, ChangeOrder, and ActionOrder are order
// mutations and are issued exactly once. The client never retries them (ADR
// 0003); an ambiguous failure (for example a timeout after the request was
// sent) is returned to the caller, who must reconcile by querying the master
// and child lists before resubmitting.
//
// # Validation
//
// Every method validates its arguments before any request is sent and returns an
// error wrapping ErrInvalidParams for an empty required field, a malformed
// amount/price, or a code outside a documented closed set. In particular a
// mutation that fails validation never reaches the Gateway.
//
// The closed sets are entrustType, sessionType, strategyParam.sensitivity,
// action, exchangeType, and entrustBs; a value outside one is rejected locally
// rather than forwarded. Two fields are deliberately not validated, because the
// SDK cannot know their correct answer: targetStrategy, whose documented codes
// contradict each other (see Strategy), and — because the field is optional on
// that one request — an omitted exchangeType on QueryOrderList.
//
// All Manager methods are safe for concurrent use.
//
// # Relationship to pkg/services
//
// This package remains the supported default and is not scheduled for removal.
// The v-next layer in pkg/services covers the same endpoints; see
// docs/MIGRATION.md for a side-by-side migration, and docs/adr/0011-v01x-compatibility.md
// for the compatibility guarantee. Migration is opt-in and incremental — the two
// surfaces coexist and nothing here changes behaviour. Algo orders are mutations
// and are never auto-retried, on either surface.
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
package algo
