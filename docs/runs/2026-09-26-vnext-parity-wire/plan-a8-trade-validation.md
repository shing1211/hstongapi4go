# A8 — Validate ExchangeType and EntrustBS in pkg/hstong/trade (plan)

## Decision (user, 2026-09-26)

"yes all of them" — fix the trade surface now, in the 0.1.x line, mirroring A6.
This is a behaviour change on a released surface, so ADR 0011 applies.

## The defect

`pkg/hstong/trade` set-validates **nothing**. After A6, the SDK is inconsistent
for `types.EntrustBS`:

| Surface | Policy |
|---|---|
| `pkg/hstong/future` | set-validated, rejects out-of-set (`future.go:158`) |
| `pkg/hstong/algo` | set-validated after A6 |
| `pkg/hstong/trade` | **emptiness only, or not checked at all** |
| `pkg/services` | validates only for HK stock/ETF (`trading.go:610`) — a fourth policy |

17 caller-reachable request sites. Full inventory with `file:line` is in
`finding-trade-code-validation.md`; read it first.

**8 sites are not even emptiness-checked**, four of them documented "required" —
`MarginFundInfo` with an empty `exchangeType` is sent today. A set check alone
would still send `exchangeType: ""`, so those 8 need an emptiness check added too.

## Valid sets (authoritative, pkg/types/enums.go)

- `ExchangeType` (122-129): `K`, `P`, `v`, `t`, case-sensitive.
- `EntrustBS` (138-145): `1`, `2`, `3`, `4`. **All four valid.** 3 = close-short,
  4 = open-short. A6's first pass treated "3" as invalid and that was wrong; a
  mutation must reproduce that mistake so it stays caught.

## Approach — mirror A6

Unexported `validExchange` / `validDirection` free functions in the trade package
(Go forbids a method on `types.ExchangeType` from another package, and
`pkg/types` is inside ADR 0011's protected surface). Shared message builders so
wording cannot drift across 17 sites. Keep the emptiness branch separate from the
set check everywhere, so a forgotten field reports "is required" rather than
something about four market codes.

`PositionsRequest` takes the optional-field shape (absent accepted, present-and-
unrecognised refused), matching A6's `QueryOrderList`.

## Traps

- **A test may pin the current behaviour.** A3 left exactly such a test in algo
  (`TestUnvalidatedCodesAreForwardedToTheGateway`) and A5 had to invert it. Look
  for the equivalent in trade before assuming a green suite means nothing is pinned.
- **`trade` has 100% coverage.** Any behaviour change must keep it there.
- Existing tests use `ExchangeType: types.ExchangeHK` fixtures throughout — those
  are valid and must keep passing.

## A8 acceptance

- All 17 sites validated, with the 8 unchecked ones given an emptiness check too.
- Shared message builders; no per-site wording drift.
- Every documented code still accepted, proven through the Manager **on the wire**.
- Any test pinning the old behaviour updated, and reported.
- ADR 0011 analysis recorded in
  `design-a8-trade-code-validation.md`.
- `pkg/hstong/trade` stays at 100%; full gate green.
- Mutation checks: removing a guard, and narrowing direction to buy/sell only.
