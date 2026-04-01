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

## Output Shape

```json
// success output
{
  "status": 200,
  "headers": {
    "content-type": "application/json",
    "x-request-id": "abc-123"
  },
  "body": {
    "id": 42,
    "name": "Alice"
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
  "error": "http.request: Get \"https://api.example.com/users/1\": dial tcp: lookup api.example.com: no such host",
  "node_id": "fetch_user",
  "node_type": "http.get"
}
```

For timeouts, the error message is: `"timeout after 5s: HTTP GET https://api.example.com/users/1"`.

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

## Examples

### Fetch and transform external data

```json
{
  "fetch_user": {
    "type": "http.get",
    "services": { "client": "user-service" },
    "config": {
      "url": "/users/{{ input.user_id }}",
      "headers": {
        "Accept": "application/json"
      },
      "timeout": "3s"
    }
  },
  "check_status": {
    "type": "control.if",
    "config": {
      "condition": "{{ nodes.fetch_user.status == 200 }}"
    }
  }
}
```

`nodes.fetch_user.body` contains the parsed JSON response. Use `nodes.fetch_user.status` to branch on HTTP status codes with `control.if` or `control.switch`.
