# Retry Semantics

| Operation                   | Behavior                             |
| --------------------------- | ------------------------------------ |
| Swap sendAndWait            | 20s                                  |
| Lua IPC command             | 10s                                  |
| Save file ready (GET /save) | 30s poll 100ms                       |
| state.json save             | 500ms debounce                       |
| ROM download                | 3 retries, 500ms exponential backoff |
| Lua reconnect               | 1s, 3s, 5s                           |
| WS client reconnect         | 2s backoff (client)                  |
