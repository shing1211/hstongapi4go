# 0005 — Gateway key model and opt-in push verification

- Status: Accepted
- Date: 2026-09-21

## Context

Under the legacy direct-to-platform protocol the developer holds an RSA keypair and
signs every outbound socket message; the platform's RSA public key verifies
responses, and an AES-ECB session key encrypts business bodies (see
[0001](./0001-gateway-transport.md), plan §F4).

Under the **local Gateway** model that responsibility moves into the Gateway:

- The **developer RSA keypair** (PKCS#8, 1024-bit, no passphrase) is created by the
  developer and registered on the HStong developer portal. The Gateway loads it from
  `privateKeyPath` in the Gateway configuration. The SDK never sees this key.
- The **platform RSA public key** is auto-selected by the Gateway according to the
  configured `domain` (test vs production). The SDK never needs to choose it for
  signing.

The TCP push frame still carries a 151-byte header whose `bodySHA1` field is a
`SHA1WithRSA` signature of the raw (uncompressed) body bytes, produced by the
platform. A client can verify it, but the Gateway is on localhost and is the
trusted transport.

## Decision

- **The SDK performs no RSA signing and no request encryption.** There is no
  developer key, no `privateKeyPath`, no AES-ECB key handling in this SDK. The
  Gateway owns all of it.
- **Push `bodySHA1` verification is opt-in and off by default.** The SDK exposes an
  option such as `WithVerifyPushSignature(true)`. When enabled, each frame's
  `SHA1WithRSA` signature is verified over the raw body bytes before the frame is
  decoded and delivered. A failed verification drops the frame and surfaces a typed
  error; a tampered payload is never delivered to subscribers.
- **Published platform public keys are bundled as constants** — one for the test
  environment and one for production — so verification works out of the box. An
  override is available via `WithPlatformPublicKey(pemOrBase64SPKI)` for rotation and
  for environments whose keys are not yet in this document.
- The bundled keys are **public reference data, not secrets**. They are copied
  verbatim from the published HStong documentation and are safe to commit.

### Bundled platform public keys (base64 SPKI)

Test environment:

```
MIGfMA0GCSqGSIb3DQEBAQUAA4GNADCBiQKBgQCbRuA8hsbbzBKePEZZWaVtYpOjq2XaLZgAeVDlYqgy4lt4D+H2h+47AxVhYmS24O5lGuYD34ENlMoJphLrZkPbVBWJVHJZcRkpC0y36LFdFw7BSEA5+5+kdPFe8gR+wwXQ7sj9usESulRQcqrl38LoIz/vYUbYKsSe3dADfEgMKQIDAQAB
```

Production environment:

```
MIGfMA0GCSqGSIb3DQEBAQUAA4GNADCBiQKBgQDu7xSKk8VNr7WVsxIbltmpe4ViEVNP9QyjRvA2IBm7KCuE6FFyFABSubjhxeZ3joDuNlga0NVtd/qfPf2iursrSOmT00j2JWcR9pQ/mZ61aWEW8BNnkFvu0nijrw0A2z1XhyoaG1p4/pNWKiqcwpA1P23vRoDkfretUF/SlLtQGQIDAQAB
```

The algorithm is `SHA1WithRSA` (PKCS#1 v1.5), matching the platform's Java
`Signature.getInstance("SHA1WithRSA")`.

## Consequences

- The SDK has no signing code and no credential material beyond public keys; there is
  nothing sensitive to redact for this concern.
- Verification is a local CPU cost on the push path, so it is off by default to keep
  the hot path allocation-light.
- Key rotation is a one-line override; the bundled defaults may lag the platform
  (plan §R7).
- Tests pin known-answer vectors for both bundled keys and the override path.
- Users who need end-to-end push authenticity enable the option; users on a
  loopback-trusted machine can leave it off.

## Alternatives considered

| Alternative | Why rejected |
|-------------|--------------|
| Sign requests in the SDK (legacy model) | The Gateway already signs; the SDK has no developer key and duplicating the work is both impossible and redundant. |
| Verify push signatures by default | Adds CPU cost on every frame for a channel that is already localhost; opt-in keeps the default fast. |
| Fetch platform keys from the Gateway at runtime | No documented local endpoint serves them; bundling the published keys plus an override is deterministic and offline-friendly. |
| Treat the keys as secrets and require environment configuration | They are published reference data; requiring configuration adds friction with no security benefit. |

## References

- Plan: [plan.md](../runs/2026-09-21-hstong-full-surface/plan.md) §Assumption 7, §F4, §R7
- Legacy documentation (key source, not implemented): https://quant-open.hstong.com/api-docs/old/
- Related: [0001](./0001-gateway-transport.md), [0004](./0004-minimal-dependencies.md)
