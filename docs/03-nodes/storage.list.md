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
