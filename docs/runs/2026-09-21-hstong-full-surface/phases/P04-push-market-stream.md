# P04 — Push + Market Stream

- **Run:** 2026-09-21-hstong-full-surface · **Status:** done (gate passed)
- **Owner role:** backend · tester
- **Depends on:** P01 · **Plan ref:** [plan.md](../plan.md) §P4 · **Tracker:** [todos.md](../todos.md)

## Objective / Exit criteria

Implement the real-time push surface end to end: a TCP client that frames the
HStong 151-byte header and decodes `PBNotify`, the market subscribe/unsubscribe
endpoints, and a channel-based public stream API with reconnect + resubscribe.

Exit criteria:
- 151-byte framing (encode/decode/read/write) with magic, `bodyLen`, and
  `compressAlgorithm` validation.
- `PBNotify` decoded by `notifyMsgType`; the `Any` payload is unmarshalled
  directly, with no dependency on the global protobuf type registry.
- Reconnect with backoff + jitter and an `OnReconnect` hook; stream re-subscribes
  every active topic automatically and emits an `ErrReconnected` notice.
- Channel API: `Updates()` / `Errors()` / `Cancel`; leak-free under `goleak`.
- Golden-frame tests for 5+ notify types against a local TCP server.

## Tasks

| ID | Task | Role | Status | Deps | Acceptance | Verify | Evidence |
|----|------|------|--------|------|-----------|--------|----------|
| T14 | TCP push client (framing, `Any`, reconnect) | backend | done | T09 | 151B header parse; `Any` decode; reconnect + `OnReconnect` | `go test ./internal/push/... -race` | [P04-T14-T17.txt](../evidence/P04-T14-T17.txt) |
| T15 | Subscribe/Unsubscribe + topic set | backend | done | T14 | topics 11,14,16,17,25,26,27,28,35,36,37; validation | `go test ./pkg/hstong/market/... -race` | [P04-T14-T17.txt](../evidence/P04-T14-T17.txt) |
| T16 | Stream channel API | backend | done | T15 | `Updates()`/`Errors()`/`Cancel` | `go test ./pkg/hstong/stream/... -race` | [P04-T14-T17.txt](../evidence/P04-T14-T17.txt) |
| T17 | Push tests (golden frames) | tester | done | T16 | local TCP server; leak-free | `go test -race -count=1 ./...` | [P04-T14-T17.txt](../evidence/P04-T14-T17.txt) |

## Decisions & deviations

