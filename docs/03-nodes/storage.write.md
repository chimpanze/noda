# storage.write

Writes data to storage.

## Config

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `path` | string (expr) | yes | File path |
| `data` | string/bytes (expr) | yes | Data to write |
| `content_type` | string (expr) | no | MIME type |

## Outputs

`success`, `error`

## Behavior

Writes the resolved `data` to the configured storage service at the given `path`. Fires `success` on completion.

## Service Dependencies

| Slot | Prefix | Required |
|------|--------|----------|
| `storage` | `storage` | Yes |

## Example

```json
{
  "type": "storage.write",
  "services": { "storage": "files" },
  "config": {
    "path": "{{ 'exports/' + $uuid() + '.json' }}",
    "data": "{{ nodes.generate }}",
    "content_type": "application/json"
  }
}
```
