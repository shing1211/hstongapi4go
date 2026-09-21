# P02 — Session

- **Run:** 2026-09-21-hstong-full-surface · **Status:** done (gate passed)
- **Owner role:** backend · tester
- **Depends on:** P01
- **Plan ref:** [plan.md](../plan.md) §P2 · **Tracker:** [todos.md](../todos.md)

## Objective / Exit criteria

Implement the trading session: login/logout via the Gateway, automatic re-login
driven by the error taxonomy, and a leak-free keep-alive poll that extends the
3-hour token.

Exit criteria:
- `Login` encrypts the configured trade password (`internal/crypto`) and posts
  `/trade/TradeLogin`; `Logout` posts `/trade/TradeLogout`.
- `EnsureLoggedIn` is serialized: N concurrent callers produce **one** login request.
- `ReLogin` fires exactly once when the cause is `1012`/`1013`/`1014`/`20033`.
- Keep-alive issues periodic requests and stops cleanly on context cancel /
  `StopKeepAlive`; no goroutine leaks; race-clean.
- The trade password never appears in errors or logs.

## Tasks

| ID | Task | Role | Status | Deps | Acceptance | Verify | Evidence |
|----|------|------|--------|------|-----------|--------|----------|
| T10 | Login/Logout + keep-alive + re-login | backend | doing | T09 | re-login on 1012/1013/1014; single-flight login | `go test ./pkg/hstong/... ./internal/session/...` | — |
| T11 | Session tests | tester | doing | T10 | leak-free; re-login + concurrency covered | `go test -race ./...` | — |

## Decisions & deviations

- **T10 and T11 are executed in one backend session** (implementation + tests),
  matching the T05+T07 treatment; the plan separated them but the test surface is
  inseparable from the state machine.
- Session lives in the root `pkg/hstong` package (`pkg/hstong/session.go`) with a
  pure, I/O-free state machine in `internal/session`. Domain managers will live in
  subpackages (`market/`, `trade/`, `future/`, `algo/`, `stream/`); session is
  cross-cutting because market permissions also require a successful trade login.
- Keep-alive default route: `/trade/TradeQueryMarginFundInfo` (a cheap read that
  proves the session is alive); interval configurable for tests.

## Files created / modified

- `internal/session/**`, `pkg/hstong/**` (pending T10/T11)

## Verification evidence

| # | Command | Result | Artifact |
|---|---------|--------|----------|
| 1 | `go build ./...` / `go vet ./...` | exit 0 | `evidence/P02-T10-T11.txt` |
| 2 | `gofmt -l .` | clean | `evidence/P02-T10-T11.txt` |
| 3 | `go test -race -count=1 ./...` | all ok | `evidence/P02-T10-T11.txt` |
| 4 | coverage | session 96.2%, hstong 84.7% | `evidence/P02-T10-T11.txt` |
| 5 | 32 concurrent `EnsureLoggedIn` | exactly 1 `/trade/TradeLogin` | `evidence/P02-T10-T11.txt` |
| 6 | keep-alive Stop + ctx cancel | request count halts; goleak clean | `evidence/P02-T10-T11.txt` |
| 7 | password safety | encrypted on the wire; never logged | `evidence/P02-T10-T11.txt` |

## Blockers / follow-ups

- The exact behavior of the token refresh through the Gateway (does a read call
  extend it?) must be confirmed in **P12/T35**; the keep-alive is a best-effort
  poll with a conservative interval.

## Gate

- Exit criteria met: **yes** — T10/T11 `done` and independently verified.
  - `Login` posts the AES-encrypted password to `/trade/TradeLogin`;
    `Logout` is idempotent.
  - `EnsureLoggedIn` is single-flight (32 callers -> 1 request).
  - `ReLogin` handled only for `1012`/`1013`/`1014`/`20033`.
  - Keep-alive is leak-free and stops on `StopKeepAlive` / ctx cancel; race-clean.
  - Password never appears in errors or logs.
- Next: **P03 Market Pull**, **P05 Trade**, **P07 Futures**, **P08 Algo**
  (dispatched as wave 1, four parallel backend sessions).
