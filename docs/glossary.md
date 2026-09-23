# Glossary

Terms used throughout this SDK and documentation. Canonical definitions live in
[SPEC.md](./SPEC.md); this page collects them in one place for reference.

## Trading terms

### Entrust

Place an order. The act of submitting an order to the Gateway.

### Deliver / Fill

An execution of an order: shares being bought or sold at a specific price. Also
called a fill or match.

### Hold / Position

A held security quantity. `Positions()` returns all open positions for an
account.

### EntrustBS — buy/sell direction

| Value | Meaning |
|-------|---------|
| `1` | buy / open long |
| `2` | sell / close long |
| `3` | close short |
| `4` | open short |

See [SPEC.md §7.4](./SPEC.md#74-entrustbs-buysell-direction).

### EntrustType — order type

Per-market order types. Key values:

| Market | Value | Meaning |
|--------|-------|---------|
| HK | `0` | 竞价限价 auction limit |
| HK | `1` | 竞价 auction |
| HK | `2` | 增强限价盘 enhanced limit |
| HK | `3` | 限价盘 limit |
| HK | `4` | 特别限价盘 special limit |
| US | `3` | 限价盘 limit |
| US | `5` | 市价盘 market |
| US | `8` | 冰山市价 iceberg market |
| US | `9` | 冰山限价 iceberg limit |
| Conditional | `31` | 止盈限价单 stop-profit limit |
| Conditional | `32` | 止盈市价单 stop-profit market |
| Conditional | `33` | 止损限价单 stop-loss limit |
| Conditional | `34` | 止损市价单 stop-loss market |

See [SPEC.md §7.5](./SPEC.md#75-entrusttype-order-type).

### EntrustStatus — order status

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

See [SPEC.md §7.2](./SPEC.md#72-entruststatus-order-status).

## Market data terms

### MktTmType — market session

| Value | Meaning |
|-------|---------|
| `-3` | overnight trading (US) |
| `-2` | post-market (US) |
| `-1` | pre-market (US) |
| `0` | Hong Kong standard |
| `1` | intraday |

### ExRightFlag — price adjustment

| Value | Meaning |
|-------|---------|
| `0` |不复权 no adjustment |
| `1` |前复权 forward adjustment |
| `2` |后复权 backward adjustment |

### CycType — K-line period

| Value | Meaning |
|-------|---------|
| `2` | 日线 daily |
| `3` | 周线 weekly |
| `4` | 月线 monthly |
| `5` | 1分钟 1-minute |
| `6` | 5分钟 5-minute |
| `7` | 15分钟 15-minute |
| `8` | 30分钟 30-minute |
| `9` | 60分钟 60-minute |
| `10` | 120分钟 120-minute |
| `11` | 季度线 quarterly |
| `12` | 年度线 yearly |

### ExchangeType — trading market

| Constant | Value | Market |
|----------|-------|--------|
| `types.ExchangeHK` | `K` | Hong Kong |
| `types.ExchangeUS` | `P` | US |
| `types.ExchangeShenzhenConnect` | `v` | Shenzhen Connect |
| `types.ExchangeShanghaiConnect` | `t` | Shanghai Connect |

### DataType — instrument type

| Value | Meaning |
|-------|---------|
| `10000` | 港股股票 HK stock |
| `10002` | 港股ETF HK ETF |
| `10003` | 港股窝轮 HK warrant |
| `10004` | 港股牛熊证 HK CBBC |
| `20000` | 美股股票 US stock |
| `20002` | 美股ETF US ETF |
| `20003` | 美股期权 US option |
| `30000` | A股股票 A-share stock |
| `30008` | A股科创板 STAR Market |

See [SPEC.md §7.10](./SPEC.md#710-datatype-instrument-type) for the full list.

## Session and transport terms

### Mutation

An order-changing operation: place, cancel, or modify. Mutations are never
auto-retried by the SDK (ADR 0003). See
[Rate Limiting](./rate-limiting.md).

### Query

A read-only request. Queries may be retried within the configured retry policy.
See [Rate Limiting](./rate-limiting.md).

### Re-login conditions

Status codes that indicate the session is no longer valid and trigger a
single-flight re-login:

| Code | Meaning |
|------|---------|
| `1012` | not logged in |
| `1013` | session displaced |
| `1014` | login timeout |
| `20033` | futures trade login timeout |

### Push topic groups

| Group | topicIds | Payload |
|-------|----------|---------|
| Quote | `11`, `35` | `BasicQotNotify` |
| Tick | `14`, `27`, `28`, `37` | `TickerNotify` |
| Broker queue | `16` | `BrokerNotify` |
| Order book | `17`, `25`, `26`, `36` | `OrderBookFullNotify` |

See [SPEC.md §4](./SPEC.md#4-market-push-topics-11) and
[Streaming](./streaming.md).

## SDK concepts

### Hybrid codec

The SDK uses different encodings for different surfaces:

| Surface | Encoding |
|---------|----------|
| TCP push payloads | binary protobuf |
| Market DTOs in HTTP `data` | `encoding/json` |
| Trade / futures / algo / assets / session HTTP bodies | `encoding/json` |

See [Protocol](./protocol.md) and ADR 0002, ADR 0007.

### Functional options

Configuration pattern using `Option func(*Config)` with `WithX` constructors.
Options are applied in order over defaults. The last option that sets a field
wins. See [Configuration](./configuration.md).

### Pagination

Cursor-based pagination using `queryParamStr`. Default page size is 20; maximum
is 99 (not 100). See [Trading](./trading.md#pagination).

### Keep-alive

The Gateway token lives for 3 hours and is extended by activity.
`SessionManager.StartKeepAlive` polls a cheap endpoint on an interval (default
30 minutes) to extend the token. See [Authentication](./authentication.md).

### Trade password encryption

On `TradeLogin`, the password is transformed to the wire format:

```
Base64(AES-ECB/PKCS7(plaintext, key = Base64Decode("m+qS04/2CH1OweCnmXZ3TDZkCQS+hBzY")))
```

The protocol key is fixed and public. See [Authentication](./authentication.md)
and [Security](./security.md).
