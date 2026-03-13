# response.error

Builds a standardized error response.

## Config

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `status` | integer/expr | yes | HTTP status code (default: 500) |
| `code` | string (expr) | yes | Error code |
| `message` | string (expr) | yes | Error message |
| `details` | string (expr) | no | Additional details |

## Outputs

`success`, `error`

## Behavior

Produces an `HTTPResponse` with the body formatted as the standardized error structure: `{ "error": { "code", "message", "details", "trace_id" } }`. The `trace_id` is automatically injected from the execution context.

## Example

```json
{
  "type": "response.error",
  "config": {
    "status": 404,
    "code": "NOT_FOUND",
    "message": "Task not found"
  }
}
```
