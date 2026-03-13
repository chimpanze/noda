# Workflows

Files in `workflows/*.json`. Each file defines one workflow.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `id` | string | yes | Unique workflow identifier |
| `name` | string | no | Display name |
| `nodes` | object | yes | Map of node ID to node definition |
| `edges` | array | yes | Execution flow edges |

## Node Definition

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `type` | string | yes | Node type (e.g., `"db.query"`, `"control.if"`) |
| `services` | object | no | Service slot mappings |
| `config` | object | yes | Node-specific configuration |

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
