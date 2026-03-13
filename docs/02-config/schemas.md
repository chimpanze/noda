# Schemas

Files in `schemas/*.json`. Each file contains named JSON Schema definitions.

```json
{
  "Task": {
    "type": "object",
    "properties": {
      "id": { "type": "integer" },
      "title": { "type": "string" },
      "completed": { "type": "boolean" }
    }
  },
  "CreateTask": {
    "type": "object",
    "properties": {
      "title": { "type": "string", "minLength": 1 }
    },
    "required": ["title"]
  }
}
```

Referenced from routes and nodes with `$ref`:

```json
{ "$ref": "schemas/Task#CreateTask" }
```
