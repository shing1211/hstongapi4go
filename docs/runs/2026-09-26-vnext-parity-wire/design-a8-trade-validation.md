# A8 — Validating `ExchangeType` and `EntrustBS` in `pkg/hstong/trade`

Task A8 of run `2026-09-26-vnext-parity-wire`. Plan:
[plan-a8-trade-validation.md](./plan-a8-trade-validation.md). The finding this
fixes is
[finding-trade-code-validation.md](./finding-trade-code-validation.md) (A7),
which is the same fix A6 applied in `pkg/hstong/algo`
([design-a6-exchange-validation.md](./design-a6-exchange-validation.md)) at
roughly twice the scale.

## Decision

**Fail closed.** `ExchangeType` is validated against the documented four-market
set and `EntrustBS` against the documented four-direction set at all seventeen
caller-reachable request sites. An out-of-set value is rejected locally with the
typed `1016` invalid-parameter error and **no request is sent**. The eight sites
that carried no check at all gained an emptiness check as well as the set check.

## The defect

`pkg/hstong/trade` set-validated nothing. Nine of the seventeen request sites
checked `exchangeType` for emptiness only; the other eight checked nothing at
all, four of them on fields whose own documentation says "It is required".
`entrustBs` was emptiness-only on the one site that carries it. So after A6 the
same `types.EntrustBS` value had three answers inside one SDK: `future` refused
it, `algo` refused it, and `trade` put it on the wire.

A7 measured the reachability from outside the repository: `trade.Entrust` with
`EntrustBS: "9"` and `ExchangeType: "Z"` produced one recorded request with
`"entrustBs":"9"` in the body, and `trade.MarginFundInfo` with an empty market
produced `{"exchangeType":""}`. Both are re-proved here through the Manager in
this package's own tests.

The cost asymmetry that decided `algo` is sharper here. `Entrust`,
`CancelEntrust`, `BatchCancelEntrust` and `ChangeEntrust` each issue exactly one
HTTP request and are never retried at any layer (ADR 0003, restated in
`pkg/hstong/trade/doc.go`). `BatchCancelEntrust` is the sharpest single case: an
empty `EntrustIDs` cancels every cancellable order in the market, so the market
is the only thing bounding the blast radius, and an unrecognised one is a
cancel-everything request against a book the caller did not name, sent once.

## The valid sets

Both come from `pkg/types/enums.go` and `docs/SPEC.md` §7.3–7.4. Neither is
annotated non-exhaustive, unlike `EntrustType` (`enums.go:150-151`) and unlike
`EntrustType` in `SPEC.md` §7.5.

| Type | Codes | Source |
|------|-------|--------|
| `types.ExchangeType` | `K` HK, `P` US, `v` Shenzhen Connect, `t` Shanghai Connect | `enums.go:122-129`; case-sensitive |
| `types.EntrustBS` | `1` open long, `2` close long, `3` close short, `4` open short | `enums.go:138-145` |

**All four directions are valid.** `3` is `EntrustCloseShort` and `4` is
`EntrustOpenShort`; they are the short-selling directions, and a check written
against only `1` and `2` would refuse legitimate orders. A6's first pass at the
identical change used `"3"` as its example of an unvalidated code, which was
wrong, and A6 added a mutation to keep that mistake caught. A8 does the same,
and goes further: the acceptance side is asserted **on the wire** for all four
codes (`TestEveryDocumentedDirectionIsAcceptedAndSends`), and mutation (b) below
reproduces exactly that narrowing and is killed by those rows.

A7 noted that `3` and `4` already work on `trade` today, because nothing checks
the field. A fix that rejected them would have been a regression, not a fix.

## Why fail closed, and why the forward-compatible precedents do not apply

A6's argument transfers verbatim and is not restated in full. The short form:
`pkg/hstong/future/future.go:158-167` already set-validates the identical four
`EntrustBS` codes on a released v0.1.x surface, and `pkg/hstong/algo` does so as
of A6. Two answers for one type in one SDK is the inconsistency A6 was made to
end; `trade` is where the inconsistency was still live. The two precedents that
point the other way are claims about what the SDK can know, and neither claim
holds here:

- `types.EntrustType` is documented non-exhaustive and says so in its own GoDoc;
  it makes no such claim, and `SPEC.md` §7.3/§7.4 are flat four-row tables.
- `targetStrategy` is unvalidated because the vendor's own documentation
  contradicts itself (`1005` is POV in the AddOrder table and INLINE in the
  request examples). There is no contradiction in `{K, P, v, t}` or
  `{1, 2, 3, 4}`.

What a fail-closed check prevents is a request the SDK already knows it does not
mean, sent once, on a mutation, with no retry. What it causes, if the vendor ever
adds a market, is a local error naming the four codes plus a one-line change to
`validExchange`. For a financial SDK that asymmetry is not close.

The response side is deliberately untouched, and A7 endorses A6's reasoning
verbatim: `OrderVo.EntrustBS`, `OrderVo.ExchangeType`, `CondOrderVo.EntrustBS`,
`CondOrderVo.ExchangeType` and `HoldsVo.ExchangeType` are decoded from the
Gateway. Validating inbound data would turn a vendor-side surprise into a decode
error for the caller, which is a different policy from validating outbound
requests. `doc.go` now says so.

## Implementation

Four unexported helpers in `pkg/hstong/trade/orders.go`, next to
`isConditional`/`priceOptional` and in the same style as `algo`'s:

| Helper | Line | Role |
|---|---|---|
| `validExchange` | `orders.go:55` | `{K, P, v, t}`, case-sensitive |
| `validDirection` | `orders.go:74` | `{1, 2, 3, 4}`, 3 and 4 included |
| `invalidExchange` | `orders.go:89` | shared market message, names all four codes |
| `invalidDirection` | `orders.go:96` | shared direction message, names 3 and 4 explicitly |
| `validateExchange` | `orders.go:111` | emptiness branch **then** set branch |
| `validateOptionalExchange` | `orders.go:127` | absent accepted, present-and-unrecognised refused |

A free function rather than a method on the type is forced by the package
boundary: `types.ExchangeType` and `types.EntrustBS` live in `pkg/types`, which
is inside ADR 0011's protected set, and a Go method cannot be declared on a
non-local type. Adding an exported `Valid()` to `pkg/types` would widen the
public surface of a protected package; that is a separate decision and is not
taken here. Duplicating the constants in `trade` instead would create a second
source of truth for a set `pkg/types` already declares, and drift between them
would be silent.

### One deliberate deviation from A6: the emptiness+set pair is hoisted into a helper

A6 inlined the two branches at each of its eight sites, because all eight
already had a `validate()` method with an `exchangeType == ""` branch to extend,
so the change was two lines each. That shape does not scale to seventeen sites
across two files, eight of which had nothing to extend. A8 therefore routes
every site through `validateExchange` (or `validateOptionalExchange` for the one
optional field) instead of repeating the pair.

This is a deviation from A6's literal shape, and it is deliberate. The two
mistakes the task has to prevent are both per-site mistakes: a site that checks
only the set still sends `exchangeType: ""`, and a site that checks only
emptiness still forwards a market the SDK does not recognise. **Both were live
in this package** — eight sites had the second, and four of them on fields
documented required. Repeating the pair seventeen times makes the correct
behaviour a matter of per-site discipline, which is exactly the discipline that
had already failed once. A shared helper makes it structural, and it makes
"no wording drift" a property of the code rather than a review outcome.

The brief's requirement is still met at every site and is asserted there: a
caller who forgot the field gets `exchangeType is required`, never a message
about four market codes. `TestEveryRequestSiteRejectsAnEmptyRequiredExchangeType`
asserts both halves at all fifteen required sites — that the message contains
`exchangeType is required` **and** that it does *not* contain
`not one of K (Hong Kong)`.

### The seventeen sites

`EntrustBS` appears at one site; `exchangeType` at sixteen.

