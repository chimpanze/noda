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

### With data flow

A document download endpoint looks up the file record in the database, then reads the file from storage using the stored path.

```json
{
  "get_record": {
    "type": "db.findOne",
    "services": { "database": "postgres" },
    "config": {
      "table": "documents",
      "where": { "id": "{{ input.doc_id }}" },
      "required": true
    }
  },
  "read_file": {
    "type": "storage.read",
    "services": { "storage": "files" },
    "config": {
      "path": "{{ nodes.get_record.storage_path }}"
    }
  }
}
```

Output stored as `nodes.read_file`:
```json
{ "data": "<file contents>", "size": 24576, "content_type": "application/pdf" }
```

Downstream nodes access the file data via `nodes.read_file.data` or check `nodes.read_file.content_type`.
