# db.query

Executes a SELECT query and returns result rows.

## Config

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `query` | string (expr) | yes | SQL SELECT statement |
| `params` | array | no | Positional query parameters ($1, $2, ...) |

## Outputs

`success`, `error`

Output: Array of row objects (empty array if no results).

## Behavior

Executes the given SQL query with parameterized values against the configured database service. Returns all matching rows as an array of objects.

## Service Dependencies

| Slot | Prefix | Required |
|------|--------|----------|
| `database` | `db` | Yes |

## Example

```json
{
  "type": "db.query",
  "services": { "database": "postgres" },
  "config": {
    "query": "SELECT * FROM tasks WHERE user_id = $1 ORDER BY created_at DESC LIMIT $2",
    "params": ["{{ auth.user_id }}", "{{ input.limit ?? 20 }}"]
  }
}
```
