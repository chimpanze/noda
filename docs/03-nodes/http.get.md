# http.get

Shorthand for GET requests. Same as `http.request` with `method: "GET"`.

## Config

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `url` | string (expr) | yes | Request URL |
| `headers` | object | no | Request headers |
| `timeout` | string | no | Request timeout |

## Outputs

`success`, `error`

Output: `{status, headers, body}`

## Behavior

Equivalent to `http.request` with `method: "GET"` and no body. Resolves the URL and headers, makes the GET request through the configured HTTP client service, and returns the response.

## Service Dependencies

| Slot | Prefix | Required |
|------|--------|----------|
| `client` | `http` | Yes |

## Example

```json
{
  "type": "http.get",
  "services": { "client": "external-api" },
  "config": {
    "url": "/users/{{ input.user_id }}",
    "timeout": "5s"
  }
}
```
