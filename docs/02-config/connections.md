# Connections

Files in `connections/*.json`. Defines WebSocket and SSE endpoints.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `sync` | object | yes | Cross-instance sync service |
| `sync.pubsub` | string | yes | PubSub service name from `noda.json` |
| `endpoints` | object | yes | Map of endpoint ID to endpoint definition |

## Endpoint Definition

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `type` | string | yes | `"websocket"` or `"sse"` |
| `path` | string | yes | URL path (supports `:param`) |
| `max_connections` | integer | no | Maximum total connections for this endpoint |
| `middleware` | array | no | Endpoint middleware |
| `middleware_preset` | string | no | Named middleware preset |
| `channels` | object | no | Channel configuration |
| `channels.pattern` | string | no | Channel name pattern (expression) |
| `channels.max_per_channel` | integer | no | Max connections per channel |
| `ping_interval` | string | no | WebSocket ping interval (default `"30s"`) |
| `max_message_size` | string | no | WebSocket max message size (default 64KB) |
| `heartbeat` | string | no | SSE heartbeat interval (default `"30s"`) |
| `retry` | string | no | SSE retry interval in milliseconds |
| `on_connect` | string | no | Workflow ID on connection |
| `on_message` | string | no | Workflow ID on message (WebSocket only) |
| `on_disconnect` | string | no | Workflow ID on disconnect |

```json
{
  "sync": {
    "pubsub": "redis-pubsub"
  },
  "endpoints": {
    "chat": {
      "type": "websocket",
      "path": "/ws/chat/:room",
      "middleware": ["auth.jwt"],
      "channels": {
        "pattern": "chat.{{ request.params.room }}",
        "max_per_channel": 50
      },
      "ping_interval": "30s",
      "on_connect": "chat-join",
      "on_message": "chat-message",
      "on_disconnect": "chat-leave"
    }
  }
}
```

## Channel Patterns

The `channels.pattern` field maps each connection to a channel name using expressions. The expression context provides:

| Variable | Description |
|----------|-------------|
| `request.params.<name>` | URL path parameters from the endpoint path |
| `auth.sub` | Authenticated user ID from JWT |

Examples:

```
"chat.{{ request.params.room }}"     -> "chat.general"
"user.{{ auth.sub }}"                -> "user.abc-123"
"doc.{{ request.params.doc_id }}"    -> "doc.doc-456"
```

## Wildcard Channel Matching

When sending messages via `ws.send` or `sse.send`, the channel field supports wildcard patterns using `*` as a segment placeholder. Wildcards match on dot-separated segments:

| Pattern | Matches | Does Not Match |
|---------|---------|----------------|
| `chat.*` | `chat.general`, `chat.random` | `chat.a.b` (different segment count) |
| `user.*` | `user.123`, `user.abc` | `user.a.b` |
| `*.notifications` | `user.notifications`, `admin.notifications` | `notifications` |
| `*` | Everything | (matches all channels) |

Wildcard matching requires the pattern and channel to have the same number of dot-separated segments. A `*` in any segment position matches any value in that position.

This is useful for broadcasting to all channels of a type:

```json
{
  "broadcast_all_rooms": {
    "type": "ws.send",
    "config": {
      "channel": "chat.*",
      "data": {
        "type": "system",
        "message": "Server maintenance in 5 minutes"
      }
    }
  }
}
```

## Lifecycle Workflows

Connection lifecycle events trigger workflows with the following input:

| Field | Description |
|-------|-------------|
| `connection_id` | Unique connection ID |
| `channel` | Resolved channel name |
| `endpoint` | Endpoint ID from config |
| `user_id` | Authenticated user ID (empty if no auth) |
| `params` | URL path parameters as a map |
| `data` | Message payload (WebSocket `on_message` only) |

### WebSocket Lifecycle

