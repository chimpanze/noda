# cache.set

Sets a value in the cache.

## Config

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `key` | string (expr) | yes | Cache key |
| `value` | any (expr) | yes | Value to store |
| `ttl` | integer | no | Time-to-live in seconds (0 = no expiry) |

## Outputs

`success`, `error`

Output: `{ok: true}`

## Behavior

Writes the given value to the cache under the specified key. If `ttl` is provided, the key expires after that many seconds. Omit `ttl` for no expiration. Fires `success` on completion.

## Service Dependencies

| Slot | Prefix | Required |
|------|--------|----------|
| `cache` | `cache` | Yes |

## Example

```json
{
  "type": "cache.set",
  "services": { "cache": "redis" },
  "config": {
    "key": "{{ 'session:' + auth.sub }}",
    "value": "{{ nodes.session_data }}",
    "ttl": 3600
  }
}
```

## Runnable example

A runnable, CI-verified example of this node lives in the cookbook:
[`examples/node-cookbook/cache`](../../examples/node-cookbook/cache/README.md) — its README documents the exact request/response pair the integration suite executes.
