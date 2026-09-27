# Finding — `pkg/hstong/trade` has the A6 gap, and two more shapes of it

- **Run:** `2026-09-26-vnext-parity-wire`
- **Task:** A7 (`tester`) — read-only investigation; **no `.go` file changed**
- **Date:** 2026-09-26
- **Status:** Finding recorded. Not fixed. The fix is a behaviour change to a
  released surface inside ADR 0011's protected set, so it is the human's call
  (§7).
- **Related:** [design-a6-exchange-validation.md](./design-a6-exchange-validation.md),
  task A6 in [todos.md](./todos.md),
  [ADR 0011](../../adr/0011-v01x-compatibility.md),
  [ADR 0003](../../adr/0003-no-auto-retry-orders.md)

## 1. Verdict

**There is a gap, and it is the same one A6 closed in `pkg/hstong/algo` — plus
two shapes A6 did not record anywhere.** After A6, `types.EntrustBS` is validated
against `{1,2,3,4}` on `future` and `algo` and on **no released surface at all
in `trade`**, so the type still has two answers inside one SDK. `trade` also has
**eight request types that carry `types.ExchangeType` and are not checked even
for emptiness**, four of them documented "required" — a strictly larger hole
than the emptiness-only shape A6 fixed, and not recorded in A6's "what A6 did
not do" list.

Reachability: **yes, by a caller, today, on a money-affecting mutation.**
`trade.Entrust` with `EntrustBS: "9"` and `ExchangeType: "Z"` is serialised and
put on the wire verbatim (§5).

## 2. The table: surface × field × current policy

Values come from `pkg/types/enums.go` — `ExchangeType` at `enums.go:122-129`
(`{K,P,v,t}`, case-sensitive) and `EntrustBS` at `enums.go:138-145`
(`{1,2,3,4}`) — and both are confirmed as flat closed tables in `SPEC.md` §7.3
and §7.4 with no non-exhaustive note, unlike `EntrustType` (§7.5, which is
marked **Non-exhaustive**). All four `EntrustBS` codes are valid; a value outside
`{1,2,3,4}` is the only thing in question.

