# lk.sendData

Sends data to participants in a LiveKit room.

## Config

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `room` | string (expr) | yes | Room name |
| `data` | any (expr) | yes | Data to send (string or object, serialized as JSON) |
| `kind` | string (expr) | no | Delivery kind: `"reliable"` or `"lossy"` (default: `"reliable"`) |
| `destination_identities` | array | no | Target participant identities (omit for broadcast) |
| `topic` | string (expr) | no | Topic label for the data message |

## Outputs

`success`, `error`

Output: `{sent: true}`

## Behavior

Sends a data message to participants in the room via LiveKit's data channel. Use `"reliable"` (default) for ordered delivery, or `"lossy"` for low-latency unordered delivery. If `destination_identities` is set, only those participants receive the message. Otherwise it broadcasts to all.

## Service Dependencies

| Slot | Prefix | Required |
|------|--------|----------|
| `livekit` | `lk` | Yes |

## Example

```json
{
  "type": "lk.sendData",
  "services": { "livekit": "lk" },
  "config": {
    "room": "{{ input.room_name }}",
    "data": {
      "type": "chat",
      "message": "{{ input.message }}",
      "sender": "{{ auth.user_id }}"
    },
    "kind": "reliable",
    "topic": "chat"
  }
}
```

### With data flow

A chat message endpoint saves the message to the database, then broadcasts it to all participants in the room.

```json
{
  "save_message": {
    "type": "db.create",
    "services": { "database": "postgres" },
    "config": {
      "table": "messages",
      "data": {
        "room": "{{ input.room_name }}",
        "sender_id": "{{ auth.user_id }}",
        "text": "{{ input.message }}"
      }
    }
  },
  "broadcast": {
    "type": "lk.sendData",
    "services": { "livekit": "lk" },
    "config": {
      "room": "{{ input.room_name }}",
      "data": {
        "type": "chat",
        "id": "{{ nodes.save_message.id }}",
        "message": "{{ input.message }}",
        "sender": "{{ auth.user_id }}"
      },
      "kind": "reliable",
      "topic": "chat"
    }
  }
}
```

Output stored as `nodes.broadcast`:
```json
{ "sent": true }
```

Downstream nodes can check `nodes.broadcast.sent` to confirm delivery.
