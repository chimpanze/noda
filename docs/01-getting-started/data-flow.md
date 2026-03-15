# Data Flow

This guide explains how data moves through a Noda workflow — from the incoming request to node outputs and the final response.

## Trigger Input

When a route triggers a workflow, the `trigger.input` mapping extracts data from the HTTP request and makes it available as `input.*` inside the workflow.

```json
{
  "trigger": {
    "workflow": "create-user",
    "input": {
      "name": "{{ request.body.name }}",
      "email": "{{ request.body.email }}",
      "user_id": "{{ request.params.id }}"
    }
  }
}
```

Inside the workflow, these are accessed as `input.name`, `input.email`, `input.user_id`.

### Available Request Fields

| Field | Description |
|-------|-------------|
| `request.body` | Parsed JSON request body |
| `request.params` | URL path parameters (e.g. `:id`) |
| `request.query` | Query string parameters |
| `request.headers` | HTTP request headers |

## Node Outputs

Each node produces output data when it executes. The output is stored and accessible to downstream nodes via `nodes.<node_id>`.

### Accessing Node Data

```json
{
  "nodes": {
    "lookup": {
      "type": "db.findOne",
      "config": {
        "table": "users",
        "where": { "id": "{{ input.user_id }}" }
      }
    },
    "respond": {
      "type": "response.json",
      "config": {
        "status": 200,
        "body": {
          "name": "{{ nodes.lookup.name }}",
          "email": "{{ nodes.lookup.email }}"
        }
      }
    }
  }
}
```

The `respond` node accesses the `lookup` node's output via `nodes.lookup.name` and `nodes.lookup.email`.

### Node Aliases

Use the `"as"` field to give a node a more descriptive reference name:

```json
{
  "fetch_user": {
    "type": "db.findOne",
    "as": "user",
    "config": {
      "table": "users",
      "where": { "id": "{{ input.user_id }}" }
    }
  }
}
```

Now downstream nodes reference `nodes.user` instead of `nodes.fetch_user`.

## Output Ports and Edges

Nodes have named output ports (typically `success` and `error`). Edges connect a node's output port to the next node:

```json
{
  "edges": [
    { "from": "validate", "to": "create", "output": "success" },
    { "from": "validate", "to": "error_response", "output": "error" },
    { "from": "create", "to": "respond", "output": "success" }
  ]
}
```

Only the node connected to the triggered output port executes next.

### Special Output Ports

| Node Type | Outputs | Description |
|-----------|---------|-------------|
| Most nodes | `success`, `error` | Standard success/failure branching |
| `control.if` | `true`, `false` | Branches based on condition evaluation |
| `control.switch` | Case names | Branches to the matching case |

## Common Output Shapes

| Node Type | Success Output |
|-----------|---------------|
| `db.create` | The inserted row as an object, including generated fields (`id`, `created_at`) |
| `db.findOne` | Single row object, or `nil` if not found |
| `db.find` / `db.query` | Array of row objects |
| `db.update` / `db.delete` / `db.exec` | Object with `rows_affected` count |
| `transform.set` | Object with the specified fields |
| `util.uuid` | UUID v4 string |
| `util.jwt_sign` | Object with `token` string field |
| `upload.handle` | Object with `filename`, `size`, `content_type` |
| `http.request` | Object with `status`, `headers`, `body` |
| `cache.get` | The cached value |

## Complete Example

A 3-node workflow that creates a user and returns the result:

```json
{
  "id": "create-user",
  "nodes": {
    "generate_id": {
      "type": "transform.set",
      "config": {
        "fields": {
          "id": "{{ $uuid() }}",
          "created_at": "{{ now() }}"
        }
      }
    },
    "create": {
      "type": "db.create",
      "config": {
        "table": "users",
        "data": {
          "id": "{{ nodes.generate_id.id }}",
          "name": "{{ input.name }}",
          "email": "{{ input.email }}",
          "created_at": "{{ nodes.generate_id.created_at }}"
        }
      }
    },
    "respond": {
      "type": "response.json",
      "config": {
        "status": 201,
        "body": "{{ nodes.create }}"
      }
    }
  },
  "edges": [
    { "from": "generate_id", "to": "create", "output": "success" },
    { "from": "create", "to": "respond", "output": "success" }
  ]
}
```

**Data flow:**
1. `generate_id` runs first → outputs `{ "id": "a1b2c3...", "created_at": "2024-..." }` → stored as `nodes.generate_id`
2. `create` reads `nodes.generate_id.id` and `input.name`/`input.email` → inserts row → outputs the full row including DB-generated fields → stored as `nodes.create`
3. `respond` reads `nodes.create` → sends the entire created row as the HTTP response body

## Environment Variables

Access environment variables (including `.env` file values) via `env.*`:

```json
{
  "config": {
    "secret": "{{ env.JWT_SECRET }}",
    "api_url": "{{ env.EXTERNAL_API_URL }}"
  }
}
```
