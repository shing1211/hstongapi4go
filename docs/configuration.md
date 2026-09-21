# Configuration

Configuration is resolved inside `client.New` from the functional options passed
to it, applied **in order on top of documented defaults**. The last option that
sets a field wins. Because `WithEnv` is itself an option, an explicit option
placed after `WithEnv` overrides the environment value, and one placed before it
does not.

```go
c, err := client.New(
    client.WithEnv(),                          // read HSTONG_* first
    client.WithTimeout(3*time.Second),         // then override the timeout
)
```

## Options

| Option | Default | Notes |
|--------|---------|-------|
| `WithBaseURL(u)` | `http://127.0.0.1:11111` | Absolute `http`/`https` URL with a host. Empty restores the default. A trailing slash is trimmed. |
| `WithPushAddr(a)` | `127.0.0.1:11112` | `host:port` for the TCP push channel. Empty restores the default. |
| `WithHTTPClient(hc)` | a client over a cloned transport | A client with no timeout is copied and given the resolved timeout. `Close` releases idle connections but does not close a supplied client. |
| `WithTimeout(d)` | `10s` | Used both as the HTTP client timeout and as the envelope's `timeout_sec`. Non-positive restores the default. |
| `WithTradePassword(p)` | unset | Plaintext trade password, encrypted before `TradeLogin`. Prefer `WithEnv` or secret storage over hardcoding. |
| `WithPlatformPublicKey(k)` | `types.PlatformPublicKeyTest` | Base64 SPKI key for opt-in push verification. Empty restores the test key. Public reference data, not a secret. |
| `WithLogger(l)` | nil (inert) | `*slog.Logger` for structured SDK diagnostics. Operation and category labels only; never payloads or secrets. |
| `WithMetrics(rec)` | nil (inert) | Metrics recorder. Takes the public `client.Recorder` alias (see [Observability](observability.md)). |
| `WithRetryPolicy(p)` | nil (one attempt) | Query-only retry policy. Takes the public `client.RetryPolicy` alias. Mutations are never retried (see [Rate Limiting](rate-limiting.md)). |
| `WithRateLimiter(l)` | nil (no limit) | Token-bucket rate limiter. Takes the public `client.RateLimiter` alias. |
| `WithCircuitBreaker(b)` | nil (no breaker) | Sets both `QueryBreaker` and `MutationBreaker` to `b` for backward compatibility. Takes the public `client.CircuitBreaker` alias. |
| `WithQueryBreaker(b)` | nil (no breaker) | Circuit breaker for read-only query endpoints. Takes the public `client.CircuitBreaker` alias. |
| `WithMutationBreaker(b)` | nil (off) | Circuit breaker for order/futures/algo mutation endpoints. Mutations can open this breaker independently of `QueryBreaker`. Takes the public `client.CircuitBreaker` alias. |
| `WithEnv()` | — | Reads the `HSTONG_*` variables below. |

`New` returns an error, never panics, when the resolved configuration is invalid
(an unparsable environment value, a non-`http(s)` or host-less base URL, or a
malformed push address). The returned client is nil on error.

## Environment variables

Unset and empty variables are treated identically: the setting keeps its default
or its value from an earlier option.

| Variable | Option equivalent | Format | Default |
|----------|-------------------|--------|---------|
| `HSTONG_GATEWAY_URL` | `WithBaseURL` | absolute `http`/`https` URL | `http://127.0.0.1:11111` |
| `HSTONG_PUSH_ADDR` | `WithPushAddr` | `host:port` | `127.0.0.1:11112` |
| `HSTONG_TIMEOUT` | `WithTimeout` | Go duration (`10s`, `1500ms`) | `10s` |
| `HSTONG_TRADE_PASSWORD` | `WithTradePassword` | plaintext | unset |
| `HSTONG_VERIFY_PUSH` | enables verification | `strconv.ParseBool` (`1`, `t`, `true`, `0`, `f`, `false`) | `false` |

`WithEnv` can fail: an unparsable duration or boolean, a malformed URL, or a
malformed push address records an error that `New` returns instead of a client.

## Inspecting the resolved configuration

A `*client.Client` is immutable after `New`. Its accessors report the resolved
values:

```go
c.BaseURL()            // Gateway HTTP root
c.PushAddr()           // Gateway TCP push address
c.Timeout()            // per-request timeout
c.TradePassword()      // sensitive; never log or print it
c.PlatformPublicKey()  // base64 SPKI
c.VerifyPush()         // opt-in push signature verification
c.HTTPClient()         // never nil on a client built by New
c.JSON()               // encoding/json codec
```

## Defaults for the local Gateway

| Item | Value |
|------|-------|
| Gateway HTTP | `http://127.0.0.1:11111` |
| Gateway TCP push | `127.0.0.1:11112` |
| Request timeout | `10s` |
| Concurrent market subscriptions | up to 200 securities |
| Test-environment hours | Mon–Fri 09:00–18:00 |

See the [API Reference](SPEC.md) for the full limits table.

## Logging the config safely

`TradePassword` is sensitive. The SDK never logs it or embeds it in an error, and
`client.Client` has no method that renders it except the explicit
`TradePassword()` accessor. Do not print it, include it in telemetry, or write it
to disk.
