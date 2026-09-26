# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.1.13] - 2026-09-26

Nothing in this release changes the behaviour of the v0.1.x public surface, so
ADR 0011 continues to hold. The one real defect fixed here is confined to
`pkg/services`, which no caller can reach yet (see `docs/VNEXT.md`); the rest
hardens the guard that should have caught it.

### Fixed

- **Market prices were being silently rounded to three decimals.** The v-next
  market service read price fields as `float64` and passed them through
  `fmt.Sprintf("%.3f", …)`, which hardcoded an assumption that every price has
  exactly three decimal places. A tick of `0.0005` became `0.001` — a wrong
  price, reported as though it were exact. 23 conversion sites were affected,
  including two rate fields that lost precision at four decimals instead. All now
  use a precision-preserving conversion, and the wire field is `json.Number`
  rather than `float64`, which keeps the Gateway's digits verbatim and rejects a
  non-numeric value that `string` would have accepted silently.

- **The money guard could be evaded by renaming a field.** `check_money.py`
  judged a field's **Go name** only. A money value on the wire could therefore
  hide behind an innocuous Go name, and one did: the order-book price tick was
  declared `TickSize float64 \`json:"spreadLevel"\``, and this repository's own
  evidence records that the field was *named* `TickSize` rather than
  `SpreadLevel` specifically so the guard would stay green
  (`docs/runs/2026-09-21-hstong-full-surface/evidence/P03-T12-T13.txt`). The guard
  now also judges the **json tag**, which is what the value actually is on the
  wire. It is covered by 19 unit tests, including a negative case so the rule
  cannot drift into blanket-firing.

### Added

- **The Python guards are now tested, and tested in CI.** `make scripts-test` runs
  the `scripts/` unit tests (stdlib `unittest`, no new dependency) and is wired
  into `make check` and the `build` job. A test nothing executes is not a test.
  The job also pins `actions/setup-python` to 3.11, because `money-check` had
  only ever worked by luck of whichever Python the runner image happened to ship.

### Changed

- **ADR 0008 amended with exactly one declared exception.** ADR 0008 forbids
  `float64` money fields anywhere in the SDK, while ADR 0011 forbids changing an
  exported type signature in the v0.1.x surface — and the order-book tick is an
  exported field. Rather than suppress the newly-hardened guard, the exception is
  recorded as a **dated, field-scoped waiver** matching on path, field name, wire
  key, and type, so a *different* `float64` in the same package is still caught.
  It expires at v1.0.0, when ADR 0011's guarantee ends and the field can be
  changed outright.

## [0.1.12] - 2026-09-26

**The first release to publish artefacts.** v0.1.7 through v0.1.11 were tagged and
green but shipped nothing, because the Release workflow only ran `goreleaser
check`, which validates configuration without building or uploading. Five green
tags were not evidence the release path worked. v0.1.12 adds the two external
binaries the pipeline needs and turns publishing on for real.

### Added

- **Signed releases.** The checksum file is signed with cosign keyless over the
  GitHub Actions OIDC identity, so consumers can verify provenance without
  trusting the release page. Only the checksums are signed, not each of the six
  archives: one signature covers every artefact, and it keeps verification to a
  single bundle. `cosign` is installed by `sigstore/cosign-installer`; no signing
  key exists to leak, and `id-token: write` was already granted.

- **`workflow_dispatch` on the Release workflow**, so an already-pushed tag can be
  published without moving it.

### Fixed

- **SBOM generation could never have worked.** The `sboms` block requires the
  external `syft` binary, which GoReleaser does not bundle and the Release
  workflow did not install. This was invisible for the four releases that shipped
  as bare tags, because the workflow only ran `goreleaser check` — which
  validates configuration without running the pipeline — and the first real
  `goreleaser release` failed on it immediately. `syft` is now installed by
  `anchore/sbom-action/download-syft` with the version pinned, so an upstream
  SBOM format change cannot alter a published release without a deliberate bump.

## [0.1.11] - 2026-09-26

**This tag published no artifacts.** Its CI was green, but the Release workflow
failed on its first real `goreleaser release` run: the `sboms` block needs the
external `syft` binary, which the workflow did not install. The tag remains valid
as a source tag, and the code in it is sound, but there is no GitHub release, no
archives, and no checksums attached to it. Artifacts first ship in 0.1.12.

