BizShuffle - Go + Alpine.js + Websockets

Overview

This repo is a migration target from a Laravel/Filament/Go/Lua stack to a pure Go server + Go client stack with an Alpine.js-powered admin UI. The goal is a single server that coordinates multiple BizHawk emulator clients across LAN/Internet. The server controls when games swap, pushes commands to clients via websockets, and serves required ROM/assets via HTTP.

High-level goals (from user request)
- Single server handles one session (no multi-session complexity).
- Websockets for commands; HTTP for file downloads/uploads.
- Minimal persistence: human-editable JSON state files saved on update and loaded at start.
- No authentication required; clients identify by username.
- Client is a simple one-click installer/CLI that asks for server URL and username once and saves it to a config file.

## Quick Start Installation Guide

Ready to play? Follow these simple steps to get BizShuffle up and running!

### Installation

1. **Download the Installer**
   - Go to the [Releases page](https://github.com/Michael4d45/bizhawk-shuffler-go/releases)
   - Download `bizshuffle-installer.exe`
   - Run the installer executable

2. **Run the Installer**
   - The installer will open a window with options:
     - Check "Install Server" to install the server component
     - Check "Install Client" to install the client component
     - Choose installation directories for each (they can be in different locations)
     - Click "Install" to begin
   - The installer will:
     - Download the latest release from GitHub
     - Extract files to your chosen directories
     - For client installations: Download and install BizHawk emulator
     - For client installations: Install Windows VC++ redistributables (if needed)
     - Configure the client automatically

3. **Start the Server** (if installed)
   - Navigate to the server installation directory
   - Run `bizshuffle-server.exe`
   - The server will start on `http://localhost:8080` by default
   - Open your web browser and go to `http://localhost:8080` to access the admin panel

4. **Add Game Files** (Optional)
   - Place ROM files in the server's `roms/` directory
   - Supported formats: NES, SNES, Game Boy, GBA, N64, PSX, and [more](https://github.com/TASEmulators/BizHawk?tab=readme-ov-file#cores)

### For Players

1. **Connect to the Server**
   - Navigate to the client installation directory
   - Run `bizshuffle-client.exe`
   - **First time setup**: The client will automatically search for servers on your local network
   - If a server is found, select it from the list
   - If no server is found, you'll be prompted to enter the server address:
     - **Same network**: Enter `http://SERVER_IP:8080` (ask the host for their IP address)
     - **Different network**: Enter `http://SERVER_HOSTNAME:8080` or the full URL provided by the host
   - Enter your username (this is how you'll appear to server)
   - Your settings are saved automatically for future sessions

### Playing Together

1. **Host Starts the Session**
   - Open the web admin panel at `http://localhost:8080` (if hosting) or ask the host for the admin URL
   - Click **Start Session** to begin

2. **Game Swapping**
   - The host can configure automatic swaps with a timer
   - Or manually trigger swaps from the admin panel
   - Choose between **Sync Mode** (everyone plays the same game) or **Save Mode** (different games with save state sharing)

3. **Enjoy!**
   - Games will automatically load on all players' computers
   - The host has full control via the web admin panel
   - Players just need to launch the client and connect - everything else is automatic!

### Troubleshooting

- **Can't find server?** Make sure you're on the same network, or enter the server IP manually
- **Connection issues?** Check Windows Firewall - port 8080 needs to be open on the server
- **Games not loading?** Ensure the host has uploaded game files via the admin panel
- **BizHawk not found?** Make sure you ran the installer and selected "Install Client" - BizHawk should be installed automatically

Repository layout

```
.
├── README.md
├── Makefile
├── go.mod, go.sum
├── server.lua             # BizHawk Lua script for client
├── cmd/
│   ├── server/main.go     # Server executable
│   └── client/main.go     # Client executable
├── internal/
│   ├── server/            # Server HTTP/WS handlers and logic
│   ├── client/            # Client logic and BizHawk integration
│   └── types/             # Shared types and message structures
├── web/
│   └── index.html         # Admin UI (Alpine.js)
├── plugins/               # Lua plugins directory
│   ├── README.md
│   ├── example-plugin/
│   └── read-door/
├── dist/                   # Build artifacts (created by make)
│   ├── server/
│   │   ├── bizshuffle-server
│   │   ├── state.json      # Server state persistence
│   │   ├── roms/           # Game ROM files directory
│   │   ├── saves/          # Save states directory
│   │   ├── plugins/        # Copied plugins
│   │   └── web/            # Copied web UI
│   └── client/
│       ├── bizshuffle-client
│       ├── config.json     # Client configuration
│       ├── server.lua      # Copied Lua script
│       └── BizHawk-*/      # Auto-downloaded BizHawk
├── roms/                  # ROM/asset storage (created as needed)
├── saves/                 # Save states directory
└── state.json             # Server state persistence (auto-created)
```

Build & run

**Prerequisites**: Go 1.24 or later is required (see `go.mod`).

1) Build server and client

```
make all
```

Or build individually:

```
make server  # Builds server with web UI and plugins
make client  # Builds client with Lua script
```

Alternative manual build method:

```
mkdir -p dist/server dist/client
cd cmd/server && go build -o ../../dist/server/bizshuffle-server
cd cmd/client && go build -o ../../dist/client/bizshuffle-client
# Note: Manual build doesn't copy web UI, plugins, or Lua script
```

2) Run server (flags override config file)

```
./dist/server/bizshuffle-server --host 0.0.0.0 --port 8080
```

3) Install client

