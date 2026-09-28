# C10 — Designing `PushOrchestration`

- **Run:** `2026-09-26-vnext-parity-wire`
- **Task:** C10 (`architect`) — a design note. **No code changed; no `.go` file
  touched.**
- **Date:** 2026-09-26
- **Status:** Decided. C11 implements, C12 tests.
- **Amended:** 2026-09-26 — the one decision this note originally deferred to
  C11 is now closed here. §4's `PushTransport` signature named
  `*push.Notification`, which broke the layering rule at
  [`layering_test.go:166-176`](../../../internal/layering/layering_test.go) and
  would have leaked an internal type into a v1.0-frozen API; §4.1–§4.3 replace
  it with `*domain.PushUpdate`, decide the signature, and record the rejected
  alternatives. **No decision is now deferred to an implementation task.** Two
  findings were recorded in `todos.md`'s C10 row instead of fixed, because both
  live outside this task's surface (§11.2).
- **Related:** [plan.md](./plan.md), [todos.md](./todos.md) C10–C12/C15/P5,
  [ADR 0005](../../adr/0005-key-model-and-push-verification.md),
  [ADR 0008](../../adr/0008-decimal-financial-types.md),
  [ADR 0010](../../adr/0010-vnext-layered-architecture.md),
  [ADR 0011](../../adr/0011-v01x-compatibility.md),
  [design-parity-guard.md](./design-parity-guard.md),
  [design-reconnect-cause.md](./design-reconnect-cause.md),
  [design-tick-model.md](./design-tick-model.md),
  [design-futures-requests.md](./design-futures-requests.md),
  [design-a6-exchange-validation.md](./design-a6-exchange-validation.md),
  [docs/SPEC.md](../../SPEC.md) §4–§6, [docs/VNEXT.md](../../VNEXT.md) §5,
  [docs/streaming.md](../../streaming.md), [docs/threat-model.md](../../threat-model.md)

## 1. Decision, in one page

| # | Question | Decision |
|---|----------|----------|
| 1 | Where does the `TopicID` → payload mapping live? | **One exported total function in `pkg/services`**, `NotifyTypeForTopic(types.TopicID) (types.NotifyMsgType, bool)`. Not a map, not a method, not a struct field, not in `internal/push`. |
| 2 | Table, method, or type-level relation? | A **function**, plus a **type-level invariant**: the input vocabulary is `TopicID`, the output carries `NotifyMsgType`, and no exported function converts one to the other. |
| 3 | Unmapped `TopicID`? | **Rejected locally, before any HTTP request**, with an `errs` `StatusInvalidParam` naming the four deliverable payload types. Never guessed. |
| 4 | 11 topicIds → 4 payloads: one TCP subscription, or tracked separately? | **Two different counts, for two different things.** TCP handler registrations: fixed at `image(mapping) ∪ tradeTypes`, installed once, no counting. HTTP `/hq/Subscribe` calls: **reference-counted per `(topicId, sorted security set)`**. |
| 5 | May a caller subscribe to a `NotifyMsgType` directly? | **No.** Two disjoint entry vocabularies, and that is the invariant the whole design rests on. |
| 6 | Reconnect | **One client-level `Errors()` stream**, always live. Every reconnect emits exactly one `ErrReconnected` notice on it, unconditionally, with a matchable cause. Subscriptions are **re-established automatically**; a failed re-subscribe **kills the subscription** rather than leaving it silently live. |
| 7 | Backpressure | **Drop-oldest, non-blocking, counted** for market push. **No silent loss** for trade delivery — an overflow is an error, not a discarded fill. `WithBuffer(n)`, default 64. |
| 8 | Verification and freshness | Verification is `WithVerification(key, required)`, off by default, and the orchestrator **forwards every push error verbatim** (not just verification errors, as the released layer does). Freshness is per-subscription `LastSeen`/`Stale(after)`; `internal/push.FreshnessMonitor` is **not adopted** (§9.2). Out-of-order frames are **counted and reported, never dropped**. |
| 9 | Trade and futures push | **In `PushOrchestration`**, as a separate method pair on the same type, composing the already-wired `TradingService.SubscribeOrders`/`UnsubscribeOrders` through a narrow interface. Not a second service. |
| 10 | The tick | The orchestrator **carries no tick** and must not be given one. The 8 `"0.001"` literals in `internal/push/decode.go` become `"0"` as a C11 precondition. |
| 11 | Acceptance criterion | **§12**: four assertions whose key set is *parsed out of `pkg/types/enums.go` at test time* and whose expected values are *parsed out of `docs/SPEC.md` §4*. No list in the test is hand-maintained, so nothing can rot. |
| 12 | Its own scan root? | **No** (§13). Today's three roots are correct and the guard is already at 51/51 on every route the orchestrator composes. |
| 13 | What type crosses the `PushTransport` seam? | **A v-next envelope declared in `pkg/domain`** — `SubscribeTypes(types.NotifyMsgType, func(*domain.PushUpdate))`. The two boundary rules *deduce* the package rather than leaving it to taste (§4.1), and no type from `internal/push` appears anywhere in `pkg/services`. |

## 2. The brief's central premise is wrong, and the correction changes the shape of the answer

The brief states: *"**That mapping is stated nowhere in code.** I checked
`internal/push` and `pkg/types`: the two enumerations sit side by side with no
link. The released `pkg/hstong/stream` carries both … and states no mapping
either."*

**The mapping is in code, complete, and tested.** Two places, both measured:

| Location | What it is |
|---|---|
| [`pkg/hstong/stream/stream.go:624-637`](../../../pkg/hstong/stream/stream.go) | `notifyTypeForTopic(t types.TopicID) (types.NotifyMsgType, bool)` — a `switch` over **all 11** declared `TopicID` constants, `default:` returning `0, false`. |
| [`pkg/hstong/stream/backpressure_test.go:17-44`](../../../pkg/hstong/stream/backpressure_test.go) | `TestNotifyTypeForTopic` — **11 rows**, one per declared constant, asserting the `NotifyMsgType` each maps to. |
| [`pkg/hstong/stream/backpressure_test.go:49-55`](../../../pkg/hstong/stream/backpressure_test.go) | `TestNotifyTypeForTopicUnknown` — `0`, `999`, `-1` must all report `ok == false`. |

And the released layer registers the resulting handler set at
[`stream.go:176-181`](../../../pkg/hstong/stream/stream.go): 4 market types
from `marketNotifyTypes` plus 3 trade types from `tradeNotifyTypes`, so the
"11 HTTP subscriptions collapse onto 4 TCP handler registrations" observation is
real and already implemented — as 7 registrations, once, at construction.

This is not a cosmetic correction. It relocates the actual problem, and the
relocated problem is the one worth designing against:

1. **The relation is unexported and package-local.** `pkg/services` cannot reach
   it: `internal/layering/layering_test.go:136-146` forbids `pkg/hstong` from
   importing any v-next package, and the inverse is the dependency direction.
   So a v-next orchestrator that needs the same relation has exactly two lawful
   options — **re-derive it** (a second hand-maintained table, which is the
   failure mode rule 5 exists to prevent) or **not have it** (and ship the bug
   this task exists to close). The gap is not "no mapping", it is "**no
   shareable mapping, and the only one that exists is pinned by a table rather
   than by its key set**".
2. **The only pin is a hand-written 11-row table.** Adding a 12th `TopicID`
   constant to `pkg/types/enums.go` leaves `TestNotifyTypeForTopic` green with
   11 rows. That is precisely the rot §12 exists to prevent, and it is why
   "the released layer has a test for it" is not an answer to "how do we know
   ours cannot rot".
3. **The mapping has never had a correct citation.** `stream.go:623` says *"The
   mapping follows docs/SPEC.md §3"*; the table is in **§4** (§3 is "Response
   schemas" and contains no push schema — verified: `#### ` headers in §3 are 31
   HTTP response shapes, none of them a notify message). And
   [`pkg/types/enums.go:56`](../../../pkg/types/enums.go) repeats the same wrong
   `§3` for the topic grouping. Meanwhile
   [`pkg/hstong/stream/trade.go:15`](../../../pkg/hstong/stream/trade.go) points
   the *other* way — at `§4` for a claim about the three trade types, which are
   in **§5**, and whose "all three reuse `TradeStockDeliverNotify`" fact is
   sourced from no SPEC section at all but from
   [`internal/push/client.go:647-652`](../../../internal/push/client.go)'s
   GoDoc. **Two citations, both wrong, in opposite directions.** A relation this
   load-bearing has never been pointed at the right document, and the reason is
   that it is stated in three places (code, type doc, SPEC) with no single owner.
   **Both citations are recorded in `todos.md`'s C10 row and neither is fixed
   here** (§11.2): `pkg/hstong` is ADR-0011-protected, and a design note that
   edits another task's files is a design note nobody can check.

The correction therefore *raises* the bar rather than lowering it: the design
must produce a mapping the released layer's test cannot be, because the released
layer's test is the thing that would rot.

## 3. The substrate, measured — and a sizing correction

The brief sizes `internal/push` at 3,592 lines. **That number does not
reproduce.** Measured on this working tree:

| Set | Lines |
|---|---|
| `internal/push`, non-test (`client.go` 708, `frame.go` 236, `verify.go` 139, `normalizer.go` 135, `decode.go` 119, `freshness.go` 90, `doc.go` 54) | **1,481** |
| `internal/push`, all `.go` | **4,009** |
| `pkg/hstong/stream`, non-test (`stream.go` 702, `trade.go` 117, `doc.go` 93) | **912** |

The brief's per-file figures (81 / 115 / 219 / 127 / 110) are also below the
current ones (90 / 135 / 236 / 139 / 119), which are the figures
[docs/VNEXT.md §5](../../VNEXT.md) itself records. So the brief was written
against a stale snapshot. This matters only for sizing: `plan.md` already flags
C11 as "the least predictable task … treat its size estimate as soft", and
1,481 lines of substrate is a smaller surface than 3,592. The exported surface
is what C11 composes, and that is small: `New`, `Connect`, `Run`, `Close`,
`Errors`, `SubscribeTypes`, `UnsubscribeTypes`, `Addr`, `DialFunc`, and eight
options.

**The 3,592 is not a fabrication, and saying only "it does not reproduce"
would be the weaker claim.** It is reproducible exactly: it is the count of
**non-blank** lines across *all* of `internal/push`'s `.go` files, test files
included. A reader who runs a blank-line-skipping count gets 3,592 back and
concludes the note is wrong, so the correction is stated as a counting rule
rather than as an accusation:

| Count | Rule | `internal/push` |
|---|---|---|
| Every line, every `.go` | what this note uses, and what `wc -l` reports | **4,009** |
| Blank lines excluded, every `.go` | the brief's figure | **3,592** |
| Every line, non-test only | the size that matters to C11 | **1,481** |

Two consequences worth keeping, because the second one decides which row to
use. **The brief's number is a valid count of a set that is not the set C11
touches** — it includes the 2,528 lines of tests, so it overstates the
composable surface by 2.4×. And **the per-file figures the brief quotes are
lower still under the brief's own rule** (81 / 115 / 219 / 127 / 110 against
its own 3,592), so the brief is internally inconsistent as well as stale: no
single counting rule reproduces all of it. §16's coverage paragraph and
`plan.md`'s size estimate should use **1,481**.

**One reachability fact the brief does not state and C11 cannot skip.**
`internal/push` currently has **exactly one production caller**,
[`pkg/hstong/stream/stream.go:175`](../../../pkg/hstong/stream/stream.go), and
`push.Normalize` and `push.NewFreshnessMonitor` have **none** — the former is
called only by its own test, the latter not even by that. So today the
`domain.PushEvent` half of the push layer is entirely dark, and C11 is what turns
it on. Everything below is a consequence of that.

## 4. Where `PushOrchestration` lives, and why the shape is forced

`PushOrchestration` composes two transports: an HTTP subscribe and a TCP receive.
Three candidate homes, and the repository's own rules decide it.

| Candidate | Verdict |
|---|---|
| `pkg/services` | ADR 0010 names it: "use-case services: Market, Account, Trading, **PushOrchestration**". |
| `pkg/transport` | ADR 0010 also names `pkg/transport` as the Push (Gateway TCP) adapter, and [`pkg/transport/doc.go:20-22`](../../../pkg/transport/doc.go) records that a `PushAdapter` "was never implemented" — so the slot is reserved. But the *orchestration* is a use case, not an adapter. |
| A new `pkg/push` | Rejected: a new package under no layering rule and no parity scan root, i.e. a hole in both boundaries at once. |

