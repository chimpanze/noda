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
