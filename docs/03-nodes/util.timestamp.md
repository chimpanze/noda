# util.timestamp

Returns the current UTC timestamp.

## Config

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `format` | string | no | `"iso8601"` (default), `"unix"`, `"unix_ms"` |

## Outputs

`success`, `error`

## Behavior

Produces the current time in the requested format. `"iso8601"` returns a string (`"2024-01-15T10:30:00Z"`). `"unix"` returns seconds as integer. `"unix_ms"` returns milliseconds as integer.

## Example

```json
{
  "type": "util.timestamp",
  "config": {
    "format": "unix_ms"
  }
}
```

### With data flow

Generate a timestamp and attach it to a record being inserted.

```json
{
  "now": {
    "type": "util.timestamp",
    "config": {
      "format": "iso8601"
    }
  }
}
```

Output stored as `nodes.now`:
```json
"2024-01-15T10:30:00Z"
```

A downstream node uses the timestamp:
```json
{
  "save_event": {
    "type": "db.insert",
    "config": {
      "table": "audit_log",
      "data": {
        "action": "{{ nodes.parse_action.type }}",
        "user_id": "{{ auth.user_id }}",
        "created_at": "{{ nodes.now }}"
      }
    }
  }
}
```
