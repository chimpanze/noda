# response.file

Sends a binary file response.

## Config

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `data` | bytes (expr) | yes | File data — typically the `data` field of a `storage.read` output |
| `content_type` | string (expr) | yes | MIME type for the `Content-Type` header |
| `status` | integer | no | HTTP status code (default: 200) |
| `filename` | string (expr) | no | When set, adds `Content-Disposition: attachment; filename="..."` |

## Outputs

`success`, `error`

## Behavior

Produces an `HTTPResponse` with the given status, a `Content-Type` header, and the raw bytes as the body. `data` must resolve to bytes (or a string, which is sent as-is) — pass `{{ nodes.<read_node>.data }}` from a `storage.read` node.

Filename validation: a `filename` containing carriage returns, newlines, or double quotes is rejected to prevent header injection.

## Example

```json
{
  "type": "response.file",
  "config": {
    "data": "{{ nodes.read_report.data }}",
    "content_type": "application/pdf",
    "filename": "report.pdf"
  }
}
```

### With data flow

Serve a stored file: read it from storage, then stream it back with a download filename.

```json
{
  "id": "download-report",
  "nodes": {
    "read_report": {
      "type": "storage.read",
      "services": { "storage": "files" },
      "config": { "path": "{{ 'reports/' + input.report_id + '.pdf' }}" }
    },
    "send_file": {
      "type": "response.file",
      "config": {
        "data": "{{ nodes.read_report.data }}",
        "content_type": "{{ nodes.read_report.content_type }}",
        "filename": "{{ input.report_id + '.pdf' }}"
      }
    }
  },
  "edges": [
    { "from": "read_report", "to": "send_file" }
  ]
}
```
