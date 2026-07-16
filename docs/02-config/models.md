# models/*.json

Model files describe database tables declaratively. The visual editor (and its HTTP API) uses them to generate SQL migrations (`migrations/*_models.up.sql` / `.down.sql`) and CRUD route/workflow scaffolding. They are validated against Noda's embedded `model.json` schema at config load.

Models are **design-time** artifacts: the runtime executes the generated migrations and workflows, not the model files themselves.

## Top-Level Fields

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `table` | string | yes | Database table name |
| `columns` | object | yes | Map of column name to column definition |
| `relations` | object | no | Map of relation name to relation definition |
| `indexes` | array | no | Index definitions |
| `timestamps` | boolean | no | Add `created_at`/`updated_at` columns |
| `soft_delete` | boolean | no | Add a `deleted_at` column for soft deletion |

## Column Definition

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `type` | string | yes | Column type (e.g. `uuid`, `text`, `integer`, `timestamp`) |
| `primary_key` | boolean | no | Mark as primary key |
| `not_null` | boolean | no | Add a NOT NULL constraint |
| `default` | string | no | Default value expression |
| `enum` | array of strings | no | Allowed values (generates a CHECK constraint) |
| `max_length` | integer | no | Maximum length (for string types) |
| `precision` | integer | no | Numeric precision |
| `scale` | integer | no | Numeric scale |

## Relation Definition

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `type` | string | yes | `"belongsTo"`, `"hasMany"`, or `"manyToMany"` |
| `table` | string | yes | Related table name |
| `foreign_key` | string | yes | Foreign key column |
| `on_delete` | string | no | ON DELETE behavior (e.g. `cascade`) |
| `junction` | string | no | Junction table (for `manyToMany`) |
| `local_key` | string | no | Local key column when it isn't the primary key |

## Index Definition

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `columns` | array of strings | yes | Columns in the index (at least one) |
| `unique` | boolean | no | Create a unique index |

## Example

```json
{
  "table": "posts",
  "columns": {
    "id": { "type": "uuid", "primary_key": true, "default": "gen_random_uuid()" },
    "author_id": { "type": "uuid", "not_null": true },
    "title": { "type": "text", "not_null": true, "max_length": 200 },
    "status": { "type": "text", "enum": ["draft", "published", "archived"], "default": "'draft'" }
  },
  "relations": {
    "author": { "type": "belongsTo", "table": "users", "foreign_key": "author_id", "on_delete": "cascade" }
  },
  "indexes": [
    { "columns": ["author_id"] },
    { "columns": ["title"], "unique": true }
  ],
  "timestamps": true,
  "soft_delete": false
}
```
