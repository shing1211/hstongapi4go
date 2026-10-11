# Trading

`trade.Manager` covers the stock trading surface: assets, positions, order
mutations, order queries, and the session-wide order push subscription. It
requires a trade session for every call.

```go
session := hstong.NewSessionManager(c)
if err := session.Login(ctx); err != nil {
    return err
}

m := trade.New(c, trade.WithSession(session))
```

See [Authentication](authentication.md) for login, keep-alive, and re-login.
`trade.New` without `WithSession` still works; it relies on the caller having
logged in already.

## Endpoints

| Group | Method | Route |
|-------|--------|-------|
| Funds | `MarginFundInfo` | `/trade/TradeQueryMarginFundInfo` |
| Positions | `Positions` | `/trade/TradeQueryHoldsList` |
| Fund journey | `RealFundJourList` | `/trade/TradeQueryRealFundJourList` |
| Fund journey | `HistoryFundJourList` | `/trade/TradeQueryHistoryFundJourList` |
| Exchange rate | `ExchangeRate` | `/hs/rate/queryList` |
| Order mutation | `Entrust` | `/trade/TradeEntrust` |
| Order mutation | `CancelEntrust` | `/trade/TradeCancelEntrust` |
| Order mutation | `BatchCancelEntrust` | `/trade/TradeBatchCancelEntrust` |
| Order mutation | `ChangeEntrust` | `/trade/TradeChangeEntrust` |
| Buy/sell capacity | `MaxAvailableAsset` | `/trade/TradeQueryMaxAvailableAsset` |
| Order query | `RealEntrustList` | `/trade/TradeQueryRealEntrustList` |
| Order query | `RealDeliverList` | `/trade/TradeQueryRealDeliverList` |
| Order query | `RealCondOrderList` | `/trade/TradeQueryRealCondOrderList` |
| Order query | `HistoryEntrustList` | `/trade/TradeQueryHistoryEntrustList` |
| Order query | `HistoryDeliverList` | `/trade/TradeQueryHistoryDeliverList` |
| Order query | `HistoryCondOrderList` | `/trade/TradeQueryHistoryCondOrderList` |
| Margin | `MarginFullInfo` | `/trade/TradeQueryMarginFullInfo` |
| Session support | `BeforeAndAfterSupport` | `/trade/TradeQueryBeforeAndAfterSupport` |
| Push | `SubscribeOrders` | `/trade/TradeSubscribe` |
| Push | `UnsubscribeOrders` | `/trade/TradeUnsubscribe` |

## Session

Every method issues exactly one HTTP request. Before a call, the manager calls
`session.EnsureLoggedIn`. If the call fails with a re-login condition
(`1012`/`1013`/`1014`/`20033`) the manager performs a best-effort re-login and
returns the **original** endpoint error; the endpoint request is never repeated.

## Assets and positions

```go
funds, err := m.MarginFundInfo(ctx, trade.MarginFundInfoRequest{
    ExchangeType: types.ExchangeHK,
})
fmt.Println(funds.AssetBalance, funds.EnableBalance)

positions, err := m.Positions(ctx, trade.PositionsRequest{
    ExchangeType: types.ExchangeHK, // empty asks for every market
})
for _, p := range positions {
    fmt.Println(p.StockCode, p.CurrentAmount, p.EnableAmount)
}
```

Every amount in `MarginFundInfo` and `HoldsVo` is the exact decimal string the
Gateway sent. `HoldsBalance`, `MarketValue`, `LastPrice`, `IncomeBalance`,
`MarketValueRate`, and `IncomeRatio` are **deprecated by the vendor as
unreliable**; the SDK decodes them but they must not be used for money or P&L.

`ExchangeType` is one of:

| Constant | Value | Market |
|----------|-------|--------|
| `types.ExchangeHK` | `K` | Hong Kong |
| `types.ExchangeUS` | `P` | US |
| `types.ExchangeShenzhenConnect` | `v` | Shenzhen Connect |
| `types.ExchangeShanghaiConnect` | `t` | Shanghai Connect |

Fund journeys:

```go
rows, err := m.RealFundJourList(ctx, trade.FundJourListRequest{
    ExchangeType: types.ExchangeHK,
    QueryCount:   20,
    QueryParamStr: "0",
})
```

The exchange rate endpoint returns a nested map keyed by source and target
currency:

```go
rates, err := m.ExchangeRate(ctx, trade.ExchangeRateRequest{RateType: "0"})
fmt.Println(rates["HKD"]["USD"])
```

## Orders

### Place an order

```go
id, err := m.Entrust(ctx, trade.EntrustRequest{
    ExchangeType:  types.ExchangeHK,
    StockCode:     "0700.HK",
    EntrustAmount: "100",
    EntrustPrice:  "300",
    EntrustBS:     types.EntrustBuy,      // 1 buy/open long, 2 sell/close long, 3 close short, 4 open short
    EntrustType:   types.EntrustTypeLimit, // 3 limit (HK/US/A-share)
})
```

