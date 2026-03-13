# Connections

Files in `connections/*.json`. Defines WebSocket and SSE endpoints.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `sync` | object | no | Cross-instance sync service |
| `endpoints` | object | yes | Map of endpoint ID to endpoint definition |

## Endpoint Definition

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `type` | string | yes | `"websocket"` or `"sse"` |
| `path` | string | yes | URL path (supports `:param`) |
| `middleware` | array | no | Endpoint middleware |
| `channels` | object | no | Channel configuration |
| `channels.pattern` | string | no | Channel name pattern (expression) |
| `channels.max_per_channel` | integer | no | Max connections per channel |
| `ping_interval` | string | no | WebSocket ping interval |
| `on_connect` | string | no | Workflow ID on connection |
| `on_message` | string | no | Workflow ID on message |
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
