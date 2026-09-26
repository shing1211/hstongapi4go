# Adopting the v-next layer

- **Status:** **N1 decided 2026-09-25 — Option A, commit to the layered API.**
  No code changed for the decision; the programme is tracked in §6.
- **Purpose:** the forcing function for the N1 decision recorded in
  `runs/2026-09-25-hstong-agent-readiness/next-phase.md` §1, and the record of
  what was decided.
- **Reader:** whoever implements the layered API, and whoever maintains the
  released flat API until v1.0.

## 1. The question

The repository contains two implementations of the same client:

| | Released layer | v-next layer |
|---|---|---|
| Packages | `client`, `pkg/hstong/*`, `internal/*` | `pkg/domain`, `pkg/services`, `pkg/transport`, `internal/auth` |
| Money | `string` / `json.Number` | `decimal.Decimal` (`Money`, `Price`, `Quantity`, `Rate`) |
| Errors | `internal/errs` typed status errors | same, reused |
| Reachable by a caller | **yes** | **no** — only tests and the coverage gate import it |
| ADR | [0001](./adr/0001-gateway-transport.md), [0007](./adr/0007-http-json-codec.md) | [0008](./adr/0008-decimal-financial-types.md), [0010](./adr/0010-vnext-layered-architecture.md) |

[ADR 0011](./adr/0011-v01x-compatibility.md) guarantees no breaking type or
wire changes to `pkg/hstong/*`, `pkg/types`, `client/`, or `internal/*` across
v0.1.x. Wiring v-next in as the default would break that guarantee. Wiring it in
as opt-in would leave two APIs shipping side by side indefinitely.

## 2. What the migration would look like

One representative endpoint, account balance, in both layers.

**Today (released):**

```go
c, err := client.New(client.WithEnv(""))
tradeMgr := trade.NewManager(c)

bal, err := tradeMgr.MarginFundInfo(ctx, trade.MarginFundInfoRequest{})
if err != nil {
    return err
}
// bal.AssetBalance is a string such as "125000.500".
total, err := strconv.ParseFloat(bal.AssetBalance, 64)
```

**With v-next:**

```go
c, err := client.New(client.WithEnv(""))
svc := services.NewAccountService(c)

bal, err := svc.MarginFundInfo(ctx, services.MarginFundInfoParams{})
if err != nil {
    return err
}
// bal.AssetBalance is a domain.Money backed by decimal.Decimal.
total := bal.AssetBalance.Add(bal.MarketValue).Decimal()
```

That is the whole migration for one call, and it is where the cost becomes
visible: **every arithmetic site in caller code changes**, because `string` plus
`strconv.ParseFloat` becomes a decimal type. It is also strictly better — the
released form rounds through `float64`, which is the exact class of bug
[ADR 0008](./adr/0008-decimal-financial-types.md) was written to eliminate.

## 3. What writing it actually revealed

Four obstacles, none of which is a bug and all of which are design consequences.

1. **There is no incremental path.** Adoption is per endpoint family, not per
   call. A caller can move account reads and keep trade reads, but then holds two
   error conventions, two pagination styles, and two money representations in one
   program. The layers do not compose at call granularity.

2. **No bridging adapter exists.** A shim that accepts `pkg/hstong` requests and
   returns `pkg/domain` values would make adoption incremental, and it does not
   exist. Writing one is roughly the same work as the migration itself, which
   means the "gradual path" argument does not reduce cost — it relocates it.

3. **The push layer has no single answer.** `internal/push` contains three
   implementations — `Client` (released, used by `pkg/hstong/stream`),
   `Manager`, and `Fanout` — which disagree about reconnect bounds, backoff, and
   freshness. Adopting v-next push means choosing one, and the choice is a
   rewrite of the stream surface, not a swap.

4. **`pkg/services` bypasses its own adapter.** It imports `client` directly, so
   `pkg/transport.Adapter`'s correlation IDs and retry classification sit unused
   on the very path the layer was built for. Adopting v-next as-is would adopt
   that bypass too, unless the services are rewired first.

## 4. The decision

