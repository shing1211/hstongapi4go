# Verifying Release Artifacts

This guide explains how to cryptographically verify that the binary archives
published on the [GitHub Releases](https://github.com/shing1211/hstongapi4go/releases)
page were built by this repository's CI and have not been tampered with.

GitHub is the primary download location. The same artifacts are also mirrored to
[Gitee](https://gitee.com/shing1211/hstongapi4go/releases) for convenience, and
verification works identically there: the cosign certificate identity is bound to
the release workflow at the tagged ref, not to the host the file came from. A
successful verification therefore says the archive came from this repository's CI
whichever host served it.

## What is signed

Only the **checksums file** is signed. That file covers every archive produced
for a release (mock Gateway binaries for Linux/macOS/Windows, each in amd64 and
arm64). Verifying the checksums file proves the integrity of all archives; you
do not need to verify each one individually.

Signing uses **cosign keyless signing** over GitHub Actions OIDC. The signing
identity is the workflow file at the tagged ref, so a signature from a different
ref or a different repository will fail verification.

## Download artifacts

From the release page, download:

1. `hstongapi4go_<VERSION>_checksums.txt` — the SHA-256 checksums for all
   archives
2. `hstongapi4go_<VERSION>_checksums.txt.sigstore.json` — the cosign bundle
3. At least one archive (e.g. `hstongapi4go_<VERSION>_linux_amd64.zip`) to
   confirm the checksum itself

## Verify the signature

```bash
cosign verify-blob \
  --certificate-identity 'https://github.com/shing1211/hstongapi4go/.github/workflows/release.yml@refs/tags/<TAG>' \
  --certificate-oidc-issuer 'https://token.actions.githubusercontent.com' \
  --bundle hstongapi4go_<VERSION>_checksums.txt.sigstore.json \
  hstongapi4go_<VERSION>_checksums.txt
```

Substitute `<TAG>` with the release tag (e.g. `v1.0.3`). A successful output
prints `Verified OK`.

If you see `error getting signer: ...`, the release may have failed its signing
step and the bundle was never uploaded. Check the
[Actions tab](https://github.com/shing1211/hstongapi4go/actions) for the
corresponding tag push workflow run.

## Verify the archives

After confirming the checksums file is authentic, verify the archives match:

```bash
sha256sum --check --ignore-missing hstongapi4go_<VERSION>_checksums.txt
```

`--ignore-missing` skips archives you did not download, which is fine — it still
validates the ones you have.

## What this proves

A successful `verify-blob` proves the checksums file was produced by the
GitHub Actions workflow running `.github/workflows/release.yml` for the exact tag
you specified. A successful `sha256sum --check` proves the archives you downloaded
were the ones uploaded by that workflow.

This does **not** prove the code is correct — that is what the integration tests,
lint, and coverage gates are for. It proves you have the same artifacts that CI
built, not something a third party injected.

## Quick checklist

```
[ ] Download checksums file
[ ] Download .sigstore.json bundle
[ ] Download one archive (e.g. linux/amd64 zip)
[ ] cosign verify-blob (certificate-identity = the workflow file at the tag ref)
[ ] sha256sum --check
```
