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

## Risk register

Accepted or deferred, with the reason. Ordered by residual risk.

| # | Risk | Severity | Status | Rationale |
|---|------|----------|--------|-----------|
| R1 | No client-side replay defence (token or password ciphertext) | Medium | **Accepted** | The wire protocol defines neither a nonce nor an idempotency key. Adding one would require a protocol change and break compatibility (ADR 0003). |
| R2 | Route alias classification is a closed set | **High** | **Open** | A mutation route missing from `mutationSet` is retried. Mitigated today because the public client validates canonical routes, but `Adapter.Do` accepts a raw string. Needs a route-inventory test. |
| R3 | Half-open push connection blocks indefinitely | Medium | **Open** | Needs a read deadline or heartbeat-liveness check. Deferred because it changes the read loop's blocking contract. |
| R4 | Dedup and gap detection inert | Medium | **Accepted** | The Gateway sends no per-event sequence. Correct gap detection is impossible without one; the limitation is now pinned by a test. |
| R5 | Free-prose secrets are not redacted | Low | **Accepted** | Redaction is key-driven. Detecting arbitrary secrets in prose is unreliable in both directions; callers must keep secrets out of message text. |
| R6 | Backward clock jump can revive a token | Low | **Accepted** | Would need a monotonic deadline. Low impact: the Gateway independently rejects an expired token. |
| R7 | No read cap in the active transport executor | Medium | **Open** | The cap exists only in the unused v-next adapter. |
| R8 | HTTP 5xx not retryable | Low | **Open** | Would change documented retry semantics; the Gateway uses `1011` in a 200 envelope. |
| R9 | `pkg/transport.Adapter` unreachable and mis-wired | Low | **Open** | `inner` is unused, the base URL is hardcoded, and `WithDeadline` stores one shared cancel that can cancel a previous caller. Dead code today; a trap for the next adopter. |
| R10 | `passwordHash` and bare `account` not redacted | Low | **Accepted** | `IsSensitiveKey` matches whole names, and `account` is a common word. Chosen deliberately so support correlation keeps working; revisit if the data policy changes. |
| R11 | No secret scanning in CI | Medium | **Open** | `gosec` and `govulncheck` run, but nothing scans commits for credentials. |
| R12 | `gosec`/`govulncheck` pinned to `@latest` | Low | **Open** | Non-reproducible CI; a new upstream release can change results without a commit. |

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
