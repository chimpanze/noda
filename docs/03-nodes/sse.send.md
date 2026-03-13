# sse.send

Sends a Server-Sent Event to a channel.

## Config

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `channel` | string (expr) | yes | Channel name (supports wildcards) |
| `data` | any (expr) | yes | Event data |
| `event` | string (expr) | no | Event type name |
| `id` | string (expr) | no | Event ID |

## Outputs

`success`, `error`

## Behavior

Resolves all fields. Calls `services["connections"].SendSSE(channel, event, data, id)`. Buffered, non-blocking. Fires `success` with no data.

## Service Dependencies

| Slot | Prefix | Required |
|------|--------|----------|
| `connections` | `sse` | Yes |

## Example

```json
{
  "type": "sse.send",
  "services": { "connections": "notifications" },
  "config": {
    "channel": "{{ 'user.' + auth.user_id }}",
    "event": "notification",
    "data": {
      "title": "{{ input.title }}",
      "body": "{{ input.body }}"
    }
  }
}
```
