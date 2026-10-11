# Proposal

## Why

The SDK is at v1.0.3 with a stable semver API, a mature 13-job CI pipeline,
comprehensive unit tests, and a full ADR decision trail. Yet the README still
carries an "alpha" badge whose sole justification is that the wire codec for
market payloads has never been validated against a live HStong Gateway. This
change tracks the checklist to close that gap, formalize the security and
support commitments that production users need, and remove the remaining
barriers to declaring the library production-ready.

## What Changes

1. **Run integration tests against a live Gateway** — execute
   `test/integration/` (gated by `HSTONG_INTEGRATION=1`) against a real
   account, confirming `int64` wire codec assumptions and envelope nesting
   across market, streaming, and trade surfaces.

2. **Drop the alpha status badge** — update README.md top badge from
   `Status: alpha` to `Status: stable` once P1 is confirmed green.

3. **Formalize security vulnerability SLA** — ~~add a severity-to-SLA
   table to `SECURITY.md`~~ **decided against (2026-10-11, issue #19):**
   the existing SECURITY.md commitments stand; no table is added.

4. **Commission a third-party security audit** — engage an external audit
   of the trade password path, push frame verification, session token
   handling, and OTel build tag surface.

5. **Resolve HK tick table completeness** — either source the real HKEX
   price-band tick table or document the approximation boundary so callers
   know when to inject `domain.TickScheduleResolver`.

6. **Deprecate or commit the v-next layer** — `pkg/services` is
   feature-complete at 51/51 endpoints but not reachable by any caller.
   Either mark `pkg/hstong/*` deprecated with a migration timeline, or
   remove the v-next layer to reduce maintenance surface.

7. **Add public GitHub issue templates** — structured templates for bug
   reports and feature requests capturing Go version, Gateway version,
   and repro steps.

8. **Document cosign release artifact verification** — a short
   `docs/SIGNING.md` so end users can verify checksums and signatures
   without needing to reverse-engineer the checklist command.

9. **Publish a Gateway version compatibility matrix** — document which
   SDK version targets which Gateway version, and how to detect drift.

## Capabilities

This change is process and governance only — no spec-level behavior changes.
`skip_specs: true` is set in `.openspec.yaml`.

## Impact

- `README.md` — status badge text
- `SECURITY.md` — SLA table additions
- `docs/` — new `SIGNING.md`; possible `COMPATIBILITY.md`
- `.github/ISSUE_TEMPLATE/` — new template files
- `pkg/services/`, `pkg/domain/`, `pkg/transport/` — deprecation or removal
  decision
- `openspec/changes/production-readiness/` — change artifacts
