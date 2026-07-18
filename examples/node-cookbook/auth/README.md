# Cookbook: auth nodes

Runnable examples for all eight `auth.*` primitives — `create_user`,
`verify_credentials`, `create_session`, `revoke_session`, `create_token`,
`consume_token`, `get_user`, and `set_password` — against a real
Postgres-backed `auth_users`/`auth_sessions`/`auth_tokens` schema.
Every request/response below is verified in CI by [`verify.json`](verify.json).

These are the **raw primitives**, one node per route, so each node's
contract (config fields, success shape, and every non-default output) is
visible in isolation. If you want a ready-made register/login/logout/
verify-email/reset-password flow instead, see `examples/auth-demo`, whose
`auth.*` workflows are scaffolded by `noda auth init` and chain several of
these same nodes together (e.g. `auth-register` = `create_user` →
`create_token` → `email.send` → `create_session`).

The `auth` service wraps the `main-db` Postgres service (`{"database":
"main-db"}` in `noda.json`) — every node also declares a `database` service
slot alongside `auth`, per `docs/03-nodes/auth.*.md` Service Dependencies.

## Run

This project needs a real Postgres instance — CI's cookbook walker starts one
via testcontainers automatically (see `deps: ["postgres"]` in `verify.json`).
To run it yourself:

```bash
docker run -d --name cookbook-auth -e POSTGRES_PASSWORD=postgres -p 5432:5432 postgres:17-alpine
export DATABASE_URL='postgres://postgres:postgres@localhost:5432/postgres?sslmode=disable'
go run ./cmd/noda migrate up --config examples/node-cookbook/auth --service main-db
go run ./cmd/noda start --config examples/node-cookbook/auth
```

## auth.create_user — `POST /api/auth/create-user`

Hashes the password with argon2id and inserts a row. The `success` output
never includes `password_hash`; a duplicate email routes to the `exists`
output instead of `error`.

```bash
curl -X POST localhost:3000/api/auth/create-user -H 'Content-Type: application/json' \
  -d '{"email": "ada@example.com", "password": "correct-horse-1"}'
# → 201 {"id":"...","email":"ada@example.com","status":"active","roles":["user"],"metadata":{},...}

curl -X POST localhost:3000/api/auth/create-user -H 'Content-Type: application/json' \
  -d '{"email": "ada@example.com", "password": "whatever-2"}'
# → 409 {"error":{"code":"EMAIL_TAKEN","message":"email already registered"}}
```

## auth.verify_credentials — `POST /api/auth/verify-credentials`

Timing-safe comparison; unknown email, wrong password, and non-active status
all route identically to `invalid` — no reason is disclosed to the caller.

```bash
curl -X POST localhost:3000/api/auth/verify-credentials -H 'Content-Type: application/json' \
  -d '{"email": "ada@example.com", "password": "correct-horse-1"}'
# → 200 {"id":"...","email":"ada@example.com",...}

curl -X POST localhost:3000/api/auth/verify-credentials -H 'Content-Type: application/json' \
  -d '{"email": "ada@example.com", "password": "wrong"}'
# → 401 {"error":{"code":"INVALID_CREDENTIALS","message":"invalid email or password"}}
```

## auth.create_session — `POST /api/auth/create-session`

Per `docs/03-nodes/auth.create_session.md` Outputs, this node has only
`success`/`error` — no dedicated failure output to route. The `success`
output carries the raw opaque `token` (the *only* place it ever exists — only
its SHA-256 hash is persisted), plus `session_id`, `expires_at`, and a
ready-to-use `cookie` descriptor.

```bash
curl -X POST localhost:3000/api/auth/create-session -H 'Content-Type: application/json' \
  -d '{"user_id": "<id>"}'
# → 200 {"token":"...","session_id":"...","expires_at":"...","cookie":{...}}
```

## auth.revoke_session — `POST /api/auth/revoke-session`

Also only `success`/`error` (confirmed against docs — no `invalid`/
`not_found` output exists). **Observed/documented behavior:** revoking an
unknown or already-revoked token is *not* an error — it matches zero rows and
returns `success` with `revoked_count: 0`. This cookbook's final verify step
exploits that directly: earlier, `set_password` ran with its default
`revoke_sessions: true`, which already revoked the session created above as a
side effect. Revoking it again here therefore demonstrates the node's
idempotent-success behavior (204, no body) rather than exercising a second,
independent session — this is the documented, honest behavior rather than a
workaround.

