# Auth Plugin Design — `plugins/auth`

**Date:** 2026-07-04
**Status:** Approved design, pending implementation plan
**Goal:** Make Noda a viable Supabase replacement by adding its missing pillar: first-party authentication (registration, login, sessions, email verification, password reset) — designed the Noda way: security-critical primitives in audited plugin code, auth *flows* scaffolded into the project as visible, editable workflows.

## 1. Motivation & Model

Noda today only *validates* externally issued credentials (`auth.jwt`, `auth.oidc` middleware). There is no user store, no password handling, no session lifecycle. Supabase/Firebase/Auth0 solve this with a sealed auth service; customizing their flows means webhooks and workarounds.

Noda's differentiator is that everything is a visible workflow. This design follows the **shadcn model**:

- The **plugin owns what must never be wrong**: password hashing, constant-time verification, token generation/consumption, session storage. These live in Go nodes and a middleware.
- The **project owns the flows**: `noda auth init` scaffolds routes, workflows, migrations, and tests into the user's project as plain JSON/SQL. Users open `register` in the visual editor and insert an invite-code check or a welcome email directly into the flow.

### In scope (v1)

Email/password auth: register, login, logout, current-user, email verification, password reset. Opaque server-side sessions (cookie + bearer). Scaffolding command. Editor/doc integration.

### Out of scope (v1, planned later)

Social login (OAuth account linking on top of existing `oidc.*` nodes), MFA/TOTP, magic links, JWT access + refresh mode, org/team memberships, row-level security (this design provides its hook: `auth.user_id` in expressions).

## 2. Architecture

Three layers:

| Layer | Location | Owns |
|---|---|---|
| Plugin | `plugins/auth/` | `auth` service (config: argon2id params, TTLs, cookie settings) + 8 primitive nodes |
| Runtime | `internal/server/` | `auth.session` middleware (validates sessions, populates the same locals as `auth.jwt`) |
| Project | scaffolded by `noda auth init` | migrations, routes, workflows, workflow tests |

**Plugin contract:** standard `api.Plugin`, `Name() = "auth"`, `Prefix() = "auth"`, `HasServices() = true`. `CreateService(config)` returns an `*auth.Service` holding *validated config and crypto helpers only — no DB handle*. Nodes declare two `ServiceDeps`, mirroring `db.query`:

```go
func (d *createUserDescriptor) ServiceDeps() map[string]api.ServiceDep {
    return map[string]api.ServiceDep{
        "auth":     {Prefix: "auth", Required: true},
        "database": {Prefix: "db", Required: true},
    }
}
```

This keeps the plugin inside existing contracts (no registry changes, no cross-service resolution at `CreateService` time). The DB that auth tables live in is the app's own database.

**Middleware:** `auth.session` is implemented in `internal/server` alongside `auth.jwt`/`auth.oidc` (plugins cannot register middleware). It needs the auth service + DB at request time. Mechanism: a `SessionAuthenticator` interface in `pkg/api` (implemented by the auth service; takes the DB handle as `any`) plus a server-scoped middleware map checked by a new `Server.buildMiddleware` method before the global registry — `internal/server` keeps depending only on `pkg/api`, other middleware factories unchanged.

### Service config

```json
{
  "services": {
    "auth": {
      "plugin": "auth",
      "config": {
        "database": "postgres",
        "session": {
          "ttl": "720h",
          "cookie": {
            "name": "noda_session",
            "secure": true,
            "http_only": true,
            "same_site": "Lax",
            "path": "/"
          }
        },
        "argon2": { "memory_kib": 65536, "iterations": 3, "parallelism": 2, "salt_len": 16, "key_len": 32 },
        "tokens": { "verify_email_ttl": "24h", "reset_password_ttl": "1h" }
      }
    }
  }
}
```

All fields optional except `database`; defaults as shown (argon2id defaults follow OWASP recommendations). `database` names the service whose GORM handle the *middleware* uses; nodes get their DB via `ServiceDeps` + node-level service binding like every other db-dependent node.

## 3. Data model

Three tables, shipped as **project-owned SQL migrations** (written by the scaffold into `migrations/`, applied via existing `noda migrate`). Postgres and SQLite both supported (same portability bar as `plugins/db`): since migrations are raw SQL, the scaffold reads the project's db service `driver` and emits dialect-appropriate SQL (the Postgres variant is shown below; the SQLite variant uses `TEXT` timestamps and `TEXT` for the json columns). Node/middleware queries stick to GORM constructs that work on both.

