# Third-Party Notices

`hstongapi4go` is distributed under the Apache License, Version 2.0 (see
[LICENSE](./LICENSE)). This file records third-party components referenced or
vendored by the project and their respective licenses.

## Vendored protobuf definitions — `proto/`

The `.proto` files vendored under `proto/` are copied unmodified from the
**official HStong Quant OpenAPI SDK** protobuf distribution (package version
v2.2.0), as published by HStong / 华盛. They describe the wire types used by
market-data DTOs and real-time push notifications.

These files remain the property of their original authors. They are included
here for interoperability only; no license or right to redistribute is claimed
by this project. See `PROVENANCE.md` (added in a later phase) for the exact
source archive and checksums. If you are the rights holder and object to this
inclusion, please open an issue and the files will be removed.

## Protocol Buffers

- **Protocol Buffers** — https://protobuf.dev/ — BSD-3-Clause.
- **google.golang.org/protobuf** (Go runtime, once generated code is adopted) —
  BSD-3-Clause.

## Build / developer tooling

These are used only in development and CI; they are not distributed with the
SDK.

- **golangci-lint** — https://github.com/golangci/golangci-lint — GPL-3.0.
- **google/addlicense** — https://github.com/google/addlicense — Apache-2.0.
- **buf** — https://github.com/bufbuild/buf — Apache-2.0.

## HStong Gateway

The HStong Quant OpenAPI Gateway itself is **not** distributed with this
project. Users install it separately and are bound by HStong's own terms.
