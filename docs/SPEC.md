# HStong OpenAPI Gateway — Canonical Specification Index (v1)

- **Protocol package:** HStong protobuf definitions **v2.2.0** (17 `.proto` files;
  see [proto/PROVENANCE.md](../proto/PROVENANCE.md))
- **Gateway:** **v2.4.1**
- **Run:** `2026-09-21-hstong-full-surface`, task T03

> **Canonical counts live here.** Any endpoint, topic, or schema count quoted
> elsewhere in this repository (READMEs, DESIGN, LEGACY, mkdocs, CI checks) must
> be sourced from this file. Do not hand-edit counts anywhere else (AGENTS.md
> hard rule 5).

## 1. Transport

| Item | Value |
|------|-------|
| Gateway base (HTTP) | `http://127.0.0.1:11111` |
| Gateway push (TCP) | `127.0.0.1:11112` |
| HTTP method | `POST` only — no HTTPS, no GET |
| Route aliases | `/X`, `/XRequest`, `/XRequestMsgType` |
| Request envelope | `{"timeout_sec":10,"params":{...}}` |
| Response envelope | `{"ok":true,"err":"","data":{...}}` |
| Push frame | 151-byte header + `PBNotify` (see §5) |
| Signature | none in the SDK — the Gateway owns signing and encryption |

## 2. HTTP endpoints — 51 total

Grouped by surface, exactly as approved in the plan (Appendix A). Every route is
`POST http://127.0.0.1:11111<route>`.

### 2.1 Market pull — 9

| # | Route | Full URL |
|---|-------|----------|
| 1 | `/hq/BasicQot` | `POST http://127.0.0.1:11111/hq/BasicQot` |
| 2 | `/hq/OrderBook` | `POST http://127.0.0.1:11111/hq/OrderBook` |
| 3 | `/hq/KL` | `POST http://127.0.0.1:11111/hq/KL` |
| 4 | `/hq/TimeShare` | `POST http://127.0.0.1:11111/hq/TimeShare` |
| 5 | `/hq/Ticker` | `POST http://127.0.0.1:11111/hq/Ticker` |
| 6 | `/hq/Broker` | `POST http://127.0.0.1:11111/hq/Broker` |
| 7 | `/hq/UsOptionChainCode` | `POST http://127.0.0.1:11111/hq/UsOptionChainCode` |
| 8 | `/hq/UsOptionChainExpireDate` | `POST http://127.0.0.1:11111/hq/UsOptionChainExpireDate` |
| 9 | `/hq/UsOverNightTradeCodes` | `POST http://127.0.0.1:11111/hq/UsOverNightTradeCodes` |

### 2.2 Market subscription — 2

| # | Route | Full URL |
|---|-------|----------|
| 10 | `/hq/Subscribe` | `POST http://127.0.0.1:11111/hq/Subscribe` |
| 11 | `/hq/Unsubscribe` | `POST http://127.0.0.1:11111/hq/Unsubscribe` |

### 2.3 Trade session — 2

| # | Route | Full URL |
|---|-------|----------|
| 12 | `/trade/TradeLogin` | `POST http://127.0.0.1:11111/trade/TradeLogin` |
| 13 | `/trade/TradeLogout` | `POST http://127.0.0.1:11111/trade/TradeLogout` |

### 2.4 Trade assets / positions — 5

| # | Route | Full URL |
|---|-------|----------|
| 14 | `/trade/TradeQueryMarginFundInfo` | `POST http://127.0.0.1:11111/trade/TradeQueryMarginFundInfo` |
| 15 | `/trade/TradeQueryHoldsList` | `POST http://127.0.0.1:11111/trade/TradeQueryHoldsList` |
| 16 | `/trade/TradeQueryRealFundJourList` | `POST http://127.0.0.1:11111/trade/TradeQueryRealFundJourList` |
| 17 | `/trade/TradeQueryHistoryFundJourList` | `POST http://127.0.0.1:11111/trade/TradeQueryHistoryFundJourList` |
| 18 | `/hs/rate/queryList` | `POST http://127.0.0.1:11111/hs/rate/queryList` |

### 2.5 Trade orders — 13

| # | Route | Full URL |
|---|-------|----------|
| 19 | `/trade/TradeEntrust` | `POST http://127.0.0.1:11111/trade/TradeEntrust` |
| 20 | `/trade/TradeCancelEntrust` | `POST http://127.0.0.1:11111/trade/TradeCancelEntrust` |
| 21 | `/trade/TradeBatchCancelEntrust` | `POST http://127.0.0.1:11111/trade/TradeBatchCancelEntrust` |
| 22 | `/trade/TradeChangeEntrust` | `POST http://127.0.0.1:11111/trade/TradeChangeEntrust` |
| 23 | `/trade/TradeQueryMaxAvailableAsset` | `POST http://127.0.0.1:11111/trade/TradeQueryMaxAvailableAsset` |
| 24 | `/trade/TradeQueryRealEntrustList` | `POST http://127.0.0.1:11111/trade/TradeQueryRealEntrustList` |
| 25 | `/trade/TradeQueryRealDeliverList` | `POST http://127.0.0.1:11111/trade/TradeQueryRealDeliverList` |
| 26 | `/trade/TradeQueryRealCondOrderList` | `POST http://127.0.0.1:11111/trade/TradeQueryRealCondOrderList` |
| 27 | `/trade/TradeQueryHistoryEntrustList` | `POST http://127.0.0.1:11111/trade/TradeQueryHistoryEntrustList` |
| 28 | `/trade/TradeQueryHistoryDeliverList` | `POST http://127.0.0.1:11111/trade/TradeQueryHistoryDeliverList` |
| 29 | `/trade/TradeQueryHistoryCondOrderList` | `POST http://127.0.0.1:11111/trade/TradeQueryHistoryCondOrderList` |
| 30 | `/trade/TradeQueryMarginFullInfo` | `POST http://127.0.0.1:11111/trade/TradeQueryMarginFullInfo` |
| 31 | `/trade/TradeQueryBeforeAndAfterSupport` | `POST http://127.0.0.1:11111/trade/TradeQueryBeforeAndAfterSupport` |

