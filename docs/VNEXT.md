# Adopting the v-next layer

- **Status:** decision draft. No code changed; nothing here is released.
- **Purpose:** the forcing function for the N1 decision recorded in
  `runs/2026-09-25-hstong-agent-readiness/next-phase.md` §1.
- **Reader:** whoever decides whether this SDK ships one layered API or the
  released flat API.

## 1. The question

The repository contains two implementations of the same client:

| | Released layer | v-next layer |
|---|---|---|
| Packages | `client`, `pkg/hstong/*`, `internal/*` | `pkg/domain`, `pkg/services`, `pkg/transport`, `internal/auth` |
| Money | `string` / `json.Number` | `decimal.Decimal` (`Money`, `Price`, `Quantity`, `Rate`) |
| Errors | `internal/errs` typed status errors | same, reused |
| Reachable by a caller | **yes** | **no** — only tests and the coverage gate import it |
| ADR | [0001](./adr/0001-gateway-transport.md), [0007](./adr/0007-http-json-codec.md) | [0008](./adr/0008-decimal-financial-types.md), [0010](./adr/0010-vnext-layered-architecture.md) |

[ADR 0011](./adr/0011-v01x-compatibility.md) guarantees no breaking type or
wire changes to `pkg/hstong/*`, `pkg/types`, `client/`, or `internal/*` across
v0.1.x. Wiring v-next in as the default would break that guarantee. Wiring it in
as opt-in would leave two APIs shipping side by side indefinitely.

## 2. What the migration would look like

One representative endpoint, account balance, in both layers.

**Today (released):**

```go
c, err := client.New(client.WithEnv(""))
tradeMgr := trade.NewManager(c)

bal, err := tradeMgr.MarginFundInfo(ctx, trade.MarginFundInfoRequest{})
if err != nil {
    return err
}
// bal.AssetBalance is a string such as "125000.500".
total, err := strconv.ParseFloat(bal.AssetBalance, 64)
```

**With v-next:**

```go
c, err := client.New(client.WithEnv(""))
svc := services.NewAccountService(c)

bal, err := svc.MarginFundInfo(ctx, services.MarginFundInfoParams{})
if err != nil {
    return err
}
// bal.AssetBalance is a domain.Money backed by decimal.Decimal.
total := bal.AssetBalance.Add(bal.MarketValue).Decimal()
```

That is the whole migration for one call, and it is where the cost becomes
visible: **every arithmetic site in caller code changes**, because `string` plus
`strconv.ParseFloat` becomes a decimal type. It is also strictly better — the
released form rounds through `float64`, which is the exact class of bug
[ADR 0008](./adr/0008-decimal-financial-types.md) was written to eliminate.

## 3. What writing it actually revealed

Four obstacles, none of which is a bug and all of which are design consequences.

1. **There is no incremental path.** Adoption is per endpoint family, not per
   call. A caller can move account reads and keep trade reads, but then holds two
   error conventions, two pagination styles, and two money representations in one
   program. The layers do not compose at call granularity.

2. **No bridging adapter exists.** A shim that accepts `pkg/hstong` requests and
   returns `pkg/domain` values would make adoption incremental, and it does not
   exist. Writing one is roughly the same work as the migration itself, which
   means the "gradual path" argument does not reduce cost — it relocates it.

3. **The push layer has no single answer.** `internal/push` contains three
   implementations — `Client` (released, used by `pkg/hstong/stream`),
   `Manager`, and `Fanout` — which disagree about reconnect bounds, backoff, and
   freshness. Adopting v-next push means choosing one, and the choice is a
   rewrite of the stream surface, not a swap.

4. **`pkg/services` bypasses its own adapter.** It imports `client` directly, so
   `pkg/transport.Adapter`'s correlation IDs and retry classification sit unused
   on the very path the layer was built for. Adopting v-next as-is would adopt
   that bypass too, unless the services are rewired first.

## 4. Recommendation

**Do not wire v-next in as the default within v0.1.x.** ADR 0011 forbids it and
the migration above is a flag-day change per endpoint family.

The decision that remains is narrower and should be made explicitly:

- **Option A — commit to the layered API.** Write the migration guide as a real
  document, schedule a v1.0 with breaking type changes, rewire `pkg/services`
  through `pkg/transport.Adapter`, and consolidate the push implementations
  first. This is a multi-release programme, not a patch.
- **Option B — delete the v-next layer.** Keep the released stack, delete
  `pkg/domain`, `pkg/services`, `pkg/transport`, and `internal/auth`, and record
  the decimal-money and layering decisions as superseded. The SDK keeps one
  implementation, and the money-precision problem is addressed inside the
  released types instead.
- **Option C — leave it unwired (status quo).** Acceptable only with a deadline
  and an owner. Without one, the next maintainer cannot tell which
  implementation is authoritative, and the deviations table in
  `runs/2026-09-23-hstong-enterprise-sdk/ARCHITECTURE.md` will drift further from
  the truth.

For what it is worth, the evidence gathered while writing this document points
toward **Option A being the better product** — decimal money is a real
correctness win, and the push problems have to be solved regardless — but it is
not a small decision, and nothing in the current release obliges anyone to make
it under time pressure.

## 5. What would make this decision cheap to revisit

Whichever option is chosen, three things would reduce the cost of changing it
later:

1. A `depguard` rule making the intended boundary machine-enforced, so drift is
   caught by CI rather than by review.
2. A published status for each of the three push implementations, so the choice
   is documented rather than inferred from which file a symbol lives in.
3. `internal/auth`'s documented-but-absent backoff, single-flight, and refresh
   either implemented or removed from `doc.go` (R14).
