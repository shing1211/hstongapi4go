# Plan: hstongapi4go — Full-Surface Go SDK

- **Run:** `2026-09-21-hstong-full-surface`
- **Mode:** BUILD
- **Status:** implementation complete; T37 release pending explicit approval
- **Repo:** `github.com/shing1211/hstongapi4go`
- **Base commit:** `461f219` (Initial commit; tree contained only `README.md`, `LICENSE`, `.gitignore`, `.gitattributes`)
- **Local:** Go 1.26.8 windows/amd64 · GOPATH `C:\Users\Tchan\go`
- **VCS:** `origin` -> github.com/shing1211/hstongapi4go · `gitee` -> gitee.com/shing1211/hstongapi4go
- **Upstream docs:** https://quant-open.hstong.com/api-docs/ (current) · https://quant-open.hstong.com/api-docs/old/ (legacy)

## Goal

Deliver a complete, idiomatic Go SDK for the HStong (华盛) Quant OpenAPI **Gateway**
interface: typed market-data, trading, futures, algo, and real-time push surfaces;
a mock Gateway for offline tests; env-gated integration tests against the real test
environment; six-language READMEs; a MkDocs Material docs site; and a v0.1.0
release pushed to GitHub and Gitee.

## Scope

**In scope — 51 HTTP endpoints + 11 market push topics + trade/futures push:**

- Protocol core: HTTP-POST envelope, hybrid codec (protojson + encoding/json),
  route aliases, typed errors.
- Market data: 9 pull + subscribe/unsubscribe (2) + TCP push (quote, order book,
  tick, broker).
- Trading: session (2), assets/positions (5), orders (13), push subscribe (2).
- Futures: 11 endpoints + push.
- Algo/strategy: 7 endpoints.
- Mock Gateway (HTTP + TCP push), hardening, docs, translations, release.

**Out of scope (explicit):**

- The **legacy direct-to-platform protocol** (`/hs/v2/login`,
  `/hs/config/queryServer`, direct 151-byte socket with developer RSA signing,
  AES-ECB dynamic key, heartbeat, device binding). Documented in
  `docs/LEGACY.md` as *not implemented*. It is the deprecated access method.
- Any UI, account-opening, device-binding tooling, or Gateway redistribution.

## Findings that shaped the plan

### F1 — Protocol surface (current docs)

- Local **OpenAPI Gateway**: HTTP POST only, `http://127.0.0.1:11111`; TCP push
  `127.0.0.1:11112`. No HTTPS/GET.
- Route aliases: `/X`, `/XRequest`, `/XRequestMsgType`.
- Request `{"timeout_sec":10,"params":{...}}`; response `{"ok":true,"err":"","data":{...}}`.
- Push frame: 151-byte header (magic `HS`, msgType=3, protoFmt=0, protoVer,
  serialNo int32 LE, bodyLen int32 LE, 128-byte `bodySHA1`, compress=0, 8B
  reserved) + `PBNotify{notifyMsgType, notifyId, notifyTime, payload Any}`.
- TradeLogin password: `Base64(AES-ECB/PKCS7(pwd, key=Base64Decode("m+qS04/2CH1OweCnmXZ3TDZkCQS+hBzY")))`.

### F2 — Official protobuf package (PB v2.2.0) is PARTIAL

Vendored archive `华盛通OpenAPI-SDK-PB.zip` (already in repo root) contains exactly
**17 `.proto` files** and **no HTTP request/response messages**:

| Group | Files |
|-------|-------|
| common | `common/constant/NotifyMsgType.proto`, `common/msg/HeartBeat.proto`, `common/msg/Notify.proto` (`PBNotify`) |
| market DTO | `hq/dto/Security`, `BasicQot`, `FutureBasicQotExData`, `OptionBasicQotExData`, `Broker`, `OrderBook`, `Ticker`, `TimeShare`, `KLine` |
| market notify | `hq/notify/BasicQotNotify`, `BrokerNotify`, `OrderBookFullNotify`, `TickerNotify` |
| trade notify | `trade/notify/TradeStockDeliverNotify` |

