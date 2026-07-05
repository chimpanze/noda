# auth.get_user

Fetches a user by id or email (password hash stripped).

## Config

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `user_id` | string (expr) | no | User id (expression); exactly one of `user_id`/`email` |
| `email` | string (expr) | no | Email (expression); exactly one of `user_id`/`email` |

## Outputs

`success`, `not_found`, `error`

- `success` — the user object (no `password_hash`).
- `not_found` — no matching user.
- `error` — infrastructure error, or neither/both of `user_id`/`email` were supplied.

## Behavior

Looks up a row in `auth_users` by `id` (if `user_id` is set) or by normalized `email` (if `email` is set). Exactly one of the two must be provided — supplying both, or neither, is an `error`, not `invalid`/`not_found`. On a match, returns the row with `password_hash` stripped and `roles`/`metadata` decoded from their JSON columns.

## Service Dependencies

| Slot | Prefix | Required |
|------|--------|----------|
| `auth` | `auth` | Yes |
| `database` | `db` | Yes |

## Example

```json
{
  "type": "auth.get_user",
  "services": { "auth": "auth", "database": "main-db" },
  "config": { "user_id": "{{ auth.sub }}" }
}
```

### With data flow

`auth-me` (scaffolded by `noda auth init`) looks the current user up by the id from the session middleware and routes `not_found` to a `401`:

```json
{
  "get": {
    "type": "auth.get_user",
    "services": { "auth": "auth", "database": "main-db" },
    "config": { "user_id": "{{ input.user_id }}" }
  },
  "respond_missing": {
    "type": "response.json",
    "config": { "status": 401, "body": { "error": "invalid token" } }
  }
}
```

`auth-request-password-reset` looks a user up by email instead, and treats `not_found` as a normal (non-error) branch so the response is identical to the success path — see `auth.create_token` and the [authentication guide](../04-guides/authentication.md) for the enumeration-safe pattern.
