# Design

## Context

The SDK is v1.0.3 with a stable API, 13-job CI, 100 test files, and a full
ADR trail. The only thing keeping it at "alpha" is an untested assumption about
the live Gateway's wire format. Several governance and process items are also
outstanding before the library can present itself as production-ready to
institutional adopters.

See `proposal.md` for motivation. This document covers the approach for each
item.

## Goals / Non-Goals

**Goals:**
- Validate (or disprove) the `int64` wire codec assumption against a real
  Gateway, removing the last factual basis for the "alpha" label.
- Add the governance artifacts (SLA, issue templates, signing docs) that
  production users and security teams require.
- Resolve the v-next layer's status — either deprecate the old surface or
  retire the parallel one.
- Drop "alpha" from the README once P0 is confirmed.

**Non-Goals:**
- No changes to the wire protocol, API surface, or runtime behavior.
- No changes to generated code under `gen/`.
- No new runtime dependencies.

## Decisions

### D1 — Integration test runner

The integration test suite lives in `test/integration/` and is gated by
`HSTONG_INTEGRATION=1`. It is skipped by default and runs only against a
real Gateway because it requires:
- A live Gateway process (`127.0.0.1:11111/11112`)
- A real account's trade password (`HSTONG_TRADE_PASSWORD`)
- Market hours (Mon–Fri 09:00–18:00) for live data tests

**Approach:** The item is a **user-run verification task**, not an
implementation task. The owner needs a live account. The checklist documents
which tests to run and what to look for. The outcome is either a confirmed
green run (P0 closed) or a wire mismatch that requires a fallback codec fix
(ADR 0007 documents the path).

**Who can close this:** Anyone with a HStong account and API access. The
maintainer (shing1211) does not have a live account, so this must be
community-driven or a paid account holder.

---

### D2 — Alpha badge removal

**Approach:** One-line text change in `README.md` top badge section. Only
committed after D1 is confirmed green. Conditional on D1 — the badge change
commit should reference the integration test run that validated it.

---

### D3 — Security vulnerability SLA table

**Approach:** Add a severity-to-SLA table to `SECURITY.md`:

| Severity | First response | Fix target |
|----------|---------------|------------|
| Critical (CVSS 9–10) | 24 hours | 72 hours |
| High (CVSS 7–8.9) | 48 hours | 7 days |
| Medium (CVSS 4–6.9) | 1 week | 30 days |
| Low (CVSS <4) | 2 weeks | next release |

This mirrors common open-source security response norms (e.g., GitHub's
recommended SLAs). The table is advisory for a solo maintainer; it does not
create a binding contract but sets expectations.