`NotifyMsgType` values: `TrsStockDeliverMsgType=0`, `TradeStockDeliverMsgType=1`,
`FuturesTradeStockDeliverMsgType=2`, `OrderBookNotifyMsgType=20001`,
`BrokerQueueNotifyMsgType=20002`, `BasicQotNotifyMsgType=20003`,
`TickerNotifyMsgType=20004`.

No `go_package` option is declared -> must be supplied via buf managed mode / `M` flags.
`java_package` is `com.huasheng.quant.open.sdk.protobuf.*`.

**Consequence:** the wire types for trade/futures/algo HTTP bodies are **not**
available as proto. We do not invent `.proto` files for them.

### F3 — Hybrid codec (corrected approach)

| Surface | Representation | Codec |
|---------|----------------|-------|
| TCP push payloads | generated proto types; `proto.Unmarshal` after `Any` unpack | binary protobuf |
| Market DTOs in HTTP `data` | generated proto types | `protojson` (int64-as-string, doubles correct) |
| Trade / futures / algo / assets / session HTTP bodies | hand-written structs, explicit `json:"..."` tags, money & qty as `string` | `encoding/json` |
| Envelope (`timeout_sec`/`params`/`ok`/`err`/`data`) | hand-written | `encoding/json` with `json.RawMessage` payloads |

This supersedes "approach A" (uniform protojson): protojson cannot encode
hand-written structs, and inventing protos for undocumented bodies would create
drift. Recorded as ADR 0002.

### F4 — Legacy doc harvested for constants

- Platform RSA public keys (test + prod) for opt-in `bodySHA1` SHA1WithRSA verification.
- Full status-code table `0000`, `1001-1018`, `20033`, `40001/40002`.
- Enum + data-dictionary inventory (entrustProp, entrustStatus, exchangeType,
  entrustBs/Type, exchange, realStatus, realType, stockType, dataType, moneyType,
  countryCode, cycType, ExRightFlag, Direction).
- Limits: pagination cursor `queryParamStr`, page size default 20 / max <100;
  ticker `limit` <= 100; condition `validDays` <= 100; batch-cancel may time out.
- Test env Mon-Fri 09:00-18:00; token 3h extended by calls; device binding
  mandatory (disabled in test); no documented QPS quota.
- Market push `topicId`s: 11/35 (quote), 14/27/28/37 (tick), 16 (broker), 17/25/26/36 (order book).
- Deprecated fields to mark in types: `holdsBalance`, `marketValue`, `lastPrice`,
  `incomeBalance`, `marketValueRate`, `incomeRatio`.

### F5 — Repo already contains downloaded vendor archives

`华盛通OpenAPI-SDK-{PB,Java,Python,Cpp}.zip` and Gateway installers
(`*.dmg`, `*.zip`, `*.tar.gz`, `*.exe`) are present in the repo root and untracked.
These must be git-ignored; only the extracted `proto/**` is committed.

## Assumptions

1. The Gateway is installed and running by the user; the SDK never bundles or launches it.
2. `protojson` is used only for proto-backed market `data`; validated in P12, with a
   per-endpoint `Codec` fallback.
3. Official protobuf package **v2.2.0** is the type source of truth; Gateway
   **v2.4.1** may drift -> `make proto-verify` + version record.
4. Order mutations are never auto-retried.
5. Money/quantities are `string` / `json.Number`, never `float64`.
6. Test environment is available Mon-Fri 09:00-18:00; device binding disabled there.
7. Platform public keys are public reference data (not secrets) and user-overridable.

## Approach — alternatives

| # | Approach | Decision |
|---|----------|----------|
| A | Uniform protojson over generated types only | Rejected: the PB package lacks all HTTP bodies (F2) |
| B | Fully hand-written structs, no codegen | Rejected: loses the authoritative push/DTO types and the `Any` registry |
| C | **Hybrid** — generated protos for push + market DTOs; hand-written structs for the rest | **PICK** (F3) |

**Transport decision:** target the local Gateway. Mirrors `futuapi4go <-> OpenD`.

## Architecture

