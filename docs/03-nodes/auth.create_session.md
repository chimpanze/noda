# auth.create_session

Mints an opaque session token and stores its hash.

## Config

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `user_id` | string (expr) | yes | User id (expression) |
| `ttl` | string | no | Session lifetime (e.g. `"720h"`); defaults to the service's `session.ttl` |

## Outputs

`success`, `error`

`success` output:

| Field | Type | Description |
|-------|------|-------------|
| `token` | string | Raw opaque session token — the *only* place the raw value exists. Send it to the client via the `cookie` object or a response body; it cannot be recovered from the database. |
| `session_id` | string | Session row id (usable with `auth.revoke_session`'s `session_id` config) |
| `expires_at` | string (timestamp) | Session expiry |
| `cookie` | object | A cookie descriptor object — pass it directly into `response.json`'s `cookies` config |

The `cookie` object has the shape `response.json` expects: `name`, `value`, `path`, `domain`, `max_age` (seconds), `secure`, `http_only`, `same_site`. Its values come from the service's `session.cookie.*` config (see [noda-json.md](../02-config/noda-json.md#auth-service-plugin-auth)).

## Behavior

Generates a 256-bit random token, stores only its SHA-256 hash in `auth_sessions` (the raw token is never persisted), and computes `expires_at` from `ttl` (or the service default). Returns the raw token plus a ready-to-use cookie object. The `auth.session` middleware later authenticates requests by hashing the incoming token and matching it against `token_hash`.

For HTTP-triggered workflows, the client IP and `User-Agent` header are stored in the row's `ip` and `user_agent` columns — useful for "active sessions" listings. Both stay `NULL` when the workflow was triggered by a schedule, event, or test. Behind a reverse proxy, the recorded IP is the connection's remote address unless your deployment terminates the proxy chain appropriately.

## Service Dependencies

| Slot | Prefix | Required |
|------|--------|----------|
| `auth` | `auth` | Yes |
| `database` | `db` | Yes |

## Example

```json
{
  "type": "auth.create_session",
  "services": { "auth": "auth", "database": "main-db" },
  "config": { "user_id": "{{ nodes.verify.id }}" }
}
```

### With data flow

Use `response.json`'s `cookies` config with the node's `cookie` output to set the session cookie on login, exactly as `auth-login` does (scaffolded by `noda auth init`):

```json
{
  "session": {
    "type": "auth.create_session",
    "services": { "auth": "auth", "database": "main-db" },
    "config": { "user_id": "{{ nodes.verify.id }}" }
  },
  "respond": {
    "type": "response.json",
    "config": {
      "status": 200,
      "body": { "user": "{{ nodes.verify }}", "token": "{{ nodes.session.token }}" },
      "cookies": "{{ [nodes.session.cookie] }}"
    }
  }
}
```

`cookies` takes an array, so a single cookie is wrapped as `[nodes.session.cookie]`. The `token` is also included in the JSON body for clients that prefer `Authorization: Bearer <token>` over cookies — the `auth.session` middleware accepts either (cookie checked first, then the bearer header).
