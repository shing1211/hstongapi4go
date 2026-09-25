# Threat Model

This document is the adversarial analysis of the SDK. It complements
[Security](./security.md), which describes how to *use* the SDK safely: the trust
boundary, the trade password, and the redaction rules an application should
follow. This page asks the other question: **what can go wrong, and what does the
SDK already stop?**

Scope: the code in this repository as of **v0.1.6 + E17**. The canonical endpoint
inventory is [SPEC.md](./SPEC.md); it is not restated here.

## Assets

Ranked by the damage a compromise causes.

| # | Asset | Where it lives | Consequence of disclosure |
|---|-------|----------------|--------------------------|
| A1 | Trade password | `client.Config`, `client/env.go`, process memory | Account takeover on the broker |
| A2 | Trading session token | `pkg/hstong.SessionManager`, `internal/auth` | Authenticated access until expiry |
| A3 | Order state (live positions) | `pkg/hstong/*` responses, push stream | Financial loss, regulatory exposure |
| A4 | Account and fund identifiers | responses, logs, traces | Customer-data breach, PII leak |
| A5 | Money and quantity values | `pkg/types`, `pkg/domain` | Silent misstatement, bad trades |

## Trust boundaries

```
  application process                     trusted
  ┌──────────────────────────────────────────────────────┐
  │ client · internal/transport · internal/push           │
  │ internal/logging · internal/otel · pkg/*              │
  └───────────────┬──────────────────────┬───────────────┘
       HTTP :11111│                TCP :11112
  ════════════════╪══════════════════════╪════════════════  B1: process <-> Gateway
  ┌───────────────▼──────────────────────▼───────────────┐
  │ HStong Gateway (local, user-run)                      │  semi-trusted
  └───────────────┬──────────────────────────────────────┘
                  │ platform connection
  ┌───────────────▼──────────────────────────────────────┐
  │ HStong platform / clearing                            │  external
  └──────────────────────────────────────────────────────┘
```

| Boundary | What crosses it | Assumption |
|----------|-----------------|------------|
| B1 process → Gateway | password ciphertext, token, order mutations, account data | The Gateway is correctly installed and not tampered with. It is **not** assumed bug-free: a confused or hostile local process may impersonate it. |
| B2 process → observability backend | logs, spans, metrics | The backend has a different audience and a longer retention than the trading process. Anything sent there must be safe to show to an operator. |
| B3 process → local disk / env | password from env, `HSTONG_*` config | Environment and logs may be readable by other local users or captured by crash reporters. |

The SDK makes **no** cryptographic claim about B1: it trusts the loopback socket
and the Gateway binary. Anything stronger would require authenticating the
Gateway, which is out of scope (ADR 0001, ADR 0005).

## Which layer is live

This matters when reading the tables below, because the repository contains two
SDK layers and only one is reachable by a caller today.

| Layer | Packages | Status |
|-------|----------|--------|
| **Released (v0.1.x)** | `client`, `internal/transport`, `internal/push.Client`, `internal/resilience`, `internal/errs`, `internal/crypto`, `internal/session`, `internal/logging`, `pkg/hstong/*`, `pkg/types` | **Active.** This is what `pkg/hstong` and `client` callers use. |
| **v-next** | `pkg/domain`, `pkg/services`, `pkg/transport.Adapter`, `internal/auth`, `internal/push.Manager`, `internal/push.Fanout`, `internal/otel` | **Not wired in.** No production package imports these yet; only `scripts/coverage_gate.go` does, for the coverage gate. |

A **Mitigated** row therefore means one of two things, and the distinction is
called out where it matters:

- **Active** — the control runs in code a current caller already executes.
- **Forward-looking** — the control is implemented and tested in the v-next layer
  but will not protect anyone until that layer is wired into the public client.

The F-numbered fixes in the risk register are tagged accordingly.

## Adversary model

| Adversary | Capability | In scope |
|-----------|------------|----------|
| A-1 Malicious local process | Can bind :11111/:11112 first, or run as the same user | Yes — this is why no secret is ever sent to the Gateway in the clear |
| A-2 Network observer | Sees loopback traffic only | Weak — loopback is not encrypted, so a local observer sees the login ciphertext |
| A-3 Log/traces reader | Full read access to the observability backend | Yes — drives the redaction requirements |
| A-4 Accidental misconfiguration | Wrong endpoint, wrong env, runaway retry | Yes — drives the resilience bounds |
| A-5 Malicious Gateway | Returns crafted bodies, codes, or frames | Yes — drives input validation and redaction of untrusted text |
| A-6 Honest-but-careless caller | Passes a raw path, an oversized page, a stale token | Yes — drives defence in depth |

## Scenario analysis

Each of the seven areas from the E17 plan, with what the SDK does today.

