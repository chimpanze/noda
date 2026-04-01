# db.exec

Executes an INSERT, UPDATE, or DELETE statement.

## Config

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `query` | string (expr) | yes | SQL statement |
| `params` | array | no | Positional query parameters |

## Outputs

`success`, `error`

Output: `{rows_affected: <count>}`

## Behavior

Executes the given SQL write statement (INSERT, UPDATE, DELETE) with parameterized values against the configured database service. Returns the number of affected rows.

## Service Dependencies

| Slot | Prefix | Required |
|------|--------|----------|
| `database` | `db` | Yes |

## Output Shape

```json
// success output
{ "rows_affected": 3 }
```

`rows_affected` is an integer indicating how many rows were inserted, updated, or deleted.

## Error Output

The `error` port fires on SQL errors such as constraint violations, syntax errors, or connection failures. The error output contains:

```json
{
  "error": "db.exec: ERROR: duplicate key value violates unique constraint \"users_email_key\" (SQLSTATE 23505)",
  "node_id": "insert_user",
  "node_type": "db.exec"
}
```

## Examples

### Soft-delete with row count check

```json
{
  "deactivate_users": {
    "type": "db.exec",
    "services": { "database": "postgres" },
    "config": {
      "query": "UPDATE users SET active = false WHERE last_login < $1",
      "params": ["{{ input.cutoff_date }}"]
    }
  },
  "log_result": {
    "type": "util.log",
    "config": {
      "message": "{{ 'Deactivated ' + string(nodes.deactivate_users.rows_affected) + ' users' }}"
    }
  }
}
```

After `deactivate_users` completes, `nodes.deactivate_users.rows_affected` contains the number of updated rows. Downstream nodes can use this value for logging, conditional logic, or response bodies.
