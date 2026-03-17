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
