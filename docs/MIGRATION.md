# Migration Guide

This document tracks breaking changes between major versions. For a complete
changelog, see [CHANGELOG.md](../CHANGELOG.md).

## v0.x to v1.0 (planned)

This section will be populated when v1.0 is released. All items below represent
current intentions; the final v1.0 release may include additional changes.

### Breaking changes anticipated

| Item | v0.x | v1.0 (planned) | Impact |
|------|-------|------------------|--------|
| Module path | `github.com/shing1211/hstongapi4go` | unchanged | Low |
| Go version | 1.26+ | 1.26+ unchanged | None |
| `client.New()` signature | `(*Client, error)` | unchanged | None |
| Manager constructors | `*Manager` return | unchanged | None |
| Streaming API | `Updates()`/`Errors()` channels | unchanged | None |
| Deprecated fields | `HoldsVo.*` fields present | May be removed | Medium |

### Before/after: removing deprecated field usage

**Before (v0.x):**
```go
position, err := tradeMgr.Positions(ctx, req)
// Accessing deprecated field:
fmt.Println(position.MarketValue)   // unreliable — do not use
```

**After (v1.0):**
```go
position, err := tradeMgr.Positions(ctx, req)
// Use EnableAmount or CurrentAmount for quantity
fmt.Println(position.EnableAmount, position.CurrentAmount)
// For valuation, fetch market quote:
quote, _ := marketMgr.BasicQot(ctx, market.BasicQotRequest{...})
```

**Before (v0.x):**
```go
funds, err := tradeMgr.MarginFundInfo(ctx, req)
fmt.Println(funds.HoldsBalance)    // deprecated — do not use
```

**After (v1.0):**
```go
funds, err := tradeMgr.MarginFundInfo(ctx, req)
// Use market value from positions + cash from enableBalance
for _, p := range positions {
    // compute total market value from quotes
}
```

### Before/after: error handling

**Before (v0.x):**
```go
if strings.Contains(err.Error(), "1012") {
    // String matching — fragile
    session.Login(ctx)
}
```

**After (v1.0):**
```go
var gwErr *errs.Error
if errors.As(err, &gwErr) && gwErr.Code == "1012" {
    // Typed error matching — robust
    session.Login(ctx)
}
```

### Migration checklist

When upgrading from v0.x to v1.0:

- [ ] **Update Go version.** Ensure Go 1.26 or newer:
  ```sh
  go version
  ```

- [ ] **Update the module:**
  ```sh
  go get github.com/shing1211/hstongapi4go@v1.0.0
  go mod tidy
  ```

- [ ] **Replace deprecated field usage.** Fields in `HoldsVo` and `MarginFundInfo`
  marked deprecated in [SPEC.md](./SPEC.md#10-deprecated-fields). See
  before/after examples above.

- [ ] **Audit error handling.** Replace any `strings.Contains(err.Error(), "...")`
  patterns with `errors.Is()` / `errors.As()` using the typed `*errs.Error`
  (accessible via `errors.As`).

- [ ] **Review mutation retry logic.** The no-auto-retry policy is permanent
  (ADR 0003). Application-level retry must include reconciliation via
  `RealEntrustList` / `RealDeliverList` before resubmitting.

- [ ] **Verify push keys.** If `HSTONG_VERIFY_PUSH=true`, confirm the platform
  public key has not rotated. See [Security](./security.md).

- [ ] **Run tests.** Execute `go test ./...` and `go build ./...` to catch
  any compile-time deprecation warnings.

## v0.1.x notes

v0.1.x is the current alpha series. The API is not yet stable; expect
breaking changes between minor versions.

### v0.1.2 changes (from v0.1.1)

- Removed stale ProtoJSON documentation references (was never in public API)
- No breaking changes to user-facing APIs

### Upgrading from v0.1.0 to v0.1.1

No breaking changes. See [CHANGELOG.md](../CHANGELOG.md).

### Upgrading from pre-v0.1.0

Pre-v0.1.0 releases are not available; this section is for future reference.

## Reporting migration issues

If you encounter issues during upgrade, file an issue at
<https://github.com/shing1211/hstongapi4go/issues> with:

- The version you are upgrading from and to
- The error message or behavior change you observed
- A minimal code snippet that reproduces the issue
