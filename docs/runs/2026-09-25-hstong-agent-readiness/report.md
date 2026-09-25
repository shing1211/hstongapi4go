# Close-Out Report — hstongapi4go agent readiness and doc reconciliation

- **Run:** `2026-09-25-hstong-agent-readiness`
- **Mode:** BUILD (documentation, tooling, release)
- **Status:** complete, 16/16
- **Baseline:** `b30b0bf`
- **Deliverable release:** v0.1.7 (`f9cfb43`, tag object `1002bc4`)
- **Scope:** documentation, build tooling, and the release. **No runtime behaviour changed**, except one corrected package comment in `pkg/domain/doc.go`.

## 1. Outcome

A post-close-out audit of git history, markdown, implementation, and tests found
that the documentation no longer matched the code and that the agent
instructions were partly impossible to follow. Ten findings (F1–F10 in
`plan.md` §2) were reconciled across eight commits, then the result was
released as v0.1.7.

| Phase | Work | Result |
|-------|------|--------|
| A | Agent readiness | GitNexus mandate made satisfiable; one architecture authority |
| B | Run artifacts | Close-out SHAs, 22/22 accounting, phantom item removed |
| C | Public docs | Six-README sync, `DESIGN.md` into nav |
| D | Tooling | Makefile targets, Python default, race guidance |
| E | Release v0.1.7 | Changelog, coverage re-baseline, tag, dual-remote push |

Commits: `7cad13c`, `755bce1`, `38cb076`, `06b9927`, `a95f271`, `2982931`,
`00ea408`, `f9cfb43`.

## 2. What was wrong, and what changed

**The agent mandate was unsatisfiable.** `AGENTS.md` and `CLAUDE.md` made
`impact` and `detect_changes` unconditional requirements, but the GitNexus
index is a local, git-ignored artifact, so a fresh clone has none — and a stale
MCP server fails every graph read while the CLI works. The rules now apply
whenever the graph can answer, with an explicit fallback requiring an agent to
*say* graph analysis was unavailable rather than skip it silently. The
statistics were stale (6,356/17,051 against an actual 6,382/17,083), and the
repository index was the wrong target for the "plan of record" and "run
tracker" rows.

**Two architecture documents, one wrong.** The graph-derived
`ARCHITECTURE.md` had **zero inbound references**, while the E02 run document
was cited as current despite four claims that are false in code. Verified by
import and symbol search:

| Declared boundary | Actual |
|-------------------|--------|
| `pkg/domain` imports stdlib and `decimal` only | imports `gen/hq/dto` (18 non-test edges) |
| `pkg/services` depends on `domain` plus interfaces | imports `client` (30 edges) |
| `pkg/transport` implements the service interfaces | `Adapter` has no consumer |
| Import rules are machine-enforced | `.golangci.yml` enables no `depguard` rule |

The run document is now marked superseded and carries that table. No code was
rewired: the v-next layer's fate is the open N1 product decision.

**A phantom defect was removed.** `next-phase.md` listed mojibake in four
files. An audit found zero U+FFFD and no Latin-1 mojibake sequences in any of
them. It was a console-encoding artifact that had been recorded as a finding.

**Tooling had drifted from CI.** `goreleaser-check` still ran
`go install …@latest`, the exact v1 dependency resolution that fails to
compile under Go 1.26 — the reason CI moved to `goreleaser-action@v6`. The
Makefile header still called proto codegen, the docs site, and the mock
Gateway unimplemented. `PYTHON ?= python3` fails on hosts that only ship
`python`. `fmt-check` carried a CRLF workaround that `.gitattributes` had made
obsolete, with a comment asserting the opposite of the current reality.

## 3. Verification

| Check | Result |
|-------|--------|
| `gofmt -l .` | clean outside `gen/` |
| `go build ./...` | pass, default and `-tags otel` |
| `go vet ./...` | pass, default and `-tags otel` |
| `go test ./... -count=1` | pass, 23 packages |
| `go test -race` | **not run locally** — no C toolchain; CI is authoritative, and `AGENTS.md` now says so |
| coverage gate | `pkg/domain` 93.8%, `internal/auth` 94.2%, `internal/transport` 99.0% |
| `check_money.py` | clean |
| `check_links.py` | 92 files, 0 unresolved |
| `check_i18n.py` | 6 languages consistent |
| `mkdocs build --strict` | exit 0, no unlisted pages |
| `gitnexus detect-changes` | 0 affected processes, risk low, before each commit batch |

## 4. Honest limitations

1. **The v0.1.7 CI run was never confirmed green.** The `gh` token is invalid
   on this host, so the release was tagged on local verification alone. Someone
   with working GitHub credentials must confirm the Actions run.
2. **v0.1.7 has a tag but no release artefacts.** `release.yml` runs
   `goreleaser check` on a tag, not `goreleaser release`, so no archives,
   checksums, or SBOM were published. Publishing is blocked on the Gitee-token
   and cosign-OIDC questions.
3. **The three new package-tree comments in the five translations are in
   English.** `check_i18n` validates structure, not comment language. A native
   speaker should polish them.
4. **Four GitNexus MCP server processes predate the current CLI** and fail
   every graph read with a storage-version mismatch. The CLI fallback works, so
   no work was blocked, but the `gitnexus_*` tools remain unusable until the
   server is restarted.
5. **The deviations table in the run document will go stale again** unless N1
   is decided. That is the single largest remaining structural issue.

## 5. What was deliberately not done

- **No `depguard` rule.** The boundary is undecided under N1; a rule could fail
  the build for code that decision may delete.
- **No v-next rewiring.** Still unreachable by a caller.
- **No coverage-gate widening.** Five released-surface packages sit below 85%
  (`pkg/hstong` 84.7%, `algo` 81.2%, `trade` 81.3%, `internal/push` 79.9%,
  `stream` 76.3%), and the work is partly wasted if N1 removes code.
- **No MCP server or editor-config change**, by decision.
- **No live Gateway run.** Needs a test account.

## 6. Verification commands

```sh
gofmt -l . | grep -v '^gen/'                        # empty
go build ./... && go build -tags otel ./...
go vet ./... && go vet -tags otel ./...
go test ./... -count=1
go run scripts/coverage_gate.go
python scripts/check_money.py
python scripts/check_links.py
python scripts/check_i18n.py
mkdocs build --strict
```

## 7. References

- Plan and tracker: `plan.md`, `todos.md`
- Current architecture map: `../../../ARCHITECTURE.md`
- Threat model and risk register: `../../threat-model.md`
- Predecessor run: `../2026-09-23-hstong-enterprise-sdk/`