**Decision: keep `SECURITY.md` as-is (2026-10-11, issue #19).** No SLA
table is added. The existing SECURITY.md already commits to acknowledgment
within 48 hours and initial assessment within 1 week, which is the honest
commitment for a solo maintainer; a four-tier table would overstate
capacity. The issue was closed with this rationale.

---

### D4 — Third-party security audit

**Approach:** Engage a professional security firm or an independent
security researcher. Scope:
1. Trade password AES-ECB path (`internal/crypto/`) — confirm the fixed
   public key is the only possible design given the Gateway's requirement.
2. Push frame SHA1WithRSA verification (`internal/push/verify_test.go`,
   `pkg/hstong/stream/verify_test.go`) — confirm the G505 suppression
   is justified by the localhost trust boundary.
3. Session token in-memory exposure — confirm no token appears in error
   messages or logs.
4. OTel build tag attack surface — confirm the optional OTel instrumentation
   does not expand the trust surface when disabled.

**Option A (pro-bono):** Post a public request for an independent audit.
Option B (paid): Budget ~$2–5k for a focused audit of a trading SDK.
Option C (self-documented): Expand the existing threat model with a
detailed proof-of-mitigation for each finding, which is what a third-party
auditor would produce anyway.

**Recommendation:** Option C first — the threat model is already thorough.
If institutional adoption is a goal, budget for Option B before declaring
full production readiness.

---

### D5 — HK tick table completeness

**Current state:** `services.WithTickSchedule` injects a
`domain.TickScheduleResolver`. The default table covers only `DataType`, not
price bands. The CHANGELOG [Unreleased] notes the band table is not filled
in because HKEX publishes it in an exchange document that must be manually
sourced.

**Approach:**
- Option A: Source the real HKEX tick table and hard-code it. Authoritative
  source: HKEX market data documentation or the Gateway's own master.
- Option B: Document the approximation boundary clearly in `docs/trading.md`
  so callers know they must inject their own resolver for HK instruments
  at non-standard price bands.
- Option C: Both — fill in the table from HKEX docs (A) and document the
  override path (B) so callers without a live master can still use the SDK.

**Recommendation:** Option C. The table is a lookup table with a finite
number of entries (price band → tick size). It can be compiled from public
HKEX data and added to `pkg/domain/` or `pkg/services/`.

---

### D6 — v-next layer deprecation or retirement

**Current state:** `pkg/services`, `pkg/domain`, `pkg/transport`,
`internal/auth` are implemented and tested (51/51 endpoint parity via
`make parity-enforce`) but not reachable by any public caller. The layer
was built as a staging ground for v1.0 but the migration from `pkg/hstong/*`
has not started.

**Approach:**
- Option A (deprecate): Mark `pkg/hstong/*` deprecated with a Go doc comment
  pointing to `pkg/services`. Set a 6-month notice period (per ADR 0011).
  Commit to a v2.0 removal timeline.
- Option B (retire): Remove the v-next layer entirely. Keep `pkg/hstong/*`
  as the permanent surface. Simplifies the codebase.
- Option C (status quo): Keep both, mark v-next as alpha, continue without
  committing to either path.

**Decision: Option A (2026-10-11).** Deprecate `pkg/hstong/*` with a
migration path to `pkg/services`. Set a 1-month notice period ending
**2026-11-11**. v2.0 removal targeted for 2027. The v2.0 removal ADR is
tracked in T6.4.

**v2.0 specifics (confirmed 2026-10-11):** `pkg/services` import path
stays as-is; `pkg/hstong/*` is deleted in v2.0 (not promoted to replace
it). The two surfaces coexist until v2.0, then `pkg/hstong/*` is removed.

---

### D7 — GitHub issue templates

**Approach:** Create `.github/ISSUE_TEMPLATE/` with:
- `bug_report.yml` — fields: Go version, Gateway version, SDK version,
  `HSTONG_INTEGRATION` status, minimal repro steps, expected vs actual
- `feature_request.yml` — fields: use case, alternatives considered,
  API sketch if applicable

Standard GitHub Forms format. No fields beyond what is genuinely useful
for a trading SDK.

---

### D8 — Cosign verification documentation

**Approach:** Create `docs/SIGNING.md` covering:
1. What the signature proves (artifacts came from this repo's CI)
2. How to download checksums, bundle, and one archive from the release page
3. The `cosign verify-blob` command with the correct certificate identity
4. `sha256sum --check` to verify the archive integrity

Keep it to one page. Reference `docs/RELEASE_CHECKLIST.md` step 16 for the
canonical command.

---

### D9 — Gateway version compatibility matrix

**Approach:** Create `docs/COMPAT.md` (or extend `docs/SPEC.md` §1) with:

| SDK version | Gateway version | Notes |
|-------------|----------------|-------|
| v1.0.3 | v2.4.1 | Current |
| v0.1.x | v2.4.x | Likely compatible |

The matrix is minimal because the SDK does not negotiate a protocol version
with the Gateway — it sends requests and the Gateway responds. Compatibility
is confirmed by integration tests, not by version matching. Document that
drift is detected by running the integration suite against a new Gateway
version.

## Risks / Trade-offs

[Risk D1] → No one with a live account runs the integration tests.
**Mitigation:** Document the test clearly in `docs/integration-testing.md`
and post a GitHub Discussion asking for a volunteer with a live account to
run them. Close the issue once one run is confirmed.

[Risk D4] → Third-party audit reveals new vulnerabilities.
**Mitigation:** This is the point of the audit. Fix any findings before
declaring production-ready. A discovered and fixed defect is better than an
undiscovered one in a trading library.

[Risk D6] → Retiring v-next means losing work already done.
**Mitigation:** The v-next code is committed and the tests are in the repo.
If a future migration is desired, the code can be restored from git.
Retiring it from `main` reduces maintenance burden now.

## Open Questions

**O1:** Who will run the integration tests (D1)? The maintainer does not
have a live account. Is there a community member or beta tester who can?

**O2:** What is the timeline for the third-party security audit (D4)?
This item can be started independently of D1, but it has a longer lead time.

**O3:** Should `pkg/services` be deprecated (Option A in D6) or removed
(Option B in D6)? The recommendation is Option B (remove), but the user
may prefer the deprecation path if they intend to drive migration later.
