# Release Plan — hstongapi4go v0.1.0 (run `2026-09-21-hstong-full-surface`)

- **Mode:** BUILD
- **Repo:** `github.com/shing1211/hstongapi4go`
- **Main branch:** `main`
- **Base commit:** `461f219` (Initial commit)
- **Status:** PLAN — **awaiting explicit human approval before any commit or push**

## Objective

Stage, commit, and push the completed run to **both** remotes' `main` branches.
No PR, no branch, no merge request, no force-push, no rebase.

## Pre-flight (must all pass first)

| # | Check | Command | Result |
|---|-------|---------|--------|
| 1 | Build | `go build ./...` | pass |
| 2 | Vet | `go vet ./...` | pass |
| 3 | Format | `gofmt -l .` | empty |
| 4 | Unit + race | `go test -race -count=1 ./...` | 19 packages ok (3 consecutive runs) |
| 5 | Money rule | `python scripts/check_money.py` | pass |
| 6 | Proto drift | `bash scripts/proto_verify.sh` | no drift |
| 7 | Docs links | `python scripts/check_links.py` | 61 files, 0 unresolved |
| 8 | i18n lockstep | `python scripts/check_i18n.py` | 6 languages consistent |
| 9 | Docs site | `mkdocs build --strict` | exit 0 |
| 10 | Examples | `go build ./examples/...` | pass |
| 11 | Mock Gateway | `go build ./cmd/...` | pass |
| 12 | Working tree | `git status --short` | only intended files (see below) |

### Known pre-flight gaps (accepted, disclosed)

- **`make` was never executed on this host** (no GNU make; WSL stub broken). Every
  Makefile recipe's underlying command was run directly; CI (ubuntu-latest) runs the
  recipes. Not a blocker, but a first CI run is advisable.
- **Live integration tests were never executed** (no Gateway/credentials in this
  session). `test/integration` skips by default. The wire assumptions listed in
  `phases/P12-integration.md` remain unconfirmed.

## Remote setup

`origin` (GitHub) exists. **`gitee` does not yet exist locally** — add it:

```sh
git remote add gitee https://gitee.com/shing1211/hstongapi4go.git
git remote -v   # verify both
```

## Files to stage

Everything except the ignored host artifacts. Explicitly **never staged**:
- `华盛通OpenAPI-*.zip`, `华盛通OpenAPI-Gateway客户端-*` (ignored vendor archives)
- `site/`, `coverage.out`, `coverage.html`, `.venv/`, `__pycache__/`
- any `*.exe`, `*.test`

Stage with an explicit allow-list (not `git add -A`) so nothing unexpected slips in:

```sh
git add .gitattributes .gitignore .editorconfig .golangci.yml .github \
        AGENTS.md CHANGELOG.md CONTRIBUTING.md DISCLAIMER.md LICENSE \
        NOTICE README.md README.zh-Hans.md README.zh-Hant.md README.ja.md \
        README.ko.md README.es.md SECURITY.md THIRD_PARTY_NOTICES.md \
        TRANSLATING.md Makefile mkdocs.yml requirements-docs.txt go.mod go.sum doc.go \
        buf.yaml buf.gen.yaml proto gen client cmd internal pkg \
        examples scripts test docs
git status --short   # review; nothing outside the list
```

## Commit message (Conventional Commits, DCO-signed)

```
feat: full-surface HStong Quant OpenAPI Gateway SDK (v0.1.0)

Implement the complete Go SDK for the HStong (华盛) Quant OpenAPI local
Gateway: 51 HTTP endpoints across market data, trading, futures, and algo,
plus 11 market push topics and trade/futures order-status push.

Highlights:
- proto-driven types for push payloads and market DTOs (PB v2.2.0, 17 protos)
  with a hybrid codec: encoding/json for HTTP bodies (ADR 0007), binary
  protobuf for TCP push
- 151-byte push framing, PBNotify decode by notifyMsgType, reconnect with
  automatic resubscription
- trade-password AES-192-ECB/PKCS7 crypto verified against the documented
  vector
- typed status-code taxonomy (0000, 1001-1018, 20033, 40001/40002) with
  Retryable/ReLoginRequired, and a mutation set that is never retried (ADR 0003)
- opt-in resilience (rate limit, retry, breaker), slog logging with secret
  redaction, dependency-free metrics, and opt-in SHA1WithRSA push verification
- in-repo mock Gateway (51 routes + TCP push) and an all-endpoint e2e suite
- six-language READMEs, MkDocs Material docs site, ADRs 0001-0007

Verification: go build/vet/gofmt clean; go test -race -count=1 ./... green
(19 packages); mock Gateway e2e 51/51 endpoints; proto-verify no drift;
docs-check and mkdocs build --strict pass.

Run: docs/runs/2026-09-21-hstong-full-surface
Tasks: T01-T40

Signed-off-by: shing1211 <shing1211@users.noreply.github.com>
```

## Push sequence

```sh
git commit -s -F <message file>          # or -m with the block above
git push origin main
git push gitee  main
```

Then verify both remotes report the new commit:

```sh
git ls-remote origin refs/heads/main
git ls-remote gitee  refs/heads/main
git rev-parse HEAD
```

## Post-push

1. Create the annotated tag **only if** the human approves shipping before the live
   wire validation (see `next-phase.md`):
   ```sh
   git tag -a v0.1.0 -m "v0.1.0"
   git push origin v0.1.0
   git push gitee  v0.1.0
   ```
   Otherwise keep `[Unreleased]` semantics and do not tag.
2. Update `docs/runs/index.md` with the feature and close-out commit hashes.
3. Record the commit hashes in `report.md`.

## Safety rules

- No `--force`, no `-f`, no rebase of `main`, no history rewrite.
- Stop and report on any rejection (non-fast-forward, auth failure, branch
  protection, Gitee mirror delay).
- If `origin` and `gitee` diverge, stop; do not resolve by force.

## Open decision (human)

- **Ship v0.1.0 with the live wire validation unrun, or run
  `test/integration` first and release after?** `next-phase.md` recommends
  validating the wire before tagging; the commit/push itself is safe either way.
