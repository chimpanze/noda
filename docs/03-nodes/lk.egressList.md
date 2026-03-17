# lk.egressList

Lists egress recordings.

## Config

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `room` | string (expr) | no | Optional room name filter |

## Outputs

`success`, `error`

Output: `{items: [...]}`

Each item contains `egress_id`, `room_id`, `room_name`, `status`, `started_at`, `ended_at`.

## Behavior

Lists all egress recordings. If `room` is provided, only egress recordings for that room are returned. Fires `success` with the items array.

## Service Dependencies

| Slot | Prefix | Required |
|------|--------|----------|
| `livekit` | `lk` | Yes |

## Example

```json
{
  "type": "lk.egressList",
  "services": { "livekit": "lk" },
  "config": {
    "room": "{{ input.room_name }}"
  }
}
```
