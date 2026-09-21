# P11 — Examples & Docs

- **Run:** 2026-09-21-hstong-full-surface · **Status:** done (gate passed)
- **Owner role:** docs
- **Depends on:** P09 · **Plan ref:** [plan.md](../plan.md) §P11 · **Tracker:** [todos.md](../todos.md)

## Objective / Exit criteria

Ship runnable examples for every major surface and a complete MkDocs Material site.

Exit criteria:
- Every `examples/*/main.go` compiles and has a `main()` + SPDX header.
- `mkdocs build --strict` passes; `docs/runs/` is excluded from the built site.
- `docs/LEGACY.md` documents the deprecated direct-to-platform protocol and states it is
  not implemented.
- All code snippets match the real exported API (source-checked).

## Tasks

| ID | Task | Role | Status | Deps | Acceptance | Verify | Evidence |
|----|------|------|--------|------|-----------|--------|----------|
| T33 | Runnable examples | docs | done | T29 | login/quote/order/stream/futures/algo compile | `go build ./examples/...` | `evidence/P11-T33-T34.txt` |
| T34 | MkDocs site + SPEC + LEGACY + ADR index | docs | done | T29 | `mkdocs build --strict` | `mkdocs build --strict` | `evidence/P11-T33-T34.txt` |

## Decisions & deviations

- `examples/mock-gateway` ships as a README (how to run `cmd/hstong-mock-gateway`)
  rather than a duplicate program.
- `scripts/mkdocs_hooks.py` was added to rewrite out-of-tree repository links
  (`../proto/**`, later `../test/**`) to GitHub URLs so strict link validation stays on.
- Verbatim code snippets in docs are source-checked but not compiled; only `examples/`
  programs are build-verified.

## Files created / modified

- `examples/**` (6 programs + README + `examples_test.go`)
- `mkdocs.yml`, `requirements-docs.txt`, `scripts/mkdocs_hooks.py`
- `docs/{index,getting-started,configuration,authentication,protocol,market-data,trading,futures,algo,streaming,mock-gateway,testing,observability,rate-limiting,security,errors,CONTRIBUTING,LEGACY}.md`

## Verification evidence

| # | Command | Result | Artifact |
|---|---------|--------|----------|
| 1 | `go build ./examples/...` | exit 0 | `evidence/P11-T33-T34.txt` |
| 2 | `go test ./examples/... -count=1` | ok | `evidence/P11-T33-T34.txt` |
| 3 | `python -c "yaml.safe_load(open('mkdocs.yml'))"` | OK | `evidence/P11-T33-T34.txt` |
| 4 | `mkdocs build --strict` | exit 0 (after the `../test/**` hook fix) | `evidence/P11-T33-T34.txt` |

## Blockers / follow-ups

- The `WithMetrics`/`WithRetryPolicy`/... internal-type limitation was fixed in P10's
  follow-up; the affected pages were updated.
- A `docs/testing.md` link to `../test/integration/README.md` broke strict mode; the
  hook was extended (orchestrator housekeeping) and strict passes.

## Gate

- Exit criteria met: **yes** — T33/T34 `done`; `mkdocs build --strict` exit 0.
- Next: **P12** integration tests (written, env-gated).
