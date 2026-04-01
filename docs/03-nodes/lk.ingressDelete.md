# lk.ingressDelete

Deletes an ingress endpoint.

## Config

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `ingress_id` | string (expr) | yes | Ingress ID to delete |

## Outputs

`success`, `error`

Output: `{deleted: true}`

## Behavior

Deletes the specified ingress endpoint. Any active stream using this ingress is stopped. Fires `success` on completion.

## Service Dependencies

| Slot | Prefix | Required |
|------|--------|----------|
| `livekit` | `lk` | Yes |

## Example

```json
{
  "type": "lk.ingressDelete",
  "services": { "livekit": "lk" },
  "config": {
    "ingress_id": "{{ input.ingress_id }}"
  }
}
```

### With data flow

A stop-stream endpoint looks up the ingress ID from the database and deletes the ingress endpoint.

```json
{
  "get_stream": {
    "type": "db.findOne",
    "services": { "database": "postgres" },
    "config": {
      "table": "streams",
      "where": { "id": "{{ input.stream_id }}" },
      "required": true
    }
  },
  "delete_ingress": {
    "type": "lk.ingressDelete",
    "services": { "livekit": "lk" },
    "config": {
      "ingress_id": "{{ nodes.get_stream.ingress_id }}"
    }
  }
}
```

Output stored as `nodes.delete_ingress`:
```json
{ "deleted": true }
```

Downstream nodes can check `nodes.delete_ingress.deleted` to confirm the ingress was removed.
