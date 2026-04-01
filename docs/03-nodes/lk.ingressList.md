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

### With data flow

A stream management endpoint lists all ingress endpoints for a room and returns them in the response.

```json
{
  "list_ingress": {
    "type": "lk.ingressList",
    "services": { "livekit": "lk" },
    "config": {
      "room": "{{ input.room_name }}"
    }
  },
  "respond": {
    "type": "response.json",
    "config": {
      "body": {
        "room": "{{ input.room_name }}",
        "streams": "{{ nodes.list_ingress.items }}"
      }
    }
  }
}
```

Output stored as `nodes.list_ingress`:
```json
{ "items": [{ "ingress_id": "IN_abc", "url": "rtmp://livekit.example.com/live", "stream_key": "sk_xyz", "room": "stream-1", "input_type": "rtmp" }] }
```

Downstream nodes access the list via `nodes.list_ingress.items`.
