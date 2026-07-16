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
| `middleware_preset` | string | no | Named middleware preset from `noda.json`'s `middleware_presets` |
| `params.schema` | object | no | JSON Schema for path parameters, validated before the workflow runs |
| `query.schema` | object | no | JSON Schema for query parameters, validated before the workflow runs |
| `body` | object | no | Request body definition |
| `body.schema` | object | no | JSON Schema or `$ref`. Validated automatically before the workflow runs |
| `body.validate` | boolean | no | Enable/disable automatic validation (default: `true`) |
| `body.content_type` | string | no | Request content type advertised in OpenAPI (e.g. `multipart/form-data`) |
| `response` | object | no | Response schemas keyed by status code |
| `response.validate` | string/boolean | no | Response validation mode (see below) |
| `response.<status>.schema` | object | no | JSON Schema for responses with this status code |
| `response_timeout` | string | no | Per-route request timeout (duration); overrides `server.response_timeout` |
| `trigger` | object | yes | Workflow to execute |
| `trigger.workflow` | string | yes | Workflow ID |
| `trigger.input` | object | no | Input mapping (expressions) |
| `trigger.files` | array of strings | no | Input keys to treat as uploaded file streams (multipart fields); each entry must have a matching `trigger.input` key |
| `trigger.coerce` | boolean | no | Numeric coercion of string-typed trigger inputs (default `true`). Set `false` to keep numeric-looking path/query/header/form values as strings. |

**Trigger input sources:** `body.*`, `params.*`, `query.*`, `headers.*`, `auth.*`, `raw_body` (when `trigger.raw_body: true`).

**Numeric coercion:** inputs that are a single bare reference to a string-typed transport — `params.*`, `query.*`, `headers.*`, or `body.*` for form-encoded requests (plus `request.*` aliases) — are converted to numbers when they parse as one (`{{ query.limit }}` → `10`). JSON body values keep their JSON types, and computed expressions and literal values are never coerced. Set `"coerce": false` on the trigger when IDs like `"0042"` must stay strings.

The `request.*` namespace provides aliases to these same fields: `request.body`, `request.params`, `request.query`, `request.headers`, `request.auth`, and `request.raw_body` (when enabled).

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
      "user_id": "{{ auth.sub }}"
    }
  }
}
```
