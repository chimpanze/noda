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
