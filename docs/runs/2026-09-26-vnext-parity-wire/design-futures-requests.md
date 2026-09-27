# C2a — The eleven futures **request** shapes

- **Run:** `2026-09-26-vnext-parity-wire`
- **Task:** C2a (`architect`) — a design note. **No code changed; no `.go` file touched.**
- **Date:** 2026-09-26
- **Status:** Decided. C3–C6 implement against this.
- **Related:** [plan.md](./plan.md), [todos.md](./todos.md) C2a/C3–C6,
  [ADR 0003](../../adr/0003-no-auto-retry-orders.md),
  [ADR 0007](../../adr/0007-http-json-codec.md),
  [ADR 0008](../../adr/0008-decimal-financial-types.md),
  [ADR 0010](../../adr/0010-vnext-layered-architecture.md),
  [ADR 0011](../../adr/0011-v01x-compatibility.md),
  [design-tick-model.md](./design-tick-model.md),
  [design-a6-exchange-validation.md](./design-a6-exchange-validation.md),
  [design-a8-trade-validation.md](./design-a8-trade-validation.md),
  [design-parity-guard.md](./design-parity-guard.md)

## 1. Decision, and the correction it rests on

**The task brief's premise is wrong, and correcting it removes almost all of the
risk the task was created to manage.**

The brief states: *"there is **no released v0.x futures at all** — the 13 released
trade endpoints are all cash — so unlike `market`/`trading`/`account` there is no
prior implementation whose request bodies can be copied."*

Both halves are false.

1. **`pkg/hstong/future` is a complete, released, 11-endpoint implementation with
   all eleven request bodies written out**, shipped in `v0.1.0` (commit `500b0a1`,
   which `git tag --contains` lists in `v0.1.0` through `v0.1.21`). It is
   documented in [docs/futures.md](../../futures.md), counted in
   [docs/index.md](../../index.md) as "Futures | `pkg/hstong/future` | 11", listed
   in ADR 0011's protected surface, and referenced by
   [docs/quick-reference.md](../../quick-reference.md) §Futures. The 13 cash trade
   endpoints are §2.5; futures is §2.8 and is 11 more.
2. **The primary source is in the repository root and has never been read for this
   purpose.** `华盛通OpenAPI-SDK-Java.zip` and `华盛通OpenAPI-SDK-Python.zip`, both
   vendor SDK **v2.3.0** (`_20251023`), contain the futures request parameter
   classes and — in Python — build the futures request bodies as **explicit JSON
   key literals**, which is wire-level evidence, not documentation prose. ADR 0007
   already established these two archives as authoritative for the codec question;
   nobody ran them for the futures request shapes.

The Python SDK, `hs/api/hs_open_api.py:704-869`, is the single most useful
artifact in this repository for this task, and it is a *wire* source:

```python
def futures_entrust(self, stock_code, entrust_type, entrust_price, entrust_amount,
                    entrust_bs, valid_time_type, valid_time,
                    order_options=FuturesOrderOptions.DEFAULT):
    request_data = {
        'stockCode': stock_code, 'entrustType': entrust_type,
        'entrustPrice': entrust_price, 'entrustAmount': entrust_amount,
        'entrustBs': entrust_bs, 'validTimeType': valid_time_type,
        'orderOptions': order_options
    }
    if valid_time is not None:
        request_data['validTime'] = valid_time
```

Every JSON key in the three mutations is a string literal in vendor source. So
this is not a case of inferring field names from a response schema or from
symmetry. It is reading them off the wire.

**Consequence for the run.** C5's mutations are no longer being built on guessed
field names. The provenance split in §5 is **12 derived, 12 inferred, 0 unknown**
across 24 distinct request fields — and the 12 inferred are *not* guesses about
names. Every one of them is a judgement about **requiredness, `omitempty`, Go
type, or enum set** on a field whose name and tag are confirmed twice over. The
number C5 needs to be reviewed against is 12, not 24, and the thing to review is
a validation policy, not a spelling.

## 2. The evidence hierarchy

The brief asked which inference bases are strong and which are weak. Ranked:

| # | Evidence | Strength | What it settles |
|---|----------|----------|-----------------|
| 1 | **Vendor Python SDK v2.3.0** — `hs/api/hs_open_api.py:704-869`, explicit `request_data` dict literals per route | **Decisive** | Every request field name, JSON tag, and per-route presence/absence. Two independent confirmations of shape. |
| 2 | **Vendor Java SDK v2.3.0** — `sdk/vo/futures/*Param.java` and `sdk/constant/futures/*.java` | **Decisive on names, authoritative on requiredness and enums** | Field names via class fields; the *required* set via the `isAnyBlank` guards in `HSQuantOpenApiHandle.java:932-1110`; four closed enum families. |
| 3 | **Released `pkg/hstong/future/future.go`** | **Strong, but downstream of 1 and 2** | The v-next layer's *parity target*. Independently confirms 1 and 2 on all 24 names, so it is a genuine second source, not an echo — it was written in `v0.1.0` from "the current docs plus the legacy proto definitions" ([P07-futures.md](../2026-09-21-hstong-full-surface/phases/P07-futures.md) §Decisions), which is the same evidence base. |
| 4 | Response schemas in [SPEC.md](../../SPEC.md) §3 | **Weak for requests** | Almost nothing. See §2.1. |
| 5 | SPEC §8 data dictionary | **Weak, and mostly inapplicable** | See §2.2. |
| 6 | Cash request shapes in `pkg/services` | **Weak, pattern-only** | See §2.3. |
| 7 | Mock fixtures, `test/mockgateway/fixtures.go:255-265` | **Zero for requests** | The brief is right about this and it is worth confirming: all eleven futures entries are `b.set(path, body)` static responses. Only the *cash* fund-journey and trade-list fixtures are `b.cursor(...)`, and `cursorFixture.page` (`fixtures.go:113-132`) reads `queryParamStr` out of the request params. No futures fixture inspects a request body at all, so the mock cannot confirm or refute any field in this document. |
| 8 | [docs/LEGACY.md](../../LEGACY.md) | **Zero** | Confirmed: futures appears there only as a push channel, with no fields. |
| 9 | `proto/trade/` | **Zero** | Confirmed: only `notify/TradeStockDeliverNotify.proto`. The vendor `华盛通OpenAPI-SDK-PB.zip` matches — its entire `trade/` tree is that one notify file. The legacy proto definitions named in P07 (`FuturesFundInfoVo`, `FuturesHoldsVo`, `FuturesQueryBuySellAmountResponse`, `FuturesProductInfoVo`) are **response** types, which is exactly why the *responses* ended up doubly sourced and the *requests* did not. |

### 2.1 SPEC §3 buys almost nothing for requests

§3 is response schemas, and its futures entries are `FundInfo`, `Hold`,
`EntrustOrder`, `EntrustResponse`, `QueryProductInfoResponse`,
`QueryMaxBuySellAmountResponse`, `QueryFundInfoResponse`, `QueryHoldsListResponse`,
`EntrustListResponse`, `DeliverListResponse`. It is worth being precise about how
little it implies, because "symmetry with the response schema" is the inference
the brief nominated and it is the weakest available:

- `EntrustOrder` (SPEC L359) carries `entrustBs`, `entrustPrice`, `entrustAmount`,
  `entrustType`, `entrustTypeNum`, `entrustId`, `validType`, `validTypeDesc`,
  `orderOptions`, `validTime`, `stockCode`. Seven of these are request fields, so
  the symmetry is real — but it is **also wrong in two places**, which is the
  proof that it is not a reliable mechanism on its own:
  - The response spells the time-in-force `validType`; the request spells it
    `validTimeType`. Symmetry would have produced the wrong key.
  - The response has no `validTimeType` and the request has no `validType`.
    A field-by-field copy in either direction produces a wire body the Gateway
    does not accept.
- `QueryMaxBuySellAmountResponse` has `positionStatus`, `maxBuyAmount`,
  `maxSellAmount`, `initialMargin`, `ccy` — and **no** request field among them.
  The request is `{stockCode}` and `stockCode` appears in no response of that
  route.
- Four of the eleven routes have responses that name nothing in the request.

So: use §3 as a **cross-check** on names, never as the source. Every field in
§4 below that is labelled *derived* is derived from evidence 1–3, and evidence 4
is cited only where it independently agrees.

### 2.2 SPEC §8 is mostly the wrong dictionary for this task

The brief nominated `entrustProp`, `entrustStatus`, `exchangeType`, `entrustBs`,
`entrustType`, `realStatus`, `realType`, `moneyType`. Checking each against the
futures **request** bodies:

| §8 family | Appears in a futures request? | Note |
|-----------|-------------------------------|------|
| `entrustProp` (L733) | No | Response/push only, and the table is marked **non-exhaustive**. |
| `entrustStatus` (L796) | No | Response only (`EntrustOrder.status`). A closed set, but not a request field. |
| `exchangeType` | **No, and this is the important one** | No futures request carries a market at all — see §7.4. A6 and A8's work does not transfer. |
| `entrustBs` (L796 §7.4) | **Yes, and it conflicts with the vendor** | See §9. |
| `entrustType` (§7.5) | Yes | But §7.5 is the **cash per-market** table (HK/US/A-share/conditional, running past 31). It is not the futures table. Using it for futures would be a category error; the futures table is `FuturesEntrustType` in the vendor SDK. |
| `realStatus` (L7.7) | No | Push only. |
| `realType` (L7.8) | No | Push only. |
| `moneyType` (L7.11) | No | Neither request nor response. `Hold.ccy` and `QueryMaxBuySellAmountResponse.ccy` are free-text currency codes (`"HKD"` in the fixture), **not** the `0/1/2` `moneyType` code. Conflating them would be an easy and expensive error. |

So the brief's §8 list is 6 of 8 inapplicable to futures requests, and the two
that do apply both point at the *wrong* table. The enum families that actually
govern futures requests are four, and they live in the vendor SDK, not in SPEC §8:
`FuturesEntrustBs`, `FuturesEntrustType`, `FuturesValidTimeType`,
`FuturesOrderOptions`. §9 tabulates them.

