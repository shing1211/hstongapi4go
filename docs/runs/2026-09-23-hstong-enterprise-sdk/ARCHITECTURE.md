# v-next Architecture Boundaries

- **Run:** `2026-09-23-hstong-enterprise-sdk`
- **Phase:** P00 / E02

## Layer map

```
pkg/domain/           pure domain model — decimal money, IDs, HK models, events
pkg/services/         use-case services — depend on domain + transport interfaces
pkg/transport/        wire adapters — REST HTTP + TCP push; implement service interfaces
internal/auth/        session/token lifecycle + AES-ECB trade password
```

## Import rules (enforced by go build, review, and golangci-lint)

| Layer | May import | Must NOT import |
|-------|-----------|-----------------|
| `pkg/domain/` | stdlib only (`decimal`, `time`, `context`) | `pkg/services`, `pkg/transport`, `client/`, `internal/*` |
| `pkg/services/` | `pkg/domain`, service interface types | `client/`, `internal/transport`, `internal/push` directly |
| `pkg/transport/` | `pkg/domain` (interface types), `internal/auth`, `internal/transport`, `internal/push`, `gen/` | `pkg/services` (dependency inversion) |
| `internal/auth/` | `pkg/domain` (money types for fee calc), stdlib, `decimal` | `client/`, `pkg/services` |

## Dependency graph (simplified)

```
client/
  └─> internal/transport/
        └─> internal/push/
internal/auth/        (standalone; used by pkg/transport)
pkg/domain/           (no downstream dependencies)
pkg/services/         (domain + transport interfaces)
pkg/transport/        (domain interfaces + internal/* + gen/)
```

## gen/ exclusion

`gen/` is never imported by `pkg/domain/` or `pkg/services/`. Only `pkg/transport/`
and `internal/auth` may reference generated types, and only via mappers that
convert wire DTOs → domain types. This preserves the boundary and ensures the
v0.1.x wire surface is the only place `gen/` is referenced.

## Build verification

```sh
go build ./...
```

must succeed with zero errors for all four new packages. A build failure in any
v-next package that imports a prohibited package is a direct ADR 0010 violation.

## Files added

| Path | Purpose |
|------|---------|
| `pkg/domain/doc.go` | Package declaration + layer rules |
| `pkg/services/doc.go` | Package declaration + layer rules |
| `pkg/transport/doc.go` | Package declaration + layer rules |
| `internal/auth/doc.go` | Package declaration + layer rules |
| `docs/runs/2026-09-23-hstong-enterprise-sdk/ARCHITECTURE.md` | This file |

## References

- [ADR 0010 — Additive v-next layered architecture](../../adr/0010-vnext-layered-architecture.md)
- [ADR 0011 — v0.1.x compatibility guarantees](../../adr/0011-v01x-compatibility.md)
