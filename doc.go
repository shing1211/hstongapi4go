// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

// Package hstongapi4go is an unofficial, community-maintained Go client for
// the HStong (华盛) Quant OpenAPI Gateway.
//
// It speaks to a locally installed Gateway over HTTP POST
// (default http://127.0.0.1:11111) for request/response calls and over a
// raw TCP push channel (default 127.0.0.1:11112) for real-time market and
// trade notifications. The Gateway itself is installed and operated by the
// user; this module never bundles, downloads, or launches it.
//
// The public surface is organised around a small number of managers:
//
//   - market: quotes, order books, k-lines, time-share, tickers, brokers,
//     option chains, overnight trading, and subscribe/unsubscribe;
//   - trade: session, assets, positions, and the order lifecycle;
//   - future: futures products, funds, positions, and orders;
//   - algo: strategy order master/sub-order queries and operations;
//   - stream: multiplexed access to market, trade, and futures push topics.
//
// Envelopes and the trade/futures/algo request bodies use encoding/json with
// money and quantities represented as strings (never binary floats), while
// market data and push payloads use the generated protobuf types and
// protojson. See the repository design notes for the rationale.
//
// This project is in alpha and is not affiliated with, endorsed by, or
// supported by HStong / 华盛. See DISCLAIMER.md.
package hstongapi4go
