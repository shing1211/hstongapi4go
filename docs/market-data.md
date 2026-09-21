# Market Data

`market.Manager` issues the nine market-data pull endpoints and the two
subscription endpoints. Create it once over a shared client; it holds no mutable
state and is safe for concurrent use.

```go
m := market.New(c)
```

Market data over HTTP needs no trade login.

## Pull endpoints

| Method | Route | Purpose |
|--------|-------|---------|
| `BasicQot` | `/hq/BasicQot` | real-time quote |
| `OrderBook` | `/hq/OrderBook` | real-time order book |
| `KL` | `/hq/KL` | candlesticks |
| `TimeShare` | `/hq/TimeShare` | intraday time-and-sales |
| `Ticker` | `/hq/Ticker` | tick-by-tick trades |
| `Broker` | `/hq/Broker` | broker queue (Hong Kong only) |
| `UsOptionChainCode` | `/hq/UsOptionChainCode` | option-chain code list |
| `UsOptionChainExpireDate` | `/hq/UsOptionChainExpireDate` | option-chain expiries |
| `UsOverNightTradeCodes` | `/hq/UsOverNightTradeCodes` | US overnight-tradable codes |

### Quote

```go
q, err := m.BasicQot(ctx, market.BasicQotRequest{
    Security: []*dto.Security{
        {DataType: int32(types.DataTypeHKStock), Code: "0700.HK"},
    },
    MktTmType: 1, // 1 intraday, -1 pre-market, -2 post-market, -3 overnight, 0 HK
})
for _, quote := range q.BasicQot {
    fmt.Println(quote.GetSecurity().GetCode(), quote.GetLastPrice(), quote.GetVolume())
}
```

### Order book

```go
book, err := m.OrderBook(ctx, market.OrderBookRequest{
    Security:  &dto.Security{DataType: int32(types.DataTypeHKStock), Code: "0700.HK"},
    MktTmType: 1,
})
fmt.Println(len(book.OrderBookAskList), len(book.OrderBookBidList))
```

`DepthBookType` selects `2` (TotalView) or `3` (Arcabook); zero omits it.

### K-line

```go
kl, err := m.KL(ctx, market.KLRequest{
    Security:    &dto.Security{DataType: int32(types.DataTypeHKStock), Code: "0700.HK"},
    StartDate:   20260101, // yyyyMMdd
    Direction:   0,
    ExRightFlag: 1,        // 0 none, 1 forward, 2 backward
    CycType:     2,        // 2 daily, 3 weekly, 4 monthly, 5..10 intraday minutes
    Limit:       10,
})
for _, candle := range kl.Kline {
    fmt.Println(candle.GetDate(), candle.GetClosePrice())
}
```

The period, direction, and adjustment dictionaries are in the
[API Reference](SPEC.md) §7.

### Time-share and ticker

```go
ts, err := m.TimeShare(ctx, market.TimeShareRequest{
    Security:  sec,
    MktTmType: 1,
})

ticks, err := m.Ticker(ctx, market.TickerRequest{
    Security:  sec,
    Limit:     50, // 1..market.MaxTickerLimit (100)
    MktTmType: 1,
})
```

`Ticker` rejects a `Limit` outside `1..market.MaxTickerLimit` locally with no
request sent.

### Broker queue

```go
b, err := m.Broker(ctx, market.BrokerRequest{Security: sec})
```

Hong Kong instruments only.

### Option chain

```go
codes, err := m.UsOptionChainCode(ctx, market.UsOptionChainCodeRequest{
    SecurityCode: "AAPL",
    ExpireDate:   "2026/01/16", // yyyy/MM/dd
    FlagInOut:    1,             // 1 in the money, 2 out; 0 omits
    OptionType:   "C",           // C call, P put; empty omits
})
fmt.Println(codes.OptionCode)

expiries, err := m.UsOptionChainExpireDate(ctx,
    market.UsOptionChainExpireDateRequest{SecurityCode: "AAPL"})
fmt.Println(expiries.ExpireDate)
```

### US overnight codes

```go
overnight, err := m.UsOverNightTradeCodes(ctx, market.UsOverNightTradeCodesRequest{})
fmt.Println(overnight.SecurityCodes)
```

## Subscribe and unsubscribe

`Subscribe` registers securities on a market push topic and `Unsubscribe`
removes them. Both reject an unknown topic or an empty (or nil-containing)
security list locally.

```go
sec := &dto.Security{DataType: int32(types.DataTypeHKStock), Code: "0700.HK"}

if err := m.Subscribe(ctx, types.TopicBasicQot, sec); err != nil {
    return err
}
defer m.Unsubscribe(ctx, types.TopicBasicQot, sec)
```

Topic IDs:

| Group | `topicId` | Payload |
|-------|-----------|---------|
| Quote | `11`, `35` | `BasicQotNotify` |
| Tick | `14`, `27`, `28`, `37` | `TickerNotify` |
| Broker queue | `16` | `BrokerNotify` |
| Order book | `17`, `25`, `26`, `36` | `OrderBookFullNotify` |

The `types.TopicID` constants name each value. Up to 200 securities may be
subscribed concurrently.

Subscribing over HTTP only registers the topic. Delivery happens on the TCP push
channel; use `stream.Client` to consume it. See [Streaming](streaming.md).

## Validation

Every batch request rejects an empty security list and a nil security, and
`Ticker` rejects a `Limit` outside `1..100`. Rejections are typed
invalid-parameter errors and no request is sent.

## Numeric representation

Market DTOs are the generated `gen/hq/dto` protobuf types. Count fields such as
`volume` are `int64`; price fields are protobuf `double`. The Gateway sends plain
JSON, so `int64` counts arrive as JSON numbers (ADR 0007). The trade, futures,
and algo surfaces expose money and quantities as exact decimal **strings**.
