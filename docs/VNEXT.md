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

`internal/push` holds three push implementations, and until this table existed
the choice between them had to be inferred from which file a symbol happened to
live in. Reachability was measured two ways: a `gitnexus` upstream walk, and an
exhaustive text search for every exported constructor and type across the
repository. Both agree.

| Implementation | File | Lines | Imports `pkg/domain` | Reachable from a caller | Status |
|---|---|---|---|---|---|
| `Client` | `client.go` | 708 | no | **yes** — owned by `pkg/hstong/stream` (`stream.go:118`) | **Released.** The only implementation a caller reaches today. |
| `Manager` | `manager.go` | 594 | yes | **no** | Forward-looking, v-next. `gitnexus` reports 1 upstream edge, its own `NewManager`; 0 affected processes. |
| `Fanout` | `fanout.go` | 388 | yes | **no** | Forward-looking, v-next. No reference outside its own definition and its internal tests. |
| `Normalizer` | `normalizer.go` | 135 | yes | no (library) | Forward-looking, v-next. Shared decode helper for the two above. |

`frame.go` (236) and `verify.go` (139) are not alternatives to the above: they
are the wire framing and signature verification used by all of them, and they
import no v-next package.

**This changes the sequencing.** `Manager`, `Fanout`, and `Normalizer` are
1,117 lines that are simultaneously v-next-coupled *and* unreachable from
production. Their fate therefore depends on N1 — under Option B they would have
been deleted along with `pkg/domain`, and they survive only because Option A was
chosen. The consequence for planning is that push consolidation cannot be done
as independent groundwork: item 3 in the obstacle list above reads as though it
could be, and it cannot. It is the first step *inside* the programme, not a
parallel task.

Two behavioural differences still need resolving, whichever is kept.
Reconnect and backoff are handled by `Client` (31 references to the knobs) and
`Manager` (12), but `Fanout` has neither — so `Fanout` currently offers no
reconnect bound at all. Separately, `Fanout` is the only implementation that
populates a `FreshnessMonitor` (15 references, against 0 in the other two), so
adopting it would *add* staleness detection rather than merely preserve it.
`Client` is the released behaviour, so consolidation defaults to preserving it.

## 6. Programme

Option A is a multi-release programme. Order matters: step 1 decides what
`pkg/services` will talk to, so it precedes the rewire in step 2.

| # | Step | Notes |
|---|---|---|
| 1 | Decide the push implementation | §5. Defaults to preserving released `Client` behaviour. |
| 2 | Rewire `pkg/services` through `pkg/transport.Adapter` | **Highest-risk step and not mechanical.** `pkg/services` holds a `*client.Client` and calls `s.client.Do(...)` (`account.go:116,145,192,239` plus market and trading), while `Adapter` wraps `internal/transport.Transport` (`middleware.go:22-47`). These sit at different layers, so this needs a narrow executor interface both satisfy, or `Adapter` accepting an interface. Decide the shape before coding. |
| 3 | Fix R9 while the Adapter is in hand | `inner` is unused, `baseURL` is hardcoded, and `WithDeadline` (`middleware.go:136`) shares one cancel across callers. |
| 4 | Resolve R14 | Recommend rewriting `doc.go` to describe only what exists rather than adding a second login implementation — the released `SessionManager.EnsureLoggedIn` already covers the released path, and a second one is new risk rather than a fix. |
| 5 | Add a `depguard` boundary rule | Makes the intended layering machine-enforced in `.golangci.yml`, so drift is caught by CI rather than by review. |
| 6 | Test `pkg/services` | `market.go` 579 and `trading.go` 780 lines sit at ~4% coverage. Written *after* the rewire, against the final interface. |
| 7 | Re-gate `internal/push` | Currently 80.3% and ungated. Measure it against the kept implementation only. |
| 8 | Publish the migration guide and schedule v1.0 | The guide in §2 is the draft; it becomes a supported document. Refresh the `ARCHITECTURE.md` deviations table at the same time. |

ADR 0011 continues to hold for all of v0.1.x: none of these steps may change a
released type or wire shape before v1.0.
