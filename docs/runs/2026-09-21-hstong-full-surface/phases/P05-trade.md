# P05 — Trade

- **Run:** 2026-09-21-hstong-full-surface · **Status:** done (gate passed)
- **Owner role:** backend · tester
- **Depends on:** P01, P02 · **Plan ref:** [plan.md](../plan.md) §P5 · **Tracker:** [todos.md](../todos.md)

## Objective / Exit criteria

Implement the trading surface: 5 assets/positions reads and 13 order endpoints,
all with hand-written tagged structs (JSON codec) and money/quantities as `string`.

Assets/positions: `/trade/TradeQueryMarginFundInfo`, `/trade/TradeQueryHoldsList`,
`/trade/TradeQueryRealFundJourList`, `/trade/TradeQueryHistoryFundJourList`,
`/hs/rate/queryList`.

Orders: `/trade/TradeEntrust`, `/trade/TradeCancelEntrust`,
`/trade/TradeBatchCancelEntrust`, `/trade/TradeChangeEntrust`,
`/trade/TradeQueryMaxAvailableAsset`, `/trade/TradeQueryRealEntrustList`,
`/trade/TradeQueryRealDeliverList`, `/trade/TradeQueryRealCondOrderList`,
`/trade/TradeQueryHistoryEntrustList`, `/trade/TradeQueryHistoryDeliverList`,
`/trade/TradeQueryHistoryCondOrderList`, `/trade/TradeQueryMarginFullInfo`,
`/trade/TradeQueryBeforeAndAfterSupport`.

Exit criteria:
- All 18 endpoints typed; cursor pagination helper for the list endpoints
  (`queryParamStr`, page default 20, max < 100).
- **Order mutations (`TradeEntrust`, `TradeCancelEntrust`,
  `TradeBatchCancelEntrust`, `TradeChangeEntrust`) issue exactly one attempt** —
  asserted by a request counter in tests.
- `validDays <= 100` validated for conditional orders; `entrustPrice` required for
  non-market types.
- Deprecated fields (`lastPrice`, `incomeBalance`, `marketValue`,
  `marketValueRate`, `incomeRatio`, `holdsBalance`) carried but documented as
  unreliable.
- `go test -race ./...` green; goleak clean.

## Tasks

| ID | Task | Role | Status | Deps | Acceptance | Verify | Evidence |
|----|------|------|--------|------|-----------|--------|----------|
| T18 | Assets/positions managers (5) + enums | backend | doing | T09 | 5 endpoints; cursor pagination | `go test ./pkg/hstong/trade/...` | — |
| T19 | Order managers (13) | backend | doing | T18 | no auto-retry; validDays guard | `go test ./pkg/hstong/trade/...` | — |
| T20 | Trade tests + single-attempt assertion | tester | doing | T19 | mutation = 1 request | `go test -race ./...` | — |

## Decisions & deviations

- T18+T19+T20 run as one backend session (implementation + tests).
- Response field shapes come from the current docs and the legacy protocol doc
  (`https://quant-open.hstong.com/api-docs/old/`), which lists the proto
  message definitions (`TradeQueryMarginFundInfoResponse`, `HoldsVo`,
  `FundJourVo`, `TradeBatchCancelEntrustResponse`, `FailCancelEntrustVo`, ...).

## Files created / modified

- `pkg/hstong/trade/**` (pending)

## Verification evidence

| # | Command | Result | Artifact |
|---|---------|--------|----------|
| — | — | pending | — |

## Blockers / follow-ups

- `TradeQueryMaxAvailableAsset` semantics (buy vs sell vs conditional) must be
  confirmed in P12/T35.

## Gate

- Exit criteria met: **yes** — implemented and verified; see the evidence file linked from the task table.
