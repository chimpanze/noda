# lk.participantGet

Gets a participant by identity from a LiveKit room.

## Config

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `room` | string (expr) | yes | Room name |
| `identity` | string (expr) | yes | Participant identity |

## Outputs

`success`, `error`

Output: participant object with `sid`, `identity`, `name`, `metadata`, `state`, `joined_at`, `region`.

## Behavior

Retrieves a single participant from the room by identity. Fires `success` with the participant object. Fires `error` if the participant is not found.

## Service Dependencies

| Slot | Prefix | Required |
|------|--------|----------|
| `livekit` | `lk` | Yes |

## Example

```json
{
  "type": "lk.participantGet",
  "services": { "livekit": "lk" },
  "config": {
    "room": "{{ input.room_name }}",
    "identity": "{{ input.user_id }}"
  }
}
```

### With data flow

A participant status endpoint fetches a participant from the room and returns their connection state.

```json
{
  "get_participant": {
    "type": "lk.participantGet",
    "services": { "livekit": "lk" },
    "config": {
      "room": "{{ input.room_name }}",
      "identity": "{{ input.user_id }}"
    }
  },
  "respond": {
    "type": "response.json",
    "config": {
      "body": {
        "identity": "{{ nodes.get_participant.identity }}",
        "name": "{{ nodes.get_participant.name }}",
        "state": "{{ nodes.get_participant.state }}",
        "joined_at": "{{ nodes.get_participant.joined_at }}"
      }
    }
  }
}
```

Output stored as `nodes.get_participant`:
```json
{ "sid": "PA_abc", "identity": "usr_42", "name": "Jane", "metadata": "{}", "state": "ACTIVE", "joined_at": 1717200000, "region": "us-east-1" }
```

Downstream nodes access fields via `nodes.get_participant.name` or `nodes.get_participant.state`.
