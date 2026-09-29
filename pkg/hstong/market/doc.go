// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

// Package market provides typed managers for the nine market-data pull
// endpoints of the HStong (华盛) Quant OpenAPI Gateway:
//
//	POST /hq/BasicQot                  real-time quote
//	POST /hq/OrderBook                real-time order book
//	POST /hq/KL                       real-time candlesticks
//	POST /hq/TimeShare                real-time time-and-sales
//	POST /hq/Ticker                   real-time tick-by-tick
//	POST /hq/Broker                   real-time broker queue (HK only)
//	POST /hq/UsOptionChainCode        option-chain code list
//	POST /hq/UsOptionChainExpireDate  option-chain expiry list
//	POST /hq/UsOverNightTradeCodes    US overnight-tradable codes
//
// # Usage
//
//	c, err := client.New(client.WithBaseURL("http://127.0.0.1:11111"))
//	if err != nil {
//		return err
//	}
//	defer c.Close()
//
//	m := market.New(c)
//	q, err := m.BasicQot(ctx, market.BasicQotRequest{
//		Security:  []*dto.Security{{DataType: int32(types.DataTypeHKStock), Code: "0700.HK"}},
//		MktTmType: 1,
//	})
//
// The manager holds the shared *client.Client and is safe for concurrent use;
// New does not own the client, so closing it remains the caller's
// responsibility.
//
// # Codec
//
// All nine endpoints use the encoding/json codec (client.JSON). The Gateway's
// HTTP body is plain JSON: the Java SDK (v2.3.0) uses Gson and the Python
// SDK (v2.3.0) uses json.loads against the same protobuf-derived POJOs. The
// Gateway emits int64 counters (volume, turnover, timestamp) as JSON numbers,
// which encoding/json maps directly onto the generated int64 fields. See
// docs/adr/0002-hybrid-codec.md and docs/adr/0007-http-json-codec.md.
//
// # Validation
//
// Every batch request rejects an empty security list and a nil security, and
// Ticker rejects a Limit outside 1..MaxTickerLimit. Rejections are typed
// *errs.Error values carrying StatusInvalidParam and no request is sent.
//
// # Relationship to pkg/services
//
// This package remains the supported default and is not scheduled for removal.
// The v-next layer in pkg/services covers the same endpoints; see
// docs/MIGRATION.md for a side-by-side migration, and docs/adr/0011-v01x-compatibility.md
// for the compatibility guarantee. Migration is opt-in and incremental — the two
// surfaces coexist and nothing here changes behaviour.
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
package market
