# Next-Phase Plan — hstongapi4go

- **Run:** `2026-09-25-hstong-agent-readiness`
- **Task:** follow-up planning
- **Status:** planning only — no code changed for this document.
- **Predecessor:** `report.md`, `plan.md`, `todos.md` (16/16), `../../threat-model.md`

> Paths are inline code rather than relative links so `scripts/check_links.py`
> cannot be broken by this artifact.

## 1. The one decision that gates the most work

**N1 was decided on 2026-09-25: Option A, commit to the layered API.** See
`../../../VNEXT.md` §4 for the decision and §6 for the ordered programme. The
question below is retained as the record of what was decided and why.

The v-next layer — `pkg/domain`, `pkg/services`, `pkg/transport`,
`internal/auth` — is complete, tested, and **not reachable by a caller**. Only
`scripts/coverage_gate.go` imports it. Meanwhile the released surface uses the
v0.1.x stack exclusively. The repository therefore contains two
implementations of the same SDK, and only the older one ships.

Two defensible outcomes:

1. Wire it in behind an opt-in constructor, released surface as the default.
2. Delete it and keep the released stack.

What is not defensible is leaving both indefinitely: the next maintainer cannot
tell which is authoritative, and the deviations table in the enterprise run's
`ARCHITECTURE.md` will rot again.

**Cheap forcing function.** Draft the v-next migration guide. If it cannot be
written coherently, that is evidence to delete rather than expose. If it can,
the migration cost is knowable and wiring it in becomes a normal engineering
decision. This is the highest-value hour available, because it determines
whether the test work in §3 is investment or waste.

**Outcome.** The guide was written (`e0c0b24`) and it could be written
coherently, which is why Option A was available as a normal engineering choice.
Writing it also disproved the assumption that push consolidation could proceed in
parallel: `Manager`, `Fanout`, and `Normalizer` were 1,117 lines that were both
v-next-coupled and unreachable, so their fate depended entirely on the decision.
That evidence is recorded in `../../../docs/VNEXT.md` §5, and the decision
retired `Manager` and `Fanout` while keeping `Normalize` as the decoder.

## 2. Active-path risks, no dependencies

All four sit on the shipped request path and need no decision from anyone.
**All four were actioned on 2026-09-25**; the register in `../../threat-model.md`
is authoritative for current status.

| Ref | Sev | Item | Outcome |
|-----|-----|------|---------|
| **R2** | **High** | `internal/resilience.mutationSet` is a closed set of 12 paths maintained separately from `client`'s 51 route constants. A mutation route missing from it is **retried**, breaking ADR 0003 | **Fixed** (`3926655`). Exhaustive allowlist in `client/route_mutation_test.go`, a 3x12 alias matrix, and two-direction over-classification. Verified the guard fires by removing an entry and watching it fail |
| R7 | Medium | `internal/transport/transport.go:187` used unbounded `io.ReadAll`; the cap existed only in the unused v-next adapter | **Fixed** (`de933f5`). `WithMaxResponseBytes`, default 8 MiB, reusing the reviewed `MaxBytesReader` pattern. Verified the test fails pre-fix. `gitnexus` rates it **critical / 33 processes** because every call flows through `Do`; verified with the all-51-route e2e suite |
| R3 | Medium | No `SetReadDeadline` anywhere in `internal/push/`; a half-open connection blocks a goroutine | **Partially fixed** (`a3e4655`). `WithReadDeadline` arms a per-read deadline, **off by default**: the Gateway documents no heartbeat cadence on this stream, so a non-zero default risks reconnect churn. Closing it needs the observed inter-frame gap (G6). The released `Client` only *receives* heartbeats, so there is no write-side heartbeat liveness to repair here |
| R11 | Medium | No secret scanning in CI | **Fixed** (`a238771`). ADR 0012, then a pinned `gitleaks` Action over full history, allowlisting three paths by justification and never a rule class. `go.mod` untouched |

**Test-design note for R2.** `docs/SPEC.md` does not mark mutations, so
expectations cannot be derived from it, and no naming rule catches every
conceivable future mutation. The defence is therefore the exhaustive allowlist:
walk `client.Routes()` and fail if any route is in neither the
expected-mutation nor the known-safe list, so adding a route forces a
conscious classification decision.

