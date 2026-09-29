# Design: the opt-in constructor for the v-next layer

- **Run:** `2026-09-26-vnext-parity-wire`
- **Task:** D1 — design the opt-in constructor (placement, naming, ADR 0011)
- **Status:** design only. No code in this task; D2 implements, D3 extends `internal/layering`, D4 covers it e2e.
- **Baseline:** `v0.1.23` (`b0a4686`)

## 1. What the task is, stated as a question

The v-next layer is complete at 51/51 endpoints and 100% coverage on
`pkg/services`, and **no caller can reach it**. `docs/VNEXT.md:20` records this
as "only tests and the coverage gate import it". D1 must decide where an opt-in
entry point lives, what it is called, and what it must not do to ADR 0011.

## 2. The premise in the task brief is wrong, and the correction is most of the answer

The tracker row for D1 says "Design opt-in constructor (placement, naming, ADR
0011)", which reads as though no v-next composition exists. One does, and it has
been reachable from outside the module since C11. This is verifiable, not
inferred:

| Half | Declared | Satisfied structurally by | Established |
|---|---|---|---|
| `services.Executor` (`Do`, `JSON`) | `pkg/services/executor.go:32` | `*client.Client` | step 5 |
| `services.TopicSubscriber` | `pkg/services/push.go:151` | `*MarketService` | C10 |
| `services.OrderPushSubscriber` | `pkg/services/push.go:161` | `*TradingService` | C10/C15 |
| `services.PushTransport` | `pkg/services/push.go:124` | `*transport.PushAdapter` | C11 |

A caller can already write this, and it compiles today:

```go
c, err := client.New(client.WithEnv(""))
adapter, err := transport.NewPushAdapter()
push := services.NewPushOrchestration(services.NewMarketService(c), adapter)
```

`executor.go:43` even pins the first row with
`var _ Executor = (*client.Client)(nil)`. **So the binding work is done; what is
missing is a single composition with a coherent lifecycle.** The gap is real but
much narrower than "the layer is unreachable", and a design that treats it as
narrower will be a better one.

"Unreachable by a caller" was always about *no released code path constructing
it*, never about importability — `pkg/services` and `pkg/transport` are ordinary
public packages.

## 3. Placement: the rules decide it, not preference

`internal/layering/layering_test.go:135` holds six rules. Four of them bear on
this decision, and `prefix` is a **string prefix match**, which produces one trap
worth writing down.

| Candidate | Verdict | Why |
|---|---|---|
| `pkg/hstong/*` | **Illegal** | Rule 1 (`layering_test.go:137`) forbids `pkg/hstong` from importing `pkg/domain`, `pkg/services`, `pkg/transport` |
| `pkg/hstong/vnext` | **Illegal** | Prefix match: `pkg/hstong/vnext` matches prefix `pkg/hstong`, so rule 1 forbids its imports too. A subdirectory is not a way out — **measured, see §11** |
| `client/` | **Illegal in spirit, and unreachable** | ADR 0011 protects `client/`, and a base-layer type importing `pkg/services` inverts the whole stack |
| `pkg/domain` | **Illegal** | Innermost layer, no dependencies |
| `pkg/transport` | **Illegal** | Rule 2 (`layering_test.go:148`): `pkg/transport` must not import `pkg/services` |
| `pkg/services` | **Viable** | ADR 0010 rule 4 permits `services` → `domain` + `transport`; no current rule forbids it |
| **new package** | Viable | The only other place a composition can legally live |

That leaves two, and the trap above is the first real result: **an opt-in
constructor cannot be added to `pkg/hstong` in any form, including a
subdirectory.** If the run owner expected that shape, the expectation is
infeasible rather than merely unattractive, and the reason is mechanical.

## 4. Why not a new package — the naming problem D1 would have to solve

A new public package is a permanent naming decision, and at v1.0 the facade
becomes *the* primary API while `pkg/hstong/*` is only deprecated. So the name
must be product-oriented, not phase-oriented:

- `pkg/vnext` — names a **phase**, not a product. At v1.0 it is the only entry
  point and would still be called `vnext`. Renaming a shipped package is a
  breaking change, which is exactly what v1.0 is for, so the cost is deferred
  rather than avoided.
- Any versioned path (`pkg/hstong/v2`, `pkg/services/v2`) — illegal, as above.
- A new neutral name (`pkg/sdk`, `pkg/api`, `pkg/hstongapi`) — legal, but it
  invents a namespace on the strength of this one decision, and D1 is a design
  task, not a naming launch.

