# Todos: v-next Parity, Wiring, and v1.0

Status: `todo` · `doing` · `blocked` · `review` · `done`

A task is not `done` until its verification command succeeds **and** the evidence
is recorded in the Evidence column.

| ID | Task | Role | Status | Depends On | Acceptance | Size | Evidence |
|---|---|---|---|---|---|---|---|
| P1 | Remove `float64` money precision loss in `pkg/services/market.go` | backend | **done** | — | No `float64` money field in v-next; no `%.Nf` price reformatting; V1–V3c green | S | build/vet/gofmt clean; `go test -count=1 ./...` and `go test -race -count=1 ./...` both pass; money-check green; `pkg/hstong`+`gen/` untouched. 23 `%.Nf` sites replaced with `strconv.FormatFloat(f,'f',-1,64)`; field is `json.Number`; 12 new tests, mutation-checked. Brief's `string` instruction was wrong and was corrected — see plan.md assumption 2 |
| P2 | Harden `check_money.py` to inspect json tags | backend | **done** | — | Guard fails on planted `float64` tagged `spreadLevel`; money-check green | S | Guard now reports `pkg/hstong/market/market.go:105` (exit 1, naming the wire key). 19 stdlib `unittest` tests in `scripts/test_check_money.py`; negative case `Count float64 json:"queryCount"` still passes, so the rule does not blanket-fire. Touched no `.go` file |
| P4 | Run the `scripts/` Python tests in CI | devops | **done** | P2 | `make scripts-test` exists and runs in CI | S | Makefile `scripts-test` target added and wired into `check`; `ci.yml` `build` job gained a step. `make` verified for real under GNU make in `golang:1.26` (19 tests OK). Also pinned `actions/setup-python@v5` 3.11 — the brief was wrong that the job already had Python; `money-check` had only worked by luck of the runner image |
| P3 | Settle the price tick model and `spreadLevel` field semantics | architect | **done** | P1 | A recorded decision: how `Price` tick is sourced, and whether `OrderBookResponse.TickSize` is renamed. Not blocked by ADR 0011 — `pkg/services` is outside its protected surface and has never shipped | S | [design-tick-model.md](./design-tick-model.md) written; ADR 0008 amended with one dated, field-scoped `float64` exception that expires at v1.0.0; `check_money.py` waiver matches path+name+wire key+type and is proven not to leak (6/6 proof cases, one live planted-field run). `check_money`/`check_links`/`check_i18n`/`mkdocs --strict` green; 19/19 python tests pass. Naming: keep `TickSize` — the vendor doc says tick, the fixture cannot discriminate; test for G6 recorded. No `.go` file touched |
| A1 | `pkg/hstong/stream` 85.6% → ≥90% | tester | todo | — | ≥90% in 3 cold clean-clone runs | S | |
| A2 | `pkg/hstong/trade` 86.3% → ≥90% | tester | todo | — | ≥90% in 3 cold clean-clone runs | S | |
| B1 | Inventory uncovered surface of `market.go`+`trading.go`; test plan | tester | todo | P1 | plan written to run folder | S | |
| B2 | Tests for `pkg/services/market.go` (23 funcs) | tester | todo | B1 | ≥85% | M | |
| B3 | Tests for `pkg/services/trading.go` (24 funcs) | tester | todo | B1 | ≥85% | M | |
| B4 | Cover residual gap in `account.go`/`executor.go` | tester | todo | B2,B3 | ≥85% all four files | S | |
| B5 | Register `./pkg/services/...` in `scripts/coverage_gate.go` | backend | todo | B4 | V7 passes with 11 packages | S | |
| C1 | Design parity guard (SPEC endpoint → required v-next service) | architect | todo | — | design in run folder | S | |
| C2 | Implement guard in report mode (logs, does not fail) | backend | todo | C1 | V3b passes, gaps listed | M | |
| C3 | `FuturesService` scaffold: wire types, domain types, ctor | backend | todo | C2 | V1,V2 clean | M | |
| C4 | `FuturesService` read endpoints | backend | todo | C3 | V3b passes | M | |
| C5 | `FuturesService` mutation endpoints | backend | todo | C3 | V3b; ADR 0003 respected | M | |
| C6 | `FuturesService` tests | tester | todo | C4,C5 | ≥85% | M | |
| C7 | `AlgoService` scaffold + read endpoints | backend | todo | C2 | V3b passes | M | |
| C8 | `AlgoService` mutation endpoints | backend | todo | C7 | V3b; ADR 0003 | M | |
| C9 | `AlgoService` tests | tester | todo | C8 | ≥85% | M | |
| C10 | Design `PushOrchestration` | architect | todo | C2 | design in run folder | M | |
| C11 | Implement `PushOrchestration` on `internal/push.Client` | backend | todo | C10 | V1,V2,V3b | L | |
| C12 | `PushOrchestration` tests incl. reconnect + dispatch | tester | todo | C11 | ≥85% | M | |
| C13 | Compose `internal/auth` single-flight + `ShouldRefresh` (closes R14) | backend | todo | C2 | V3b; registered in gate | M | |
| C14 | Flip parity guard to fail-on-gap | backend | todo | C6,C9,C12,C13 | V3b passes only at full parity | S | |
| D1 | Design opt-in constructor (placement, naming, ADR 0011) | architect | todo | C14 | design in run folder | S | |
| D2 | Implement opt-in constructor; `pkg/hstong/*` default unchanged | backend | todo | D1 | V1–V3c; no ADR-0011 violation | M | |
| D3 | Extend `internal/layering` with new boundary rules | backend | todo | D2 | V3b; rule fires when violated | S | |
| D4 | e2e coverage of the wired path via mock Gateway | tester | todo | D2,D3 | V3b passes | M | |
| E1 | `MIGRATION.md` draft → supported, real before/after | docs | todo | D4 | V4,V8 | M | |
| E2 | Full `ARCHITECTURE.md` regeneration | docs | todo | D4 | V4,V8; no stale refs | M | |
| E3 | Close VNEXT 7/9; ADR 0010/0011 status; deprecation Go docs | docs | todo | D4 | V4,V5,V8 | M | |
| E4 | CHANGELOG + 6 READMEs for v1.0.0 | docs | todo | E1–E3 | V4,V5; i18n lockstep | S | |
| F1 | Record G6 deferral: no test account, stated cost | planner | todo | — | decision in `next-phase.md` | S | |
| F2 | Decline Gitee mirror; date MiniMax rotation + translation pass | planner | todo | — | recorded with rationale | S | |
| G1 | v1.0.0 tag → signed release | release | todo | E4,F1,F2 | V1–V6 green; both remotes; `cosign verify-blob` OK | S | |

## Notes

- **P1 is first because it is a correctness defect, not debt.** See the finding in
  `plan.md`; `%.3f` silently rounds a price tick (`0.0005` → `0.001`).
- **The released `pkg/hstong/market.OrderBookResponse.TickSize float64` is NOT in
  P1's scope.** It is an exported field with a deliberate "it is a price tick, so it
  stays a float64" comment; changing it would breach ADR 0011 guarantee 1. It is
  recorded as a documented acceptance for v1.0 instead.
