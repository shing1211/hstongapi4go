# Legacy Protocol

> **Status: deprecated and NOT implemented.** `hstongapi4go` targets the local
> HStong OpenAPI **Gateway** only. The legacy direct-to-platform protocol
> described on this page is documented for reference and migration; there is no
> code for it in this SDK. See ADR 0001.

## Background

HStong has published two access methods:

1. **Legacy direct-to-platform protocol** — the SDK talks straight to HStong's
   platform servers. Documented at
   <https://quant-open.hstong.com/api-docs/old/>.
2. **Local OpenAPI Gateway** — a process HStong installs on the user's machine
   that exposes HTTP on `http://127.0.0.1:11111` and TCP push on
   `127.0.0.1:11112`, and owns the platform connection on the SDK's behalf.
   Documented at <https://quant-open.hstong.com/api-docs/>.

This SDK implements **method 2 only**.

## What the legacy protocol involved

- **HTTP bootstrap.** `POST /hs/v2/login` to authenticate, then
  `POST /hs/config/queryServer` to obtain the `tradeServer`/`hqServer` socket
  addresses.
- **A raw direct TCP socket** to the platform, framed with the same 151-byte
  header the Gateway now uses for push:

  | Offset | Size | Field | Notes |
  |--------|------|-------|-------|
  | 1–2 | 2 | `szHeaderFlag` | fixed `"HS"` |
  | 3–4 | 2 | `msgType` | request/response/heartbeat/push |
  | 5 | 1 | `protoFmtType` | protobuf |
  | 6 | 1 | `protoVer` | protocol version |
  | 7–10 | 4 | `serialNo` | int32 LE sequence |
  | 11–14 | 4 | `bodyLen` | int32 LE body length |
  | 15–142 | 128 | `bodySHA1` | signature of the raw body |
  | 143 | 1 | `compressAlgorithm` | 0 = none |
  | 144–151 | 8 | `reserved` | reserved |

- **Developer RSA signing.** The developer held an RSA keypair (PKCS#8,
  1024-bit, no passphrase), registered its public key on the HStong developer
  portal, and signed every outbound message with `SHA1WithRSA` into `bodySHA1`.
- **RSA + AES-ECB dynamic-key encryption.** Session keys were negotiated with
  RSA and business bodies encrypted with AES-ECB.
- **Serial-number management and heartbeat.** A client-managed sequence number
  and periodic `HeartBeat` messages kept the long connection alive.
- **Mandatory device binding.** Production access required a device-binding
  flow; it was disabled only in the test environment.

Because of that surface — key management, dynamic-key crypto, serials,
heartbeats, and device binding — the legacy path is a much larger and more
fragile integration than the Gateway, and HStong has deprecated it.

## Why it is not implemented here

Implementing it would require the SDK to carry developer key material, dynamic
session keys, heartbeat timing, and device-binding state, none of which belong in
a client library, and all of which duplicate functionality the Gateway already
performs. See ADR 0001 for the decision and the alternatives considered.

Consequences:

- The SDK carries **no** developer private key, no AES session-key handling, and
  no device-binding flow (ADR 0005).
- Bootstrap endpoints such as `/hs/v2/login` and `/hs/config/queryServer` are not
  modelled.
- Users still on the legacy protocol cannot use this SDK.

## Published platform public keys

The legacy documentation is the source of the platform RSA **public** keys that
the SDK bundles for **opt-in** verification of Gateway push frames. They are
public reference data, not secrets, and are copied verbatim.

Test environment:

```
MIGfMA0GCSqGSIb3DQEBAQUAA4GNADCBiQKBgQCbRuA8hsbbzBKePEZZWaVtYpOjq2XaLZgAeVDlYqgy4lt4D+H2h+47AxVhYmS24O5lGuYD34ENlMoJphLrZkPbVBWJVHJZcRkpC0y36LFdFw7BSEA5+5+kdPFe8gR+wwXQ7sj9usESulRQcqrl38LoIz/vYUbYKsSe3dADfEgMKQIDAQAB
```

Production environment:

```
MIGfMA0GCSqGSIb3DQEBAQUAA4GNADCBiQKBgQDu7xSKk8VNr7WVsxIbltmpe4ViEVNP9QyjRvA2IBm7KCuE6FFyFABSubjhxeZ3joDuNlga0NVtd/qfPf2iursrSOmT00j2JWcR9pQ/mZ61aWEW8BNnkFvu0nijrw0A2z1XhyoaG1p4/pNWKiqcwpA1P23vRoDkfretUF/SlLtQGQIDAQAB
```

They are exposed as `types.PlatformPublicKeyTest` and
`types.PlatformPublicKeyProd`. The signature algorithm is `SHA1WithRSA`
(PKCS#1 v1.5), matching the platform's Java
`Signature.getInstance("SHA1WithRSA")`. See [Security](security.md) and ADR 0005.

## Migrating

| Legacy concept | Gateway equivalent in this SDK |
|----------------|-------------------------------|
| `/hs/v2/login` + keypair | Gateway installs and holds credentials; attach a trade session with `hstong.SessionManager` |
| `/hs/config/queryServer` | Gateway addresses are fixed: `http://127.0.0.1:11111`, push `127.0.0.1:11112` |
| Direct signed TCP socket | HTTP POST calls via `client.Client` |
| Developer RSA signing / AES dynamic key | Owned by the Gateway; the SDK sends plain JSON over loopback |
| Heartbeat keep-alive | `SessionManager.StartKeepAlive` polls a cheap endpoint |
| Device binding | Not applicable to the Gateway path in this SDK |
| Raw push frames | `stream.Client` decodes push frames into typed events |

## What the SDK does implement

The current Gateway surface: 51 HTTP endpoints, 11 market push topics, the
trade/futures push channels, and the channel-based streaming API. Counts and
schemas are in the [API Reference](SPEC.md). The architecture is in
[Protocol](protocol.md); the transport decision is ADR 0001.
