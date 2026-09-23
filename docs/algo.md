# Algo

`algo.Manager` wraps the seven algorithm-trading endpoints. Trading requires a
live session.

```go
session := hstong.NewSessionManager(c)
if err := session.Login(ctx); err != nil {
    return err
}
m := algo.New(c, algo.WithDefaultExchangeType(types.ExchangeHK))
```

A configured default exchange type fills an empty `ExchangeType` on the mutation
methods and on `QueryEntrustIDList`. `QueryOrderList` does not apply the default
because its `ExchangeType` is optional.

## Endpoints

| Method | Route | Retryable |
|--------|-------|:---------:|
| `AddOrder` | `/trade/AlgoAddOrder` | no (mutation) |
| `CancelOrder` | `/trade/AlgoCancelOrder` | no (mutation) |
| `CancelEntrust` | `/trade/AlgoCancelEntrust` | no (mutation) |
| `ChangeOrder` | `/trade/AlgoChangeOrder` | no (mutation) |
| `ActionOrder` | `/trade/AlgoActionOrder` | no (mutation) |
| `QueryOrderList` | `/trade/AlgoQueryOrderList` | yes (read) |
| `QueryEntrustIDList` | `/trade/AlgoQueryEntrustIdList` | yes (read) |

Local validation failures wrap the exported sentinel `algo.ErrInvalidParams` and
no request is sent.

## Add a master order

```go
id, err := m.AddOrder(ctx, algo.AddOrderParams{
    StockCode:      "0700.HK",
    EntrustType:    algo.EntrustTypeLimit, // 1 limit, 2 market
    EntrustPrice:   "300",
    EntrustAmount:  "1000",
    EntrustBS:      types.EntrustBuy,
    TargetStrategy: algo.StrategyVWAP,
    SessionType:    algo.SessionTypeOff,  // 0 off, 1 pre/after hours
    StrategyParam: algo.StrategyParam{
        MaxVolume:   "100",
        Sensitivity: algo.SensitivityNeutral, // 1 neutral, 2 aggressive, 3 passive
    },
})
```

`AddOrder` requires `stockCode`, `exchangeType`, `entrustType`, a positive
`entrustPrice`, a positive `entrustAmount`, `entrustBs`, `targetStrategy`,
`sessionType`, and a positive `strategyParam.maxVolume` plus an in-set
`sensitivity`.

`StrategyParam` carries the per-strategy tuning. `MaxVolume` and `Sensitivity`
are required by `AddOrder`; every other member is optional.

| Field | Meaning |
|-------|---------|
| `OrigStartTime` / `OrigEndTime` | strategy window, `HHmmSS` Hong Kong time |
| `MaxVolume` | maximum quantity per child order |
| `MinAmount` | minimum traded amount per child order |
| `Sensitivity` | execution aggressiveness |
| `ShowQty` | displayed quantity per ICE_BERG child |
| `QtyPercent` | cumulative participation percentage (1–99) |
| `Interval` | child-order interval in seconds (default 60) |

## Change, cancel, and action

```go
changed, err := m.ChangeOrder(ctx, algo.ChangeOrderParams{
    OrderID:       id,
    StockCode:     "0700.HK",
    EntrustPrice:  "301",
    EntrustAmount: "1200",
    StrategyParam: algo.StrategyParam{Sensitivity: algo.SensitivityAggressive},
})

cancelled, err := m.CancelOrder(ctx, algo.CancelOrderParams{OrderID: id})

actioned, err := m.ActionOrder(ctx, algo.ActionOrderParams{
    OrderID:        id,
    Action:         algo.ActionStop,
    TargetStrategy: algo.StrategyVWAP,
})
```

`Action` is a closed set: `ActionStart` (`"1"`), `ActionStop` (`"2"`),
`ActionSuspend` (`"3"`), `ActionResume` (`"4"`). An unlisted value is rejected
before any request.

To cancel one child order of a master:

```go
child, err := m.CancelEntrust(ctx, algo.CancelEntrustParams{
    OrderID:   id,
    EntrustID: "123456789",
})
```

### Mutations are never retried

All five mutations issue exactly one attempt. On an ambiguous failure, reconcile
before resubmitting (ADR 0003).

## Query masters and child entrusts

```go
masters, err := m.QueryOrderList(ctx, algo.QueryOrderListParams{
    PageNo:    "1",
    PageSize:  "20",
    StartDate: "20260101", // yyyyMMdd
    EndDate:   "20260131",
    // ExchangeType and StockCode are optional; StockCode requires ExchangeType.
})
for _, o := range masters {
    fmt.Println(o.OrderID, o.StockCode, o.Status, o.TargetStrategy, o.CumQty, o.LeavesQty)
}

entrustIDs, err := m.QueryEntrustIDList(ctx, algo.QueryEntrustIDListParams{
    OrderID:   id,
    TradeDate: "20260131", // yyyyMMdd
})
```

## Enums

| Type | Constants |
|------|-----------|
| `Strategy` | `StrategyVWAP` (`1`), `StrategyTWAP` (`1001`), `StrategyIceberg` (`1002`), `StrategyTPOV` (`1003`), `StrategyPOV` (`1005`) |
| `EntrustType` | `EntrustTypeLimit` (`1`), `EntrustTypeMarket` (`2`) |
| `SessionType` | `SessionTypeOff` (`0`), `SessionTypeOn` (`1`) |
| `Sensitivity` | `SensitivityNeutral` (`1`), `SensitivityAggressive` (`2`), `SensitivityPassive` (`3`) |
| `Status` | `StatusRegistered` (`0`) … `StatusWaitModify` (`E`) |
| `StrategyStatus` | `StrategyStatusStart` (`1`) … `StrategyStatusResume` (`4`) |

`Strategy` is **non-exhaustive and ambiguous**: the reference labels `1005` as
both POV and INLINE, so only an empty value is rejected locally and unknown codes
are forwarded to the Gateway unchanged.

The algo `EntrustType` is distinct from the trade surface's
`types.EntrustType`; do not mix them.

## Status reference

Algo master orders use `algo.Status`, a separate enumeration from the trade
surface's `types.EntrustStatus`; do not mix them. Unknown codes are forwarded
to the Gateway unchanged.

| Value | Constant | Chinese | Meaning |
|-------|----------|---------|---------|
| `0` | `StatusRegistered` | 已报 | Accepted by the platform |
| `1` | `StatusPartFilled` | 部分成交 | Partially filled |
| `2` | `StatusFilled` | 全部成交 | Fully filled |
| `3` | `StatusCompleted` | 当日完成 | Completed for the day |
| `4` | `StatusCancelled` | 已撤 | Cancelled |
| `5` | `StatusModified` | 已改 | Modified |
| `6` | `StatusWaitCancel` | 待撤 | Waiting to cancel |
| `8` | `StatusRejected` | 废单 | Rejected |
| `A` | `StatusPending` | 待报 | Waiting to be submitted |
| `C` | `StatusExpired` | 过期 | Expired |
| `E` | `StatusWaitModify` | 待改 | Waiting to modify |
