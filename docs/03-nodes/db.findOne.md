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
