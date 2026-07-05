# auth.revoke_session

Revokes one session (by token or id) or all sessions for a user.

## Config

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `token` | string (expr) | one of three | Raw session token to revoke (expression) |
| `session_id` | string (expr) | one of three | Session id to revoke (expression) |
| `user_id` | string (expr) | one of three | Revoke **all** sessions for this user (expression) |

Exactly one of `token`, `session_id`, `user_id` must be set; supplying zero or more than one is an `error`.

## Outputs

`success`, `error`

`success` output:

| Field | Type | Description |
|-------|------|-------------|
| `revoked_count` | number | Number of session rows revoked (0 if nothing matched — not an error) |
| `clear_cookie` | object | A cookie descriptor that deletes the session cookie — pass it into `response.json`'s `cookies` config |

## Behavior

Sets `revoked_at` on matching, not-already-revoked rows in `auth_sessions`. Revocation is instant: because sessions are opaque tokens looked up against the database on every request (not self-verifying JWTs), a revoked session is rejected by `auth.session` on its very next use — there is no token lifetime to wait out. `clear_cookie` reuses the service's cookie config with `max_age: -1`, which tells the browser to delete the cookie immediately.

## Service Dependencies

| Slot | Prefix | Required |
|------|--------|----------|
| `auth` | `auth` | Yes |
| `database` | `db` | Yes |

## Example

```json
{
  "type": "auth.revoke_session",
  "services": { "auth": "auth", "database": "main-db" },
  "config": { "session_id": "{{ input.session_id }}" }
}
```

### With data flow

`auth-logout` (scaffolded by `noda auth init`) revokes the current session and clears the cookie in one response:

```json
{
  "revoke": {
    "type": "auth.revoke_session",
    "services": { "auth": "auth", "database": "main-db" },
    "config": { "session_id": "{{ input.session_id }}" }
  },
  "respond": {
    "type": "response.json",
    "config": { "status": 204, "cookies": "{{ [nodes.revoke.clear_cookie] }}" }
  }
}
```

To force-logout a user everywhere (e.g. "log out of all devices", or as part of `auth.set_password`'s own session revocation), pass `user_id` instead of `session_id` — every active session for that user is revoked in one call.
