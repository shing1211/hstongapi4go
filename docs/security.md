# Security

## Trust model

The SDK talks to a Gateway on **localhost**. The Gateway owns platform
connectivity, login, request signing, platform-key selection, and encryption
(ADR 0001). The SDK holds:

- **no** platform credentials,
- **no** developer RSA private key,
- **no** device-binding state,
- **no** signing or request-encryption code beyond the fixed trade-password
  transformation.

The only sensitive value the SDK handles is the plaintext trade password, which it
encrypts on the way out and never logs (see below). The bundled platform public
keys are **public reference data**, not secrets.

## Trade password

On `/trade/TradeLogin` the password is sent as:

```
Base64(AES-ECB/PKCS7(plaintext, key = Base64Decode("m+qS04/2CH1OweCnmXZ3TDZkCQS+hBzY")))
```

- The protocol key is fixed and public, published by HStong. It is the same for
  every client.
- AES-ECB is mandated by the wire protocol, not chosen by the SDK; ECB is not a
  general-purpose secure mode and must not be reused elsewhere.
- The plaintext is held only in memory for the life of the client. It is never
  logged and never appears in an error.

Prefer loading it from the environment or a secret store rather than hardcoding
it:

```go
// reads HSTONG_TRADE_PASSWORD
c, err := client.New(client.WithEnv())
```

Do not call `client.TradePassword()` for logging or telemetry.

## Opt-in push signature verification

Each push frame carries a 128-byte `bodySHA1` field: a `SHA1WithRSA` (PKCS#1
v1.5) signature over the raw, uncompressed body bytes. Verification is **off by
default** because the push channel is already localhost.

Enable it with the environment:

```sh
HSTONG_VERIFY_PUSH=true
```

```go
c, err := client.New(client.WithEnv())
if c.VerifyPush() {
    // verification requested via HSTONG_VERIFY_PUSH
}
```

In this build the verification toggle is read from `HSTONG_VERIFY_PUSH`;
`client.WithPlatformPublicKey` selects the key used when verification is on. The
key may be a PEM `PUBLIC KEY`/`RSA PUBLIC KEY` block, a base64 SPKI DER string
(the bundled constant form), or raw SPKI DER. When verification is enabled, an
unsigned frame is rejected and a frame whose signature does not verify is dropped
rather than delivered; the failure is reported on the subscription's `Errors()`
channel. The bundled keys are base64 SPKI constants in
`pkg/types/platformkeys.go`:

| Constant | Environment |
|----------|-------------|
| `types.PlatformPublicKeyTest` | test (`openapi-daily.hstong.com`) |
| `types.PlatformPublicKeyProd` | production (`openapi.hstong.com`) |

Override the key after a rotation or for an environment whose key is not bundled:

```go
client.WithPlatformPublicKey(types.PlatformPublicKeyProd)
```

An empty value restores the bundled test key. Verification is a local CPU cost on
the push path; a failed verification drops the frame rather than delivering it.

## No secrets in the repository

- The SDK commits no credentials and no private keys.
- The bundled platform public keys are copied verbatim from the published HStong
  documentation and are safe to commit.
- Vendor installer and SDK archives in the repository root are git-ignored.

## Transport

- The HTTP surface is plain **HTTP on loopback** (`http://127.0.0.1:11111`); there
  is no TLS. Treat the machine as the trust boundary.
- The TCP push surface is plain TCP on loopback (`127.0.0.1:11112`).
- Route validation rejects an unregistered path before any request is sent.

## Redaction rules for application code

If you add logging around the SDK:

- Never log `client.TradePassword()`.
- Never log request or response bodies for `/trade/TradeLogin`.
- Prefer logging the Gateway path, the operation label, and the typed error text,
  none of which contain secrets.

See ADR 0005 for the full key-model decision and the legacy
[protocol](LEGACY.md) for what the SDK deliberately does not implement.
