# Plan: hstongapi4go — Enterprise Gateway SDK (v-next)

- **Run:** `2026-09-23-hstong-enterprise-sdk`
- **Mode:** BUILD
- **Status:** planning — awaiting approval of the E01 approach (see **Approval gates**)
- **Repo:** existing `github.com/shing1211/hstongapi4go`
- **Base commit:** `d524281` (v0.1.4); the working tree additionally holds three uncommitted doc fixes from the prior task (`docs/algo.md`, `docs/protocol.md`, `mkdocs.yml`)
- **Target API:** the current 華盛 (HStong / VBrokers) Quant OpenAPI **Gateway** — HTTP `127.0.0.1:11111`, TCP push `127.0.0.1:11112`
- **Authoritative docs:** `https://quant-open.hstong.com/api-docs` (current) only
- **VCS:** `origin` = github.com/shing1211/hstongapi4go · `gitee` = gitee.com/shing1211/hstongapi4go · branch `main`
- **Stack:** Go 1.26 · protobuf v1.36 · golangci-lint v2 · buf · mkdocs
- **New deps (ADR-gated):** `github.com/shopspring/decimal`, OpenTelemetry (`go.opentelemetry.io/otel`)
- **Paths** are written as inline code (not relative links) so `scripts/check_links.py` cannot be broken by this artifact.

## Goal

Elevate the existing Gateway SDK to an enterprise-grade, layered Go SDK per the
engineering blueprint: clean architecture / DDD separation, decimal-backed financial
types, explicit HK-market correctness, order safety, a real-time push event engine,
fuzz testing, and CI/CD with GoReleaser and observability — **without breaking the
released v0.1.x public surface**.

## Scope

**In scope**

- A parallel "v-next" layered surface: `pkg/domain`, `pkg/services`, `pkg/transport`,
  `internal/auth`, adapting the existing `client/` and `internal/*` primitives.
- Decimal-backed money/price/quantity/rate types with explicit scale + rounding, wired
  through a revised `make money-check`.
- HK-market correctness models: structured market-qualified symbols, lot/tick rules,
  sessions, odd lots, short-selling flags, order channels, and market-data entitlements
  (where the Gateway API exposes them).
- Order safety: client correlation + idempotency keys, single-attempt mutations,
  reconciliation after reconnect, duplicate-submission protection.
- Push event engine over the Gateway's TCP channel: framing, typed normalizers, bounded
  fan-out, gap/freshness monitoring, reconnect + resubscribe.
- Observability (`log/slog` redaction + OpenTelemetry), fuzz tests, gosec/govulncheck,
  coverage gates, GoReleaser with SBOM/checksums/provenance.
- Documentation: compatibility matrix, secure-config guide, examples, release checklist.

**Out of scope (explicit)**

- The **legacy direct-to-platform protocol** (`/api-docs/old/`: developer RSA signing,
  AES-ECB dynamic key, device binding, direct socket) — remains documented, not implemented.
- The brief's **HMAC REST + WebSocket** model (`X-Vbroker-Id`/`X-Timestamp`/`X-Signature`,
  `wss://openapi.vbkr.com/v1/ws`) — **does not exist** in the Gateway contract; re-mapped
  (see Appendix A). Not implemented as described.
- A breaking in-place restructure of the released `pkg/hstong/*` surface.
- Live-credential verification in CI; any commit of credentials or private keys.

## Findings that shaped the plan

### F1 — The brief's protocol does not match the authoritative API

A grep of the fetched official docs (`quant-open.hstong.com/api-docs`) found
**`HMAC = 0`, `X-Vbroker = 0`, `X-Signature = 0`, `X-Timestamp = 0`, `wss:// = 0`**,
versus `SHA1WithRSA = 2` and `AES/ECB = 1`. The documented contract is either the
**current local Gateway** (plain JSON, Gateway owns signing) or the **legacy** TCP/RSA/AES
protocol. The blueprint's HMAC/WebSocket specifics are therefore re-mapped to the Gateway
reality (Appendix A) and recorded as adaptations, not implemented literally.

### F2 — The current Gateway API is already implemented by this repo

`hstongapi4go` v0.1.x already covers the full current-Gateway surface (canonical counts in
`docs/SPEC.md`), with `client/`, `internal/transport`, `internal/push`, `internal/crypto`,
`internal/errs`, `internal/resilience`, `pkg/hstong/*`, `pkg/types`, a mock Gateway, and an
all-endpoint e2e suite. This run is therefore a **re-architecture and hardening**, not a
green-field build. It must be **additive**.

### F3 — Money and dependency rules conflict with the blueprint