| # | Request field | Site after | Before | After |
|---|---|---|---|---|
| 1 | `EntrustRequest.ExchangeType` | `orders.go:214` | emptiness only | emptiness + set |
| 2 | `EntrustRequest.EntrustBS` | `orders.go:224` (empty) / `orders.go:227` (set) | emptiness only | emptiness + set |
| 3 | `CancelEntrustRequest.ExchangeType` | `orders.go:293` | emptiness only | emptiness + set |
| 4 | `BatchCancelEntrustRequest.ExchangeType` | `orders.go:344` | emptiness only | emptiness + set |
| 5 | `ChangeEntrustRequest.ExchangeType` | `orders.go:399` | emptiness only | emptiness + set |
| 6 | `MaxAvailableAssetRequest.ExchangeType` | `orders.go:507` | emptiness only | emptiness + set |
| 7 | `CondOrderListRequest.ExchangeType` | `orders.go:748` | emptiness only | emptiness + set |
| 8 | `HistoryCondOrderListRequest.ExchangeType` | `orders.go:839` | emptiness only | emptiness + set |
| 9 | `BeforeAndAfterSupportRequest.ExchangeType` | `orders.go:928` | emptiness only | emptiness + set |
| 10 | `RealEntrustListRequest.ExchangeType` | `orders.go:645` | **nothing** | **emptiness + set** |
| 11 | `RealDeliverListRequest.ExchangeType` | `orders.go:659` | **nothing** | **emptiness + set** |
| 12 | `HistoryEntrustListRequest.ExchangeType` | `orders.go:811` | **nothing** | **emptiness + set** |
| 13 | `HistoryDeliverListRequest.ExchangeType` | `orders.go:825` | **nothing** | **emptiness + set** |
| 14 | `MarginFundInfoRequest.ExchangeType` | `assets.go:185` | **nothing** | **emptiness + set** |
| 15 | `FundJourListRequest.ExchangeType` | `assets.go:356` | **nothing** | **emptiness + set** |
| 16 | `HistoryFundJourListRequest.ExchangeType` | `assets.go:371` | **nothing** | **emptiness + set** |
| 17 | `PositionsRequest.ExchangeType` | `assets.go:292` | **nothing** | optional-field shape: absent accepted, present-and-unrecognised refused |

**The eight that gained an emptiness check** are rows 10–17. Rows 10–13 and 14–16
are documented required; row 17 is documented optional and keeps the
`algo.QueryOrderList` shape, so a set check alone would be the wrong answer there
and an emptiness check alone would be wrong everywhere else.

`pkg/hstong/trade/doc.go` gained a **Validation** section naming both closed
sets, the one exemption, and the deliberately unvalidated response fields.

## Tests

New file `pkg/hstong/trade/codes_test.go`, plus rows in the two existing test
files. `exchangeSites()` is the work list — sixteen rows — and each of the three
table tests runs all of them, so a guard dropped from any single request type
fails the suite.

- `TestEveryRequestSiteRejectsAnUnrecognisedExchangeType` — 16 sites × 10
  out-of-set markets, each through a `Manager`, each asserted to produce a typed
  `1016` error, the right operation label, and **zero** recorded requests. The
  values are the near-misses a caller is likely to send: wrong case (`k`, `V`,
  `T` — the field is case-sensitive and `v`/`t` are lowercase), a trailing
  space, a numeric where a letter belongs, and the market name.
- `TestEveryRequestSiteRejectsAnEmptyRequiredExchangeType` — all 15 required
  sites, asserting both message halves, plus the `Positions` control row that an
  absent optional market *is* sent (1 request).
- `TestEveryRequestSiteAcceptsEveryDocumentedMarket` — **all four markets on all
  sixteen sites, asserted on the wire**. The `exchangeType` is read back out of
  the recorded request body, not from a decoded struct, so a value dropped or
  rewritten in transit is caught too. Using all four rather than the `K` the
  rest of the suite uses throughout is deliberate: a guard hard-coded to the
  fixture's market, or one that accepted only the two uppercase markets, would
  otherwise hide behind the common fixture.
- `TestEveryDocumentedDirectionIsAcceptedAndSends` — `1`, `2`, `3`, `4` through
  `Entrust`, each asserted on the wire. This is the test the `"3"`-is-invalid
  mistake cannot survive.
- `TestValidatedDirectionSetRejectsEveryOutOfSetValue` — 12 out-of-set
  directions, including the non-ASCII digit `١` that a Unicode-aware check would
  fold to `1`.
