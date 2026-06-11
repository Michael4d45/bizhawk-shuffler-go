# WebSocket Protocol Contract

## Envelope

```json
{ "cmd": "<name>", "id": "<uuid>", "payload": {} }
```

## Ack contract

- Recipient sends `{ "cmd": "ack", "id": "<same>" }` or `nack` with `payload.reason`
- Server `sendAndWait`: **20s** timeout (`serverhost/ws.go`; used for `swap` only)

## Ping

- Server sends WebSocket **Ping** control frame (not JSON), payload = Unix nanoseconds string
- Client must echo the Ping payload in its Pong (gorilla/websocket default); server `PongHandler` updates `player.ping_ms`

## Commands

See `protocol.CommandName` in `protocol/schemas.go`.
