# auth.verify_credentials

Verifies email+password with timing-safe comparison.

## Config

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `email` | string (expr) | yes | Email (expression) |
| `password` | string (expr) | yes | Plaintext password (expression) |

## Outputs

`success`, `invalid`, `error`

- `success` — the authenticated user (no `password_hash`).
- `invalid` — credentials rejected. Unknown email, wrong password, and a non-`active` user status all route here identically — no reason is disclosed.
- `error` — infrastructure error.

## Behavior

Looks up the user by normalized email. If no row matches, the node still runs a full argon2id verification against a fixed dummy hash (`VerifyDummy`) before returning `invalid`, so a request for an unknown email takes the same time as one for a known email with a wrong password — this defeats email-enumeration-by-timing attacks.

If the user exists, the stored hash is checked with `VerifyPassword`, which accepts either format:

- **argon2id** (`$argon2id$...`) — verified directly with constant-time comparison.
- **bcrypt** (`$2a$`/`$2b$`/`$2y$`) — verified via `bcrypt.CompareHashAndPassword`. This lets you migrate an existing user table (e.g. from a bcrypt-based system) into `auth_users` without a bulk rehash: existing hashes verify as-is.

When a bcrypt hash verifies successfully, the node opportunistically re-hashes the password with argon2id and updates `password_hash` in place (best-effort — a failure here does not fail the login). Every subsequent login for that user verifies against argon2id directly. The same rehash-on-login also applies to argon2id hashes whose embedded parameters differ from the service's currently configured `argon2` settings — raising `memory_kib`/`iterations` in the auth service config automatically upgrades existing users' hashes as they log in. A non-`active` `status` (e.g. suspended) also yields `invalid`, after the password check runs, so status alone is never a distinguishing timing signal for an attacker who doesn't already know the password.

## Service Dependencies

| Slot | Prefix | Required |
|------|--------|----------|
| `auth` | `auth` | Yes |
| `database` | `db` | Yes |

## Example

```json
{
  "type": "auth.verify_credentials",
  "services": { "auth": "auth", "database": "main-db" },
  "config": { "email": "{{ input.email }}", "password": "{{ input.password }}" }
}
```

### With data flow

`auth-login` (scaffolded by `noda auth init`) verifies credentials, routes `invalid` to a `401`, and otherwise opens a session using the id from the verified user:

```json
{
  "verify": {
    "type": "auth.verify_credentials",
    "services": { "auth": "auth", "database": "main-db" },
    "config": { "email": "{{ input.email }}", "password": "{{ input.password }}" }
  },
  "respond_invalid": {
    "type": "response.json",
    "config": { "status": 401, "body": { "error": "invalid credentials" } }
  },
  "session": {
    "type": "auth.create_session",
    "services": { "auth": "auth", "database": "main-db" },
    "config": { "user_id": "{{ nodes.verify.id }}" }
  }
}
```