This is the same class of gap that made v0.1.7 through v0.1.10 artifact-free: the
workflow ran `goreleaser check`, which validates configuration and never runs the
pipeline, so nothing about the release path was actually exercised until a tag
demanded it. Five green tags were not evidence the release worked.

It also changes no behaviour of the v0.1.x public surface, so ADR 0011 continues
to hold.

### Fixed

- **The coverage gate was failing on `internal/push`, and the recorded figure for
  that package was wrong.** `internal/push` was documented at 86.1%, but that
  number was measured in a dirty working tree. On a clean checkout — what CI sees
  — the package measured **83.3%, below the 85% gate**. The cause was that the
  reconnect paths in `Run` were only exercised when a dial attempt happened to
  fit inside a test deadline, so the same commit reported anywhere from 83.3% to
  86.5% and the gate was a coin flip. `internal/push/client_paths_test.go` now
  covers those paths deterministically, using substituted dialers, explicit
  handshakes, and pre-cancelled contexts instead of timing: option normalization,
  the three `Connect` interleavings, the `Run` dial-failure path, drop-oldest on
  both the handler queue and the error channel, and the `writeFull` short-write
  paths. The package is now at 93.9% with roughly a 9pp margin, verified stable
  across repeated cold runs. No behaviour change.

## [0.1.10] - 2026-09-26

Nothing in this release changes the behaviour of the v0.1.x public surface, so
ADR 0011 continues to hold. Almost all of it is internal consolidation of code
that no caller could reach. The two user-visible changes are the opt-in
correlation-ID option and a constructor signature in `pkg/services`, which is
still unwired.

### Removed

- **Two of the three push implementations.** `internal/push` contained `Client`
  (released, used by `pkg/hstong/stream`), `Manager`, and `Fanout`. `Manager`
  and `Fanout` had no production caller, and they were not two versions of one
  protocol but **different protocols**: the released path subscribes over HTTP
  and re-subscribes over HTTP on reconnect, while `Manager` sent topic frames
  over the TCP push socket — an assumption no test had ever checked, because the
  live Gateway run has never executed. Adopting it would have made an unverified
  protocol guess the foundation of the layered API. `Manager`, `Fanout`, and the
  `Normalizer` that served them are gone, along with `DedupCache`, which was
  inert because the Gateway sends no per-event sequence.
- **`pkg/transport.Adapter`.** Unreachable, and *less* capable than the released
  `client.Client`: no rate limiter, no circuit breaker, no metrics, no tracing
  spans. Routing `pkg/services` through it would have been a regression. It was
  also broken in three untested ways — it discarded the transport it was
  constructed with and built a fresh one per request against a hardcoded base
  URL, and its `WithDeadline` cancelled a *previous* caller's in-flight context.
  Removing it took `pkg/transport` from 88.7% to 100%: the deleted code was the
  uncovered part.
- **`pkg/transport/doc.go` corrected.** It documented a `RESTAdapter` and a
  `PushAdapter` that were never implemented.

### Added

- **Opt-in correlation IDs on the request path.** `client.WithCorrelationIDHeader`
  adds a request header carrying a fresh 32-character hex identifier per request,
  so a Gateway or proxy log line can be tied back to one SDK call. **Off by
  default**: the header is visible to the Gateway and anything proxying it, so
  ADR 0011 keeps it opt-in and a caller who enables it should confirm their
  deployment tolerates the extra header. Identifiers come from `crypto/rand`,
  because a predictable value in a log is useful to anyone trying to collide
  requests or forge a plausible one. The header *name* is configurable rather
  than fixed, and a whitespace-only name is treated as off rather than producing
  a request `net/http` rejects.
- **Machine-checked package layering.** `internal/layering` parses the
  repository's own imports and asserts six boundary rules. This replaces a
  `depguard` configuration, because depguard in golangci-lint v2.9 silently
  ignores path globs in its `files` field: the rules were accepted, reported
  "0 issues" on a clean graph, and reported "0 issues" with a deliberately
  planted violating import present. A rule that enforces nothing while looking
  like enforcement is worse than no rule. The replacement also asserts that no
  rule matches zero packages.