### 1. Secret leakage

| Threat | Status |
|--------|--------|
| Password in logs | **Mitigated.** The SDK's own log sites emit only `op`, `route`, and `category`; request bodies are never logged. `logging.Redacting` masks any sensitive attribute a caller passes. |
| Password in an error | **Mitigated (E17).** A non-2xx body is passed through `logging.RedactText` before being embedded in an error, so an echoing Gateway cannot inject a password into the error text. |
| Password in a trace | **Mitigated (E17).** `otel.SafeError` redacts key-shaped secrets before `EndSpan` and the client span factory record an error. |
| Account identifier in logs | **Mitigated (E17).** `accountid` and `fundaccount` are redacted, matching ADR 0009. |
| Secret in free prose | **Not mitigated.** Redaction is key-driven. A secret in text with no recognisable key is undetectable; see the risk register. |
| Password in process memory | **Accepted.** The password is a Go `string` and cannot be zeroed. It is encrypted before it leaves the process. |
| Trade-password ciphertext replay | **Not mitigated.** AES-ECB is deterministic with no nonce. The protocol specifies it, so the SDK cannot prevent replay of an observed login ciphertext (see the risk register). |

### 2. Token replay and session integrity

| Threat | Status |
|--------|--------|
| Reusing a token after re-login | **Mitigated.** A second `Login` replaces the stored session, so a captured token no longer resolves. |
| Reusing a token after logout | **Mitigated.** `Logout` clears the session and the next authenticated call fails closed. |
| Kicked session (1013) reused | **Mitigated.** `ReLoginRequired` covers 1012/1013/1014/20033 and forces a fresh login. |
| Token theft via logs or traces | **Mitigated.** `token`, `accesstoken`, `refreshtoken`, and `sessionid` are redacted keys. |
| **No client-side replay defence** | **Not mitigated.** There is no nonce or idempotency key in the protocol, so a captured token is replayable until it expires. This is a property of the wire contract, not an SDK bug. |

### 3. Clock drift

| Threat | Status |
|--------|--------|
| Forward drift past expiry | **Mitigated.** `IsExpired` is `now >= Expiry`; a token is rejected at and after its expiry, with tests at 59 and 61 minutes against a 1-hour TTL. |
| Backward drift reviving a token | **Accepted.** Expiry is a pure comparison against the caller's clock with no monotonic component, so a backward jump makes an expired token acceptable again. Pinned by `TestClockDriftBackwardDoesNotCorruptState`. |
| Refresh window logic | **Mitigated.** `RefreshAt` opens 10 minutes before expiry; `TestTokenManagerSaveComputesExpiryAndRefresh` pins both boundaries. |

The SDK trusts the host clock. It does not implement clock-skew correction, and
it deliberately does not implement HMAC request signing (the Gateway owns
signing), so there is no skew policy to enforce.

### 4. Duplicate orders

The single most consequential failure mode: a retry that resubmits a mutation.

| Threat | Status |
|--------|--------|
| Auto-retry of a mutation | **Mitigated.** `resilience.ClassMutation` forces exactly one attempt at every configuration (ADR 0003). |
| Retry via a route **alias** | **Mitigated (E17).** `IsMutation` now strips the `Request` and `RequestMsgType` suffixes. Previously `/trade/TradeEntrustRequest` fell through to `ClassQuery` and became retryable. All three forms of all twelve mutation paths are tested. |
| Unknown/new mutation endpoint | **Residual risk.** Classification is a closed set. A mutation route added upstream but not added to `mutationSet` would be treated as a query and retried. `MutationPaths()` is the single place to update; a route-inventory test should be added when the Gateway surface grows. |
| Ambiguous failure (request sent, response lost) | **Mitigated by design.** The SDK returns a typed error instructing reconciliation and never resubmits. HStong provides no idempotency key, so this is the strongest available guarantee. |
| Duplicate detection / recovery | **Not implemented.** Status 1007 is surfaced as a typed non-retryable error, but there is no local duplicate-order cache or automated recovery. |

### 5. Reconnect storms

| Threat | Status |
|--------|--------|
| Unbounded reconnect loop | **Mitigated (E17).** `WithReconnectMaxRetries` now accepts only positive values and the bound is always enforced. Previously `0` silently meant *infinite*; a test measured **53 dials where 11 were expected**. |
| Reconnecting with an empty address | **Mitigated (E17).** The address passed to `Dial` is retained and reused. Previously `readLoop` cleared the connection before reconnecting, so the dialer received `""`. |
| Reconnect storm across many managers | **Residual risk.** Backoff is per-manager with no shared budget or jitter source control; a fleet of managers hitting one dead Gateway synchronises on similar backoff. |
| Half-open connection | **Not mitigated.** The push read path has no read deadline, so a silently dead TCP connection blocks until the OS gives up. |