Validation before any request: `exchangeType`, `stockCode`, `entrustBs`, and
`entrustType` are required; `entrustAmount` must be a positive decimal;
`entrustPrice` is required unless the order is a market or conditional type;
a conditional order requires `condValue` and a `validDays` integer in `[1, 100]`;
an iceberg order requires `iceBergDisplaySize` greater than zero and no greater
than `entrustAmount`.

### Cancel, batch cancel, change

```go
cancelled, err := m.CancelEntrust(ctx, trade.CancelEntrustRequest{
    ExchangeType:  types.ExchangeHK,
    StockCode:     "0700.HK",
    EntrustAmount: "100",
    EntrustID:     id,
    EntrustType:   types.EntrustTypeLimit,
})

result, err := m.BatchCancelEntrust(ctx, trade.BatchCancelEntrustRequest{
    ExchangeType: types.ExchangeHK,
    EntrustIDs:   []string{id}, // empty cancels every cancellable order in the market
})
fmt.Println(result.SuccessEntrustID, result.FailCancelEntrust)

changed, err := m.ChangeEntrust(ctx, trade.ChangeEntrustRequest{
    ExchangeType:  types.ExchangeHK,
    StockCode:     "0700.HK",
    EntrustAmount: "200",
    EntrustPrice:  "301",
    EntrustID:     id,
})
```

A batch cancel can time out when the day's order volume is high.

### Mutations are never retried

`Entrust`, `CancelEntrust`, `BatchCancelEntrust`, and `ChangeEntrust` issue
exactly one attempt. On an ambiguous failure, reconcile by querying the
real/history entrust and deliver lists before resubmitting. There is no option
that turns on retry for mutations (ADR 0003).

### Hong Kong tick validation (v-next `pkg/services` only)

