# event.emit

Publishes an event to a stream or pub/sub channel.

## Config

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `mode` | string | yes | `"stream"` or `"pubsub"` (static) |
| `topic` | string (expr) | yes | Topic or stream name |
| `payload` | any (expr) | yes | Event payload |

## Outputs

`success`, `error`

Output: `{message_id}` for stream mode, `{ok: true}` for pubsub mode.

## Behavior

Resolves `topic` and `payload`. Publishes the event to the service matching the configured `mode`. Stream = durable (consumed by workers). PubSub = real-time fan-out. Fires `success` after the event is accepted.

## Service Dependencies

| Slot | Prefix | Required |
|------|--------|----------|
| `stream` | `stream` | Only when mode = `"stream"` |
| `pubsub` | `pubsub` | Only when mode = `"pubsub"` |

## Example

```json
{
  "type": "event.emit",
  "services": { "stream": "redis-stream" },
  "config": {
    "mode": "stream",
    "topic": "orders.created",
    "payload": {
      "order_id": "{{ nodes.insert.id }}",
      "user_id": "{{ auth.user_id }}"
    }
  }
}
```
