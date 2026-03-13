# util.log

Logs a structured message.

## Config

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `level` | string | yes | `"debug"`, `"info"`, `"warn"`, `"error"` (static) |
| `message` | string (expr) | yes | Log message |
| `fields` | object | no | Additional structured fields (expressions) |

## Outputs

`success`, `error`

## Behavior

Resolves `message` and `fields`. Writes a structured log entry through Noda's logging pipeline. In dev mode, appears in the live trace. In production, routed via slog to OpenTelemetry. Fires `success` with no data.

## Example

```json
{
  "type": "util.log",
  "config": {
    "level": "info",
    "message": "Order created: {{ nodes.insert.id }}",
    "fields": {
      "user_id": "{{ auth.user_id }}",
      "total": "{{ input.total }}"
    }
  }
}
```