```
Application
  +-- client/Client            (New, options, env, Codec selection, Close)
        +-- market/            (BasicQot, OrderBook, KL, TimeShare, Ticker, Broker, OptionChain, Overnight, Subscribe)
        +-- trade/             (Session, Assets, Orders, Push subscribe)
        +-- future/            (11 Futures* endpoints)
        +-- algo/              (7 Algo* endpoints)
        +-- stream/            (market + trade + futures push subscriptions)
              +-- internal/push   (TCP dial, 151B framing, PBNotify, Any unpack, reconnect+resubscribe)
        +-- internal/transport    (HTTP POST, envelope, codec dispatch, error mapping)
              +-- OpenAPI Gateway
                    +-- HTTP 127.0.0.1:11111
                    +-- TCP  127.0.0.1:11112
```

**Package layout**

```
proto/                     vendored .proto (PB v2.2.0) + provenance
gen/hstong/...             generated Go protobuf (committed; never hand-edited)
client/                    core: options, env, codec, alias routing, Close
internal/transport/        HTTP executor + envelope + codec dispatch
internal/push/             151-byte framing, PBNotify, Any unpack, reconnect
internal/crypto/           AES-ECB/PKCS7 trade-password encryption
internal/errs/             typed errors + status-code table
internal/resilience/       rate limit, retry budgets, circuit breaker
internal/logging/          slog + redaction
internal/metrics/          dependency-free, OTel-bridgeable
pkg/hstong/                market, trade, future, algo, stream (public managers)
pkg/types/                 enums + domain types (from dictionaries)
test/mockgateway/          mock HTTP(51) + TCP push; cmd/hstong-mock-gateway
examples/  scripts/  docs/  .github/workflows/
```

## Task breakdown (40 tasks)

> Columns: **ID · Objective · Role · Deps · Outputs · Acceptance / Verify · Size**

### P0 — Foundation
| ID | Objective | Role | Deps | Outputs | Acceptance / Verify | Size |
|----|-----------|------|------|---------|---------------------|------|
| T01 | Record design decisions | architect | — | `docs/adr/0001..0005`, `DESIGN.md`, `AGENTS.md` | ADRs reviewed; ADR 0002 = hybrid codec; ADR 0005 = Gateway transport + key model + opt-in push verify | S |
| T02 | Repo/CI scaffolding | devops | — | `go.mod`, `doc.go`, `Makefile`, `.golangci.yml`, `.editorconfig`, `.gitignore`, `LICENSE/NOTICE/DISCLAIMER`, `scripts/*`, `.github/workflows/*` | `go build ./...` ok; lint + SPDX + money targets exist; vendor archives ignored | S |
| T03 | Vendor PB + bundle keys + extract enums | data | T02 | `proto/**`, `docs/SPEC.md` v1, `PROVENANCE.md`, `pkg/types` enum inventory, platform key constants | 17 protos vendored; provenance + versions recorded; dictionaries enumerated | M |

### P1 — Protocol core
| ID | Objective | Role | Deps | Outputs | Acceptance / Verify | Size |
|----|-----------|------|------|---------|---------------------|------|
| T04 | Protobuf codegen | data | T03 | `buf.yaml`, `buf.gen.yaml`, `gen/hstong/**`, `make proto`/`proto-verify` | generated code compiles; `make proto-verify` clean | M |
| T05 | HTTP transport + envelope + codec dispatch | backend | T04 | `internal/transport/**` | httptest round-trip (both codecs); timeout/ctx honored | M |
| T06 | Client, options, env, alias routing | backend | T05 | `client/**` | alias resolution test; `HSTONG_*` env | M |
| T07 | Error taxonomy + status codes | backend | T05 | `internal/errs/**` | `0000/1001-1018/20033/40001/40002` mapped; `Retryable()`/`ReLoginRequired()` | S |
| T08 | AES-ECB trade-password crypto | backend | T02 | `internal/crypto/**` | doc vector `123456 -> W1U8iZIppSE+mBMtzy9vZQ==` | S |
| T09 | Core tests + goleak | tester | T06,T07,T08 | tests | `go test -race ./...` green; no leaks after `Close` | M |

### P2 — Session
| ID | Objective | Role | Deps | Outputs | Acceptance / Verify | Size |
|----|-----------|------|------|---------|---------------------|------|
| T10 | Login/Logout + keep-alive + auto re-login | backend | T09 | `internal/session`, `pkg/hstong/session.go` | re-login on 1012/1013/1014; poll helper; single-flight | M |
| T11 | Session tests | tester | T10 | tests | leak-free; re-login path covered | S |