**Decision: no new public package in the D-series.** Composition goes in
`pkg/services`, which must be the v1.0 API anyway, so this adds a fourth naming
question to a programme that already has enough. If the run owner wants a facade
package, it belongs to E3/v1.0 where the deprecation of `pkg/hstong/*` is
announced and the name can be chosen against a known final shape.

## 5. Composition shape

The narrow finding from §2 has an exact consequence: the composition needs no
new imports, because every edge it needs already exists as a structural
interface. So it is additive within a package that already depends on `client`.

```go
// Stack is the v-next service set over one request path, with one Close.
type Stack struct {
    Market  *MarketService
    Account *AccountService
    Trading *TradingService
    Algo    *AlgoService
    Futures *FuturesService
    Session *SessionService
    Push    *PushOrchestration // nil unless WithPushTransport is given
}

func NewStack(c StackExecutor, opts ...StackOption) *Stack
func (s *Stack) Close() error
```

`PushOrchestration` needs a `TopicSubscriber` and an `OrderPushSubscriber` as well
as a `PushTransport`, so it takes both HTTP halves explicitly, and the HTTP halves
are the Stack's own `Market` and `Trading` services.

> **Correction, found while implementing D2.** This snippet was wrong twice when
> first written, and both errors were silent. The real signature is
> `NewPushOrchestration(sub TopicSubscriber, tr PushTransport, opts ...PushOption)`
> — **two** required arguments — and the version below passed three, treating
> `*TradingService` as the second parameter. That would not have compiled, which is
> the lucky outcome: had the arity happened to line up, the trade HTTP half would
> have been dropped and `SubscribeOrders` would have had no way to reach
> `/trade/TradeSubscribe`. The trade half arrives as the `WithTradeSubscriber`
> *option*, not as a parameter, and that is not obvious from the parameter list.
> This is the tenth recorded instance of the run's recurring wrong-premise pattern
> (`todos.md` P1, P4, A4, A6, C2a, C4, C10, C13, D1), and it landed in a design
> note written specifically to prevent it.

```go
func NewStack(c StackExecutor, opts ...StackOption) *Stack {
    s := &Stack{
        Market:  NewMarketService(c),
        Account: NewAccountService(c),
        Trading: NewTradingService(c),
        Algo:    NewAlgoService(c),
        Futures: NewFuturesService(c),
        Session: NewSessionService(c),
    }
    for _, opt := range opts {
        if opt != nil {
            opt(s)
        }
    }
    if s.pushTransport != nil {
        s.Push = NewPushOrchestration(s.Market, s.pushTransport,
            WithTradeSubscriber(s.Trading))
    }
    return s
}
```

### 5.1 The one decision that is forced: `Close` cannot follow `Executor`

This is the detail the design turns on, and it is worth stating precisely because
the obvious signature is wrong.

`services.Executor` has exactly two methods, `Do` and `JSON`
(`executor.go:32-39`). **It has no `Close`.** So a `Stack` built on `Executor`
owns nothing it can close, and the naive `defer s.Close()` would silently leave
the TCP push connection open — the one resource in this SDK that a forgotten
`Close` turns into a leaked socket rather than an idle-pooled one.

Two options, and the first is rejected on evidence rather than taste:

- **Have `Stack` take `*client.Client`** so it can own the lifecycle. Rejected:
  it collapses ADR 0010 rule 6 and step 5, which exist so the services can be
  driven by a fake. Every test of every service would need a live-shaped client,
  and the structural-interface property that made C11's `PushTransport` seam work
  would be undone at the composition layer.
- **Declare a wider composition-time interface** (below). The fake grows one
  method, and the inversion survives.

```go
// StackExecutor is what NewStack needs: the request path plus the lifecycle it
// may own. Executor alone has no Close (executor.go:32), so a Stack built on it
// could not release what it opened.
type StackExecutor interface {
    Executor
    Close() error
}
```

`Executor` is embedded, so every existing `Executor` value that also has `Close`
satisfies this, `*client.Client` among them. A test fake needs one extra method —
`sequencedExecutor`, the shared helper in `testsupport_test.go`, grows a three-line
`Close` and a counter, so `Close`-was-called is asserted rather than assumed, and no
other fake in the package has to change because the per-service constructors still
take the narrower `Executor`.

**This shape was chosen over a type assertion during D2, and the reason is
specific.** With a type assertion in `Close`, "the Stack closes what it opened"
would be a *documented* promise: pass a non-closable wrapper around your client and
the Stack silently never closes it, with no compile error and no runtime signal.
The whole reason the composition exists is coherent lifecycle, and an interface
that makes the promise enforceable is worth one method on a test fake. The
counter-argument — a faked client having to implement `Close` — was measured and is
three lines in one shared helper.

### 5.2 `Close` ordering, and why the adapter is not closed separately

