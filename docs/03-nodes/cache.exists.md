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
  "error": "cache.exists: connection refused",
  "node_id": "check_rate_limit",
  "node_type": "cache.exists"
}
```

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
