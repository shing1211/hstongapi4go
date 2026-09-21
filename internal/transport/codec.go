// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package transport

import "encoding/json"

// Codec converts the params/data payload of a Gateway call to and from bytes.
// The codec is chosen per endpoint by the router, never as a global mode (see
// docs/adr/0002-hybrid-codec.md): the envelope itself is always encoding/json,
// while a Codec only sees the inner payload.
type Codec interface {
	// Marshal encodes v into the wire representation of the endpoint.
	Marshal(v any) ([]byte, error)
	// Unmarshal decodes data into v, which must be a non-nil pointer.
	Unmarshal(data []byte, v any) error
}

// JSONCodec encodes and decodes payloads with encoding/json. It is used for
// the hand-written trade, futures, algo, assets, and session HTTP bodies, where
// money and quantities are represented as strings rather than binary floats.
type JSONCodec struct{}

// Marshal returns the JSON encoding of v.
func (JSONCodec) Marshal(v any) ([]byte, error) { return json.Marshal(v) }

// Unmarshal decodes the JSON-encoded data into v, which must be a non-nil pointer.
func (JSONCodec) Unmarshal(data []byte, v any) error { return json.Unmarshal(data, v) }
