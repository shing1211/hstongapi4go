# Authentication

Market data over HTTP does not require a login. **Trading, futures, and algo
calls do.** The SDK models a trade session with `hstong.SessionManager`.

## Trade password

The Gateway requires the trade password on `TradeLogin`. The SDK encrypts it
before it leaves the process:

```
password_on_wire = Base64(AES-ECB/PKCS7(plaintext, key = Base64Decode("m+qS04/2CH1OweCnmXZ3TDZkCQS+hBzY")))
```

The fixed protocol key is public reference data published by HStong, not a
secret. The documented known-answer vector is
`123456 -> W1U8iZIppSE+mBMtzy9vZQ==`. The plaintext password is held only in
memory and is never logged or embedded in an error.

Set the password once, preferably through the environment:

```go
c, err := client.New(client.WithEnv()) // reads HSTONG_TRADE_PASSWORD
```

or explicitly:

```go
c, err := client.New(client.WithTradePassword("..."))
```

If no password is configured, `Login` returns a typed error **without sending a
request**.

## Log in

```go
session := hstong.NewSessionManager(c)
if err := session.Login(ctx); err != nil {
    return err
}
defer session.Logout(ctx)
```

`Login` posts `/trade/TradeLogin`. A Gateway rejection or a `{"success":false}`
body leaves the manager logged out.

## Gate authenticated calls

`EnsureLoggedIn` returns immediately when the session is live; otherwise it
performs a **single-flight** login, so concurrent callers share one
`/trade/TradeLogin` request and observe the same outcome:

```go
if err := session.EnsureLoggedIn(ctx); err != nil {
    return err
}
```

The `trade.Manager` calls `EnsureLoggedIn` itself before every request when it
was created with `trade.WithSession(session)`:

```go
m := trade.New(c, trade.WithSession(session))
```

`trade.Session` is the narrow interface the manager depends on:

```go
type Session interface {
    EnsureLoggedIn(ctx context.Context) error
    ReLogin(ctx context.Context, cause error) (handled bool, err error)
}
```

## Re-login after a rejected session

When the Gateway reports that the session is no longer valid — `1012` not logged
in, `1013` session displaced, `1014` login timeout, or `20033` futures login
timeout — the trade manager best-effort re-logs in, then returns the original
endpoint error to the caller. The endpoint request itself is never repeated.

To drive re-login yourself:

```go
handled, err := session.ReLogin(ctx, callErr)
```

`ReLogin` returns `(false, nil)` and sends nothing when the cause is not a
re-login trigger.

## Keep the three-hour token alive

The trading token lives for three hours and is extended by activity.
`StartKeepAlive` polls a cheap read endpoint on an interval to extend it:

```go
session.StartKeepAlive(ctx)
defer session.StopKeepAlive()
```

| Setting | Default | Option |
|---------|---------|--------|
| Poll interval | `30m` (`hstong.DefaultKeepAliveInterval`) | `hstong.WithKeepAliveInterval(d)` |
| Keep-alive route | `client.RouteTradeQueryMarginFundInfo` | `hstong.WithKeepAliveRoute(route)` |

The loop stops when the context passed to `StartKeepAlive` is cancelled or when
`StopKeepAlive` is called. Calling `StartKeepAlive` while a loop is already
running is a no-op. Every poll outcome, including errors, is intentionally
ignored: the poll is best-effort.

## Log out

```go
if err := session.Logout(ctx); err != nil {
    // Gateway error: the session state is left unchanged.
}
```

`Logout` is idempotent: when the manager is not logged in it returns `nil`
without sending a request.

## Order-status push

Attach a trade manager to the stream client to have `SubscribeTrade` issue
`/trade/TradeSubscribe` and `Cancel` issue `/trade/TradeUnsubscribe`:

```go
s := stream.New(c, stream.WithTradeManager(tradeManager))
```

Without an attached manager the subscription is local only, which suits a caller
that manages the Gateway subscription itself. See [Streaming](streaming.md).

## What the SDK does *not* do

- No developer RSA key, no request signing, no AES request encryption beyond the
  fixed trade-password transformation above. The Gateway owns all platform
  credentials and signing (ADR 0001, ADR 0005).
- No device binding. That belongs to the deprecated legacy protocol; see
  [Legacy Protocol](LEGACY.md).
- No account-opening or credential-storage tooling.
