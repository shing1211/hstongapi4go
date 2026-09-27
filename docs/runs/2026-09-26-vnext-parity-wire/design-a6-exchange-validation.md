# A6 — Validating `ExchangeType` and `EntrustBS` in `pkg/hstong/algo`

Task A6 of run `2026-09-26-vnext-parity-wire`. Plan:
[plan-a6-exchange-validation.md](./plan-a6-exchange-validation.md). Finding A was
raised by A3 and is fixed here.

## Decision

**Fail closed.** `ExchangeType` is validated against the documented four-market
set and `EntrustBS` against the documented four-direction set at all eight
sites. An out-of-set value is rejected locally with `ErrInvalidParams` and no
request is sent.

## The defect

`pkg/hstong/algo` checked `ExchangeType` and `EntrustBS` for emptiness only,
while `EntrustType`, `SessionType`, `Sensitivity`, and `Action` were all
validated against closed sets. Neither `types.ExchangeType` nor
`types.EntrustBS` carries a `Valid()` method, so no check existed anywhere in
the SDK for either. A caller could send `ExchangeType: "Z"` and the SDK would
transmit it — on `CancelOrder` a cancel naming a book the caller did not
choose, issued once and never retried (ADR 0003), with no second chance.

## The valid sets

Both come from `pkg/types/enums.go` and `docs/SPEC.md` §7.3–7.4, and neither
is annotated non-exhaustive:

| Type | Codes | Source |
|------|-------|--------|
| `types.ExchangeType` | `K` HK, `P` US, `v` Shenzhen Connect, `t` Shanghai Connect | `enums.go:122-129`; case-sensitive |
| `types.EntrustBS` | `1` open long, `2` close long, `3` close short, `4` open short | `enums.go:138-145` |

`3` and `4` are `EntrustCloseShort` and `EntrustOpenShort` — legitimate
short-selling directions. A check written against only `1` and `2` is wrong,
and the task brief that produced the original finding used `"3"` as its example
of an invalid code. That example was incorrect; both codes are accepted here
and the acceptance is pinned by test
(`TestEveryDocumentedCodeIsAccepted`, and the `add/entrustBs accepted 3` /
`… 4` rows of the validation matrix).

## Why fail closed, and why the forward-compatible precedent does not apply

The plan frames a real tension, and the fail-open side has two genuine
precedents in this repository. Both are claims about **what the SDK can know**,
not a general policy of forwarding unknown codes, and neither claim holds for
these two types.

**`types.EntrustType` is the wrong precedent because its documentation makes a
different promise about a different set.** `types.EntrustType` says so
explicitly (`enums.go:150-151`): "The set is non-exhaustive and the vendor may
add codes; unknown values are forwarded unchanged." `docs/SPEC.md` §7.5 repeats
it and explains why: the code set runs past 31, carries per-market meanings,
and the legacy direct-protocol dictionary uses a *different* code set for the
same field. The type is telling callers "do not treat my constant list as the
domain", and the SDK takes it at its word. `types.ExchangeType` and
`types.EntrustBS` make no such claim. `SPEC.md` §7.3 and §7.4 are flat
four-row tables with no non-exhaustive note, `enums.go` adds no caveat, and
`pkg/types` defines a constant for every value. A closed set published as
complete is a different kind of statement from a set published as open.

**`targetStrategy` is the wrong precedent because its set is genuinely
unknowable.** `algo.go:36-40` records why: "1005" is POV in the AddOrder
parameter table and INLINE in the request examples, and 1003 appears only in
the examples. The SDK cannot decide which values are correct, so a local closed
set would reject requests the vendor's own documentation endorses. Refusing to
guess is the right response to a contradiction. There is no contradiction in
`{K, P, v, t}` or `{1, 2, 3, 4}`; a set that is not contradictory is one the
SDK can check.

**The decisive argument: the SDK already chose fail-closed for this exact type,
on this exact set.** `pkg/hstong/future/future.go:158-167` rejects anything
outside `{1, 2, 3, 4}` locally, on the released v0.1.x surface, and
`docs/futures.md:97` documents it. So `types.EntrustBS("9")` is already refused
by `future.Entrust` while `algo.AddOrder` would forward it — one SDK, one type,
one set, two answers. Any forward-compatibility argument for algo's `EntrustBS`
applies verbatim to futures, where the maintainers already took the opposite
decision. Choosing fail-open here would preserve the inconsistency; choosing
fail-closed makes the type behave the same everywhere. A7 is investigating
whether the same gap exists in `pkg/hstong/trade`, which is the same argument
one surface over and is recorded as a finding, not fixed in this task.

**The cost asymmetry settles the rest.** The failure a fail-closed check
prevents is a request the SDK already knows it cannot mean, sent once, on a
mutation, with no retry. The failure a fail-closed check causes, if the vendor
ever adds a market, is `errors.Is(err, algo.ErrInvalidParams)` with a message
naming the four codes, at the call site, before any request — a loud local
error followed by a one-line change to `validExchange` and a rebuild. One side
is a silent misroute; the other is a diagnosable error. For a financial SDK the
asymmetry is not close.

