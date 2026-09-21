// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

// Package mockgateway is an in-repo, offline mock of the HStong (华盛) Quant
// OpenAPI local Gateway. It lets the SDK's real public managers be driven
// end-to-end without the Gateway, credentials, or network access.
//
// # What is mocked
//
//   - The HTTP surface on the Gateway's local port: every one of the 51
//     canonical endpoints from docs/SPEC.md §2 (and the three Gateway route
//     aliases) answers POST with the {"ok":..,"err":..,"data":..} envelope and
//     Content-Type application/json.
//   - The TCP push surface: the server accepts connections and emits
//     151-byte-framed PBNotify messages (magic "HS", msgType=3, protobuf body)
//     through the same internal/push framing helpers the SDK reads with.
//   - Error paths: a per-route error injection answers an endpoint with an
//     ok:false envelope whose err text starts with a documented status code, so
//     the SDK's error mapping and re-login logic can be exercised.
//
// # What is not mocked
//
//   - No authentication, signing, encryption, or device binding: the Gateway
//     owns those, so the mock accepts any body and the trade password is never
//     checked.
//   - No real market data or account state: response bodies are static,
//     wire-level JSON fixtures (see fixtures.go), not live values.
//   - No rate limiting, heartbeats, or reconnect negotiation beyond the push
//     frame format. The mock broadcasts emitted frames to every connected push
//     client; per-topic filtering is left to the SDK's subscriptions.
//   - No Gateway-side signing of the push bodySHA1 field: the header field is
//     left zero because push-signature verification is opt-in and off by
//     default (ADR 0005).
//
// # How tests use it
//
//	srv := mockgateway.New()
//	srv.StartT(t) // starts HTTP and push, closes them at test cleanup
//	c, _ := client.New(client.WithBaseURL(srv.HTTPBaseURL()), client.WithPushAddr(srv.PushAddr()))
//
// The package is test-support code: it may import internal/push and client from
// the same module, but production code must never depend on it.
package mockgateway
