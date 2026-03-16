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