**The decisive constraint is a layering rule, not a preference.**
[`internal/layering/layering_test.go:166-176`](../../../internal/layering/layering_test.go)
forbids `pkg/services` from importing `internal/push` or `internal/transport`:

> *services reaches the Gateway only through `client.Client`* — because
> "the indirection through `client.Client` is where the rate limiter, circuit
> breakers, metrics, and tracing spans live; calling the executor directly would
> bypass all of them".

The rule's stated rationale is HTTP-specific, but its effect is general: **`pkg/services`
may not hold a `*push.Client` either.** So the orchestrator cannot embed the push
transport. It must declare an interface for it, in `pkg/services`, and the
concrete wrapper goes in `pkg/transport` — which is the only package ADR 0010
rule 4 permits to depend on `internal/*`, and which
[`pkg/transport/doc.go:26-29`](../../../pkg/transport/doc.go) already states
("transport may depend on internal/auth, internal/transport, internal/push").

**The rule governs a type *reference* as well as a call, and that is what
decides the seam's payload type.** `parser.ImportsOnly` in
[`loadImports`](../../../internal/layering/layering_test.go) cannot tell an
interface method's parameter type from a value it is handed, so a signature that
*names* `internal/push` is an import like any other and is refused identically.
The rule's rationale being HTTP-specific is the strongest argument available for
arguing around it; §4.2.1 states that case at its strongest and then loses it on
a functional point, and §4.1 shows the rules leave exactly one lawful home.

**Two seams, and the HTTP one costs no code at all.**

```go
// pkg/services/push.go — declared in the dependent package (ADR 0010 rule 6)

// PushTransport is the TCP half. Implemented by pkg/transport. It delivers
// *domain.PushUpdate and names no type from internal/push; §4.1 is why.
type PushTransport interface {
    Connect(context.Context) error
    Run(context.Context) error
    Close() error
    Errors() <-chan error
    SubscribeTypes(types.NotifyMsgType, func(*domain.PushUpdate))
}

// TopicSubscriber is the HTTP half. Satisfied by *MarketService with no adapter.
type TopicSubscriber interface {
    Subscribe(ctx context.Context, topic types.TopicID, sec ...*Security) error
    Unsubscribe(ctx context.Context, topic types.TopicID, sec ...*Security) error
}

// OrderPushSubscriber is the session-wide trade half. Satisfied by *TradingService.
type OrderPushSubscriber interface {
    SubscribeOrders(ctx context.Context, accountID domain.AccountID) error
    UnsubscribeOrders(ctx context.Context, accountID domain.AccountID) error
}
```

`TopicSubscriber` is satisfied by `*MarketService` **structurally**:
[`pkg/services/market.go:418`](../../../pkg/services/market.go) and
`:435` already have exactly those signatures. `OrderPushSubscriber` is
satisfied by `*TradingService`: C15's
[`pkg/services/trading.go:585`](../../../pkg/services/trading.go) and `:617`.
**Both are compile-time assertions with zero adapter code** — and both pairs of
`client.Route` constants are already credited by the parity guard
(`market.go:429`, `market.go:446`, `trading.go:589`, `trading.go:621`). §13
follows from this.

### 4.1 What crosses the seam, and why the rules deduce it

**`func(*domain.PushUpdate)`, with `PushUpdate` declared in `pkg/domain`.** This
is not a preference among three reasonable shapes. It is the only answer the two
boundary rules leave open, and the enumeration below is exhaustive over the
module's packages.

| Home for the delivered type | May `pkg/services` name it? | May `pkg/transport` name it? | |
|---|---|---|---|
| `internal/push` | **No** — [`layering_test.go:166-176`](../../../internal/layering/layering_test.go) | Yes — ADR 0010 rule 4, [`pkg/transport/doc.go:26-27`](../../../pkg/transport/doc.go) | names it from the half the rule forbids |
| `pkg/services` | its own package | **No** — [`layering_test.go:148-153`](../../../internal/layering/layering_test.go), ADR 0010 rule 6 | names it from the other half |
| `pkg/transport` | **No** — ADR 0010 rule 4 | its own package | names it from neither |
| `pkg/types` | yes | yes | available and **empty**: no event type, no envelope, and §5.3's own argument forbids widening an ADR-0011-frozen surface for a v-next concern |
| **`pkg/domain`** | yes — it imports it today | yes — [`mappers.go:14`](../../../pkg/transport/mappers.go) | **chosen** |
| `gen/*` | importable | importable | AGENTS.md rule 1: generated code is never hand-edited, and a wire type is §11's whole subject |
| `pkg/hstong` | **No** — [`layering_test.go:136-146`](../../../internal/layering/layering_test.go) | **No** | the released surface, and ADR 0011 protects it |

**Both sides must be able to name it, because both sides write it.**
`pkg/services` declares the method and passes the handler; `pkg/transport`
implements the method and calls the handler. A type one side cannot spell is a
type the other side cannot honour. ADR 0010 rule 4 forbids `services → internal/*`
and `transport → services`, and rule 6 fixes the direction of the interface, so
the intersection of the two import sets is `pkg/domain` — which is the package
ADR 0010 line 39 already describes as holding "push events", and the one both
layers already import.

**The shape, and why the seam delivers the envelope rather than a bare
`domain.PushEvent`.** A bare event is the smaller answer and it is not enough:
`PushEvent` implementations carry `Symbol` and a `Timestamp` *string*, so a
handler receiving only the event cannot recover the `notifyId` — which is
§11.3's primary routing key and the thing §18's Q2 is about — without parsing
that string, and cannot read the notify `Time` §9.3 compares without parsing it
too. `Type` is the one envelope field that is recoverable, and only by accident
of the dispatch: the handler is registered per `NotifyMsgType`, so its closure
knows the type. Two of three fields would be gone, and both are facts about the
*frame* rather than about the payload — which is what an envelope is for.

```go
// pkg/domain/push_event.go — beside the PushEvent interface it wraps

// PushUpdate is one delivered push notification: the PBNotify envelope facts
// plus the normalised event. Neither half is derivable from the other — ID is
// the notifyId the Gateway puts on the frame, which §11.3's routing reads and
// which no event carries, and Time is the notifyTime §9.3's monotonic check
// compares, which reaches the caller only as a formatted string on the event.
type PushUpdate struct {
    Type  types.NotifyMsgType // the PBNotify discriminator
    ID    string              // notifyId; for market pushes the security code
    Time  time.Time           // notifyTime in UTC; zero when the Gateway sent none
    Event PushEvent           // never nil for a delivered update; see §4.3
}
```

A `pkg/types` value on a domain type is not new and needs no exception:
`TickerEvent.Side` is already a `types.EntrustBS` and `TradeEvent.Side` a
`types.EntrustStatus` ([`push_event.go:38, 82`](../../../pkg/domain/push_event.go)),
and the `pkg/domain` row of the layering table forbids `client`, `internal/`,
`pkg/hstong`, `pkg/services` and `pkg/transport` — **not** `pkg/types`
([`layering_test.go:184-189`](../../../internal/layering/layering_test.go)).
`time.Time` is stdlib.

**What ADR 0011 says about the timing, because it is the whole reason to decide
this now.** Line 93 puts `pkg/domain`, `pkg/services` and `pkg/transport`
outside the guarantee: "no compatibility guarantee yet — they are new and may
change across minor versions until they reach v1.0". So the choice is free today
and expensive after: at v1.0 this signature freezes, and a frozen signature that
names an internal package pins a caller to something the ADR never promised to
stabilise.

### 4.2 The rejected shapes

| Candidate | Verdict |
|---|---|
| `func(*push.Notification)` | **Rejected.** The strongest case for it is real and is argued in full in §4.2.1, and it loses on one functional point: the seam becomes **unimplementable outside this module**. |
| `func(Notification)` — a services-local mirror with `Payload any` | **Rejected.** It republishes the released layer's `Payload any` defect, which §11 exists to remove, and it still needs the same bridge (§4.3) to produce a typed value — it pays the cost and keeps the defect. |
| Narrow the **key**: `Subscribe(types.TopicID, …)` | **Rejected.** The topic→type relation lives in `pkg/services` (§5.1) and `pkg/transport` may not import `pkg/services`, so the transport cannot be handed a topic: it would need a second copy of the relation, which is the rot §12 exists to prevent. The caller must translate through `NotifyTypeForTopic` first, which is exactly §5.6's two-vocabulary invariant. |
| Narrow to `func(id string, ev *domain.PushEvent)` | **Rejected, and the closest loser.** It drops `Time`, so §9.3's out-of-order check would have to parse a `string` out of a domain event's `Timestamp` on the hot path, and it cannot gain a field — §11.3's open questions and §9.3's own unresolved questions are exactly the kind of finding that adds one. A parameter list is a signature that must be broken to extend; the envelope is the same information with room to grow. |
| `func(*domain.PushUpdate)` | **Chosen.** |

#### 4.2.1 The case for `*push.Notification`, stated at its strongest

Four arguments, none of them foolish. (1) **The rule's own rationale is
HTTP-specific**: it says `services` reaches the Gateway only through
`client.Client` *because* that is where the rate limiter, circuit breakers,
metrics and tracing live, and a TCP socket is a different path with none of those
to bypass. (2) **A type reference in a signature is not a call**, so nothing the
rule protects is actually exposed. (3) **It costs zero code** — no domain type, no
mapper, and it is the value `internal/push` already dispatches
([`client.go:468-493`](../../../internal/push/client.go)). (4) **There is
precedent in this very package**: [`executor.go:21-27`](../../../pkg/services/executor.go)
argues at length that an interface may name another layer's shared vocabulary and
calls it a boundary rather than a hole, and §14 adopts that sentence.

**What it costs.** The first cost is functional, not aesthetic, and it is
decisive on its own. `internal/` is importable only from within this module, so
**no code outside this repository can write the method.** An exported interface
with an unnameable parameter type cannot be implemented by a third party — not a
fake for a test, not a recorded fixture, not a different push engine.
`pkg/services` is testable today precisely because `Executor` is a two-method
interface anyone can write; this choice would make the one service whose
behaviour is hardest to test the one no caller outside can fake. And the
comparison with `Executor` is not close: `client.Route` is at least a *public*
type, so a caller can spell it when implementing the interface.
`*push.Notification` has no public spelling at all.

Three further costs, in descending order of how long they last:

- **A verified boundary becomes an asserted one.** The rule is machine-checked
  today ([`layering_test.go:166-176`](../../../internal/layering/layering_test.go));
  adopting the signature means a scoped exception in the table, which trades
  "cannot compile" for "a comment says so" — the downgrade §12.3 calls out when it
  says a guard that cannot fail is not a guard.
- **It freezes a package the ADR does not promise to stabilise.**
  `internal/push` is in ADR 0011's table as something not to *break*, and C11
  *adds to it* (§4.3). Its `Payload any` and `Time uint64` are wire shapes a v1.0
  domain API would want to shed, and a frozen signature would pin them.
- **It makes the defect look intentional.** `Payload any` in a v1.0 signature,
  under an ADR whose whole subject is that the services layer speaks domain
  types, is precisely what §11 was written to remove.

### 4.3 The bridge is no longer a preference but a precondition

§11.1 chose `push.NormalizeNotification(*Notification) (*domain.PushEvent, error)`
from three options. **§4.1 makes it the only option**, and the reason is
mechanical rather than stylistic: the seam delivers a `domain.PushEvent`, and
`decodeNotification` has already consumed the envelope into `*Notification`
([`client.go:622-637`](../../../internal/push/client.go)), so `Normalize` — which
wants a `*pbmsg.PBNotify` it can no longer be given — cannot produce one.

The envelope halves of `PushUpdate` are free: `Type` is `n.Type`, `ID` is `n.ID`,
and `Time` is `time.UnixMilli(int64(n.Time))` from the wire's millisecond
`notifyTime` ([`client.go:199-201`](../../../internal/push/client.go)).
**`Event` is the entire cost, and it is mandatory.** It cannot be dodged by
duplicating the decoders: they are unexported in `internal/push`
([`decode.go:16-104`](../../../internal/push/decode.go)), and a copy in
`pkg/services` is §11.2's rejected fourth mapper set.

Two properties the bridge must state, recorded here because C11 would otherwise
choose them by accident:

1. **`NormalizeNotification` must never return `(nil, nil)`.** `Normalize` does,
   at its five `len(payload.GetValue()) == 0` arms
   ([`normalizer.go:33, 47, 59, 70, 82`](../../../internal/push/normalizer.go)),
   and `PushUpdate.Event` is documented never nil. The bridge substitutes a
   `domain.SystemEvent{Code: "EMPTY_PAYLOAD", Level: "warn"}` for those five and
   `normalizeUnknownEvent`'s for the `default:` arm. `Normalize` itself is left
   exactly as it is: it has no production caller (§3) but it has a 439-line test
   file ([`normalizer_test.go`](../../../internal/push/normalizer_test.go)), and
   rewriting a tested internal function to serve a new caller is how a repair
   gets lost.