### 2.6 Trade push subscribe — 2

| # | Route | Full URL |
|---|-------|----------|
| 32 | `/trade/TradeSubscribe` | `POST http://127.0.0.1:11111/trade/TradeSubscribe` |
| 33 | `/trade/TradeUnsubscribe` | `POST http://127.0.0.1:11111/trade/TradeUnsubscribe` |

### 2.7 Algo / strategy — 7

| # | Route | Full URL |
|---|-------|----------|
| 34 | `/trade/AlgoAddOrder` | `POST http://127.0.0.1:11111/trade/AlgoAddOrder` |
| 35 | `/trade/AlgoCancelOrder` | `POST http://127.0.0.1:11111/trade/AlgoCancelOrder` |
| 36 | `/trade/AlgoCancelEntrust` | `POST http://127.0.0.1:11111/trade/AlgoCancelEntrust` |
| 37 | `/trade/AlgoChangeOrder` | `POST http://127.0.0.1:11111/trade/AlgoChangeOrder` |
| 38 | `/trade/AlgoActionOrder` | `POST http://127.0.0.1:11111/trade/AlgoActionOrder` |
| 39 | `/trade/AlgoQueryOrderList` | `POST http://127.0.0.1:11111/trade/AlgoQueryOrderList` |
| 40 | `/trade/AlgoQueryEntrustIdList` | `POST http://127.0.0.1:11111/trade/AlgoQueryEntrustIdList` |

### 2.8 Futures — 11

| # | Route | Full URL |
|---|-------|----------|
| 41 | `/trade/FuturesQueryProductInfo` | `POST http://127.0.0.1:11111/trade/FuturesQueryProductInfo` |
| 42 | `/trade/FuturesQueryMaxBuySellAmount` | `POST http://127.0.0.1:11111/trade/FuturesQueryMaxBuySellAmount` |
| 43 | `/trade/FuturesQueryFundInfo` | `POST http://127.0.0.1:11111/trade/FuturesQueryFundInfo` |
| 44 | `/trade/FuturesQueryHoldsList` | `POST http://127.0.0.1:11111/trade/FuturesQueryHoldsList` |
| 45 | `/trade/FuturesEntrust` | `POST http://127.0.0.1:11111/trade/FuturesEntrust` |
| 46 | `/trade/FuturesCancelEntrust` | `POST http://127.0.0.1:11111/trade/FuturesCancelEntrust` |
| 47 | `/trade/FuturesModifyEntrust` | `POST http://127.0.0.1:11111/trade/FuturesModifyEntrust` |
| 48 | `/trade/FuturesQueryRealEntrustList` | `POST http://127.0.0.1:11111/trade/FuturesQueryRealEntrustList` |
| 49 | `/trade/FuturesQueryHistoryEntrustList` | `POST http://127.0.0.1:11111/trade/FuturesQueryHistoryEntrustList` |
| 50 | `/trade/FuturesQueryRealDeliverList` | `POST http://127.0.0.1:11111/trade/FuturesQueryRealDeliverList` |
| 51 | `/trade/FuturesQueryHistoryDeliverList` | `POST http://127.0.0.1:11111/trade/FuturesQueryHistoryDeliverList` |

**Total: 51 HTTP endpoints** (9 + 2 + 2 + 5 + 13 + 2 + 7 + 11).

Response schemas are documented in §3. Each table's fields are copied verbatim from the
corresponding Go struct field comments in `pkg/hstong/trade/`, `pkg/hstong/future/`, and
`pkg/hstong/algo/`. Deprecated fields are marked; see §9.

## 3. Response schemas

Field types follow the Go struct definition: `string` is the JSON string the Gateway
sent; `int` / `int32` / `int64` are JSON numbers; `[]T` is a JSON array.

---

### Tier 1 — complex nested objects

---

#### OrderVo — one entrust or deliver row (shared by real and history queries)

| Field | Type | Description |
|-------|------|-------------|
| `stockCode` | `string` | Security code |
| `stockName` | `string` | Security name |
| `businessPrice` | `string` | Execution price |
| `entrustBs` | `string` | Order direction |
| `entrustPrice` | `string` | Order price |
| `businessBalance` | `string` | Executed amount |
| `entrustAmount` | `string` | Order quantity |
| `businessAmount` | `string` | Executed quantity |
| `date` | `string` | Execution date (deliver) or order date (entrust) |
| `businessTime` | `string` | Execution time |
| `entrustTime` | `string` | Order time |
| `queryParamStr` | `string` | Cursor for the next page |
| `statusDesc` | `string` | Localized order-status description |
| `status` | `string` | Order-status code |
| `entrustId` | `string` | Order identifier |
| `unBusinessAmount` | `string` | Unfilled quantity |
| `canBeCanceled` | `int32` | 1 when the order can be cancelled, 0 otherwise |
| `entrustType` | `string` | Order type |
| `opponentSeat` | `string` | Counterparty seat; present for intraday delivers only |
| `entrustTypeNum` | `string` | Numeric order type |
| `remarkType` | `string` | Remark type: 0 none, 1 rejected |
| `remark` | `string` | Free-text remark |
| `fareVo` | `*FareVo` | Itemized fees; nil for unsettled intraday delivers |
| `exchangeType` | `string` | Market |
| `canBeUpdated` | `int32` | 1 when the order can be modified, 0 otherwise |
| `exchange` | `string` | Exchange |

##### FareVo — itemized fee breakdown

| Field | Type | Description |
|-------|------|-------------|
| `fare0` | `string` | Commission |
| `fare1` | `string` | HK stamp duty / US SEC fee |
| `fare2` | `string` | HK trading fee / US trading activity fee |
| `fare3` | `string` | Transaction levy |
| `fare4` | `string` | US option regulatory fee |
| `fare5` | `string` | US option clearing fee |
| `fare6` | `string` | Platform fee |
| `fare7` | `string` | Reserved |
| `fare8` | `string` | US consolidated audit trail regulatory fee |
| `fare9` | `string` | HK AFRC transaction levy |
| `farex` | `string` | HK settlement fee / US settlement fee |
| `faret` | `string` | Total trading fee |

