# Default: build server + desktop.
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

# scripts/lint-step.sh needs POSIX sh (Git Bash on Windows; /bin/sh on Linux CI).
ifeq ($(OS),Windows_NT)
RUN_SH := C:/Program Files/Git/bin/sh.exe
else
RUN_SH := sh
endif

# Pinned in tools/go.mod; reinstall with `make tools-install` after Go upgrades.
GOLANGCI_LINT ?= golangci-lint
GOVULNCHECK ?= govulncheck
DEADCODE ?= deadcode

SERVER_BIN := $(BIN)/bizshuffle-server$(EXE)
DESKTOP_BIN := $(BIN)/bizshuffle-desktop$(EXE)
# go.work has no root module; quality targets use explicit package/module lists.
GO_PKGS := ./assets/... ./protocol/... ./domain/... ./savestate/... ./serverhost/... ./clienthost/... ./testing/... ./cmd/server/... ./cmd/desktop/...
GO_MOD_DIRS := assets protocol domain savestate serverhost clienthost testing cmd/server cmd/desktop

# Portable directory create (Windows make may not have mkdir in PATH).
ifeq ($(OS),Windows_NT)
ensure_coverage_dir = powershell -NoProfile -Command "New-Item -ItemType Directory -Force -Path '$(COVERAGE_DIR)' | Out-Null"
else
ensure_coverage_dir = mkdir -p $(COVERAGE_DIR)
endif

.PHONY: all test test-race lint lint-prereq lint-vet lint-timed lint-modules lint-one \
	lint-protocol lint-domain lint-savestate lint-serverhost lint-clienthost lint-testing \
	lint-cmd-server lint-cmd-desktop \
	vet fmt fix mod-tidy tools-install coverage coverage-html vuln deadcode check check-all \
	build-admin build-server build-desktop clean

# --- default build (run this with plain `make`) ---

all: $(SERVER_BIN) $(DESKTOP_BIN)
	@echo "All builds finished: $(SERVER_BIN) $(DESKTOP_BIN)"

# --- binaries ---

$(SERVER_BIN): build-admin
	@$(if $(filter Windows_NT,$(OS)),powershell -NoProfile -Command "New-Item -ItemType Directory -Force -Path '$(BIN)' | Out-Null",mkdir -p $(BIN))
	@echo building $@
	CGO_ENABLED=0 $(GO) build -o $@ ./cmd/server
	@echo done: $@

DESKTOP_VERSION ?= dev
DESKTOP_LDFLAGS := -X github.com/michael4d45/bizshuffle/cmd/desktop/updates.Version=$(DESKTOP_VERSION)
ifeq ($(or $(GOOS),$(shell go env GOOS)),windows)
DESKTOP_LDFLAGS += -H windowsgui
endif

$(DESKTOP_BIN): build-admin
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

# Per-module lint: make lint-protocol, lint-cmd-desktop, … (see lint-one).
.PHONY: lint-one lint-protocol lint-domain lint-savestate lint-assets lint-serverhost \
	lint-clienthost lint-testing lint-cmd-server lint-cmd-desktop

LINT_MOD_TARGETS := lint-protocol lint-domain lint-savestate lint-assets lint-serverhost \
	lint-clienthost lint-testing lint-cmd-server lint-cmd-desktop

lint-one:
	@"$(RUN_SH)" scripts/lint-step.sh "$(DIR)" "$(RUN_SH)" -c 'cd "$(DIR)" && export CGO_ENABLED=1 && exec $(GOLANGCI_LINT) run ./...'

lint-protocol:
	@$(MAKE) lint-one DIR=protocol
lint-domain:
	@$(MAKE) lint-one DIR=domain
lint-savestate:
	@$(MAKE) lint-one DIR=savestate
lint-assets:
	@$(MAKE) lint-one DIR=assets
lint-serverhost:
	@$(MAKE) lint-one DIR=serverhost
lint-clienthost:
	@$(MAKE) lint-one DIR=clienthost
lint-testing:
	@$(MAKE) lint-one DIR=testing
lint-cmd-server:
	@$(MAKE) lint-one DIR=cmd/server
lint-cmd-desktop:
	@$(MAKE) lint-one DIR=cmd/desktop

lint-prereq:
	@command -v $(GOLANGCI_LINT) >/dev/null 2>&1 || (echo "golangci-lint not found; run: make tools-install (installs to $(GOBIN))" && exit 1)
	@export CGO_ENABLED=1 && $(GOLANGCI_LINT) version

lint-vet:
	@"$(RUN_SH)" scripts/lint-step.sh vet $(GO) vet $(GO_PKGS)

lint-modules: lint-prereq $(LINT_MOD_TARGETS)

# Full lint with per-step timings (CI-safe; same checks as before).
lint: lint-prereq lint-vet $(LINT_MOD_TARGETS)

lint-timed: lint

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
