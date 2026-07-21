# cache.exists

Checks if a key exists in the cache.

## Config

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `key` | string (expr) | yes | Cache key |

## Outputs

`success`, `error`

Output: `{exists: true/false}`

## Behavior

Checks whether the specified key exists in the cache. Fires `success` with a boolean `exists` field.

## Service Dependencies

| Slot | Prefix | Required |
|------|--------|----------|
| `cache` | `cache` | Yes |

## Output Shape

```json
// success output (key exists)
{ "exists": true }

// success output (key does not exist)
{ "exists": false }
```

## Error Output

The `error` port fires if the Redis connection fails. The error output contains:

```json
{
  "code": "INTERNAL_ERROR",
  "error": "cache.exists: connection refused",
  "node_id": "check_rate_limit",
  "node_type": "cache.exists"
}
```

> **`error` is a diagnostic field.** It may contain driver, network, or filesystem detail such as
> constraint names, internal hostnames, or file paths. Do not forward it to clients — branch on
> `code` instead, and return your own message.

## Examples

### Rate-limit guard

```json
{
  "check_rate_limit": {
    "type": "cache.exists",
    "services": { "cache": "redis" },
    "config": {
      "key": "{{ 'ratelimit:' + input.client_ip }}"
    }
  },
  "is_rate_limited": {
    "type": "control.if",
    "config": {
      "condition": "{{ nodes.check_rate_limit.exists == true }}"
    }
  }
}
```

`nodes.check_rate_limit.exists` is `true` if the rate-limit key is present. Wire `is_rate_limited`'s `then` output to a `response.error` node returning 429, and the `else` output to the normal request handler.

## Runnable example

A runnable, CI-verified example of this node lives in the cookbook:
[`examples/node-cookbook/cache`](../../examples/node-cookbook/cache/README.md) — its README documents the exact request/response pair the integration suite executes.
