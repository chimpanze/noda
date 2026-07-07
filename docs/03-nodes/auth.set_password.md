# auth.set_password

Sets a new password (argon2id) and revokes the user's sessions.

## Config

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `user_id` | string (expr) | one of | User id (expression); mutually exclusive with `token` |
| `token` | string (expr) | one of | Password-reset token to consume atomically; mutually exclusive with user_id |
| `password` | string (expr) | yes | New plaintext password (expression) |
| `revoke_sessions` | boolean | no | Revoke all existing sessions for the user. Default `true`. |

## Outputs

`success`, `invalid`, `error`

`success` output: `{ "revoked_sessions": <count> }` — number of sessions revoked (`0` if `revoke_sessions` is `false`, or the user had none).

`invalid` is returned only in token mode when the token is unknown, expired, or already used (undifferentiated, mirroring `auth.consume_token`).

`error` is also returned (not a dedicated output) when `user_id` does not match any row — there is no `not_found` output for this node.

## Behavior

Validates the new password (8–512 characters, Unicode code points, matching the scaffolded route schemas), hashes it with argon2id, and updates `password_hash` on the `auth_users` row. Unless `revoke_sessions` is explicitly `false`, every non-revoked session for that user is revoked in the same call — this is the standard "changing your password logs out all your devices" behavior, and it matters most for the password-reset flow: if an attacker had a still-valid session from a compromised password, resetting the password immediately kills it too.

In **token mode** (`token` instead of `user_id`) the node consumes a
`reset_password` one-time token and updates the password in a single
transaction: the password is validated *before* any write, and a failure at
any later step rolls the consumption back, so a valid reset token is never
burned by a rejected password or an infrastructure error. The user is the
token's owner; no `user_id` is needed.

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

`auth-reset-password` (scaffolded by `noda auth init`) sets the new password by consuming the reset token in the same transaction, then revokes all sessions by default; the scaffolded workflow wires `invalid` → a 400 response:

```json
{
  "set_password": {
    "type": "auth.set_password",
    "services": { "auth": "auth", "database": "main-db" },
    "config": { "token": "{{ input.token }}", "password": "{{ input.password }}" }
  }
}
```

To change a password from an authenticated "account settings" flow without forcing the current session to log out, pass `"revoke_sessions": false` — though in that case you should still verify the user's *current* password with `auth.verify_credentials` first, since this node performs no such check.
