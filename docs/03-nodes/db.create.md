# db.create

Inserts a row into a table.

## Config

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `table` | string (expr) | yes | Table name |
| `data` | object | yes | Column values (expressions) |

## Outputs

`success`, `exists`, `error`

Output: The inserted row data (with generated fields like `id`).

## Behavior

Inserts a new record into the specified table using the key-value pairs in `data`. Returns the created record including any database-generated fields.

If the insert collides with a unique constraint, the node fires `exists` rather than `error`, so a duplicate value can be answered with a field-scoped 409/422 without also catching unrelated database failures. Any other database error fires `error`.

> **Wire `exists` if you care about duplicates.** An output with no outbound edge silently ends that path — the workflow neither continues nor fails.

`data` values are resolved at any depth, so a nested object destined for a JSON/JSONB column may use expressions in its leaves.

## Service Dependencies

| Slot | Prefix | Required |
|------|--------|----------|
| `database` | `db` | Yes |

## Example

```json
{
  "type": "db.create",
  "services": { "database": "postgres" },
  "config": {
    "table": "tasks",
    "data": {
      "title": "{{ input.title }}",
      "user_id": "{{ auth.sub }}",
      "completed": false
    }
  }
}
```

### With data flow

A registration workflow validates input, then creates a user record. The `transform.set` node prepares the data, and `db.create` inserts it.

```json
{
  "create_user": {
    "type": "db.create",
    "services": { "database": "postgres" },
    "config": {
      "table": "users",
      "data": {
        "email": "{{ nodes.prepare.email }}",
        "display_name": "{{ nodes.prepare.display_name }}",
        "role": "{{ nodes.prepare.role }}"
      }
    }
  }
}
```

Output stored as `nodes.create_user`:
```json
{
  "id": 42,
  "email": "jane@example.com",
  "display_name": "Jane Doe",
  "role": "user",
  "created_at": "2026-03-15T10:30:00Z"
}
```

Downstream nodes access fields via `nodes.create_user.id` or `nodes.create_user.email`.

## Runnable example

A runnable, CI-verified example of this node lives in the cookbook:
[`examples/node-cookbook/db`](../../examples/node-cookbook/db/README.md) — its README documents the exact request/response pair the integration suite executes.
