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
  "code": "INTERNAL_ERROR",
  "error": "cache.del: connection refused",
  "node_id": "remove_session",
  "node_type": "cache.del"
}
```

> **`error` is a diagnostic field.** It may contain driver, network, or filesystem detail such as
> constraint names, internal hostnames, or file paths. Do not forward it to clients — branch on
> `code` instead, and return your own message.

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

## Runnable example

A runnable, CI-verified example of this node lives in the cookbook:
[`examples/node-cookbook/cache`](../../examples/node-cookbook/cache/README.md) — its README documents the exact request/response pair the integration suite executes.
