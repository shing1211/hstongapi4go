# Todos — hstongapi4go Full-Surface Go SDK

- **Run:** `2026-09-21-hstong-full-surface`
- **Plan:** [plan.md](./plan.md)
- **Status legend:** `todo` · `doing` · `blocked` · `review` · `done` · `cancelled`
- **Rule:** this file is the run-wide single source of truth. Phase files hold local
  detail and evidence only.

## Progress

| Phase | Tasks | Done |
|-------|-------|------|
| P00 Foundation | 3 | 3 |
| P01 Protocol Core | 6 | 6 |
| P02 Session | 2 | 2 |
| P03 Market Pull | 2 | 2 |
| P04 Push + Market Stream | 4 | 4 |
| P05 Trade | 3 | 3 |
| P06 Trade Push | 2 | 2 |
| P07 Futures | 3 | 3 |
| P08 Algo | 2 | 2 |
| P09 Mock Gateway | 2 | 2 |
| P10 Hardening | 3 | 3 |
| P11 Examples & Docs | 2 | 2 |
| P12 Integration | 1 | 1 |
| P13 Docs Sync | 1 | 1 |
| P14 Translations | 1 | 1 |
| P15 Release | 1 | 0 (awaiting approval) |
| P16 Close-out | 2 | 1 |
| **Total** | **40** | **38** |

## Tasks

| ID | Task | Role | Status | Depends On | Phase | Acceptance |
|----|------|------|--------|-----------|-------|-----------|
| T01 | ADRs 0001-0005 + DESIGN + AGENTS | architect | done | - | P00 | 5 ADRs + DESIGN + AGENTS written; 0 dangling links |
| T02 | Repo/CI scaffolding | devops | done | - | P00 | build/vet/gofmt clean; 18 targets; archives ignored |
| T03 | Vendor PB + keys + enums | data | done | T02 | P00 | 17 protos vendored; provenance recorded; dictionaries enumerated |
| T04 | Protobuf codegen | data | done | T03 | P01 | 17 .pb.go; proto-verify no drift |
| T05 | HTTP transport + envelope + codec dispatch | backend | done | T04 | P01 | httptest round-trip both codecs |
| T06 | Client/options/env/alias routing | backend | done | T05 | P01 | 51 routes + 3 alias forms tested |
| T07 | Error taxonomy + status codes | backend | done | T05 | P01 | codes mapped; Retryable/ReLoginRequired |
| T08 | AES-ECB trade-password crypto | backend | done | T02 | P01 | doc vector passes |
| T09 | Core tests + goleak | tester | done | T06,T07,T08 | P01 | client 100%, errs 100%, transport 99.1%; goleak clean |
| T10 | Login/Logout + keep-alive + re-login | backend | done | T09 | P02 | re-login on 1012/1013/1014; single-flight login |
| T11 | Session tests | tester | done | T10 | P02 | leak-free; re-login + keep-alive covered |
| T12 | Market pull managers (9) | backend | done | T09 | P03 | all 9 typed; limit guard |
| T13 | Market pull tests + fixtures | tester | done | T12 | P03 | fixture round-trips |
| T14 | TCP push client (framing, Any, reconnect) | backend | done | T09 | P04 | 151B framing + enum decode + reconnect |
| T15 | Subscribe/Unsubscribe + topic enum | backend | done | T14 | P04 | topics 11,14,16,17,25,26,27,28,35,36,37 |
| T16 | Stream channel API | backend | done | T15 | P04 | Updates/Errors/cancel + resubscribe |
| T17 | Push tests (golden frames) | tester | done | T16 | P04 | local TCP server; leak-free |
| T18 | Assets/positions (5) + enums | backend | done | T09 | P05 | 5 endpoints; cursor pagination |
| T19 | Order managers (13) | backend | done | T18 | P05 | no auto-retry; validDays guard |
| T20 | Trade tests + single-attempt assertion | tester | done | T19 | P05 | mutation = 1 request |
| T21 | Trade push subscribe + decoders | backend | done | T16,T19 | P06 | TradeStockDeliverNotify decoded |
| T22 | Trade push tests | tester | done | T21 | P06 | scenarios: place/change/cancel/fill |
| T23 | Futures endpoints (11) | backend | done | T09 | P07 | all 11 typed |
| T24 | Futures push mapping | backend | done | T23 | P07 | futures notify decoded |
| T25 | Futures tests | tester | done | T23,T24 | P07 | fixtures pass |
| T26 | Algo endpoints (7) | backend | done | T09 | P08 | master/sub-order + operate |
| T27 | Algo tests | tester | done | T26 | P08 | fixtures pass |
| T28 | Mock HTTP(51) + TCP push server | devops | done | T20,T25,T27 | P09 | 51 routes + push topics |
| T29 | All-endpoint e2e + coverage | tester | done | T28 | P09 | SDK to mock for every endpoint |
| T30 | Rate limit / retry / breaker | backend | done | T29 | P10 | mutations excluded from retry |
| T31 | Logging + metrics + OTel bridge | backend | done | T29 | P10 | redaction; never-nil logger |
| T32 | Security review + push-verify opt-in | security | done | T30,T31 | P10 | no secrets in logs; keys override-able |
| T33 | Runnable examples | docs | done | T29 | P11 | login/quote/order/stream/futures/algo |
| T34 | MkDocs site + SPEC + LEGACY + ADR index | docs | done | T29 | P11 | mkdocs build --strict |
| T35 | Integration tests + wire validation | tester | done | T28 | P12 | written + env-gated; live confirmation pending user run |
| T36 | Full markdown sweep + counts from SPEC | docs | done | T34,T35 | P13 | make docs-check |
| T40 | 6-language READMEs + i18n check | docs | done | T36 | P14 | check_i18n.py passes |
| T37 | Release to GitHub + Gitee main | release | todo | T40 | P15 | both remotes at new SHA |
| T38 | Next-phase planning | planner | done | T37 | P16 | next-phase.md |
| T39 | Close-out report + index | orchestrator | doing | T38 | P16 | report.md + runs/index.md |

