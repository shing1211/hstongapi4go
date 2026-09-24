# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.1.6] - 2026-09-24

### Added

- **REST service layer (`pkg/services`).** Market (9 pull endpoints plus
  Subscribe/Unsubscribe), account/asset/position (5), and trading (13) endpoints
  wired through the new HTTP adapter, with HK lot/tick/session/type/TIF
  validation and a closed order-mutation set that is never retried.
- **Domain wire mappers (`pkg/domain`).** Mappers for `BasicQot`, `KLine`,
  `TimeShare`, `Ticker`, `Broker`, margin funds, holdings, fund journals,
  interest rates, entrusts, fills, fares, conditional orders, max-available, and
  margin info, plus typed `Symbol`/`Market` helpers.
- **HTTP adapter (`pkg/transport`).** Deadlines, response-size caps, correlation
  IDs, cursor-based pagination, and query-only retry layered over the core
  transport.
- **Push engine (`internal/push`).** TCP client (151-byte framing, `PBNotify`
  decode, subscription registry, heartbeat, reconnect with exponential backoff,
  resubscribe), typed normalizers with unknown/versioned tolerance, and fan-out
  with dedup, gap/freshness detection, and drop-oldest backpressure.
- **OpenTelemetry (`otel` build tag).** Traces and metrics (`internal/otel`),
  `client` options `WithTracerProvider`, `WithMeterProvider`, and
  `WithPropagator`, and the `OTelHook` helper; stdlib-only when the tag is absent.
- **Integration and fuzz suites.** Env-gated mock/real-Gateway push integration
  tests and `FuzzReadFrame` for the frame reader.
- **Enterprise toolchain and CI/CD.** `golangci-lint`, `gosec`, `govulncheck`,
  a coverage gate (>=85% on `pkg/domain`, `internal/auth`, `internal/transport`),
  GoReleaser config with checksums, SPDX SBOM, and Cosign provenance, and the
  GitHub + Gitea release workflow.
- **Documentation.** `docs/COMPAT.md` compatibility matrix and
  `docs/RELEASE_CHECKLIST.md`.

### Changed

- `make coverage` now enforces the 85% gate via `scripts/coverage_gate.go`.
- `go.mod` adds `go.opentelemetry.io/otel` v1.36.0 (otel build tag only) and
  `go.uber.org/goleak` v1.3.0 (test-only).

### Fixed

- Pre-existing coverage gaps in `pkg/domain` (25.4% to 94.3%) and
  `internal/auth` (44.3% to 92.9%) closed with targeted unit tests.

## [0.1.5] - 2026-09-24

### Added

- **ADRs 0008–0011.** Four new architecture decision records covering the
  v-next enterprise SDK layer: ADR 0008 (decimal-backed financial types with
  `shopspring/decimal`), ADR 0009 (OpenTelemetry observability with `otel`
  build tag), ADR 00010 (additive v-next layered architecture: `pkg/domain`,
  `pkg/services`, `pkg/transport`, `internal/auth`), ADR 0011 (v0.1.x
  compatibility guarantees — no breaking type or wire changes).
- **`pkg/domain/` — decimal financial types.** `Money`, `Price`, `Quantity`,
  `Rate` types backed by `github.com/shopspring/decimal`, with scale, rounding,
  and validation. Typed `AccountID`, `OrderID`, `EntrustID`, `ContractID`,
  `SessionToken`. HK market models: `Symbol`, `Market`, `LotSize`,
  `TickSchedule`, `MarketSession`, `DefaultHKTickSchedule`. Order state machine
  (`OrderState`, `NextOrderState`, lifecycle events). Push event types
  (`QuoteEvent`, `TickerEvent`, `OrderBookEvent`, `BrokerEvent`, `TradeEvent`,
  `AccountEvent`, `SystemEvent`).
- **`pkg/transport/mappers.go`.** Wire-to-domain mappers for `BasicQot`,
  `Ticker`, `OrderBook`, `TradeStockDeliverNotify` using the correct generated
  proto field paths (`GetLastPrice` → `float64`, `GetBusinessPrice` → `string`,
  etc.).
- **`internal/auth/` — session and token lifecycle.** `Session` struct with
  expiry/refresh tracking, `SessionStore` interface with `InMemoryStore`,
  `TokenManager` with injectable `Clock` and configurable TTL/refresh window,
  `Authenticator` with `Login`/`Logout`/`GetSession`/`MustBeAuthenticated`,
  re-export of `crypto.EncryptTradePassword`.
