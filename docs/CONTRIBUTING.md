# Contributing

Thanks for contributing to `hstongapi4go`. This page is the short form of the
repository guide; the authoritative rules are in
[`AGENTS.md`](https://github.com/shing1211/hstongapi4go/blob/main/AGENTS.md).

## Build and verify

Prefer the Makefile targets:

```sh
make build            # go build ./...
make fmt              # gofmt
make vet              # go vet ./...
make test             # unit tests
make test-race        # go test -race -count=1 ./...
make check            # fmt + vet + tests + money-check
make money-check      # reject float misuse of money fields
make proto-verify     # fail if gen/ differs from proto/
make docs-check       # markdown link check + mkdocs build --strict
make license-check    # verify SPDX headers
```

Direct commands that must pass without credentials:

```sh
go build ./...
go vet ./...
gofmt -l .                 # must print nothing
go test ./...
go test -race -count=1 ./...
```

Unit tests are offline and credential-free. Integration tests are env-gated and
skipped by default; see [Testing](testing.md).

## Conventions

- **SPDX header.** Every new hand-written Go file starts with:

  ```go
  // Copyright 2026 shing1211
  // SPDX-License-Identifier: Apache-2.0
  ```

- **Generated code.** Never edit anything under `gen/`. Change the `.proto`
  sources or `buf*.yaml` and run `make proto`; committed output must match
  `make proto-verify`.
- **No auto-retry on order mutations.** Trade, futures, and algo mutations issue
  exactly one attempt (ADR 0003).
- **Money and quantities are strings**, never `float64` (ADR 0002, ADR 0007).
- **No new dependencies without an ADR** (ADR 0004).
- **GoDoc.** Every exported identifier has a doc comment starting with the
  identifier name, covering behavior, defaults, and edge cases.
- **Errors.** Use the typed errors in `internal/errs`; never match on error
  strings in library code, and keep `errors.Is`/`errors.As` traversal working.
- **Options.** Functional options (`Option func(*Config)`, `WithX`) applied in
  order over defaults.
- **Context.** Every blocking call takes `context.Context` first.
- **Streaming.** `Updates()`/`Errors()` channels; cancel by context or `Close`;
  reconnect re-subscribes all active topics.
- **Tests.** Table-driven where practical; add `_test.go` coverage for behavior
  changes.

## Commits

Conventional Commits referencing the run and task IDs, for example:

```
docs(adr): add ADR 0002 hybrid codec (T01)
```

Use `git commit -s` for the `Signed-off-by` trailer.

## Documentation

The site is built with MkDocs Material from `docs/`:

```sh
pip install -r requirements-docs.txt
mkdocs build --strict
```

Only public documentation belongs on the site. Internal run artifacts under
`docs/runs/` are excluded. Keep every code snippet aligned with the real exported
API.

## Architecture decisions

Design decisions are recorded in [`docs/adr/`](adr/README.md). Superseding a
decision means adding a new ADR, not editing the old one.
