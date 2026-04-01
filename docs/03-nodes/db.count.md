# db.count

Counts rows matching conditions.

## Config

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `table` | string (expr) | yes | Table name |
| `where` | object | no | Equality conditions as key-value pairs |
| `where_clause` | object | no | Raw WHERE with `query` (string) and `params` (array) |
| `joins` | array | no | JOIN clauses |

## Outputs

`success`, `error`

Output: `{"count": <int64>}`

## Behavior

Builds and executes a `SELECT COUNT(*)` query from the structured config fields. Returns the count of matching rows.

## Service Dependencies

| Slot | Prefix | Required |
|------|--------|----------|
| `database` | `db` | Yes |

## Example

```json
{
  "type": "db.count",
  "services": { "database": "postgres" },
  "config": {
    "table": "tasks",
    "where": {
      "user_id": "{{ auth.user_id }}",
      "completed": false
    }
  }
}
```

### With data flow

A dashboard workflow counts tasks per status for a project fetched earlier, then builds a summary response.

```json
{
  "count_open": {
    "type": "db.count",
    "services": { "database": "postgres" },
    "config": {
      "table": "tasks",
      "where": {
        "project_id": "{{ nodes.get_project.id }}",
        "status": "open"
      }
    }
  }
}
```

Output stored as `nodes.count_open`:
```json
{ "count": 12 }
```

Downstream nodes access `nodes.count_open.count` to include the value in a response or conditional logic.
