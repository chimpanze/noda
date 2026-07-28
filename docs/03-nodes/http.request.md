# http.request

Makes an HTTP request.

## Config

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `method` | string (expr) | yes | HTTP method |
| `url` | string (expr) | yes | Request URL |
| `headers` | object | no | Request headers (expressions) |
| `body` | any (expr) | no | Request body (auto-encodes maps as JSON) |
| `query` | object (expr) | no | Query parameters, URL-encoded and appended to `url`. Accepts an object or an expression resolving to one. |
| `timeout` | string | no | Per-request timeout override |

### Query parameters

`query` takes an object whose entries are URL-encoded and appended to `url`:

```json
{
  "type": "http.request",
  "services": { "client": "inventory" },
  "config": {
    "method": "GET",
    "url": "/items",
    "query": { "limit": "{{ input.limit }}", "sort": "name" }
  }
}
```

Rules:

- Values are stringified. Arrays become repeated parameters (`tag=new&tag=sale`).
- A value that resolves to null **drops the parameter entirely**, so an optional
  parameter needs no conditional.
- Nested objects and nested arrays (an array inside an array) are rejected.
- If `url` already contains a query string, setting a non-empty `query` is an
  error rather than a merge — use one or the other.
- Parameters are emitted sorted by key, and spaces encode as `+`.

## Outputs

`success`, `error`

Output: `{status, headers, body}`

## Behavior

Resolves all config fields and makes an outbound HTTP request using the configured client service. The `url` is relative to the client's base URL. Maps are automatically JSON-encoded as the request body, with expression templates nested inside the map or slice (e.g. `{{ ... }}` strings in nested fields) deep-resolved before encoding. Returns the response status code, headers, and parsed body.

Response bodies are limited to 100 MB. Responses exceeding this limit produce an error.

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
      "X-Webhook-Secret": "{{ secrets.WEBHOOK_SECRET }}"
    },
    "body": {
      "event": "order.created",
      "data": "{{ nodes.order }}"
    },
    "timeout": "10s"
  }
}
```

## Runnable example

A runnable, CI-verified example of this node lives in the cookbook:
[`examples/node-cookbook/http`](../../examples/node-cookbook/http/README.md) — its README documents the exact request/response pair the integration suite executes.
