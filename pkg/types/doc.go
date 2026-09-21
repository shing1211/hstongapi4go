// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

// Package types holds the shared domain enums, identifiers, gateway status
// codes, and published platform keys used across the HStong (华盛) SDK.
//
// Values are transcribed from two vendor sources: the official protobuf
// package v2.2.0 (vendored under proto/), which fixes the wire enumerations
// such as NotifyMsgType, and the legacy HStong OpenAPI documentation
// (https://quant-open.hstong.com/api-docs/old/), which documents the request
// and response dictionaries such as exchangeType, entrustBs, and dataType.
//
// Where the vendor did not publish a closed set, a type is marked
// non-exhaustive in its documentation; callers should treat unknown values as
// forward-compatible rather than errors. See docs/SPEC.md for the canonical
// index of these enumerations.
//
// Money and quantities are never represented as binary floats in this module;
// they use string or json.Number. See scripts/check_money.py.
package types