### 2.3 The cash pattern, and how far it stretches

`realFundJourListWireRequest` uses `exchangeType` / `queryCount` /
`queryParamStr`; `historyFundJourListWireRequest` adds `startDate` / `endDate`.
That is the cash cursor idiom, and **futures does not use it**. The vendor's
`FuturesPageQueryParam` has exactly four fields — `pageNo`, `pageSize`,
`startDate`, `endDate` — and no `exchangeType`, no `queryCount`, no
`queryParamStr`, no `stockCode`. The four account-scoped futures reads
(`QueryFundInfo`, `QueryHoldsList`, `QueryRealEntrustList`,
`QueryRealDeliverList`) send **no parameters at all**: Java calls
`postMapRequest(..., Maps.newHashMap())` (`HSQuantOpenApiHandle.java:441,461,1024,1071`)
and Python passes `request_data={}`.

Two structural consequences for C3:

- **Futures pagination is page-number based, not cursor based.** It is
  incompatible with the cursor semantics of `transport.Pagination`
  (`pkg/transport/pagination.go`: `Cursor`, `PageSize`, `HasMore`, and an `Apply`
  that writes `cursor` / `page_size` keys). `transport` is outside ADR 0011's
  protected surface, so it *may* grow a `PageNo`, but doing so for futures alone
  would give one struct two incompatible meanings. §6.3 recommends the shape.
- **There is no `exchangeType` anywhere in futures.** Cash `HoldsList` needs one
  because the account holds positions in several books; futures positions are
  keyed by contract code in the response, and the request needs nothing. §7.4
  records the consequence for method signatures.

## 3. What the vendor requiredness guards say

`HSQuantOpenApiHandle.java` validates before building the request, and those
guards are the vendor's own statement of which fields are required. This matters
because §4 has to choose between the vendor's requiredness and the released
implementation's, and they disagree in seven places.

| Route | Vendor guard (`HSQuantOpenApiHandle.java`) | Released `future.go` |
|-------|------------------------------------------|---------------------|
| `FuturesQueryProductInfo` | `CollectionUtils.isEmpty(getStockCode())` → reject (L507) | non-empty list required ✓ **agree** |
| `FuturesQueryMaxBuySellAmount` | `isBlank(getStockCode())` → reject (L481) | non-empty, no whitespace ✓ **agree** |
| `FuturesQueryFundInfo` / `QueryHoldsList` / `QueryRealEntrustList` / `QueryRealDeliverList` | no param object at all | `struct{}{}` ✓ **agree** |
| `FuturesQueryHistoryEntrustList` / `HistoryDeliverList` | `isAnyBlank(startDate, endDate)` **or** `pageNo == nil` **or** `pageSize == nil` → reject (L1044, L1091) | `startDate`/`endDate` **optional**; `pageNo`/`pageSize` defaulted ✗ **disagree** |
| `FuturesEntrust` | `isAnyBlank(stockCode, entrustType, entrustPrice, entrustAmount, entrustBs, validTimeType)` → reject; plus `validTimeType=="4"` ⇒ `validTime` non-blank (L934-942) | `entrustType` and `validTimeType` `omitempty`; price waived for a market order ✗ **disagree** |
| `FuturesCancelEntrust` | `isAnyBlank(stockCode, entrustId)` → reject (L967) | both non-empty ✓ **agree** |
| `FuturesModifyEntrust` | `isAnyBlank(stockCode, entrustId)` → reject (L993); plus the `validTime` coupling. **Note `getValidTimeType().equals(...)` at L997 is called unguarded, so a null `validTimeType` NPEs — the guard implies it is required.** | also requires positive `entrustAmount`, well-formed `entrustPrice`, `entrustBs` ∈ 1–4 ✗ **disagree** |

Python agrees with Java on every guard except one: `futures_entrust` and
`futures_change_entrust` make `valid_time_type` a **positional, non-defaulted**
parameter, so it is required in Python too.

## 4. The eleven request bodies

Go type conventions follow `pkg/services`: unexported `…WireRequest` structs with
explicit `json:"…"` tags, an exported `…Request`/`…Filter` shape for the caller,
and money/quantity as `string` on the wire ([ADR 0008](../../adr/0008-decimal-financial-types.md)).
**No futures request field is numeric on the wire** — the only non-string fields
anywhere are `pageNo` and `pageSize`, which the vendor types as boxed `Integer`,
and both are page counters, not money or quantity. This is the same
quoted-money convention the cash layer established (plan.md assumption 2) and it
means `json.Number` is not needed for futures requests.

### 4.1 The eight reads

#### `FuturesQueryProductInfo` → `futuresProductInfoWireRequest`

| Go field | JSON tag | Go type | Req/Opt | Why |
|---|---|---|---|---|
| `StockCodes` | `stockCode` | `[]string` | **required**, non-empty | The Gateway's own array key. Python `{'stockCode': stock_code_list}` (`hs_open_api.py:736-738`); Java `FuturesCodeListParam.stockCode: List<String>`. Singular tag for a list is the vendor's spelling, not a typo — do not "fix" it to `stockCodes`. |

No `exchangeType`, no `dataType`. A futures contract code is globally unique across
the HK and US books (`Hold.dataType` exists precisely to *read back* which book a
code belongs to, SPEC L353).

#### `FuturesQueryMaxBuySellAmount` → `futuresMaxBuySellAmountWireRequest`

| Go field | JSON tag | Go type | Req/Opt | Why |
|---|---|---|---|---|
| `StockCode` | `stockCode` | `string` | **required**, non-empty, no internal whitespace | Python `{'stockCode': stock_code}` (L725-727); Java `FuturesCodeParam.stockCode`. One code: the response has no list, so there is no plural variant. |

#### `FuturesQueryFundInfo`, `FuturesQueryHoldsList`, `FuturesQueryRealEntrustList`, `FuturesQueryRealDeliverList` → no request body

**These four send an empty `params` object and therefore have no wire struct.**
Java `Maps.newHashMap()` at L441/461/1024/1071; Python `request_data={}` at
L711/718/826/851. The released layer sends `struct{}{}`, which marshals to `{}`.

C3 should not invent a `…WireRequest` for these. `struct{}{}` matches the
released layer byte-for-byte and the parity test can assert `params == {}`. The
one thing to get right is that they are *not* `nil`: `client.Do` marshalling a nil
`params` would send `"params":null`, which is a different body. `struct{}{}` is
the correct literal and it is what `future.go` already uses.

#### `FuturesQueryHistoryEntrustList`, `FuturesQueryHistoryDeliverList` → `futuresPageQueryWireRequest`

**One struct for both routes.** The vendor uses one `FuturesPageQueryParam` class
and one Python signature shape for both (`hs_open_api.py:828-869` are
line-for-line identical apart from the URL), so C3 must not define two
structures. Note this is the mirror image of the cash layer, which defines
`historyEntrustListWireRequest` and `historyDeliverListWireRequest` separately
with identical bodies (`trading.go:770-784`) — that duplication has no vendor
justification and should not be copied.

| Go field | JSON tag | Go type | Req/Opt | Why |
|---|---|---|---|---|
| `PageNo` | `pageNo` | `int` | **required**, ≥ 1 | Python `{'pageNo': page_no}` (L837); Java `FuturesPageQueryParam.pageNo: Integer`, guarded non-null (L1045). Vendor docstring: "页码 默认1". Default 1 when the caller leaves it zero. |
| `PageSize` | `pageSize` | `int` | **required**, 1 ≤ n < 100 | Python L838; Java non-null (L1045). "每页返回数量 默认20". The `< 100` bound is the released layer's `maxPageSizeExclusive` (`future.go:46-48`) and matches the released `DefaultPageSize = 20`. `condOrderListWireRequest` in the v-next layer also uses `int` here, so `int` is the house type — **not** `int32` as the released layer uses. |
| `StartDate` | `startDate` | `string` | **optional** (see below), `yyyyMMdd` when set | Python L839; Java `startDate`. Docstring: "起始日期 格式为：yyyyMMdd". |
| `EndDate` | `endDate` | `string` | **optional** (see below), `yyyyMMdd` when set | Python L840; Java `endDate`. |

**The `startDate`/`endDate` requiredness is a real divergence and this note
resolves it in favour of the released layer.** The vendor rejects a history query
with a blank date (L1044); the released layer treats both as optional and
validates format only when non-empty. **Recommendation: match the released layer
(optional).** Reasons, in order:

1. Parity is the run's stated purpose, and `pkg/hstong/future` is the released
   surface a caller may be migrating from. A v-next method that rejects a request
   the released one accepted is a behavioural regression in the *migration*
   path, which is the path this run exists to make safe.
2. The vendor's own SDK is not a specification of what the Gateway requires. It is
   one vendor's client-side convenience check. `hs_open_api.py:828` also declares
   `page_no` and `page_size` as required positional arguments while documenting
   both as having Gateway defaults — a signature that contradicts its own docstring
   is evidence of a strict client, not of a strict server.
3. The cost asymmetry is the ADR 0003 argument in miniature: sending a blank date
   range is rejected by the Gateway with a clear message and no side effect, while
   refusing a legitimate unbounded history query locally breaks a caller.

Record it as a deliberate divergence, not an oversight. **A G6 test settles it**
(§12).

No `stockCode` filter. The vendor offers none, so a futures history query cannot
be narrowed to one contract. That is a capability gap a caller will notice; it is
a vendor limitation, not an SDK choice, and the GoDoc should say so.

### 4.2 The three mutations

#### `FuturesEntrust` → `futuresEntrustWireRequest`

