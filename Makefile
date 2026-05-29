# Default: build server + desktop (not just sync-server-lua).
.DEFAULT_GOAL := all

GO := go
BIN := dist
COVERAGE_DIR := coverage
COVER_PROFILE := $(COVERAGE_DIR)/profile
ADMIN := frontend/admin
EXE := $(shell go env GOEXE)
# Make on Windows runs recipes under /bin/sh; never embed raw GOBIN paths in recipes.
# Forward slashes + PATH prepend lets sh find golangci-lint.exe without escape issues.
GOBIN := $(subst \,/,$(shell $(GO) env GOBIN))
ifeq ($(GOBIN),)
GOBIN := $(subst \,/,$(shell $(GO) env GOPATH))/bin
endif
export PATH := $(GOBIN):$(PATH)

# Pinned in tools/go.mod; reinstall with `make tools-install` after Go upgrades.
GOLANGCI_LINT ?= golangci-lint
GOVULNCHECK ?= govulncheck
DEADCODE ?= deadcode

SERVER_BIN := $(BIN)/bizshuffle-server$(EXE)
DESKTOP_BIN := $(BIN)/bizshuffle-desktop$(EXE)
CLIENTHOST_ASSETS := clienthost/assets/server.lua

# go.work has no root module; quality targets use explicit package/module lists.
GO_PKGS := ./protocol/... ./domain/... ./savestate/... ./serverhost/... ./clienthost/... ./testing/... ./cmd/server/... ./cmd/desktop/...
GO_MOD_DIRS := protocol domain savestate serverhost clienthost testing cmd/server cmd/desktop

# Portable directory create (Windows make may not have mkdir in PATH).
ifeq ($(OS),Windows_NT)
ensure_coverage_dir = powershell -NoProfile -Command "New-Item -ItemType Directory -Force -Path '$(COVERAGE_DIR)' | Out-Null"
else
ensure_coverage_dir = mkdir -p $(COVERAGE_DIR)
endif

.PHONY: all test test-race lint vet fmt fix mod-tidy tools-install coverage coverage-html vuln deadcode check check-all \
	build-admin build-server build-desktop sync-server-lua clean

# --- default build (run this with plain `make`) ---

all: $(SERVER_BIN) $(DESKTOP_BIN)
	@echo "All builds finished: $(SERVER_BIN) $(DESKTOP_BIN)"

# --- binaries ---

$(SERVER_BIN): build-admin sync-server-lua
	@$(if $(filter Windows_NT,$(OS)),powershell -NoProfile -Command "New-Item -ItemType Directory -Force -Path '$(BIN)' | Out-Null",mkdir -p $(BIN))
	@echo building $@
	CGO_ENABLED=0 $(GO) build -o $@ ./cmd/server
	@echo done: $@

DESKTOP_VERSION ?= dev
DESKTOP_LDFLAGS := -X github.com/michael4d45/bizshuffle/cmd/desktop/updates.Version=$(DESKTOP_VERSION)
ifeq ($(or $(GOOS),$(shell go env GOOS)),windows)
DESKTOP_LDFLAGS += -H windowsgui
endif

$(DESKTOP_BIN): build-admin sync-server-lua
	@$(if $(filter Windows_NT,$(OS)),powershell -NoProfile -Command "New-Item -ItemType Directory -Force -Path '$(BIN)' | Out-Null",mkdir -p $(BIN))
	@echo building $@
	CGO_ENABLED=1 $(GO) build -ldflags "$(DESKTOP_LDFLAGS)" -o $@ ./cmd/desktop
	@echo done: $@

# Aliases (always rebuild when invoked by name)
build-server: $(SERVER_BIN)
build-desktop: $(DESKTOP_BIN)

# --- admin + lua assets ---

build-admin:
	cd $(ADMIN) && bun install && bun run build

sync-server-lua: $(CLIENTHOST_ASSETS)
	@echo synced server.lua -\> $(CLIENTHOST_ASSETS)

$(CLIENTHOST_ASSETS): assets/server.lua
	@$(if $(filter Windows_NT,$(OS)),powershell -NoProfile -Command "New-Item -ItemType Directory -Force -Path 'clienthost/assets' | Out-Null",mkdir -p clienthost/assets)
	cp assets/server.lua $(CLIENTHOST_ASSETS)

