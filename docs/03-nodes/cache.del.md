# cache.del

Deletes a key from the cache.

## Config

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `key` | string (expr) | yes | Cache key |

## Outputs

`success`, `error`

Output: `{ok: true}`

## Behavior

Deletes the specified key from the cache. Fires `success` on completion.

## Service Dependencies

| Slot | Prefix | Required |
|------|--------|----------|
| `cache` | `cache` | Yes |

## Output Shape

```json
// success output
{ "ok": true }
```

The node returns `{ "ok": true }` regardless of whether the key existed before deletion.

## Error Output

The `error` port fires if the Redis connection fails. The error output contains:

```json
{
  "error": "cache.del: connection refused",
  "node_id": "remove_session",
  "node_type": "cache.del"
}
```

## Examples

### Invalidate cache after update

```json
{
  "update_user": {
    "type": "db.exec",
    "services": { "database": "postgres" },
    "config": {
      "query": "UPDATE users SET name = $1 WHERE id = $2",
      "params": ["{{ input.name }}", "{{ input.user_id }}"]
    }
  },
  "invalidate_cache": {
    "type": "cache.del",
    "services": { "cache": "redis" },
    "config": {
      "key": "{{ 'user:' + input.user_id }}"
    }
  }
}
```

After `update_user` succeeds, `invalidate_cache` removes the stale cached entry. The output `nodes.invalidate_cache.ok` is `true` on success.
