# BizHawk assets

Source files copied or bundled into runtime data directories.

| Path         | Purpose                                                 |
| ------------ | ------------------------------------------------------- |
| `server.lua` | BizHawk Lua IPC listener; synced to the client data dir |
| `plugins/`   | Bundled sample plugins; copy into `{dataDir}/plugins/`  |

At runtime the server loads plugins from `{dataDir}/plugins/` (not this repo folder). See `plugins/README.md` for plugin structure and hooks.
