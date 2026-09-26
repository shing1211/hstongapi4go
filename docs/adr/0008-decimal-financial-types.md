# 0008 — Decimal-backed financial types

- Status: Accepted
- Date: 2026-09-23
- Supersedes: [0004 — Minimal dependency set](./0004-minimal-dependencies.md) (financial-types branch)
- Amended: 2026-09-26 — see the *Amendment* section below

## Context

[ADR 0004](./0004-minimal-dependencies.md) adopted `string` / `json.Number` for all
money, price, quantity, and rate fields — a correct decision for the v0.1.x wire layer,
because the Gateway sends these as unquoted JSON numbers and the stdlib `json.Decoder`
can read them directly into `string` fields without precision loss.

The v-next domain layer (`pkg/domain`) operates one level above the wire: it holds
business invariants, enforces scale and rounding, and exposes typed constructors to
callers. For a financial SDK, `string` is insufficient at this layer because:

- Go `float64` arithmetic on financial values produces rounding errors (e.g.
  `0.1 + 0.2 != 0.3`). Relying on `string` alone defers the problem to callers, who
  then must implement decimal arithmetic themselves.
- Business rules such as "price must be a multiple of the tick size" require exact
  division, which `float64` cannot guarantee.
- HKEX lot sizes, tick schedules, and contract multipliers are small integers; a
  decimal type with explicit scale avoids implicit float promotion.
- External integrations (accounting, risk systems) expect `decimal.Decimal` values.

## Decision

Introduce `github.com/shopspring/decimal` as a runtime dependency, confined to
`pkg/domain/`. All financial types in the domain layer are backed by
`decimal.Decimal` with explicit scale and rounding mode.

### Money type

```go
// Money represents a monetary value in a given currency with explicit decimal scale.
// It is immutable. All arithmetic returns a new Money.
type Money struct {
    dec     decimal.Decimal
    currency string          // ISO 4217, e.g. "HKD", "USD"
}
```

Constructors enforce scale and rounding: `MustNewMoney(value string, currency string, scale uint8, rounding decimal.RoundingMode)`. A `String()` method returns the canonical string form suitable for wire serialization.

### Price type

```go
// Price represents a security price with a defined tick size and scale.
type Price struct {
    dec     decimal.Decimal
    tick    decimal.Decimal  // minimum price increment
}
```

`Validate(step bool)` checks the price is a non-negative multiple of `tick`; `Round(tick)` rounds to the nearest valid tick.

### Quantity type

```go
// Quantity represents a share or contract count. Lot sizes are enforced at the service layer.
type Quantity struct {
    dec decimal.Decimal
}
```

`Validate(lot uint64)` checks `qty % lot == 0` for integer lots or allows any decimal for odd-lot/fractional.

### Rate type

```go
// Rate represents a ratio: interest rate, margin ratio, P/E, etc.
type Rate struct {
    dec decimal.Decimal
}
```

Plain decimal; constructed from string or `decimal.Decimal` directly.

### Wire bridge

The transport layer (`pkg/transport`) continues to use `string` / `json.Number` for
wire serialization (per ADR 0004 and 0007). Domain types provide `MarshalJSON()` /
`UnmarshalJSON()` that emit and parse the canonical string, so no changes to the wire
protocol or existing client types.

```go
func (m Money) MarshalJSON() ([]byte, error)
func (m *Money) UnmarshalJSON([]byte) error   // delegates to MustNewMoney
```

The existing `make money-check` script is updated to allow `decimal.Decimal` fields
in `pkg/domain/` while rejecting `float64` anywhere in the SDK.

### Scale table

| Field | Typical scale | Rounding |
|-------|--------------|----------|
| Price | 3 (0.001) | `decimal.RoundDown` |
| Money (HKD) | 3 | `decimal.RoundHalfUp` |
| Money (CNY) | 2 | `decimal.RoundHalfUp` |
| Quantity (shares) | 0 | `decimal.RoundDown` |
| Quantity (futures) | 4 | `decimal.RoundDown` |
| Rate | 6 | `decimal.RoundHalfUp` |
| Turnover / volume | 3 | `decimal.RoundDown` |