---

#### CondOrderVo — one conditional order

| Field | Type | Description |
|-------|------|-------------|
| `condOrderId` | `string` | Condition-order identifier |
| `dataType` | `string` | Numeric instrument type |
| `stockCode` | `string` | Security code |
| `stockName` | `string` | Security name |
| `stockNameTc` | `string` | Traditional-Chinese security name |
| `stockNameEn` | `string` | English security name |
| `exchangeType` | `string` | Market |
| `entrustType` | `string` | Conditional order type (31–36) |
| `sessionType` | `string` | Trading session |
| `status` | `string` | Status (1 pending, 2 triggered, 3 paused, 4 expired, 5 deleted, 6 error, 8 stop invalidated, 9 ex-rights invalidated) |
| `canBeCancel` | `string` | "1" when the order can be cancelled |
| `canBeModify` | `string` | "1" when the order can be modified |
| `entrustBs` | `string` | Order direction |
| `entrustAmount` | `string` | Order quantity |
| `createTime` | `string` | Order creation time |
| `startTime` | `string` | Order activation time |
| `endTime` | `string` | Order expiry time |
| `errorCode` | `string` | Error code (when status is error) |
| `errorMsg` | `string` | Error message (when status is error) |
| `condValue` | `string` | Trigger value: price, spread, or percentage |
| `condPrice` | `string` | Trigger reference price |
| `condTrackType` | `string` | Tracking type: 1 percentage, 2 spread, 3 price |

---

#### HoldsVo — one position

| Field | Type | Description |
|-------|------|-------------|
| `stockName` | `string` | Security name |
| `enableAmount` | `string` | Sellable quantity |
| `currentAmount` | `string` | Held quantity |
| `stockCode` | `string` | Security code |
| `costPrice` | `string` | Cost price |
| `lastPrice` | `string` | Latest price (**deprecated**: unreliable) |
| `incomeBalance` | `string` | Floating profit/loss (**deprecated**) |
| `marketValue` | `string` | Market value (**deprecated**) |
| `marketValueRate` | `string` | Profit/loss ratio (**deprecated**) |
| `incomeRatio` | `string` | Market-value proportion (**deprecated**) |
| `dayCostPrice` | `string` | Intraday cost price |
| `dayInComeAmount` | `string` | Effective intraday deposit |
| `dayOutComeAmount` | `string` | Effective intraday withdrawal |
| `keepCostPrice` | `string` | Break-even price |
| `exchangeType` | `string` | Market of the position |

---

#### MarginFundInfo — account-funds snapshot (per market)

| Field | Type | Description |
|-------|------|-------------|
| `holdsBalance` | `string` | Position profit/loss (**deprecated**: unreliable) |
| `assetBalance` | `string` | Total assets |
| `enableBalance` | `string` | Available cash |
| `marketValue` | `string` | Securities market value (**deprecated**) |
| `cashOnHold` | `string` | Frozen trading amount |
| `creditValue` | `string` | Used credit |
| `creditLine` | `string` | Credit limit |
| `fetchBalance` | `string` | Withdrawable cash |
| `frozenBalance` | `string` | Total frozen amount |
| `accountStatus` | `string` | Account status |
| `spentRatio` | `string` | Fund-usage ratio |
| `currentCreditLimit` | `string` | Current credit limit |
| `maxCreditLimit` | `string` | Raised maximum credit limit |
| `unitCreditLimit` | `string` | Unified credit limit |
| `unitMaxCreditLimit` | `string` | Maximum unified credit limit |
| `buyPower` | `string` | Purchasing power for the primary currency |
| `buyPowerCredit` | `string` | Purchasing power after credit-limit increase |
| `buyPowerHk` | `string` | Hong Kong purchasing power |
| `buyPowerUs` | `string` | US purchasing power |
| `buyPowerCn` | `string` | Stock Connect purchasing power |
| `unitedBuyPowerHk` | `string` | Unified Hong Kong purchasing power |
| `unitedBuyPowerUs` | `string` | Unified US purchasing power |
| `unitedBuyPowerCn` | `string` | Unified Stock Connect purchasing power |
| `thirdBuyPowerHk` | `string` | Third-party Hong Kong purchasing power |
| `thirdBuyPowerUs` | `string` | Third-party US purchasing power |
| `thirdBuyPowerCn` | `string` | Third-party Stock Connect purchasing power |
| `buyPowerShortMarket` | `string` | Short-selling purchasing power |
| `buyPowerMoney` | `string` | Cash purchasing power |

---

#### MaxAvailableAsset — maximum buy/sell quantity and option margin

| Field | Type | Description |
|-------|------|-------------|
| `positionStatus` | `string` | Position status: 0 flat, 1 long, 2 short |
| `position` | `string` | Current position quantity |
| `longOpenAvailable` | `string` | Long open (buy) quantity available |
| `longCloseAvailable` | `string` | Long close (sell) quantity available |
| `cashAvailableToOpen` | `string` | Long open quantity available with cash |
| `marginAvailableToOpen` | `string` | Long open quantity available with margin |
| `cashAndMarginAvailableToOpen` | `string` | Long open quantity available with cash plus margin |
| `cashAvailableAmount` | `string` | Cash purchasing power |
| `marginAvailableAmount` | `string` | Margin purchasing power |
| `cashAndMarginAvailableAmount` | `string` | Combined cash-plus-margin purchasing power |
| `shortOpenAvailable` | `string` | Short open (sell) quantity available |
| `shortCloseAvailable` | `string` | Short close (buy) quantity available |
| `shortCloseCashAvailable` | `string` | Short close (buy) quantity available with cash |
| `shortOpenPool` | `string` | Short-selling pool |
| `shortBuyPower` | `string` | Short-selling purchasing power |
| `contractSize` | `string` | Option contract multiplier |
| `unitedBuyPowerStatus` | `string` | Unified-purchasing-power status |
| `creditLimit` | `string` | Overdraft limit |
| `optionLongMarginAmount` | `string` | Option margin for a long position |
| `optionShortMarginAmount` | `string` | Option margin for a short position |

