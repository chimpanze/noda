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
