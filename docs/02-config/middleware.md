# Middleware

## Middleware Presets

Named collections of middleware for reuse across routes and route groups. Defined in `noda.json` under `middleware_presets`.

```json
{
  "middleware_presets": {
    "authenticated": ["auth.jwt"],
    "public": ["cors", "rate_limit"],
    "admin": ["auth.jwt", "auth.casbin"]
  }
}
```

Available middleware: `auth.jwt`, `auth.oidc`, `auth.casbin`, `cors`, `rate_limit`, `helmet`, `compress`, `etag`, `livekit.webhook`.

## Route Groups

Apply middleware presets to URL path prefixes. Defined in `noda.json` under `route_groups`.

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

## security.cors (alias: `cors`)

Adds CORS headers to responses. Configured under `security.cors` in `noda.json`.

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

| Field | Type | Description |
|---|---|---|
| `allow_origins` | string | Comma-separated list of allowed origins. Wildcard `*` works but **cannot** be combined with `allow_credentials: true` (rejected at startup). |
| `allow_methods` | string | Comma-separated allowed HTTP methods |
| `allow_headers` | string | Comma-separated headers the client may send |
| `allow_credentials` | bool | Allow cookies / Authorization header on cross-origin requests |

If `allow_origins` is omitted, Noda defaults to `localhost` only and logs a warning — don't ship with no origins configured.

Apply via preset or per-route:

```json
{
  "middleware_presets": {
    "public": ["security.cors", "rate_limit"]
  }
}
```

A route whose middleware chain includes `security.cors` also gets an auto-registered `OPTIONS` handler for CORS preflight.

## auth.oidc

Validates OIDC ID tokens from external identity providers (Google, Keycloak, Auth0, Okta, etc.). Uses OIDC discovery to fetch provider configuration and JWKS keys automatically. Populates the same authentication locals as `auth.jwt`, so Casbin authorization and trigger input mapping work identically.

Configure in `noda.json`:

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

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `issuer_url` | string | yes | OIDC provider issuer URL (e.g. `https://accounts.google.com`) |
| `client_id` | string | yes | OAuth2 client ID — used as the expected audience |
| `user_id_claim` | string | no | Claim to extract as user ID (default: `sub`) |
| `roles_claim` | string | no | Claim to extract as roles (default: `roles`) |
| `required_scopes` | string[] | no | Scopes that must be present in the token |

Use on routes the same way as `auth.jwt`:

```json
{
  "id": "get-profile",
  "method": "GET",
  "path": "/api/profile",
  "middleware": ["auth.oidc"]
}
```

### Multiple Providers

Use middleware instances to support multiple OIDC providers simultaneously:

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

### Casbin Compatibility

`auth.oidc` works with `casbin.enforce` the same way `auth.jwt` does — the user ID and roles from the OIDC token are used as the Casbin subject:

```json
{
  "middleware_presets": {
    "oidc-admin": ["auth.oidc", "casbin.enforce"]
  }
}
```

## livekit.webhook

Verifies LiveKit webhook signatures. Credentials are resolved from the middleware config first, then fall back to the `lk` service config in `noda.json`.

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

## Route-Level Middleware

Individual routes can specify middleware directly:

```json
{
  "id": "update-task",
  "method": "PUT",
  "path": "/api/tasks/:id",
  "middleware": ["auth.jwt"]
}
```