---

#### FundInfo — futures account funds snapshot

| Field | Type | Description |
|-------|------|-------------|
| `assetBalance` | `string` | Net asset value (资产净值) |
| `enableBalance` | `string` | Available funds / buying power (可用金额) |
| `marginCall` | `string` | Margin call amount (追缴保证金) |
| `incomeBalance` | `string` | Open-position profit/loss (持仓盈亏) |
| `cashBal` | `string` | Cash balance (现金结余) |
| `iMargin` | `string` | Initial margin (基本保证金) |
| `mMargin` | `string` | Maintenance margin (维持保证金) |
| `marginLevel` | `string` | Margin level (保证金水平) |
| `maxMargin` | `string` | Maximum margin (最高保证金) |
| `creditLimit` | `string` | Credit limit (信贷限额) |
| `ctrlLevel` | `string` | Control level (控制级数) |
| `marginClass` | `string` | Margin type (保证金类型) |
| `aeId` | `string` | Broker (经纪) |
| `cashBalHKD` | `string` | HKD cash balance (港币现金结余) |
| `cashBalUSD` | `string` | USD cash balance (美元现金结余) |
| `cycRateUSDtoHSD` | `string` | USD/HKD exchange rate (美元兑港币汇率) |
| `marginStatus` | `string` | Risk state: 1 safe, 2 warning, 3 danger, 4 liquidation |
| `statusPercent` | `string` | Risk gauge percentage (风险状态画图百分比) |
| `closeProfit` | `string` | Realised profit/loss (已实现盈亏) |
| `cashBalCNH` | `string` | CNH cash balance (人民币现金结余) |

---

#### Hold — one futures position row

| Field | Type | Description |
|-------|------|-------------|
| `stockName` | `string` | Contract name (股票名称) |
| `stockCode` | `string` | Contract code; HK codes carry ".HK" suffix |
| `lastDayQty` | `string` | Previous day's position quantity (上日持仓数量) |
| `lastDayPrice` | `string` | Previous day's holding cost (上日持仓成本) |
| `depQty` | `string` | Stored position (存储仓位) |
| `dayLongQty` | `string` | Today's long quantity (今日长仓数量) |
| `dayLongPrice` | `string` | Today's long average price (今日长仓均价) |
| `dayShortQty` | `string` | Today's short quantity (今日短仓数量) |
| `dayShortPrice` | `string` | Today's short average price (今日短仓均价) |
| `dayNetQty` | `string` | Today's net quantity (今日净仓数量) |
| `dayNetPrice` | `string` | Today's net average price (今日净仓均价) |
| `currentQty` | `string` | Current position quantity (持仓数量) |
| `costPrice` | `string` | Cost price (成本价) |
| `lastPrice` | `string` | Current price (现价) (**deprecated**: unreliable) |
| `preClosePrice` | `string` | Previous close (昨收价) |
| `profitLoss` | `string` | Unrealised profit/loss (盈亏) |
| `ccyRate` | `string` | Reference conversion rate (参考兑换率) |
| `contractValue` | `string` | Contract value (合约值) |
| `profitLossBaseCcy` | `string` | Profit/loss in the base currency (盈亏基本货币) |
| `ccy` | `string` | Product series trading currency |
| `dataType` | `string` | Futures instrument type (distinguishes HK from US futures) |
| `closeProfit` | `string` | Realised profit/loss (已实现盈亏) |
| `closeProfitHKD` | `string` | Realised profit/loss in HKD (已实现盈亏HKD) |

---

#### EntrustOrder — one futures entrust or deliver row

| Field | Type | Description |
|-------|------|-------------|
| `stockCode` | `string` | Contract code; HK codes carry ".HK" suffix |
| `stockName` | `string` | Contract name |
| `businessPrice` | `string` | Fill price (成交价格) |
| `entrustBs` | `string` | Direction: 1 buy, 2 sell |
| `entrustPrice` | `string` | Order price (委托价格) |
| `entrustAmount` | `string` | Order quantity (委托数量) |
| `businessAmount` | `string` | Filled quantity (成交数量) |
| `date` | `string` | Fill date (deliver) or order date (entrust) |
| `businessTime` | `string` | Fill time (成交时间) |
| `entrustTime` | `string` | Order time (委托时间) |
| `queryParamStr` | `string` | Record cursor for pagination |
| `statusDesc` | `string` | Order status description (委托状态中文描述) |
| `status` | `string` | Order status code (委托状态) |
| `entrustId` | `string` | Order number (委托编号) |
| `canBeCanceled` | `int32` | 1 when the order can be cancelled, else 0 |
| `entrustType` | `string` | Order type |
| `entrustTypeNum` | `string` | Numeric order type |
| `isValid` | `int32` | 1 when the row is valid, else 0 |
| `canBeUpdated` | `int32` | 1 when the order can be modified, else 0 |
| `validType` | `string` | Time-in-force code |
| `validTypeDesc` | `string` | Description of ValidType; for a specified date holds the date |
| `orderOptions` | `int32` | 0 default or 1 T+1 |
| `validTime` | `string` | Expiry in yyyy/MM/dd form |

---

#### MasterOrder — one algorithm master order

| Field | Type | Description |
|-------|------|-------------|
| `orderId` | `string` | Master order identifier |
| `stockCode` | `string` | Security code; HK codes carry ".HK" suffix |
| `exchangeType` | `string` | Market the order trades on |
| `tradeDate` | `string` | Order's trade date |
| `entrustType` | `string` | Algorithm order type |
| `entrustPrice` | `string` | Limit price |
| `entrustAmount` | `string` | Total ordered quantity |
| `cumQty` | `string` | Cumulative filled quantity |
| `leavesQty` | `string` | Remaining unfilled quantity |
| `status` | `string` | Master's lifecycle state |
| `entrustBs` | `string` | Buy/sell direction |
| `targetStrategy` | `string` | Bound execution algorithm |
| `strategyStatus` | `string` | Strategy's run state |
| `strategyParam` | `*StrategyParam` | Algorithm tuning |
| `roundLot` | `string` | Security's board lot size |
| `sendingTime` | `string` | Transport timestamp |
| `transactionTime` | `string` | Order timestamp |
| `avgPx` | `string` | Average execution price |

