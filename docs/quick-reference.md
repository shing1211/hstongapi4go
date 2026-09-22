# Quick Reference

One-page summary of all 51 HTTP endpoints, push topics, and common request/response
shapes. For full documentation, see the linked pages.

## HTTP endpoints — 51 total

All routes are `POST http://127.0.0.1:11111<route>`. See [SPEC.md](./SPEC.md).

### Market data pull — 9

| Method | Route | Request | Response |
|--------|-------|---------|----------|
| `BasicQot` | `/hq/BasicQot` | `BasicQotRequest{Security[], MktTmType}` | `BasicQotResponse` |
| `OrderBook` | `/hq/OrderBook` | `OrderBookRequest{Security, MktTmType}` | `OrderBookResponse` |
| `KL` | `/hq/KL` | `KLRequest{Security, StartDate, CycType, ...}` | `KLResponse` |
| `TimeShare` | `/hq/TimeShare` | `TimeShareRequest{Security, MktTmType}` | `TimeShareResponse` |
| `Ticker` | `/hq/Ticker` | `TickerRequest{Security, Limit, MktTmType}` | `TickerResponse` |
| `Broker` | `/hq/Broker` | `BrokerRequest{Security}` | `BrokerResponse` |
| `UsOptionChainCode` | `/hq/UsOptionChainCode` | `UsOptionChainCodeRequest{SecurityCode, ExpireDate, ...}` | `UsOptionChainCodeResponse` |
| `UsOptionChainExpireDate` | `/hq/UsOptionChainExpireDate` | `UsOptionChainExpireDateRequest{SecurityCode}` | `UsOptionChainExpireDateResponse` |
| `UsOverNightTradeCodes` | `/hq/UsOverNightTradeCodes` | `UsOverNightTradeCodesRequest{}` | `UsOverNightTradeCodesResponse` |

### Market subscribe / unsubscribe — 2

| Method | Route | Request | Response |
|--------|-------|---------|----------|
| `Subscribe` | `/hq/Subscribe` | `SubscribeRequest{TopicID, Security[]}` | `SubscribeResponse` |
| `Unsubscribe` | `/hq/Unsubscribe` | `UnsubscribeRequest{TopicID, Security[]}` | `UnsubscribeResponse` |

### Trade session — 2

| Method | Route | Request | Response |
|--------|-------|---------|----------|
| `TradeLogin` | `/trade/TradeLogin` | `TradeLoginRequest{TradePassword}` | `TradeLoginResponse` |
| `TradeLogout` | `/trade/TradeLogout` | `TradeLogoutRequest{}` | `TradeLogoutResponse` |

### Trade assets / positions — 5

| Method | Route | Request | Response |
|--------|-------|---------|----------|
| `MarginFundInfo` | `/trade/TradeQueryMarginFundInfo` | `MarginFundInfoRequest{ExchangeType}` | `MarginFundInfo` |
| `Positions` | `/trade/TradeQueryHoldsList` | `PositionsRequest{ExchangeType}` | `[]HoldsVo` |
| `RealFundJourList` | `/trade/TradeQueryRealFundJourList` | `FundJourListRequest{ExchangeType, QueryCount, QueryParamStr}` | `[]FundJourVo` |
| `HistoryFundJourList` | `/trade/TradeQueryHistoryFundJourList` | `FundJourListRequest{ExchangeType, QueryCount, QueryParamStr}` | `[]FundJourVo` |
| `ExchangeRate` | `/hs/rate/queryList` | `ExchangeRateRequest{RateType}` | `map[string]map[string]string` |

### Trade orders — 13

| Method | Route | Request | Response |
|--------|-------|---------|----------|
| `Entrust` | `/trade/TradeEntrust` | `EntrustRequest{...}` | `string` (entrust ID) |
| `CancelEntrust` | `/trade/TradeCancelEntrust` | `CancelEntrustRequest{...}` | `string` |
| `BatchCancelEntrust` | `/trade/TradeBatchCancelEntrust` | `BatchCancelEntrustRequest{...}` | `BatchCancelEntrustResult` |
| `ChangeEntrust` | `/trade/TradeChangeEntrust` | `ChangeEntrustRequest{...}` | `string` |
| `MaxAvailableAsset` | `/trade/TradeQueryMaxAvailableAsset` | `MaxAvailableAssetRequest{...}` | `MaxAvailableAsset` |
| `RealEntrustList` | `/trade/TradeQueryRealEntrustList` | `RealEntrustListRequest{...}` | `[]OrderVo` |
| `RealDeliverList` | `/trade/TradeQueryRealDeliverList` | `RealDeliverListRequest{...}` | `[]OrderVo` |
| `RealCondOrderList` | `/trade/TradeQueryRealCondOrderList` | `CondOrderListRequest{...}` | `*CondOrderPage` |
| `HistoryEntrustList` | `/trade/TradeQueryHistoryEntrustList` | `HistoryEntrustListRequest{...}` | `[]OrderVo` |
| `HistoryDeliverList` | `/trade/TradeQueryHistoryDeliverList` | `HistoryDeliverListRequest{...}` | `[]OrderVo` |
| `HistoryCondOrderList` | `/trade/TradeQueryHistoryCondOrderList` | `HistoryCondOrderListRequest{...}` | `*CondOrderPage` |
| `MarginFullInfo` | `/trade/TradeQueryMarginFullInfo` | `MarginFullInfoRequest{ExchangeType}` | `MarginFullInfo` |
| `BeforeAndAfterSupport` | `/trade/TradeQueryBeforeAndAfterSupport` | `BeforeAndAfterSupportRequest{ExchangeType}` | `string` ("1" supported) |

### Trade push subscribe — 2