```sql
CREATE TABLE auth_users (
  id                TEXT PRIMARY KEY,            -- uuid
  email             TEXT NOT NULL UNIQUE,        -- stored lowercased/trimmed
  password_hash     TEXT NOT NULL,
  email_verified_at TIMESTAMPTZ,
  status            TEXT NOT NULL DEFAULT 'active',  -- 'active' | 'disabled'
  roles             JSONB NOT NULL DEFAULT '["user"]',
  metadata          JSONB NOT NULL DEFAULT '{}',
  created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE auth_sessions (
  id           TEXT PRIMARY KEY,
  user_id      TEXT NOT NULL REFERENCES auth_users(id) ON DELETE CASCADE,
  token_hash   TEXT NOT NULL UNIQUE,             -- SHA-256 hex of raw token
  created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
  expires_at   TIMESTAMPTZ NOT NULL,
  last_used_at TIMESTAMPTZ,
  ip           TEXT,
  user_agent   TEXT,
  revoked_at   TIMESTAMPTZ
);
CREATE INDEX idx_auth_sessions_user ON auth_sessions(user_id);

CREATE TABLE auth_tokens (
  id          TEXT PRIMARY KEY,
  user_id     TEXT NOT NULL REFERENCES auth_users(id) ON DELETE CASCADE,
  purpose     TEXT NOT NULL,                     -- 'verify_email' | 'reset_password'
  token_hash  TEXT NOT NULL UNIQUE,
  expires_at  TIMESTAMPTZ NOT NULL,
  consumed_at TIMESTAMPTZ,
  created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_auth_tokens_user_purpose ON auth_tokens(user_id, purpose);
```

Raw session/one-time tokens are **never stored** — only SHA-256 hashes. Users extend the schema by editing the migration (pre-first-deploy) or adding their own follow-up migrations; `metadata` jsonb is the no-migration escape hatch.

## 4. Session model

- Opaque 256-bit random tokens (`crypto/rand`, base64url). Not JWTs: instant revocation, single code path, no refresh dance. JWT+refresh is a possible later mode; `util.jwt_sign` remains available for service-to-service tokens.
- Nodes never touch the HTTP response directly (cookies are set by `response.json`). `auth.create_session` returns both the raw token (mobile/API clients use `Authorization: Bearer`) and a ready-made cookie object that the scaffolded flows pass to `response.json`'s `cookies` config (browser clients).
- Fixed TTL (default 30 days), no sliding expiration in v1.
- Passwords hashed with **argon2id** (`golang.org/x/crypto/argon2`). Verification detects hash format by prefix: `$argon2id$…` and `$2a$/$2b$` (bcrypt) both verify, enabling user imports from existing apps. New/updated passwords always argon2id; a successful bcrypt login transparently re-hashes to argon2id (opportunistic upgrade).

## 5. Nodes

All nodes: prefix `auth`, standard descriptor (name, description, `ConfigSchema`, `OutputDescriptions`, `ServiceDeps` as in §2), config values resolved as expressions via `internal/plugin` helpers. Structured outputs listed per node; all also have `error` for infrastructure failures (DB down etc.). Domain failures use dedicated outputs, never `error`.

### `auth.create_user`
Config: `email` (required), `password` (required), `roles` (optional array, default `["user"]`), `metadata` (optional object).
Behavior: normalize email (trim, lowercase); validate password length (min 8, max 512 — no other composition rules); argon2id-hash; insert. Unique violation → `exists`.
Outputs: `success` (user object, `password_hash` stripped), `exists`, `error`.

### `auth.verify_credentials`
Config: `email`, `password` (both required).
Behavior: lookup by normalized email; verify hash (argon2id or bcrypt by prefix). **When user is missing, verify against a fixed dummy hash so timing does not reveal account existence.** Disabled users → `invalid`. Successful bcrypt verify triggers re-hash to argon2id.
Outputs: `success` (user, hash stripped), `invalid`, `error`. `invalid` carries no reason; the reason (no such user / wrong password / disabled) goes to trace/debug logs only.

