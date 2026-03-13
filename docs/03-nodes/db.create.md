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
