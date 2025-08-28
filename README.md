BizShuffle - Go + HTMX + Websockets

Overview

This repo is a migration target from a Laravel/Filament/Go/Lua stack to a pure Go server + Go client stack with an HTMX-powered admin UI. The goal is a single server that coordinates multiple BizHawk emulator clients across LAN/Internet. The server controls when games swap, pushes commands to clients via websockets, and serves required ROM/assets via HTTP.

High-level goals (from user request)
- Single server handles one session (no multi-session complexity).
- Websockets for commands; HTTP for file downloads/uploads.
- Minimal persistence: human-editable JSON state files saved on update and loaded at start.
- No authentication required; clients identify by username.
- Client is a simple one-click installer/CLI that asks for server URL and username once and saves it to a config file.

Repository layout

- cmd/server - server executable (main.go)
- cmd/client - client executable (main.go)
- internal/types - shared types and message envelopes
- web - HTMX admin UI (index.html, static assets)
- files - directory for files to serve to clients (ROMs, CUE/BINs, save bundles)
- state.json - server persisted session state (auto-created)

Build & run

1) Build server and client

```
cd cmd/server; go build -o ../../bin/bizshuffle-server
cd cmd/client; go build -o ../../bin/bizshuffle-client
```

2) Run server (flags override config file)

```
./bin/bizshuffle-server --host 0.0.0.0 --port 8080
```

3) Install client

Run the client binary once. On first run it will prompt for server websocket URL (e.g. ws://host:8080/ws) and a username, then save to `client_config.json` in the working folder. Subsequent runs read that file and will not prompt again.

Communication protocol (detailed)

Websocket envelope (JSON):

{
	"cmd": "<command>",
	"payload": { ... },
	"id": "<uuid or timestamp>"
}

Server -> Client commands (examples):
- start: start emulation loop
- pause: pause emulation
- reset: reset server session state
- clear_saves: instruct clients to delete local save states
- toggle_swaps: enable/disable automatic swaps (payload: {"enabled": bool})
- update_games: payload contains the new ordered list of games; clients should download missing files
- download_file: instruct client to fetch a file via HTTP from server and store locally

Client -> Server messages:
- ack: acknowledge command (must include id of original message)
- state_update: client sends current status (current game, is emulator running, etc.)
- file_upload_complete: notify server that upload finished (if implemented)

Ack contract

Every command SHOULD be acknowledged by the recipient by sending an `ack` message with the same `id`. The server will also persist state changes immediately after handling commands.

File transfer

- Server serves files from `./files` at `/files/`.
- Clients download via HTTP GET to `/files/<path>`.
- Clients may upload save state via POST `/upload/state` (TBD).

Save filename convention

- Saves are stored under `./saves/<file>` on the server..
- Upload handlers honor an explicit `filename` form field; otherwise the server will fall back to the uploaded filename or `<game>.state` when `game` is provided.

Persistence

- `state.json` stores `ServerState` (see `internal/types`). It's saved on updates via `saveState()` and loaded at server start. It's intentionally simple so manual edits are possible.

Orchestration persistence

- Swap orchestration runs (triggered by `/api/do_swap`) are now persisted inside `ServerState.Orchestrations` keyed by an orchestration ID. This lets an admin inspect partial or failed swap runs in `state.json` and resume or debug them manually.
- Orchestration state includes mapping (player->game), per-player statuses and results, timestamps, and an overall completed flag.

Game Modes

BizShuffle supports two main game modes that control how games are swapped between players:

### Sync Mode (`sync`)

**Description**: All players play the same game and swap simultaneously. No save files are uploaded or downloaded during swaps.

**Behavior**:
- When a swap occurs, all connected players receive the same game
- No save state management - players start fresh on each game
- Ideal for competitive scenarios where everyone should play the same game at the same time
- Default mode when no mode is explicitly set

**Use Cases**:
- Racing scenarios where all players compete on the same game
- Synchronized gameplay sessions
- When save state management is not needed

### Save Mode (`save`) 

**Description**: Players play different games and perform save upload/download orchestration on swap. Each player gets a different game assigned based on a hash of their name.

**Behavior**:
- Each player gets assigned a different game using deterministic distribution
- Save states are uploaded/downloaded during swaps to maintain progress
- Players can continue from where the previous player left off
- More complex orchestration with potential for partial failures

**Use Cases**:
- Collaborative gameplay where players take turns on different games
- Scenarios where preserving game progress is important
- When you want different players working on different games simultaneously

### Changing Game Modes

Game modes can be changed through:
- The web admin UI mode selector
- API endpoint: `POST /api/mode` with JSON body `{"mode": "sync"}` or `{"mode": "save"}`
- The mode is persisted in `state.json` and survives server restarts

**Note**: Changing modes while players are actively playing may cause disruption. It's recommended to pause the session before changing modes.

Client UX & installation notes

- The client is a simple Go binary. Wanting a one-click installer: packaging for Windows (NSIS or similar) can wrap the binary and drop a shortcut. The client will write `client_config.json` in the working directory; you can change this to %APPDATA% or user profile dir later.

Command-line flags

- Server: `--host` and `--port` to override listening address (these override config file if present).

Notes

- No authentication is implemented by design.

Contact

Keep iterating. This README will be updated as features are implemented.

