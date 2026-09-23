# 0010 — Additive v-next layered architecture

- Status: Accepted
- Date: 2026-09-23
- Supersedes: [0004 — Minimal dependency set](./0004-minimal-dependencies.md) (architecture branch)

## Context

The existing `hstongapi4go` v0.1.x exposes a flat, single-layer public API under
`pkg/hstong/`: `market`, `trade`, `future`, `algo`, `stream`, `session`. The types
in `pkg/types` use `string` / `json.Number` throughout (per ADR 0004 and 0007). The
HTTP executor lives in `client/` and `internal/transport/`, and the TCP push engine
in `internal/push/`.

The engineering blueprint calls for enterprise-grade architecture (clean architecture /
DDD), decimal-backed financial types, HK-market correctness models, order safety,
and observability — all without breaking the released v0.1.x public surface.

Two restructuring strategies were considered:

| Approach | Description |
|----------|-------------|
| **A — In-place restructure** | Rewrite `pkg/hstong/*` and `pkg/types` in place to use `decimal.Decimal`, introduce the layered package structure, and migrate all callers. This is a v1.0-class change: every released type signature changes, the e2e suite and all consumer code breaks. |
| **B — Additive v-next layer** | Introduce a parallel `pkg/domain`, `pkg/services`, `pkg/transport`, `internal/auth` surface that wraps and adapts the existing primitives. The released v0.1.x types stay untouched. |

[ADR 0004 §Decision](../adr/0004-minimal-dependencies.md) says "keep the dependency set
minimal" and "generated code under `gen/` is committed; the SDK speaks plain JSON over
localhost HTTP and protobuf over a local TCP socket". Neither it nor any other ADR
precludes a new internal package hierarchy, a new `pkg/domain` layer, or the use of
`decimal.Decimal` in new packages. The constraint is "no new runtime dependencies
without an ADR". ADR 0008 covers `decimal`; ADR 0009 covers OTel; this ADR covers the
package layout.

## Decision

Adopt **Approach B — additive v-next layer**. The new packages are:

```
pkg/domain/           domain models: decimal Money/Price/Quantity/Rate,
                      typed IDs, HK market models, order states, push events
                      (no transport, no logger, no clock — injected by services)

pkg/services/         use-case services: Market, Account, Trading, PushOrchestration
                      (depends on domain + transport; injected config + logger + clock)

pkg/transport/        wire adapters: REST (Gateway HTTP), Push (Gateway TCP)
                      (depends on internal/auth, internal/transport, internal/push;
                       implements interfaces declared by domain/services)

internal/auth/        session/token lifecycle, AES-ECB trade password, injectable clock
                      (no external dependencies beyond stdlib + decimal)
```

### Rules

1. **No breaking changes to `pkg/hstong/*` or `pkg/types`**: these are the released
   v0.1.x surface. All new code lives under the new packages only.
2. **Bridge functions**: `pkg/transport` provides mappers between wire DTOs
   (`gen/*`, `pkg/types` string-based structs) and domain types (`pkg/domain`). The
   mappers are unidirectional: wire → domain. The wire layer is never rewritten.
3. **`gen/` is never edited**: per AGENTS.md hard rule 1.
4. **Cross-layer calls only outward**: `domain` → no dependencies; `services` →
   `domain` + `transport`; `transport` → `internal/*`. No reverse imports.
5. **`internal/auth`** is the only new package allowed to use `decimal.Decimal` at
   the wire bridge; all other wire/transport code uses `string` per ADRs 0004 and 0007.
6. **Interfaces declared in the dependent package**: service interfaces are declared
   in `pkg/services` and implemented by `pkg/transport`, not the other way around
   (dependency inversion).
7. **Build tags**: the `otel` tag gates OTel imports (per ADR 0009); no other new
   build tags without a new ADR.

### Deprecation path

When v-next reaches feature parity with v0.1.x, the deprecation of `pkg/hstong/*`
is announced in the README and CHANGELOG. Until then, both surfaces coexist. Consumers
migrate incrementally by replacing `pkg/hstong` calls with `pkg/services` calls at
their own pace.

## Consequences

- The v0.1.x surface is fully preserved; existing consumers are not broken.
- The new layer has a clean import graph: `domain` has zero internal dependencies,
  `services` depends only on `domain` + interface-only transport, `transport` depends
  on `internal/*`.
- Decimal financial types (ADR 0008) and OTel (ADR 0009) are confined to the new
  layer and do not infect the existing surface.
- The `internal/auth` package can be tested entirely offline with crypto vectors.
- The deprecation path is explicit but deferred; no dual-maintenance burden until
  v-next is complete.
- Existing CI (`go build`, `go vet`, `go test`, `golangci-lint`) continues to cover
  both surfaces; new tests cover only the new packages.

## Alternatives considered

| Alternative | Why rejected |
|-------------|--------------|
| In-place restructure (Approach A) | v1.0-class change; breaks all released type signatures, e2e tests, and consumer code; requires a major version jump and a migration guide. |
| Green-field module in a new repository | User requested keeping the existing repo; new module would fragment the module namespace and duplicate `gen/` maintenance. |
| Only add domain types, keep existing `pkg/hstong/*` as-is | Insufficient: the service-layer use cases (HK validation, order safety, push orchestration) need a services package to coordinate domain + transport. |
| Put domain types in `pkg/hstong/` | Pollutes the released v0.1.x namespace; makes deprecation and migration harder. |

## References

- Supersedes: [0004 — Minimal dependency set](./0004-minimal-dependencies.md) (architecture branch)
- Run: [plan.md](../runs/2026-09-23-hstong-enterprise-sdk/plan.md) §Approach, §E01, §E02
- Related: [0008 — Decimal-backed financial types](./0008-decimal-financial-types.md),
  [0009 — OpenTelemetry observability](./0009-opentelemetry-observability.md)
