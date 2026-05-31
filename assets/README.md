# BizHawk assets

Source files bundled or copied into runtime data directories.

| Path         | Purpose                                                                 |
| ------------ | ----------------------------------------------------------------------- |
| `server.lua` | BizHawk Lua IPC listener; embedded via `embed.go` into desktop binaries |
| `plugins/`   | Bundled sample plugins; copy into `{dataDir}/plugins/`                  |

`luasocket/linux-amd64/core.so` is **not** version controlled (see `.gitignore`). On Linux amd64, `make` runs `scripts/build-luasocket-linux-amd64.sh` when the file is missing; rerun with `FORCE=1` to rebuild after LuaSocket/Lua changes.

The `assets` Go module (`embed.go`) embeds `server.lua` for `clienthost`; edit only this copy of the script.

At runtime the server loads plugins from `{dataDir}/plugins/` (not this repo folder). See `plugins/README.md` for plugin structure and hooks.