The repo enforces money as `string` / `json.Number`, never `float64` (`docs/DESIGN.md` §7,
`make money-check`) and stdlib-first with an ADR per new dependency (ADR 0004). The
blueprint mandates `shopspring/decimal` and OpenTelemetry. Both need **superseding ADRs**
and a revised `money-check` before adoption (Approval gate 2).

### F4 — No live credentials; official docs only

There is no test account, trade password, or Gateway instance assumed. All automated tests
must remain offline against the mock/`httptest` servers; live verification is deferred and
env-gated. HK-model gaps that the API does not expose are recorded in
`docs/compatibility-matrix.md` rather than invented (brief: "documentation-backed").

### F5 — Pre-flight: the baseline is not clean

Three doc files are modified but uncommitted (`docs/algo.md`, `docs/protocol.md`,
`mkdocs.yml`) from the immediately preceding task. The run baseline should be committed or
the changes explicitly folded in before E01 (Approval gate 3).

## Assumptions

1. The target remains the **current Gateway** API; the brief's HMAC/WS are adapted.
2. The new layer is **additive**; released `pkg/hstong/*` types and signatures stay stable.
3. Decimal is introduced in the **v-next domain layer**; existing string-based public
   fields are bridged, not rewritten in place.
4. `proto/` and generated `gen/` (PB v2.2.0) are reused as-is; never hand-edited.
5. All CI runs without live credentials; live checks are env-gated.
6. GoReleaser/gosec/govulncheck behavior is validated by CI on Linux; this Windows host
   may not exercise every recipe directly.

## Approach — alternatives

| # | Approach | Decision |
|---|----------|----------|
| A | In-place restructure: rewrite `pkg/hstong/*` and migrate all money to decimal | Rejected: breaks the released v0.1.x API and the e2e/CI surface; v1.0-class change |
| B | **Additive v-next layer** adapting existing primitives; decimal confined to the new domain layer; superseding ADRs | **PICK** |
| C | Green-field module in a new repository | Rejected by the user: keep the existing repo |

**Pick rationale:** B satisfies the blueprint's clean-architecture/DDD intent while
preserving compatibility, CI, and the mock/e2e harness. It isolates risk and lets the
decimal and OTel decisions be governed by ADRs.

## Architecture (v-next)

```text
Application
  +-- pkg/services/          use-cases: market, account, trading, push orchestration
        +-- pkg/domain/      decimal money, typed IDs, HK models, order states, events
        +-- pkg/transport/   adapters: REST (Gateway HTTP) + push (Gateway TCP)
              +-- internal/auth     session/token lifecycle + AES-ECB trade password
              +-- internal/transport (existing HTTP executor)
              +-- internal/push      (existing 151B framing + PBNotify decode)
                    +-- OpenAPI Gateway
                          +-- HTTP 127.0.0.1:11111
                          +-- TCP  127.0.0.1:11112
```

**Rules**

- Wire DTOs (`gen/**`) and hand-written wire structs stay separate from `pkg/domain`.
- `pkg/domain` holds no transport, clock, or logger; services inject those.
- New code obeys AGENTS.md: SPDX headers, `context.Context` first, typed `internal/errs`
  errors, `Option func(*Config)` constructors, GoDoc on every export.
- `gen/**` is never edited; PB v2.2.0 is the type source of truth.

## Task breakdown

> Columns: **ID · Objective · Role · Deps · Acceptance / Verify · Size**

### P00 — Foundation & Domain (blueprint Phase 1)

| ID | Objective | Role | Deps | Acceptance / Verify | Size |
|----|-----------|------|------|---------------------|------|
| E01 | Superseding ADRs: decimal money (supersedes DESIGN §7 / ADR 0004 relevance), OpenTelemetry dep, v-next layering, compatibility with v0.1.x | architect | — | 4 ADRs present; `make docs-check`; no dangling links | M |
| E02 | v-next layout + boundaries: `pkg/domain`, `pkg/services`, `pkg/transport`, `internal/auth` | architect | E01 | `go build ./...`; boundary doc; `gen/` untouched | M |
| E03 | Decimal financial types (scale/rounding), typed IDs, HKEX symbol/contract, order states, wire↔domain mappers | backend | E02 | table tests; `go test -race` green | L |
| E04 | Auth foundation: AES-ECB trade password, login/logout, token lifecycle, re-login, injectable clock, redaction | backend | E02 | crypto vector `123456`→`W1U8iZIppSE+mBMtzy9vZQ==`; session tests | M |
| E05 | Toolchain: golangci-lint (+gosec, revive, errcheck, ineffassign), govulncheck, coverage gates, no-live-credential Makefile targets | devops | E01 | targets run; CI-parity check | M |

