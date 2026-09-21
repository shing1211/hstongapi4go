# Rate Limiting and Resilience

The SDK includes a stdlib-only resilience layer: a token-bucket rate limiter, a
retry policy with a hard mutation guard, and a three-state circuit breaker.

**It is inert by default.** With no options installed, the client issues exactly
one HTTP attempt per call, applies no rate limit, and never trips a breaker.

## The mutation guard

The retry class is derived from the endpoint path, never from a caller flag. A
closed set of order-mutation endpoints is **never retried, at any configuration**
(ADR 0003):

- Trade: `TradeEntrust`, `TradeCancelEntrust`, `TradeBatchCancelEntrust`,
  `TradeChangeEntrust`
- Futures: `FuturesEntrust`, `FuturesCancelEntrust`, `FuturesModifyEntrust`
- Algo: `AlgoAddOrder`, `AlgoCancelOrder`, `AlgoCancelEntrust`, `AlgoChangeOrder`,
  `AlgoActionOrder`

Everything else is a read-only query. The set cannot be extended or shrunk by a
caller.

## Retry policy

`client.RetryPolicy` (a public alias for `internal/resilience.Policy`)
configures retries for read-only queries:

| Field | Meaning |
|-------|---------|
| `MaxAttempts` | total attempts including the first; below 1 means 1 |
| `BaseBackoff` | delay before the second attempt; each further delay doubles |
| `MaxBackoff` | cap on the exponential delay |
| `Jitter` | when true, spreads each delay by a random amount |

`RetryPolicy` is an immutable value. Its `DoRoute(ctx, path, fn)` method derives
the class from the path, so a mutation path is issued exactly once regardless of
`MaxAttempts`. The default policy is three attempts with 100 ms → 2 s backoff and
jitter.

Only errors that classify as retryable are retried: `1011`, `1015`, `1017`,
`1018`, and transport connection/timeout failures. Trading rejections and
account/session failures are not retried.

## Rate limiter

`client.RateLimiter` (a public alias for `internal/resilience.Limiter`) is a
hierarchical token-bucket limiter:

| Item | Meaning |
|------|---------|
| `Limit{TokensPerSecond, Burst}` | refill rate and immediate-call depth; a non-positive rate disables the scope |
| `NewLimiter(global, perEndpoint)` | a global bucket plus optional per-endpoint buckets |
| `(*Limiter).Wait(ctx)` | block for the global bucket |
| `(*Limiter).WaitEndpoint(ctx, endpoint)` | block for the global **and** the endpoint bucket |

An endpoint limit tightens the global limit rather than replacing it. A zero
limit admits every call immediately.

## Circuit breaker

| Item | Meaning |
|------|---------|
| `NewBreaker(threshold, cooldown)` | closed on construction; defaults `5` failures / `30s` |
| `(*Breaker).Allow()` | admit a call; transitions open → half-open after the cooldown |
| `(*Breaker).OnSuccess()` / `OnFailure()` | record the outcome |
| `(*Breaker).State()` | `closed`, `open`, or `half-open` |
| `ErrCircuitOpen` | returned when the breaker refuses a call |

While closed it counts consecutive failures and opens at the threshold. While
open it refuses calls until the cooldown, then admits one half-open probe; a
success closes it, a failure reopens it.

## Availability

The types above are exposed publicly as `client.RetryPolicy`,
`client.RateLimiter`, and `client.CircuitBreaker`, aliases for the corresponding
`internal/resilience` types. `client.WithRateLimiter`, `client.WithCircuitBreaker`,
and `client.WithRetryPolicy` therefore accept names external code can use without
importing an internal package. `client.RetryPolicy` is a plain struct and can be
constructed directly; limiter and breaker values are produced by the SDK's
in-module constructors in this build.

The mutation guard, however, is always active: even a caller-supplied retry policy
cannot retry a mutation.

## If you retry from application code

If you prefer to keep query retries in application code rather than install a
`client.RetryPolicy`, implement them yourself:

