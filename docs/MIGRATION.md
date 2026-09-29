# Migration Guide

Breaking changes between major versions, and how to move between the two SDK
layers. For the complete history see [CHANGELOG.md](../CHANGELOG.md).

**Every Go sample in this document is compiled.** Each ```go block below is a
complete function from [`internal/migration/samples.go`](../internal/migration/samples.go),
which builds as part of `go build ./...` and is checked against this file by
`TestEverySampleInTheGuideIsPublished`. A sample that does not compile cannot be
published here, which is a stronger guarantee than any migration guide normally
offers — and it is one this repository needed, because the first draft of this
document contained a sample with a `{...}` placeholder and samples whose field
names did not exist.

The samples live under `internal/` rather than beside this file on purpose.
`mkdocs` copies every non-markdown file under its `docs_dir` into `site/`, which
would place a second copy of the package inside the module: `go list ./...` would
compile the samples twice, and the duplicate would run its own tests against a
guide path that does not exist there — so `go test ./...` would fail on any
machine that had built the documentation while CI stayed green. A documentation
build must not be able to create a Go package.

## What changes at v1.0

| | v0.x | v1.0 |
|---|---|---|
| Module path | `github.com/shing1211/hstongapi4go` | unchanged |
| Go version | 1.26+ | unchanged |
| `client.New()` | `(*Client, error)` | unchanged |
| `pkg/hstong/*` managers | supported | **deprecated**, still present and still supported |
| v-next layer | unreachable by a caller | **the recommended path** |
| Money on the wire | `string`, parsed by the caller | `domain.Money`, exact decimals |
| Push payloads | raw protobuf behind a failed type assertion | `domain.PushEvent`, never nil |
| Order mutation retries | never retried | **unchanged, permanently** ([ADR 0003](./adr/0003-no-auto-retry-orders.md)) |

**Nothing is removed at v1.0.** `pkg/hstong/*` keeps working and keeps its
signatures; it is marked deprecated in GoDoc so tooling surfaces it, and the
migration is available at your own pace. See
[ADR 0010](./adr/0010-vnext-layered-architecture.md) for the layering and
[ADR 0011](./adr/0011-v01x-compatibility.md) for what v0.1.x promised.

The deprecated *fields* are a separate matter from the deprecated *package*:
`HoldsVo.MarketValue` and its siblings are marked unreliable by the Gateway's own
field documentation, and the SDK repeats that rather than hiding it. They are
**not removed** in v1.0 either. See
[SPEC.md §10](./SPEC.md#10-deprecated-fields).

## The migration in four examples

Each block is a **complete function**, not an excerpt, so you can paste it and it
will build. Each is also compiled as part of `go build ./...`.

### 1. A REST call

**Before (v0.x)** — the released layer, a manager on `*client.Client`, and a
response of plain strings.

```go
func SampleReleasedBasicQot(ctx context.Context, c *client.Client) error {
	mkt := market.New(c)

	resp, err := mkt.BasicQot(ctx, market.BasicQotRequest{
		Security: []*dto.Security{{
			DataType: int32(types.DataTypeHKStock),
			Code:     "00700.HK",
		}},
	})
	if err != nil {
		return err
	}

	// A string. Converting it is the caller's problem, and a float64 is where
	// v0.1.x's money defect lived.
	_ = resp
	return nil
}
```

**After (v1.0)** — the v-next Stack, which composes the six services over one
request path and closes as a unit.

```go
func SampleVNextBasicQot(ctx context.Context, c *client.Client) error {
	stack := services.NewStack(c)
	defer func() { _ = stack.Close() }()

	resp, err := stack.Market.BasicQot(ctx, services.BasicQotRequest{
		Security: []*services.Security{{
			DataType: types.DataTypeHKStock,
			Code:     "00700.HK",
		}},
	})
	if err != nil {
		return err
	}

	// A decimal-backed Price. No ParseFloat, and nothing is rounded on the way
	// through, because a read-path price carries the zero tick.
	for _, q := range resp.BasicQot {
		if q != nil {
			fmt.Println(q.LastPrice.String())
		}
	}
	return nil
}
```

Two changes worth noticing. `dto.Security` becomes `services.Security`, so the
`int32` cast disappears — and you can no longer construct a value that will not
survive the round trip. And the price is a `domain.Price`, not a string.

### 2. Positions, and the deprecated fields

**Before (v0.x)** — `HoldsVo.MarketValue` and its siblings are marked deprecated
in the Gateway's own field documentation; the SDK repeats that in GoDoc.

```go
func SampleReleasedPositions(ctx context.Context, c *client.Client) error {
	trd := trade.New(c)

	positions, err := trd.Positions(ctx, trade.PositionsRequest{})
	if err != nil {
		return err
	}

	for _, p := range positions {
		// Deprecated and unreliable. Present, and not removed in v1.0.
		_ = p.MarketValue
		// The two fields that are actually usable.
		_ = p.EnableAmount
		_ = p.CurrentAmount
	}
	return nil
}
```

**After (v1.0)**

```go
func SampleVNextPositions(ctx context.Context, c *client.Client, accountID domain.AccountID) error {
	stack := services.NewStack(c)
	defer func() { _ = stack.Close() }()

	positions, err := stack.Account.HoldsList(ctx, accountID, services.HoldsFilter{})
	if err != nil {
		return err
	}

	for _, p := range positions {
		if p == nil {
			continue
		}
		// A Quantity is an exact decimal. Note that Quantity has no Add method -
		// Money does, because adding two Mights of different currencies is a
		// category error, while two Quantities of the same instrument are not.
		// Arithmetic goes through Decimal, so it stays exact.
		total := p.EnableAmount.Decimal().Add(p.CurrentAmount.Decimal())
		fmt.Println(total.String())
	}
	return nil
}
```

The deprecated fields have **no** counterpart, because the v-next mapper does not
populate them: a value the Gateway marks unreliable is not reconstructed as though
it were reliable.

### 3. Push

**Before (v0.x)** — the payload behind `Event` is still a raw protobuf message,
so the price is reached through two nested getters.

```go
func SampleReleasedPush(ctx context.Context, c *client.Client) error {
	s := stream.New(c)
	if err := s.Connect(ctx); err != nil {
		return err
	}
	defer func() { _ = s.Close() }()

	sub, err := s.Subscribe(ctx, types.TopicBasicQot, &dto.Security{
		DataType: int32(types.DataTypeHKStock),
		Code:     "00700.HK",
	})
	if err != nil {
		return err
	}

	for ev := range sub.Updates() {
		// A failed assertion is a runtime branch on a push goroutine, and the
		// payload behind it is still a raw protobuf message, so the price is
		// reached through two nested getters.
		if q, ok := ev.BasicQot(); ok {
			fmt.Println(q.GetBasicQot().GetLastPrice())
		}
	}
	return nil
}
```

**After (v1.0)**

```go
func SampleVNextPush(ctx context.Context, c *client.Client) error {
	adapter, err := transport.NewPushAdapter()
	if err != nil {
		return err
	}
	stack := services.NewStack(c, services.WithPushTransport(adapter))
	// One Close releases the push connection and the HTTP client together, in
	// that order, and returns the first error while attempting both.
	defer func() { _ = stack.Close() }()

	if err := stack.Push.Connect(ctx); err != nil {
		return err
	}
	runCtx, cancelRun := context.WithCancel(ctx)
	defer cancelRun()
	go func() { _ = stack.Push.Run(runCtx) }()

	sub, err := stack.Push.Subscribe(ctx, types.TopicBasicQot, &services.Security{
		DataType: types.DataTypeHKStock,
		Code:     "00700.HK",
	})
	if err != nil {
		return err
	}

	for u := range sub.Updates() {
		switch ev := u.Event.(type) {
		case domain.QuoteEvent:
			// A value type, so there is no failed assertion and no nil check.
			fmt.Println(ev.LastPrice.String())
		case domain.SystemEvent:
			// A frame the SDK could not classify is surfaced, not dropped.
			fmt.Println(ev.Code)
		}
	}
	return nil
}
```

Three differences. Push is **opt-in**: a Stack with no transport never dials, so
a REST-only caller does not acquire a socket it did not ask for. The `ok` bool is
gone, because a delivered update's `Event` is never nil — a frame the SDK cannot
classify arrives as a `domain.SystemEvent` rather than being dropped. And one
`Close` releases both resources.

### 4. Error handling

This one is worth doing regardless of which layer you are on, and it is the only
change here that does not wait for v1.0.

**Before** — compiles, and is wrong in a way no compiler can see.

```go
func SampleReleasedErrorMatching(err error) bool {
	return err != nil && strings.Contains(err.Error(), "1012")
}
```

**After**

```go
func SampleVNextErrorMatching(err error) bool {
	var gwErr *errs.Error
	if !errors.As(err, &gwErr) {
		return false
	}
	return gwErr.Code == types.StatusNotLoggedIn
}
```

The message text is not part of any contract, and a Gateway that rewords a message
breaks the string form silently. `errors.As` reaches the typed error through any
wrapping, which is the property the string form can never have. The code is a
named constant (`types.StatusNotLoggedIn`) rather than a literal.

## Checklist

- [ ] **Keep order mutations at one attempt.** The no-auto-retry policy is
      permanent ([ADR 0003](./adr/0003-no-auto-retry-orders.md)). If you retry at
      the application level, reconcile through `RealEntrustList` / `RealDeliverList`
      before resubmitting — an ambiguous failure may already have placed an order.
- [ ] **Replace `strings.Contains(err.Error(), ...)`** with `errors.Is` / `errors.As`
      against the typed `*errs.Error`, and with the named `types.StatusCode`
      constants.
- [ ] **Stop reading deprecated fields** if you can: `HoldsVo.MarketValue`,
      `LastPrice`, `IncomeBalance`, `MarketValueRate`, `IncomeRatio`, and
      `MarginFundInfo.HoldsBalance` / `MarketValue`. They are not removed, but the
      Gateway documents them as unreliable.
- [ ] **Move to `services.NewStack`** when convenient. It is opt-in, so adopting
      it endpoint by endpoint is fine.
- [ ] **If you use push**, enable it explicitly with
      `services.WithPushTransport`, and remember that a delivered `PushUpdate` has
      a non-nil `Event`.
- [ ] **Verify the platform public key** if `HSTONG_VERIFY_PUSH=true`. See
      [Security](./security.md) and [ADR 0005](./adr/0005-key-model-and-push-verification.md).
- [ ] **Build and test:** `go build ./... && go test ./...`.

## v0.1.x notes

v0.1.x is the current series. Its API is stable under
[ADR 0011](./adr/0011-v01x-compatibility.md), and since v0.1.21 no change under
`pkg/hstong/*`, `pkg/types`, `client/` or `internal/` has altered anything a
caller can observe.

### Upgrading from v0.1.0 to v0.1.1

No breaking changes. See [CHANGELOG.md](../CHANGELOG.md).

### Upgrading from pre-v0.1.0

Pre-v0.1.0 releases are not available; this section is for future reference.

## Reporting migration issues

If an upgrade gives you trouble, file an issue at
<https://github.com/shing1211/hstongapi4go/issues> with:

- the version you are upgrading from and to
- the error message or behaviour change you observed
- a minimal snippet that reproduces it