Scale is not hard-coded in the type; callers or domain constructors specify it. The
table above documents typical values for documentation and tests.

## Consequences

- `pkg/domain` carries `shopspring/decimal` at runtime; the rest of the SDK (wire,
  transport, client) remains stdlib + protobuf only.
- Existing `pkg/hstong/*` and `pkg/types` use `string` / `json.Number` unchanged;
  bridge functions in `pkg/transport` convert between wire strings and domain decimals.
- `make money-check` is updated to allow `decimal.Decimal` in `pkg/domain/` only.
- Financial arithmetic in domain services (P01) uses `decimal.Decimal` methods directly.
- Tests use `decimal.RequireFromString` for fixture construction.
- The dependency is listed in `go.mod` with a comment referencing this ADR.

## Alternatives considered

| Alternative | Why rejected |
|-------------|--------------|
| Keep all financial values as `string` and push decimal arithmetic to callers | Shifts the burden and error surface to every consumer; not viable for an enterprise SDK. |
| Use `int64` storing the smallest unit (cents / 0.001) | Works for prices/quantities but breaks for rates (which have negative exponents) and requires callers to know the unit. Does not generalize. |
| Use `float64` with rounding helpers | `float64` precision errors are well-documented; no way to detect or correct them without a decimal type anyway. |
| `github.com/ericlagergren/decimal` | Same interface; `shopspring/decimal` is more widely used in the Go ecosystem and has stable APIs. |

## Amendment — 2026-09-26: one declared `float64` exception

### The conflict

The Decision above states that `make money-check` rejects `float64` **anywhere in
the SDK**, and that consequence is unqualified: the guard was written to allow
`decimal.Decimal` in `pkg/domain/` and to reject floats everywhere else. One
field in the released v0.1.x surface does not satisfy that rule, and no ADR
granted it an exemption:

```go
// pkg/hstong/market/market.go
// TickSize is the minimum price step ("spreadLevel" on the wire). It is a
// price tick, so it stays a float64.
TickSize float64 `json:"spreadLevel"`
```

The field's own comment claims a local exception that no record supports. This
was not an oversight but a decision with a receipt: the run that implemented
`pkg/hstong/market` named the exported field `TickSize` rather than
`SpreadLevel` *specifically so that* `check_money.py`, which then judged Go
field names only, would not flag it
(`docs/runs/2026-09-21-hstong-full-surface/evidence/P03-T12-T13.txt` §1). The
field was therefore never in violation of the guard as the guard was then
written; it was a violation of the *decision*, hidden by a gap in the
enforcement. The json-tag rule added on 2026-09-26 closed that gap and the
conflict became visible, which is the correct outcome: a guard that only holds
because the thing it forbids was renamed is not a guard.

Three ADRs bear on the resolution and they do not agree:

| ADR | Bearing |
|-----|---------|
| [0008](./0008-decimal-financial-types.md) (this one) | `float64` money fields are rejected everywhere in the SDK |
| [0011](./0011-v01x-compatibility.md) guarantee 1 | No existing exported type signature in `pkg/hstong/*` may be modified; `float64` → `json.Number` on an exported field is a breaking change |
| [0011](./0011-v01x-compatibility.md) §Consequences | `pkg/domain`, `pkg/services`, `pkg/transport`, `internal/auth` carry no compatibility guarantee until v1.0 |

### Decision

**`TickSize float64` is a single, declared, dated exception, recorded in the
guard itself rather than resolved in the field, and it is deleted at v1.0.0.**

`scripts/check_money.py` gains a `WAIVERS` table whose one entry names the
complete declaration site — repository-relative path, Go field name, JSON wire
key, and the exact declared float type — and all four must match. The exception
is therefore narrower than a path exclusion: a new float money field in the same
file, a retyped `float32` on the same field, or the same field name in another
file of the same package are each still reported. The waived field is printed on
every run, including runs that fail for another reason, so the suppression is
visible in the build log rather than being indistinguishable from a guard that
never ran.

