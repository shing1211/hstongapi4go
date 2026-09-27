# A6 — Validate ExchangeType and EntrustBS in algo (plan)

## Decision (user, 2026-09-26)

Option 1: fix Finding A in this run rather than defer to v1.0. This is a behaviour
change on the released `pkg/hstong/algo` surface, so ADR 0011 applies.

## The defect

`pkg/hstong/algo` forwards `ExchangeType` and `EntrustBS` **unvalidated**. Unlike
`entrustType`, `sessionType`, `sensitivity`, and `action` — all locally-validated
closed sets in `algo.go` — these two only check for the empty string:

- `algo.go:218, 281, 307, 340, 386, 440, 469` — `ExchangeType == ""`
- `algo.go:239` — `EntrustBS == ""`

`types.ExchangeType` (`pkg/types/enums.go:119`) and `types.EntrustBS`
(`enums.go:135`) declare no `Valid()` method, so no such check exists anywhere.
The concrete risk: `ExchangeType: "Z"` sends a request naming a market the SDK
does not recognise; on `CancelOrder` that is a cancel the Gateway may resolve
against the wrong book.

## The valid sets (authoritative, from pkg/types/enums.go)

- `ExchangeType` (enums.go:122-129): `K` (HK), `P` (US), `v` (Shenzhen Connect),
  `t` (Shanghai Connect). Case-sensitive.
- `EntrustBS` (enums.go:138-145): `1` buy, `2` sell, `3` close-short, `4`
  open-short. **All four are valid** — a3's brief example used only 1/2, but
  3/4 are legitimate short-position directions and must not be rejected.

## Why this needs care

- **Forward compatibility.** ADR 0008's `EntrustType` explicitly documents that
  its set is non-exhaustive and "unknown values are forwarded unchanged". If the
  vendor adds an exchange code, a hard reject here would break a working request
  until the SDK is updated. The architect must decide whether to fail closed on
  the full 4-set, or fail open for unknown-but-nonempty with a documented
  rationale. A3's Finding B (targetStrategy, deliberately unvalidated) is the
  precedent for the fail-open choice; the exchange/direction sets are small and
  stable, which argues for fail-closed. **This is the judgement to make.**
- **A3 left a test that pins the bug.** `pkg/hstong/algo/validation_matrix_test.go`
  `TestUnvalidatedCodesAreForwardedToTheGateway` asserts the current fail-open
  behaviour and will fail under any fix. It must be inverted or replaced, exactly
  as A1's `assertReconnectNotice` had to be. A fix that flips the code and leaves
  that test green is impossible; a fix that flips it and does not update the test
  is incomplete.
- **The trade surface has the same `EntrustBS` gap** (`pkg/hstong/trade/orders.go:121`).
  Out of A6's scope by default; see A7.

## A6 acceptance

- Every `ExchangeType`/`EntrustBS` in algo is either validated against the 4-set or
  documented as intentionally forwarded, per the architect's decision.
- A3's pinning test updated to assert the decided behaviour.
- Existing `fullAddOrder()`-style well-formed fixtures still pass.
- ADR 0011 analysis recorded; if fail-closed, the commit body names it a
  behaviour change.
- `pkg/hstong/algo` stays at 100% and the full gate stays green.

## A7 (report-only, parallel)

Determine whether `pkg/hstong/trade/orders.go:121` and the other trade request
types that carry `EntrustBS` have the same unvalidated-direction gap, and report
it. Do **not** fix it in A7 — trade is a different released surface and a
behaviour change there needs its own decision. Output: a finding recorded for a
future task.
