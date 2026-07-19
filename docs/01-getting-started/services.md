# Service Wiring Guide

## What Are Services?

Services are named, configured connections to external systems -- databases, caches, file storage, email providers, and more. Noda creates each service at startup based on your `noda.json` configuration and injects them into workflows and nodes at runtime.

## Configuring Services

Services live in the `services` section of `noda.json`. Each key is the **instance name** you choose, and the value specifies which plugin to use and its configuration:

```json
{
  "services": {
    "postgres": {
      "plugin": "db",
      "config": {
        "url": "{{ $env('DATABASE_URL') }}"
      }
    },
    "redis": {
      "plugin": "cache",
      "config": {
        "url": "{{ $env('REDIS_URL') }}"
      }
    }
  }
}
```

- **`plugin`** -- the plugin type that manages this service (e.g. `"db"`, `"cache"`, `"storage"`).
- **`config`** -- plugin-specific fields. Expressions like `$env('...')` are resolved at startup.
- The top-level key (e.g. `"postgres"`, `"redis"`) is the instance name you reference from routes, workflows, and nodes.

---

## Service Reference

### Database (`plugin: "db"`)

Provides PostgreSQL and SQLite access via GORM. Used by all `db.*` nodes.

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `driver` | string | no | `"postgres"` | Database driver: `"postgres"` or `"sqlite"` |
| `url` | string | yes (postgres) | -- | PostgreSQL connection URL |
| `path` | string | yes (sqlite) | -- | Path to SQLite database file |
| `max_open` | integer | no | 25 | Maximum open connections |
| `max_idle` | integer | no | 5 | Maximum idle connections |
| `conn_lifetime` | string | no | `"5m"` | Connection max lifetime (Go duration) |

```json
{
  "postgres": {
    "plugin": "db",
    "config": {
      "driver": "postgres",
      "url": "{{ $env('DATABASE_URL') }}",
      "max_open": 25,
      "max_idle": 5,
      "conn_lifetime": "5m"
    }
  }
}
```

SQLite example:

```json
{
  "local-db": {
    "plugin": "db",
    "config": {
      "driver": "sqlite",
      "path": "./data/app.db"
    }
  }
}
```

**Nodes:** `db.query`, `db.exec`, `db.create`, `db.update`, `db.delete`, `db.find`, `db.findOne`, `db.count`, `db.upsert`

---

### Cache (`plugin: "cache"`)

Redis-backed key-value cache. Used by all `cache.*` nodes.

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `url` | string | yes | -- | Redis URL (e.g. `redis://host:6379/0`) |
| `pool_size` | integer | no | Redis default | Maximum connections in the pool |
| `min_idle` | integer | no | Redis default | Minimum idle connections |

```json
{
  "redis": {
    "plugin": "cache",
    "config": {
      "url": "{{ $env('REDIS_URL') }}",
      "pool_size": 20,
      "min_idle": 5
    }
  }
}
```

**Nodes:** `cache.get`, `cache.set`, `cache.del`, `cache.exists`

---

### Storage (`plugin: "storage"`)

File storage backed by the local filesystem or in-memory. Used by `storage.*`, `upload.handle`, and `image.*` nodes.

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `backend` | string | no | `"local"` | Backend type: `"local"` or `"memory"` |
| `path` | string | yes (local) | -- | Root directory for local storage |

```json
{
  "files": {
    "plugin": "storage",
    "config": {
      "backend": "local",
      "path": "./uploads"
    }
  }
}
```

**Nodes:** `storage.read`, `storage.write`, `storage.list`, `storage.delete`
**Also used by:** `upload.handle` (slot `destination`), `image.*` nodes (slots `source` and `target`)

---

### Redis Streams (`plugin: "stream"`)

Redis Streams for event-driven workers. Used by workers and `event.emit`.

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `url` | string | yes | -- | Redis URL |
| `pool_size` | integer | no | Redis default | Maximum connections in the pool |
| `min_idle` | integer | no | Redis default | Minimum idle connections |

```json
{
  "redis-stream": {
    "plugin": "stream",
    "config": {
      "url": "{{ $env('REDIS_URL') }}"
    }
  }
}
```

