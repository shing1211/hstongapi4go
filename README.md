# hstongapi4go

<p align="center">
  <img src="https://img.shields.io/badge/Go-1.26+-00ADD8?style=flat-square&logo=go" alt="Go">
  <img src="https://img.shields.io/badge/License-Apache%202.0-blue?style=flat-square" alt="License">
  <img src="https://img.shields.io/badge/Status-alpha-blue?style=flat-square" alt="Status">
  <a href="https://github.com/shing1211/hstongapi4go/actions/workflows/ci.yml"><img src="https://github.com/shing1211/hstongapi4go/actions/workflows/ci.yml/badge.svg" alt="CI"></a>
  <img src="https://img.shields.io/badge/Gateway-v2.4.1-brightgreen?style=flat-square" alt="Gateway version">
  <img src="https://img.shields.io/badge/Protobuf-v2.2.0-blueviolet?style=flat-square" alt="Protobuf package version">
  <img src="https://img.shields.io/badge/Endpoints-51-orange?style=flat-square" alt="HTTP endpoints">
  <img src="https://img.shields.io/badge/Push%20topics-11-yellowgreen?style=flat-square" alt="Market push topics">
  <a href="https://shing1211.github.io/hstongapi4go/"><img src="https://img.shields.io/badge/Docs-GitHub%20Pages-97CAFF?style=flat-square&logo=github" alt="Docs"></a>
</p>

> **Unofficial and alpha.** `hstongapi4go` is a community Go SDK for the HStong
> (华盛) Quant OpenAPI **local Gateway**. It is **not** affiliated with,
> authorized by, endorsed by, or sponsored by HStong / 华盛 (Huasheng) or any of
> its subsidiaries. "HStong", "华盛", "华盛通", and related names and marks are
> the property of their respective owners and are used here only for descriptive,
> interoperability purposes. See [DISCLAIMER.md](./DISCLAIMER.md).

> **Go-native. Typed. Gateway-first.** An idiomatic Go client for the local
> HStong Gateway — market data, trading, futures, algo, and real-time push — with
> money and quantities kept as strings and order mutations never retried
> automatically.

[English](./README.md) · [简体中文](./README.zh-Hans.md) · [繁體中文](./README.zh-Hant.md) · [日本語](./README.ja.md) · [한국어](./README.ko.md) · [Español](./README.es.md)

> Canonical English source. Last synced: 2026-09-25

## Key Concepts

Before writing your first call, understand these SDK abstractions:

| Concept | Package | Description |
|---------|--------|-------------|
| `Client` | [client](./docs/configuration.md) | Thread-safe HTTP transport; create once, share widely |
| `SessionManager` | [pkg/hstong](./docs/authentication.md) | Trade login/logout, keep-alive, single-flight re-login |
| `Manager` | [pkg/hstong/market](./docs/market-data.md) | Surface-specific API (market, trade, futures, algo) |
| `stream.Client` | [pkg/hstong/stream](./docs/streaming.md) | TCP push subscriber with auto-reconnect |

**Choose your surface:**

- **Market data only?** → `market.New(c)` — no authentication required
- **Trading, futures, algo?** → `hstong.NewSessionManager(c)` + `Login` first
- **Real-time quotes?** → `stream.New(c)` + `Connect` + `Subscribe`

## Table of Contents

