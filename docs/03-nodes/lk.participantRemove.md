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
