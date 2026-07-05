# auth.set_password

Sets a new password (argon2id) and revokes the user's sessions.

## Config

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `user_id` | string (expr) | yes | User id (expression) |
| `password` | string (expr) | yes | New plaintext password (expression) |
| `revoke_sessions` | boolean | no | Revoke all existing sessions for the user. Default `true`. |

## Outputs

`success`, `error`

`success` output: `{ "revoked_sessions": <count> }` — number of sessions revoked (`0` if `revoke_sessions` is `false`, or the user had none).

`error` is also returned (not a dedicated output) when `user_id` does not match any row — there is no `not_found` output for this node.

## Behavior

Validates the new password (8–512 characters), hashes it with argon2id, and updates `password_hash` on the `auth_users` row. Unless `revoke_sessions` is explicitly `false`, every non-revoked session for that user is revoked in the same call — this is the standard "changing your password logs out all your devices" behavior, and it matters most for the password-reset flow: if an attacker had a still-valid session from a compromised password, resetting the password immediately kills it too.

## Service Dependencies

| Slot | Prefix | Required |
|------|--------|----------|
| `auth` | `auth` | Yes |
| `database` | `db` | Yes |

## Example

```json
{
  "type": "auth.set_password",
  "services": { "auth": "auth", "database": "main-db" },
  "config": { "user_id": "{{ input.user_id }}", "password": "{{ input.password }}" }
}
```

### With data flow

`auth-reset-password` (scaffolded by `noda auth init`) sets the new password using the `user_id` returned by `auth.consume_token`, then revokes all sessions by default:

```json
{
  "set_password": {
    "type": "auth.set_password",
    "services": { "auth": "auth", "database": "main-db" },
    "config": { "user_id": "{{ nodes.consume.user_id }}", "password": "{{ input.password }}" }
  }
}
```

To change a password from an authenticated "account settings" flow without forcing the current session to log out, pass `"revoke_sessions": false` — though in that case you should still verify the user's *current* password with `auth.verify_credentials` first, since this node performs no such check.
