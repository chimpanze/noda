# http.request

Makes an HTTP request.

## Config

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `method` | string (expr) | yes | HTTP method |
| `url` | string (expr) | yes | Request URL |
| `headers` | object | no | Request headers (expressions) |
| `body` | any (expr) | no | Request body (auto-encodes maps as JSON) |
| `timeout` | string | no | Per-request timeout override |

## Outputs

`success`, `error`

Output: `{status, headers, body}`

## Behavior

Resolves all config fields and makes an outbound HTTP request using the configured client service. The `url` is relative to the client's base URL. Maps are automatically JSON-encoded as the request body. Returns the response status code, headers, and parsed body.

## Service Dependencies

| Slot | Prefix | Required |
|------|--------|----------|
| `client` | `http` | Yes |

## Example

```json
{
  "type": "http.request",
  "services": { "client": "external-api" },
  "config": {
    "method": "POST",
    "url": "/webhooks/notify",
    "headers": {
      "X-Webhook-Secret": "{{ $env('WEBHOOK_SECRET') }}"
    },
    "body": {
      "event": "order.created",
      "data": "{{ nodes.order }}"
    },
    "timeout": "10s"
  }
}
```
