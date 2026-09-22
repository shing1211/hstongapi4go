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
- **Money and quantities are strings**, never `float64` (see [DESIGN.md §7](./DESIGN.md)).
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

## Adding a new HTTP endpoint

1. **Add the route constant** to `client/routes.go` with the canonical path
   (e.g., `RouteHqBasicQot = "/hq/BasicQot"`).
2. **Add the request struct** in the appropriate manager package
   (`pkg/hstong/market/`, `pkg/hstong/trade/`, `pkg/hstong/future/`,
   `pkg/hstong/algo/`) with `json:"..."` tags and a `//go:generate`
   comment if using a generator.
3. **Add the manager method** that calls `c.Do(ctx, op, route, params, codec, &out)`.
4. **Add local validation** before the HTTP call; return `ErrInvalidParams` or
   a typed `1016` error for malformed input.
5. **Add the mock response fixture** in `test/mockgateway/fixtures.go`.
6. **Add an e2e test** in `test/e2e/` that exercises the route against the mock.
7. **Update SPEC.md** with the new endpoint (endpoint count, response schema,
   topic if applicable). Counts live in §2 and §4 only.
8. **Update the feature matrix** in `README.md` and `docs/index.md`.
9. **Add an example** in `examples/` if the endpoint is commonly used.

Do not invent a new transport mechanism or codec mode. If the endpoint is a
mutation (place/cancel/modify), add it to the mutation guard in
`internal/resilience/retry.go` to prevent retry.

## Adding a new push topic

1. **Add the topic ID constant** to `pkg/types/enums.go` (e.g., `TopicBasicQot = 11`).
2. **Add the decoder** in `internal/push/client.go` `Decode()` to dispatch on the
   `notifyMsgType` to the generated protobuf type.
3. **Add the typed accessor** to `pkg/hstong/stream/stream.go` `Event` type.
4. **Update SPEC.md** §4 (push topic count) and §5 (NotifyMsgType table).
5. **Add a mock emit** in `test/mockgateway/push.go` for testing.
6. **Add the topic to the documentation** (market-data.md or trading.md).

## Adding a new ADR

1. Copy the template in `docs/adr/README.md`.
2. Name the file `NNNN-title-slug.md` with the next available number.
3. Set status to `Proposed` initially; change to `Accepted` after review.
4. Add the ADR to the table in `docs/adr/README.md`.
5. If the decision supersedes an existing one, update the old ADR's status to
   `Superseded by NNNN` and add a reference to it.

## Common patterns

### Pagination

Use `queryParamStr` cursor. The last page is detected by an empty or short
response or a non-advancing cursor:

```go
func paginate(ctx context.Context, m *trade.Manager) ([]trade.OrderVo, error) {
    var all []trade.OrderVo
    cursor := "0"
    for {
        rows, err := m.RealEntrustList(ctx, trade.RealEntrustListRequest{
            QueryCount:   20,
            QueryParamStr: cursor,
        })
        if err != nil {
            return nil, err
        }
        all = append(all, rows...)
        if len(rows) == 0 || cursor == rows[len(rows)-1].QueryParamStr {
            break
        }
        cursor = rows[len(rows)-1].QueryParamStr
    }
    return all, nil
}
```

### Reconciliation after a mutation failure

When `Entrust` or `CancelEntrust` returns an ambiguous error (timeout with no
response), reconcile before resubmitting:

```go
// After an ambiguous Entrust failure:
filled, err := tradeMgr.RealDeliverList(ctx, ...)
for _, f := range filled {
    if f.EntrustID == entrustID {
        // Order was actually placed or filled
        return nil
    }
}
// If not found in delivers, check open entrusts:
open, err := tradeMgr.RealEntrustList(ctx, ...)
for _, o := range open {
    if o.EntrustID == entrustID {
        // Order is still open; decide whether to cancel or modify
        return nil
    }
}
// Not found anywhere; safe to retry
```

### Context cancellation

Always pass context with a deadline to blocking calls:

```go
ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
defer cancel()
```

For long-running streaming, use the stream's context or the caller's context:

```go
select {
case <-ctx.Done():
    return ctx.Err()
case ev := <-sub.Updates():
    // handle event
}
```

### Reconnection handling

The push client reconnects automatically. On `ErrReconnected`, consumers should
expect a brief gap and then resuming events:

```go
for {
    select {
    case err := <-sub.Errors():
        if errors.Is(err, stream.ErrReconnected) {
            log.Println("reconnected, waiting for events to resume")
        } else {
            return err
        }
    case ev := <-sub.Updates():
        // handle event
    }
}
```