2. **The millisecond→`time.Time` conversion happens in `pkg/transport`**, not in
   `internal/push`, because `Notification.Time` is a wire value and ADR 0010 rule
   2 puts wire→domain conversion in the transport layer. The released layer
   already performs the same conversion at
   [`stream.go:639`](../../../pkg/hstong/stream/stream.go) (`notifyTime`). This is
   also the one mapper the push path does **not** add to the deviation
   [`pkg/domain/doc.go:20-24`](../../../pkg/domain/doc.go) already records,
   because this one lands where rule 2 puts it.

**The same reasoning bounds the error channel, and the note states it once so it
is not re-derived.** A sentinel a caller is expected to match must be owned by
`pkg/services`, for the same unnameability reason; §7.1's verbatim forwarding
keeps `internal/push`'s sentinels matchable *in the chain* by `errors.Is` and
keeps their text, which is what it already claims. C11 owns the named version.

## 5. The mapping

### 5.1 One exported total function in `pkg/services`

```go
// NotifyTypeForTopic reports the PBNotify notifyMsgType of the payload the
// Gateway sends for topic t, and false for a TopicID the SDK does not map.
func NotifyTypeForTopic(t types.TopicID) (types.NotifyMsgType, bool)
```

Exported, not unexported. `stream.notifyTypeForTopic` is unexported because
`stream` is its only consumer; in v-next the consumer set is wider — a caller
holding a `TopicID` needs to know which accessor to reach for, and C12's tests
need it — and hiding it would push every caller back to writing the same table
this design exists to eliminate. `pkg/services` is not yet reachable by a caller
([docs/VNEXT.md §1](../../VNEXT.md)), so exporting costs nothing against
ADR 0011 and disappears into the v1.0 surface at the right time.

### 5.2 A `switch`, not a `map`

A `map[types.TopicID]types.NotifyMsgType` is mutable package state, and its
failure mode is the one that has to be impossible here: a missing key yields the
**zero value** `0`, which is `TrsStockDeliverMsgType` — a *valid, real* type. A
forgotten entry would therefore deliver order-status notifications to a
subscriber that asked for a quote. A `switch` has no such state: `default:`
returns `(0, false)`, `ok` is explicit at every call site, and the `bool` return
is what §5.6 needs anyway.

This is A6's shape applied to a relation rather than a set:
[`design-a6-exchange-validation.md`](./design-a6-exchange-validation.md) chose
unexported `switch` predicates over a set for exactly this reason, and its GoDoc
argument — "the messages name **all** accepted codes so a value cannot be read as
evidence that only some are accepted" — becomes a naming obligation on the
failure message here.

### 5.3 Not a method, and not in `pkg/types`

A Go method cannot be declared on a non-local type, so `TopicID.PayloadType()`
is impossible. A6 reached the same wall and resolved it with a package-local
unexported helper rather than widening ADR 0011's protected surface; that
resolution does not transfer here, because the consumer is a *different* package
and the function has to be exported.

Putting it in `pkg/types` was considered and rejected:
[`ADR 0011`](../../adr/0011-v01x-compatibility.md) line 10 puts `pkg/types` in
the protected v0.1.x surface, and the relation is a v-next-layer concern. Adding
it there widens a frozen public API with a function whose only caller is an
unwired package.

### 5.4 Not a `Topic` struct — the type-level answer

The strongest alternative is a value that carries **both** halves:

```go
type Topic struct {
    ID      types.TopicID
    MsgType types.NotifyMsgType
}
```

so the pair is structurally impossible to get wrong. **Rejected**, and the reason
is the whole point of the task: a struct **admits an inconsistent value**.
`Topic{ID: 25, MsgType: TickerNotifyMsgType}` compiles, and the caller who
builds it by hand — or a future maintainer who adds a fifth field — has silently
produced a topic that will never receive the payload it claims. A bare
`types.TopicID` cannot carry a wrong payload, because it carries no payload at
all. The relation is enforced by the *only* mechanism that cannot be bypassed:
the payload is not a parameter.

**The invariant, stated so C12 can test it:**

> The **input** vocabulary of the push layer is `types.TopicID` (11 values).
> The **output** vocabulary is `types.NotifyMsgType` (7 values). No exported
> identifier in `pkg/services` accepts a `NotifyMsgType` as an input, and no
> exported identifier returns a `TopicID`. The two sets are joined by exactly one
> function, `NotifyTypeForTopic`, plus the three trade types reached by
> `SubscribeOrders` and by nothing else.

### 5.5 What happens on an unmapped `TopicID`

**Rejected locally, at zero HTTP requests, with an `errs` typed error.** Parity
with [`stream.go:295-302`](../../../pkg/hstong/stream/stream.go) and A6's
fail-closed decision, and the same cost asymmetry: guessing a payload type for an
unrecognised topic means the caller receives events under a type they will
mis-parse with every accessor, and nothing anywhere reports it.

Three specific properties:

1. **It is a legitimate state, not an error in the mapping.**
   [`pkg/types/enums.go:56`](../../../pkg/types/enums.go) says the `TopicID` set
   is "non-exhaustive". So an unmapped topic is the *designed* forward-
   compatibility case — the same reasoning that made A6 keep `EntrustType`
   permissive. The difference is that `EntrustType` can be forwarded unchanged
   and still mean something, whereas a topic's payload is not on the topic.
2. **The message names the four deliverable payload types**, plus the function
   to consult, so the error is a complete answer and not a dead end. Naming 11
   topic constants in a message would be unreadable; naming 4 payload types is
   the information the caller actually lacks.
3. **The check is on the service's own path, before `market.Subscribe` is
   called.** `MarketService.Subscribe` also validates the topic
   ([`market.go:419`](../../../pkg/services/market.go) → `validateTopic`), so the
   orchestrator's check is the earlier of two, and it is the one that knows
   whether a payload exists.

### 5.6 May a caller subscribe to a `NotifyMsgType` directly?

**No, and the reason is the finding that makes the answer safe rather than
merely conservative.**

The three trade types — `TrsStockDeliverMsgType` (0),
`TradeStockDeliverMsgType` (1), `FuturesTradeStockDeliverMsgType` (2) — have
**no `TopicID` at all**. They are enabled by the session-wide
`POST /trade/TradeSubscribe`, not by a topic and not by a security list. So the
two push vocabularies do not line up: 4 of the 7 `NotifyMsgType` values are
reachable from a `TopicID` and 3 are not, and the wire does not carry a field
distinguishing them.

A `SubscribeType(types.NotifyMsgType)` entry point would therefore have to
encode "is this a market type that needs a topic and a security list, or a
trade type that needs a login?" — a variant the wire does not express and
`pkg/types` does not declare. It would also let a caller name
`OrderBookNotifyMsgType` without ever issuing `/hq/Subscribe`, so the Gateway
would push nothing and the caller would wait forever for an order book. That is
the precise defect this task was raised to close, moved up one layer.

So the entry points are exactly two, and they are not variations of each other:

```go
func (o *PushOrchestration) Subscribe(ctx context.Context, topic types.TopicID, sec ...*Security) (*Subscription, error)
func (o *PushOrchestration) SubscribeOrders(ctx context.Context, accountID domain.AccountID) (*OrderSubscription, error)
```

## 6. The collapse: 11 → 4, and which count needs a reference count

The brief asks whether the topicIds sharing a payload should share one TCP
subscription or be tracked separately. **Both, and they are different questions
about different objects.** Conflating them is what makes this look hard.

### 6.1 TCP handler registrations: a fixed set, no counting

`push.Client.handlers` is a `map[types.NotifyMsgType]*dispatcher` and
`SubscribeTypes` replaces-or-deregisters by key
([`client.go:468-493`](../../../internal/push/client.go)). So the handler set is
naturally a set, and the right value is `image(NotifyTypeForTopic) ∪ tradeTypes`
— 4 + 3 = 7 — installed once at construction, exactly as
[`stream.go:176-181`](../../../pkg/hstong/stream/stream.go) does with two
hand-written slices.

**The one improvement over the released layer: the set is derived, not
declared.** The released layer hardcodes `marketNotifyTypes` and
`tradeNotifyTypes`; the v-next orchestrator derives the market half from
`NotifyTypeForTopic`'s image, so a 12th `TopicID` mapping to a 5th payload type
cannot be *mapped but not delivered*. That single derivation is what makes §12's
G2 worth having.

No reference count is needed here. A handler is either registered or not, and
re-registering is a replace.

### 6.2 HTTP `/hq/Subscribe` calls: reference-counted per `(topicId, security set)`

**This is where the counting belongs, and where the released layer has a bug
worth not inheriting.** `stream.Subscription.Cancel` issues
`/hq/Unsubscribe` for *its own* subscription
([`stream.go:550-562`](../../../pkg/hstong/stream/stream.go)), and
`onReconnect` issues one `/hq/Subscribe` per `Subscription`
([`stream.go:411-424`](../../../pkg/hstong/stream/stream.go)). Two callers
subscribing to the same topic with the same security list therefore send two
`Subscribe` calls and one `Unsubscribe` each — the first `Unsubscribe` tears down
a stream the second caller is still using, and nothing reports it.

Decision: **coalesce.** The orchestrator keeps a registry keyed by
`(topic, canonical security set)`, holding a reference count and the set of
local `*Subscription` handles.

- `Subscribe`: look up the key. Absent → issue `/hq/Subscribe`, on success
  register the key with `ref = 1`. Present → `ref++`, **no HTTP request**.
- `Cancel`: `ref--`. Only at `ref == 0` issue `/hq/Unsubscribe`; on a failure
  the local registration is still dropped and the error is returned — the
  released layer's own rule, "the channels are closed even when the HTTP call
  fails, so the error is returned but does not leave the subscription live"
  ([`stream.go:550-562`](../../../pkg/hstong/stream/stream.go)).
- Reconnect: re-issue `/hq/Subscribe` **once per key whose HTTP subscribe
  succeeded**, tracked in a per-key `subscribed bool`, not once per local
  subscription.

**The key must be order-insensitive and value-based**, because `Subscribe` takes
a variadic `...*Security` and two callers may pass the same instruments in
different orders. Canonical form: sort by `(Code, DataType)` and serialise. This
is the one place where a caller-supplied pointer is read, so the registry stores
**copies** — parity with `stream.Subscribe`'s
`append([]*dto.Security(nil), securities...)` ([`stream.go:317`](../../../pkg/hstong/stream/stream.go))
and with `Subscription.Securities()`'s documented copy semantics.

**What this costs, stated plainly.** It is a deliberate behaviour divergence from
the released layer: a duplicate subscriber issues one HTTP request where the
released layer issues two. That is a *reduction* in requests, and SPEC §2.2
declares `/hq/Subscribe` and `/hq/Unsubscribe` as ordinary per-topic HTTP
endpoints with no reference-counting semantics, so idempotent repetition is the
natural reading. It is observable, which is the right property for a divergence
to have: `test/mockgateway` can assert the request count, and a caller migrating
from `stream` will see fewer `/hq/Subscribe` calls for a duplicate subscription.
It is a divergence in the *only* direction where fewer requests cannot be a bug
(a duplicated subscribe is never something a caller depends on).

Rejected: **coalescing on topic alone** (ignoring the security list) — two
subscribers on the same topic for *different* instruments would collapse into
one call carrying one instrument's list, and the other would never arrive.
Rejected: **letting the Gateway arbitrate** — the SDK cannot observe the
Gateway's own reference state, and the released layer's shape proves nobody has
tried.

## 7. Reconnect

### 7.1 What a v-next caller observes

**One client-level `Errors()` stream, and it is always live.**

```
PushOrchestration.Errors() <-chan error
```

This is the one place where v-next deliberately diverges from the released layer,
and the divergence answers a constraint the brief states: *the answer must not
depend on a notification nobody is watching.*

The released layer has two holes, both measured:

1. **A reconnect with zero subscriptions is invisible.**
   [`stream.go:411-413`](../../../pkg/hstong/stream/stream.go) iterates
   `s.subs`; with no subscriptions the loop body never runs, so `ErrReconnected`
   is delivered to nobody and the reconnect is unobservable. A caller that
   connects, is disconnected, reconnects, and *then* subscribes has learned
   nothing.
