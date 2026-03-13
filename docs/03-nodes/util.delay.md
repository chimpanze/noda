# util.delay

Pauses execution for a specified duration. Respects context cancellation.

## Config

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `timeout` | string | yes | Duration: `"5s"`, `"100ms"`, `"1m"` |

## Outputs

`success`, `error`

## Behavior

Waits for the specified duration, respecting the `context.Context` deadline. If the context expires before the delay completes, fires `error` with a `TimeoutError`. Otherwise fires `success` with no data.

## Example

```json
{
  "type": "util.delay",
  "config": {
    "timeout": "2s"
  }
}
```
