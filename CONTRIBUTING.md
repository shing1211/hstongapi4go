# Contributing to hstongapi4go

Thanks for your interest in contributing. This project is an unofficial,
community-developed Go SDK for the HStong (华盛) Quant OpenAPI local Gateway.
By contributing you agree that your contributions are licensed under the
[Apache License 2.0](./LICENSE) and you certify the DCO sign-off described
below.

- [Security Policy](./SECURITY.md)
- [Disclaimer](./DISCLAIMER.md)
- [AGENTS.md](./AGENTS.md) — the authoritative repository guide for AI agents
  and contributors

---

## Getting Started

### Prerequisites

- Go 1.26+
- `git`
- GNU `make` + `bash` for the Makefile targets (Git Bash or WSL on Windows;
  `cmd.exe`/PowerShell are not supported for `make`)
- Python 3 (for `scripts/check_money.py` and `scripts/check_links.py`)
- `buf` and `protoc-gen-go` only if you regenerate protobuf code (`make tools`)
- `mkdocs` / `mkdocs-material` only if you build the docs site
  (`pip install -r requirements-docs.txt`)

### Setup

```bash
git clone https://github.com/shing1211/hstongapi4go
cd hstongapi4go

# Install the codegen tools used by the Makefile.
make tools

# Format, vet, check money types, and run the unit tests.
make check
```

The repository compiles and tests offline with no credentials:

```bash
go build ./...
go vet ./...
gofmt -l .                 # must print nothing
go test ./...
go test -race -count=1 ./...
```

---

## Repository Layout

```
client/            Core client: options, env, routes, codecs, Close
pkg/hstong/        Public managers: session + market/trade/future/algo/stream
pkg/types/         Enums, status codes, platform public keys
internal/          Private implementation (transport, push, crypto, errs, ...)
gen/               Generated protobuf code — DO NOT EDIT
proto/             Vendored .proto sources + provenance
test/mockgateway/  Offline mock HTTP + TCP push server
test/e2e/          All-endpoint SDK-to-mock tests
test/integration/  Env-gated real-Gateway tests (skipped by default)
cmd/               Standalone binaries (hstong-mock-gateway)
examples/          Runnable examples
scripts/           Build, link, money, and proto verification
docs/              MkDocs site, SPEC, ADRs, DESIGN, LEGACY
```

The public surface is `pkg/hstong/...`, `pkg/types`, and `client`. Nothing
outside the module may import `internal/`.

---

## Makefile Targets

```sh
make help              # list targets
make tools             # install/verify build tools
make build             # go build ./...
make fmt               # gofmt
make vet               # go vet ./...
make test              # unit tests
make test-race         # unit tests with -race -count=1
make test-integration  # env-gated real-Gateway tests (HSTONG_* required)
make coverage          # coverage report
make check             # fmt + vet + money-check + tests
make money-check       # reject float misuse of money fields
make proto             # regenerate gen/ from proto/
make proto-verify      # fail if gen/ differs from proto/
make docs-check        # markdown link check + mkdocs build --strict
make license           # apply SPDX headers
make license-check     # verify SPDX headers
make mock-gateway      # run the standalone mock Gateway
make clean             # remove build artifacts
```

CI runs `make money-check`, `go build`, `gofmt`, `go vet`, and the unit and
race test suites. A change is not ready until those pass locally too.

---

## Hard Rules

1. **SPDX header.** Every new hand-written Go file starts with:

   ```go
   // Copyright 2026 shing1211
   // SPDX-License-Identifier: Apache-2.0
   ```

2. **Never edit generated code.** Nothing under `gen/` is edited by hand.
   Change the `.proto` sources or `buf.yaml` / `buf.gen.yaml` and run
   `make proto`; committed generated code must match `make proto-verify`.

3. **Never auto-retry order mutations.** Trade, futures, and algo mutations
   issue exactly one attempt. An ambiguous failure requires caller
   reconciliation, never an automatic retry. See
   [ADR 0003](./docs/adr/0003-no-auto-retry-orders.md).

4. **Money and quantities are `string` / `json.Number`, never `float64`.** See
   [docs/DESIGN.md](./docs/DESIGN.md) §7 and `make money-check`.

