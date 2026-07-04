# auth.consume_token

Atomically consumes a single-use token.

## Config

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `token` | string (expr) | yes | Raw token (expression) |
| `purpose` | string | yes | Expected purpose: `"verify_email"` or `"reset_password"` |

## Outputs

`success`, `invalid`, `error`

- `success` — `{ "user_id": "..." }`, the id of the token's owner.
- `invalid` — the token is unknown, expired, has the wrong purpose, or was already used. These cases are undifferentiated by design — the caller cannot distinguish "expired" from "already used" from "never existed".
- `error` — infrastructure error.

## Behavior

Consumption is atomic: the node issues a single `UPDATE auth_tokens SET consumed_at = now() WHERE token_hash = ? AND purpose = ? AND consumed_at IS NULL AND expires_at > now()`. The `consumed_at IS NULL` guard in the `WHERE` clause means the database itself enforces single-use — under concurrent requests presenting the same token, exactly one `UPDATE` can match and affect a row; every other concurrent attempt sees zero rows affected and gets `invalid`. There is no separate read-then-write race window.

If `purpose` is `"verify_email"`, a successful consumption also stamps `email_verified_at` on the owning `auth_users` row (and updates `updated_at`) — this is the only way `email_verified_at` gets set, and it is why `auth.session`'s `email_verified` claim only becomes `true` after the user completes the verify-email link.

## Service Dependencies

| Slot | Prefix | Required |
|------|--------|----------|
| `auth` | `auth` | Yes |
| `database` | `db` | Yes |

## Example

```json
{
  "type": "auth.consume_token",
  "services": { "auth": "auth", "database": "main-db" },
  "config": { "token": "{{ input.token }}", "purpose": "verify_email" }
}
```

### With data flow

`auth-reset-password` (scaffolded by `noda auth init`) consumes the reset token, and only on `success` does it proceed to `auth.set_password` with the returned `user_id`:

```json
{
  "consume": {
    "type": "auth.consume_token",
    "services": { "auth": "auth", "database": "main-db" },
    "config": { "token": "{{ input.token }}", "purpose": "reset_password" }
  },
  "respond_invalid": {
    "type": "response.json",
    "config": { "status": 400, "body": { "error": "invalid or expired token" } }
  },
  "set_password": {
    "type": "auth.set_password",
    "services": { "auth": "auth", "database": "main-db" },
    "config": { "user_id": "{{ nodes.consume.user_id }}", "password": "{{ input.password }}" }
  }
}
```
