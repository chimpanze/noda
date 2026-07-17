# Realtime: WebSocket & SSE Connections

Noda serves realtime endpoints (WebSocket and Server-Sent Events) defined in `connections/*.json`. A connection endpoint subscribes each client to a **channel**, runs **lifecycle workflows** as clients connect/message/disconnect, and lets your workflows **broadcast** to a channel with `ws.send`/`sse.send`. This guide covers the whole model end to end.

## Defining an Endpoint

A connections file has a required `sync` block and an `endpoints` map (keyed by endpoint name):

```json
// connections/board.json
{
  "sync": { "pubsub": "events" },
  "endpoints": {
    "board": {
      "type": "websocket",
      "path": "/ws/board/:room_id",
      "middleware": ["auth.jwt"],
      "channels": {
        "pattern": "board.{{ request.params.room_id }}",
        "max_per_channel": 100
      },
      "ping_interval": "30s",
      "on_connect": "board-on-connect",
      "on_message": "board-on-message",
      "on_disconnect": "board-on-disconnect"
    }
  }
}
```

- **`type`** — `websocket` or `sse`.
- **`path`** — the URL clients connect to. `:name` segments are path parameters.
- **`middleware`** — runs on the upgrade request. `auth.jwt` authenticates the client and makes the user id available (as `auth.sub` in the pattern, and `input.user_id` in handlers).
- **`sync.pubsub`** — **required.** Names a `pubsub` service in `noda.json`. Broadcasts are published through it so they fan out to clients connected to *any* instance, not just the one that called `ws.send`. Without it, broadcasts only reach clients on the local instance.

## The Subscription / Channel Model

When a client connects, the endpoint's **`channels.pattern`** is evaluated to produce the **channel** that client subscribes to. The pattern is an expression resolved against the connection context:

| Variable | Meaning |
|----------|---------|
| `auth.sub` | The authenticated user id (when an auth middleware ran) |
| `request.params.<name>` | A path parameter from the endpoint's `path` |

So with `path: "/ws/board/:room_id"` and `pattern: "board.{{ request.params.room_id }}"`, a client connecting to `/ws/board/42` is subscribed to channel **`board.42`**. A pattern of `tasks.{{ auth.sub }}` gives every user their own private channel.

`max_per_channel` caps how many simultaneous connections may share one resolved channel; further connections are rejected.

> Clients do not choose a channel by sending a "subscribe" message — the channel is fixed at connect time by the URL (path params) and identity (`auth.sub`) via the pattern. To listen on a different channel, connect to a different URL.

## Lifecycle Handlers

`on_connect`, `on_message`, and `on_disconnect` are **workflow-id strings** — the id of a workflow Noda runs when that event fires. (They are *not* `{workflow, input}` objects; the input is supplied automatically.)

Each handler workflow receives this `input`:

| `input` field | Available in | Value |
|---------------|--------------|-------|
| `input.connection_id` | all | Unique id for this connection |
| `input.channel` | all | The resolved channel the client is on (e.g. `board.42`) |
| `input.endpoint` | all | The endpoint name (e.g. `board`) |
| `input.user_id` | all | Authenticated user id, or empty string |
| `input.params` | all | Map of path parameters (e.g. `{ "room_id": "42" }`) |
| `input.data` | `on_message` only | The parsed incoming client message |

Example `on_message` workflow that echoes the message to everyone on the channel:

```json
// workflows/board-on-message.json
{
  "id": "board-on-message",
  "nodes": {
    "broadcast": {
      "type": "ws.send",
      "services": { "connections": "board" },
      "config": {
        "channel": "{{ input.channel }}",
        "data": {
          "from": "{{ input.user_id }}",
          "text": "{{ input.data.text }}"
        }
      }
    }
  },
  "edges": []
}
```

## Broadcasting with `ws.send` / `sse.send`

`ws.send` (and `sse.send`) deliver a message to subscribers of a channel. Two things wire it up:

1. **The `connections` service slot** binds to the **endpoint name**, not a `noda.json` service: `"services": { "connections": "board" }`. (At startup each endpoint is registered as a service under its own name.) `noda_validate_config` flags a slot that names an undefined endpoint.
2. **The `channel` field** selects recipients. It must be a **literal channel name** matching a channel clients are subscribed to (one produced by `channels.pattern`): `"channel": "board.42"` → only clients on `board.42`. Wildcard patterns (e.g. `"board.*"` or `"*"`) are **rejected with a validation error** — to reach several rooms, send once per room (e.g. with `control.loop`).

So `ws.send`'s `channel` must line up with what `channels.pattern` produces, or no one receives the message.

## End-to-End: POST → store → broadcast

A REST route persists a message, then broadcasts it to everyone subscribed to the room's channel.

`noda.json` (the pubsub service `sync.pubsub` references):

```json
{
  "services": {
    "db":     { "plugin": "postgres", "config": { "url": "{{ $env('DATABASE_URL') }}" } },
    "events": { "plugin": "pubsub",   "config": { "url": "{{ $env('REDIS_URL') }}" } }
  }
}
```

`routes/post-message.json`:

```json
{
  "id": "post-message",
  "method": "POST",
  "path": "/api/board/:room_id/messages",
  "middleware": ["auth.jwt"],
  "trigger": {
    "workflow": "post-message",
    "input": {
      "room_id": "{{ request.params.room_id }}",
      "user_id": "{{ auth.sub }}",
      "text": "{{ request.body.text }}"
    }
  }
}
```

> **Numeric-string coercion:** path params are string-typed transport, so `/api/board/42/messages` delivers `input.room_id` as the number `42` by default. If `room_id` must stay a string (TEXT column, leading zeros), set `"coerce": false` on the route trigger, or convert explicitly with `{{ string(input.room_id) }}`. JSON body values always keep their JSON types.

`workflows/post-message.json`:

```json
{
  "id": "post-message",
  "nodes": {
    "store": {
      "type": "db.create",
      "services": { "database": "db" },
      "config": {
        "table": "messages",
        "data": {
          "id": "{{ $uuid() }}",
          "room_id": "{{ input.room_id }}",
          "user_id": "{{ input.user_id }}",
          "body": "{{ input.text }}"
        }
      }
    },
    "broadcast": {
      "type": "ws.send",
      "services": { "connections": "board" },
      "config": {
        "channel": "board.{{ input.room_id }}",
        "data": { "from": "{{ input.user_id }}", "text": "{{ input.text }}" }
      }
    },
    "respond": {
      "type": "response.json",
      "config": { "status": 201, "body": { "ok": true } }
    }
  },
  "edges": [
    { "from": "store", "to": "broadcast", "output": "success" },
    { "from": "broadcast", "to": "respond", "output": "success" }
  ]
}
```

A client connected to `/ws/board/42` (subscribed to `board.42`) receives the broadcast the moment anyone POSTs to `/api/board/42/messages`. The `messages` table must exist first — create it with a migration (see the Migrations guide, `noda://docs/migrations`).

## SSE

Server-Sent Events work the same way: set `"type": "sse"` on the endpoint and broadcast with `sse.send` (which also takes a `connections` slot and a `channel`). SSE is one-way (server → client), so SSE endpoints have no `on_message` handler.