### `auth.create_session`
Config: `user_id` (required), `ttl` (optional, defaults from service config).
Behavior: mint 256-bit token; store SHA-256 + `expires_at` + request `ip`/`user_agent` (from execution context when available).
Outputs: `success` (`{ token, session_id, expires_at, cookie }` — `cookie` is a ready-made object matching `response.json`'s cookie fields, built from service cookie config), `error`.

### `auth.revoke_session`
Config: exactly one of `token` (revoke that session), `session_id`, or `user_id` (revoke **all** sessions for the user).
Behavior: set `revoked_at`. Unknown/already-revoked token still → `success` (idempotent).
Outputs: `success` (`{ revoked_count, clear_cookie }` — `clear_cookie` is a cookie object with empty value and `max_age: -1` for `response.json`), `error`.

### `auth.create_token`
Config: `user_id` (required), `purpose` (required: `verify_email` | `reset_password`), `ttl` (optional, defaults from service config per purpose).
Behavior: mint one-time token, store hash; creating a new token for the same `user_id`+`purpose` invalidates prior unconsumed ones.
Outputs: `success` (`{ token, expires_at }`) — the raw token exists only in workflow state, to be sent via `email.send`; `error`.

### `auth.consume_token`
Config: `token` (required), `purpose` (required, must match).
Behavior: hash, lookup, check purpose/expiry/unconsumed; consume **atomically** (`UPDATE … WHERE consumed_at IS NULL` guard) so a token can never be consumed twice, including concurrently. For `verify_email`, also sets `auth_users.email_verified_at`.
Outputs: `success` (`{ user_id }`), `invalid` (expired, wrong purpose, already used, unknown — undifferentiated), `error`.

### `auth.set_password`
Config: `user_id` (required), `password` (required), `revoke_sessions` (optional bool, default `true`).
Behavior: validate + argon2id-hash new password; update; revoke all user sessions by default (post-reset hygiene).
Outputs: `success`, `error`.

### `auth.get_user`
Config: exactly one of `user_id` or `email`.
Behavior: fetch user, strip `password_hash`.
Outputs: `success` (user), `not_found`, `error`.

Deliberately *not* nodes: user listing/updating/deleting — the tables are project-owned, so `db.find`/`db.update`/`db.delete` already cover admin CRUD (workflows must avoid selecting `password_hash` into responses; the scaffolded `/auth/me` flow uses `auth.get_user`).

## 6. `auth.session` middleware

- Name `auth.session`; config path `security.session` (added to `middlewareConfigPaths`), plus `middleware_instances` support like other auth middleware. Config: `{ "service": "auth" }` (which auth service instance; default `"auth"`).
- Token source: session cookie (name from service config) first, then `Authorization: Bearer <token>`.
- Validation: SHA-256 the token; single query joining session (not revoked, not expired) + user (status `active`).
- On success, populates the **same Fiber locals as `auth.jwt`** so downstream is uniform: `auth.user_id` (user id), `auth.roles` (from `auth_users.roles`), `auth.claims` (`{ sub, email, email_verified, session_id, roles }`). `casbin.enforce` chains after it unchanged; ordering validation (`ValidateMiddlewareOrder`) treats it like the other auth middleware.
- On failure: 401 with the same generic JSON error shape as `auth.jwt`. No detail about why.
- `last_used_at` updated at most once per minute per session (throttled write).
- Middleware ordering: `casbin.enforce` must follow `auth.session` *or* `auth.jwt`/`auth.oidc`.

## 7. Scaffolding — `noda auth init`

New cobra subcommand in `cmd/noda` (templates under `cmd/noda/templates/auth/`, following `noda init` precedent). Given a project dir, it writes:

- `migrations/<timestamp>_auth_tables.up.sql` / `.down.sql` (§3 schema)
- `workflows/auth.register.json`, `auth.login.json`, `auth.logout.json`, `auth.me.json`, `auth.verify-email.json`, `auth.request-password-reset.json`, `auth.reset-password.json`
- `routes/auth.json` — the seven routes below
- `tests/auth.*.json` — workflow tests for every flow (happy path + key failure path each)
- Patches `noda.json`: adds the `auth` service block (§2) and a `middleware_presets.authenticated_session: ["auth.session"]` preset. If an `email` service is absent, scaffold still completes but prints a warning and the verify/reset workflows are generated with the `email.send` node present-but-documented (user must configure an email service before those flows work).

The command **never overwrites existing files**: any collision aborts with a listing before writing anything (all-or-nothing).

### Generated routes & flows

| Route | Workflow (nodes, simplified) |
|---|---|
| `POST /auth/register` | `auth.create_user` → `auth.create_token(verify_email)` → `email.send` → `auth.create_session` → `response.json` 201; `exists` → generic 400 |
| `POST /auth/login` | `auth.verify_credentials` → `auth.create_session` → `response.json`; `invalid` → generic 401 |
| `POST /auth/logout` | `auth.revoke_session(token from cookie/bearer)` → 204 (route uses `auth.session`) |
| `GET /auth/me` | `auth.get_user(auth.user_id)` → `response.json` (route uses `auth.session`) |
| `POST /auth/verify-email` | `auth.consume_token(verify_email)` → 200; `invalid` → 400 |
| `POST /auth/request-password-reset` | `auth.get_user(email)`; `success` → `auth.create_token(reset_password)` → `email.send`; **both branches → identical 200** (no user enumeration — the branch is visible in the editor) |
| `POST /auth/reset-password` | `auth.consume_token(reset_password)` → `auth.set_password` → 200; `invalid` → 400 |

Security defaults baked into the generated config: `limiter` middleware on register/login/request-password-reset/reset-password; cookie is HttpOnly + Secure + SameSite=Lax; all failure responses are generic. Generated route config includes short comments (`"_comment"` keys or per-file README) pointing at customization spots.

## 8. Security requirements (plugin invariants)

1. Argon2id for all new password hashes; parameters configurable, OWASP defaults.
2. Constant-time comparisons throughout; dummy-hash verification on unknown email (§5).
3. Raw tokens (session + one-time) never persisted or logged — hashes only. Trace output must redact `password`, `token`, and `password_hash` fields (integrates with existing trace redaction if present; otherwise nodes mark these fields sensitive).
4. One-time tokens: single-use enforced atomically; purpose-bound; TTL-bound.
5. Session fixation: `auth.create_session` always mints a fresh token (login never reuses).
6. No user enumeration through node outputs, response bodies, or status codes in the scaffolded flows; credential verification is additionally timing-safe (dummy-hash). Known residual: `request-password-reset` does more work for existing users (token + email) — documented in the guide, with `event.emit`-based async decoupling as the recommended hardening for projects that need it.
7. Password reset revokes all existing sessions by default.
8. Cookie auth + state-changing routes: scaffolded config keeps SameSite=Lax; docs note enabling `security.csrf` middleware when hosting the frontend cross-site.

## 9. Error handling

- Domain outcomes (`exists`, `invalid`, `not_found`) are node outputs — workflows branch on them; they carry no sensitive detail.
- `error` output = infrastructure failure only; message safe for logs, workflow decides the response (scaffolded flows → generic 500 via `response.error`).
- Service config validation (bad TTL, unknown database service, malformed argon2 params) fails at startup via `CreateService`, consistent with other plugins.

## 10. Testing

Bottom-up per project convention:

1. **Unit** (`plugins/auth/*_test.go`): hashing round-trips + bcrypt migration path, token mint/consume atomicity (concurrent consumption race test with polarity-flipped assertion — must fail if the atomic guard is removed), expiry math, email normalization, cookie attribute construction. Target ≥75% coverage like other plugins.
2. **Integration** (real config from `testdata/`, SQLite): each node against a real DB; middleware validation paths (valid/expired/revoked/disabled-user/bearer-vs-cookie).
3. **E2E**: new `examples/auth-demo/` project created *by running the scaffold*; its generated workflow tests (register→verify→login→me→logout, reset flow, enumeration-safe reset request) run under the existing workflow test runner. Email assertions via the Mailpit setup used by email e2e tests (note: known flaky container startup — rerun policy applies).
4. **Docker Compose stays green**; `noda auth init` output validates against config schemas (`noda validate`).

## 11. Deliverables checklist

- `plugins/auth/` (plugin, service, 8 nodes, tests)
- `internal/server` `auth.session` middleware + ordering rules + tests
- `cmd/noda` `auth init` subcommand + templates + tests
- `docs/03-nodes/auth.*.md` (8 files), `docs/02-config/middleware.md` (auth.session section), `docs/02-config/noda-json.md` (auth service section), guide `docs/04-guides/authentication.md`
- `examples/auth-demo/`
- Editor: nodes appear automatically via descriptors; verify config schema fields render.

## 12. Future work (explicit non-goals now)

Social login (OAuth linking tables + `auth.link_identity` node on top of `oidc.*`), MFA/TOTP + recovery codes, magic-link login (trivial on top of `auth.create_token`/`consume_token` with a new purpose), JWT access + refresh mode, sliding sessions, org/team model, admin UI in the editor, row-level security built on `auth.user_id` expression context.

## Deviations from spec (as merged)

- **Session `ip`/`user_agent` columns exist but are not yet populated.** The `auth_sessions` schema carries `ip` and `user_agent` columns per the design, but `auth.create_session` never writes them: `ExecutionContext` has no accessor for request metadata (client IP, User-Agent header) today. Populating them is a follow-up once `ExecutionContext` exposes request meta.
- **Engine e2e uses a mocked mailer instead of Mailpit.** The workflow-runner-level e2e suite in `plugins/auth` stubs the mailer to keep those tests fast and hermetic; live-SMTP coverage (real Mailpit container) is provided separately by the `docker-compose` e2e suite, consistent with how other plugins split unit-speed vs. live-integration coverage.
- **Routes scaffolded as 7 files rather than one `routes/auth.json`.** `noda auth init` emits one route file per endpoint (`auth.register.json`, `auth.login.json`, etc.) instead of a single combined `routes/auth.json`, matching the one-concept-per-file convention already used for workflows and tests in the scaffold output.
- **Scaffolded config carries no `_comment` keys.** Rather than inline `"_comment"` fields in the generated JSON (which the design allowed as one option), customization guidance lives in `docs/04-guides/authentication.md` and the `examples/auth-demo/README.md`, keeping the generated config schema-clean.
