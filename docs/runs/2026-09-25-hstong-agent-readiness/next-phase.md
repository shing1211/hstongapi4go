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
| Coverage gate scope | **Done.** Gate widened to eight packages: the released public surface `pkg/hstong` 90.7%, `stream` 85.9%, `trade` 86.3%, `algo` 87.5%, `pkg/types` 100.0%, alongside `pkg/domain` 93.8%, `internal/auth` 94.2%, `internal/transport` 99.1%. `internal/push` 80.0% remains ungated (step 1c) | — |

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
- Step 2 — rewire `pkg/services` through `pkg/transport.Adapter`. The
  highest-risk item and not mechanical: `pkg/services` holds a `*client.Client`
  and calls `s.client.Do(...)`, while `Adapter` wraps `internal/transport`, so
  the two sit at different layers and a narrow interface is needed.
- R9 — fix the Adapter's unused `inner`, hardcoded base URL, and shared
  `WithDeadline` cancel while it is in hand.
- R14 — rewrite `internal/auth/doc.go` to match reality rather than adding a
  second login implementation.
- Step 1c — re-gate `internal/push`, which is still 80.0% and whose gap is in
  `client.go` rather than in the retired code.
- Then: `depguard` boundary rule, `pkg/services` tests, the migration guide,
  and the v1.0 schedule.

## 5. Blocked on credentials or a decision

- **G6 — live Gateway integration run.** `test/integration` is env-gated and
  has never executed. The only item that can *retire* an assumption rather than
  tighten a guarantee: ADR 0007's "market `int64` arrives as a JSON number" is
  inferred solely from the vendored Java and Python SDKs. Needs a test account.
- **G4 — release publishing.** `release.yml` runs `goreleaser check` on a tag
  and never publishes, so v0.1.7 has a tag and no artefacts. Needs: does the
  Gitee token exist, and is cosign keyless OIDC acceptable?
- **Confirm the v0.1.7 CI run is green.** Unverified at tag time because the
  `gh` token was invalid on the release host.

## 6. Housekeeping

- Rotate the MiniMax API key held in plaintext at
  `~/.config/opencode/opencode.json` if it has been synced anywhere. Not a git
  repository today, and that directory's own `.gitignore` does not exclude the
  file.
- Restart the four stale `gitnexus mcp` processes (they predate the current CLI
  and fail graph reads with a storage-version mismatch). The CLI fallback works,
  so nothing is blocked meanwhile.
- Native-speaker pass on the three English package-tree comments in the five
  translations.
- Fix the misleading `enabled: !ENV [CI, false]` in `mkdocs.yml`: `!ENV`
  substitutes the string `"true"` when `CI` is set, so the plugin stays
  enabled, which is the opposite of how the line reads.
- Reword `proto/PROVENANCE.md`'s byte-for-byte-upstream claim; `.gitattributes`
  now normalises line endings to LF. Committed bytes did not change.

## 7. Recommended order

**Completed on 2026-09-25:** §2 in one pass (`3926655`, `de933f5`, `a3e4655`,
`a238771`) and the three independent rows of §3 (`c6b5226`, `5a14c99`). The
forcing function in §1 was drafted at `../../../docs/VNEXT.md` (`e0c0b24`), and
**N1 was then decided: Option A**. §4 is no longer gated on anything but the
programme's own order.

What remains, in order:

1. **§4 step 2 — rewire `pkg/services` through `pkg/transport.Adapter`.** Now
   unblocked, since step 1 established that the transport is `Client`. This is
   the highest-risk item in the programme: the two sit at different layers, so
   the interface shape has to be decided before any code moves.
2. **§4 steps 3-8**, in the order given in `../../../docs/VNEXT.md` §6.
3. **§5** whenever credentials arrive. G6 should be scheduled early because its
   findings could change §4 — it is also the only way to finish R3.

The coverage gate was widened ahead of the decision, on released public surface
only (`pkg/hstong*` and `pkg/types`). Those packages are live public surface, so
their tests survive either N1 outcome; the deliberate omission is
`internal/push`, now waiting on step 1c.

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