**Option A — commit to the layered API — was chosen on 2026-09-25.** The
deliberation below is kept because it is the evidence for the decision, and
because the rejected options record why the obvious cheaper path was not taken.

The options as they stood:

- **Option A — commit to the layered API.** Write the migration guide as a real
  document, schedule a v1.0 with breaking type changes, rewire `pkg/services`
  through `pkg/transport.Adapter`, and consolidate the push implementations
  first. A multi-release programme, not a patch.
- **Option B — delete the v-next layer.** Keep the released stack, delete
  `pkg/domain`, `pkg/services`, `pkg/transport`, and `internal/auth`, and record
  the decimal-money and layering decisions as superseded.
- **Option C — leave it unwired (status quo).** Acceptable only with a deadline
  and an owner.

Option A was chosen because decimal money is a real correctness win, and because
the push layer has to end up with one implementation whichever way this goes.
That second point is sharper than the original draft implied: because
`Manager`, `Fanout`, and `Normalizer` are v-next-coupled, the two options
required *opposite* work on the same files — Option B would have deleted them,
Option A consolidates them. Push work therefore could not be de-risked as
shared groundwork in front of the decision. It is sequenced first inside the
programme instead. §5 records the evidence.

**Option C is now foreclosed.** Leaving the layer unwired without an owner and a
deadline is what made this repository ambiguous in the first place; a positive
decision is strictly better than the status quo.

## 5. Push implementation status

**Decided 2026-09-25: `Client` is the single push transport. `Manager` and
`Fanout` are retired. `Normalize` is kept as the decoder, and
`FreshnessMonitor` is lifted out of `Fanout` as a standalone component.**

`internal/push` held three push implementations, and until this table existed
the choice between them had to be inferred from which file a symbol happened to
live in. Reachability was measured two ways — a `gitnexus` upstream walk and an
exhaustive text search for every exported constructor and type — and both agree.

| Implementation | File | Lines | Imports `pkg/domain` | Reachable from a caller | Status |
|---|---|---|---|---|---|
| `Client` | `client.go` | 708 | no | **yes** — owned by `pkg/hstong/stream` (`stream.go:118`) | **Kept.** The push transport. |
| `Normalize` | `normalizer.go` | 135 | yes | no (library) | **Kept.** The `PBNotify` → `domain.PushEvent` decoder. |
| `FreshnessMonitor` | `fanout.go` | ~50 | yes | no (library) | **Kept, lifted** into its own file. |
| `Manager` | `manager.go` | 594 | yes | **no** | **Retire.** `gitnexus`: 0 affected processes; only its own `NewManager`. |
| `Fanout` | `fanout.go` | 388 | yes | **no** | **Retire.** All upstream refs are inside `fanout.go` itself. |

`frame.go` (236) and `verify.go` (139) are not alternatives: they are the wire
framing and signature verification all implementations share, and they import no
v-next package.

### 5.1 The finding that decided it

`Manager` is not a second implementation of the same protocol. It implements a
**different one**, and the difference is load-bearing:

| | Released path | `Manager` |
|---|---|---|
| Subscribe | **HTTP** `POST RouteHqSubscribe` on :11111 (`market/subscribe.go:62`) | **TCP** topic-request frame on :11112 (`manager.go:382`) |
| Receive | TCP :11112 | TCP :11112 |
| Re-subscribe on reconnect | **HTTP**, per subscription (`stream.go:419`) | TCP, `resubscribeAll()` |
| Keep-alive | Receives heartbeats only; sends none | Sends heartbeats (`heartbeatLoop`) |

ADR 0001 defines the target as HTTP on :11111 for requests and TCP on :11112
for push. `Client` plus `market.Manager` is the only pair that matches it.
`Manager` assumes the Gateway accepts topic subscriptions on the push socket —
an assumption **no test has ever checked**, because `test/integration` has never
been executed against a live Gateway (G6).

Adopting `Manager` would therefore have made an unverified protocol guess the
foundation of the layered API, and would have discarded the re-subscribe
behaviour the released path demonstrably relies on.

### 5.2 Why the rest can go

