# Examples

Runnable, credential-free-to-compile programs for every major surface of the
SDK. Each example is its own `main` package, reads configuration from the
`HSTONG_*` environment with safe defaults, applies a context timeout, and prints
its results. None of them hardcode credentials or require network access to
build.

Start the local HStong Gateway, or the in-repo mock Gateway, before running an
example. See [`mock-gateway/README.md`](./mock-gateway/README.md).

## Environment

| Variable | Used by | Default |
|----------|---------|---------|
| `HSTONG_GATEWAY_URL` | all | `http://127.0.0.1:11111` |
| `HSTONG_PUSH_ADDR` | all | `127.0.0.1:11112` |
| `HSTONG_TIMEOUT` | all | `10s` |
| `HSTONG_TRADE_PASSWORD` | trading, futures, algo, quickstart login | unset |
| `HSTONG_VERIFY_PUSH` | all | `false` |
| `HSTONG_EXAMPLE_SECURITY` | quickstart, market-data, trading, algo | `0700.HK` |
| `HSTONG_EXAMPLE_FUTURES_CODE` | futures | `HSI2603` |
| `HSTONG_EXAMPLE_PRICE` | trading, futures, algo | `300` (futures `20000`) |
| `HSTONG_EXAMPLE_AMOUNT` | trading, futures, algo | `100` (futures `1`) |
| `HSTONG_EXAMPLE_PLACE_ORDER` | trading, futures, algo | `false` |
| `HSTONG_EXAMPLE_ALGO_ORDER_ID` | algo | unset |

## Run

```sh
go run ./examples/quickstart     # env config, login, one quote, one order query
go run ./examples/market-data    # quote, order book, K-line, ticker, time-share
go run ./examples/trading        # funds, positions, place/query/cancel
go run ./examples/futures        # futures funds, positions, place/query
go run ./examples/algo           # algo add, master/sub queries, action
go run ./examples/streaming      # connect push, subscribe, consume until Ctrl-C
```

Order placement and cancellation are mutations: the trading, futures, and algo
examples skip them unless `HSTONG_EXAMPLE_PLACE_ORDER` is `1` or `true`. Nothing
is submitted to a funded account by default.

## Build and test

Every example compiles without credentials:

```sh
go build ./examples/...
go test ./examples/... -count=1
```

The test (`examples/examples_test.go`) walks every `examples/*/main.go`, parses
it with `go/parser`, and asserts the file declares `package main` with a
`func main()`. This is the simplest check that the directory really is a runnable
program; it does not execute the examples against a live Gateway.