| Go field | JSON tag | Go type | Req/Opt | Why |
|---|---|---|---|---|
| `StockCode` | `stockCode` | `string` | **required**, non-empty, no whitespace | Python L762; Java `FuturesEntrustParam extends FuturesCodeParam`. |
| `EntrustType` | `entrustType` | `string` | **required** | Python L763; Java L934. No Go enum type exists for it — see §9.2. |
| `EntrustPrice` | `entrustPrice` | `string` | **required**, non-negative decimal | Python L764; Java L935 (`isAnyBlank`). **No `omitempty`.** The vendor sends it unconditionally and the guard requires it non-blank, with no market-order exemption. |
| `EntrustAmount` | `entrustAmount` | `string` | **required**, positive decimal | Python L765; Java L935. |
| `EntrustBS` | `entrustBs` | `string` | **required** | Python L766; Java L936. See §9.1 — the Go type is contested. |
| `ValidTimeType` | `validTimeType` | `string` | **required** | Python L767 (positional, no default); Java L936. No `omitempty`. |
| `ValidTime` | `validTime` | `string` | **conditional** — required iff `validTimeType == "4"` | Python L770-771 inserts the key only `if valid_time is not None`; Java L940-942 enforces the coupling. Both vendors document it: "当 valid_time_type==4 必填yyyyMMdd". The only genuinely optional field on this request. |
| `OrderOptions` | `orderOptions` | `string` | **required**, default `"0"` | Python L768 — always in the dict, defaulted to `FuturesOrderOptions.DEFAULT` = `"0"` by the signature. Java carries it with no guard. The field is always sent, so **no `omitempty`**, and C3 must default it to `"0"` or a caller who leaves it empty sends `""` and gets a Gateway rejection. |

Field order in the struct follows the Python dict (`stockCode`, `entrustType`,
`entrustPrice`, `entrustAmount`, `entrustBs`, `validTimeType`, `orderOptions`,
`validTime` last) — cosmetic, since JSON objects are unordered, but it matches
both vendors and costs nothing.

#### `FuturesCancelEntrust` → `futuresCancelEntrustWireRequest`

| Go field | JSON tag | Go type | Req/Opt | Why |
|---|---|---|---|---|
| `EntrustID` | `entrustId` | `string` | **required**, non-blank | Python L782; Java `FuturesEntrustIdParam.entrustId`, guarded (L967). |
| `StockCode` | `stockCode` | `string` | **required** | Inherited from `FuturesCodeParam`; Python L783; Java L967. |

Two fields, both required, no `omitempty`. This is the smallest request body in
the SDK and there is nothing to infer about it.

#### `FuturesModifyEntrust` → `futuresModifyEntrustWireRequest`

| Go field | JSON tag | Go type | Req/Opt | Why |
|---|---|---|---|---|
| `EntrustID` | `entrustId` | `string` | **required**, non-blank | Python L808; Java `FuturesChangeEntrustParam extends FuturesEntrustIdParam`, guarded (L993). |
| `StockCode` | `stockCode` | `string` | **required** | Python L809; Java L993. |
| `EntrustPrice` | `entrustPrice` | `string` | **required**, non-negative decimal | Python L810; Java carries it with no blank-guard. **Requiredness is a judgement** — see §4.3. |
| `EntrustAmount` | `entrustAmount` | `string` | **required**, positive decimal | Python L811; Java no guard. **Requiredness is a judgement** — §4.3. |
| `EntrustBS` | `entrustBs` | `string` | **required** | Python L812; Java no guard. **Requiredness is a judgement** — §4.3. |
| `ValidTimeType` | `validTimeType` | `string` | **required** | Python L813 (positional); Java L997 dereferences it unguarded, so null NPEs. |
| `ValidTime` | `validTime` | `string` | **conditional** on `"4"` | Python L816-817; Java L997-999. |
| `OrderOptions` | `orderOptions` | `string` | **required**, default `"0"` | Python L814. |

**`entrustType` is deliberately absent from this request.** Both vendors omit it:
the Python dict at L807-815 has no such key and `FuturesChangeEntrustParam` has no
such field, while `FuturesEntrustParam` does. A symmetry argument would add it
and would be wrong. Recorded explicitly because it is the one field a
"the modify body mirrors the entrust body" reading gets wrong, and because a
`change`-that-retypes-an-order is a strictly more powerful operation than the
vendor's endpoint offers.

**`stockCode` is required even though the order id is globally unique.** The
vendor requires it (L993) and the released layer requires it. Keep it.

### 4.3 Where this note departs from the released layer, and why

Two deliberate divergences, both in the same direction: **the vendor's requiredness
is adopted for the three mutations, the released layer's is retained for the
history dates.**

| Field | Released | This note | Rationale |
|-------|----------|-----------|-----------|
| `Entrust.EntrustType` | `omitempty`, unset allowed | required | The vendor refuses to send an entrust without it (L934) and Python makes it positional. An absent `entrustType` is a request the Gateway cannot classify. The one released behaviour that survives: `entrustType == "2"` waives the price — §4.4. |
| `Entrust.EntrustPrice` | `omitempty`; waived for `entrustType == "2"` | required, **no waiver** | Python L764 always sends it and Java L935 requires it non-blank. A market futures order carrying `"entrustPrice": ""` is a request whose meaning depends on the Gateway's tolerance for an empty price, and there is no vendor statement that it is tolerated. **This is the one place where being stricter than the released layer could reject an order a caller believed valid** — see the caveat below. |
| `Entrust.ValidTimeType` | `omitempty` | required | Java L936, Python L747. |
| `Entrust.OrderOptions` | `omitempty` | required, default `"0"` | Python L768 always sends it. The risk here is one-directional and mechanical: with `omitempty` and an unset field, the key is *absent*; without it, the key is `""`. `""` is a worse default than `"0"`, so C3 must substitute `"0"` at the mapper boundary. |
| `Modify.EntrustPrice` / `EntrustAmount` / `EntrustBS` | required | required (**unchanged**) | The vendor's guard does not require them, but both vendors *send* them and the released layer rejects a missing one. Keeping the released requirement is both the parity choice and the safer one: it is exactly A8's fail-closed argument over an order mutation. |
| `History.StartDate` / `EndDate` | optional | optional (**unchanged**) | §4.1. |

**The caveat on `entrustPrice`.** Requiring a price on a market order is stricter
than the released layer, and the cost if I am wrong is a *rejected order*, not a
corrupted one. That is the good direction for a financial SDK, but it is a
behaviour change on a mutation and it should be reviewed as one. Two ways out,
both acceptable: (a) keep it required and let a market-order caller pass `"0"`,
which is a well-formed non-negative decimal and is the conventional market-order
price; (b) keep the released waiver and record the divergence. **Recommendation:
(a)**, because it needs no special case in the validation path and it removes the
one judgement in this note that could reject a legitimate order. A G6 request
settles it (§12).

## 5. Per-field provenance

Labelling rule, chosen so the label names **the weakest load-bearing claim** about
the field:

- **derived** — the vendor evidence determines the name, the JSON tag, *and* the
  requiredness, or determines that the field is absent.
- **inferred** — the name and tag are vendor-confirmed, but **requiredness,
  `omitempty`, the Go type, or the enum set is a judgement** the vendor evidence
  does not settle.
- **unknown** — not determinable from this repository.

24 distinct request fields across 8 wire structs; 4 routes have no body.

| # | Struct.field | Label | Basis |
|---|--------------|-------|-------|
| 1 | `futuresProductInfoWireRequest.StockCodes` | **derived** | Python L736-738, Java `FuturesCodeListParam`, guard L507. All three agree, including requiredness. |
| 2 | `futuresMaxBuySellAmountWireRequest.StockCode` | **derived** | Python L725-727, Java `FuturesCodeParam`, guard L481. |
| 3 | `futuresPageQueryWireRequest.PageNo` | **derived** | Python L837, Java `FuturesPageQueryParam`, guard L1045. Requiredness: the vendor requires it, and the released layer also always sends it (default 1). No disagreement to resolve. |
| 4 | `futuresPageQueryWireRequest.PageSize` | **derived** | Python L838, Java, guard L1045. The `<100` upper bound is a released-layer constraint, consistently applied, and the vendor's own default (20) matches. |
| 5 | `futuresPageQueryWireRequest.StartDate` | **inferred** | Name/tag derived (Python L839, Java). **Requiredness contested**: vendor rejects blank (L1044), released treats as optional. Resolved in §4.1 in favour of the released layer. |
| 6 | `futuresPageQueryWireRequest.EndDate` | **inferred** | As #5 (Python L840). |
| 7 | `futuresEntrustWireRequest.StockCode` | **derived** | Python L762, Java, guard L934. |
| 8 | `futuresEntrustWireRequest.EntrustType` | **inferred** | Name/tag derived. **Requiredness + `omitempty` + enum set contested**: vendor requires (L934); released `omitempty`; Java's enum has 4 values and Python's has 3 (§9.2). |
| 9 | `futuresEntrustWireRequest.EntrustPrice` | **inferred** | Name/tag derived. **Requiredness contested**: vendor requires unconditionally (L935) and always sends (L764); released waives for a market order. §4.3/§4.4. |
| 10 | `futuresEntrustWireRequest.EntrustAmount` | **derived** | Python L765, Java L935, released. All three require a positive decimal. |
| 11 | `futuresEntrustWireRequest.EntrustBS` | **inferred** | Name/tag derived. **Go type and set contested** — vendor's futures enum is `{1,2}`, SPEC §7.4 and the released layer say `{1,2,3,4}`. §9.1. |
| 12 | `futuresEntrustWireRequest.ValidTimeType` | **inferred** | Name/tag derived. **Requiredness + `omitempty` contested**: vendor requires (L936 / Python L747); released `omitempty`. Set `{0..4}` is agreed by both vendors, so the *set* is derived. |
| 13 | `futuresEntrustWireRequest.ValidTime` | **derived** | Python L770-771 conditional insert, Java L940-942 enforcement, both docstrings. The coupling is documented and implemented on both sides. |
| 14 | `futuresEntrustWireRequest.OrderOptions` | **inferred** | Name/tag derived. **`omitempty` contested**: vendor always sends (L768); released `omitempty`. Set `{0,1}` agreed. |
| 15 | `futuresCancelEntrustWireRequest.EntrustID` | **derived** | Python L782, Java `FuturesEntrustIdParam`, guard L967, released. |
| 16 | `futuresCancelEntrustWireRequest.StockCode` | **derived** | Python L783, inherited `FuturesCodeParam`, guard L967, released. |
| 17 | `futuresModifyEntrustWireRequest.EntrustID` | **derived** | Python L808, Java `FuturesChangeEntrustParam`, guard L993, released. |
| 18 | `futuresModifyEntrustWireRequest.StockCode` | **derived** | Python L809, Java L993, released. |
| 19 | `futuresModifyEntrustWireRequest.EntrustPrice` | **inferred** | Name/tag derived. **Requiredness contested**: vendor's guard (L993) does not require it; the released layer does. Resolved toward the released layer (§4.3). |
| 20 | `futuresModifyEntrustWireRequest.EntrustAmount` | **inferred** | As #19 (Python L811). |
| 21 | `futuresModifyEntrustWireRequest.EntrustBS` | **inferred** | As #19, plus the §9.1 set conflict. |
| 22 | `futuresModifyEntrustWireRequest.ValidTimeType` | **inferred** | Name/tag derived. **Requiredness contested**: Java L997 dereferences it unguarded (a null NPEs) while the released layer makes it `omitempty`. |
| 23 | `futuresModifyEntrustWireRequest.ValidTime` | **derived** | Python L816-817, Java L997-999. |
| 24 | `futuresModifyEntrustWireRequest.OrderOptions` | **inferred** | Name/tag derived. `omitempty` contested (Python L814 always sends). |

