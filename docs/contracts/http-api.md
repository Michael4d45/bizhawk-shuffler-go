# HTTP API Contract

Routes implemented by `serverhost` — see `RegisterRoutes` in `serverhost/server.go`.

Base: `http://{host}:{port}`. Most mutations return plain `"ok"` or JSON as noted.

## WebSocket

- `GET /ws` — player and admin WebSocket (see `ws-protocol.md`)

## Session & scheduling

| Method   | Path                            | Notes                                      |
| -------- | ------------------------------- | ------------------------------------------ |
| POST     | `/api/start`                    | `running=true`; broadcast `start`          |
| POST     | `/api/pause`                    | `running=false`; broadcast `pause`         |
| POST     | `/api/reset`                    | Optional `{ pause, clear_saves, clear_completions }` (default true) |
| POST     | `/api/clear_saves`              | Trash `./saves`; broadcast `clear_saves`   |
| POST     | `/api/toggle_swaps`             | Toggle `swap_enabled`                      |
| POST     | `/api/toggle_countdown`         | Toggle 3-2-1 before auto swap              |
| POST     | `/api/toggle_prevent_same_game` | Toggle better random                       |
| POST     | `/api/toggle_player_name_hash`  | Toggle save-mode player name hash assignment |
| POST     | `/api/do_swap`                  | Async full swap                            |
| POST     | `/api/random_swap`              | Body `{ "player": "name" }`                |
| GET/POST | `/api/mode`                     | Body `{ "mode": "sync" \| "save" }` on POST |
| POST     | `/api/mode/setup`               | Scan `./roms/`, setup catalog              |
| GET/POST | `/api/interval`                 | `min_interval_secs` / `max_interval_secs`  |

## State & sharing

| Method | Path               | Notes                                                        |
| ------ | ------------------ | ------------------------------------------------------------ |
| GET    | `/state.json`      | `{ "state": ServerState }`                                   |
| GET    | `/api/share_urls`  | `{ "lan": string[], "wan": string \| null, "local_only": boolean }` |
| GET    | `/`                | Admin UI (embedded static)                                   |

## Games & players

| Method      | Path                                              | Notes                                           |
| ----------- | ------------------------------------------------- | ----------------------------------------------- |
| GET         | `/api/games`                                      | `main_games`, `game_instances`, `games`         |
| POST        | `/api/games`                                      | Partial state update + `games_update` broadcast |
| POST        | `/api/swap_player`                                | `{ player, game?, instance_id? }`               |
| POST        | `/api/swap_all_to_game`                           | `{ game }`                                      |
| POST        | `/api/add_player`                                 | Add player to registry                          |
| POST        | `/api/remove_player`                              | Remove player                                   |
| POST        | `/api/players/{player}/completed_games`           | Body `{ "game": "..." }`                        |
| DELETE      | `/api/players/{player}/completed_games?game=...`  | Remove one completed game                       |
| POST        | `/api/players/{player}/completed_instances`       | Body `{ "instance": "..." }`                    |
| DELETE      | `/api/players/{player}/completed_instances?instance=...` | Remove one completed instance          |
| POST        | `/api/players/remove_all_completions`             | Clear all players' completions                  |
| POST        | `/api/games/{game}/mark_completed_all`            | Mark game completed for all players             |
| POST        | `/api/instances/{instance}/mark_completed_all`    | Mark instance completed for all players         |

## Messaging & config

| Method | Path                                                    |
| ------ | ------------------------------------------------------- |
| POST   | `/api/message_player`                                   |
| POST   | `/api/message_all`                                      |
| POST   | `/api/check_player_config`                              |
| POST   | `/api/update_player_config`                             |
| POST   | `/api/set_config_keys`                                  |

## Plugins

| Method   | Path                                                    |
| -------- | ------------------------------------------------------- |
| GET      | `/api/plugins`                                          |
| GET      | `/api/plugins/{name}`                                   |
| GET/POST | `/api/plugins/{name}/settings` (POST requires `status`) |
| POST     | `/api/plugins/{name}/reload`                            |
| DELETE   | `/api/plugins/{name}`                                   |
| POST     | `/api/open_plugins_folder`                              |
| POST     | `/api/open_roms_folder`                                 |

## Files & saves

| Method | Path                    | Notes                                      |
| ------ | ----------------------- | ------------------------------------------ |
| GET    | `/files/list.json`      | ROM listing                                |
| GET    | `/files/{path}`         | Download from `./roms/`                    |
| GET    | `/files/plugins/{path}` | Plugin files                               |
| POST   | `/upload`               | Multipart `file` → `./roms/`               |
| GET    | `/api/BizhawkFiles.zip` | BizHawk-related bundle for client setup    |
| GET    | `/save/{filename}`      | Save download (30s wait for ready state)   |
| POST   | `/save/upload`          | Multipart `save` (+ optional `filename`)   |
| POST   | `/save/no-save`         | Form `instance_id` → instance `none`       |
