# 0001 — Target the local Gateway (HTTP + TCP push)

- Status: Accepted
- Date: 2026-09-21

## Context

HStong (华盛) exposes two access methods:

1. **Legacy direct-to-platform protocol.** HTTP login (`POST /hs/v2/login`),
   `POST /hs/config/queryServer` to obtain `tradeServer`/`hqServer` addresses, then a
   raw TCP socket carrying a 151-byte header and `PBRequest` / `PBResponse` /
   `HeartBeat` / `PBNotify` bodies, with developer RSA signing (`SHA1WithRSA`,
   `bodySHA1`), RSA + AES-ECB dynamic-key encryption, serial-number management,
   heartbeat keep-alive, and mandatory device binding. Documented at
   https://quant-open.hstong.com/api-docs/old/ (legacy).
2. **Local OpenAPI Gateway.** A separately installed process exposing HTTP POST on
   `http://127.0.0.1:11111` and a TCP push channel on `127.0.0.1:11112`. The Gateway
   owns platform connectivity, login, request signing, platform-key selection, and
   encryption; the SDK sees plain JSON over localhost.

The current documentation (https://quant-open.hstong.com/api-docs/) defines the
Gateway surface as 51 HTTP endpoints plus 11 market push topics and the trade/futures
push channels.

## Decision

- The SDK targets **only the local Gateway**: HTTP `127.0.0.1:11111` and TCP push
  `127.0.0.1:11112`.
- The **legacy direct-to-platform protocol is documented but NOT implemented**. It is
  recorded as deprecated and out of scope: `/hs/v2/login`, `/hs/config/queryServer`,
  the direct 151-byte socket with developer RSA signing, AES-ECB dynamic key,
  heartbeat, and device binding. [`docs/LEGACY.md`](../LEGACY.md) states this
  explicitly.
- The SDK never bundles, installs, launches, or redistributes the Gateway; the user
  installs and runs it (plan assumption 1).
- Legacy documentation remains a source of reference constants only: status codes,
  enums, data dictionaries, and limits (plan §F4).

## Consequences

- The SDK carries no platform credentials, no developer private key, and no
  device-binding flow (see [0005](./0005-key-model-and-push-verification.md)).
- Connection setup is a local HTTP/TCP dial. Failure modes are local (Gateway not
  running, port unreachable, HTTP status) rather than remote socket failures.
- The legacy protocol's signing, encryption, serial, and heartbeat machinery is out
  of scope for code and tests.
- Users still on the legacy protocol cannot use this SDK; the docs must say so.

## Alternatives considered

| Alternative | Why rejected |
|-------------|--------------|
| Implement the legacy direct-to-platform protocol | Deprecated by HStong; requires developer RSA signing, AES-ECB dynamic keys, heartbeat, serial management, and mandatory device binding. A much larger, unmaintainable surface for a path being retired. |
| Implement both surfaces | Doubles protocol, auth, crypto, test, and doc surface for a deprecated path. |
| Ship a wrapper around a third-party binary | No typed Go surface; not an SDK. |

## References

- Plan: [plan.md](../runs/2026-09-21-hstong-full-surface/plan.md) §Goal, §Scope, §F1, §F4
- Legacy documentation (not implemented): https://quant-open.hstong.com/api-docs/old/
- Current documentation: https://quant-open.hstong.com/api-docs/
- Precedent: `futuapi4go` targets the local OpenD daemon rather than the broker's
  remote gateway.
- Related: [0005](./0005-key-model-and-push-verification.md)
