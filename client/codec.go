// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package client

import "github.com/shing1211/hstongapi4go/internal/transport"

// Codec is the public name for a request/response body codec. It is an alias
// for the transport codec interface so callers never need an internal type.
// The concrete implementation is JSONCodec, obtained from (*Client).JSON.
// Because Codec is an alias, a JSONCodec value satisfies it directly, and the
// client package's own signatures name only this public type.
type Codec = transport.Codec
