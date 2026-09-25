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
| Architecture overview | [ARCHITECTURE.md](./ARCHITECTURE.md) (graph-derived, source-verified) · [docs/DESIGN.md](./docs/DESIGN.md) (design rationale) |
| Plan of record | [plan.md](./docs/runs/2026-09-25-hstong-agent-readiness/plan.md) |
| Run tracker | [todos.md](./docs/runs/2026-09-25-hstong-agent-readiness/todos.md) |
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
| Public managers | `pkg/hstong/` (`market`, `trade`, `future`, `algo`, `stream`, `session.go`) |
| Domain types + enums | `pkg/types/` |
| v-next layer (present, **not yet reachable by a caller**) | `pkg/domain/`, `pkg/services/`, `pkg/transport/`, `internal/auth/` |
| Mock Gateway | `test/mockgateway/`, `cmd/hstong-mock-gateway/` |
| Examples | `examples/` |
| Build and verification targets | `Makefile` |

## Build and verify

Use the Makefile targets:

```sh
make help              # list targets
make tools             # install/verify build tools
make build             # go build ./...
make fmt               # gofmt -s -w .
make fmt-check         # gofmt check exactly as CI sees it (gen/ excluded)
make vet               # go vet ./...
make test              # unit tests
make test-race         # unit tests with -race -count=1
make test-integration  # env-gated real-Gateway tests (HSTONG_INTEGRATION=1)
make coverage          # coverage gate: >=85% on pkg/domain, internal/auth, internal/transport, internal/push, pkg/hstong{,/stream,/trade,/algo}, pkg/types, pkg/transport
make check             # fmt + vet + tests + money-check
make money-check       # reject float misuse of money fields
make lint              # golangci-lint (v2.9+, required for the go 1.26 directive)
make gosec             # gosec, generated code excluded
make govulncheck       # govulncheck
make enterprise-check  # lint + gosec + govulncheck + coverage
make proto             # regenerate gen/ from proto/ with buf
make proto-verify      # fail if gen/ differs from proto/
make docs-check        # markdown link check + i18n lockstep + mkdocs build --strict
make license           # apply SPDX headers
make license-check     # verify SPDX headers
make sbom              # generate the SPDX SBOM
make goreleaser-check  # validate .goreleaser.yaml (needs the goreleaser binary on PATH)
make mock-gateway      # run the standalone mock Gateway
make clean             # remove build artifacts
```

Direct commands that must pass without credentials:

```sh
go build ./...
go vet ./...
gofmt -l .                 # must print nothing outside gen/
go test ./...
go test -race -count=1 ./...
```

- Unit tests are offline and credential-free; they must not require network access.
- Generated code under `gen/` is excluded from the `gofmt` and linter checks.
- `go test -race` needs a C toolchain. Where the host has none — notably Windows
  without the MSVC build tools — it cannot run locally and the race gate is
  satisfied by the CI `build` job instead. Report which of the two you relied on.
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

<!-- gitnexus:start -->
# GitNexus — Code Intelligence

This project is indexed by GitNexus as **hstongapi4go** (6,382 symbols, 17,083 relationships, 518 execution flows; index built at `b30b0bf`).

The index lives in `.gitnexus/` and is **not committed** — it is a local artifact, so a fresh clone has none.

> No index yet? Run `gitnexus analyze --index-only` from the project root; the CLI is installed globally. If `.gitnexus/run.cjs` exists, `node .gitnexus/run.cjs analyze --index-only` works too. Otherwise bootstrap with `npx`, `bunx`, or `pnpm dlx` — e.g. `bunx gitnexus@latest analyze` (npm 11 npx crash; #1939).
>
> `Database file version: N, Current build storage version: M`? The MCP server process is older than the installed CLI. Restart the MCP server or the editor — **do not** re-run `analyze`; the index is healthy.

## Always Do

- **MUST run impact before editing a function, class, or method, whenever the graph can answer.** Use `impact({target: "symbolName", direction: "upstream"})` or `node .gitnexus/run.cjs impact "symbolName" --direction upstream --repo .`; report callers, processes, and risk. Text search is not a substitute while the graph answers.
- **MUST analyze graph changes before committing, whenever the graph can answer.** Use `detect_changes({scope: "all"})` (MCP) or `node .gitnexus/run.cjs detect-changes --scope all --repo .` (CLI fallback). `partial: true` or `truncated: true` is not a clean check — a zero means unseen, not unaffected; re-run it. For regression review: `detect_changes({scope: "compare", base_ref: "main"})` or `node .gitnexus/run.cjs detect-changes --scope compare --base-ref "main" --repo .`.
- **If the graph cannot answer — no index on a fresh clone, or a failing graph reader — say so, fall back to text search plus tests, and record in your summary that graph analysis was unavailable.** Never skip the step silently, and never rebuild a working index to satisfy it.
- MUST warn on HIGH/CRITICAL `risk` pre-edit; never use `riskSharedAxes` to waive a HIGH/CRITICAL `risk` warning. Compare File/symbol: MCP File omits axes; Graph-RAG expands File.
- **MUST treat `risk: UNKNOWN` as unresolved, not as low.** An empty caller set is not evidence the symbol is unused — it can also mean the callers are not resolvable by the index (plain-object property access, dynamic dispatch, cross-language calls). `impact` pairs `UNKNOWN` with a `riskNote` saying so. Confirm with a text search before treating the symbol as safe to change or delete; do not proceed on the strength of a zero.
- **MUST use `query({search_query: "concept"})` for concepts/flows, `context({name: "symbolName"})` for a named symbol, or `impact` for blast radius, on read-only callers, dependencies, imports, or execution flow.** Graph first; text search only for empty/`UNKNOWN`/literals.
- For security review, `explain({target: "fileOrSymbol"})` lists taint findings (source→sink flows; needs `analyze --pdg`).

## Never Do

- NEVER edit a function, class, or method before MCP/CLI impact analysis, when the graph can answer.
- NEVER ignore HIGH or CRITICAL risk warnings from impact analysis, and never read `UNKNOWN` as an all-clear — it means the walk could not answer, which is the one verdict that requires confirming by other means.
- NEVER rename symbols with find-and-replace — use `rename` which understands the call graph.
- NEVER commit before MCP/CLI graph change analysis, when the graph can answer.

## Resources

| Resource | Use for |
| --- | --- |
| `gitnexus://repo/hstongapi4go/context` | Codebase overview, check index freshness |
| `gitnexus://repo/hstongapi4go/clusters` | All functional areas |
| `gitnexus://repo/hstongapi4go/processes` | All execution flows |
| `gitnexus://repo/hstongapi4go/process/{name}` | Step-by-step execution trace |

The `gitnexus://` URIs need a live MCP server. With the CLI alone, the equivalents are `gitnexus cypher` (structural queries), `gitnexus query`, and `gitnexus context`.

## CLI

| Task | Read this skill file |
| --- | --- |
| Understand architecture / "How does X work?" | `.claude/skills/gitnexus-exploring/SKILL.md` |
| Blast radius / "What breaks if I change X?" | `.claude/skills/gitnexus-impact-analysis/SKILL.md` |
| Trace bugs / "Why is X failing?" | `.claude/skills/gitnexus-debugging/SKILL.md` |
| Rename / extract / split / refactor | `.claude/skills/gitnexus-refactoring/SKILL.md` |
| Tools, resources, schema reference | `.claude/skills/gitnexus-guide/SKILL.md` |
| Index, status, clean, wiki CLI commands | `.claude/skills/gitnexus-cli/SKILL.md` |

<!-- gitnexus:end -->
