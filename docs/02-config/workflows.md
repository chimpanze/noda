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
        "schema": { "$ref": "schemas/CreateOrder" }
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
    },
    "conflict": {
      "type": "response.error",
      "config": {
        "status": 409,
        "code": "ORDER_EXISTS",
        "message": "Order already exists"
      }
    },
    "notify": {
      "type": "email.send",
      "services": { "mailer": "smtp" },
      "config": {
        "to": "{{ input.email }}",
        "subject": "Order confirmed",
        "body": "Your order {{ nodes.create.id }} for {{ input.total }} has been placed."
      }
    }
  },
  "edges": [
    { "from": "validate", "to": "create" },
    {
      "from": "create", "to": "notify", "output": "success",
      "retry": { "attempts": 3, "backoff": "exponential", "delay": "1s" }
    },
    { "from": "create", "to": "conflict", "output": "exists" }
  ]
}
```

### Outcome outputs must be wired

Some outputs report an operation outcome rather than a control-flow branch:
`exists` on `db.create`/`db.update`/`db.upsert`/`auth.create_user`, `not_found`
on `auth.get_user`, and `invalid` on `auth.set_password`/`auth.verify_credentials`/
`auth.consume_token`. A fired output with no outbound edge silently ends that
execution path, so `noda validate` (and boot) reject a workflow that leaves an
outcome output unwired:

```
workflow "create-user", node "insert" (db.create): outcome output "exists" has no
outbound edge — a fired outcome output with no edge silently ends the path; wire
it (e.g. to an error response, or to the same target as "success" if the
distinction does not matter)
```

Control-flow branches are exempt: leaving `control.if`'s `else`,
`control.switch`'s `default`, or `control.loop`'s `done` unwired is a normal
workflow shape.
