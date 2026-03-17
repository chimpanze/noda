# lk.roomList

Lists LiveKit rooms.

## Config

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `names` | array | no | Optional room name filter |

## Outputs

`success`, `error`

Output: `{rooms: [...]}`

## Behavior

Lists all active rooms on the LiveKit server. If `names` is provided, only rooms matching those names are returned. Fires `success` with the rooms array.

## Service Dependencies

| Slot | Prefix | Required |
|------|--------|----------|
| `livekit` | `lk` | Yes |

## Example

```json
{
  "type": "lk.roomList",
  "services": { "livekit": "lk" },
  "config": {}
}
```