- **`internal/auth` tests.** Crypto vector `123456 -> W1U8iZIppSE+mBMtzy9vZQ==`,
  `Session` expiry/refresh, `TokenManager` CRUD, `InMemoryStore` concurrency.

### Changed

- **`go.mod`** — added `github.com/shopspring/decimal v1.4.0` as runtime
  dependency (ADR 0008). Grouped require block.
- **`scripts/check_money.py`** — updated docstring to reflect that `pkg/domain/`
  uses `decimal.Decimal` (exempt from float rejection) while the rest of
  `pkg/` stays `string`/`json.Number`.

### Changed

- **Docs: algo status table, protocol curl examples, mkdocs nav.** Fixed the
  algo `EntrustStatus` table to match the actual 11 constants; corrected the
  login route and curl request bodies in `docs/protocol.md`; removed the
  duplicate nav entry from `mkdocs.yml`.
- **`docs/runs/`** — added the `hstong-enterprise-sdk` run plan and tracker
  (E01–E22 across 4 phases).

## [0.1.4] - 2026-09-23

### Changed

- **Last synced date bumped** to 2026-09-23 across all six READMEs to reflect
  the current repository state.

## [0.1.3] - 2026-09-22

### Added

- **Key Concepts section in README** — Client, SessionManager, Manager, and
  stream.Client explained with surface-selection guide.
- **Complete auth flow in authentication.md** — 5-step pattern with expected
  outputs and troubleshooting table (1012/1013/1014/20033).
- **EntrustStatus reference tables** — added to trading.md (20 states),
  futures.md (17 states), and algo.md (15 states), with Chinese labels.
- **curl examples in protocol.md** — login, market query, push subscribe.
- **Expected output comments in getting-started.md** — error and success
  output shown for minimal program.

### Changed

- **MIGRATION.md overhaul** — migration table with before/after code examples,
  typed error handling guidance, v0.1.2 notes.

## [0.1.2] - 2026-09-21

### Fixed

- **Stale ProtoJSON references removed (G5).** `client/doc.go`,
  `internal/transport/doc.go`, and `pkg/hstong/market/doc.go` were updated
  to remove references to the removed `client.ProtoJSON()` public API. The
  market codec section in `pkg/hstong/market/doc.go` was rewritten to
  confirm `client.JSON` is used for all nine endpoints.
- **Stale documentation references reconciled.** CHANGELOG `[Unreleased]`
  compare link fixed (`v0.1.0` → `v0.1.2`); `[0.1.1]` and `[0.1.2]`
  release links added. ADR 0007 version references corrected (`v0.2.0` →
  `v0.1.1`, "only release" → "first release"). `docs/CONTRIBUTING.md`
  money-as-string rule now cites `DESIGN.md §7` instead of unrelated
  codec ADRs. README Status badge version string dropped to match
  translations.

## [0.1.1] - 2026-09-21

### Changed

- **`client.ProtoJSON()` removed.** `ProtoJSONCodec` was never functional for market
  data (the wrapper structs are not `proto.Message`) and the documented fallback
  could never work at runtime. The `proto` package dependency stays — it is still
  required for TCP push binary protobuf. The v0.1.0 `ProtoJSONCodec` fallback
  bullet below is superseded.

### Fixed

- **Circuit breaker isolation (G7).** The SDK now has two independent circuit
  breakers: `QueryBreaker` (gates read-only queries) and `MutationBreaker` (gates
  order/futures/algo mutations). A storm of rejected orders can no longer block
  reads. The legacy `WithCircuitBreaker(b)` option sets both for backward
  compatibility.
- **Response schemas (G10).** `docs/SPEC.md` §3 now documents all 34 response types
  for the trade, futures, and algo surfaces, transcribed from Go struct field
  comments. Complex nested types (`OrderVo`, `HoldsVo`, `FundInfo`, etc.) and all
  scalar/array wrappers are now indexed.

## [0.1.0] - 2026-09-21

First alpha release of the SDK, covering the whole current HStong Quant OpenAPI
Gateway surface: **51 HTTP endpoints** and **11 market push topics**, plus the
trade and futures order-status push channels. Endpoint and topic counts are
canonical in [docs/SPEC.md](./docs/SPEC.md).