- [Key Concepts](#key-concepts)
- [Status](#status)
- [Install](#install)
- [Quickstart](#quickstart)
- [Feature Matrix](#feature-matrix)
- [Configuration](#configuration)
- [Package Layout](#package-layout)
- [Documentation](#documentation)
- [Build & Test](#build--test)
- [Contributing](#contributing)
- [Security](#security)
- [License](#license)

---

## Status

| Item | State |
|------|-------|
| Protocol core (HTTP envelope, route aliases, hybrid codec, typed errors) | Implemented |
| Market data pull (9 endpoints) | Implemented |
| Market subscription + TCP push (11 topics) | Implemented |
| Trade session (login/logout, keep-alive, single-flight re-login) | Implemented |
| Trade assets / positions (5) and orders (13) | Implemented |
| Trade push subscribe + order-status decoders | Implemented |
| Futures (11 endpoints + push) | Implemented |
| Algo / strategy (7 endpoints) | Implemented |
| Channel-based streaming API (`Updates()` / `Errors()`) | Implemented |
| Mock Gateway (51 HTTP routes + TCP push) and standalone binary | Implemented |
| Hardening (rate limit, retry budget, circuit breaker, logging, metrics) | Implemented |
| Documentation (READMEs, MkDocs site, ADRs, SPEC, LEGACY) | Implemented |
| Offline tests + all-endpoint SDK-to-mock e2e | Implemented |
| Integration tests against a real Gateway | Written and env-gated; live confirmation pending a user run |
| OpenTelemetry traces and metrics | Implemented behind the `otel` build tag; stdlib-only without it |
| Threat model and risk register | [docs/threat-model.md](./docs/threat-model.md); 7 adversarial defects found and fixed |
| Enterprise CI (lint, security, coverage gate, SBOM, GoReleaser config) | 12 jobs green |
| v-next layer (`pkg/domain`, `pkg/services`, `pkg/transport`, `internal/auth`) | Implemented and tested, **not yet reachable by a caller** — see [ARCHITECTURE.md](./ARCHITECTURE.md) |
| Release (GitHub + Gitee) | v0.1.12 ✓ |

All 51 HTTP endpoints and 11 market push topics are implemented. Counts are
canonical in [docs/SPEC.md](./docs/SPEC.md); do not hand-edit them elsewhere.

## Install

```bash
go get github.com/shing1211/hstongapi4go
```

Requires **Go 1.26+** and a running local [HStong OpenAPI Gateway](https://quant-open.hstong.com/api-docs/)
(or the in-repo [Mock Gateway](./docs/mock-gateway.md)). The SDK never installs,
starts, or redistributes the Gateway; it dials `http://127.0.0.1:11111` (HTTP)
and `127.0.0.1:11112` (TCP push) by default.

## Quickstart

Build the client once, share it, and close it when the program exits. The client
is safe for concurrent use.

```go
package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/shing1211/hstongapi4go/client"
	"github.com/shing1211/hstongapi4go/gen/hq/dto"
	"github.com/shing1211/hstongapi4go/pkg/hstong"
	"github.com/shing1211/hstongapi4go/pkg/hstong/market"
	"github.com/shing1211/hstongapi4go/pkg/hstong/trade"
	"github.com/shing1211/hstongapi4go/pkg/types"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// 1. Resolve configuration from HSTONG_* (see Configuration).
	c, err := client.New(client.WithEnv())
	if err != nil {
		log.Fatal(err)
	}
	defer c.Close()

	// 2. Log in to the trade session (needs HSTONG_TRADE_PASSWORD).
	session := hstong.NewSessionManager(c)
	if err := session.Login(ctx); err != nil {
		log.Fatalf("trade login: %v", err)
	}
	defer session.Logout(ctx)

	// 3. Request one market quote.
	marketMgr := market.New(c)
	quote, err := marketMgr.BasicQot(ctx, market.BasicQotRequest{
		Security: []*dto.Security{
			{DataType: int32(types.DataTypeHKStock), Code: "0700.HK"},
		},
		MktTmType: 1,
	})
	if err != nil {
		log.Fatal(err)
	}
	for _, q := range quote.BasicQot {
		fmt.Printf("quote %s last=%v\n", q.GetSecurity().GetCode(), q.GetLastPrice())
	}

	// 4. Query today's real orders (authenticated).
	tradeMgr := trade.New(c, trade.WithSession(session))
	orders, err := tradeMgr.RealEntrustList(ctx, trade.RealEntrustListRequest{
		ExchangeType: types.ExchangeHK,
		QueryCount:   20,
	})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("today's real orders: %d\n", len(orders))
}
```

Subscribe to a market push topic over the TCP channel (import
`github.com/shing1211/hstongapi4go/pkg/hstong/stream`):

```go
s := stream.New(c)
if err := s.Connect(ctx); err != nil {
	log.Fatal(err)
}
defer s.Close()

sub, err := s.Subscribe(ctx, types.TopicBasicQot,
	&dto.Security{DataType: int32(types.DataTypeHKStock), Code: "0700.HK"})
if err != nil {
	log.Fatal(err)
}
defer sub.Cancel(context.Background())

for {
	select {
	case <-ctx.Done():
		return
	case ev := <-sub.Updates():
		if q, ok := ev.BasicQot(); ok {
			fmt.Printf("%s last=%v\n", ev.ID, q.GetBasicQot().GetLastPrice())
		}
	case err := <-sub.Errors():
		log.Printf("stream: %v", err)
	}
}
```

Runnable, credential-free-to-compile programs for every surface live in
[`examples/`](./examples/README.md): `quickstart`, `market-data`, `trading`,
`futures`, `algo`, and `streaming`.

## Feature Matrix

Endpoint counts are taken from [docs/SPEC.md](./docs/SPEC.md) only. All routes
are `POST http://127.0.0.1:11111<route>`.

| Surface | Package | Endpoints |
|---------|---------|-----------|
| Market data pull | `pkg/hstong/market` | 9 |
| Market subscribe / unsubscribe | `pkg/hstong/market` | 2 |
| Trade session | `pkg/hstong` | 2 |
| Trade assets / positions | `pkg/hstong/trade` | 5 |
| Trade orders | `pkg/hstong/trade` | 13 |
| Trade push subscribe | `pkg/hstong/trade` | 2 |
| Algo / strategy | `pkg/hstong/algo` | 7 |
| Futures | `pkg/hstong/future` | 11 |
| **Total HTTP** | | **51** |

Market push topics (11): `11`, `35` (quote); `14`, `27`, `28`, `37` (tick);
`16` (broker queue); `17`, `25`, `26`, `36` (order book). See
[docs/SPEC.md §3](./docs/SPEC.md). Trade and futures order-status push use the
`TradeStockDeliverNotify` family on the same TCP channel.

The hybrid codec is **per endpoint**: market HTTP bodies are decoded with
`encoding/json` over typed DTOs, TCP push payloads with binary protobuf; trade,
futures, algo, assets, and session bodies are hand-written JSON structs. Money
and quantities are `string` (or `json.Number`), never `float64`. Order, futures,
and algo mutations issue exactly one attempt and are never auto-retried.

## Configuration

`client.New` applies functional options in order over documented defaults. Every
`HSTONG_*` variable below is read by `client.WithEnv()`; an explicit option
placed after `WithEnv` overrides the environment.

| Variable | Format | Default | Purpose |
|----------|--------|---------|---------|
| `HSTONG_GATEWAY_URL` | absolute `http`/`https` URL | `http://127.0.0.1:11111` | Gateway HTTP root |
| `HSTONG_PUSH_ADDR` | `host:port` | `127.0.0.1:11112` | Gateway TCP push address |
| `HSTONG_TIMEOUT` | Go duration (`10s`, `1500ms`) | `10s` | Per-request timeout and envelope `timeout_sec` |
| `HSTONG_TRADE_PASSWORD` | plaintext | unset | Trade password; encrypted before `TradeLogin`, never logged |
| `HSTONG_VERIFY_PUSH` | `strconv.ParseBool` | `false` | Opt-in push-frame `SHA1WithRSA` verification |

Integration tests against a real Gateway are skipped by default and add their
own variables (see [`test/integration/README.md`](./test/integration/README.md)):

| Variable | Required | Default | Purpose |
|----------|----------|---------|---------|
| `HSTONG_INTEGRATION` | yes (gate) | — | Must be `1`, otherwise every test skips |
| `HSTONG_GATEWAY_URL` | no | `http://127.0.0.1:11111` | Gateway HTTP root |
| `HSTONG_PUSH_ADDR` | no | `127.0.0.1:11112` | Gateway TCP push address |
| `HSTONG_TRADE_PASSWORD` | for session/trade tests | — | Plaintext trade password |
| `HSTONG_TEST_SYMBOL` | no | `00700.HK` | Instrument under test |
| `HSTONG_TEST_EXCHANGE` | no | `K` | `K` HK, `P` US, `v` Shenzhen, `t` Shanghai |
| `HSTONG_VERIFY_PUSH` | no | off | Enable push-signature verification |
| `HSTONG_PLACE_ORDERS` | no | off | Must be `1` to enable the order-mutation test |

## Package Layout

```
hstongapi4go/
├── client/            # Core client: options, env, routes, codecs, Close
├── pkg/hstong/        # Public managers: session + market/trade/future/algo/stream
├── pkg/types/         # Enums, status codes, platform public keys
├── pkg/domain/        # v-next: decimal value types, typed IDs, DTO mappers
├── pkg/services/      # v-next: market/account/trading use cases
├── pkg/transport/     # v-next: HTTP adapter (deadlines, caps, retry)
├── internal/          # Private: transport, push, crypto, errs, resilience, logging, metrics
├── gen/               # Generated protobuf code (DO NOT EDIT)
├── proto/             # Vendored .proto sources + provenance
├── test/mockgateway/  # Offline mock HTTP + TCP push server
├── test/e2e/          # All-endpoint SDK-to-mock tests
├── test/integration/  # Env-gated real-Gateway tests (skipped by default)
├── cmd/               # Standalone binaries (hstong-mock-gateway)
├── examples/          # Runnable examples (mock-friendly)
├── scripts/           # Build and verification scripts
└── docs/              # MkDocs site, SPEC, ADRs, DESIGN, LEGACY
```

## Documentation

- Documentation site: <https://shing1211.github.io/hstongapi4go/>
- Canonical API index and counts: [docs/SPEC.md](./docs/SPEC.md)
- Architecture: [ARCHITECTURE.md](./ARCHITECTURE.md) · [docs/DESIGN.md](./docs/DESIGN.md)
- Decisions: [docs/adr/README.md](./docs/adr/README.md) (ADRs 0001–0011)
- Legacy protocol (documented, **not implemented**): [docs/LEGACY.md](./docs/LEGACY.md)
- Disclaimer: [DISCLAIMER.md](./DISCLAIMER.md)

The **legacy direct-to-platform protocol** — `/hs/v2/login`,
`/hs/config/queryServer`, the direct 151-byte socket with developer RSA signing,
AES-ECB dynamic keys, heartbeat, and device binding — is documented for
reference only and is **not implemented** in this SDK. See
[docs/LEGACY.md](./docs/LEGACY.md).

## Build & Test

```bash
make help            # list targets
make build           # go build ./...
make check           # fmt + vet + money-check + unit tests
make test-race       # go test -race -count=1 ./...
make test-integration  # env-gated real-Gateway tests (HSTONG_* required)
make proto-verify    # fail if gen/ drifts from proto/
make docs-check      # markdown link check + mkdocs build --strict
make mock-gateway    # run the standalone mock Gateway
```

Direct commands that must pass without credentials:

```bash
go build ./...
go vet ./...
gofmt -l .            # must print nothing
go test ./...
go test -race -count=1 ./...
```

Unit tests are offline and credential-free. Integration tests are the only tests
that may require the network or credentials.

## Contributing

See [CONTRIBUTING.md](./CONTRIBUTING.md). All commits must be DCO-signed
(`git commit -s`) and follow Conventional Commits referencing the run and task
IDs. New runtime dependencies require an ADR; never edit generated code under
`gen/`; never auto-retry order mutations.

## Security

See [SECURITY.md](./SECURITY.md). The SDK holds no platform credentials and no
developer private key; the bundled platform public keys are public reference
data. The only sensitive value the SDK handles is the plaintext trade password,
which it encrypts on the wire and never logs.

## License

[Apache License 2.0](./LICENSE). See [NOTICE](./NOTICE),
[DISCLAIMER.md](./DISCLAIMER.md), and
[THIRD_PARTY_NOTICES.md](./THIRD_PARTY_NOTICES.md).