### 6. Stale data

| Threat | Status |
|--------|--------|
| Missing a quiet topic | **Mitigated (E17).** `FreshnessMonitor.Record` now updates `LastSeen` even with no sequence number, so the dispatch path actually populates freshness. Previously it returned early on `seq == 0` and staleness never worked at all. |
| Duplicate event delivery | **Not mitigated.** `seqOf` returns a constant `0` because the Gateway envelope carries no per-event sequence, so the dedup guard never engages. Pinned by `TestFanout_DedupInertWithoutSequence`. |
| Sequence gap detection | **Not possible today.** Gap detection needs a wire sequence the protocol does not provide. `LastSeq` only advances when a non-zero sequence is supplied. |
| Staleness notification | **Partial.** `Check` is a pull API; the fanout does not push a stale or gap event to subscribers. |
| Freshness clock | **Residual risk.** `FreshnessMonitor` calls `time.Now` directly, so it cannot be driven by a test clock and inherits any host clock jump. |

### 7. Partial outage

| Threat | Status |
|--------|--------|
| Unbounded pagination | **Mitigated (E17).** Cursor walks stop on an empty page, an empty cursor, a non-advancing cursor, or `maxPages`; page size is clamped to 99. Previously a Gateway repeating a cursor looped forever. |
| Partial results discarded on error | **Mitigated (E17).** A mid-walk failure returns the pages already read **together with** the error, so a partial outage is visible instead of silently empty. |
| Oversized response | **Partial.** The v-next adapter caps the body with `http.MaxBytesReader`, but the active `internal/transport` executor still uses `io.ReadAll` with no cap. |
| Query retry amplifying load | **Mitigated.** Query-only retry with exponential backoff, rate limiting, and separate query/mutation circuit breakers. |
| Non-2xx classified as retryable | **Gap.** HTTP 500/503 map to an empty status code and are therefore **not** retryable, even though the Gateway reports overload as `1011` inside a 200 envelope. |

## What E17 fixed

All five defects were confirmed in source before the change, and each fix is
backed by a test that fails against the pre-fix code.

| ID | Defect | Layer | Test |
|----|--------|-------|------|
| E17-F1 | `bodySnippet` embedded the first 256 bytes of a non-2xx body verbatim, so an echoing Gateway could put a password into an error and from there into a trace | Active | `TestBodySnippetRedactsSecrets`, `TestTransportDoDoesNotLeakBodySecret` |
| E17-F2 | `IsMutation` did an exact lookup, so `/trade/TradeEntrustRequest` classified as a retryable query and could duplicate an order | Active | `TestIsMutationRecognisesGatewayAliases`, `TestIsMutationAliasIsSingleAttempt` |
| E17-F3 | Cursor walks had no stalled-cursor, page-count, or page-size bound, and discarded completed pages on error | Forward-looking | `TestWalkFundJourPagesStopsOnStalledCursor` and six siblings |
| E17-F4 | `maxRetries == 0` meant *infinite* (measured at 53 dials where 11 were expected), and the dialer received an empty address after a read failure | Forward-looking | `TestManager_ReconnectRetriesAreBounded`, `TestManager_ReconnectNeverDialsEmptyAddress` |
| E17-F5 | `accountid` and `fundaccount` were not redacted, contradicting ADR 0009 | Active | `TestIsSensitiveKey` |
| E17-F6 | `EndSpan` and the client span factory recorded raw `err.Error()` into spans | Active (with the `otel` tag) | `TestEndSpanRedactsKeyShapedSecrets`, `TestTransportErrorIsSafeForSpans` |
| E17-F7 | `FreshnessMonitor.Record` returned early on `seq == 0`, so staleness detection never populated in the dispatch path | Forward-looking | `TestFreshnessMonitor_RecordWithoutSequence`, `TestFanout_DispatchPopulatesFreshness` |

## Risk register

Accepted or deferred, with the reason. Ordered by residual risk.