### Changed

- **`pkg/services` depends on an interface it declares itself.** The three
  constructors and their `With*Client` options now take an `Executor` rather
  than `*client.Client`, with the released client as the injected implementation
  (ADR 0010 rule 6). This is a signature change, but only in the unwired layer,
  and it is what lets that layer be tested against a fake instead of a Gateway.
  It deliberately still names `client.Route` and `client.Codec`: inverting the
  dependency removes the dependency on the concrete type, not on the client's
  shared route vocabulary, and private copies of those types would duplicate the
  canonical route table in `docs/SPEC.md`.
- **The N1 v-next decision is recorded.** Option A, commit to the layered API, is
  now the recorded decision rather than an open question, with the ordered
  programme and the evidence for each step in `docs/VNEXT.md`. The deciding
  finding was that push consolidation could not be treated as pre-decision
  groundwork, which the evidence for that decision disproved.

### Fixed

- **The risk register was understating what had been fixed.** R2 (route-mutation
  classification) and R7 (unbounded response read) were still recorded as open
  after being fixed, and the partial-outage table still claimed the active
  executor used an uncapped `io.ReadAll`. Each row now cites the commit that
  changed its status. R9 is resolved by the Adapter removal. R14 was re-scoped:
  its rationale named symbols that no longer exist and described
  `internal/auth/doc.go` as claiming unimplemented behaviour when the doc was in
  fact accurate — the real gap is that the single-flight and refresh primitives
  exist with no non-test caller, and the doc now says so.

### Internal

- **Coverage gate widened from three packages to ten**, now covering the entire
  release path: `pkg/domain`, `internal/auth`, `internal/transport`,
  `internal/push`, `pkg/hstong{,/stream,/trade,/algo}`, `pkg/types`, and
  `pkg/transport`. Getting there meant testing branches that had never executed
  rather than just uncovered lines — `WithKeepAliveRoute` had no test at all, the
  topic-to-message-type map was half covered (a wrong entry hands every event to
  the caller under the wrong type), each request `validate()` had only its happy
  path, and `entrustBSFromInt32` — which decides whether a fill is labelled buy
  or sell — had one of its five cases covered.
- **`go test -race` runs and passes locally.** The release host does have a C
  toolchain — MinGW gcc with `CGO_ENABLED=1` — so
  `go test -race -count=1 ./...` was run for this release with no data races.
  Corrected 2026-09-26: the release notes for this version originally claimed the
  race gate was CI-only, which was wrong and had been carried unchallenged since
  v0.1.0.

## [0.1.9] - 2026-09-25

### Fixed

- **The secret-scanning allowlist was incomplete.** 0.1.8 introduced the pinned
  `gitleaks` job with an allowlist of three paths, but five legitimately-flagged
  locations were not covered, so the job reported a leak and failed on every run.
  The allowlist is now complete and the scan exits 0. Each entry is justified:
  the public platform keys (ADR 0005), the vendored `proto/` tree, the published
  AES test vector, and two test files holding deliberately-planted credential
  strings. No rule class is disabled, so a new suppression stays a visible review
  decision. Verified with planted AWS, GitHub, and RSA keys still failing the
  scan, so the rules are proven to fire rather than merely silenced.

### Added

- **The coverage gate now covers the released surface.** It gated only
  `pkg/domain`, `internal/auth`, and `internal/transport`, so the four public
  managers a caller actually imports were ungated — and all four were below the
  85% threshold: `pkg/hstong` 84.7%, `stream` 76.3%, `trade` 81.3%, `algo`
  81.2%. They now sit at 90.7%, 85.9%, 86.3%, and 87.5%, and all seven packages
  are gated.
- **Tests aimed at branches that had never executed, not just uncovered lines.**
  `WithKeepAliveRoute` had no test at all, and a wrong keep-alive endpoint turns
  every poll into an error. The topic-to-message-type map in `stream` was half
  covered, where a wrong entry silently hands every event to the caller under the
  wrong type. Each request `validate()` in `trade` and `algo` had only its happy
  path, so a dropped required-field or format guard was undetectable. Also added:
  the drop-oldest backpressure and closed-subscription paths, and all three wire
  shapes `HoldsListResponse` must decode.