**Incidental defect found while writing the `pkg/transport` mapper tests**
(`c6b5226`). `MapTradeDeliveryToTradeEvent` passed raw protobuf strings into
`domain.MustNew*`, which panic on unparseable input. An unset field is empty, so
a zero-value delivery notification panicked, and there is no `recover()`
anywhere in the push or stream path, so that would have killed a consumer
goroutine and the process. Latent rather than live, because the mapper is in the
unwired layer. A non-numeric value still panics by design: `domain` exposes no
non-panicking constructor, and substituting a number for a malformed price would
be worse than failing.

## 3. Test and CI debt

| Item | State | Note |
|------|-------|------|
| `pkg/services` direct tests | **1,187 untested lines** (`market.go` 513, `trading.go` 674); only `account_test.go` exists | **Gated on §1** — wasted if the layer is deleted |
| `pkg/transport/mappers.go` | **Done** (`c6b5226`). All mappers covered, including nil-safety; package coverage 58.8% → 88.7% | — |
| `otel`-tag test job | **Done** (`5a14c99`). Builds, vets, and tests with the tag on every push, not only on a release tag | — |
| Bounded fuzz job | **Done** (`5a14c99`). 30s on `FuzzReadFrame`; verified locally at 734k executions, 18 newly interesting inputs, no crash | — |
| Coverage gate scope | **Done.** Gate covers ten packages: the released public surface `pkg/hstong` 90.7%, `stream` 85.6%, `trade` 86.3%, `algo` 87.5%, `pkg/types` 100.0%, `pkg/transport` 100.0%, alongside `pkg/domain` 93.8%, `internal/auth` 94.2%, `internal/transport` 97.6%, `internal/push` 93.9%. Every package in the release path is now gated. Figures are from a clean Linux checkout; see the correction below | — |
| `internal/push` coverage correction | **Done.** The 86.1% previously recorded here was measured in a dirty working tree and was never reproducible. On a clean checkout the package sat at **83.3%, below the 85% gate**, which is why CI failed on it. The gap was `Run`'s reconnect paths, which the suite only reached when a dial attempt happened to fit inside a test deadline, so the total swung 83.3–86.5% between runs of the same commit. `internal/push/client_paths_test.go` now covers those paths deterministically — option normalization, the three `Connect` interleavings, the `Run` dial-failure path, drop-oldest on both queues, and the `writeFull` short-write paths — taking the package to 93.9% with a ~9pp margin. Lesson recorded: measure coverage from a clean checkout, never a working tree | — |

## 4. The Option A programme

N1 chose Option A on 2026-09-25, so this section is no longer speculative. The
ordered steps, with the evidence for each, are tracked in `../../../VNEXT.md` §6.
In summary, in order:

- Steps 1 and 1b — **done.** The push implementation is `Client`; `Manager` and
  `Fanout` are retired, `Normalize` is kept as the decoder, and
  `FreshnessMonitor` was lifted into its own file. The deciding evidence was
  that the two implementations used *different protocols*: the released path
  subscribes over HTTP and re-subscribes over HTTP on reconnect, while `Manager`
  sent topic frames over the TCP push socket — an assumption no test had ever
  checked, because the live Gateway run has never executed.
- Step 2 — **cancelled, and the Adapter removed instead.** Reading
  `client.Client.Do` and `Adapter.Do` side by side showed the rewire would have
  dropped rate limiting, circuit breaking, metrics, and tracing from every
  v-next call: `Adapter` is a second, thinner pipeline, not a richer layer. It
  had no production caller, discarded the transport it was constructed with, and
  its `WithDeadline` cancelled other callers' in-flight contexts. Removed rather
  than repaired, which resolved R9 and took `pkg/transport` from 88.7% to 100%.
  Evidence in `../../../docs/VNEXT.md` §5.4.
- Step 3 — add **opt-in** correlation-ID injection to `client`. The one
  capability the Adapter had that the live path lacks; it is wire-visible, so it
  must default off under ADR 0011.
- Step 5 — **done.** `pkg/services` now depends on an `Executor` interface it
  declares itself, with `*client.Client` injected, which is ADR 0010 rule 6 and
  needs no second pipeline. The constructors and their `With*Client` options take
  the interface, so the layer is testable against a fake instead of a Gateway.