| # | Risk | Severity | Status | Rationale |
|---|------|----------|--------|-----------|
| R1 | No client-side replay defence (token or password ciphertext) | Medium | **Accepted** | The wire protocol defines neither a nonce nor an idempotency key. Adding one would require a protocol change and break compatibility (ADR 0003). |
| R2 | Route alias classification is a closed set | **High** | **Open (active path)** | A mutation route missing from `mutationSet` is retried. The public client validates canonical routes, so aliasing is blocked there, but `resilience` is shared and a future caller could pass a raw path. Needs a route-inventory test. |
| R3 | Half-open push connection blocks indefinitely | Medium | **Partially fixed (client.go)** | `WithReadDeadline` now arms a per-read deadline in the released `push.Client` read loop, bounding the gap between frames so a silent peer is detected. It is **off by default**: the local Gateway documents no heartbeat cadence on this stream (`docs/LEGACY.md` records that the Gateway path replaced heartbeat keep-alive with `SessionManager.StartKeepAlive` polling), so any non-zero default would risk tearing down a healthy connection in a quiet market. Fully closing this needs the observed inter-frame gap from a live Gateway run. |
| R4 | Dedup and gap detection inert | Medium | **Accepted (forward-looking)** | `push.Fanout` supplies no per-event sequence. Correct gap detection is impossible without one; pinned by `TestFanout_DedupInertWithoutSequence`. |
| R5 | Free-prose secrets are not redacted | Low | **Accepted (active path)** | Redaction is key-driven. Callers must keep secrets out of message text. |
| R6 | Backward clock jump can revive a token | Low | **Accepted (forward-looking)** | Would need a monotonic deadline. Low impact: the Gateway independently rejects an expired token. |
| R7 | No read cap in the active transport executor | Medium | **Open (active path)** | The cap exists only in the unused v-next adapter; `internal/transport` uses `io.ReadAll`. |
| R8 | HTTP 5xx not retryable | Low | **Open (active path)** | Would change documented retry semantics; the Gateway reports overload as `1011` inside a 200 envelope. |
| R9 | `pkg/transport.Adapter` unreachable and mis-wired | Low | **Open (forward-looking)** | `inner` is unused, the base URL is hardcoded, and `WithDeadline` stores one shared cancel that can cancel a previous caller. A trap for the next adopter. |
| R10 | `passwordHash` and bare `account` not redacted | Low | **Accepted (active path)** | `IsSensitiveKey` matches whole names and `account` is a common word. Chosen so support correlation keeps working; revisit if the data policy changes. |
| R11 | No secret scanning in CI | Medium | **Fixed (ADR 0012)** | A `secrets` job runs `gitleaks` as a pinned Action over the full history of the pushed ref (`fetch-depth: 0`). `.gitleaks.toml` allowlists three paths by justification — the public platform keys (ADR 0005), the vendored `proto/` tree, and the published AES test vector — and never disables a rule class. `go.mod` is untouched, so consumers gain no dependency. |
| R12 | `gosec`/`govulncheck` were pinned to `@latest` | Low | **Fixed (f8535b9)** | Both scanners are now pinned: `gosec@v2.22.10` and `govulncheck@v1.1.4` in `.github/workflows/ci.yml`, so a scanner result cannot change without a commit. Corrected 2026-09-25; the entry previously still read Open. |
| R13 | `main` is red in CI and v0.1.6 shipped that way | **High** | **Fixed (d20e0ed)** | Repaired after v0.1.6: the gofmt gate that was skipping four steps, 39 lint findings, a gosec install path that pointed at staticcheck's module, a GoReleaser config that did not parse, 14 standard-library vulnerabilities, and a platform-dependent codegen drift. Run 36025226892 is 9/9 green. Details in `evidence/P03-CI-repair.txt`. |
| R14 | `internal/auth` declares backoff, single-flight, and refresh that do not exist | Medium | **Open (forward-looking)** | `defaultRetryDelay`, `defaultMaxRetryDelay`, and `Authenticator.mu` are unused, and `doc.go` claims refresh and backoff behaviour that is not implemented. The public `SessionManager` provides single-flight `EnsureLoggedIn`; the v-next `Authenticator` does not. |

## What the SDK deliberately does not do

- **No auto-retry of order mutations**, at any layer, under any configuration
  (ADR 0003). Ambiguous outcomes require caller reconciliation.
- **No signing, key selection, or device binding.** The Gateway owns it
  (ADR 0001, ADR 0005).
- **No idempotency key on orders.** The protocol has no field for it.
- **No replay nonce.** Same reason.
- **No platform credentials or private keys in the repository.** The bundled
  platform public keys are public reference data.

## Verifying these claims

Each mitigated row above is backed by a test. The mapping is recorded in the run
evidence:

```
docs/runs/2026-09-23-hstong-enterprise-sdk/evidence/P03-E17-threat-model.txt
```

Reproduce the adversarial suite with:

```sh
go test ./internal/logging/... ./internal/transport/...
go test ./internal/resilience/... ./internal/auth/... ./internal/push/...
go test -tags otel ./internal/otel/...
```

## Related

- [Security](./security.md) — trust model, trade password, application redaction rules
- [COMPAT.md](./COMPAT.md) — feature-by-feature compatibility
- [Observability](./observability.md) — metrics, logs, and what they never carry
- [Errors](./errors.md) — status codes and retry classification
- ADR 0001 (Gateway transport), ADR 0003 (no auto-retry), ADR 0005 (key model),
  ADR 0008 (decimal money), ADR 0009 (observability), ADR 0011 (compatibility)
