# Mock Gateway

The repository ships a standalone mock of the HStong OpenAPI Gateway so the SDK
and the examples can be exercised without the real Gateway. It serves the HTTP
surface on `127.0.0.1:11111` and the TCP push surface on `127.0.0.1:11112`,
matching the real Gateway defaults. The mock responses are the fixtures under
`test/mockgateway/`.

## Start the mock

From the repository root:

```sh
go run ./cmd/hstong-mock-gateway
```

It prints the bound addresses and serves until Ctrl-C:

```
hstong-mock-gateway: HTTP  http://127.0.0.1:11111
hstong-mock-gateway: push  127.0.0.1:11112
hstong-mock-gateway: serving 51 endpoints; press Ctrl-C to stop
```

Use different ports with `-http` and `-push`:

```sh
go run ./cmd/hstong-mock-gateway -http 127.0.0.1:21111 -push 127.0.0.1:21112
```

`-http 127.0.0.1:0` binds an ephemeral HTTP port (the bound address is printed).

## Point the SDK at the mock

The SDK reads the Gateway address from `HSTONG_GATEWAY_URL` and the push address
from `HSTONG_PUSH_ADDR` through `client.WithEnv()`. With the mock on its default
ports no environment is needed:

```sh
go run ./examples/quickstart
go run ./examples/market-data
go run ./examples/streaming
```

For non-default ports:

```sh
# PowerShell
$env:HSTONG_GATEWAY_URL = "http://127.0.0.1:21111"
$env:HSTONG_PUSH_ADDR = "127.0.0.1:21112"
go run ./examples/quickstart
```

```sh
# POSIX shell
HSTONG_GATEWAY_URL=http://127.0.0.1:21111 \
HSTONG_PUSH_ADDR=127.0.0.1:21112 \
go run ./examples/quickstart
```

The mock accepts any trade password, so `HSTONG_TRADE_PASSWORD` may be set to any
non-empty value when running the trading, futures, or algo examples.

## Notes

- The mock is also usable from a Go test: `mockgateway.New().StartT(t)` starts it
  on ephemeral ports and registers cleanup.
- It does not simulate market hours, order matching, or real account state; its
  fixtures are static or cursor-paginated JSON. See `docs/testing.md`.
