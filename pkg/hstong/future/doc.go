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
package future