| Surface | Field | Kind | Policy today | Site |
|---|---|---|---|---|
| `algo` | `AddOrderParams.EntrustBS` | request | **set-checked (fail closed)** — A6 | `pkg/hstong/algo/algo.go:302` |
| `algo` | `AddOrderParams.ExchangeType` | request | **set-checked (fail closed)** — A6 | `pkg/hstong/algo/algo.go:278` |
| `algo` | `CancelOrderParams.ExchangeType` | request | **set-checked** — A6 | `pkg/hstong/algo/algo.go:348` |
| `algo` | `CancelEntrustParams.ExchangeType` | request | **set-checked** — A6 | `pkg/hstong/algo/algo.go:378` |
| `algo` | `ChangeOrderParams.ExchangeType` | request | **set-checked** — A6 | `pkg/hstong/algo/algo.go:414` |
| `algo` | `ActionOrderParams.ExchangeType` | request | **set-checked** — A6 | `pkg/hstong/algo/algo.go:464` |
| `algo` | `QueryOrderListParams.ExchangeType` | request (optional) | **set-checked, optional-field shape** — A6 | `pkg/hstong/algo/algo.go:524` |
| `algo` | `QueryEntrustIDListParams.ExchangeType` | request | **set-checked** — A6 | `pkg/hstong/algo/algo.go:557` |
| `algo` | `MasterOrder.EntrustBS` | **response** | not validated — deliberate | `pkg/hstong/algo/algo.go:587` |
| `future` | `EntrustRequest.EntrustBS` (declared `string`) | request | **set-checked (fail closed)** | `pkg/hstong/future/future.go:490`, via `validateEntrustBS` at `future.go:158-167` |
| `future` | `ModifyEntrustRequest.EntrustBS` (declared `string`) | request | **set-checked (fail closed)** | `pkg/hstong/future/future.go:551` |
| `future` | `EntrustOrder.EntrustBS` (declared `string`) | **response** | not validated — deliberate | `pkg/hstong/future/future.go:589` |
| `future` | `ExchangeType` | — | **the type is not used in this package at all** | — |
| `market` | `EntrustBS` / `ExchangeType` | — | **neither type is used in this package at all** — it addresses instruments with `*dto.Security` from `gen/` | — |
| `stream` | `EntrustBS` / `ExchangeType` | — | **neither type is used in this package at all** — it surfaces `*tradenotify.TradeStockDeliverNotify` from `gen/` | — |
| `trade` | `EntrustRequest.EntrustBS` | request | **emptiness only** | `pkg/hstong/trade/orders.go:121` |
| `trade` | `EntrustRequest.ExchangeType` | request | emptiness only | `pkg/hstong/trade/orders.go:111` |
| `trade` | `CancelEntrustRequest.ExchangeType` | request | emptiness only | `pkg/hstong/trade/orders.go:185` |
| `trade` | `BatchCancelEntrustRequest.ExchangeType` | request | emptiness only | `pkg/hstong/trade/orders.go:232` |
| `trade` | `ChangeEntrustRequest.ExchangeType` | request | emptiness only | `pkg/hstong/trade/orders.go:286` |
| `trade` | `MaxAvailableAssetRequest.ExchangeType` | request | emptiness only | `pkg/hstong/trade/orders.go:393` |
| `trade` | `CondOrderListRequest.ExchangeType` | request | emptiness only | `pkg/hstong/trade/orders.go:628` |
| `trade` | `HistoryCondOrderListRequest.ExchangeType` | request | emptiness only | `pkg/hstong/trade/orders.go:713` |
| `trade` | `BeforeAndAfterSupportRequest.ExchangeType` | request | emptiness only | `pkg/hstong/trade/orders.go:802` |
| `trade` | `RealEntrustListRequest.ExchangeType` | request (**doc: required**) | **not checked at all** | `pkg/hstong/trade/orders.go:412`; method `orders.go:530-537` |
| `trade` | `RealDeliverListRequest.ExchangeType` | request (**doc: required**) | **not checked at all** | `pkg/hstong/trade/orders.go:425`; method `orders.go:541-548` |
| `trade` | `HistoryEntrustListRequest.ExchangeType` | request (**doc: required**) | **not checked at all** | `pkg/hstong/trade/orders.go:645`; method `orders.go:690-697` |
| `trade` | `HistoryDeliverListRequest.ExchangeType` | request (**doc: required**) | **not checked at all** | `pkg/hstong/trade/orders.go:659`; method `orders.go:701-708` |
| `trade` | `MarginFundInfoRequest.ExchangeType` | request (**doc: required**) | **not checked at all** | `pkg/hstong/trade/assets.go:111`; method `assets.go:182-188` |
| `trade` | `FundJourListRequest.ExchangeType` | request (**doc: required**) | **not checked at all** | `pkg/hstong/trade/assets.go:295`; method `assets.go:344-351` |
| `trade` | `HistoryFundJourListRequest.ExchangeType` | request (**doc: required**) | **not checked at all** | `pkg/hstong/trade/assets.go:308`; method `assets.go:355-362` |
| `trade` | `PositionsRequest.ExchangeType` | request (**doc: optional**) | not checked — the emptiness exemption is right for an optional field, but a *present* value is still unchecked | `pkg/hstong/trade/assets.go:194`; method `assets.go:284-290` |
| `trade` | `OrderVo.EntrustBS` | **response** | not validated | `pkg/hstong/trade/orders.go:442` |
| `trade` | `OrderVo.ExchangeType` | **response** | not validated | `pkg/hstong/trade/orders.go:484` |
| `trade` | `CondOrderVo.EntrustBS` | **response** | not validated | `pkg/hstong/trade/orders.go:592` |
| `trade` | `CondOrderVo.ExchangeType` | **response** | not validated | `pkg/hstong/trade/orders.go:577` |
| `trade` | `HoldsVo.ExchangeType` | **response** | not validated | `pkg/hstong/trade/assets.go:236` |

