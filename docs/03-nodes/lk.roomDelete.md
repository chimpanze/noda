# lk.roomDelete

Deletes a LiveKit room.

## Config

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `room` | string (expr) | yes | Room name to delete |

## Outputs

`success`, `error`

Output: `{deleted: true}`

## Behavior

Deletes the specified room. All participants are disconnected and any active egress is stopped. Fires `success` on completion.

## Service Dependencies

| Slot | Prefix | Required |
|------|--------|----------|
| `livekit` | `lk` | Yes |

## Example

```json
{
  "type": "lk.roomDelete",
  "services": { "livekit": "lk" },
  "config": {
    "room": "{{ input.room_name }}"
  }
}
```

### With data flow

An end-meeting endpoint looks up the room name from the database, deletes the LiveKit room, then marks the meeting as ended.

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
  "delete_room": {
    "type": "lk.roomDelete",
    "services": { "livekit": "lk" },
    "config": {
      "room": "{{ nodes.get_meeting.room_name }}"
    }
  },
  "mark_ended": {
    "type": "db.update",
    "services": { "database": "postgres" },
    "config": {
      "table": "meetings",
      "where": { "id": "{{ input.meeting_id }}" },
      "data": { "ended_at": "{{ $timestamp() }}" }
    }
  }
}
```

Output stored as `nodes.delete_room`:
```json
{ "deleted": true }
```

Downstream nodes can check `nodes.delete_room.deleted` to confirm the room was removed.
