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
