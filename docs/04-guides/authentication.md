# Authentication & Authorization

This guide covers securing Noda APIs with JWT tokens, OIDC providers, Casbin authorization policies, and the first-party `auth` plugin's session-based email+password flows.

## JWT Authentication

> **Using RS256, RS384, RS512, ES256, ES384, or ES512?** Skip to [Asymmetric keys (RSA / ECDSA)](#asymmetric-keys-rsa--ecdsa) below — Noda supports both inline (`public_key`) and file-based (`public_key_file`) keys.

### Configuration

Add JWT settings to `noda.json` under `security.jwt`:

```json
{
  "security": {
    "jwt": {
      "secret": "{{ $env('JWT_SECRET') }}",
      "algorithm": "HS256"
    }
  }
}
```

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `secret` | string | for HMAC | - | Signing secret (minimum 32 bytes) |
| `algorithm` | string | no | `"HS256"` | Signing algorithm |
| `public_key` | string | for RSA/ECDSA | - | PEM-encoded public key (inline) |
| `public_key_file` | string | for RSA/ECDSA | - | Path to PEM-encoded public key file |

Supported algorithms: `HS256`, `HS384`, `HS512`, `RS256`, `RS384`, `RS512`, `ES256`, `ES384`, `ES512`.

### Asymmetric keys (RSA / ECDSA)

For RSA or ECDSA algorithms, provide a public key instead of a secret. Either inline or as a file:

```json
{
  "security": {
    "jwt": {
      "algorithm": "RS256",
      "public_key_file": "/etc/noda/jwt-public.pem"
    }
  }
}
```

Or inline:

```json
{
  "security": {
    "jwt": {
      "algorithm": "RS256",
      "public_key": "-----BEGIN PUBLIC KEY-----\n...\n-----END PUBLIC KEY-----"
    }
  }
}
```

### Protecting Routes

Apply `auth.jwt` middleware to individual routes or entire route groups.

**Single route:**

```json
{
  "id": "get-profile",
  "method": "GET",
  "path": "/api/profile",
  "middleware": ["auth.jwt"],
  "trigger": {
    "workflow": "get-profile",
    "input": {
      "user_id": "{{ auth.sub }}"
    }
  }
}
```

**Route group (all routes under a prefix):**

```json
{
  "middleware_presets": {
    "authenticated": ["auth.jwt"]
  },
  "route_groups": {
    "/api": {
      "middleware_preset": "authenticated"
    }
  }
}
```

Every route under `/api` now requires a valid JWT. Requests without a valid `Authorization: Bearer <token>` header receive a `401 Unauthorized` response.

### Accessing Auth Data in Workflows

When `auth.jwt` (or `auth.oidc`) middleware validates a token, the following data is available in trigger input mappings and workflow expressions:

| Expression | Description |
|-----------|-------------|
| `auth.sub` | User ID from the `sub` claim |
| `auth.roles` | Roles array from the `roles` claim |
| `auth.claims` | All token claims as an object |
| `auth.claims.email` | Any specific claim by name |

Use these in trigger input mappings to pass auth context into workflows:

```json
{
  "trigger": {
    "workflow": "create-task",
    "input": {
      "user_id": "{{ auth.sub }}",
      "roles": "{{ auth.roles }}",
      "email": "{{ auth.claims.email }}"
    }
  }
}
```

### Signing Tokens

Use the `util.jwt_sign` node to create JWT tokens in workflows:

```json
{
  "sign_token": {
    "type": "util.jwt_sign",
    "config": {
      "claims": {
        "sub": "{{ input.user_id }}",
        "roles": "{{ input.roles }}",
        "email": "{{ input.email }}"
      },
      "secret": "{{ $env('JWT_SECRET') }}",
      "algorithm": "HS256",
      "expiry": "24h"
    }
  }
}
```

The `expiry` field accepts durations like `"1h"`, `"24h"`, `"7d"` and automatically sets the `exp` claim. The node outputs the signed token string on its `success` output.

### Complete Login Flow

A login workflow that validates credentials against the database and returns a JWT:

**Route** (`routes/auth.json`):

```json
{
  "id": "login",
  "method": "POST",
  "path": "/auth/login",
  "body": {
    "schema": {
      "type": "object",
      "properties": {
        "email": { "type": "string", "format": "email" },
        "password": { "type": "string", "minLength": 8 }
      },
      "required": ["email", "password"]
    }
  },
  "trigger": {
    "workflow": "login",
    "input": {
      "email": "{{ body.email }}",
      "password": "{{ body.password }}"
    }
  }
}
```

**Workflow** (`workflows/login.json`):

```json
{
  "id": "login",
  "nodes": {
    "find_user": {
      "type": "db.findOne",
      "services": { "database": "main-db" },
      "config": {
        "table": "users",
        "where": {
          "email": "{{ input.email }}"
        }
      }
    },
    "check_password": {
      "type": "control.if",
      "config": {
        "condition": "{{ nodes.find_user != nil and bcrypt_verify(input.password, nodes.find_user.password_hash) }}"
      }
    },
    "sign_token": {
      "type": "util.jwt_sign",
      "config": {
        "claims": {
          "sub": "{{ nodes.find_user.id }}",
          "roles": "{{ nodes.find_user.roles }}",
          "email": "{{ nodes.find_user.email }}"
        },
        "secret": "{{ $env('JWT_SECRET') }}",
        "expiry": "24h"
      }
    },
    "respond_success": {
      "type": "response.json",
      "config": {
        "status": 200,
        "body": {
          "token": "{{ nodes.sign_token.token }}"
        }
      }
    },
    "invalid_credentials": {
      "type": "response.error",
      "config": {
        "status": 401,
        "code": "INVALID_CREDENTIALS",
        "message": "Invalid email or password"
      }
    }
  },
  "edges": [
    { "from": "find_user", "to": "check_password", "output": "success" },
    { "from": "find_user", "to": "invalid_credentials", "output": "error" },
    { "from": "check_password", "to": "sign_token", "output": "then" },
    { "from": "check_password", "to": "invalid_credentials", "output": "else" },
    { "from": "sign_token", "to": "respond_success", "output": "success" }
  ]
}
```

### Complete Registration Flow

A registration workflow that hashes the password and creates a user:

**Route** (`routes/auth.json`):

```json
{
  "id": "register",
  "method": "POST",
  "path": "/auth/register",
  "body": {
    "schema": {
      "type": "object",
      "properties": {
        "email": { "type": "string", "format": "email" },
        "password": { "type": "string", "minLength": 8 },
        "name": { "type": "string" }
      },
      "required": ["email", "password", "name"]
    }
  },
  "trigger": {
    "workflow": "register",
    "input": {
      "email": "{{ body.email }}",
      "password": "{{ body.password }}",
      "name": "{{ body.name }}"
    }
  }
}
```

**Workflow** (`workflows/register.json`):

```json
{
  "id": "register",
  "nodes": {
    "check_existing": {
      "type": "db.findOne",
      "services": { "database": "main-db" },
      "config": {
        "table": "users",
        "where": {
          "email": "{{ input.email }}"
        }
      }
    },
    "already_exists": {
      "type": "response.error",
      "config": {
        "status": 409,
        "code": "EMAIL_EXISTS",
        "message": "An account with this email already exists"
      }
    },
    "create_user": {
      "type": "db.create",
      "services": { "database": "main-db" },
      "config": {
        "table": "users",
        "fields": {
          "email": "{{ input.email }}",
          "name": "{{ input.name }}",
          "password_hash": "{{ bcrypt_hash(input.password) }}",
          "roles": ["user"]
        }
      }
    },
    "sign_token": {
      "type": "util.jwt_sign",
      "config": {
        "claims": {
          "sub": "{{ nodes.create_user.id }}",
          "roles": "{{ nodes.create_user.roles }}",
          "email": "{{ nodes.create_user.email }}"
        },
        "secret": "{{ $env('JWT_SECRET') }}",
        "expiry": "24h"
      }
    },
    "respond_success": {
      "type": "response.json",
      "config": {
        "status": 201,
        "body": {
          "token": "{{ nodes.sign_token.token }}"
        }
      }
    },
    "server_error": {
      "type": "response.error",
      "config": {
        "status": 500,
        "code": "SERVER_ERROR",
        "message": "Registration failed"
      }
    }
  },
  "edges": [
    { "from": "check_existing", "to": "already_exists", "output": "success" },
    { "from": "check_existing", "to": "create_user", "output": "error" },
    { "from": "create_user", "to": "sign_token", "output": "success" },
    { "from": "create_user", "to": "server_error", "output": "error" },
    { "from": "sign_token", "to": "respond_success", "output": "success" }
  ]
}
```

### Anti-enumeration in the scaffolded flows

The flows generated by `noda auth init` are hardened against account enumeration, and the templates show the patterns to follow if you write your own:

- **Registration is verification-first.** Both a brand-new email and an already-registered one return an identical `200 {"message":"Check your email to continue"}` with **no** session cookie, and both send an email (a verification link for a new account; an "account already exists" notice for an existing one). Registration therefore does not disclose which addresses have accounts — and it does **not** auto-log-in; the user verifies, then logs in. If you want auto-login instead, add an `auth.create_session` node on the success branch, accepting that a returned cookie then reveals which emails are free.
- **Password-reset and resend-verification respond at a fixed deadline.** A known account runs a synchronous SMTP `email.send` (tens–hundreds of ms) while an unknown account would otherwise return in ~1 ms — a timing oracle. The templates record a start timestamp (`util.timestamp` with `format: "unix_ms"`) and, on every branch, pad to a fixed ~500 ms deadline with `util.delay` (`timeout: "{{ (nodes.start_ts + 500) > nodes.now_ts_X ? (nodes.start_ts + 500 - nodes.now_ts_X) : 0 }}ms"`), so all branches respond at ~500 ms regardless of whether an email was sent.
- **Send failures are absorbed too.** Each `email.send` node routes its `error` output into the same `now_ts → pad → respond` chain as its success output, so a hard SMTP failure (mail server down) still returns the generic padded `200` on the known branch rather than a fast `500` — otherwise a mail outage would turn into a status-code oracle. The send failure is still recorded in the workflow trace for operators.
- **Residual and the hard fix.** Padding only holds while real SMTP stays under the deadline; if your mail server can exceed ~500 ms, raise the constant. For a guarantee that the response never depends on SMTP at all, decouple the send: `event.emit` an email job to a stream and have a worker send it out-of-band, so the HTTP response returns immediately on every branch.

## OIDC Authentication

### Provider Configuration

Configure an OIDC provider in `noda.json` under `security.oidc`:

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

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `issuer_url` | string | yes | - | OIDC provider issuer URL (e.g. `https://accounts.google.com`) |
| `client_id` | string | yes | - | OAuth2 client ID (used as expected audience) |
| `user_id_claim` | string | no | `"sub"` | Claim to extract as user ID |
| `roles_claim` | string | no | `"roles"` | Claim to extract as roles |
| `required_scopes` | string[] | no | - | Scopes that must be present in the token |

At startup, Noda performs OIDC discovery on the issuer URL to fetch the provider configuration and JWKS keys. Tokens are validated against the provider's public keys automatically.

Apply the middleware to routes the same way as `auth.jwt`:

```json
{
  "middleware_presets": {
    "authenticated": ["auth.oidc"]
  },
  "route_groups": {
    "/api": {
      "middleware_preset": "authenticated"
    }
  }
}
```

The OIDC middleware populates the same auth context (`auth.sub`, `auth.roles`, `auth.claims`) as the JWT middleware, so workflows work identically regardless of which middleware is used.

### Authorization Code Flow

Use the OIDC nodes to implement the full authorization code flow: redirect the user to the provider, handle the callback, and exchange the code for tokens.

**Routes** — each route is its own file containing a single route object (a route file is never a top-level array):

`routes/auth-login.json`:

```json
{
  "id": "oidc-login",
  "method": "GET",
  "path": "/auth/login",
  "trigger": {
    "workflow": "oidc-login"
  }
}
```

`routes/auth-callback.json`:

```json
{
  "id": "oidc-callback",
  "method": "GET",
  "path": "/auth/callback",
  "trigger": {
    "workflow": "oidc-callback",
    "input": {
      "code": "{{ query.code }}",
      "state": "{{ query.state }}"
    }
  }
}
```

**Login workflow** (`workflows/oidc-login.json`) -- generates a state parameter, stores it in cache, and redirects to the provider:

```json
{
  "id": "oidc-login",
  "nodes": {
    "gen_state": {
      "type": "transform.set",
      "config": {
        "fields": {
          "state": "{{ $uuid() }}"
        }
      }
    },
    "cache_state": {
      "type": "cache.set",
      "services": { "cache": "redis" },
      "config": {
        "key": "{{ 'oidc_state:' + nodes.gen_state.state }}",
        "value": "1",
        "ttl": 300
      }
    },
    "build_url": {
      "type": "oidc.auth_url",
      "config": {
        "issuer_url": "{{ $env('OIDC_ISSUER_URL') }}",
        "client_id": "{{ $env('OIDC_CLIENT_ID') }}",
        "redirect_uri": "{{ $env('APP_URL') + '/auth/callback' }}",
        "state": "{{ nodes.gen_state.state }}",
        "scopes": ["openid", "profile", "email"]
      }
    },
    "redirect": {
      "type": "response.redirect",
      "config": {
        "url": "{{ nodes.build_url.url }}"
      }
    }
  },
  "edges": [
    { "from": "gen_state", "to": "cache_state", "output": "success" },
    { "from": "cache_state", "to": "build_url", "output": "success" },
    { "from": "build_url", "to": "redirect", "output": "success" }
  ]
}
```

**Callback workflow** (`workflows/oidc-callback.json`) -- verifies the state, exchanges the code, and creates or updates the user:

```json
{
  "id": "oidc-callback",
  "nodes": {
    "verify_state": {
      "type": "cache.get",
      "services": { "cache": "redis" },
      "config": {
        "key": "{{ 'oidc_state:' + input.state }}"
      }
    },
    "check_state": {
      "type": "control.if",
      "config": {
        "condition": "{{ nodes.verify_state != nil }}"
      }
    },
    "exchange_code": {
      "type": "oidc.exchange",
      "config": {
        "issuer_url": "{{ $env('OIDC_ISSUER_URL') }}",
        "client_id": "{{ $env('OIDC_CLIENT_ID') }}",
        "client_secret": "{{ $env('OIDC_CLIENT_SECRET') }}",
        "redirect_uri": "{{ $env('APP_URL') + '/auth/callback' }}",
        "code": "{{ input.code }}"
      }
    },
    "upsert_user": {
      "type": "db.upsert",
      "services": { "database": "main-db" },
      "config": {
        "table": "users",
        "conflict": ["provider_id"],
        "fields": {
          "provider_id": "{{ nodes.exchange_code.claims.sub }}",
          "email": "{{ nodes.exchange_code.claims.email }}",
          "name": "{{ nodes.exchange_code.claims.name }}",
          "refresh_token": "{{ nodes.exchange_code.refresh_token }}"
        }
      }
    },
    "sign_session_token": {
      "type": "util.jwt_sign",
      "config": {
        "claims": {
          "sub": "{{ nodes.upsert_user.id }}",
          "email": "{{ nodes.upsert_user.email }}",
          "roles": ["user"]
        },
        "secret": "{{ $env('JWT_SECRET') }}",
        "expiry": "24h"
      }
    },
    "redirect_to_app": {
      "type": "response.redirect",
      "config": {
        "url": "{{ $env('APP_URL') + '/?token=' + nodes.sign_session_token.token }}"
      }
    },
    "invalid_state": {
      "type": "response.error",
      "config": {
        "status": 400,
        "code": "INVALID_STATE",
        "message": "Invalid or expired state parameter"
      }
    },
    "exchange_failed": {
      "type": "response.error",
      "config": {
        "status": 401,
        "code": "EXCHANGE_FAILED",
        "message": "Failed to exchange authorization code"
      }
    }
  },
  "edges": [
    { "from": "verify_state", "to": "check_state", "output": "success" },
    { "from": "verify_state", "to": "invalid_state", "output": "error" },
    { "from": "check_state", "to": "exchange_code", "output": "then" },
    { "from": "check_state", "to": "invalid_state", "output": "else" },
    { "from": "exchange_code", "to": "upsert_user", "output": "success" },
    { "from": "exchange_code", "to": "exchange_failed", "output": "error" },
    { "from": "upsert_user", "to": "sign_session_token", "output": "success" },
    { "from": "sign_session_token", "to": "redirect_to_app", "output": "success" }
  ]
}
```

### Token Refresh

Use `oidc.refresh` to obtain new tokens when the access token expires:

```json
{
  "refresh_token": {
    "type": "oidc.refresh",
    "config": {
      "issuer_url": "{{ $env('OIDC_ISSUER_URL') }}",
      "client_id": "{{ $env('OIDC_CLIENT_ID') }}",
      "client_secret": "{{ $env('OIDC_CLIENT_SECRET') }}",
      "refresh_token": "{{ input.refresh_token }}"
    }
  }
}
```

On success, the node outputs `access_token`, `refresh_token` (if rotated), `id_token`, `claims`, and `expires_at`.

### Multiple Providers

Use middleware instances to support multiple OIDC providers simultaneously. Each instance gets its own configuration:

```json
{
  "middleware_instances": {
    "auth.oidc:google": {
      "type": "auth.oidc",
      "config": {
        "issuer_url": "https://accounts.google.com",
        "client_id": "{{ $env('GOOGLE_CLIENT_ID') }}",
        "user_id_claim": "sub",
        "roles_claim": "roles"
      }
    },
    "auth.oidc:keycloak": {
      "type": "auth.oidc",
      "config": {
        "issuer_url": "{{ $env('KEYCLOAK_ISSUER_URL') }}",
        "client_id": "{{ $env('KEYCLOAK_CLIENT_ID') }}",
        "roles_claim": "realm_access.roles"
      }
    }
  }
}
```

Reference instances by their full name on routes:

```json
{
  "id": "google-profile",
  "method": "GET",
  "path": "/api/google/profile",
  "middleware": ["auth.oidc:google"]
}
```

```json
{
  "id": "keycloak-profile",
  "method": "GET",
  "path": "/api/internal/profile",
  "middleware": ["auth.oidc:keycloak"]
}
```

## Casbin Authorization

Casbin provides policy-based access control. It runs after authentication middleware and uses the authenticated user's identity to enforce rules.

### Model and Policy Files

Casbin uses a model to define the access control pattern and policies to define the rules. The most common pattern is RBAC (role-based access control).

An RBAC model defines that a subject (user) with a role can perform an action on a resource:

```
[request_definition]
r = sub, obj, act

[policy_definition]
p = sub, obj, act

[role_definition]
g = _, _

[policy_effect]
e = some(where (p.eft == allow))

[matchers]
m = g(r.sub, p.sub) && keyMatch2(r.obj, p.obj) && r.act == p.act
```

The `keyMatch2` matcher supports path patterns: `/api/tasks/:id` matches `/api/tasks/123`.

### Configuration

Define the Casbin model and policies in `noda.json` under `security.casbin`:

```json
{
  "security": {
    "casbin": {
      "model": "[request_definition]\nr = sub, obj, act\n\n[policy_definition]\np = sub, obj, act\n\n[role_definition]\ng = _, _\n\n[policy_effect]\ne = some(where (p.eft == allow))\n\n[matchers]\nm = g(r.sub, p.sub) && keyMatch2(r.obj, p.obj) && r.act == p.act",
      "policies": [
        ["p", "admin", "/api/*", "GET"],
        ["p", "admin", "/api/*", "POST"],
        ["p", "admin", "/api/*", "PUT"],
        ["p", "admin", "/api/*", "DELETE"],
        ["p", "user", "/api/tasks", "GET"],
        ["p", "user", "/api/tasks", "POST"],
        ["p", "user", "/api/tasks/:id", "GET"],
        ["p", "user", "/api/tasks/:id", "PUT"],
        ["p", "user", "/api/tasks/:id", "DELETE"]
      ],
      "role_links": [
        ["g", "alice", "admin"],
        ["g", "bob", "user"]
      ]
    }
  }
}
```

The model can also reference a file path instead of inline text:

```json
{
  "security": {
    "casbin": {
      "model": "config/rbac_model.conf",
      "policies": [
        ["p", "admin", "/api/*", "*"]
      ]
    }
  }
}
```

If the model string contains `[request_definition]`, it is treated as inline text. Otherwise, it is loaded as a file path.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `model` | string | yes | Inline Casbin model text or path to `.conf` file |
| `policies` | array | yes | Policy rule tuples (first element is the policy type: `p`, `p2`, etc.) |
| `role_links` | array | no | Role assignment tuples (first element is the grouping type: `g`, `g2`, etc.) |
| `tenant_param` | string | no | URL or query parameter for multi-tenant RBAC |

### Route Protection

Apply `casbin.enforce` middleware after an authentication middleware. Casbin reads the user identity from the auth locals set by `auth.jwt` or `auth.oidc`:

```json
{
  "middleware_presets": {
    "admin": ["auth.jwt", "casbin.enforce"]
  },
  "route_groups": {
    "/api/admin": {
      "middleware_preset": "admin"
    }
  }
}
```

The Casbin middleware extracts the subject from `auth.sub` (set by the preceding auth middleware), the object from the request path, and the action from the HTTP method. If the policy does not allow the request, a `403 Forbidden` response is returned.

Casbin works identically with OIDC:

```json
{
  "middleware_presets": {
    "oidc-admin": ["auth.oidc", "casbin.enforce"]
  }
}
```

### Multi-Tenant RBAC

For multi-tenant applications, set `tenant_param` to include a tenant identifier in the enforcement check. The tenant value is read from the URL parameter or query string:

```json
{
  "security": {
    "casbin": {
      "model": "[request_definition]\nr = sub, dom, obj, act\n\n[policy_definition]\np = sub, dom, obj, act\n\n[role_definition]\ng = _, _, _\n\n[policy_effect]\ne = some(where (p.eft == allow))\n\n[matchers]\nm = g(r.sub, p.sub, r.dom) && r.dom == p.dom && keyMatch2(r.obj, p.obj) && r.act == p.act",
      "tenant_param": "workspace_id",
      "policies": [
        ["p", "admin", "workspace-1", "/api/*", "*"],
        ["p", "editor", "workspace-1", "/api/docs/*", "GET"],
        ["p", "editor", "workspace-1", "/api/docs/*", "PUT"]
      ],
      "role_links": [
        ["g", "alice", "admin", "workspace-1"],
        ["g", "bob", "editor", "workspace-1"]
      ]
    }
  }
}
```

### Enforcing on the roles claim

`casbin.enforce` always uses the **user id** (`auth.sub`) as the subject. It does **not** read the token's `auth.roles` claim. So there is no way to write a single middleware-level rule like "allow any token carrying the `admin` role" — Casbin only knows a user has a role if a `role_links` (`g`) entry maps that specific user id to it. With Casbin you enumerate users in `role_links`:

```json
{
  "security": {
    "casbin": {
      "model": "...",
      "policies": [["p", "admin", "/api/admin/*", "*"]],
      "role_links": [
        ["g", "alice", "admin"],
        ["g", "carol", "admin"]
      ]
    }
  }
}
```

If your tokens already carry roles (e.g. an OIDC provider issues `roles: ["admin"]`) and you want a generic "must have the `admin` role" gate **without** listing every user, enforce it **in the workflow** instead of at the middleware. Branch on the `auth.roles` claim with `control.if` and return `403` via `response.error`:

```json
{
  "id": "admin-only-report",
  "nodes": {
    "check_admin": {
      "type": "control.if",
      "config": { "condition": "{{ 'admin' in auth.roles }}" }
    },
    "forbidden": {
      "type": "response.error",
      "config": {
        "status": 403,
        "code": "forbidden",
        "message": "Admin role required"
      }
    },
    "report": {
      "type": "response.json",
      "config": { "status": 200, "body": "{{ nodes.build_report }}" }
    }
  },
  "edges": [
    { "from": "check_admin", "to": "report", "output": "true" },
    { "from": "check_admin", "to": "forbidden", "output": "false" }
  ]
}
```

Use middleware-level `casbin.enforce` for per-user/per-resource policies; use the in-workflow `control.if` pattern when a coarse role-claim check is all you need and you don't want to enumerate users.

## Session Authentication (`auth` plugin)

The sections above assume you bring your own token issuer (an external service, or hand-rolled login logic behind `util.jwt_sign`). The `auth` plugin is the alternative: a complete, ownable email+password authentication system — registration, login, logout, "who am I", email verification, and password reset — scaffolded straight into your project.

### The model: scaffold, don't wrap

The plugin follows a shadcn/ui-style split, not a black-box "auth server":

- **The plugin owns primitives.** 8 nodes (`auth.create_user`, `auth.get_user`, `auth.verify_credentials`, `auth.create_session`, `auth.revoke_session`, `auth.create_token`, `auth.consume_token`, `auth.set_password`) plus the `auth.session` middleware. These are stable, tested building blocks — argon2id hashing, opaque session tokens, single-use tokens — and they live in `plugins/auth/`, not your project.
- **Your project owns the flows.** `noda auth init` generates real, editable files into *your* project: migrations, routes, and workflows built from those 8 nodes. There is no hidden control flow — the generated `workflows/auth-register.json` etc. are ordinary workflow files you can open in the editor and change node-by-node (add a CAPTCHA check, require an invite code, change the email copy, add an audit log node) exactly like any other workflow.

This means upgrading the plugin never silently changes your auth behavior — the generated files are yours from the moment `noda auth init` runs.

### `noda auth init` walkthrough

```
noda auth init [--dir .]
```

Preconditions: the project's `noda.json` must already have a database service (`plugin: "db"`) — the command detects it by scanning `services` and fails with a clear error if none exists. It also looks for an `email` service; if none is found it still scaffolds the email-dependent flows (verify-email, password reset) but prints a warning, since those workflows won't be able to send mail until you add one.

What it writes:

1. **Migrations** — a driver-specific (`postgres` or `sqlite`, detected from the db service's `driver` config) up/down pair creating `auth_users`, `auth_sessions`, `auth_tokens`, timestamped like any other migration.
2. **Seven workflows** (in `workflows/`, one file each): `auth-register`, `auth-login`, `auth-logout`, `auth-me`, `auth-request-password-reset`, `auth-reset-password`, `auth-verify-email`.
3. **Routes** wiring each workflow to an HTTP endpoint (`POST /auth/register`, `POST /auth/login`, etc.), using a `limiter` middleware on the credential-guessing-sensitive ones.
4. **`noda.json` patches** — applied in place, not appended:
   - Adds `services.auth` (`{"plugin": "auth", "config": {"database": "<detected db service>"}}`).
   - Adds a `middleware_presets.authenticated_session: ["auth.session"]` preset, if one doesn't already exist.
   - If the project has no `middleware.limiter` config yet, adds a default (`{"max": 20, "expiration": "1m"}`) — the scaffolded login/register/reset/resend routes reference the `limiter` middleware by name, and the server refuses to start without an explicit `max` for it.
   - **Rewrites the entire `noda.json` file** via `json.MarshalIndent`, which serializes Go maps with their keys in alphabetical order. Every top-level and nested key in `noda.json` comes out alphabetized after `noda auth init` runs, even keys unrelated to auth. This is a one-time reformatting side effect of the patch mechanism — review the diff (`git diff noda.json`) after running it so the reordering doesn't surprise you in a PR full of unrelated moved lines.

All files are generated in memory first and checked for collisions against existing files before anything is written — if any target file already exists, the whole command fails with no partial writes.

After scaffolding: run your migration tool, then open the generated workflows in the editor to customize them.

### The eight flows, and how to customize them

| Workflow | Route | What it does |
|---|---|---|
| `auth-register` | `POST /auth/register` | `auth.create_user` → `auth.create_token` (`verify_email`) → `email.send` → `auth.create_session` → `201` with `user`, `token`, and a `Set-Cookie` |
| `auth-login` | `POST /auth/login` | `auth.verify_credentials` → `auth.create_session` → `200` with `user`, `token`, cookie; `invalid` → `401` |
| `auth-logout` | `POST /auth/logout` | `auth.revoke_session` → `204` with a cookie that clears the session |
| `auth-me` | `GET /auth/me` | `auth.get_user` (by the session's `user_id`) → `200`; `not_found` → `401` |
| `auth-request-password-reset` | `POST /auth/request-password-reset` | `auth.get_user` (by email) → `auth.create_token` (`reset_password`) → `email.send` → `200` (same body whether or not the account exists — see below) |
| `auth-reset-password` | `POST /auth/reset-password` | `auth.consume_token` (`reset_password`) → `auth.set_password` (revokes sessions) → `200`; `invalid` → `400` |
| `auth-verify-email` | `POST /auth/verify-email` | `auth.consume_token` (`verify_email`) → `200`; `invalid` → `400` |
| `auth-resend-verification` | `POST /auth/resend-verification` | `auth.get_user` (by email) → `control.if` (skip if already verified) → `auth.create_token` (`verify_email`) → `email.send` → `200` (same body for unknown, already-verified, and unverified accounts) |

Every one of these is a plain workflow — customize by editing nodes and edges directly, same as any workflow. For example, to require an invite code at registration, add a `control.if` node right after the trigger that checks `{{ input.invite_code }}` against a lookup (e.g. `db.query` on an `invite_codes` table) and routes failures to a `response.error` before `auth.create_user` ever runs — no plugin change required.

### Sessions vs. `auth.jwt` / `auth.oidc`

Use `auth.session` (opaque, server-validated sessions) when:

- You own both the login flow and the client (first-party web/mobile app) and want instant, server-side revocation — logout, "force logout everywhere", and password-reset-revokes-sessions all take effect immediately, because every request re-checks the database. A JWT, by contrast, stays valid until it expires no matter what the server does, unless you build a separate revocation list.
- You want the plugin's built-in argon2id password storage and single-use email/reset tokens instead of hand-rolling them.

Use `auth.jwt` when:

- Tokens need to be self-contained and verifiable without a database round-trip (e.g. service-to-service calls, or a separate service validating tokens issued elsewhere). `util.jwt_sign` still exists precisely for this — nothing about the `auth` plugin removes it, and you can mix the two (e.g. sign a short-lived JWT for a downstream microservice from a session-authenticated request).

Use `auth.oidc` when an external identity provider (Google, Keycloak, Auth0, ...) is the source of truth for identity — the `auth` plugin does not do federated login.

All three middleware populate the same locals (`auth.sub`, `auth.roles`, `auth.claims`), so workflows and Casbin policies work identically regardless of which one authenticated the request — you can even run different middleware on different route groups of the same app.

### Security defaults, and why

| Default | Reasoning |
|---|---|
| Passwords hashed with **argon2id** (memory 64 MiB, 3 iterations, parallelism 2 — OWASP-recommended) | Memory-hard hashing resists GPU/ASIC cracking far better than bcrypt at equivalent CPU cost. |
| Sessions are **opaque, random 256-bit tokens**, hashed (SHA-256) before storage | The database never holds a value that can be replayed if leaked; a stolen session hash is useless without reversing SHA-256. Compare to a self-verifying JWT, where the raw token itself is the credential and a DB leak of the token store is moot but a *client-side* leak of the token is immediately usable until expiry. |
| Password reset / email verification tokens are also opaque and **single-use** (atomic consume, see `auth.consume_token`) | Removes the class of bugs where a token can be replayed twice, or where two concurrent requests both succeed. |
| `auth-request-password-reset` returns the **same response body** whether or not the account exists | Prevents attackers from enumerating registered emails by observing a different error for unknown addresses. |
| `auth.set_password` **revokes all sessions by default** | If a password reset happens because credentials were compromised, any session opened with the old (compromised) password is killed at the same moment the new password takes effect — there's no window where both the old and new credentials are simultaneously valid. |
| Session cookie defaults: `HttpOnly: true`, `Secure: true`, `SameSite: Lax` | `HttpOnly`/`Secure` block script access and non-TLS transmission. `Lax` is the safe default for same-site apps — it's sent on top-level navigations but not on cross-site subresource/XHR requests, which blocks the classic CSRF vector without extra configuration. |

**Dev-mode traces and the cookie object:** the dev-mode trace stream redacts session tokens in HTTP responses and in the `cookie`/`clear_cookie` objects produced by `auth.create_session`/`auth.revoke_session` — but that redaction is keyed to those field names. Don't copy the cookie object into a differently-named field of workflow state (e.g. via `transform.set` into `session_cookie`): the renamed copy carries the raw token and will appear unredacted in dev-mode traces.

### CSRF guidance for cross-site frontends

`SameSite=Lax` (the default) protects same-site apps — where your frontend and API share a registrable domain — from CSRF without any extra setup, because the browser withholds the cookie on cross-site `fetch`/XHR calls.

If your frontend is served from a **different origin** than the API (a common SPA deployment: `app.example.com` calling `api.example.com`, or a mobile webview), the browser still sends the cookie on cross-site requests once you loosen `SameSite` (or if you use `SameSite=None`, which itself requires `Secure`). In that configuration, add `security.csrf` (double-submit cookie protection) to the routes that accept the session cookie — see [`security.csrf`](../02-config/middleware.md#securitycsrf) in the middleware reference. Alternatively, have the cross-site frontend use the `Authorization: Bearer <token>` path instead of the cookie (the `token` field returned by `auth.create_session`/`auth-login`) — bearer tokens sent via an explicit header are not subject to CSRF the way ambient cookies are, since the browser never attaches them automatically.

### The reset-request timing signal

`auth-request-password-reset` returns an identical response body for both a known and an unknown email — but the two code paths are not perfectly identical in *timing*: for a known email, the workflow additionally runs `auth.create_token` and `email.send` before responding; for an unknown one, it returns immediately after `auth.get_user`'s `not_found`. A sufficiently precise timing measurement could still distinguish the two cases, especially if the mail provider call is slow.

To close this residual gap, decouple the email send from the request/response cycle with `event.emit`: have the workflow emit an event (e.g. `password_reset_requested`) carrying the token instead of calling `email.send` inline, and handle the actual send in a separate event-triggered workflow. The HTTP response then returns at the same point (right after the `db` lookup) regardless of whether the account exists, and the extra token-creation/email work happens asynchronously off the request's critical path.

### Migrating an existing user table (bcrypt)

If you already have a users table with bcrypt password hashes (`$2a$`/`$2b$`/`$2y$` prefix), you don't need a bulk rehash migration before switching to the `auth` plugin. `auth.verify_credentials` recognizes both hash formats: argon2id hashes (`$argon2id$...`) are verified directly, and bcrypt hashes are verified via `bcrypt.CompareHashAndPassword`. On a successful bcrypt verification, the node transparently re-hashes the password with argon2id and updates `password_hash` in place — a purely opportunistic, best-effort upgrade that never fails the login if it errors. Every account converges to argon2id the first time its owner logs in after the migration; there is no forced-reset step and no "the reset didn't apply" corner case, since the conversion only ever happens alongside a successful login.

The same opportunistic upgrade applies to argon2id parameter changes: if you later raise `argon2.memory_kib` or `iterations` in the service config, existing hashes created with the old parameters are re-hashed with the new ones on each user's next successful login.

### Purging expired sessions and tokens

Expired or revoked `auth_sessions` rows and consumed or expired `auth_tokens` rows are never deleted by the plugin — they stop authenticating immediately, but the rows stay behind and the tables grow without bound. Add a scheduled purge workflow; in Noda that's one schedule file and one two-node workflow.

`schedules/purge-auth-rows.json`:

```json
{
  "id": "purge-auth-rows",
  "cron": "0 0 3 * * *",
  "description": "Nightly cleanup of dead auth_sessions/auth_tokens rows",
  "trigger": { "workflow": "purge-auth-rows" }
}
```

`workflows/purge-auth-rows.json`:

```json
{
  "id": "purge-auth-rows",
  "name": "Purge expired auth rows",
  "nodes": {
    "purge_sessions": {
      "type": "db.exec",
      "services": { "database": "main-db" },
      "config": {
        "sql": "DELETE FROM auth_sessions WHERE expires_at < ? OR revoked_at IS NOT NULL",
        "params": ["{{ now() }}"]
      }
    },
    "purge_tokens": {
      "type": "db.exec",
      "services": { "database": "main-db" },
      "config": {
        "sql": "DELETE FROM auth_tokens WHERE expires_at < ? OR consumed_at IS NOT NULL",
        "params": ["{{ now() }}"]
      }
    }
  },
  "edges": [
    { "from": "purge_sessions", "to": "purge_tokens" }
  ]
}
```

Deleting these rows has no effect on live traffic: expiry and revocation are enforced on every request by `auth.session`/`auth.consume_token` regardless of whether the dead rows still exist. Keep them longer (e.g. add `AND expires_at < <30 days ago>`) only if you want an audit window for session history.

## Middleware Presets

Group middleware into reusable presets for consistent security across routes:

```json
{
  "middleware_presets": {
    "public": ["security.cors", "limiter"],
    "authenticated": ["auth.jwt"],
    "admin": ["auth.jwt", "casbin.enforce"],
    "oidc-authenticated": ["auth.oidc"],
    "oidc-admin": ["auth.oidc", "casbin.enforce"]
  }
}
```

Apply presets to route groups so you don't repeat middleware on every route:

```json
{
  "route_groups": {
    "/api/admin": {
      "middleware_preset": "admin"
    },
    "/api": {
      "middleware_preset": "authenticated"
    },
    "/public": {
      "middleware_preset": "public"
    }
  }
}
```

Route groups are matched by longest prefix first: a request to `/api/admin/users` matches the `/api/admin` group, not `/api`.

## Common Patterns

### Public and Protected Routes

Mix public and protected routes by placing them in different route groups or by specifying middleware per route:

```json
{
  "middleware_presets": {
    "public": ["security.cors"],
    "authenticated": ["auth.jwt"]
  },
  "route_groups": {
    "/api": {
      "middleware_preset": "authenticated"
    }
  }
}
```

Routes outside any group (or in a group with no auth middleware) are public. Each route lives in its own file (one route object per file — never a top-level array):

`routes/login.json`:

```json
{
  "id": "login",
  "method": "POST",
  "path": "/auth/login",
  "trigger": { "workflow": "login" }
}
```

`routes/register.json`:

```json
{
  "id": "register",
  "method": "POST",
  "path": "/auth/register",
  "trigger": { "workflow": "register" }
}
```

`routes/list-tasks.json`:

```json
{
  "id": "list-tasks",
  "method": "GET",
  "path": "/api/tasks",
  "trigger": {
    "workflow": "list-tasks",
    "input": { "user_id": "{{ auth.sub }}" }
  }
}
```

The login and register routes have no auth middleware. The `/api/tasks` route is protected by the route group.

### Role-Based Response Filtering

Use `auth.roles` in workflow conditions to customize responses based on the user's role:

```json
{
  "check_admin": {
    "type": "control.if",
    "config": {
      "condition": "{{ 'admin' in auth.roles }}"
    }
  }
}
```

This pattern lets you serve different data from the same endpoint. Admins might see all fields while regular users see a filtered view.

### Scoping Database Queries to the Current User

Always include the user ID in database queries to ensure users only access their own data:

```json
{
  "list_tasks": {
    "type": "db.query",
    "services": { "database": "main-db" },
    "config": {
      "sql": "SELECT * FROM tasks WHERE user_id = $1 ORDER BY created_at DESC",
      "params": ["{{ input.user_id }}"]
    }
  }
}
```

The `input.user_id` value comes from `{{ auth.sub }}` in the trigger mapping, which is set by the JWT or OIDC middleware from the validated token.
