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

### With data flow

Handle a document upload and store the resulting file metadata in the database.

```json
{
  "handle_upload": {
    "type": "upload.handle",
    "services": { "destination": "documents-storage" },
    "config": {
      "max_size": 10485760,
      "allowed_types": ["application/pdf", "image/*"],
      "path": "{{ 'uploads/' + nodes.gen_id + '/' + trigger.filename }}",
      "max_files": 1
    }
  }
}
```

When `nodes.gen_id` produced `"a1b2c3d4"` and the uploaded file is `report.pdf` (2 MB), the file is stored at `uploads/a1b2c3d4/report.pdf`. Output stored as `nodes.handle_upload`:
```json
{
  "path": "uploads/a1b2c3d4/report.pdf",
  "size": 2097152,
  "content_type": "application/pdf",
  "filename": "report.pdf"
}
```

A downstream node can save this metadata:
```json
{
  "save_file_record": {
    "type": "db.insert",
    "config": {
      "table": "files",
      "data": {
        "path": "{{ nodes.handle_upload.path }}",
        "size": "{{ nodes.handle_upload.size }}",
        "type": "{{ nodes.handle_upload.content_type }}"
      }
    }
  }
}
```
