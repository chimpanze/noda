# noda.json

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

## server

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `port` | integer | `3000` | HTTP listen port |
| `read_timeout` | string | `"30s"` | Read timeout (duration) |
| `write_timeout` | string | `"30s"` | Write timeout (duration) |
| `body_limit` | integer | `5242880` | Max request body size in bytes (5 MB) |
| `expression_memory_budget` | integer | `1000000` | Memory budget for expression evaluation (in allocation units). Limits array, map, and range allocations. Expressions exceeding this budget return an error. `0` uses the default |
| `expression_strict_mode` | boolean | `false` | When `true`, undefined variables in expressions produce compile errors instead of silently returning nil. Catches typos like `{{ auth.is_admim }}` at load time |

```json
{
  "server": {
    "port": 8080,
    "read_timeout": "60s",
    "write_timeout": "60s",
    "body_limit": 10485760,
    "expression_memory_budget": 2000000
  }
}
```

## services

Map of service instance name to service config. Each service connects to an external system via a plugin.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `plugin` | string | yes | Plugin identifier: `db`, `cache`, `storage`, `stream`, `pubsub`, `http`, `email`, `lk` |
| `config` | object | yes | Plugin-specific configuration |

### Database Service (`plugin: "db"`)

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

### Cache Service (`plugin: "cache"`)

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

### Storage Service (`plugin: "storage"`)

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

### Stream Service (`plugin: "stream"`)

| Config Field | Type | Required | Description |
|-------------|------|----------|-------------|
| `url` | string | yes | Redis URL (e.g., `redis://host:6379/0`) |
| `pool_size` | integer | no | Maximum number of connections in the pool |
| `min_idle` | integer | no | Minimum number of idle connections |

### PubSub Service (`plugin: "pubsub"`)

| Config Field | Type | Required | Description |
|-------------|------|----------|-------------|
| `url` | string | yes | Redis URL (e.g., `redis://host:6379/0`) |
| `pool_size` | integer | no | Maximum number of connections in the pool |
| `min_idle` | integer | no | Minimum number of idle connections |

### HTTP Client Service (`plugin: "http"`)

| Config Field | Type | Required | Description |
|-------------|------|----------|-------------|
| `base_url` | string | no | Base URL prepended to all requests |
| `timeout` | string | no | Default request timeout |
| `headers` | object | no | Default headers for all requests |

### LiveKit Service (`plugin: "lk"`)

| Config Field | Type | Required | Description |
|-------------|------|----------|-------------|
| `url` | string | yes | LiveKit server URL (e.g., `wss://myapp.livekit.cloud`) |
| `api_key` | string | yes | LiveKit API key |
| `api_secret` | string | yes | LiveKit API secret |

```json
{
  "services": {
    "lk": {
      "plugin": "lk",
      "config": {
        "url": "{{ $env('LIVEKIT_URL') }}",
        "api_key": "{{ $env('LIVEKIT_API_KEY') }}",
        "api_secret": "{{ $env('LIVEKIT_API_SECRET') }}"
      }
    }
  }
}
```

Noda acts as the **control plane only** — token generation, room management, egress/ingress orchestration, and webhook handling. Clients connect to LiveKit directly for media transport (WebRTC).

### Email Service (`plugin: "email"`)

| Config Field | Type | Required | Description |
|-------------|------|----------|-------------|
| `host` | string | yes | SMTP host |
| `port` | integer | yes | SMTP port |
| `username` | string | no | SMTP username |
| `password` | string | no | SMTP password |
| `from` | string | yes | Default sender address |

## security

### security.jwt

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `secret` | string | yes | JWT signing secret |
| `algorithm` | string | no | Signing algorithm (default: `"HS256"`) |
| `token_lookup` | string | no | Where to find the token (default: `"header:Authorization"`) |

### security.casbin

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
- **Response validation:** When a route defines `response.<status>.schema`, workflow responses are checked against the schema. The `response.validate` field controls behavior (`"warn"`, `"strict"`, or `false`). See [Route Config](routes.md) for details.
- **In-workflow validation:** Use `transform.validate` nodes to validate intermediate data against a JSON Schema within a workflow.
- **SQL injection prevention:** Database nodes validate table names, column names, ORDER BY clauses, JOIN types, and SQL fragments. Dangerous patterns (semicolons, comments) are rejected. Always pass dynamic values through `params`.
- **Redirect URL validation:** `response.redirect` rejects protocol-relative URLs and URLs containing newlines to prevent open redirects and header injection.
- **Recursion limits:** `workflow.run` and `control.loop` share a maximum recursion depth of 64 to prevent stack exhaustion.

## middleware_presets

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

## route_groups

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

## wasm_runtimes

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `module` | string | yes | Path to `.wasm` file |
| `tick_rate` | integer | no | Tick frequency in Hz (default: 20) |
| `encoding` | string | no | `"json"` or `"msgpack"` (default: `"json"`) |
| `services` | array | no | Service instances accessible from Wasm |
| `connections` | array | no | Connection endpoints accessible from Wasm |
| `allow_outbound` | object | no | Allowed outbound hosts |
| `config` | object | no | Opaque config passed to module's `initialize` |
| `tick_timeout` | string | no | Max duration for a single tick call (e.g. `"5s"`, `"500ms"`). Default: 10x tick budget |

```json
{
  "wasm_runtimes": {
    "game-server": {
      "module": "wasm/game.wasm",
      "tick_rate": 20,
      "tick_timeout": "500ms",
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