- `TestCodeSetErrorMessageIsIdenticalAtEverySite` — the no-drift guard. Both
  messages are compared against the shared builder's own output at every site,
  and the market message is asserted to name all four markets. A site that grew
  its own wording fails here.
- `TestPredicatesAcceptExactlyTheDocumentedSets` — pins `validExchange` and
  `validDirection` directly, so a set that gains a value, loses one, or grows a
  case-insensitive comparison fails without depending on any endpoint.
- `TestValidateExchangeSeparatesAbsenceFromAnUnknownValue` — the shared helper's
  two branches, plus the purity check that neither shape reaches the Gateway.
- Five new rows in the existing per-type branch tables: `exchangeType not in set`
  for `CancelEntrust`/`ChangeEntrust`/`MaxAvailableAsset` in
  `validation_test.go`, and `exchangeType not in set` and `entrustBs not in set`
  in `TestOrders_EntrustValidation`.

### Was a test pinning the old behaviour found?

**No.** A7 predicted none (`finding-trade-code-validation.md:269`) and said a
future task should grep again rather than assume it. That was verified
empirically rather than by grep alone: the pre-change source was restored with
`git checkout`, the new test file moved aside, and the pre-existing suite run
against it. It was green — including
`TestOrders_EntrustValidation`, `TestCancelEntrustRequestValidateBranches`,
`TestChangeEntrustRequestValidateBranches` and
`TestMaxAvailableAssetRequestValidateBranches`. Nothing pinned the fail-open
behaviour, so unlike A6's `TestUnvalidatedCodesAreForwardedToTheGateway` there
was nothing to invert. The change to the two existing test files is purely
additive: five new rejection rows, no row removed or weakened.

The `ExchangeType: types.ExchangeHK` fixtures throughout the package, in
`orders_test.go`, `assets_test.go`, `branches_test.go`, `error_paths_test.go` and
`pagination_test.go`, are valid codes and were left untouched. They still pass.

## Mutation checks

Four mutations, all killed. Each was applied, the source was inspected to confirm
the guard was genuinely gone, and only then were the tests run — a partial revert
that changes nothing cannot fail a test, so treating a green run as "the test is
blind" would have proved nothing.

| # | Mutation | Result |
|---|----------|--------|
| a | Delete the `!validExchange` set guard from `validateExchange` | **killed** — 7 tests, 154 failing subtests, across all 16 sites and 10 out-of-set values |
| b | Narrow `validDirection` to buy/sell only — the exact `"3"`-is-invalid mistake | **killed** — 2 tests, 4 subtests: both wire-level acceptance rows for `3` and `4`, plus both predicate rows |
| c1 | Delete the `if e == ""` emptiness branch from `validateExchange` | **killed** — 2 tests, 15 subtests: every required site reported the four-code message instead of "is required" |
| c2 | Delete the emptiness check at **one** of the eight newly-checked sites (`MarginFundInfo` switched to the optional shape) | **killed** — 1 test, 1 subtest: `MarginFundInfoRequest(exchangeType="") = nil error, want a local rejection` |

Mutation (b) is the one that matters most, and its result is the point: the
*rejection* matrix does not notice at all when the set is narrowed to `1`/`2`,
because every value it tests is still outside that set. Only the acceptance side
catches it. `TestValidatedDirectionSetRejectsEveryOutOfSetValue` passed under
mutation (b); `TestEveryDocumentedDirectionIsAcceptedAndSends` and
`TestPredicatesAcceptExactlyTheDocumentedSets` did not. That is why the
acceptance assertions are on the wire and why the brief's instruction to enforce
the acceptance side rather than assert it in a comment was followed literally.

After all four mutations the tracked diff was confirmed byte-identical to the
pre-mutation backup (`git diff --output` hashes equal).

`pkg/hstong/trade` holds at **100.0%**, measured three ways: in a clean Linux
container on the working tree, from a fresh `git clone` with only the A8 diff
applied, and via the repository's own gate (`go run scripts/coverage_gate.go`,
which reports `PASS: .../pkg/hstong/trade 100.0%`).

