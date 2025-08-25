# BizHawk Shuffler Go

BizHawk Shuffler is a Go-based client/server application that coordinates multiple BizHawk emulator clients for synchronized game swapping. The server provides an HTMX-powered web UI for administration and uses websockets for real-time client communication.

Always reference these instructions first and fallback to search or bash commands only when you encounter unexpected information that does not match the info here.

## Working Effectively

### Bootstrap and Build
- **Install Go 1.21+**: The project requires Go 1.21 minimum (works with Go 1.24+)
- **Build all components**: `make all` -- takes 1-20 seconds depending on dependency cache. NEVER CANCEL. Set timeout to 60+ seconds for first build.
- **Build server only**: `make server` -- takes <1 second incremental. NEVER CANCEL. Set timeout to 30+ seconds.
- **Build client only**: `make client` -- takes <1 second incremental. NEVER CANCEL. Set timeout to 30+ seconds.
- **Alternative build method (README style)**: 
  ```bash
  mkdir -p bin/server bin/client
  cd cmd/server && go build -o ../../bin/server/bizshuffle-server
  cd cmd/client && go build -o ../../bin/client/bizshuffle-client
  ```
- **Clean build artifacts**: `make clean`

### Dependencies and Modules
- **Download dependencies**: Dependencies download automatically during first build
- **Verify modules**: `go mod verify` -- takes <1 second. Set timeout to 10+ seconds.
- **Update dependencies**: `go mod tidy` -- takes <5 seconds. Set timeout to 30+ seconds.

### Run the Applications
- **ALWAYS build first** using the bootstrap steps above
- **Run server**: `./bin/server/bizshuffle-server --host 127.0.0.1 --port 8080`
  - Server serves admin web UI at http://127.0.0.1:8080/
  - API endpoints available at `/api/*` and `/state.json`
  - File serving from `./files/` directory at `/files/*`
- **Run client**: `./bin/client/bizshuffle-client`
  - **First run**: Will prompt for server websocket URL (e.g., `ws://127.0.0.1:8080/ws`) and username
  - **Subsequent runs**: Reads configuration from `client_config.json`
  - **BizHawk dependency**: Client attempts to download BizHawk emulator on first run (Windows .exe)

### Validation and Testing
- **ALWAYS run validation** after making changes:
  - `go vet ./...` -- takes ~2 seconds. NEVER CANCEL. Set timeout to 30+ seconds.
  - `gofmt -d .` -- takes <1 second. Set timeout to 10+ seconds.
- **Manual functional testing**:
  - Start server and access web UI at http://127.0.0.1:8080/
  - Test API endpoint: `curl http://127.0.0.1:8080/api/games` should return `{"games":[],"main_games":[]}`
  - Test state endpoint: `curl http://127.0.0.1:8080/state.json` should return server state
  - Create test file: `mkdir -p files && echo "test" > files/test.txt`
  - Test file serving: `curl http://127.0.0.1:8080/files/test.txt` should return file content

## Known Limitations and Workarounds

### Linting Issues
- **golangci-lint incompatibility**: golangci-lint v1.59.0 fails with Go 1.24+ (exit code 3)
- **Workaround**: Use `go vet ./...` and `gofmt -d .` for validation instead
- **GitHub Actions**: The lint workflow (.github/workflows/lint.yml) uses golangci-lint v1.59.0 with Go 1.21

### Client Platform Limitations
- **Windows-focused**: Client downloads Windows BizHawk executable (EmuHawk.exe)
- **Linux testing limitation**: Client setup will fail on Linux when downloading BizHawk, but configuration flow can be tested
- **Workaround**: For testing client configuration on Linux, expect "zip: not a valid zip file" error after configuration setup

### Build Environment
- **No test files**: Repository contains no test files - validation is functional only
- **PowerShell script**: build.ps1 exists for Windows builds but Makefile works on all platforms

## Validation Scenarios

After making changes, ALWAYS run through these complete scenarios:

### Server Validation
1. **Build and run server**: `make server && ./bin/server/bizshuffle-server --port 8080`
2. **Test web UI**: Navigate to http://127.0.0.1:8080/ and verify admin interface loads
3. **Test API**: `curl http://127.0.0.1:8080/api/games` returns valid JSON
4. **Test state persistence**: Verify `state.json` file is created/updated
5. **Test file serving**: Create test file and verify HTTP access

### Client Validation  
1. **Build client**: `make client`
2. **Test configuration flow**: Run `./bin/client/bizshuffle-client` and verify prompts appear
3. **Expected behavior**: Client will attempt BizHawk download (may fail on non-Windows)

### Full Integration Validation
1. **Start server** with file serving enabled
2. **Create test ROM files** in `./files/` directory
3. **Access admin UI** and verify file list updates
4. **Test websocket connectivity** via browser dev tools

## Common Tasks

### Repository Structure
```
.
├── README.md
├── Makefile
├── build.ps1              # Windows build script
├── go.mod, go.sum
├── cmd/
│   ├── server/main.go     # Server executable  
│   └── client/main.go     # Client executable
├── internal/
│   ├── server/            # Server HTTP/WS handlers
│   ├── client/            # Client logic and BizHawk integration
│   └── types/             # Shared types and message structures
├── web/
│   └── index.html         # Admin UI (HTMX + Alpine.js + Tailwind)
├── server.lua             # BizHawk Lua script for client
├── files/                 # ROM/asset storage (created as needed)
├── saves/                 # Save states directory
└── state.json             # Server state persistence (auto-created)
```

### Key Commands Reference
- **Build everything**: `make all` (1-20s, timeout 60s)
- **Lint code**: `go vet ./... && gofmt -d .` (2-3s, timeout 30s)  
- **Run server**: `./bin/server/bizshuffle-server --port 8080`
- **Test APIs**: `curl http://127.0.0.1:8080/state.json`
- **Clean builds**: `make clean`

### Frequent File Locations
- **Server main logic**: `internal/server/server.go`
- **Client main logic**: `internal/client/run.go`
- **API handlers**: `internal/server/api_*.go`
- **Message types**: `internal/types/types.go`
- **Web admin UI**: `web/index.html`
- **Build artifacts**: `bin/server/` and `bin/client/`

### Development Workflow
1. **Make code changes**
2. **Build affected components**: `make server` or `make client`
3. **Run validation**: `go vet ./... && gofmt -d .`
4. **Test functionality manually** using validation scenarios above
5. **Check git status**: Review changes before committing

## Time Expectations

- **First build**: 1-20 seconds (includes dependency download)
- **Incremental builds**: <1 second  
- **go vet**: ~2 seconds
- **gofmt**: <1 second
- **Server startup**: <1 second
- **Client startup**: 2-5 seconds (plus BizHawk download time on first run)

**CRITICAL**: NEVER CANCEL any build or test command. Builds may take up to 20 seconds on first run. Always set timeouts of 30-60+ seconds for build commands.