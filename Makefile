# Copyright 2026 shing1211
# SPDX-License-Identifier: Apache-2.0
#
# hstongapi4go — developer tasks.
#
# CI runs on Linux with bash, so SHELL is pinned to /bin/bash and the snippets
# below are written for that shell. Windows developers should run these targets
# from Git Bash or WSL (GNU make + bash are required; cmd.exe/PowerShell are not
# supported for `make`). The Go tooling itself works natively on Windows:
#
#   git bash:  make help
#   wsl:       make help
#
# Every target below is live. Historical no-op stubs for proto codegen, the docs
# site, and the mock Gateway were removed once those landed; a target that
# cannot run says so in its own recipe rather than pretending to succeed.
#
# Targets also guard on go.mod so the file stays readable before the module
# exists.

SHELL := /bin/bash
GO ?= go
# Prefer python3, fall back to python: Git Bash and WSL images do not always
# provide python3, while Windows Python installs usually only provide `python`.
# An explicit `make PYTHON=...` still wins.
PYTHON ?= $(shell command -v python3 2>/dev/null || command -v python 2>/dev/null || echo python3)
LICENSE_HOLDER ?= shing1211
LICENSE_YEAR ?= 2026
ADDLICENSE_VERSION ?= latest
OAPI_CODEGEN_VERSION ?= v2.8.0
BUF_VERSION ?= v1.47.2
PROTOC_GEN_GO_VERSION ?= v1.36.6

.DEFAULT_GOAL := help

.PHONY: help tools build fmt fmt-check vet test test-race test-integration coverage check \
        money-check scripts-test proto proto-verify docs-check license license-check \
        mock-gateway clean lint gosec govulncheck enterprise-check goreleaser-check sbom

help: ## List targets
	@grep -E '^[a-zA-Z0-9_-]+:.*?## .*$$' $(MAKEFILE_LIST) \
		| awk 'BEGIN{FS=":.*?## "}{printf "  \033[36m%-18s\033[0m %s\n", $$1, $$2}'

tools: ## Install dev tools (addlicense; codegen tools guarded per-phase)
	$(GO) install github.com/google/addlicense@$(ADDLICENSE_VERSION)
	@# Reserved for later phases — installed only once their config exists.
	@if [ -f buf.yaml ]; then $(GO) install github.com/bufbuild/buf/cmd/buf@$(BUF_VERSION); fi
	@if [ -f buf.gen.yaml ]; then $(GO) install google.golang.org/protobuf/cmd/protoc-gen-go@$(PROTOC_GEN_GO_VERSION); fi
	@if [ -f oapi-codegen.yaml ]; then $(GO) install github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@$(OAPI_CODEGEN_VERSION); fi

build: ## Compile all packages
	@if [ -f go.mod ]; then $(GO) build ./...; else echo "no go.mod yet; skipping build"; fi

fmt: ## Format Go sources
	@if [ -f go.mod ]; then gofmt -s -w .; else echo "no go.mod yet; skipping fmt"; fi

# Reports formatting exactly as the Linux CI runner sees it. `.gitattributes`
# forces LF checkouts on every platform, so a plain `gofmt -l` is authoritative
# and no line-ending normalisation is needed first. Generated code under gen/ is
# excluded, as in .golangci.yml and in the CI gofmt step.
fmt-check: ## Check gofmt formatting the way CI does (gen/ excluded)
	@if [ -f go.mod ]; then \
		unformatted=$$(gofmt -l . | grep -v '^gen/' || true); \
		if [ -n "$$unformatted" ]; then \
			echo "The following files are not gofmt-clean:"; echo "$$unformatted"; exit 1; \
		fi; \
		echo "fmt-check OK: no unformatted files outside gen/"; \
	else echo "no go.mod yet; skipping fmt-check"; fi

vet: ## Run go vet
	@if [ -f go.mod ]; then $(GO) vet ./...; else echo "no go.mod yet; skipping vet"; fi

test: ## Run unit tests
	@if [ -f go.mod ]; then $(GO) test ./... -count=1; else echo "no go.mod yet; skipping tests"; fi

test-race: ## Run unit tests with the race detector
	@if [ -f go.mod ]; then $(GO) test ./... -race -count=1; else echo "no go.mod yet; skipping"; fi

test-integration: ## Run env-gated integration tests (HSTONG_INTEGRATION=1 required)
	@if [ -d test/integration ]; then \
		$(GO) test ./test/integration/... -count=1 -v; \
	else echo "test-integration: not yet available"; fi

coverage: ## Run tests with coverage gate (>=85% on pkg/domain, internal/auth, internal/transport, internal/push, pkg/hstong{,/stream,/trade,/algo}, pkg/types, pkg/transport)
	$(GO) run scripts/coverage_gate.go

check: fmt vet money-check scripts-test test ## Format, vet, check money types, test scripts/ and test

