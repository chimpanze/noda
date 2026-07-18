# auth.create_user

Creates a user with an argon2id-hashed password.

## Config

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `email` | string (expr) | yes | Email address (expression) |
| `password` | string (expr) | yes | Plaintext password (expression); never stored |
| `roles` | array | no | Role names; defaults to `["user"]` |
| `metadata` | object | no | Arbitrary user metadata |

## Outputs

`success`, `exists`, `error`

- `success` — created user: `id`, `email`, `status`, `roles`, `metadata`, timestamps. No `password_hash` field.
- `exists` — a user with this email already exists (unique constraint on `email`).
- `error` — infrastructure error.

## Behavior

Normalizes the email (lowercased, trimmed), validates the password (8–512 characters), hashes it with argon2id (`Service.HashPassword`, using the service's `argon2` parameters), and inserts a row into `auth_users` with `status: "active"` and `email_verified_at: null`. If the email already exists, the unique constraint violation is caught and routed to `exists` instead of `error`. Roles and metadata are stored as JSON columns and decoded back into arrays/objects on output.

## Service Dependencies

| Slot | Prefix | Required |
|------|--------|----------|
| `auth` | `auth` | Yes |
| `database` | `db` | Yes |

## Example

```json
{
  "type": "auth.create_user",
  "services": { "auth": "auth", "database": "main-db" },
  "config": { "email": "{{ input.email }}", "password": "{{ input.password }}" }
}
```

### With data flow

`auth-register` (scaffolded by `noda auth init`) creates the user, then mints an email-verification token, sends it, and finally opens a session:

```json
{
  "create_user": {
    "type": "auth.create_user",
    "services": { "auth": "auth", "database": "main-db" },
    "config": { "email": "{{ input.email }}", "password": "{{ input.password }}" }
  },
  "verify_token": {
    "type": "auth.create_token",
    "services": { "auth": "auth", "database": "main-db" },
    "config": { "user_id": "{{ nodes.create_user.id }}", "purpose": "verify_email" }
  }
}
```

Edges route the `exists` output straight to a `400` response so registration fails cleanly on a duplicate email, without ever confirming *which* email is taken.

## Runnable example

A runnable, CI-verified example of this node lives in the cookbook:
[`examples/node-cookbook/auth`](../../examples/node-cookbook/auth/README.md) — its README documents the exact request/response pair the integration suite executes.
