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
