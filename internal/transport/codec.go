// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package transport

import (
	"encoding/json"
	"errors"
	"fmt"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

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

// ErrNotProtoMessage is returned by ProtoJSONCodec when the value passed to
// Marshal or Unmarshal does not implement proto.Message. It is wrapped with the
// offending Go type so errors.Is(err, ErrNotProtoMessage) keeps working.
var ErrNotProtoMessage = errors.New("transport: value is not a proto.Message")

// JSONCodec encodes and decodes payloads with encoding/json. It is used for
// the hand-written trade, futures, algo, assets, and session HTTP bodies, where
// money and quantities are represented as strings rather than binary floats.
type JSONCodec struct{}

// Marshal returns the JSON encoding of v.
func (JSONCodec) Marshal(v any) ([]byte, error) {
	return json.Marshal(v)
}

// Unmarshal decodes the JSON-encoded data into v, which must be a non-nil
// pointer.
func (JSONCodec) Unmarshal(data []byte, v any) error {
	return json.Unmarshal(data, v)
}

// ProtoJSONCodec encodes and decodes market DTOs with
// google.golang.org/protobuf/encoding/protojson. It is used for the
// proto-backed market data payloads of the nine pull endpoints and the
// subscribe endpoints. Unknown fields are rejected (DiscardUnknown is false) so
// that Gateway drift is surfaced rather than silently dropped.
type ProtoJSONCodec struct{}

// Marshal encodes v with protojson. It returns ErrNotProtoMessage (wrapped with
// the offending type) when v does not implement proto.Message.
func (ProtoJSONCodec) Marshal(v any) ([]byte, error) {
	msg, ok := v.(proto.Message)
	if !ok {
		return nil, fmt.Errorf("%w: %T", ErrNotProtoMessage, v)
	}
	return protojson.Marshal(msg)
}

// Unmarshal decodes data into v with protojson, rejecting unknown fields. It
// returns ErrNotProtoMessage (wrapped with the offending type) when v does not
// implement proto.Message.
func (ProtoJSONCodec) Unmarshal(data []byte, v any) error {
	msg, ok := v.(proto.Message)
	if !ok {
		return fmt.Errorf("%w: %T", ErrNotProtoMessage, v)
	}
	return protojson.UnmarshalOptions{DiscardUnknown: false}.Unmarshal(data, msg)
}