Counts: **17 caller-supplied request sites in `trade`**, spread over 16 request
types (`EntrustRequest` carries both fields) — 9 checked for emptiness only, 8
not checked at all. Plus 5 Gateway-supplied response sites.

So `pkg/hstong/trade` has **no set check on either type, anywhere**. Across the
released surfaces, `EntrustBS` has one on `future` and `algo` and none on
`trade`; `ExchangeType` has one on `algo` only. A6's design note already
observed the `ExchangeType` side at its lines 251-255; this investigation
confirms it and quantifies it.

## 3. Parity verdict

`types.EntrustBS` is **not** validated consistently across every surface that
sends it. It has two answers inside one SDK:

| Value | `future.Entrust` | `algo.AddOrder` | `trade.Entrust` |
|---|---|---|---|
| `1`, `2`, `3`, `4` | sent | sent | sent |
| anything else | **rejected locally, 0 requests** | **rejected locally, 0 requests** | **forwarded on the wire** |

That is A6's argument one surface over, and A6 named `trade` in its own "what A6
did not do" list (`design-a6-exchange-validation.md:248-250`: "`pkg/hstong/trade`
has the same `EntrustBS` gap at `orders.go:121`"). This task confirms the line
number and adds that the same type, on the same endpoint family
(`/trade/TradeEntrust` is reachable from `trade.Entrust` and, in the v-next
layer, from `pkg/services`), is now the odd one out in a third place.

### 3.1 Every remaining divergence, with `file:line`

**Reachable by a caller (request fields) — actionable:**

| # | Divergence | Site |
|---|---|---|
| D1 | `EntrustRequest.EntrustBS` is emptiness-only, so an out-of-set direction reaches a **mutation** that is issued once and never retried | `pkg/hstong/trade/orders.go:121` |
| D2 | `EntrustRequest.ExchangeType` is emptiness-only, on the same mutation | `pkg/hstong/trade/orders.go:111` |
| D3 | `CancelEntrustRequest.ExchangeType` — the cancel names the book the Gateway resolves the master against | `pkg/hstong/trade/orders.go:185` |
| D4 | `BatchCancelEntrustRequest.ExchangeType` — an **empty** `EntrustIDs` cancels *every* cancellable order in the market, in the market the SDK was handed | `pkg/hstong/trade/orders.go:232` |
| D5 | `ChangeEntrustRequest.ExchangeType` | `pkg/hstong/trade/orders.go:286` |
| D6 | `MaxAvailableAssetRequest.ExchangeType` | `pkg/hstong/trade/orders.go:393` |
| D7 | `CondOrderListRequest.ExchangeType` | `pkg/hstong/trade/orders.go:628` |
| D8 | `HistoryCondOrderListRequest.ExchangeType` | `pkg/hstong/trade/orders.go:713` |
| D9 | `BeforeAndAfterSupportRequest.ExchangeType` | `pkg/hstong/trade/orders.go:802` |
| D10 | `RealEntrustListRequest.ExchangeType` — **no check, not even emptiness**, though the field is documented required | `pkg/hstong/trade/orders.go:412` |
| D11 | `RealDeliverListRequest.ExchangeType` — same | `pkg/hstong/trade/orders.go:425` |
| D12 | `HistoryEntrustListRequest.ExchangeType` — same | `pkg/hstong/trade/orders.go:645` |
| D13 | `HistoryDeliverListRequest.ExchangeType` — same | `pkg/hstong/trade/orders.go:659` |
| D14 | `MarginFundInfoRequest.ExchangeType` — same; the field doc names all four codes and then says "It is required" | `pkg/hstong/trade/assets.go:111` |
| D15 | `FundJourListRequest.ExchangeType` — same | `pkg/hstong/trade/assets.go:295` |
| D16 | `HistoryFundJourListRequest.ExchangeType` — same | `pkg/hstong/trade/assets.go:308` |
| D17 | `PositionsRequest.ExchangeType` — optional, so no emptiness check is right, but a *present* out-of-set value is unchecked; this is the `algo.QueryOrderList` optional-field shape (§6) | `pkg/hstong/trade/assets.go:194` |

