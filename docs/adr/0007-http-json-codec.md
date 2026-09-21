# 0007 — Gateway HTTP bodies are plain JSON

- Status: Accepted
- Date: 2026-09-21

## Context

[ADR 0002](./0002-hybrid-codec.md) chose a per-endpoint codec and assigned the
proto-backed market DTOs in the HTTP `data` payload to `protojson`, while leaving
trade, futures, algo, assets, and session bodies on `encoding/json`. Because the
market DTOs are generated protobuf messages, ADR 0002 assumed `protojson` was the
natural decoder and that `int64` counters would arrive in protobuf's JSON mapping
(as quoted strings), not as JSON numbers.

T12, the first phase to exercise the market-pull endpoints, checked that assumption
against the vendored vendor SDKs rather than the SDK's own generated types:

- **Java SDK v2.3.0** (`华盛通OpenAPI-SDK-Java.zip`). In
  `.../gateway/api/HSQuantOpenApiHandle.java` every HTTP response is decoded with
  `gson.fromJson(jsonData, new TypeToken<HttpResult<...>>(){})`, including the
  market payloads (`BasicQotVo`, `BrokerVo`, `OrderBookVo`, `TickerVo`, `KLineVo`,
  `TimeShareVo`, and the option-chain value objects). Gson is a plain JSON parser;
  it never applies protobuf's JSON mapping (`JsonFormat`/`protojson`).
- **Python SDK v2.3.0** (`华盛通OpenAPI-SDK-Python.zip`). In
  `hs/common/socket_client.py`, `post_request` runs
  `json.loads(rsa_utils.bytes_to_str(response))` over the whole HTTP body.

Both SDKs therefore treat the Gateway's HTTP bodies as **generic JSON**, not
protobuf-JSON. Consequently `int64` counters such as `volume`, `turnover`, and
timestamps arrive as JSON **numbers**, and enum values arrive as JSON numbers, not
protobuf-JSON names. `protojson` would instead require quoted integers and its own
enum spelling, so it is the wrong decoder for these bodies.

The push channel is different and unaffected: the same SDKs frame and decode binary
protobuf over TCP (for example `protobuf_utils.unpack_response` in the Python SDK).

## Decision

Treat the Gateway's HTTP `params`/`data` bodies as **plain JSON** and decode them
with `encoding/json` over typed structs:

| Surface | Representation | Codec |
|---------|----------------|-------|
| TCP push payload | generated proto types; `proto.Unmarshal` after `Any` unpack | binary protobuf (unchanged) |
| Market DTOs in HTTP `data` (9 pull + subscribe) | hand-written wrappers whose element types are the generated `gen/hq/dto` protobuf messages | `client.JSON()` (`encoding/json`) |
| Trade / futures / algo / assets / session HTTP bodies | hand-written Go structs with explicit `json:"..."` tags | `client.JSON()` (unchanged) |
| Envelope (`timeout_sec`, `params`, `ok`, `err`, `data`) | hand-written structs; `json.RawMessage` payloads | `encoding/json` (unchanged) |

Rules:

- `int64` is a JSON **number**, not a quoted string. The market wrappers map it
  directly onto the generated `int64` fields.
- Generated `gen/hq/dto` types are still used for typing; only the decoder changes.
- The TCP push path remains binary protobuf; this ADR does not touch it.
- No `.proto` files are invented for the hand-written bodies, and no new dependency
  is added (`encoding/json` is stdlib).

This amends ADR 0002's market branch only; the per-endpoint codec mechanism and the
rest of ADR 0002 stand.

## Consequences

- ADR 0002's "protojson for market DTOs" branch is **deleted** for the HTTP path.
  The hybrid codec and the per-endpoint `Codec` selection are unchanged; only the
  market endpoints' default decoder moves from `protojson` to `encoding/json`.
- `client.ProtoJSON()` was removed from the public API (post-release change, v0.1.0
  was the only release). The `proto` package dependency remains — it is still needed
  for TCP push binary protobuf.
- Market responses use hand-written wrapper structs; their element types remain the
  generated DTOs, so the authoritative field names and precision live in `gen/` and
  the DTO set stays stable.
- The `int64`-as-number assumption is inferred from the vendor SDKs, not yet
  confirmed against a live Gateway. The P12/T35 integration validation must confirm
  the real Gateway's `int64` representation (number versus quoted string) for each
  market endpoint.
- `make proto-verify` and the integration checks detect drift. `make money-check`
  still governs money/quantity fields.

## Alternatives considered

| # | Approach | Decision |
|---|----------|----------|
| A | Keep ADR 0002's `protojson`-for-market as written | Rejected: the vendor SDKs prove the body is generic JSON; `protojson` expects quoted `int64` and protobuf-JSON enum names, neither of which the Gateway sends. |
| B | Hand-written structs only, drop the generated DTOs | Rejected: loses the authoritative DTO field names and types, and duplicates the push DTO set that `Any` unpacking already depends on. |
| C | Use `encoding/json` for push as well | Rejected: the push channel is framed binary protobuf, not JSON. |
| D | `encoding/json` over typed structs; retain `protojson` per endpoint as fallback | Superseded by v0.2.0: the fallback was removed because the market wrappers are not `proto.Message` and the fallback could never work at runtime. |

## References

- Supersedes-in-part: [0002 — Hybrid codec](./0002-hybrid-codec.md) (market DTO branch narrowed)
- Phase: [P03-market-pull.md](../runs/2026-09-21-hstong-full-surface/phases/P03-market-pull.md)
- Plan: [plan.md](../runs/2026-09-21-hstong-full-surface/plan.md) §F3
- Vendor SDKs (v2.3.0): `华盛通OpenAPI-SDK-Java.zip`, `华盛通OpenAPI-SDK-Python.zip` (repository root)
- Implementation: `pkg/hstong/market/doc.go` (codec choice rationale)
- Design: [DESIGN.md](../DESIGN.md) §7 (money and quantities)
