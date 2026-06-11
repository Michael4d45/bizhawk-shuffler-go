# Retry Semantics

| Operation                   | Behavior                                      |
| --------------------------- | --------------------------------------------- |
| Swap `sendAndWait`          | 20s (`serverhost/ws.go`)                      |
| Lua IPC command             | 10s per command (`clienthost/bizhawk_ipc.go`) |
| Save file ready (GET /save) | 30s poll every 100ms (`serverhost/api_saves.go`) |
| `state.json` save           | 500ms debounce (`serverhost/state.go`)        |
| ROM download                | 3 retries, 500ms fixed delay between attempts (`clienthost/progress_tracking_api.go`) |
| Lua IPC reconnect           | 1s fixed delay between connect attempts (`clienthost/bizhawk_ipc.go`) |
| WS client reconnect         | 2s backoff after dial failure (`clienthost/wsclient.go`) |
| Save file create (locked)   | 3 retries, 500ms between attempts (`clienthost/api.go`) |
| Save verify after SAVE      | 3 attempts, 200ms between attempts (`clienthost/controller.go`) |
