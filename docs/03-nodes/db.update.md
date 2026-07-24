# db.update

Updates rows matching a condition.

## Config

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `table` | string (expr) | yes | Table name |
| `data` | object | yes | Fields to update (expressions) |
| `where` | object | yes | Equality conditions as key-value pairs |

## Outputs

`success`, `exists`, `error`

Output: `{rows_affected: <count>}`

## Behavior

Updates all rows in the specified table that match the `where` conditions, setting the fields specified in `data`. Returns the number of affected rows.

`data` values are resolved at any depth, so a nested object destined for a JSON/JSONB column may use expressions in its leaves.

If the update collides with a unique constraint, the node fires `exists` rather than `error`, so a duplicate value can be answered with a field-scoped 409/422 without also catching unrelated database failures. Any other database error fires `error`.

> **Wire `exists` if you care about duplicates.** An output with no outbound edge silently ends that path — the workflow neither continues nor fails. A workflow that wired `error` to catch duplicates must move that edge to `exists`.

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
      "user_id": "{{ auth.sub }}"
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
        "user_id": "{{ auth.sub }}"
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

## Runnable example

A runnable, CI-verified example of this node lives in the cookbook:
[`examples/node-cookbook/db`](../../examples/node-cookbook/db/README.md) — its README documents the exact request/response pair the integration suite executes.