##### StrategyParam — algorithm tuning object

| Field | Type | Description |
|-------|------|-------------|
| `origStartTime` | `string` | Strategy start time in Hong Kong time, HHmmSS. Empty means exchange open |
| `origEndTime` | `string` | Strategy end time in Hong Kong time, HHmmSS. Empty means exchange close |
| `maxVolume` | `string` | Maximum quantity of each child order |
| `minAmount` | `string` | Minimum traded amount of each child order |
| `sensitivity` | `string` | Execution aggressiveness: 1 neutral, 2 aggressive, 3 passive |
| `showQty` | `string` | Displayed quantity of each ICE_BERG child order |
| `qtyPercent` | `string` | Cumulative participation percentage (1–99) for POV/INLINE strategies |
| `interval` | `int` | Child-order interval in seconds for POV/INLINE strategies; zero means 60s default |

---

#### MarginFullInfo — securities margin snapshot

| Field | Type | Description |
|-------|------|-------------|
| `marginAllow` | `string` | "1" when margin financing is supported, "0" otherwise |
| `marginInitRatio` | `string` | Margin initial ratio |
| `marginKeepRatio` | `string` | Margin maintenance ratio |
| `shortAllow` | `string` | "1" when securities lending is supported, "0" otherwise |
| `shortInitMarginRatio` | `string` | Short initial margin ratio |
| `shortInterestRate` | `string` | Short reference interest rate |
| `shortKeepMarginRatio` | `string` | Short maintenance margin ratio |
| `shortLastAvailableQty` | `string` | Remaining short-selling pool |
| `rate` | `[]RateVo` | Margin interest rate per currency |

##### RateVo — currency margin interest rate

| Field | Type | Description |
|-------|------|-------------|
| `currencyCode` | `string` | Currency code |
| `currencyDesc` | `string` | Currency description |
| `interestRateWithinMortgage` | `string` | Margin reference interest rate |

---

#### FundJourVo — one fund-journey (资金流水) row

| Field | Type | Description |
|-------|------|-------------|
| `businessBalance` | `string` | Occurred amount |
| `type` | `string` | Numeric flow type |
| `typeDesc` | `string` | Flow type description |
| `fundJourParentType` | `string` | Parent flow type |
| `time` | `string` | Settlement date; present for historical rows only |
| `queryParamStr` | `string` | Cursor for the next page |

---

### Tier 2 — scalar wrappers, array containers, and simple types

---

#### CommonStringResponse — single-string data body

Used by: `/trade/TradeEntrust`, `/trade/TradeChangeEntrust`, `/trade/TradeQueryBeforeAndAfterSupport`

| Field | Type | Description |
|-------|------|-------------|
| `data` | `string` | The response value (entrust identifier or support flag) |

#### CommonIntResponse — single-integer data body

Used by: `/trade/TradeCancelEntrust` (stockCode field)

| Field | Type | Description |
|-------|------|-------------|
| `data` | `int` | The response value |

#### EntrustResponse — futures order mutation body

Used by: `/trade/FuturesEntrust`, `/trade/FuturesCancelEntrust`, `/trade/FuturesModifyEntrust`

| Field | Type | Description |
|-------|------|-------------|
| `data` | `string` | Order number, or empty string when the Gateway omitted it |

#### orderIDResponse — algo mutation body

Used by: `/trade/AlgoAddOrder`, `/trade/AlgoCancelOrder`, `/trade/AlgoCancelEntrust`, `/trade/AlgoChangeOrder`, `/trade/AlgoActionOrder`

| Field | Type | Description |
|-------|------|-------------|
| `data` | `string` | Master or child identifier echoed by the Gateway |

#### ExchangeRates — currency rate map

Used by: `/hs/rate/queryList`

| Field | Type | Description |
|-------|------|-------------|
| (outer key) | `string` | Source currency code |
| (inner key) | `string` | Target currency code |
| (value) | `string` | Decimal rate string, for example `rates["HKD"]["USD"]` |

#### BatchCancelEntrustResult — batch cancel outcome

Used by: `/trade/TradeBatchCancelEntrust`

| Field | Type | Description |
|-------|------|-------------|
| `successEntrustId` | `[]string` | Identifiers that were cancelled |
| `failCancelEntrust` | `[]FailCancelEntrustVo` | Identifiers that could not be cancelled |

##### FailCancelEntrustVo — one batch-cancel failure

| Field | Type | Description |
|-------|------|-------------|
| `failEntrustId` | `string` | Identifier that could not be cancelled |
| `remark` | `string` | Failure reason |

#### HoldsListResponse — wrapped position list

Used by: `/trade/TradeQueryHoldsList`

| Field | Type | Description |
|-------|------|-------------|
| `holdsList` | `[]HoldsVo` | Position rows |

#### FundJourListResponse — wrapped fund-journey list

Used by: `/trade/TradeQueryRealFundJourList`, `/trade/TradeQueryHistoryFundJourList`

| Field | Type | Description |
|-------|------|-------------|
| `data` | `[]FundJourVo` | Fund-journey rows |

#### OrderListResponse — wrapped order list (entrust or deliver)

Used by: `/trade/TradeQueryRealEntrustList`, `/trade/TradeQueryRealDeliverList`, `/trade/TradeQueryHistoryEntrustList`, `/trade/TradeQueryHistoryDeliverList`

| Field | Type | Description |
|-------|------|-------------|
| `data` | `[]OrderVo` | Order rows |

#### CondOrderPage — paged conditional order list

Used by: `/trade/TradeQueryRealCondOrderList`, `/trade/TradeQueryHistoryCondOrderList`

| Field | Type | Description |
|-------|------|-------------|
| `data` | `[]CondOrderVo` | Conditional order rows |
| `curPageNo` | `int32` | 1-based page number |
| `curPageSize` | `int32` | Number of rows in this page |
| `totalPages` | `int64` | Total number of pages |

