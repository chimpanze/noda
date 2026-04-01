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

### With data flow

After saving a message to the database, broadcast it to all WebSocket clients in the room.

```json
{
  "broadcast_message": {
    "type": "ws.send",
    "services": { "connections": "chat-ws" },
    "config": {
      "channel": "{{ 'chat.' + nodes.save_message.room_id }}",
      "data": {
        "type": "new_message",
        "id": "{{ nodes.save_message.id }}",
        "from": "{{ nodes.save_message.author }}",
        "text": "{{ nodes.save_message.text }}",
        "sent_at": "{{ nodes.save_message.created_at }}"
      }
    }
  }
}
```

When `nodes.save_message` produced `{"id": 9001, "room_id": "general", "author": "alice", "text": "Hello!", "created_at": "2024-01-15T10:30:00Z"}`, the data is sent to channel `chat.general`. Output stored as `nodes.broadcast_message`:
```json
null
```
