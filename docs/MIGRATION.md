# Migration Guide

This document tracks breaking changes between major versions.

## v0.x to v1.0 (planned)

This section will be populated when v1.0 is released.

### Breaking changes anticipated

- [ ] **Module path.** The module path `github.com/shing1211/hstongapi4go` will
  remain stable.
- [ ] **Go version.** Go 1.26+ will be required.
- [ ] **Client construction.** `client.New()` currently returns `(*Client, error)`.
  This API is stable and will not change.
- [ ] **Manager construction.** `market.New()`, `trade.New()`, `future.New()`,
  `algo.New()` currently return a `*Manager`. These APIs are stable.
- [ ] **Streaming API.** `stream.New()`, `stream.Client.Subscribe()`,
  `stream.Subscription.Updates()`, and `stream.Subscription.Errors()` are stable.

### Migration checklist (draft)

When upgrading from v0.x to v1.0:

1. **Update Go version.** Ensure Go 1.26 or newer is installed:
   ```sh
   go version
   ```

2. **Update the module:**
   ```sh
   go get github.com/shing1211/hstongapi4go@v1.0.0
   go mod tidy
   ```

3. **Review deprecated fields.** Fields marked as deprecated in
   [SPEC.md](./SPEC.md#10-deprecated-fields) may be removed in v1.0. Replace
   usage of:
   - `HoldsVo.HoldsBalance` — use `HoldsVo.EnableAmount` or `HoldsVo.CurrentAmount`
   - `HoldsVo.MarketValue` — use market data quote for current valuation
   - `HoldsVo.LastPrice` — use market data quote for current price
   - `HoldsVo.IncomeBalance` — compute from market data quote minus cost price
   - `HoldsVo.MarketValueRate` — compute from market data
   - `HoldsVo.IncomeRatio` — compute from market data

4. **Review mutation retry behavior.** v1.0 will not change the no-auto-retry
   policy for mutations (ADR 0003). If you have implemented application-level
   retry logic for mutations, ensure it includes reconciliation (query
   `RealEntrustList` before retrying).

5. **Review error handling.** Ensure you use `errors.Is()` and `errors.As()`
   for error matching rather than string matching. The SDK never changes error
   text but may add error types.

6. **Update push verification.** If you have `HSTONG_VERIFY_PUSH=true`, verify
   that the platform public key has not rotated. See [Security](./security.md).

## v0.1.x notes

v0.1.x is the current alpha series. The API is not yet stable; expect
breaking changes between minor versions.

### Upgrading from v0.1.0 to v0.1.1

No breaking changes. See [CHANGELOG.md](../CHANGELOG.md).

### Upgrading from pre-v0.1.0

Pre-v0.1.0 releases are not available; this section is for future reference.

## Reporting migration issues

If you encounter issues during upgrade, please file an issue at
https://github.com/shing1211/hstongapi4go/issues with:

- The version you are upgrading from and to
- The error message or behavior change you observed
- A minimal code snippet that reproduces the issue
