# Installation

## Requirements

- **Go 1.26 or newer.**
- The **HStong OpenAPI Gateway** installed and running locally, or the in-repo
  [Mock Gateway](mock-gateway.md) for offline work.
- A trading account and trade password if you will use the trading, futures, or
  algo surfaces.

The SDK never installs, starts, or redistributes the Gateway. It dials
`http://127.0.0.1:11111` (HTTP) and `127.0.0.1:11112` (TCP push) by default.

## Add the module

```sh
go get github.com/shing1211/hstongapi4go
```

The only runtime dependency the SDK adds is `google.golang.org/protobuf`, used by
the generated push and market-data types. All HTTP payloads are decoded with the
standard library `encoding/json`.

## Import paths

| Package | Import path | Purpose |
|---------|-------------|---------|
| Core client | `github.com/shing1211/hstongapi4go/client` | Options, env, HTTP dispatch, codecs |
| Session | `github.com/shing1211/hstongapi4go/pkg/hstong` | `SessionManager` (login, keep-alive) |
| Market | `github.com/shing1211/hstongapi4go/pkg/hstong/market` | 9 pull + subscribe/unsubscribe |
| Trade | `github.com/shing1211/hstongapi4go/pkg/hstong/trade` | Assets, positions, orders, push |
| Futures | `github.com/shing1211/hstongapi4go/pkg/hstong/future` | 11 futures endpoints |
| Algo | `github.com/shing1211/hstongapi4go/pkg/hstong/algo` | 7 algorithm endpoints |
| Streaming | `github.com/shing1211/hstongapi4go/pkg/hstong/stream` | Channel-based push client |
| Types | `github.com/shing1211/hstongapi4go/pkg/types` | Enums, status codes, platform keys |
| DTOs | `github.com/shing1211/hstongapi4go/gen/hq/dto` | Generated market DTO types |
| Mock (test) | `github.com/shing1211/hstongapi4go/test/mockgateway` | Offline HTTP + push server |

## Minimal program

Create a client once and share it. The client is safe for concurrent use; close
it when the program exits.

```go
c, err := client.New(client.WithEnv())
if err != nil {
    return err
}
defer c.Close()
```

`client.WithEnv()` reads the `HSTONG_*` variables listed in
[Configuration](configuration.md) and falls back to the Gateway defaults.

## Build and run the examples

The repository ships runnable programs for every major surface:

```sh
go run ./examples/quickstart
go run ./examples/market-data
go run ./examples/trading
go run ./examples/futures
go run ./examples/algo
go run ./examples/streaming
```

They compile without credentials. Running the trading, futures, and algo examples
requires a Gateway session and contact with a real (or mock) Gateway; the trading,
futures, and algo examples skip order mutations unless
`HSTONG_EXAMPLE_PLACE_ORDER` is set. See
[`examples/README.md`](https://github.com/shing1211/hstongapi4go/blob/main/examples/README.md).

## Verify the module without a Gateway

```sh
go build ./...
go vet ./...
go test ./...
```

Unit tests are offline and credential-free. Integration tests against a real
Gateway are env-gated; see [Testing](testing.md).