**The package already states the rule it now follows.** `action.go:6-11` says
the action set "is treated as closed for local validation: an unlisted value is
rejected before any request is sent, not forwarded. If the vendor extends the
set, this constant group and `Action.Valid` must be updated together." Applying
that same rule, with that same receipt, to `exchangeType` and `entrustBs` is
the existing policy applied to the two stragglers — not a new one. `validExchange`
and `validDirection` carry that receipt in their GoDoc, and `doc.go` now names
the six closed sets and the one field that is deliberately forwarded.

## Implementation

Two local unexported predicates in `pkg/hstong/algo/algo.go`, next to the
existing `EntrustType.valid`, `SessionType.valid`, and `Sensitivity.valid`:

```go
func validExchange(e types.ExchangeType) bool   // {K, P, v, t}
func validDirection(bs types.EntrustBS) bool    // {1, 2, 3, 4}
```

and two message builders, `invalidExchange` and `invalidDirection`, shared by
all call sites so the wording cannot drift between endpoints. Both messages name
**all four** accepted codes; the direction message names 3 and 4 explicitly, so
it cannot be read as evidence that only buy and sell are accepted.

A free function rather than a method on the type is forced by the package
boundary: `types.ExchangeType` and `types.EntrustBS` live in `pkg/types`, which
is inside ADR 0011's protected set, and a Go method cannot be declared on a
non-local type. Adding an exported `Valid()` to `pkg/types` would widen the
public surface of a protected package; that is a separate decision and is not
taken here. The alternative of duplicating the constants in `algo` was rejected
because it would create a second source of truth for a set `pkg/types` already
declares, and drift between them would be silent.

### The eight sites

| Line (post-change) | Request type | Before | After |
|---|---|---|---|
| 278 | `AddOrderParams.ExchangeType` | `== ""` → required | `== ""` → required, then `!validExchange` → not in set |
| 302 | `AddOrderParams.EntrustBS` | `== ""` → required | `== ""` → required, then `!validDirection` → not in set |
| 348 | `CancelOrderParams.ExchangeType` | `== ""` → required | + `!validExchange` |
| 378 | `CancelEntrustParams.ExchangeType` | `== ""` → required | + `!validExchange` |
| 414 | `ChangeOrderParams.ExchangeType` | `== ""` → required | + `validExchange` |
| 464 | `ActionOrderParams.ExchangeType` | `== ""` → required | + `validExchange` |
| 524 | `QueryOrderListParams.ExchangeType` | `StockCode != "" && == ""` → required when set | + `!= "" && !validExchange` (optional-field shape) |
| 557 | `QueryEntrustIDListParams.ExchangeType` | `== ""` → required | + `!validExchange` |

The emptiness branch is kept at every site rather than folded into the set
check, for two reasons: the caller who forgot the field gets "exchangeType is
required" instead of a message about four market codes, and the existing
"required" rows of the validation matrix keep their meaning.

`QueryOrderList` is the one site where the field is optional, so it uses the
optional-field shape: absent is accepted, present-and-unrecognised is refused.
Omitting the check there because the field is optional would be a second,
different policy for the same type on the same release.

`WithDefaultExchangeType` is affected and is now documented as such. The
default is substituted inside the Manager method *before* `validate` runs, so it
is checked by the same rule as an explicit value; without that, the option
would be a way to smuggle an unchecked market into every call relying on it.
`TestUnrecognisedDefaultExchangeTypeIsRejected` pins this.

## Tests

`TestUnvalidatedCodesAreForwardedToTheGateway` (in `query_failure_test.go`, not
`validation_matrix_test.go` as the brief stated) asserted the old fail-open
behaviour, including `ExchangeType: "Z"` and `EntrustBS: "3"` being forwarded.
It was **inverted, not deleted**: it now pins only `targetStrategy`, the one
code that remains deliberately forwarded, and its GoDoc records both prior
errors so a future relaxation of the other two is visible. Restoring A3's
original assertions makes it fail (proof below), so it was genuinely
exercising the path.

Added:

- `TestValidatedCodeSetsRejectEveryOutOfSetValue` — twelve out-of-set markets
  and eleven out-of-set directions, each driven through a `Manager` and
  asserted to produce **zero** recorded requests. Includes the near-misses a
  caller is likely to send: wrong case (`k`, `p`, `T`, `V`), a neighbouring
  code, a numeric where a letter belongs, the name rather than the code, a
  trailing space, and a non-ASCII digit.
- `TestEveryEndpointRejectsAnUnrecognisedExchangeType` — all seven request
  types, so a check dropped from any single `validate` method is caught. One
  control row asserts that `QueryOrderList` with both `exchangeType` and
  `stockCode` absent *is* sent, so the optional-field exemption is pinned too.
- `TestEveryDocumentedCodeIsAccepted` — all four markets and all four
  directions accepted and asserted on the wire, which is the proof that 3 and 4
  reach the Gateway.
- `TestUnrecognisedDefaultExchangeTypeIsRejected`.
- Eight new rows in `TestValidateRejectsEveryField` (one per site) and twelve
  new rows in `TestValidateAcceptsWellFormedRequests` (the accepted sets on
  `AddOrder` and `QueryOrderList`).

