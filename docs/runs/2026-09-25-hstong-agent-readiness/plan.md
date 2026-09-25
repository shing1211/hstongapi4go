# Plan — hstongapi4go agent readiness and doc reconciliation

- **Run:** `2026-09-25-hstong-agent-readiness`
- **Mode:** BUILD (documentation, tooling, release)
- **Baseline:** `b30b0bf` (v0.1.6 + enterprise-run close-out + graph-derived `ARCHITECTURE.md`)
- **Predecessor:** `../2026-09-23-hstong-enterprise-sdk/` (closed 22/22)
- **Status:** in progress

> Paths are inline code rather than relative links so `scripts/check_links.py`
> cannot be broken by this artifact.

## 1. Why this run exists

The enterprise run closed at 22/22, but a post-close-out audit of git history,
markdown, implementation, and tests found that the documentation no longer
matched the code, and that the agent instructions were partly impossible to
follow. This run fixes the documentation and tooling only. It deliberately
changes **no runtime behaviour**.

## 2. Findings this run addresses

| ID | Finding | Evidence |
|----|---------|----------|
| F1 | The GitNexus mandate in `AGENTS.md` / `CLAUDE.md` was unsatisfiable as written: unconditional `MUST` rules with no defined behaviour when the graph is absent, and stale index statistics | `AGENTS.md:157` claimed 6356 symbols / 17051 relationships; actual index is 6382 / 17083 |
| F2 | Two architecture documents, one authoritative but orphaned and one stale with four claims that are false in code | root `ARCHITECTURE.md` had zero inbound references; `docs/runs/2026-09-23-hstong-enterprise-sdk/ARCHITECTURE.md:19,20,38` and `:15` |
| F3 | No `[Unreleased]` section for 19 commits, including 7 security and resilience fixes | `CHANGELOG.md` jumps straight to `[0.1.6]` |
| F4 | Coverage evidence is a pre-E17 snapshot and was never re-baselined | `evidence/P03-E19-coverage.txt` |
| F5 | Run index carried wrong close-out SHAs and omitted an evidence file | `docs/runs/index.md:6` |
| F6 | Close-out documents predated three later commits; tracker said "twenty of twenty-two" | `report.md:7`, `next-phase.md:14` |
| F7 | All six READMEs claimed ADRs 0001-0007 and omitted the v-next packages | `README.md:282`, package-layout block |
| F8 | Makefile had a stale header, a known-broken goreleaser install path, a non-portable Python default, and an obsolete fmt workaround | `Makefile:14-17,21,52-67,158-163` |
| F9 | `docs/DESIGN.md` exists and is cited by `AGENTS.md` and all six READMEs, but is absent from the mkdocs nav | `mkdocs.yml:77-81` |
| F10 | Four documented "mojibake" defects do not exist: zero U+FFFD and no Latin-1 sequences in the named files | verified across `mkdocs.yml`, `docs/LEGACY.md`, prior run docs, `scripts/check_i18n.py` |

## 3. Phases

| Phase | Work | Gate |
|-------|------|------|
| A | Agent readiness: conditional GitNexus mandate, one architecture authority | `check_links`, `mkdocs --strict`, `gofmt`, `go build`, `go vet` |
| B | Run-artifact accuracy: close-out SHAs, 22/22 accounting, phantom-item removal | `check_links` |
| C | Public docs: six-README sync, `DESIGN.md` into nav | `check_i18n`, `check_links`, `mkdocs --strict` |
| D | Tooling: Makefile targets, Python default, race-test guidance | `gofmt`, `go build`, `go vet`, `go test` |
| E | Release `v0.1.7`: changelog, coverage re-baseline, tag, dual-remote push | full local verification before tagging |

## 4. Decisions taken

1. **GitNexus:** documentation change only. No MCP server upgrade, no host or
   editor-config modification. The global CLI is already current, and the
   CLI fallback named in the mandate works.
2. **Architecture:** the root `ARCHITECTURE.md` becomes canonical and is linked
   from `README.md` and `AGENTS.md`. It stays at the repository root, outside
   `docs_dir`, so the MkDocs site cannot hold a second divergent copy.
3. **Enforcement:** boundary claims are corrected in prose only. No `depguard`
   rule is added, because the v-next layer's fate is an open product decision
   and a rule could fail the build for code that decision may delete.
4. **Release:** `v0.1.7` is cut last, after documentation and tooling.
5. **Phantom findings:** deleted rather than annotated, since the audit trail
   lives in this plan.

## 5. Out of scope

Recorded as candidates, not actioned here:

- **N1** the v-next layer's fate: wire it in behind an opt-in constructor, or
  delete it and keep the released stack. Gates F2's durable resolution.
- **N2** consolidate the three overlapping `internal/push` implementations and
  add read deadlines plus heartbeat liveness.
- **G5** test debt: `pkg/services` market and trading services have no direct
  tests, `pkg/transport/mappers.go` is untested, CI has no `otel`-tag test job
  and no bounded fuzz job, and the coverage gate covers three packages only.
- **G4** the release workflow runs `goreleaser check` on a tag and never
  publishes; cosign provenance needs credentials the repository does not hold.