`PushOrchestration.Close` already closes the transport it owns
(`push.go:561-563`), under a `sync.Once`, and both it and `*client.Client.Close`
are idempotent and return the same result on every call. So `Stack.Close` is:

1. close push, if present — which closes the adapter with it
2. close the executor
3. return the first non-nil error, having attempted both

Step 2 is unconditional, because `NewStack` takes a `StackExecutor` and the
compiler has already guaranteed there is something to call. The earlier draft made
it a type assertion on the stored value; that was correct but weaker, and it is
replaced for the reason given at the end of §5.1.

The alternative — closing the adapter separately — would double-close a component
that is already idempotent, which is harmless but implies an ownership the
orchestrator actually has. The design states the real one.

### 5.3 Options

Only two, because every other knob already exists on the component it would
duplicate:

- `WithPushTransport(tr PushTransport)` — the opt-in. `tr == nil` means no push:
  no TCP dial, no `Push` field value, and `Close` skips step 1.
- `WithStackExecutor(c StackExecutor)` — overrides the request path for testing.
  Kept so a test can swap the executor after construction without rebuilding the
  composition; it is the same shape as the constructor argument, so it cannot
  introduce a value the constructor would have rejected.

Per-service options are deliberately **not** re-exposed. `NewMarketService`
already takes `MarketOption`; a `Stack` that forwarded them would need one option
type per service and would make `services` import its own option vocabulary five
times over. A caller needing a non-default service builds it and ignores the
Stack's copy.

### 5.4 Naming

`Stack` and `NewStack` are recommended over the alternatives:

- `Client` — collides conceptually with `client.Client` and with
  `*SessionService`, which *is* the login client. Two meanings for one word in a
  package where both already exist.
- `Suite` / `Set` / `API` — `Suite` reads as a test helper and D4 will add one.
- `Services` — reads as a plural noun for a single composition.

`Stack` is used here in the layering sense, which is the sense the design is
about: one set of services over one request path, closed as a unit.

## 6. ADR 0011 compliance

Against the five guarantees at `docs/adr/0011-v01x-compatibility.md:42-57`:

| Guarantee | Status | Evidence |
|---|---|---|
| 1. No breaking type changes | **Untouched** | no exported symbol in `pkg/hstong/*`, `pkg/types`, or `client/` is modified, removed, or shadowed |
| 2. No breaking wire changes | **Untouched** | no `gen/` edit; no `pkg/types` wire struct edit; the parity guard is unchanged, so the route table is identical |
| 3. No new mandatory dependencies | **Untouched** | `stack.go` imports only `context` and this package's own types; `go.mod` does not change |
| 4. Mock Gateway compatibility | **Untouched, and this is the D4 risk** | `test/mockgateway/` serves HTTP; the Stack adds no new route. D4 must prove the push path, which is TCP, works against the mock |
| 5. `gen/` never touched | **Untouched** | no proto change |

The "What IS allowed" table at `adr/0011:66` lists "Adding new exported functions
/ methods" as additive and non-breaking. `Stack`, `NewStack`, `StackExecutor` and
the two options are all new exported symbols in a **new** package
(`pkg/services`, itself never shipped in a released form — it is outside the
protected surface entirely).

**The strongest evidence is not the table but the diff:** `stack.go` is a new
file in a package ADR 0011 does not protect, so the guarantee is satisfied
structurally rather than by argument.

## 7. Parity and the guard

`Stack` names **no** `client.Route`, so the parity gate is unaffected: 51/51
before and after, and `paritygate --enforce` must exit 0 throughout.

This is the same constraint C11 worked under, and it carries the same trap in
reverse. A facade is exactly the kind of thing that *looks* like it should own a
scan root, because it is the outermost v-next symbol a caller touches. It must
not: the guard credits a route to a **method that references it**, and a `Stack`
that merely forwards to services references none. Adding a scan root containing
zero routes would not strengthen the guard, it would weaken it, because the
guard's own design note says a missing root makes it stricter.

D3 adds the rule that keeps this true: `pkg/services` must not gain a stored
`client.Route` field, and the facade must not name one.

## 8. What D2 must build, and what D3 must enforce

**D2** — `pkg/services/stack.go` and `stack_test.go`:
1. `Stack`, `StackExecutor`, `NewStack`, `WithPushTransport`,
   `WithStackExecutor`, `Close`.
2. Every field non-nil; `Push` non-nil iff push was opted into.
3. `Close` idempotent, closes push before the executor, returns the first error
   while attempting both.
4. `Close` with no push and a non-closable executor is not a panic.
5. `pkg/services` holds at **100.0%** afterwards, measured from a clean clone —
   the run has twice been misled by a dirty-tree figure.

