# Tasks

## 1. Live Gateway integration testing

> Owner: community / account holder. The maintainer has no live account.
> Verification: all tests in `test/integration/` pass with `HSTONG_INTEGRATION=1`.

- [ ] 1.1 Post a GitHub Discussion asking a live-account holder to run
  the integration suite — copy the commands from `docs/integration-testing.md`
  §Run verbatim. Reference `TestIntegration_WireCodecProbe` as the
  critical test to confirm.
- [ ] 1.2 Once a green run is confirmed, record the run's date, Gateway
  version, and account region in `docs/integration-testing.md` §Verification.
- [ ] 1.3 If `TestIntegration_WireCodecProbe` fails — follow ADR 0007's
  fallback path: switch the affected endpoint's market decoder from
  `encoding/json` to `json.RawMessage` + helper, update the ADR, and
  re-run. Do not change `gen/` types. Repeat until green.

## 2. Drop alpha badge

> Verification: `grep -i alpha README.md` returns zero matches.

- [ ] 2.1 Update `README.md` top badge from `Status: alpha` to
  `Status: stable`. Commit message references the integration test run
  (task 1.2's record).

## 3. Formalize security vulnerability SLA — decision: keep `SECURITY.md` as-is

> Decision (2026-10-11, issue #19): no severity-to-SLA table. The existing
> SECURITY.md commitments (48-hour acknowledgment, 1-week assessment) stand;
> a four-tier table would overstate a solo maintainer's capacity.
> Recorded in design.md D3.

- [x] 3.1 ~~Add the severity-to-SLA table to `SECURITY.md` §Response
  timeline.~~ Not done by decision — see the section note above.

## 4. Third-party security audit

> Verification: audit report or public thread is linked from
> `SECURITY.md` or `docs/security.md`.

- [ ] 4.1 Post a public request for an independent security audit
  (GitHub Discussion or security advisory). Scope the four areas from
  design.md D4: trade password path, push SHA1WithRSA verification,
  session token exposure, OTel build tag surface.
- [ ] 4.2 If findings are reported, fix each one and add a regression
  test to the appropriate `_test.go` file.
- [ ] 4.3 Link the audit result (or the "no findings" thread) from
  `SECURITY.md`.

## 5. Resolve HK tick table

> Verification: either the HKEX band table is populated in `pkg/domain/`
> or `docs/trading.md` documents the approximation boundary for callers.

- [ ] 5.1 Source the authoritative HKEX price-band tick table from public
  exchange documentation. Populate `pkg/domain/tick_schedule.go` or the
  existing `services.TickSchedule` table.
- [ ] 5.2 If the table cannot be fully sourced, add a clearly-worded
  warning in `docs/trading.md` that HK price-band validation uses an
  approximation and callers with strict requirements must inject
  `domain.TickScheduleResolver`.

## 6. v-next layer — deprecate `pkg/hstong/*` (Option A chosen)

> Owner: shing1211
> Verification: `pkg/hstong/` Go doc comments contain `@deprecated` and
> point to `pkg/services`. `docs/MIGRATION.md` has a v1→v2 section.
> CHANGELOG entry records the 1-month deprecation window (2026-11-11).

- [x] 6.1 Add `@deprecated` Go doc to all public symbols in
  `pkg/hstong/*`, pointing to `pkg/services` as the migration target.
  Add to: `Client`, `SessionManager`, `market.New`, `trade.New`,
  `future.New`, `algo.New`, `stream.New`, all exported request/response
  types, and all exported errors. Run `make build` to confirm no broken
  import cycles.
- [x] 6.2 Update `docs/MIGRATION.md` with a v1-to-v2 section:
  before/after code samples for the most common calls (`BasicQot`,
  `TradeLogin`, `GetFunds`, `PlaceOrder`), per-package migration notes,
  and the 1-month notice timeline ending 2026-11-11.
- [x] 6.3 Add CHANGELOG entry: "Deprecated: `pkg/hstong/*` is deprecated
  in favor of `pkg/services`; removal targeted for v2.0 in 2027 (approx.
  2026-11-11). See `docs/MIGRATION.md`."
- [x] 6.4 Create new ADR for v2.0 removal timeline: `pkg/hstong/*`
  deleted in v2.0 (targeted 2027), `pkg/services` import path unchanged,
  deprecation period = 1 month (2026-11-11).

## 7. Public GitHub issue templates

> Verification: `.github/ISSUE_TEMPLATE/bug_report.yml` and
> `feature_request.yml` exist and GitHub shows them when filing a new issue.

- [x] 7.1 Create `.github/ISSUE_TEMPLATE/bug_report.yml` with fields:
  Go version, SDK version, Gateway version, `HSTONG_INTEGRATION` status,
  minimal repro steps, expected vs actual.
- [x] 7.2 Create `.github/ISSUE_TEMPLATE/feature_request.yml` with fields:
  use case, alternatives considered, API sketch (optional).

## 8. Cosign verification documentation

> Verification: `docs/SIGNING.md` exists and the `cosign verify-blob`
> command it contains runs against a real published release.

- [x] 8.1 Create `docs/SIGNING.md` with: what the signature proves,
  download steps for checksums/bundle/archive, the `cosign verify-blob`
  command with the correct `--certificate-identity`, and the
  `sha256sum --check` command.
- [ ] 8.2 Verify the documented command against the latest release
  (v1.0.3 or later).

## 9. Gateway version compatibility matrix

> Verification: `docs/COMPAT.md` (or `docs/SPEC.md` §1) contains a table
> mapping SDK version to Gateway version.

- [x] 9.1 Create `docs/COMPAT.md` with the SDK/Gateway version matrix
  from design.md D9. Note that compatibility is confirmed by integration
  tests, not version matching, and that drift is detected by running the
  integration suite against a new Gateway version.
- [x] 9.2 Run `make docs-check` to confirm no link breaks.

## 10. Final review and CHANGELOG entry

> Verification: `make check` passes; CHANGELOG has a v1.1.0 (or next
> patch) entry for this work.

- [x] 10.1 Run `make check` (fmt + vet + tests + money-check) locally.
  (`make` is absent on the Windows host; the six component commands ran
  directly — `go build`, `gofmt -l` excluding `gen/`, `go vet ./...`,
  `scripts/check_money.py`, scripts unittest, `go test ./... -count=1` —
  all green on 2026-10-11.)
- [x] 10.2 Add a CHANGELOG entry for the production-readiness change:
  enumerate each task that produced a visible behavior change.
  (CHANGELOG `[Unreleased]` covers T6 deprecation, T7 issue templates,
  T8 SIGNING.md, T9 COMPAT.md, plus the MIGRATION/ADR index updates.)
- [x] 10.3 Open the master GitHub issue tracking this work, referencing
  this change's artifacts. (Master issue #16 with sub-issues #17–#26;
  artifacts referenced from the PR and from issue comments.)

---

## Workflow follow-up

- Archive the change after all P0 and P1 tasks are closed.
  Use `/opsx-archive` or `openspec archive "production-readiness"`.
- File the master GitHub issue and its sub-issues using the structure
  from the proposal, referencing each task's checklist box.
- Close the master issue when all sub-issues are resolved.
