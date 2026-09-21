# P00 — Foundation

- **Run:** 2026-09-21-hstong-full-surface · **Status:** done (gate passed)
- **Owner role:** architect · devops · data (parallel)
- **Depends on:** —
- **Plan ref:** [plan.md](../plan.md) §P0 · **Tracker:** [todos.md](../todos.md)

## Objective / Exit criteria

Establish truth (ADRs), a buildable/lintable repo skeleton, and the vendored
protocol source of truth (17 official `.proto` files + platform keys + enums).

Exit criteria:
- ADRs 0001-0005 written and consistent with `plan.md` (hybrid codec, Gateway
  transport, key model, minimal deps, numeric precision).
- `go build ./...`, `go vet ./...`, `gofmt -l .` run clean on the skeleton.
- All 17 protos vendored under `proto/` with provenance; dictionaries/enums
  enumerated into `pkg/types` (inventory) and `docs/SPEC.md` v1.

## Tasks

| ID | Task | Role | Status | Deps | Acceptance | Verify | Evidence |
|----|------|------|--------|------|-----------|--------|----------|
| T01 | ADRs + DESIGN + AGENTS | architect | done | — | 5 ADRs (0001-0005) + DESIGN.md + AGENTS.md; 0 dangling links | manual review | `evidence/P00-T01.txt` |
| T02 | Repo/CI scaffolding | devops | done | — | build/vet/gofmt clean; 18 Make targets; CI workflow; archives ignored | `go build ./...`, `go vet ./...`, `gofmt -l .` | `evidence/P00-T02.txt` |
| T03 | Vendor PB + keys + enums | data | done | T02 | 17 protos vendored byte-for-byte; provenance (sha256); dictionaries enumerated | tree check + `docs/SPEC.md` | `evidence/P00-T03.txt` |

## Decisions & deviations

- T01 does **not** create `doc.go` (moved to T02) so the module can be initialised
  once by the devops task without a file conflict.
- Plan v1 assumed uniform `protojson`; corrected to **hybrid codec** after finding
  that `华盛通OpenAPI-SDK-PB.zip` contains only 17 protos with no HTTP request/response
  messages (see `plan.md` §F2/F3).

## Files created / modified

- Run: `plan.md`, `todos.md`, `phases/P00-foundation.md`
- T01: `docs/adr/README.md`, `docs/adr/0001-gateway-transport.md`,
  `docs/adr/0002-hybrid-codec.md`, `docs/adr/0003-no-auto-retry-orders.md`,
  `docs/adr/0004-minimal-dependencies.md`,
  `docs/adr/0005-key-model-and-push-verification.md`, `docs/DESIGN.md`, `AGENTS.md`
- T02: `go.mod`, `doc.go`, `Makefile`, `.golangci.yml`, `.editorconfig`,
  `.gitignore` (modified), `NOTICE`, `DISCLAIMER.md`, `THIRD_PARTY_NOTICES.md`,
  `scripts/check_money.py`, `.github/workflows/ci.yml`
- Orchestrator: `Makefile` line 75 reference corrected (`ADR 0008` -> `docs/DESIGN.md §7`)

## Verification evidence

| # | Command | Result | Artifact |
|---|---------|--------|----------|
| 1 | `go build ./...` | exit 0 | `evidence/P00-build.txt` |
| 2 | `go vet ./...` | exit 0 | `evidence/P00-build.txt` |
| 3 | `gofmt -l .` | empty (clean) | `evidence/P00-build.txt` |
| 4 | `go test ./...` | no test files (expected at P00) | `evidence/P00-build.txt` |
| 5 | `python scripts/check_money.py` | `money-check OK: pkg/ does not exist yet` | `evidence/P00-build.txt` |
| 6 | `golangci-lint run ./...` | 0 issues (v2.9.0) | `evidence/P00-lint.txt` |
| 7 | `git status --short` | vendor archives absent (ignored) | `evidence/P00-build.txt` |
| 8 | scope review | T01 touched only `docs/adr/**`, `docs/DESIGN.md`, `AGENTS.md`; T02 touched only tooling/module metadata; T03 touched only `proto/**`, `docs/SPEC.md`, `pkg/types/**`; no overlap | this file |
| 9 | proto inventory | exactly 17 `.proto`; 0 legacy protos; `proto/` not gitignored | `evidence/P00-T03.txt` |

## Blockers / follow-ups

- `make` is not available on this Windows host (no GNU make; WSL stub broken).
  Makefile delivered and its recipe lines verified by inspection; a `make` smoke
  run on Linux/Git Bash is recommended before release (T37 pre-flight).
- `.golangci.yml` uses the v2 schema; confirm it loads in CI (the ubuntu job runs it).
- FOLLOW-UP (P13/T36): `scripts/check_money.py` docstring still cites `ADR 0008`;
  re-point to `docs/DESIGN.md §7` during the docs sweep.
- FOLLOW-UP (T03): vendor ONLY the 17 protos from `华盛通OpenAPI-SDK-PB.zip`; the
  Python SDK archive additionally contains legacy direct-protocol protos
  (`Request.proto`, `Response.proto`, `RequestMsgType`, `ResponseMsgType`) which
  are out of scope.

## Gate

- Exit criteria met: **yes** — T01/T02/T03 all `done` and independently verified.
  - ADRs 0001-0005 + DESIGN.md + AGENTS.md written (0 dangling links).
  - `go build ./...`, `go vet ./...`, `gofmt -l .`, `golangci-lint run ./...`,
    `python scripts/check_money.py` all clean on the skeleton.
  - Exactly 17 official protos vendored with SHA-256 provenance; legacy protos
    excluded; `docs/SPEC.md` v1 published; `pkg/types` enum + key inventory added.
- Next: **P01 Protocol Core** (T04 + T08 dispatched in parallel).