**Totals: 12 derived, 12 inferred, 0 unknown.**

Reframed by what can actually be wrong:

| Axis | Result |
|------|--------|
| Field name + JSON tag correct | **24 / 24** — two independent vendor SDKs plus the released implementation |
| Go type correct | 22 / 24 — the two exceptions are `EntrustBS` and `Modify.EntrustBS` (§9.1) |
| Requiredness correct | 12 / 24 settled by vendor agreement; 12 are judgements, all recorded above with the competing alternative stated |
| Enum set correct | 3 / 4 families settled (`validTimeType`, `orderOptions`, and `entrustBs`-pending); `entrustType` **cannot** be settled |

**Zero unknowns is the headline, and it is only true because the vendor SDKs are
in the repository root.** Had the brief's premise been accurate — no released
futures, no request-schema source — the honest answer would have been 24 inferred
and 0 derived, and this task would have been a request for a vendor document
rather than a design. It was not, and the archives had been sitting there since
before `v0.1.0`.

## 6. Go type shapes and the `omitempty` rule

### 6.1 `omitempty` — the rule, and where futures sits

`pkg/services` is inconsistent about `omitempty` today, and B4 proved both
directions reachable. Reading the three account structs:

- `marginFundInfoWireRequest.ExchangeType` — **no** `omitempty`. Required: the
  released layer rejects an empty market (A8's `validateExchange`), so the key is
  always emitted and the emptiness is a caller error, not a wire state.
- `holdsListWireRequest.ExchangeType` — **`omitempty`**. Optional: the released
  layer accepts an absent market and the Gateway defaults it.
- `realFundJourListWireRequest` — **no** `omitempty` on any field, including
  `QueryParamStr`, which is legitimately `""` on the first page. The key is
  emitted empty rather than omitted, and B4 pinned the present-but-empty
  behaviour on the wire.

**The rule, which futures follows exactly:**

> **`omitempty` is present if and only if the released surface treats the field as
> genuinely optional, so that its absence is a legitimate wire state. A field that
> is merely often-empty — a cursor on the first page, a defaulted page counter —
> is emitted, because the Gateway's own default and an omitted key are not
> guaranteed to be the same request.**

Applied to futures, that gives a clean split:

| Wire field | `omitempty` | Reason |
|-----------|------------|--------|
| `futuresEntrustWireRequest.ValidTime` | **yes** | Absent unless `validTimeType == "4"`. Python omits the key; the Gateway's contract is a conditional field. |
| `futuresPageQueryWireRequest.StartDate` / `EndDate` | **yes** | Optional in the released layer, so absence is legitimate. |
| everything else, all 21 fields | **no** | Required, or defaulted (see below) so a zero value never reaches the wire. |

**`PageNo` and `PageSize` are the interesting case, and the released layer's
`omitempty` on them is a bug-shaped choice that v-next should not copy.** With
`omitempty` and a caller-supplied `PageNo: 0`, the key vanishes — which happens to
be what the released `historyParams` wants, because it substitutes 1 first. But
that makes the wire body depend on a *pre-substitution* value, so a refactor that
moves the defaulting after the marshal silently changes the request. C3 should
drop `omitempty` from both and let the mapper substitute `1` and the service's
default page size, exactly as `condOrderListWireRequest` (`trading.go:756-761`)
does for `pageNo` / `pageSize` — that struct has no `omitempty` on either. That
is the v-next house pattern and it is the safer one. Note the divergence from
released is *observationally* nil (same bytes for every valid request) and that
makes it a safe change; say so in the GoDoc so a future reader does not "fix" it
back.

Two defaults belong at the mapper boundary, not in the caller-facing struct:
`PageNo → 1` and `OrderOptions → "0"`. Both are vendor-documented Gateway
defaults (Python docstrings "默认1", "默认20", and the `FuturesOrderOptions.DEFAULT`
signature default). `pkg/services` has no `WithDefaultPageSize` option and
futures should not grow one — the released `future.WithDefaultPageSize` is a
`pkg/hstong` surface, outside ADR 0011 and outside the v-next surface, and
reproducing an option in v-next creates two places to change the default.

### 6.2 Two structs, not four

`FuturesQueryHistoryEntrustList` and `FuturesQueryHistoryDeliverList` share one
wire struct, because the vendor shares one parameter class and one Python
signature between them. The cash layer's duplicated-but-identical
`historyEntrustListWireRequest` / `historyDeliverListWireRequest`
(`trading.go:770-784`) is a duplication this task has no reason to reproduce.
One struct, two methods, and a comment recording the vendor's sharing as the
reason — so the next reader does not "fix" the apparent asymmetry.

### 6.3 Pagination type

`transport.Pagination` is cursor-shaped (`Cursor`, `PageSize`, `HasMore`, and an
`Apply` that writes `cursor` / `page_size`). Futures is page-number-shaped and its
wire keys are `pageNo` / `pageSize`. `transport` is outside ADR 0011's protected
surface so it may grow, but the two meanings in one struct is a worse outcome
than a second type.

**Recommendation: a futures-local page request, not a change to
`transport.Pagination`.**

```go
// PageRequest is the page-number pagination the two futures history queries
// take. It is deliberately not transport.Pagination: the futures Gateway takes
// pageNo/pageSize and has no cursor, so a cursor field would be dead weight and
// a shared struct would carry two incompatible meanings.
type PageRequest struct {
    PageNo    int
    PageSize  int
    StartDate string
    EndDate   string
}
```

This also keeps the failure mode local: when P5's `TickSchedule` work eventually
touches `transport`, futures is not a second consumer of a type whose semantics
just changed.

## 7. Account identification

### 7.1 No futures request carries an account id, and none can

The brief's question — cash `HoldsList` takes an `accountID` while
`QueryHoldsListResponse` carries `fundInfo` + `holdsList` and no account field, so
how is a futures account identified on the wire? — resolves cleanly, and the
answer is that **the question does not arise**: the futures account is identified
by the authenticated session, and the wire body has no account field at all.

Evidence, which is complete rather than partial:

- `FuturesCodeParam`, the base class of every futures param type that carries
  anything, has exactly **one** field: `protected String stockCode`. There is no
  `accountId`, `userId`, `clientId`, or `subAccountId` in any
  `sdk/vo/futures/*Param.java`.
- The four account-scoped reads pass `Maps.newHashMap()` / `{}` — no params.
- No futures route has a market field either (§2.3), so there is not even a
  book-selection key. The account *and* the book are both resolved from the
  session.
- The released layer agrees exactly: `future.Manager` holds no account state, and
  `future.go:98` documents that trading requires a live session established by
  `hstong.SessionManager` before any method is called.

**The cash asymmetry is explained, and it explains itself.** Cash `HoldsList`
sends `exchangeType` because one cash account holds positions in HK, US, and both
Connect books, and the Gateway cannot guess which one was meant. A futures account
holds positions keyed by contract code, and the response says which book each one
belongs to via `Hold.dataType` (SPEC L353: "distinguishes HK from US futures").
So futures needs no market *because it can read the market back*, and needs no
account *because the session supplies it*. The absence is a property of the
protocol, not an omission in either SDK.

### 7.2 Method signatures

`accountID` is a **session key and a local precondition only** — it never crosses
the wire. This is the established v-next pattern: all 17 cash methods take it
first, all of them reject a zero value, and none of them puts it in a wire struct.

```go
// Reads
func (s *FuturesService) QueryProductInfo(ctx context.Context, accountID domain.AccountID, codes []string) ([]*domain.FuturesProduct, error)
func (s *FuturesService) QueryMaxBuySellAmount(ctx context.Context, accountID domain.AccountID, symbol domain.Symbol) (*domain.FuturesCapacity, error)
func (s *FuturesService) QueryFundInfo(ctx context.Context, accountID domain.AccountID) (*domain.FuturesAccount, error)
func (s *FuturesService) QueryHoldsList(ctx context.Context, accountID domain.AccountID) ([]*domain.FuturesPosition, error)
func (s *FuturesService) QueryRealEntrustList(ctx context.Context, accountID domain.AccountID) ([]*domain.FuturesOrder, error)
func (s *FuturesService) QueryRealEntrustPage(ctx context.Context, accountID domain.AccountID, page PageRequest) (*domain.FuturesOrderPage, error)
func (s *FuturesService) QueryRealDeliverList(ctx context.Context, accountID domain.AccountID) ([]*domain.FuturesFill, error)
func (s *FuturesService) QueryRealDeliverPage(ctx context.Context, accountID domain.AccountID, page PageRequest) (*domain.FuturesFillPage, error)

// Mutations
func (s *FuturesService) Entrust(ctx context.Context, accountID domain.AccountID, order FuturesOrderRequest) (*domain.OrderResult, error)
func (s *FuturesService) CancelEntrust(ctx context.Context, accountID domain.AccountID, entrustID domain.EntrustID, symbol domain.Symbol) error
func (s *FuturesService) ModifyEntrust(ctx context.Context, accountID domain.AccountID, change FuturesModifyRequest) error
```

> **Correction, found while implementing C3: `FuturesOrderRequest` and
> `FuturesModifyRequest` belong in `pkg/services`, not `pkg/domain`, so the
> `domain.`-qualified forms this section originally wrote would not compile.**
> The package split is the cash layer's, and the cash layer is unambiguous: every
> caller-facing request wrapper lives in `pkg/services` — `MarginFundInfoRequest`,
> `HoldsFilter`, `FundJourFilter`, `RealFundJourListRequest` in `account.go`,
> `EntrustFilter` in `trading.go` — while `pkg/domain` holds the domain models and
> the exported `…Wire` row DTOs the mappers consume, and nothing else. A request type
> is a caller's construct, and the layer that owns a request is the layer that
> decodes its reply, so a request type in `pkg/domain` would be a type with no
> decoder in its own package. The names are unchanged from what was written here, so
> what a later move costs is a qualifier rather than a rename. **C4 and C5 are
> written against the three corrected signatures above.**

Four points C3 should get right, each of which a careless implementation gets
wrong:

1. **`accountID` is validated and then unused on the wire.** The `IsZero` check is
   not dead code — it is the only place the SDK enforces that the caller named an
   account, and it is what the cash layer does. But the GoDoc must say plainly
   that `accountID` selects the session and appears in no request body, or a
   future maintainer will "fix" the unused parameter by adding it to the wire
   struct. That single change would be a leak of the account identifier into every
   futures request body and would be invisible on the mock.
2. **No `…Filter` type for the four no-param reads.** The v-next layer's
   convention is `HoldsFilter` / `EntrustFilter` / `DeliverFilter` because cash
   needs `exchangeType` and (for the cond-order and history lists) pagination.
   Futures has none of those. Inventing an empty `FuturesHoldsFilter` would be a
   type that carries no information. §4.1's four methods take no request
   parameter at all.
3. **The two real-list methods return slices, the two history methods return
   pages.** The real lists are unpaginated on the wire (`{}` in, at most 500 /
   200 rows out), so they have no page state to return; the history lists are
   page-numbered and carry `lastPage` / `curPageNo` / `curPageSize`, so they do.
   This is a genuine difference from the cash layer, where the real lists *are*
   cursor-paginated and `RealEntrustList` returns a slice after walking pages
   internally via `walkFundJourPages`. Futures must **not** walk pages: there are
   no cursors, and one call returns the whole day.
4. **`EntrustListResponse` and `DeliverListResponse` are distinct types with
   identical bodies in SPEC, and `Hold.dataType` makes `QueryHoldsList` a two-part
   response.** C3 should return `(*[]Position, *Account, error)` or a small
   struct for the holds query, not flatten it — the funds snapshot and the
   positions are one atomic read and discarding either is a loss.

### 7.3 What C13 has to add, and the one open question

`accountID` is first-class in these signatures from day one precisely so C13 can
insert `internal/auth` without a breaking change: every method then has an account
to resolve a session for, and `authenticator.MustBeAuthenticated(ctx, accountID)`
(`internal/auth/authenticator.go:63`) drops in at the top of each body. C3 should
leave a call site for it and not a stub — an unexported `s.mustAuth(ctx, accountID)`
that returns `nil` until C13, so C13's change is one function body, not eleven.

**Open question, recorded rather than guessed: does futures need its own
`TradeLogin`?** Status `20033` is "futures trade login timeout"
(SPEC L721, `pkg/types/status.go`), and [docs/authentication.md](../../authentication.md)
L77 tells the caller to `session.Login(ctx)` for futures — a futures-specific
remedy for a futures-specific code, which is weak evidence of a separate session.
Against it: neither vendor SDK exposes a futures login. All eleven futures
routes are ordinary `/trade/Futures*` HTTP calls on the same single
`TradeLogin`-authenticated session, and the Java handle holds one
`Application`/session for everything. **This note does not invent a
`FuturesLogin`, and C3 must not either.** One live request settles it (§12).

## 8. The three mutations under ADR 0003

### 8.1 The guarantee is already structural — C5's job is a test, not code

The three futures paths are **already in the closed mutation set** in
`internal/resilience/retry.go:43-45`:

```go
"/trade/FuturesEntrust",
"/trade/FuturesCancelEntrust",
"/trade/FuturesModifyEntrust",
```

and `IsMutation` (`retry.go:88-91`) strips both Gateway alias suffixes before
lookup, so a caller cannot reclassify a futures mutation as a retryable query by
addressing `/trade/FuturesEntrustRequestMsgType`. `Policy.DoRoute` then issues a
`ClassMutation` operation exactly once regardless of `MaxAttempts`
(`retry.go:159,176`), and the alias stripping is a closed set a caller cannot
extend.

**So: C5 adds nothing to `internal/resilience`.** What C5 must deliver is the
*proof*, and B3 already wrote it: under `MaxAttempts: 5` with a retryable `1015`,
`trading_adr0003_test.go` shows all four cash mutations issuing exactly one
request while a read-only query under the identical client takes all five — the
mandatory control, because a single-attempt result on a client whose policy is
inert proves nothing. C5 needs the same shape for the three futures mutations,
and should also carry B3's unanticipated case forward: **a mutation that is
locally rejected and never sent still counts as one attempt** (it must not be
counted as zero, or a "one request" assertion becomes vacuous for the wrong
reason).

### 8.2 What the SDK must guarantee

1. **Exactly one attempt, at every configuration.** No `Option` can raise it. This
   is `IsMutation` + `ClassMutation` and is already true; C5 tests it rather than
   implementing it.
2. **No pre-flight request.** All validation is local and precedes the call, so a
   bad request is rejected with a typed `errs.Error` at `types.StatusInvalidParam`
   and **zero** HTTP requests. §9's fail-closed rules are what make this true for
   the enumerated fields. B-series established the cost of getting this wrong: a
   service method with no error-arm test leaves its whole failure surface dark.
3. **Reconciliation is the caller's, and the SDK must say so at every one of the
   three.** On an ambiguous failure the returned error must be typed and
   recoverable, and the GoDoc on all three methods must name the reconciliation
   path: `QueryRealEntrustList` / `QueryHistoryEntrustList` /
   `QueryRealDeliverList` for a submit or modify, and additionally
   `EntrustOrder.canBeCanceled` / `canBeUpdated` to tell a pending modify from a
   live order. ADR 0003 requires the typed error; the released layer's
   `future.go:477-480, 509-512, 530-533` already carries this prose on all three
   and the v-next version must too.
4. **A modify is not idempotent in the way a caller may assume.** See §8.4.
5. **The mutation breaker is separate.** `client.WithMutationBreaker`
   ([docs/configuration.md](../../configuration.md)) exists so a storm of rejected
   mutations cannot starve reads. C5 should not reimplement breaker selection; the
   route path already selects the class.

### 8.3 What cancel and modify must require

| Method | Must require | Must **not** require |
|--------|--------------|---------------------|
| `CancelEntrust` | `entrustId` non-blank, `stockCode` non-blank and whitespace-free. Nothing else exists on the wire to require. | Nothing. The two-field body is fully determined. |
| `ModifyEntrust` | `entrustId` non-blank; `stockCode` non-blank; `entrustPrice` a well-formed non-negative decimal; `entrustAmount` a strictly positive decimal; `entrustBs` in the admissible set; `validTimeType` in the admissible set; `validTimeType == "4"` ⇒ a real `yyyyMMdd` `validTime`, and any supplied `validTime` format-validated. | `entrustType`. The vendor's modify body has no such field (§4.2) and the v-next request type must not grow one. |

The `ModifyEntrust` requirements beyond the vendor's own guard (L993, which checks
only `stockCode` + `entrustId`) are the released layer's, retained deliberately
(§4.3). They are also exactly A8's argument applied to futures: a modify that
silently forwards a non-positive quantity or an out-of-set direction is a live
order mutation issued once, with no retry and no second chance.