| Method | Route | Request | Response |
|--------|-------|---------|----------|
| `SubscribeOrders` | `/trade/TradeSubscribe` | `TradeSubscribeRequest{}` | `TradeSubscribeResponse` |
| `UnsubscribeOrders` | `/trade/TradeUnsubscribe` | `TradeUnsubscribeRequest{}` | `TradeUnsubscribeResponse` |

### Algo / strategy — 7

| Method | Route | Request | Response |
|--------|-------|---------|----------|
| `AddOrder` | `/trade/AlgoAddOrder` | `AddOrderParams{...}` | `string` (order ID) |
| `CancelOrder` | `/trade/AlgoCancelOrder` | `CancelOrderParams{OrderID}` | `string` |
| `CancelEntrust` | `/trade/AlgoCancelEntrust` | `CancelEntrustParams{...}` | `string` |
| `ChangeOrder` | `/trade/AlgoChangeOrder` | `ChangeOrderParams{...}` | `string` |
| `ActionOrder` | `/trade/AlgoActionOrder` | `ActionOrderParams{...}` | `string` |
| `QueryOrderList` | `/trade/AlgoQueryOrderList` | `QueryOrderListParams{...}` | `[]MasterOrder` |
| `QueryEntrustIDList` | `/trade/AlgoQueryEntrustIdList` | `QueryEntrustIDListParams{...}` | `[]string` |

### Futures — 11

| Method | Route | Request | Response |
|--------|-------|---------|----------|
| `QueryProductInfo` | `/trade/FuturesQueryProductInfo` | `QueryProductInfoRequest{StockCodes[]}` | `[]ProductInfo` |
| `QueryMaxBuySellAmount` | `/trade/FuturesQueryMaxBuySellAmount` | `QueryMaxBuySellAmountRequest{StockCode}` | `QueryMaxBuySellAmountResponse` |
| `QueryFundInfo` | `/trade/FuturesQueryFundInfo` | `QueryFundInfoRequest{}` | `FundInfo` |
| `QueryHoldsList` | `/trade/FuturesQueryHoldsList` | `QueryHoldsListRequest{}` | `QueryHoldsListResponse` |
| `Entrust` | `/trade/FuturesEntrust` | `EntrustRequest{...}` | `string` (entrust ID) |
| `CancelEntrust` | `/trade/FuturesCancelEntrust` | `CancelEntrustRequest{...}` | `string` |
| `ModifyEntrust` | `/trade/FuturesModifyEntrust` | `ModifyEntrustRequest{...}` | `string` |
| `QueryRealEntrustList` | `/trade/FuturesQueryRealEntrustList` | `QueryRealEntrustListRequest{}` | `EntrustListResponse` |
| `QueryHistoryEntrustList` | `/trade/FuturesQueryHistoryEntrustList` | `HistoryQueryRequest{...}` | `EntrustListResponse` |
| `QueryRealDeliverList` | `/trade/FuturesQueryRealDeliverList` | `QueryRealDeliverListRequest{}` | `DeliverListResponse` |
| `QueryHistoryDeliverList` | `/trade/FuturesQueryHistoryDeliverList` | `HistoryQueryRequest{...}` | `DeliverListResponse` |

## Push topics — 11

| Group | topicId | Payload type |
|-------|---------|--------------|
| Quote | `11`, `35` | `BasicQotNotify` |
| Tick | `14`, `27`, `28`, `37` | `TickerNotify` |
| Broker queue | `16` | `BrokerNotify` |
| Order book | `17`, `25`, `26`, `36` | `OrderBookFullNotify` |

Trade and futures push use `TradeStockDeliverNotify` on types `0`, `1`, and `2`.

## Common types

### EntrustBS values

| Value | Meaning |
|-------|---------|
| `1` | buy / open long |
| `2` | sell / close long |
| `3` | close short |
| `4` | open short |

### EntrustType values

| Value | Market | Meaning |
|-------|--------|---------|
| `0` | HK | 竞价限价 auction limit |
| `1` | HK | 竞价 auction |
| `2` | HK | 增强限价盘 enhanced limit |
| `3` | HK/US/A-share | 限价盘 limit |
| `5` | US | 市价盘 market |
| `31` | Conditional | 止盈限价 stop-profit limit |
| `33` | Conditional | 止损限价 stop-loss limit |

### ExchangeType values

| Constant | Value | Market |
|----------|-------|--------|
| `ExchangeHK` | `K` | Hong Kong |
| `ExchangeUS` | `P` | US |
| `ExchangeShenzhenConnect` | `v` | Shenzhen Connect |
| `ExchangeShanghaiConnect` | `t` | Shanghai Connect |

### MktTmType values

| Value | Meaning |
|-------|---------|
| `-3` | overnight (US) |
| `-2` | post-market (US) |
| `-1` | pre-market (US) |
| `0` | Hong Kong standard |
| `1` | intraday |

### ExRightFlag values

| Value | Meaning |
|-------|---------|
| `0` | no adjustment |
| `1` | forward adjustment |
| `2` | backward adjustment |

## Client setup

```go
c, err := client.New(
    client.WithEnv(),                   // reads HSTONG_*
    client.WithTimeout(10*time.Second),
    client.WithLogger(slog.Default()),
)
```

## Pagination constants

| Constant | Value |
|----------|-------|
| `DefaultPageSize` | `20` |
| `MaxPageSize` | `99` |

## Limits

| Item | Value |
|------|-------|
| Gateway HTTP | `http://127.0.0.1:11111` |
| Gateway TCP push | `127://127.0.0.1:11112` |
| Concurrent subscriptions | ≤ 200 securities |
| Ticker limit | ≤ 100 |
| Condition order valid days | ≤ 100 |
| Token TTL | 3 hours |
