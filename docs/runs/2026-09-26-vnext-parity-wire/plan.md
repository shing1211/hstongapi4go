# Plan: v-next Parity, Wiring, and v1.0

- **Run:** `2026-09-26-vnext-parity-wire`
- **Mode:** BUILD
- **Predecessor:** `../2026-09-25-hstong-agent-readiness/`
- **Baseline:** `efdea0d`, tag `v0.1.12` (first signed release, `cosign verify-blob` verified)

## Goal

Complete N1 Option A. The v-next layer currently covers roughly half the released
surface: `stream`, `algo`, and `future` have no v-next equivalent, and
`PushOrchestration` — named by ADR 0010 — was never implemented. N1's own analysis
called leaving both implementations indefinitely "not defensible", so Option A is
only finished once v-next reaches parity and is wired in.

## Scope

In scope: the 20 of 51 `docs/SPEC.md` endpoints with no v-next equivalent, testing
the existing services, wiring v-next behind an opt-in constructor, deprecating
`pkg/hstong/*` per ADR 0010/0011, and the documentation that ships with that.

Out of scope: `frontend`, `data`, and `security` roles. This is a Go library. The
Gitee artifact mirror is declined (no GoReleaser publisher exists; source and all
13 tags are already mirrored).

## Assumptions

1. **No Gateway test account exists** (confirmed 2026-09-26). G6 can never run in
   this run. Consequences are recorded in F1, not silently absorbed.
2. **Corrected 2026-09-26 by P1's evidence — the original assumption was false.**
   The Gateway uses **two different wire conventions**: `/trade/*` and `/account/*`
   send money as *quoted* strings (`"costPrice":"350.00"`), which is why 132 of 154
   `json`-tagged `pkg/domain` fields are `string`; the HQ market endpoints send
   *unquoted* numbers (`"spreadLevel":0.2`). A `string` field cannot decode an
   unquoted number — `encoding/json` raises `UnmarshalTypeError` — which is why
   those fields were `float64`, and why `float64` corrupted silently. `json.Number`
   is the only type that accepts both conventions: it preserves the Gateway's digits
   verbatim (a 19-digit value round-trips exactly, which `float64` cannot do) and
   *rejects* a non-numeric string that `string` would accept silently. Verified
   directly, not inferred. Any future service must use `json.Number` for numeric wire
   fields, not `string` and never `float64`.
3. `WithReadDeadline` stays **off by default**. R3 cannot close without an observed
   inter-frame gap, which requires G6.
4. `make` is unavailable on the dev host; `gosec`, `goreleaser`, `syft`, `cosign`
   and `gitleaks` are not installed. Verification uses direct commands; release
   pipeline steps run in `golang:1.26` via Docker.

## Approach

**Parity first, then wire, then document once.**

Chosen: build all missing services, flip a parity guard to enforcing, then wire
behind an opt-in constructor, then regenerate docs once.

Alternatives rejected:
- *Wire then parity* — publishes an incomplete public API and contradicts ADR 0010,
  whose deprecation path requires parity first.
- *Delete v-next, consolidate on the released stack* — reverses N1, discards ~4,000
  finished and tested lines. Remains the correct fallback if the schedule collapses.

## Order of work

1. **P1, P2** — the `float64` money defect and the guard gap that hid it
2. **A1 ∥ A2** — `stream` 85.6% and `trade` 86.3% to ≥90%
3. **B1 → B2 ∥ B3 → B4 → B5** — test `pkg/services`, then gate it
4. **C1, C2** — parity guard in report mode (must not break CI mid-run)
5. **C3–C6** futures · **C7–C9** algo · **C10–C12** push · **C13** auth composition
6. **C14** — flip the parity guard to enforcing
7. **D1–D4** — opt-in constructor, layering rules, e2e
8. **E1–E4** docs once · **F1, F2** record blocked items · **G1** release v1.0.0

## Verification

`make` is absent; use: `go build ./...`, `go vet ./...`, `gofmt -l .` (nothing
outside `gen/`), `go test -count=1 ./...`, `go test -race -count=1 ./...`
(`CC=C:\Users\Tchan\mingw64\bin\gcc.exe`, `CGO_ENABLED=1`,
`GOTMPDIR=C:\Users\Tchan\AppData\Local\Temp\opencode`), `go run
scripts/coverage_gate.go` **from a clean clone only**, `scripts/check_links.py`,
`check_i18n.py`, `check_money.py`, `mkdocs build --strict`.

## Risks

- **The price tick model is unresolved and now visible (P3).** Every price in
  `pkg/services` is constructed with a hardcoded tick of `"0.001"`, which is wrong
  for US and sub-cent instruments. P1 stopped rounding prices to 3 decimals, so a
  faithful `0.0005` inside a `Price` whose tick is `0.001` now fails validation
  loudly. That is the correct trade — a loud failure beats a silent wrong number —
  but it is a real defect that must be settled before `pkg/services` ships. Related:
  `OrderBookResponse.TickSize` is populated from a wire key named `spreadLevel`,
  which is a bid-ask spread rather than a tick, so the field name and the wire key
  disagree. `pkg/services` is **not** in ADR 0011's protected surface and has never
  shipped, so both are correctable now.
- **C11 `PushOrchestration` is the least predictable task** — no precedent in this
  layer. Treat its size estimate as soft.
- **C14 makes CI able to fail** on a SPEC/service mismatch. Intended, but requires
  C6, C9, C12, C13 all green first.
- **Coverage must be measured from a clean clone** (AGENTS.md). A dirty tree
  overstated `internal/push` by 2.8pp and produced a wrong published figure.
- **B2/B3 are the largest test tasks.** If they stall, report partial coverage
  rather than lowering the gate.