## ADR 0011 analysis — is this a breaking change?

**No, and it is explicitly permitted.** ADR 0011 lists `pkg/hstong/trade` in its
protected surface table (`0011:29`), so the change is measured against its five
guarantees.

| Guarantee | Assessment |
|---|---|
| 1. No breaking type changes | **Untouched.** No exported type, field, signature, or method is added, removed, or retyped. All sixteen request types keep every field with the same name, type, and JSON tag; no field became optional or required. The only new identifiers are unexported: `validExchange`, `validDirection`, `invalidExchange`, `invalidDirection`, `validateExchange`, `validateOptionalExchange`. `pkg/types` is not touched and gains no methods, so no protected package's public surface widens. |
| 2. No breaking wire changes | **Untouched.** No `gen/` file and no wire struct is edited. A request carrying an in-set value serialises byte-identically to before, which `TestEveryRequestSiteAcceptsEveryDocumentedMarket` proves by reading the recorded body. Nothing is renamed, retyped, added, or removed from any body. |
| 3. No new mandatory dependencies | **Untouched.** `go.mod` and `go.sum` are not modified. The predicates are `switch` statements over existing constants; the only import added is `fmt` to `orders.go`, which is standard library and already used across the module (AGENTS.md rule 8). |
| 4. Mock Gateway compatibility | **Untouched.** `test/mockgateway/` and `cmd/hstong-mock-gateway/` are not modified, and `go test -count=1 ./...` passes with `test/e2e` and `test/mockgateway` green, so no e2e test needed updating. |
| 5. `gen/` never touched | **Untouched.** |

What *is* a runtime behaviour change: a request that previously reached the
Gateway now does not. ADR 0011's permitted-changes table covers this in the row
**"Bug fixes that change runtime behaviour — expected; bugs are not specified
behaviour"** (`0011:71`) — the same row A6 relied on. **No waiver and no ADR
amendment is required.**

The difference from A6 is scope, not mechanism: seventeen sites instead of eight,
two files instead of one, and a wider enumerated blast radius. A7 recorded that
blast radius as "every caller of sixteen request types", and it was measured
here rather than assumed. Every in-repo consumer of `pkg/hstong/trade` is
`test/e2e`, `test/integration`, `examples/trading` and `examples/quickstart`
(confirmed by text search; `pkg/services` does not import the package). All four
use `types.ExchangeHK` or, for the integration test, an env var defaulting to
`"K"` — all in set — and all pass unmodified. `GitNexus impact` reported the
four `validate` methods at LOW risk with one direct caller each; the index is 49
commits behind HEAD, so the text search above is the load-bearing evidence, per
AGENTS.md's rule on `risk: UNKNOWN` and stale graphs.

Two classes of affected caller exist, and both are worth naming:

1. **A caller sending a genuinely new vendor code.** Broken loudly and locally
   until `validExchange` is updated. Mitigated by the message naming the four
   codes and by the update being one line.
2. **A caller sending a code the SDK never supported** — a typo, a case-folded
   value, a name instead of a code. Already failing at the Gateway; now failing
   before it. Nothing that worked stops working.

There is no third class: `types.ExchangeType` and `types.EntrustBS` are typed
string aliases whose complete sets are declared in `pkg/types`, with a constant
for every value, and no caller can be relying on a fifth market or a fifth
direction through a documented path.

Because the guarantee is about type and wire compatibility and neither moves, the
decision is recorded here and in the GoDoc at each site rather than in a new
ADR — an ADR would be the right instrument if this changed a signature or the
wire. Should a future task expose `Valid()` on `pkg/types`, that **does** widen a
protected package's public surface and does need an ADR; that decision is
deliberately not pre-empted here, and should be taken once for `future`, `algo`
and `trade` together.

## What A8 did not do

- **`pkg/services` still has a fourth policy.** `trading.go:610-621`
  `validateShortSellingFlag` accepts exactly `{1,2,3,4}` but only when `DataType`
  is HK stock/ETF, so a US order's `types.EntrustBS` is unvalidated there. Out of
  scope: A7 recorded it as completeness, `pkg/services` carries no compatibility
  guarantee (`ADR 0011:92-94`), and it is a different decision from a released
  surface. A7's other `pkg/services` observation — that `MarketShenzhenConnect`
  and `MarketShanghaiConnect` fall through `trading.go:123-129` to the empty
  string — is likewise untouched.
