# 0008 — Decimal-backed financial types

- Status: Accepted
- Date: 2026-09-23
- Supersedes: [0004 — Minimal dependency set](./0004-minimal-dependencies.md) (financial-types branch)

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

## References

- Supersedes: [0004 — Minimal dependency set](./0004-minimal-dependencies.md) (financial-types branch)
- Design: [DESIGN.md](../DESIGN.md) §7
- Run: [plan.md](../runs/2026-09-23-hstong-enterprise-sdk/plan.md) §F3, §E03
- Related: [0007 — Gateway HTTP bodies are plain JSON](./0007-http-json-codec.md) (wire bridge)