**Nodes:** `event.emit`
**Also used by:** Worker configs (slot `stream`)

---

### Redis PubSub (`plugin: "pubsub"`)

Redis Pub/Sub for real-time messaging. Used by `event.emit` (in pubsub mode) and for WebSocket/SSE cross-instance sync.

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `url` | string | yes | -- | Redis URL |
| `pool_size` | integer | no | Redis default | Maximum connections in the pool |
| `min_idle` | integer | no | Redis default | Minimum idle connections |

```json
{
  "redis-pubsub": {
    "plugin": "pubsub",
    "config": {
      "url": "{{ $env('REDIS_URL') }}"
    }
  }
}
```

**Nodes:** `event.emit` (in pubsub mode)
**Also used by:** WebSocket/SSE cross-instance message routing

---

### HTTP Client (`plugin: "http"`)

Outbound HTTP client with optional circuit breaker. Used by all `http.*` nodes.

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `timeout` | string or number | no | `"30s"` | Request timeout (Go duration string or seconds as number) |
| `base_url` | string | no | -- | Base URL prepended to all requests (must use `http://` or `https://`) |
| `headers` | object | no | -- | Default headers sent with every request |
| `circuit_breaker` | object | no | -- | Circuit breaker configuration (see below) |
| `allow_private_networks` | boolean | no | `false` | Allow requests to private/loopback IP ranges (SSRF protection is on by default) |
| `allowed_hosts` | array of strings | no | -- | Allowlist of bare hostnames (no scheme/port/path, no IP literals) the client may reach |
| `redirects` | string | no | `"strip_auth"` | Redirect policy: `"none"`, `"same_origin"`, or `"strip_auth"` (follow but drop auth headers cross-origin) |
| `max_redirects` | integer | no | `10` | Maximum redirects to follow (0–50) |

Circuit breaker fields (inside `circuit_breaker`):

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `max_requests` | integer | -- | Max requests allowed in half-open state |
| `interval` | string | -- | Interval for clearing failure counts (Go duration) |
| `timeout` | string | -- | Time to wait before moving from open to half-open |
| `threshold` | integer | -- | Number of failures before opening the circuit |

```json
{
  "external-api": {
    "plugin": "http",
    "config": {
      "base_url": "https://api.example.com",
      "timeout": "10s",
      "headers": {
        "Authorization": "Bearer {{ $env('API_TOKEN') }}"
      },
      "circuit_breaker": {
        "threshold": 5,
        "timeout": "30s"
      }
    }
  }
}
```

**Nodes:** `http.request`, `http.get`, `http.post`

> **Use `base_url` + relative paths.** Set the host once on the service, then use relative URLs (`/users/{{ input.id }}`) in every `http.*` node. Do **not** put `{{ $env('API_URL') }}/users/...` inside per-workflow URLs — `$env()` doesn't resolve in workflow expressions anyway (it's root-config-only), and centralizing the host makes environment switching trivial. For a full proxy walkthrough see the [proxy cookbook](../04-guides/proxy-cookbook.md).

---

### Email (`plugin: "email"`)

SMTP email sender. Used by the `email.send` node.

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `host` | string | yes | -- | SMTP server hostname |
| `port` | integer | no | 587 | SMTP server port |
| `username` | string | no | -- | SMTP username |
| `password` | string | no | -- | SMTP password |
| `from` | string | no | -- | Default sender address |
| `tls` | boolean | no | port-dependent | Use implicit TLS (SMTPS). Defaults to `true` only for port 465; `false` for every other port, including 587. When `false`, the connection is still upgraded via STARTTLS if the server offers it. Set explicitly to override. |

```json
{
  "smtp": {
    "plugin": "email",
    "config": {
      "host": "{{ $env('SMTP_HOST') }}",
      "port": 587,
      "username": "{{ $env('SMTP_USER') }}",
      "password": "{{ $env('SMTP_PASS') }}",
      "from": "noreply@example.com"
    }
  }
}
```

**Nodes:** `email.send`

---

### Auth (`plugin: "auth"`)

