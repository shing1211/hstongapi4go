// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

// Package transport executes HStong Gateway HTTP requests.
//
// It owns the request/response envelope
// ({"timeout_sec":10,"params":{...}} / {"ok":true,"err":"","data":{...}}),
// per-endpoint Codec dispatch, and the mapping from a Gateway failure to a
// typed *errs.Error. The transport issues exactly one attempt per call and
// never retries: retry policy, including the exclusion of order mutations, is
// decided by internal/resilience and ADR 0003, not here.
//
// A Transport is immutable after construction and safe for concurrent use; it
// holds no per-request state. The caller supplies a concrete path (route-alias
// resolution is the client's responsibility) and a Codec (JSONCodec for
// hand-written trade/futures/algo/assets/session bodies, ProtoJSONCodec for
// proto-backed market data). A nil Codec defaults to JSONCodec.
package transport
