# Config Reference

This document covers every config file format in Noda with all fields, types, defaults, and examples.

## Config Directory Structure

```
project/
├── noda.json              # Root config (required)
├── vars.json              # Shared variables (optional)
├── routes/*.json          # HTTP route definitions
├── workflows/*.json       # Workflow DAGs
├── workers/*.json         # Event-driven worker subscriptions
├── schedules/*.json       # Cron job definitions
├── connections/*.json     # WebSocket and SSE endpoints
├── schemas/*.json         # JSON Schema definitions
├── tests/*.json           # Workflow test suites
├── migrations/*.sql       # SQL migration files
└── wasm/*.wasm            # Wasm modules
```

Noda discovers config files automatically from the config directory. Environment-specific overlays can be applied via `.env.json` or `--env` flag.

---

## noda.json

The root config file. All fields are optional except where noted.

```json
{
  "server": { ... },
  "services": { ... },
  "security": { ... },
  "middleware_presets": { ... },
  "route_groups": { ... },
  "wasm_runtimes": { ... }
}
```

### server

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `port` | integer | `3000` | HTTP listen port |
| `read_timeout` | string | `"30s"` | Read timeout (duration) |
| `write_timeout` | string | `"30s"` | Write timeout (duration) |
| `body_limit` | integer | `5242880` | Max request body size in bytes (5 MB) |

```json
{
  "server": {
    "port": 8080,
    "read_timeout": "60s",
    "write_timeout": "60s",
    "body_limit": 10485760
  }
}
```

### services

Map of service instance name to service config. Each service connects to an external system via a plugin.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `plugin` | string | yes | Plugin identifier: `db`, `cache`, `storage`, `stream`, `pubsub`, `http`, `email` |
| `config` | object | yes | Plugin-specific configuration |

#### Database Service (`plugin: "db"`)

| Config Field | Type | Required | Description |
|-------------|------|----------|-------------|
| `driver` | string | yes | Database driver: `"postgres"` |
| `dsn` | string | yes | Connection string |

```json
{
  "services": {
    "postgres": {
      "plugin": "db",
      "config": {
        "driver": "postgres",
        "dsn": "{{ $env('DATABASE_URL') }}"
      }
    }
  }
}
```

#### Cache Service (`plugin: "cache"`)

| Config Field | Type | Required | Description |
|-------------|------|----------|-------------|
| `url` | string | yes | Redis URL (e.g., `redis://user:pass@host:6379/0`) |
| `pool_size` | integer | no | Maximum number of connections in the pool |
| `min_idle` | integer | no | Minimum number of idle connections |

```json
{
  "services": {
    "redis": {
      "plugin": "cache",
      "config": {
        "url": "{{ $env('REDIS_URL') }}",
        "pool_size": 20,
        "min_idle": 5
      }
    }
  }
}
```

#### Storage Service (`plugin: "storage"`)

| Config Field | Type | Required | Description |
|-------------|------|----------|-------------|
| `backend` | string | yes | `"local"` or `"memory"` |
| `base_path` | string | for local | Root directory for local storage |

```json
{
  "services": {
    "files": {
      "plugin": "storage",
      "config": {
        "backend": "local",
        "base_path": "./uploads"
      }
    }
  }
}
```

#### Stream Service (`plugin: "stream"`)

| Config Field | Type | Required | Description |
|-------------|------|----------|-------------|
| `url` | string | yes | Redis URL (e.g., `redis://host:6379/0`) |
| `pool_size` | integer | no | Maximum number of connections in the pool |
| `min_idle` | integer | no | Minimum number of idle connections |

#### PubSub Service (`plugin: "pubsub"`)

| Config Field | Type | Required | Description |
|-------------|------|----------|-------------|
| `url` | string | yes | Redis URL (e.g., `redis://host:6379/0`) |
| `pool_size` | integer | no | Maximum number of connections in the pool |
| `min_idle` | integer | no | Minimum number of idle connections |

