# storage.read

Reads a file from storage.

## Config

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `path` | string (expr) | yes | File path |

## Outputs

`success`, `error`

Output: `{data, size, content_type}`. Fires `error` with `NotFoundError` if the path doesn't exist.

## Behavior

Reads the file at `path` from the configured storage service. Returns the file contents along with metadata. Fires `error` with `NotFoundError` if the path doesn't exist.

## Service Dependencies

| Slot | Prefix | Required |
|------|--------|----------|
| `storage` | `storage` | Yes |

## Example

```json
{
  "type": "storage.read",
  "services": { "storage": "files" },
  "config": {
    "path": "{{ 'documents/' + input.file_id }}"
  }
}
```
