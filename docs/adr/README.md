# Architecture Decision Records

Short, dated records of decisions that shape this project. Each records the
context, the decision, and its consequences. Superseding a decision means adding a
new ADR, not editing the old one.

| ADR | Title | Status | Summary |
|-----|-------|--------|---------|
| [0001](./0001-gateway-transport.md) | Target the local Gateway (HTTP + TCP push) | Accepted | The SDK speaks to the local HStong Gateway; the legacy direct-to-platform protocol is documented but not implemented. |
| [0002](./0002-hybrid-codec.md) | Hybrid codec: generated protobuf + hand-written JSON | Accepted | Generated proto types for push payloads and market DTOs, hand-written `encoding/json` structs for trade/futures/algo/assets/session HTTP bodies. |
| [0003](./0003-no-auto-retry-orders.md) | Never auto-retry order mutations | Accepted | Order/futures/algo mutations issue exactly one attempt; ambiguous failures require caller reconciliation. |
| [0004](./0004-minimal-dependencies.md) | Minimal dependency set | Accepted | stdlib-first; the protobuf runtime is the only forced runtime dependency; anything new needs an ADR. |
| [0005](./0005-key-model-and-push-verification.md) | Gateway key model and opt-in push verification | Accepted | RSA key handling lives in the Gateway, so the SDK does no signing; push `bodySHA1` verification is opt-in and off by default. |
| [0006](./0006-test-dependencies.md) | Test-only dependencies | Accepted | `go.uber.org/goleak` is allowed as a test-only dependency for goroutine-leak verification. |
| [0007](./0007-http-json-codec.md) | Gateway HTTP bodies are plain JSON | Accepted | Gateway HTTP bodies decode with `encoding/json` over typed structs (`int64` is a JSON number); `protojson` is a per-endpoint fallback only and TCP push stays binary protobuf. |
| [0008](./0008-decimal-financial-types.md) | Decimal-backed financial types | Accepted | `shopspring/decimal` at runtime in `pkg/domain/`; wire layer stays `string`/`json.Number`; `make money-check` updated. |
| [0009](./0009-opentelemetry-observability.md) | OpenTelemetry observability | Accepted | OTel `otel` build tag; no-op default; tracer/meter injectable via `Client` options; full span + metric coverage of auth, HTTP, push, orders. |
| [0010](./0010-vnext-layered-architecture.md) | Additive v-next layered architecture | Accepted | New `pkg/domain`, `pkg/services`, `pkg/transport`, `internal/auth` layer; v0.1.x surface stays stable. |
| [0011](./0011-v01x-compatibility.md) | v0.1.x compatibility guarantees | Accepted | No breaking type or wire changes to `pkg/hstong/*`, `pkg/types`, `client/`, `internal/*`; mock Gateway and e2e tests unchanged. |
| [0012](./0012-ci-secret-scanning.md) | CI-only secret scanning | Accepted | `gitleaks` runs as a pinned CI-only Action over the full history, with a path allowlist for the public platform keys, vendored protos, and the published AES test vector; `go.mod` is untouched. |

Template:

```markdown
# NNNN — Title
- Status: Proposed | Accepted | Superseded by NNNN
- Date: YYYY-MM-DD

## Context
## Decision
## Consequences
## Alternatives considered
## References
```
