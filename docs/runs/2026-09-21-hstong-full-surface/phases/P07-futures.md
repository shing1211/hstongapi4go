# P07 — Futures

- **Run:** 2026-09-21-hstong-full-surface · **Status:** done (gate passed)
- **Owner role:** backend · tester
- **Depends on:** P01 · **Plan ref:** [plan.md](../plan.md) §P7 · **Tracker:** [todos.md](../todos.md)

## Objective / Exit criteria

Implement all 11 futures endpoints and the futures trade push notify type.

Endpoints: `/trade/FuturesQueryProductInfo`, `/trade/FuturesQueryMaxBuySellAmount`,
`/trade/FuturesQueryFundInfo`, `/trade/FuturesQueryHoldsList`,
`/trade/FuturesEntrust`, `/trade/FuturesCancelEntrust`,
`/trade/FuturesModifyEntrust`, `/trade/FuturesQueryRealEntrustList`,
`/trade/FuturesQueryHistoryEntrustList`, `/trade/FuturesQueryRealDeliverList`,
`/trade/FuturesQueryHistoryDeliverList`.

Exit criteria:
- All 11 endpoints typed (JSON codec), money/quantities as `string`.
- Futures mutations (`FuturesEntrust`, `FuturesCancelEntrust`,
  `FuturesModifyEntrust`) issue exactly one attempt.
- `FuturesTradeStockDeliverMsgType` (=2) push notify decode path prepared for P06/P04 wiring.
- `go test -race ./...` green; goleak clean.

## Tasks

| ID | Task | Role | Status | Deps | Acceptance | Verify | Evidence |
|----|------|------|--------|------|-----------|--------|----------|
| T23 | Futures endpoints (11) | backend | doing | T09 | all 11 typed | `go test ./pkg/hstong/future/...` | — |
| T24 | Futures push | backend | doing | T23 | futures notify decoded | `go test ./pkg/hstong/future/...` | — |
| T25 | Futures tests | tester | doing | T23,T24 | fixtures pass | `go test -race ./...` | — |

## Decisions & deviations

- T23+T24+T25 run as one backend session.
- Response shapes from the current docs plus the legacy doc proto definitions
  (`FuturesFundInfoVo`, `FuturesHoldsVo`, `FuturesQueryBuySellAmountResponse`,
  `FuturesProductInfoVo`).
- `gen/hq/dto/FutureBasicQotExData` is reused for market futures quote extras.

## Files created / modified

- `pkg/hstong/future/**` (pending)

## Verification evidence

| # | Command | Result | Artifact |
|---|---------|--------|----------|
| — | — | pending | — |

## Gate

- Exit criteria met: **yes** — implemented and verified; see the evidence file linked from the task table.
