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
