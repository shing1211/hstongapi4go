# Protocol

The SDK speaks to the local HStong OpenAPI Gateway over two channels:

| Channel | Address | Direction |
|---------|---------|-----------|
| HTTP | `http://127.0.0.1:11111` | request/response |
| TCP push | `127.0.0.1:11112` | server push |

There is no HTTPS and no `GET`. The Gateway owns all platform signing,
encryption, and credentials; the SDK sends plain JSON over localhost.

## HTTP envelope

Every call is `POST http://127.0.0.1:11111<route>`.

Request:

```json
{ "timeout_sec": 10, "params": { "...": "..." } }
```

Response:

```json
{ "ok": true, "err": "", "data": { "...": "..." } }
```

- `params` and `data` are carried as `json.RawMessage` at the envelope layer and
  decoded by the endpoint's codec.
- `timeout_sec` is the resolved client timeout in whole seconds, reduced to the
  whole seconds remaining on the context deadline when that is sooner, and never
  below 1.
- `ok:false` with a non-empty `err` maps to a typed error. See
  [Error Codes](errors.md).
- `params` is omitted entirely when there are no parameters.

## Route aliases

Every logical route is reachable under three alias forms. `client.NormalizePath`
maps all three to one canonical endpoint.

| Alias | Example |
|-------|---------|
| `/X` | `/hq/BasicQot` |
| `/XRequest` | `/hq/BasicQotRequest` |
| `/XRequestMsgType` | `/hq/BasicQotRequestMsgType` |

The canonical routes are the 51 `client.Route` constants; `client.Routes()`
returns them sorted, and `Route.Validate()` rejects an unregistered route before
any request is sent.

## Hybrid codec

Payload encoding is chosen **per endpoint**, never as a global mode
(ADR 0002, amended by ADR 0007).

| Surface | Representation | Decoder |
|---------|----------------|---------|
| TCP push payload | generated proto types after `Any` unpack | binary protobuf |
| Market DTOs in HTTP `data` (9 pull + subscribe) | generated `gen/hq/dto` types | `encoding/json` |
| Trade / futures / algo / assets / session HTTP bodies | hand-written structs with explicit `json:"..."` tags | `encoding/json` |
| Envelope (`timeout_sec`, `params`, `ok`, `err`, `data`) | hand-written structs | `encoding/json` |

Why not `protojson` for the market bodies? The official protobuf package (v2.2.0)
contains no HTTP request/response messages, and the vendor SDKs decode the HTTP
body with a plain JSON parser (Gson in Java, `json.loads` in Python) — not
protobuf's JSON mapping. Gateway `int64` counters therefore arrive as JSON
**numbers**, which `encoding/json` maps onto the generated `int64` fields.
`client.ProtoJSON()` is retained as a per-endpoint fallback only.

The public codec surface:

```go
var jsonCodec  client.Codec = c.JSON()      // encoding/json
var protoCodec client.Codec = c.ProtoJSON() // protojson
```

`client.Codec` is an alias for the transport codec interface, so no internal
type appears in the public API. Both accessors return stateless singletons safe
for concurrent use.

## Money and quantities

Money and quantities are never binary floats.

- Hand-written HTTP bodies carry them as `string`.
- Generated market DTOs keep the proto field types; the prototype uses `int64`
  for counts and `double` for prices.
- `make money-check` scans for `float32`/`float64` misuse of monetary fields.

Never parse a price or amount into `float64` yourself; keep the decimal string
intact or use `math/big` when an exact comparison is needed.

## Push frame

The push channel frames every message as a fixed 151-byte header followed by a
protobuf body. Fields are little-endian.

| Offset | Size | Field | Meaning |
|--------|------|-------|---------|
| 1–2 | 2 | `szHeaderFlag` | fixed `"HS"` |
| 3–4 | 2 | `msgType` | `3` = push |
| 5 | 1 | `protoFmtType` | `0` = protobuf |
| 6 | 1 | `protoVer` | protocol version |
| 7–10 | 4 | `serialNo` | int32 LE sequence |
| 11–14 | 4 | `bodyLen` | int32 LE body length |
| 15–142 | 128 | `bodySHA1` | `SHA1WithRSA` signature of the raw body |
| 143 | 1 | `compressAlgorithm` | `0` = none |
| 144–151 | 8 | `reserved` | reserved |

The body is a `PBNotify`:

```proto
message PBNotify {
    NotifyMsgType notifyMsgType = 1;
    string        notifyId      = 2;
    uint64        notifyTime    = 3;
    google.protobuf.Any payload = 4;
}
```

`payload` is unpacked by the `notifyMsgType` wire enum rather than the `Any`
`type_url` — the vendored protos declare no proto package, so dispatching on the
enum is deterministic. The recognized types are:

| `notifyMsgType` | Value | Payload |
|-----------------|-------|---------|
| `TrsStockDeliverMsgType` | 0 | `TradeStockDeliverNotify` |
| `TradeStockDeliverMsgType` | 1 | `TradeStockDeliverNotify` |
| `FuturesTradeStockDeliverMsgType` | 2 | `TradeStockDeliverNotify` |
| `OrderBookNotifyMsgType` | 20001 | `OrderBookFullNotify` |
| `BrokerQueueNotifyMsgType` | 20002 | `BrokerNotify` |
| `BasicQotNotifyMsgType` | 20003 | `BasicQotNotify` |
| `TickerNotifyMsgType` | 20004 | `TickerNotify` |

Heartbeat, request, and response frame types are read and ignored by the client;
only push frames are dispatched.

## Reconnect and re-subscribe

The push client reconnects with exponential backoff (500 ms initial, 30 s
ceiling, with jitter) and, after each successful reconnect, re-issues
`/hq/Subscribe` for every active market subscription and `/trade/TradeSubscribe`
when trade push is active. Each `Subscription.Errors()` channel receives an
`ErrReconnected` notice, and a re-subscribe failure is reported on the same
channel. Subscription state is restored, never silently dropped.

## Signature verification

`bodySHA1` is a `SHA1WithRSA` (PKCS#1 v1.5) signature over the raw, uncompressed
body bytes. Verification is **opt-in and off by default**:

```go
c, err := client.New(client.WithEnv()) // HSTONG_VERIFY_PUSH=true enables it
if c.VerifyPush() {
    // each frame is verified before it is decoded and delivered
}
```

The key is taken from `WithPlatformPublicKey`, which accepts a PEM `PUBLIC KEY`
or `RSA PUBLIC KEY` block, a base64 SPKI DER string (the bundled constant form),
or raw SPKI DER. When verification is enabled:

- a frame with an all-zero `bodySHA1` is rejected as unsigned;
- a frame whose signature does not verify is **dropped**, not delivered;
- the failure is surfaced on the subscription's `Errors()` channel.

The bundled platform public keys are public reference data, not secrets. See
[Security](security.md) and ADR 0005.

## Transport behavior

- `client.Client.Do` issues **exactly one** HTTP request and never retries. Order
  mutations are never retried at any layer (ADR 0003).
- A non-2xx HTTP status, a malformed envelope, or an `ok:false` response becomes
  a typed error tagged with the operation label.
- If the response `data` is absent or JSON `null`, a supplied output value is
  left untouched.