#### HTTP Client Service (`plugin: "http"`)

| Config Field | Type | Required | Description |
|-------------|------|----------|-------------|
| `base_url` | string | no | Base URL prepended to all requests |
| `timeout` | string | no | Default request timeout |
| `headers` | object | no | Default headers for all requests |

#### Email Service (`plugin: "email"`)

| Config Field | Type | Required | Description |
|-------------|------|----------|-------------|
| `host` | string | yes | SMTP host |
| `port` | integer | yes | SMTP port |
| `username` | string | no | SMTP username |
| `password` | string | no | SMTP password |
| `from` | string | yes | Default sender address |

### security

#### security.jwt

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `secret` | string | yes | JWT signing secret |
| `algorithm` | string | no | Signing algorithm (default: `"HS256"`) |
| `token_lookup` | string | no | Where to find the token (default: `"header:Authorization"`) |

#### security.casbin

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `model` | string | yes | Casbin model definition |
| `policies` | array | yes | Policy rule tuples |

```json
{
  "security": {
    "jwt": {
      "secret": "{{ $env('JWT_SECRET') }}",
      "algorithm": "HS256",
      "token_lookup": "header:Authorization"
    },
    "casbin": {
      "model": "...",
      "policies": [
        ["p", "admin", "/api/*", "*"],
        ["p", "user", "/api/tasks", "GET"]
      ]
    }
  }
}
```

### security.validation

Noda enforces input and output validation at multiple levels:

- **Request body validation:** When a route defines `body.schema`, request bodies are validated against the JSON Schema before the workflow runs. Invalid requests receive a `422` response with field-level errors. Set `body.validate: false` to use the schema for documentation only.
- **Response validation:** When a route defines `response.<status>.schema`, workflow responses are checked against the schema. The `response.validate` field controls behavior (`"warn"`, `"strict"`, or `false`). See [Route Config](#route-config) for details.
- **In-workflow validation:** Use `transform.validate` nodes to validate intermediate data against a JSON Schema within a workflow.
- **SQL injection prevention:** Database nodes validate table names, column names, ORDER BY clauses, JOIN types, and SQL fragments. Dangerous patterns (semicolons, comments) are rejected. Always pass dynamic values through `params`.
- **Redirect URL validation:** `response.redirect` rejects protocol-relative URLs and URLs containing newlines to prevent open redirects and header injection.
- **Recursion limits:** `workflow.run` and `control.loop` share a maximum recursion depth of 64 to prevent stack exhaustion.

### middleware_presets

Named collections of middleware for reuse across routes and route groups.

```json
{
  "middleware_presets": {
    "authenticated": ["auth.jwt"],
    "public": ["cors", "rate_limit"],
    "admin": ["auth.jwt", "auth.casbin"]
  }
}
```

Available middleware: `auth.jwt`, `auth.casbin`, `cors`, `rate_limit`, `helmet`, `compress`, `etag`.

### route_groups

Apply middleware presets to URL path prefixes.

```json
{
  "route_groups": {
    "/api/admin": {
      "middleware_preset": "admin"
    },
    "/api": {
      "middleware_preset": "authenticated"
    }
  }
}
```

### wasm_runtimes

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `module` | string | yes | Path to `.wasm` file |
| `tick_rate` | integer | no | Tick frequency in Hz (default: 20) |
| `encoding` | string | no | `"json"` or `"msgpack"` (default: `"json"`) |
| `services` | array | no | Service instances accessible from Wasm |
| `connections` | array | no | Connection endpoints accessible from Wasm |
| `allow_outbound` | object | no | Allowed outbound hosts |
| `config` | object | no | Opaque config passed to module's `initialize` |

```json
{
  "wasm_runtimes": {
    "game-server": {
      "module": "wasm/game.wasm",
      "tick_rate": 20,
      "encoding": "msgpack",
      "services": ["redis", "postgres"],
      "connections": ["game-ws"],
      "allow_outbound": {
        "http": ["api.example.com"],
        "ws": ["gateway.discord.gg"]
      },
      "config": {
        "max_players": 100
      }
    }
  }
}
```

---

## Route Config

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

---

## Workflow Config

Files in `workflows/*.json`. Each file defines one workflow.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `id` | string | yes | Unique workflow identifier |
| `name` | string | no | Display name |
| `nodes` | object | yes | Map of node ID to node definition |
| `edges` | array | yes | Execution flow edges |

### Node Definition

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `type` | string | yes | Node type (e.g., `"db.query"`, `"control.if"`) |
| `services` | object | no | Service slot mappings |
| `config` | object | yes | Node-specific configuration |

### Edge Definition

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

---

## Worker Config

Files in `workers/*.json`. Each file defines one event-driven worker.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `id` | string | yes | Unique worker identifier |
| `services` | object | yes | Stream or PubSub service binding |
| `subscribe` | object | yes | Subscription configuration |
| `subscribe.topic` | string | yes | Topic or stream name |
| `subscribe.group` | string | yes | Consumer group name |
| `concurrency` | integer | no | Concurrent message processing (default: 1) |
| `retry` | object | no | Retry configuration |
| `retry.max_attempts` | integer | no | Max delivery attempts |
| `retry.dlq` | string | no | Dead letter queue topic |
| `trigger` | object | yes | Workflow trigger |

```json
{
  "id": "order-processor",
  "services": {
    "stream": "redis-stream"
  },
  "subscribe": {
    "topic": "orders.created",
    "group": "order-processors"
  },
  "concurrency": 5,
  "retry": {
    "max_attempts": 3,
    "dlq": "orders.failed"
  },
  "trigger": {
    "workflow": "process-order",
    "input": {
      "order_id": "{{ message.payload.order_id }}"
    }
  }
}
```

---

## Schedule Config

Files in `schedules/*.json`.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `id` | string | yes | Unique schedule identifier |
| `cron` | string | yes | Cron expression |
| `trigger` | object | yes | Workflow trigger |

```json
{
  "id": "daily-cleanup",
  "cron": "0 2 * * *",
  "trigger": {
    "workflow": "cleanup-expired",
    "input": {}
  }
}
```

---

## Connection Config

Files in `connections/*.json`. Defines WebSocket and SSE endpoints.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `sync` | object | no | Cross-instance sync service |
| `endpoints` | object | yes | Map of endpoint ID to endpoint definition |

### Endpoint Definition

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `type` | string | yes | `"websocket"` or `"sse"` |
| `path` | string | yes | URL path (supports `:param`) |
| `middleware` | array | no | Endpoint middleware |
| `channels` | object | no | Channel configuration |
| `channels.pattern` | string | no | Channel name pattern (expression) |
| `channels.max_per_channel` | integer | no | Max connections per channel |
| `ping_interval` | string | no | WebSocket ping interval |
| `on_connect` | string | no | Workflow ID on connection |
| `on_message` | string | no | Workflow ID on message |
| `on_disconnect` | string | no | Workflow ID on disconnect |

```json
{
  "sync": {
    "pubsub": "redis-pubsub"
  },
  "endpoints": {
    "chat": {
      "type": "websocket",
      "path": "/ws/chat/:room",
      "middleware": ["auth.jwt"],
      "channels": {
        "pattern": "chat.{{ request.params.room }}",
        "max_per_channel": 50
      },
      "ping_interval": "30s",
      "on_connect": "chat-join",
      "on_message": "chat-message",
      "on_disconnect": "chat-leave"
    }
  }
}
```

---

## Test Config

Files in `tests/*.json`. Each file defines a test suite for one workflow.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `id` | string | yes | Test suite identifier |
| `workflow` | string | yes | Workflow under test |
| `tests` | array | yes | Test cases |

### Test Case

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | yes | Test name |
| `input` | object | no | Workflow input data |
| `mocks` | object | no | Node output mocks (node ID → output) |
| `expect` | object | yes | Expectations |
| `expect.status` | string | no | `"success"` or `"error"` |
| `expect.output` | object | no | Expected output values |

```json
{
  "id": "test-create-task",
  "workflow": "create-task",
  "tests": [
    {
      "name": "creates task with valid input",
      "input": { "title": "Test task" },
      "mocks": {
        "insert": {
          "output": { "id": 1, "title": "Test task", "completed": false }
        }
      },
      "expect": {
        "status": "success"
      }
    },
    {
      "name": "fails with empty title",
      "input": { "title": "" },
      "expect": {
        "status": "error"
      }
    }
  ]
}
```

---

## Schema Config

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

---

## Shared Variables (`vars.json`)

Define named values in a `vars.json` file at the project root to avoid repeating strings across config files:

```json
{
  "MAIN_DB": "main-db",
  "TOPIC_MEMBER_INVITED": "member.invited",
  "TABLE_TASKS": "tasks"
}
```

All values must be strings. Reference them with `$var()` in any config section:

```json
{
  "subscribe": {
    "topic": "{{ $var('TOPIC_MEMBER_INVITED') }}"
  }
}
```

### How it works

- **Standalone** `{{ $var('X') }}` is resolved at **config load time** — the entire field value is replaced before the workflow is loaded
- **Inside expressions** like `{{ "prefix." + $var('TOPIC') }}`, `$var()` is a **runtime function** evaluated by the expression engine
- Config-time resolution works across **all** config sections: root, routes, workflows, workers, schedules, connections, tests, and models
- Resolution happens after `$env()` and before `$ref`, so you can use environment variables inside `vars.json` values but not `$var()` inside `$ref` targets
- An undefined variable name produces a load error (config-time) or runtime error (expression) with the variable name

### When to use `$var()` vs `$env()`

| | `$var()` | `$env()` |
|---|---|---|
| **Source** | `vars.json` (checked into version control) | OS environment / `.env` file |
| **Scope** | All config sections | Root config only |
| **Use case** | Shared logical names (topics, tables, service names) | Secrets and environment-specific values (DSNs, keys) |

### Example

```
vars.json
```
```json
{
  "STREAM_SVC": "redis-stream",
  "TOPIC_TASK_CREATED": "task.created",
  "TOPIC_TASK_FAILED": "task.failed"
}
```

```
workers/process-task.json
```
```json
{
  "id": "process-task",
  "services": { "stream": "{{ $var('STREAM_SVC') }}" },
  "subscribe": { "topic": "{{ $var('TOPIC_TASK_CREATED') }}" },
  "dead_letter": { "topic": "{{ $var('TOPIC_TASK_FAILED') }}" },
  "trigger": { "workflow": "process-task" }
}
```

---

## Environment Variables and Overlays

### `$env()` Function

Use `$env('VAR_NAME')` in any string value in the root config to reference environment variables:

```json
{
  "dsn": "{{ $env('DATABASE_URL') }}"
}
```

**Note:** `$env()` only resolves in `noda.json` (and its overlay). For values needed across all config sections, define them in `vars.json` using `$var()` instead.

### `.env` File

Noda auto-loads `.env` files from the config directory.

### Environment Overlays

Create environment-specific overlays that merge on top of base config:

```bash
noda start --env production
```

This loads `noda.json` first, then deep-merges `noda.production.json` on top.

---

## Config Conventions

- **All field names** use `snake_case`
- **Duration values**: `"5s"`, `"100ms"`, `"1m"` (units: ms, s, m)
- **Size values**: `"10mb"`, `"64kb"`, `"1gb"` (units: kb, mb, gb)
- **Array fields** use plural names: `params`, `cases`, `fields`, `headers`, `cookies`
- **Static fields** (never expressions): `mode`, `cases`, `workflow`, `method`, `type`, `backoff`
- **Expression fields**: everything else that evaluates at runtime
