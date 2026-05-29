# Persistence Contract

- File: `{dataDir}/state.json`
- Format: `ServerState` JSON (plugins omitted on save, reloaded from disk dirs)
- Write: temp file + rename
- Debounce: 500ms
- Block save if any plugin status is `error`
- On load: all players `connected: false` until WS hello

No SQLite.
