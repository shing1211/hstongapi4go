# P06 — Trade Push

- **Run:** 2026-09-21-hstong-full-surface · **Status:** done (gate passed)
- **Owner role:** backend · tester
- **Depends on:** P04 (push/stream), P05 (trade) · **Plan ref:** [plan.md](../plan.md) §P6 · **Tracker:** [todos.md](../todos.md)

## Objective / Exit criteria

Wire the trade/futures order-push path end to end: the two HTTP
subscribe/unsubscribe endpoints, decoding of `TradeStockDeliverNotify` into the
existing channel stream API, and tests covering the documented order-status
scenarios.

Exit criteria:
- `TradeStockDeliverNotify` decoded for notify types 0, 1, and 2.
- Scenarios 下单 (place) / 改单 (change) / 撤单 (cancel) / 成交 (fill) covered.
- `Cancel` closes the channels and, when a trade manager is configured, issues
  the HTTP unsubscribe.
- The explicit-subscribe deviation from the legacy auto-subscribe is recorded.
- `go test -race ./...` green; goleak clean.

## Tasks

| ID | Task | Role | Status | Deps | Acceptance | Verify | Evidence |
|----|------|------|--------|------|-----------|--------|----------|
| T21 | Trade push subscribe + decoders | backend | done | T16, T19 | `TradeStockDeliverNotify` decoded; explicit-subscribe noted | `go test ./pkg/hstong/trade/... ./pkg/hstong/stream/... -race` | [P06-T21-T22.txt](../evidence/P06-T21-T22.txt) |
| T22 | Trade push tests | tester | done | T21 | 下单/改单/撤单/成交 scenarios; cancel unsubscribes | `go test -race -count=1 ./...` | [P06-T21-T22.txt](../evidence/P06-T21-T22.txt) |

## Decisions & deviations

- **Explicit subscribe instead of legacy auto-subscribe.** The reference docs
  state that initializing the trade connection auto-subscribes to order-fill
  push (`初始化交易链接会默认订阅订单成交推送`), and the legacy direct protocol
  relied on that. The Gateway also exposes explicit
  `POST /trade/TradeSubscribe` and `POST /trade/TradeUnsubscribe` endpoints, so
  the SDK requires an explicit `SubscribeOrders` / `SubscribeTrade` call. This
  is the intentional deviation recorded for the run.
- **Subscribe params are empty.** Both trade-push endpoints take
  `{"timeout_sec":10,"params":{}}`; order push is session-wide, not
  per-security, unlike the market topics. The Gateway response
  (`data.success`) is discarded, matching `market.Subscribe`.
- **Trade session required.** `SubscribeOrders` is routed through
  `Manager.call`, which ensures the attached `trade.Session` is logged in before
  the request and never retries it.
- **No hard dependency on the trade package.** `stream` depends on the narrow
  `stream.TradePusher` interface (`SubscribeOrders` / `UnsubscribeOrders`).
  `*trade.Manager` satisfies it and is attached with
  `stream.WithTradeManager`. This avoids an `stream -> trade` import (and the
  corresponding cycle risk with `trade -> client`).
- **Push-only mode.** With no `WithTradeManager`, `SubscribeTrade` registers the
  local push subscription and sends no HTTP request; `Cancel` sends none either.
  This supports a caller that owns the Gateway subscription itself.
- **Session-wide refcount.** `Client.tradeSubs` counts active trade
  subscriptions: the HTTP subscribe is issued once for the first and the HTTP
  unsubscribe once for the last to be cancelled. A concurrent first call may
  issue a second, idempotent subscribe; it never affects correctness.
- **Type-precise accessors.** `Event.TradeDeliver` now accepts only notify types
  0 (`TrsStockDeliverMsgType`) and 1 (`TradeStockDeliverMsgType`);
  `Event.FuturesTradeDeliver` accepts only type 2
  (`FuturesTradeStockDeliverMsgType`). Earlier both returned `true` for any of
  the three because they share the `TradeStockDeliverNotify` payload.
- **Futures payload is a documented subset.** Fields absent for futures stay at
  their zero value, and `matchNo` (成交流水号) is populated only when the
  notification reports a fill; `recordNo == entrustNo`.