### P3 — Market pull
| ID | Objective | Role | Deps | Outputs | Acceptance / Verify | Size |
|----|-----------|------|------|---------|---------------------|------|
| T12 | 9 market pull managers | backend | T09 | `pkg/hstong/market/**` | typed req/resp for all 9 endpoints | L |
| T13 | Market pull tests + fixtures | tester | T12 | tests | fixture round-trips; `limit <= 100` guard | M |

### P4 — Push + market stream
| ID | Objective | Role | Deps | Outputs | Acceptance / Verify | Size |
|----|-----------|------|------|---------|---------------------|------|
| T14 | TCP push client (framing, Any, reconnect) | backend | T09 | `internal/push/**` | 151B header parse; `Any` unpack; reconnect + resubscribe | L |
| T15 | Subscribe/Unsubscribe + topic enum | backend | T14 | `pkg/hstong/market/subscribe.go` | topics 11,14,16,17,25,26,27,28,35,36,37 | M |
| T16 | Stream channel API | backend | T15 | `pkg/hstong/stream/**` | `Updates()`/`Errors()`/cancel | M |
| T17 | Push tests (golden frames) | tester | T16 | tests | local TCP server; leak-free | M |

### P5 — Trade
| ID | Objective | Role | Deps | Outputs | Acceptance / Verify | Size |
|----|-----------|------|------|---------|---------------------|------|
| T18 | Assets/positions managers (5) + enums | backend | T09 | `pkg/hstong/trade/assets.go` | 5 endpoints; cursor pagination helper | M |
| T19 | Order managers (13) | backend | T18 | `pkg/hstong/trade/orders.go` | no auto-retry on mutations; `validDays <= 100` | L |
| T20 | Trade tests + single-attempt assertion | tester | T19 | tests | mutation = 1 outbound request | M |

### P6 — Trade push
| ID | Objective | Role | Deps | Outputs | Acceptance / Verify | Size |
|----|-----------|------|------|---------|---------------------|------|
| T21 | Trade push subscribe + decoders | backend | T16,T19 | `pkg/hstong/stream/trade.go` | `TradeStockDeliverNotify` decoded; init-auto-subscribe noted | M |
| T22 | Trade push tests | tester | T21 | tests | scenarios: 下单/改单/撤单/成交 | S |

### P7 — Futures
| ID | Objective | Role | Deps | Outputs | Acceptance / Verify | Size |
|----|-----------|------|------|---------|---------------------|------|
| T23 | 11 futures endpoints | backend | T09 | `pkg/hstong/future/**` | typed req/resp for all 11 | L |
| T24 | Futures push | backend | T23 | `pkg/hstong/stream/future.go` | `FuturesTradeStockDeliver` notify | S |
| T25 | Futures tests | tester | T23,T24 | tests | fixtures pass | S |

### P8 — Algo
| ID | Objective | Role | Deps | Outputs | Acceptance / Verify | Size |
|----|-----------|------|------|---------|---------------------|------|
| T26 | 7 algo endpoints | backend | T09 | `pkg/hstong/algo/**` | master/sub-order query + operate | M |
| T27 | Algo tests | tester | T26 | tests | fixtures pass | S |

### P9 — Mock gateway
| ID | Objective | Role | Deps | Outputs | Acceptance / Verify | Size |
|----|-----------|------|------|---------|---------------------|------|
| T28 | Mock HTTP(51) + TCP push server | devops | T20,T25,T27 | `test/mockgateway/**`, `cmd/hstong-mock-gateway` | all 51 routes + push topics served | L |
| T29 | All-endpoint e2e + coverage | tester | T28 | tests, coverage | SDK <-> mock for every endpoint | M |

### P10 — Hardening
| ID | Objective | Role | Deps | Outputs | Acceptance / Verify | Size |
|----|-----------|------|------|---------|---------------------|------|
| T30 | Rate limit / retry / breaker | backend | T29 | `internal/resilience/**` | mutations excluded from retry | M |
| T31 | Logging + metrics + OTel bridge | backend | T29 | `internal/logging`, `internal/metrics` | redaction; never-nil logger | M |
| T32 | Security review + push-verify opt-in | security | T30,T31 | hardening + ADR 0005 impl | no secrets in logs; keys override-able | S |

