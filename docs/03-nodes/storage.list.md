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
    "prefix": "{{ 'users/' + auth.sub + '/documents/' }}"
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
      "prefix": "{{ 'users/' + auth.sub + '/' + input.folder + '/' }}"
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

## Path constraints

- Paths must be relative (no leading `/`).
- Paths must not contain `..` segments that escape the storage root.
- Paths must not contain NUL bytes.

For the `local` backend, the configured root directory must be a real
directory — not a symlink. This is enforced at service creation. Admins
should not create symlinks under the storage root either.
