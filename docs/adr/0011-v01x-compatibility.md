# 0011 — v0.1.x compatibility guarantees

- Status: Accepted
- Date: 2026-09-23
- Verified: 2026-09-26 — see [Implementation status](#implementation-status)

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

## Implementation status

Added 2026-09-26, when v-next reached feature parity and the guarantee was
checked rather than assumed. A compatibility guarantee that has only ever been
asserted is indistinguishable from one that is about to be broken, so this
records what was actually done to test each guarantee.

| Guarantee | How it was verified | Verdict |
|-----------|--------------------|---------|
| 1. No breaking type changes in the v0.1.x packages | `internal/migration` samples compile the old and new call shapes side by side; the mock-Gateway e2e exercises the v0.1.x path unchanged | **Held.** The released surface is byte-for-byte the same shape it had at v0.1.23. |
| 2. No breaking wire changes | `make proto-verify` in CI; `gen/` never edited | **Held.** |
| 3. No new mandatory dependencies | No ADR 0004 exception was needed; `go.mod` gained nothing | **Held.** |
| 4. Mock Gateway compatibility, no e2e test updated for v-next | The v-next e2e in `test/e2e/vnext_stack_test.go` was written *against* the existing mock Gateway rather than extending it, which is the strongest available evidence the guarantee held | **Held**, and the shape of that test is itself the evidence. |
| 5. `gen/` never touched | `make proto-verify` in CI | **Held.** |

**The one breaking change in this period, and why it did not violate the
guarantee.** `transport.Pagination` was retyped and its constructor signature
changed when it was split into `domain.Pagination` (a value type) and
`transport.ApplyPagination` (the wire encoding). That is a breaking change to an
exported symbol — and it is not a violation, because `pkg/transport` is v-next
code. Guarantee 1 covers the v0.1.x packages listed above, and `pkg/transport` is
not among them; it carries no compatibility promise until v1.0 by this ADR's own
final consequence. It shipped in `v0.1.24`, two patch releases, with the
migration documented.

**Reachability is not compatibility.** The v0.1.x surface is still the default
and is not deprecated in any build-flag sense: `services.NewStack` is opt-in, and
`pkg/hstong/*` remains the documented default until v1.0 announces the
deprecation. Deprecation GoDoc has been added, which is the documentation half of
the policy above; the removal half is not scheduled and would need its own ADR.

## References

- Context: [0010 — Additive v-next layered architecture](./0010-vnext-layered-architecture.md)
- Run: [plan.md](../runs/2026-09-23-hstong-enterprise-sdk/plan.md) §Approach, §E01
- Related: [0004 — Minimal dependency set](./0004-minimal-dependencies.md),
  [0008 — Decimal-backed financial types](./0008-decimal-financial-types.md)
