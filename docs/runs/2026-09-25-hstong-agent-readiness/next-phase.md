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

| Ref | Sev | Item | Fix |
|-----|-----|------|-----|
| **R2** | **High** | `internal/resilience.mutationSet` is a closed set of 12 paths maintained separately from `client`'s 51 route constants. A mutation route missing from it is **retried**, breaking ADR 0003 | Exhaustive allowlist test in `client/` (the only package that can import both), plus a 3x12 alias matrix and two-direction over-classification checks |
| R7 | Medium | `internal/transport/transport.go:187` uses unbounded `io.ReadAll`; the cap exists only in the unused v-next adapter | Add a `maxBytes` option reusing the reviewed pattern at `pkg/transport/middleware.go:29-30,119`, replace `io.ReadAll`, test the over-limit path |
| R3 | Medium | No `SetReadDeadline` anywhere in `internal/push/`; `client.go:393` handles `MsgHeartbeat` on the receive side only, so a half-open connection blocks a goroutine and a failed heartbeat write never reconnects | Read deadline plus write-side liveness on the released `push.Client` |
| R11 | Medium | No secret scanning in CI | Small ADR (hard rule 8: any new dependency needs one; ADR 0006 is the precedent), then a pinned scanner with the bundled public platform keys allowlisted per ADR 0005 |

**Test-design note for R2.** `docs/SPEC.md` does not mark mutations, so
expectations cannot be derived from it, and no naming rule catches every
conceivable future mutation. The defence is therefore the exhaustive allowlist:
walk `client.Routes()` and fail if any route is in neither the
expected-mutation nor the known-safe list, so adding a route forces a
conscious classification decision.

## 3. Test and CI debt

| Item | State | Note |
|------|-------|------|
| `pkg/services` direct tests | **1,187 untested lines** (`market.go` 513, `trading.go` 674); only `account_test.go` exists | **Gated on §1** — wasted if the layer is deleted |
| `pkg/transport/mappers.go` | 86 lines, no test file | Independent |
| `otel`-tag test job | Absent from CI; only the release workflow builds it, only on tags | Independent |
| Bounded fuzz job | Absent; `FuzzReadFrame` runs seeds only | Independent |
| Coverage gate scope | Three packages. Released surface is ungated, and **five packages are below 85%**: `pkg/hstong` 84.7%, `algo` 81.2%, `trade` 81.3%, `internal/push` 79.9%, `stream` 76.3% | **Partly gated on §1** — `internal/push` holds three implementations, two of which §1 may remove |

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

1. **§2 in one pass** — it clears the only High-severity open risk and three
   more active-path items, none of which depend on a decision.
2. **§1 forcing function** next, before any §3/§4 test writing, so that writing
   is not speculative.
3. **§3's independent rows** (mappers test, `otel` job, fuzz job) can proceed
   alongside step 1.
4. **§4** immediately after §1 resolves.
5. **§5** whenever credentials arrive; G6 should be scheduled early because its
   findings could change §1 and §4.

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