```bash
curl -X POST localhost:3000/api/auth/revoke-session -H 'Content-Type: application/json' \
  -d '{"token": "<session-token>"}'
# → 204 (empty body, even for an already-revoked or unknown token)
```

## auth.create_token — `POST /api/auth/create-token`

Mints a single-use token for `verify_email` or `reset_password`. The raw
token is returned only in this response — verified against
`docs/03-nodes/auth.create_token.md` Outputs, the field is `token` (not,
e.g., `raw_token`). Only `success`/`error` outputs exist.

```bash
curl -X POST localhost:3000/api/auth/create-token -H 'Content-Type: application/json' \
  -d '{"user_id": "<id>", "purpose": "reset_password"}'
# → 201 {"token":"...","expires_at":"..."}
```

## auth.consume_token — `POST /api/auth/consume-token`

Atomic single-use consumption (`UPDATE ... WHERE consumed_at IS NULL AND
expires_at > now()`). `invalid` covers unknown/expired/wrong-purpose/
already-used tokens identically by design.

```bash
curl -X POST localhost:3000/api/auth/consume-token -H 'Content-Type: application/json' \
  -d '{"token": "<token>", "purpose": "verify_email"}'
# → 200 {"user_id":"..."}

# second attempt with the same (now-consumed) token:
# → 400 {"error":{"code":"TOKEN_INVALID","message":"token is unknown, expired, or already used"}}
```

## auth.get_user — `GET /api/auth/users?user_id=<id>`

```bash
curl 'localhost:3000/api/auth/users?user_id=<id>'
# → 200 {"id":"...","email":"ada@example.com",...}

curl 'localhost:3000/api/auth/users?user_id=nope'
# → 404 {"error":{"code":"USER_NOT_FOUND","message":"user not found"}}
```

## auth.set_password — `POST /api/auth/set-password`

This cookbook demonstrates **token mode** (mutually exclusive with
`user_id`): the node consumes a `reset_password` token and updates the
password atomically in one transaction — a rejected password or an
infrastructure error rolls the token consumption back, so a valid token is
never burned for nothing. `revoke_sessions` is left at its default (`true`),
so every active session for the user is revoked as a side effect (see
`auth.revoke_session` above for how that interacts with this cookbook's
final step). Per `docs/03-nodes/auth.set_password.md` Outputs, the failure
output in token mode is `invalid` (not `error` — `error` is reserved for
`user_id` mode's "no matching row" case, which token mode cannot hit).

```bash
curl -X POST localhost:3000/api/auth/set-password -H 'Content-Type: application/json' \
  -d '{"token": "<reset-token>", "password": "new-horse-9"}'
# → 200 {"updated": true}

# reusing the same (now-consumed) reset token:
# → 400 {"error":{"code":"TOKEN_INVALID","message":"reset token is unknown, expired, or already used"}}
```

## Uncertainties resolved (with evidence)

- **`create_token`'s raw-token field** — `token`, confirmed against
  `docs/03-nodes/auth.create_token.md` Outputs table.
- **`set_password`'s failure output name in token mode** — `invalid` (not
  `error`), confirmed against `docs/03-nodes/auth.set_password.md` Outputs
  ("`invalid` is returned only in token mode when the token is unknown,
  expired, or already used ... `error` is also returned ... when `user_id`
  does not match any row").
- **`revoke_session` on an unknown/already-revoked token** — `success` with
  `revoked_count: 0`, not an error, confirmed against
  `docs/03-nodes/auth.revoke_session.md` Behavior ("Sets `revoked_at` on
  matching, not-already-revoked rows ... 0 if nothing matched — not an
  error"). No separate error routing was added because the node truly has no
  output for this case to route.
- **The `set_password`-revokes-sessions interaction with the final
  revoke-session step** — chose to assert the *observed, documented*
  behavior rather than mint a fresh session to work around it: the session
  created earlier is already revoked by the time `set_password` (default
  `revoke_sessions: true`) runs, so the final `revoke_session` call
  legitimately demonstrates idempotent success (`204`) on an
  already-revoked token, which is itself a useful, honest thing for this
  cookbook to show.
