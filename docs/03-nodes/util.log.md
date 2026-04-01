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

### With data flow

Log the result of a payment processing step for audit purposes.

```json
{
  "log_payment": {
    "type": "util.log",
    "config": {
      "level": "info",
      "message": "Payment processed for order {{ nodes.charge.order_id }}",
      "fields": {
        "amount": "{{ nodes.charge.amount }}",
        "currency": "{{ nodes.charge.currency }}",
        "transaction_id": "{{ nodes.charge.transaction_id }}"
      }
    }
  }
}
```

When `nodes.charge` produced `{"order_id": 42, "amount": 59.99, "currency": "USD", "transaction_id": "txn_abc123"}`, the log entry reads: `Payment processed for order 42` with structured fields attached. Output stored as `nodes.log_payment`:
```json
null
```

The node produces no output data; downstream nodes typically do not reference it.
