# Docs Plan — T36 Full Documentation Sync

- **Run:** `2026-09-21-hstong-full-surface`
- **Task:** T36 (full `.md` sweep + counts from SPEC)
- **Plan ref:** [plan.md](./plan.md) §P13 · **Tracker:** [todos.md](./todos.md)
- **Canonical counts:** [docs/SPEC.md](../../SPEC.md) — 51 HTTP endpoints, 11
  market push topics, 17 vendored protos, Gateway v2.4.1, protobuf v2.2.0.

Every Markdown file in the repository was reviewed. The columns are the path,
the action taken (`updated` / `created` / `no change needed`), and the reason.

## Root files

| Path | Action | Reason |
|------|--------|--------|
| `README.md` | updated | Replaced the 2-line stub with the canonical English README: badges, disclaimer, status, install, quickstart, feature matrix, config, layout, docs, build, contributing/security/license, LEGACY pointer. Counts sourced from SPEC. |
| `AGENTS.md` | updated | Removed stale "planned, T03/T34" wording; documented the `make docs-check` behaviour; kept all hard rules. |
| `CONTRIBUTING.md` | created | Dev setup, Makefile targets, SPDX, ADR-for-deps, never-retry, money-as-string, generated-code rule, integration env, commit style, Windows binary hygiene. |
| `SECURITY.md` | created | Supported versions, private reporting, key model (Gateway owns keys; bundled public keys are reference data), no-secrets policy. |
| `CHANGELOG.md` | created | Keep a Changelog format; `Unreleased` then `[0.1.0] - 2026-09-21` with Added / Changed / Fixed / Security for this run. |
| `DISCLAIMER.md` | no change needed | Unofficial-SDK, no-warranty, and trading-risk wording already accurate and consistent with the README. |
| `THIRD_PARTY_NOTICES.md` | no change needed | Vendor protobuf provenance and tooling licenses accurate; Gateway not redistributed. |

## `docs/` pages

| Path | Action | Reason |
|------|--------|--------|
| `docs/index.md` | no change needed | Surface counts match SPEC (9/2/2/5/13/2/7/11 = 51, 11 topics); quickstart matches the real API. |
| `docs/getting-started.md` | no change needed | Import paths, module, requirements, and example commands match the tree. |
| `docs/configuration.md` | no change needed | Options and `HSTONG_*` tables match `client/options.go` and `client/env.go`. |
| `docs/authentication.md` | no change needed | Login, single-flight, re-login, keep-alive, and push-subscribe behaviour match `pkg/hstong/session.go` and `stream/trade.go`. |
| `docs/protocol.md` | no change needed | Envelope, route aliases, hybrid codec, and 151-byte frame match `client/` and `internal/push/`. |
| `docs/errors.md` | no change needed | Status table matches SPEC §6 and `pkg/types/status.go`; 51-route claim correct. |
| `docs/market-data.md` | no change needed | Typed pull managers and topic IDs match `pkg/hstong/market/`. |
| `docs/trading.md` | no change needed | Assets/orders endpoints, pagination, and mutation rules match `pkg/hstong/trade/`. |
| `docs/futures.md` | no change needed | Eleven futures endpoints and push mapping match `pkg/hstong/future/`. |
| `docs/algo.md` | no change needed | Seven algo endpoints match `pkg/hstong/algo/`. |
| `docs/streaming.md` | no change needed | Stream API, channel semantics, and reconnect behaviour match `pkg/hstong/stream/`. |
| `docs/mock-gateway.md` | no change needed | Mock API and "serving 51 endpoints" line match `cmd/hstong-mock-gateway` and `test/mockgateway/`. |
| `docs/testing.md` | no change needed | Test tiers and the "51 canonical HTTP endpoints" e2e claim match `test/e2e/`. |
| `docs/observability.md` | no change needed | Logging redaction and metric names match `internal/logging/` and `internal/metrics/`. |
| `docs/rate-limiting.md` | no change needed | Rate limit, retry class, and breaker behaviour match `internal/resilience/` and `client/`. |
| `docs/security.md` | no change needed | Trust model, trade-password transformation, and push-verify behaviour match the code. |
| `docs/CONTRIBUTING.md` | updated | Corrected the `make docs-check` description from "README translations" to link check + `mkdocs build --strict` (i18n arrives in T40). |
| `docs/DESIGN.md` | updated | Replaced the stale `docs/SPEC.md (planned, T03)` / `docs/LEGACY.md (planned, T34)` bullet with live links. |
| `docs/SPEC.md` | no change needed | The canonical count source; verified against `client/routes.go` and `docs/SPEC.md` itself. |
| `docs/LEGACY.md` | no change needed | Legacy status and 51-endpoint/11-topic counts match SPEC; no implementation is claimed. |

