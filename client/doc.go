// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

// Package client is the entry point of the hstongapi4go SDK for the HStong
// (华盛) Quant OpenAPI local Gateway.
//
// The SDK talks to a Gateway the user installs and runs locally: HTTP POST on
// http://127.0.0.1:11111 and a TCP push channel on 127.0.0.1:11112. The SDK
// never bundles, launches, or manages the Gateway, and it performs no RSA
// signing or request encryption; the Gateway owns the platform credentials (see
// docs/adr/0001-gateway-transport.md and docs/adr/0005-key-model-and-push-verification.md).
//
// # Routes and aliases
//
// Every call targets a canonical route of the form "/<surface>/<Name>", listed
// as Route constants (51 endpoints, see docs/SPEC.md). The Gateway also accepts
// the aliases "<route>Request" and "<route>RequestMsgType"; NormalizePath maps
// those back to the canonical form. Route.Validate rejects an unregistered
// route before any request is sent.
//
// # Codecs
//
// Every endpoint uses the encoding/json codec (client.JSON). The codec is
// obtained from (*Client).JSON; see docs/adr/0002-hybrid-codec.md.
//
// # Usage
//
//	c, err := client.New(
//		client.WithEnv(),
//		client.WithBaseURL("http://127.0.0.1:11111"),
//	)
//	if err != nil {
//		return err
//	}
//	defer c.Close()
//
//	var out myResponse
//	err = c.Do(ctx, "trade/TradeLogin", client.RouteTradeLogin, params, c.JSON(), &out)
//
// # Concurrency and cleanup
//
// A Client is immutable after New and safe for concurrent use. Do issues exactly
// one HTTP request per call and never retries; in particular order mutations are
// never retried (docs/adr/0003-no-auto-retry-orders.md). Close is idempotent,
// releases idle connections, and makes subsequent Do calls fail with
// ErrClientClosed.
package client
