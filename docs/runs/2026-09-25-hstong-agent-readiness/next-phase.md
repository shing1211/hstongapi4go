# Next-Phase Plan — hstongapi4go

- **Run:** `2026-09-25-hstong-agent-readiness`
- **Task:** follow-up planning
- **Status:** planning only — no code changed for this document.
- **Predecessor:** `report.md`, `plan.md`, `todos.md` (16/16), `../../threat-model.md`

> Paths are inline code rather than relative links so `scripts/check_links.py`
> cannot be broken by this artifact.

## 1. The one decision that gates the most work

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
| Coverage gate scope | Still three packages. Released surface ungated, and **five packages below 85%**: `pkg/hstong` 84.7%, `algo` 81.2%, `trade` 81.3%, `internal/push` 79.9%, `stream` 76.3% | **Deferred, partly gated on §1** — `internal/push` holds three implementations, two of which §1 may remove. 2–4 days of test writing |

## 4. Gated on §1

Once the v-next decision is made:

- R9 — `pkg/transport.Adapter` is unreachable and mis-wired: `inner` unused,
  base URL hardcoded, and `WithDeadline` stores one shared cancel that can
  cancel a previous caller. A trap for the next adopter.
- R14 — `internal/auth` documents backoff, single-flight, and refresh that do
  not exist; `defaultRetryDelay`, `defaultMaxRetryDelay`, and
  `Authenticator.mu` are unused.
- N2 — consolidate the three overlapping `internal/push` implementations
  (`client.go` 612, `manager.go` 506, `fanout.go` 346 = 1,464 lines), which
  disagree about reconnect bounds, backoff, and freshness.

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
forcing function in §1 is drafted at `../../../docs/VNEXT.md` (`e0c0b24`); the
decision itself is still unmade.

What remains, in order:

1. **Read `docs/VNEXT.md` and make the N1 call.** The draft reaches the evidence
   and lands on Option A being the better product, while being a multi-release
   programme. Everything in §3's coverage row and all of §4 waits on it.
2. **Widen the coverage gate** to the released surface, 2–4 days, after N1 so the
   tests are not written against code that gets deleted.
3. **§4** immediately after N1 resolves: R9's adapter rewiring or removal, R14's
   documented-but-absent auth behaviour, and push consolidation (N2).
4. **§5** whenever credentials arrive. G6 should be scheduled early because its
   findings could change both §1 and §4 — it is also the only way to finish R3.

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
