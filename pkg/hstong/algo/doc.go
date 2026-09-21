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
// amount/price, or an action code outside the documented set. In particular a
// mutation that fails validation never reaches the Gateway.
//
// All Manager methods are safe for concurrent use.
package algo