2. **A transport read error is never forwarded.**
   [`stream.go:249-263`](../../../pkg/hstong/stream/stream.go) forwards
   *verification* errors only, and the GoDoc says why: *"Other push errors are
   left untouched: reconnect notices are emitted by onReconnect, and the stream
   API has never surfaced transport read errors, so forwarding them here would be
   a behaviour change."* That is a compatibility argument against a channel
   whose capacity reconnect notices already consume. A client-level stream has no
   such constraint.

So: **one drain of `push.Client.Errors()`, one client-level channel, and every
error forwarded verbatim.** `errors.Is`/`errors.As` chains are preserved, so
`errors.Is(err, push.ErrSignatureMismatch)` — the exact assertion in
[`pkg/hstong/stream/verify_test.go:160`](../../../pkg/hstong/stream/verify_test.go)
— keeps working, and a caller that today cannot match `internal/push`'s framing
sentinels still gets their text (A4 §1.1, unchanged).

Rejected: **both a per-subscription and a client-level channel.** A caller with
50 subscriptions would have to drain 50 channels or lose the diagnostics, which
is the same failure the single stream removes. Rejected: **keeping only the
per-subscription channel** for parity. It fails constraint (1) above on its own
terms, and "we did it this way for eleven releases" is not a reason to keep a
channel that can be silent.

The cost of the single stream: a caller migrating from `stream` must restructure
their error handling, and a subscription-scoped error is now labelled rather
than isolated. §16 specifies the label (a `*SubscriptionError` carrying the
topic and the instrument), which is strictly more information than the released
channel gave.

### 7.2 The notice

Reuse A4's mechanism, in `pkg/services`, unexported:

```go
var ErrReconnected = errors.New("services: push connection re-established")

type reconnectNotice struct{ cause error }   // Error / Is / Unwrap
```

with `Is(ErrReconnected)` and `Unwrap() → cause`, exactly as
[`pkg/hstong/stream/stream.go:447-473`](../../../pkg/hstong/stream/stream.go).
A4's measured reason applies verbatim: multi-`%w` yields an `Unwrap() []error`,
so `errors.Unwrap` returns `nil` and the documented mechanism is falsified. A
nil cause yields the bare sentinel.

A4 also established, and it constrains what a caller can usefully branch on: **a
signature-verification failure can never be the reconnect cause**, because
[`client.go:445-450`](../../../internal/push/client.go) reports and *continues*;
only `ReadFrame` and deadline errors reach `fireOnReconnect`. So the cause is
`io.EOF` / `io.ErrUnexpectedEOF` / `*net.OpError` / an `internal/push` framing
sentinel — the stdlib-reachable part is exactly the operational distinction
between "the Gateway hung up, expect a gap" and "the read deadline expired, the
feed is stale". A4's T7 (end-to-end `io.EOF`) is the end-to-end proof and is
still owed; C12 should pay it here.

### 7.3 Subscriptions are re-established automatically, and a failure is terminal

**Automatic, from the orchestrator's own registry, never from a caller-visible
notification.** `push.Client` reconnects and fires `onReconnect(cause)`, but it
does not know what was subscribed, so the orchestrator must hold the registry —
which §6.2 gives it.

Two decisions, both divergences from the released layer:

1. **The re-subscribe context is the orchestrator's, not `context.Background()`.**
   [`stream.go:418`](../../../pkg/hstong/stream/stream.go) derives
   `context.WithTimeout(context.Background(), 10s)` with the GoDoc *"the run
   context may already be cancelled"*. That is correct for the released layer
   and wrong here: a `Close()` racing a reconnect leaves an in-flight
   `/hq/Subscribe` against a Gateway the caller has released. The orchestrator
   keeps a `resubscribeCtx` it cancels in `Close`, and applies the timeout on
   top. Same 10 s default, own cancelable parent.
2. **A failed re-subscribe kills the subscription.** The released layer only
   reports it ([`stream.go:421-423`](../../../pkg/hstong/stream/stream.go)),
   leaving a subscription that is registered locally and will never deliver
   again. For a market topic the caller's next observation is *the absence of
   news*, which during a quiet market is indistinguishable from a working feed.
   v-next: the subscription is cancelled locally, its update channel is closed,
   the refcount is decremented (issuing `/hq/Unsubscribe` if it was the last —
   the Gateway cannot have honoured the subscribe, so this is best-effort and
   the error is reported if it fails), and a terminal error naming the topic,
   the instruments, and the Gateway's reason goes on the client stream. **A
   loud dead subscription beats a silent one.** This is the same trade as A6's
   fail-closed argument and B7's "tolerate nil" argument in one line: on a *read*
   path with no un-reconciled side effect, the correct response to a broken
   assumption is to stop pretending.

Rejected: **retry the re-subscribe with the push client's backoff.** ADR 0003
applies by analogy to a subscription that is *not* an order mutation but is
indefinitely repeatable, and an unbounded retry loop inside a reconnect hook that
runs on the `Run` goroutine would stall the read loop. A single attempt per
reconnect, with the next reconnect trying again, is both simpler and strictly
more likely to be observed.

## 8. Backpressure

### 8.1 What is already upstream of this decision

`push.Client` **already drops, before v-next sees anything.**
[`client.go:35`](../../../internal/push/client.go) sets `dispatchBuffer = 64` per
notify type, and [`client.go:503-525`](../../../internal/push/client.go)
implements a drop-oldest `dispatch` that never blocks the read loop. The error
channel does the same at `errorBuffer = 16`
([`client.go:38`](../../../internal/push/client.go), `:529-550`).

This is the load-bearing measurement for the whole question. The brief asks
whether a v-next channel should drop or block, and notes that blocking "stalls
every other subscriber". **Neither.** A subscriber-level block cannot stall the
read loop, because v-next is not on the read loop — the read loop already handed
off to a per-type goroutine 64 frames ago and dropped 65 of them to get there.
Blocking downstream cannot recover a frame that was already discarded, and it
costs a goroutine stall for every *other* subscriber of the same type. So:

**Non-blocking, drop-oldest, for the update stream. And the drop is counted,
because drop-oldest is only defensible when it is loud.** The released layer
drops silently ([`stream.go:566-584`](../../../pkg/hstong/stream/stream.go),
pinned by
[`backpressure_test.go:76-95`](../../../pkg/hstong/stream/backpressure_test.go));
that is a defect to fix, not a standard to match. `Subscription.Dropped()`
returns the count, and it is asserted in C12's tests and documented in the
GoDoc.

### 8.2 Drop-oldest, not drop-newest — and why it is right for market data

A quote, a tick, and an order-book level are **state**, not **events**: each
supersedes its predecessor, and a stale sample is worth less than the current
one. The released layer's reasoning at `docs/streaming.md:148-151` is correct and
this design adopts it. `WithBuffer(n)` sets the depth, default 64, matching both
[`stream.go:39`](../../../pkg/hstong/stream/stream.go) and `push.dispatchBuffer`.

### 8.3 The exception: trade delivery must not drop

`TradeStockDeliverNotify` is a **per-fill event**. Dropping the oldest queue
entry loses a fill permanently and silently; there is no later notification for
it. So the rule is not uniform:

- **Market topics** (`Quote`, `Tick`, `OrderBook`, `Broker`): drop-oldest,
  counted, `Dropped()` exposed.
- **Trade and futures delivery**: **no silent loss.** An overflow is a
  `ErrTradeBacklog` on the client stream carrying the count, and the counter is
  asserted to stay zero in C12's tests. The guarantee is *"no silent loss"*, not
  *"no loss"* — a caller that never drains will eventually be told, which is the
  correct loud failure, and the alternative (blocking) would stall the
  `SubscribeTypes` dispatcher goroutine for `TradeStockDeliverMsgType`, i.e. for
  every trade subscriber in the process.
- **The error stream itself**: bounded, drop-oldest, counted — a lost
  diagnosis is recoverable, an unbounded queue on a flapping connection is a
  memory leak. `ErrorsDropped()` on the orchestrator, same shape as
  `Dropped()`.

Rejected: **one uniform policy.** It would force a choice between corrupting an
order book and losing a fill, and the two are not the same kind of value.
Rejected: **block on trade delivery.** It trades a loud, immediate, one-caller
error for a silent, process-wide stall — the worse of the two on both axes.

## 9. Verification and freshness

### 9.1 Verification: keep the mechanism, change the surface

`internal/push`'s verifier is sound and opt-in
([`internal/push/verify.go`](../../../internal/push/verify.go); ADR 0005, and
SPEC §6 restates that verification is off by default). Four sentinels:
`ErrNoPublicKey`, `ErrMissingSignature`, `ErrSignatureMismatch`,
`ErrUnsupportedKey`. Decisions:

1. **Configuration is the orchestrator's own option**, `WithVerification(key, required)`,
   defaulting to **off**. The released layer reads `VerifyPush()` and
   `PlatformPublicKey()` off the `*client.Client` and hardcodes `required = true`
   ([`stream.go:172-174`](../../../pkg/hstong/stream/stream.go)). v-next must
   not reach into the released concrete type — `pkg/services` deliberately takes
   the `Executor` *interface* ([`pkg/services/executor.go:12-39`](../../../pkg/services/executor.go))
   precisely so the layer can be driven by a fake. So a caller configures
   verification on the orchestrator, and `required` is exposed rather than
   forced, matching `push.WithVerification`'s own contract (drop on failure vs.
   report and deliver anyway).
2. **Every verification failure reaches the client error stream**, with its chain
   intact. This is the parity target from
   [`verify_test.go`](../../../pkg/hstong/stream/verify_test.go) — a correctly
   signed frame is delivered, a tampered frame is dropped and
   `errors.Is(err, push.ErrSignatureMismatch)` is true — and it now arrives
   without needing a subscription to exist. Under the released layer a caller with
   no subscription learns nothing about a bad frame.
3. **A key that will not parse fails `Connect`, not `New`** — `push.New` never
   fails and stores the error for `Connect` to return
   ([`client.go:277-284`](../../../internal/push/client.go)). The orchestrator
   forwards it. Documented, tested, no change.

### 9.2 Freshness: per-subscription, and `FreshnessMonitor` is not adopted

[`internal/push/freshness.go`](../../../internal/push/freshness.go) is a
`FreshnessMonitor` over `map[int]FreshnessEntry` with `Record(topicID int, seq
uint64)`, `Check(topicID int)`, and a 5-minute default tolerance.
[docs/VNEXT.md §5.2](../../VNEXT.md) kept it precisely because it is
"time-based, so it works even though `seq` is always 0".

**It is not adopted, and the reason is its key.** The unit of a push
subscription is `(topic, security)` — `Subscribe` takes a security list — but
`FreshnessMonitor` keys on a single `int` and its struct field and parameter are
both *named* `TopicID`. Two securities on the same topic are indistinguishable to
it, so "is `00700.HK` still arriving?" cannot be answered with it. Its `TopicID`
field would have to carry a subscription index, which is a naming lie in a type
`ADR 0011` protects and which VNEXT §5's justification does not anticipate.

This is a correction to a recorded decision, stated narrowly: the capability
FNEXT identified is real and remains needed; the component as keyed cannot
express it. Rather than rename an ADR-0011-protected exported field in `internal/`
as a side effect of a push-orchestration task, the orchestrator puts
`LastSeen`/`LastUpdate`/`Stale(after)` on the subscription, where the state
already lives. The consolidation of the two — one monitor in `internal/push`
keyed on `(topic, security)`, or none — is a `pkg/domain`/`internal/push` surface
decision and belongs with P5 or after it, not here.

`Stale(after)` is a **caller-supplied** threshold, not a fixed 5 minutes: a
5-minute silence means something very different for a liquid HK stock and an
illiquid HK warrant, and the SDK has no business guessing which regime the
caller is in. A non-positive `after` restores the 5-minute default rather than
accepting zero, which would mark every subscription stale on the first check —
`NewFreshnessMonitor`'s own rule ([`freshness.go:45-50`](../../../internal/push/freshness.go)),
adopted because it is right.

### 9.3 Out-of-order frames: counted, never dropped

Two different conditions that are easy to conflate:

- **Freshness** — nothing has arrived for a while. §9.2. Operational meaning:
  "the feed is stale".
- **Out-of-order** — a frame arrives whose `notifyTime` is *older* than one
  already delivered for the same `(topic, security)`. Operational meaning: "the
  Gateway's clock moved, or we crossed a reconnect boundary".

Strict sequence-based gap detection is **impossible** on this protocol and the
repository says so: R4 in
[docs/threat-model.md:192](../../threat-model.md) — "The Gateway push envelope
carries no per-event sequence number, so correct gap detection is impossible on
this protocol" — and `DedupCache` was removed as inert for that reason. So the
check is a **monotonic `notifyTime` per subscription**, which is a real
observation and a real alarm, not a dedup.