`Fanout`'s capabilities split three ways, and only one is worth keeping:

- **Multi-subscriber multiplexing and drop-oldest backpressure** already exist
  in `pkg/hstong/stream` (`Subscription`, `sendUpdate`). Keeping `Fanout` would
  be a second implementation of them.
- **`DedupCache` is inert.** The Gateway sends no per-event sequence, so
  `CheckAndInsert` cannot distinguish a duplicate (R4). It is documented dead
  code, pinned by `TestFanout_DedupInertWithoutSequence`.
- **`FreshnessMonitor` is genuinely new** and is time-based, so it works even
  though `seq` is always 0 — the E17-F7 fix exists precisely for that case. It
  has no dependency on the rest of `Fanout`, so it lifts out cleanly.

`Manager` also carries its own `decodePushEvent` and per-event decoders
(`manager.go:427-581`), duplicating `Normalize`. `Normalize` wins that
comparison: it is the one with a 439-line test file.

**One correction to the evidence above.** Deleting `manager.go` did not compile:
`normalizer.go` *calls* the five per-event decoders (`decodeBasicQotEvent`,
`decodeTickerEvent`, `decodeOrderBookEvent`, `decodeBrokerEvent`,
`decodeTradeEvent`) and `entrustBSFromInt32`, and all six lived in `manager.go`.
The reachability check answered "who calls `Normalize`" and found only its own
test, which reads as unused — but the dependency runs the other way. Those six
pure functions are now in `decode.go` beside the normalizer that needs them,
which is where they belonged. The lesson generalises: a symbol with no callers
can still be load-bearing, so a delete needs both directions checked.

`internal/push` after the retirement: `client.go` (708), `frame.go` (236),
`verify.go` (139), `normalizer.go` (135), `decode.go` (119), `freshness.go` (90).
Package coverage is 80.0%, essentially unchanged — the retired code was well
covered by the 5 test files that went with it, so the remaining gap is in
`client.go` and is unrelated to this decision.

### 5.3 Consequence for the test evidence

`test/integration/push_integration_test.go` is three `TestPushManager_*` cases
and nothing else, so retiring `Manager` retires that file. TCP protocol coverage
is **not** lost with it: `test/integration/integration_test.go:521` exercises
`stream.Client` `Connect` and `Subscribe` against the live Gateway, which is the
path that ships. G6 therefore still validates the released protocol — arguably
better than before, since it now validates only what callers reach.

### 5.4 Step 2 was the wrong step: the rewire would have been a regression

The plan called for rewiring `pkg/services` through `pkg/transport.Adapter`.
Reading both implementations before moving any code showed that would have made
things worse, not better.

| | `client.Client.Do` | `Adapter.Do` |
|---|---|---|
| Rate limiter | yes (`WaitEndpoint`) | **no** |
| Circuit breaker | yes, split query/mutation | **no** |
| Retry, mutation-guarded | yes | yes, but duplicated logic |
| Metrics | request, latency, error, order outcome | **no** |
| Tracing span | yes | **no** |
| Route validation | yes | **no** |
| Correlation ID | **no** | yes |

`Adapter` is not a richer layer sitting above `client.Client`. It is a **second,
thinner HTTP pipeline**. Pointing `pkg/services` at it would have dropped rate
limiting, circuit breaking, metrics, and tracing from every v-next call — the
layer the adapter was built to protect.

It was also broken in three ways that the existing tests did not catch:

- `NewAdapter` stored the `*internal/transport.Transport` passed to it in
  `inner`, and `executor()` then built a **new** Transport per request using a
  hardcoded `inettransport.DefaultBaseURL`. The caller's base URL, HTTP client,
  timeout, and logger were all silently discarded.
- `WithDeadline` stored a single `context.CancelFunc` on the Adapter and
  cancelled the previous one, so caller B's call cancelled caller A's in-flight
  context. It was also called from nowhere.
- The response-size cap it applied was already covered by
  `internal/transport`'s `WithMaxResponseBytes` after R7, making it a third copy
  of a mechanism that now has one home.

