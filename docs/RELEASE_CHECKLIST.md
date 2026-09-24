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
    `pkg/domain`, `internal/auth`, `internal/transport`.

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
    `v{major}.{minor}.{patch}`. Example: `v0.1.5`.

- [ ] **13. Tag push** — after tagging, push to both remotes:

    ```bash
    git tag v0.1.x
    git push origin v0.1.x
    git push gitee v0.1.x
    ```

- [ ] **14. GitHub release** — the
    [release workflow](https://github.com/shing1211/hstongapi4go/blob/main/.github/workflows/release.yml)
    automatically creates a GitHub Release with artifacts (checksums, SBOM,
    archives). Verify the release at
    <https://github.com/shing1211/hstongapi4go/releases>.

- [ ] **15. Gitea release** — verify the Gitea release at
    <https://gitee.com/shing1211/hstongapi4go/releases>.

## Post-release

- [ ] **16. SBOM published** — the `sbom` job uploads the SPDX JSON artifact.
    Download from the CI run and publish alongside the release.

- [ ] **17. Documentation update** — if the MkDocs site is auto-deployed,
    verify the new version appears at
    <https://shing1211.github.io/hstongapi4go/>.

- [ ] **18. Close milestone** — close the corresponding GitHub milestone and
    mark all issues as completed.

- [ ] **19. Announce** — post release notes to any relevant channels
    (optional, depending on release size).

## Hotfix Release

For a hotfix on a past version:

1. Create a branch from the tag: `git checkout -b hotfix/v0.1.x v0.1.x`
2. Apply the fix and add a hotfix-specific test.
3. Bump the patch version in the tag: `v0.1.{patch+1}`.
4. Follow steps 2–11, 12–15, 17–19.

## Rollback

If a release is broken and cannot wait for a hotfix:

1. **Do not delete the tag** from GitHub/Gitea — tags are immutable.
2. Create a new tag at the last known good commit.
3. Add a note to the broken release explaining the regression.
4. File a bug and schedule the hotfix.
