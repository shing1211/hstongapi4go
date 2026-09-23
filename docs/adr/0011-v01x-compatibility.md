# 0011 — v0.1.x compatibility guarantees

- Status: Accepted
- Date: 2026-09-23

## Context

[ADR 0010](./0010-vnext-layered-architecture.md) introduced the additive v-next layer
(`pkg/domain`, `pkg/services`, `pkg/transport`, `internal/auth`). This ADR formalises
the compatibility guarantees for the existing v0.1.x surface (`pkg/hstong/*`,
`pkg/types`, `client/`, `internal/*`) during the v-next development period and beyond.

The guarantee is important because:
- Existing consumers relying on `pkg/hstong/*` must not see breaking changes.
- The mock Gateway and all existing e2e tests must continue to pass without
  modification.
- The `gen/` directory (generated protobuf code) is never edited (AGENTS.md hard
  rule 1).

## Decision

### Surface covered

The v0.1.x compatibility guarantee covers all exported symbols in:

| Package | Description |
|---------|-------------|
| `pkg/hstong/market` | Market data pull + subscribe |
| `pkg/hstong/trade` | Trade entrust, cancel, change |
| `pkg/hstong/future` | Futures entrust, cancel, modify |
| `pkg/hstong/algo` | Algo order management |
| `pkg/hstong/stream` | Push event stream |
| `pkg/hstong/session` | Login, logout, token |
| `pkg/types` | All domain enums and wire structs |
| `client/` | `Client`, options, HTTP executor |
| `internal/transport/` | HTTP codec dispatch |
| `internal/push/` | TCP framing, PBNotify decode |
| `internal/crypto/` | Trade password AES-ECB |
| `internal/errs/` | Typed errors |
| `internal/resilience/` | Rate limit, retry, circuit breaker |

### Guarantees

1. **No breaking type changes**: no existing exported type signature is modified
   (no field removals, no type changes, no function signature changes) in the above
   packages.
2. **No breaking wire changes**: the wire protocol (request/response JSON shapes,
   protobuf push frames) is not altered. The `gen/` types and `pkg/types` wire
   structs remain identical.
3. **No new mandatory dependencies**: no new runtime dependency is added to
   `go.mod` that is not already present, unless gated behind an explicit build tag
   that is not set by default.
4. **Mock Gateway compatibility**: the `test/mockgateway/` and
   `cmd/hstong-mock-gateway/` continue to serve all endpoints with the same
   response shapes; no e2e test needs updating for v-next.
5. **Gen/ is never touched**: per AGENTS.md hard rule 1. Any required proto changes
   follow `make proto` + `make proto-verify`.

### What IS allowed in v0.1.x packages

The following are explicitly permitted within the v0.1.x packages without
constituting a breaking change:

| Change | Rationale |
|--------|-----------|
| Adding new exported functions / methods | Additive; does not break existing callers |
| Adding new fields to unexported structs | Implementation detail |
| Adding new `Option` constructors (`WithX`) | Additive; existing calls unaffected |
| Adding new error types to `internal/errs` | Additive; `errors.Is`/`errors.As` still works |
| Performance improvements to `internal/*` | No API change |
| Bug fixes that change runtime behaviour | Expected; bugs are not specified behaviour |
| New fields to internal request/response structs | If not wire-breaking (wire types are frozen) |
| Adding a `Close()` method to types that did not have one | Additive; `Close()` returning `nil` is safe |
| Updating `go.mod` to bump existing transitive deps | Security patches, within semver of the dep |

### Deprecation of v0.1.x

When v-next reaches feature parity, `pkg/hstong/*` types will be marked deprecated
with a Go doc comment directing consumers to `pkg/services`. The deprecation is
announced in the CHANGELOG and has a minimum 6-month notice period before removal.

Removal of the v0.1.x surface is out of scope for this run and requires a separate
ADR.

## Consequences

- Existing consumers of `hstongapi4go` v0.1.x can upgrade to a version that includes
  v-next without any code changes.
- The mock Gateway and all e2e tests are a regression suite: they must continue to
  pass on every commit.
- The `make check` target (fmt + vet + tests + money-check) covers both surfaces.
- `pkg/domain`, `pkg/services`, `pkg/transport`, `internal/auth` have no compatibility
  guarantee yet — they are new and may change across minor versions until they reach
  v1.0.

## References

- Context: [0010 — Additive v-next layered architecture](./0010-vnext-layered-architecture.md)
- Run: [plan.md](../runs/2026-09-23-hstong-enterprise-sdk/plan.md) §Approach, §E01
- Related: [0004 — Minimal dependency set](./0004-minimal-dependencies.md),
  [0008 — Decimal-backed financial types](./0008-decimal-financial-types.md)
