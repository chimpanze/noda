# lk.roomCreate

Creates a LiveKit room.

## Config

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string (expr) | yes | Room name |
| `empty_timeout` | integer (expr) | no | Seconds before an empty room is closed |
| `max_participants` | integer (expr) | no | Maximum number of participants |
| `metadata` | string (expr) | no | Room metadata (JSON string) |

## Outputs

`success`, `error`

Output: room object with `sid`, `name`, `empty_timeout`, `max_participants`, `metadata`, `num_participants`, `creation_time`, `active_recording`.

## Behavior

Creates a new room on the LiveKit server. If a room with the same name already exists, returns the existing room. Fires `success` with the room object.

## Service Dependencies

| Slot | Prefix | Required |
|------|--------|----------|
| `livekit` | `lk` | Yes |

## Example

```json
{
  "type": "lk.roomCreate",
  "services": { "livekit": "lk" },
  "config": {
    "name": "{{ input.room_name }}",
    "empty_timeout": 300,
    "max_participants": 10,
    "metadata": "{{ input.room_metadata }}"
  }
}
```

### With data flow

A meeting creation endpoint creates a LiveKit room and stores the room SID in the database.

```json
{
  "create_room": {
    "type": "lk.roomCreate",
    "services": { "livekit": "lk" },
    "config": {
      "name": "{{ 'meeting-' + $uuid() }}",
      "empty_timeout": 600,
      "max_participants": "{{ input.max_participants }}",
      "metadata": "{{ toJSON({host: auth.user_id}) }}"
    }
  },
  "save_meeting": {
    "type": "db.create",
    "services": { "database": "postgres" },
    "config": {
      "table": "meetings",
      "data": {
        "room_sid": "{{ nodes.create_room.sid }}",
        "room_name": "{{ nodes.create_room.name }}",
        "host_id": "{{ auth.user_id }}"
      }
    }
  }
}
```

Output stored as `nodes.create_room`:
```json
{ "sid": "RM_abc123", "name": "meeting-d4e5f6", "empty_timeout": 600, "max_participants": 10, "metadata": "{\"host\":\"usr_42\"}", "num_participants": 0, "creation_time": 1717200000, "active_recording": false }
```

Downstream nodes access the room via `nodes.create_room.sid` or `nodes.create_room.name`.