- `on_connect` -- fires after the WebSocket upgrade completes and the connection is registered.
- `on_message` -- fires for each message received. Messages are processed concurrently (up to 100 by default). If the concurrency limit is reached, messages are dropped with a warning log.
- `on_disconnect` -- fires when the connection closes (clean close or error). Runs after the connection is unregistered from the manager.

### SSE Lifecycle

- `on_connect` -- fires after the SSE response headers are sent and the connection is registered.
- `on_disconnect` -- fires when the client disconnects or the connection is closed server-side.
- SSE has no `on_message` -- it is a server-to-client-only protocol.

## SSE Endpoints

SSE (Server-Sent Events) endpoints push data from the server to the client over a long-lived HTTP connection. Clients connect with a standard `GET` request and receive events as they are sent.

```json
{
  "sync": {
    "pubsub": "redis-pubsub"
  },
  "endpoints": {
    "notifications": {
      "type": "sse",
      "path": "/events/notifications",
      "middleware": ["auth.jwt"],
      "channels": {
        "pattern": "notify.{{ auth.sub }}"
      },
      "heartbeat": "30s",
      "on_connect": "sse-notify-connect",
      "on_disconnect": "sse-notify-disconnect"
    }
  }
}
```

### SSE with Event Types

The `sse.send` node supports an `event` field that maps to the SSE `event:` line. Clients can filter on event types using `EventSource.addEventListener`:

```json
{
  "send_notification": {
    "type": "sse.send",
    "config": {
      "channel": "notify.{{ input.user_id }}",
      "event": "new_message",
      "data": {
        "from": "{{ input.from }}",
        "preview": "{{ input.preview }}"
      }
    }
  }
}
```

The client receives:

```
event: new_message
data: {"from":"alice","preview":"Hey, are you free?"}
```

Different event types can be sent to the same channel, letting the client subscribe to specific categories:

```javascript
const source = new EventSource('/events/notifications');
source.addEventListener('new_message', (e) => { /* handle message */ });
source.addEventListener('status_change', (e) => { /* handle status */ });
```

## Cross-Instance Message Routing

When running multiple Noda instances behind a load balancer, WebSocket and SSE connections are distributed across instances. The `sync.pubsub` service enables message delivery across instances using Redis PubSub.

### How It Works

1. Each instance tracks its own connections in the local connection manager.
2. When a `ws.send` or `sse.send` node fires, the message is delivered locally to matching connections on this instance.
3. The message is also published to Redis PubSub so other instances receive it.
4. Each instance subscribes to PubSub and delivers incoming messages to its local connections.
5. The Redis routing table tracks which channels have connections on which instances, enabling targeted delivery -- only instances holding relevant connections receive the message.

### Configuration

Cross-instance sync requires a PubSub service in `noda.json`:

```json
{
  "services": {
    "redis-pubsub": {
      "plugin": "pubsub",
      "config": {
        "url": "redis://localhost:6379"
      }
    }
  }
}
```

Then reference it in the connections config:

```json
{
  "sync": {
    "pubsub": "redis-pubsub"
  }
}
```

## Service Wiring in Connection Handlers

Connection lifecycle workflows use the same service registry as route-triggered workflows. Services defined in `noda.json` (databases, caches, etc.) are available to all lifecycle workflows.

A common pattern is using the cache service for presence tracking and the database for persistent state:

```json
{
  "add_presence": {
    "type": "cache.set",
    "config": {
      "service": "app-cache",
      "key": "presence:{{ input.channel }}:{{ input.user_id }}",
      "value": {
        "user_id": "{{ input.user_id }}",
        "connected_at": "{{ $now() }}"
      },
      "ttl": "60s"
    }
  }
}
```

## Complete Example: Chat Room with Presence

This example shows a full chat system with join/leave notifications and presence tracking.

### connections/chat.json

```json
{
  "sync": {
    "pubsub": "redis-pubsub"
  },
  "endpoints": {
    "chat": {
      "type": "websocket",
      "path": "/ws/chat/:room",
      "middleware": ["auth.jwt"],
      "channels": {
        "pattern": "chat.{{ request.params.room }}",
        "max_per_channel": 100
      },
      "ping_interval": "30s",
      "on_connect": "chat-on-connect",
      "on_message": "chat-on-message",
      "on_disconnect": "chat-on-disconnect"
    }
  }
}
```

