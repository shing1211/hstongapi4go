# 0013 — Accept SHA-1 for push frame verification, and record why

- Status: Accepted
- Date: 2026-09-29

## Context

The TCP push frame carries a 151-byte header whose `bodySHA1` field is a
`SHA1WithRSA` signature over the raw, uncompressed body bytes. The SDK verifies
that signature when a caller opts in, using `crypto/sha1` at
`internal/push/verify.go:10`.

SHA-1 is not a collision-resistant hash by modern standards. A chosen-prefix
collision is practical, and `crypto/sha1` is flagged by `gosec` as G505, which
`verify.go:10` suppresses with a `#nosec` annotation and a justification. The
annotation is the only place in the repository that records the algorithm as a
weakness, and that is too thin a record for a deliberate security decision.

The algorithm is not the SDK's to choose. The Gateway is the signer, and it
signs with `Signature.getInstance("SHA1WithRSA")` in the platform's Java
implementation. `docs/SPEC.md:159` and ADR 0005 both record this; the ADR is
[0005 — Gateway key model and opt-in push
verification](./0005-key-model-and-push-verification.md).

This decision was previously captured only as a residual-risk line in
the P10-hardening.md residual-risk line (archived), an
implementation-phase note. This ADR promotes it to the standing record the rest
of the security decisions live in. It adds no behaviour and changes no default.

## Decision

- **The SDK verifies push `bodySHA1` with `SHA1WithRSA`, as the Gateway signs
  it.** The algorithm is not negotiable at the SDK layer without breaking every
  frame, so `crypto/sha1` stays, and the `#nosec` annotation at `verify.go:10`
  stays with it.
- **Verification remains opt-in and off by default.** This is [ADR
  0005](./0005-key-model-and-push-verification.md)'s decision, unchanged. A
  caller enables it with `WithVerifyPushSignature(true)`.
- **The residual risk is accepted and bounded by the transport, not by the
  hash.** The Gateway listens on `127.0.0.1` and is the trusted transport
  ([ADR 0001](./0001-gateway-transport.md)). An attacker who could substitute a
  colliding frame body already has local code execution on the developer's
  machine, which is a stronger position than the collision buys them. The hash
  choice is therefore not the weakest link on this path.
- **`SHA1WithRSA` is PKCS#1 v1.5, which is also legacy.** Same reasoning, same
  bound: the signature scheme is fixed by the signer, and no alternative exists
  to migrate to.
- **When the platform ships a stronger algorithm, this ADR is superseded, not
  amended.** A new ADR records the migration; `docs/SPEC.md` changes first, since
  the wire format is the source of truth.

## Consequences

- `gosec` G505 remains suppressed at exactly one site, with a justification that
  points here. A second SHA-1 use anywhere in the repository is not covered by
  this record and should be refused.
- The `#nosec` annotation is load-bearing. Removing it fails the `make gosec`
  gate, which is the intended behaviour: the suppression is reviewed rather than
  silent.
- Callers who enable verification on a machine they do not fully trust are
  relying on localhost as the real boundary. This ADR does not make that safe; it
  records that it is the actual trust model.
- No forward migration is available. A deprecation path cannot be written because
  there is no alternative to migrate to.
- The risk register in `docs/threat-model.md` does not carry this row, because the
  register tracks active-path code risks and this is a decided platform
  constraint. It is here instead, which is why this ADR exists.

## Alternatives considered

- **Drop push signature verification entirely.** Removes the `crypto/sha1` use
  and the G505 suppression. Rejected: verification is opt-in, so a caller who
  wants the check should get it, and removing a security capability because its
  hash is deprecated is the wrong trade while the signature is the only
  authenticity signal on the frame.
- **Hash the body with a stronger algorithm and verify that instead.** Rejected:
  it would not be the Gateway's signature. Nothing the SDK computes can
  substitute for a signature over bytes it does not control.
- **Accept the collision risk silently, as the phase file did.** Rejected: a
  suppressed linter finding with no decision record is indistinguishable from an
  oversight six months later. This ADR is the fix for that.

## References

- [0005 — Gateway key model and opt-in push verification](./0005-key-model-and-push-verification.md)
- [0001 — Target the local Gateway](./0001-gateway-transport.md)
- [0004 — Minimal dependency set](./0004-minimal-dependencies.md)
- P10-hardening.md:61 — the residual-risk line this ADR supersedes (archived;
  rationale preserved in this ADR)
- `internal/push/verify.go:10` — the single `crypto/sha1` site
