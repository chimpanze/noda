# db.find

Structured SELECT returning an array of row objects.

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
| `having` | string or object | no | HAVING clause. Prefer the parameterized object form `{"query": "count(*) > ?", "params": [5]}`; a bare string still works but is deprecated (logs a warning) |
| `limit` | integer | no | Max rows to return |
| `offset` | integer | no | Rows to skip |

## Outputs

`success`, `error`

Output: `[]map[string]any` (empty array if no rows).

## Behavior

Builds and executes a SELECT query from the structured config fields. Returns all matching rows as an array of objects.

SQL injection prevention: All database nodes validate SQL fragments to prevent injection attacks. Table names, column names, and identifiers must match `^[a-zA-Z_][a-zA-Z0-9_.]*$`. ORDER BY clauses are validated per-item. JOIN types must be one of `INNER`, `LEFT`, `RIGHT`, `FULL`, or `CROSS`. SQL fragments (`where_clause.query`, `joins[].on`, `group`, `having`) reject semicolons (`;`), line comments (`--`), block comments (`/*`), and any fragment containing one of these keywords as a whole word: `DROP`, `DELETE`, `INSERT`, `UPDATE`, `ALTER`, `CREATE`, `EXEC`, `UNION`, `SELECT`, `GRANT`, `REVOKE`, `TRUNCATE` — this can also reject legitimate column or alias names that happen to contain a keyword as a separate word. Always pass dynamic values through `params` rather than interpolating them into SQL strings.

## Service Dependencies

| Slot | Prefix | Required |
|------|--------|----------|
| `database` | `db` | Yes |

## Example

```json
{
  "type": "db.find",
  "services": { "database": "postgres" },
  "config": {
    "table": "tasks",
    "select": ["id", "title", "completed"],
    "where": {
      "user_id": "{{ auth.sub }}",
      "completed": false
    },
    "order": "created_at DESC",
    "limit": 20
  }
}
```

### With data flow

A project dashboard workflow fetches a project by ID, then finds all tasks belonging to that project.

```json
{
  "list_tasks": {
    "type": "db.find",
    "services": { "database": "postgres" },
    "config": {
      "table": "tasks",
      "select": ["id", "title", "status", "assignee_id"],
      "where": {
        "project_id": "{{ nodes.get_project.id }}"
      },
      "order": "created_at DESC",
      "limit": 50
    }
  }
}
```

Output stored as `nodes.list_tasks`:
```json
[
  { "id": 1, "title": "Design API schema", "status": "done", "assignee_id": 5 },
  { "id": 2, "title": "Implement auth", "status": "in_progress", "assignee_id": 3 }
]
```

Downstream nodes access fields via `nodes.list_tasks[0].title` or iterate with `transform.map` over `nodes.list_tasks`.
