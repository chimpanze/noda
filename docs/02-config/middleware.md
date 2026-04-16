# Middleware

Noda routes pass through a chain of HTTP middleware before reaching the workflow trigger. Middleware is chosen by name; configuration lives alongside the rest of `noda.json`. This page documents how to wire middleware into routes (Concepts), every middleware Noda ships with (Reference), and a few common combinations (Recipes).

## Concepts

### Global middleware

Listed in `noda.json` under `global_middleware`. Applied to every request via `app.Use()` *before* route matching. Use this for cross-cutting concerns: panic recovery, request IDs, access logging, metrics.

```json
{
  "global_middleware": ["recover", "requestid", "logger"]
}
```

Order matters: `recover` must run first so it can catch panics in any later middleware.

### Middleware presets

Named lists of middleware for reuse across routes and route groups. Defined under `middleware_presets`.

```json
{
  "middleware_presets": {
    "authenticated": ["auth.jwt"],
    "public":        ["security.cors", "limiter"],
    "admin":         ["auth.jwt", "casbin.enforce"]
  }
}
```

### Route groups

Apply a preset (or a literal list) to all routes whose path starts with a prefix. Defined under `route_groups`.

```json
{
  "route_groups": {
    "/api/admin": { "middleware_preset": "admin" },
    "/api":       { "middleware_preset": "authenticated" }
  }
}
```

If a route matches multiple group prefixes, the winner is non-deterministic (Go map iteration). Define disjoint prefixes (e.g. `/api/admin` and `/api/public`) rather than nested ones (`/api` and `/api/admin`). If you need different middleware on a subset of routes under a shared prefix, use route-level `middleware_preset` (or `middleware`) on the inner routes instead of overlapping group prefixes.

### Route-level middleware

Individual routes specify middleware directly via `middleware` (a list) or `middleware_preset` (a name).

```json
{
  "id": "update-task",
  "method": "PUT",
  "path": "/api/tasks/:id",
  "middleware": ["auth.jwt"]
}
```

### Resolution order

For each route, Noda builds the middleware chain in this order: group middleware → route preset → route-level `middleware`. Duplicates are removed while preserving the first occurrence. Global middleware is *not* part of this per-route chain — it is applied separately via `app.Use()`. A single route may set both `middleware_preset` and `middleware` — both contribute to the chain, in that order, and duplicates are dropped.

(Source: `internal/server/presets.go:18-64`.)

### Middleware instances

Multiple configurations of the same middleware can coexist using the `name:instance` syntax. Each instance is configured under `middleware_instances`.

```json
{
  "middleware_instances": {
    "auth.oidc:google": {
      "type": "auth.oidc",
      "config": {
        "issuer_url": "https://accounts.google.com",
        "client_id": "{{ $env('GOOGLE_CLIENT_ID') }}"
      }
    },
    "auth.oidc:keycloak": {
      "type": "auth.oidc",
      "config": {
        "issuer_url": "{{ $env('KEYCLOAK_ISSUER_URL') }}",
        "client_id": "{{ $env('KEYCLOAK_CLIENT_ID') }}"
      }
    }
  }
}
```

Reference an instance from a route or preset by its full name (`auth.oidc:google`). Ordering rules apply to the base type, not the instance name.

### Ordering rules

Some middleware combinations have ordering constraints enforced at startup:

- `casbin.enforce` must appear *after* `auth.jwt` or `auth.oidc` in the chain (it needs the authenticated subject).

(Source: `internal/server/presets.go:180-216`.)

## Reference

One entry per registered middleware, alphabetical. Each entry: purpose (one line), config block (or "no config"), example.

### auth.jwt

Validates a JWT bearer token from the `Authorization` header. Stores parsed claims in Fiber locals so trigger mapping and `casbin.enforce` can use them.

Config under `security.jwt` (or via `middleware_instances`):

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `algorithm` | string | no | One of `HS256`/`HS384`/`HS512`/`RS256`/`RS384`/`RS512`/`ES256`/`ES384`/`ES512`. Default: `HS256`. |
| `secret` | string | conditional | Required for `HS*`. Must be at least 32 bytes. |
| `public_key` | string | conditional | Required for `RS*`/`ES*` (PEM-encoded). |
| `public_key_file` | string | conditional | Alternative to `public_key` — path to a PEM file. |

```json
{
  "security": {
    "jwt": { "secret": "{{ $env('JWT_SECRET') }}" }
  }
}
```

### auth.oidc

Validates OIDC ID tokens via discovery + JWKS. Populates the same locals as `auth.jwt`.

Config under `security.oidc`:

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `issuer_url` | string | yes | OIDC issuer (e.g. `https://accounts.google.com`). |
| `client_id` | string | yes | OAuth2 client ID — used as the expected audience. |
| `user_id_claim` | string | no | Claim to extract as user ID. Default: `sub`. |
| `roles_claim` | string | no | Claim to extract as roles. Default: `roles`. |
| `required_scopes` | string[] | no | Scopes that must be present in the token. |

