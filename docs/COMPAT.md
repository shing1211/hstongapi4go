# Compatibility Matrix

This document tracks the compatibility of `hstongapi4go` with Go versions and
the HStong Gateway API version.

## Go Version Compatibility

| hstongapi4go | Minimum Go | Notes |
|--------------|------------|-------|
| v0.1.x | Go 1.26 | Current; uses `go 1.26` directive |
| v0.0.x | Go 1.22 | Pre-release history |

The `go` directive in `go.mod` is the **single source of truth** for the minimum
supported Go version. CI runs on the latest stable Go (via `setup-go` with
`check-latest: true`).

## Gateway API Version Compatibility

| hstongapi4go | Gateway Version | Protocol | Notes |
|--------------|----------------|----------|-------|
| v1.0.x | v2.4.x (local) | HTTP+TCP | Current; all 51 endpoints + 11 push topics |
| v0.1.x | v2.4.x (local) | HTTP+TCP | Likely compatible; mock-tested only |
| v0.0.x | v2.3.x (local) | HTTP+TCP | Pre-release history |

The SDK targets the **local HStong Gateway** (HTTP `127.0.0.1:11111`, TCP push
`127.0.0.1:11112`). It does **not** implement the legacy direct-to-platform
protocol. See [LEGACY.md](./LEGACY.md).

**How compatibility is confirmed.** The SDK has no built-in protocol version
negotiation with the Gateway — there is no version handshake. Compatibility is
confirmed by the **integration test suite**, not by matching version numbers:

```
HSTONG_INTEGRATION=1 go test ./test/integration/...
```

A green run against a live Gateway proves that the SDK's request shapes and the
Gateway's response shapes are mutually intelligible. When you upgrade the Gateway,
re-run the suite to detect any drift. If a market data endpoint fails after a
Gateway upgrade, it may indicate the Gateway changed a field type in a response —
the wire codec probe test (`TestIntegration_WireCodecProbe`) is designed to catch
this.

## OpenTelemetry Build Tag

The `otel` build tag enables OpenTelemetry tracing and metrics instrumentation:

| Tag | Build | Stdlib Only | OTel Packages |
|-----|-------|------------|---------------|
| Absent (default) | `go build ./...` | Yes | Not linked |
| Present | `go build -tags otel ./...` | No | v1.36.0 linked |

Without the `otel` tag the SDK compiles with stdlib only. With the tag, the
OTel v1.36.0 packages enter `go.mod` and `spanFactory` in `client` creates
real spans.

## Public API Stability

| Package | Status | Stability |
|---------|--------|-----------|
| `client` | Stable | Semver-guarded; breaking changes in major releases only |
| `pkg/hstong`, `pkg/hstong/market`, `pkg/hstong/trade`, `pkg/hstong/future`, `pkg/hstong/algo`, `pkg/hstong/stream` | Stable | Same as `client` |
| `pkg/types` | Stable | Enums and constants only |
| `pkg/domain` | Stable | Domain models |
| `internal/*` | Internal | No API stability guarantee |
| `gen/*` | Generated | Regenerated from proto; not directly imported |

See [adr/0011-v01x-compatibility.md](./adr/0011-v01x-compatibility.md) for
the full compatibility and deprecation policy.

## Push Topic Support

All 11 documented push topics are implemented in `pkg/hstong/stream` backed by the
push transport (`internal/push/client.go`):

| Topic ID | Description | Package |
|----------|-------------|---------|
| 11, 35 | Quote (basic quote) | market |
| 14, 27, 28, 37 | Tick (trade tick) | market |
| 16 | Broker queue | market |
| 17, 25, 26, 36 | Order book (摆盘) | market |
| TradeStockDeliverNotify | Order status update | trade |
| QotStockDeliverNotify | Futures order update | future |

Counts are canonical in [SPEC.md](./SPEC.md) and must not be hand-edited
elsewhere.
