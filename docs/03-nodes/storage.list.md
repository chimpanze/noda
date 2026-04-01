# storage.list

Lists files under a prefix.

## Config

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `prefix` | string (expr) | yes | Path prefix |

## Outputs

`success`, `error`

Output: `{paths: [...]}`

## Behavior

Lists all files/objects under the given `prefix` in the configured storage service. Fires `success` with an object containing a `paths` array of matching file paths.

## Service Dependencies

| Slot | Prefix | Required |
|------|--------|----------|
| `storage` | `storage` | Yes |

## Example

```json
{
  "type": "storage.list",
  "services": { "storage": "files" },
  "config": {
    "prefix": "{{ 'users/' + auth.user_id + '/documents/' }}"
  }
}
```

### With data flow

A file browser endpoint lists all files under a user's folder and returns the paths in the response.

```json
{
  "list_files": {
    "type": "storage.list",
    "services": { "storage": "files" },
    "config": {
      "prefix": "{{ 'users/' + auth.user_id + '/' + input.folder + '/' }}"
    }
  },
  "respond": {
    "type": "response.json",
    "config": {
      "body": {
        "folder": "{{ input.folder }}",
        "files": "{{ nodes.list_files.paths }}"
      }
    }
  }
}
```

Output stored as `nodes.list_files`:
```json
{ "paths": ["users/42/photos/a.jpg", "users/42/photos/b.png"] }
```

Downstream nodes access the file list via `nodes.list_files.paths`.
