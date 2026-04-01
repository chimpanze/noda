# db.upsert

Inserts a row or updates it on conflict.

## Config

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `table` | string (expr) | yes | Table name |
| `data` | object | yes | Column values (expressions) |
| `conflict` | string or array | yes | Conflict column(s) for ON CONFLICT |
| `update` | array or object | no | Columns to update on conflict (array of names, or object of assignments). Defaults to updating all non-conflict columns. |

## Outputs

`success`, `error`

Output: The upserted row data.

## Behavior

Inserts a new record into the specified table. If a conflict occurs on the specified column(s), updates the existing row instead. Returns the upserted record including any database-generated fields.

## Service Dependencies

| Slot | Prefix | Required |
|------|--------|----------|
| `database` | `db` | Yes |

## Example

```json
{
  "type": "db.upsert",
  "services": { "database": "postgres" },
  "config": {
    "table": "user_settings",
    "data": {
      "user_id": "{{ auth.user_id }}",
      "theme": "{{ input.theme }}",
      "language": "{{ input.language }}",
      "updated_at": "{{ now() }}"
    },
    "conflict": "user_id",
    "update": ["theme", "language", "updated_at"]
  }
}
```

### With data flow

A profile update workflow reads the authenticated user's ID and upserts their preferences. The result feeds into a response node.

```json
{
  "save_preferences": {
    "type": "db.upsert",
    "services": { "database": "postgres" },
    "config": {
      "table": "user_preferences",
      "data": {
        "user_id": "{{ auth.user_id }}",
        "timezone": "{{ nodes.validate_input.timezone }}",
        "notifications_enabled": "{{ nodes.validate_input.notifications_enabled }}",
        "updated_at": "{{ now() }}"
      },
      "conflict": "user_id",
      "update": ["timezone", "notifications_enabled", "updated_at"]
    }
  }
}
```

Output stored as `nodes.save_preferences`:
```json
{
  "id": 8,
  "user_id": 15,
  "timezone": "Europe/Berlin",
  "notifications_enabled": true,
  "updated_at": "2026-03-20T14:00:00Z"
}
```

Downstream nodes access fields via `nodes.save_preferences.timezone` or `nodes.save_preferences.id`.
