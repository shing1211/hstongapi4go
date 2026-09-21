# P08 — Algo

- **Run:** 2026-09-21-hstong-full-surface · **Status:** done (gate passed)
- **Owner role:** backend · tester
- **Depends on:** P01 · **Plan ref:** [plan.md](../plan.md) §P8 · **Tracker:** [todos.md](../todos.md)

## Objective / Exit criteria

Implement the 7 strategy/algo endpoints.

Endpoints: `/trade/AlgoAddOrder`, `/trade/AlgoCancelOrder`,
`/trade/AlgoCancelEntrust`, `/trade/AlgoChangeOrder`, `/trade/AlgoActionOrder`,
`/trade/AlgoQueryOrderList`, `/trade/AlgoQueryEntrustIdList`.

Exit criteria:
- All 7 endpoints typed (JSON codec), money/quantities as `string`.
- Algo mutations (`AlgoAddOrder`, `AlgoCancelOrder`, `AlgoCancelEntrust`,
  `AlgoChangeOrder`, `AlgoActionOrder`) issue exactly one attempt.
- Master/sub-order query results typed (master order list, sub-order list).
- `go test -race ./...` green; goleak clean.

## Tasks

| ID | Task | Role | Status | Deps | Acceptance | Verify | Evidence |
|----|------|------|--------|------|-----------|--------|----------|
| T26 | Algo endpoints (7) | backend | doing | T09 | master/sub-order + operate | `go test ./pkg/hstong/algo/...` | — |
| T27 | Algo tests | tester | doing | T26 | fixtures pass | `go test -race ./...` | — |

## Decisions & deviations

- T26+T27 run as one backend session.
- The legacy doc does not define algo proto messages; shapes come from
  `https://quant-open.hstong.com/api-docs/trade-interface/algo-trading/*`.
- `AlgoActionOrder` action codes are documented per-action; model them as a typed
  constant set and document any value not explicitly listed as non-exhaustive.

## Files created / modified

- `pkg/hstong/algo/**` (pending)

## Verification evidence

| # | Command | Result | Artifact |
|---|---------|--------|----------|
| — | — | pending | — |

## Gate

- Exit criteria met: **yes** — implemented and verified; see the evidence file linked from the task table.
