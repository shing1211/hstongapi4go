# Next-Phase Plan — hstongapi4go

- **Run:** `2026-09-21-hstong-full-surface`
- **Task:** T38 (next-phase planning)
- **Status of this document:** planning only — no code or other docs were changed.
- **Predecessor:** `plan.md`, `todos.md` (40 tasks), `phases/P00`–`P12`, `docs-plan.md`,
  `evidence/*`.

> Citations are file/line or file §. Relative links are intentionally not used so
> `scripts/check_links.py` cannot be broken by this artifact.

## 1. What this run completed

The run built a complete, offline-green Go SDK for the HStong local Gateway surface:
the hybrid codec and envelope (`internal/transport/`, ADR 0002/0007), typed status
errors (`internal/errs/`, `pkg/types/status.go`), AES-ECB trade-password crypto
(`internal/crypto/`), the 151-byte push framing with enum-based `Any` decode and
reconnect (`internal/push/`), the session with single-flight re-login and keep-alive
(`internal/session/`, `pkg/hstong/session.go`), and typed managers for all 51 HTTP
endpoints — market 9 (`pkg/hstong/market/`), trade 18 (`pkg/hstong/trade/`), futures 11
(`pkg/hstong/future/`), algo 7 (`pkg/hstong/algo/`) — plus the channel streaming API
(`pkg/hstong/stream/`), a 51-route mock Gateway with TCP push (`test/mockgateway/`,
`cmd/hstong-mock-gateway/`), an all-endpoint e2e suite (`test/e2e/`, 51/51 subtests),
opt-in hardening (`internal/resilience|logging|metrics`, `client/hardening.go`), 7 ADRs,
the MkDocs site and SPEC/LEGACY, 6 READMEs with an i18n lockstep check, and a CI
workflow. Offline verification passed: `gofmt`/`go build`/`go vet` clean, `go test
-race -count=1 ./...` green across all packages (evidence/P12-T35.txt:149-183), 17
protos vendored and 17 `.pb.go` generated with no drift (evidence/P01-T04-T08.txt),
`money-check` OK. Two things are explicitly unfinished: live Gateway wire validation
(T35 written but never executed) and the release/close-out (T37, T39 still `todo`).

## 2. Gaps, tech debt, and deferred items

### G1 — Live wire validation is unrun; the core wire assumptions are unconfirmed
P12 is marked "done (tests written, NOT executed — no Gateway/credentials)"
(`phases/P12-integration.md:3`), with the open list at `:58-67` and the seven
assumptions at `evidence/P12-T35.txt:61-82`. Specifically:
- **Market `int64` representation.** `pkg/hstong/market` decodes with `client.JSON()`
  (`pkg/hstong/market/market.go:75,116,155,187,227,258,292,320,342`) over generated
  `int64` DTO fields, assuming a JSON number. This is inferred from the vendored Java
  (Gson) and Python (`json.loads`) SDKs, not observed (`docs/adr/0007-http-json-codec.md:15-32,75-78`).
- **`TradeQueryMaxAvailableAsset` envelope.** The SDK's `maxAvailableAssetResponse`
  expects the payload at `data.data` (`pkg/hstong/trade/orders.go:370-389`). A payload
  directly under `data` would silently decode zeros (P09 finding #2,
  `phases/P09-mock-gateway.md:70-75`).
- **`TradeQueryHoldsList` shape.** `HoldsListResponse.UnmarshalJSON` tolerates three
  forms (`pkg/hstong/trade/assets.go:243-280`); the live form is unknown (P09 finding #3,
  `phases/P09-mock-gateway.md:76-78`).
- **Push delivery and entitlements.** That a real `/hq/Subscribe` + TCP frame yields a
  decoded `Event` within 10s, and which of the 9 pulls the account may call, are
  unproven (`evidence/P12-T35.txt:74-78`). The mock shares the SDK's own framing
  (`phases/P09-mock-gateway.md:39-41`), so it cannot prove the live frame.

### G2 — CI has never run, and it does not enforce the repo's hard rules
`git log --oneline` shows a single commit (`461f219 Initial commit`); the entire run is
untracked/uncommitted (`git status`). Only `.github/workflows/ci.yml` exists, and it
triggers on push/PR to `main` — so it has never executed for this work. It invokes
`make` exactly once (`make money-check`, `ci.yml:63`). **No job runs `make proto-verify`,
`make docs-check`, or `make license-check`**, so AGENTS.md hard rules 1 (generated code
must match `.proto`) and 6 (no dangling links) are unenforced in CI. `make proto-verify`
and `make docs-check` are also not run anywhere else (see G3).