#### QueryProductInfoResponse — futures product lookup

Used by: `/trade/FuturesQueryProductInfo`

| Field | Type | Description |
|-------|------|-------------|
| `productInfoVos` | `[]ProductInfo` | Matched product rows |

##### ProductInfo — one futures product/contract series

| Field | Type | Description |
|-------|------|-------------|
| `prodCode` | `string` | Product code (产品代码) |
| `instCode` | `string` | Contract series type (合约系列类型) |
| `lotSize` | `int32` | Number of units per lot (每手数量) |
| `decInPrice` | `int32` | Price decimal power: 0 means 1, 1 means 0.1, 2 means 0.01 (产品价格小数位) |
| `contractSize` | `string` | Contract value (合约值) |
| `priceDecimalPoint` | `string` | Price decimal point matching DecInPrice |
| `expiryDate` | `string` | Product expiry date in yyyy-MM-dd form |
| `isSupportT1` | `int32` | T+1 support: 0 no, 1 yes |

#### QueryMaxBuySellAmountResponse — futures max buy/sell quantity

Used by: `/trade/FuturesQueryMaxBuySellAmount`

| Field | Type | Description |
|-------|------|-------------|
| `positionStatus` | `int32` | Position state: 0 flat, 1 long, 2 short |
| `maxBuyAmount` | `int64` | Maximum buyable quantity (integer count, not monetary) |
| `maxSellAmount` | `int64` | Maximum sellable quantity (integer count, not monetary) |
| `initialMargin` | `string` | Opening margin (开仓按金) |
| `ccy` | `string` | Currency of the amounts |

#### QueryFundInfoResponse — futures account funds (wrapped)

Used by: `/trade/FuturesQueryFundInfo`

| Field | Type | Description |
|-------|------|-------------|
| `fundInfo` | `FundInfo` | Futures account funds snapshot |

#### QueryHoldsListResponse — futures positions + account funds

Used by: `/trade/FuturesQueryHoldsList`

| Field | Type | Description |
|-------|------|-------------|
| `fundInfo` | `FundInfo` | Account funds snapshot returned alongside positions |
| `holdsList` | `[]Hold` | Futures position rows |

#### EntrustListResponse — futures entrust list (real or history)

Used by: `/trade/FuturesQueryRealEntrustList`, `/trade/FuturesQueryHistoryEntrustList`

| Field | Type | Description |
|-------|------|-------------|
| `data` | `[]EntrustOrder` | Entrust rows |
| `curPageNo` | `int32` | Current page number (history only) |
| `curPageSize` | `int32` | Current page size (history only) |
| `totalPageNo` | `int32` | Total page count; documented as obsolete |
| `lastPage` | `int32` | 1 on the last page, else 0 (history only) |

#### DeliverListResponse — futures fill list (real or history)

Used by: `/trade/FuturesQueryRealDeliverList`, `/trade/FuturesQueryHistoryDeliverList`

| Field | Type | Description |
|-------|------|-------------|
| `data` | `[]DeliverOrder` | Fill rows (DeliverOrder aliases EntrustOrder) |
| `curPageNo` | `int32` | Current page number (history only) |
| `curPageSize` | `int32` | Current page size (history only) |
| `totalPageNo` | `int32` | Total page count; documented as obsolete |
| `lastPage` | `int32` | 1 on the last page, else 0 (history only) |

#### masterOrderListResponse — algo master order list

Used by: `/trade/AlgoQueryOrderList`

| Field | Type | Description |
|-------|------|-------------|
| `algoOrderList` | `[]MasterOrder` | Page of master orders |

#### entrustIDListResponse — algo child entrust IDs

Used by: `/trade/AlgoQueryEntrustIdList`

| Field | Type | Description |
|-------|------|-------------|
| `entrustId` | `[]string` | Child entrust identifiers of one master |

---

## 4. Market push topics — 11

Subscribe/unsubscribe with `topicId` on `/hq/Subscribe` and `/hq/Unsubscribe`.
Grouped per plan §F4; only topics 11, 14, 16, 17, 25, and 26 carry explicit
`topicId` labels in the reference documentation — the remainder are the current
Gateway's variant topics.

| Group | topicIds | Push payload |
|-------|----------|--------------|
| Quote | `11`, `35` | `BasicQotNotify` |
| Tick | `14`, `27`, `28`, `37` | `TickerNotify` |
| Broker queue | `16` | `BrokerNotify` |
| Order book | `17`, `25`, `26`, `36` | `OrderBookFullNotify` |

## 5. NotifyMsgType (push message discriminator)

Wire enum, protobuf v2.2.0 (`common/constant/NotifyMsgType.proto`).

| Name | Value |
|------|-------|
| `TrsStockDeliverMsgType` | `0` |
| `TradeStockDeliverMsgType` | `1` |
| `FuturesTradeStockDeliverMsgType` | `2` |
| `OrderBookNotifyMsgType` | `20001` |
| `BrokerQueueNotifyMsgType` | `20002` |
| `BasicQotNotifyMsgType` | `20003` |
| `TickerNotifyMsgType` | `20004` |

## 6. Push frame (`PBNotify`)

151-byte header: magic `HS`, `msgType=3` (push), `protoFmt=0`, `protoVer`,
`serialNo` int32 LE, `bodyLen` int32 LE, 128-byte `bodySHA1`, `compress=0`,
8-byte reserved. Body is protobuf `PBNotify{notifyMsgType, notifyId,
notifyTime, payload Any}`.

`bodySHA1` is a `SHA1WithRSA` signature over the raw, uncompressed body bytes.
Verification is **opt-in and off by default** (see
[ADR 0005](./adr/0005-key-model-and-push-verification.md)); the bundled platform
public keys live in `pkg/types/platformkeys.go`.

## 7. Gateway status codes

Reference: legacy HStong OpenAPI documentation (data source for the Gateway),
https://quant-open.hstong.com/api-docs/old/ .

