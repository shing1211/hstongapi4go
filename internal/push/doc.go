// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

// Package push implements the HStong (华盛) Gateway TCP push channel: the
// 151-byte wire header, the PBNotify envelope, per-notify-type payload decoding,
// and a reconnecting client with a handler registry.
//
// # Wire format
//
// Every message on the push socket is a fixed 151-byte little-endian header
// followed by exactly HeaderSize bytes' worth of header plus bodyLen body bytes:
//
//	offset  size  field
//	1-2     2     szHeaderFlag      ASCII "HS"
//	3-4     2     msgType           int16: 0 heartbeat, 1 request, 2 response, 3 push
//	5       1     protoFmtType      0 = protobuf
//	6       1     protoVer          0
//	7-10    4     serialNo          int32
//	11-14   4     bodyLen           int32 (post-encryption body length)
//	15-142  128   bodySHA1          SHA1WithRSA signature
//	143     1     compressAlgorithm 0 = none
//	144-151 8     reserved          int64
//
// The body of a push frame is a protobuf PBNotify:
//
//	PBNotify{ notifyMsgType uint32, notifyId string, notifyTime uint64, payload google.protobuf.Any }
//
// # Decoding the Any payload
//
// The vendored protobuf definitions declare no proto package, so the Any
// type_url cannot be resolved through the global protobuf type registry in a
// portable way. This package therefore dispatches on PBNotify.notifyMsgType and
// unmarshals payload.Value directly into the concrete generated Go type (see
// Decode). The type_url is never consulted. This is a deliberate deviation from
// docs/DESIGN.md §3.3, recorded in the P04 phase file.
//
// # Signatures
//
// bodySHA1 carries a SHA1WithRSA signature over the raw body bytes. The raw
// signature is preserved on the decoded Header. Verification is opt-in and off
// by default (ADR 0005); enable it with WithVerification, which accepts a PEM
// PUBLIC KEY block, a base64 SPKI string, or raw SPKI DER. With required=true a
// frame whose signature is missing or invalid is dropped and reported on the
// error channel; with required=false it is reported but still delivered. When
// verification is disabled no signature check is performed.
//
// # Backpressure
//
// Handlers are invoked from a per-type dispatcher goroutine fed by a buffered
// channel. When a handler falls behind, the read loop drops the oldest queued
// notification for that type and enqueues the newest (drop-oldest). The read
// loop therefore never blocks on a slow subscriber and never stalls the
// connection. Errors are surfaced the same way on the buffered error channel.
package push
