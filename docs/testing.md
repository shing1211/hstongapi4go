# Testing

## Offline unit tests

The unit suite is offline and credential-free:

```sh
go test ./...
go test -race -count=1 ./...
```

- `test/mockgateway` provides the HTTP + TCP-push mock.
- `test/e2e` drives every one of the 51 canonical HTTP endpoints through its
  typed manager method against the mock, plus the session login/re-login path and
  the push path with an intentionally unrecognisable `Any` `type_url`. It is
  offline and credential-free.
- Goroutine leaks are verified with `go.uber.org/goleak`, a test-only dependency
  (ADR 0006). `Close` must leave no SDK goroutine running.

Build and run the examples too:

```sh
go build ./examples/...
go test ./examples/... -count=1
```

`examples/examples_test.go` statically checks that every `examples/*/main.go`
declares `package main` with a `func main()`.

## Mock-based tests

Start a mock on ephemeral ports and point the client at it:

```go
func TestEntrust(t *testing.T) {
    srv := mockgateway.New(mockgateway.WithHTTPAddr("127.0.0.1:0"))
    srv.StartT(t)

    c, err := client.New(client.WithBaseURL(srv.HTTPBaseURL()))
    if err != nil {
        t.Fatal(err)
    }
    t.Cleanup(func() { _ = c.Close() })
    // ...
}
```

Inject failures to exercise error paths:

```go
srv.Inject("/hq/BasicQot", mockgateway.ErrorInjection{Code: "1011", Message: "busy", Times: 1})
```

See [Mock Gateway](mock-gateway.md) for the full API.

## Integration tests against a real Gateway

Tests against a real Gateway live in `test/integration`. They are env-gated,
skipped by default, and never run in CI. See
[test/integration/README.md](../test/integration/README.md) for the full run
guide, the Windows PowerShell and bash commands, and the device-binding /
account-activation (开通指引) pointers.

```sh
# POSIX shell
export HSTONG_INTEGRATION=1
export HSTONG_GATEWAY_URL=http://127.0.0.1:11111
export HSTONG_PUSH_ADDR=127.0.0.1:11112
export HSTONG_TRADE_PASSWORD=...
go test ./test/integration/... -count=1 -v
```

The Makefile's `make test-integration` target runs the same command once
`HSTONG_*` is exported.

| Variable | Required | Default | Purpose |
|----------|----------|---------|---------|
| `HSTONG_INTEGRATION` | yes | — | Gate; must be `1` or every test is skipped. |
| `HSTONG_GATEWAY_URL` | no | `http://127.0.0.1:11111` | Gateway HTTP root. |
| `HSTONG_PUSH_ADDR` | no | `127.0.0.1:11112` | Gateway TCP push address. |
| `HSTONG_TRADE_PASSWORD` | for session/trade tests | — | Plaintext trade password. |
| `HSTONG_TEST_SYMBOL` | no | `00700.HK` | Instrument under test. |
| `HSTONG_TEST_EXCHANGE` | no | `K` | Market: `K` HK, `P` US, `v` Shenzhen, `t` Shanghai. |
| `HSTONG_VERIFY_PUSH` | no | off | Enable push-signature verification. |
| `HSTONG_PLACE_ORDERS` | no | off | Must be `1` to enable the opt-in order mutation test. |

The suite validates the wire assumptions the offline mock cannot — in particular
the `int64` representation in market payloads (ADR 0007) and the envelope shape
of `TradeQueryMaxAvailableAsset` / `TradeQueryHoldsList`. The test environment
is available Mon–Fri 09:00–18:00 only.

> Integration tests are the only tests that may require the network or
> credentials. The default `go test ./...` never does.

## What to assert for mutations

Order mutations issue exactly one HTTP request. A regression test should count
outbound requests (or use `mockgateway.ErrorInjection` with `Times: 1`) and fail
if a second attempt is made. This pins ADR 0003.
