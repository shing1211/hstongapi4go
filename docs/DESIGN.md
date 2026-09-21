# Design — hstongapi4go

- **Module:** `github.com/shing1211/hstongapi4go`
- **Status:** Living document. Reflects the accepted ADRs; update it when they change.

This document is the architecture overview. The binding decisions live in
[`docs/adr/`](./adr/README.md); the plan of record is
[`plan.md`](./runs/2026-09-21-hstong-full-surface/plan.md).

## 1. Overview

`hstongapi4go` is an idiomatic Go SDK for the HStong (华盛) Quant OpenAPI **local
Gateway**. It targets a Gateway installed by the user at `127.0.0.1:11111` (HTTP) and
`127.0.0.1:11112` (TCP push). The legacy direct-to-platform protocol (developer RSA
signing, AES-ECB dynamic key, device binding) is documented but not implemented — see
[ADR 0001](./adr/0001-gateway-transport.md).

The SDK never bundles, launches, or manages the Gateway. It exposes typed managers
for market data, trading, futures, and algo surfaces, plus a channel-based streaming
API over the push channel.

```
Application
  +-- client/Client            (New, options, env, Codec selection, Close)
        +-- market/            (BasicQot, OrderBook, KL, TimeShare, Ticker, Broker, OptionChain, Overnight, Subscribe)
        +-- trade/             (Session, Assets, Orders, Push subscribe)
        +-- future/            (11 Futures* endpoints)
        +-- algo/              (7 Algo* endpoints)
        +-- stream/            (market + trade + futures push subscriptions)
              +-- internal/push   (TCP dial, 151B framing, PBNotify, Any unpack, reconnect+resubscribe)
        +-- internal/transport    (HTTP POST, envelope, codec dispatch, error mapping)
              +-- OpenAPI Gateway
                    +-- HTTP 127.0.0.1:11111
                    +-- TCP  127.0.0.1:11112
```

## 2. Package layout

| Path | Contents | Notes |
|------|----------|-------|
| `proto/` | Vendored official `.proto` sources (PB v2.2.0) + provenance | Source of truth for push/DTO types |
| `gen/hstong/...` | Generated Go protobuf code | Committed; **never hand-edited** |
| `client/` | Core: options, environment, codec selection, alias routing, `Close` | Public entry point |
| `internal/transport/` | HTTP executor, envelope, codec dispatch, error mapping | Not importable by users |
| `internal/push/` | 151-byte framing, `PBNotify`, `Any` unpack, reconnect + resubscribe | Not importable by users |
| `internal/crypto/` | AES-ECB/PKCS7 trade-password encryption | Not importable by users |
| `internal/errs/` | Typed errors + status-code table | Not importable by users |
| `internal/resilience/` | Rate limit, retry budgets, circuit breaker | Mutations excluded; see [ADR 0003](./adr/0003-no-auto-retry-orders.md) |
| `internal/logging/` | `log/slog` helpers + redaction | Never-nil logger |
| `internal/metrics/` | Dependency-free, OTel-bridgeable interface | See [ADR 0004](./adr/0004-minimal-dependencies.md) |
| `pkg/hstong/` | Public managers: `market`, `trade`, `future`, `algo`, `stream` | Domain API surface |
| `pkg/types/` | Enums and domain types from the data dictionaries | Public |
| `test/mockgateway/` | Mock HTTP (51 routes) + TCP push server | Offline tests |
| `examples/`, `scripts/`, `docs/`, `.github/workflows/` | Examples, tooling, docs, CI | |
| `cmd/hstong-mock-gateway/` | Standalone mock Gateway binary | |

## 3. Wire protocol

### 3.1 HTTP envelope

All Gateway HTTP calls are `POST` to `http://127.0.0.1:11111`. There is no HTTPS and
no `GET`.

Request:

```json
{ "timeout_sec": 10, "params": { "...": "..." } }
```

Response:

```json
{ "ok": true, "err": "", "data": { "...": "..." } }
```

- `params` and `data` are carried as `json.RawMessage` at the envelope layer and
  decoded by the endpoint's `Codec`.
- `ok: false` with a non-empty `err` maps to a typed error (see §5).

### 3.2 Route aliases

Every logical route is reachable under three alias forms:

| Alias | Example |
|-------|---------|
| `/X` | `/hq/BasicQot` |
| `/XRequest` | `/hq/BasicQotRequest` |
| `/XRequestMsgType` | `/hq/BasicQotRequestMsgType` |

The router normalizes all three to one canonical endpoint.

### 3.3 Push frame (151-byte header)

The TCP push channel frames every message as a fixed 151-byte header followed by the
body. Fields are little-endian.

| Offset | Size | Field | Type | Value / meaning |
|--------|------|-------|------|-----------------|
| 1-2 | 2 | `szHeaderFlag` | UTF-8 chars | Fixed `"HS"` |
| 3-4 | 2 | `msgType` | int16 LE | `3` = push |
| 5 | 1 | `protoFmtType` | byte | `0` = Protobuf |
| 6 | 1 | `protoVer` | byte | Protocol version |
| 7-10 | 4 | `serialNo` | int32 LE | Sequence number |
| 11-14 | 4 | `bodyLen` | int32 LE | Body length |
| 15-142 | 128 | `bodySHA1` | bytes | `SHA1WithRSA` signature of raw body (see §3.5) |
| 143 | 1 | `compressAlgorithm` | byte | `0` = none |
| 144-151 | 8 | `reserved` | int64 LE | Reserved |

The body is a `PBNotify`:

