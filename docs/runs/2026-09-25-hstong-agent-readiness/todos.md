# Todos — hstongapi4go agent readiness and doc reconciliation

- **Run:** `2026-09-25-hstong-agent-readiness`
- **Plan:** `plan.md`
- **Status legend:** `todo` · `doing` · `done` · `cancelled`
- **Rule:** this file is the run-wide single source of truth.

## Progress

| Phase | Tasks | Done |
|-------|-------|------|
| A Agent readiness | 2 | 2 |
| B Run artifacts | 3 | 3 |
| C Public docs | 2 | 2 |
| D Tooling | 5 | 5 |
| E Release v0.1.7 | 4 | 4 |
| **Total** | **16** | **16** |

## Tasks

| ID | Task | Status | Acceptance |
|----|------|--------|------------|
| A1 | GitNexus mandate conditional; refresh index stats; add storage-error and fresh-clone guidance | done | `AGENTS.md` + `CLAUDE.md` state what to do when the graph is absent |
| A2 | Root `ARCHITECTURE.md` canonical; correct four false boundary claims; fix `pkg/domain/doc.go` | done | zero inbound references to the run doc presented as current; `go build` clean |
| B1 | Correct run index close-out SHAs; add E19 evidence; note post-close-out commits | done | SHAs resolve to real commits |
| B2 | Add post-close-out addendum to `report.md` | done | report matches git history |
| B3 | `next-phase.md`: 22/22 accounting; delete phantom mojibake items; keep real G8 items | done | no documented defect that does not exist |
| C1 | Sync six READMEs: ADR range, v-next layout, status rows, re-sync date | done | `check_i18n` and `check_links` pass |
| C2 | Add `docs/DESIGN.md` to the mkdocs nav | done | `mkdocs build --strict` exit 0 |
| D1 | Correct the stale Makefile header comment | done | header matches reality |
| D2 | Fix `goreleaser-check` install path to match the pinned CI mechanism | done | target no longer uses the known-broken path |
| D3 | Make the Python default portable | done | `money-check`, `docs-check`, `sbom` work on this host |
| D4 | Simplify or drop the obsolete `fmt-check` CRLF workaround | done | no misleading CRLF guidance |
| D5 | Reconcile `AGENTS.md` race-test guidance with the CI-only reality on Windows | done | guidance matches what the host can run |
| E1 | `CHANGELOG.md`: add `[Unreleased]`, promote to `[0.1.7]` with security fixes foregrounded | done | all 25 unreleased commits represented |
| E2 | Re-baseline `evidence/P03-E19-coverage.txt` against current code | done | gate re-run; failures fixed with tests, never by lowering the bar |
| E3 | Annotated tag `v0.1.7`; push `main` and tag to `origin` and `gitee` | done | both remotes verified by `ls-remote` |
| E4 | README release badge to `v0.1.7` across six languages | done | `check_i18n` passes |

## Log

| When | Note |
|------|------|
| 2026-09-25 | Run created. Audit of git history, markdown, implementation, and tests found F1-F10 (see `plan.md` §2). Decisions: docs-only GitNexus change, root `ARCHITECTURE.md` canonical, prose-only boundary fix, release last, phantom findings deleted. Test debt (G5) and the v-next layer decision (N1) recorded as out of scope. |
| 2026-09-25 | **A-D done** (`7cad13c`, `755bce1`, `38cb076`, `06b9927`, `a95f271`, `2982931`). Verified: gofmt clean outside `gen/`, `go build` and `go vet` pass with and without `-tags otel`, `go test ./... -count=1` green across 23 packages, `go test -race` not run locally (no C toolchain; CI is authoritative), money-check clean, `check_links` 92 files / 0 unresolved, `check_i18n` 6 languages, `mkdocs build --strict` exit 0 with no unlisted pages. `gitnexus detect-changes` reported 15 files, 46 symbols, **0 affected processes, risk low** before committing. |
| 2026-09-25 | **E1-E2 done.** `CHANGELOG.md` gains `[0.1.7]` covering all 25 unreleased commits, with the seven E17 security fixes and the CI repair foregrounded. Coverage re-baselined: `pkg/domain` 93.8%, `internal/auth` 94.2%, `internal/transport` 99.0%, gate holds at 85% without moving the threshold. |
| 2026-09-25 | **E3-E4 done; run closed at 16/16.** Annotated tag `v0.1.7` created at `f9cfb43` and `main` + tag pushed to `origin` and `gitee`; both remotes verified by `ls-remote` at `main = f9cfb43`, `tag = 1002bc4`. README badge bumped to v0.1.7 (English only — the five translations carry no release badge). `docs/RELEASE_CHECKLIST.md` example tag refreshed. Post-release verification still owed: the v0.1.7 CI run was **not** confirmed green, because the `gh` token is invalid on this host. |
