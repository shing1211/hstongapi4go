# Security Policy

## Supported Versions

`hstongapi4go` is pre-1.0 alpha software. Only the latest release on `main`
receives security fixes.

| Version | Supported |
|---------|-----------|
| 0.1.x (latest) | Yes |
| older | No |

## Reporting a Vulnerability

If you discover a security vulnerability in `hstongapi4go`, please report it
privately using [GitHub Security Advisories](https://github.com/shing1211/hstongapi4go/security/advisories/new)
or by email to **shing1211@users.noreply.github.com**.

**Do not report security vulnerabilities through public GitHub issues.**

Please include:

- a description of the vulnerability and its impact;
- steps to reproduce;
- the affected version (git commit or tag) and Go version;
- any suggested fix.

### Response timeline

- **Acknowledgment:** within 48 hours
- **Initial assessment:** within 1 week
- **Fix or mitigation:** depends on severity; coordinated disclosure is preferred

## Key Model

The SDK talks to a Gateway process on **localhost**. All platform credentials,
signing, and encryption belong to the Gateway, not to this library:

- The SDK holds **no** platform username, password, API key, or token.
- The SDK holds **no** developer RSA private key. The legacy direct-to-platform
  protocol required developer RSA signing and device binding; it is deprecated
  and **not implemented** here (see [docs/LEGACY.md](./docs/LEGACY.md)).
- The SDK performs no request signing and no AES request encryption beyond the
  fixed trade-password transformation described below.

### Bundled platform public keys

For **opt-in** verification of Gateway push frames, the SDK bundles the RSA
**public** keys published by HStong for the test and production environments as
`types.PlatformPublicKeyTest` and `types.PlatformPublicKeyProd`. These are
public reference data, not secrets, and are safe to commit. They can be
overridden with `client.WithPlatformPublicKey` for a rotated or unlisted
environment.

### Trade password

`/trade/TradeLogin` sends the trade password as:

```
Base64(AES-ECB/PKCS7(plaintext, key = Base64Decode("m+qS04/2CH1OweCnmXZ3TDZkCQS+hBzY")))
```

The AES key is fixed and public, mandated by the wire protocol; AES-ECB is
**not** a general-purpose secure mode and must not be reused elsewhere. The
plaintext is held in memory only, is never logged, and never appears in an
error. Prefer `HSTONG_TRADE_PASSWORD` or a secret store over hardcoding, and
never call `client.TradePassword()` for logging or telemetry.

## No Secrets in the Repository

- The SDK commits no credentials, tokens, or private keys.
- The bundled platform public keys are public reference data copied from the
  published HStong documentation.
- Vendor installer and SDK archives in the repository root are git-ignored and
  never committed.

## Transport and Trust Boundary

The local Gateway exposes plain HTTP on `http://127.0.0.1:11111` and plain TCP
push on `127.0.0.1:11112`. There is no TLS; treat the local machine as the trust
boundary and do not expose those ports to a network. Push-frame `SHA1WithRSA`
verification is available but off by default because the channel is already
loopback-local; enable it with `HSTONG_VERIFY_PUSH=true` when defense in depth
is warranted.

See [docs/security.md](./docs/security.md) for the full security model and
[ADR 0005](./docs/adr/0005-key-model-and-push-verification.md) for the key-model
decision.
