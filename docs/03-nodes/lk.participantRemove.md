# lk.participantRemove

Removes a participant from a LiveKit room.

## Config

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `room` | string (expr) | yes | Room name |
| `identity` | string (expr) | yes | Participant identity |

## Outputs

`success`, `error`

Output: `{removed: true}`

## Behavior

Disconnects the specified participant from the room. The participant's client receives a disconnection event. Fires `success` on completion.

## Service Dependencies

| Slot | Prefix | Required |
|------|--------|----------|
| `livekit` | `lk` | Yes |

## Example

```json
{
  "type": "lk.participantRemove",
  "services": { "livekit": "lk" },
  "config": {
    "room": "{{ input.room_name }}",
    "identity": "{{ input.user_id }}"
  }
}
```

### With data flow

A kick-participant endpoint looks up the room from the database, removes the user, and logs the action.

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
  "kick": {
    "type": "lk.participantRemove",
    "services": { "livekit": "lk" },
    "config": {
      "room": "{{ nodes.get_meeting.room_name }}",
      "identity": "{{ input.user_id }}"
    }
  },
  "log_action": {
    "type": "util.log",
    "config": {
      "message": "{{ 'Removed ' + input.user_id + ' from ' + nodes.get_meeting.room_name }}"
    }
  }
}
```

Output stored as `nodes.kick`:
```json
{ "removed": true }
```

Downstream nodes can check `nodes.kick.removed` to confirm the participant was disconnected.
