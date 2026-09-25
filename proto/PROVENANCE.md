# Protobuf vendor provenance

This directory holds the **official HStong (华盛) Quant OpenAPI protobuf
definitions**, vendored verbatim from the vendor's own SDK download. It is the
type source of truth for the generated code under `gen/hstong/...`.

## Source

| Item | Value |
|------|-------|
| Download page | https://quant-open.hstong.com/api-docs/quick-start/openapi-sdk-download.html |
| Package | HStong OpenAPI SDK — protobuf definitions, **v2.2.0** |
| Archive filename (as downloaded) | `华盛通OpenAPI-SDK-PB.zip` |
| Archive SHA-256 | `74367ceeefaa1aac1153b9c28afcf5aa68856ab1bdcd983dde697a6318759329` |
| Archive size | 9,229 bytes |
| Extraction date | 2026-09-21 |
| Extracted by | task T03 (data), run `2026-09-21-hstong-full-surface` |

Re-download the archive, verify the SHA-256 above, then extract it over this
directory.

## Line endings

The committed tree is not literally byte-for-byte identical to the archive, and
should not be described as such. `.gitattributes` applies `text=auto eol=lf` to
`*.proto`, so git normalises CRLF to LF in the repository. A `.proto` file that
used CRLF in the vendor archive is therefore stored here with LF endings.

This is the only transformation, and it is applied by git rather than by any
tool in this repository: no file is re-encoded, reformatted, or hand-edited. It
is also load-bearing, not cosmetic. buf normalises comments differently for CRLF
and LF input, so before `.gitattributes` was added, `make proto-verify` reported
drift that depended on the contributor's platform.

To compare against the archive, normalise line endings on both sides before
hashing, or read the SHA-256 values in the table below as authoritative.

## Extracted files (17)

All files are copied unmodified; none are re-encoded, reformatted, or
hand-edited. The Chinese comments are UTF-8 in the vendor archive and are
preserved as-is.

| # | Relative path | Bytes | SHA-256 |
|---|---------------|-------|---------|
| 1 | `common/constant/NotifyMsgType.proto` | 450 | `4f47c2b84b4bd1ad76146f5ca455eeb00ed7f8332b94f914ab75a65379ec4d5c` |
| 2 | `common/msg/HeartBeat.proto` | 169 | `122c0e1e68291d2285b891d8f88fb5341ddaf98b72a9d179e3102c82c28bcdd9` |
| 3 | `common/msg/Notify.proto` | 368 | `c094a2f2be959b155912b2f329c126b5073a81334d5d396105978c9646cfdd7a` |
| 4 | `hq/dto/BasicQot.proto` | 1375 | `e3f27a54a43d17d6a509caebc823c75d4fec582ab76a21e38f9880f762f2b668` |
| 5 | `hq/dto/Broker.proto` | 316 | `a277c534b1d17eca41c17f87bc1416b5c935b77d4105f0bf8826d472e18c99b4` |
| 6 | `hq/dto/FutureBasicQotExData.proto` | 622 | `c3e2b33a3e9e1e3cfba909ae1be1bc868cabb825d57e2bddd5912d343f32276c` |
| 7 | `hq/dto/KLine.proto` | 597 | `8d4fb31bc5489b6b2a9abd2440827afbff1377974f322caa02683a1c536f0b55` |
| 8 | `hq/dto/OptionBasicQotExData.proto` | 711 | `6f1cdb8e6ca34b22036460ec32f2e040d14bba7edefa2a9478a62418a38114d0` |
| 9 | `hq/dto/OrderBook.proto` | 366 | `1c9bd2f3ff0651ed6a2e68447049b12b21b7cc991257a3624ebdf6a83bc7dbf6` |
| 10 | `hq/dto/Security.proto` | 243 | `e70cb9f567d56426524cae1cda6873988b1200805e2a80c3a362d63a0c034bdd` |
| 11 | `hq/dto/Ticker.proto` | 533 | `63405b665dd048427cb6c4cf3f32d4190d3330f0f3c344de51ed1a5ae6fbe490` |
| 12 | `hq/dto/TimeShare.proto` | 444 | `6f48150390769a50af00fe304726418cd6aa5a1acd7005deb6d0f65c4669c148` |
| 13 | `hq/notify/BasicQotNotify.proto` | 318 | `33c0c9c6284463da0eee80183ed91d72955cb27434954cda62ed8ddd431eed92` |
| 14 | `hq/notify/BrokerNotify.proto` | 334 | `f779c243ed401a3e33ab78035c92f2a8e5986c9a09e293a94961ac4fa0cf3da4` |
| 15 | `hq/notify/OrderBookFullNotify.proto` | 410 | `e7b0c8853b1965e31989cf9ba7b0f291e268a0af958361cee272ab619d0a5e98` |
| 16 | `hq/notify/TickerNotify.proto` | 302 | `155cc74d303771497ab21d8fe62bae374be0a016d0e3f7d0117893401e392b70` |
| 17 | `trade/notify/TradeStockDeliverNotify.proto` | 1174 | `33db5d182f5101386c51753614c5a31384fcce9495ddd7306d9e92769dd351cf` |

## What this package contains — and what it does not

The v2.2.0 package is **partial by design**. It contains the push `PBNotify`
envelope, the market-data DTOs, and the market/trade notify payloads, but it
contains **no HTTP request/response messages**: the wire bodies for trade,
futures, algo, assets, and session calls are not published as proto in this
package. Those are modelled as hand-written Go structs. See
[ADR 0002](../docs/adr/0002-hybrid-codec.md).

## Intentionally excluded

The archive itself ships only the 17 files above. The **legacy
direct-to-platform protocol** protobuf definitions (`Request.proto`,
`Response.proto`, `RequestMsgType.proto`, `ResponseMsgType.proto`, the
`InitConnect*` messages, and the `common/response/*` wrappers) live only in a
separate, unrelated SDK archive (the Python/Java direct-protocol packages) and
are **deliberately not vendored here**. This SDK targets the local Gateway, not
the deprecated direct protocol. See
[ADR 0001](../docs/adr/0001-gateway-transport.md).
