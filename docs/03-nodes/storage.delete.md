# storage.delete

Deletes a file from storage.

## Config

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `path` | string (expr) | yes | File path |

## Outputs

`success`, `error`

## Behavior

Deletes the file at `path` from the configured storage service. Fires `success` on completion.

## Service Dependencies

| Slot | Prefix | Required |
|------|--------|----------|
| `storage` | `storage` | Yes |

## Output Shape

```json
// success output
{}
```

The node returns an empty object on successful deletion.

## Error Output

The `error` port fires if the file does not exist or the delete operation fails (e.g., permission denied). The error output contains:

```json
{
  "error": "file not found",
  "node_id": "delete_avatar",
  "node_type": "storage.delete"
}
```

## Examples

### Delete user avatar after account removal

```json
{
  "delete_account": {
    "type": "db.exec",
    "services": { "database": "postgres" },
    "config": {
      "query": "DELETE FROM users WHERE id = $1",
      "params": ["{{ input.user_id }}"]
    }
  },
  "delete_avatar": {
    "type": "storage.delete",
    "services": { "storage": "uploads" },
    "config": {
      "path": "{{ 'avatars/' + input.user_id + '.png' }}"
    }
  }
}
```

After `delete_account` succeeds, `delete_avatar` removes the associated file from storage. If the file does not exist, the `error` output fires -- wire it to a no-op or log node if the missing file is acceptable.