The released `trade.Manager` above does **not** validate order prices against
the instrument's tick. The v-next layer (`services.NewStack`, opted into per
[Migrating to v1.0](https://github.com/shing1211/hstongapi4go#migrating-to-v10)) does: `Entrust` and
`ChangeEntrust` check an HK order price against the instrument's minimum
price increment before sending it.

For HK stocks that increment is set by **price band**, not per instrument. The
SDK ships the HKEX table (Phase 2, in force from 2026-08-03):

| Order price (HKD) | Minimum increment |
|---|---|
| up to 0.25 | 0.001 |
| over 0.25 to 10 | 0.005 |
| over 10 to 20 | 0.010 |
| over 20 to 50 | 0.020 |
| over 50 to 100 | 0.050 |
| over 100 to 200 | 0.100 |
| over 200 to 500 | 0.200 |
| over 500 to 1,000 | 0.500 |
| over 1,000 to 2,000 | 1.000 |
| over 2,000 to 5,000 | 2.000 |
| over 5,000 to 9,995 | 5.000 |

Bands are inclusive at the upper bound. A price above 9,995 has no published
band; the SDK skips the check rather than inventing a coarser grid, and the
Gateway decides. A price below the published floor (0.01) resolves to the
lowest band; a negative price is still rejected on its sign.

Scope, as published by HKEX: equities, REITs and equity warrants. ETFs, bonds,
equity tradeable options, inline warrants and structured products (CBBCs) are
excluded and keep fixed per-type increments (0.001, or 0.0001 for bonds).

Two caveats:

- The table is a **snapshot** of an exchange rule that can change. Source:
  [HKEX minimum-spread reduction FAQ (Phase 2)](https://www.hkex.com.hk/-/media/HKEX-Market/Services/Trading/Securities/Overview/Trading-Mechanism/Reduction-of-Minimum-Spreads/Reduction-of-Minimum-Spreads-FAQP2_E.pdf)
  and the [December 2024 consultation conclusions](https://www.hkex.com.hk/-/media/HKEX-Market/News/Market-Consultations/2016-Present/June-2024-Review-of-Minimum-Spreads/Conclusions-Dec-2024/cp202406cc.pdf).
- The check is a **client-side convenience, not a substitute for the
  Gateway**. It uses the symbol's `DataType`, not a per-instrument master, so
  a misclassified symbol is validated against the wrong table.

Callers with strict requirements should inject their own resolver:

```go
// domain.TickScheduleResolver answers lot+tick; implementing the optional
// domain.PricedTickScheduleResolver additionally controls the per-price
// answer used for HK stock prices.
trading.WithTickSchedule(myMaster)
```

### Query open and historical orders

```go
open, err := m.RealEntrustList(ctx, trade.RealEntrustListRequest{
    ExchangeType: types.ExchangeHK,
    QueryCount:   20,
})
for _, o := range open {
    fmt.Println(o.EntrustID, o.StockCode, o.StatusDesc, o.CanBeCanceled)
}

fills, err := m.RealDeliverList(ctx, trade.RealDeliverListRequest{
    ExchangeType: types.ExchangeHK,
})

history, err := m.HistoryEntrustList(ctx, trade.HistoryEntrustListRequest{
    ExchangeType: types.ExchangeHK,
    StartDate:    "20260101", // yyyyMMdd
    EndDate:      "20260131",
})
```

`RealCondOrderList` returns a `*CondOrderPage` with `Data`, `CurPageNo`,
`CurPageSize`, and `TotalPages`. `HistoryCondOrderList` takes `PageNo` and
`PageSize` as **strings** and dates as `"yyyy-MM-dd HH:mm:ss"`.

### Buy/sell capacity

```go
max, err := m.MaxAvailableAsset(ctx, trade.MaxAvailableAssetRequest{
    ExchangeType: types.ExchangeHK,
    StockCode:    "0700.HK",
    EntrustPrice: "300",
    EntrustType:  types.EntrustTypeLimit,
})
fmt.Println(max.LongOpenAvailable, max.CashAvailableAmount)
```

## Pagination

Cursor list endpoints page with `queryParamStr`. The documented page size is 20
by default and strictly below 100.

| Constant | Value |
|----------|-------|
| `trade.DefaultPageSize` | `20` |
| `trade.MaxPageSize` | `99` |
| `trade.ClampPageSize(n)` | bounds `n` to `[1, 99]`, non-positive → 20 |

`trade.Paginate` walks every page, starting at cursor `"0"`, stopping at an empty
or short page, a non-advancing cursor, or context cancellation:

```go
rows, err := trade.Paginate(ctx, 20,
    func(ctx context.Context, size int, cursor string) (trade.Page[trade.OrderVo], error) {
        items, err := m.RealEntrustList(ctx, trade.RealEntrustListRequest{
            ExchangeType:  types.ExchangeHK,
            QueryCount:    size,
            QueryParamStr: cursor,
        })
        if err != nil {
            return trade.Page[trade.OrderVo]{}, err
        }
        next := ""
        if len(items) > 0 {
            next = items[len(items)-1].QueryParamStr
        }
        return trade.Page[trade.OrderVo]{Items: items, Cursor: next}, nil
    })
```

`Paginate` issues one fetch per page and never retries; a fetch error is returned
as-is.

## Order-status push

```go
if err := m.SubscribeOrders(ctx); err != nil {
    return err
}
defer m.UnsubscribeOrders(ctx)
```

The subscription is session-wide, not per-security. Notifications arrive on the
TCP push channel as `TradeStockDeliverNotify`; consume them with
`stream.SubscribeTrade`. See [Streaming](streaming.md).

## EntrustStatus reference

`EntrustStatus` is the order lifecycle status published in push notifications and
queried order lists.

| Value | Constant | Chinese | Meaning |
|-------|----------|---------|---------|
| `0` | `EntrustStatusNoRegister` | 未报 | Not yet submitted |
| `1` | `EntrustStatusWaitToRegister` | 待报 | Waiting to submit |
| `2` | `EntrustStatusRegistered` | 已报 | Accepted by the host |
| `3` | `EntrustStatusWaitCancel` | 已报待撤 | Accepted; queued for cancel |
| `4` | `EntrustStatusPartFilledWaitCancel` | 部成待撤 | Partially filled; queued for cancel |
| `5` | `EntrustStatusPartCancelled` | 部撤 | Partially cancelled |
| `6` | `EntrustStatusCancelled` | 已撤 | Cancelled |
| `7` | `EntrustStatusPartFilled` | 部成 | Partially filled |
| `8` | `EntrustStatusFilled` | 已成 | Fully filled |
| `9` | `EntrustStatusHostReject` | 废单 | Rejected by the host |
| `A` | `EntrustStatusWaitModifyRegistered` | 已报待改 | Accepted; queued for modify |
| `B` | — | — | Unused |
| `C` | — | — | Unused |
| `D` | — | — | Unused |
| `E` | `EntrustStatusWaitModifyPartFilled` | 部成待改 | Partially filled; queued for modify |
| `F` | `EntrustStatusRejectPreOrder` | 预埋单检查废单 | Rejected pre-order check |
| `G` | `EntrustStatusCancelledPreOrder` | 预埋单已撤 | Pre-order cancelled |
| `H` | `EntrustStatusWaitReview` | 待审核 | Waiting for review |
| `J` | `EntrustStatusReviewFail` | 审核失败 | Review failed |
| `W` | `EntrustStatusWaitConfirming` | 待确认 | Waiting for confirmation |
| `X` | — | — | Unused |