**Not caller-reachable (response fields) — only reachable if the Gateway is
wrong, and correctly not actionable as a validation matter:**

| Site | Note |
|---|---|
| `pkg/hstong/trade/orders.go:442` `OrderVo.EntrustBS` | decoded from the Gateway |
| `pkg/hstong/trade/orders.go:484` `OrderVo.ExchangeType` | decoded from the Gateway |
| `pkg/hstong/trade/orders.go:592` `CondOrderVo.EntrustBS` | decoded from the Gateway |
| `pkg/hstong/trade/orders.go:577` `CondOrderVo.ExchangeType` | decoded from the Gateway |
| `pkg/hstong/trade/assets.go:236` `HoldsVo.ExchangeType` | decoded from the Gateway |
| `pkg/hstong/algo/algo.go:587` `MasterOrder.EntrustBS` | A6 already recorded this decision |
| `pkg/hstong/future/future.go:589` `EntrustOrder.EntrustBS` | declared `string`, not the typed alias |

A6's reasoning for the response side (`design-a6-exchange-validation.md:259-262`)
applies verbatim and this task endorses it: validating inbound data would turn a
vendor-side surprise into a decode error, which is a different policy from
validating outbound requests. **The response rows are listed for completeness of
the table, not as gaps.**

**Out of scope, but recorded so the count is honest.** The v-next layer carries
a **fourth** answer for the same type:
`pkg/services/trading.go:610-621` `validateShortSellingFlag` accepts exactly
`{1,2,3,4}` but **only when `DataType` is HK stock/ETF** — it returns `nil` for
every other instrument, so a US order's `types.EntrustBS` is unvalidated there
too. `pkg/services` derives `ExchangeType` from a `domain.Market` rather than
taking a raw `types.ExchangeType` from the caller (`trading.go:123-129`), so it
has no caller-facing market gap; note separately that
`MarketShenzhenConnect` and `MarketShanghaiConnect` fall through that `switch`
to the empty string, which is a different defect and not this one. `pkg/services`
is unwired and carries no compatibility guarantee (`ADR 0011:92-94`), so it is
not counted as a released-surface divergence.

## 4. Why this matters more on `trade` than on `algo`

One paragraph, because A6's cost asymmetry is the argument that decided `algo`
and `trade` is where that asymmetry is sharpest.
`trade.Entrust`, `trade.CancelEntrust`, `trade.BatchCancelEntrust` and
`trade.ChangeEntrust` place, cancel, mass-cancel and modify real orders; each
issues **exactly one** HTTP request and is never retried at any layer
(`pkg/hstong/trade/doc.go:35-41`, ADR 0003). A mistyped direction that reaches
the Gateway is answered once, with no second attempt by the SDK. D4 is the
sharpest single case in the table: an unvalidated market combined with an empty
`EntrustIDs` is a cancel-everything request against a book the caller did not
name, sent once.

## 5. Reachability — reproduction

**A caller can reach this today.** No test was added to the package, as the task
requires. The reproduction below was run as a scratch program in a separate
module outside the repository (`GOTMPDIR`, per AGENTS.md binary hygiene), pointed
at an `httptest` server standing in for the Gateway, and then deleted. It imports
the public packages only, so it is exactly what any consumer can do.

```go
// scratch module, with replace github.com/shing1211/hstongapi4go => <repo>
rec := &recorder{}                       // records path + body, replies a canned ok envelope
srv := httptest.NewServer(rec)
c, _ := client.New(client.WithBaseURL(srv.URL))
tm, fm, am := trade.New(c), future.New(c), algo.New(c)

// 1. trade, out-of-set direction AND out-of-set market, on a mutation
_, err := tm.Entrust(ctx, trade.EntrustRequest{
    ExchangeType: "Z", StockCode: "01810.HK",
    EntrustAmount: "100", EntrustPrice: "35.900",
    EntrustBS: "9", EntrustType: types.EntrustTypeLimit,
})

// 2. the same value, the same SDK, two other surfaces
_, err = fm.Entrust(ctx, future.EntrustRequest{StockCode: "HSI2603", EntrustBS: "9", ...})
_, err = am.AddOrder(ctx, algo.AddOrderParams{ExchangeType: types.ExchangeHK, EntrustBS: "9", ...})

// 3. a documented-required field with no check at all
_, err = tm.MarginFundInfo(ctx, trade.MarginFundInfoRequest{})   // ExchangeType stays ""

// 4. the response side
//    the Gateway replies with one row carrying "entrustBs":"7","exchangeType":"Q"
rows, _ := trade.New(c2).RealEntrustList(ctx, trade.RealEntrustListRequest{ExchangeType: types.ExchangeHK})
//    rows[0].EntrustBS == types.EntrustBS("7"), err == nil
```