5. **No new dependencies without an ADR.** Standard library first. See
   [ADR 0004](./docs/adr/0004-minimal-dependencies.md).

6. **No secrets committed.** The SDK holds no platform credentials and no
   developer private key. The bundled platform public keys are public reference
   data, not secrets. See [ADR 0005](./docs/adr/0005-key-model-and-push-verification.md)
   and [SECURITY.md](./SECURITY.md).

7. **No dangling doc links.** If you reference a document, create it in the same
   change. `make docs-check` enforces relative links.

8. **Counts come from `docs/SPEC.md`.** Endpoint and topic counts are canonical
   in [docs/SPEC.md](./docs/SPEC.md); do not hand-edit them elsewhere.

---

## Code Style

- **Formatting.** `gofmt` (enforced by CI). Generated code is excluded.
- **GoDoc.** Every exported identifier has a comment starting with the
  identifier name, covering behavior, defaults, and edge cases.
- **Errors.** Use the typed errors in `internal/errs`; never match on error
  strings in library code, and keep `errors.Is` / `errors.As` traversal working.
- **Options.** Functional options (`Option func(*Config)`, `WithX` constructors)
  applied in order over defaults.
- **Context.** Every blocking call takes `context.Context` first.
- **Concurrency.** A `*client.Client` and every manager are safe for concurrent
  use; `Close` is idempotent and leak-free.
- **Streaming.** `Updates()` / `Errors()` channels; cancel by context or
  `Close`; reconnect re-subscribes all active topics.
- **Tests.** Prefer table-driven tests; add `_test.go` coverage for behavior
  changes. Only the `internal/` and `pkg/` packages may import `internal/`.

---

## Testing

| Test type | Location | Runs in CI | Needs credentials |
|-----------|----------|------------|-------------------|
| Unit tests | `*_test.go` alongside source | Yes | No |
| SDK-to-mock e2e | `test/e2e/` | Yes | No |
| Mock server tests | `test/mockgateway/` | Yes | No |
| Integration tests | `test/integration/` | No | Yes (skipped by default) |

Unit tests are offline and credential-free; they must never require network
access. Integration tests against a real Gateway are gated on
`HSTONG_INTEGRATION=1` and are skipped otherwise. See
[`test/integration/README.md`](./test/integration/README.md) for the full
environment matrix and run commands.

---

## Commits

Use [Conventional Commits](https://www.conventionalcommits.org/) referencing the
run and task IDs, for example:

```
docs(adr): add ADR 0002 hybrid codec (T01)
feat(market): add BasicQot manager (T12)
fix(session): count keep-alive requests client-side (T11)
```

All commits must be DCO-signed:

```bash
git commit -s -m "feat(market): add BasicQot manager (T12)"
```

This adds a `Signed-off-by: Your Name <you@example.com>` trailer. Commits
without a sign-off will not be merged.

---

## Documentation

Public documentation lives under `docs/` and is built with MkDocs Material:

```bash
pip install -r requirements-docs.txt
make docs-check            # link check + mkdocs build --strict
```

Internal run artifacts under `docs/runs/` are excluded from the published site.
Keep every code snippet aligned with the real exported API, and update
[docs/SPEC.md](./docs/SPEC.md) whenever the endpoint surface changes.

---

## Windows Binary Hygiene

The SDK imports `crypto/aes`, `crypto/rsa`, and `crypto/sha1`. Unsigned Go
binaries that import those packages are frequently flagged by endpoint
antivirus, so:

- Never `go build -o <name>.exe` inside the repository.
- Use `go test`, `go vet`, and `go run`; they place transient binaries under
  `GOCACHE`/`GOTMPDIR`.
- If a build fails on Windows with a file lock on `a.out.exe`, set `GOTMPDIR`
  to a writable, non-scanned directory and retry. This is a host
  antivirus/indexing issue, not a code problem.
- Run the standalone mock with `go run ./cmd/hstong-mock-gateway` rather than
  building a local `.exe`.

---

## Reporting Issues

Bug reports are welcome. Please include:

- Go version (`go version`)
- SDK version (git commit or tag)
- Gateway version and test/production designation
- Minimal reproduction case
- Full error output with secrets redacted

Report security vulnerabilities privately as described in
[SECURITY.md](./SECURITY.md); do not open a public issue.
