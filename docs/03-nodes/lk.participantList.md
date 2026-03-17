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
