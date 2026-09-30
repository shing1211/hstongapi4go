# Going Live — HStong API Access and Trading

This guide walks you through getting API access, installing and configuring the
HStong OpenAPI Gateway, and placing your first real order with this SDK.  It is
the counterpart to [Getting Started (Offline)](getting-started.md), which uses the
in-repo mock Gateway and requires no credentials.

> **Safety first.**  The integration test suite in
> [Integration Testing](integration-testing.md) validates the full wire stack
> before you place any live orders.  Run it first.

---

## Prerequisites

- An **HStong (华盛) trading account**.  API access is not included automatically
  when you open an account — it requires a separate application (Step 1 below).
- A machine capable of running the HStong OpenAPI Gateway (Windows is supported).
- Go 1.26 or newer.

---

## Step 1 — Apply for API Access

API access requires a signed set of documents and an RSA key pair.  Approval
takes **2–3 working days**.

### Application URL

`
https://passport.hstong.com/login?target=https%3A%2F%2Fquant-open.hstong.com%2Fdeveloper%2Fapply
`

### Documents to sign

1. **API Questionnaire Assessment** (API问卷评估调查)
2. **API Test Results Declaration Form** (API测试结果申报表)
3. **API Developer Authorization Agreement** (API开发者授权协议)
4. **API Disclaimer** (API免责声明)

### RSA key pair

After approval you will upload your **public key**.  Requirements:

| Field | Value |
|-------|-------|
| Format | PKCS#8 |
| Key length | 1024 bits |
| Passphrase | None (leave empty) |

Generate with OpenSSL:

`sh
openssl genrsa -out private_key.pem 1024
openssl pkcs8 -topk8 -nocrypt -in private_key.pem -out public_key.pem
`

Upload public_key.pem to the developer portal.  Keep private_key.pem — you
will point the Gateway at it.

---

## Step 2 — Install the HStong OpenAPI Gateway

The SDK communicates with a **local Gateway process** installed on your machine.
It is not bundled with the SDK and not included in your account opening.

### Download

`
https://quant-open.hstong.com/api-docs/quick-start/openapi-sdk-download.html
`

### Default endpoints

| Protocol | Address |
|----------|---------|
| HTTP | http://127.0.0.1:11111 |
| TCP push | 127.0.0.1:11112 |

### Configure the Gateway

During Gateway setup you will provide:

- **Private key path** — path to your private_key.pem file
- **Account credentials** — your HStong account login
- **Market permissions** — HK, US, futures, depending on what your account holds

The Gateway handles all cryptographic signing (RSA) and never exposes your private
key to the SDK.  The SDK sends plaintext JSON over localhost.

---

## Step 3 — Set Credentials

The SDK needs only your **trade password**.  No developer token, no RSA key
management.

| Variable | Default | Required | Purpose |
|----------|---------|----------|---------|
| HSTONG_GATEWAY_URL | http://127.0.0.1:11111 | No | Gateway HTTP root |
| HSTONG_PUSH_ADDR | 127.0.0.1:11112 | No | Gateway TCP push address |
| HSTONG_TRADE_PASSWORD | — | **Yes** | Plaintext trade password |
| HSTONG_VERIFY_PUSH | alse | No | Set to 1 to enable push-frame SHA1WithRSA signature verification |
| HSTONG_TIMEOUT | 10s | No | Per-request timeout |

`powershell
# Windows PowerShell
 = "your-trade-password"
`

`sh
# Linux / macOS / bash on Windows
export HSTONG_TRADE_PASSWORD="your-trade-password"
`

---

## Step 4 — Verify Connectivity

Confirm the Gateway is reachable before running any trading code.

`sh
# Should print the Gateway version and build info
curl http://127.0.0.1:11111/version
`

Then run the quickstart example with your credentials:

`sh
HSTONG_TRADE_PASSWORD="your-trade-password" go run ./examples/quickstart
`

If both succeed, your SDK and Gateway are correctly configured.

---

## Step 5 — Validate with Integration Tests

Run the integration test suite against your live Gateway **before placing any
real orders**.  This confirms the wire assumptions are correct for your account
and network path.  See [Integration Testing](integration-testing.md) for the
full guide, or jump to the commands below.

### Session and market read tests (no orders)

`powershell
 = "1"
 = "your-trade-password"
go test ./test/integration/... -count=1 -v -run TestIntegration_Session
`

### Order mutation test (places and cancels one real order)

`powershell
 = "1"
 = "1"
 = "your-trade-password"
go test ./test/integration/... -count=1 -v -run TestIntegration_PlaceAndCancelOrder
`

> **Note:** The order mutation test is gated by HSTONG_PLACE_ORDERS=1 so that a
> forgotten environment variable cannot accidentally submit an order.

Tests run **Mon–Fri 09:00–18:00** only.  Outside those hours market reads may be
empty or delayed.

A permission rejection (status 1006) on a market or read call is reported as a
SKIP, not a failure — entitlements are account-specific.

---

## Step 6 — Place Your First Real Order

Use the trading example with the explicit HSTONG_EXAMPLE_PLACE_ORDER opt-in:

`sh
HSTONG_TRADE_PASSWORD="your-trade-password" \
HSTONG_EXAMPLE_PLACE_ORDER=1 \
  go run ./examples/trading
`

This logs in, queries funds and positions, places one far-from-market limit
order, waits for it to be visible in the open-orders list, then cancels it.  The
HSTONG_EXAMPLE_PLACE_ORDER flag is the SDK's safety mechanism — without it the
example runs but skips the order mutation.

---

## What the SDK Does NOT Need

- **No RSA key management in the SDK.**  The Gateway handles all RSA signing.
- **No developer token.**  Only the trade password is required.
- **No server-side deployment.**  Everything runs on 127.0.0.1.
- **No platform credentials in the repository.**  The bundled platform public
  keys are public reference data.

---

## Important Notes

- Your account's market permissions in the Gateway must match what you have
  enabled in the HStong app.  If you cannot access US market data, the Gateway
  will return a permission error (status 1006) rather than real data.
- Order mutations require HSTONG_PLACE_ORDERS=1 in addition to a valid session.
- The SDK enforces exactly-one semantics on every order mutation (see
  [ADR 0003](./adr/0003-no-auto-retry-orders.md)).  A failed or ambiguous
  response is returned as a typed error; the SDK never resubmits.
- Push-frame signature verification (HSTONG_VERIFY_PUSH=1) is opt-in.  The
  bundled platform public keys handle it; no additional configuration is needed.

---

## Getting Help

- SDK docs: https://github.com/shing1211/hstongapi4go
- HStong API docs: https://quant-open.hstong.com/api-docs/
- Report SDK issues: https://github.com/shing1211/hstongapi4go/issues