### P11 — Examples & docs
| ID | Objective | Role | Deps | Outputs | Acceptance / Verify | Size |
|----|-----------|------|------|---------|---------------------|------|
| T33 | Runnable examples | docs | T29 | `examples/**` | login, quote, order, stream, futures, algo | M |
| T34 | MkDocs site + SPEC + LEGACY + ADR index | docs | T29 | `mkdocs.yml`, `docs/**/*.md` | `mkdocs build --strict` passes | L |

### P12 — Integration
| ID | Objective | Role | Deps | Outputs | Acceptance / Verify | Size |
|----|-----------|------|------|---------|---------------------|------|
| T35 | Env-gated integration tests + wire validation | tester | T28 | `test/integration/**`, findings | hybrid codec confirmed (or fallback applied) | M |

### P13-P16 — Docs sync, Translations, Release, Close-out
| ID | Objective | Role | Deps | Outputs | Acceptance / Verify | Size |
|----|-----------|------|------|---------|---------------------|------|
| T36 | Full `.md` sweep + counts from SPEC | docs | T34,T35 | updated docs | no stale refs; `make docs-check` | M |
| T40 | 6-language READMEs + i18n check | docs | T36 | `README*.md`, `scripts/check_i18n.py` | lockstep switcher + `Last synced` | M |
| T37 | Release to GitHub + Gitee `main` | release | T40 | commits + pushes | both remotes at new SHA | S |
| T38 | Next-phase planning | planner | T37 | `next-phase.md` | gaps + candidates | S |
| T39 | Close-out report + index | orchestrator | T38 | `report.md`, plan actuals, `runs/index.md` | artifacts complete | S |

## Order of work

`T01/T02` (parallel) -> `T03` -> `T04` -> `T05..T08` -> `T09` -> `T10/T11` ->
(`T12/T13`, `T14..T17`) -> (`T18/T19/T20`) -> `T21/T22` ->
(`T23/T24/T25`, `T26/T27`) -> `T28/T29` -> (`T30/T31/T32`) -> (`T33/T34`) ->
`T35` -> `T36` -> `T40` -> `T37` -> `T38/T39`.

Parallelizable: T01 with T02; T12 & T14 after T09; T18 & T23 & T26 after T09;
T33 & T34 after T29.

## Artifacts

```
docs/runs/2026-09-21-hstong-full-surface/
  plan.md            run plan (frozen after approval; Actuals section at close-out)
  todos.md           run-wide tracker — the single source of truth (40 tasks)
  phases/            one file per phase, opened at first `doing`, closed at gate
    P00-foundation.md            P09-mock-gateway.md
    P01-protocol-core.md         P10-hardening.md
    P02-session.md               P11-examples-docs.md
    P03-market-pull.md           P12-integration.md
    P04-push-market-stream.md    P13-docs-sync.md
    P05-trade.md                 P14-translations.md
    P06-trade-push.md            P15-release.md
    P07-futures.md               P16-close-out.md
    P08-algo.md
  evidence/          raw command output, coverage, proto-verify diffs, gateway logs
  docs-plan.md  release-plan.md  report.md  next-phase.md
```

### Per-phase file template

```markdown
# P04 — Push + Market Stream

- Run: 2026-09-21-hstong-full-surface · Status: doing -> done
- Owner role: backend · Depends on: P01
- Plan ref: plan.md §P4 · Tracker: todos.md

## Objective / Exit criteria
<copy from plan.md>

## Tasks
| ID | Task | Role | Status | Deps | Acceptance | Verify | Evidence |
|----|------|------|--------|------|-----------|--------|----------|

## Decisions & deviations
## Files created / modified
## Verification evidence
| # | Command | Result | Artifact |
|---|---------|--------|----------|
## Blockers / follow-ups
## Gate
- Exit criteria met: yes/no — <note>
```

### Lifecycle rules

- `todos.md` remains the run-wide tracker; phase files add local detail + evidence
  only — no duplication of the plan.
- A phase opens when its first task hits `doing`; closes when every task is
  `done`/`cancelled` and the gate is recorded.
- Every task's verification output is pasted into `evidence/` and linked from its
  phase file before status flips to `done`.