Measured output:

```
trade.Entrust(BS=9, exchange=Z)                requests=1  SENT
     wire: /trade/TradeEntrust {"timeout_sec":10,"params":{"exchangeType":"Z","stockCode":"01810.HK",
           "entrustAmount":"100","entrustPrice":"35.900","entrustBs":"9","entrustType":"3"}}
trade.Entrust(BS=3 valid, exchange=k)          requests=1  SENT
     wire: /trade/TradeEntrust {... "exchangeType":"k" ... "entrustBs":"3" ...}
trade.CancelEntrust(exchange=Z)                requests=1  SENT
     wire: /trade/TradeCancelEntrust {"timeout_sec":10,"params":{"exchangeType":"Z","stockCode":"01810.HK","entrustId":"ENT-1"}}
trade.RealEntrustList(exchange=Z, unchecked)   requests=1  SENT
     wire: /trade/TradeQueryRealEntrustList {"timeout_sec":10,"params":{"exchangeType":"Z",...}}
trade.MarginFundInfo(exchange="", unchecked)   requests=1  SENT
     wire: /trade/TradeQueryMarginFundInfo {"timeout_sec":10,"params":{"exchangeType":""}}

future.Entrust(BS=9)   requests=0  REJECTED-LOCALLY: trade/FuturesEntrust: 1016 entrustBs must be one of 1, 2, 3, 4
algo.AddOrder(BS=9)    requests=0  REJECTED-LOCALLY: trade/AlgoAddOrder: entrustBs "9" is not one of 1
                                  (open long), 2 (close long), 3 (close short), 4 (open short): algo: invalid params

response-side decode: err=<nil> rows=1
response-side decode: entrustBs="7" (types.EntrustBS) exchangeType="Q"
```

Near-misses a caller is likely to send, all forwarded by `trade.Entrust` today,
alongside the four documented directions, all correctly accepted:

```
trade.Entrust(BS="0")   requests=1  SENT          trade.Entrust(BS="1")  requests=1  SENT
trade.Entrust(BS="5")   requests=1  SENT          trade.Entrust(BS="2")  requests=1  SENT
trade.Entrust(BS="buy") requests=1  SENT          trade.Entrust(BS="3")  requests=1  SENT
trade.Entrust(BS="1 ")  requests=1  SENT          trade.Entrust(BS="4")  requests=1  SENT
trade.Entrust(BS="١")   requests=1  SENT          trade.Entrust(BS="")   requests=0  REJECTED: 1016 entrustBs is required
```

Three things this pins down:

1. **The request is actually sent**, not merely accepted by a validator that
   forgets to return. `requests=1` plus a wire body containing
   `"entrustBs":"9"` is the proof.
2. **`3` and `4` already work on `trade`**, so a future fix that rejected them
   would repeat the exact mistake A6's design note corrected (its lines 34-40).
   Any fix must keep all four accepted and pin that with a test.
3. **`algo` rejects the direction on its own** once the market is in-set, so the
   A6 fix is confirmed live and not merely present in the source.

## 6. Recommended scope for a future fix task

Not done here. A6 is the template and the same reasoning transfers verbatim: the
deciding argument A6 recorded was that `future` already chose fail-closed for
this exact type, and `trade` is now the single released surface where an
out-of-set value survives.

