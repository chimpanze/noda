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

Reads the file stream from the trigger input (marked via the `files` array on the trigger config). The file is read fully into memory (bounded by the size limit), then the MIME type is detected from the content and validated, then the buffer is written to the destination storage service. Size your `max_size` (and the server `body_limit`) with this in-memory buffering in mind. Fires `success` with file metadata. Fires `error` with a `ValidationError` if size or type constraints are violated.

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
    "path": "{{ 'avatars/' + auth.sub + '/' + $uuid() }}",
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

## Path validation

`storage_path` (resolved from the `path` config field) must be a relative
path within the destination storage root. The following are rejected at
node execution:

- empty path
- absolute paths (e.g. `/etc/passwd`)
- paths containing NUL bytes
- paths whose `filepath.Clean` form starts with `..` (i.e. would escape upward)

These checks happen before any byte is written. The destination storage
service applies the same checks again as defence-in-depth.
