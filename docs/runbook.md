# Runbook

Operational guidance for running applications built on `hstongapi4go`.
For development questions, see [CONTRIBUTING.md](./CONTRIBUTING.md).

## Common failure modes

### Connection refused to Gateway

**Symptom:** `Error: dial tcp 127.0.0.1:11111: connect: connection refused`

**Cause:** The Gateway is not running.

**Resolution:**
1. Start the HStong Gateway application.
2. Verify it is listening on the expected ports:
   ```sh
   ss -tlnp | grep 11111
   ss -tlnp | grep 11112
   ```

### Trade calls return "not logged in" (1012)

**Symptom:** `Error: gateway: 1012 not logged in`

**Cause:** No valid trade session. The trade password was not set, or the
session expired.

**Resolution:**
1. Set `HSTONG_TRADE_PASSWORD` environment variable.
2. Ensure `session.Login(ctx)` completes successfully before trade calls.
3. If the token expired, call `session.Login(ctx)` again.

### Push events stop arriving

**Symptom:** `Updates()` channel blocks or returns no events.

**Cause:** TCP connection dropped, push client reconnecting, or subscription lost.

**Resolution:**
1. Check `Errors()` channel for `ErrReconnected`.
2. The SDK reconnects automatically with backoff.
3. If events do not resume, verify the Gateway is still running.
4. Check that `Subscribe` was called successfully for the desired topic.

### Order mutation returns timeout (1015)

**Symptom:** `Error: gateway: 1015 call timeout`

**Cause:** The Gateway did not respond within the timeout. The order may or may
not have been placed.

**Resolution:**
1. **Do not retry immediately.** Query `RealEntrustList` or `RealDeliverList`
   to determine the actual order state.
2. If the order exists and is in a terminal state (filled, cancelled, rejected),
   the original request was processed.
3. If the order is not found, it was not placed and may be retried safely.

### Circuit breaker open

**Symptom:** `Error: circuit open`

**Cause:** The circuit breaker opened after consecutive failures.

**Resolution:**
1. Wait for the cooldown period (default 30 seconds).
2. The breaker will transition to half-open and admit a probe request.
3. If the probe succeeds, the breaker closes.
4. If failures continue, the breaker reopens.

### Rate limiter blocked

**Symptom:** `Error: rate limit exceeded`

**Cause:** Too many requests per second.

**Resolution:**
1. Implement exponential backoff in the caller.
2. Reduce request frequency.
3. Consider a higher `TokensPerSecond` in the rate limiter configuration.

## Debugging

### Enable structured logging

```go
c, err := client.New(
    client.WithEnv(),
    client.WithLogger(slog.Default()),
)
```

The SDK logs operation names, routes, and error categories. It never logs
request payloads or the trade password.

### Verify Gateway connectivity

```go
c, err := client.New(client.WithEnv())
if err != nil {
    log.Fatal(err)
}

// Test HTTP with a simple market query
marketMgr := market.New(c)
_, err = marketMgr.BasicQot(ctx, market.BasicQotRequest{
    Security: []*dto.Security{
        {DataType: int32(types.DataTypeHKStock), Code: "0700.HK"},
    },
    MktTmType: 1,
})
if err != nil {
    log.Fatalf("Gateway not reachable: %v", err)
}
```

### Test push connectivity

```go
s := stream.New(c)
if err := s.Connect(ctx); err != nil {
    log.Fatalf("Push not reachable: %v", err)
}
defer s.Close()

// Subscribe to a topic
sub, err := s.Subscribe(ctx, types.TopicBasicQot, sec)
if err != nil {
    log.Fatalf("Subscribe failed: %v", err)
}

// Wait briefly for events
select {
case ev := <-sub.Updates():
    log.Printf("Push working: %v", ev)
case <-time.After(5 * time.Second):
    log.Fatal("No push events received in 5s")
}
```

### Inspect error codes

```go
var gwErr *errs.Error
if errors.As(err, &gwErr) {
    log.Printf("Code: %s, Category: %s, Retryable: %v",
        gwErr.Code, gwErr.Category, gwErr.Retryable)
}
```

### Run against the mock Gateway

For offline development and testing:

```sh
go run ./cmd/hstong-mock-gateway
```

Then run your application with the default Gateway URL (or set
`HSTONG_GATEWAY_URL=http://127.0.0.1:11111`).

## Log interpretation

### HTTP request log

```
http request  op=trade/TradeEntrust route=/trade/TradeEntrust
```

A request was sent. `op` is the operation label; `route` is the HTTP path.

### HTTP error log

```
http error    op=trade/TradeEntrust category=api
```

A request returned `ok:false`. `category` is the error category
(`api`, `account`, `rate_limit`, etc.).

### Push reconnect log

```
push reconnect  addr=127.0.0.1:11112 attempt=3
```

The push connection dropped and the client is reconnecting. After successful
reconnect, the client re-subscribes all active topics.

### Push verify fail log

```
push verify fail  err="crypto/rsa: verification error"
```

A push frame failed signature verification. This only occurs when
`HSTONG_VERIFY_PUSH=true`. The frame is dropped.

## Performance considerations

### Buffer sizing

The push `Updates()` and `Errors()` channels have a default buffer of 64.
If a consumer is bursty (for example, processing batches), increase the
buffer:

```go
sub, err := s.Subscribe(ctx, topic, sec, stream.WithBuffer(256))
```

### Subscription limits

The Gateway enforces a maximum of 200 concurrent security subscriptions per
connection. The SDK does not enforce this limit; exceeding it returns a
Gateway error.

### Push reader starvation

A slow consumer blocks the shared push read loop. If the buffer fills with
drop-oldest policy, events are dropped. Monitor `Errors()` for backpressure
signals and increase the buffer or add consumer goroutines.

## Getting help

1. Check this runbook for common issues.
2. Run `make test` to verify the SDK against the mock Gateway.
3. Enable `WithLogger(slog.Default())` for detailed operation logs.
4. File an issue at https://github.com/shing1211/hstongapi4go/issues with:
   - SDK version (`go list -m github.com/shing1211/hstongapi4go`)
   - Gateway version (from your Gateway application's about screen)
   - Go version (`go version`)
   - Minimal reproducible example
   - Full error output with stack trace
