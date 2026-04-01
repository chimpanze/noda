# lk.participantList

Lists participants in a LiveKit room.

## Config

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `room` | string (expr) | yes | Room name |

## Outputs

`success`, `error`

Output: `{participants: [...]}`

Each participant object contains `sid`, `identity`, `name`, `metadata`, `state`, `joined_at`, `region`.

## Behavior

Retrieves all participants currently connected to the specified room. Fires `success` with the participants array.

## Service Dependencies

| Slot | Prefix | Required |
|------|--------|----------|
| `livekit` | `lk` | Yes |

## Example

```json
{
  "type": "lk.participantList",
  "services": { "livekit": "lk" },
  "config": {
    "room": "{{ input.room_name }}"
  }
}
```

### With data flow

A meeting details endpoint fetches the room from the database, then lists all connected participants.

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
  "list_participants": {
    "type": "lk.participantList",
    "services": { "livekit": "lk" },
    "config": {
      "room": "{{ nodes.get_meeting.room_name }}"
    }
  }
}
```

Output stored as `nodes.list_participants`:
```json
{ "participants": [{ "sid": "PA_abc", "identity": "usr_42", "name": "Jane", "state": "ACTIVE", "joined_at": 1717200000 }] }
```

Downstream nodes access the list via `nodes.list_participants.participants`.