Run the client binary once. On first run it will:

1. **Attempt LAN discovery**: Automatically search for BizShuffle servers on the local network
2. **Fallback to manual entry**: If no servers are found, prompt for server websocket URL (e.g. `ws://host:8080/ws`) and username
3. **Save configuration**: Store settings in `client_config.json` in the working directory

Subsequent runs read the saved configuration and will not prompt again unless the config file is missing.

**Discovery Configuration**

The client supports automatic server discovery with configurable timeouts:

```json
{
  "discovery_enabled": "true",
  "discovery_timeout_seconds": "5",
  "multicast_address": "239.255.255.250:1900"
}
```

**Manual Server Entry**

If discovery fails or is disabled, enter the server URL in one of these formats:
- `ws://host:port/ws` (WebSocket URL)
- `wss://host:port/ws` (Secure WebSocket URL)  
- `http://host:port` (HTTP URL, will be converted to WebSocket)
- `https://host:port` (HTTPS URL, will be converted to Secure WebSocket)

Communication protocol (detailed)

Websocket envelope (JSON):

```json
{
  "cmd": "<command>",
  "payload": { ... },
  "id": "<uuid>"
}
```

Server -> Client commands:
- `ping`: Health check
- `start`: Start emulation session
- `pause`: Pause emulation
- `swap`: Trigger game swap
- `message`: Display message to user
- `games_update`: Update available games list
- `clear_saves`: Clear local save states
- `reset`: Reset server session

Client -> Server messages:
- `hello`: Initial connection handshake
- `ack`: Acknowledge command receipt
- `nack`: Negative acknowledgment
- `games_update_ack`: Confirm games update
- `lua_command`: Execute Lua script command

Ack contract

Every command SHOULD be acknowledged by the recipient by sending an `ack` message with the same `id`. The server will also persist state changes immediately after handling commands.

File transfer

- Server serves ROM files from `./roms` directory at `/files/` (HTTP endpoint remains `/files/` for compatibility).
- Clients download via HTTP GET to `/files/<path>`.
- Clients may upload save state via POST `/save/upload`.

Save filename convention

- Saves are stored under `./saves/<file>` on the server.
- Upload handlers honor an explicit `filename` form field; otherwise the server will fall back to the uploaded filename or `<game>.state` when `game` is provided.

Persistence

- `state.json` stores `ServerState` (see `internal/types`). It's saved on updates via `saveState()` and loaded at server start. It's intentionally simple so manual edits are possible.

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

## Plugin System

BizShuffle supports Lua plugins that extend BizHawk functionality. Plugins can hook into various events and provide custom game logic.

### Plugin Structure

Each plugin resides in its own directory under `plugins/`:

```
plugins/my-plugin/
├── plugin.lua     # Main plugin code
├── meta.kv        # Plugin metadata (simple key=value format)
└── README.md      # Plugin documentation
```

### Plugin Metadata (meta.kv)

Plugin metadata uses a tiny key=value format (no comments, single-line values). Example:

```
name = my-plugin
version = 1.0.0
description = Example plugin
author = Plugin Author
bizhawk_version = >=2.8.0
status = disabled
```

### Plugin Management

Plugins can be managed through:
- The web admin UI plugin section
- API endpoints: `GET /api/plugins`, `POST /api/plugins/upload`, `POST /api/plugins/{name}/enable`, `POST /api/plugins/{name}/disable`
- Plugins are automatically synchronized to clients on connection

