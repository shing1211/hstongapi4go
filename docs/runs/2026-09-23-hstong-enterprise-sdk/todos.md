# Todos — hstongapi4go Enterprise Gateway SDK (v-next)

- **Run:** `2026-09-23-hstong-enterprise-sdk`
- **Plan:** `plan.md`
- **Status legend:** `todo` · `doing` · `blocked` · `review` · `done` · `cancelled`
- **Rule:** this file is the run-wide single source of truth. Phase files hold local
  detail and evidence only.

## Progress

| Phase | Tasks | Done |
|-------|-------|------|
| P00 Foundation & Domain | 5 | 2 |
| P01 REST Services | 5 | 0 |
| P02 Push Engine | 4 | 0 |
| P03 DevOps & Hardening | 8 | 0 |
| **Total** | **22** | **2** |

## Tasks

| ID | Task | Role | Status | Depends On | Phase | Acceptance |
|----|------|------|--------|-----------|-------|-----------|
| E01 | Superseding ADRs (decimal, OTel, v-next layering, v0.1.x compat) | architect | done | - | P00 | 4 ADRs; docs-check clean |
| E02 | v-next layout + boundaries | architect | done | E01 | P00 | go build; boundary doc; gen/ untouched |
| E03 | Decimal financial types + typed IDs + HK models + mappers | backend | todo | E02 | P00 | table tests; -race green |
| E04 | Auth foundation (AES-ECB trade password, token lifecycle, clock) | backend | todo | E02 | P00 | crypto vector; session tests |
| E05 | Toolchain: golangci-lint/gosec/govulncheck/coverage/Makefile | devops | todo | E01 | P00 | targets run; CI-parity |
| E06 | HTTP adapter (deadlines, caps, correlation, paging, query-only retry) | backend | todo | E03,E05 | P01 | httptest round-trips; mutation=1 attempt |
| E07 | Market services (9 pull + subscribe) | backend | todo | E06 | P01 | fixture round-trips; limit guard |
| E08 | Account/asset/position services (5) | backend | todo | E06 | P01 | fixture round-trips; cursor paging |
| E09 | Trading services (13) + HK validation | backend | todo | E06 | P01 | validation tests; mutation guard |
| E10 | Mock-server integration suites | tester | todo | E09 | P01 | -race; single-attempt assertion |
| E11 | Push manager (framing, registry, heartbeat, reconnect, resubscribe) | backend | todo | E06 | P02 | golden-frame tests; leak-free |
| E12 | Typed push normalizers + unknown/versioned tolerance | backend | todo | E11 | P02 | decode tests incl. unknown types |
| E13 | Fan-out, backpressure, gap/freshness, dedup, reconciliation | backend | todo | E12 | P02 | race + backpressure tests |
| E14 | Push integration/race/reconnect + fuzz tests | tester | todo | E13 | P02 | fuzz seeds; -race green |
| E15 | slog redaction + OTel traces/metrics | backend | todo | E10,E14 | P03 | metric-name tests; no secrets logged |
| E16 | CI/CD + GoReleaser + SBOM/checksums/provenance | devops | todo | E15 | P03 | workflows run without creds |
| E17 | Threat model + failure-injection tests | security | todo | E15 | P03 | adversarial tests in evidence/ |
| E18 | Docs: README, GoDoc, compatibility matrix, examples, release checklist | docs | todo | E15 | P03 | mkdocs --strict; check_links 0 |
| E19 | Coverage ≥85% (auth/transport/domain) + fuzz suite | tester | todo | E14 | P03 | coverage report in evidence/ |
| E20 | Release: SemVer tag, push origin + gitee main | release | todo | E18,E19 | P03 | both remotes at new SHA |
| E21 | Next-phase planning | planner | todo | E20 | P03 | next-phase.md |
| E22 | Close-out report + runs index | orchestrator | todo | E21 | P03 | artifacts complete |

## Approval gates (blocking E01)

1. Confirm **additive v-next layering** (Approach B) vs in-place restructure.
2. Confirm **superseding ADRs** for `shopspring/decimal` + OpenTelemetry and the revised
   `make money-check`.
3. Decide baseline hygiene: commit/release the three pending doc fixes
   (`docs/algo.md`, `docs/protocol.md`, `mkdocs.yml`) before E01, or fold them in.

## Log

| When | Note |
|------|------|
| 2026-09-23 | Run created from blueprint intake. Target confirmed = current 華盛 Gateway API; repo = existing `hstongapi4go`; deps = decimal + OTel (ADR-gated). Brief's HMAC/WebSocket re-mapped (Appendix A). Awaiting approval gates 1-3 before E01. |
| 2026-09-24 | **E01 done.** Added ADRs 0008-0011 (decimal, OTel, v-next layering, v0.1.x compat); updated ADR README; updated check_money.py docstring; pushed to origin + gitee (`281f530`). Gate 1 (additive layering) and Gate 2 (superseding ADRs) approved. Gate 3 (baseline hygiene) already handled in prior commit. E01 complete. |
