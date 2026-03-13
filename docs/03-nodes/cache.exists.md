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
