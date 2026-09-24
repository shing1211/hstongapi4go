# Next-Phase Plan — hstongapi4go

- **Run:** `2026-09-23-hstong-enterprise-sdk`
- **Task:** E21
- **Status:** planning only — no code changed for this document.
- **Predecessor:** `plan.md`, `todos.md` (22 tasks), `ARCHITECTURE.md`,
  `evidence/P03-E17-threat-model.txt`, `evidence/P03-CI-repair.txt`

> Paths are inline code rather than relative links so `scripts/check_links.py`
> cannot be broken by this artifact.

## 1. What this run completed

Twenty of twenty-two tasks, then a CI repair that was not in the original plan.

**P00 Foundation & Domain.** ADRs 0008–0011 (decimal money, OpenTelemetry,
additive v-next layering, v0.1.x compatibility). The four v-next packages
(`pkg/domain`, `pkg/services`, `pkg/transport`, `internal/auth`) and a boundary
matrix. Decimal-backed `Money`/`Price`/`Quantity`/`Rate`, typed IDs, HK market
models, order-state transitions, and push-event types. An auth foundation with an
injectable clock, a session store, and the trade-password vector. The enterprise
toolchain.

**P01 REST Services.** A bounded HTTP adapter, nine market pull services, five
account services, thirteen trading services with HK validation, and mock-Gateway
integration suites.

**P02 Push Engine.** A push manager with reconnect and resubscribe, typed
normalizers with unknown-type tolerance, bounded fan-out with dedup and freshness,
and fuzz plus race tests.

**P03 DevOps & Hardening.** OTel behind a build tag, CI/CD with GoReleaser and
SBOM, a threat model with a 14-entry risk register, compatibility documentation,
a coverage gate, and the v0.1.6 release. Then E17, which found and fixed **seven**
security and resilience defects, and a CI repair that took the pipeline from
**red to 9/9 green** after finding it had been red since E15.

## 2. The structural finding that shapes everything below

The v-next layer exists, is tested, and **is not reachable by a caller**. Nothing
in the released API imports it:

```
$ grep -rl 'hstongapi4go/\(pkg/domain\|pkg/services\|pkg/transport\|internal/auth\)' \
    --include=*.go . | grep -v '^\./\(pkg/domain\|pkg/services\|pkg/transport\|internal/auth\)'
scripts/coverage_gate.go
```

The only importer is the coverage gate. `client` and `pkg/hstong/*` still use the
v0.1.x stack exclusively. So the SDK has two implementations, and only the older
one is in production. `docs/threat-model.md` records this explicitly and tags
every mitigation **Active** or **Forward-looking** for that reason.

This is the single most important thing for the next phase to resolve, in one
direction or the other. Everything else is smaller.

## 3. Gaps and deferred work

### G1 — The v-next layer is unwired (High)

`internal/auth`, `pkg/domain`, `pkg/services`, `pkg/transport.Adapter`,
`internal/push.Manager`, and `internal/push.Fanout` are all reachable only from
tests and the coverage gate. Six of the seven E17 fixes (F3, F4, F7, and the
`internal/auth` hardening) therefore protect nobody yet.

Two defensible outcomes: wire it in behind an opt-in constructor with the
released surface as the default, or delete it and keep the released stack. What
is not defensible is leaving two implementations indefinitely, because the next
maintainer cannot tell which one is authoritative.

### G2 — `internal/push` has three overlapping implementations (High)

`client.go` (released, used by `pkg/hstong/stream`), `manager.go` (E11, unused in
production), and `fanout.go` (E13, test-only) each implement connection
lifecycle, reconnect, and dispatch. They disagree: only `Client` is wired to
`pkg/hstong/stream`, only `Manager` has bounded retries and a retained dial
address, and only `Fanout` has backpressure and freshness. Fixing one leaves the
other two wrong.

### G3 — Push liveness is unaddressed (Medium, R3)

Neither the released `push.Client` nor `Manager` sets a read deadline, so a
silently dead TCP connection blocks a goroutine until the OS gives up. The
heartbeat is sent but a heartbeat write error does not trigger a reconnect.
This is the largest remaining availability gap on the push path.

### G4 — The Release workflow never publishes (Medium)

`.github/workflows/release.yml` runs `goreleaser check` on a tag. It does not run
`goreleaser release`, so tagging v0.1.6 produced a git tag and no artefacts. No
binary, checksum, or SBOM has ever been published. Switching the step is a
functional change that writes to a public repository and needs the credential
story settled: the config previously referenced a Gitee token that may not exist,
and cosign/provenance needs OIDC or key material this repository does not hold.

Note that GoReleaser builds only `cmd/hstong-mock-gateway`; the library itself
needs no binary, so a tag-only release is otherwise sufficient.

### G5 — Test and coverage debt (Medium)

- `pkg/services` has one test file, `account_test.go` (E17). The market, account,
  and trading services added in E07–E09 have no direct tests; they are covered
  only indirectly.
- CI has no bounded fuzz job. `FuzzReadFrame` exists but only its seed corpus
  runs under a normal `go test ./...`.
- CI has no job that runs tests under the `otel` tag. Only the release workflow
  builds with it.
- The 85% coverage gate covers `pkg/domain`, `internal/auth`, and
  `internal/transport` only.

### G6 — No live Gateway validation has ever run (Medium, carried from the
previous run)

