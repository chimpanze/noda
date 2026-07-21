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

## Error Output

The `error` port fires with `NotFoundError` when the path doesn't exist. The error output contains:

```json
{
  "code": "NOT_FOUND",
  "error": "storage not found: documents/report.pdf",
  "node_id": "read_file",
  "node_type": "storage.read"
}
```

> **`error` is a diagnostic field.** It may contain driver, network, or filesystem detail such as
> constraint names, internal hostnames, or file paths. Do not forward it to clients — branch on
> `code` instead, and return your own message.

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

## Path constraints

- Paths must be relative (no leading `/`).
- Paths must not contain `..` segments that escape the storage root.
- Paths must not contain NUL bytes.

For the `local` backend, the configured root directory must be a real
directory — not a symlink. This is enforced at service creation. Admins
should not create symlinks under the storage root either.

## Runnable example

A runnable, CI-verified example of this node lives in the cookbook:
[`examples/node-cookbook/storage`](../../examples/node-cookbook/storage/README.md) — its README documents the exact request/response pair the integration suite executes.
