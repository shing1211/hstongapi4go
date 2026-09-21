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
# Tooling that does not exist yet (proto codegen, docs site, mock gateway) is
# represented by no-op targets that print a "not yet available (Pnn)" notice and
# exit 0, so CI stays green until the owning phase lands. Targets also guard on
# go.mod/directories so the empty skeleton builds and tests cleanly.

SHELL := /bin/bash
GO ?= go
PYTHON ?= python3
LICENSE_HOLDER ?= shing1211
LICENSE_YEAR ?= 2026
ADDLICENSE_VERSION ?= latest
OAPI_CODEGEN_VERSION ?= v2.8.0
BUF_VERSION ?= v1.47.2
PROTOC_GEN_GO_VERSION ?= v1.36.6

.DEFAULT_GOAL := help

.PHONY: help tools build fmt vet test test-race test-integration coverage check \
        money-check proto proto-verify docs-check license license-check \
        mock-gateway clean

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

coverage: ## Write coverage.out and coverage.html
	@if [ -f go.mod ]; then \
		$(GO) test ./... -coverprofile=coverage.out -count=1 && \
		$(GO) tool cover -html=coverage.out -o coverage.html && \
		echo "wrote coverage.out and coverage.html"; \
	else echo "no go.mod yet; skipping"; fi

check: fmt vet money-check test ## Format, vet, check money types, and test

money-check: ## Fail if pkg/ exposes float money/quantity fields (docs/DESIGN.md §7)
	$(PYTHON) scripts/check_money.py

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