- **`Any` decode by enum, not registry.** The vendored protos declare no proto
  `package`, so `anypb.UnmarshalTo`/the global registry would be brittle.
  `internal/push.Decode` switches on `notifyMsgType` and calls
  `proto.Unmarshal(any.Payload.Value, concrete)` directly. The `Any.type_url` is
  never consulted; tests deliberately set a bogus `type_url` to prove it. This
  deviates from `docs/DESIGN.md` §3.3 ("unpacked via the full 17-proto `Any`
  registry"); DESIGN is out of scope to edit in this phase, so the deviation is
  recorded here.
- **Backpressure = drop-oldest.** Each handler type has one dispatcher
  goroutine fed by a buffered channel (64). When a handler is slow the read loop
  discards the oldest queued notification and enqueues the newest, so the read
  loop never blocks. `Errors()` uses the same policy (depth 16). The stream
  `Updates()`/`Errors()` channels (default 64, `WithBuffer`) also drop oldest.
- **Heartbeat frames are ignored.** The read loop skips `MsgHeartbeat`,
  `MsgRequest`, and `MsgResponse` frames; only `MsgPush` is decoded. The client
  does not send heartbeats in this phase.
- **`bodySHA1` preserved, never verified.** The raw 128-byte signature stays on
  `push.Header` for the opt-in verifier wired in P10/T32.
- **Subscribe wire shape confirmed from the official Python SDK v2.3.0** (the
  vendored `华盛通OpenAPI-SDK-Python.zip`): params are
  `{"topicId": <int>, "security": [{"dataType": n, "code": "..."}, ...]}`. The
  existing `market` package uses the hand-written JSON codec, matching this.
- **Codec = JSON, not ProtoJSON.** Subscribe/Unsubscribe bodies are plain JSON
  and the response `data` is discarded (`nil` out), avoiding a guess at the
  response wrapper shape. This follows the per-endpoint codec choice in ADR
  0002.
- **Stream owns the push client, not the HTTP client.** `stream.Client`
  reuses the caller's `client.Client` for HTTP and creates its own
  `internal/push.Client`. `Close` is idempotent and does not close the caller's
  HTTP client.
- **Connect context bounds connection lifetime.** The context passed to
  `Connect` bounds the dial and the connection lifetime (cancel it to stop the
  stream); `Subscribe`/`Cancel` take their own context for the HTTP call.
- **Resubscribe is aggressive on reconnect.** Every active subscription is
  re-issued via `/hq/Subscribe` after a reconnect; a failure is surfaced on that
  subscription's `Errors()` but does not tear down the others.
- `internal/push` and `pkg/hstong/stream` are new packages; no existing package
  outside the allowed set was modified. `pkg/hstong/market/subscribe.go` was
  added to the existing `market` package.

## Files created / modified

- `internal/push/doc.go`
- `internal/push/frame.go`
- `internal/push/client.go`
- `internal/push/frame_test.go`
- `internal/push/client_test.go`
- `pkg/hstong/market/subscribe.go`
- `pkg/hstong/market/subscribe_test.go`
- `pkg/hstong/stream/doc.go`
- `pkg/hstong/stream/stream.go`
- `pkg/hstong/stream/stream_test.go`
- `docs/runs/2026-09-21-hstong-full-surface/phases/P04-push-market-stream.md` (this file)
- `docs/runs/2026-09-21-hstong-full-surface/evidence/P04-T14-T17.txt`

## Verification evidence

| # | Command | Result | Artifact |
|---|---------|--------|----------|
| 1 | `gofmt -l .` | empty | [P04-T14-T17.txt](../evidence/P04-T14-T17.txt) |
| 2 | `go build ./...` | ok | [P04-T14-T17.txt](../evidence/P04-T14-T17.txt) |
| 3 | `go vet ./...` | ok | [P04-T14-T17.txt](../evidence/P04-T14-T17.txt) |
| 4 | `go test ./internal/push/... -race -count=1 -v` | PASS | [P04-T14-T17.txt](../evidence/P04-T14-T17.txt) |
| 5 | `go test ./pkg/hstong/market/... -race -count=1 -v` | PASS | [P04-T14-T17.txt](../evidence/P04-T14-T17.txt) |
| 6 | `go test ./pkg/hstong/stream/... -race -count=1 -v` | PASS | [P04-T14-T17.txt](../evidence/P04-T14-T17.txt) |
| 7 | `go test -race -count=1 ./...` | ok (all packages) | [P04-T14-T17.txt](../evidence/P04-T14-T17.txt) |
| 8 | `python scripts/check_money.py` | money-check OK | [P04-T14-T17.txt](../evidence/P04-T14-T17.txt) |

## Blockers / follow-ups

- **P06/P07 must extend `stream`** with trade and futures notify handlers
  (`TradeStockDeliverMsgType`, `FuturesTradeStockDeliverMsgType`) and their
  HTTP subscribe routes. The stream fan-out already filters by topic; trades
  will need their own registration path.
- **P09 mock gateway** should reuse `push.WriteFrame`/`push.ReadFrame` for its
  TCP listener; the subscribe request shape it must serve is
  `{"topicId","security"}`.
- **P10/T32** should consume `push.Header.BodySHA1` for opt-in verification.
- `stream` currently registers the four market notify types unconditionally at
  `New`; that is cheap and leaves no goroutine behind when `Close` is called,
  but P06/P07 will add more.

## Gate

- Exit criteria met: yes — framing + enum decode + reconnect/resubscribe + channel
  API covered by golden-frame, reconnect, and leak tests; `go test -race ./...`
  and `check_money` green.