- Step 6 — **done, but not with depguard.** depguard in golangci-lint v2.9
  silently ignores path globs in `files`, so scoped rules enforce nothing while
  appearing to. Replaced with `internal/layering`, a test that parses the
  repository's imports and asserts six boundary rules, and which also checks
  that no rule matches zero packages. Verified by planting a violating import.
  Evidence in `../../../docs/VNEXT.md` §5.6.
  - R14 — **done, then fixed.** Recorded here first as a correction on
    2026-09-25: `doc.go` was accurate and the register was wrong to claim it
    advertised unimplemented behaviour, so the entry was re-scoped to say the
    primitives exist with no non-test caller. That was correct on 2026-09-25 and
    is superseded: `3b38e43` then implemented the composition, and `53b3a2e`
    closed the register entry and recorded the two-exclusion layering. See §5a.
- Step 8 and step 1c — **done.** `pkg/transport` is at 100% and `internal/push`
  at 93.9%; both are gated. The gate now covers ten packages, and every package
  in the release path is covered. `internal/push` needed a second pass: the
  original 86.1% was a dirty-tree measurement, and the real figure was 83.3%,
  under the gate. See the correction in §3.
- Then: `depguard` boundary rule, `pkg/services` tests, the migration guide,
  and the v1.0 schedule.

## 5. Blocked on credentials or a decision

- **G6 — live Gateway integration run.** `test/integration` is env-gated and
  has never executed. The only item that can *retire* an assumption rather than
  tighten a guarantee: ADR 0007's "market `int64` arrives as a JSON number" is
  inferred solely from the vendored Java and Python SDKs. Needs a test account.
- **G4 — release publishing.** **Done for GitHub on 2026-09-26** (v0.1.12); the
  **Gitee mirror remains**. v0.1.12 is the first release with artifacts: six
  archives, six SPDX SBOMs, a signed checksums file, and a `.sigstore.json`
  bundle, all verified with `cosign verify-blob` against the tag ref plus
  `sha256sum --check`, with the wrong-identity case confirmed to fail.
  Getting there took two attempts, and both failures were worth recording:
  - **v0.1.7 through v0.1.10 shipped no artifacts at all.** The workflow only
    ran `goreleaser check`, which validates configuration without running the
    pipeline, so four green tags were not evidence the release path worked.
  - **v0.1.11 ran `goreleaser release` for the first time and failed** on
    `exec: "syft": executable file not found`. The `sboms` block needs the
    external `syft` binary, which GoReleaser does not bundle and the workflow
    never installed — the same blind spot as above, one level deeper. Fixed by
    installing `syft` (version pinned) alongside `cosign`.
  - A `.goreleaser.yaml` fix **cannot** be recovered by re-dispatching the failed
    tag, because the `tag` input also selects the commit the config is read
    from. v0.1.11 therefore needed a new tag, not a retry. The checklist now
    says so.
  - The **Gitee release mirror** is not merely awaiting a token. GoReleaser
    publishes releases to GitHub, GitLab, and Gitea only — it accepts exactly one
    of those three in `release`, and a `gitee:` key fails validation with
    `field gitee not found in type config.Release` (verified on goreleaser
    v2.18.2). Gitee is a separate host, not a Gitea deployment GoReleaser can be
    aimed at, so mirroring artifacts means calling the Gitee REST API from a
    hand-written workflow step. That is a supply-chain and failure-mode decision
    deserving its own ADR, and it still needs a `GITEE_TOKEN` API secret, which
    does not exist — `gh secret list` reports no Actions secrets at all. Being
    able to `git push` the `gitee` remote proves nothing here: those credentials
    authenticate the git protocol, not the API. **Source is mirrored to Gitee
    with every tag; only the release artifacts are GitHub-only.**
- **Confirm the CI run is green.** **Done 2026-09-26** for v0.1.7 through
  v0.1.10. The `gh` token is still invalid, but the repository is public, so the
  unauthenticated GitHub API answers without one.

## 6. Housekeeping

