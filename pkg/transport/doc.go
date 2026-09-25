// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0

// Package transport bridges the Gateway wire format and the v-next domain types.
//
// It contains the protobuf-to-domain mappers (see [Normalize] callers in
// mappers.go), the PBNotify-to-domain.PushEvent mapping, and the bounded cursor
// walk used by list endpoints.
//
// # Scope
//
// The HTTP request path is **not** implemented here. The `Adapter` that used to
// live in this package was removed on 2026-09-25: it had no production caller,
// it discarded the transport it was constructed with, and it provided less than
// the released `client.Client` already does — no rate limiter, no circuit
// breaker, no metrics, and no tracing spans. Routing `pkg/services` through it
// would have been a regression, not an improvement. See docs/VNEXT.md 5.4 and
// the R9 entry in docs/threat-model.md.
//
// Two types this file previously documented, `RESTAdapter` and `PushAdapter`,
// were never implemented. The package has never provided a request path, and
// this documentation now describes only what exists.
//
// # Rules (per ADR 0010, ADR 0002, ADR 0007)
//
//   - transport may depend on internal/auth, internal/transport, internal/push,
//     and gen/ (generated protobuf types).
//   - transport must not import pkg/services (dependency inversion: interfaces
//     are declared in services, implemented here).
//   - wire types use string/json.Number throughout; decimal.Decimal lives only
//     in pkg/domain. Bridge functions perform the conversion.
//   - gen/ is never edited (AGENTS.md hard rule 1).
package transport