`Adapter` was therefore **removed** rather than repaired, which resolved R9 and
took `pkg/transport` from 88.7% to **100%** — the removed code was the uncovered
part. `pkg/transport/doc.go` was also corrected: it documented a `RESTAdapter`
and a `PushAdapter` that were never implemented.

One capability is genuinely worth carrying forward: **correlation-ID injection**
(`X-Correlation-ID`). The released path sets only `Content-Type` today. It is a
feature rather than a repair, it is wire-visible, and ADR 0011 means it has to be
opt-in — so it is tracked as its own step below rather than smuggled in here.

### 5.5 Correlation IDs, now on the live path

Step 3 is done. `client.WithCorrelationIDHeader(name)` sends a header named
`name` whose value is a fresh 32-character lowercase hex identifier per request.
The default is off, so an existing caller sends exactly what it sent before.

Three decisions worth recording, because each was a choice rather than an
obvious default:

- **`crypto/rand`, not `math/rand`.** The value lands in Gateway and proxy logs.
  A predictable identifier is useful to anyone trying to collide requests or
  forge a plausible one, so 128 bits from `crypto/rand` costs nothing to get
  right.
- **A header *name* is configured, not a fixed name.** A caller with an existing
  convention should not have to strip ours. A custom name does not also enable
  `X-Correlation-ID`, which is asserted.
- **A whitespace-only name disables it.** `net/http` rejects a header name
  containing invalid characters, so `WithCorrelationIDHeader("   ")` would turn a
  typo into every request failing. The name is trimmed and an empty result
  behaves as off.

The tests assert the properties that make the feature useful rather than just
its presence: ids are 32 lowercase hex characters, decode to 16 bytes, and are
**distinct across 25 sequential and 80 concurrent requests**. Uniqueness is the
whole point — a shared id would make a log line ambiguous, which is the failure
this feature exists to prevent. The header is also asserted to be present on a
request that is about to fail, since that is when correlating a log line matters
most. Ten tests in total; the transport-level ones were verified to fail with the
header-setting code removed.

### 5.6 The layering boundary is a test, not a depguard rule

Step 6 was specified as a `depguard` rule in `.golangci.yml`. It could not be
done that way, and the reason is worth recording because the failure is silent.

`depguard`'s `files` field, in golangci-lint v2.9, honours only the `$all` and
`$test` tokens. A path glob such as `pkg/hstong/**` is **accepted without
warning and then matches no files**, so a rule scoped that way reports nothing
forever. Five such rules were written, all of them correct as specifications, and
all of them silently enforced nothing — they reported "0 issues" both when the
graph was clean and when a deliberately planted `pkg/hstong` → `pkg/domain` import
was present. That is worse than no rule, because it looks like enforcement.

`internal/layering` replaces it. The test walks the module, parses each file's
imports, and asserts the boundary table directly. Two properties make it
trustworthy in a way the depguard version was not:

- It asserts the import walk found something, so a refactor that breaks the walk
  cannot turn it into a test that passes because it inspected nothing.
- It asserts every rule's prefix matches at least one real package, so a rule
  cannot pass by matching nothing — the exact failure depguard exhibited.

Verified by planting a `pkg/domain` import in `pkg/hstong/session.go`: the test
failed with `pkg/hstong imports pkg/domain`, and passed again once removed.

The six rules encode the graph as it exists, so they are regression guards rather
than aspirations. The one known deviation, `pkg/domain` importing `gen/hq/dto`, is
deliberately **not** encoded: it is real, it is recorded in `ARCHITECTURE.md` §6,
and encoding it would fail every build until it is fixed. Rules are added as
deviations close, not before.

## 6. Programme

Option A is a multi-release programme. Order matters: step 1 decides what
`pkg/services` will talk to, so it precedes the rewire in step 2.

