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