### G3 — `make` was never exercised on this host; several targets are unverified
`phases/P00-foundation.md:64` records "`make` is not available on this Windows host";
`phases/P01-protocol-core.md:68`, `evidence/P13-T36.txt:6`, and
`evidence/P14-T40.txt:6` repeat it. The Makefile defaults `PYTHON ?= python3`
(`Makefile:21`), which is a broken Microsoft Store stub on this host, while
`make docs-check` requires `mkdocs` (`Makefile:91-94`) and `make proto-verify` requires
`buf` + `protoc-gen-go` (`Makefile:86-89`). Each underlying command was run directly and
passed (e.g. `evidence/P13-T36.txt:13-15,27-61`), but the Makefile recipe itself has not
been executed. `make coverage` is also not wired into CI.

### G4 — `make test-integration` uses a tag that no test file declares
`Makefile:61-64` runs `go test -tags=integration ./test/...`. No file under
`test/integration` contains a `//go:build` directive (verified: zero matches), so the
target compiles and runs `test/mockgateway` and `test/e2e` too and relies solely on the
`HSTONG_INTEGRATION` env gate. The target name and its description ("env-gated") are
therefore misleading, and the extra packages run unexpectedly.

### G5 — `client.ProtoJSON()` is a documented fallback that cannot run as written
ADR 0007 says a market endpoint can be switched to `client.ProtoJSON()`
(`docs/adr/0007-http-json-codec.md:54-55,79-83`), and the integration test tells the
user to do exactly that (`test/integration/integration_test.go:271`). But
`ProtoJSONCodec` requires `out` to implement `proto.Message`
(`internal/transport/codec.go:56-72`), while every market response is a hand-written,
non-proto wrapper (`pkg/hstong/market/market.go:62-65,208-213`). Switching the codec
would return `ErrNotProtoMessage`, not decode. ADR 0007 acknowledges the wrapper problem
in prose but there is no helper, no production call path, and no test of the fallback
against a wrapper: `ProtoJSON` is exercised only by direct codec/client tests
(`client/client_test.go:290-324`, `client/routes_e2e_test.go:145`). So it is currently
both a dead public API and an unusable fallback.

### G6 — Resilience/logging/metrics are wired into the client, but only opt-in
The candidate claim that they are "not wired into the transport" is only half true. The
transport is intentionally single-attempt (`internal/transport/transport.go:95-97,152`),
but `client.execute` owns the policy: breaker → rate limiter → retry
(`client/client.go:166-199`), and logging/metrics fire per call (`client/client.go:141-159`).
All are nil by default (`client/options.go:66-77`) and covered by
`client/resilience_test.go` (`TestRetryPolicyRetriesQuery`,
`TestRetryPolicyNeverRetriesMutation`, `TestCircuitBreakerOpensAndRefuses`,
`TestRateLimiterGatesCalls`, `TestMetricsRecorderReceivesMeasurements`). The gap is
product-level: there is no runnable example or docs page showing a fully wired client,
and metrics are silently absent when `Recorder` is nil.

### G7 — The circuit breaker counts mutation failures (policy gap)
`client/client.go:191-197` calls `OnFailure`/`OnSuccess` for every route, including the
closed 12-endpoint mutation set (`internal/resilience/retry.go:35-51`). With a breaker
configured, a handful of ambiguous mutation failures can open it and then refuse
subsequent reads **and** mutations for the cooldown (`internal/resilience/breaker.go:99-118`).
This is exactly the financial-safety-adjacent case ADR 0003 guards elsewhere, yet it is
recorded only as a residual-risk note (`phases/P10-hardening.md:63`) with no ADR.

### G8 — SHA-1 is platform-mandated and weak
`internal/push/verify.go:10,71-72` verifies with `crypto/sha1`; the algorithm is fixed by
the platform (`docs/adr/0005-key-model-and-push-verification.md:58`,
`docs/SPEC.md:159`). Verification is off by default (`phases/P10-hardening.md:61`). No
alternative or forward migration is documented.

### G9 — Bundled platform public keys may rotate
`pkg/types/platformkeys.go` carries the test/prod keys; ADR 0005 flags that they "may
lag the platform" (`docs/adr/0005-key-model-and-push-verification.md:67-68`,
`phases/P10-hardening.md:62`). `WithPlatformPublicKey` (`client/options.go:160-172`)
allows override, but there is no rotation playbook or periodic-verification job.

