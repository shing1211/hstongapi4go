// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

package services

import (
	"context"

	"github.com/shing1211/hstongapi4go/client"
)

// Executor is the request path the services in this package depend on.
//
// It is declared here, in the dependent package, rather than in whatever
// implements it, so the dependency is inverted: the services say what they need
// and a caller supplies it. That is the rule ADR 0010 states for this layer, and
// it is what lets the services be tested against a fake instead of a live
// Gateway.
//
// The two methods are exactly what the call sites use, so the interface adds no
// surface of its own. Note that Do and JSON still speak client.Route and
// client.Codec: inverting the dependency removes the dependency on the concrete
// *client.Client, not on the client's vocabulary, which is still the SDK's
// shared route and codec language. Giving the services private copies of those
// types would duplicate the route table this repository treats as canonical
// (docs/SPEC.md), which is a worse trade than the residual coupling.
//
// *client.Client is the intended implementation. It is the only request path
// with the rate limiter, split query/mutation circuit breakers, metrics, and
// tracing spans; the former pkg/transport.Adapter had none of those and was
// removed rather than adopted (docs/VNEXT.md 5.4).
type Executor interface {
	// Do issues one request for op against route and decodes the reply into out.
	// Implementations must issue exactly one attempt for a mutation route and
	// must not retry it (docs/adr/0003-no-auto-retry-orders.md).
	Do(ctx context.Context, op string, route client.Route, params any, codec client.Codec, out any) error
	// JSON returns the JSON codec for this request path.
	JSON() client.Codec
}

// Executor must stay satisfied by the released client. If this ever fails to
// compile, a service constructor has drifted from the client it documents.
var _ Executor = (*client.Client)(nil)
