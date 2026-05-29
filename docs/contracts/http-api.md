# HTTP API Contract

Routes implemented by `serverhost` — see `RegisterRoutes` in `serverhost/server.go`.

## Session

- POST `/api/start`, `/api/pause`, `/api/clear_saves`
- POST `/api/toggle_swaps`, `/api/toggle_countdown`, `/api/toggle_prevent_same_game`
- POST `/api/do_swap`, `/api/random_swap`
- GET/POST `/api/mode`, POST `/api/mode/setup`
- GET/POST `/api/interval`

## State

- GET `/state.json` → `{ "state": ServerState }`
- GET `/api/share_urls` → `{ "lan": string[], "wan": string | null, "local_only": boolean }`

## Files

- GET `/files/*`, `/files/list.json`, POST `/upload`
- GET `/files/plugins/*`
- GET `/save/*`, POST `/save/upload`, POST `/save/no-save`

## Players, games, plugins

- Player, game, and plugin endpoints as registered in `serverhost/server.go`.