- **The response side is unchanged**, as A7 and A6 both recommend.
- **`pkg/hstong/future` was not changed.** It already set-validates `EntrustBS`,
  and `types.ExchangeType` is absent from the package, so it needs nothing. A7
  also noted the other `EntrustType`-family checks in that package; they were not
  re-litigated here.
- **`entrustType`, `sessionType`, `condTrackType` and the other `EntrustRequest`
  string fields are still emptiness-only.** Their documented code sets are not
  published as closed — `types.EntrustType` says in its own GoDoc that the set is
  non-exhaustive and unknown values are forwarded unchanged — so the same
  fail-open argument that A6 rejected for these two types does not apply to them.
  `doc.go` now says so, so the policy is documented rather than accidental.
- **No shared `Valid()` was added to `pkg/types`.** Same reasoning as A6: it
  widens an ADR 0011 protected package and needs its own ADR.
- Nothing was committed or pushed.

## Verification run for this task

`make` is not installed on this host, so the direct equivalents were run with
`GOTMPDIR` set to the pre-approved temp directory, and the race gate with the
dev host's MinGW gcc and `CGO_ENABLED=1` (AGENTS.md: the race gate was satisfied
locally, not deferred to CI).

```
$ gofmt -l .
(no output)

$ go vet ./pkg/hstong/trade/
exit=0

$ go test -count=1 ./pkg/hstong/trade/
ok  github.com/shing1211/hstongapi4go/pkg/hstong/trade  1.066s

$ go test -count=1 -cover ./...
(all 25 testable packages ok; pkg/hstong/trade 100.0%, pkg/hstong/algo 100.0%,
 pkg/hstong/stream 100.0%, pkg/types 100.0%, pkg/transport 100.0%)

$ CC=C:\Users\Tchan\mingw64\bin\gcc.exe CGO_ENABLED=1 go test -race -count=1 ./...
(25 packages ok, no data races reported)

$ go run scripts/coverage_gate.go
PASS: github.com/shing1211/hstongapi4go/pkg/hstong/trade 100.0%
All packages meet the 85% coverage gate
exit=0

$ python scripts/check_money.py
money-check OK: no float money/quantity fields in pkg/
exit=0

$ python scripts/check_links.py
check_links: scanned 106 Markdown files, 0 unresolved link(s)
exit=0
```

Coverage, measured in a clean Linux container per AGENTS.md:

```
$ docker run --rm -v "${PWD}:/repo" -w /repo -e GOTMPDIR=/tmp/gotmp golang:1.26 \
    sh -c "mkdir -p /tmp/gotmp && go test -count=1 -coverprofile=/tmp/t.out \
           ./pkg/hstong/trade >/dev/null 2>&1 && go tool cover -func=/tmp/t.out | tail -1"
total:									(statements)			100.0%
```

and, because a dirty tree can read high, from a fresh `git clone` of the local
repo with only the A8 diff applied and nothing else present:

```
fresh clone + A8 patch:
total:									(statements)			100.0%
```

All three measurements agree at 100.0%, and the coverage figure is stable across
runs of the same commit — the tests reach every branch by construction, with no
sleep, poll, or deadline anywhere in `codes_test.go`.

`check_links.py` scanned 106 files. A7 recorded 104; this note is the new
Markdown file since.
`docs/runs/**` is excluded from the mkdocs nav, so `mkdocs build --strict` does
not build this file and is not claimed as coverage of it; `check_links.py` is
what validated it. The links it relies on —
`./plan-a8-trade-validation.md`, `./finding-trade-code-validation.md`,
`./design-a6-exchange-validation.md`, `./todos.md`,
`../../adr/0011-v01x-compatibility.md` — all resolved on disk. Go sources are
cited as bare `path:line` text rather than links, following the convention in
the A6 and A7 notes, because `check_links.py` correctly rejects a link to a
source file.