| Code | Meaning |
|------|---------|
| `0000` | 处理成功 — success |
| `1001` | 系统未知错误 — unknown system error |
| `1002` | 签名错误 — signature error |
| `1003` | 数据加密错误 — data encryption error |
| `1004` | Socket未初始化 — socket not initialized |
| `1005` | 接口已经废弃 — endpoint deprecated |
| `1006` | 用户暂未授权 — user not yet authorized |
| `1007` | 重复提交 — duplicate submission |
| `1008` | 接口调用失败 — call failed |
| `1009` | 接口不存在 — endpoint not found |
| `1010` | 非法请求 — illegal request |
| `1011` | 服务繁忙，请稍后重试 — service busy, retry later |
| `1012` | 用户未登录 — not logged in |
| `1013` | 登陆被挤下线 — session displaced / kicked offline |
| `1014` | 登录超时 — login timeout |
| `1015` | 接口调用超时 — call timeout |
| `1016` | 接口调用参数不合法 — invalid parameter |
| `1017` | 建立长连接失败 — long-connection establishment failed |
| `1018` | 正在尝试重连，请稍后重试 — reconnecting, retry later |
| `20033` | 期货交易接口登录超时 — futures trade login timeout |
| `40001` | 查询品种信息失败 — query product info failed |
| `40002` | 查询合约信息失败 — query contract info failed |

Modelled in `pkg/types/status.go` as `StatusCode`.

## 8. Data dictionary

