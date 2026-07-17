# Workflows

Files in `workflows/*.json`. Each file defines one workflow.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `id` | string | yes | Unique workflow identifier |
| `name` | string | no | Display name |
| `description` | string | no | Human-readable description |
| `version` | string | no | Free-form version string |
| `timeout` | string | no | Maximum execution time (e.g., `"30s"`, `"5m"`). Workflow is cancelled if exceeded. |
| `nodes` | object | yes | Map of node ID to node definition |
| `edges` | array | yes | Execution flow edges |

## Node Definition

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `type` | string | yes | Node type (e.g., `"db.query"`, `"control.if"`) |
| `services` | object | no | Service slot mappings |
| `config` | object | yes | Node-specific configuration |
| `as` | string | no | Output alias: the node's output is stored (and referenced in expressions) under this name instead of the node ID. Must not collide with another node ID. |
| `position` | object | no | Visual-editor coordinates (`{"x": ..., "y": ...}`); ignored by the runtime |

Every node's `config` object is validated against the node's schema at two points: when you run `noda validate`, and when the server starts. Validation checks that all required fields are present, that literal values match their declared types, and that unknown top-level config fields are rejected. Expression values (`{{ … }}`) satisfy any declared type. Validation errors surface in the CLI output or server startup logs respectively.

## Edge Definition

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `from` | string | yes | Source node ID |
| `to` | string | yes | Target node ID |
| `output` | string | no | Named output (e.g., `"then"`, `"else"`, `"error"`) |
| `retry` | object | no | Retry configuration |
| `retry.attempts` | integer | no | Max retry attempts |
| `retry.backoff` | string | no | `"fixed"` or `"exponential"` |
| `retry.delay` | string | no | Base delay between retries |

```json
{
  "id": "process-order",
  "name": "Process Order",
  "nodes": {
    "validate": {
      "type": "transform.validate",
      "config": {
        "schema": { "$ref": "schemas/Order#CreateOrder" }
      }
    },
    "create": {
      "type": "db.create",
      "services": { "database": "postgres" },
      "config": {
        "table": "orders",
        "data": {
          "user_id": "{{ input.user_id }}",
          "total": "{{ input.total }}"
        }
      }
    }
  },
  "edges": [
    { "from": "validate", "to": "create" },
    {
      "from": "create", "to": "notify", "output": "success",
      "retry": { "attempts": 3, "backoff": "exponential", "delay": "1s" }
    }
  ]
}
```
