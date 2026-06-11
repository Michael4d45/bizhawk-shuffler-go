# Implementation Roadmap

Planned work not yet in the codebase. See [SPEC.md](SPEC.md) for what ships today. Remove a section here when it lands (same PR as SPEC/contracts updates).

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
