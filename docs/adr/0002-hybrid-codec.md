# 0002 — Hybrid codec: generated protobuf + hand-written JSON

- Status: Accepted
- Date: 2026-09-21

## Context

The official protobuf package (v2.2.0, `华盛通OpenAPI-SDK-PB.zip`) contains exactly
**17 `.proto` files** and **no HTTP request or response messages** (plan §F2):

| Group | Files |
|-------|-------|
| common | `NotifyMsgType`, `HeartBeat`, `Notify` (`PBNotify`) |
| market DTO | `Security`, `BasicQot`, `FutureBasicQotExData`, `OptionBasicQotExData`, `Broker`, `OrderBook`, `Ticker`, `TimeShare`, `KLine` |
| market notify | `BasicQotNotify`, `BrokerNotify`, `OrderBookFullNotify`, `TickerNotify` |
| trade notify | `TradeStockDeliverNotify` |

The protocols carry no `go_package` option, so Go package paths must be supplied at
codegen time.

The Gateway HTTP wire format is JSON: request
`{"timeout_sec":10,"params":{...}}`, response
`{"ok":true,"err":"","data":{...}}`. The TCP push channel frames a protobuf
`PBNotify` inside a 151-byte header.

Uniform `protojson` ("approach A") therefore cannot work: the trade, futures, algo,
assets, and session HTTP bodies have **no proto types**. Inventing `.proto` files for
undocumented bodies would guarantee drift against the Gateway.

## Decision

Adopt a **per-endpoint `Codec` abstraction** with a hybrid representation:

| Surface | Representation | Codec |
|---------|----------------|-------|
| TCP push payload | generated proto types; `notifyMsgType` enum dispatch | binary protobuf |
| Market DTOs in HTTP `data` (9 pull + subscribe) | hand-written wrappers over `gen/hq/dto` types | `encoding/json` (**superseded by ADR 0007**) |
| Trade / futures / algo / assets / session HTTP bodies | hand-written Go structs with explicit `json:"..."` tags | `encoding/json` |
| Envelope (`timeout_sec`, `params`, `ok`, `err`, `data`) | hand-written structs; `json.RawMessage` payloads | `encoding/json` |

Rules:

- `Codec` is selected **per endpoint** by the router, never as a global mode.
- Money and quantities in hand-written structs are `string` / `json.Number`, never
  `float64` (plan assumption 5; see [0003](./0003-no-auto-retry-orders.md) for the
  retry rule and `make money-check`).
- No `.proto` files are invented for trade/futures/algo/session bodies.

**Fallback (superseded).** ADR 0007 replaced the market DTO `protojson` branch with
`encoding/json`. The documented `protojson` fallback no longer exists. If a market
endpoint needs a different decoder, the fix is a `json.RawMessage` decode against a
hand-written mirror struct scoped to that endpoint.

## Consequences

- Push decoding and market precision depend on the vendored PB v2.2.0 types. The
  complete 17-proto set must be generated so `google.protobuf.Any` unpacks correctly
  (plan §R3).
- Hand-written bodies are maintained from the documentation tables and pinned by
  mock-gateway fixtures (plan §R4).
- Two JSON encoders coexist; the router, not the caller, decides which is used.
- Every new endpoint must declare its codec explicitly.

## Alternatives considered

| # | Approach | Decision |
|---|----------|----------|
| A | Uniform `protojson` over generated types only | Rejected: the PB package lacks all HTTP bodies (F2). |
| B | Fully hand-written structs, no codegen | Rejected: loses the authoritative push/DTO types and the `Any` registry. |
| C | Hybrid — generated protos for push + market DTOs; hand-written for the rest | **Chosen** (plan §F3). |

## References

- Plan: [plan.md](../runs/2026-09-21-hstong-full-surface/plan.md) §F2, §F3, §Approach
- [docs/SPEC.md](../SPEC.md) — per-endpoint codec inventory and PB version record
- Related: [0004](./0004-minimal-dependencies.md), [0005](./0005-key-model-and-push-verification.md)