## Notes carried forward

- **Merged sessions (deviations from the original plan):** T05+T07, T10+T11,
  T12+T13, T18+T19+T20, T23+T24+T25, T26+T27. Rationale: implementation and its
  tests are inseparable, and in two cases the plan's declared dependency order
  would have forced a throwaway intermediate layer.
- **ADR 0007 added** (T12 finding): Gateway HTTP bodies are plain JSON, verified
  against the vendored Java/Python SDKs; `protojson` is a per-endpoint fallback.
  ADR 0002 is narrowed, not superseded (ADR 0007 references it).
- **Session keep-alive flake** fixed in `pkg/hstong/session_test.go`
  (client-side request counting + real goroutine join); 0 failures in 30 runs.
- **Open for P12/T35:** confirm the real Gateway's `int64` representation
  (number vs quoted string) and whether a read call extends the 3h token.

## Log

| When | Note |
|------|------|
| 2026-09-21 | Run created. Plan approved. P00 opened with T01 + T02 in parallel. |
| 2026-09-21 | T01 done (5 ADRs, DESIGN.md, AGENTS.md). T02 done (go.mod, doc.go, Makefile 18 targets, CI, SPDX/archives). Corrected Makefile ADR 0008 ref to DESIGN.md section 7. |
| 2026-09-21 | T03 done: PB zip sha256 74367cee..., 17 protos vendored, docs/SPEC.md v1, pkg/types enums + platform keys. P00 gate PASSED. |
| 2026-09-21 | T04 done: buf 1.73.0 codegen, 17 .pb.go, proto-verify no drift. T08 done: AES-192-ECB/PKCS7 doc vector passes. |
| 2026-09-21 | T05+T07 done (errs + transport). T06 done (client, 51 routes, ADR 0006 goleak). T09 done (goleak, 51-route e2e, client.Codec alias). Coverage client 100% / errs 100% / transport 99.1%. P01 gate PASSED. |
| 2026-09-21 | T10+T11 done: SessionManager (single-flight EnsureLoggedIn, ReLogin, keep-alive) + state machine. 32 concurrent callers = 1 login request. P02 gate PASSED. |
| 2026-09-21 | Wave 1 done: T12+T13 market (9), T18+T19+T20 trade (5 assets + 13 orders), T23+T24+T25 futures (11 + push), T26+T27 algo (7). T14-T17 done: internal/push + market Subscribe + stream API. Keep-alive flake fixed. ADR 0007 added. P03/P04/P05/P07/P08 gates PASSED. |
| 2026-09-21 | T21+T22 done: trade SubscribeOrders/UnsubscribeOrders + stream.SubscribeTrade (types 0/1/2, TradePusher interface avoids hard dep) + scenario tests (place/change/cancel/fill, futures). P06 gate PASSED. P09 dispatched (mock gateway 51 routes + TCP push, then all-endpoint e2e). |
| 2026-09-21 | T28+T29 done: test/mockgateway (51/51 routes, TCP push server reusing internal/push, error injection) + test/e2e (51/51 endpoint subtests, session re-login, push, error mapping) + cmd/hstong-mock-gateway + Makefile mock-gateway target. Mock coverage 78.6%. Doc/SDK disagreements flagged for T35 (market codec, MaxAvailableAsset nesting, HoldsList wrapping). P09 gate PASSED. P10 (T30+T31+T32) and P11 (T33+T34) dispatched in parallel. |
| 2026-09-21 | T30+T31+T32 done: internal/resilience (mutation-single-attempt guard), internal/logging (redaction) + internal/metrics, opt-in push bodySHA1 verification (default OFF). T33+T34 done: 6 runnable examples + MkDocs Material site + docs/LEGACY.md; mkdocs build --strict 0. T35 done: env-gated integration suite (7 tests skip by default) + run instructions. Fixes: public client aliases for hardening options; internal/push flake (cross-type ordering assumption) fixed; mkdocs hook extended for ../test links. Verified: gofmt/build/vet clean, race x3 green, 20x push load green, integration skips, examples ok, money-check OK. P10/P11/P12 gates closed (P12 live-run deferred to user). T36 dispatched. |
| 2026-09-21 | T36 done: canonical README (12.6 KB), CHANGELOG/CONTRIBUTING/SECURITY, docs/runs/index.md, scripts/check_links.py + make docs-check, docs-plan.md (55 md files reviewed) and AGENTS.md stale wording fixed. T40 done: 5 translations + switcher + Last synced + TRANSLATING.md + scripts/check_i18n.py wired into docs-check. Verified: check_links 61 files/0 unresolved, check_i18n OK 6 languages, mkdocs build --strict exit 0, counts 51/11 identical across 6 READMEs. P13/P14 gates PASSED. T38 dispatched. |
| 2026-09-21 | T38 done: next-phase.md (17.9 KB) with gaps/tech-debt, 3-7 candidates, recommended P17 live-wire validation. Orchestrator: report.md, release-plan.md, plan.md Actuals, runs/index.md written. Fixed: make test-integration target (-tags mismatch), CHANGELOG Unreleased staleness. T37 (release) HELD pending explicit user approval; live integration run also pending user. |