### Added

- **Core client (`client/`).** `New` with functional options applied in order
  over defaults, `WithEnv` for `HSTONG_*`, all 51 canonical routes and the three
  Gateway alias forms (`/X`, `/XRequest`, `/XRequestMsgType`),
  `JSON`/`ProtoJSON` codec accessors, typed unknown-route errors, and an
  idempotent leak-free `Close`. The client issues exactly one HTTP attempt per
  call by default.
- **HTTP transport (`internal/transport/`).** POST-only executor over the
  documented envelope (`{"timeout_sec":...,"params":{...}}` /
  `{"ok":...,"err":...,"data":{...}}`) with per-endpoint codec dispatch and
  context/timeout handling.
- **Typed errors and status codes (`internal/errs/`, `pkg/types`).** The full
  Gateway status table (`0000`, `1001`–`1018`, `20033`, `40001`, `40002`)
  modelled as `types.StatusCode`, with `Retryable()` and `ReLoginRequired()`
  classification and `errors.Is`/`errors.As` traversal.
- **Trade-password crypto (`internal/crypto/`).** AES-192-ECB/PKCS7 encryption
  of the trade password with the documented key, verified against the
  `123456 -> W1U8iZIppSE+mBMtzy9vZQ==` vector.
- **Market data (`pkg/hstong/market/`).** Typed managers for the nine pull
  endpoints (BasicQot, OrderBook, KL, TimeShare, Ticker, Broker, UsOptionChain
  Code, UsOptionChainExpireDate, UsOverNightTradeCodes) plus
  Subscribe/Unsubscribe for the 11 push topics, with local validation for empty
  security lists and the `limit <= 100` ticker cap.
- **TCP push client (`internal/push/`).** 151-byte frame parsing, `PBNotify`
  `Any` unpacking by `notifyMsgType`, reconnect with backoff, and automatic
  re-subscription of active topics on reconnect.
- **Streaming API (`pkg/hstong/stream/`).** `Connect`, market `Subscribe`, and
  `SubscribeTrade`, with buffered `Updates()`/`Errors()` channels,
  `Cancel`, drop-oldest backpressure, decoded `Event` accessors
  (`BasicQot`, `Ticker`, `OrderBook`, `Broker`, `TradeDeliver`,
  `FuturesTradeDeliver`), and `ErrReconnected` notifications.
- **Trade session (`pkg/hstong`).** `SessionManager` with login/logout,
  single-flight `EnsureLoggedIn`, `ReLogin` driven by `errs.ReLoginRequired`,
  and an optional keep-alive poll to extend the three-hour token.
- **Trading (`pkg/hstong/trade/`).** Five assets/positions endpoints, thirteen
  order endpoints, and the `TradeSubscribe`/`TradeUnsubscribe` push
  subscription, with cursor pagination helpers, the `validDays <= 100` guard,
  and a closed mutation set that is never retried.
- **Futures (`pkg/hstong/future/`).** All eleven futures endpoints plus futures
  trade-delivery push decoding.
- **Algo (`pkg/hstong/algo/`).** All seven algorithm-trading endpoints
  (master/sub-order queries and operate actions).
- **Domain types (`pkg/types`).** Enum families from the data dictionary
  (order property/status/type, market, direction, fill status/type, security
  type, instrument type, currency, region, K-line period, price adjustment,
  query direction), status codes, and the bundled test/production platform
  public keys.
- **Vendored protobuf and codegen (`proto/`, `gen/`).** HStong protobuf package
  v2.2.0 (17 `.proto` files) vendored with provenance, and generated Go types
  committed under `gen/`, reproducible with `make proto` / `make proto-verify`.
- **Mock Gateway (`test/mockgateway/`, `cmd/hstong-mock-gateway/`).** An offline
  HTTP server for all 51 routes plus a TCP push server, with fixture sets,
  per-path error injection, push type-URL override, and a standalone binary. The
  all-endpoint `test/e2e` suite drives the SDK against it.
- **Resilience (`internal/resilience/`).** Token-bucket rate limiter, query-only
  retry policy, and circuit breaker, all opt-in; mutation endpoints are excluded
  from retry at every configuration.