**Detect, count, report; do not drop.** A drop here would violate §8's own
principle (a silent loss is worse than a stall) and would be wrong on reconnect
anyway, where a legitimately earlier timestamp arrives. `OutOfOrder()` counts;
the first occurrence emits an error on the client stream and later ones are
counted silently, so a clock that is permanently skewed produces one alert
rather than a flood that crowds out the drop reports.

## 10. Trade and futures push ownership

**In `PushOrchestration`, as a separate method pair on the same type. Not a
separate service, and not a separate file's worth of state.**

The reasons, in the order they decide it:

1. **It is the same connection, the same dispatch path, and the same reconnect.**
   `SubscribeOrders` enables the Gateway to *send* types 0/1/2 on the socket the
   orchestrator already owns, and the reconnect must restore it with the market
   subscriptions or the caller silently loses order-status updates while still
   receiving quotes. Two owners of one socket is the mistake `Manager` and
   `Fanout` already were ([docs/VNEXT.md §5](../../VNEXT.md) §5.1–5.2).
2. **But the entry is not a topic**, so it cannot be a `Subscription` with a
   `TopicID`. It    is session-wide, per-account, and needs a login — the shape the
   released layer encodes as `Subscription.trade bool`
   ([`stream.go:481-483`](../../../pkg/hstong/stream/stream.go)). A `bool` with
   three meanings — which notify types, whether the HTTP call is needed, whether
   to re-subscribe — is a variant wearing a boolean's clothes, and A3's finding
   ("a rule asserted on one branch is half a rule") is what that produces. So a
   **distinct `*OrderSubscription` type** with its own `Updates()`/`Errors()`
   and no `TopicID`/`Securities()` accessors at all: the absence of those
   methods *is* the type-level statement that this subscription has no topic.
3. **The reference count is a different shape and must not share a counter.** The
   market counter is per `(topic, security set)`; the trade subscription is
   session-wide, so it is **one HTTP call for the first and one for the last**,
   across all accounts on the connection if the Gateway's scope is per-session
   (which [`trade.go:56-60`](../../../pkg/hstong/stream/trade.go) asserts and
   which is what `tradeSubs` implements at
   [`stream.go:541-545`](../../../pkg/hstong/stream/stream.go)). A single
   `tradeSubs int` is right and must not be generalised into the market map.

**The HTTP half is not new work, and this is what C1a was for.** C15 already
wired `RouteTradeSubscribe` and `RouteTradeUnsubscribe` as
`TradingService.SubscribeOrders`/`UnsubscribeOrders`, decided them to be a
*separate task* precisely so `PushOrchestration` would stay single-purpose. This
design honours that split: `PushOrchestration` **composes** those methods
through `OrderPushSubscriber` and adds **no route of its own**. `PushOrchestration`
is therefore free of `client.Route` entirely — see §14.

Rejected: **a separate `OrderPushService`.** It would own the `/trade/*` calls
(which C15 placed on `TradingService`, beside the rest of the trade surface) and
need a handle on the orchestrator's subscription registry to know whether anyone
is listening, so the two types would be mutually referential. One type, two entry
vocabularies, one registry. Rejected: **leaving trade push to a future task**,
because `internal/push` would then have its third partial consumer and the
reconnect invariant would have a permanent hole.

## 11. What the orchestrator delivers, and the decoder problem on that path

**`domain.PushEvent`, not `Payload any`.** That is the v-next layer's one
substantive gain over the released layer: `stream.Event.Payload` is `any` and the
caller must reach for `Event.BasicQot()`, `Event.Ticker()`, … and handle a `false`
([`stream.go:666-702`](../../../pkg/hstong/stream/stream.go)).

The delivered value is the envelope §4.1 chose, and it lives in `pkg/domain`
beside the `PushEvent` interface it wraps — **not** in `pkg/services`, because
`pkg/transport` may not import `pkg/services` and so could not spell a type
declared there:

```go
// pkg/domain/push_event.go

// PushUpdate is one delivered notification. Event is never nil for a delivered
// update: a frame whose payload cannot be classified is delivered as a
// SystemEvent, and §4.3 makes that substitution the normaliser's job rather
// than the caller's.
type PushUpdate struct {
    Type  types.NotifyMsgType // the PBNotify discriminator
    ID    string              // notifyId; for market pushes the security code
    Time  time.Time           // notifyTime in UTC; zero when the Gateway sent none
    Event PushEvent
}
```

`PushEvent` is an **interface** ([`push_event.go:10-12`](../../../pkg/domain/push_event.go)),
so the `Event` field is a typed value the caller type-switches on, with a
`domain.SystemEvent` default arm — which is the whole of §11's claim, and it is
now a property of a *domain* type rather than of a decoder that had no caller.

### 11.1 The two decoders cannot currently meet — a hard blocker for C11

`internal/push` has **two** decoders and **no bridge**:

| Decoder | Input | Output | Production callers |
|---|---|---|---|
| `decodeNotification` ([`client.go:622-637`](../../../internal/push/client.go)) | raw frame body `[]byte` | `*Notification{Type, ID, Time, Payload any}`, `Payload` = generated message | one, `readLoop` |
| `Normalize` ([`normalizer.go:19`](../../../internal/push/normalizer.go)) | `*pbmsg.PBNotify` (the envelope) | `*domain.PushEvent` | **none** |

They are not two ends of a pipe: `decodeNotification` already consumed the
envelope, so `Normalize` has nothing left to be given, and `Notification` has no
way to be turned back into a `PBNotify` without re-marshalling. **C11 cannot
deliver a `domain.PushEvent` without resolving this**, and the three available
resolutions are:

| Option | Verdict |
|---|---|
| **Add `push.NormalizeNotification(*Notification) (*domain.PushEvent, error)`** | **Chosen — and §4.1 makes it the only one left.** The seam delivers a `domain.PushEvent`, so a path from `*Notification` to one must exist; the other two rows do not create one. ~30 lines: dispatch on `n.Type`, assert `n.Payload` to the generated pointer, call the five existing decoders, and never return `(nil, nil)` (§4.3). Additive to `internal/` (not externally importable, no wire or exported-type change), reuses the component [docs/VNEXT.md §5.2](../../VNEXT.md) already chose over `Manager`'s duplicate set and the one with the 439-line test file. |
| Re-marshal the payload into a `PBNotify` to feed `Normalize` | Rejected: a serialise/deserialise round trip on every frame, to reuse a function that was written for a different input type. |
| Duplicate the five decoders in `pkg/services` | Rejected: a **fourth** copy of the wire→domain mapping (§11.2), and it would also have to be duplicated again wherever the event is built. |

### 11.2 A third copy already exists, and it is worse than duplication

`pkg/transport/mappers.go` carries a **separate** set of wire→domain mappers over
the same generated DTOs — `MapBasicQotToQuoteEvent`, `MapTickerToTickerEvent`,
`MapOrderBookAskLevel`, `MapTradeDeliveryToTradeEvent` — with their own 8
`"0.001"` literals. So the push wire→domain mapping exists **twice** today
(`internal/push/decode.go` via `Normalize`, and `pkg/transport/mappers.go`), and
the two are not equivalent.

**The broker-queue mapping is not merely duplicated, it is wrong.**
`decodeBrokerEvent` ([`decode.go:71-90`](../../../internal/push/decode.go))
produces `domain.BrokerLevel{BrokerID, Quantity: MustNewQuantity("0")}` — a
**hardcoded zero quantity** — and discards `Item`, `Type`, and `Name`. But
[`dto.Broker`](../../../gen/hq/dto) carries **only** `Level`, `Item`, `Type`,
`Name`: there is no quantity and no price on the wire. The pull path gets this
right ([`domain/market.go:154-171`](../../../pkg/domain/market.go) →
`BrokerQueueEntry{Level, Item, Type, Name}` via
[`market.go:605-616`](../../../pkg/services/market.go)); the push path invents a
`Quantity` field and fills it with a fabricated `0`.

**A fabricated zero presented as an observed quantity is the same class of defect
P1 exists to eliminate** — a wrong number that reads as fact — and it is *newly
reachable* the moment C11 publishes a `BrokerEvent` to a caller. It is also, in
one specific way, worse than a duplicated mapper: **the type is what makes the
mistake look correct.** `domain.BrokerLevel.Quantity Quantity`
([`push_event.go:66-69`](../../../pkg/domain/push_event.go)) is a `Quantity`, so
`MustNewQuantity("0")` type-checks, appears in a field named like the three
sibling decoders' fields, and a reader comparing `decodeBrokerEvent` with
`decodeOrderBookEvent` sees the same shape and concludes the wire carries a
quantity. There is no compile-time or review signal that the value is invented.
**So the fix is a type change, not a literal change** — deleting the field and
adding `Item`, `Type`, `Name` is the only fix that makes the mistake
unrepresentable. Decision:

- **C11 does not consolidate the two mapper sets.** Consolidating means either
  moving the shared mapping into `pkg/domain` as exported `…FromDTO` functions
  (`pkg/domain` already imports `gen/hq/dto` and already hosts
  `SymbolFromSecurity`), which is a `pkg/domain` public-surface decision, or
  changing `internal/push`'s import direction, which is an ADR 0010 question.
  Neither belongs in a task whose subject is the orchestrator.
- **C11 fixes the broker mapping**, because it is the one defect on its own
  delivery path, and the fix is to give the push event the same four fields the
  wire has. That is a `pkg/domain` type change (`BrokerEvent`/`BrokerLevel`),
  which is safe *now* — `pkg/domain` is not reachable by a caller, which is
  exactly why it should be done before D1 wires the layer. §4.1's
  `domain.PushUpdate` is the second reason to do it in C11 rather than after:
  the envelope is what first carries a `BrokerEvent` to anyone.
- **The remaining duplication is recorded with an owner** (below), not absorbed.
- **Both this defect and the two wrong SPEC citations of §2.3 are recorded in
  `todos.md`'s C10 row** rather than fixed here: each one lives in a file outside
  this task's surface (`internal/push`, `pkg/hstong/stream`), and a design note
  that edits them is a design note nobody can check. `pkg/hstong` is additionally
  under ADR 0011, where a doc-comment correction would be permitted (guarantee 1
  is about exported type signatures, not comments) but is still a change to a
  package C10 has no business touching.

### 11.3 The routing key is unresolved by the repository, and the design reports it rather than guessing

For a subscription to receive anything, the orchestrator must match an incoming
frame to a subscriber. The candidates are `Notification.ID` (the `notifyId`) and
the code inside the payload's `security`. **They may not be comparable**, and the
repository cannot say:

- `Notification.ID`'s GoDoc says "for market pushes this is the security code"
  ([`client.go:195-196`](../../../internal/push/client.go)) — no format. §4.1 is
  what keeps this reachable: `domain.PushUpdate.ID` is a verbatim copy of it, so
  the orchestrator reads an envelope field and never touches `internal/push`. Had
  the seam kept `*push.Notification`, this question would have been unanswerable
  from `pkg/services` at all — the type would have been unnameable there and the
  routing key with it.
- The only shape in the repository is HTTP-side:
  `test/mockgateway/fixtures.go:191` uses `"00700.HK"`, a **market-suffixed**
  code, and that is a synthetic fixture written to be decodable, not an
  observation.
- `domain.NewSymbol` **appends** a suffix
  ([`symbol.go:43-54`](../../../pkg/domain/symbol.go): `fullCode = code + "." +
  market`), and `SymbolFromSecurity` derives `market` *from* the code
  ([`symbol.go:73-82`](../../../pkg/domain/symbol.go)). So for a code that
  already carries a suffix, `FullCode` becomes `"00700.HK.HK"`. That is a latent
  defect on the push path, and it is a second reason not to key on `FullCode`.

**The observation that settles it:** one G6 push frame — read `notifyId` and
`payload.security.code` and compare both with the codes sent in `/hq/Subscribe`.
Same run, same security.

**The design is defensive in the meantime, and the defensiveness is the
deliverable:** the subscription registry indexes on **both** `notifyId` and the
payload's `security.code`, normalised (upper-cased, and stripped of a trailing
`.HK`/`.US`/`.SZ`/`.SH` so both spellings hit the same entry). An update that
matches no subscription is **counted as `Unrouted` and reported once per
distinct id** on the client error stream. An update nobody receives while the
caller believes they subscribed is the single worst outcome this layer has, and
it must be the loudest thing in the log. C12 asserts the counter and the report;
G6 resolves the underlying question and the normaliser is then pinned by a test
carrying the observed codes.

## 12. The acceptance criterion that can fail

### 12.1 The problem, stated exactly