- A phase that fails its gate stays `blocked` in `todos.md` with the blocker
  written into its phase file.

## Phase registry

| Phase | Tasks | Exit criteria |
|-------|-------|---------------|
| P00 Foundation | T01-T03 | ADRs merged; scaffolding builds/lints; PB vendored + keys + enums enumerated |
| P01 Protocol Core | T04-T09 | `proto-verify` clean; transport round-trip both codecs; codes mapped; crypto vector; `-race` green |
| P02 Session | T10-T11 | login/logout + keep-alive + re-login (1012/1013/1014) tested |
| P03 Market Pull | T12-T13 | all 9 endpoints typed + fixture round-trips |
| P04 Push + Market Stream | T14-T17 | 151B framing + `Any` + reconnect/resubscribe; channel API; golden-frame tests |
| P05 Trade | T18-T20 | 13 order endpoints; mutation single-attempt asserted; cursor pagination |
| P06 Trade Push | T21-T22 | `TradeStockDeliverNotify` decoded (下单/改单/撤单/成交) |
| P07 Futures | T23-T25 | 11 endpoints + futures push |
| P08 Algo | T26-T27 | 7 endpoints typed |
| P09 Mock Gateway | T28-T29 | 51 HTTP routes + push topics; SDK<->mock e2e for all endpoints |
| P10 Hardening | T30-T32 | resilience/logging/metrics; mutations excluded from retry; push-verify opt-in works |
| P11 Examples & Docs | T33-T34 | runnable examples; `mkdocs build --strict` passes; SPEC/LEGACY/ADR index |
| P12 Integration | T35 | real-gateway tests pass; hybrid codec confirmed |
| P13 Docs Sync | T36 | no stale refs; `make docs-check`; counts from SPEC.md |
| P14 Translations | T40 | 6 READMEs in lockstep; `scripts/check_i18n.py` passes |
| P15 Release | T37 | both GitHub + Gitee `main` at the new SHA |
| P16 Close-out | T38-T39 | `report.md` + `next-phase.md` + `runs/index.md` |

## Risks

| # | Risk | Mitigation |
|---|------|------------|
| R1 | Hybrid codec mismatch with Gateway | P12 validates both codecs against the real test env; per-endpoint `Codec` fallback |
| R2 | PB v2.2.0 vs Gateway v2.4.1 drift | `make proto-verify`; version recorded in `docs/SPEC.md` |
| R3 | `Any` unpack needs full registry | generate the complete 17-proto set; register all types |
| R4 | Trade/futures/algo bodies undocumented as proto | hand-written structs from doc tables; fixtures pinned in mock gateway |
| R5 | Order mutation retried by accident | T20 asserts a single outbound attempt |
| R6 | Money precision loss | `string`/`json.Number`; `scripts/check_money.py` in `make check` |
| R7 | Legacy keys rotate | public constants + `WithPlatformPublicKey` override |
| R8 | 40-task scope | phase-gated, resumable via `todos.md` |
| R9 | Windows file-lock on `a.out.exe` | set `GOTMPDIR` (documented in AGENTS.md) |
| R10 | Vendor archives committed accidentally | T02 git-ignores `*.zip/*.dmg/*.tar.gz/*.7z/*.rar` archives in root |

## Verification (global)

`go build ./...` -> `go vet ./...` -> `gofmt -l .` -> `go test ./...` ->
`go test -race -count=1 ./...` -> `golangci-lint run ./...` -> `make proto-verify`
-> `make docs-check` -> `make license-check` -> `mkdocs build --strict`.

## Post-implementation

- **Docs sync** — `docs-plan.md`: every `.md` reviewed; counts sourced only from
  `docs/SPEC.md`.
- **Release** — `release-plan.md`: add `gitee` remote, conventional commits
  referencing run + task IDs, push `origin main` then `gitee main`, verify both.
  No force-push/rebase.
- **Next phase** — `next-phase.md`: gaps, tech debt, 3-7 candidates, recommended
  breakdown.

## Appendix A — Endpoint inventory (51)

- **Market pull (9):** `/hq/BasicQot`, `/hq/OrderBook`, `/hq/KL`, `/hq/TimeShare`,
  `/hq/Ticker`, `/hq/Broker`, `/hq/UsOptionChainCode`, `/hq/UsOptionChainExpireDate`,
  `/hq/UsOverNightTradeCodes`