- **Reconnect.** After a reconnect the stream re-issues the market
  `/hq/Subscribe` per market subscription and one `/trade/TradeSubscribe` for
  all active trade subscriptions, surfacing any failure on each trade
  subscription's `Errors()` channel.

## Files created / modified

- `pkg/hstong/trade/push.go` (new)
- `pkg/hstong/trade/push_test.go` (new)
- `pkg/hstong/stream/trade.go` (new)
- `pkg/hstong/stream/trade_test.go` (new)
- `pkg/hstong/stream/stream.go` (trade field/refcount, `accepts` fan-out,
  reconnect + cancel branches, type-precise `TradeDeliver`)
- `pkg/hstong/stream/doc.go` (trade-push section)
- `docs/runs/2026-09-21-hstong-full-surface/phases/P06-trade-push.md` (this file)
- `docs/runs/2026-09-21-hstong-full-surface/evidence/P06-T21-T22.txt`

No file outside the allowed set was modified; `client/**`, `internal/**`,
`gen/**`, `pkg/types/**`, and the other managers were read only.

## Verification evidence

| # | Command | Result | Artifact |
|---|---------|--------|----------|
| 1 | `gofmt -l .` | empty | [P06-T21-T22.txt](../evidence/P06-T21-T22.txt) |
| 2 | `go build ./...` | ok (exit 0) | [P06-T21-T22.txt](../evidence/P06-T21-T22.txt) |
| 3 | `go vet ./...` | ok (exit 0) | [P06-T21-T22.txt](../evidence/P06-T21-T22.txt) |
| 4 | `go test ./pkg/hstong/trade/... ./pkg/hstong/stream/... -race -count=1 -v` | PASS (7 new tests) | [P06-T21-T22.txt](../evidence/P06-T21-T22.txt) |
| 5 | `go test -race -count=1 ./...` | ok (all packages) | [P06-T21-T22.txt](../evidence/P06-T21-T22.txt) |
| 6 | `python scripts/check_money.py` | money-check OK | [P06-T21-T22.txt](../evidence/P06-T21-T22.txt) |

Decoded scenario rows captured from test 4:

```
notifyType=0 id=TRS-1    stock.TradeStockDeliverNotify{fundAccount=F123 stockCode=TRS-1 entrustStatus=2}
notifyType=1 id=00700.HK stock.TradeStockDeliverNotify{fundAccount=F123 stockCode=00700.HK entrustStatus=2}
notifyType=2 id=HSI2603  futures.TradeStockDeliverNotify{fundAccount=F123 stockCode=HSI2603 entrustStatus=2}
scenario=place         status=2 entrustNo=E100 matchNo=""      entrustAmount=1000 leftAmount=1000
scenario=change        status=A entrustNo=E101 matchNo=""      entrustAmount=2000 leftAmount=2000
scenario=cancel        status=6 entrustNo=E102 matchNo=""      entrustAmount=1000 leftAmount=1000
scenario=fill          status=8 entrustNo=E103 matchNo="M9001" entrustAmount=1000 leftAmount=0 businessPrice=320.50
scenario=futures-place status=2 entrustNo=F200 matchNo=""      entrustAmount=5 leftAmount=5
scenario=futures-fill  status=8 entrustNo=F201 matchNo="M9100" entrustAmount=5 leftAmount=0 businessPrice=19850
```

## Blockers / follow-ups

- **P09 mock gateway** should serve `/trade/TradeSubscribe` and
  `/trade/TradeUnsubscribe` with `{"params":{}}` semantics and emit
  `TradeStockDeliverNotify` frames; the tests here use a local TCP server and a
  fake pusher, so the mock is not yet wired.
- **P12/T35** should confirm against the real Gateway that the response shape is
  `{"success":true}` and that trade push is truly session-wide (one subscribe
  per trade login).
- Futures push (P07/T24) is delivered here as notify type 2 within the trade
  stream; a dedicated `stream/future.go` was not added because the futures
  payload reuses `TradeStockDeliverNotify`.

## Gate

- Exit criteria met: **yes** — types 0/1/2 decode through the public stream API,
  the four documented order-status scenarios pass with `matchNo` present only on
  fills, cancel closes channels and triggers the HTTP unsubscribe, and
  `go test -race ./...` plus `check_money` are green.