First-party email+password authentication (users, opaque sessions, single-use tokens). Used by the 8 `auth.*` nodes and the `auth.session` middleware. See the [Authentication guide](../04-guides/authentication.md#session-authentication-auth-plugin) for the full walkthrough and `noda auth init`.

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `database` | string | yes | -- | Name of the db service (`services.*`) the auth plugin stores its `auth_users`/`auth_sessions`/`auth_tokens` tables in |
| `session.ttl` | string | no | `720h` | Session lifetime as a Go duration |
| `session.cookie.name` | string | no | `noda_session` | Session cookie name |
| `session.cookie.path` | string | no | `/` | Session cookie path |
| `session.cookie.domain` | string | no | (empty = host-only) | Session cookie domain |
| `session.cookie.same_site` | string | no | `Lax` | SameSite attribute; conventional values are `"Lax"`, `"Strict"`, `"None"` (passed through as-is) |
| `session.cookie.secure` | boolean | no | `true` | Secure attribute |
| `session.cookie.http_only` | boolean | no | `true` | HttpOnly attribute |
| `argon2.memory_kib` | integer | no | library default | Argon2id memory cost in KiB |
| `argon2.iterations` | integer | no | library default | Argon2id number of iterations |
| `argon2.salt_len` | integer | no | library default | Argon2id salt length in bytes |
| `argon2.key_len` | integer | no | library default | Argon2id derived key length in bytes |
| `argon2.parallelism` | integer | no | library default | Argon2id degree of parallelism |
| `tokens.verify_email_ttl` | string | no | `24h` | Email verification token TTL as a Go duration |
| `tokens.reset_password_ttl` | string | no | `1h` | Password reset token TTL as a Go duration |

```json
{
  "auth": {
    "plugin": "auth",
    "config": {
      "database": "main-db",
      "session": {
        "ttl": "720h",
        "cookie": { "name": "noda_session", "same_site": "Lax" }
      },
      "argon2": { "memory_kib": 65536, "iterations": 3, "parallelism": 2 },
      "tokens": { "verify_email_ttl": "24h", "reset_password_ttl": "1h" }
    }
  }
}
```

**Nodes:** `auth.create_user`, `auth.get_user`, `auth.verify_credentials`, `auth.create_session`, `auth.revoke_session`, `auth.create_token`, `auth.consume_token`, `auth.set_password`

The machine-readable version of this table (and every other plugin's) is available via the `noda_get_service_schema` MCP tool.

---

### LiveKit (`plugin: "livekit"`)

LiveKit WebRTC server integration. Used by all `lk.*` nodes.

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `url` | string | yes | -- | LiveKit server URL |
| `api_key` | string | yes | -- | LiveKit API key |
| `api_secret` | string | yes | -- | LiveKit API secret |
| `timeout` | string | no | -- | Per-API-call deadline as a Go duration (e.g. `"5s"`). Applies to every Room/Egress/Ingress client call. Unset means no deadline (today's behavior) — a call can block as long as the LiveKit server takes to respond, which matters for egress/ingress calls against a server with no available worker. |

```json
{
  "lk": {
    "plugin": "livekit",
    "config": {
      "url": "{{ $env('LIVEKIT_URL') }}",
      "api_key": "{{ $env('LIVEKIT_API_KEY') }}",
      "api_secret": "{{ $env('LIVEKIT_API_SECRET') }}",
      "timeout": "5s"
    }
  }
}
```

**Nodes:** `lk.token`, `lk.roomCreate`, `lk.roomList`, `lk.roomDelete`, `lk.roomUpdateMetadata`, `lk.sendData`, `lk.participantList`, `lk.participantGet`, `lk.participantRemove`, `lk.participantUpdate`, `lk.muteTrack`, `lk.egressStartRoomComposite`, `lk.egressStartTrack`, `lk.egressStop`, `lk.egressList`, `lk.ingressCreate`, `lk.ingressList`, `lk.ingressDelete`

---

### Image (`plugin: "image"`)

Stateless image processing via libvips. This plugin has **no service configuration** -- it does not connect to any external system. Image nodes use storage services (via `source` and `destination` slots) for reading and writing files.

**Nodes:** `image.resize`, `image.crop`, `image.watermark`, `image.convert`, `image.thumbnail`

---

## Wiring Services to Nodes

Each node that needs a service declares a **slot** -- a logical role name. In the workflow node definition, you map each slot to a service instance name:

```json
{
  "type": "db.query",
  "services": { "database": "postgres" },
  "config": {
    "table": "users",
    "where": { "id": "{{ input.user_id }}" }
  }
}
```

Here `"database"` is the slot name that `db.*` nodes expect, and `"postgres"` is the service instance name from `noda.json`.

The slot name is fixed by the plugin -- you cannot change it. The instance name is whatever you chose when you defined the service in `noda.json`.

### Slot Names by Plugin

Different node families expect different slot names:

| Slot Name | Used By | Service Plugin |
|-----------|---------|----------------|
| `database` | `db.*` nodes | `db` |
| `cache` | `cache.*` nodes | `cache` |
| `storage` | `storage.*` nodes | `storage` |
| `destination` | `upload.handle` | `storage` |
| `source` | `image.*` nodes (input) | `storage` |
| `target` | `image.*` nodes (output) | `storage` |
| `pubsub` | `event.emit` (pubsub mode) | `pubsub` |
| `client` | `http.*` nodes | `http` |
| `mailer` | `email.send` | `email` |
| `stream` | `event.emit`, workers | `stream` |
| `auth` | `auth.*` nodes | `auth` |
| `livekit` | `lk.*` nodes | `livekit` |
| `connections` | `ws.send`, `sse.send` | A **connections endpoint name** (see below), not a `noda.json` service |
| `runtime` | `wasm.send`, `wasm.query` | Wasm runtime (built-in) |

### The `connections` slot (`ws.send` / `sse.send`)

The `connections` slot is the one exception to "instance name from `noda.json`." There is no connections service in `noda.json`. Instead, the slot value is the **name of a connections endpoint** you define in a `connections/*.json` file. At startup Noda registers each endpoint as a service under its own name, so `ws.send`/`sse.send` resolve the slot to that endpoint's connection manager.

Given this endpoint:

```json
// connections/board.json
{
  "sync": { "pubsub": "events" },
  "endpoints": {
    "board": {
      "type": "websocket",
      "path": "/ws/board/:room_id",
      "channels": { "pattern": "board.{{ request.params.room_id }}" }
    }
  }
}
```

a `ws.send` node binds the slot to the endpoint name `"board"`:

```json
{
  "type": "ws.send",
  "services": { "connections": "board" },
  "config": {
    "channel": "board.{{ input.room_id }}",
    "data": { "text": "{{ input.text }}" }
  }
}
```

**How `channel` relates to `channels.pattern`.** When a client connects, the endpoint's `channels.pattern` is resolved against that connection's context (e.g. `board.{{ request.params.room_id }}` → `board.42`) and the client is subscribed to the resulting channel. The `channel` value on `ws.send` selects which subscribers receive the message: it must be a literal channel name (e.g. `board.42`) — wildcard patterns (e.g. `board.*` or `*`) are rejected with a validation error. So `ws.send`'s `channel` must exactly match one of the channels produced by the endpoint's `pattern` for clients to receive it.

`noda_validate_config` cross-checks the `connections` slot: a `ws.send`/`sse.send` binding that names an endpoint no `connections/*.json` defines is reported as an error. See [`noda://docs/realtime`](realtime.md) for the full subscription and lifecycle model.

---

## Multiple Service Instances

You can define multiple instances of the same plugin. This is common for connecting to separate databases or using different storage backends:

```json
{
  "services": {
    "main-db": {
      "plugin": "db",
      "config": {
        "url": "{{ $env('MAIN_DATABASE_URL') }}"
      }
    },
    "analytics-db": {
      "plugin": "db",
      "config": {
        "url": "{{ $env('ANALYTICS_DATABASE_URL') }}",
        "max_open": 10
      }
    }
  }
}
```

In your workflows, each node picks the instance it needs:

```json
[
  {
    "id": "get-user",
    "type": "db.findOne",
    "services": { "database": "main-db" },
    "config": {
      "table": "users",
      "where": { "id": "{{ input.user_id }}" }
    }
  },
  {
    "id": "log-event",
    "type": "db.create",
    "services": { "database": "analytics-db" },
    "config": {
      "table": "events",
      "data": {
        "user_id": "{{ input.user_id }}",
        "action": "login"
      }
    },
    "depends_on": ["get-user"]
  }
]
```

---

## Quick Reference Table

| Slot | Node Prefixes | Plugin | Required Config Fields |
|------|--------------|--------|----------------------|
| `database` | `db.*` | `db` | `url` (postgres) or `path` (sqlite) |
| `cache` | `cache.*` | `cache` | `url` |
| `storage` | `storage.*` | `storage` | `backend`, `path` (local) |
| `destination` | `upload.handle` | `storage` | `backend`, `path` (local) |
| `source`, `target` | `image.*` | `storage` | `backend`, `path` (local) |
| `pubsub` | `event.emit` (pubsub mode) | `pubsub` | `url` |
| `client` | `http.*` | `http` | (none required) |
| `mailer` | `email.send` | `email` | `host` |
| `stream` | `event.emit`, workers | `stream` | `url` |
| `auth` | `auth.*` | `auth` | `database` |
| `livekit` | `lk.*` | `livekit` | `url`, `api_key`, `api_secret` |

---

## End-to-End Example

A complete setup with a database, cache, and HTTP client -- a route that fetches a user, checks a cache, and calls an external API.

### noda.json

```json
{
  "server": {
    "port": 3000
  },
  "services": {
    "postgres": {
      "plugin": "db",
      "config": {
        "url": "{{ $env('DATABASE_URL') }}"
      }
    },
    "redis": {
      "plugin": "cache",
      "config": {
        "url": "{{ $env('REDIS_URL') }}"
      }
    },
    "billing-api": {
      "plugin": "http",
      "config": {
        "base_url": "https://billing.example.com/v1",
        "timeout": "5s",
        "headers": {
          "X-API-Key": "{{ $env('BILLING_API_KEY') }}"
        }
      }
    }
  }
}
```

### routes/get-user.json

```json
{
  "id": "get-user",
  "method": "GET",
  "path": "/users/:id",
  "trigger": {
    "workflow": "get-user",
    "input": {
      "user_id": "{{ params.id }}"
    }
  }
}
```

### workflows/get-user.json

```json
{
  "id": "get-user",
  "nodes": {
    "check_cache": {
      "type": "cache.get",
      "services": { "cache": "redis" },
      "config": {
        "key": "{{ 'user:' + input.user_id }}"
      }
    },
    "respond_cached": {
      "type": "response.json",
      "config": {
        "status": 200,
        "body": {
          "user": "{{ nodes.check_cache.value }}",
          "cached": true
        }
      }
    },
    "fetch_user": {
      "type": "db.find",
      "services": { "database": "postgres" },
      "config": {
        "table": "users",
        "where": { "id": "{{ input.user_id }}" }
      }
    },
    "get_billing": {
      "type": "http.get",
      "services": { "client": "billing-api" },
      "config": {
        "path": "{{ '/customers/' + input.user_id + '/balance' }}"
      }
    },
    "cache_user": {
      "type": "cache.set",
      "services": { "cache": "redis" },
      "config": {
        "key": "{{ 'user:' + input.user_id }}",
        "value": "{{ nodes.fetch_user[0] }}",
        "ttl": 300
      }
    },
    "respond": {
      "type": "response.json",
      "config": {
        "status": 200,
        "body": {
          "user": "{{ nodes.fetch_user[0] }}",
          "balance": "{{ nodes.get_billing.body.balance }}"
        }
      }
    }
  },
  "edges": [
    { "from": "check_cache", "output": "success", "to": "respond_cached" },
    { "from": "check_cache", "output": "error", "to": "fetch_user" },
    { "from": "fetch_user", "to": "get_billing" },
    { "from": "fetch_user", "to": "cache_user" },
    { "from": "get_billing", "to": "respond" },
    { "from": "cache_user", "to": "respond" }
  ]
}
```

This workflow:

1. Checks the Redis cache for the user. `cache.get` fires its `success` output on a hit and its `error` output on a miss (or connection error).
2. On a cache hit, responds immediately with the cached user.
3. On a miss, queries PostgreSQL (`db.find` returns an array of rows), then in parallel calls the billing API and caches the database result for 5 minutes.
4. Once both branches finish, returns the combined response.
