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
| `port` | integer or string | `3000` | HTTP listen port. May be an integer or a string containing `{{ $env('NAME') }}` to read from an environment variable. |
| `read_timeout` | string | `"30s"` | Read timeout (duration) |
| `write_timeout` | string | `"30s"` | Write timeout (duration) |
| `body_limit` | integer or string | `5242880` | Max request body size in bytes (5 MB). May be an integer or a string containing `{{ $env('NAME') }}` to read from an environment variable. |
| `expression_memory_budget` | integer or string | `1000000` | Memory budget for expression evaluation (in allocation units). Limits array, map, and range allocations. Expressions exceeding this budget return an error. `0` uses the default. May be an integer or a string containing `{{ $env('NAME') }}` to read from an environment variable. |
| `expression_strict_mode` | boolean | `false` | When `true`, undefined variables in expressions produce compile errors instead of silently returning nil. Catches typos like `{{ auth.is_admim }}` at load time |
| `health_timeout` | string | `"5s"` | Timeout for health check calls to services (duration). Prevents hung service checks from blocking the health endpoint. |
| `trust_proxy` | object | `{}` | Trusted proxy configuration (see subsection below). Off by default. |

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

### Trusted proxies (`server.trust_proxy`)

When noda runs behind a reverse proxy (Caddy, nginx, a cloud load balancer),
the client IP seen by rate limiting and session tracking is the proxy's IP
unless you tell noda which peers to trust:

```json
{
  "server": {
    "trust_proxy": {
      "enabled": true,
      "proxies": ["10.0.0.0/8"],
      "private": true,
      "header": "X-Forwarded-For"
    }
  }
}
```

| Field | Type | Default | Description |
|---|---|---|---|
| `enabled` | boolean | `false` | Master switch. Off = header never trusted. |
| `proxies` | string[] | `[]` | Trusted proxy IPs or CIDR ranges. |
| `loopback` | boolean | `false` | Trust all loopback addresses (127.0.0.0/8, ::1). |
| `link_local` | boolean | `false` | Trust link-local ranges (169.254.0.0/16, fe80::/10). |
| `private` | boolean | `false` | Trust private ranges (10/8, 172.16/12, 192.168/16, fc00::/7) — handy for Docker networks where the proxy IP is dynamic. |
| `header` | string | `"X-Forwarded-For"` | Header the proxy writes the client IP to. |

Only requests arriving **from** a trusted address have the header honored;
direct clients spoofing `X-Forwarded-For` keep their socket IP. Enabling
`trust_proxy` without any trusted set (no `proxies`, no class flag) is a
config error. Only enable this when every hop that can reach noda's port is
your own proxy.

> **Memory note:** the server buffers each request body in memory up to
> `body_limit` *before* auth runs, so a large limit is an unauthenticated
> memory-pressure vector. Keep the edge proxy's own body limit at or below
> noda's, and don't raise `body_limit` beyond what uploads actually need.

**Prometheus metrics** are served on the same port at `/metrics` when enabled via `observability.metrics.enabled`. See [Observability](../04-guides/observability.md) for the metric list and scrape config.

## services

Map of service instance name to service config. Each service connects to an external system via a plugin.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `plugin` | string | yes | Plugin identifier — see table below |
| `config` | object | yes | Plugin-specific configuration |

### Built-in plugin identifiers

Use the exact identifier in the left column when writing `"plugin": "..."`. A wrong name (e.g. `"redis"` instead of `"cache"`) fails at startup with an `unknown plugin` error.

| Identifier | Connects to | Node prefix | Use case |
|---|---|---|---|
| `db` | PostgreSQL | `db.*` | Relational queries, transactions |
| `cache` | Redis | `cache.*` | Key/value cache, TTL |
| `stream` | Redis Streams | `stream.*` | Durable event streams with consumer groups |
| `pubsub` | Redis Pub/Sub | `pubsub.*` | Fire-and-forget broadcast |
| `storage` | Local FS / S3 (via Afero) | `storage.*` | File read/write/list/delete |
| `http` | Outbound HTTP | `http.*` | Call external APIs; use `base_url` + relative URLs |
| `email` | SMTP | `email.*` | Send mail |
| `image` | libvips (via bimg) | `image.*` | Resize, convert, metadata |
| `lk` | LiveKit | `lk.*` | Video rooms, room tokens |

Node types use the **prefix** column (e.g. `db.query`, `http.get`). The plugin identifier goes in `noda.json`; node types go in workflows.

### Database Service (`plugin: "db"`)

| Config Field | Type | Required | Description |
|-------------|------|----------|-------------|
| `url` | string | yes | PostgreSQL connection URL (e.g., `postgres://user:pass@host:5432/db`) |

