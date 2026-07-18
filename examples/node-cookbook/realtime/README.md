# Cookbook: realtime (`ws.send`, `sse.send`)

Runnable examples for `ws.send` (WebSocket broadcast) and `sse.send`
(Server-Sent Events feed). Every step below is verified in CI by
[`verify.json`](verify.json), which dials real WebSocket/SSE clients
against a real socket and asserts they receive the broadcast message —
this is genuine two-client WS fan-out and SSE delivery, not a mock.

**Honest scope note on `sync.pubsub`:** `connections/rooms.json` declares
`"sync": { "pubsub": "realtime" }` because the connections JSON schema
(`internal/config/schemas/connections.json`) currently requires a
`sync.pubsub` block on any project with WebSocket/SSE endpoints. That
Redis pubsub service is instantiated and pinged at boot
(`plugins/pubsub/plugin.go:34`), but nothing on the `ws.send`/`sse.send`
delivery path reads from it — `registerConnections` never wires the sync
block to the connection Manager. In this single-process cookbook run,
delivery to both WebSocket clients (and to the SSE client) happens
entirely through the in-process `connmgr.Manager`; there is currently no
mechanism that would fan a broadcast out to a *second* Noda instance. That
cross-instance sync is a real, but currently unimplemented, product gap —
tracked as a follow-up issue, not something this cookbook demonstrates.

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
      "channels": { "pattern": "room.{{ request.params.room }}" },
      "on_connect": "room-joined"
    },
    "feed": {
      "type": "sse",
      "path": "/events/:channel",
      "channels": { "pattern": "feed.{{ request.params.channel }}" },
      "heartbeat": "1s"
    }
  }
}
```

`on_message`/`on_disconnect` are optional per the connections schema
(`docs/02-config/connections.md`'s Endpoint Definition table) and this
family omits them — there's no message routing or presence tracking to
exercise here, only the send-side node under test. `docs/03-nodes/sse.send.md`
confirms the `sse.send` node's service slot is `connections` (prefix `sse`),
the same slot name `ws.send` uses (prefix `ws`) — both point at the
endpoint's own service name (`rooms`, `feed`), not the `realtime` pubsub
service, which is only referenced via `sync.pubsub`.

`rooms` **does** set `"on_connect": "room-joined"`
(`workflows/room-joined.json`), which `ws.send`s `{"type":"user_joined"}`
back to the connecting client's own channel. This isn't presence-tracking
product surface — it's how `verify.json` gets a deterministic signal that
a client's WebSocket connection has finished registering with the
connmgr `Manager` before the next step broadcasts to it. The server
registers the connection with the Manager *before* firing `on_connect`
(`internal/connmgr/websocket.go:236` then `:248`, both synchronous in the
same handler goroutine), so receiving the `user_joined` message is proof
the client is deliverable. Without this gate, the fasthttp/gorilla dial
returns as soon as the 101 handshake completes, which can race the
server's own post-handshake registration step under load — CI hit this
exactly as a `client b`/broadcast flake before this fix (`ws expect: i/o
timeout`), since `ws.send`'s only output is `{channel}`, giving the test
nothing else to poll for delivery. Each client's own join message is
consumed by an `expect` step immediately after its `connect` step;
later `chat` broadcasts are unaffected because `expectWSMessage` reads
past (skips) any earlier, non-matching frames — including another
client's `user_joined` broadcast that a still-open client also receives
on the shared room channel.

## `ws.send` — `POST /api/rooms/:room/broadcast`

Two WebSocket clients join the same room, then a single POST broadcasts a
chat message that both receive:

```bash
websocat ws://localhost:3000/ws/rooms/42 &   # client a
# client a immediately prints its own join confirmation:
# {"type":"user_joined"}

websocat ws://localhost:3000/ws/rooms/42 &   # client b
# client b prints its own join confirmation, and client a also sees it
# (both are on the shared room.42 channel):
# {"type":"user_joined"}

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

`feed`'s endpoint config also sets `"heartbeat": "1s"` (see
[Config shape](#config-shape) above) — a demo of the connections schema's
SSE `heartbeat` field (`docs/02-config/connections.md`), which sends a
keep-alive comment on that interval so idle SSE connections don't sit on
an open socket indefinitely. It also bounds this cookbook's own test
teardown: without it, an SSE response can hold its connection open past
the harness's shutdown deadline.

## Test harness notes

`verify.json` uses the cookbook harness's named ws/sse clients
(`ws: {client, connect|send|expect}` / `sse: {client, connect|expect}`,
`internal/testing/cookbook/verify.go`). Each `ws.connect` step is followed
by a `ws.expect` for that same client's own `{"type":"user_joined"}`
welcome message — this both consumes the connect-time frame and proves
the connection is registered with the connmgr `Manager` before the test
moves on to a step that broadcasts to it (see the `on_connect`
discussion above). `expectWSMessage`'s read loop skips any frame that
doesn't match the current step's assertions, so client a's later
`{"path":"message","equals":"hello room"}` expect transparently skips
past the `user_joined` broadcast it also receives when client b joins —
the same skip-logic `realtime-collab` relies on to reuse `ws.send`/
`ws.expect` steps across families with a connect-time welcome message.