- Rotate the MiniMax API key held in plaintext at
`~/.config/opencode/opencode.json` if it has been synced anywhere. The key is
still present; that directory's own `.gitignore` now excludes the file, and it
is not a git repository.
**Re-verified 2026-09-26 (F2): no exposure found.** The key is not in this
repository or in any commit of its history, and `C:\Users\Tchan\.config\opencode`
is not a git repository and lists `opencode.json` in its own `.gitignore`. A
`git log --all -S eyJ` sweep returns exactly one hit, and it is a `go.sum` module
checksum (`...+yeyJt4xig9DEw9kuUFe5C3zLbVjV2PzT6qzbs=`) introduced by `c592072`,
not a credential. This is therefore **housekeeping against a hypothetical copy,
not an incident**, and it is recorded as such so a later reader does not infer a
leak from the phrase "rotate the key". If that config file has never been copied,
synced, or backed up anywhere, the rotation is optional hygiene and need not
block a release.
- Native-speaker pass on the three English package-tree comments in the five
  translations.
- ~~Restart the four stale `gitnexus mcp` processes.~~ **Done** — they were
  restarted and the MCP tools answer again.

## 7. Recommended order

**Completed on 2026-09-25:** §2 in one pass (`3926655`, `de933f5`, `a3e4655`,
`a238771`) and the three independent rows of §3 (`c6b5226`, `5a14c99`). The
forcing function in §1 was drafted at `../../../docs/VNEXT.md` (`e0c0b24`), and
**N1 was then decided: Option A**. §4 is no longer gated on anything but the
programme's own order.

What remains, in order:

1. **§4 step 7 — test `pkg/services`.** `market.go` 579 and `trading.go` 780
   lines sit at ~4% coverage. The `Executor` interface landed first specifically so
   these can be written against a fake rather than a live Gateway, so they are not
   written twice.
2. **§4 steps 8-9**: the migration guide and the v1.0 schedule, plus a full
   `ARCHITECTURE.md` regeneration, since the derived diagram still shows the
   removed push implementations.
3. **§5** whenever credentials arrive. G6 should be scheduled early because its
   findings could change §4 — it is also the only way to finish R3.

The coverage gate was widened ahead of the decision, on released public surface
only, precisely so those tests would survive either N1 outcome. `internal/push`
and `pkg/transport` were left out at that point because the v-next decision could
have deleted most of them; with the decision made, both are gated and the gate
covers every package in the release path.

## 8. Documented acceptances — not to be fixed

| Ref | Item | Why |
|-----|------|-----|
| R1 | No client-side replay defence | The protocol defines no nonce and no idempotency key; adding one would break ADR 0003 compatibility |
| R4 | Dedup and gap detection inert | The Gateway sends no per-event sequence, so correct gap detection is impossible |
| R5 | Free-prose secrets not redacted | Redaction is key-driven; callers must keep secrets out of message text |
| R6 | Backward clock jump can revive a token | Would need a monotonic deadline; low impact because the Gateway independently rejects an expired token |
| R10 | `passwordHash` and bare `account` not redacted | `IsSensitiveKey` matches whole names and `account` is a common word; chosen so support correlation keeps working |

R8 (HTTP 5xx not retryable) is open but should stay that way: changing it would
alter documented retry semantics, and the Gateway reports overload as `1011`
inside a 200 envelope.

## 9. Standing open items, re-derived 2026-09-29

Sections 1 to 8 are written from the perspective of the session that produced
them, and several items they still show as outstanding have since been closed.
This section is the current list, re-derived from the tree rather than inherited
from the prose above. An item is listed as blocked only when the blocker is
named, and a declined item is a decision rather than a deferral: both carry a
date, so a reader can tell an oversight from a judgement.