`internal/push` is deliberately still ungated: it holds three implementations,
two of which the pending v-next decision may delete.

## [0.1.8] - 2026-09-25

### Security

- **Bounded Gateway response read.** `internal/transport` buffered every
  response with an unbounded `io.ReadAll`, so a malformed or hostile Gateway body
  could force an arbitrarily large allocation. Reads are now capped at 8 MiB by
  default, configurable with `WithMaxResponseBytes`; a non-positive value restores
  the default rather than disabling the cap, so an accidental zero cannot
  reinstate unbounded reads. An over-limit body returns a typed error naming the
  cap. The cap reuses the `MaxBytesReader` pattern already reviewed in
  `pkg/transport`, so there is one mechanism rather than two.
- **Secret scanning in CI** ([ADR 0012](./docs/adr/0012-ci-secret-scanning.md)).
  `gosec` analyses source for insecure patterns and `govulncheck` queries the
  vulnerability database, but neither looked for credentials, so a leaked token
  could enter through a pull request and reach every clone. A pinned `gitleaks`
  Action now scans the full history of the pushed ref. `.gitleaks.toml` allowlists
  three paths by justification — the public platform keys (ADR 0005), the vendored
  `proto/` tree, and the published AES test vector — and never disables a rule
  class, so a new suppression is a visible review decision. `go.mod` is untouched,
  so consumers gain no dependency.
- **Opt-in push read deadline.** A silently dead peer left the stream read
  goroutine parked until the OS gave up. `push.WithReadDeadline` now arms a
  deadline before every read, bounding the inter-frame gap rather than total
  connection lifetime. It is **off by default**: the local Gateway documents no
  heartbeat cadence on this stream, so a non-zero default would risk tearing down
  a healthy connection in a quiet market.

### Fixed

- **A latent process-killing panic in a wire mapper.** `MapTradeDeliveryToTradeEvent`
  passed raw protobuf string fields into the domain constructors, which panic on
  an unparseable value. An unset field is empty, so a zero-value or partially
  populated delivery notification panicked — and there is no `recover()` anywhere
  in the push or stream dispatch path, so that would have taken down a consumer
  goroutine and the process. Empty fields now map to zero. A non-numeric value
  still panics by design: the domain exposes no non-panicking constructor, and
  silently substituting a number for a malformed price would be worse than
  failing. Latent rather than live, because the mapper sits in the not-yet-wired
  v-next layer.
- **The mutation classification could drift silently.** `internal/resilience`
  maintained a closed set of 12 mutation paths separately from `client`'s 51 route
  constants, and a mutation route missing from that set would be classified as a
  retryable query — breaking the ADR 0003 guarantee that order mutations issue
  exactly one attempt. Nothing tied the two lists together. `docs/SPEC.md` does not
  mark which endpoints mutate and no naming rule catches every future mutation, so
  the guard is exhaustion rather than inference: every registered route must be in
  either the expected-mutation or the known-safe list, making route addition a
  deliberate classification decision.

### Added

- **`otel` build-tag CI job.** The OpenTelemetry instrumentation is behind a build
  tag, so the default build never compiled or exercised it. It was built and
  tested only on a release tag, so a break could reach `main` unnoticed and
  surface at release time. CI now builds, vets, and tests with the tag on every
  push and pull request.
- **Bounded fuzz job.** A normal `go test` runs only the twelve inline seed cases.
  CI now runs `FuzzReadFrame` for 30 seconds; verified locally at 734k executions
  with 18 newly interesting inputs and no crash.
- **Tests for the wire mappers** and the mutation classifier, taking
  `pkg/transport` from 58.8% to 88.7% and `internal/transport` to 99.1%.
- **[ADR 0012](./docs/adr/0012-ci-secret-scanning.md)** and **`docs/VNEXT.md`**, a
  decision draft on whether to wire the v-next layer in or delete it, written as the
  forcing function for that decision.
- **Agent instructions that can be followed.** The GitNexus rules in `AGENTS.md`
  and `CLAUDE.md` required graph analysis unconditionally, but the index is a
  local, git-ignored artifact, so a fresh clone has none, and a stale MCP server
  fails every graph read while the CLI works. The rules now apply whenever the
  graph can answer, with an explicit fallback requiring an agent to state that
  graph analysis was unavailable rather than skip it silently.