| Item | Recommendation |
|---|---|
| Surfaces in scope | `pkg/hstong/trade` only. `market` and `stream` need nothing (neither carries the type). `future` needs no `EntrustBS` change, and its missing `ExchangeType` validation is moot because the type is absent from the package. |
| Direction | **Fail closed**, on A6's reasoning: closed four-row sets in `SPEC.md` §7.3 and §7.4, a constant for every value in `pkg/types`, no non-exhaustive note, and one mutation (D4, cancel-everything) where the failure is silent and unrepeatable. The `EntrustType` and `targetStrategy` forward-compatible precedents do not apply, for the reasons in the A6 note. |
| Mechanism | Two unexported free functions in `pkg/hstong/trade`, `validExchange` and `validDirection`, plus two shared message builders. A free function, not a method on `types.ExchangeType`, because `pkg/types` is inside ADR 0011's protected surface and a Go method cannot be declared on a non-local type (`design-a6-exchange-validation.md:116-123`). |
| Sites | All 17 request sites, in two shapes: emptiness-first then set-check at the 9 that check emptiness (D1-D9), and set-check at the 8 that check nothing (D10-D17) — where **the missing emptiness check must be added too**, because four of those fields are documented "required" and a fix that only added a set check would still send `exchangeType: ""`. |
| Optional field | `PositionsRequest.ExchangeType` (D17) takes the `algo.QueryOrderList` optional-field shape: absent accepted, present-and-unrecognised refused (`design-a6-exchange-validation.md:143-146`). |
| `doc.go` | Add a validation section naming the closed sets, matching what A6 added to `pkg/hstong/algo/doc.go`. `pkg/hstong/trade/doc.go` currently has codec / money / session / no-auto-retry / pagination sections and says nothing about validation. |
| Tests | A `TestValidatedCodeSetsRejectEveryOutOfSetValue` matrix over all 17 sites, a `TestEveryDocumentedCodeIsAccepted` that pins all four markets and all four directions **on the wire**, and one row per site in the existing rejection matrix (`pkg/hstong/trade/validation_test.go`, which already carries `exchangeType required` rows for `CancelEntrust`, `ChangeEntrust` and `MaxAvailableAsset`). **No existing trade test asserts that an out-of-set value is forwarded** — verified by grep — so unlike A6's `TestUnvalidatedCodesAreForwardedToTheGateway` there is nothing to invert. A future task should grep again rather than assume it. |
| Shared `Valid()` on `pkg/types` | Out of scope for this fix. It would widen a protected package's public surface and needs its own ADR (`design-a6-exchange-validation.md:242-244`). If wanted, take it once for `algo` and `trade` together. |

### 6.1 ADR 0011

`pkg/hstong/trade` is in ADR 0011's protected surface table (`0011:29`), so the
change is measured against its guarantees:

| Guarantee | Assessment |
|---|---|
| 1. No breaking type changes | **Untouched.** No exported type, field, signature or method changes; the new identifiers are unexported. `pkg/types` gains nothing. |
| 2. No breaking wire changes | **Untouched.** No `gen/` file and no wire struct is edited; an in-set request serialises byte-identically. |
| 3. No new mandatory dependencies | **Untouched.** A `switch` over existing constants; `fmt` and `pkg/types` are already imported by `pkg/hstong/trade`. |
| 4. Mock Gateway compatibility | **Untouched.** `test/mockgateway/` and `cmd/hstong-mock-gateway/` need no change; the mock returns in-set values. |
| 5. `gen/` never touched | **Untouched.** |

What *is* a behaviour change: requests that previously reached the Gateway will
not. ADR 0011's permitted-changes table covers this in the row **"Bug fixes
that change runtime behaviour — expected; bugs are not specified behaviour"**
(`0011:71`), the same row A6 relied on. **So no waiver and no ADR amendment is
mechanically required, exactly as in A6.** The difference from A6 is not
mechanical, it is scope: 17 sites instead of 8, one released surface instead of
one, and a wider enumerated blast radius (every caller of 16 request types). That
is why this is a finding and not a fix.

## 7. The open question for the human

**Fix in the 0.1.x line now, or defer to v1.0 when ADR 0011 expires?**

The case for fixing now, which is the recommendation:

- `future` and `algo` already reject these values. Leaving `trade` open keeps the
  inconsistency the A6 decision was made to end; the longer it stands, the more
  callers will build on the fail-open behaviour of the one surface that permits
  it, and the more expensive the eventual break.
- ADR 0011 permits the change, and the mechanism is proven, small and offline.
- The worst-affected endpoint is D4, a cancel-everything request on a book the
  caller may not have named, issued once with no retry.

The case for deferring, which is legitimate:

- ADR 0011's Deprecation section (`0011:76-83`) says `pkg/hstong/*` will be
  deprecated once v-next reaches parity, with a 6-month notice before removal.
  Fixing a surface that is on its way out spends a behaviour change on a
  shrinking audience.
- **But "wait for v-next" does not currently mean "the gap gets fixed."** The
  v-next path is `pkg/services`, and today it has its own fourth policy:
  `validateShortSellingFlag` validates the direction **only for HK stock/ETF**
  (`pkg/services/trading.go:610-621`). Deferring on the assumption that parity
  work will close the gap is deferring on an assumption, not on a plan.

Whichever way it goes, the decision belongs to you and this note is the evidence
for it. A sequencing note: if the answer is *defer*, the minimum that costs
nothing is to add a line to `pkg/hstong/trade/doc.go` recording that
`exchangeType` and `entrustBs` are forwarded unvalidated on this surface, so the
policy is documented rather than accidental — the same treatment A6 gave the one
deliberately-forwarded field.

## 8. What A7 did not do

- **No `.go` file was modified, no test added, no code fixed.** The only new
  file in this repository is this note.
- A6's uncommitted `pkg/hstong/algo` work was read, not touched.
- `pkg/services`, `internal/`, `gen/`, `scripts/` and every workflow were read
  only, and nothing in them was modified.
- Nothing was committed or pushed.
- The reproduction in §5 was run from a scratch module outside the repository
  and deleted; it is described here rather than committed, so no in-repo
  assertion pins the fail-open behaviour and a future fix starts from a clean
  slate.
- The response-side rows in §2 and §3 are completeness, not findings. They are
  Gateway-supplied, and A6's rationale for leaving them unvalidated is endorsed
  unchanged.
- No v-next task was planned. `pkg/services/trading.go:610-621` is named in
  §3.1 because counting the answers honestly requires it; a task for the v-next
  surface is a different decision from a task for the released one.

## 9. Verification run for this task

No source file changed, so these are a no-change baseline.

```
$ python scripts/check_links.py
check_links: scanned 104 Markdown files, 0 unresolved link(s)
exit=0

$ python scripts/check_i18n.py
i18n OK: 6 languages consistent
exit=0
```

`check_links.py` scanned 104 files. A6's note reported 100 when it ran; the
delta is not this note's doing — `git ls-tree HEAD` carries 101 Markdown files
and this run folder has three untracked notes (`design-a6-exchange-validation.md`,
`plan-a6-exchange-validation.md` and this one), so 104 is the working-tree total
with all three present. The links this note relies on are
`./design-a6-exchange-validation.md`, `./todos.md`,
`../../adr/0011-v01x-compatibility.md` and
`../../adr/0003-no-auto-retry-orders.md`; all four resolved on disk. Go source
files are cited as bare `path:line` text rather than links, following the
convention in the A6 note and the rest of the run folder — the first draft of
this note linked them and `check_links.py` correctly rejected all ten, which is
why they are plain text now.

**Caveat, stated rather than glossed.** `docs/runs/**` is excluded from the
mkdocs nav, so `mkdocs build --strict` **does not build this file**;
`check_links.py` is what validated it. The evidence that the exclusion is real
is the pre-existing `INFO - Doc file ... contains a link to 'runs/...' which is
excluded from the built site` line emitted by
[adr/0008-decimal-financial-types.md](../../adr/0008-decimal-financial-types.md),
recorded in `design-reconnect-cause.md` §7. `--strict` was therefore not run and
is not claimed as coverage of this file. `check_i18n.py` is unaffected because
the run folder has no translations.
