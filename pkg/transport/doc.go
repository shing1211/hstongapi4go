// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

// Package transport provides the wire adapters for the v-next SDK.
//
// It translates between the Gateway wire protocol (HTTP JSON + TCP protobuf)
// and the domain types in pkg/domain, and implements the service interfaces
// declared in pkg/services.
//
// Two adapters are provided:
//
//   - RESTAdapter: HTTP POST to the Gateway (127.0.0.1:11111), using the
//     existing client and internal/transport infrastructure.
//   - PushAdapter: TCP connection to the Gateway (127.0.0.1:11112), framing
//     and decoding the 151-byte header + PBNotify push stream, normalising
//     events into domain push event types.
//
// Rules (per ADR 0010, ADR 0002, ADR 0007):
//
//   - transport may depend on internal/auth, internal/transport, internal/push,
//     and gen/ (generated protobuf types).
//   - transport must not import pkg/services (dependency inversion: interfaces
//     are declared in services, implemented here).
//   - wire types use string/json.Number throughout; decimal.Decimal lives only
//     in pkg/domain.  Bridge functions perform the conversion.
//   - gen/ is never edited (AGENTS.md hard rule 1).
package transport