```json
{
  "security": {
    "oidc": {
      "issuer_url": "{{ $env('OIDC_ISSUER_URL') }}",
      "client_id": "{{ $env('OIDC_CLIENT_ID') }}",
      "user_id_claim": "sub",
      "roles_claim": "roles",
      "required_scopes": ["openid", "profile"]
    }
  }
}
```

(Multi-provider example: see Recipes.)

### casbin.enforce

Enforces a Casbin policy using the authenticated subject from `auth.jwt` or `auth.oidc`. Must follow one of those middleware in the chain.

Config under `security.casbin` (per `internal/server/casbin.go`):

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `model` | string | yes | Either inline Casbin model text (recognized by the presence of `[request_definition]`) or a file path. |
| `policies` | array of string arrays | no | Policy rules, each as `[ptype, ...params]`. Example: `["p", "admin", "/api/*", "GET"]`. |
| `role_links` | array of string arrays | no | Role assignments (grouping policies), each as `[gtype, ...params]`. Example: `["g", "alice", "admin"]`. |
| `tenant_param` | string | no | If set, enforcement uses 4 args (`sub, tenant, obj, act`). The tenant value is read from the route param of this name, falling back to the query string. |

```json
{
  "security": {
    "casbin": {
      "model": "auth/model.conf",
      "policies": [
        ["p", "admin", "/api/*", "*"]
      ]
    }
  }
}
```

### compress

Gzip/deflate response compression. No config.

```json
{ "global_middleware": ["recover", "requestid", "logger", "compress"] }
```

### etag

Adds `ETag` header to responses and short-circuits with `304 Not Modified` on matching `If-None-Match`. No config.

### idempotency

