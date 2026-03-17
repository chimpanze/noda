# lk.roomUpdateMetadata

Updates metadata on a LiveKit room.

## Config

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `room` | string (expr) | yes | Room name |
| `metadata` | string (expr) | yes | New metadata value |

## Outputs

`success`, `error`

Output: updated room object.

## Behavior

Replaces the metadata on the specified room. All participants receive a metadata update event. Fires `success` with the updated room object.

## Service Dependencies

| Slot | Prefix | Required |
|------|--------|----------|
| `livekit` | `lk` | Yes |

## Example

```json
{
  "type": "lk.roomUpdateMetadata",
  "services": { "livekit": "lk" },
  "config": {
    "room": "{{ input.room_name }}",
    "metadata": "{{ toJSON(input.room_settings) }}"
  }
}
```
