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

### With data flow

Wait before retrying a failed external API call. Upstream data passes through unchanged.

```json
{
  "retry_delay": {
    "type": "util.delay",
    "config": {
      "timeout": "3s"
    }
  }
}
```

Output stored as `nodes.retry_delay`:
```json
null
```

The node produces no output data. A downstream retry node references the original failure:
```json
{
  "retry_call": {
    "type": "http.request",
    "config": {
      "method": "POST",
      "url": "{{ nodes.build_request.url }}",
      "body": "{{ nodes.build_request.payload }}"
    }
  }
}
```

## Runnable example

A runnable, CI-verified example of this node lives in the cookbook:
[`examples/node-cookbook/util`](../../examples/node-cookbook/util/README.md) — its README documents the exact request/response pair the integration suite executes.
