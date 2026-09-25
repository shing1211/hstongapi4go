# v-next Architecture Boundaries

> **Read the repository-root [ARCHITECTURE.md](../../../ARCHITECTURE.md) for the
> current, source-verified map.** This file records the E02 *design intent*. The
> import rules below are **violated in code** and are enforced only by human
> review, not by `go build` or `golangci-lint`. The known deviations are listed
> in [Deviations](#deviations-verified-against-source).

- **Run:** `2026-09-23-hstong-enterprise-sdk`
- **Phase:** P00 / E02

## Layer map

```
pkg/domain/           pure domain model — decimal money, IDs, HK models, events
pkg/services/         use-case services — depend on domain + transport interfaces
pkg/transport/        wire adapters — REST HTTP + TCP push; implement service interfaces
internal/auth/        session/token lifecycle + AES-ECB trade password
```

## Import rules (design intent; enforced by review only)

| Layer | May import | Must NOT import |
|-------|-----------|-----------------|
| `pkg/domain/` | stdlib only (`decimal`, `time`, `context`) — **violated**, see Deviations | `pkg/services`, `pkg/transport`, `client/`, `internal/*` |
| `pkg/services/` | `pkg/domain`, service interface types — **violated**, it imports `client` | `client/`, `internal/transport`, `internal/push` directly |
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

Intended: `gen/` is never imported by `pkg/domain/` or `pkg/services/`; only
`pkg/transport/` and `internal/auth` reach generated types, and only via mappers
that convert wire DTOs into domain types.

Actual: `pkg/domain/` imports `gen/hq/dto` directly (see Deviations), and the
DTO-to-domain mappers live in `pkg/domain` rather than in `pkg/transport`.

## Build verification

```sh
go build ./...
```

must succeed with zero errors for all four new packages. A build failure in any
v-next package that imports a prohibited package is a direct ADR 0010 violation.

## Deviations (verified against source)

Recorded 2026-09-25 by import and symbol search. These are **findings, not
defects to fix here**: deciding the v-next layer's fate is a product decision
tracked as N1 in `../2026-09-23-hstong-enterprise-sdk/next-phase.md`.

| Declared boundary | Actual | Evidence |
|-------------------|--------|----------|
| `pkg/domain` depends on stdlib and `decimal` only | It imports generated code | `pkg/domain/market.go:11`, `pkg/domain/symbol.go:7` import `gen/hq/dto` |
| Wire-to-domain conversion happens in the transport layer | `pkg/domain` performs it | `QuoteFromDTO`, `AccountBalanceFromDTO`, `EntrustFromWire` and peers live in `pkg/domain` |
| `pkg/services` depends on `domain` plus service interfaces | It imports `client` | `pkg/services/account.go:9`, `market.go:10`, `trading.go:12` |
| `pkg/transport` implements the service interfaces | Nothing consumes it | no production importer of `Adapter` |
| Import rules are machine-enforced | Only review enforces them | `.golangci.yml` enables no `depguard` or import-boundary rule |

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
