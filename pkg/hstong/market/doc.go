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
package market