**The exception is removed when v1.0.0 is cut** (run task G1). That release
retires [ADR 0011](./0011-v01x-compatibility.md) guarantee 1 and the v0.1.x
deprecation path with it, so the promise the waiver exists to honour no longer
exists. The field becomes `json.Number` — the same change the v-next twin at
`pkg/services` already carries — and the `WAIVERS` entry is deleted outright,
with nothing replacing it. If the field change is deferred past v1.0.0, the
waiver stays and the entry must be re-dated rather than left to rot.

### Why the field is not wrong, merely unread

The field is typed `float64` because the Gateway sends this value as an
unquoted JSON `double` and the SDK only ever reads it: it is never re-emitted,
never summed, and never rounded, so the silent precision loss this ADR exists to
prevent is not reachable through it. The value is read-only to a consumer, who
is free to read it as an `int64` of ten-thousandths, a `float64`, or a string.

### Why not the alternatives

| Alternative | Why rejected |
|-------------|--------------|
| Change the field to `json.Number` now | Breaches [ADR 0011](./0011-v01x-compatibility.md) guarantee 1, which is an explicit prohibition on modifying an exported type signature in `pkg/hstong/*`. A guard is not a licence to break a published API, and the v-next twin already carries the correct type, so nothing is lost by waiting. |
| Suppress the field in the guard silently — a path or package exclusion, or a `#nosec`-style inline marker | This is the option that has to be rejected outright. Suppressing in the guard without recording it reproduces exactly the evasion the json-tag rule was written to end, one level up: the next reviewer sees a green build and an ADR that says the opposite. An unnamed exception is indistinguishable from a bug in the guard, and it never expires. |
| Rename the wire-key-hiding Go field back, or delete the field | A rename does not change the type, so it satisfies neither ADR. Deleting an exported field is a larger breach than leaving it. |
| Revert or weaken the json-tag rule | The rule is correct and the field is the thing that does not fit it. Reverting it would hide the next `float64` behind an innocuous Go name too, which is how this one hid. |
| Add a general "structural metadata" category to the guard — a way for a field to declare itself non-money and be exempt | This is an escape hatch with a nicer name. It moves the decision from this ADR, where it is dated and reviewable, to each field that wants one, where it is not. Rejected on the same reasoning as [ADR 0012](./0012-ci-secret-scanning.md): an allowlist may be narrow, justified, and dated, but it must never become a rule class that any future field can opt into silently. |
| Wait for v1.0 without waiving, and leave `make check` red until then | Leaves CI failing for a run to close over a decision that is already taken, and pressures the next agent into the undocumented suppression. The waiver is the dated version of the same wait. |

### Consequences

- `make money-check` and the CI `build` job pass again, and the one exception is
  stated in three places that a reviewer will read together: this amendment, the
  `WAIVERS` block in `scripts/check_money.py`, and the money-check output.
- The Decision section above is left unedited. The exception is additive and
  expires; superseding the rule outright would delete the standard the waiver is
  measured against.
- v1.0.0 carries a follow-on obligation, tracked as part of G1: change
  `OrderBookResponse.TickSize` to `json.Number` and delete the `WAIVERS` entry.
  If that does not happen, this ADR is wrong and the entry must be re-dated.

## References

- Supersedes: [0004 — Minimal dependency set](./0004-minimal-dependencies.md) (financial-types branch)
- Design: [DESIGN.md](../DESIGN.md) §7
- Run: [plan.md](../runs/2026-09-23-hstong-enterprise-sdk/plan.md) §F3, §E03
- Related: [0007 — Gateway HTTP bodies are plain JSON](./0007-http-json-codec.md) (wire bridge)
- Amended by run: [plan.md](../runs/2026-09-26-vnext-parity-wire/plan.md) (task P3),
  [design-tick-model.md](../runs/2026-09-26-vnext-parity-wire/design-tick-model.md)
- Related: [0011 — v0.1.x compatibility guarantees](./0011-v01x-compatibility.md),
  [0012 — CI-only secret scanning](./0012-ci-secret-scanning.md) (allowlist discipline)