### 8.4 Modify versus cancel-then-entrust — the difference that matters to a caller

They are **not** interchangeable, and the difference is the reason
`FuturesModifyEntrust` exists at all:

| | `CancelEntrust` + `Entrust` | `FuturesModifyEntrust` |
|---|---|---|
| Requests on the wire | **2** | **1** |
| Exposure window | The old order is cancelled and the new one does not exist. Between the two, the position is flat. A competing order, a margin call, or a limit-move fills the flat window. | The order never leaves the book. It reprices in place. |
| If the second call fails | You are flat and you may not know it. | You still hold the original order, and you may not know it. |
| Priority / time-in-force | Resets. The new order goes to the back of the queue and a modified order does not. | Preserved. |
| Fill history | A cancel/replace is recorded as a cancel plus a new order; `EntrustListResponse` shows two rows. | One row, updated in place. |
| `EntrustType` | Re-sent on the new order. | **Not sent at all** — the vendor's modify body has no such field (§4.2), so the order keeps its original type. |
| Under ADR 0003 | **Two** unreconciled mutations. If the cancel succeeded and the entrust timed out, the SDK cannot tell you which, and the caller holds a flat position they may not know about. | **One** unreconciled mutation, and the failure mode is bounded: worst case the modification did not apply and the original order stands. |

**The last row is the argument.** Under ADR 0003, replacing a mutation that has a
bounded failure mode with two that do not is a downgrade, and a caller who does it
by hand inherits a reconciliation problem the `FuturesModifyEntrust` path does not
have. The GoDoc on `ModifyEntrust` should say so, because the SDK otherwise looks
like it is offering a convenience over a cancel-plus-entrust that a caller could
compose themselves. It is not a convenience; it is a strictly safer sequence, and
the reason is ADR 0003.