```proto
message PBNotify {
    NotifyMsgType notifyMsgType = 1;
    string notifyId = 2;
    uint64 notifyTime = 3;
    google.protobuf.Any payload = 4;
}
```

`payload` is unpacked via the full 17-proto `Any` registry, then decoded by the
topic's decoder.

### 3.4 NotifyMsgType values

| Name | Value | Surface |
|------|-------|---------|
| `TrsStockDeliverMsgType` | 0 | Trade |
| `TradeStockDeliverMsgType` | 1 | Trade |
| `FuturesTradeStockDeliverMsgType` | 2 | Futures |
| `OrderBookNotifyMsgType` | 20001 | Market |
| `BrokerQueueNotifyMsgType` | 20002 | Market |
| `BasicQotNotifyMsgType` | 20003 | Market |
| `TickerNotifyMsgType` | 20004 | Market |

Market subscription `topicId`s are `11, 35` (quote), `14, 27, 28, 37` (tick), `16`
(broker), and `17, 25, 26, 36` (order book).

### 3.5 Trade password encryption

`TradeLogin` sends the trade password as
`Base64(AES-ECB/PKCS7(password, key = Base64Decode("m+qS04/2CH1OweCnmXZ3TDZkCQS+hBzY")))`.
The AES-ECB/PKCS7 helper lives in `internal/crypto/` and is verified against the
documented vector `123456 -> W1U8iZIppSE+mBMtzy9vZQ==`.

### 3.6 Push signature verification

`bodySHA1` is a `SHA1WithRSA` signature over the raw body bytes, produced by the
platform. Verification is **opt-in and off by default**; bundled test and production
platform public keys are constants and overridable. See
[ADR 0005](./adr/0005-key-model-and-push-verification.md).

## 4. Hybrid codec

Per-endpoint codec selection. See [ADR 0002](./adr/0002-hybrid-codec.md).

| Surface | Representation | Codec |
|---------|----------------|-------|
| TCP push payloads | generated proto types; `Any` unpack | binary protobuf |
| Market DTOs in HTTP `data` | generated proto types | `protojson` |
| Trade / futures / algo / assets / session HTTP bodies | hand-written structs with explicit `json:"..."` tags | `encoding/json` |
| Envelope (`timeout_sec`, `params`, `ok`, `err`, `data`) | hand-written; `json.RawMessage` payloads | `encoding/json` |

The official PB package contains **no HTTP request/response messages**, which is why
the non-market HTTP bodies are hand-written. The fallback is documented in ADR 0002:
any market endpoint may switch to `encoding/json` against a mirror struct without a
public API change.

## 5. Error categories

`internal/errs` maps Gateway status codes to typed, categorized errors. Callers match
with `errors.Is` / `errors.As`, never by string. Categories consulted by the
resilience layer:

| Category | Codes | Behavior |
|----------|-------|----------|
| Success | `0000` | No error |
| Authentication / session | `1006`, `1012`, `1013`, `1014`, `20033` | Trigger single-flight re-login; `20033` is the futures trade-login timeout |
| Validation | `1010`, `1016` | Not retryable; caller must fix the request |
| Protocol / state | `1002`, `1003`, `1004`, `1005`, `1009`, `1017` | Not retryable without re-initializing the connection or upgrading |
| Deprecated | `1005` | Not retryable; endpoint retired |
| Transient / retryable | `1011`, `1015`, `1018` | Retryable for **query** endpoints only (see [ADR 0003](./adr/0003-no-auto-retry-orders.md)) |
| Duplicate | `1007` | Not retryable; reconcile before resubmitting |
| Business | `1001`, `1008`, `40001`, `40002` | Not retryable; surface as-is |

Ambiguous mutation failures (timeout after send) never retry and instruct the caller
to reconcile via the real/history entrust and deliver queries.

## 6. Concurrency and context conventions

- Every blocking call takes `context.Context` as its first parameter. Cancellation
  and deadlines propagate to the HTTP request and the dial.
- The `client.Client` is safe for concurrent use. Managers are obtained once and
  shared.
- The streaming API exposes channels: `Updates()` and `Errors()`. Cancel by
  cancelling the stream's context or calling `Close`.
- `Close` is idempotent and releases the HTTP transport and the push connection.
  After `Close`, no goroutine spawned by the SDK remains (verified with `goleak`).
- Re-login is single-flight: concurrent `1012/1013/1014` failures trigger one login,
  and waiters resume afterward.
- The push client reconnects with backoff and re-subscribes all active topics;
  subscription state is restored, not silently dropped.
- Options follow the functional-option pattern (`Option func(*Config)`, `WithX`
  constructors) and are applied in order on top of defaults.
- No internal types appear in public signatures; public mirrors are defined in the
  public package when needed.

## 7. Numeric precision

- Money and quantities are never `float64`.
- In hand-written HTTP bodies they are `string` (or `json.Number` where the wire
  value is numeric).
- Generated market DTOs keep the proto types' `int64`/`double` fields and are decoded
  with `protojson`, which renders 64-bit integers as strings.
- `make money-check` scans for `float32`/`float64` misuse of monetary fields.

## 8. Related documents

- Decisions: [`docs/adr/`](./adr/README.md)
- Plan of record: [`plan.md`](./runs/2026-09-21-hstong-full-surface/plan.md)
- Canonical counts: [docs/SPEC.md](./SPEC.md) — endpoint/topic/schema inventory
- Legacy protocol: [docs/LEGACY.md](./LEGACY.md) — documented but not implemented
