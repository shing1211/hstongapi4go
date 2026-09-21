# Observability

The SDK ships a small, dependency-free observability layer. Both logging and
metrics are **inert by default**: with no logger and no recorder installed, the
SDK emits nothing.

## Logging

Install a structured logger with `client.WithLogger`:

```go
c, err := client.New(
    client.WithEnv(),
    client.WithLogger(slog.Default()),
)
```

The SDK logs at debug level, using operation and category labels only:

```
http request  op=trade/TradeEntrust route=/trade/TradeEntrust
http error    op=trade/TradeEntrust category=api
```

It never logs request payloads or secrets. With a nil logger (`WithLogger(nil)`
or no option) logging is fully inert.

### Redaction helpers

`internal/logging` provides the redaction helpers the SDK uses. It is internal
and not importable outside the module, but the behavior is the contract:

| Item | Behavior |
|------|----------|
| `logging.Mask` | the fixed replacement text `***` |
| `logging.IsSensitiveKey(key)` | case-insensitive match after stripping `_`, `-`, and spaces; covers `password`, `tradePassword`, `token`, `secret`, `apiKey`, `privateKey`, `authorization`, `signature`, `cookie`, `sessionId`, `credential`, and similar |
| `logging.RedactValue(v)` | empty stays empty; every other value becomes `***`, revealing neither content nor length |
| `logging.Redact(key, value)` | a `slog.Attr` with sensitive values masked |
| `logging.RedactAttrs` / `logging.Redacting` | mask a whole attribute list or wrap an `slog.Handler` |

If you forward externally supplied attributes to a logger, wrap it with
`logging.Redacting` so sensitive keys are masked.

## Metrics

`client.WithMetrics` installs a recorder:

```go
c, err := client.New(
    client.WithEnv(),
    client.WithMetrics(rec),
)
```

Measurements use stable dot-separated names and non-sensitive labels only
(operation, category, outcome, state, address, error kind):

| Metric | Kind | Meaning |
|--------|------|---------|
| `hstong.http.requests` | counter | HTTP calls that reached the transport |
| `hstong.http.errors` | counter | failed HTTP calls |
| `hstong.http.latency_ms` | histogram | end-to-end call duration |
| `hstong.order.outcomes` | counter | order-mutation outcomes (ok/error) |
| `hstong.ratelimit.waits` | counter | calls that waited for a token |
| `hstong.ratelimit.wait_ms` | histogram | rate-limit wait duration |
| `hstong.breaker.state` | gauge | `0` closed, `1` half-open, `2` open |
| `hstong.push.connects` | counter | successful push connections |
| `hstong.push.reconnects` | counter | push reconnections |
| `hstong.push.errors` | counter | push transport and decode errors |

Labels: `op`, `category`, `outcome`, `state`, `addr`, `kind`.

`internal/metrics` defines a `Recorder` interface (`Count`, `Observe`, `Gauge`)
and a `Hook` single-entry adapter an OpenTelemetry bridge can implement; `Nop` is
the default.

> **Availability.** The interface is exposed publicly as `client.Recorder`, an
> alias for the internal `metrics.Recorder`, so external code can implement and
> name it and pass it to `client.WithMetrics` without importing an internal
> package. A custom `RoundTripper` (below) remains an option when you want to
> instrument the HTTP layer instead.

## Push errors

Push transport and decode failures surface on each subscription's `Errors()`
channel; a reconnect reports an error wrapping `stream.ErrReconnected`. See
[Streaming](streaming.md).

## Instrumenting HTTP with a RoundTripper

`client.WithHTTPClient` accepts an `*http.Client`, so you can wrap the transport
for metrics, tracing, or logging without the internal packages:

```go
type instrumentedTransport struct {
    base   http.RoundTripper
    logger *slog.Logger
}

func (t *instrumentedTransport) RoundTrip(r *http.Request) (*http.Response, error) {
    start := time.Now()
    resp, err := t.base.RoundTrip(r)
    t.logger.Info("hstong call",
        "path", r.URL.Path,
        "method", r.Method,
        "duration", time.Since(start),
        "err", err,
    )
    return resp, err
}

hc := &http.Client{Transport: &instrumentedTransport{
    base:   http.DefaultTransport,
    logger: slog.Default(),
}}

c, err := client.New(client.WithHTTPClient(hc))
```

The request path identifies the endpoint, so latency can be attributed per
endpoint. Do not log request bodies if you add them.

## What to watch

| Signal | Source | Notes |
|--------|--------|-------|
| Request latency | `WithMetrics` or `RoundTripper` | attribute by operation/path |
| Error rate by category | `WithMetrics`/logs | separate transport failures from `ok:false` |
| Push state | `Subscription.Errors()` | `ErrReconnected` marks a reconnect |
| Buffer drops | not exposed | drop-oldest is internal; raise `WithBuffer` if observed |
| Breaker state | `WithMetrics` | `0` closed, `1` half-open, `2` open |
