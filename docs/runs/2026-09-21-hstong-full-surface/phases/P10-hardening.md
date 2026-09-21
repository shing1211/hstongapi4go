# P10 — Hardening

- **Run:** 2026-09-21-hstong-full-surface · **Status:** done (gate passed)
- **Owner role:** backend · security
- **Depends on:** P09 · **Plan ref:** [plan.md](../plan.md) §P10 · **Tracker:** [todos.md](../todos.md)

## Objective / Exit criteria

Add opt-in resilience and observability and the opt-in push-signature verification,
without changing default behavior. Order mutations must never be retried.

Exit criteria:
- Rate limiter, retry policy, and circuit breaker exist and are unit-tested; the retry
  policy refuses mutations (closed 12-endpoint set).
- Logging is never-nil, prefixes subsystems, and redacts secrets; metrics are
  dependency-free and OTel-bridgeable.
- Push `bodySHA1` SHA1WithRSA verification is implementable, **off by default**, and
  works with the bundled platform keys and a caller override.
- No secrets in logs; no `float64` money; defaults unchanged.

## Tasks

| ID | Task | Role | Status | Deps | Acceptance | Verify | Evidence |
|----|------|------|--------|------|-----------|--------|----------|
| T30 | Rate limit / retry / breaker | backend | done | T29 | mutations excluded from retry | `go test ./internal/resilience/...` | `evidence/P10-T30-T32.txt` |
| T31 | Logging + metrics + OTel bridge | backend | done | T29 | redaction; never-nil logger | `go test ./internal/logging/... ./internal/metrics/...` | `evidence/P10-T30-T32.txt` |
| T32 | Security review + push-verify opt-in | security | done | T30,T31 | no secrets in logs; keys override-able | `go test ./internal/push/...` | `evidence/P10-T30-T32.txt` |

## Decisions & deviations

- T30+T31+T32 ran as one backend/security session.
- All hardening is **opt-in**; `client.New()` leaves logger/metrics/retry/rate/breaker
  nil, preserving the pre-P10 semantics.
- **Follow-up fix:** the new client options initially accepted `internal/metrics` and
  `internal/resilience` types, which external consumers cannot name. Public aliases
  (`client.Recorder`, `client.RetryPolicy`, `client.RateLimiter`,
  `client.CircuitBreaker`) were added in `client/hardening.go`, with a
  `package client_test` compile-time assertion.

## Files created / modified

- `internal/resilience/**`, `internal/logging/**`, `internal/metrics/**`
- `internal/push/verify.go`, `internal/push/verify_test.go`, `internal/push/client.go`, `internal/push/doc.go`
- `client/hardening.go`, `client/options.go`, `client/client.go`, `client/resilience_test.go`
- `pkg/hstong/stream/{stream.go,doc.go,verify_test.go}`
- `docs/observability.md`, `docs/rate-limiting.md`, `docs/configuration.md`

## Verification evidence

| # | Command | Result | Artifact |
|---|---------|--------|----------|
| 1 | `go test ./internal/resilience/...` | mutation single-attempt + retryable query PASS | `evidence/P10-T30-T32.txt` |
| 2 | `go test ./internal/logging/...` | redaction tests PASS | `evidence/P10-T30-T32.txt` |
| 3 | `go test ./internal/push/...` | signature verify (round-trip, tamper-drop, disabled-accept) PASS | `evidence/P10-T30-T32.txt` |
| 4 | `go test -race -count=1 ./...` | 19 packages ok, 3 consecutive runs | `evidence/P10-fixes.txt` |
| 5 | `go test ./internal/push/... -race -count=20 -parallel 16` | 20 runs, 0 failures | `evidence/P10-fixes.txt` |
| 6 | `python scripts/check_money.py` | OK | `evidence/P10-T30-T32.txt` |

## Blockers / follow-ups

- SHA-1 is platform-mandated and weak by modern standards; documented as residual risk.
- Bundled platform keys may lag rotation; `WithPlatformPublicKey` overrides.
- The circuit breaker counts mutation failures when enabled; off by default.

## Gate

- Exit criteria met: **yes** — T30/T31/T32 `done`, plus the public-alias and push-flake
  follow-ups verified by the orchestrator.
- Next: **P11** (examples + docs site) completed; **P12** integration tests written and
  env-gated.
