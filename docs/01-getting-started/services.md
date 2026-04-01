# Service Wiring Guide

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

**Nodes:** `storage.read`, `storage.write`, `storage.list`
**Also used by:** `upload.handle` (slot `destination`), `image.*` nodes (slots `source` and `destination`)

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

Redis Pub/Sub for real-time messaging.

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

---

### HTTP Client (`plugin: "http"`)

Outbound HTTP client with optional circuit breaker. Used by all `http.*` nodes.

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `timeout` | string or number | no | `"30s"` | Request timeout (Go duration string or seconds as number) |
| `base_url` | string | no | -- | Base URL prepended to all requests (must use `http://` or `https://`) |
| `headers` | object | no | -- | Default headers sent with every request |
| `circuit_breaker` | object | no | -- | Circuit breaker configuration (see below) |

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
| `tls` | boolean | no | `true` | Use TLS |

```json
{
  "smtp": {
    "plugin": "email",
    "config": {
      "host": "{{ $env('SMTP_HOST') }}",
      "port": 587,
      "username": "{{ $env('SMTP_USER') }}",
      "password": "{{ $env('SMTP_PASS') }}",
      "from": "noreply@example.com",
      "tls": true
    }
  }
}
```

**Nodes:** `email.send`

---

### LiveKit (`plugin: "livekit"`)

LiveKit WebRTC server integration. Used by all `lk.*` nodes.

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `url` | string | yes | -- | LiveKit server URL |
| `api_key` | string | yes | -- | LiveKit API key |
| `api_secret` | string | yes | -- | LiveKit API secret |

```json
{
  "lk": {
    "plugin": "livekit",
    "config": {
      "url": "{{ $env('LIVEKIT_URL') }}",
      "api_key": "{{ $env('LIVEKIT_API_KEY') }}",
      "api_secret": "{{ $env('LIVEKIT_API_SECRET') }}"
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
| `destination` | `image.*` nodes (output) | `storage` |
| `client` | `http.*` nodes | `http` |
| `mailer` | `email.send` | `email` |
| `stream` | `event.emit`, workers | `stream` |
| `livekit` | `lk.*` nodes | `livekit` |
| `connections` | `ws.send`, `sse.send` | Connection manager (built-in) |
| `runtime` | `wasm.send`, `wasm.query` | Wasm runtime (built-in) |

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
| `source`, `destination` | `image.*` | `storage` | `backend`, `path` (local) |
| `client` | `http.*` | `http` | (none required) |
| `mailer` | `email.send` | `email` | `host` |
| `stream` | `event.emit`, workers | `stream` | `url` |
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
  "nodes": [
    {
      "id": "check-cache",
      "type": "cache.get",
      "services": { "cache": "redis" },
      "config": {
        "key": "{{ 'user:' + input.user_id }}"
      }
    },
    {
      "id": "fetch-user",
      "type": "db.findOne",
      "services": { "database": "postgres" },
      "config": {
        "table": "users",
        "where": { "id": "{{ input.user_id }}" }
      },
      "condition": "{{ nodes.check_cache == nil }}",
      "depends_on": ["check-cache"]
    },
    {
      "id": "get-billing",
      "type": "http.get",
      "services": { "client": "billing-api" },
      "config": {
        "path": "{{ '/customers/' + input.user_id + '/balance' }}"
      },
      "depends_on": ["fetch-user"]
    },
    {
      "id": "cache-user",
      "type": "cache.set",
      "services": { "cache": "redis" },
      "config": {
        "key": "{{ 'user:' + input.user_id }}",
        "value": "{{ nodes.fetch_user }}",
        "ttl": 300
      },
      "condition": "{{ nodes.fetch_user != nil }}",
      "depends_on": ["fetch-user"]
    },
    {
      "id": "respond",
      "type": "response.json",
      "config": {
        "status": 200,
        "body": {
          "user": "{{ nodes.check_cache ?? nodes.fetch_user }}",
          "balance": "{{ nodes.get_billing.balance }}"
        }
      },
      "depends_on": ["get-billing", "cache-user"]
    }
  ]
}
```

This workflow:

1. Checks the Redis cache for the user.
2. If not cached, queries PostgreSQL.
3. Calls the billing API for the user's balance.
4. Caches the database result for 5 minutes.
5. Returns the combined response.
