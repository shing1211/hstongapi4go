# Integration tests against a real HStong Gateway

These tests live in `test/integration` and run against a **real, locally
installed HStong (华盛) Quant OpenAPI Gateway**, not the offline mock in
`../mockgateway`. They confirm the wire assumptions the mock cannot prove —
above all the `int64` representation in market payloads that
[ADR 0007](../../docs/adr/0007-http-json-codec.md) assumes.

They are **skipped by default** and are **never run in CI**. The offline suite
(`go test ./...`) must stay credential-free and network-free, so every test here
calls `t.Skip` unless `HSTONG_INTEGRATION=1`.

## Prerequisites

1. The HStong OpenAPI Gateway is installed and running on the machine, with the
   account logged in. The default endpoints are HTTP `http://127.0.0.1:11111`
   and TCP push `127.0.0.1:11112` ([docs/SPEC.md](../../docs/SPEC.md) §1).
2. The account has a trading password configured in the SDK
   (`HSTONG_TRADE_PASSWORD`) for the session and trade tests.
3. The test environment is reachable. It is available **Mon–Fri 09:00–18:00**
   only; outside those hours market reads may be empty or delayed.

Device binding is **mandatory in production but disabled in the test
environment** ([docs/SPEC.md](../../docs/SPEC.md) §8). The legacy direct-protocol
device-binding and developer-key flow is documented, and explicitly **not
implemented** here, in [docs/LEGACY.md](../../docs/LEGACY.md). For the account
activation flow (开通指引) and the current API documentation, see
<https://quant-open.hstong.com/api-docs/>.

## Run

Set the gate plus the connection variables, then run the suite.

### Windows PowerShell

```powershell
$env:HSTONG_INTEGRATION    = "1"
$env:HSTONG_GATEWAY_URL    = "http://127.0.0.1:11111"   # optional, this is the default
$env:HSTONG_PUSH_ADDR      = "127.0.0.1:11112"          # optional, this is the default
$env:HSTONG_TRADE_PASSWORD = "<your trade password>"    # required for session/trade tests
$env:HSTONG_TEST_SYMBOL    = "00700.HK"                 # optional, this is the default
$env:HSTONG_TEST_EXCHANGE  = "K"                        # optional, this is the default

go test ./test/integration/... -count=1 -v
```

To also exercise the order mutation (place then cancel one far-from-market limit
order):

```powershell
$env:HSTONG_PLACE_ORDERS = "1"
go test ./test/integration/... -count=1 -v -run TestIntegration_PlaceAndCancelOrder
```

### bash

```sh
HSTONG_INTEGRATION=1 \
HSTONG_TRADE_PASSWORD='<your trade password>' \
HSTONG_GATEWAY_URL=http://127.0.0.1:11111 \
HSTONG_PUSH_ADDR=127.0.0.1:11112 \
HSTONG_TEST_SYMBOL=00700.HK \
HSTONG_TEST_EXCHANGE=K \
go test ./test/integration/... -count=1 -v
```

The Makefile wraps the same command (`make test-integration`); it requires the
environment to be exported first, because the tests gate on the variables rather
than on a build tag.

## Environment matrix

| Variable | Required | Default | Purpose |
|----------|----------|---------|---------|
| `HSTONG_INTEGRATION` | yes | — | Gate. Must be `1`; otherwise every test is skipped. |
| `HSTONG_GATEWAY_URL` | no | `http://127.0.0.1:11111` | Gateway HTTP root. |
| `HSTONG_PUSH_ADDR` | no | `127.0.0.1:11112` | Gateway TCP push address. |
| `HSTONG_TRADE_PASSWORD` | for session/trade tests | — | Plaintext trade password; never logged. |
| `HSTONG_TEST_SYMBOL` | no | `00700.HK` | Instrument used by the market and trade tests. |
| `HSTONG_TEST_EXCHANGE` | no | `K` | Market: `K` HK, `P` US, `v` Shenzhen, `t` Shanghai. |
| `HSTONG_VERIFY_PUSH` | no | off | Parseable bool; enables push-signature verification. |
| `HSTONG_PLACE_ORDERS` | no | off | Must be `1` to enable the order mutation test. |

The connection and verification variables are also read by `client.WithEnv`
(see `client/env.go`); the remaining ones gate this suite only.

## What each test asserts

| Test | Asserts |
|------|---------|
| `TestIntegration_Session` | `TradeLogin` succeeds and a follow-up authenticated read works. |
| `TestIntegration_WireCodecProbe` | The observed JSON representation of `basicQot[].volume` and `ticker[].volume`/`timestamp` (`int64`) is a **number**, and `volumeStr` is a string. Fails, naming the field and file to change, when the SDK's number assumption is wrong. |
| `TestIntegration_EnvelopeNesting` | Whether `TradeQueryMaxAvailableAsset` nests the asset at `data.data` (the SDK's expectation) or directly at `data`; and which of the wrapped/bare/inline shapes `TradeQueryHoldsList` uses. |
| `TestIntegration_MarketPull` | The nine market pull endpoints return without error for `HSTONG_TEST_SYMBOL`; missing entitlements are reported as skips. |
| `TestIntegration_Streaming` | The TCP push channel delivers at least one decoded `Event` for a quote subscription within 10s. |
| `TestIntegration_TradeReads` | Funds and positions reads succeed (no mutations). |
| `TestIntegration_PlaceAndCancelOrder` | With `HSTONG_PLACE_ORDERS=1`, one far-from-market limit order is placed with exactly **one** outbound `TradeEntrust` attempt (ADR 0003), then cancelled; the cancel is always attempted from `t.Cleanup` even if the test fails first. |

A permission/entitlement rejection (status `1006`) on a market or read call is
reported as a `SKIP`, not a failure, because entitlements are account-specific.

## Interpreting a wire mismatch

If `TestIntegration_WireCodecProbe` fails, the Gateway sent an `int64` as a
quoted string while `pkg/hstong/market` decodes with `encoding/json`. Follow the
documented fallback in
[ADR 0007](../../docs/adr/0007-http-json-codec.md): select `client.ProtoJSON()`
for that endpoint in `pkg/hstong/market/market.go` and decode the DTO list from
`json.RawMessage` elements, or correct the ADR if the vendor SDKs were wrong.
Record the raw observation in the run's `evidence/` directory before changing
code.

## Verification

```sh
# Must SKIP, not fail, without the gate:
go test ./test/integration/... -count=1 -v

# The full offline suite must stay green and credential-free:
go test -race -count=1 ./...
```
