# response.redirect

Builds an HTTP redirect response.

## Config

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `url` | string (expr) | yes | Redirect target URL |
| `status` | integer | no | HTTP status code (default: 302) |

## Outputs

`success`, `error`

## Behavior

Produces an `HTTPResponse` with the given status, a `Location` header set to `url`, and an empty body.

URL validation: URLs must start with `/` (relative), `http://`, or `https://` (absolute). Protocol-relative URLs (`//`) are blocked to prevent open redirects. URLs containing carriage returns or newlines are rejected to prevent header injection.

## Example

```json
{
  "type": "response.redirect",
  "config": {
    "url": "{{ '/api/tasks/' + string(nodes.insert.id) }}",
    "status": 301
  }
}
```