money-check: ## Fail if pkg/ exposes float money/quantity fields (docs/DESIGN.md §7)
	$(PYTHON) scripts/check_money.py

# The Python guards under scripts/ are covered by stdlib unittest, discovered by
# the test_*.py naming convention: no third-party runner, since AGENTS.md rule 8
# admits no new dependency without an ADR. Discovery is rooted at scripts/ so the
# tests exercise check_money.py against throwaway temp trees and never plant a
# float money field in the repository. Run from the repo root.
scripts-test: ## Run the scripts/ Python unit tests (stdlib unittest)
	$(PYTHON) -m unittest discover -s scripts -p "test_*.py" -v

proto: ## Regenerate proto/ into gen/ (protobuf codegen)
	@if [ -f buf.gen.yaml ]; then \
		buf generate && echo "proto: regenerated gen/ from proto/"; \
	else echo "proto: buf.gen.yaml missing; run from the repo root"; fi
	@# Lint is informational only: the protos are byte-for-byte upstream and
	@# have no package declaration, so style findings are not actionable here.
	@if [ -f buf.yaml ]; then buf lint || true; fi

proto-verify: ## Fail if generated protobuf code drifts from proto/
	@if [ -f buf.gen.yaml ]; then \
		bash scripts/proto_verify.sh; \
	else echo "proto-verify: buf.gen.yaml missing; run from the repo root"; fi

docs-check: ## Check markdown links, README translations, and build the docs site (strict)
	$(PYTHON) scripts/check_links.py
	$(PYTHON) scripts/check_i18n.py
	mkdocs build --strict

license: ## Apply SPDX headers to sources
	@if [ -f go.mod ]; then \
		dirs="$$(for d in client pkg internal cmd scripts; do [ -d "$$d" ] && echo "$$d"; done)"; \
		if [ -z "$$dirs" ]; then echo "no source dirs yet; skipping license"; else \
			$(GO) run github.com/google/addlicense@$(ADDLICENSE_VERSION) \
				-c "$(LICENSE_HOLDER)" -y "$(LICENSE_YEAR)" -l apache -s=only $$dirs; \
		fi; \
	else echo "no go.mod yet; run scripts manually"; fi

license-check: ## Verify SPDX headers are present
	@if [ -f go.mod ]; then \
		dirs="$$(for d in client pkg internal cmd scripts; do [ -d "$$d" ] && echo "$$d"; done)"; \
		if [ -z "$$dirs" ]; then echo "no source dirs yet; skipping license-check"; else \
			$(GO) run github.com/google/addlicense@$(ADDLICENSE_VERSION) -check $$dirs; \
		fi; \
	else echo "no go.mod yet; skipping"; fi

mock-gateway: ## Run the standalone mock HStong gateway
	$(GO) run ./cmd/hstong-mock-gateway "$@"

clean: ## Remove build artifacts
	rm -f coverage.out coverage.html
	rm -rf "$(CURDIR)/dist"

# Enterprise toolchain targets (E05)

LINTER := golangci-lint
GOSEC := gosec
GOVULN := govulncheck
GORELEASER := goreleaser

.PHONY: lint
lint: ## Run golangci-lint
	$(LINTER) run ./...

.PHONY: gosec
gosec: ## Run gosec security scanner
	$(GOSEC) -exclude-generated ./...

.PHONY: govulncheck
govulncheck: ## Run govulncheck vulnerability scanner
	$(GOVULN) ./...

.PHONY: enterprise-check
enterprise-check: ## Run full enterprise pre-flight (lint + security + coverage)
	make lint
	make gosec
	make govulncheck
	make coverage

.PHONY: goreleaser-check
goreleaser-check: ## Verify GoReleaser configuration
	@if [ ! -f .goreleaser.yaml ]; then echo "goreleaser-check: .goreleaser.yaml missing; skipping"; exit 0; fi; \
	if ! command -v $(GORELEASER) >/dev/null 2>&1; then \
		echo "goreleaser-check: '$(GORELEASER)' is not on PATH."; \
		echo "  Install the published binary (https://goreleaser.com/install), or rely on CI,"; \
		echo "  whose 'goreleaser check' job uses goreleaser/goreleaser-action@v6."; \
		echo "  'go install github.com/goreleaser/goreleaser@latest' is deliberately not used here:"; \
		echo "  it resolves a v1 dependency set that fails to compile under Go 1.26."; \
		exit 1; \
	fi; \
	$(GORELEASER) check --config .goreleaser.yaml

.PHONY: sbom
sbom: ## Generate SPDX SBOM for the module
	@if [ -f scripts/gen_sbom.py ]; then \
		python scripts/gen_sbom.py hstongapi4go_sbom.json; \
	else echo "sbom: gen_sbom.py missing; skipping"; fi