| Ref | Item | Status | Blocker or note |
|-----|------|--------|-----------------|
| R14 | `internal/auth` single-flight and refresh un-composed | **Closed** (`3b38e43`, then `53b3a2e`) | Fixed, not merely disclosed. Two exclusions are kept deliberately: `SessionService.loginGate` owns production coalescing, `TokenManager.beginLogin`/`endLogin` is defence in depth that production traffic does not reach. `pkg/services/session_test.go` asserts the properties, and `TestAuthDocCallerClaimsAreAccurate` keeps the register and `doc.go` honest |
| G8 | SHA-1 is platform-mandated and weak | **Closed** ([ADR 0013](../../adr/0013-sha1-push-verification.md)) | The Gateway signs with `SHA1WithRSA` and the SDK cannot choose otherwise. Previously recorded only as a residual-risk line in `P10-hardening.md:61`; promoted to a standing ADR. No behaviour change |
| money-check | The `spreadLevel` waiver outlived v1.0.0 | **Closed** (`f28eccd`, v1.0.1) | The waiver's own terms said delete it rather than re-date it, so `OrderBookResponse.TickSize` is now `json.Number` and the `WAIVERS` table is empty. The table is kept so the next entry cannot be waved through |
| N5 | Test debt: coverage, fuzz job, `otel` job, wider gate | **Closed** (`5a14c99`) | Both CI jobs exist: `ci.yml:232` fuzzes `FuzzReadFrame` for 30s and `ci.yml:211` builds, vets and tests with the `otel` tag. The gate covers eleven packages |
| **G6** | **Live Gateway integration run** | **Blocked: needs a test account** | `test/integration` is env-gated and has never executed. The only item that can *retire* an assumption rather than tighten a guarantee: ADR 0007's "market `int64` arrives as a JSON number" is inferred solely from the vendored Java and Python SDKs |
| **R3** | **Push read deadline stays off by default** | **Blocked behind G6** | `WithReadDeadline` is implemented and off. A non-zero default risks tearing down a healthy connection because the observed inter-frame gap is unknown, and measuring that gap is exactly what G6 would do |
| **R8** | **HTTP 5xx not retryable** | **Open, and intended to stay** | Changing it would contradict documented retry semantics, and the Gateway reports overload as `1011` inside a 200 envelope. This is a disposition, not a defect |
| **G4** | **Gitee release mirror** | **Declined 2026-09-29** | Not a blocker but a decision, and a narrow one. Source and every tag are already mirrored to Gitee, so the only gap is the release artifacts: six archives, six SBOMs, the signed checksums file, and the cosign bundle. GoReleaser cannot close it, because it publishes to GitHub, GitLab and Gitea only and a `gitee:` key is rejected with `field gitee not found in type config.Release`. Gitee is a separate host, not a Gitea deployment GoReleaser can be aimed at, so the only route is a hand-written workflow step calling the Gitee REST API. That is a new credential and a new supply-chain surface, in exchange for download convenience rather than a security boundary. `docs/RELEASE_CHECKLIST.md` step 17 already records the gap as "not expected", so declining aligns the backlog with the checklist rather than contradicting it. Anyone who later wants the artifacts there should treat it as a new change with its own ADR, not as resuming this row |
| **P5-3** | **`TickSchedule` interface and rewire** | **Done 2026-09-30** | `validatePriceForHK` consulted the schedule only as a boolean and then called `Price.Validate(true)`, which tests a price against the tick the caller attached to it — so the check could not fail for a reason the caller had not already accepted. It now resolves the instrument's tick through `domain.TickScheduleResolver` and validates against that. Step 3 of `design-tick-model.md` §5 |
| **P5-4** | **HK price bands and non-HK defaults** | **Blocked: needs the HKEX document** | The band table must be sourced from the exchange, not from memory, and inventing the bands is the exact defect the task exists to remove. The default table is deliberately DataType-keyed and approximate until then. The rewire is in place and waiting for real data |
| **P5-5** | **The `spreadLevel` naming test** | **Blocked behind G6** | `/hq/OrderBook` twice, seconds apart: constant across replies while the book moves means tick, tracking the level-1 spread means spread. Decides whether `TickSize` is renamed in `pkg/services` at v1.0. `design-tick-model.md` §3.1 |
| **C1** | **`ChangeEntrust` validates every amend as an HK stock** | **Open: needs a `Symbol` parameter** | `ChangeEntrust` hardcodes `types.DataTypeHKStock` at `pkg/services/trading.go`, so a US or HK-ETF amend is checked against the HK-stock lot and tick. It takes no `domain.Symbol`, so the real DataType is unobtainable and a DataType-keyed schedule cannot be resolved for it. Fixing it is a public API change on the v-next layer, which is free before v1.0 and expensive after. Found while implementing P5-3 |
| **C2** | **`changeEntrustWireRequest.StockCode` is never set** | **Open: joins G6** | The struct carries `StockCode` and `docs/SPEC.md:143` documents it as "Security code", but the literal built in `ChangeEntrust` never populates it. Whether the Gateway requires it is not decidable from this repository — the mock is a stub and the vendored SDKs do not settle it. One live request answers it |

Four of the remaining items need something this environment does not have: a test
account for G6, which P5-5 and C2 are also waiting on; and the HKEX price-band
document for P5-4. C1 needs a public API change on the v-next layer, which is a
decision rather than an input. Recording these as blocked is the honest state,
not a deferral.
