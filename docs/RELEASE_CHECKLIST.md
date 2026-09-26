# Release Checklist

This checklist must be completed for every release (version tag). The release
manager runs through this list before tagging and pushing.

## Pre-release (branch: main, all CI green)

- [ ] **1. Feature complete** — all planned tasks for the milestone are done
  and merged to `main`.

- [ ] **2. CI is green** — go to
    <https://github.com/shing1211/hstongapi4go/actions> and confirm all jobs
    (`build`, `lint`, `security`, `coverage`, `goreleaser`, `sbom`, `docs`,
    `license`, `proto`) pass on the release commit.

- [ ] **3. Offline tests** — `go test -race -count=1 ./...` passes locally
    without any credentials or network access.

- [ ] **4. Coverage gate** — `make coverage` shows ≥85% on
    `pkg/domain`, `internal/auth`, `internal/transport`, `internal/push`, the
    released public managers `pkg/hstong`, `pkg/hstong/stream`,
    `pkg/hstong/trade`, `pkg/hstong/algo`, plus `pkg/types` and
    `pkg/transport`.

- [ ] **5. Proto verify** — `make proto-verify` passes (generated code matches
    source protos).

- [ ] **6. Docs check** — `make docs-check` passes: `check_links` reports 0
    unresolved links and `mkdocs build --strict` succeeds.

- [ ] **7. License check** — `make license-check` passes and all new files
    carry the Apache-2.0 SPDX header.

- [ ] **8. Money check** — `make money-check` passes: no `float64` money or
    quantity fields in any `pkg/` or `client/` file.

- [ ] **9. goreleaser check** — `goreleaser check --config .goreleaser.yaml`
    passes.

- [ ] **10. OTel build** — `go build -tags otel ./...` and
    `go test -tags otel -count=1 ./...` pass.

- [ ] **11. Update version** — If this is a version-tagged release (not a
    pre-release snapshot), update `version` field references in documentation
    and confirm the version is consistent across docs.

## Tagging

- [ ] **12. Tag format** — tags follow [SemVer](https://semver.org/):
    `v{major}.{minor}.{patch}`. Example: `v0.1.8`.

- [ ] **13. Tag push** — after tagging, push to both remotes:

    ```bash
    git tag v0.1.x
    git push origin v0.1.x
    git push gitee v0.1.x
    ```

- [ ] **14. GitHub release** — the
    [release workflow](https://github.com/shing1211/hstongapi4go/blob/main/.github/workflows/release.yml)
    automatically creates a GitHub Release with artifacts (checksums, SBOM,
    archives). The tag push in step 13 is what triggers it, and the automatic
    `GITHUB_TOKEN` is what authorizes it, so no local token is needed. Verify the
    release at <https://github.com/shing1211/hstongapi4go/releases>.

- [ ] **15. Signature verifies** — the checksum file is signed with cosign
    keyless over the Actions OIDC identity. This is the step that actually proves
    the artifacts came from this repository's workflow, so do not skip it because
    the release page looks correct. Download the checksums file, its bundle, and
    one archive, then:

    ```bash
    cosign verify-blob \
      --certificate-identity "https://github.com/shing1211/hstongapi4go/.github/workflows/release.yml@refs/tags/v0.1.11" \
      --certificate-oidc-issuer "https://token.actions.githubusercontent.com" \
      --bundle hstongapi4go_0.1.11_checksums.txt.sigstore.json \
      hstongapi4go_0.1.11_checksums.txt
    sha256sum --check --ignore-missing hstongapi4go_0.1.11_checksums.txt
    ```

    Substitute the tag and version. The identity is the workflow file at the tag
    ref, which is why a signature from a different ref or a different repository
    must fail. Only the checksums are signed, so one successful
    `verify-blob` covers every archive.

- [ ] **16. Gitea release** — verify the Gitee release at
    <https://gitee.com/shing1211/hstongapi4go/releases>. **Not expected yet:** the
    Gitee mirror is not configured, because it needs a `GITEE_TOKEN` API secret
    and the mirror cannot be added until one exists. Git push to the `gitee`
    remote working is *not* evidence the mirror works, since the mirror creates
    the release over the Gitee REST API rather than over git.

## Post-release

- [ ] **17. SBOM published** — the `sbom` job uploads the SPDX JSON artifact.
    Download from the CI run and publish alongside the release.

- [ ] **18. Documentation update** — if the MkDocs site is auto-deployed,
    verify the new version appears at
    <https://shing1211.github.io/hstongapi4go/>.

- [ ] **19. Close milestone** — close the corresponding GitHub milestone and
    mark all issues as completed.

- [ ] **20. Announce** — post release notes to any relevant channels
    (optional, depending on release size).

## Hotfix Release

For a hotfix on a past version:

1. Create a branch from the tag: `git checkout -b hotfix/v0.1.x v0.1.x`
2. Apply the fix and add a hotfix-specific test.
3. Bump the patch version in the tag: `v0.1.{patch+1}`.
4. Follow steps 2–11, 12–16, 18–20.

## Rollback

If a release is broken and cannot wait for a hotfix:

1. **Do not delete the tag** from GitHub/Gitea — tags are immutable.
2. Create a new tag at the last known good commit.
3. Add a note to the broken release explaining the regression.
4. File a bug and schedule the hotfix.

### Recovering a release that failed before publishing

Signing runs *before* publish, so a `cosign` failure means the run produced no
artifacts at all rather than a broken release. That is recoverable, because the
workflow can be re-run against an existing tag without moving it:

1. Fix the cause. If signing itself is at fault, delete the `signs` block from
   `.goreleaser.yaml` and commit — an incorrect signing stanza takes down the
   whole release, so publishing unsigned is strictly better than publishing
   nothing.
2. Re-dispatch the Release workflow with the `tag` input set to the affected
   tag. This is the one release operation that needs a personal GitHub token,
   because a manual dispatch is not covered by the automatic `GITHUB_TOKEN`.
3. Re-run step 15 to confirm what actually shipped.

