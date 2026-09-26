# Design note — the price-tick model and `spreadLevel`

- **Run:** `2026-09-26-vnext-parity-wire`
- **Task:** P3 (`architect`) — record a decision; **no code changed**
- **Date:** 2026-09-26
- **Status:** Decided. Implementation is a follow-on task.
- **Related:** [ADR 0008](../../adr/0008-decimal-financial-types.md) (amended the same
  day), [ADR 0011](../../adr/0011-v01x-compatibility.md), task P1 in
  [todos.md](./todos.md)

## 1. The defect, stated precisely

`domain.Price` carries two values: the price and the **tick** it is valid on.
`domain.MustNewPrice(value, tick)` forces every construction site to supply a
tick, and 40 production call sites supply the same literal, `"0.001"`:

| File | Sites with hardcoded `"0.001"` |
|------|------|
| `pkg/services/market.go` | 17 |
| `pkg/transport/mappers.go` | 8 |
| `internal/push/decode.go` | 8 |
| `pkg/domain/account.go` | 4 |
| `pkg/domain/trading.go` | 3 |

`0.001` is a plausible HKEX ETF or warrant tick and wrong for HK stocks, whose
tick is set by a price band, and wrong for every US instrument, where the grid
is `$0.01` for most names and `$0.0001` for sub-dollar ones.

Two consequences, in increasing order of severity:

1. **Loud, and correct.** `services.trading.validatePriceForHK` calls
   `price.Validate(true)`, so a caller that builds a `Price` with tick `0.001`
   and value `0.0005` is rejected with `StatusInvalidParam`. Nothing is
   silently wrong; the SDK refuses a price it cannot vouch for.

2. **Silent, and the real hazard.** `Price.Round()` floors to a multiple of the
   tick. On a **read** path — a quote, a candle close, a fill — that is silent
   corruption of a value the Gateway actually sent: `0.0005` read from the wire
   with a tick of `0.001` would round to `0.000`. `Round()` has no production
   caller today, so nothing is corrupted yet. It is one caller away from being
   corrupted, and the type makes the mistake look correct.

A second, sharper instance of the same modelling error: `services.OrderBookResponse.TickSize`
is a `domain.Price` built as `MustNewPrice(<the tick>, "0.001")` — a price whose
own tick is a *different* price's tick. A caller who validates it as a price
gets a spurious error on a faithful `0.0005`. **A tick is not a price.** It has
no tick of its own; representing it as one guarantees the two disagree whenever
the instrument is not a 0.001-grid instrument.

## 2. Decision

### 2.1 A read-path price carries no tick

`MustNewPrice` on a wire-derived price passes the zero tick `"0"`, not `"0.001"`.
The domain already defines the semantics: `Price.Validate` skips the modulus
check when the tick is zero, and `Price.Round` returns the value unchanged
(`domain_mappers_test.go` pins both). "I observed this price; I do not know this
instrument's tick schedule" is the truthful state, and a zero tick states it in
the type rather than in a comment.

This removes 40 literals, not 40 lines of behaviour. No read-path caller reads
`.Tick()` and no read-path caller calls `Validate(true)` or `Round()`, so nothing
about today's output changes — what changes is that the trap is disarmed.

### 2.2 A write-path price carries the caller's tick, checked against the instrument's

`OrderRequest.Price` is caller-supplied and already carries the caller's tick.
That is right: the caller is the only party that knows what grid they intend to
trade on, and the SDK must not silently substitute a grid of its own.

What is wrong today is the check, not the value. `validatePriceForHK` asks
`DefaultHKTickSchedule(dtype).TickSize` only whether it is empty, then validates
against `price.Tick()` — the caller's own number, which the caller chose and
which therefore makes the check vacuous. It holds two different ticks and uses
one of them as a boolean. The follow-on introduces a resolved schedule:

```go
// TickSchedule resolves lot size and tick for one instrument.
type TickSchedule interface {
    ScheduleFor(Symbol) (lot uint64, tick string, ok bool)
}
```

with the existing `DefaultHKTickSchedule` as the default HK implementation,
injected so a caller can supply a real instrument master. Validation then
compares the caller's price against **the instrument's** tick, and an HK order
with an unset tick is rejected as `StatusInvalidParam` ("tick unknown") rather
than waved through — the exchange would reject it, and the SDK should say so
first.

### 2.3 Correct defaults

| Instrument | Default tick | Why |
|------------|--------------|-----|
| HK stock | **not a constant** | HKEX sets the tick by price band, so no single default is correct. Derive the band from the order price, from the exchange's published schedule. Do not hardcode it from memory; source it at implementation time. |
| HK ETF / warrant / CBBC | `0.001` | Matches `DefaultHKTickSchedule` today and the exchange's published grids. |
| HK bond | `0.0001` | As today. |
| US equity | `0.01` | The regular grid; sub-dollar names are the exception, not the rule. |
| US sub-dollar / other | `0.0001` | Only when the instrument master says so. |
| **Unknown instrument** | `""` → validation skipped, lot check still on | An absent tick must read as "unknown", not as "0.001". |

Note the direction of today's error: `0.001` is a *finer* grid than almost every
real tick, so "is a multiple of 0.001" **under**-constrains rather than
over-constrains. An under-constrained order is rejected by the exchange with a
clear message, which is survivable; an over-constrained one would lock a
caller out of a valid price. The bug is the *invention* of a grid, not its
fineness.

