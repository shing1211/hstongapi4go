# 0009 — OpenTelemetry observability

- Status: Accepted
- Date: 2026-09-23
- Supersedes: [0004 — Minimal dependency set](./0004-minimal-dependencies.md) (observability branch)

## Context

[ADR 0004](./0004-minimal-dependencies.md) deferred observability to a future phase,
stating that a metrics library would be introduced when the SDK's observability needs
outgrew `*slog.Logger`. The v-next plan (Phase 4 / P03) explicitly includes
"observability (`log/slog` redaction + OpenTelemetry)".

Enterprise SDK users need:

- **Distributed traces** across HTTP calls and push event dispatch, correlated with
  a single trace ID propagated via `context.Context`.
- **Metrics** for key operational signals: auth failures, retry budget exhaustion,
  rate-limit hits, reconnect attempts, heartbeat drops, queue depth, and order
  lifecycle latency.
- **A path to bridge** these signals into the consumer's existing observability stack
  (Prometheus, Jaeger, Datadog, OTLP collectors) without the SDK taking a hard
  dependency on any specific backend.

OpenTelemetry (OTel) is the industry standard for vendor-neutral observability. The
OTel Go SDK provides a `*otel.TracerProvider` and `*metric.MeterProvider` that are
configured by the consumer and accepted as interface types in the SDK, making the
observability backend a consumer-side concern.

## Decision

Introduce the following OpenTelemetry packages as runtime dependencies, gated on an
`otel` build tag so consumers who do not need tracing/metrics can `go build` without
them:

```
go.opentelemetry.io/otel            v1.32+
go.opentelemetry.io/otel/metric     v1.32+
go.opentelemetry.io/otel/sdk/trace  v1.32+
go.opentelemetry.io/otel/sdk/metric v1.32+
go.opentelemetry.io/otel/transport  v1.32+   (OTLP exporter internals)
```

A new `WithTracerProvider(*otel.TrTracerProvider)` and
`WithMeterProvider(*otel.MeterProvider)` option on `Client` (and on individual
`Service` constructors) allows consumers to inject their own providers. When no
provider is set, the SDK uses a no-op provider — zero overhead, zero dependency on
an actual collector.

### Instrumentation

All public service methods and the push event dispatcher are instrumented with spans
and metrics:

| Signal | Attribute | Notes |
|--------|-----------|-------|
| `traces` | `hstong.operation`, `hstong.endpoint`, `hstong.market` | Created at service method entry |
| `hstong.auth.failures` (Counter) | `reason=invalid_password\|token_expired\|...` | Auth layer |
| `hstong.http.retries` (Counter) | `endpoint`, `attempt` | Query-only retries (ADR 0003) |
| `hstong.http.rate_limit` (Counter) | `endpoint` | Triggered when Gateway returns 1011/1018 |
| `hstong.push.reconnects` (Counter) | — | Every TCP reconnect |
| `hstong.push.heartbeat_miss` (Counter) | — | Missed heartbeat |
| `hstong.push.queue_depth` (Gauge) | `topic` | Current pending fan-out size |
| `hstong.push.drops` (Counter) | `topic` | Events dropped due to backpressure |
| `hstong.order.latency` (Histogram) | `market`, `order_type` | Time from submission to first push update |
| `hstong.order.mutations` (Counter) | `operation=entrust\|cancel\|change`, `market` | Per mutation type |

Spans are created with `span.SetStatus` based on OTel error codes. Attributes are
low-cardinality strings only — no money values, no session tokens, no passwords.

### Redaction

`slog` redaction: `internal/logging` already redacts sensitive fields (account,
password, token). OTel span attributes follow the same redaction rules. No `db.statement`
or `http.request.body` attributes are set, so request bodies (which contain trade
passwords) are never placed in traces.

### Build tag

The OTel imports live behind `//go:build otel` and are compiled only when the `otel`
tag is active. The no-op path (no tag) uses a stub tracer/meter that compiles to
near-zero overhead. CI and consumers enable the tag explicitly.

## Consequences

- `go.opentelemetry.io/otel` enters `go.mod` as a runtime dependency when the `otel`
  build tag is set. Without the tag, the SDK compiles against the stdlib only.
- The `otel` tag must be explicitly set to activate instrumentation; default build
  is unchanged.
- `WithTracerProvider` / `WithMeterProvider` options are additive to `Client` and
  service constructors — no existing `Client` or `Service` signatures change.
- Metrics use OTel's up-down-counter for queue depth and histogram for latency;
  counters use add for recountable events.
- The OTel SDK version is pinned in `go.mod` with a minimum of v1.32.
- This ADR is required before P03 / E15 can implement the observability work.

## Alternatives considered

| Alternative | Why rejected |
|-------------|--------------|
| Prometheus client directly (`github.com/prometheus/client_golang`) | Hard-codes Prometheus as the backend; consumers on Jaeger/Datadog would need to bridge. OTel is the standard abstraction. |
| `log/slog` only | Insufficient for distributed trace correlation; callers would need to implement their own trace context propagation. |
| `expvar` + custom endpoint | Does not support distributed traces; requires callers to poll, not push. |
| No instrumentation | Rejected: enterprise SDK users require operational signals out of the box. |

## References

- Supersedes: [0004 — Minimal dependency set](./0004-minimal-dependencies.md) (observability branch)
- Run: [plan.md](../runs/2026-09-23-hstong-enterprise-sdk/plan.md) §E05, §E15
- OTel Go: https://opentelemetry.io/docs/languages/go/
- Related: [0003 — No auto-retry orders](./0003-no-auto-retry-orders.md) (retry metrics boundary)
