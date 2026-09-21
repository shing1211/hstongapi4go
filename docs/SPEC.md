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

## 3. Market push topics — 11

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

## 4. NotifyMsgType (push message discriminator)

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

## 5. Push frame (`PBNotify`)

151-byte header: magic `HS`, `msgType=3` (push), `protoFmt=0`, `protoVer`,
`serialNo` int32 LE, `bodyLen` int32 LE, 128-byte `bodySHA1`, `compress=0`,
8-byte reserved. Body is protobuf `PBNotify{notifyMsgType, notifyId,
notifyTime, payload Any}`.

`bodySHA1` is a `SHA1WithRSA` signature over the raw, uncompressed body bytes.
Verification is **opt-in and off by default** (see
[ADR 0005](./adr/0005-key-model-and-push-verification.md)); the bundled platform
public keys live in `pkg/types/platformkeys.go`.

## 6. Gateway status codes

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

## 7. Data dictionary

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

## 8. Limits & constants

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

## 9. Deprecated fields

The reference marks these fields unreliable; consumers should prefer the
documented replacements and never rely on them for money or P&L:

`holdsBalance`, `marketValue`, `lastPrice`, `incomeBalance`, `marketValueRate`,
`incomeRatio`.
