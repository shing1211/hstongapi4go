# 0006 — Test-only dependency: go.uber.org/goleak

- Status: Accepted
- Date: 2026-09-21

## Context

[ADR 0004](./0004-minimal-dependencies.md) permits no new dependency — runtime,
test, or build — without its own ADR. It already names `go.uber.org/goleak` in the
dependency table as the goroutine-leak detector for tests, but no ADR yet records
the decision, so the dependency is not yet authorised.

The SDK spawns goroutines that outlive individual calls: the HTTP client's
keep-alive connection readers and, from later phases, the TCP push reader and its
reconnect loop. `docs/DESIGN.md` §6 requires that "After `Close`, no goroutine
spawned by the SDK remains (verified with `goleak`)". The standard library offers
no supported way to enumerate live goroutines other than `runtime.Stack` parsing,
which is exactly what `goleak` packages correctly and portably (including the
`testing` and runtime-internal stacks that must be ignored to avoid false
positives).

## Decision

Adopt **`go.uber.org/goleak` v1.3.0 as a test-only dependency**.

- It is imported only from `_test.go` files; no non-test package in the module
  imports it, so it never enters a consumer's compiled binary.
- The core package test suite installs it once with
  `goleak.VerifyTestMain(m)`, covering every test in the package, and individual
  tests may use `goleak.VerifyNone(t)` for tighter scopes.
- It is pinned by `go.mod`. Its own transitive requirements
  (`testify`, `go-spew`, `difflib`, `yaml.v3`) appear in `go.sum` only as part of
  goleak's module graph and are never built into this module.
- No other dependency is added by this ADR.

## Consequences

- A regression that leaks a goroutine on `Close` (or on an error path) fails
  `go test` instead of surfacing as a slow resource leak in production.
- `go.mod` gains one test-only requirement; `go.sum` gains entries for goleak's
  module graph. The runtime dependency set for consumers is unchanged:
  `google.golang.org/protobuf` remains the only transitive module compiled into
  their binaries.
- `go mod tidy` must stay clean so the pin does not drift silently.
- The dependency is bounded to tests, so the supply-chain surface for SDK users is
  not enlarged.

## Alternatives considered

| Alternative | Why rejected |
|-------------|--------------|
| Parse `runtime.Stack` output by hand | Reimplements goleak's ignore lists poorly; fragile across Go versions and easy to get false positives/negatives. |
| `runtime.NumGoroutine()` assertions at test end | A count is racy and cannot attribute a leftover goroutine; no useful failure detail. |
| No leak detection | Violates `docs/DESIGN.md` §6, which requires `Close` to be goroutine-leak-free and names `goleak` as the verification. |

## References

- Plan: [plan.md](../runs/2026-09-21-hstong-full-surface/plan.md) §T06, §T09, §P01
- Design: [DESIGN.md](../DESIGN.md) §6 (concurrency and cleanup)
- Related: [0004](./0004-minimal-dependencies.md)