### Fixed (documentation)

- The v-next boundary document asserted four rules that are false in code —
  `pkg/domain` importing generated protobuf, `pkg/services` importing `client`,
  an unconsumed `Adapter`, and "enforced by golangci-lint" when no boundary rule
  exists. It is now marked superseded and carries a source-verified deviations
  table.
- All six READMEs claimed ADRs 0001-0007 and omitted the v-next layer from their
  package layouts. `docs/DESIGN.md` was present but absent from the MkDocs nav.
- Run artifacts carried wrong close-out SHAs and a "twenty of twenty-two" count.
- A documented "mojibake" defect that did not exist: zero replacement characters
  and no Latin-1 mojibake in any of the files it named.
- `Makefile` still ran the `go install …@latest` path that fails to compile under
  Go 1.26, described three live targets as unimplemented stubs, defaulted
  `PYTHON` to a `python3` that many hosts lack, and carried a CRLF workaround
  that `.gitattributes` had made obsolete.
- `proto/PROVENANCE.md` claimed a byte-for-byte tree that `.gitattributes`
  line-ending normalisation makes untrue.
- Threat-model R12 was still marked open although the scanners were pinned.

### Known limitations

- The push read deadline is off by default, so R3 is only partially closed;
  finishing it needs the Gateway's observed inter-frame gap from a live run.
- The coverage gate still covers three packages. Five released-surface packages
  remain below 85% (`pkg/hstong` 84.7%, `algo` 81.2%, `trade` 81.3%,
  `internal/push` 79.9%, `stream` 76.3%), and widening it is deferred until the
  v-next decision so tests are not written against code that may be deleted.
- The Release workflow runs `goreleaser check` on a tag, not `goreleaser release`,
  so tags ship without artefacts. Publishing needs the Gitee-token and
  cosign-OIDC questions settled.
- The v-next layer is still unreachable by a caller, so its hardening protects
  nobody today. `docs/VNEXT.md` lays out the decision.

## [0.1.7] - 2026-09-25

### Security

Seven defects found by the E17 adversarial pass. Each was fixed with a
regression test verified to fail against the pre-fix code.

- **F1 — credential leak into errors and traces.** A non-2xx response body was
  embedded in the error verbatim, so an echoing Gateway could push a password
  into the error and from there into a span. Body snippets are now masked.
- **F2 — ADR 0003 bypass through route aliases.** `IsMutation` did an exact
  lookup, so `/trade/TradeEntrustRequest` classified as a retryable query. All
  three alias forms of all twelve mutation paths are now classified as
  mutations and tested.
- **F3 — unbounded cursor walks.** `AccountService` pagination had no
  stalled-cursor guard, page cap, or page-size clamp.
- **F4 — reconnect storm.** `maxRetries == 0` meant infinite (measured 53
  dials where 11 were expected), and the dialer received an empty address after
  a read failure.
- **F5 — unredacted account identifiers.** `accountid` and `fundAccount` were
  not redacted, contradicting ADR 0009.
- **F6 — raw errors in spans.** `EndSpan` recorded `err.Error()` verbatim.
- **F7 — freshness never populated.** `FreshnessMonitor.Record` returned early
  on `seq == 0` and `seqOf` always yields 0, so staleness detection could never
  fire. Both freshness tests failed before the fix.

Two further defects surfaced while repairing CI:

- **`Quantity.ValidateLot` integer overflow.** A `uint64` lot above `MaxInt64`
  wrapped into a negative modulus.
- **A test that asserted nothing.** `client/hardening_test.go` contained an
  empty branch.

### Fixed

**CI had been red on `main` since E15, and v0.1.6 shipped while red.** A single
early failure had masked everything after it. All nine jobs are green.

- A gofmt gate failing on three files skipped vet, tests, race, and the money
  check on every run since E15.
- 39 golangci-lint findings, now zero.
- The security job installed gosec from `honnef.co/go/tools` — staticcheck's
  module, which does not exist — so the step failed and `govulncheck` never ran.
  Both scanners are now pinned.
