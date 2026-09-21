# P01 — Protocol Core

- **Run:** 2026-09-21-hstong-full-surface · **Status:** done (gate passed)
- **Owner role:** data · backend · tester
- **Depends on:** P00
- **Plan ref:** [plan.md](../plan.md) §P1 · **Tracker:** [todos.md](../todos.md)

## Objective / Exit criteria

Turn the vendored protos and the documented wire protocol into the SDK core:
codegen, HTTP transport + envelope + codec dispatch, typed errors, client/options,
and the trade-password crypto — all covered by offline tests.

Exit criteria:
- `make proto-verify` equivalent reports no drift from committed `gen/`.
- Transport round-trips both codecs (protojson for proto-backed market data,
  `encoding/json` for hand-written bodies) against `httptest`.
- Status codes `0000`, `1001`-`1018`, `20033`, `40001`, `40002` mapped with
  `Retryable()` / `ReLoginRequired()` classification.
- Trade-password doc vector passes.
- `go test -race ./...` green; no goroutine leaks after `Close`.

## Tasks

| ID | Task | Role | Status | Deps | Acceptance | Verify | Evidence |
|----|------|------|--------|------|-----------|--------|----------|
| T04 | Protobuf codegen | data | done | T03 | 17 `.pb.go`; `proto-verify` no drift | `scripts/proto_verify.sh` | `evidence/P01-T04.txt` |
| T05 | HTTP transport + envelope + codec dispatch | backend | done | T04 | httptest round-trip both codecs | `go test ./internal/transport/...` | `evidence/P01-T05-T07.txt` |
| T06 | Client, options, env, alias routing | backend | done | T05 | 51 routes + 3 alias forms tested | `go test ./client/...` | `evidence/P01-T06.txt` |
| T07 | Error taxonomy + status codes | backend | done | T05 | codes mapped; `Retryable`/`ReLoginRequired` | `go test ./internal/errs/...` | `evidence/P01-T05-T07.txt` |
| T08 | AES-ECB trade-password crypto | backend | done | T02 | doc vector passes | `go test ./internal/crypto/...` | `evidence/P01-T08.txt` |
| T09 | Core tests + goleak | tester | done | T06,T07,T08 | client 100%, errs 100%, transport 99.1%; goleak clean | `go test -race ./...` | `evidence/P01-T09.txt` |

## Decisions & deviations

- **T05 and T07 are executed in one backend session.** They are tightly coupled
  (transport needs the error taxonomy to decode `ok=false` envelopes), and the
  plan's declared `T07 depends on T05` ordering would otherwise force a
  throwaway error layer. `internal/errs` is written first, then
  `internal/transport` consumes it.
- T04 pinned `protoc-gen-go` to the Makefile's `v1.36.6` (host had `v1.36.1`) so
  generated headers match CI and `proto-verify` does not report false drift.
- `buf lint` fails on upstream protos (no `package`, non-snake filenames) and is
  deliberately non-fatal in the `proto` target.

## Files created / modified

- T04: `buf.yaml`, `buf.gen.yaml`, `scripts/proto_verify.sh`, `gen/**` (17 files),
  `go.mod`/`go.sum` (`google.golang.org/protobuf v1.36.12`), `Makefile` (`proto`,
  `proto-verify` only)
- T08: `internal/crypto/doc.go`, `internal/crypto/aes.go`, `internal/crypto/aes_test.go`
- (pending T05/T06/T07/T09)

## Verification evidence

| # | Command | Result | Artifact |
|---|---------|--------|----------|
| 1 | `go build ./...` | exit 0 (incl. `gen/**`) | `evidence/P01-T04-T08.txt` |
| 2 | `go vet ./...` | exit 0 | `evidence/P01-T04-T08.txt` |
| 3 | `gofmt -l .` | empty | `evidence/P01-T04-T08.txt` |
| 4 | `scripts/proto_verify.sh` | `gen/ matches proto/ (no drift)` | `evidence/P01-T04-T08.txt` |
| 5 | second `buf generate` | no diff (`git diff --exit-code -- gen/`) | `evidence/P01-T04-T08.txt` |
| 6 | `go test ./internal/crypto/... -v` | all pass incl. doc vector | `evidence/P01-T04-T08.txt` |
| 7 | `python scripts/check_money.py` | OK | `evidence/P01-T04-T08.txt` |

## Blockers / follow-ups

- `make` remains unavailable on this Windows host; `scripts/proto_verify.sh` was
  exercised via Git Bash. CI covers the `make` path.
- `BUF_VERSION` in the `tools` target is pinned to `v1.47.2` while the host has
  `1.73.0`; both support the v2 config schema and buf version is not embedded in
  output, so no drift is expected. Revisit if CI installs a different buf.

## Gate

- Exit criteria met: **yes** — T04/T05/T06/T07/T08/T09 all `done` and independently verified.
  - `scripts/proto_verify.sh` -> `gen/ matches proto/ (no drift)` (2nd `buf generate` = 0 diff).
  - Transport round-trips both codecs; single-attempt (no-retry) and 64-goroutine
    race tests pass; `ok=false` yields typed `*errs.Error`.
  - Full status-code table mapped with `Retryable`/`ReLoginRequired`/`IsDeprecated`.
  - Crypto doc vector `"123456" -> W1U8iZIppSE+mBMtzy9vZQ==` passes.
  - `go test -race -count=1 ./...` green with `goleak` in client/errs/transport;
    coverage client 100%, errs 100%, transport 99.1%, crypto 87.5%.
  - 51 canonical routes + 3 alias forms exercised end-to-end.
- Open item carried to **P12/T35**: confirm against the real Gateway whether the
  status code arrives inside the `err` string or a dedicated field.
- Next: **P02 Session** (T10 + T11 merged into one backend session).
