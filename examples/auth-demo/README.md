# Auth Demo

A minimal example showing the auth flows scaffolded by `noda auth init`:
register, verify email, login, get the current user, request/complete a
password reset, and logout — backed by Postgres and Mailpit.

## What got scaffolded

This project started as an empty `noda.json` declaring a `main-db` (Postgres)
and `email` (SMTP) service, then:

```bash
go run ./cmd/noda auth init --dir examples/auth-demo
```

added everything else:

- `migrations/*_auth_tables.up.sql` / `.down.sql` — `auth_users`, `auth_sessions`, `auth_tokens` tables
- `workflows/auth.*.json` — register, login, logout, me, verify-email, resend-verification, request-password-reset, reset-password
- `routes/auth.*.json` — the HTTP routes that trigger each workflow
- `tests/test-auth-*.json` — workflow tests with every plugin node mocked (run with `noda test`)
- `services.auth`, an `authenticated_session` middleware preset, and a
  `middleware.limiter` config added to `noda.json`

Nothing here is hand-written application code — it's all config, and it's
yours to edit. Open any `workflows/auth.*.json` file in the visual editor
(`noda start --editor`) to see and change the node graph.

## Running

```bash
docker compose -f examples/auth-demo/docker-compose.yml up -d --build
```

Wait for Postgres to report healthy, then apply the auth migrations:

```bash
docker compose -f examples/auth-demo/docker-compose.yml run --rm noda \
  migrate up --config /app/config --service main-db
```

Restart the `noda` service afterwards so it picks up the new tables:

```bash
docker compose -f examples/auth-demo/docker-compose.yml restart noda
```

Mailpit's web UI (inspect the verification/reset emails the demo sends) is at
<http://localhost:8025>.

## Try it: register → verify → login → me → logout

```bash
# 1. Register (201, sets a session cookie, sends a verification email)
curl -i -c cookies.txt -X POST http://localhost:3000/auth/register \
  -H 'Content-Type: application/json' \
  -d '{"email": "alice@example.com", "password": "password123"}'

# 2. Grab the verification token from Mailpit and confirm the email
#    (open http://localhost:8025, find the "Verify your email" message,
#    copy the token out of the body)
curl -i -X POST http://localhost:3000/auth/verify-email \
  -H 'Content-Type: application/json' \
  -d '{"token": "<paste-token-here>"}'

# 3. Login (200, sets a fresh session cookie)
curl -i -c cookies.txt -X POST http://localhost:3000/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"email": "alice@example.com", "password": "password123"}'

# 4. Who am I (200, uses the session cookie)
curl -i -b cookies.txt http://localhost:3000/auth/me

# 5. Logout (204, revokes the session)
curl -i -b cookies.txt -X POST http://localhost:3000/auth/logout
```

Forgot-password flow works the same way: `POST /auth/request-password-reset`
with `{"email": ...}` (always returns the same generic 200 response, whether
or not the address exists — check Mailpit for the reset token), then
`POST /auth/reset-password` with `{"token": ..., "password": "new-password"}`.
Resetting the password revokes all of that user's existing sessions.

## Tests

```bash
go run ./cmd/noda test --config examples/auth-demo --verbose
```

Every scaffolded test mocks the `auth.*` and `email.send` nodes, so this runs
without a database or SMTP server.

## Customization points

- **Invite codes / signup gating** — add a node before `create_user` in
  `workflows/auth.register.json` (e.g. `db.find` against an `invites` table,
  routed through `control.if`) to reject registrations without a valid code.
- **Extra profile fields** — `auth.create_user`'s `metadata` config accepts
  any object; add fields to the register route's request body and pass them
  through as `"metadata": "{{ input.metadata }}"`.
- **Welcome emails** — add an `email.send` node after `session` in
  `workflows/auth.register.json` (parallel to the verification email) once
  the account is created.
- **Roles** — `auth.create_user`'s `roles` config defaults to `["user"]`;
  set it from input (with server-side validation!) or hard-code additional
  roles for invite-only signup flows.

## Environment Variables

| Variable | Description | Example |
|----------|--------------|---------|
| `DATABASE_URL` | PostgreSQL connection string | `postgres://noda:noda@postgres:5432/noda?sslmode=disable` |
| `SMTP_HOST` | SMTP server host | `mailpit` |

## Teardown

```bash
docker compose -f examples/auth-demo/docker-compose.yml down -v
```
