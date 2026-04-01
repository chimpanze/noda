# http.post

Shorthand for POST requests. Same as `http.request` with `method: "POST"`.

## Config

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `url` | string (expr) | yes | Request URL |
| `headers` | object | no | Request headers |
| `body` | any (expr) | yes | Request body |
| `timeout` | string | no | Request timeout |

## Outputs

`success`, `error`

Output: `{status, headers, body}`

## Behavior

Equivalent to `http.request` with `method: "POST"`. Resolves the URL, headers, and body, makes the POST request through the configured HTTP client service, and returns the response.

## Service Dependencies

| Slot | Prefix | Required |
|------|--------|----------|
| `client` | `http` | Yes |

## Output Shape

```json
// success output
{
  "status": 201,
  "headers": {
    "content-type": "application/json",
    "location": "/orders/abc-123"
  },
  "body": {
    "id": "abc-123",
    "created": true
  }
}
```

- `status` -- HTTP status code (integer).
- `headers` -- Response headers with lowercase keys. Single-value headers are strings; multi-value headers are arrays.
- `body` -- Auto-parsed as JSON when the response Content-Type is `application/json` or the body is valid JSON. Otherwise returned as a plain string.

## Error Output

The `error` port fires on connection errors, DNS failures, or timeouts. It does NOT fire for non-2xx status codes -- those still route to `success` with the status code in the output. The error output contains:

```json
{
  "error": "http.request: Post \"https://api.example.com/orders\": dial tcp: connection refused",
  "node_id": "create_order",
  "node_type": "http.post"
}
```

For timeouts, the error message is: `"timeout after 10s: HTTP POST https://api.example.com/orders"`.

## Example

```json
{
  "type": "http.post",
  "services": { "client": "external-api" },
  "config": {
    "url": "/webhooks/notify",
    "body": {
      "event": "order.created",
      "data": "{{ nodes.order }}"
    },
    "timeout": "10s"
  }
}
```

## Examples

### Create a resource and use the response

```json
{
  "create_order": {
    "type": "http.post",
    "services": { "client": "order-service" },
    "config": {
      "url": "/orders",
      "headers": {
        "X-Idempotency-Key": "{{ input.idempotency_key }}"
      },
      "body": {
        "items": "{{ input.items }}",
        "customer_id": "{{ input.customer_id }}"
      },
      "timeout": "10s"
    }
  },
  "respond": {
    "type": "response.json",
    "config": {
      "status": "{{ nodes.create_order.status }}",
      "body": {
        "order_id": "{{ nodes.create_order.body.id }}",
        "status": "{{ nodes.create_order.body.status }}"
      }
    }
  }
}
```

`nodes.create_order.body` contains the parsed JSON response from the external service. `nodes.create_order.status` holds the HTTP status code, which can be forwarded or checked with `control.if`.
