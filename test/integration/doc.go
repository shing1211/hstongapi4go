// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

// Package integration contains env-gated integration tests that run against a
// real, locally installed HStong (华盛) Quant OpenAPI Gateway.
//
// The suite is skipped unless HSTONG_INTEGRATION=1 is set and a Gateway is
// running on the configured HTTP and TCP-push addresses with a logged-in
// account. It is never run in CI: the offline suite (`go test ./...`) must stay
// credential-free and network-free, and every test in this package calls
// t.Skip up front when the gate is unset.
//
// These tests exist to confirm the wire assumptions the offline mock
// (test/mockgateway) cannot prove:
//
//   - the int64 representation (JSON number versus quoted string) the Gateway
//     actually sends in market-domain payloads, which ADR 0007 assumes is a
//     number;
//   - the envelope nesting of /trade/TradeQueryMaxAvailableAsset and the shape
//     of /trade/TradeQueryHoldsList;
//   - the real TCP push path, once, from subscribe to a decoded Event;
//   - an opt-in, single-attempt order mutation followed by a cancel.
//
// See README.md in this directory for the environment matrix and the exact run
// commands. Findings from a run of this suite belong in
// docs/runs/2026-09-21-hstong-full-surface/evidence/, not in silent edits to
// this package's expectations.
package integration