### workflows/chat-on-connect.json

When a user joins, add them to the presence set and broadcast a join notification:

```json
{
  "id": "chat-on-connect",
  "nodes": {
    "add_presence": {
      "type": "cache.set",
      "config": {
        "service": "app-cache",
        "key": "presence:{{ input.channel }}:{{ input.user_id }}",
        "value": {
          "user_id": "{{ input.user_id }}",
          "joined_at": "{{ $now() }}"
        },
        "ttl": "120s"
      }
    },
    "broadcast_join": {
      "type": "ws.send",
      "config": {
        "channel": "{{ input.channel }}",
        "data": {
          "type": "user_joined",
          "user_id": "{{ input.user_id }}",
          "timestamp": "{{ $now() }}"
        }
      }
    }
  },
  "edges": [
    { "from": "add_presence", "to": "broadcast_join", "output": "success" }
  ]
}
```

### workflows/chat-on-message.json

Route incoming messages by type -- chat messages are stored and broadcast, typing indicators are broadcast only:

```json
{
  "id": "chat-on-message",
  "nodes": {
    "route": {
      "type": "control.switch",
      "config": {
        "value": "{{ input.data.type }}"
      }
    },
    "save_message": {
      "type": "db.create",
      "config": {
        "service": "main-db",
        "table": "messages",
        "data": {
          "id": "{{ $uuid() }}",
          "room": "{{ input.params.room }}",
          "user_id": "{{ input.user_id }}",
          "content": "{{ input.data.content }}",
          "created_at": "{{ $now() }}"
        }
      }
    },
    "broadcast_message": {
      "type": "ws.send",
      "config": {
        "channel": "{{ input.channel }}",
        "data": {
          "type": "message",
          "id": "{{ nodes.save_message.id }}",
          "user_id": "{{ input.user_id }}",
          "content": "{{ input.data.content }}",
          "created_at": "{{ nodes.save_message.created_at }}"
        }
      }
    },
    "broadcast_typing": {
      "type": "ws.send",
      "config": {
        "channel": "{{ input.channel }}",
        "data": {
          "type": "typing",
          "user_id": "{{ input.user_id }}"
        }
      }
    },
    "log_unknown": {
      "type": "util.log",
      "config": {
        "message": "Unknown message type: {{ input.data.type }}"
      }
    }
  },
  "edges": [
    { "from": "route", "to": "save_message", "output": "message" },
    { "from": "route", "to": "broadcast_typing", "output": "typing" },
    { "from": "route", "to": "log_unknown", "output": "default" },
    { "from": "save_message", "to": "broadcast_message", "output": "success" }
  ]
}
```

### workflows/chat-on-disconnect.json

Remove the user from presence and broadcast a leave notification:

```json
{
  "id": "chat-on-disconnect",
  "nodes": {
    "remove_presence": {
      "type": "cache.del",
      "config": {
        "service": "app-cache",
        "key": "presence:{{ input.channel }}:{{ input.user_id }}"
      }
    },
    "broadcast_leave": {
      "type": "ws.send",
      "config": {
        "channel": "{{ input.channel }}",
        "data": {
          "type": "user_left",
          "user_id": "{{ input.user_id }}",
          "timestamp": "{{ $now() }}"
        }
      }
    }
  },
  "edges": [
    { "from": "remove_presence", "to": "broadcast_leave", "output": "success" }
  ]
}
```

### Presence with TTL

Presence entries use a TTL (e.g. 120 seconds). The WebSocket ping interval (30 seconds by default) keeps the connection alive, and a workflow can refresh the presence TTL on each ping or message. If an instance crashes without sending disconnect events, the TTL expires and the user automatically disappears from presence. To read all users in a room, query cache keys with the prefix `presence:<channel>:`.
