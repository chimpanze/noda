# transform.delete

Removes fields from an object.

## Config

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `data` | string (expr) | yes | Expression resolving to object |
| `fields` | array of strings | yes | Field names to remove |

## Outputs

`success`, `error`

## Behavior

Resolves `data` to an object. Returns a copy with the named fields removed. Does not error if a field doesn't exist.

## Example

```json
{
  "type": "transform.delete",
  "config": {
    "data": "{{ nodes.fetch[0] }}",
    "fields": ["password", "internal_notes"]
  }
}
```