`make parity-enforce` reports `51/51 gap=0` on the tree as it stands, **and
would report `51/51 gap=0` on a tree where `PushOrchestration` does not exist** —
because `internal/push` speaks TCP/protobuf and will never name a
`client.Route`. Measured, not assumed:

```
$ go run ./scripts/paritygate --enforce
  PARITY: 51/51 gap=0 mode=enforce enforce=on
exit=0
```

`PushOrchestration` composes four already-wired methods
(`market.go:429`, `market.go:446`, `trading.go:589`, `trading.go:621`), so C10,
C11 and C12 move the parity number by **exactly 0**. A green guard across all
three is the failure the guard programme exists to prevent, and the run's
standing instruction — *do not invent a permanent exclusion list to make a gate
pass* — rules out the obvious dodge, because an exclusion list for a task that
is being *done* is a carve-out with no gate behind it.

**C14 narrowed a false positive and did not touch this.** What it removed was a
`client.Route`-typed *function parameter* being reported as a blind spot. The
blind spot here is different in kind: not a shape the guard mis-classifies, but a
whole transport it cannot observe. C14's evidence is that zero `client.Route`
struct fields exist in any scan root, and that is still true.

### 12.2 The three options, weighed

| Option | Verdict |
|---|---|
| **An exhaustiveness test over the `TopicID` → payload relation** | **Chosen**, in the form below. But the *naive* reading of this option — a hand-written 11-row table like `TestNotifyTypeForTopic` — is the one thing that must not be built, because §2.2 shows it passes when a 12th constant is added. The exhaustiveness must be **derived**. |
| **A test that the mapping covers every declared `TopicID`** | The same test, and it is a genuine strengthening *only if "every declared" is read from the declaration site* rather than from a written list. It is chosen together with the first, not against it: they are the key-side and the value-side of one property. |
| **A package-level test that fails if a new `TopicID` is added without a mapping entry** | This is not a third option; it is the *name* of the first, and it is a correct description of what the chosen test does. Listed separately because it is the one a reader is most likely to accept uncritically, and the one most likely to be implemented as a snapshot of today's 11 rows. |

Rejected as substitutes:

- **A new `scripts/` guard** (a second `paritygate`-shaped tool for push). Rejected
  because it adds a CI job, a Makefile target, a release-checklist row and a
  second thing to keep in step, for a property that a `go test` in the owning
  package already checks. `design-parity-guard.md` §8.3's "not a second, weaker
  mode" applies.
- **A permanent exclusion list in the parity guard.** Rejected by the run's
  standing instruction, and C1a rejected the same shape for the same reason.
- **Coverage as the criterion** (`pkg/services` ≥85%). Rejected: coverage measures
  how much of a function ran, not whether a relation is complete. A `switch` with
  11 of 12 cases is 92% covered and wrong.
- **A doc-comment promise** ("if you add a `TopicID`, add a case"). Rejected: that
  is what §2.2 shows the released layer did, and it did not prevent the rot.

### 12.3 The criterion

**One test file in `pkg/services`, four assertions, no hand-maintained list.**
The key set is parsed out of the declaration site with `go/ast` at test time; the
expected values are parsed out of `docs/SPEC.md` §4 with the guard's own CRLF
discipline.

| # | Assertion | Kills |
|---|---|---|
| **G1 — key-space totality** | For every `TopicID` constant declared in `pkg/types/enums.go`, `NotifyTypeForTopic` returns `ok == true`. And the parsed count equals the number of constants whose names the mapping's `switch` actually mentions, so removing a constant does not leave an orphan case. | a new `TopicID` with no mapping entry (the §2.2 rot); a stale case for a removed constant |
| **G2 — value-space closure, both directions** | Every value `NotifyTypeForTopic` returns is a declared `NotifyMsgType` constant. **And** the declared `NotifyMsgType` constants the mapping never returns are exactly `TrsStockDeliverMsgType`, `TradeStockDeliverMsgType`, `FuturesTradeStockDeliverMsgType` — named by their Go identifiers, so a rename in `pkg/types` breaks the test rather than passing it. | a typo'd or invented payload type; a whole payload group losing its topic (11 rows all edited to one type passes G1 alone); a mapping that names a type `pkg/types` does not declare |
| **G3 — the relation is the one SPEC §4 states** | Parse the §4 table and assert `NotifyTypeForTopic` agrees with it, group by group. | a *wrong* mapping that is internally consistent — G1 and G2 are both blind to this, and it is the defect that actually ships (topic 25 delivering ticker updates) |
| **G4 — delivery is live** | For each declared `TopicID` constant, subscribing to it installs the handler for the mapped type and a synthetic frame of that type reaches the subscriber. | a mapping that is correct but undelivered — the 7th handler registered, the 8th topic mapped, nobody wired |
| **G5 — non-vacuity** | The `go/ast` walk must find **≥ 8** `TopicID` constants and **≥ 7** `NotifyMsgType` constants in `pkg/types/enums.go`, and the §4 parse must find **exactly 4** group rows. A zero- or wrong-match is an **error**, never a pass. | `pkg/types/enums.go` renamed or split, leaving an empty parse that reads as "everything is covered" |

**Why it cannot rot.** The expected key set is **read from the declaration site**
and the expected values are **read from `docs/SPEC.md`**. There is no list in the
test for a human to maintain, so there is nothing to forget. Concretely:

- Adding a `TopicID` constant to `pkg/types/enums.go` — a one-line change, and
  the natural one a vendor topic addition makes — breaks **G1** on the next
  `go test ./...`, immediately and with the constant's name in the message.
- Changing the mapping without changing SPEC §4 breaks **G3**.
- Changing SPEC §4 without changing the mapping breaks **G3** in the other
  direction, which is the correct direction for a *specification*: the document
  and the code are made to agree deliberately rather than by coincidence.
- Deleting or emptying the parse breaks **G5** instead of passing vacuously —
  the same rule `design-parity-guard.md` §8.4 states for its own inputs
  ("a zero-match must never read as 'all clear'") and
  `internal/layering/layering_test.go:108-110` enforces for its import walk.

**The irreducible limit, stated rather than glossed.** No gate in this repository
can detect the *deletion* of the criterion test, and the coverage gate cannot
either — a deleted test lowers the covered-statement count but the threshold is
85% and the package is at 100%. What the run has established for exactly this
situation is the negative control: the criterion must be **shown to fail** on a
planted fixture, and the output plus the exit code recorded in `todos.md`'s
Evidence cell, per `design-parity-guard.md` §9.1. Three plants, each against a
throwaway `--root` tree or a temporary local edit reverted immediately:

1. Add a 12th `TopicID` constant to `pkg/types/enums.go` with no mapping case →
   **G1 fails**, naming the constant.
2. Change one `switch` case in `NotifyTypeForTopic` (e.g. move `TopicOrderBookArcabook`
   from order book to tick) → **G3 fails**, naming SPEC §4's row.
3. Point the §4 parse at a nonexistent file → **G5 fails** with the input-error
   message, and must **not** print "covered".

This is the same "a guard that cannot fail is not a guard" discipline that B5
satisfied by raising the coverage threshold to 100.1, and that
`internal/layering` satisfies by refusing to pass when its walk found nothing.

### 12.4 What "done" means for a task no gate measures

Stated plainly, because the run asks for it:

- **C10 is done** when this note exists, every decision in §1 is recorded with
  its derivation and its rejected alternatives, and §12.3 is specific enough
  that C12 implements it without re-deriving it. No code, no gate.
- **C11 is done** when `go build ./...`, `go vet ./...`, `gofmt -l .` and
  `go test -count=1 ./...` are green, `pkg/services` and `pkg/transport` hold
  their coverage, and **G1–G5 pass**. The run's acceptance column for C11 must be
  changed from "V1,V2,V3b" — V3b is the parity guard, which is provably
  indifferent to this work — to name G1–G5. **That edit to `todos.md` is part of
  C11's definition of done**, and it is the mechanism by which a task the guard
  cannot see becomes a task with a written, checkable bar.
- **What the criterion protects, and what it cannot.** G1–G5 protect the *shape*
  of the relation: that it is total, closed, correct against the specification,
  and wired to delivery. They cannot protect the *protocol truth* — whether the
  Gateway really sends `OrderBookFullNotify` for topic 25, and whether `notifyId`
  matches the code sent to `/hq/Subscribe`. That truth is SPEC §4 plus one G6
  observation (§11.3), and no test in this repository can substitute for the
  observation. The criterion makes the *disagreement* loud; it cannot make the
  agreement true.

## 13. Does `PushOrchestration` need its own scan root?

**No. Today's three roots are correct, and the derivation is a count, not a
preference.**

`PushOrchestration` names **no route of its own**. Its HTTP half is satisfied by
`*MarketService` and `*TradingService`, whose four `client.Route` constants are
already credited inside `pkg/services` — a scanned root — and were credited
before this task existed:

| Route | Credited at | Wired by |
|---|---|---|
| `/hq/Subscribe` | `pkg/services/market.go:429` | shipped |
| `/hq/Unsubscribe` | `pkg/services/market.go:446` | shipped |
| `/trade/TradeSubscribe` | `pkg/services/trading.go:589` | C15 |
| `/trade/TradeUnsubscribe` | `pkg/services/trading.go:621` | C15 |

Its TCP half names no route and never will: `internal/push` speaks protobuf on
`127.0.0.1:11112`. So C10–C12 contribute **zero** references, and the guard is at
`51/51 gap=0` before and after.

**Adding a root would make the guard weaker, not stronger.** The guard's value is
proportional to how few packages it scans, because every root added is a chance
for a reference to be credited somewhere it should not be. A push root containing
zero routes adds a directory to walk and nothing to check.

**C2's reasoning holds, and the direction of the risk is worth stating precisely
because it is not symmetric:**

- **A root that goes missing is fatal, loudly.** The guard treats "any scan root
  missing" as an error (`design-parity-guard.md` §8.4) and "zero references
  found" as an error, so a renamed or deleted root cannot read as "nothing
  referenced".
- **More importantly for this task, a *missing* root for push makes the guard
  stricter, not laxer.** If `PushOrchestration` were ever moved to a package under
  no root and it named a route, the guard would report that route **unwired** and
  `--enforce` would fail. The blind spot C14 could not remove is therefore a
  *false negative in the other direction only* — the guard cannot see a
  non-HTTP reference — and never a false green caused by a missing root.
- **A root added after the code lands leaves the work invisible until then**, which
  is the real cost, and the mitigation is not a guard feature but a task-level
  one: **whoever adds a route reference to a package outside the three roots must
  extend `defaultScanRoots` in the same commit** (`scripts/paritygate/main.go:167`),
  and that commit must also refresh the real-tree baseline in
  `scripts/paritygate/main_test.go` — which is C4b's chore, still `todo`, and
  which C5, C7 and C15 have each already had to do by hand.

**Forward obligation this note records.** If a later change makes
`PushOrchestration` a distinct package (for example to give push its own
`Close`/lifecycle), the same commit must add that package to
`defaultScanRoots` *and* the task's acceptance must name it. The obligation is
recorded here because the parity guard cannot record it for itself: a guard that
warned about "code that composes a transport" would be guessing, and
`design-parity-guard.md` §8.3 forbids extending it to anything it cannot
establish from the tree.

## 14. The C14 asymmetry, and why it constrains nothing proposed here

C14's rule, stated from the guard's own GoDoc
([`scripts/paritygate/main.go:877-908`](../../../scripts/paritygate/main.go)):

- A `client.Route`-typed **function or method parameter** is suppressed, and will
  stay suppressed forever. It states what a signature *accepts*; a value reaches
  it only at a call site, and every call site inside the scan roots is walked.
  `Executor.Do` at [`pkg/services/executor.go:36`](../../../pkg/services/executor.go)
  is the permanent example.
- A `client.Route`-typed **struct field** in a scan root is **fatal** under
  `--enforce`, because "a route can be withdrawn into a field, or a route can be
  implemented through one, and neither direction is resolvable from the
  declaration".

**It constrains nothing in this design, and the reason is structural rather than
a compliance dodge: `PushOrchestration` never names a route at all.**

- `TopicSubscriber` and `OrderPushSubscriber` (§4) contain **no** `client.Route`
  in any signature. `*MarketService` and `*TradingService` satisfy them
  structurally, so the composition adds no route vocabulary, and there is nothing
  for the guard to see and nothing for it to miss.
- The one interface that crosses a transport boundary names `domain.PushUpdate`,
  **not** a route and **not** an internal type. `internal/push` is named only
  inside `pkg/transport`'s implementation, which is where ADR 0010 rule 4 puts
  it, so the guard's scan of `pkg/services` sees a domain type where §4.1's
  table says it must see one.
