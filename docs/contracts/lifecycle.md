# Lifecycle

## Shutdown order (desktop app exit)

1. **Join session** — `JoinSession.Stop()` (`clienthost/join_session.go`):
   - Cancel session context
   - Stop WebSocket client
   - Close Lua IPC (`BizhawkIPC.Close`)
   - Terminate BizHawk (`BizHawkController.Terminate`)
2. **Host session** — `hostsession.Session.Stop()`:
   - `Server.BeginShutdown()`
   - Close listener (stop accepting)
   - Cancel session context (in-flight `/ws` handlers)
   - `Server.Shutdown()` — drain WebSockets (up to 3s)
   - `http.Server.Close()` — not graceful `Shutdown` (avoids blocking on open `/ws`)
   - Wait for serve goroutine

`StopJoinSession` also waits 300ms settle delay before a re-join.

## Desktop Host

1. `serverhost` HTTP server starts in `dataDir` (after `Chdir`)
2. Open admin at `http://127.0.0.1:{port}/` (or bind-specific URL)

No player client or BizHawk on this path.

## Desktop Join

1. Dependencies panel: BizHawk (+ VC++ on Windows, Mono on Linux) satisfied
2. Reserve Lua port → write `lua_server_port.txt` under `dataDir`
3. `EnsureServerLua` → launch `EmuHawk` / `EmuHawkMono.sh` with `{dataDir}/server.lua`
4. `StartJoinSession` → WebSocket `hello` to server URL (30s connect timeout)

To host and play on one machine: **Host**, then **Join** using the hosted URL (auto-filled when the server URL field is empty).

## Headless / release server

`bizshuffle-server` — HTTP admin + session only; no client/emulator in that process.