- **Market subscription (2):** `/hq/Subscribe`, `/hq/Unsubscribe`
- **Trade session (2):** `/trade/TradeLogin`, `/trade/TradeLogout`
- **Trade assets/positions (5):** `/trade/TradeQueryMarginFundInfo`,
  `/trade/TradeQueryHoldsList`, `/trade/TradeQueryRealFundJourList`,
  `/trade/TradeQueryHistoryFundJourList`, `/hs/rate/queryList`
- **Trade orders (13):** `/trade/TradeEntrust`, `/trade/TradeCancelEntrust`,
  `/trade/TradeBatchCancelEntrust`, `/trade/TradeChangeEntrust`,
  `/trade/TradeQueryMaxAvailableAsset`, `/trade/TradeQueryRealEntrustList`,
  `/trade/TradeQueryRealDeliverList`, `/trade/TradeQueryRealCondOrderList`,
  `/trade/TradeQueryHistoryEntrustList`, `/trade/TradeQueryHistoryDeliverList`,
  `/trade/TradeQueryHistoryCondOrderList`, `/trade/TradeQueryMarginFullInfo`,
  `/trade/TradeQueryBeforeAndAfterSupport`
- **Trade push (2):** `/trade/TradeSubscribe`, `/trade/TradeUnsubscribe`
- **Algo (7):** `/trade/AlgoAddOrder`, `/trade/AlgoCancelOrder`,
  `/trade/AlgoCancelEntrust`, `/trade/AlgoChangeOrder`, `/trade/AlgoActionOrder`,
  `/trade/AlgoQueryOrderList`, `/trade/AlgoQueryEntrustIdList`
- **Futures (11):** `/trade/FuturesQueryProductInfo`,
  `/trade/FuturesQueryMaxBuySellAmount`, `/trade/FuturesQueryFundInfo`,
  `/trade/FuturesQueryHoldsList`, `/trade/FuturesEntrust`,
  `/trade/FuturesCancelEntrust`, `/trade/FuturesModifyEntrust`,
  `/trade/FuturesQueryRealEntrustList`, `/trade/FuturesQueryHistoryEntrustList`,
  `/trade/FuturesQueryRealDeliverList`, `/trade/FuturesQueryHistoryDeliverList`

**Market push topics (11):** `11, 35` (quote), `14, 27, 28, 37` (tick),
`16` (broker), `17, 25, 26, 36` (order book).

## Appendix B — Limits & constants

| Item | Value |
|---|---|
| Gateway HTTP / TCP | `127.0.0.1:11111` / `127.0.0.1:11112` |
| Env domains | test `https://openapi-daily.hstong.com`, prod `https://openapi.hstong.com` |
| Token TTL | 3h, extended by calls |
| Concurrent subscriptions | <= 200 securities |
| Pagination | cursor `queryParamStr`, page size default 20, max < 100 |
| Ticker | `limit` <= 100 |
| Condition order | `validDays` <= 100 natural days |
| Test env hours | Mon-Fri 09:00-18:00 |
| Trade password key | `m+qS04/2CH1OweCnmXZ3TDZkCQS+hBzY` (Base64) |
| PB package version | v2.2.0 (17 protos) |
| Gateway version | v2.4.1 |

## Actuals (close-out)

- **40 tasks: 39 done, T37 (release) pending approval.** Phases P00-P14 and P16
  executed; P15 (release) outstanding.
- **6 sessions merged** implementation+tests: T05+T07, T10+T11, T12+T13,
  T18+T19+T20, T23+T24+T25, T26+T27.
- **Approach A corrected to hybrid** after finding the PB package has no HTTP
  request/response messages and the vendor SDKs use plain JSON: ADR 0007 added.
- **ADR set grew to 0001-0007** (0006 test-dependencies, 0007 HTTP JSON codec).
- **51/51 endpoints** covered by the mock e2e suite; 19 packages race-clean.
- **Deferred:** live integration confirmation (suite written, env-gated, unexecuted);
  make never run on this host; release/tag/Pages not wired.
- See eport.md and 
ext-phase.md.