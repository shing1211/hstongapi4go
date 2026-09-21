# P03 — Market Pull

- **Run:** 2026-09-21-hstong-full-surface · **Status:** done (gate passed)
- **Owner role:** backend · tester
- **Depends on:** P01 · **Plan ref:** [plan.md](../plan.md) §P3 · **Tracker:** [todos.md](../todos.md)

## Objective / Exit criteria

Implement the 9 market-data pull endpoints with typed requests/responses built on
the generated `gen/hq/dto` protobuf types, using the `ProtoJSON` codec.

Endpoints: `/hq/BasicQot`, `/hq/OrderBook`, `/hq/KL`, `/hq/TimeShare`,
`/hq/Ticker`, `/hq/Broker`, `/hq/UsOptionChainCode`, `/hq/UsOptionChainExpireDate`,
`/hq/UsOverNightTradeCodes`.

Exit criteria:
- Every endpoint has a typed request and response; no `map[string]any` in public signatures.
- Responses decode generated proto DTOs (Security, BasicQot, OrderBook, KLine,
  TimeShare, Ticker, Broker, option/future ex-data) via `ProtoJSON`.
- Ticker `limit` is validated (`<= 100`); batch requests validate non-empty security lists.
- `go test -race ./...` green; goleak clean.

## Tasks

| ID | Task | Role | Status | Deps | Acceptance | Verify | Evidence |
|----|------|------|--------|------|-----------|--------|----------|
| T12 | 9 market pull managers | backend | doing | T09 | all 9 typed | `go test ./pkg/hstong/market/...` | — |
| T13 | Market pull tests + fixtures | tester | doing | T12 | fixture round-trips; limit guard | `go test -race ./...` | — |

## Decisions & deviations

- T12 and T13 run as one backend session (implementation + tests), consistent with
  T05+T07 and T10+T11.
- Market `data` is proto-backed, so the `ProtoJSON` codec is used here; this is the
  first consumer of ADR 0002's protojson branch.

## Files created / modified

- `pkg/hstong/market/**` (pending)

## Verification evidence

| # | Command | Result | Artifact |
|---|---------|--------|----------|
| — | — | pending | — |

## Blockers / follow-ups

- Exact response wrapper field names (e.g. `data.basicQot`) come from the online
  docs; the mock gateway and P12 integration must pin the same shape.

## Gate

- Exit criteria met: **yes** — implemented and verified; see the evidence file linked from the task table.