The corollary, which C5's GoDoc must also state: **a modify is not a
price-increase and a cancel is not a failure.** A caller who wants a new price must
choose between them deliberately, and a modify whose result is ambiguous leaves
an order whose current price is unknown — so the reconciliation query is
`QueryRealEntrustList`, and `EntrustOrder.entrustPrice` is the field to read.

## 9. Enumerated fields, and what can be bounded

A6 ([design-a6-exchange-validation.md](./design-a6-exchange-validation.md)) and A8
([design-a8-trade-validation.md](./design-a8-trade-validation.md)) settled the
precedent: the v-next layer validates enumerated codes fail-closed, because a
silently-accepted bad code becomes a rejected or misrouted order at the exchange,
issued once, with no retry. The futures case is *easier* than A6's in one respect —
the vendor ships the enums — and *harder* in one respect, because two of the four
families are contradicted across sources.

### 9.1 The four families

| Field | Admissible values | Source | Boundable? |
|-------|-------------------|--------|------------|
| `validTimeType` | `0` today · `1` deal-and-cancel · `2` full-or-cancel · `3` valid-on-expiry-date · `4` valid-on-specified-date | Java `FuturesValidTimeType` (5 rows, no non-exhaustive note); Python `futures_constant.FuturesValidTimeType` (same 5); released `future.go:431-432` (same 5) | **Yes. Closed set of 5, three independent sources agree.** Fail closed. |
| `orderOptions` | `0` default · `1` T+1 | Java `FuturesOrderOptions`; Python (same 2); released `future.go:436-437`; and the *response* `EntrustOrder.orderOptions` is `int32` "0 default or 1 T+1" (SPEC L384), which independently corroborates | **Yes. Closed set of 2.** Fail closed. |
| `entrustBs` | **`1`, `2`** per the vendor's futures enum; **`1`, `2`, `3`, `4`** per SPEC §7.4 and the released layer | Java `FuturesEntrustBs` = `{BUY "1", SELL "2"}`; Python `FuturesEntrustBs` = `{1, 2}`; **but** the same SDK's *cash* `trade/EntrustBs` = `{1,2,3,4}`, SPEC §7.4 `entrustBs` = 4 rows with no non-exhaustive note, and the released layer accepts 1–4 | **No — genuinely unresolved. See below.** |
| `entrustType` | Java `{0, 1, 2, 3=OPTION}`; Python `{0, 1, 2}` | Java `FuturesEntrustType` has four; Python `futures_constant.FuturesEntrustType` has three and its docstring says "0 限价 1 竞价 2 市价" | **No — the two vendor SDKs contradict each other.** |

### 9.2 `entrustType` cannot be bounded, so it is not bounded

Java's `FuturesEntrustType` declares `OPTION("3")`. Python's does not, and its
docstring enumerates exactly three. Both are vendor SDK **v2.3.0**, shipped the
same day. This is precisely the `targetStrategy` situation A6 refused to guess
at: "the SDK cannot decide which values are correct, so a local closed set would
reject requests the vendor's own documentation endorses" — except that here the
SDK *has* the vendor's documentation and it says two different things.

**Decision: `entrustType` gets no set validation in the v-next layer.** It gets
(1) required-ness (§4.2), (2) a well-formedness check — a non-empty string of
decimal digits — and (3) a GoDoc that names the conflict and points at G6. The
released layer validates it not at all, so this is parity-safe. Inventing `{0,1,2}`
would be inventing a range the brief explicitly forbids, and would lock a caller
out of code `3` if `OPTION` is real.

The same applies to the Go type: **there is no `types.FuturesEntrustType`.**
`pkg/types` is in ADR 0011's protected surface, and adding a closed enum there
would be a public-surface change for a set this note cannot establish. A `string`
field, validated only for digit-ness, is the honest type. If G6 later resolves the
conflict, a `FuturesEntrustType` becomes a *non-breaking* addition (a named
string type is assignable from an untyped constant, but not from a `string`
variable — so it is a breaking change for a caller holding a `string`, and must
be weighed then, not now).

### 9.3 `entrustBs` is unresolved, and the released layer's choice is the better one

The conflict is real: the vendor's **futures** enum has two values, the shared
**cash** enum and SPEC §7.4 have four. Note the direction of the evidence — a
futures order is a single-leg position instruction on one contract, so
"buy"/"sell" (1/2) is semantically sufficient and `3`/`4` (close short / open
short) belong to the *cash* margin/short vocabulary. That reading says the two-value
futures enum is the correct one and the released layer's 1–4 acceptance is a
carry-over from the cash type.

Against that: SPEC §3's own futures row says `entrustBs` is "Direction: 1 buy, 2
sell" — two values, corroborating the vendor. And the v0.1.x SDK has shipped
`{1,2,3,4}` acceptance since `v0.1.0`, so a caller may be using `3`.

**Decision: keep the released layer's `{1,2,3,4}` and do not narrow it.** Three
reasons, and the first is decisive:

1. **Narrowing would be a silent behaviour change on a mutation that a caller
   depends on.** A caller who has been sending `entrustBs: "4"` on a futures
   `Entrust` has, for seven releases, received whatever the Gateway does with it.
   Rejecting it locally in v-next is the ADR 0011 "bug fix that changes runtime
   behaviour" category at best and a caller-visible regression at worst — on a
   money-moving call, decided on incomplete evidence.
2. **The failure mode of being permissive is a Gateway rejection, not a
   misrouted order.** A6's cost asymmetry applies with the same force: an
   out-of-market `entrustBs` is refused by the Gateway with a clear message and no
   side effect, whereas a fail-closed rejection refuses a request the vendor's
   shared dictionary endorses.
3. **The Go type question is separable and more important.** The field should be
   `types.EntrustBS`, not `string`, so the four-value set is stated once in
   `pkg/types` instead of twice in futures. That gives the fail-closed check
   without a second source of truth, which is the A6 argument for unexported
   `validDirection` helpers applied to the v-next layer.

**Record the conflict in the GoDoc of both futures methods**, because it is
exactly the kind of thing a reader needs to know and cannot discover: the SDK
validates against the cash four-value set because the vendor's futures enum says
two, and G6 is the test. C5 should write a test that *pins* `{3, 4}` as accepted,
in the spirit of A6's `TestEveryDocumentedCodeIsAccepted`, so a future narrowing
attempt fails loudly rather than quietly.

### 9.4 What is not an enumerated field here

- **`exchangeType` — absent from every futures request.** No futures request
  carries a market, so A6's and A8's `validExchange` work has no futures
  counterpart and C5 should not add one. This is the single most likely
  "consistency" mistake in C3–C5 and it is worth stating in the code as a
  comment on the service, not just here.
- **`moneyType` — absent.** `Hold.ccy` and `QueryMaxBuySellAmountResponse.ccy`
  are free-text currency codes (`"HKD"` in the fixture at
  `test/mockgateway/fixtures.go:281`), not the `0/1/2` `moneyType` code. A
  `types.MoneyType` on a futures field would be a category error. A currency
  string has no admissible set this repository can bound; it is a `Money`'s
  currency, not a validated enum.
- **`stockCode`** — not enumerable. Keep the released `validateStockCode`
  (non-empty, no internal whitespace, deliberately permissive about characters so
  `"HSI2603"` and `"01810.HK"` pass). Do **not** invent a code pattern: the HK
  and US futures code formats differ and neither is documented in this repository.
- **`pageNo` / `pageSize`** — numeric bounds, not enums: `PageNo ≥ 1`,
  `1 ≤ PageSize < 100`. Keep the released `maxPageSizeExclusive`.
- **`startDate` / `endDate`** — format bound: `yyyyMMdd` and a real calendar date,
  per the released `datePattern` + `time.Parse`. Note the response's
  `EntrustOrder.validTime` is documented in `yyyy/MM/dd` form (SPEC L385) while
  the *request* `validTime` is `yyyyMMdd` — two different formats for two
  different directions, and the response one must not be applied to the request.

## 10. Two smaller decisions, confirmed

### 10.1 `maxBuyAmount` / `maxSellAmount` — plain `int64`, `MustNewQuantity` at the boundary. Confirmed.

The brief asks me to confirm the existing v-next pattern
(`pkg/services/market.go:530`: `Volume: domain.MustNewQuantity(fmt.Sprintf("%d", q.Volume))`)
is the right treatment for SPEC's `int64` "integer count, not monetary". **It is,
and here is why, including where the ADR 0007 tension actually sits.**

- **The vendor types them `long`.** `FuturesMaxBuySellAmountVo` declares
  `private long maxBuyAmount; private long maxSellAmount;` — Java `long`, signed
  64-bit. That is the same width as Go `int64` and it is the vendor's own choice
  for a count, which is evidence the value is a count and not a scaled decimal.
- **ADR 0007 is not actually in tension with a plain `int64`.** ADR 0007's rule
  is "*`int64` is a JSON **number**, not a quoted string*" — and a plain Go
  `int64` wire field with `encoding/json` is exactly that. The tension the brief
  senses is different and narrower, and it is worth stating precisely: the
  vendor decodes with **Gson**, and Gson's `LONG` adapter accepts a JSON number
  *and* a quoted string. So `private long` is consistent with either wire
  convention and **discriminates nothing**. Go's `int64` accepts only the number.
  A Gateway that sent `"maxBuyAmount":"10"` would decode cleanly in Java and raise
  `UnmarshalTypeError` in Go. That is the unvalidated inference, and it is
  G6-blocked exactly as ADR 0007 says it is.
- **The failure mode is loud, which is the part that matters here.** This is a
  **response** field, so a mismatch is a decode error on read, not a wrong number
  on a write. A caller gets a typed error, not a silently wrong quantity. Compare
  the mutation case in §8, where a wrong guess is an order.
- **`float64` is the only genuinely unacceptable option, and the reason is
  arithmetic, not style.** A `maxBuyAmount` is a contract count and can be large;
  `float64` cannot represent every integer above 2⁵³, and B2's own fixture work
  found a value (`1e-330`) that is positive as a decimal and **zero to every
  `float64`**. `string` is also wrong: it would accept a non-numeric string that
  the Gateway never sends, silently, and it cannot be range-checked.