- **Observability (`internal/logging/`, `internal/metrics/`).** Structured
  `slog` logging with secret redaction and a dependency-free, OTel-shaped
  metrics layer with an in-memory recorder.
- **Opt-in push verification.** `SHA1WithRSA` verification of the push
  `bodySHA1` field, off by default, enabled with `HSTONG_VERIFY_PUSH` and an
  overridable platform public key.
- **Examples (`examples/`).** Runnable, credential-free-to-compile programs for
  quickstart, market data, trading, futures, algo, and streaming; order
  mutations are skipped unless explicitly enabled.
- **Integration tests (`test/integration/`).** Env-gated tests that probe the
  real Gateway's `int64` wire representation, envelope nesting, market reads,
  push delivery, and trade reads, with an opt-in place-and-cancel order test
  asserting a single mutation attempt.
- **Documentation.** Root README, MkDocs Material site under `docs/`, ADRs
  0001–0007, [docs/DESIGN.md](./docs/DESIGN.md),
  [docs/SPEC.md](./docs/SPEC.md),
  [docs/LEGACY.md](./docs/LEGACY.md), [AGENTS.md](./AGENTS.md),
  [DISCLAIMER.md](./DISCLAIMER.md), and SPDX/license tooling.
- **Six-language README set.** The English [README](./README.md) is canonical,
  with `zh-Hans`, `zh-Hant`, `ja`, `ko`, and `es` translations kept in lockstep
  by a shared language switcher, a `Last synced` banner, and
  `scripts/check_i18n.py`; see [TRANSLATING.md](./TRANSLATING.md).
- **Documentation link checking.** `scripts/check_links.py` walks every Markdown
  file and fails on a relative link whose target is missing; `make docs-check`
  runs it together with the i18n lockstep check and `mkdocs build --strict`.

### Changed

- Market HTTP bodies decode with `encoding/json` over typed DTOs (ADR 0007),
  narrowing ADR 0002's original uniform-`protojson` market branch; `protojson`
  remains a per-endpoint fallback (**superseded by v0.1.1: `ProtoJSONCodec`
  removed**).
- The public hardening options (`WithRetryPolicy`, `WithRateLimiter`,
  `WithCircuitBreaker`, `WithMetrics`) are exposed through `client` aliases so
  callers never name internal types.

### Fixed

- **Session keep-alive test flake.** `pkg/hstong/session_test.go` now counts
  requests client-side and joins the real keep-alive goroutine instead of
  relying on timing; 0 failures across 30 runs.
- **Push cross-type ordering.** `internal/push` no longer assumes a fixed
  arrival order across notification types, removing a load-dependent flake.
- **Generated-code link hook.** The MkDocs hook now also rewrites repository
  `../test/...` links, so `mkdocs build --strict` validates every in-docs link
  while out-of-tree links resolve to GitHub.
- **`make test-integration` filter.** The target now runs
  `test/integration/...` directly; the previous `-tags=integration` filter did
  not match the env-gated tests, which skip unless `HSTONG_INTEGRATION=1`.

### Security

- No platform credentials, developer private key, or device-binding state are
  held by the SDK; key handling and request signing belong to the Gateway
  (ADR 0001, ADR 0005).
- The bundled platform public keys are public reference data, not secrets; no
  secrets are committed to the repository.
- Trade, futures, and algo mutations issue exactly one attempt and are never
  auto-retried, so an ambiguous timeout cannot silently duplicate an order
  (ADR 0003).
- The plaintext trade password is held in memory only, encrypted before it
  leaves the process, and never logged or embedded in an error.

[Unreleased]: https://github.com/shing1211/hstongapi4go/compare/v0.1.6...HEAD
[0.1.0]: https://github.com/shing1211/hstongapi4go/releases/tag/v0.1.0
[0.1.1]: https://github.com/shing1211/hstongapi4go/releases/tag/v0.1.1
[0.1.2]: https://github.com/shing1211/hstongapi4go/releases/tag/v0.1.2
[0.1.3]: https://github.com/shing1211/hstongapi4go/releases/tag/v0.1.3
[0.1.4]: https://github.com/shing1211/hstongapi4go/releases/tag/v0.1.4
[0.1.5]: https://github.com/shing1211/hstongapi4go/releases/tag/v0.1.5
[0.1.6]: https://github.com/shing1211/hstongapi4go/releases/tag/v0.1.6
