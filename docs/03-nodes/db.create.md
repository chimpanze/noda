# db.create

Inserts a row into a table.

## Config

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `table` | string (expr) | yes | Table name |
| `data` | object | yes | Column values (expressions) |

## Outputs

`success`, `error`

Output: The inserted row data (with generated fields like `id`). Returns `ConflictError` on duplicate key.

## Behavior

Inserts a new record into the specified table using the key-value pairs in `data`. Returns the created record including any database-generated fields. Fires `error` with `ConflictError` if a unique constraint is violated.

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
      "user_id": "{{ auth.user_id }}",
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