## `docs/adr/`

| Path | Action | Reason |
|------|--------|--------|
| `docs/adr/README.md` | no change needed | Index lists all seven ADRs with accurate status and summaries. |
| `docs/adr/0001-gateway-transport.md` | updated | `docs/LEGACY.md` reference no longer says "planned, T34"; now a live link. |
| `docs/adr/0002-hybrid-codec.md` | updated | `docs/SPEC.md` reference no longer says "planned, T03"; now a live link. |
| `docs/adr/0003-no-auto-retry-orders.md` | no change needed | Mutation set and single-attempt rule match `internal/resilience/` and the trade/futures/algo managers. |
| `docs/adr/0004-minimal-dependencies.md` | no change needed | Dependency policy matches `go.mod` (protobuf runtime; goleak test-only). |
| `docs/adr/0005-key-model-and-push-verification.md` | no change needed | Key model and opt-in verification match `pkg/types/platformkeys.go` and `internal/push/verify.go`. |
| `docs/adr/0006-test-dependencies.md` | no change needed | Its "not yet authorised" sentence is pre-decision context; the Decision records the authorization. |
| `docs/adr/0007-http-json-codec.md` | no change needed | Plain-JSON market decoding and the protojson fallback match `pkg/hstong/market/`. |

## `docs/runs/2026-09-21-hstong-full-surface/`

| Path | Action | Reason |
|------|--------|--------|
| `docs/runs/index.md` | created | Runs index (ibkrapi4go format) with the single BUILD run row. |
| `.../plan.md` | no change needed | Frozen plan of record; out of scope for this task. |
| `.../todos.md` | no change needed | Run-wide tracker owned by the orchestrator; out of scope. |
| `.../phases/P00-foundation.md` | no change needed | Phase artifact; frozen at gate, out of scope. |
| `.../phases/P01-protocol-core.md` | no change needed | Phase artifact; frozen at gate, out of scope. |
| `.../phases/P02-session.md` | no change needed | Phase artifact; frozen at gate, out of scope. |
| `.../phases/P03-market-pull.md` | no change needed | Phase artifact; frozen at gate, out of scope. |
| `.../phases/P04-push-market-stream.md` | no change needed | Phase artifact; frozen at gate, out of scope. |
| `.../phases/P05-trade.md` | no change needed | Phase artifact; frozen at gate, out of scope. |
| `.../phases/P06-trade-push.md` | no change needed | Phase artifact; frozen at gate, out of scope. |
| `.../phases/P07-futures.md` | no change needed | Phase artifact; frozen at gate, out of scope. |
| `.../phases/P08-algo.md` | no change needed | Phase artifact; frozen at gate, out of scope. |
| `.../phases/P09-mock-gateway.md` | no change needed | Phase artifact; frozen at gate, out of scope. |
| `.../phases/P10-hardening.md` | no change needed | Phase artifact; frozen at gate, out of scope. |
| `.../phases/P11-examples-docs.md` | no change needed | Phase artifact; frozen at gate, out of scope. |
| `.../phases/P12-integration.md` | no change needed | Phase artifact; frozen at gate, out of scope. |
| `.../docs-plan.md` | created | This review table. |

## Other Markdown

| Path | Action | Reason |
|------|--------|--------|
| `examples/README.md` | no change needed | Example environment matrix and run commands match `examples/`. |
| `examples/mock-gateway/README.md` | no change needed | "serving 51 endpoints" matches SPEC; mock usage accurate. |
| `test/integration/README.md` | no change needed | Environment matrix and asserted behaviours match `test/integration/`. |
| `proto/PROVENANCE.md` | no change needed | Vendored-proto provenance; excluded from the site and from the link walk by design. |

## Reviewed non-Markdown files (listed separately)

| Path | Type | Action | Reason |
|------|------|--------|--------|
| `mkdocs.yml` | YAML | no change needed | Parses with `yaml.safe_load`; nav targets all exist; `mkdocs build --strict` passes. |
| `NOTICE` | text | no change needed | Apache-2.0 + non-affiliation notice accurate. |
| `Makefile` | make | updated | Replaced the placeholder `docs-check` recipe with `python3 scripts/check_links.py` + `mkdocs build --strict` (plus the T40 i18n extension point). No other target changed. |
| `scripts/check_links.py` | Python | created | Dependency-free relative-link checker used by `make docs-check`. |

## Count audit

No endpoint, topic, or schema count in any reviewed Markdown file disagrees
with [docs/SPEC.md](../../SPEC.md). Values observed: 51 HTTP endpoints; 11 market
push topics; per-surface 9 / 2 / 2 / 5 / 13 / 2 / 7 / 11; 17 vendored `.proto`
files; Gateway v2.4.1; protobuf package v2.2.0.
