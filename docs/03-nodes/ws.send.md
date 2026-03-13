# ws.send

Sends data to WebSocket connections on a channel.

## Config

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `channel` | string (expr) | yes | Channel name (supports wildcards) |
| `data` | any (expr) | yes | Data to send |

## Outputs

`success`, `error`

## Behavior

Resolves `channel` and `data`. Calls `services["connections"].Send(channel, data)` on the connection manager. The send is buffered -- it does not block. Fires `success` with no data.

## Service Dependencies

| Slot | Prefix | Required |
|------|--------|----------|
| `connections` | `ws` | Yes |

## Example

```json
{
  "type": "ws.send",
  "services": { "connections": "chat-ws" },
  "config": {
    "channel": "{{ 'chat.' + input.room_id }}",
    "data": {
      "type": "message",
      "from": "{{ auth.user_id }}",
      "text": "{{ input.text }}"
    }
  }
}
```
