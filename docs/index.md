# hstongapi4go

An idiomatic Go SDK for the **HStong (华盛) Quant OpenAPI local Gateway**.

The Gateway is a process HStong installs and runs on your machine. It exposes an
HTTP request surface on `http://127.0.0.1:11111` and a TCP push surface on
`127.0.0.1:11112`. The SDK talks only to that local Gateway; it never bundles,
launches, or manages it, and it performs no platform signing or request
encryption.

- **Module:** `github.com/shing1211/hstongapi4go`
- **License:** Apache-2.0
- **Go:** 1.26+
- **Gateway:** v2.4.1 (HTTP + TCP push)
- **Protocol package:** HStong protobuf definitions v2.2.0

The legacy direct-to-platform protocol (developer RSA signing, AES-ECB dynamic
keys, heartbeat, device binding) is **documented but not implemented** — see
[Legacy Protocol](LEGACY.md).

## What the SDK covers

| Surface | Package | Endpoints |
|---------|---------|-----------|
| Market data pull | `pkg/hstong/market` | 9 |
| Market subscribe | `pkg/hstong/market` | 2 |
| Trade session | `pkg/hstong` | 2 |
| Trade assets / positions | `pkg/hstong/trade` | 5 |
| Trade orders | `pkg/hstong/trade` | 13 |
| Trade push subscribe | `pkg/hstong/trade` | 2 |
| Algo / strategy | `pkg/hstong/algo` | 7 |
| Futures | `pkg/hstong/future` | 11 |
| **Total HTTP** | | **51** |
| Market push topics | `pkg/hstong/market`, `pkg/hstong/stream` | 11 |

Canonical endpoint and schema counts live in the [API Reference](SPEC.md).

## Quick start

```go
package main

import (
	"context"
	"log"
	"time"

	"github.com/shing1211/hstongapi4go/client"
	"github.com/shing1211/hstongapi4go/gen/hq/dto"
	"github.com/shing1211/hstongapi4go/pkg/hstong/market"
	"github.com/shing1211/hstongapi4go/pkg/types"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	c, err := client.New(client.WithEnv())
	if err != nil {
		log.Fatal(err)
	}
	defer c.Close()

	m := market.New(c)
	q, err := m.BasicQot(ctx, market.BasicQotRequest{
		Security: []*dto.Security{
			{DataType: int32(types.DataTypeHKStock), Code: "0700.HK"},
		},
		MktTmType: 1,
	})
	if err != nil {
		log.Fatal(err)
	}
	for _, quote := range q.BasicQot {
		log.Printf("%s %v", quote.GetSecurity().GetCode(), quote.GetLastPrice())
	}
}
```

More runnable programs live under [`examples/`](https://github.com/shing1211/hstongapi4go/tree/main/examples).

## Where to go next

- [Installation](getting-started.md) — add the module and build the examples.
- [Configuration](configuration.md) — `client` options and `HSTONG_*` variables.
- [Authentication](authentication.md) — trade session login and keep-alive.
- [Protocol](protocol.md) — HTTP envelope, route aliases, hybrid codec, push framing.
- [Market Data](market-data.md), [Trading](trading.md), [Futures](futures.md), [Algo](algo.md),
  [Streaming](streaming.md).
- [Error Codes](errors.md) — the Gateway status table and how the SDK reports failures.
- [Mock Gateway](mock-gateway.md) — run the whole SDK offline.