## Network Discovery

BizShuffle includes automatic LAN discovery for easy server setup:

### Server Discovery Broadcasting

- Servers broadcast their presence via UDP multicast
- Configurable broadcast interval (default: 5 seconds)
- Includes server name, host, port, and version information

### Client Discovery

- Clients can automatically discover servers on the network
- Discovery timeout configurable (default: 10 seconds)
- Falls back to manual server entry if discovery fails

### Discovery Configuration

```json
{
  "enabled": true,
  "multicast_address": "239.255.255.250:1900",
  "broadcast_interval_sec": 5,
  "listen_timeout_sec": 10
}
```

## File and Save Management

### Game File Distribution

- Server serves game files from `./roms/` directory at `/files/*` (HTTP endpoint remains `/files/` for compatibility)
- Clients download games via HTTP GET requests
- Support for primary game files and extra files (assets, patches)
- Thread-safe concurrent downloads with progress tracking

### Save State Orchestration

- Save states stored in `./saves/` directory on server
- Automatic upload/download during game swaps in save mode
- File state tracking: `none`, `pending`, `ready`
- Per-player save state management

Client UX & installation notes

- The client is a simple Go binary. Wanting a one-click installer: packaging for Windows (NSIS or similar) can wrap the binary and drop a shortcut. The client will write `config.json` in the working directory; you can change this to %APPDATA% or user profile dir later.

Download Progress Display

The client features a pacman-style download progress display that shows real-time information for file downloads:

```
Super Mario Bros. 3 (USA).zip              512.0 KiB  2.50 MiB/s 00:00 [########################################] 100%
Spyro - Year of the Dragon (USA).cue       650.2 MiB  3.20 MiB/s 03:25 [#####################                   ]  45%
```

Features:
- Real-time progress bars with visual completion indicators
- File size and download speed display
- Estimated time remaining (ETA)
- Automatic download of extra files associated with main games
- Thread-safe progress tracking for concurrent downloads

The client maintains a cache of main games configuration from the server, allowing it to automatically download any extra files (assets, patches, etc.) when downloading primary game files.

Command-line flags

- Server: `--host` and `--port` to override listening address (these override config file if present).

## Communication Protocol

### Websocket Protocol

BizShuffle uses websockets for real-time communication between server and clients. The protocol uses JSON envelopes:

```json
{
  "cmd": "<command>",
  "payload": { ... },
  "id": "<uuid>"
}
```

### Server to Client Commands

- `ping`: Health check
- `start`: Start emulation session
- `pause`: Pause emulation
- `swap`: Trigger game swap
- `message`: Display message to user
- `games_update`: Update available games list
- `clear_saves`: Clear local save states
- `reset`: Reset server session

### Client to Server Messages

- `hello`: Initial connection handshake
- `ack`: Acknowledge command receipt
- `nack`: Negative acknowledgment
- `games_update_ack`: Confirm games update
- `lua_command`: Execute Lua script command

### API Endpoints

Core server management:
- `GET /api/games` - List available games
- `POST /api/start` - Start session
- `POST /api/pause` - Pause session
- `POST /api/reset` - Reset session
- `POST /api/do_swap` - Trigger manual swap
- `POST /api/random_swap` - Trigger random swap
- `POST /api/mode` - Change game mode
- `POST /api/toggle_swaps` - Enable/disable swaps

Player management:
- `GET /api/players` - List connected players
- `POST /api/message_player` - Send message to specific player
- `POST /api/message_all` - Broadcast message
- `POST /api/remove_player` - Disconnect player

Plugin management:
- `GET /api/plugins` - List available plugins
- `POST /api/plugins/upload` - Upload new plugin
- `POST /api/plugins/{name}/enable` - Enable plugin
- `POST /api/plugins/{name}/disable` - Disable plugin

File operations:
- `GET /files/*` - Download game files
- `POST /upload` - Upload files
- `GET /save/*` - Download save states
- `POST /save/upload` - Upload save states

## Web Admin Interface

The server provides a modern Alpine.js-powered web interface at the root URL:

Features:
- Real-time session status and controls
- Player management and monitoring
- Game library management
- Plugin configuration
- Swap scheduling and progress tracking
- File upload interface
- Live websocket connection status

The interface uses:
- **Alpine.js** for lightweight reactive components
- **Tailwind CSS** for styling
- **WebSockets** for real-time updates

Notes

- No authentication is implemented by design.

