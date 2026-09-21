# 0004 — Minimal dependency set

- Status: Accepted
- Date: 2026-09-21

## Context

An SDK is imported transitively by its users: every dependency becomes their
dependency. Framework-heavy or dependency-injection-heavy designs leak complexity
into consumers and enlarge the supply-chain surface. HStong's Gateway speaks plain
JSON over localhost HTTP and protobuf over a local TCP socket, neither of which
requires a framework.

## Decision

Keep the dependency set minimal, explicit, and justified. **Standard library
first**: reach for `net/http`, `encoding/json`, `crypto/*`, `context`, `log/slog`,
`sync`, and `time` before considering a module.

| Dependency | Purpose | Runtime |
|------------|---------|:-------:|
| `google.golang.org/protobuf` | Generated-code runtime, `proto`, `protojson`, `Any` registry (ADR 0002) | yes |
| `go.uber.org/goleak` | Goroutine-leak detection in tests | no (test) |
| `buf` + `protoc-gen-go` | Protobuf code generation | no (build) |

- No web framework (this is a client; stdlib `net/http` suffices).
- No DI framework (plain constructors and functional options).
- No logging framework (accept `*slog.Logger` from the standard library).
- No third-party rate-limiter or circuit-breaker; `internal/resilience` is
  hand-written with `sync` and `time`.
- Generated code under `gen/` is committed; codegen tools are not required to build.

**Adding any new dependency — runtime, test, or build — requires a new ADR** that
records the purpose and the alternative considered.

## Consequences

- A small transitive dependency footprint for users; only `google.golang.org/protobuf`
  is pulled in at runtime.
- Some plumbing (rate limiting, retry budget, circuit breaker, metrics) is
  hand-written rather than delegated to a library.
- Dependency updates are cheap to review.
- The single runtime dependency is already unavoidable for any Go consumer of the
  vendored protobuf types.

## Alternatives considered

| Alternative | Why rejected |
|-------------|--------------|
| `golang.org/x/time/rate` for rate limiting | One token bucket is not worth a module; stdlib implementation is small and testable. |
| A logging framework (`zap`, `zerolog`) | Consumers should choose their own logger; the SDK accepts `*slog.Logger`. |
| A metrics library (Prometheus client) | Exposes a dependency-free interface that consumers can bridge to OTel or Prometheus (plan §P10). |
| Vendoring and not generating protobuf code | Would require shipping a protobuf runtime anyway; generated code is the supported path. |

## References

- Plan: [plan.md](../runs/2026-09-21-hstong-full-surface/plan.md) §Assumptions, §P10
- Related: [0002](./0002-hybrid-codec.md)