### P01 — REST: Market, Accounts, Trading (blueprint Phase 2)

| ID | Objective | Role | Deps | Acceptance / Verify | Size |
|----|-----------|------|------|---------------------|------|
| E06 | HTTP adapter: deadlines, response-size caps, correlation IDs, structured error decode, pagination, jittered **query-only** retry | backend | E03,E05 | `httptest` round-trips; mutation = 1 attempt | L |
| E07 | Market services (9 pull + subscribe) → domain | backend | E06 | fixture round-trips; `limit <= 100` guard | M |
| E08 | Account / asset / position services (5) | backend | E06 | fixture round-trips; cursor pagination | M |
| E09 | Trading services (13) + HK validation (lot/tick/session/order-type/TIF/permission/idempotency) | backend | E06 | validation table tests; mutation guard | L |
| E10 | Mock-server integration suites: login/token, rate limit, malformed payloads, partial fills, duplicate-order recovery | tester | E09 | `go test -race`; single-attempt assertion | M |

### P02 — Real-Time Push Engine (blueprint Phase 3)

| ID | Objective | Role | Deps | Acceptance / Verify | Size |
|----|-----------|------|------|---------------------|------|
| E11 | Push manager: TCP dial, 151-byte framing, `PBNotify`/`Any` decode, subscription registry, heartbeat, reconnect state machine, resubscribe, clean shutdown | backend | E06 | golden-frame tests; leak-free | L |
| E12 | Typed normalizers: quote / tick / depth / broker / order-status / executions / account / system → domain; unknown + versioned tolerance | backend | E11 | decode tests incl. unknown types | M |
| E13 | Bounded fan-out, backpressure policy, freshness/gap monitoring, event dedup, post-reconnect reconciliation | backend | E12 | race + backpressure tests | M |
| E14 | Push integration/race/cancellation/reconnect tests + **fuzz** (frames, deserialization) | tester | E13 | `go test -fuzz` seeds; `-race` green | M |

### P03 — DevOps, Observability, Hardening (blueprint Phase 4)

| ID | Objective | Role | Deps | Acceptance / Verify | Size |
|----|-----------|------|------|---------------------|------|
| E15 | `log/slog` redaction; OTel traces/metrics (latency, auth failures, retries, rate limit, reconnects, heartbeat, drops, queue depth, order lifecycle) | backend | E10,E14 | metric-name tests; no secret in logs | M |
| E16 | CI/CD: lint, vuln, gosec, secret scan, coverage gate, race, cross-build; GoReleaser (linux/amd64+arm64, darwin/arm64, windows); SBOM, checksums, provenance | devops | E15 | workflows run without creds | L |
| E17 | Threat model + failure injection (secret leakage, token replay, clock drift, duplicate orders, reconnect storms, stale data, partial outage) | security | E15 | adversarial tests recorded in `evidence/` | M |
| E18 | Docs: README, GoDoc, `docs/compatibility-matrix.md`, secure-config guide, paper-trading examples, release checklist | docs | E15 | `mkdocs build --strict`; `check_links` 0 unresolved | L |
| E19 | Coverage ≥85% on auth/transport/domain + fuzz suite | tester | E14 | coverage report in `evidence/` | M |
| E20 | Release: SemVer tag, push `origin` + `gitee` `main` | release | E18,E19 | both remotes at the new SHA | S |
| E21 | `next-phase.md` (gaps, tech debt, candidates) | planner | E20 | artifact complete | S |
| E22 | Close-out report + `docs/runs/index.md` | orchestrator | E21 | artifacts complete | S |

## Order of work

`E01 -> E02 -> {E03, E04, E05} -> E06 -> {E07, E08, E09} -> E10 -> E11 -> E12 -> E13
-> E14 -> E15 -> {E16, E17, E18} -> E19 -> E20 -> E21 -> E22`.

Parallelizable: `E03`/`E04`/`E05`; `E07`/`E08`/`E09`; `E16`/`E17`/`E18`.

## Artifacts

```text
docs/runs/2026-09-23-hstong-enterprise-sdk/
  plan.md          this file (frozen after approval; Actuals at close-out)
  todos.md         run-wide tracker — single source of truth (E01-E22)
  phases/          one file per phase, opened at first `doing`, closed at gate
    P00-foundation-domain.md   P02-push-engine.md
    P01-rest-services.md       P03-devops-observability.md
  evidence/        command output, coverage, fuzz results, CI run links
  docs-plan.md     docs sync plan (Phase 5.1)
  release-plan.md  release steps + message (Phase 5.2)
  report.md        close-out report
  next-phase.md    next-phase planning
```

