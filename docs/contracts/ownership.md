# Ownership Matrix

| Resource           | Owner                                |
| ------------------ | ------------------------------------ |
| Session state      | `ServerSession` / `BizShuffleServer` |
| state.json writes  | `Persistence`                        |
| BizHawk            | `DesktopEmulatorService` (desktop)   |
| WS connections     | `WsHub`                              |
| Admin static + API | `serverhost/`                        |
| Admin UI           | `frontend/admin/` (HTTP/WS)          |