Enum families to be modelled in `pkg/types` (or, where marked, types only).
Reference for every table below: the legacy HStong OpenAPI documentation
(https://quant-open.hstong.com/api-docs/old/) and the protobuf v2.2.0 package.
Values are reproduced as the vendor documented them; tables are marked
**non-exhaustive** where the reference does not enumerate a closed set.

### 7.1 `entrustProp` — order property

Source: legacy data dictionary. **Non-exhaustive** (the reference also describes
a per-property business type: `0` 下单 place, `2` 撤单 cancel, `B` 订单修改
modify, `C` 成交拒绝 fill reject, `E` 成交修改 fill modify).

| Value | Meaning |
|-------|---------|
| `d` | 竞价单 auction |
| `g` | 竞价限价单 auction limit |
| `e` | 增强限价单 enhanced limit |
| `j` | 特殊限价单 special limit |
| `h` | 限价单 limit |
| `m` | 碎股挂单 odd lot |
| `o` | 手动交易 manual |
| `p` | 批量撤单 batch cancel |

### 7.2 `entrustStatus` — order status

Closed set as documented.

| Value | Meaning |
|-------|---------|
| `0` | No Register 未报 |
| `1` | Wait to Register 待报 |
| `2` | Host Registered 已报 |
| `3` | Wait for Cancel 已报待撤 |
| `4` | Wait for Cancel (Partially Matched) 部成待撤 |
| `5` | Partially Cancelled 部撤 |
| `6` | Cancelled 已撤 |
| `7` | Partially Filled 部成 |
| `8` | Filled 已成 |
| `9` | Host Reject 废单 |
| `A` | Wait for Modify (Registered) 已报待改 |
| `B` | Unregistered 无用 |
| `C` | Registering 无用 |
| `D` | Revoke Cancel 无用 |
| `W` | Wait for Confirming 待确认 |
| `X` | Pre Filled 无用 |
| `E` | Wait for Modify (Partially Matched) 部成待改 |
| `F` | Reject 预埋单检查废单 |
| `G` | Cancelled (Pre-Order) 预埋单已撤 |
| `H` | Wait for Review 待审核 |
| `J` | Review Fail 审核失败 |

### 7.3 `exchangeType` — trading market

| Value | Meaning |
|-------|---------|
| `K` | 港股 Hong Kong (uppercase) |
| `P` | 美股 US (uppercase) |
| `v` | 深股通 Shenzhen Connect (lowercase) |
| `t` | 沪股通 Shanghai Connect (lowercase) |

### 7.4 `entrustBs` — buy/sell direction

| Value | Meaning |
|-------|---------|
| `1` | 多头开仓（买入） open long / buy |
| `2` | 多头平仓（卖出） close long / sell |
| `3` | 空头平仓 close short |
| `4` | 空头开仓 open short |

### 7.5 `entrustType` — order type

The Gateway order endpoint documents **per-market** values. Codes are shared
across markets (for example `3` is limit in HK, US, and A-share); one Go
constant per distinct code is defined in `pkg/types`. **Non-exhaustive** — the
vendor may add codes, and the legacy direct-protocol dictionary uses a different
code set (`0` 买卖, `1` 查询, `2` 撤单, `3` 补单, `B` 改单).

| Market | Value | Meaning |
|--------|-------|---------|
| HK | `0` | 竞价限价 auction limit |
| HK | `1` | 竞价 auction |
| HK | `2` | 增强限价盘 enhanced limit |
| HK | `3` | 限价盘 limit |
| HK | `4` | 特别限价盘 special limit |
| HK | `6` | 暗盘 dark pool |
| HK | `7` | 碎股 odd lot |
| US | `3` | 限价盘 limit |
| US | `5` | 市价盘 market |
| US | `8` | 冰山市价 iceberg market |
| US | `9` | 冰山限价 iceberg limit |
| US | `10` | 隐藏市价 hidden market |
| US | `11` | 隐藏限价 hidden limit |
| A-share | `3` | 限价盘 limit |
| Conditional | `31` | 止盈限价单 stop-profit limit |
| Conditional | `32` | 止盈市价单 stop-profit market (US) |
| Conditional | `33` | 止损限价单 stop-loss limit |
| Conditional | `34` | 止损市价单 stop-loss market (US) |
| Conditional | `35` | 追踪止损限价单 trailing stop-loss limit |
| Conditional | `36` | 追踪止损市价单 trailing stop-loss market (US) |

### 7.6 `exchange` — IB-routed US exchange

Closed set as documented (US SMART routing destinations). **Non-exhaustive**
across venues generally; the values below are those the vendor listed.

`SMART`, `AMEX`, `ARCA`, `BATS`, `BEX`, `BYX`, `CBOE`, `CHX`, `DRCTEDGE`,
`EDGEA`, `EDGX`, `IBKRTS`, `IEX`, `ISE`, `ISLAND`, `LTSE`, `MEMX`, `NYSE`,
`NYSENAT`, `PEARL`, `PHLX`, `PSX`.

### 7.7 `realStatus` — fill status

| Value | Meaning |
|-------|---------|
| `0` | 成交 filled |
| `2` | 废单 rejected |
| `4` | 确认 confirmed |

### 7.8 `realType` — fill type

| Value | Meaning |
|-------|---------|
| `0` | 买卖 trade |
| `1` | 查询 query |
| `2` | 撤单 cancel |

### 7.9 `stockType` — security type

| Value | Meaning |
|-------|---------|
| `0` | 股票 stock |
| `1` | 基金 Fund |
| `2` | 红利 Dividend |
| `D` | 交易权证 WRNT |
| `F` | 一篮子权证 BWRT |
| `U` | 债券 BOND |

### 7.10 `dataType` — instrument type

Modelled in `pkg/types` as `DataType`. Closed set as documented; the reference
has gaps in the numbering (for example no `10008`), which are intentional.

| Value | Meaning |
|-------|---------|
| `10000` | 港股股票 HK stock |
| `10001` | 港股指数 HK index |
| `10002` | 港股ETF HK ETF |
| `10003` | 港股窝轮 HK warrant |
| `10004` | 港股牛熊证 HK CBBC |
| `10005` | 港股债券 HK bond |
| `10006` | 港股行业板块 HK sector |
| `10007` | 港股概念板块 HK concept |
| `10009` | 衍生品期货 derivatives futures |
| `10010` | 香港指数期货 HK index futures |
| `10011` | 港股股票期货 HK single-stock futures |
| `10012` | 港股恒指股息期货 HSI dividend futures |
| `10013` | 港股人民币货币期货 HKD/CNY currency futures |
| `10014` | 港股中华交易服务期货 CES futures |
| `10015` | 恒指波幅指数期货 HSI volatility futures |
| `10016` | 界内证 inline warrant |
| `20000` | 美股股票 US stock |
| `20001` | 美股指数 US index |
| `20002` | 美股ETF US ETF |
| `20003` | 美股期权 US option |
| `20006` | 美股行业板块 US sector |
| `20007` | 美股概念板块 US concept |
| `20009` | 美股OTC股票 US OTC stock |
| `30000` | A股股票 A-share stock |
| `30001` | A股指数 A-share index |
| `30002` | A股ETF A-share ETF |
| `30006` | A股行业 A-share sector |
| `30007` | A股概念 A-share concept |
| `30008` | A股科创板 STAR Market |

### 7.11 `moneyType` — currency

**Non-exhaustive.** The reference enumerates a large single-character ISO-4217
style set; the SDK models only the three trading currencies below plus a note
that the code is `0-2` for the core markets.

| Value | Meaning |
|-------|---------|
| `0` | 人民币 CNY |
| `1` | 美圆 USD |
| `2` | 港币 HKD |

The legacy reference additionally lists codes `4-9`, `A-Z`, and `a-x` for other
currencies (for example `E` CAD, `F` CHF, `M` EUR, `Q` GBP, `W` JPY, `t` SGD).

### 7.12 `countryCode` — dialing/region code

| Value | Meaning |
|-------|---------|
| `CHN` | 中国大陆 |
| `HKG` | 中国香港 |
| `MAC` | 中国澳门 |
| `TWN` | 中国台湾 |
| `SGP` | 新加坡 |
| `KOR` | 韩国 |
| `JPN` | 日本 |
| `USA` | 美国 |
| `GBR` | 英国 |
| `FRA` | 法国 |
| `RUS` | 俄罗斯 |
| `BEL` | 比利时 |
| `DEU` | 德国 |
| `CAN` | 加拿大 |
| `ESP` | 西班牙 |
| `NZL` | 新西兰 |

### 7.13 `cycType` — K-line period

| Value | Meaning |
|-------|---------|
| `2` | 日线 daily |
| `3` | 周线 weekly |
| `4` | 月线 monthly |
| `11` | 季度线 quarterly |
| `12` | 年度线 yearly |
| `5` | 1分钟 1-minute |
| `6` | 5分钟 5-minute |
| `7` | 15分钟 15-minute |
| `8` | 30分钟 30-minute |
| `9` | 60分钟 60-minute |
| `10` | 120分钟 120-minute |

### 7.14 `ExRightFlag` — price adjustment

| Value | Meaning |
|-------|---------|
| `0` | 不复权 no adjustment |
| `1` | 前复权 forward adjustment |
| `2` | 后复权 backward adjustment |

### 7.15 `Direction` — K-line query direction

| Value | Meaning |
|-------|---------|
| `0` | 往左查询 query left |
| `1` | 往右查询 query right |

## 9. Limits & constants

| Item | Value |
|------|-------|
| Gateway HTTP / TCP | `127.0.0.1:11111` / `127.0.0.1:11112` |
| Env domains | test `https://openapi-daily.hstong.com`, prod `https://openapi.hstong.com` |
| Token TTL | 3h, extended by calls |
| Concurrent subscriptions | <= 200 securities |
| Pagination | cursor `queryParamStr`, page size default 20, max < 100 |
| Ticker | `limit` <= 100 |
| Condition order | `validDays` <= 100 natural days |
| Test env hours | Mon-Fri 09:00-18:00 |
| Trade password key | `m+qS04/2CH1OweCnmXZ3TDZkCQS+hBzY` (Base64) |
| PB package version | v2.2.0 (17 protos) |
| Gateway version | v2.4.1 |
| Documented QPS quota | none published |
| Device binding | mandatory in production, disabled in test |

## 10. Deprecated fields

The reference marks these fields unreliable; consumers should prefer the
documented replacements and never rely on them for money or P&L:

`holdsBalance`, `marketValue`, `lastPrice`, `incomeBalance`, `marketValueRate`,
`incomeRatio`.
