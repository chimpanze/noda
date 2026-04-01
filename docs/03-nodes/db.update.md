# db.update

Updates rows matching a condition.

## Config

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `table` | string (expr) | yes | Table name |
| `data` | object | yes | Fields to update (expressions) |
| `where` | object | yes | Equality conditions as key-value pairs |

## Outputs

`success`, `error`

Output: `{rows_affected: <count>}`

## Behavior

Updates all rows in the specified table that match the `where` conditions, setting the fields specified in `data`. Returns the number of affected rows.

## Service Dependencies

| Slot | Prefix | Required |
|------|--------|----------|
| `database` | `db` | Yes |

## Example

```json
{
  "type": "db.update",
  "services": { "database": "postgres" },
  "config": {
    "table": "tasks",
    "data": {
      "completed": "{{ input.completed }}",
      "updated_at": "{{ now() }}"
    },
    "where": {
      "id": "{{ input.id }}",
      "user_id": "{{ auth.user_id }}"
    }
  }
}
```

### With data flow

A workflow fetches a task to verify ownership, then updates its status based on request input.

```json
{
  "update_status": {
    "type": "db.update",
    "services": { "database": "postgres" },
    "config": {
      "table": "tasks",
      "data": {
        "status": "{{ input.status }}",
        "updated_at": "{{ now() }}"
      },
      "where": {
        "id": "{{ nodes.verify_task.id }}",
        "user_id": "{{ auth.user_id }}"
      }
    }
  }
}
```

Output stored as `nodes.update_status`:
```json
{ "rows_affected": 1 }
```

Downstream nodes access `nodes.update_status.rows_affected` to check whether any row was modified.
