# Authentication & Authorization

This guide covers securing Noda APIs with JWT tokens, OIDC providers, and Casbin authorization policies.

## JWT Authentication

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

For RSA or ECDSA algorithms, provide a public key instead of a secret:

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

**Routes** (`routes/auth.json`):

```json
[
  {
    "id": "oidc-login",
    "method": "GET",
    "path": "/auth/login",
    "trigger": {
      "workflow": "oidc-login"
    }
  },
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
]
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

## Middleware Presets

Group middleware into reusable presets for consistent security across routes:

```json
{
  "middleware_presets": {
    "public": ["cors", "rate_limit"],
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
    "public": ["cors"],
    "authenticated": ["auth.jwt"]
  },
  "route_groups": {
    "/api": {
      "middleware_preset": "authenticated"
    }
  }
}
```

Routes outside any group (or in a group with no auth middleware) are public:

```json
[
  {
    "id": "login",
    "method": "POST",
    "path": "/auth/login",
    "trigger": { "workflow": "login" }
  },
  {
    "id": "register",
    "method": "POST",
    "path": "/auth/register",
    "trigger": { "workflow": "register" }
  },
  {
    "id": "list-tasks",
    "method": "GET",
    "path": "/api/tasks",
    "trigger": {
      "workflow": "list-tasks",
      "input": { "user_id": "{{ auth.sub }}" }
    }
  }
]
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
