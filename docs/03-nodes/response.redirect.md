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

### With data flow

After creating a resource, redirect the client to the new resource's URL.

```json
{
  "redirect_to_order": {
    "type": "response.redirect",
    "config": {
      "url": "{{ '/api/orders/' + string(nodes.create_order.id) }}",
      "status": 302
    }
  }
}
```

When `nodes.create_order` produced `{"id": 784, "status": "pending"}`, the URL resolves to `"/api/orders/784"`. The client receives:
```
HTTP/1.1 302 Found
Location: /api/orders/784
```
