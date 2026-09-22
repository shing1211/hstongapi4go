# Examples

Runnable, credential-free-to-compile programs for every major surface of the
SDK. Each example is its own `main` package, reads configuration from the
`HSTONG_*` environment with safe defaults, applies a context timeout, and prints
its results. None of them hardcode credentials or require network access to
build.

Start the local HStong Gateway, or the in-repo mock Gateway, before running an
example. See [`mock-gateway/README.md`](./mock-gateway/README.md).

## Difficulty ratings

| Example | Difficulty | Prerequisites |
|---------|------------|---------------|
| `quickstart` | Beginner | None |
| `market-data` | Beginner | None |
| `trading` | Intermediate | Trade session |
| `futures` | Intermediate | Trade session |
| `algo` | Intermediate | Trade session |
| `streaming` | Advanced | Market push topic |

**Beginner:** No authentication required. Run against the mock Gateway or a
real Gateway without a trade account.

**Intermediate:** Requires `HSTONG_TRADE_PASSWORD` set (or mock Gateway with any
password).

**Advanced:** Uses the TCP push channel; requires understanding of the push
reconnect and subscription lifecycle.

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
go run ./examples/quickstart     # Beginner: env config, login, one quote, one order query
go run ./examples/market-data    # Beginner: quote, order book, K-line, ticker, time-share
go run ./examples/trading        # Intermediate: funds, positions, place/query/cancel
go run ./examples/futures        # Intermediate: futures funds, positions, place/query
go run ./examples/algo           # Intermediate: algo add, master/sub queries, action
go run ./examples/streaming      # Advanced: connect push, subscribe, consume until Ctrl-C
```

Order placement and cancellation are mutations: the trading, futures, and algo
examples skip them unless `HSTONG_EXAMPLE_PLACE_ORDER` is `1` or `true`. Nothing
is submitted to a funded account by default.

## What each example covers

| Example | Covers |
|---------|--------|
| `quickstart` | Client creation, session login/logout, one market quote, one order query |
| `market-data` | BasicQot, OrderBook, KL, TimeShare, Ticker, Broker, option chain, US overnight codes |
| `trading` | MarginFundInfo, Positions, RealFundJourList, Entrust, CancelEntrust, RealEntrustList, paginated history |
| `futures` | QueryFundInfo, QueryHoldsList, QueryProductInfo, QueryMaxBuySellAmount, FuturesEntrust, QueryRealEntrustList |
| `algo` | AddOrder, QueryOrderList, QueryEntrustIDList, ChangeOrder, CancelOrder, ActionOrder |
| `streaming` | Connect, Subscribe, Updates/Errors channels, reconnect handling, backpressure |

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

## Adding a new example

1. Create `examples/<name>/main.go` with `package main` and `func main()`.
2. Add the example to the environment table and the run section above.
3. Add a row to the "What each example covers" table.
4. Ensure `go build ./examples/...` succeeds without credentials.
5. If the example requires trade credentials, add it to the "Intermediate" or
   "Advanced" difficulty tier.