## Phase registry

| Phase | Tasks | Exit criteria |
|-------|-------|---------------|
| P00 Foundation & Domain | E01-E05 | Superseding ADRs merged; v-next builds; decimal domain + auth foundation tested; toolchain green |
| P01 REST Services | E06-E10 | REST adapter + market/account/trading services typed; mutations single-attempt; mock integration green |
| P02 Push Engine | E11-E14 | TCP framing + typed normalizers + reconnect/resubscribe; race + fuzz green |
| P03 DevOps & Hardening | E15-E22 | Observability, CI/CD + GoReleaser, threat tests, docs, coverage ≥85%, released to both remotes |

## Risks

| # | Risk | Mitigation |
|---|------|------------|
| R1 | Decimal mandate conflicts with ADR 0004 / DESIGN §7 / `money-check` | E01 superseding ADRs + revised `money-check`; Approval gate 2 |
| R2 | v-next duplicates the existing surface | Adapters over `client`/`internal/*`, not forks; deprecation path documented |
| R3 | HK lot/tick/session data absent from the Gateway API | Record gaps in `docs/compatibility-matrix.md`; do not invent |
| R4 | No live credentials (docs-only) | Offline mock/`httptest` only; live verification env-gated and deferred |
| R5 | OTel/gosec/GoReleaser toolchain untested on Windows | CI on Linux is the arbiter; document host limitations |
| R6 | Scope creep across 4 phases | Phase-gated, resumable via `todos.md`; each phase closes at a gate |
| R7 | Accidental edit under `gen/` | AGENTS hard rule 1; `make proto-verify` in CI |
| R8 | Uncommitted prior-task docs pollute the baseline | Approval gate 3: commit or fold in first |

## Verification (global)

`go build ./...` -> `go vet ./...` -> `gofmt -l .` -> `go test ./...` ->
`go test -race -count=1 ./...` -> `golangci-lint run ./...` -> `govulncheck ./...` ->
`gosec ./...` -> `go test -fuzz=... ` (bounded) -> `make money-check` -> `make proto-verify`
-> `make docs-check` -> `make license-check` -> `mkdocs build --strict` -> coverage gate.

## Post-implementation

- **Docs sync** — `docs-plan.md`: every `.md` reviewed; counts sourced only from
  `docs/SPEC.md`; compatibility matrix reflects observed/assumed behavior.
- **Release** — `release-plan.md`: conventional commits referencing run + task IDs, push
  `origin main` then `gitee main`, annotated SemVer tag, verify both remotes. No force-push.
- **Next phase** — `next-phase.md`: gaps, tech debt, 3-7 candidates, recommended breakdown.

## Approval gates (open decisions)

1. **Additive v-next layering** (Approach B) instead of in-place restructure — confirm.
2. **Superseding ADRs** that admit `shopspring/decimal` + OpenTelemetry and revise
   `make money-check` — confirm.
3. **Baseline hygiene** — commit/release the three pending doc fixes before E01, or fold
   them into this run — choose.

Until these are confirmed, E01 stays `todo` and no execution begins.

## Appendix A — Brief-to-Gateway adaptation matrix

| Brief element | Gateway reality (authoritative) | Plan treatment |
|---------------|--------------------------------|----------------|
| HMAC-SHA256 signing | none; Gateway owns platform signing | replaced by session/token lifecycle + AES-ECB trade password (`internal/auth`) |
| `X-Vbroker-Id` / `X-Timestamp` / `X-Signature` | not present | not implemented; documented in compatibility matrix |
| Clock-skew policy | only token TTL (~3h) + keep-alive | injectable clock for token lifecycle; no request signing clock |
| `wss://openapi.vbkr.com/v1/ws` | TCP push `127.0.0.1:11112`, 151-byte header + proto `PBNotify` | push engine over existing TCP framing (`internal/push`) |
| REST to remote endpoints | local Gateway `127.0.0.1:11111`, plain-JSON POST envelope | REST adapter targets the Gateway |
| Direct accounts/positions/orders endpoints | Gateway paths under `/hq/*`, `/trade/*` | same semantics, layered into `pkg/services` |
| Nonce/replay protection | not specified | mapped to single-flight login/idempotent reconciliation; documented |

## Appendix B — Surface reference

The canonical endpoint, topic, and schema inventory lives in `docs/SPEC.md` and is not
restated here (AGENTS.md hard rule 5). Protobuf types: PB v2.2.0 under `proto/` with
generated Go in `gen/`.