# --- quality (run from repo root; requires CGO for clienthost/desktop tests) ---

# Install tools pinned in tools/go.mod (tool block). Run tidy so gopls/IDE match module graph.
tools-install:
	cd tools && $(GO) mod tidy
	cd tools && $(GO) install tool
	@echo "Installed tools to $(GOBIN). Also: go -C tools tool golangci-lint | govulncheck | deadcode"

test:
	CGO_ENABLED=1 $(GO) test $(GO_PKGS)

test-race:
	CGO_ENABLED=1 $(GO) test -race $(GO_PKGS)

vet:
	$(GO) vet $(GO_PKGS)

# Format all Go sources (gofmt via `go fmt`; writes files).
fmt:
	@set -e; for dir in $(GO_MOD_DIRS); do \
		echo "==> $$dir"; \
		(cd $$dir && $(GO) fmt ./...); \
	done

# Automated fixes from golangci-lint (partial; not all linters support --fix).
fix:
	@command -v $(GOLANGCI_LINT) >/dev/null 2>&1 || (echo "golangci-lint not found; run: make tools-install" && exit 1)
	@set -e; for dir in $(GO_MOD_DIRS); do \
		echo "==> $$dir"; \
		(cd $$dir && CGO_ENABLED=1 $(GOLANGCI_LINT) run --fix ./...); \
	done

# Keep go.mod graphs clean (direct vs indirect, tool block) — same checks gopls shows on go.mod.
mod-tidy:
	@set -e; for dir in tools $(GO_MOD_DIRS); do \
		echo "==> $$dir"; \
		(cd $$dir && $(GO) mod tidy); \
	done

lint: vet
	@command -v $(GOLANGCI_LINT) >/dev/null 2>&1 || (echo "golangci-lint not found; run: make tools-install (installs to $(GOBIN))" && exit 1)
	CGO_ENABLED=1 $(GOLANGCI_LINT) version
	@set -e; for dir in $(GO_MOD_DIRS); do \
		echo "==> $$dir"; \
		(cd $$dir && CGO_ENABLED=1 $(GOLANGCI_LINT) run ./...); \
	done

coverage:
	@$(ensure_coverage_dir)
	CGO_ENABLED=1 $(GO) test -covermode=atomic -coverprofile=$(COVER_PROFILE) $(GO_PKGS)
	$(GO) tool cover -func=$(COVER_PROFILE)

coverage-html: coverage
	$(GO) tool cover -html=$(COVER_PROFILE) -o $(COVERAGE_DIR)/index.html
	@echo "Wrote $(COVERAGE_DIR)/index.html"

vuln:
	@command -v $(GOVULNCHECK) >/dev/null 2>&1 || (echo "govulncheck not found; run: make tools-install" && exit 1)
	@echo "==> protocol" && $(GOVULNCHECK) -C protocol ./...
	@echo "==> domain" && $(GOVULNCHECK) -C domain ./...
	@echo "==> savestate" && $(GOVULNCHECK) -C savestate ./...
	@echo "==> serverhost" && $(GOVULNCHECK) -C serverhost ./...
	@echo "==> clienthost" && $(GOVULNCHECK) -C clienthost ./...
	@echo "==> testing" && $(GOVULNCHECK) -C testing ./...
	@echo "==> cmd/server" && $(GOVULNCHECK) -C cmd/server ./...
	@echo "==> cmd/desktop" && $(GOVULNCHECK) -C cmd/desktop ./...

# deadcode only traces from main packages; -test includes test-only reachability.
deadcode:
	@command -v $(DEADCODE) >/dev/null 2>&1 || (echo "deadcode not found; run: make tools-install" && exit 1)
	@echo "==> cmd/server" && cd cmd/server && $(DEADCODE) -test ./...
	@echo "==> cmd/desktop" && cd cmd/desktop && $(DEADCODE) -test ./...

# Fast local gate (stops on first failure).
check: mod-tidy vet lint test coverage vuln deadcode

# Run every check even when one fails (for triage).
check-all:
	-$(MAKE) mod-tidy
	-$(MAKE) vet
	-$(MAKE) lint
	-$(MAKE) test
	-$(MAKE) coverage
	-$(MAKE) vuln
	-$(MAKE) deadcode

clean:
	rm -rf $(BIN) $(COVERAGE_DIR)

# Backward-compatible alias.
lint-install: tools-install