| # | Step | Notes |
|---|---|---|
| 1 | ~~Decide the push implementation~~ | **Done.** `Client` kept, `Manager` and `Fanout` retired, `Normalize` kept, `FreshnessMonitor` lifted. Evidence in §5. |
| 1b | ~~Execute the retirement~~ | **Done.** Deleted `manager.go`, `fanout.go`, their 5 test files, and `test/integration/push_integration_test.go`. `FreshnessMonitor` lifted to `freshness.go` with its tests. R4's rationale rewritten, since `DedupCache` no longer exists. |
| 1c | ~~Re-gate `internal/push`~~ | **Done, in two passes.** 80.0% → 86.1% → **93.9%**. The gap was in `client.go`, not in what was removed. `entrustBSFromInt32` had 1 of 5 switch cases covered — it decides whether a fill is labelled buy or sell, so that was the priority — plus `Addr`, `exitCause`, `UnsubscribeTypes`, and `NormalizeUnknown` at zero. The first pass reported 86.1% from a dirty working tree and was wrong: on a clean checkout the package measured **83.3%, under the 85% gate**, because `Run`'s reconnect paths were only reached when a dial attempt happened to fit inside a test deadline, swinging the total 83.3–86.5% run to run. `client_paths_test.go` closes them deterministically, giving a ~9pp margin. The gate covers ten packages, every one in the release path. |
| ~~2~~ | ~~Rewire `pkg/services` through `pkg/transport.Adapter`~~ | **Cancelled, and the Adapter removed instead.** Reading both `Do` implementations showed the rewire would have dropped rate limiting, circuit breaking, metrics, and tracing from every v-next call. `Adapter` had no production caller and three defects. Evidence in §5.4. R9 resolved. `pkg/transport` went 88.7% → 100%. |
| 3 | ~~Add opt-in correlation-ID injection to `client`~~ | **Done.** `client.WithCorrelationIDHeader` and `internal/transport.WithCorrelationIDHeader`. Off by default, 32-char lowercase hex from `crypto/rand`, fresh per request. 10 tests, verified to fail with the header-setting code removed. |
| 4 | ~~Resolve R14~~ | **Done as a correction, not a rewrite.** `doc.go` turned out to be accurate — it already disclosed the missing behaviour. The register was wrong to claim otherwise. Re-scoped: `Session.ShouldRefresh`, `TokenManager.IsLoginInProgress`, `markLoginPending`, and `clearLoginPending` exist with **no non-test caller**, so single-flight and refresh are un-composed. `Authenticator` does check `IsExpired`, so expiry detection exists. `doc.go` now says so explicitly, so a reader does not mistake the methods for live behaviour. |
| 5 | ~~Decide the `pkg/services` request path~~ | **Done.** `pkg/services` now depends on an `Executor` interface it declares itself, with `*client.Client` as the injected implementation — ADR 0010 rule 6, and no second pipeline. The three constructors and their `With*Client` options take the interface, so a fake can drive the layer without a Gateway. `pkg/services` still names `client.Route` and `client.Codec` in the signature: inverting the dependency removes the dependency on the concrete type, not on the client's shared route vocabulary, and private copies of those types would duplicate the canonical route table in `docs/SPEC.md`. |
| 6 | ~~Add a `depguard` boundary rule~~ | **Done, but not with depguard.** depguard in golangci-lint v2.9 honours only the `$all` and `$test` tokens in `files`; a glob like `pkg/hstong/**` is accepted silently and matches nothing, so the rule passes forever while enforcing nothing. Replaced with `internal/layering`, a test that parses the repository's own imports. It also asserts that every rule's prefix matches at least one real package, so a rule cannot pass by matching nothing. See §5.6. |
| 7 | Test `pkg/services` | `market.go` 579 and `trading.go` 780 lines sit at ~4% coverage. Written after the request-path decision, so they are not written twice. |
| 8 | ~~Re-gate `pkg/transport`~~ | **Done.** `pkg/transport` is at 100% after the Adapter removal and is now gated. |
| 9 | Publish the migration guide and schedule v1.0 | The guide in §2 is the draft; it becomes a supported document. Refresh the `ARCHITECTURE.md` deviations table at the same time — a full graph regeneration is due, since the derived diagram still shows the removed push implementations. |

ADR 0011 continues to hold for all of v0.1.x: none of these steps may change a
released type or wire shape before v1.0.