`test/integration` is env-gated on `HSTONG_INTEGRATION=1` and has never been
executed, so the wire assumptions behind ADR 0007 — in particular whether market
`int64` fields arrive as JSON numbers or quoted strings — remain inferred from
the vendored Java and Python SDKs rather than observed. This needs a test
account and is the highest-value item that cannot be done offline.

### G7 — Open items in the security register (Medium)

| Ref | Item | Why it is still open |
|-----|------|----------------------|
| R2 | Route-alias classification is a closed set | A mutation route missing from `internal/resilience.mutationSet` would be treated as a retryable query. Needs a test that asserts the set covers every mutation route in `client/routes.go`. |
| R7 | No response-size cap in the released transport | `internal/transport` uses `io.ReadAll`; the cap exists only in the unused v-next adapter. |
| R11 | No secret scanning in CI | `gosec` and `govulncheck` run, but nothing scans commits for credentials. |
| R9 | `pkg/transport.Adapter` is mis-wired | `inner` is unused, the base URL is hardcoded, and `WithDeadline` stores one shared cancel that can cancel a previous caller. Moot if G1 resolves toward deletion. |
| R14 | `internal/auth` declares behaviour it lacks | Re-login backoff and single-flight login were documented but never implemented; the scaffolding was removed in the CI repair. |

### G8 — Documentation debt (Low)

- `docs/DESIGN.md` is still absent from the mkdocs nav.
- `mkdocs.yml` uses `enabled: !ENV [CI, false]` for the git-revision plugin, which
  does the opposite of what it looks like: `!ENV` substitutes the string `"true"`
  when `CI` is set, so the plugin stays enabled. The CI job now uses
  `fetch-depth: 0` and works, but the expression is misleading.
- Several files carry mojibake from an earlier encoding pass, including the
  `mkdocs.yml` footer and parts of `docs/LEGACY.md`.
- `proto/PROVENANCE.md` asserts the vendored tree is byte-for-byte upstream, while
  `.gitattributes` now normalises line endings to LF. The committed bytes did not
  change, but the document's claim is no longer literally true and should be
  reworded.
- The prior run's `next-phase.md` and `plan.md` contain the same mojibake.

### G9 — Local workflow friction (Low)

`go test -race` cannot run on this Windows host without a C toolchain, so race
coverage depends on CI. LF checkouts are now enforced by `.gitattributes`, which
means a fresh clone needs one `git add --renormalize` or re-checkout before the
working tree matches, and Windows editors must not silently re-introduce CRLF.

## 4. Candidate work

Seven candidates, ordered by the value they unlock rather than by size.

| # | Candidate | Addresses | Size | Risk |
|---|-----------|-----------|------|------|
| N1 | Decide and execute the v-next layer's fate: wire it in opt-in, or delete it | G1, G2, R9, R14 | L | High — touches the public surface |
| N2 | Consolidate push to one implementation with read deadlines and heartbeat liveness | G2, G3 | L | Medium — concurrency |
| N3 | Make tagging actually publish, with the credential story settled | G4 | M | Medium — writes to a public repo |
| N4 | Close the open security items: route-set coverage test, response-size cap, secret scanning in CI | G7 | M | Low |
| N5 | Test debt: `pkg/services` coverage, bounded fuzz job, an `otel`-tag CI job, widen the coverage gate | G5 | M | Low |
| N6 | Run the live Gateway integration suite against a real account | G6 | S to run, M to fix what it finds | Needs credentials |
| N7 | Documentation cleanup: DESIGN.md nav, the `!ENV` expression, mojibake, PROVENANCE wording | G8, G9 | S | None |

## 5. Recommended breakdown

**N7 first.** It is small, risk-free, and it removes known-wrong statements from
the documentation, which matters more once N1 starts changing the public surface
and people are reading the docs to understand it.

Then **N4**, because it is low-risk and two of its three items are security
postures that should not wait behind a large refactor.

Then **N1**, and it should begin with a written decision, not code. The decision
record needs to answer: does this SDK ship a layered API, or is the released flat
API the product? Everything else in the v-next layer is downstream of that answer.
A useful forcing function is to write the migration guide first — if it cannot be
written coherently, the layer should probably be deleted rather than exposed.

**N2** follows N1, because which push implementation survives depends on N1.

**N3** should wait until N1 settles, so the release publishes the final surface
rather than an intermediate one.

**N5** is best done continuously alongside the others rather than as a block.

**N6** needs a test account from the user and is the only item that can retire an
assumption rather than tighten a guarantee. It should be scheduled early if
credentials are available, because its findings may change the design of
everything else.

## 6. Carried-forward verification expectations

Whatever is chosen, the gate established by the CI repair should hold:

```
gofmt -l .                     now meaningful locally, LF is enforced
go build ./...                 and with -tags otel
go vet ./...                   and with -tags otel
go test ./... -count=1         and with -tags otel
golangci-lint run ./...        exit 0
gosec -exclude-generated ./... 0 issues, each suppression with a reason
govulncheck ./...              no vulnerabilities
goreleaser check               configuration valid
buf generate                   matches gen/ byte-for-byte
make money-check               no float money fields
check_links.py                 0 unresolved
mkdocs build --strict          exit 0
```

Details of how each was repaired, and the two real code defects found while
repairing them, are in `evidence/P03-CI-repair.txt`.
