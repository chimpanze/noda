# Routes

Files in `routes/*.json`. Each file defines one route.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `id` | string | yes | Unique route identifier |
| `method` | string | yes | HTTP method: `GET`, `POST`, `PUT`, `PATCH`, `DELETE` |
| `path` | string | yes | URL path pattern (supports `:param` placeholders) |
| `summary` | string | no | OpenAPI summary |
| `tags` | array | no | OpenAPI tags |
| `middleware` | array | no | Route-specific middleware |
| `body` | object | no | Request body definition |
| `body.schema` | object | no | JSON Schema or `$ref`. Validated automatically before the workflow runs |
| `body.validate` | boolean | no | Enable/disable automatic validation (default: `true`) |
| `response` | object | no | Response schemas keyed by status code |
| `response.validate` | string/boolean | no | Response validation mode (see below) |
| `response.<status>.schema` | object | no | JSON Schema for responses with this status code |
| `trigger` | object | yes | Workflow to execute |
| `trigger.workflow` | string | yes | Workflow ID |
| `trigger.input` | object | no | Input mapping (expressions) |

**Trigger input sources:** `body.*`, `params.*`, `query.*`, `auth.*`, `request.*`.

When `body.schema` is present, request bodies are validated automatically before the workflow runs. Invalid requests receive a `422` response with `VALIDATION_ERROR` code and field-level error details. Set `body.validate: false` to use the schema only for OpenAPI documentation without runtime enforcement.

**Response validation** detects when the server produces output that doesn't match the documented response schema. The `response.validate` field controls behavior:

| Value | Dev mode | Production |
|-------|----------|------------|
| absent (default) | Validate, log warning, send original response | Skip |
| `"warn"` | Warn + send original | Warn + send original |
| `"strict"` | Return 500 on mismatch | Return 500 on mismatch |
| `false` | Skip | Skip |

Response schemas are keyed by HTTP status code. Only responses from workflow response nodes are validated — infrastructure error responses (timeouts, workflow failures) are not checked.

```json
{
  "id": "update-task",
  "method": "PUT",
  "path": "/api/tasks/:id",
  "summary": "Update a task",
  "tags": ["tasks"],
  "middleware": ["auth.jwt"],
  "body": {
    "schema": { "$ref": "schemas/Task#UpdateTask" }
  },
  "trigger": {
    "workflow": "update-task",
    "input": {
      "id": "{{ params.id }}",
      "title": "{{ body.title }}",
      "completed": "{{ body.completed }}",
      "user_id": "{{ auth.user_id }}"
    }
  }
}
```