- **The existing pattern is already proven on the money path.** B2 and B3 proved
  `json.Number` and the `MustNew*` mappers byte-for-byte, including that
  reintroducing `float64` fails 44 tests and `%.3f` fails 103.

**Recommendation, with one caveat C3 should decide explicitly.** Wire
`MaxBuyAmount int64` / `MaxSellAmount int64` with no `omitempty` — note that
`omitempty` on a response field would be a defect, because a legitimate
"you can buy nothing" `0` would vanish from the decoded struct, and B4's
`Money(0)` discipline is the same point. Map with
`domain.MustNewQuantity(strconv.FormatInt(v, 10))`.

**The caveat is `Must`.** `MustNewQuantity` panics on a non-decimal string, which
`FormatInt` can never produce — so the panic is unreachable from *this* call site.
B1's finding stands for the mappers that take a wire `string`, where a partial
reply panics. Since a `Must` panic is a crash in a caller's process rather than an
error, and since `Quantity` has no currency (a contract count is unitless, so
`MustNewQuantity` is the right constructor and `MustNewMoney` would be wrong), the
recommendation is: **use `MustNewQuantity` here, and note in the service GoDoc
that this mapper cannot panic because its input is a formatted integer.** If C3
prefers a uniform `New…`-returns-error shape across all of `pkg/services`, that is
a defensible consistency argument and it is C3's to make — but it should be made
once for the whole service, not per-field.

### 10.2 `decInPrice` / `priceDecimalPoint` is a third tick source, and it is the best-evidenced one

`ProductInfo.decInPrice` is a **per-product price decimal power** — `0 ⇒ 1`,
`1 ⇒ 0.1`, `2 ⇒ 0.01` — and `priceDecimalPoint` is its string rendering
(`"1"`, `"0.1"`, `"0.01"`). The mock fixture is consistent: `decInPrice: 0` with
`priceDecimalPoint: "1"` for HSI (`test/mockgateway/fixtures.go:257`). Confirmed
by the vendor: `FuturesProductInfoVo` declares `private int decInPrice;` and
`private String priceDecimalPoint;`.

**Recommendation: record `decInPrice` in `design-tick-model.md` as tick source #3,
ranked above `spreadLevel` and below an instrument master, and implement it in the
P5 follow-on as a `TickSchedule` keyed by contract code.** It beats `spreadLevel`
on every axis the tick note cares about:

| | `decInPrice` | `spreadLevel` (from §3 of the tick note) |
|---|---|---|
| What it is | The product's price decimal places — **definitional** | Unresolved: the vendor calls it "最小价格单位" (tick), the fixture cannot discriminate it from a spread, and the note leaves it open |
| Certainty | Three sources agree, including the vendor's own field type | **Open question.** The note declines to resolve it and schedules a G6 test |
| Coverage | Every futures product reachable from one `FuturesQueryProductInfo` call, keyed by contract code | One security per `/hq/OrderBook` call |
| Cost | One call per batch of contract codes | A round trip **per price** — the tick note already rejects this as "tempting but … costs a round trip per price" |
| Scope | Futures only | The one endpoint where the note says the semantics are unknown |

The one thing `decInPrice` does **not** give is a price band. §2.3 of the tick note
is about HKEX's per-band cash tick table, and a futures product's decimal power
says nothing about the band the price sits in. So `decInPrice` is a resolution for
the *futures* leg of the tick model and no help at all for the HK cash band table
that P5 steps 3–4 are blocked on.

**Mechanically, for the tick note's amendment:**

- Add a §2.5: *"A futures product carries its own tick."* One short paragraph,
  the table above, and the `domain.Tick` construction rule.
- The amendment belongs in P5's scope, not in C3. `Tick` does not exist yet
  (§2.4 of the tick note), and adding it now for futures alone would create the
  type twice.
- **`priceDecimalPoint` is the redundant rendering and must not be the source.**
  It is a `string`, and a string invites `ParseFloat` — the exact defect P1 fixed
  across 23 sites. Derive the tick from the `int32` `decInPrice` as
  `1 / 10^decInPrice` on a `decimal.Decimal`, and if `priceDecimalPoint` is
  exposed at all, cross-check it against the derived value and record a mismatch
  rather than preferring it. A `decInPrice` of 0 is a legitimate whole-unit grid
  (HSI, per the fixture), so the derivation must not treat 0 as "unset" — which is
  also why a `Tick` built from it is never the zero tick.
- Add a step to the tick note's §5 list: *"implement the futures `TickSchedule`
  from `decInPrice`"* — which makes it a sixth follow-on step and one more reason
  P5's scope is growing while its owner count is zero.

**Sequencing: does P5 have to precede C3–C6? No — but C3 must not make it worse.**
The tick note's own P5 entry says this is "a judgement call, not a correctness
requirement", and the reasoning holds up under the futures request shapes. What
changes is that the futures exposure is now **narrower** than the tick note
assumed, and I can say by how much:

- Every futures price the Gateway quotes is a multiple of `10^-decInPrice`. If
  `decInPrice ≤ 3` — that is, a 0.001 grid or coarser — then **every futures price
  is a multiple of 0.001**, so the hardcoded tick is a multiple of the real tick and
  `Price.Validate` passes on all of them. The `decInPrice = 0` HSI fixture is the
  extreme case: `25000` against a real tick of `1` and an invented tick of `0.001`.
- The exposure is therefore limited to `decInPrice ≥ 4` instruments — sub-0.001
  grids, which in practice means certain US futures. On those, the failure is
  **loud**: a faithful price is rejected by `validatePriceForHK`'s
  `price.Validate(true)`, which is the outcome the tick note calls "loud, and
  correct". The silent-corruption path is `Price.Round()`, which has no production
  caller.
- And the honest fix — a `TickSchedule` fed by `decInPrice` — is **new code in
  P5's step 3**, not a prerequisite. Sequencing futures behind it would make the
  C-series wait on a `domain.Tick` type and a `TickSchedule` interface that P5
  must build anyway, and P5 steps 3–4 are blocked on a human decision about a
  **cash** HKEX band table that has nothing to do with futures. That is a
  guaranteed stall of the whole C-series on an unrelated human.

**Recommendation: split P5. Do P5a (steps 1–2: the 40 `"0.001"` → `"0"`, plus
`domain.Tick` and the `OrderBookResponse.TickSize` retype) before C3, because
they are mechanical, testable, and unblocked — and P5 steps 3–5 after, where they
belong.** Then add one hard rule to C3's acceptance:

> **C3 must introduce zero new `"0.001"` literals.** The count is checkable today
> (`rg -c '0\.001' -g '*.go'` → 17 in `pkg/services/market.go`, 8 in
> `pkg/transport/mappers.go`, 8 in `internal/push/decode.go`, 4 in
> `pkg/domain/account.go`, 3 in `pkg/domain/trading.go`), and C3 must not raise
> any of the five. C3's futures prices pass the **zero** tick `"0"` — "I observed
> this price; I do not know this product's tick schedule" — which is the tick
> note's §2.1 rule and is correct today, with `decInPrice` arriving later as the
> resolved value.

That rule is cheap, it is one `rg` in review, and it means the C-series neither
depends on P5 nor deepens the debt P5 exists to remove. If the run owner would
rather not split P5, the fallback is C3 with the zero-tick rule and P5 whole,
which is also correct — just leaves 40 literals in place for longer.

## 11. Risk: what a wrong field name costs, and the mitigations

**A wrong field name on a futures mutation is the worst failure mode available in
this task, and it is worth being precise about the mechanism rather than saying
"risky".**

Three things stack:

1. **The Gateway ignores unknown keys.** The envelope is
   `{timeout_sec, params, ok, err, data}` and `params` is a free-form JSON object
   that `encoding/json` decodes field-by-field. A key the Gateway does not
   recognise is dropped. So a typo produces **no error**: the request succeeds,
   the order is placed, and the omitted field takes the Gateway's default.
2. **The default may be the dangerous value.** If `entrustBs` is misspelled, the
   order goes in with a default direction. If `entrustAmount` is misspelled, it
   goes in as a default quantity. If `entrustId` is misspelled on a
   `CancelEntrust`, the cancel names an order the Gateway resolves by its own
   default — which is nothing, or something else.
3. **ADR 0003 removes the safety net.** There is no retry, so there is no second
   attempt that might reveal the problem, and no idempotency key that would make
   a second attempt safe. The caller's only remedy is a manual reconciliation
   query, on an account whose state they may not be able to read yet.

And the mock **cannot catch any of it.** All eleven futures fixtures are static
`b.set(path, body)` responses keyed on path; none inspects a request body
(`test/mockgateway/fixtures.go:255-265`, and `cursorFixture.page` at
`fixtures.go:113-132` reads `queryParamStr`, which futures does not use). A
v-next futures test that asserts "the call succeeded" passes with **every field
name misspelled**, because the mock answers by path. So a green C5/C6 proves
nothing about field names. This is the sharpest form of the risk and it deserves
to be stated in C6's task brief.

**What mitigates it here, and how far each goes:**

| Mitigation | Strength |
|------------|----------|
| Two independent vendor SDKs confirm all 24 names, at the wire-key level in Python | **Strong.** The failure mode requires both vendors and the released layer to be wrong in the same way. |
| Parity with the released `pkg/hstong/future`, which has shipped 11 releases | **Strong.** A wrong name would mean the released SDK has been silently mis-ordering since `v0.1.0`, which is a different and more visible bug. |
| `pkg/services` is unreachable by a caller (D1–D4 wire it) | **Strong for now, and it is the reason the sequencing works.** Nothing depends on a wrong name until D2 exposes the constructor, and D2 is gated on C14 (full parity) and E1–E4 (docs). There is a real window in which a wrong name is a documentation defect rather than a financial one. |
| The parity guard is in report mode (C2) and does not check field names | **Weak, and it is worth being clear about this.** C1's guard is a route-count-and-reference check. It would report "C4 implemented `FuturesQueryFundInfo`" for a body that is `{"foo": "bar"}`. It cannot and should not be extended to field-level checking — that is a mock that would have to model the Gateway, and the mock is the thing that cannot be trusted here. |
| C6 tests would assert on the wire | **Necessary but not sufficient.** B-series established the pattern — a helper that re-types the recorded params to the wire struct, e.g. `tradingParamsAs` applied to `entrustWireRequest` in `trading_happy_test.go:283` — and B4 proved `omitempty` both ways on the wire. A wire assertion catches a *regression*; it cannot catch a *consistent* error, because the test and the code share the same wrong assumption. |