```json
{
  "services": {
    "postgres": {
      "plugin": "db",
      "config": {
        "url": "{{ $env('DATABASE_URL') }}"
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

| Config Field | Type | Default | Description |
|-------------|------|---------|-------------|
| `timeout` | string | `"30s"` | Per-request timeout (Go duration). |
| `base_url` | string | `""` | Prepended to relative URLs. Must use `http://` or `https://`. |
| `headers` | object | `{}` | Default headers applied to every request. |
| `allow_private_networks` | bool | `false` | If true, lifts the deny on RFC1918 / loopback / link-local / IPv6-ULA / CGN. The two cloud-metadata IPs remain blocked. |
| `allowed_hosts` | []string | `[]` | Exact hostname bypass for the deny list. No globs. The two cloud-metadata IPs remain blocked even if a hostname here resolves to one. |
| `redirects` | string | `"strip_auth"` | One of `"none"` (don't follow), `"same_origin"` (follow within same scheme/host/port), `"strip_auth"` (follow, removing `Authorization`/`Cookie`/`Proxy-Authorization`/`X-Api-Key`/`X-Auth-Token` and any `X-*-Token`/`X-*-Key` on cross-origin hops). |
| `max_redirects` | int | `10` | Hop limit for `same_origin` and `strip_auth`. Range `[0, 50]`. |

Example for an HTTP service that needs to call an internal `/metrics` endpoint:

```json
{
  "services": {
    "internal": {
      "plugin": "http",
      "config": {
        "base_url": "http://prometheus.internal",
        "allowed_hosts": ["prometheus.internal"]
      }
    }
  }
}
```

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

### Auth Service (`plugin: "auth"`)

Backs the 8 `auth.*` nodes and the `auth.session` middleware. Has no DB handle of its own — `database` names the `db`-plugin service that the auth tables (`auth_users`, `auth_sessions`, `auth_tokens`) live in; nodes still declare their own `database` service dependency the same way every db-dependent node does.

| Config Field | Type | Required | Default | Description |
|-------------|------|----------|---------|-------------|
| `database` | string | yes | — | Name of the `db`-plugin service holding the auth tables |
| `session.ttl` | duration string | no | `"720h"` | Session lifetime |
| `session.cookie.name` | string | no | `"noda_session"` | Session cookie name |
| `session.cookie.path` | string | no | `"/"` | Cookie path |
| `session.cookie.domain` | string | no | `""` (unset) | Cookie domain |
| `session.cookie.same_site` | string | no | `"Lax"` | Cookie `SameSite` attribute |
| `session.cookie.secure` | boolean | no | `true` | Cookie `Secure` attribute |
| `session.cookie.http_only` | boolean | no | `true` | Cookie `HttpOnly` attribute |
| `argon2.memory_kib` | integer | no | `65536` | Argon2id memory cost (KiB) |
| `argon2.iterations` | integer | no | `3` | Argon2id time cost |
| `argon2.parallelism` | integer | no | `2` | Argon2id parallelism |
| `argon2.salt_len` | integer | no | `16` | Salt length (bytes) |
| `argon2.key_len` | integer | no | `32` | Derived key length (bytes) |
| `tokens.verify_email_ttl` | duration string | no | `"24h"` | Email-verification token lifetime |
| `tokens.reset_password_ttl` | duration string | no | `"1h"` | Password-reset token lifetime |

```json
{
  "services": {
    "auth": {
      "plugin": "auth",
      "config": {
        "database": "postgres",
        "session": {
          "ttl": "720h",
          "cookie": {
            "name": "noda_session",
            "secure": true,
            "http_only": true,
            "same_site": "Lax",
            "path": "/"
          }
        },
        "argon2": { "memory_kib": 65536, "iterations": 3, "parallelism": 2, "salt_len": 16, "key_len": 32 },
        "tokens": { "verify_email_ttl": "24h", "reset_password_ttl": "1h" }
      }
    }
  }
}
```

All fields are optional except `database`; defaults are as shown above and follow OWASP recommendations for argon2id. `noda auth init` writes a minimal `services.auth` block (`{"plugin": "auth", "config": {"database": "<detected db service>"}}`) and leaves the rest to defaults — edit `noda.json` directly to override any of them. See [the authentication guide](../04-guides/authentication.md) for the full `noda auth init` walkthrough.

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
    "public": ["security.cors", "limiter"],
    "admin": ["auth.jwt", "casbin.enforce"]
  }
}
```

See [`docs/02-config/middleware.md`](middleware.md) for the canonical list of registered middleware names.

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
