# upload.handle

Handles multipart file uploads with validation.

## Config

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `max_size` | integer | yes | Max file size in bytes |
| `allowed_types` | array | yes | MIME type patterns (supports wildcards) |
| `path` | string (expr) | yes | Storage destination path |
| `max_files` | integer | no | Max files (default: 1) |
| `field` | string | no | Form field name (default: `"file"`) |

## Outputs

`success`, `error`

Single file output: `{path, size, content_type, filename}`. Multiple files output: `{files: [...]}`.

## Behavior

Reads the file stream from the trigger input (marked via the `files` array on the trigger config). Validates file size and MIME type before fully consuming the stream. Streams the file directly to the destination storage service -- no full in-memory buffering. Fires `success` with file metadata. Fires `error` with a `ValidationError` if size or type constraints are violated.

## Service Dependencies

| Slot | Prefix | Required |
|------|--------|----------|
| `destination` | `storage` | Yes |

## Example

```json
{
  "type": "upload.handle",
  "services": { "destination": "files" },
  "config": {
    "max_size": 5242880,
    "allowed_types": ["image/*"],
    "path": "{{ 'avatars/' + auth.user_id + '/' + $uuid() }}",
    "max_files": 1
  }
}
```
