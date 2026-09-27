# Test plan — `pkg/services`

- **Run:** `2026-09-26-vnext-parity-wire`
- **Task:** B1 (`tester`) — planning only. **No test code, no production
  change, no edit to any `pkg/services/*.go` or `_test.go`.**
- **Date:** 2026-09-26
- **Status:** Ready. B2, B3, and B4 execute §5–§7 as written; §4 is the
  per-function assignment and §8 is what could go wrong.
- **Related:** [plan.md](./plan.md) (why this layer is tested at all),
  [todos.md](./todos.md) (B2/B3/B4/B5 acceptance), A1 and A2 in
  [todos.md](./todos.md), [design-tick-model.md](./design-tick-model.md),
  [ADR 0003](../../adr/0003-no-auto-retry-orders.md),
  [ADR 0008](../../adr/0008-decimal-financial-types.md), AGENTS.md

## 1. What is actually untested, in one paragraph

`pkg/services` is 29 public service methods over three files, and **not one of
them has ever been driven end to end.** The 365
uncovered statements are not scattered: they are almost entirely the two shapes
a happy-path-only suite cannot reach — the `return …, err` after a failed
`Do`, and the rejection branch of each `validate*` helper. Every converter in
`market.go` is already at 100% because P1's regression tests called them
directly. The service *methods* that call those converters have never been
called at all. This is the same hole A2 found in `pkg/hstong/trade`, one layer
up and larger: there, 20 endpoints and a dark `Manager.call` error surface;
here, 29 methods and a wholly dark error surface.

## 2. Baseline and target

Measured at `f7ef92b` on this host, `go test -count=1 -coverprofile`, from a
clean tree:

```
ok  github.com/shing1211/hstongapi4go/pkg/services  0.530s  coverage: 17.4% of statements
```

| File | Functions | Statements | Covered | Uncovered | Now |
|------|-----------|-----------|---------|-----------|-----|
| `account.go` | 9 | 73 | 24 | **49** | 32.9% |
| `market.go` | 25 | 133 | 47 | **86** | 35.3% |
| `trading.go` | 24 | 236 | 6 | **230** | 2.5% |
| **total** | 58 | **442** | **77** | **365** | **17.4%** |

Target: **≥85%**, the gate B5 registers in `scripts/coverage_gate.go`. In
statements that is `ceil(0.85 × 442) = 376` covered, so **+299 statements** are
needed. The whole uncovered set is 365, which leaves 66 statements of slack
(15.0pp) before the gate is at risk. §8 says where the realistic leak is.

`executor.go` contributes **zero statements** — it is an interface, two
`WithXClient`/`NewXService` pairs live in the other files, and a `var _ Executor`
assertion. Its coverage is not a lever at any percentage. B4's work there is
contract, not coverage (§4.3).

`walkFundJourPages` and `clampPageSize` are already at 100% (`account_test.go`),
so the fund-journal gap is in the two closures in `account.go`, not in the walk.

## 3. The four things the sibling packages establish

Carried over from A1, A2, and A3. Each is a design constraint on §5, not
background.

1. **The `Do` error arm must be a table, not an afterthought.** A2's finding:
   `pkg/hstong/trade` had 20 endpoints, a fully covered happy path, and a
   `return …, err` branch no test had ever entered — which is why four
   mutations sat at exactly 83.3% and six list queries at exactly 80.0%. Two
   different clusters, one identical untested statement. §5.2 is therefore
   29 rows, one per method, and it is the largest single group in the plan.
2. **Validators need a field × input matrix, not one case per field.** A3's
   finding: `pkg/hstong/algo`'s `validate` methods return on the *first*
   failure, so a suite that starts from one valid fixture and mutates a single
   sampled field can only ever reach the branches it happens to name. §5.1 is
   built the way `pkg/hstong/algo/validation_matrix_test.go` builds it: a
   known-valid fixture per helper, then one mutated field per row, plus an
   explicit accept table so a rejection row cannot be attributed to a bad
   fixture.
3. **Money must cross the request path byte-for-byte.** A sibling task fixed a
   shipped bug where `fmt.Sprintf("%.3f", …)` silently turned a `0.0005` tick
   into `0.001`. §5.3 is the regression net, with a value set measured against
   `strconv.ParseFloat`/`FormatFloat` so no row can quietly become benign.
4. **No `time.Sleep`, ever.** Where retries are exercised, install
   `client.RetryPolicy{MaxAttempts: 5, BaseBackoff: 0}` so the request count is
   a property of the retry classification rather than of elapsed time. A1
   established what happens otherwise: `internal/push` swung 3.2pp between runs
   of one commit and failed CI while every local run passed.

ADR 0003 adds a fifth constraint specific to this layer, stated in §5.4: the
single-attempt guarantee lives in `client`, not in `pkg/services`, so a
fake-only test of it would prove nothing.

## 4. Per-function work table

Need codes: **HP** happy path · **VAL** validation matrix · **ERR** error arm ·
**MONEY** money/quantity round-trip · **SHAPE** request-shape assertion (op,
route, and the wire request) · **DIR** direct call of an unexported helper ·
**MAP** page/clamp arithmetic · **CLK** wall-clock-dependent.

`Uncov` is the measured uncovered statement count for that function.

### 4.1 B2 owns `market.go` — 16 functions, 86 statements

| # | Function | Loc | Uncov | Needs | Owner |
|---|----------|-----|-------|-------|-------|
| 1 | `BasicQot` | `market.go:78` | 12 | HP (2 securities) · VAL (empty list, nil element) · ERR · SHAPE | B2 |
| 2 | `OrderBook` | `market.go:133` | 1 | VAL (nil security) · ERR | B2 |
| 3 | `KL` | `market.go:181` | 10 | HP (2 candles) · VAL (nil) · ERR · SHAPE | B2 |
| 4 | `TimeShare` | `market.go:224` | 10 | HP (2 points) · VAL (nil) · ERR · SHAPE | B2 |
| 5 | `Ticker` | `market.go:265` | 12 | HP (2 ticks) · VAL (nil security, `Limit` 0, `Limit` 101) · ERR · SHAPE | B2 |
| 6 | `Broker` | `market.go:309` | 6 | HP (2+2 entries) · VAL (nil) · ERR | B2 |
| 7 | `UsOptionChainCode` | `market.go:344` | 6 | HP · VAL (empty `SecurityCode`) · ERR · SHAPE | B2 |
| 8 | `UsOptionChainExpireDate` | `market.go:369` | 6 | HP · VAL (empty) · ERR · SHAPE | B2 |
| 9 | `UsOverNightTradeCodes` | `market.go:388` | 4 | HP · ERR — **no validation arm exists**; do not invent one | B2 |
| 10 | `Unsubscribe` | `market.go:425` | 8 | HP · VAL (unknown topic, empty list, nil element) · ERR · SHAPE | B2 |
| 11 | `convertBrokerEntries` | `market.go:564` | 4 | DIR (nil, 1 entry, 3 entries) | B2 |
| 12 | `validateSecurityList` | `market.go:577` | 1 | VAL (nil element at index 0 **and** at index 1, so the index in the message is exercised twice) | B2 |
| 13 | `validateSecurity` | `market.go:589` | 1 | VAL (nil) | B2 |
| 14 | `validateSecurityCode` | `market.go:596` | 3 | VAL (`""`, non-empty) | B2 |
| 15 | `validateTopic` | `market.go:617` | 1 | VAL (unknown id) + the closed-set count pin (§5.1.6) | B2 |
| 16 | `Subscribe` | `market.go:408` | 1 | VAL (unknown topic — the *only* statement missing; its `validateSecurityList` arm is already covered by `executor_test.go:127`) | B2 |

Already at 100%, no work: `WithMarketClient`, `NewMarketService`, `floatToString`,
`decimalOrZero`, `convertToQuote`, `convertOrderBookLevels`, `convertToKLine`,
`convertToTimeSharePoint`, `convertToTickerTick` (9 functions).

