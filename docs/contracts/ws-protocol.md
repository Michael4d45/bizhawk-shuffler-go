# WebSocket Protocol Contract

## Envelope

```json
{ "cmd": "<name>", "id": "<uuid>", "payload": {} }
```

## Ack contract

- Recipient sends `{ "cmd": "ack", "id": "<same>" }` or `nack` with `payload.reason`
- Server `sendAndWait`: 20s timeout (`SWAP_WAIT_MS`)

## Ping

- Server sends WebSocket **Ping** control frame (not JSON), payload = Unix nanoseconds string
- Client Pong updates `player.ping_ms`

## Commands

See `protocol.CommandName` in `protocol/schemas.go`.
