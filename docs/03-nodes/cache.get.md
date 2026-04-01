# cache.get

Retrieves a value from the cache.

## Config

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `key` | string (expr) | yes | Cache key |

## Outputs

`success`, `error`

Output: `{value: <any>}` (value is `nil` if key not found).

## Behavior

Reads the cached value for the given key. Fires `success` with the value. Fires `error` with `NotFoundError` if the key doesn't exist.

## Service Dependencies

| Slot | Prefix | Required |
|------|--------|----------|
| `cache` | `cache` | Yes |

## Output Shape

```json
// success output (cache hit — string value)
{ "value": "the-cached-value" }

// success output (cache hit — object value, auto-deserialized from JSON)
{ "value": { "name": "Alice", "role": "admin" } }
```

## Error Output

The `error` port fires when the key does not exist in the cache (`NotFoundError`) or when the Redis connection fails. The error output contains:

```json
{
  "error": "cache not found: user:999",
  "node_id": "check_cache",
  "node_type": "cache.get"
}
```

## Examples

### Cache-aside pattern

```json
{
  "check_cache": {
    "type": "cache.get",
    "services": { "cache": "redis" },
    "config": {
      "key": "{{ 'user:' + input.user_id }}"
    }
  }
}
```

On a cache hit, `nodes.check_cache.value` contains the cached data. On a cache miss, the `error` output fires with a `NotFoundError` — use a `control.if` or wire the error edge to fall through to a database lookup.