- Therefore **no config struct, option, or field may carry a `client.Route`** —
  not because the guard forbids it (the guard would simply *fail* it, which is the
  same outcome) but because §10's composition makes the route unreachable from
  the orchestrator. If a future need appears, the shape is a *method call* on
  `MarketService`, with the constant at the call site inside `market.go` where
  four already are, and never a stored value.

Two general forms, for the record:

- **An `Executor.Do`-style seam is unconstrained** — and the repository already
  depends on that being so, since `Executor` must keep naming `client.Route` and
  `client.Codec` ([`design-parity-guard.md` §4.1`](./design-parity-guard.md),
  [docs/VNEXT.md](../../VNEXT.md) step 5: "inverting the dependency removes the
  dependency on the concrete type, not on the client's shared route vocabulary").
  The lesson generalises past routes: **an interface that names the shared
  vocabulary is a boundary; a struct that stores it is a hole.** §4.1 draws
  exactly where that lesson stops, and the qualifier is worth stating here
  because the two sections would otherwise read as contradicting each other:
  the vocabulary must be **nameable by whoever implements the interface**.
  `client.Route` qualifies, which is why `Executor` is right. A type under
  `internal/` does not — not for a caller, not for a fake, not for anyone
  outside this module — so naming one is a hole in the same sentence.
- **A config struct with a route field is now unshippable without a recorded
  decision**, and if it were ever wanted, the decision would be an ADR — not a
  waiver, because there is no list of waived items to add it to.

## 15. Rejected alternatives

| Alternative | Why rejected |
|---|---|
| Keep `*push.Notification` in the `PushTransport` signature | §4.2.1. The rule's rationale is HTTP-specific, a type reference is not a call, it costs no code, and `Executor` is precedent for naming another layer's vocabulary — and it still loses: `internal/` is unimportable outside this module, so the seam becomes **unimplementable by any third party**, a machine-checked boundary becomes an asserted one, and v1.0 would freeze a package ADR 0011 never promised to stabilise. |
| A `pkg/services`-local `Notification` mirror with `Payload any` | §4.2. Republishes the released layer's `Payload any` defect, which §11 exists to remove, and still pays §4.3's bridge cost to get a typed value. |
| Narrow the seam to `func(id string, ev *domain.PushEvent)` | §4.2. Drops `Time`, so §9.3 parses a `string` out of a domain event on the hot path, and a parameter list cannot gain a field — which §11.3's open questions may require. The closest loser, and the only one worth revisiting. |
| Narrow the seam's *key* to `types.TopicID` | §4.2. `pkg/transport` may not import `pkg/services`, so it cannot hold the topic→type relation; the caller must translate, which is §5.6's two-vocabulary invariant. |
| Put the mapping in `internal/push` | The temptation is real — the frame handling is there. `internal/push` is below the HTTP layer and has no topic concept at all ([docs/VNEXT.md §5.1](../../VNEXT.md): subscribe is HTTP, per-subscription), so putting a topic relation there inverts ADR 0010's direction and puts wire vocabulary inside an ADR-0011-protected package. It also cannot be the *only* copy, because `pkg/services` must not import it. |

| Put the mapping in `pkg/types` as a method | Go cannot declare a method on a non-local type (A6), and `pkg/types` is ADR-0011-protected. Widening it for a v-next-layer concern is the wrong direction for a frozen surface. |
| A `Topic` struct carrying both halves | §5.4. A struct admits an inconsistent value, which is the entire failure the mapping prevents. |
| A `map[TopicID]NotifyMsgType` | §5.2. Its zero value is `TrsStockDeliverMsgType` — a real type — so a missing entry would deliver order notifications to a quote subscriber. |
| Let a caller subscribe by `NotifyMsgType` | §5.6. Three of the seven types have no topic, so the entry point would have to encode a variant the wire does not carry, and a market-type subscription with no `/hq/Subscribe` would wait forever. |
| Guess a payload for an unmapped topic | §5.3. The failure is a caller mis-parsing every event with every accessor, silently. |
| Per-subscription HTTP subscribe (released-layer parity) | §6.2. It is a reference-count bug: the first cancel tears down a stream another caller is using. |
| Coalesce on topic alone | Ignores the security list; the other subscriber's instruments never arrive. |
| Keep `WithOnReconnect` re-subscribe out of v-next and make it caller-driven | "The answer must not depend on a notification nobody is watching" — a caller-driven re-subscribe is a notification nobody is watching. |
| Retry a failed re-subscribe inside the hook | Unbounded retry on the `Run` goroutine stalls the read loop; the next reconnect tries again anyway. |
| Keep only the per-subscription `Errors()` channel | §7.1. A reconnect with zero subscriptions is unobservable, and transport read errors never arrive. |
| Keep both a per-subscription and a client-level channel | A caller with 50 subscriptions must drain 50 channels or lose the diagnostics — the failure the single stream removes. |
| Block on a full update channel | §8.1. The drop already happened upstream at `push.dispatchBuffer`; blocking recovers nothing and stalls every other subscriber of the same type. |
| One uniform backpressure policy | §8.3. It forces a choice between corrupting an order book and losing a fill. |
| Adopt `internal/push.FreshnessMonitor` keyed by subscription index | §9.2. Its key is a single `int` named `TopicID` and its unit is a topic, while the subscription unit is `(topic, security)`; reusing it means a naming lie in an ADR-0011-protected type. |
| Drop out-of-order frames | §9.3. A silent loss, and wrong across a reconnect where an earlier timestamp is expected. |
| Sequence-based gap detection | R4: the push envelope carries no per-event sequence, so it is not implementable. `DedupCache` was removed as inert for this reason. |
| A separate `OrderPushService` | §10. Two owners of one socket, and a mutual reference between the two types. |
| A new scan root for push | §13. Zero routes to credit, one more directory to walk, and the guard's value falls with every root added. |
| A new `scripts/` guard for push parity | §12.2. A new job, target, checklist row and thing to keep in step, for a property `go test` already checks in the owning package. |
| Consolidate the three wire→domain mapper sets in C11 | §11.2. It is a `pkg/domain` public-surface decision and an ADR 0010 direction question, not a push-orchestration task. Recorded with an owner instead. |
| A `WithTick` option on the orchestrator | §16. `design-tick-model.md` §2.3 rejects a client-global tick because a portfolio spans HK and US instruments; the instrument is the right scope, and push has no instrument master. |

## 16. What C11 builds, and what C12 tests

Signatures, so neither task re-derives this note. `pkg/services/push.go`:

```go
func NewPushOrchestration(sub TopicSubscriber, tr PushTransport, opts ...PushOption) *PushOrchestration
func (o *PushOrchestration) Connect(ctx context.Context) error   // idempotent; ErrClosed after Close
func (o *PushOrchestration) Errors() <-chan error                // client-level; closed by Close
func (o *PushOrchestration) Close() error                        // idempotent, leak-free
func (o *PushOrchestration) NotifyTypeForTopic(t types.TopicID) (types.NotifyMsgType, bool)
func (o *PushOrchestration) Subscribe(ctx context.Context, topic types.TopicID, sec ...*Security) (*Subscription, error)
func (o *PushOrchestration) SubscribeOrders(ctx context.Context, accountID domain.AccountID) (*OrderSubscription, error)
```

Options: `WithPushAddr`, `WithDialer`, `WithReconnect(min, max)`,
`WithBuffer(n)` (default 64), `WithVerification(key, required)` (default off),
`WithReadDeadline(d)` (**default off**, per plan assumption 3 and R3),
`WithResubscribeTimeout(d)` (default 10s), `WithTradeSubscriber(OrderPushSubscriber)`.

`Subscription`: `Updates() <-chan domain.PushUpdate`,
`TopicID() types.TopicID`, `Securities() []*Security` (a copy),
`Dropped() uint64`, `OutOfOrder() uint64`, `LastSeen() time.Time`,
`Stale(after time.Duration) bool`, `Cancel(ctx) error`.
`OrderSubscription`: `Updates() <-chan domain.PushUpdate`, `Dropped() uint64`,
`Cancel(ctx) error` — and **no** `TopicID`/`Securities`, because the absence of
those methods is the type-level statement that a trade subscription has no
topic. Both channels carry `domain.PushUpdate` and not a `services`-local type,
because `pkg/transport` may not import `pkg/services` and could not spell one
(§4.1).

`pkg/transport/push.go`: `NewPushTransport(opts ...PushOption) (services.PushTransport, error)`
wrapping `*push.Client` — the **first** import of `internal/push` in that package,
which `doc.go:26-27` has always permitted — converting each `*push.Notification`
into a `*domain.PushUpdate` (`Type`/`ID` verbatim, `Time` via `time.UnixMilli`,
`Event` via the new `push.NormalizeNotification`) and forwarding
`push.Client.Errors()`. It also owns the ~30 lines that map a `NotifyMsgType` to
a domain event per §4.3, which is where ADR 0010 rule 2 puts wire→domain
conversion and where `internal/push` stops.

`pkg/domain/push_event.go`: one new type, `PushUpdate` (§4.1, §11), beside the
`PushEvent` interface. `pkg/domain` is outside ADR 0011's guarantee, so this is
free now and a deprecation after v1.0.

`internal/push`: new exported `NormalizeNotification(*Notification) (*domain.PushEvent, error)`,
which **never** returns `(nil, nil)` — the five empty-payload arms substitute a
`domain.SystemEvent{Code: "EMPTY_PAYLOAD"}` (§4.3). `Normalize` itself is
unchanged.

`docs`: `pkg/services/doc.go` gains the two entry vocabularies;
`docs/streaming.md` gains a v-next section at E1/E2, not here.

**C11's non-negotiable preconditions**, each from a finding above:

1. The 8 `"0.001"` literals in `internal/push/decode.go` become `"0"` (§17). This
   is P5 step 1 restricted to one file, and C11 is the caller that makes them
   reachable.
2. The broker mapping stops fabricating a quantity — as a **type** change
   (`BrokerLevel` loses `Quantity`, gains `Item`/`Type`/`Name`), not a literal
   change, so the mistake becomes unrepresentable (§11.2).
3. `PushOrchestration` contains no `client.Route` in any field (§14).
4. **`push.NormalizeNotification` exists and never returns `(nil, nil)`.** It is a
   precondition, not a preference: §4.1's seam delivers a `domain.PushEvent`, and
   `decodeNotification` has already consumed the `PBNotify` that `Normalize` needs,
   so the seam cannot be implemented without it (§4.3, §11.1).
5. **`internal/layering` passes with no edit to it.** §4.1 chose
   `domain.PushUpdate` precisely so that the table at
   [`layering_test.go:135-190`](../../../internal/layering/layering_test.go) needs
   no scoped exception, `pkg/services` needs no new import at all for the seam,
   and `TestExecutorInterfaceIsDeclaredByServices` keeps its meaning because
   `PushTransport` is declared in `pkg/services` like `Executor` (ADR 0010 rule 6).
   **The exception route is now closed, not deferred** — it was only ever open
   because the signature was undecided.

**C12's non-negotiable tests**, in the order they earn their keep:

| # | Test | Kills |
|---|---|---|
| T1 | G1–G5 (§12.3) as five test functions | the §2.2 rot; an inconsistent-but-total mapping; an undelivered topic |
| T2 | Subscribe to each declared `TopicID`; assert the `/hq/Subscribe` body, the installed handler, and a synthetic frame's delivery | a topic that subscribes but never delivers |
| T3 | Two subscribers, same `(topic, securities)` in **different order** → **one** `/hq/Subscribe`; cancelling one sends **no** `/hq/Unsubscribe`; cancelling the last sends one | the §6.2 reference-count bug |
| T4 | Reconnect over a substituted dialer + `net.Pipe` (never a sleep, per AGENTS.md): assert one `ErrReconnected` on the client stream **with zero subscriptions**, `errors.Unwrap` → `io.EOF`, and the automatic re-subscribe | A4's T7; the §7.1(1) invisible-reconnect hole |
| T5 | Re-subscribe fails → the subscription's `Updates()` is **closed** and a terminal error is on the client stream | the §7.3(2) silent-dead-subscription hole |
| T6 | A slow consumer: fill the buffer, assert `Dropped() > 0` and that **no other subscriber of the same type** is affected | a block-instead-of-drop regression; a silent drop |
| T7 | Trade delivery overflow asserts an error and a counted drop, and asserts the update stream was **not** silently short | the §8.3 no-silent-loss rule |
| T8 | Signed frame delivered; tampered frame dropped and `errors.Is(err, push.ErrSignatureMismatch)`; the error arrives with **no subscription live**; an unparseable key fails `Connect` not `New` | `verify_test.go`'s standard, plus the §7.1(1) hole on the verification path |
| T9 | Per-subscription `Stale(after)`: a non-positive `after` restores 5 minutes and does not mark a fresh subscription stale | `NewFreshnessMonitor`'s rule adopted at `freshness.go:45-50` |
| T10 | A frame whose `notifyTime` is older than one delivered → `OutOfOrder() == 1`, frame **still delivered**, one error, no flood | a drop-on-out-of-order regression |
| T11 | An update matching no subscription → `Unrouted` counter and one error per distinct id | §11.3's silent-no-data hole |
| T12 | `Close()` during a reconnect leaves no in-flight `/hq/Subscribe`; `Close` is idempotent and `Run` returns | the §7.3(1) context-parent hole; AGENTS.md's leak-free-`Close` rule |

**Where each test file is allowed to live, because §4.1's rule is enforced over
test files too.** [`layering_test.go:44-46`](../../../internal/layering/layering_test.go)
parses `_test.go` files deliberately — "a layering rule that holds for production
code but is violated by a test still obscures the boundary". So:

- **T1–T3, T5–T7, T9–T12 live in `pkg/services`**, against a fake `PushTransport`.
  A fake is now possible *because* §4.1 chose a public type: a test in
  `pkg/services` can implement `SubscribeTypes(types.NotifyMsgType, func(*domain.PushUpdate))`
  with no import beyond `pkg/domain` and `pkg/types`. Under the §4.2.1 alternative
  this suite could not exist at all, which is the practical form of that
  alternative's cost.
- **T4 and T8's `errors.Is(err, push.ErrSignatureMismatch)` half live in
  `pkg/transport`**, which may import `internal/push`. **T8 also asserts a
  `pkg/services`-named sentinel** for the same failure, because §4.3's rule is
  that a sentinel a caller is expected to match must be nameable by the caller,
  and the internal one is not. The services half is what the v1.0 API owes.
- **T2's synthetic frame is fed to `push.Client` through the substituted dialer**,
  not hand-built as a `*domain.PushUpdate`, so it exercises `NormalizeNotification`
  and the broker/tick preconditions rather than bypassing them.

**Coverage.** `pkg/services` and `pkg/transport` are both registered in
`scripts/coverage_gate.go` at 85.0, and both sit at **100.0%** today. C11 must
not lower either. The measurement must be from a clean clone — AGENTS.md records
that a dirty tree overstated `internal/push` by 2.8pp and published a wrong
figure.

## 17. The tick

**The orchestrator must not carry a tick, must not be configurable with one, and
its payload path must stop inventing one.** Three parts.

1. **Where the tick already is.** `internal/push/decode.go` has 8 hardcoded
   `"0.001"` literals in `domain.MustNewPrice(…, "0.001")` calls — `decode.go:21-25`
   (five quote prices), `:38` (tick price), `:58` (order-book level), `:95`
   (trade price). Eight of P5's 40. They are reached only by
   `push.Normalize`, which has **no production caller** (§3), so nothing is
   corrupted today.
2. **What changes when C11 lands.** C11 makes `Normalize` reachable from a public
   API, which is P3's exact framing for `Price.Round()`: *"It is one caller away
   from being corrupted, and the type makes the mistake look correct."* A caller
   who reads a push price and calls `Validate(true)` against the tick `0.001` the
   SDK invented gets a spurious rejection; a caller who calls `Round()` gets
   `0.0005` rounded to `0.000`. `Price.Round` still has no production caller, but
   that is a property of *this* repository's callers, not of a shipped public API.
3. **The decision.** Per
   [design-tick-model.md](./design-tick-model.md) §2.1 — "a read-path price
   carries no tick", `MustNewPrice` on a wire-derived price passes `"0"`, and
   `Price.Validate` skips the modulus check and `Price.Round` returns the value
   unchanged when the tick is zero. **A push price is the most wire-derived price
   in the SDK**: the Gateway sent the digits, and the SDK has no instrument
   master for the instrument. `"0"` states "I observed this; I do not know this
   instrument's tick schedule" in the type rather than in a comment.

**No `WithTick`, and no `TickSchedule` on the orchestrator.**
`design-tick-model.md` §2.3 rejects a client-global tick because a portfolio spans
HK and US instruments; §2.2's `TickSchedule` interface is a **write-path**
concern (`validatePriceForHK`, the lot check) and belongs to `TradingService`,
which has a caller-supplied price to validate. Push has no write path and no
instrument master, so it has nothing to schedule from. The only tick that may ever
be a *value* is `domain.Tick` (P5 step 2), used for
`services.OrderBookResponse.TickSize` — and that is a pull response, not a push
frame.

**Sequencing.** This makes C11 depend on a one-file slice of P5, which is a
recommendation, not a hard requirement. The honest statement: if P5 slips past
C11, then C11 ships a public API whose payloads carry a tick the SDK invented,
which is **strictly worse than today** — today the defect is reachable only by a
test. Eight literal edits is a small price for not doing that, and P5's own row
already records that nothing else in the run will implement them.

## 18. What the repository cannot answer, and what would settle it

Recorded rather than guessed, per C1a and C2a's discipline.

| # | Question | What would settle it | Until then |
|---|---|---|---|
| Q1 | Does the Gateway really send `OrderBookFullNotify` for topic 25, and `TickerNotify` for 28? | One live `/hq/Subscribe` + push frame per group. SPEC §4 is the only source and it is documentation. | §12.3 G3 pins the code to SPEC §4, so a wrong SPEC and wrong code agree and G3 stays green. This is the criterion's stated limit (§12.4). |
| Q2 | Is `notifyId` the same string as the `security.code` sent to `/hq/Subscribe`, and is it market-suffixed? | One G6 push frame, comparing both against the subscribe body. | §11.3's dual index + normalisation + the `Unrouted` counter and its error, so a mismatch is loud. |
| Q3 | Is a `/hq/Subscribe` idempotent, and does a single `/hq/Unsubscribe` remove only the securities named? | Two live subscribes and one unsubscribe for a subset. | §6.2's coalescing only *reduces* duplicate calls, and the released layer's behaviour is preserved for the non-duplicate case. |
| Q4 | Does the Gateway require `/trade/TradeSubscribe` per account or per session? | Two accounts on one connection. | `stream.go:541-545` implements per-connection (one call, first-to-last) and the design follows the released layer, because a divergence here is a divergence in *call count* with no local evidence either way. |
| Q5 | What is the maximum inter-frame gap on a quiet market? | One observed G6 run. | `WithReadDeadline` stays **off** by default, per plan assumption 3 and R3. Unchanged by this design. |
| Q6 | Is `internal/push.FreshnessMonitor` still wanted, given §9.2's key objection? | A maintainer decision alongside P5, which already owns `internal/push`. | Left unused and documented as such rather than adopted with a wrong key. |

## 19. Corrections to the brief

1. **"That mapping is stated nowhere in code"** — false. `notifyTypeForTopic`
   (`stream.go:624-637`) plus two tests cover all 11 constants. §2. The real gap
   is that it is unexported and pinned by a hand-written table.
2. **"The released `pkg/hstong/stream` … states no mapping either"** — false, and
   the specific instance named (`topicID: types.TopicBasicQot` paired with
   `Type: types.TickerNotifyMsgType` in a test) is not a wrong pairing: the
   pairing at [`accessors_test.go:86-89`](../../../pkg/hstong/stream/accessors_test.go)
   is a `Subscription` field and a `TopicID()` accessor assertion, and the
   topic→type pairs are asserted in `TestNotifyTypeForTopic`, where
   `TopicBasicQot` correctly maps to `BasicQotNotifyMsgType`.
3. **"`internal/push` … 3,592 lines"** — the number is a valid count under a
   different rule, not a stale one: 3,592 is the **blank-line-excluded** total
   across every `.go` file in the package, tests included. Every line, every
   `.go`, is 4,009 and the non-test surface C11 composes is 1,481. The brief is
   still internally inconsistent, because its per-file figures are below what its
   own rule yields. §3. Sizing only, and the correction is stated as a counting
   rule so a reader who reproduces 3,592 knows which row they are looking at.
4. **"The HTTP half is already wired … so this composes two transports rather
   than adding a third"** — right, and the design leans on it harder than the
   brief suggests: **both HTTP seams are satisfied structurally by existing
   methods with no adapter code**, which is what makes §13 (no new scan root) a
   count rather than a judgement.
5. **The brief frames the C14 constraint as something the design must work
   around.** It does not: `internal/layering/layering_test.go:166-176` already
   forbids `pkg/services` from importing `internal/push`, so the interface seam
   was **already mandatory**. C14's rule binds nothing that was not bound
   already. §14. §4.1 adds the second half, which the brief also did not state:
   the same rule forbids `pkg/transport` from importing `pkg/services`
   (`:148-153`), so the seam's payload type had exactly one lawful home and no
   freedom at all.
6. **The brief's `internal/push` file list omits `doc.go`** and understates each
   file's length (81/115/219/127/110 against 90/135/236/139/119). The current
   figures are the ones [docs/VNEXT.md §5](../../VNEXT.md) records, so the brief
   predates the retirement work.
7. **Not stated in the brief, and load-bearing for C11:** the two `internal/push`
   decoders cannot meet (§11.1), the wire→domain mapping exists twice with the
   broker one fabricating a quantity (§11.2), and the orchestrator's routing key
   is unresolved (§11.3). Any of the three can make C11 fail to compile or ship
   a wrong number, and none is visible from the brief's inventory.

## 20. Verification run for this task

No source file changed — this is a no-change baseline, not a proof of a fix —
and the checks were **re-run after the §4.1 amendment** so the numbers below
are the ones a reader of the amended note gets. Run with
`$env:GOTMPDIR="C:\Users\Tchan\AppData\Local\Temp\opencode"`; `make` is
not installed on the dev host, so targets are invoked directly.

```
$ gofmt -l .
(no output)

$ go build ./...
exit=0

$ go vet ./...
exit=0

$ go test -count=1 ./...
exit=0

$ go test -count=1 ./internal/layering/
ok  	github.com/shing1211/hstongapi4go/internal/layering	0.313s

$ go run ./scripts/paritygate --enforce
  PARITY: 51/51 gap=0 mode=enforce enforce=on
exit=0
```

**What the layering run does and does not prove about §4.1.** It cannot prove a
signature that does not exist yet compiles — C11's code is that proof. What it
*does* prove is the half that was actually in doubt: the two import edges the
decision relies on are already live in the tree and already accepted by the
table. `pkg/domain` appears in **no** rule's `forbidden` list
([`layering_test.go:135-190`](../../../internal/layering/layering_test.go)),
`services → domain` is exercised by 24 files in `pkg/services` today, and
`services → transport` is exercised by
[`pkg/transport/mappers.go:14`](../../../pkg/transport/mappers.go). So the
decision is not "a new edge that will probably be allowed"; it is "two edges that
already exist, pointed at a package the seam was always going to have to name".

The parity line is the point of §12.1: it is the number the guard reports today,
and it is the number it would report if `PushOrchestration` did not exist.

```
$ python scripts/check_links.py
check_links: scanned 109 Markdown files, 0 unresolved link(s)
exit=0

$ git status --short
?? docs/runs/2026-09-26-vnext-parity-wire/design-push-orchestration.md
```

**Encoding.** Both edited files were written by the editor and never
round-tripped through a PowerShell read/write, because `Get-Content` decodes
UTF-8 as the ANSI code page and silently corrupts every non-ASCII character in a
file like this one. The non-ASCII count was taken before and after each edit, and
the distinct code points were re-listed afterwards. (This paragraph is ASCII on
purpose: a table that spelled the glyphs would change the number it reports.)

| File | Before | After | Distinct code points after (name / count) |
|---|---|---|---|
| `design-push-orchestration.md` | 275 | 397 | section-sign 178, em-dash 160, right-arrow 32, en-dash 12, ellipsis 9, greater-equal 3, union 2, multiply 1 |
| `todos.md` | 279 | 314 | em-dash 152, right-arrow 55, section-sign 50, en-dash 17, greater-equal 14, ellipsis 11, middot 5, double-arrow 3, element-of 2, less-equal 1, multiply 1, CJK 3 |

Both are gains and neither is a substitution: `U+FFFD` appears in neither file,
and the three CJK characters in `todos.md` are the pre-existing vendor name in
C2a's provenance note.

One caveat on the docs target, the same as
[design-reconnect-cause.md](./design-reconnect-cause.md) §7: `mkdocs.yml`
`exclude_docs` lists `runs/`, so `mkdocs build --strict` does not build this
file and `check_links.py` is what validates its links. `check_i18n.py` is
unaffected because the run folder has no translations.

No `.go` file was modified. Nothing was committed.