### G10 — `docs/SPEC.md` is an endpoint index, not a response-schema spec for trade/futures/algo
SPEC.md has transport (§1), 51 endpoints (§2), 11 topics (§3), `NotifyMsgType` (§4), the
frame (§5), status codes (§6), and the data dictionary (§7) — but **no request/response
field tables** for the hand-written trade/futures/algo bodies. Those shapes are
reverse-engineered from the current and legacy docs and pinned only by mock fixtures
(`phases/P05-trade.md:48-51`, `phases/P07-futures.md:35-38`, `phases/P08-algo.md:31-33`).
Several dictionaries are explicitly non-exhaustive (`docs/SPEC.md:202,273,359,373`), so
SPEC cannot be diffed against the online docs automatically.

### G11 — Deprecated fields are carried in the trade/futures types
`HoldsBalance`, `MarketValue`, `LastPrice`, `IncomeBalance`, `MarketValueRate`,
`IncomeRatio` are decoded in `pkg/hstong/trade/assets.go:120-129,213-226`, and
`IncomeBalance`/`LastPrice` in `pkg/hstong/future/future.go:294,377`. They are documented
as unreliable (`docs/SPEC.md:456-462`) but nothing prevents a caller from using them.

### G12 — Release and close-out are incomplete; the index is stale
T37 `todo` and T39 `todo` (`todos.md:73-76`); T38 is `doing` while its declared
dependency T37 is still `todo` (`todos.md:73-74`) — the plan order was `T37 -> T38/T39`
(`plan.md:271-274`). `report.md` and `release-plan.md` (both required by `plan.md:296`)
do not exist. `docs/runs/index.md:5` still reads "in progress" with feature/close-out
commits "-". No git tag exists and only `origin` is configured — the plan's `gitee`
remote (`plan.md:9`) is absent (`git remote -v`).

### G13 — README advertises infrastructure that does not exist yet
`README.md:11` and `:260` link `https://shing1211.github.io/hstongapi4go/` and render a
"Docs GitHub Pages" badge, but there is no Pages deploy workflow (only `ci.yml`), so the
URL 404s until Pages is enabled and deployed. There is no CI status badge (all badges are
static shields.io) and no release automation/tagging.

### G14 — Translations are machine-authored; the i18n check does not assess quality
`scripts/check_i18n.py` verifies switcher lockstep and a native-content threshold;
`evidence/P14-T40.txt:147-149` states the thresholds "catch an untranslated English copy,
not translation quality". Native review of `README.zh-Hans/zh-Hant/ja/ko/es.md` is
advisable before a stable release.

### G15 — `CHANGELOG.md` `[Unreleased]` entry is stale
`CHANGELOG.md:12-15` says the translations "land in the follow-up translations task",
but T40 is done (`todos.md:72`). The entry should move into `[0.1.0]` or be reworded.

## 3. Candidate next-phase items

| # | Title | Objective | Why now | Effort | Dependencies | Risks |
|---|-------|-----------|---------|--------|--------------|-------|
| C1 | Live Gateway wire validation | Execute `test/integration` against a real Gateway and record the observed representations, closing P12/T35. | Every public wire claim is currently inferred (G1); a release without it publishes unverified behavior. | M | Gateway install, trade credentials, market entitlements, Mon-Fri 09:00-18:00 window, `HSTONG_INTEGRATION`. | No account/entitlements; some endpoints may legitimately be unentitled; failures force code + fixture changes. |
| C2 | CI truth: enforce generated-code and docs invariants | Add CI jobs for `proto-verify`, `docs-check`, and `license-check`, and get the first ubuntu run green. | CI has never executed and does not cover AGENTS hard rules 1 and 6 (G2, G3). | S/M | buf + protoc-gen-go + mkdocs + a real python3 in the runner. | First run likely surfaces real drift; pinned tool versions (P01:70-72) may need bumping. |
| C3 | Release integrity: commit, dual-remote push, Pages, tag | Commit the run, push `origin` + add `gitee`, publish GitHub Pages, tag `v0.1.0`, reconcile the CHANGELOG. | T37/T39 pending; the tree is uncommitted and README points at a non-existent Pages site (G12, G13, G15). | M | C1 for truthful notes, C2 for a green badge, gitee remote/token, DCO signing. | One large initial commit; secret scan; Pages may need repo settings. |
| C4 | Make the codec fallback real or remove it | Provide a working wrapper-level protobuf-JSON fallback for market endpoints, or drop `ProtoJSON()` from the public API and ADR 0007. | The documented fallback cannot run as written (G5), and C1 may require it. | S/M | C1 findings. | Public API change if removed; extra decode path; must keep `money-check` green. |
| C5 | Circuit-breaker / mutation failure policy | Decide and implement whether mutation failures may open the breaker; record the decision in an ADR. | G7 is an unrecorded financial-safety-adjacent policy introduced in P10. | S | none. | Changes P10 behavior; must preserve the single-attempt mutation guarantee and existing tests. |
| C6 | SPEC response-schema completion (trade/futures/algo) | Add canonical request/response field tables and non-exhaustive markers so hand-written bodies have a checkable source. | G10 is the largest remaining drift surface, currently validated only by mock fixtures. | L | C1 (confirm live shapes first). | Docs-only drift vs code unless generated or checked. |
| C7 | Translation quality review | Have native speakers review the five translations and tighten `check_i18n.py` to a quality gate. | G14; machine translation is acceptable for alpha, not for a stable tag. | M | native reviewers. | Reviewer availability; subjective quality bar. |

