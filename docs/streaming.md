# Streaming

`stream.Client` ties the Gateway's HTTP subscription endpoints to its TCP push
channel. `Subscribe` performs `/hq/Subscribe` and returns a `Subscription` whose
`Updates` and `Errors` channels deliver decoded push events. On a reconnect the
client re-issues `/hq/Subscribe` for every active subscription and pushes an
`ErrReconnected` notice.

```go
s := stream.New(c)
if err := s.Connect(ctx); err != nil {
    return err
}
defer s.Close()
```

`Connect` dials the TCP push channel and starts the read/reconnect loop. The
connection lives until `Close` or until the context passed to `Connect` is
cancelled. `New` takes the push address from the client; override it with
`stream.WithPushAddr`.

## Subscribe to a market topic

```go
sec := &dto.Security{DataType: int32(types.DataTypeHKStock), Code: "0700.HK"}

sub, err := s.Subscribe(ctx, types.TopicBasicQot, sec)
if err != nil {
    return err
}
defer sub.Cancel(ctx)

for {
    select {
    case <-ctx.Done():
        return nil
    case ev, ok := <-sub.Updates():
        if !ok {
            return nil
        }
        if q, ok := ev.BasicQot(); ok {
            bq := q.GetBasicQot()
            fmt.Println(ev.ID, bq.GetLastPrice(), bq.GetVolume())
        }
    case err := <-sub.Errors():
        if err != nil {
            log.Println("stream:", err)
        }
    }
}
```

`Subscribe` rejects an unknown topic or an empty (or nil-containing) security
list with an invalid-parameter error before any request. The returned
`Subscription` is owned by the caller and must be released with `Cancel`.
`Cancel` performs the HTTP unsubscribe and closes both channels.

Topics and their payloads are listed in [Market Data](market-data.md).

## The Event type

| Field | Type | Meaning |
|-------|------|---------|
| `Type` | `types.NotifyMsgType` | push discriminator |
| `ID` | `string` | notification id; for market pushes, the security code |
| `Time` | `time.Time` (UTC) | notification time; zero when unset |
| `Payload` | `any` | decoded typed notification body |

Decode the payload with the typed accessors rather than a type assertion:

| Accessor | Payload type |
|----------|--------------|
| `ev.BasicQot()` | `*hqnotify.BasicQotNotify` |
| `ev.Ticker()` | `*hqnotify.TickerNotify` |
| `ev.OrderBook()` | `*hqnotify.OrderBookFullNotify` |
| `ev.Broker()` | `*hqnotify.BrokerNotify` |
| `ev.TradeDeliver()` | `*tradenotify.TradeStockDeliverNotify` (types 0 and 1) |
| `ev.FuturesTradeDeliver()` | `*tradenotify.TradeStockDeliverNotify` (type 2) |

## Trade and futures order push

Order push is **session-wide**, not per-security. Attach a trade manager so
`SubscribeTrade` issues `/trade/TradeSubscribe` and `Cancel` issues
`/trade/TradeUnsubscribe`:

```go
s := stream.New(c, stream.WithTradeManager(tradeManager))

sub, err := s.SubscribeTrade(ctx)
if err != nil {
    return err
}
defer sub.Cancel(ctx)

for ev := range sub.Updates() {
    if deliver, ok := ev.TradeDeliver(); ok {
        fmt.Println(deliver.GetStockCode(), deliver.GetEntrustStatus(), deliver.GetMatchNo())
    }
    if deliver, ok := ev.FuturesTradeDeliver(); ok {
        n := future.FromDeliverNotify(deliver)
        if n.HasFill() {
            fmt.Println(n.StockCode, n.BusinessPrice)
        }
    }
}
```

Without `WithTradeManager` the subscription is local only and no HTTP call is
sent, which suits a caller that manages the Gateway subscription itself.

The `TradePusher` interface keeps the stream package independent of the trade
package; pass a `*trade.Manager`, which satisfies it.

## Subscription

| Method | Behavior |
|--------|----------|
| `Updates()` | channel of decoded `Event` values |
| `Errors()` | channel of subscription-level errors, including `ErrReconnected` |
| `TopicID()` | the market topic; `0` for a trade subscription |
| `Securities()` | a copy of the subscribed instruments |
| `Cancel(ctx)` | deregisters locally and performs the HTTP unsubscribe; idempotent |

Both channels are closed by `Cancel`, by `Client.Close`, or when the stream client
shuts down.

## Options

| Option | Default | Purpose |
|--------|---------|---------|
| `WithPushAddr(a)` | the client's push address | override the TCP push address |
| `WithReconnect(min, max)` | push defaults (500 ms / 30 s) | reconnect backoff bounds |
| `WithBuffer(n)` | `64` | depth of each `Updates`/`Errors` channel |
| `WithTradeManager(t)` | none | enable HTTP order-push subscribe |
| `WithDialer(d)` | a `net.Dialer` | test hook to redirect reconnects |

## Reconnection

After the push connection drops, the client reconnects with exponential backoff
and jitter, then re-issues the HTTP subscribe for every active market
subscription and `/trade/TradeSubscribe` when trade push is active. Each
`Errors()` channel receives an error that wraps `stream.ErrReconnected`, and a
re-subscribe failure is reported on the same channel. Subscription state is
restored, never silently dropped.

## Backpressure

`Updates` and `Errors` are buffered channels that use a **drop-oldest** policy:
when a buffer is full the oldest queued item is discarded to make room for the
newest, so a slow consumer never blocks the shared push read loop. Increase the
buffer with `WithBuffer` when a consumer can be bursty.

## Lifecycle

- `Close` is idempotent, cancels the connection, stops the read loop and every
  dispatcher, and closes every subscription channel. It does not close the
  caller-owned `*client.Client`.
- After `Close`, `Connect` and `Subscribe` return `stream.ErrClosed`.
- A client built without a `*client.Client` returns `stream.ErrNoClient` from
  `Subscribe` and `SubscribeTrade`.
