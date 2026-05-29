# Lifecycle

## Shutdown order

1. Stop join session (`JoinSession.Stop` — WebSocket client, Lua IPC, BizHawk process)
2. Flush pending saves / disconnect Lua
3. Stop embedded server (desktop Host)
4. Stop discovery listener

## Desktop Host

1. `serverhost` HTTP server starts in `dataDir` (after `Chdir`)
2. Open admin at `http://127.0.0.1:{port}/` (or bind-specific URL)

No player client or BizHawk on this path.

## Desktop Join

1. Dependencies panel: BizHawk (+ VC++ on Windows) satisfied
2. Reserve Lua port → write `lua_server_port.txt` under `dataDir`
3. `EnsureServerLua` → launch `EmuHawk` with `{dataDir}/server.lua`
4. `StartJoinSession` → WebSocket `hello` to server URL

To host and play on one machine: **Host**, then **Join** against the hosted URL (or pick it from discovery).

## Headless / release server

`bizshuffle-server` — HTTP admin + session only; no client/emulator in that process.