```go
backoff := 200 * time.Millisecond
for attempt := 0; attempt < 3; attempt++ {
    rows, err := m.RealEntrustList(ctx, req)
    if err == nil {
        return rows, nil
    }
    // Retry only calls you know are reads. Never retry a mutation.
    select {
    case <-ctx.Done():
        return nil, ctx.Err()
    case <-time.After(backoff):
    }
    backoff *= 2
}
```

Throttle on the caller side too. There is **no documented QPS quota**, but:

- Concurrent market subscriptions are capped at 200 securities.
- The Gateway is a single local process; bound your concurrency so a burst does
  not starve the push reader.
- Keep the keep-alive poll at its default `30m`.

See [Error Codes](errors.md) for the full status table and [Observability](observability.md)
for the metric names the resilience layer records.

## Hardened client example

The example below wires every hardening layer together — rate limiter, separate
query/mutation circuit breakers, query-only retry policy, structured logging,
and metrics:

```go
import (
    "context"
    "log/slog"
    "time"

    "github.com/shing1211/hstongapi4go/client"
    "github.com/shing1211/hstongapi4go/internal/logging"
    "github.com/shing1211/hstongapi4go/internal/resilience"
)

// simpleRecorder implements client.Recorder without any external dependency.
type simpleRecorder struct{}

func (s *simpleRecorder) Count(ctx context.Context, name string, n int64, labels ...string) {}
func (s *simpleRecorder) Observe(ctx context.Context, name string, value float64, labels ...string) {}
func (s *simpleRecorder) Gauge(ctx context.Context, name string, value float64, labels ...string) {}

c, err := client.New(
    client.WithEnv(), // read HSTONG_*; override individual values below

    // Timeout: 3 s per request, applied to both HTTP and Gateway envelope.
    client.WithTimeout(3*time.Second),

    // Structured logging with secret redaction.
    // Never log payloads or the trade password.
    client.WithLogger(slog.Default()),

    // Metrics: a no-op recorder above is fine; swap for OTLP, Prometheus, etc.
    client.WithMetrics(&simpleRecorder{}),

    // Rate limiter: 100 calls/s globally, 10 calls/s per endpoint.
    client.WithRateLimiter(
        resilience.NewLimiter(
            resilience.Limit{TokensPerSecond: 100, Burst: 20},
            resilience.Limit{TokensPerSecond: 10, Burst: 5},
        ),
    ),

    // Query breaker: opens after 5 consecutive failures, cools down for 30 s.
    client.WithQueryBreaker(
        resilience.NewBreaker(5, 30*time.Second),
    ),

    // Mutation breaker: independent; a storm of rejected orders does not block reads.
    client.WithMutationBreaker(
        resilience.NewBreaker(5, 30*time.Second),
    ),

    // Retry policy: up to 3 attempts for reads, with jittered exponential backoff.
    // Mutations (orders) are never retried regardless of this setting.
    client.WithRetryPolicy(resilience.RetryPolicy{
        MaxAttempts: 3,
        BaseBackoff: 100 * time.Millisecond,
        MaxBackoff:  2 * time.Second,
        Jitter:      true,
    }),
)
if err != nil {
    panic(err)
}
defer c.Close()
```

### What each layer does

| Layer | What it does | Why it matters |
|-------|-------------|----------------|
| Rate limiter | Token bucket; global + per-endpoint | Prevents the Gateway's 200-subscription cap and local resource exhaustion |
| Query breaker | Opens after 5 failures, blocks reads for 30 s | Stops cascading failures from propagating to callers |
| Mutation breaker | Independent from query breaker | Order rejections cannot starve market data reads |
| Retry policy | 3 attempts with backoff for reads only | Retries transient errors (1011, 1015, 1018); never retries orders |
| Logger | Structured, secrets redacted | Observability without leaking credentials |
| Metrics | Stable dot-names, no payloads | Plug in Prometheus, OTLP, etc. |

The two-breaker model means a back-pressure event on orders (a surge of `ERR_REJECTED`) opens the mutation breaker but leaves market data reads unaffected. `WithCircuitBreaker(b)` sets both breakers to `b` for backward compatibility; use `WithQueryBreaker` + `WithMutationBreaker` to separate them.