**The honest summary: the mitigation is the vendor evidence, not the test suite.**
The tests keep the code honest against the design; the design is only as good as
its sourcing, and the sourcing here is a wire-level source, which is the strongest
kind this repository contains.

**What to do when G6 unblocks.** Not a broad integration sweep — the smallest set
of requests that would confirm or refute the inferred fields. Ordered by
information per request:

1. **`FuturesQueryHistoryEntrustList` with no dates** (one request). This is the
   single highest-value request in the set. It resolves **two** inferred fields
   (#5, #6 — are the dates required?) *and* confirms that the whole futures request
   family is shaped like the vendor's page-number body and not like the cash cursor
   body. If it succeeds with the dates omitted, §4.1's choice is confirmed. If it
   returns `1016`, the vendor's guard was right and released `future.go` has been
   relying on a Gateway default. Either answer is a one-line change.
2. **`FuturesQueryProductInfo` for one contract code** (one request, already
   reachable today — it is a read, and it needs no order). Confirms #1, and
   answers §10.2 directly: read `decInPrice` and `priceDecimalPoint` for a real
   product and check that `priceDecimalPoint == 10^-decInPrice` as a string. If
   `decInPrice` is 0 for an instrument whose prices have decimals, the "power"
   reading is wrong and the tick note's §2.5 amendment needs rewriting before any
   code depends on it. **This is the one request that should be run first**, because
   it is the cheapest, it is a pure read, and it is the input to a decision that
   three other tasks depend on.
3. **`FuturesEntrust` for a market order with `entrustPrice: "0"`** (one order, in
   a sandbox account, then cancel it). Resolves #9, the only place where this note
   is *stricter* than the released layer. It also confirms the response: SPEC says
   `EntrustResponse.data` is the order number "or empty string when the Gateway
   omitted it", and whether a real Gateway populates it changes how C5's GoDoc
   should tell a caller to reconcile.
4. **`FuturesModifyEntrust` on a resting order** (one order, then cancel it).
   Confirms #19–#22: whether price/amount/`entrustBs` are honoured as sent, and
   whether omitting `entrustType` really preserves the original order type (§8.4).
   This is the only way to confirm that a modify keeps its priority, which is the
   entire argument for preferring modify over cancel-then-entrust.
5. **`entrustBs: "3"` and `"4"` on a futures entrust** (two orders, then cancel
   them). Resolves §9.3. If the Gateway rejects them, the vendor's two-value
   futures enum is authoritative and the released layer's 1–4 acceptance should be
   narrowed — a caller-visible behaviour change that needs its own task and its own
   ADR 0011 record, which is why it cannot be pre-empted here.
6. **`entrustType: "3"`** (one order, then cancel it). Resolves §9.2, the Java-vs-
   Python conflict. Highest risk of the six, because if `3` is rejected the SDK has
   been forwarding a code the Gateway refuses — though with no effect, since the
   caller had to choose it deliberately.
7. **One `TradeLogin`, then one `FuturesQueryFundInfo`** (one login, one read).
   Resolves §7.3: does the ordinary cash session serve futures, or does `20033`
   mean a separate `FuturesLogin` exists. Two requests, and it settles whether C13
   needs a futures-specific re-login path.

Items 1, 2 and 7 are **reads or a single login** and carry no financial risk.
Items 3, 4, 5 and 6 place real orders and must be run in a futures sandbox with the
orders cancelled afterwards. That ordering is deliberate: **three of the seven
answers need no order at all**, and they should be taken first.

## 12. Rejected alternatives

| # | Alternative | Why rejected |
|---|-------------|--------------|
| 1 | **Write the 11 requests from the response schemas by symmetry**, as the brief's fallback | Demonstrably wrong in two places (§2.1): the response spells the time-in-force `validType` and the request spells it `validTimeType`, and each side has a field the other lacks. A symmetry-derived body is one the Gateway rejects. |
| 2 | **Treat the 24 fields as inferred, since the brief says there is no request source** | The premise is false. Two vendor SDKs in the repository root state every request key as a source literal, and a released implementation has shipped the same names for 11 releases. Writing "inferred" on that evidence would be a false statement in the direction that makes the document *less* useful — it would invite C5 to second-guess names that are settled. The provenance split in §5 is a measurement, not a hedge. |
| 3 | **Add `exchangeType` to futures requests "for consistency" with the cash layer** | No futures request has one, in either vendor SDK or the released layer. Adding it is a wire change to a mutation on a guess, and it is the most likely single mistake in C3–C5. It is also, if added, invisible to the mock. |
| 4 | **Give the four no-param reads an empty `…WireRequest` struct for symmetry** | `struct{}{}` marshals to `{}`, which is byte-identical to the vendor's `{}`. An empty named struct adds a type that encodes nothing and gives a place for a field to be added by accident. |
| 5 | **Make `startDate`/`endDate` required, following the vendor's guard** | Rejected in §4.1. It would reject a history query that the released layer accepts, on the migration path, for a guard that is a client-side convenience check contradicting its own docstring. The divergence is recorded and G6 settles it. |
| 6 | **Validate `entrustType` against `{0,1,2}`** | The two vendor SDKs disagree (Java has a fourth, `OPTION`; Python does not). Guessing picks a side on a money-moving field with no evidence. §9.2 validates digit-ness and documents the conflict instead. |
| 7 | **Narrow `entrustBs` to the vendor's futures `{1,2}`** | Rejected in §9.3. It is a caller-visible behaviour change on a mutation, made on incomplete evidence, to fix a problem whose failure mode is a clear Gateway rejection. Widening the Go type to `types.EntrustBS` gives the check without a second source of truth. |
| 8 | **Drop `entrustType` from `FuturesModifyEntrust` because it is "not supported"** | It is already absent — both vendors omit it and this note keeps it absent. Listed because the mirror-image error (adding it, by symmetry with `EntrustRequest`) is the likely one, and it is called out explicitly in §4.2. |
| 9 | **Reuse `transport.Pagination` for futures, adding a `PageNo`** | One struct with two incompatible meanings (cursor vs page number), and futures' `Apply` would write `cursor`/`page_size` keys that the futures Gateway does not accept. §6.3 uses a futures-local type. |
| 10 | **Copy the cash layer's duplicated `historyEntrustListWireRequest` / `historyDeliverListWireRequest`** | Two identical types with no vendor justification. The vendor shares one parameter class; so does the v-next futures service (§6.2). |
| 11 | **Define a `FuturesLogin` because `20033` is a futures-specific code** | Both vendor SDKs have one login and no futures login, and all eleven futures routes are ordinary `/trade/*` calls on that session. `20033` is weak evidence of a distinct session and no more. §7.3 records it as an open question with the two-request test. |
| 12 | **Make P5 (the tick model) a hard prerequisite for C3–C6** | Rejected in §10.2, with a bounded argument rather than a preference: every futures price with `decInPrice ≤ 3` is already a multiple of the invented `0.001`, so the only exposure is `decInPrice ≥ 4` instruments, and their failure mode is the loud one. Meanwhile P5's blocking steps are about a **cash** HKEX band table. Sequencing the C-series behind it guarantees a stall on an unrelated human decision. The cheap substitute — split P5's mechanical steps forward, and forbid C3 from adding a new `"0.001"` literal — gets the protection without the dependency. |
| 13 | **Extend the parity guard to check futures request field names** | The guard would have to model the Gateway's request schema, and the only available model is the mock, which answers by path and cannot check a field name. A guard that cannot fail is worse than no guard — the exact reasoning C1 used to reject a test-based guard whose report-mode output `go test` discards. |
| 14 | **Ship a `FuturesService` whose futures prices carry the invented `0.001` tick, for consistency with the other 40 sites** | It would add 6–10 new literals to a set P5 exists to remove, and for futures the invented tick is *usually a multiple of the real one*, so it is not even a uniform error. The zero tick `"0"` is the tick note's §2.1 rule and is correct today. |

## 13. What this note does not settle

Named so that the next task does not mistake silence for a decision:

- **Whether `startDate`/`endDate` are required** (§4.1). Resolved in favour of
  parity with the released layer; G6 request 1 settles it.
- **Whether `FuturesEntrust` accepts an empty `entrustPrice` on a market order**
  (§4.3). This note requires it, with `"0"` as the recommended caller value; G6
  request 3 settles it.
- **The `entrustType` set** (§9.2). Deliberately unvalidated. G6 request 6 settles
  it, and a `pkg/types` addition is a separate ADR 0011 decision when it does.
- **The `entrustBs` set for futures** (§9.3). Kept at the released `{1,2,3,4}`.
  G6 request 5 settles it; narrowing is a separate caller-visible task.
- **Whether futures needs its own login** (§7.3). G6 request 7 settles it; C13's
  scope depends on the answer.
- **Whether `decInPrice` really is a decimal power, and whether
  `priceDecimalPoint == 10^-decInPrice`** (§10.2). G6 request 2 settles it, and it
  gates the tick note's amendment, which in turn gates P5's futures `TickSchedule`.
- **The HKEX futures price-band table** (§10.2). Not derivable from this repository
  and not attempted. P5 step 4's blocker, for the cash leg; futures inherits the
  same gap.

**No `.go` file was modified in producing this note.** The vendor SDK archives were
read from a scratch extraction under the pre-approved temp directory and nothing
was written into the repository.
