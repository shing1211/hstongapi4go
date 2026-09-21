# P09 — Mock Gateway

- **Run:** 2026-09-21-hstong-full-surface · **Status:** done (gate passed)
- **Owner role:** devops · tester
- **Depends on:** P05, P07, P08 · **Plan ref:** [plan.md](../plan.md) §P9 · **Tracker:** [todos.md](../todos.md)

## Objective / Exit criteria

Build an in-repo mock of the HStong OpenAPI Gateway — HTTP `:11111` (51
endpoints) plus TCP push `:11112` emitting framed `PBNotify` messages — and an
end-to-end suite that drives the real SDK managers against it.

Exit criteria (plan §P9):
- 51 HTTP routes + push topics served.
- SDK <-> mock e2e for every endpoint.

## Tasks

| ID | Task | Role | Status | Deps | Acceptance | Verify | Evidence |
|----|------|------|--------|------|-----------|--------|----------|
| T28 | Mock HTTP(51) + TCP push server | devops | done | T20,T25,T27 | all 51 routes + push topics served | `go test ./test/mockgateway/...` | [evidence](../evidence/P09-T28-T29.txt) |
| T29 | All-endpoint e2e + coverage | tester | done | T28 | SDK <-> mock for every endpoint | `go test ./test/e2e/...` | [evidence](../evidence/P09-T28-T29.txt) |

## Deliverables

| Artifact | Purpose |
|----------|---------|
| `test/mockgateway/doc.go` | package scope: what is / is not mocked, how tests use it |
| `test/mockgateway/server.go` | `Server`, `New`, `WithHTTPAddr/WithPushAddr/WithFixtureSet/WithErrorInjection/WithPushTypeURL`, `Start`/`StartT`/`Close`, `HTTPBaseURL`/`PushAddr`, route table, error injection |
| `test/mockgateway/fixtures.go` | wire-level JSON for all 51 endpoints; cursor pagination (default 20, max 99) |
| `test/mockgateway/push.go` | TCP push server, `Emit`, `EmitQuote`/`EmitTicker`/`EmitOrderBook`/`EmitBroker`/`EmitTradeDeliver`/`EmitFuturesTradeDeliver` |
| `test/mockgateway/server_test.go` | self-test: 51 routes registered/respond, unknown 404, cursor paging, injection, subscriptions, raw push frame, idempotent close, `goleak` |
| `cmd/hstong-mock-gateway/main.go` | standalone binary `-http`/`-push`, prints bound addresses, runs until interrupted |
| `test/e2e/doc.go`, `test/e2e/e2e_test.go` | 51-endpoint table + session re-login + error mapping + push, `goleak` |
| `Makefile` | `mock-gateway` target implemented |

## Decisions & deviations

- **Push framing reuses `internal/push`.** `Emit` packs the payload with
  `anypb.New` and writes it through `push.WriteFrame`, so the mock and the SDK
  share one framing implementation and cannot drift.
- **Enum-based decoding is proven.** `WithPushTypeURL` overwrites the `Any`
  `type_url` with `type.googleapis.com/does.not.Exist`; the push e2e still
  receives typed events, proving the SDK dispatches on `notifyMsgType`.
- **Push broadcast, not per-topic filtering.** The mock tracks
  `/hq/Subscribe` topics and `/trade/TradeSubscribe`, and `Emit` broadcasts to
  every connected client; per-topic delivery is the SDK subscription's job. The
  tracking is asserted in `TestSubscriptionsRecorded`.
- **Pagination scope.** The six cursor list endpoints (`queryParamStr`:
  fund-jour real/history, entrust real/history, deliver real/history) are
  served page-by-page with default size 20 and a `< 100` cap (99). The
  page-number endpoints (conditional orders, futures history) return a fixed
  first page with the documented metadata, because the SDK's manager methods
  expose a single page rather than a walker.
- **Error injection is per-path** (`WithErrorInjection`, `Server.Inject`) with
  an optional finite `Times`; the envelope remains HTTP 200 with `ok:false` and
  `err:"<code> <message>"`, matching the Gateway.
- **No bodySHA1 signing.** The push header's signature field is left zero;
  verification is opt-in and off by default (ADR 0005).

## Findings (doc vs SDK) — not silently conformed

1. **Market codec.** `plan.md` §F3 / ADR 0002 describe market `data` as
   protojson over generated DTOs, but every `market.Manager` method calls
   `client.JSON()` (`encoding/json`) — see `pkg/hstong/market/market.go`. The
   generated DTOs carry `json` tags, so plain JSON round-trips; the mock emits
   plain JSON and the e2e decodes typed results. Recorded as a design/impl
   divergence for T35 to confirm against the real Gateway (either both codecs
   are accepted, or ADR 0002 should be corrected to JSON for market pulls).
2. **`/trade/TradeQueryMaxAvailableAsset` envelope.** The SDK's private
   `maxAvailableAssetResponse` expects the payload at `data.data`, so the fixture
   is `{"data":{...MaxAvailableAsset...}}`. If the real Gateway returns the asset
   object directly under `data`, the SDK decode would silently leave the result
   zero. Flagged for T35 real-Gateway validation; not changed here (pkg/** is
   read-only for this phase).
3. **`/trade/TradeQueryHoldsList` shape.** `HoldsListResponse.UnmarshalJSON`
   tolerates `{"holdsList":[...]}`, a bare array, or a single inline row; the
   docs describe the inline form, the mock uses the wrapped form. Both decode.

## Files created / modified

- `test/mockgateway/doc.go`, `server.go`, `fixtures.go`, `push.go`, `server_test.go`
- `test/e2e/doc.go`, `test/e2e/e2e_test.go`
- `cmd/hstong-mock-gateway/main.go`
- `Makefile` (`mock-gateway` target only)
- `docs/runs/2026-09-21-hstong-full-surface/phases/P09-mock-gateway.md`
- `docs/runs/2026-09-21-hstong-full-surface/evidence/P09-T28-T29.txt`

No existing SDK behavior was modified; `client/**`, `internal/**`, `pkg/**`,
`gen/**`, and `proto/**` are untouched.

## Verification evidence

| # | Command | Result | Artifact |
|---|---------|--------|----------|
| 1 | `gofmt -l .` | clean (no output) | [P09-T28-T29.txt](../evidence/P09-T28-T29.txt) |
| 2 | `go build ./...` | exit 0 | same |
| 3 | `go build ./cmd/...` | exit 0 | same |
| 4 | `go vet ./...` | exit 0 | same |
| 5 | `go test ./test/mockgateway/... -race -count=1 -v` | PASS; 51 route subtests | same |
| 6 | `go test ./test/e2e/... -race -count=1 -v` | PASS; 51 endpoint subtests | same |
| 7 | `go test -race -count=1 ./...` | all packages ok | same |
| 8 | `go test -count=1 -cover ./test/...` | mockgateway 78.6% | same |
| 9 | `python scripts/check_money.py` | OK | same |
| 10 | standalone binary smoke run | binds HTTP + push, prints addresses | same |

## Blockers / follow-ups

- None blocking. Follow-ups (T35): validate finding #1 (market codec) and
  finding #2 (`TradeQueryMaxAvailableAsset` envelope) against the real test
  Gateway.

## Gate

- Exit criteria met: **yes** — all 51 routes registered and served; the SDK <->
  mock e2e suite drives every endpoint, the session re-login, the push path
  (with a bogus `Any` type_url), and typed error mapping; `-race` suite green
  and `goleak` clean.
