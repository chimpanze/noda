# transform.delete

Removes fields from an object.

## Config

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `data` | string (expr) | yes | Expression resolving to object |
| `fields` | array of strings | no | Field names to remove (absent = no-op) |

## Outputs

`success`, `error`

## Behavior

Resolves `data` to an object. Returns a copy with the named fields removed. Does not error if a field doesn't exist.

## Example

```json
{
  "type": "transform.delete",
  "config": {
    "data": "{{ nodes.fetch[0] }}",
    "fields": ["password", "internal_notes"]
  }
}
```

### With data flow

After fetching a user record from the database, `transform.delete` strips sensitive fields before returning the data in an API response.

```json
{
  "sanitize_user": {
    "type": "transform.delete",
    "config": {
      "data": "{{ nodes.get_user }}",
      "fields": ["password_hash", "reset_token", "internal_flags"]
    }
  }
}
```

Output stored as `nodes.sanitize_user`:
```json
{
  "id": 3,
  "email": "jane@example.com",
  "display_name": "Jane Doe",
  "created_at": "2025-01-10T08:00:00Z"
}
```

Downstream nodes access the cleaned object via `nodes.sanitize_user.email` or `nodes.sanitize_user.display_name`.

## Runnable example

A runnable, CI-verified example of this node lives in the cookbook:
[`examples/node-cookbook/transform`](../../examples/node-cookbook/transform/README.md) — its README documents the exact request/response pair the integration suite executes.
