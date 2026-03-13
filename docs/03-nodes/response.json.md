# response.json

Builds an HTTP JSON response.

## Config

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `status` | integer/expr | yes | HTTP status code (default: 200) |
| `body` | any | yes | Response body |
| `headers` | object | no | Response headers |
| `cookies` | array | no | Response cookies |

## Outputs

`success`, `error`

## Behavior

Resolves all fields. Produces an `HTTPResponse` object. The trigger layer intercepts this and writes the HTTP response to the client immediately. The node fires `success` after producing the response -- downstream nodes continue executing asynchronously.

## Cookies

The `cookies` field accepts an array of cookie objects. Each cookie supports these fields:

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | yes | Cookie name |
| `value` | string/expr | yes | Cookie value (supports expressions) |
| `path` | string | no | Cookie path (e.g. `"/"`) |
| `domain` | string | no | Cookie domain (e.g. `"example.com"`) |
| `max_age` | number | no | Time to live in seconds |
| `secure` | boolean | no | Send only over HTTPS |
| `http_only` | boolean | no | Prevent JavaScript access |
| `same_site` | string | no | `"Strict"`, `"Lax"`, or `"None"` |

The visual editor provides a dedicated cookie editor with form fields for each property, expression autocomplete on the value field, and support for adding multiple cookies.

## Examples

Basic response:

```json
{
  "type": "response.json",
  "config": {
    "status": 200,
    "body": {
      "data": "{{ nodes.fetch }}",
      "total": "{{ nodes.count.count }}"
    },
    "headers": {
      "X-Request-Id": "{{ trigger.trace_id }}"
    }
  }
}
```

Response with cookies:

```json
{
  "type": "response.json",
  "config": {
    "status": 200,
    "body": { "message": "Logged in" },
    "cookies": [
      {
        "name": "session",
        "value": "{{ nodes.create_session.token }}",
        "path": "/",
        "max_age": 86400,
        "secure": true,
        "http_only": true,
        "same_site": "Strict"
      }
    ]
  }
}
```

Multiple cookies:

```json
{
  "type": "response.json",
  "config": {
    "status": 200,
    "body": { "ok": true },
    "cookies": [
      {
        "name": "session",
        "value": "{{ nodes.login.token }}",
        "path": "/",
        "max_age": 86400,
        "secure": true,
        "http_only": true
      },
      {
        "name": "preferences",
        "value": "{{ nodes.prefs.encoded }}",
        "path": "/",
        "max_age": 31536000
      }
    ]
  }
}
```