**D3** — three rules added to `internal/layering`:
1. `pkg/services` must not import `pkg/transport` (keeps the facade from
   transitively pulling `internal/push`, which rule 4 already forbids directly).
2. `pkg/hstong` must not import `pkg/services`' `Stack` — this is rule 1
   restated, and per VNEXT.md:311-313 rules are added as deviations close, so
   D3 should only add it once the guard is in place and the deviation is closed
   by construction.
3. A stored `client.Route` field in `pkg/services` is fatal — the C14
   `checkStoredRoutes` precedent, applied to the new file.

Each must be verified by **planting** a violation and watching the test fail, per
the C14 method. A rule that has never been seen to fail is not evidence.

## 9. Rejected alternatives

| Alternative | Why rejected |
|---|---|
| Add the constructor to `pkg/hstong` | Illegal: layering rule 1, prefix-matched. Infeasible, not merely unattractive |
| `pkg/hstong/vnext` subdirectory | Illegal for the same reason — the prefix match is the trap |
| `pkg/vnext` | Names a phase. At v1.0 it is the only entry point and would still be `vnext` |
| Facade importing `pkg/transport` | Legal under ADR 0010 rule 4, but it makes `pkg/services` depend on `internal/*` transitively, undoing the C10 decision that put the concrete adapter in `pkg/transport` *because* services cannot hold it |
| `NewStack` takes `*client.Client` | Collapses rule 6 and step 5; the layer loses its fake-driven testability, which is the property C11's seam exists to provide |
| A separate `NewStackWithClient` for the real client | Two entry points for one composition, differing only in which fake you may pass. The `StackExecutor` widening is the whole of the difference |
| Stack re-exposing per-service options | One option type per service, and `services` importing its own vocabulary five times. Each service's options already exist |
| No constructor at all; document the 8-line wiring | Honest cost: the wiring is 8 lines and already compiles. Rejected only because the resource with a real failure mode is the TCP push connection, whose `Close` currently lives in two places the caller must not double-close — but this remains the cheapest correct outcome, and the run owner may prefer it. **Recorded as the live alternative, not a straw man** |

## 10. Unresolved, and what would settle it

- **The facade's fate at v1.0.** This design deliberately does not decide it.
  v1.0 deprecates `pkg/hstong/*`; whether `Stack` becomes the documented
  primary entry or is dissolved into per-service construction is an E3/v1.0
  question that depends on the migration guide (E1) and on what the guide tells
  users to write. Deciding it now would pre-empt E1.
- **Whether `Stack` should expose a `Logger` or metrics handle.** `pkg/services`
  takes an injected logger per service today; the Stack does not aggregate them.
  Deferred to D2, and only if a test needs it.
- **G6 remains unavailable**, so none of this is verified against a live Gateway.
  D4's mock coverage is the ceiling, and F1 records the gap.
- **`internal/layering` is 78 commits stale in the code graph** (indexed at
  `eb00cbd0`; HEAD `b0a4686` as of this note), so impact for this design was
  established by reading the six rules and by blob hash, not by the graph. The
  fallback evidence is recorded here rather than the step being skipped.

## 11. Verification for D1

D1 is a design note, so its own verification is a review, not a command. The
mechanical claims in it are each checkable, and the ones that would invalidate
the design are:

- **V-fact §2** — the four structural-satisfaction rows are read off
  `executor.go:32`, `push.go:124/151/161`, and pinned by existing `var _` lines.
- **V-fact §3 — measured, not read.** The illegality of `pkg/hstong/vnext` was
  planted and observed while writing this note. A package `pkg/hstong/vnext`
  importing `pkg/services` was created, and `internal/layering` failed with

  ```
  --- FAIL: TestLayeringRules/released_surface_does_not_reach_the_v-next_layer
  pkg/hstong/vnext imports pkg/services
  ```

  which is the mechanism: `layering_test.go:210` matches a rule's packages with
  `strings.HasPrefix(pkg, r.prefix+"/")`, so a subdirectory of a forbidden prefix
  is inside the rule, not outside it. The plant was removed and the suite
  re-run green. §3's table is therefore a measurement, and D3 inherits a proven
  case to re-plant rather than a hypothesis to test.
- **V-fact §5.1** — `Executor` has no `Close`; `PushOrchestration.Close` closes
  the transport; both `Close` methods are idempotent. All three are read from
  source at the line numbers cited.
- **V7** — the coverage gate must still pass. A design cannot break it, but D2
  can, and `pkg/services` at 100.0% is a D2 obligation, not a D1 one.

No new V-label is introduced. `V3c` in D2's acceptance column means "no ADR 0011
boundary violation — see `internal/layering`", and §6 is the argument for it.
