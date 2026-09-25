# Close-Out Report — hstongapi4go Enterprise Gateway SDK (v-next)

- **Run:** `2026-09-23-hstong-enterprise-sdk`
- **Task:** E22
- **Mode:** BUILD
- **Status:** complete, 22/22
- **Baseline:** `be22ebe` (v0.1.5) · **Close-out commits:** `19591bc` (CI-repair evidence), `bc4d1a4` (this report and `next-phase.md`)
- **Deliverable release:** v0.1.6 (`c8b6084`)
- **Post-close-out commits:** `d28310d` graph-derived `ARCHITECTURE.md`, `eb00cbd` GitNexus agent wiring, `b30b0bf` `ARCHITECTURE.md` regenerated from a refreshed index. These landed after close-out and are tracked in `../2026-09-25-hstong-agent-readiness/`.

## 1. Outcome

Twenty-two planned tasks delivered across four phases, followed by a CI repair
that was not in the plan and turned out to be necessary.

| Phase | Tasks | Result |
|-------|-------|--------|
| P00 Foundation & Domain | E01–E05 | complete |
| P01 REST Services | E06–E10 | complete |
| P02 Push Engine | E11–E14 | complete |
| P03 DevOps & Hardening | E15–E22 | complete |

89 files changed, +9,710 / −120, across 36 commits.

## 2. What shipped

**Architecture.** ADRs 0008–0011 record four decisions: decimal-backed financial
types, OpenTelemetry behind a build tag, an additive v-next layering, and the
v0.1.x compatibility guarantee. The v-next packages are `pkg/domain`,
`pkg/services`, `pkg/transport`, and `internal/auth`, with an import-boundary
matrix in `ARCHITECTURE.md`.

**Domain.** `Money`, `Price`, `Quantity`, and `Rate` on `shopspring/decimal`;
typed identifiers; HK market models with lot and tick schedules; an order-state
transition table; and push-event types.

**Services.** Nine market pull calls, five account calls, and thirteen trading
calls with HK validation for lot size, tick, session, order type, time-in-force,
and permission.

**Push.** A manager with reconnect and resubscribe, typed normalizers that
tolerate unknown message types, and bounded fan-out with dedup and freshness.

**Operations.** OpenTelemetry instrumentation, a coverage gate at 85%, GoReleaser
configuration, SPDX and SBOM tooling, and a six-language documentation set.

**Security.** A threat model covering five assets, three trust boundaries, six
adversaries, and all seven E17 scenarios, closing with a 14-entry risk register
that separates what was fixed from what is accepted or still open.

## 3. Defects found and fixed

E17 was specified as adversarial testing. Reviewing the seven named scenarios
showed five were not merely untested but broken. Seven defects were confirmed in
source and fixed, each with a test verified to fail against the pre-fix code.

| ID | Defect | Layer | Evidence |
|----|--------|-------|----------|
| F1 | A non-2xx body was embedded in the error verbatim, so an echoing Gateway could push a password into the error and from there into a trace | Active | The function's own comment claimed it "never returns credentials" |
| F2 | `IsMutation` did an exact lookup, so `/trade/TradeEntrustRequest` classified as a retryable query — an alias could bypass the ADR 0003 single-attempt guarantee | Active | All three alias forms of all twelve mutation paths now tested |
| F3 | Cursor walks had no stalled-cursor guard, page cap, or page-size clamp | Forward | — |
| F4 | `maxRetries == 0` meant infinite, and the dialer received an empty address after a read failure | Forward | **53 dials where 11 were expected** |
| F5 | `accountid` and `fundAccount` were not redacted, contradicting ADR 0009 | Active | — |
| F6 | `EndSpan` recorded raw `err.Error()` into spans | Active | — |
| F7 | `FreshnessMonitor.Record` returned early on `seq == 0`, and `seqOf` always yields 0, so staleness detection never populated at all | Forward | Both freshness tests failed pre-fix |

Two more real defects surfaced while repairing CI: `Quantity.ValidateLot`
wrapped a `uint64` lot above `MaxInt64` into a negative modulus, and
`client/hardening_test.go` contained an empty branch that asserted nothing.

## 4. The CI repair

The most consequential finding of the run was not in the plan. **CI had been red
on `main` since at least E15, and v0.1.6 was tagged and pushed while red.**
Closing out on that baseline would have been a false attestation, so the pipeline
was repaired first.

Root causes, none of which had ever been caught because a single early failure
masked the rest:

1. A gofmt gate failing on three files, which skipped vet, tests, race, and the
   money check on every run since E15.
2. 39 golangci-lint findings.
3. The security job installed `honnef.co/go/tools/cmd/gosec` — staticcheck's
   module, which does not exist — so the step failed and `govulncheck` never ran.
