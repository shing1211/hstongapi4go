# AGENTS.md

Repository guide for AI coding agents and assistants working in
`github.com/shing1211/hstongapi4go`, an idiomatic Go SDK for the HStong (华盛) Quant
OpenAPI local Gateway. Read this before making changes.

## Project facts

- **Module:** `github.com/shing1211/hstongapi4go`
- **License:** Apache-2.0
- **Go version:** 1.26+
- **Target:** the local HStong Gateway — HTTP `127.0.0.1:11111`, TCP push
  `127.0.0.1:11112`. The legacy direct-to-platform protocol is documented but **not
  implemented** ([ADR 0001](./docs/adr/0001-gateway-transport.md)).
- **Public surface:** `pkg/hstong/...`, `pkg/types`, `client`.
- **Generated code:** committed under `gen/`.

Every new hand-written Go file starts with the SPDX header:

```go
// Copyright 2026 shing1211
// SPDX-License-Identifier: Apache-2.0
```

Generated `*.pb.go` files carry their own generator header and must not be edited by
hand.

## Hard rules

1. **Never edit generated code under `gen/`.** Change the `.proto` sources or the
   codegen configuration (`buf.yaml`, `buf.gen.yaml`) and run `make proto`; committed
   generated code must match `make proto-verify`.
2. **Never auto-retry order mutations.** Trade, futures, and algo mutations issue
   exactly one attempt. Ambiguous failures require caller reconciliation. See
   [ADR 0003](./docs/adr/0003-no-auto-retry-orders.md).
3. **Money and quantities are `string` / `json.Number`, never `float64`.** See
   `docs/DESIGN.md` §7 and `make money-check`.
4. **No secrets committed.** The SDK holds no platform credentials and no developer
   private key; the bundled platform public keys are public reference data, not
   secrets. See [ADR 0005](./docs/adr/0005-key-model-and-push-verification.md).
5. **One canonical endpoint count.** Endpoint, topic, and schema counts come only
   from `docs/SPEC.md`. Do not hand-edit counts anywhere else.
6. **No dangling doc links.** If you reference a doc, create it in the same change.
   `make docs-check` runs `scripts/check_links.py` (every relative link in every
   `*.md` must resolve on disk) and `mkdocs build --strict` (every in-docs link
   must resolve), so a dangling relative link fails the build.
7. **The hybrid codec is per-endpoint.** Do not add a global codec mode or invent
   `.proto` files for undocumented HTTP bodies. See
   [ADR 0002](./docs/adr/0002-hybrid-codec.md).
8. **No new dependencies without an ADR.** stdlib first. See
   [ADR 0004](./docs/adr/0004-minimal-dependencies.md).

## Where things live

| Area | Location |
|------|----------|
| Design decisions | `docs/adr/` (index: [docs/adr/README.md](./docs/adr/README.md)) |
| Architecture overview | [docs/DESIGN.md](./docs/DESIGN.md) |
| Plan of record | [plan.md](./docs/runs/2026-09-21-hstong-full-surface/plan.md) |
| Run tracker | [todos.md](./docs/runs/2026-09-21-hstong-full-surface/todos.md) |
| Endpoint index (canonical) | `docs/SPEC.md` |
| Legacy protocol (not implemented) | `docs/LEGACY.md` |
| Vendored protos + provenance | `proto/` |
| Generated protobuf code | `gen/hstong/...` |
| Core client, options, env | `client/` |
| HTTP executor + codec dispatch | `internal/transport/` |
| Push framing + reconnect | `internal/push/` |
| Trade-password crypto | `internal/crypto/` |
| Typed errors + status codes | `internal/errs/` |
| Rate limit / retry / breaker | `internal/resilience/` |
| Logging + metrics | `internal/logging/`, `internal/metrics/` |
| Public managers | `pkg/hstong/` (`market`, `trade`, `future`, `algo`, `stream`) |
| Domain types + enums | `pkg/types/` |
| Mock Gateway | `test/mockgateway/`, `cmd/hstong-mock-gateway/` |
| Examples | `examples/` |
| Build and verification targets | `Makefile` |

## Build and verify

Use the Makefile targets:

```sh
make help            # list targets
make tools           # install/verify build tools
make build           # go build ./...
make fmt             # gofmt
make vet             # go vet ./...
make test            # unit tests
make test-race       # unit tests with -race -count=1
make test-integration  # env-gated real-Gateway tests (available from P12)
make coverage        # coverage report
make check           # fmt + vet + tests + money-check
make money-check     # reject float misuse of money fields
make proto           # regenerate gen/ from proto/ (available from P01)
make proto-verify    # fail if gen/ differs from proto/ (available from P01)
make docs-check      # markdown link check + mkdocs build --strict
make license         # apply SPDX headers
make license-check   # verify SPDX headers
make mock-gateway    # run the standalone mock Gateway (available from P09)
make clean           # remove build artifacts
```

Direct commands that must pass without credentials:

```sh
go build ./...
go vet ./...
gofmt -l .                 # must print nothing
go test ./...
go test -race -count=1 ./...
```

- Unit tests are offline and credential-free; they must not require network access.
- Generated code under `gen/` is excluded from the `gofmt` and linter checks.
- On Windows, if `go build ./...` fails with a file-lock error on `a.out.exe`, set
  `GOTMPDIR` to a writable, non-scanned directory and retry. This is a host
  antivirus/indexing issue, not a code problem.

### Binary hygiene (Windows / endpoint AV)

- Never `go build -o <name>.exe` inside the repository. A committed-output build
  leaves an unsigned binary in the tree (masked by `.gitignore`) and endpoint
  antivirus heuristics flag unsigned Go binaries that import `crypto/*`
  (`crypto/aes`, `crypto/rsa`, `crypto/sha1`) as suspicious.
- Use `go test`, `go vet`, and `go run` (which place transient binaries under
  `GOCACHE`/`GOTMPDIR`). For any scratch program, write it under
  `GOTMPDIR` (the pre-approved temp directory), not the repo, and delete it after.
- To exercise the standalone mock Gateway, prefer `go run ./cmd/hstong-mock-gateway`
  over building a local `.exe`.

## Conventions

- **GoDoc.** Every exported identifier has a comment starting with the identifier
  name and explaining behavior, including edge cases and defaults.
- **Errors.** Use `internal/errs` typed errors. Never match on error strings in
  library code; keep `errors.Is` / `errors.As` traversal working.
- **Options.** Follow `Option func(*Config)` with `WithX` constructors applied in
  order over defaults.
- **Context.** Every blocking call takes `context.Context` first. The client is safe
  for concurrent use; `Close` is idempotent and leak-free.
- **Streaming.** `Updates()` / `Errors()` channels; cancel by context or `Close`;
  reconnect re-subscribes all active topics.
- **Tests.** Prefer table-driven tests; add `_test.go` coverage for behavior changes;
  integration tests are env-gated and skipped by default.
- **Comments.** Explain intent, invariants, and non-obvious decisions; do not restate
  the code.

## Commit style

Conventional Commits referencing the run and task IDs (for example
`docs(adr): add ADR 0002 hybrid codec (T01)`). `Signed-off-by` trailer via
`git commit -s`. Do not commit unless explicitly asked.
