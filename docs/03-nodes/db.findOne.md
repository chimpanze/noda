# db.findOne

Single row SELECT returning a single row object.

## Config

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `table` | string (expr) | yes | Table name |
| `select` | array | no | Column names to select (default: all) |
| `where` | object | no | Equality conditions as key-value pairs |
| `where_clause` | object | no | Raw WHERE with `query` (string) and `params` (array) |
| `joins` | array | no | JOIN clauses |
| `order` | string | no | ORDER BY clause |
| `group` | string | no | GROUP BY clause |
| `having` | string | no | HAVING clause |
| `required` | boolean | no | If `true` (default), returns `NotFoundError` when no row matches. If `false`, returns `nil`. |

## Outputs

`success`, `error`

Output: `map[string]any`. Forces `LIMIT 1`.

## Behavior

Builds and executes a SELECT query with `LIMIT 1`. Returns a single row object. When `required` is `true` (the default), fires `error` with `NotFoundError` if no row matches. When `required` is `false`, returns `nil` instead.

## Service Dependencies

| Slot | Prefix | Required |
|------|--------|----------|
| `database` | `db` | Yes |

## Example

```json
{
  "type": "db.findOne",
  "services": { "database": "postgres" },
  "config": {
    "table": "tasks",
    "where": {
      "id": "{{ input.task_id }}",
      "user_id": "{{ auth.user_id }}"
    },
    "required": true
  }
}
```

### With data flow

A task detail endpoint receives a task ID from the route params, fetches the task, then uses the task's `user_id` to look up the assignee.

```json
{
  "get_task": {
    "type": "db.findOne",
    "services": { "database": "postgres" },
    "config": {
      "table": "tasks",
      "select": ["id", "title", "status", "user_id"],
      "where": {
        "id": "{{ input.task_id }}"
      },
      "required": true
    }
  },
  "get_assignee": {
    "type": "db.findOne",
    "services": { "database": "postgres" },
    "config": {
      "table": "users",
      "select": ["id", "display_name", "email"],
      "where": {
        "id": "{{ nodes.get_task.user_id }}"
      }
    }
  }
}
```

Output stored as `nodes.get_task`:
```json
{ "id": 7, "title": "Write tests", "status": "in_progress", "user_id": 3 }
```

Downstream nodes access fields via `nodes.get_task.title` or `nodes.get_assignee.display_name`.
