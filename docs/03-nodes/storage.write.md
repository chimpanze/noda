# storage.write

Writes data to storage.

## Config

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `path` | string (expr) | yes | File path |
| `data` | string/bytes (expr) | yes | Data to write |
| `content_type` | string (expr) | no | MIME type |

## Outputs

`success`, `error`

## Behavior

Writes the resolved `data` to the configured storage service at the given `path`. Fires `success` on completion.

## Service Dependencies

| Slot | Prefix | Required |
|------|--------|----------|
| `storage` | `storage` | Yes |

## Example

```json
{
  "type": "storage.write",
  "services": { "storage": "files" },
  "config": {
    "path": "{{ 'exports/' + $uuid() + '.json' }}",
    "data": "{{ nodes.generate }}",
    "content_type": "application/json"
  }
}
```

### With data flow

A report generation workflow queries data, then writes the result as a JSON file to storage.

```json
{
  "fetch_data": {
    "type": "db.find",
    "services": { "database": "postgres" },
    "config": {
      "table": "orders",
      "where": { "status": "completed" },
      "select": ["id", "total", "created_at"]
    }
  },
  "save_report": {
    "type": "storage.write",
    "services": { "storage": "files" },
    "config": {
      "path": "{{ 'reports/' + $uuid() + '.json' }}",
      "data": "{{ nodes.fetch_data }}",
      "content_type": "application/json"
    }
  }
}
```

Output stored as `nodes.save_report`:
```json
{ "path": "reports/a1b2c3d4.json", "size": 10240 }
```

Downstream nodes access the written file path via `nodes.save_report.path`.
