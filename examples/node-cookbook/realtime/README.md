# Cookbook: realtime (`ws.send`, `sse.send`)

Runnable examples for `ws.send` (WebSocket broadcast) and `sse.send`
(Server-Sent Events feed), both synced across instances through a Redis
`pubsub` service. Every step below is verified in CI by
[`verify.json`](verify.json).

This family is also the **pubsub-delivery proof** referenced by the
[`events` cookbook README](../events/README.md#event-emit--pubsub-mode--api-notify):
`event.emit(mode=pubsub)` only proves the publish call succeeds — it does
not prove a subscriber receives anything. Here, `connections/rooms.json`
wires `sync.pubsub` to the same kind of Redis pubsub service, and
`ws.send`/`sse.send` publish through it; `verify.json` connects real
WebSocket/SSE clients and asserts they actually receive the broadcast
message end-to-end, closing the loop that the `events` family leaves open.

## Run

This project needs Redis. CI's cookbook walker starts a Redis container and
exports `REDIS_URL` automatically. To run it yourself:

```bash
export REDIS_URL=redis://localhost:6379/0
go run ./cmd/noda start --config examples/node-cookbook/realtime
```

## Config shape

`connections/rooms.json` (mirrors `examples/realtime-collab/connections/collaboration.json`,
confirmed against `docs/02-config/connections.md`):

```json
{
  "sync": { "pubsub": "realtime" },
  "endpoints": {
    "rooms": {
      "type": "websocket",
      "path": "/ws/rooms/:room",
      "channels": { "pattern": "room.{{ request.params.room }}" }
    },
    "feed": {
      "type": "sse",
      "path": "/events/:channel",
      "channels": { "pattern": "feed.{{ request.params.channel }}" }
    }
  }
}
```

`on_connect`/`on_message`/`on_disconnect` are all optional per the
connections schema (`docs/02-config/connections.md`'s Endpoint Definition
table) — this family omits them entirely; there's no presence tracking or
message routing to exercise here, only the send-side node under test. That
also means there is **no** connect-time welcome message (no `user_joined`
broadcast) on this endpoint — unlike `realtime-collab`, which broadcasts one
from its `on_connect` workflow. `docs/03-nodes/sse.send.md` confirms the
`sse.send` node's service slot is `connections` (prefix `sse`), the same
slot name `ws.send` uses (prefix `ws`) — both point at the endpoint's own
service name (`rooms`, `feed`), not the `realtime` pubsub service, which is
only referenced via `sync.pubsub`.

## `ws.send` — `POST /api/rooms/:room/broadcast`

Two WebSocket clients join the same room, then a single POST broadcasts a
chat message that both receive:

```bash
websocat ws://localhost:3000/ws/rooms/42 &   # client a
websocat ws://localhost:3000/ws/rooms/42 &   # client b

curl -X POST localhost:3000/api/rooms/42/broadcast \
  -H 'Content-Type: application/json' -d '{"message": "hello room"}'
# → 200 {"channel":"room.42"}

# both websocat sessions print:
# {"type":"chat","message":"hello room"}
```

## `sse.send` — `POST /api/feeds/:channel/notify`

An SSE client subscribes to a feed channel, then a POST pushes a note:

```bash
curl -N localhost:3000/events/news &

curl -X POST localhost:3000/api/feeds/news/notify \
  -H 'Content-Type: application/json' -d '{"text": "breaking"}'
# → 200 {"channel":"feed.news"}

# the curl -N session prints:
# data: {"kind":"note","text":"breaking"}
```

## Test harness notes

`verify.json` uses the cookbook harness's named ws/sse clients
(`ws: {client, connect|send|expect}` / `sse: {client, connect|expect}`,
`internal/testing/cookbook/verify.go`). Because this endpoint has no
`on_connect` welcome message, there's nothing for the expect-scan's
skip-logic to skip over here — but that skip exists precisely so a family
*with* a welcome broadcast (like `realtime-collab`) can reuse the same
`ws.send`/`ws.expect` steps without the connect-time message showing up as
an unexpected first frame.