4. `.goreleaser.yaml` did not parse, and separately set `main: .` on a library
   root and forced the `otel` tag on every build.
5. `govulncheck` reported 14 standard-library vulnerabilities, all fixed in
   go1.26.6.
6. buf normalises comments differently for CRLF and LF input, so the committed
   `gen/` produced on a Windows checkout did not match a Linux regeneration and
   `proto-verify` could never pass on CI.

The last one was fixed at the root with an `.gitattributes` change forcing LF
checkouts, which also removed a long-standing local trap: on a CRLF checkout
`gofmt -l .` listed every file, making a local gofmt check meaningless.

**Result: GitHub Actions run 36025226892, 9 jobs, 0 failures.**

Full detail, including the method used to get a trustworthy signal from a
CRLF working tree, is in `evidence/P03-CI-repair.txt`.

## 5. Verification

| Check | Result |
|-------|--------|
| `gofmt -l .` | clean, and now meaningful on Windows |
| `go build ./...` | pass, default and `-tags otel` |
| `go vet ./...` | pass, default and `-tags otel` |
| `go test ./... -count=1` | pass, 24 packages, both tag sets |
| `go test -race` | CI only; no C toolchain on the development host |
| `golangci-lint run` | exit 0, from 39 findings |
| `gosec -exclude-generated ./...` | 0 issues, from 23 outside generated code |
| `govulncheck ./...` | no vulnerabilities, from 14 |
| `goreleaser check` | valid, from an unparseable config |
| `goreleaser build --snapshot` | all six targets build |
| `buf generate` | matches `gen/` byte-for-byte, from a one-line drift |
| `make money-check` | no float money fields |
| `addlicense -check` | pass |
| `check_links.py` | 81 files, 0 unresolved |
| `check_i18n.py` | 6 languages consistent |
| `mkdocs build --strict` | exit 0 |

## 6. Honest limitations

1. **The v-next layer is not reachable by a caller.** Nothing in the released API
   imports it; only the coverage gate does. Three of the seven E17 fixes protect
   only the v-next path, and the threat model tags every mitigation **Active** or
   **Forward-looking** for this reason. This is the most important item for the
   next phase.
2. **The Release workflow never publishes.** It runs `goreleaser check` on a tag,
   not `goreleaser release`, so v0.1.6 produced a tag and no artefacts. Changing
   it writes to a public repository and needs the credential story settled, so it
   was left as a decision rather than changed silently.
3. **No live Gateway validation has ever run.** `test/integration` is env-gated
   and has never executed, so the ADR 0007 wire assumptions remain inferred from
   the vendored vendor SDKs rather than observed.
4. **Three push implementations coexist** and disagree about reconnect, backoff,
   and freshness.
5. **Push read deadlines are unimplemented**, so a half-open connection can block
   a goroutine indefinitely.
6. **Accepted by protocol, not fixed:** no replay defence, because the wire
   protocol defines no nonce and no idempotency key; and inert dedup and gap
   detection, because the Gateway sends no per-event sequence.

## 7. Deliverables

| Artifact | Contents |
|----------|----------|
| `plan.md` | The four-phase plan and its assumptions |
| `todos.md` | Run tracker, 22/22, with a dated log |
| `ARCHITECTURE.md` | v-next layer map and import-boundary matrix |
| `next-phase.md` | Seven ranked candidates with a recommended order |
| `report.md` | This document |
| `evidence/P03-E17-threat-model.txt` | Seven defect write-ups, pre-fix failure measurements, adversarial test inventory |
| `evidence/P03-CI-repair.txt` | Six CI root causes with reproduction and the method for each |
| `docs/threat-model.md` | Assets, boundaries, adversaries, seven scenarios, 14-entry risk register |
| `docs/adr/0008`–`0011` | Decimal money, OpenTelemetry, v-next layering, v0.1.x compatibility |

## 8. Verification commands

```sh
gofmt -l . | grep -v '^gen/'      # empty
go build ./... && go build -tags otel ./...
go vet ./... && go vet -tags otel ./...
go test ./... -count=1 && go test -tags otel ./... -count=1
golangci-lint run ./...
gosec -exclude-generated ./...
govulncheck ./...
goreleaser check --config .goreleaser.yaml
make money-check
python scripts/check_links.py
python scripts/check_i18n.py
mkdocs build --strict
```

## 9. References

- Plan and tracker: `plan.md`, `todos.md`
- Evidence: `evidence/P03-E17-threat-model.txt`, `evidence/P03-CI-repair.txt`
- Threat model and risk register: `../../threat-model.md`
- Architecture decisions: `../../adr/README.md` (0008–0011)
- Compatibility: `../../COMPAT.md`
- Release checklist: `../../RELEASE_CHECKLIST.md`
- Prior run: `../2026-09-21-hstong-full-surface/`