- `.goreleaser.yaml` did not parse, and separately set `main: .` on a library
  root and forced the `otel` tag on every build. It now builds only the mock
  Gateway, and unverifiable signing was removed rather than left to fail a
  release.
- 14 standard-library vulnerabilities, fixed by the `go1.26.6` toolchain.
- buf normalises comments differently for CRLF and LF input, so `gen/` produced
  on a Windows checkout never matched a Linux regeneration and `proto-verify`
  could not pass. `.gitattributes` now forces LF checkouts, which also makes a
  local `gofmt -l .` match CI.

### Added

- **`docs/threat-model.md`** — assets, trust boundaries, adversaries, seven
  scenarios, and a 14-entry risk register separating fixed from accepted and
  open.
- **Adversarial test coverage** for OTel attribute leakage, token replay, clock
  drift, and stale-data handling.
- **`ARCHITECTURE.md`** — a graph-derived architecture map (6,382 nodes, 219
  clusters, 518 execution flows) with 26 functional areas, five traced
  execution flows, a Mermaid diagram, and layering deviations verified against
  source rather than trusted from the graph.
- **GitNexus agent wiring** in `AGENTS.md`, `CLAUDE.md`, and six repository-local
  skills.

### Changed

- **Agent instructions.** The GitNexus `MUST`/`NEVER` rules were unsatisfiable:
  the index is a local, git-ignored artifact, so a fresh clone has none, and a
  stale MCP server fails every graph read while the CLI works. The rules now
  apply whenever the graph can answer, with an explicit fallback that requires
  saying graph analysis was unavailable rather than skipping silently.
- **`Makefile`.** `goreleaser-check` no longer runs the `go install …@latest`
  path that fails under Go 1.26; the Python default auto-detects `python3` then
  `python`; `fmt-check` drops a CRLF workaround that `.gitattributes` made
  obsolete; a stale header claiming proto, docs, and mock-Gateway targets were
  unimplemented was corrected.
- **Documentation accuracy.** All six READMEs claimed ADRs 0001-0007 and omitted
  the v-next layer from their package layouts. `docs/DESIGN.md` was present but
  absent from the MkDocs nav. The run index carried wrong close-out SHAs. The
  v-next boundary document asserted four rules that are false in code; it is now
  marked superseded and carries a verified deviations table.
- New run `2026-09-25-hstong-agent-readiness` tracks the reconciliation.

### Known limitations

Unchanged, and recorded rather than fixed: the v-next layer is still not
reachable by a caller, so its hardening protects nobody today; the Release
workflow runs `goreleaser check` on a tag and never publishes artefacts; and no
live Gateway integration run has ever executed.

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

[Unreleased]: https://github.com/shing1211/hstongapi4go/compare/v0.1.13...HEAD
[0.1.0]: https://github.com/shing1211/hstongapi4go/releases/tag/v0.1.0
[0.1.1]: https://github.com/shing1211/hstongapi4go/releases/tag/v0.1.1
[0.1.2]: https://github.com/shing1211/hstongapi4go/releases/tag/v0.1.2
[0.1.3]: https://github.com/shing1211/hstongapi4go/releases/tag/v0.1.3
[0.1.4]: https://github.com/shing1211/hstongapi4go/releases/tag/v0.1.4
[0.1.5]: https://github.com/shing1211/hstongapi4go/releases/tag/v0.1.5
[0.1.6]: https://github.com/shing1211/hstongapi4go/releases/tag/v0.1.6
[0.1.7]: https://github.com/shing1211/hstongapi4go/releases/tag/v0.1.7
[0.1.8]: https://github.com/shing1211/hstongapi4go/releases/tag/v0.1.8
[0.1.9]: https://github.com/shing1211/hstongapi4go/releases/tag/v0.1.9
[0.1.10]: https://github.com/shing1211/hstongapi4go/releases/tag/v0.1.10
[0.1.11]: https://github.com/shing1211/hstongapi4go/releases/tag/v0.1.11
[0.1.12]: https://github.com/shing1211/hstongapi4go/releases/tag/v0.1.12
[0.1.13]: https://github.com/shing1211/hstongapi4go/releases/tag/v0.1.13
