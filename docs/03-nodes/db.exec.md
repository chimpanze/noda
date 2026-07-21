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

The `error` port fires on SQL errors such as constraint violations, syntax errors, or connection failures. Constraint violations, invalid input, and similar caller-triggerable failures are classified into typed errors (e.g. `ConflictError`, `ValidationError`) rather than a generic 500. The error output contains:

```json
{
  "code": "CONFLICT",
  "error": "conflict on query: unique constraint violation: ERROR: duplicate key value violates unique constraint \"users_email_key\" (SQLSTATE 23505)",
  "node_id": "insert_user",
  "node_type": "db.exec"
}
```

> **`error` is a diagnostic field.** It may contain driver, network, or filesystem detail such as
> constraint names, internal hostnames, or file paths. Do not forward it to clients — branch on
> `code` instead, and return your own message.

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

## Runnable example

A runnable, CI-verified example of this node lives in the cookbook:
[`examples/node-cookbook/db`](../../examples/node-cookbook/db/README.md) — its README documents the exact request/response pair the integration suite executes.