### 4.2 B3 owns `trading.go` — 22 functions, 230 statements

| # | Function | Loc | Uncov | Needs | Owner |
|---|----------|-----|-------|-------|-------|
| 1 | `Entrust` | `trading.go:115` | 13 | HP (HK + US) · VAL (delegates to #14) · ERR · MONEY · SHAPE | B3 |
| 2 | `CancelEntrust` | `trading.go:156` | 9 | HP · VAL (zero account, zero entrustID) · ERR · SHAPE | B3 |
| 3 | `BatchCancelEntrust` | `trading.go:176` | 10 | HP (2 ids) · VAL (zero account) · ERR · SHAPE | B3 |
| 4 | `ChangeEntrust` | `trading.go:198` | 13 | HP · VAL (zero account, zero id, off-tick price, off-lot qty) · ERR · MONEY (loud tick rejection, §5.3.4) | B3 |
| 5 | `MaxAvailableAsset` | `trading.go:227` | 13 | HP · VAL (zero account, zero symbol) · ERR · MONEY (§5.3.3: no validation gate) · SHAPE | B3 |
| 6 | `RealEntrustList` | `trading.go:257` | 13 | HP (2 rows) · VAL · ERR · SHAPE | B3 |
| 7 | `RealDeliverList` | `trading.go:286` | 10 | HP (2 rows) · VAL · ERR · SHAPE | B3 |
| 8 | `RealCondOrderList` | `trading.go:309` | 18 | HP · VAL · ERR · MAP (`PageNo` 0/negative → 1, `PageSize` 0/negative → 50, 501 → 500, in-range passthrough) | B3 |
| 9 | `HistoryEntrustList` | `trading.go:345` | 10 | HP · VAL · ERR · SHAPE (`StartDate`/`EndDate` pass-through) | B3 |
| 10 | `HistoryDeliverList` | `trading.go:370` | 10 | HP · VAL · ERR · SHAPE | B3 |
| 11 | `HistoryCondOrderList` | `trading.go:395` | 18 | HP · VAL · ERR · MAP + SHAPE (`StartTime` = `StartDate` + `" 00:00:00"`, `EndTime` = `EndDate` + `" 23:59:59"`, and `PageNo`/`PageSize` are **strings** here, not ints) | B3 |
| 12 | `MarginFullInfo` | `trading.go:433` | 7 | HP · VAL · ERR · SHAPE (`DataType` is the literal `"10000"`, `StockCode` the empty string) | B3 |
| 13 | `BeforeAndAfterSupport` | `trading.go:450` | 11 | HP (`support` `"1"` **and** `"0"`) · VAL (zero symbol) · ERR · SHAPE | B3 |
| 14 | `validateOrderForHK` | `trading.go:475` | 21 | VAL matrix — non-HK returns nil immediately; the six sub-checks and the conditional-order block are each reachable only from a fixture that passes the five before them | B3 |
| 15 | `validateQuantityForHK` | `trading.go:519` | 8 | VAL: per-dtype lot table, `lot == 0` skip, non-integer, not a multiple of lot | B3 |
| 16 | `validatePriceForHK` | `trading.go:533` | 6 | VAL: `tick == ""` skip, negative price, off-tick price | B3 |
| 17 | `validateSessionWindow` | `trading.go:544` | 20 | CLK — see §5.1.4. Six of the twenty statements are only reachable for a given wall-clock time, and there is a design that makes all twenty deterministic. | B3 |
| 18 | `validateOrderTypeEligibility` | `trading.go:584` | 5 | VAL: non-HK skip, each of the 10 eligible HK types accepted, an ineligible one rejected | B3 |
| 19 | `validateTimeInForce` | `trading.go:595` | 4 | VAL: all 5 valid values accepted, one invalid rejected | B3 |
| 20 | `validateShortSellingFlag` | `trading.go:610` | 5 | VAL: non-HK/ETF dtype skip, the 4 valid sides accepted, one invalid rejected | B3 |
| 21 | `isPriceOptional` | `trading.go:623` | 3 | DIR: true case-group, default | B3 |
| 22 | `isConditionalOrder` | `trading.go:640` | 3 | DIR: true case-group, default | B3 |

Already at 100%: `WithTradingClient`, `NewTradingService`. This is why `todos.md`
sizes B3 as "24 funcs" — the file has 24 functions, 22 of which have uncovered
statements. (B2's "23 funcs" is similarly off: `market.go` has 25 functions,
16 with uncovered statements. The table sizes are 16 / 22 / 5.)

### 4.3 B4 owns `account.go` residual + the `executor.go` contract

| # | Function | Loc | Uncov | Needs | Owner |
|---|----------|-----|-------|-------|-------|
| 1 | `MarginFundInfo` | `account.go:111` | 6 | HP · VAL · ERR · SHAPE | B4 |
| 2 | `HoldsList` | `account.go:139` | 9 | HP (2 rows) · VAL · ERR · SHAPE | B4 |
| 3 | `RealFundJourList` | `account.go:184` | 13 | HP (2-page walk) · VAL · ERR (first page) · ERR (mid-walk — **rows gathered so far are returned beside the error, not the zero value**) · CURSOR (`next == cursor`, `next == ""`, empty page) | B4 |
| 4 | `HistoryFundJourList` | `account.go:231` | 13 | Same four, plus `StartDate`/`EndDate` on every page | B4 |
| 5 | `RateQueryList` | `account.go:274` | 8 | HP (multi-key, **order-independent** assertion) · HP (empty map → nil slice, no error) · ERR | B4 |

Already at 100%: `clampPageSize`, `walkFundJourPages`, `WithAccountClient`,
`NewAccountService`.

`executor.go` — 0 statements, so no coverage lever. B4 adds
`executor_wiring_test.go`: one table over **all 29 methods** asserting that each
one reaches the injected `Executor` with the `op` and `Route` in §5.2's table,
through a real `*client.Client`. This is the ADR 0010 dependency-inversion
contract stated across the whole package rather than at one method, and it is
the only place a route/op mismatch introduced by a later service (C3–C13) can
be caught without a live Gateway. B4 depends on B2 and B3 for coverage, not for
this: it can be written against any one method's row and extended as they land.

## 5. Test matrix designs

### 5.1 Validation matrices (B2 §4.1 rows 12–16; B3 §4.2 rows 14–22)

Shape, following `pkg/hstong/algo/validation_matrix_test.go`:

- One **known-valid fixture per helper**, with a comment naming every branch it
  satisfies.
- One **reject table**: `struct { name, wantOp string; run func() error }`,
  where `run` copies the fixture and changes **exactly one field**. Because
  every helper returns on the first failure, the rejection is then attributable
  to that field alone, and deleting any single check turns its row into a nil
  error and fails the test.
- One **accept table** over the same fixtures plus the closed-set members, so a
  rejection row can never be attributed to a fixture defect.
- Assertions go through `errors.As(*errs.Error)` and the typed `Code`/`Op`
  fields, never a rendered message (AGENTS.md).

**5.1.1 `market.go` validators — 13 statements, the cheapest win in B2.**
`validateSecurityList`: empty slice; nil at index 0; nil at index 1 (proves the
index is the element's, not a constant); all-valid. `validateSecurity`: nil;
non-nil. `validateSecurityCode`: `""`; `"00700.HK"`; `"  "` (a space is not
empty, and the code is passed through unvalidated — assert that it is *accepted*
so a future `strings.TrimSpace` is a visible change, not a silent one).
`validateTopic`: every one of the 11 `knownTopics` members accepted; `0`; `-1`;
`99`; `types.TopicID(11) + 1` rejected. **Ordering rows**, because the ordering
is real behaviour and a reordering is a behaviour change: `Subscribe` with an
unknown topic *and* an empty list must fail on the topic.

**5.1.2 `validateQuantityForHK` — 8 statements.** One row per
`DefaultHKTickSchedule` entry, because the helper delegates its whole decision
to that table and the table is the real subject:

| dtype | lot | tick | rows |
|-------|-----|------|------|
| `DataTypeHKStock` (10000) | 100 | `0.001` | `100` ok · `150` not a multiple · `100.5` not an integer · `0` ok (zero is a legal multiple — pin the *current* behaviour and flag it, see F6) |
| `DataTypeHKETF` (10002) | 100 | `0.001` | `200` ok · `50` not a multiple |
| `DataTypeHKWarrant` (10003) | 1 | `0.001` | `7` ok (odd lot is legal) |
| `DataTypeHKCBBC` (10004) | 1 | `0.001` | `3` ok |
| `DataTypeHKBond` (10005) | 1 | `0.0001` | `1` ok |
| `DataTypeHKIndex` (10001) | 0 | `""` | `150` ok — the `lot == 0` skip |
| `DataType(-1)` | 0 | `""` | `0.5` ok — an unknown dtype is "unknown", not "rejected" |

**5.1.3 `validatePriceForHK`, `validateOrderTypeEligibility`,
`validateTimeInForce`, `validateShortSellingFlag`, `isPriceOptional`,
`isConditionalOrder` — 26 statements.**

- `validatePriceForHK`: `DataTypeHKIndex` → `TickSize == ""` → skip (a negative
  price is *accepted* on a dtype with no schedule; pin it, F7). `DataTypeHKStock`
  → `-1` rejected as negative; `123.456` accepted; `123.4565` rejected as off
  `0.001`. A zero tick → `Validate(true)` skips the step check, so
  `MustNewPrice("0.0005", "0")` is accepted: this is the
  [design-tick-model.md](./design-tick-model.md) §2.1 read-path tick and it is
  the precondition for §5.3's value set.
- `validateOrderTypeEligibility`: `MarketUS` → nil regardless of type (the
  `market != HK` skip). `MarketHK` → all ten of `knownHKOrderTypes` accepted;
  `EntrustTypeMarket` ("5"), `EntrustTypeIcebergLimit` ("9"),
  `EntrustTypeHiddenLimit` ("11"), and `""` rejected.
- `validateTimeInForce`: `""`, `DAY`, `IOC`, `FOK`, `GTD` accepted; `day` (wrong
  case) and `XGTD` rejected. The case row is the point — a case-insensitive
  implementation would be a wire change.
- `validateShortSellingFlag`: `DataTypeHKStock` and `DataTypeHKETF` → the four
  sides `1`/`2`/`3`/`4` accepted, `"9"` and `""` rejected. `DataTypeHKBond` and
  `DataTypeUSStock` → skip, `"9"` **accepted**.
- `isPriceOptional` / `isConditionalOrder`: direct tables over all 17
  `EntrustType` values, with the want column spelling out the nine price-optional
  and six conditional members.

**5.1.4 `validateSessionWindow` — 20 statements, and the one place a careless
suite would make the coverage number a function of the clock.**

The helper calls `time.Now()`, converts to `Asia/Hong_Kong`, formats `15:04`, and
compares against three windows. The three `if currentTime < start || currentTime
> end` conditions are evaluated on every call, so their statements are covered
by any one invocation; the three **`return errs.New(…)`** arms are not.

Measured over the whole day (brute force of all 1440 minutes against the same
window table in `trading.go:62-69`): **there is no minute at which none of the
three session types rejects.** So a single run that drives `"1"`, `"2"`, and
`"3"` covers all three error arms, at any hour. The design that follows from
this, and that B3 must use:

1. Compute the expectation in the test from the same window the code uses:
   `wantErr := cur < start || cur > end`, with `cur` read once at the top of the
   test and `start`/`end` from a table mirroring `hkSessionWindows`. Assert the
   **relationship**, never a hardcoded `nil`.
2. Drive `"1"`, `"2"`, `"3"`, an out-of-set value (`"7"`, which must reach the
   trailing `return nil` at `trading.go:581` — clock-independent), and `""`
   (which returns at `trading.go:564`). Plus `MarketUS` and
   `MarketUS` + `"1"` for the two early returns.
3. **The one `!ok` return at `trading.go:552-554` is unreachable as written.**
   The line above pins `market == MarketHK` and the map's only key is `"HK"`, so
   `ok` is always true. Cover it by `delete(hkSessionWindows, "HK")` inside a
   test that restores it in `t.Cleanup`, and **do not** call `t.Parallel()` in
   that test — the map is a package global and the delete is visible to every
   other test in the package. One statement; 0.2pp; but it is the only statement
   in the package that needs a mutation to reach, so say so in the test's
   comment.
4. Read the time **once** into a local and reuse it, so a run that straddles
   16:00 cannot see two different answers in one table.

Hardcoding the expected outcome instead (e.g. "sessionType 2 always succeeds")
turns this into a coin flip: outside 09:00–16:00 HK all three arms reject and
the accept statements go uncovered, which is precisely the AGENTS.md failure
mode ("a coverage figure that moves between runs of the same commit is a defect
in the tests").

**5.1.5 `validateOrderForHK` — 21 statements.** One valid HK fixture plus one
valid US fixture, and one reject row per sub-check, each mutating exactly one
field of the HK fixture so the earlier checks still pass:

| Row | Mutation | Reached check |
|-----|----------|---------------|
| non-HK passthrough | `Symbol.Market = MarketUS` with an otherwise invalid order | returns nil at `trading.go:477` |
| quantity not a lot | `Quantity = 150` (HK stock lot 100) | `validateQuantityForHK` |
| price negative | `Price = MustNewPrice("-1","0.001")` | `validatePriceForHK` |
| price off tick | `Price = MustNewPrice("0.0005","0.001")` | `validatePriceForHK` — the loud-failure case from [design-tick-model.md](./design-tick-model.md) §1 |
| price skipped | `OrderType = "31"` with `Price` zero | `isPriceOptional` true → no price check |
| session window | `SessionType = "7"` | never rejected (out of set) — see §5.1.4 |
| ineligible type | `OrderType = "5"` | `validateOrderTypeEligibility` |
| bad TIF | `TimeInForce = "XGTD"` | `validateTimeInForce` |
| bad side | `Side = "9"` with `DataType = DataTypeHKStock` | `validateShortSellingFlag` |
| `validDays` low | `OrderType = "31"`, `ValidDays = 0` | conditional block, first check |
| `validDays` high | `OrderType = "31"`, `ValidDays = 101` | conditional block, first check |
| `condValue` empty | `OrderType = "31"`, `CondValue = ""` | conditional block, second check |

`validDays` boundaries: `1` and `100` accepted (`maxValidDays`), `0` and `101`
rejected. Note that `Op` on every one of these is `opEntrust`, including when
the helper is reached from `ChangeEntrust` — `ChangeEntrust` calls
`validatePriceForHK`/`validateQuantityForHK` directly, and those hardcode
`opEntrust`, so a price rejected by `ChangeEntrust` reports
`Op == "trade/TradeEntrust"`. **Assert that as it is**, and record it as F8: a
caller inspecting `Op` on a change-order rejection is told the wrong endpoint.

**5.1.6 The `knownTopics` closed set.** `validateTopic` gates `Subscribe` and
`Unsubscribe` against an 11-entry map, and `types.TopicID` has no exported
enumeration to diff it against — so, like `client/route_mutation_test.go`,
pin **exhaustion by count** instead of inferring: assert `len(knownTopics) == 11`
and that each of the eleven named `types` constants is accepted. Adding a topic
to `types` without adding it here, or the reverse, fails the test.

### 5.2 The per-method error-arm table — 29 rows

Every row asserts three things: the error is the executor's (via `errors.Is`
against a sentinel, and `errors.As` to `*errs.Error` for the typed case), the
**zero value** is returned beside it, and exactly one `Do` was recorded. A
method that returned a half-decoded response beside the error would let a caller
mistake a failure for an empty success, which is the failure mode the row
exists to prevent.

**B2 — `market.go` (11).**

| Method | op | Route | Beside the error |
|--------|----|-------|------------------|
| `BasicQot` | `opBasicQot` | `client.RouteHqBasicQot` | `BasicQotResponse{}` |
| `OrderBook` | `opOrderBook` | `client.RouteHqOrderBook` | `OrderBookResponse{}` |
| `KL` | `opKL` | `client.RouteHqKL` | `KLResponse{}` |
| `TimeShare` | `opTimeShare` | `client.RouteHqTimeShare` | `TimeShareResponse{}` |
| `Ticker` | `opTicker` | `client.RouteHqTicker` | `TickerResponse{}` |
| `Broker` | `opBroker` | `client.RouteHqBroker` | `BrokerResponse{}` |
| `UsOptionChainCode` | `opUsOptionChainCode` | `client.RouteHqUsOptionChainCode` | `UsOptionChainCodeResponse{}` |
| `UsOptionChainExpireDate` | `opUsOptionChainExpireDate` | `client.RouteHqUsOptionChainExpireDate` | `UsOptionChainExpireDateResponse{}` |
| `UsOverNightTradeCodes` | `opUsOverNightTradeCodes` | `client.RouteHqUsOverNightTradeCodes` | `UsOverNightTradeCodesResponse{}` |
| `Subscribe` | `opSubscribe` | `client.RouteHqSubscribe` | `error` only |
| `Unsubscribe` | `opUnsubscribe` | `client.RouteHqUnsubscribe` | `error` only |

**B3 — `trading.go` (13).**

| Method | op | Route | Beside the error |
|--------|----|-------|------------------|
| `Entrust` | `opEntrust` | `client.RouteTradeEntrust` | `nil` |
| `CancelEntrust` | `opCancelEntrust` | `client.RouteTradeCancelEntrust` | `error` only |
| `BatchCancelEntrust` | `opBatchCancelEntrust` | `client.RouteTradeBatchCancelEntrust` | `error` only |
| `ChangeEntrust` | `opChangeEntrust` | `client.RouteTradeChangeEntrust` | `error` only |
| `MaxAvailableAsset` | `opMaxAvailableAsset` | `client.RouteTradeQueryMaxAvailableAsset` | `nil` |
| `RealEntrustList` | `opRealEntrustList` | `client.RouteTradeQueryRealEntrustList` | `nil` |
| `RealDeliverList` | `opRealDeliverList` | `client.RouteTradeQueryRealDeliverList` | `nil` |
| `RealCondOrderList` | `opRealCondOrderList` | `client.RouteTradeQueryRealCondOrderList` | `nil` |
| `HistoryEntrustList` | `opHistoryEntrustList` | `client.RouteTradeQueryHistoryEntrustList` | `nil` |
| `HistoryDeliverList` | `opHistoryDeliverList` | `client.RouteTradeQueryHistoryDeliverList` | `nil` |
| `HistoryCondOrderList` | `opHistoryCondOrderList` | `client.RouteTradeQueryHistoryCondOrderList` | `nil` |
| `MarginFullInfo` | `opMarginFullInfo` | `client.RouteTradeQueryMarginFullInfo` | `nil` |
| `BeforeAndAfterSupport` | `opBeforeAndAfterSupport` | `client.RouteTradeQueryBeforeAndAfterSupport` | `nil` |

**B4 — `account.go` (5).**

| Method | op | Route | Beside the error |
|--------|----|-------|------------------|
| `MarginFundInfo` | `opMarginFundInfo` | `client.RouteTradeQueryMarginFundInfo` | `nil` |
| `HoldsList` | `opHoldsList` | `client.RouteTradeQueryHoldsList` | `nil` |
| `RealFundJourList` | `opRealFundJourList` | `client.RouteTradeQueryRealFundJourList` | `nil` on a first-page failure; **the completed rows** on a mid-walk failure |
| `HistoryFundJourList` | `opHistoryFundJourList` | `client.RouteTradeQueryHistoryFundJourList` | same |
| `RateQueryList` | `opRateQueryList` | `client.RouteHsRateQueryList` | `nil` |

Two levels, both required. Level 1 is the `sequencedExecutor` with
`err: sentinel` and asserts `errors.Is` + the zero value + one call; it is 29
rows of pure unit work. Level 2, one row per service group, drives a real
`*client.Client` over the `wireRecorder` and asserts the **typed** error the
Gateway rejection actually produces: `internal/transport` turns
`{"ok":false,"err":"1007 : duplicate submission"}` into
`*errs.Error{Code: "1007", Category: CategoryTrading, Op: <the service's op>}`,
so Level 2 asserts `errs.CodeOf(err) == types.StatusDuplicateSubmit`,
`errs.CategoryOf(err) == errs.CategoryTrading`, `e.Op == wantOp`, and
`rec.total() == 1`. That is what proves the services do not launder a typed
Gateway error into an opaque one — a failure the fake cannot show, because the
fake never builds a typed error.

**Two rows buy no coverage and are still required.** `Subscribe` and
`Unsubscribe` `return s.client.Do(...)` directly, so they have no separate
`if err != nil` block and their rows add nothing to the percentage. They stay
in the table because the error is their *only* return value: a row is the sole
way to distinguish a failed unsubscribe from a successful one, and dropping them
because the number does not move would trade a real assertion for a metric. That
is also why §7.2 counts nine `Do`-error statements but eleven market rows.

### 5.3 Money and quantity round-trip

**What is actually achievable, and where.** Two different wire conventions, and
the honest limit of each ([plan.md](./plan.md) assumption 2):

- **Trade/account request path — decimal-exact, byte-for-byte required.** The
  wire fields are quoted strings built from `domain.Price`/`domain.Quantity`,
  which are `shopspring/decimal`. Nothing between the caller and the body can
  lose a digit unless a future change reintroduces a `float64` field or a
  `%.Nf` conversion. This is where the regression net belongs.
- **HQ market response path — float64-limited, do not over-promise.** The
  generated `dto.BasicQot`/`KLine`/`TimeShare`/`Ticker` numerics are
  `float64`, so `json.Unmarshal` has already destroyed a 19-digit wire value
  before `floatToString` sees it. The most the market group can honestly pin is
  *shortest-round-trip float64* fidelity. The one market field that **is**
  verbatim-capable is `OrderBookResponse.TickSize`, typed `json.Number`
  (`market.go:130`), and P1's `market_test.go` already covers it well. **B2 must
  not write an unsatisfiable byte-for-byte test against an HQ response field.**

**5.3.1 The value set.** Two tables, because the two failure modes are
different and a value that discriminates one is often blind to the other. Every
row was measured against `strconv.ParseFloat` → `strconv.FormatFloat(f, 'f', -1,
64)` on this host:

*Float64-hostile* (a `float64` field or a parse/format round-trip would change
the digits):

| Label | Literal | What float64 does to it |
|-------|---------|------------------------|
| 19 significant digits | `123.4567890123456789` | `123.45678901234568` — the last two digits are lost |
| 25 significant digits | `0.1234567890123456789012345` | `0.12345678901234568` — eight digits lost |
| **underflows binary64** | `1e-330` | **`0`** — positive as a decimal, *zero* to every float64. `domain.MustNewPrice` expands it to 330 zeros and a `1`; `String()` gives the full positional form. This is the brief's "zero to every float64 yet positive as a decimal" case, confirmed to work. |
| 27-digit integer quantity | `999999999999999999999999900` | `1000000000000000000000000000` — the low-order digits are replaced |

*Scale-hostile* (float64-stable, so a `float64` bug would not show — but a
`%.3f` bug would, and that is the bug that shipped):

| Label | Literal | What `%.3f` did to it |
|-------|---------|----------------------|
| the shipped bug | `0.0005` | `0.001` — a *different, wrong* price |
| sub-micro | `0.0000001` | `0.000` — a price rounded to nothing |
| trailing zeros | `0.1` | `0.100` — a different string for the same number |
| one third | `0.3333333333333333` | `0.333` |

The tables live in the shared file as `float64HostilePrices`,
`float64HostileQuantities`, and `scaleHostilePrices`, each guarded by
`requireFloat64Hostile` / `requireScaleHostile`, which assert the *property*
(`ParseFloat`→`FormatFloat` differs from the literal, resp. `%.3f` differs) on
every row. The guard is the point: a future edit that softens a literal into a
benign one fails the test instead of quietly removing the regression net. Note
`0.0005`, `0.0000001`, `0.1`, and `0.3333333333333333` are all float64-stable,
which is exactly why they belong in the second table and not the first.

**5.3.2 How the bytes are asserted — two levels, both required.**

*Level 1, the field.* Type-assert the recorded `params` and compare the string
field to the input literal with `==`. `fakeExecutor` already records `params`,
so this is free, and it is the assertion that would fail first if a
`%.3f` reappeared.

*Level 2, the body.* Drive the real client over the `wireRecorder` and assert
the recorded request body **contains** `"entrustPrice":"123.4567890123456789"`.
This is the one that catches a changed *field type*, because the `params` member
of the envelope is a `json.RawMessage` (`internal/transport/transport.go:51`),
so a `float64` field would render unquoted and the substring would not match.
This is the P1 regression, restated at the wire.

**5.3.3 Where each value is legal.** Validation will reject most hostile values
before they reach the wire, which is correct behaviour, not an obstacle — the
host is chosen per case:

| Host | Gate | How a hostile value gets through |
|------|------|---------------------------------|
| `MaxAvailableAsset` | none | `price.String()` goes straight into the request. The cleanest host: no tick, no lot, no TIF. Use it for the headline value. |
| `Entrust` with a zero tick | `validatePriceForHK` → `Price.Validate(true)`, which **skips the step check when the tick is zero** | `MustNewPrice("123.4567890123456789", "0")` — verified to return nil from `Validate(true)`. This is the [design-tick-model.md](./design-tick-model.md) §2.1 read-path tick, so the fixture is the decision working as designed, not a workaround. |
| `Entrust` with `OrderType = "31"` | `isPriceOptional` is true for `31`/`33`/`35` | the price is not validated at all; set `ValidDays` and `CondValue` so the conditional block passes |
| hostile **quantity** on HK stock | `ValidateInteger` + `ValidateLot(100)` | use `999999999999999999999999900` — verified integer *and* a multiple of 100. Or set `Symbol.DataType = DataTypeHKIndex` (10001), whose `DefaultHKTickSchedule` has `Lot == 0`, so the lot check is skipped and `0.5` is legal. |
| `ChangeEntrust` | hardcodes `types.DataTypeHKStock` for both helpers | only a **zero-tick** price survives; the 27-digit quantity needs the trailing `00` |

**5.3.4 The loud-failure row (as important as the round-trip rows).**
[design-tick-model.md](./design-tick-model.md) §1 says the honest state is that
a faithful `0.0005` inside a `Price` whose tick is `0.001` now fails validation
loudly. That is a *positive* claim and needs a test, or the next person will
"fix" it by rounding:
`Entrust` with `Price = MustNewPrice("0.0005", "0.001")` must return
`StatusInvalidParam` and must **not** place `0.001` on the wire. Assert both
halves: the typed error, and `len(sequencedExecutor.calls) == 0`.

### 5.4 ADR 0003: exactly one attempt

The four mutations are `Entrust`, `CancelEntrust`, `BatchCancelEntrust`, and
`ChangeEntrust` ([ADR 0003](../../adr/0003-no-auto-retry-orders.md) §Decision,
"Trade"). Three points make this non-obvious for this layer:

1. **The guarantee lives in `client`, not in `pkg/services`.**
   `client.Client.execute` derives the retry class from the route path
   (`client/client.go:182-201`) and consults `resilience.IsMutation`. A
   `fakeExecutor` has no retry logic at all, so a fake-only assertion —
   `len(f.calls) == 1` — is **trivially true and proves nothing** about
   retries. It is worth keeping as a guard against a service growing a retry
   loop of its own, but it is not the ADR 0003 proof.
2. **The proof therefore needs a real `*client.Client` over `httptest`**, which
   is not a live Gateway: `client.New(client.WithBaseURL(srv.URL), client.
   WithRetryPolicy(client.RetryPolicy{MaxAttempts: 5, BaseBackoff: 0}))`, with
   the `wireRecorder` answering every path with
   `gatewayFailure(types.StatusServiceBusy, "service busy, retry later")`.
   `"1011"` is the code `errs.Retryable` reports as retryable, so a read-only
   route has every reason to send five requests. `BaseBackoff: 0` makes the
   request count a property of the classification rather than of elapsed time.
   **Never point a services test at `127.0.0.1:11111`.**
3. **Do not duplicate the route allowlist.** `client/route_mutation_test.go`
   holds the exhaustive classification check, and `internal/resilience.mutationPaths`
   is the closed set it verifies. B3's table names the four routes
   [ADR 0003](../../adr/0003-no-auto-retry-orders.md) names for Trade and adds a
   one-line comment pointing at that file for exhaustiveness. The
   `services` test asserts the *behaviour*; the `client` test asserts the
   *classification*; neither re-derives the other's list.

**The two layers.**

| # | Layer | Driver | Assertion |
|---|-------|--------|-----------|
| D1 | service issues one `Do` | `sequencedExecutor` returning a retryable `*errs.Error` | `callCount() == 1` and `errors.Is(err, sentinel)` for each of the four |
| D2 | **the ADR 0003 proof** | real client + `wireRecorder`, `MaxAttempts: 5`, `BaseBackoff: 0`, `"1011"` on every path | `rec.count(route) == 1` **and** `rec.total() == 1` for each of the four |
| D3 | **the control** | the *same* client options and the *same* `"1011"`, on `client.RouteTradeQueryRealEntrustList` via `RealEntrustList` | `rec.total() == 5` |

**D3 is not optional.** Without it, D2 passes just as well against a retry
policy that was never applied — and a test that cannot fail is worse than no
test, because it is read as coverage. A2 hit this exact shape in
`pkg/hstong/trade` and the control is what made its result mean anything. The
control must also assert `errs.CodeOf(err) == types.StatusServiceBusy` and
`e.Op == opRealEntrustList`, so it is not merely a request counter.

One more D2 detail: the four mutations must be driven with **valid** arguments.
`Entrust` in particular validates before it calls `Do`, so an invalid fixture
would record zero requests — which would look identical to "exactly one attempt"
if the assertion were `rec.total() == 1` rather than `rec.count(route) == 1` plus
an explicit check that a call was made at all. Use `hkOrder()` from §6.

## 6. Fixtures and helpers to build first

All of it goes in **one new file, `pkg/services/testsupport_test.go`**, in
`package services` (not `_test` — the tests need the unexported `op` constants,
`knownTopics`, and `hkSessionWindows`, exactly as `executor_test.go` and
`market_test.go` already do). Everything is a test-only symbol; nothing here
touches production.

**Do not redeclare what already exists.** Three helpers are already shared
package-wide and a duplicate definition is a compile error that would block B2
and B3 simultaneously: `orderBookBody(spread string)` and `legacyRounded(f)` in
`market_test.go:18-26`, and `entry()` in `account_test.go:14`. Reuse them.

### 6.1 Assertions

```go
func requireZero[T any](t *testing.T, v T)
func errRejects(t *testing.T, err error, wantCode types.StatusCode, wantOp string) *errs.Error
func assertInvalidParam(t *testing.T, err error, wantOp string) *errs.Error
func requireCalls(t *testing.T, f *sequencedExecutor, want int)
```

`requireZero` is the generic zero-value check from
`pkg/hstong/trade/error_paths_test.go`; it is re-declared here because that
package's copy is not importable. `assertInvalidParam` is the fail-closed
assertion: `errors.As` to `*errs.Error`, `Code == types.StatusInvalidParam`
(`"1016"`), `Op` equal, `Category == errs.CategoryAPI`. A local rejection
carries a code (unlike `pkg/hstong/algo`, whose `ErrInvalidParams` is a
sentinel), so the fail-closed property here is that the code is `1016` and
**not** a Gateway trading or account code.

### 6.2 The sequenced fake (needed by B4, and by the money tests)

`fakeExecutor` replies identically to every call, which is fine for a
single-request method and useless for `walkFundJourPages`, which must see page 1
then page 2. `sequencedExecutor` is the fix. `fakeExecutor` is **not** modified
— `executor_test.go` is pre-existing and shared.

```go
type sequencedReply struct{ reply any; err error }

type sequencedExecutor struct{ /* replies, calls, index — all mutex-guarded */ }

func newSequencedExecutor(replies ...sequencedReply) *sequencedExecutor
func (s *sequencedExecutor) JSON() client.Codec
func (s *sequencedExecutor) Do(ctx context.Context, op string, route client.Route, params any, _ client.Codec, out any) error
func (s *sequencedExecutor) callCount() int
func (s *sequencedExecutor) lastParams(t *testing.T) any
func (s *sequencedExecutor) paramsAt(t *testing.T, i int) any
```

Guarded, because `pkg/hstong/stream` learned the hard way that a fake which is
not race-safe turns `go test -race` into a coin flip. Also: an exhausted reply
list must `t.Fatal` rather than silently return the last reply, so a walk that
fetches more pages than the fixture provides fails loudly.

### 6.3 The wire recorder and a real client (for §5.2 level 2 and §5.4)

```go
type recordedRequest struct{ method, path, body string }

type wireRecorder struct{ /* mu sync.Mutex; requests []recordedRequest; responses map[string]string */ }

func (r *wireRecorder) ServeHTTP(w http.ResponseWriter, req *http.Request)
func (r *wireRecorder) count(path string) int
func (r *wireRecorder) total() int
func (r *wireRecorder) lastBody(t *testing.T, path string) string

func gatewayFailure(code types.StatusCode, text string) string
func newWireExecutor(t *testing.T, rec *wireRecorder, copts ...client.Option) Executor
```

`gatewayFailure` renders the documented `"<code> : <text>"` form
`internal/transport.classifyFailure` parses. `newWireExecutor` builds
`client.New(append([]client.Option{client.WithBaseURL(srv.URL)}, copts...)...)`,
registers `t.Cleanup` for both the server and the client, and returns it as an
`Executor` — which is the inversion the whole package is built on, now used
against the real implementation. Default `{"ok":true,"err":"","data":{}}` for an
unconfigured path, so a test that asks for an endpoint it did not set up gets a
decodable empty reply rather than a 404.

### 6.4 Money value sets

```go
type moneyCase struct{ name, in, want string }

var float64HostilePrices     = []moneyCase{ /* 3 rows, §5.3.1 */ }
var float64HostileQuantities = []moneyCase{ /* 1 row */ }
var scaleHostilePrices       = []moneyCase{ /* 4 rows */ }

func requireFloat64Hostile(t *testing.T, in string)
func requireScaleHostile(t *testing.T, in string)
```

`want` is the exact string that must reach the wire; the `require*` guard is
what keeps the row hostile.

### 6.5 Wire fixtures — **every decimal-bearing field populated**

This is the constraint that will bite hardest, so it goes here rather than in
each task's brief. Measured: `domain.AccountBalanceFromDTO`,
`PositionFromDTO`, `FundJournalEntryFromDTO`, `InterestRateFromDTO`,
`EntrustFromWire`, `FillFromWire`, `CondOrderFromWire`, `MaxAvailableFromWire`,
and `MarginInfoFromWire` **all panic** on a partially-populated wire struct,
because every numeric field is a `string` and `domain.MustNew*` panics on `""`.
Verified on this host:

```
AccountBalanceFromDTO(&MarginFundInfoWire{})  PANIC: domain: Money: invalid decimal string ""
PositionFromDTO(&HoldsVoWire{})               PANIC: domain: Quantity: invalid decimal string ""
FundJournalEntryFromDTO(&FundJourVoWire{})    PANIC: domain: Money: invalid decimal string ""
InterestRateFromDTO("HK","US","")             PANIC: domain: Rate: invalid decimal string ""
EntrustFromWire(&EntrustWire{}, "K")          PANIC: domain: Price: invalid decimal string ""
CondOrderFromWire(&CondOrderWire{})           PANIC: domain: Quantity: invalid decimal string ""
MaxAvailableFromWire(&MaxAvailableWire{})     PANIC: domain: Quantity: invalid decimal string ""
MarginInfoFromWire(&MarginFullInfoWire{})     PANIC: domain: Rate: invalid decimal string ""
```

`MarginFundInfoWire` alone has 28 money fields. So: **populate every field, and
never wrap a fixture in `recover()`.** A `recover()`-based test would pass, and
in passing it would *pin the panic* — turning a defect into a specification.
`MarginFundInfoWire` is the extreme case and B4 should budget for it.

Separately, five market methods dereference `wireResp.Security` with no nil
check (`market.go:146`, `:196`, `:235`, `:281`, `:320`), so **every market reply
fixture must carry a `security` object** even where the assertion is about
something else. A `security`-less reply panics; that is F1, recorded in §9, not
tested.

```go
// market.go fixtures (B2) — each includes security
func marketSecurity() *Security            // {DataType: 10000, Code: "00700.HK"}
func basicQotBody() json.RawMessage
func klBody() json.RawMessage
func timeShareBody() json.RawMessage
func tickerBody() json.RawMessage
func brokerBody() json.RawMessage
func usOptionChainBody() json.RawMessage
func usOptionChainExpireBody() json.RawMessage
func usOverNightBody() json.RawMessage
// orderBookBody already exists in market_test.go — reuse it

// trading.go fixtures (B3)
func hkSymbol() domain.Symbol               // NewSymbol(MarketHK, "00700", DataTypeHKStock)
func usSymbol() domain.Symbol               // NewSymbol(MarketUS, "AAPL", DataTypeUSStock)
func hkOrder() domain.Order                 // passes every branch of validateOrderForHK
func usOrder() domain.Order                 // MarketUS: validateOrderForHK returns nil immediately
func entrustBody(id string) json.RawMessage
func maxAvailableBody() json.RawMessage     // 20 fields, all populated
func marginFullInfoBody() json.RawMessage   // incl. a two-element rate array
func beforeAndAfterBody(support string) json.RawMessage

// account.go fixtures (B4)
func marginFundInfoBody() json.RawMessage   // 28 fields, all populated
func holdsListBody() json.RawMessage        // 2 rows, all 14 fields each
func fundJourBody(cursor string) json.RawMessage
func rateQueryBody() json.RawMessage        // 2 sources x 2 targets
```

## 7. Ordered, de-risked sequence, and the parallel split point

### 7.1 The split point

**B2 and B3 may run concurrently from the moment `testsupport_test.go` is
frozen.** Before that they must not both create it. Concretely:

1. **Serialised step 0 (B2, ~30 min):** author `testsupport_test.go` alone,
   land it, and confirm `go build ./... && go vet ./pkg/services/` is clean
   with **no other new file present**. Then B3 starts.
2. **After the freeze, file ownership is disjoint:**

| Owner | Files it may create or edit |
|-------|-----------------------------|
| B2 | `market_validation_matrix_test.go`, `market_happy_test.go`, `market_error_arms_test.go`, `market_money_test.go` |
| B3 | `trading_validation_matrix_test.go`, `trading_happy_test.go`, `trading_error_arms_test.go`, `trading_money_test.go`, `trading_adr0003_test.go` |
| B4 | `account_errors_test.go`, `executor_wiring_test.go`, `account_test.go` (extend) |
| **nobody** | `executor_test.go`, `market_test.go` (existing), `testsupport_test.go` (frozen), any `*.go` that is not `_test.go` |

3. **If B3 needs a helper that is not in the frozen file, it adds it to its own
   file under a `trading`-prefixed name** — never by editing the shared file,
   and never by duplicating a name. A duplicate definition is a compile error
   that takes down B2's run too, which is the whole reason the freeze exists.
4. `go test ./pkg/services/` is a shared resource, not a shared file. Two agents
   running it concurrently is fine; `-count=1` and no shared `-coverprofile`
   path (write coverage to `$env:GOTMPDIR`, per AGENTS.md's Windows note).

### 7.2 B2 — `market.go`, 7 steps

| Step | Work | Exit criterion (`market.go`, 133 statements, 47 covered) |
|------|------|----------------|
| 1 | Land `testsupport_test.go` (§6) | `go vet ./pkg/services/` clean; signal B3 |
| 2 | `market_validation_matrix_test.go` — §5.1.1, §5.1.6, plus the `OrderBook` nil-security and `Subscribe` unknown-topic rows and `Ticker`'s two limit rows | the five validators at 100% and **56/133 = 42.1%** |
| 3 | **One** happy path first: `OrderBook` via the existing `orderBookBody` | the fixture shape is proven on the one method that already has a working fixture before 64 statements depend on it |
| 4 | The rest of the happy paths: `BasicQot`, `KL`, `TimeShare`, `Ticker`, `Broker` | no panic; every fixture carries `security` |
| 5 | `UsOptionChainCode`, `UsOptionChainExpireDate`, `UsOverNightTradeCodes`, `Unsubscribe`, `convertBrokerEntries` direct | **124/133 = 93.2%** — only the 9 `Do`-error returns remain |
| 6 | `market_error_arms_test.go` — 11 rows, both §5.2 levels | 11 `t.Run` cases; `errors.Is` + zero value + one call each; **133/133 = 100%** |
| 7 | `market_money_test.go` — §5.3's scale-hostile rows added to the converter tables; pin that `floatToString` never emits an exponent (`"1e-07"` panics in `MustNewPrice`) | already at 100%; this step adds **no** coverage, only the precision contract |

Step 2's 9 statements are the six validation rejections inside the helpers
(`validateSecurityList` 1, `validateSecurity` 1, `validateSecurityCode` 3,
`validateTopic` 1) plus the three the methods own (`OrderBook` 1, `Subscribe` 1,
`Ticker`'s limit range 1). Step 5's total is 133 minus the 9 `Do`-error returns
— **nine, not eleven**, because `Subscribe` and `Unsubscribe` `return
s.client.Do(...)` directly and so have no separate error block. Step 3 before
step 4 is the de-risking: the existing `orderBookBody` proves the
`security`-carrying reply shape works, so a shape mistake is found in one test
rather than in six.

### 7.3 B3 — `trading.go`, 7 steps

| Step | Work | Exit criterion (`trading.go`, 236 statements, 6 covered) |
|------|------|----------------|
| 0 | Wait for `testsupport_test.go`; write only `hkOrder`, `usOrder`, `hkSymbol`, `usSymbol` into the shared file **if step 0 of §7.1 is still open**, or into `trading_happy_test.go` if it is frozen | `hkOrder` passes `validateOrderForHK` with a nil error |
| 1 | `trading_validation_matrix_test.go` — §5.1.2, §5.1.3, §5.1.5, §5.1.6 | **81/236 = 34.3%** — the 75 statements in the nine helpers |
| 2 | §5.1.4 `validateSessionWindow`, with the computed expectation | still 81/236, but all 20 of the helper's statements are now reached; **run it at two different wall-clock times and confirm the number does not move** — that is the check that the design worked |
| 3 | `trading_happy_test.go` — 13 methods, each asserting op, route, and the mapped domain values; the four `pageNo`/`pageSize` clamp blocks in `RealCondOrderList` and `HistoryCondOrderList` | **129/236 = 54.7%**; all fixtures fully populated; no panic |
| 4 | `trading_money_test.go` — §5.3 levels 1 and 2 on `Entrust`, `ChangeEntrust`, `MaxAvailableAsset`, plus §5.3.4 | no coverage change; the 19-digit and `1e-330` values appear verbatim in the recorded body |
| 5 | `trading_error_arms_test.go` — 13 rows, both levels, plus the zero-argument rows for each method | **236/236 = 100%** |
| 6 | `trading_adr0003_test.go` — D1, D2, D3 | no coverage change; `rec.total() == 1` per mutation, `== 5` for the control |
| 7 | Clean-clone re-measure, three cold runs | report the figure with the commit hash |

Steps 1–3 account for the arithmetic: 75 statements in the nine helpers, 129 in
the thirteen methods' request-build/map paths, and 32 left for step 5 — 13
error arms plus 19 zero-argument rejections (`Entrust` 2, `CancelEntrust` 2,
`ChangeEntrust` 4, `MaxAvailableAsset` 2, and one each for the other nine).

Step 2 has its own exit criterion on purpose: `validateSessionWindow` is the
only function in the package whose coverage could depend on when the suite runs,
so proving stability by running it twice at different times is part of the work,
not an extra.

### 7.4 B4 — `account.go` + `executor.go`, 4 steps

| Step | Work | Exit criterion (`account.go`, 73 statements, 24 covered) |
|------|------|----------------|
| 1 | `sequencedExecutor`-driven walk tests: 2-page accumulation, mid-walk error returning the completed rows, `next == cursor`, `next == ""`, empty page, cancelled context | the two closures' cursor branches are covered without touching the already-100% `walkFundJourPages` |
| 2 | Happy paths + error arms for the five methods, §5.2 both levels, with `RateQueryList`'s assertion order-independent | **73/73 = 100%** — all 49 remaining statements |
| 3 | `executor_wiring_test.go` — all 29 methods, op + route, through a real client | no coverage change; a route/op mismatch anywhere in the package fails |
| 4 | Clean-clone re-measure, three cold runs; report the package total for B5 | package ≥85%; report the figure with the commit hash |

### 7.5 Measurement discipline (all three tasks)

- Measure from a **clean clone**, never the working tree. This working tree is
  dirty with another agent's `pkg/hstong/algo` and `pkg/hstong/stream` work,
  which is the exact condition that overstated `internal/push` by 2.8pp and
  published a wrong figure.
- `go test -race -count=1 ./pkg/services/` with
  `CC=C:\Users\Tchan\mingw64\bin\gcc.exe`, `CGO_ENABLED=1`,
  `GOTMPDIR=C:\Users\Tchan\AppData\Local\Temp\opencode`. A2 ran it locally rather
  than deferring to CI; the guard is only worth anything if the fakes are
  race-safe.
- Three cold runs before the figure is recorded in `todos.md`, and the commit
  hash stated with it. A1 established that a figure which moves between runs of
  one commit is a defect in the tests, and `validateSessionWindow` is the one
  place in this plan that could reintroduce it.
- `gofmt -l .` must print nothing, and `go vet ./pkg/services/` must be clean.

## 8. Is ≥85% reachable? — the honest assessment

**Yes, comfortably.** The arithmetic is not close. 442 statements, a gate at 376,
a baseline at 77: the plan needs **+299** of the **365** uncovered, leaving 66
statements (15.0pp) of slack. B2 (86) + B3 (230) + B4 (49) covers the whole
uncovered set, and the expected shortfall across all realistic causes below is
under 10 statements — 2.3pp, against 15.0pp of slack. The binding constraint is
**not** coverage; it is fixture authoring effort in `account.go`, where one
reply needs 28 populated money fields.

Ranked by how likely each is to cost time:

1. **Fixture shape, the real constraint.** Thirteen of the 29 methods delegate
   to a mapper that panics on a partially-populated reply (§6.5, measured), and
   five market methods dereference `wireResp.Security` with no nil check. A
   "minimal reply" fixture panics on first use, and the two available responses
   are both wrong: hand-populating 28 fields is slow, and a `recover()` test
   passes while *pinning the defect*. Prescribed: populate everything, never
   `recover()`. This is the single most likely cause of a stalled B4.
2. **One structurally-guarded statement.** `trading.go:552-554` needs the
   `delete(hkSessionWindows, "HK")` mutation of §5.1.4. 1 statement; skip it and
   nothing is at risk, but say so rather than leaving it unexplained.
3. **The clock.** Solved by design (§5.1.4): zero minutes in a day where none of
   the three session types rejects, so all three error arms are covered in one
   run at any hour. If B3 hardcodes the expected outcome instead, up to 3
   statements (−0.7pp) go missing outside 09:00–16:00 HK and the number starts
   moving between runs — inside the slack, but it violates the AGENTS.md rule
   that matters more than the percentage.
4. **`t.Parallel()` plus the `hkSessionWindows` delete.** Two tests in the same
   package mutating a shared global concurrently is a race the `-race` run will
   catch. Prescribed: no `t.Parallel()` in that one test.
5. **`RateQueryList`'s map iteration.** A multi-key fixture with an
   order-sensitive assertion flakes. Prescribed: assert as a keyed set.

What could make the target genuinely unreachable: **nothing in this plan.** There
is no unreachable block in the uncovered set other than §5.1.4's map guard, and
that one is reachable with a two-line mutation. If B2 and B3 deliver, ≥85% is not
a risk; the realistic outcome is 100% on the package, which is what A1 and A2
both achieved. **The one thing that would put the gate at risk is lowering it**,
and [plan.md](./plan.md) already forecloses that: report partial coverage, do
not move the bar.

## 9. Findings recorded, not fixed

A1 and A2 both found defects, recorded them in `todos.md`, and did not fix them
outside their task's scope. Same discipline here. None of these is a B2/B3/B4
deliverable; each needs its own task.

| # | Finding | Where | Why it is not fixed here |
|---|---------|-------|--------------------------|
| F1 | Five market methods dereference `wireResp.Security` with no nil check, so a reply that omits `security` panics in the caller's goroutine. | `market.go:146`, `:196`, `:235`, `:281`, `:320` | a production change, outside a tester's scope. Also the reason §6.5 requires `security` in every market fixture. |
| F2 | Every `domain.*FromWire`/`FromDTO` mapper panics on a partially-populated reply, so a Gateway that omits a quoted-string decimal takes down the caller's goroutine on **13 of 29** methods. `market.go:470` (`decimalOrZero`) defends exactly this for `spreadLevel` and nothing else. | `pkg/domain/account.go`, `pkg/domain/trading.go` | the fix is a domain change, not a services one. The precedent for the fix already exists in this package and is one line per call site. |
| F3 | `trading.go:557` discards the `time.LoadLocation` error, and `now.In(nil)` panics with `time: missing Location in call to Time.In` (verified). A host with no tz database turns an HK order into a panic. | `trading.go:557` | production change. CI's `golang:1.26` images ship `zoneinfo.zip`, so the gate environment is safe; a consumer on a stripped runtime is not. |
| F4 | `validateSessionWindow` reads `time.Now()` directly, so six of its twenty statements are only reachable for a given wall-clock time and the helper is not testable against a fixed clock. | `trading.go:544-582` | production change. §5.1.4 works around it; a follow-up should take a `now func() time.Time` or an explicit time. |
| F5 | `isPriceOptional` and `isConditionalOrder` return true for order types that `validateOrderTypeEligibility` rejects for HK, and are never consulted for a non-HK market (which returns from `validateOrderForHK` first). Six of nine price-optional and three of six conditional members are therefore unobservable through any public entry point. | `trading.go:623`, `:640` vs `:41-52` | harmless today, and coverage-neutral (each `switch` is one block). Noted because it is the shape that becomes a bug when a future market is added. |
| F6 | `validateQuantityForHK` accepts a zero quantity for an HK stock, because `0` is a multiple of lot 100 and is not negative. | `trading.go:519-531` | behaviour question, not a test question. §5.1.2 pins the current behaviour so a change is visible. |
| F7 | `validatePriceForHK` accepts a **negative** price for a data type whose schedule has no tick (`TickSize == ""`, e.g. `DataTypeHKIndex`), because the early return skips `Price.Validate` entirely. | `trading.go:533-542` | same. §5.1.3 pins it. |
| F8 | A price or quantity rejected inside `ChangeEntrust` reports `Op == "trade/TradeEntrust"`, because the two helpers hardcode `opEntrust`. A caller branching on `Op` to tell which endpoint failed is told the wrong one. | `trading.go:525`, `:528`, `:539` | production change. §5.1.5 asserts the current behaviour and flags it. |
| F9 | The risk recorded in [plan.md](./plan.md): the `0.001` tick literal and the `spreadLevel`/`TickSize` naming are unresolved. **This plan does not resolve them and does not test either as settled.** | [design-tick-model.md](./design-tick-model.md) §5 | P3's follow-on, not a test task. §5.3's zero-tick fixtures are written to be *correct under both readings* — they carry the caller's tick and assert nothing about `TickSize`'s semantics, which §3.1 defers to G6. |

## 10. Verification run for this task

No source file changed, so these are a no-change baseline, not a proof of a fix.
Run with `$env:GOTMPDIR="C:\Users\Tchan\AppData\Local\Temp\opencode"`. `make` is
not installed, so targets are invoked directly.

```
$ go test -count=1 -coverprofile=$env:GOTMPDIR\svc.out ./pkg/services/
ok  	github.com/shing1211/hstongapi4go/pkg/services	0.530s	coverage: 17.4% of statements

$ go tool cover -func=$env:GOTMPDIR\svc.out   (tail)
total:								(statements)			17.4%

$ python scripts/check_links.py
check_links: scanned 101 Markdown files, 0 unresolved link(s)

$ python scripts/check_i18n.py
i18n OK: 6 languages consistent
```

**One caveat on the links, stated rather than glossed.** `mkdocs.yml` puts
`runs/` in `exclude_docs`, so **`mkdocs build --strict` does not build this
file** and cannot be claimed as covering it — the 16
`INFO - Doc file … links to 'runs/…' which is excluded from the built site`
lines are pre-existing and `INFO`, not warnings.
`scripts/check_links.py` is what resolved this file's relative links, exactly as
for [design-tick-model.md](./design-tick-model.md) and
[design-reconnect-cause.md](./design-reconnect-cause.md). `check_i18n.py` is
unaffected because the run folder has no translations.

The baseline was also re-derived per-function for §4, not taken on trust: the
uncovered-block list was read out of the profile and every row's `Uncov` count
reconciles against the file totals (49 + 86 + 230 = 365). The §5.3.1 value set,
the §5.1.4 window arithmetic, the §6.5 panics, and F3's `now.In(nil)` panic were
each measured, not reasoned about: a scratch program under `$env:GOTMPDIR` (with
a `replace` to this module) exercised `domain.MustNewPrice`,
`MustNewQuantity`, every `*FromWire`/`*FromDTO` mapper, `strconv`
`ParseFloat`→`FormatFloat`, and all 1440 minutes of the HK day. It has been
deleted; nothing was written inside the repository.
