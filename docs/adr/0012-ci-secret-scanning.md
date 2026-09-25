# 0012 — CI-only dependency: secret scanning in the pipeline

- Status: Accepted
- Date: 2026-09-25

## Context

[ADR 0004](./0004-minimal-dependencies.md) permits no new dependency — runtime,
test, or build — without its own ADR. The threat model's R11 records "no secret
scanning in CI" as **Open**: `gosec` analyses source for insecure patterns and
`govulncheck` queries the vulnerability database, but neither looks for
credentials. A leaked token or private key can therefore enter the repository
through a pull request and reach every clone.

The gap is narrow but concrete, and this repository already commits material a
naive scanner flags as secret:

- `pkg/types/platformkeys.go` holds the HStong **public** keys — public reference
  data by design ([ADR 0005](./0005-key-model-and-push-verification.md)). The
  SDK holds no credentials.
- `proto/` is vendored from the vendor SDKs and contains CJK comments that can
  trip entropy heuristics.
- The test vector `123456 -> W1U8iZIppSE+mBMtzy9vZQ==` is a published protocol
  constant, not a secret.

Scanning without an allowlist for these would fail every run and train reviewers
to ignore the job, which is worse than not having one.

## Decision

Add **`gitleaks` as a CI-only dependency, pinned by version, with an explicit
allowlist committed to the repository.**

- It runs as a pinned GitHub Action in a dedicated `secrets` job and scans the
  full history of the pushed ref, not only the diff, so a credential that landed
  in an earlier commit is still reported.
- The action is pinned to an exact release tag. `@latest` is rejected for the
  same reason `gosec` and `govulncheck` are pinned: an upstream release must not
  change a security result without a commit.
- Configuration is committed as `.gitleaks.toml` and **allowlists by path**, never
  by disabling a rule class. Each path carries a comment naming why its content
  is public.
- It has **no effect on the Go module**: nothing in `go.mod`, and no Go package
  imports it, so consumers see no new dependency.

## Consequences

- A credential committed to any branch is reported by CI, including in history.
- False positives on the cases above are suppressed by path, and the allowlist is
  reviewable in a normal pull request, so a new entry is a visible decision rather
  than a silent suppression.
- The job adds a third-party Action to the CI trust boundary. It runs under the
  repository's default `contents: read` permission and does not write to the
  repository, so its blast radius is a scan result, not a code change.
- Detection is a backstop, not a control; it does not replace keeping secrets out
  of the repository in the first place.

## Alternatives considered

| Alternative | Why rejected |
|-------------|--------------|
| `trufflehog` | Comparable detection; a second tool adds CI time and a second allowlist to maintain for no coverage this configuration does not already get. |
| Rely on `gosec` G101 alone | G101 only matches Go identifiers that look like credentials. It cannot see a token in JSON, YAML, a shell script, or commit metadata. |
| Scan only the current diff | Misses anything already in history, which is the case that matters most after the fact. |
| No secret scanning | Leaves R11 open and accepts that a leaked credential is discovered only if someone notices. |

## References

- Risk register: [threat-model.md](../threat-model.md) R11
- Related: [0004](./0004-minimal-dependencies.md),
  [0005](./0005-key-model-and-push-verification.md),
  [0006](./0006-test-dependencies.md)
