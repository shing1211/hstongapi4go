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
// # Codec choice
//
// Requests and responses are encoded with the hand-written JSON codec
// (client.JSON), not with ProtoJSON. The Gateway's HTTP body is generic JSON:
// both official Gateway SDKs of the same protocol generation decode it with a
// plain JSON parser — the Java SDK (v2.3.0) uses Gson directly against the
// protobuf-derived POJOs and the Python SDK (v2.3.0) uses json.loads — and
// neither applies protobuf's JSON mapping (JsonFormat/protojson) to the HTTP
// payload. The Gateway therefore emits int64 counters (volume, turnover,
// timestamp) as JSON numbers, which encoding/json maps onto the generated
// int64 fields; protojson would instead emit them as JSON strings.
//
// Concretely, each response is a hand-written wrapper with the exact
// documented field names whose element types are the generated gen/hq/dto
// protobuf messages (for example "data.basicQot" decodes into
// []*dto.BasicQot). This is the per-endpoint codec choice allowed by
// docs/adr/0002-hybrid-codec.md. Should a Gateway build ever emit
// protojson-style quoted integers, the affected endpoint's codec must switch
// to client.ProtoJSON over the generated DTOs; the wrapper itself is not a
// proto message, so that fallback requires decoding the DTO lists from
// json.RawMessage elements.
//
// # Validation
//
// Every batch request rejects an empty security list and a nil security, and
// Ticker rejects a Limit outside 1..MaxTickerLimit. Rejections are typed
// *errs.Error values carrying StatusInvalidParam and no request is sent.
package market
