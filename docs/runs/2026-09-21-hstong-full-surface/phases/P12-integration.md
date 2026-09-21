# P12 — Integration

- **Run:** 2026-09-21-hstong-full-surface · **Status:** done (tests written, NOT executed — no Gateway/credentials)
- **Owner role:** tester
- **Depends on:** P09 · **Plan ref:** [plan.md](../plan.md) §P12 · **Tracker:** [todos.md](../todos.md)

## Objective / Exit criteria

Write env-gated integration tests that confirm the wire assumptions the mock cannot
prove, and document exactly how to run them.

Exit criteria:
- Tests skip cleanly when `HSTONG_INTEGRATION` is unset; offline `go test ./...` stays green.
- The suite probes: market `int64` representation, `TradeQueryMaxAvailableAsset` nesting,
  `TradeQueryHoldsList` shape, streaming delivery, trade reads, and (opt-in) a single
  place+cancel mutation.
- Run instructions and the env matrix are documented.

**Not met here:** the live run. Confirming the assumptions requires the user to run the
suite against the real test Gateway (Mon-Fri 09:00-18:00).

## Tasks

| ID | Task | Role | Status | Deps | Acceptance | Verify | Evidence |
|----|------|------|--------|------|-----------|--------|----------|
| T35 | Integration tests + wire validation | tester | done (unexecuted) | T28 | skips by default; probe tests present | `go test ./test/integration/... -v` | `evidence/P12-T35.txt` |

## Decisions & deviations

- The mutation test is double-gated (`HSTONG_INTEGRATION=1` **and**
  `HSTONG_PLACE_ORDERS=1`) so it can never fire accidentally.
- Entitlement failures on market pulls are reported as skips with a clear message, not
  hard failures.

## Files created / modified

- `test/integration/doc.go`, `test/integration/integration_test.go`, `test/integration/README.md`
- `docs/testing.md` (integration section)

## Verification evidence

| # | Command | Result | Artifact |
|---|---------|--------|----------|
| 1 | `go test ./test/integration/... -count=1 -v` | 7 tests SKIP, 0 fail | `evidence/P12-T35.txt` |
| 2 | `go test -race -count=1 ./...` | green; integration skipped | `evidence/P12-T35.txt` |
| 3 | `go build ./...` / `go vet ./...` / `gofmt -l .` | clean | `evidence/P12-T35.txt` |

## Blockers / follow-ups

**User action required** to close the open wire assumptions:

```powershell
$env:HSTONG_INTEGRATION    = "1"
$env:HSTONG_TRADE_PASSWORD = "<trade password>"
go test ./test/integration/... -count=1 -v
```

Unconfirmed until then:
1. market `int64` is a JSON number (`basicQot[].volume`, `ticker[].volume`, `ticker[].timestamp`);
2. `TradeQueryMaxAvailableAsset` nests at `data.data` vs `data`;
3. `TradeQueryHoldsList` real shape (wrapped / bare / inline);
4. `/hq/Subscribe` + TCP push delivers a decoded `Event` within 10s;
5. which market pulls the account is entitled to;
6. mutation returns an entrust id with exactly one outbound attempt.

If (1) fails, apply the ADR 0007 fallback in `pkg/hstong/market/market.go` (switch the
affected endpoint to `client.ProtoJSON()`), or correct ADR 0007.

## Gate

- Exit criteria met: **partial** — deliverables and offline verification complete; the
  live confirmation is deferred to the user by design (no credentials in this session).
