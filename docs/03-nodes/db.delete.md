# db.delete

Deletes rows matching a condition.

## Config

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `table` | string (expr) | yes | Table name |
| `where` | object | yes | Equality conditions as key-value pairs |

## Outputs

`success`, `error`

Output: `{rows_affected: <count>}`

## Behavior

Deletes all rows in the specified table that match the `where` conditions. Returns the number of affected rows.

## Service Dependencies

| Slot | Prefix | Required |
|------|--------|----------|
| `database` | `db` | Yes |

## Example

```json
{
  "type": "db.delete",
  "services": { "database": "postgres" },
  "config": {
    "table": "tasks",
    "where": {
      "id": "{{ input.id }}",
      "user_id": "{{ auth.user_id }}"
    }
  }
}
```

### With data flow

A workflow first verifies a task belongs to the authenticated user, then deletes it and returns a confirmation.

```json
{
  "remove_task": {
    "type": "db.delete",
    "services": { "database": "postgres" },
    "config": {
      "table": "tasks",
      "where": {
        "id": "{{ nodes.verify_task.id }}",
        "user_id": "{{ auth.user_id }}"
      }
    }
  }
}
```

Output stored as `nodes.remove_task`:
```json
{ "rows_affected": 1 }
```

Downstream nodes access `nodes.remove_task.rows_affected` to confirm the deletion occurred.
