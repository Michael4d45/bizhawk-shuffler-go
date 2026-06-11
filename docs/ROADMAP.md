# Implementation Roadmap

Planned work not yet in the codebase. See [SPEC.md](SPEC.md) for what ships today. Remove a section here when it lands (same PR as SPEC/contracts updates).

---

## Player-name hashing for game assignment

**Problem:** In save mode, instance assignment is driven by shuffle / first-free / preference tiers. Some designs want deterministic mapping from player name → instance or game (e.g. same name always gets the same slot until swapped).

**Today:** `serverhost/game_modes.go` uses `findAvailableInstanceForPlayer`, round-robin over shuffled instances, and random preference logic — no hash of `player.Name`.

**Likely direction:** Add optional mode or flag on `ServerState`; hash name to index into `game_instances` with collision fallback to current round-robin. Must not break sync mode (single game for all).

**Touches:** `serverhost/game_modes.go`, `protocol.ServerState`, admin mode/settings UI, tests in `serverhost/game_modes_test.go` and `testing/integration/`.

---

## `check_config` / `update_config` (WebSocket)

**Problem:** Admin **Config** on a player should read and write keys in that player’s local `config.json` (paths, names, etc.) without manual file edit on the player machine.

**Today:**

- Server sends `check_config` / `update_config` over WS (`serverhost/api_config.go`).
- Client `Controller` **acks only** — no file read, no `config_response` payload (`clienthost/controller.go`).
- Admin expects `config_response` with key values (see `frontend/admin` config flow).

**Likely direction:**

1. `check_config` — read requested keys from `config.json`, reply `config_response` with values.
2. `update_config` — merge `config_updates` into `config.json`, persist, ack/nack on failure.
3. Restrict key allowlist (no arbitrary secrets) if needed.

**Touches:** `clienthost/controller.go`, `clienthost/config.go`, `protocol/schemas.go`, admin UI, integration test with real WS client.

---

## Retry backoff policy

**Problem:** Retries use **fixed** delays today. Older docs sometimes described exponential or staged backoff.

**Today (authoritative — see [contracts/retry-semantics.md](contracts/retry-semantics.md)):**

| Operation | Behavior |
| --------- | -------- |
| ROM download | 3 attempts, 500ms between |
| Lua IPC reconnect | 1s between connect attempts |
| WS reconnect | 2s after dial failure |

**Decision needed:** Keep fixed delays (simpler, current behavior) or adopt staged/exponential backoff for flaky LAN downloads and slow BizHawk startup. If unchanged, no code work — roadmap item closes as “won’t do.”

**Touches (only if changing):** `clienthost/progress_tracking_api.go`, `clienthost/bizhawk_ipc.go`, `clienthost/wsclient.go`, `docs/contracts/retry-semantics.md`.