Caches responses keyed by an idempotency-key request header so retries return the same response (Fiber's built-in middleware).

Config (per `internal/server/idempotency.go`):

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `key_header` | string | no | Header name carrying the idempotency key. Default: `X-Idempotency-Key`. |
| `lifetime` | duration string | no | Cache TTL (e.g. `"30m"`). Default: `30m`. |
| `storage` | string | no | `"redis"` for distributed storage. Default: in-memory. |
| `redis_url` | string | conditional | Required when `storage` is `"redis"`. |

### limiter

Per-IP request rate limiting.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `max` | int | yes | Maximum requests per `expiration` window. Must be greater than 0. |
| `expiration` | duration string | no | Window length (e.g. `"1m"`). |
| `storage` | string | no | `"redis"` for distributed limiting (default: in-memory). |
| `redis_url` | string | conditional | Required when `storage` is `"redis"`. |

```json
{
  "middleware": {
    "limiter": { "max": 100, "expiration": "1m" }
  }
}
```

Config lives under `middleware.limiter` — there is no `security.*` alternative path for this middleware (unlike `security.cors`, `security.jwt`, etc.).

### livekit.webhook

Verifies LiveKit webhook signatures. Credentials are resolved from the middleware config first, then fall back to the `lk` service config in `noda.json`.

Config under `security.livekit`:

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `api_key` | string | yes (if no `lk` service) | LiveKit API key. |
| `api_secret` | string | yes (if no `lk` service) | LiveKit API secret. |

```json
{
  "path": "/webhooks/livekit",
  "method": "POST",
  "middleware": ["livekit.webhook"],
  "trigger": {
    "raw_body": true,
    "workflow": "on-livekit-event",
    "input": {
      "event": "{{ body.event }}",
      "room": "{{ body.room }}",
      "participant": "{{ body.participant }}"
    }
  }
}
```

The middleware only verifies the signature — the webhook body is accessed in your workflow through the normal `{{ body.* }}` trigger input mapping. LiveKit event types include: `room_started`, `room_finished`, `participant_joined`, `participant_left`, `track_published`, `track_unpublished`, `egress_started`, `egress_ended`, `ingress_started`, `ingress_ended`.

To provide credentials explicitly (instead of using the lk service config):

```json
{
  "security": {
    "livekit": {
      "api_key": "{{ $env('LIVEKIT_API_KEY') }}",
      "api_secret": "{{ $env('LIVEKIT_API_SECRET') }}"
    }
  }
}
```

### logger

Writes one access log line per request to stdout. Includes `time`, `ip`, `status`, `method`, `path`, `request_id` (set by `requestid`), and `latency`. No config. Omitting `requestid` from the chain leaves the `request_id` field empty — it does not error.

### recover

Catches panics in downstream middleware/handlers and returns `500`. Should appear first in any chain.

### requestid

Generates an `X-Request-Id` if the inbound request has none, sets it on the response, and stores it in the Fiber context. Trigger mapping picks it up as `trigger.request_id`. No config.

### response.status_remap

Rewrites the outgoing HTTP status code based on a static map. Runs after the workflow's response has been set; unmapped statuses pass through unchanged. Body, headers, and content-type are left untouched.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `map` | object | yes | Keys are stringified upstream status codes (100–599); values are integer outgoing status codes (100–599). Must be non-empty. A self-map (e.g. `"403": 403`) is rejected as a configuration mistake. |

Common use case — remap upstream `403` to `401` at the public edge so clients retry with credentials:

```json
{
  "middleware": {
    "response.status_remap": {
      "map": { "403": 401, "502": 503 }
    }
  },
  "middleware_presets": {
    "public": ["security.cors", "response.status_remap"]
  },
  "route_groups": {
    "/api/public": { "middleware_preset": "public" }
  }
}
```

Ordering: place `response.status_remap` *before* `compress`, `etag`, or any access-logging middleware that should observe the rewritten status.

`WWW-Authenticate` is not auto-added on remapped `401` responses. Pair with `security.headers` or set the header explicitly in `response.error` when needed.

Observability: each remap increments the `http.status_remaps.total` counter with `from` and `to` labels.

### security.cors

CORS headers and automatic `OPTIONS` preflight registration on routes whose chain includes this middleware.

Config under `security.cors`:

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `allow_origins` | string | no | Comma-separated list of allowed origins. Wildcard `*` works but **cannot** be combined with `allow_credentials: true` (rejected at startup). Defaults to localhost-only with a startup warning when omitted. |
| `allow_methods` | string | no | Comma-separated allowed HTTP methods. |
| `allow_headers` | string | no | Comma-separated headers the client may send. |
| `allow_credentials` | bool | no | Allow cookies / `Authorization` header on cross-origin requests. |

```json
{
  "security": {
    "cors": {
      "allow_origins": "https://app.example.com,https://admin.example.com",
      "allow_methods": "GET,POST,PUT,DELETE,OPTIONS",
      "allow_headers": "Content-Type,Authorization,X-Request-Id",
      "allow_credentials": true
    }
  }
}
```

### security.csrf

CSRF protection via double-submit cookie.

Config under `security.csrf` — fields the factory reads (per `internal/server/middleware.go:215-238`):

| Field | Type | Description |
|-------|------|-------------|
| `cookie_name` | string | Cookie name. |
| `cookie_secure` | bool | Set `Secure` flag. |
| `cookie_http_only` | bool | Set `HttpOnly` flag. |
| `cookie_same_site` | string | `Strict` / `Lax` / `None`. |
| `cookie_session_only` | bool | Don't persist past session. |
| `single_use_token` | bool | Rotate token after each use. |

```json
{
  "security": {
    "csrf": {
      "cookie_secure": true,
      "cookie_same_site": "Strict"
    }
  }
}
```

### security.headers

Standard security headers via `helmet` (CSP, X-Frame-Options, etc.). No config in current implementation.

### timeout

Cancels handler execution if it exceeds a deadline.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `duration` | duration string | no | Timeout window (e.g. `"30s"`). Default: `30s`. |

## Recipes

### Public SPA endpoint with CORS and a rate limit

```json
{
  "security": {
    "cors": {
      "allow_origins": "https://app.example.com",
      "allow_methods": "GET,POST",
      "allow_headers": "Content-Type"
    }
  },
  "middleware": {
    "limiter": { "max": 60, "expiration": "1m" }
  },
  "middleware_presets": {
    "public": ["security.cors", "limiter"]
  },
  "route_groups": {
    "/api/public": { "middleware_preset": "public" }
  }
}
```

### Authenticated API with Casbin

```json
{
  "security": {
    "jwt": { "secret": "{{ $env('JWT_SECRET') }}" },
    "casbin": {
      "model": "auth/model.conf",
      "policies": [
        ["p", "admin", "/api/*", "*"],
        ["p", "user",  "/api/me", "GET"]
      ],
      "role_links": [
        ["g", "alice", "admin"]
      ]
    }
  },
  "middleware_presets": {
    "authenticated": ["auth.jwt", "casbin.enforce"]
  },
  "route_groups": {
    "/api": { "middleware_preset": "authenticated" }
  }
}
```

### Multi-tenant OIDC with Google and Keycloak

```json
{
  "middleware_instances": {
    "auth.oidc:google": {
      "type": "auth.oidc",
      "config": {
        "issuer_url": "https://accounts.google.com",
        "client_id": "{{ $env('GOOGLE_CLIENT_ID') }}"
      }
    },
    "auth.oidc:keycloak": {
      "type": "auth.oidc",
      "config": {
        "issuer_url": "{{ $env('KEYCLOAK_ISSUER_URL') }}",
        "client_id": "{{ $env('KEYCLOAK_CLIENT_ID') }}"
      }
    }
  },
  "route_groups": {
    "/api/google":   { "middleware": ["auth.oidc:google"] },
    "/api/keycloak": { "middleware": ["auth.oidc:keycloak"] }
  }
}
```

> Add routes under `/api/google/` and `/api/keycloak/` respectively — each will receive the corresponding OIDC middleware from its group.
