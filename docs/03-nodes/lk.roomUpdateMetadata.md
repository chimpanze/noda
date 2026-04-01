# lk.roomUpdateMetadata

Updates metadata on a LiveKit room.

## Config

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `room` | string (expr) | yes | Room name |
| `metadata` | string (expr) | yes | New metadata value |

## Outputs

`success`, `error`

Output: updated room object.

## Behavior

Replaces the metadata on the specified room. All participants receive a metadata update event. Fires `success` with the updated room object.

## Service Dependencies

| Slot | Prefix | Required |
|------|--------|----------|
| `livekit` | `lk` | Yes |

## Example

```json
{
  "type": "lk.roomUpdateMetadata",
  "services": { "livekit": "lk" },
  "config": {
    "room": "{{ input.room_name }}",
    "metadata": "{{ toJSON(input.room_settings) }}"
  }
}
```

### With data flow

A room settings update endpoint merges new settings into the existing metadata and updates the room.

```json
{
  "get_meeting": {
    "type": "db.findOne",
    "services": { "database": "postgres" },
    "config": {
      "table": "meetings",
      "where": { "id": "{{ input.meeting_id }}" },
      "required": true
    }
  },
  "update_meta": {
    "type": "lk.roomUpdateMetadata",
    "services": { "livekit": "lk" },
    "config": {
      "room": "{{ nodes.get_meeting.room_name }}",
      "metadata": "{{ toJSON(input.settings) }}"
    }
  }
}
```

Output stored as `nodes.update_meta`:
```json
{ "sid": "RM_abc123", "name": "meeting-d4e5f6", "metadata": "{\"recording\":true}", "num_participants": 4 }
```

Downstream nodes access the updated room via `nodes.update_meta.metadata`.
