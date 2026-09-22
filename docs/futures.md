# Futures

`future.Manager` exposes the eleven futures endpoints over a shared client.
Trading requires a live session; log in before calling any method.

```go
session := hstong.NewSessionManager(c)
if err := session.Login(ctx); err != nil {
    return err
}
m := future.New(c)
```

`future.Manager` does not take a session option: it issues each call directly and
relies on the login performed above.

## Endpoints

| Method | Route | Mutation |
|--------|-------|:--------:|
| `QueryProductInfo` | `/trade/FuturesQueryProductInfo` | no |
| `QueryMaxBuySellAmount` | `/trade/FuturesQueryMaxBuySellAmount` | no |
| `QueryFundInfo` | `/trade/FuturesQueryFundInfo` | no |
| `QueryHoldsList` | `/trade/FuturesQueryHoldsList` | no |
| `Entrust` | `/trade/FuturesEntrust` | **yes** |
| `CancelEntrust` | `/trade/FuturesCancelEntrust` | **yes** |
| `ModifyEntrust` | `/trade/FuturesModifyEntrust` | **yes** |
| `QueryRealEntrustList` | `/trade/FuturesQueryRealEntrustList` | no |
| `QueryHistoryEntrustList` | `/trade/FuturesQueryHistoryEntrustList` | no |
| `QueryRealDeliverList` | `/trade/FuturesQueryRealDeliverList` | no |
| `QueryHistoryDeliverList` | `/trade/FuturesQueryHistoryDeliverList` | no |

## Product information

```go
info, err := m.QueryProductInfo(ctx, future.QueryProductInfoRequest{
    StockCodes: []string{"HSI2603"},
})
for _, p := range info.ProductInfoVos {
    fmt.Println(p.ProdCode, p.LotSize, p.ContractSize, p.PriceDecimalPoint)
}
```

## Funds and positions

```go
funds, err := m.QueryFundInfo(ctx)
fmt.Println(funds.FundInfo.AssetBalance, funds.FundInfo.EnableBalance, funds.FundInfo.MarginStatus)

holds, err := m.QueryHoldsList(ctx)
for _, h := range holds.HoldsList {
    fmt.Println(h.StockCode, h.CurrentQty, h.ProfitLoss)
}
```

`QueryHoldsList` returns the positions **and** the account funds snapshot
(`holds.FundInfo`). Every monetary and quantity field is an exact decimal string.
`Hold.LastPrice` is documented as unreliable; prefer a market-data quote for
valuation.

## Capacity

```go
capacity, err := m.QueryMaxBuySellAmount(ctx, future.QueryMaxBuySellAmountRequest{
    StockCode: "HSI2603",
})
fmt.Println(capacity.MaxBuyAmount, capacity.MaxSellAmount, capacity.InitialMargin, capacity.Ccy)
```

## Place, change, and cancel

```go
resp, err := m.Entrust(ctx, future.EntrustRequest{
    StockCode:     "HSI2603",
    EntrustType:   "0",   // 0 limit, 1 auction, 2 market
    EntrustPrice:  "20000",
    EntrustAmount: "1",
    EntrustBS:     string(types.EntrustBuy), // 1 open long/buy, 2 close long/sell, 3 close short, 4 open short
    ValidTimeType: "0",   // 0 day, 1 IOC, 2 FOK, 3 GTD, 4 good-till-specified-date
})

changed, err := m.ModifyEntrust(ctx, future.ModifyEntrustRequest{
    EntrustID:     resp.Data,
    StockCode:     "HSI2603",
    EntrustPrice:  "20100",
    EntrustAmount: "1",
    EntrustBS:     string(types.EntrustBuy),
})

cancelled, err := m.CancelEntrust(ctx, future.CancelEntrustRequest{
    EntrustID: resp.Data,
    StockCode: "HSI2603",
})
```

Validation before any request: the contract code must be non-empty and contain no
whitespace; `entrustBs` must be `1`–`4`; `entrustAmount` must be a positive
decimal; `entrustPrice` is required for every type except a market order and must
always be a well-formed decimal when supplied; `ValidTimeType` `"4"` requires a
real `yyyyMMdd` `ValidTime`.

### Mutations are never retried

`Entrust`, `ModifyEntrust`, and `CancelEntrust` issue exactly one attempt. On an
ambiguous failure, reconcile by querying the real/history entrust and deliver
lists before resubmitting (ADR 0003).

## Queries

```go
real, err := m.QueryRealEntrustList(ctx)
for _, o := range real.Data {
    fmt.Println(o.EntrustID, o.StockCode, o.StatusDesc, o.CanBeCanceled)
}

delivers, err := m.QueryRealDeliverList(ctx)
fmt.Println(len(delivers.Data))

history, err := m.QueryHistoryEntrustList(ctx, future.HistoryQueryRequest{
    PageNo:    1,
    PageSize:  20,        // must be below 100
    StartDate: "20260101", // yyyyMMdd, optional
    EndDate:   "20260131",
})
```

`QueryRealEntrustList` returns at most 500 rows; `QueryRealDeliverList` at most
200. `EntrustListResponse` and `DeliverListResponse` carry `Data` plus, for a
history query, `CurPageNo`, `CurPageSize`, and `LastPage` (`1` on the last page).
`TotalPageNo` is documented as obsolete.

## Pagination defaults

| Setting | Default | Option |
|---------|---------|--------|
| Page size when `PageSize` is zero | `future.DefaultPageSize` (20) | `future.WithDefaultPageSize(n)` |
| First page when `PageNo` is zero | `1` | — |

An oversized `PageSize` (≥ 100) or a malformed `StartDate`/`EndDate` is rejected
locally before any request.

## Futures push

Futures trade deliveries arrive over the TCP push channel with notify type
`FuturesTradeStockDeliverMsgType` (2). Decode them with
`stream.Event.FuturesTradeDeliver`, or map the generated payload to
`future.DeliverNotification`:

```go
deliver, ok := ev.FuturesTradeDeliver()
if ok {
    n := future.FromDeliverNotify(deliver)
    if n.HasFill() {
        fmt.Println(n.StockCode, n.BusinessPrice, n.BusinessAmount, n.MatchNo)
    }
}
```

`future.FromDeliverNotify` returns a typed, string-only view of the payload;
`DeliverNotification.HasFill` reports whether `MatchNo` is populated (a fill
rather than a pure state change). See [Streaming](streaming.md).
