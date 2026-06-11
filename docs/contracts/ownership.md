# Ownership Matrix

Reflects **implemented** types in this repository (see `serverhost/`, `clienthost/`, `cmd/desktop/`).

| Resource              | Owner                                                                 |
| --------------------- | --------------------------------------------------------------------- |
| Session state         | `serverhost.Server` (`state`, game modes, scheduler)                  |
| `state.json` writes   | `serverhost.Server` (`state.go` — debounced saver, atomic write)      |
| WebSocket hub         | `serverhost.Server` (`ws.go` — player/admin registries, `sendAndWait`) |
| REST + admin static   | `serverhost.Server` (`server.go`, `api_*.go`, embedded `static/`)   |
| Admin UI (React)      | `frontend/admin/` (built into `serverhost/static/`)                   |
| Desktop Host shell    | `cmd/desktop/fyneapp/` + `hostsession.Session` (embedded server)    |
| Desktop Join session  | `clienthost.JoinSession` (`join_session.go`)                          |
| BizHawk process       | `clienthost.BizHawkController` (`bizhawk.go`)                         |
| Lua TCP IPC           | `clienthost.BizhawkIPC` (`bizhawk_ipc.go`)                            |
| Player WS + downloads | `clienthost.WSClient`, `clienthost.Controller`                        |
| Plugin file sync      | `clienthost.PluginSyncManager` (`plugin_sync.go`)                     |
| BizHawk Lua runtime   | `assets/server.lua` (embedded; copied to `{dataDir}/server.lua`)      |
| Structured log hints  | `obslog/` (URL/share helpers; not session authority)                |