`pkg/hstong/algo` holds at **100.0%**, measured both in a clean Linux container
and from a fresh `git clone`.

## Mutation checks

Four mutations, all killed; a partial revert that changes nothing cannot fail a
test, so the code was inspected after each mutation to confirm the checks had
actually been removed.

| # | Mutation | Result |
|---|----------|--------|
| 1 | Restore A3's original `TestUnvalidatedCodesAreForwardedToTheGateway` assertions | killed — `exchangeType "Z" is not one of K…` |
| 2 | Delete all seven `!validExchange` guards | killed — 7 matrix rows, 12 values, 7 endpoints, default-exchangeType test |
| 3 | Delete the `!validDirection` guard | killed — 1 matrix row, 11 values |
| 4 | Narrow `validDirection` to buy/sell only (the `"3`-is-invalid` mistake`) | killed — 2 accept rows, 2 wire assertions |

## ADR 0011 analysis — is this a breaking change?

**No, and it is explicitly permitted.** ADR 0011 lists `pkg/hstong/algo` in its
protected surface, so the change is measured against its three guarantees:

| Guarantee | Assessment |
|---|---|
| 1. No breaking type changes | **Untouched.** No exported type, field, signature, or method is added, removed, or retyped. `AddOrderParams` and the six other request types keep every field with the same name, type, and JSON tag. The only new identifiers are unexported: `validExchange`, `validDirection`, `invalidExchange`, `invalidDirection`. `types.ExchangeType` and `types.EntrustBS` are not modified and gain no methods, so `pkg/types` is unchanged. |
| 2. No breaking wire changes | **Untouched.** No `gen/` file and no `pkg/types` wire struct is edited. The bytes on the wire are identical for every request that was valid before: a request carrying an in-set value serialises exactly as it did. Nothing is renamed, retyped, added, or removed from any body. |
| 3. No new mandatory dependencies | **Untouched.** `go.mod` and `go.sum` are not touched. The predicates are `switch` statements over existing constants; no import was added to `algo.go` (`fmt` and `pkg/types` were already imported). |
| 4. Mock Gateway compatibility | **Untouched.** `test/mockgateway/` and `cmd/hstong-mock-gateway/` are not modified, and `go test -count=1 ./...` passes with the e2e suite green, so no e2e test needed updating. |
| 5. `gen/` never touched | **Untouched.** |

What *is* a runtime behaviour change: a request that previously reached the
Gateway now does not. ADR 0011's permitted-changes table lists "**Bug fixes
that change runtime behaviour** — expected; bugs are not specified behaviour",
and this is that category. A3 raised it as Finding A, the most serious of its
three findings, and characterised the fail-open behaviour as a defect rather
than a design choice; the original test's own GoDoc agreed ("it is not an
endorsement"). The recorded blast radius is narrow and enumerable: a caller who
was sending a value outside `{K, P, v, t}` or `{1, 2, 3, 4}` for algo, and who
received a Gateway response for it. Two classes of such caller exist, and both
are worth naming:

1. **A caller sending a genuinely new vendor code.** The change breaks them,
   loudly and locally, until `validExchange` is updated. Mitigated by the
   message naming the four codes and by the update being one line.
2. **A caller sending a code the SDK never supported** — a typo, a
   case-folded value, a name instead of a code. These were already failing at
   the Gateway; they now fail before it. Nothing that worked stops working.

There is no third class: `types.ExchangeType` and `types.EntrustBS` are typed
string aliases whose complete sets are declared in `pkg/types`, and no caller
can be relying on a fifth market or a fifth direction through a documented
path.

Because the guarantee is about type and wire compatibility and neither moves,
no ADR amendment is required. The decision is recorded here and in the GoDoc at
each of the eight sites rather than in a new ADR: an ADR would be the right
instrument if this changed a signature or the wire, and a design note with the
GoDoc beside the code is the right one for a validation policy over existing
types. Should a future task choose to expose `Valid()` on `pkg/types`, that
*does* widen a protected package's public surface and does need an ADR; that
decision is deliberately not pre-empted here.

## What A6 did not do

- `pkg/hstong/trade` has the same `EntrustBS` gap at `orders.go:121` (and
  `exchangeType` at `orders.go:112`). Out of scope: a different released surface
  with its own decision. Recorded for A7.
- `pkg/hstong/future` validates `EntrustBS` but not `ExchangeType`, and
  `pkg/hstong/trade`, `market`, and `future` all still validate `ExchangeType`
  for emptiness only. The same reasoning would apply to them; widening the
  change to four released surfaces at once is a different task from fixing the
  one Finding A named.
- No shared `Valid()` was added to `pkg/types`. If the maintainers want one, it
  is a separate decision that touches an ADR 0011 protected package, and it
  should be taken once for all four surfaces rather than introduced here.
- `MasterOrder.EntrustBS` (`algo.go:586`) is a **response** field decoded from
  the Gateway. It is deliberately not validated: validating inbound data would
  turn a vendor-side surprise into a decode error for the caller, which is a
  different policy from validating outbound requests.
