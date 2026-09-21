# Report — hstongapi4go Full-Surface Go SDK

- **Run:** `2026-09-21-hstong-full-surface`
- **Mode:** BUILD
- **Status:** released — feature commit `500b0a1` and tag `v0.1.0` pushed to both remotes
- **Base commit:** `461f219`
- **Plan:** [plan.md](./plan.md) · **Tracker:** [todos.md](./todos.md) ·
  **Next phase:** [next-phase.md](./next-phase.md) ·
  **Release:** [release-plan.md](./release-plan.md) · **Docs sweep:** [docs-plan.md](./docs-plan.md)

## Release (T37)

- **Feature commit:** `500b0a139615405fa63095b6889eb4d4b5a311ec` —
  `feat: full-surface HStong Quant OpenAPI Gateway SDK (v0.1.0)` (248 files,
  DCO-signed).
- **Tag:** `v0.1.0`, annotated, object `25aaeb89d5f3a4ee1cdd228b565d9623fdf8199f`.
- **Remotes:** `origin` = https://github.com/shing1211/hstongapi4go.git,
  `gitee` = https://gitee.com/shing1211/hstongapi4go.git (added in this step).
- **Push results:** `origin` `461f219..500b0a1  main -> main`; `gitee` new branch
  `main -> main`; tag `v0.1.0` new on both remotes.
- **Verification:** `git ls-remote` reports `refs/heads/main` = `500b0a1` and
  `refs/tags/v0.1.0` = `25aaeb8` on **both** remotes.

## Shipped

| Scope | Result |
|-------|--------|
| HTTP endpoints | **51/51** implemented and exercised end-to-end against the in-repo mock |
| Market push topics | **11** (`11,35` quote; `14,27,28,37` tick; `16` broker; `17,25,26,36` order book) |
| Push channels | market quote/tick/order-book/broker + trade/futures order status |
| Protobuf | PB v2.2.0, 17 protos vendored byte-for-byte with SHA-256 provenance; generated code committed |
| Tests | 19 packages green under `-race`; client 100%, errs 100%, transport 99.1% coverage |
| Mock Gateway | 51 routes + TCP push server + standalone `cmd/hstong-mock-gateway` |
| Resilience | opt-in rate limiter, query-only retry (mutations never retried), circuit breaker |
| Observability | `slog` logging with secret redaction; dependency-free OTel-shaped metrics |
| Security | opt-in SHA1WithRSA push verification (off by default); no secrets committed |
| Docs | README (6 languages, lockstep-checked), MkDocs Material site, ADRs 0001–0007, SPEC, LEGACY |
| Verification | `gofmt`/`go vet`/`go build` clean; `check_links` 61 files/0 unresolved; `check_i18n` 6 languages; `mkdocs build --strict` exit 0 |

## Actuals vs plan

| Planned | Actual |
|---------|--------|
| 40 tasks, 17 phases | 40 tasks done; T37 pending; phases reconfigured to 17 (P00–P16) |
| Separate implementation + tester tasks | **6 sessions merged** implementation+tests (T05+T07, T10+T11, T12+T13, T18+T19+T20, T23+T24+T25, T26+T27) — the plan's dependency order would have forced throwaway layers |
| Uniform `protojson` for market data (approach A) | **Corrected to hybrid**: market decodes with `encoding/json` over generated DTOs (ADR 0007), discovered from the vendored Java/Python SDKs |
| ADRs 0001–0005 | ADRs 0001–**0007** (0006 test-dependencies, 0007 HTTP JSON codec) |
| Session in `internal/session` + `pkg/hstong/session.go` | As planned |
| T37 release to GitHub + Gitee | **Done**: feature commit `500b0a1`, tag `v0.1.0` on both remotes; `gitee` remote added |
| Integration confirmation | **Deferred**: suite written and env-gated; live run requires the user |

## Deferred / not done

1. **Live wire validation (blocking a confident tag).** `test/integration` is written
   but never executed. Unconfirmed: market `int64` representation, `TradeQueryMaxAvailableAsset`
   nesting, `TradeQueryHoldsList` shape, push delivery, entitlements, mutation
   single-attempt against the real Gateway (see `phases/P12-integration.md`).
2. **`make` never exercised on this host** (no GNU make). Recipes verified by running
   their underlying commands; CI is untested against this tree's first push.
3. **Release and tag done** (`v0.1.0` on both remotes); Pages/coverage badge not yet wired.
4. **Translation quality** is machine-authored; human review advisable.
5. **`client.ProtoJSON()`** is now an unused-by-default fallback; make-or-remove is an
   open decision (`next-phase.md`).

## Risks (carried)

| Risk | State |
|------|-------|
| R1 hybrid codec mismatch with the real Gateway | unverified; ADR 0007 documents the fallback and where to change it |
| R2 PB v2.2.0 vs Gateway v2.4.1 drift | `make proto-verify` enforces the pinned set |
| R4 trade/futures/algo bodies not in the PB package | hand-written structs pinned by the mock and the docs; live shapes unverified |
| R7 legacy platform keys rotate | public constants + `WithPlatformPublicKey` override |
| Push `bodySHA1` uses SHA-1 | platform-mandated; documented residual risk |

## Follow-ups

- Run `test/integration` against the real test Gateway (Mon–Fri 09:00–18:00) and paste
  the `-v` output into `evidence/`.
- Execute `make help`/`make check` once on Linux/Git Bash (or let CI do it) to validate
  the Makefile recipes.
- Consider a second run for release automation, GitHub Pages, coverage badge, and the
  `next-phase.md` recommendations.

## Close-out

- All required run artifacts exist: `plan.md`, `todos.md`, `phases/P00–P16`,
  `docs-plan.md`, `evidence/`, `release-plan.md`, `report.md`, `next-phase.md`.
- `docs/runs/index.md` updated with this run.
- **T37** (release) done: feature commit `500b0a1`, annotated tag `v0.1.0`
  (`25aaeb8`) pushed to `origin` and `gitee`; both remotes verified at the same
  `main` SHA.
- `docs/runs/index.md` records the feature commit SHA; its **Close-out commit**
  column is left as the feature SHA (the close-out SHA cannot be embedded in the
  commit it describes).
