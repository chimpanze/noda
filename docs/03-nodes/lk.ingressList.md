# lk.ingressList

Lists ingress endpoints.

## Config

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `room` | string (expr) | no | Optional room name filter |

## Outputs

`success`, `error`

Output: `{items: [...]}`

Each item contains `ingress_id`, `url`, `stream_key`, `room`, `participant_identity`, `participant_name`, `input_type`.

## Behavior

Lists all ingress endpoints. If `room` is provided, only ingress endpoints for that room are returned. Fires `success` with the items array.

## Service Dependencies

| Slot | Prefix | Required |
|------|--------|----------|
| `livekit` | `lk` | Yes |

## Example

```json
{
  "type": "lk.ingressList",
  "services": { "livekit": "lk" },
  "config": {
    "room": "{{ input.room_name }}"
  }
}
```
