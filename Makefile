.DEFAULT_GOAL := help

override CACHE_DIR := $(CURDIR)/.cache
GO_BUILD_CACHE := $(CACHE_DIR)/go-build
GO_MOD_CACHE := $(CACHE_DIR)/go/pkg/mod
GO_TMP_DIR := $(CACHE_DIR)/go/tmp

GOENV := env GOCACHE=$(GO_BUILD_CACHE) GOMODCACHE=$(GO_MOD_CACHE) GOTMPDIR=$(GO_TMP_DIR)

GOPATH := $(shell go env GOPATH 2>/dev/null)
GH_BIN := $(shell command -v gh 2>/dev/null)
ifeq ($(strip $(GH_BIN)),)
GH_BIN := $(GOPATH)/bin/gh
endif

VERSION ?= $(strip $(shell sed -n 's/.*Version = "\([^"]*\)".*/\1/p' internal/buildinfo/buildinfo.go | head -n 1))
LDFLAGS := -ldflags "-X github.com/crevissepartners/wt/internal/buildinfo.Version=$(VERSION)"

.PHONY: help
help: ## Show help
	@awk 'BEGIN {FS = ":.*##"; print "Targets:"} /^[a-zA-Z0-9_.-]+:.*##/ {printf "  %-14s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

.PHONY: init
init: ## Create local cache dirs
	@mkdir -p $(GO_BUILD_CACHE) $(GO_MOD_CACHE) $(GO_TMP_DIR)

.PHONY: fmt
fmt: ## Run gofmt (writes files)
	@files="$$(find . -name '*.go' -not -path './.cache/*' -not -path './.wt/*')"; \
	if [ -n "$$files" ]; then $(GOENV) gofmt -w $$files; fi

.PHONY: fmt-check
fmt-check: ## Verify gofmt is clean
	@files="$$(find . -name '*.go' -not -path './.cache/*' -not -path './.wt/*')"; \
	if [ -z "$$files" ]; then exit 0; fi; \
	out="$$(gofmt -l $$files)"; \
	if [ -n "$$out" ]; then echo "gofmt needed:"; echo "$$out"; exit 1; fi

.PHONY: fix
fix: init ## Run go fix (writes files)
	@$(GOENV) go fix ./...

.PHONY: fix-diff
fix-diff: init ## Show go fix patch (no writes)
	@$(GOENV) go fix -diff ./...

.PHONY: fix-check
fix-check: init ## Verify go fix is clean
	@out="$$( $(GOENV) go fix -diff ./... )"; if [ -n "$$out" ]; then echo "$$out"; echo; echo "go fix needed: run 'make fix'"; exit 1; fi

.PHONY: check
check: init fmt-check fix-check ## Run required checks

.PHONY: test
test: check ## Run tests (requires fmt/fix clean)
	@$(GOENV) go test ./...

.PHONY: build
build: check ## Build (requires fmt/fix clean)
	@$(GOENV) go build $(LDFLAGS) ./cmd/wt

.PHONY: run
run: init ## Run (ARGS="--help")
	@$(GOENV) go run $(LDFLAGS) ./cmd/wt $(ARGS)

.PHONY: clean
clean: ## Remove local caches
	@case "$(CACHE_DIR)" in \
		"$(CURDIR)/.cache") ;; \
		*) echo "refusing to clean unexpected CACHE_DIR=$(CACHE_DIR)"; exit 1 ;; \
	esac
	@rm -rf -- "$(CACHE_DIR)"

.PHONY: premerge
premerge: check test ## Pre-merge gate (release-please config + tests)
	@./scripts/premerge_verify.sh

.PHONY: pr-create
pr-create: premerge ## Create GitHub PR via gh (requires gh auth login)
	@[ -x "$(GH_BIN)" ] || { echo "gh not found. Install: 'go install github.com/cli/cli/v2/cmd/gh@latest'"; exit 1; }
	@$(GH_BIN) auth status >/dev/null 2>&1 || { echo "gh not authenticated. Run: '$(GH_BIN) auth login'"; exit 1; }
	@$(GH_BIN) pr create --fill

.PHONY: completion-zsh
completion-zsh: init ## Generate zsh completion file to dist/completions/_wt (no install)
	@mkdir -p dist/completions
	@$(GOENV) go run $(LDFLAGS) ./cmd/wt completion zsh > dist/completions/_wt
	@echo "generated: dist/completions/_wt"
