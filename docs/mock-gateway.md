# Mock Gateway

The repository ships a standalone mock of the HStong OpenAPI Gateway so the SDK
and the examples run without the real Gateway. It serves the HTTP surface on
`127.0.0.1:11111` and the TCP push surface on `127.0.0.1:11112` by default,
matching the real Gateway, and answers from the fixtures in `test/mockgateway/`.

## Run the standalone mock

```sh
go run ./cmd/hstong-mock-gateway
```

```
hstong-mock-gateway: HTTP  http://127.0.0.1:11111
hstong-mock-gateway: push  127.0.0.1:11112
hstong-mock-gateway: serving 51 endpoints; press Ctrl-C to stop
```

Options:

| Flag | Default | Purpose |
|------|---------|---------|
| `-http` | `127.0.0.1:11111` | HTTP listen address; `127.0.0.1:0` binds an ephemeral port |
| `-push` | `127.0.0.1:11112` | TCP push listen address |

## Point the SDK at the mock

With the default ports, no environment is needed:

```sh
go run ./examples/quickstart
go run ./examples/market-data
go run ./examples/streaming
```

For non-default ports, set the environment read by `client.WithEnv()`:

```sh
# POSIX shell
HSTONG_GATEWAY_URL=http://127.0.0.1:21111 \
HSTONG_PUSH_ADDR=127.0.0.1:21112 \
go run ./examples/quickstart
```

The mock accepts any trade password, so any non-empty
`HSTONG_TRADE_PASSWORD` works for the trading, futures, and algo examples.

## Use the mock in a Go test

`test/mockgateway` starts on ephemeral ports and registers cleanup:

```go
func TestQuote(t *testing.T) {
    srv := mockgateway.New() // ephemeral HTTP port, push on 127.0.0.1:11112
    srv.StartT(t)            // starts and registers t.Cleanup(Close)

    c, err := client.New(
        client.WithBaseURL(srv.HTTPBaseURL()),
        client.WithPushAddr(srv.PushAddr()),
    )
    if err != nil {
        t.Fatal(err)
    }
    defer c.Close()
    // ... drive the SDK and assert against the fixtures.
}
```

Use `WithHTTPAddr("127.0.0.1:0")` and `WithPushAddr("127.0.0.1:0")` to bind
both surfaces on ephemeral ports.

## API

| Type / method | Purpose |
|---------------|---------|
| `mockgateway.New(opts...)` | build a server; it does not listen until `Start` |
| `WithHTTPAddr(addr)` | HTTP listen address; empty binds an ephemeral port |
| `WithPushAddr(addr)` | push listen address; empty restores the default |
| `WithFixtureSet(f)` | replace the response fixture table |
| `WithErrorInjection(m)` | seed per-path error injections |
| `WithPushTypeURL(url)` | override the `Any` `type_url` on emitted frames |
| `Start()` / `StartT(t)` | start serving; `StartT` registers test cleanup |
| `Close()` | stop both listeners and wait for goroutines; idempotent |
| `HTTPBaseURL()` | the bound HTTP base URL, or empty before `Start` |
| `PushAddr()` | the bound push address, or empty before `Start` |
| `ClientCount()` | number of connected push clients |
| `SubscribedTopics()` | market topics registered via `/hq/Subscribe` |
| `TradeSubscribed()` | whether `/trade/TradeSubscribe` is active |
| `Inject(path, inj)` / `ClearInjections()` | fail a route with a chosen status code |
| `Emit(topicID, msgType, payload)` | write one push frame to every client |

## Error injection

`ErrorInjection` makes a route answer `ok:false` with `"<Code> <Message>"`, so the
SDK's error mapping, retryability, and re-login paths can be exercised:

```go
srv.Inject("/trade/TradeEntrust", mockgateway.ErrorInjection{
    Code:    "1011",
    Message: "service busy",
    Times:   1, // zero means every request
})
```

An injection with an empty `Code` is removed.

## Push

`Emit` writes one frame carrying a generated `proto.Message` to every connected
client:

```go
srv.Emit(types.TopicBasicQot, types.BasicQotNotifyMsgType, &hqnotify.BasicQotNotify{...})
```

`WithPushTypeURL` overrides the `Any` `type_url`; the SDK decodes by the
`notifyMsgType` enum, so this is a way to prove that behavior.

## Limitations

The mock does not simulate market hours, order matching, session rules, or real
account state. Its fixtures are static or cursor-paginated JSON. The push server
reads client frames only to detect a peer close; it never acts on them.
