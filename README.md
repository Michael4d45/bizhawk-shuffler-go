# BizShuffle (Go)

Coordinated retro-gaming session host for BizHawk. Behavior spec: [docs/SPEC.md](docs/SPEC.md).

## Requirements

- Go 1.26+
- [Bun](https://bun.sh) 1.3+ (admin UI build only)
- CGO for `cmd/desktop` (Fyne)

## Setup

```bash
go work sync
make build-admin   # React admin → serverhost/static/
```

## Build binaries

```bash
make              # default: dist/bizshuffle-server + dist/bizshuffle-desktop
make build-desktop
make clean && make
```

On Windows without Make: `.\build.ps1` (same outputs under `dist\`).

Plain `make` used to only sync `server.lua` (first Makefile target). It now defaults to **full builds** via `.DEFAULT_GOAL := all`.

## Run

**Headless server** (defaults `--data-dir` to `%USERPROFILE%\BizShuffle`):

```bash
go run ./cmd/server -- --host 127.0.0.1 --port 8080
```

**Desktop (Host + Join):**

```bash
go run ./cmd/desktop
```

Data directory defaults to `%USERPROFILE%\BizShuffle\` (or `~/BizShuffle/`). **Host** starts the embedded server and opens the admin UI. **Join** installs BizHawk/VC++ via the dependencies panel when needed, then launches the emulator and connects to the server URL.

Shipped release binaries: `bizshuffle-server` (no CGO) and `bizshuffle-desktop` (Fyne/CGO). There is no separate player CLI binary.

## Test & lint

```bash
go work sync
make tools-install   # installs tools from tools/go.mod (go install tool; needs Go 1.24+)
make mod-tidy        # go mod tidy in tools + every workspace module (fixes direct/indirect warnings)
make fmt             # gofmt via `go fmt` (writes files)
make fix             # golangci-lint --fix per module (partial auto-fixes)
make test
make lint            # go vet + golangci-lint per module (bodyclose, errorlint, govet, …)
make coverage        # atomic profile + summary (see coverage/profile)
make vuln            # govulncheck per workspace module
make deadcode        # unreachable funcs from cmd/server and cmd/desktop mains
make check           # vet, lint, test, coverage, vuln, deadcode (stops on first failure)
make check-all       # same, but continues after failures (triage)
```

`go test ./...` does not work at the workspace root (no root module). Use `make test` or the explicit package list in the [Makefile](Makefile).

**golangci-lint and Go 1.26:** the linter binary must be built with Go ≥ your workspace version. After upgrading Go, run `make tools-install` again. Pinned versions live in [tools/go.mod](tools/go.mod).

**Windows:** GNU Make runs recipes in `/bin/sh`; dev tools are on `PATH` (not full `C:\...` paths). `make lint` runs golangci-lint **once per workspace module** (avoids `path_relativity` noise and matches `go.work`).

**IDE vs `make lint`:** VS Code runs `golangci-lint` on save (`go.lintTool`); gopls staticcheck is off so rules match [.golangci.yml](.golangci.yml) (e.g. `QF1012` on, `ST1000` package comments off). gopls may still show a few analysis hints (`writestring`, `scannererr`) not yet in golangci-lint.

## Layout

| Path | Role |
|------|------|
| `protocol/` | WS types, codec, KV, Lua, discovery |
| `domain/` | Pure session logic |
| `savestate/` | `.state` zip verification |
| `serverhost/` | HTTP/WS server + embedded admin |
| `clienthost/` | Player session library (BizHawk IPC, deps, Join) |
| `frontend/admin/` | React admin SPA |
| `cmd/server`, `cmd/desktop` | Shipped binaries |
| `testing/` | Arch + integration tests |

## Manual smoke (desktop)

1. Launch desktop — bind host, port, server URL, and player name should restore from `%USERPROFILE%\BizShuffle\config.json`.
2. **Host** with port `0` — admin opens; port field updates to the chosen port; **Stop host** ends the session.
3. **Join** while BizHawk/VC++ missing — Join disabled with deps message; install via panel (or **Install all**).
4. **Join** with valid server URL — staged status messages; session connects (or times out after 30s with a clear error).
5. Close BizHawk — status shows disconnect message.
6. **Refresh servers** / wait 5s — LAN list updates; hosted session marked `(hosting)`.
7. **Check updates** — version label; opens release download when a newer GitHub release exists.