## 4. Recommended next phase

**Phase `P17 — Live Wire Validation & Codec Correctness`.** C1 is the gate: it is the
only item that can falsify shipped behavior, it is already engineered (T35's suite
exists and skips cleanly), and C4's shape depends on its result. C2 and C3 should follow
immediately as the releasing run, but they can be planned separately once the wire is
confirmed.

### Task breakdown (all roles, acceptance, and verify commands)

| ID | Objective | Role | Depends | Acceptance criteria | Verification command |
|----|-----------|------|---------|---------------------|----------------------|
| N01 | Run the env-gated integration suite against a real Gateway and capture full `-v` output into `evidence/`. | tester | Gateway + credentials | All 7 tests execute (not skip); observed `volume`/`timestamp` representation, `MaxAvailableAsset` path, `HoldsList` shape, first decoded push `Event`, and per-endpoint entitlement results are recorded. | `$env:HSTONG_INTEGRATION="1"; $env:HSTONG_TRADE_PASSWORD="…"; go test ./test/integration/... -count=1 -v` |
| N02 | Reconcile the market codec with the observed `int64` representation; update ADR 0007 and remove or correct the misleading fallback guidance. | backend | N01 | If `int64` is a JSON number (expected), ADR 0007 is marked confirmed and `test/integration/integration_test.go:271` no longer tells users to "switch to `client.ProtoJSON()`"; if quoted, a real fallback is implemented (see N04). | `go build ./...; go test -race -count=1 ./...` |
| N03 | Correct `TradeQueryMaxAvailableAsset` and `TradeQueryHoldsList` decoding if the live envelope differs from the fixtures, and re-pin the mock. | backend | N01 | SDK decodes the live shape; mock fixtures and `test/e2e` reflect the observed shape; 51/51 e2e subtests still pass. | `go test ./test/e2e/... ./test/mockgateway/... -count=1` |
| N04 | Make the protojson fallback operable, or remove `client.ProtoJSON()` from the public API and ADR 0007. | backend | N01, N02 | A wrapper can be decoded from a real payload (helper + test), or the API and ADR are removed; no non-proto type can be passed to `ProtoJSONCodec` and still compile. | `go build ./...; go test -race ./client/... ./internal/transport/...` |
| N05 | Record mutation single-attempt and reconciliation against the live Gateway (opt-in). | tester | N01 | `TradeEntrust` returns a non-empty entrust id with exactly one outbound attempt; the order cancels cleanly; result captured. | `$env:HSTONG_PLACE_ORDERS="1"; go test ./test/integration/... -run TestIntegration_PlaceAndCancelOrder -count=1 -v` |
| N06 | Fold observations into `docs/SPEC.md`, ADR 0007, and `phases/P12-integration.md`; close the P12 gate. | docs | N01-N05 | SPEC/ADR/P12 carry observed wire facts, not inferences; no dangling links. | `python scripts/check_links.py; mkdocs build --strict` |

Non-goals for P17: CI jobs, release/tagging, Pages, and translation review (C2, C3, C7),
which belong to the releasing run that follows.

## 5. Open questions for the human

1. **Is a real HStong Gateway with a funded test account, trade password, and market
   entitlements available in the next test window — and which markets (HK/US/A-share)
   is it entitled to?** C1/N01 cannot start without this.
2. **Must the wire be validated before any public tag, or should `v0.1.0-alpha` ship now
   labelled "wire unconfirmed"?** This decides whether C3 waits on P17.
3. **Gitee: keep both remotes in sync as the plan intended, or GitHub-only?** Is a gitee
   token/account available for T37/C3?
4. **Should `client.ProtoJSON()` stay public as a fallback or be removed?** (G5) Keeping
   it costs an unusable API; removing it is a pre-1.0 breaking change.
5. **Should mutation failures count toward opening the circuit breaker, or be excluded
   like retries?** (G7) The current behavior can block reads after order failures.
6. **Are native reviewers available for the five translations, or is machine translation
   acceptable for the alpha tag?** (G14)