### 2.4 `TickSize` is a tick, so it must not be a `Price`

The follow-on introduces a type with no nested tick:

```go
// Tick is a minimum price increment. It is a scale, not a price: Validate and
// Round are not defined on it, because a tick is not on its own grid.
type Tick struct{ dec decimal.Decimal }
```

`services.OrderBookResponse.TickSize` becomes `domain.Tick`, constructed from the
wire value with no tick argument. The 0.0005 case can then neither fail
validation nor be rounded, because neither operation exists on the type. The
`pkg/hstong` twin stays `float64` for ADR 0011 reasons and is fixed at v1.0.0
(see [ADR 0008 § Amendment](../../adr/0008-decimal-financial-types.md)).

## 3. `spreadLevel`: what the field actually is

The premise handed to this task was that `spreadLevel` is a bid-ask spread and
that `TickSize` is therefore misnamed. The repository's own record contradicts
that, and the fixture settles nothing.

**What the record says.** The run that implemented `pkg/hstong/market` recorded
the vendor's own field documentation as
`spreadLevel — double 最小价格单位`, "minimum price unit", i.e. a **tick**
(§1 and §4 of `docs/runs/2026-09-21-hstong-full-surface/evidence/P03-T12-T13.txt`),
citing `https://quant-open.hstong.com/api-docs/market-interface/market-get/get-realtime-order-book.html`.
So does `BasicQot`, which carries a *separate* `double priceSpread = 8` — the
Gateway has one field for the spread and another for the step, and calling the
step a spread would give it two names for one concept.

**What the fixture shows.** `test/mockgateway/fixtures.go:191` sends
`"spreadLevel":0.2` for `00700.HK`, and the level-1 quotes in that same fixture
are bid `388.0` / ask `388.2` — a difference of exactly `0.2`. So the one number
in the repository that looks like evidence is equally consistent with both
readings: it is a tick that happens to equal the spread, or a spread that
happens to equal the tick. **The fixture cannot discriminate, so it is not
evidence for the spread reading.** It was a synthetic fixture, written to make
the field decodable, not observed from a Gateway.

**Conclusion: unresolved by anything in this repository.** Not by the code, not
by the fixture, and not by ADR 0011.

### 3.1 Naming recommendation: keep `TickSize`; do not rename

`pkg/services` is outside ADR 0011's protected surface and could be renamed
today. Do not.

- The only primary evidence — the vendor's own field documentation — says tick.
  Renaming to `SpreadLevel` would assert a semantics no observation supports,
  into an API that freezes at v1.0.
- The two packages exist to be twins. Renaming one and not the other leaves
  `market.TickSize` and `services.SpreadLevel` describing the same wire field
  with different names, which is a worse defect than a debatable name.
- The name is cheap to change later; a wrong name in a frozen public API is not.

**Deferred, with the test that decides it.** This is falsifiable with one
request, and the follow-on should run it as soon as a Gateway account exists
(G6). Call `/hq/OrderBook` for one security twice, seconds apart, and compare
`spreadLevel` against the level-1 spread of each reply:

- `spreadLevel` constant across replies while the book moves → **tick**. Keep
  `TickSize`; no change.
- `spreadLevel` tracking the level-1 spread → **spread**. Rename to
  `SpreadLevel` in `pkg/services` only, and file the ADR 0011 follow-on for the
  `pkg/hstong` twin at v1.0.0.

Until then the follow-on change is limited to the **type** (`domain.Tick`), which
is correct under either reading, and to a doc comment recording both. A test
asserting the semantics is not written, because asserting an unverified belief in
a test is how it becomes an unquestioned fact.

## 4. Rejected alternatives

| Alternative | Why rejected |
|-------------|--------------|
| Keep `0.001` and document it | It is a claim the SDK cannot make, and it is load-bearing for `Round()`. Documenting a wrong invariant is worse than encoding an absent one. |
| Default the read-path tick from `DefaultHKTickSchedule(DataType)` | Better than a literal, still wrong: the tick is per instrument and price band, not per data type, and it is HK-only. It would make the same invention with extra steps. |
| Fetch the tick from `/hq/OrderBook`'s `spreadLevel` and use it everywhere | Tempting because the tick *is* on the wire, but it is one endpoint of eight, it costs a round trip per price, and its semantics are the open question in §3. Opportunistic refinement at most, and only after §3 is settled. |
| Make the tick a `Client` option (`WithTick("0.0001")`) | A single global default cannot be right for a portfolio spanning HK and US instruments, and it invites a caller to apply one market's grid to another. The instrument is the right scope, not the client. |
| Rename `TickSize` to `SpreadLevel` now | Argued and rejected in §3.1. |
| Change `pkg/hstong`'s `TickSize` now | Breaches ADR 0011 guarantee 1; scheduled for v1.0.0 instead. |

## 5. Follow-on scope (not done here)

1. Replace 40 `"0.001"` literals with `"0"` in the five files in §1.
2. Add `domain.Tick`; retype `services.OrderBookResponse.TickSize`.
3. Add the `TickSchedule` interface; rewire `validatePriceForHK` and the lot
   check to it; reject an HK order price with an unknown tick.
4. Add the non-HK defaults from §2.3 and the HK price-band table, sourced from
   the exchange document rather than from memory.
5. Run the §3.1 test when G6 unblocks, then rename or confirm.

Steps 1 and 2 are mechanical and testable. Steps 3 and 4 are the ones that need
a human decision on the HK band table before code is written.
