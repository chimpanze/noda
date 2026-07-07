# Homebase Foundation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build `projects/homebase/` — a config-only Noda project giving the owner a single-admin "drops" feed (markdown text + files), revocable share links, and per-device sessions, E2E-tested against the real docker-compose stack.

**Architecture:** Pure Noda JSON config, zero application code. Postgres (via `db` plugin) holds auth tables + `drops` + `share_links`; file bytes go through the `storage` plugin to a compose volume; auth is the first-party `auth` plugin with opaque sessions and a token-guarded one-time `/setup`. Every endpoint is one route file + one workflow file; each workflow has a `tests/` suite; a Go E2E test drives the whole compose stack.

**Tech Stack:** Noda (this repo, run via `go run ./cmd/noda`), PostgreSQL 16, Docker Compose, Caddy (production edge), Go stdlib for the E2E suite.

**Spec:** `docs/superpowers/specs/2026-07-07-homebase-foundation-design.md`

## Global Constraints

- **Config only.** No Go code except the E2E test. All behavior lives in `projects/homebase/*.json`.
- **Branch:** all work on `homebase-foundation` (already exists; spec committed there).
- **Server:** default port 3000; `body_limit` 1073741824 (1 GiB, literal — the server config does not take expressions).
- **Rate limiter:** one shared config `{ "max": 30, "expiration": "1m" }`, applied per-route via `"middleware": ["limiter"]` on `/setup`, `/auth/login`, and both `/s/*` routes.
- **IDs:** every app-generated id/token is TEXT, produced with `{{ $uuid() }}` in workflows (matches the auth plugin's TEXT ids).
- **No multi-in-edge joins.** The engine treats a node with several incoming edges as a wait-all join; on mutually exclusive branches that starves and fails loudly. Rule: every branch terminates in its OWN response node, even when several responses are identical.
- **Workflow tests mock every service-backed node** (`db.*`, `auth.*`, `storage.*`, `upload.*`) — `noda test` runs with a core-only node registry and no live services. Core nodes (`control.*`, `transform.*`, `response.*`, `util.*`) execute for real. Mocking a core node (e.g. `respond`) is allowed and follows the `examples/auth-demo` pattern.
- **Uniform-response security rules:** public `/s/*` failures are always `404` code `NOT_FOUND` message `"not found"`; `/setup` failures are always `403` code `SETUP_DISABLED` message `"setup unavailable"`.
- **Verification env:** config load resolves `$env(...)`, so prefix every `validate`/`test` command with:
  `DATABASE_URL='postgres://noda:noda@localhost:5432/noda?sslmode=disable' FILES_PATH=/tmp/homebase-files SETUP_TOKEN=test-setup-token PUBLIC_BASE_URL=http://localhost:3000`
  (No live Postgres needed — these commands only parse config and run mocked workflows.)
- **Commits:** one per task, on `homebase-foundation`, message style `feat(homebase): <what>`, ending with the Claude co-author trailer.

---

### Task 1: Project skeleton — services, migrations, compose

**Files:**
- Create: `projects/homebase/noda.json`
- Create: `projects/homebase/.env.example`
- Create: `projects/homebase/.gitignore`
- Create: `projects/homebase/migrations/20260707000001_auth_tables.up.sql`
- Create: `projects/homebase/migrations/20260707000001_auth_tables.down.sql`
- Create: `projects/homebase/migrations/20260707000002_homebase.up.sql`
- Create: `projects/homebase/migrations/20260707000002_homebase.down.sql`
- Create: `projects/homebase/docker-compose.yml`
- Create: `projects/homebase/Caddyfile`
- Create: `projects/homebase/README.md` (stub; Task 10 completes it)

**Interfaces:**
- Produces: service names used by every later task — database `main-db`, auth `auth`, storage `files`; tables `auth_users`, `auth_sessions`, `auth_tokens`, `drops`, `share_links`; middleware name `limiter`; env contract `DATABASE_URL`, `FILES_PATH`, `SETUP_TOKEN`, `PUBLIC_BASE_URL`.

- [ ] **Step 1: Write `noda.json`**

```json
{
  "server": {
    "body_limit": 1073741824
  },
  "global_middleware": ["recover", "requestid", "logger"],
  "middleware": {
    "limiter": { "max": 30, "expiration": "1m" }
  },
  "security": {},
  "services": {
    "main-db": {
      "plugin": "db",
      "config": { "driver": "postgres", "url": "{{ $env('DATABASE_URL') }}" }
    },
    "auth": {
      "plugin": "auth",
      "config": { "database": "main-db" }
    },
    "files": {
      "plugin": "storage",
      "config": { "backend": "local", "path": "{{ $env('FILES_PATH') }}" }
    }
  }
}
```

- [ ] **Step 2: Write `.env.example` and `.gitignore`**

`.env.example`:
```bash
# --- required on the server ---
SETUP_TOKEN=change-me-to-a-long-random-string
PUBLIC_BASE_URL=https://your.domain
# domain Caddy serves (production edge profile only)
DOMAIN=your.domain

# --- only needed when running noda outside docker ---
DATABASE_URL=postgres://noda:noda@localhost:5432/noda?sslmode=disable
FILES_PATH=./data/files
```

`.gitignore`:
```
.env
data/
```

- [ ] **Step 3: Write the auth migration (verbatim copy of `examples/auth-demo/migrations/20260704201244_auth_tables.{up,down}.sql`)**

`migrations/20260707000001_auth_tables.up.sql`:
```sql
CREATE TABLE auth_users (
  id                TEXT PRIMARY KEY,
  email             TEXT NOT NULL UNIQUE,
  password_hash     TEXT NOT NULL,
  email_verified_at TIMESTAMPTZ,
  status            TEXT NOT NULL DEFAULT 'active',
  roles             JSONB NOT NULL DEFAULT '["user"]',
  metadata          JSONB NOT NULL DEFAULT '{}',
  created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE auth_sessions (
  id           TEXT PRIMARY KEY,
  user_id      TEXT NOT NULL REFERENCES auth_users(id) ON DELETE CASCADE,
  token_hash   TEXT NOT NULL UNIQUE,
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
  purpose     TEXT NOT NULL,
  token_hash  TEXT NOT NULL UNIQUE,
  expires_at  TIMESTAMPTZ NOT NULL,
  consumed_at TIMESTAMPTZ,
  created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_auth_tokens_user_purpose ON auth_tokens(user_id, purpose);
```

`migrations/20260707000001_auth_tables.down.sql`:
```sql
DROP TABLE auth_tokens;
DROP TABLE auth_sessions;
DROP TABLE auth_users;
```

- [ ] **Step 4: Write the homebase migration**

`migrations/20260707000002_homebase.up.sql`:
```sql
CREATE TABLE drops (
  id         TEXT PRIMARY KEY,
  text       TEXT,
  file_name  TEXT,
  file_key   TEXT,
  file_size  BIGINT,
  file_mime  TEXT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT drop_has_content CHECK (text IS NOT NULL OR file_key IS NOT NULL)
);
CREATE INDEX idx_drops_created ON drops(created_at DESC);

CREATE TABLE share_links (
  id         TEXT PRIMARY KEY,
  drop_id    TEXT NOT NULL REFERENCES drops(id) ON DELETE CASCADE,
  token      TEXT NOT NULL UNIQUE,
  expires_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_share_links_drop ON share_links(drop_id);
```

`migrations/20260707000002_homebase.down.sql`:
```sql
DROP TABLE share_links;
DROP TABLE drops;
```

- [ ] **Step 5: Write `docker-compose.yml`**

Pattern follows `examples/auth-demo/docker-compose.yml`, plus a one-shot `migrate` service so `docker compose up` alone yields a fully migrated system (spec: "Docker Compose always green").

```yaml
services:
  noda:
    build: ../..
    ports:
      - "3000:3000"
    depends_on:
      postgres:
        condition: service_healthy
      migrate:
        condition: service_completed_successfully
    environment:
      DATABASE_URL: postgres://noda:noda@postgres:5432/noda?sslmode=disable
      FILES_PATH: /data/files
      SETUP_TOKEN: ${SETUP_TOKEN:?set SETUP_TOKEN in .env}
      PUBLIC_BASE_URL: ${PUBLIC_BASE_URL:-http://localhost:3000}
    volumes:
      - .:/app/config
      - files-data:/data/files
    command: ["start", "--config", "/app/config", "--server"]

  migrate:
    build: ../..
    depends_on:
      postgres:
        condition: service_healthy
    environment:
      DATABASE_URL: postgres://noda:noda@postgres:5432/noda?sslmode=disable
      FILES_PATH: /data/files
    volumes:
      - .:/app/config
    command: ["migrate", "up", "--config", "/app/config", "--service", "main-db"]

  postgres:
    image: postgres:16-alpine
    environment:
      POSTGRES_USER: noda
      POSTGRES_PASSWORD: noda
      POSTGRES_DB: noda
    volumes:
      - pgdata:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U noda -d noda"]
      interval: 5s
      timeout: 5s
      retries: 5

  caddy:
    image: caddy:2
    profiles: ["edge"]
    ports:
      - "80:80"
      - "443:443"
    environment:
      DOMAIN: ${DOMAIN:?set DOMAIN in .env}
    volumes:
      - ./Caddyfile:/etc/caddy/Caddyfile
      - caddy-data:/data

volumes:
  pgdata:
  files-data:
  caddy-data:
```

- [ ] **Step 6: Write `Caddyfile`**

```
{$DOMAIN} {
    reverse_proxy noda:3000
}
```

- [ ] **Step 7: Write `README.md` stub**

```markdown
# Homebase

Personal private-cloud API built on [Noda](../../README.md). Single admin, a
chronological "drops" feed (markdown text + files), revocable share links.

Spec: `docs/superpowers/specs/2026-07-07-homebase-foundation-design.md`.

(Full usage docs land with the E2E task.)
```

- [ ] **Step 8: Validate the config**

Run (from repo root):
```bash
DATABASE_URL='postgres://noda:noda@localhost:5432/noda?sslmode=disable' FILES_PATH=/tmp/homebase-files SETUP_TOKEN=test-setup-token PUBLIC_BASE_URL=http://localhost:3000 go run ./cmd/noda validate --config projects/homebase
```
Expected: validation succeeds (no routes/workflows yet is fine).

- [ ] **Step 9: Commit**

```bash
git add projects/homebase
git commit -m "feat(homebase): project skeleton — services, migrations, compose

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 2: `/setup` — one-time admin bootstrap

**Files:**
- Create: `projects/homebase/tests/test-setup.json`
- Create: `projects/homebase/workflows/setup.json`
- Create: `projects/homebase/routes/setup.json`

**Interfaces:**
- Consumes: services from Task 1.
- Produces: `POST /setup` `{setup_token, email, password}` → `201 {id, email}` on first run; `403 {"error":{"code":"SETUP_DISABLED","message":"setup unavailable",...}}` for wrong token AND already-initialized (identical bodies — no oracle).

- [ ] **Step 1: Write the failing test** — `tests/test-setup.json`:

```json
{
  "id": "test-setup",
  "workflow": "setup",
  "tests": [
    {
      "name": "creates admin when token matches and no users exist",
      "input": { "setup_token": "test-setup-token", "email": "admin@example.com", "password": "password123" },
      "mocks": {
        "count_users": { "output": { "count": 0 } },
        "create_admin": { "output": { "id": "u1", "email": "admin@example.com", "roles": ["admin"] } },
        "respond": { "output": { "status": 201 } }
      },
      "expect": { "status": "success", "output": { "respond.status": 201 } }
    },
    {
      "name": "wrong setup token gets 403",
      "input": { "setup_token": "wrong", "email": "admin@example.com", "password": "password123" },
      "mocks": {
        "respond_bad_token": { "output": { "status": 403 } }
      },
      "expect": { "status": "success", "output": { "respond_bad_token.status": 403 } }
    },
    {
      "name": "already initialized gets 403",
      "input": { "setup_token": "test-setup-token", "email": "admin@example.com", "password": "password123" },
      "mocks": {
        "count_users": { "output": { "count": 1 } },
        "respond_not_empty": { "output": { "status": 403 } }
      },
      "expect": { "status": "success", "output": { "respond_not_empty.status": 403 } }
    },
    {
      "name": "duplicate admin (setup race) gets 403",
      "input": { "setup_token": "test-setup-token", "email": "admin@example.com", "password": "password123" },
      "mocks": {
        "count_users": { "output": { "count": 0 } },
        "create_admin": { "output_name": "exists", "output": {} },
        "respond_exists": { "output": { "status": 403 } }
      },
      "expect": { "status": "success", "output": { "respond_exists.status": 403 } }
    }
  ]
}
```

- [ ] **Step 2: Run it to verify it fails**

```bash
DATABASE_URL='postgres://noda:noda@localhost:5432/noda?sslmode=disable' FILES_PATH=/tmp/homebase-files SETUP_TOKEN=test-setup-token PUBLIC_BASE_URL=http://localhost:3000 go run ./cmd/noda test --config projects/homebase
```
Expected: FAIL (config validation error: test references unknown workflow `setup`).

- [ ] **Step 3: Write `workflows/setup.json`**

The token check hashes both sides (`sha256(a) == sha256(b)`) so the string comparison can't leak matching-prefix timing. All three failure paths return byte-identical 403s via separate response nodes (no-joins rule).

```json
{
  "id": "setup",
  "name": "One-time admin bootstrap",
  "nodes": {
    "check_token": {
      "type": "control.if",
      "config": { "condition": "{{ sha256(input.setup_token) == sha256(secrets.SETUP_TOKEN) }}" }
    },
    "count_users": {
      "type": "db.count",
      "services": { "database": "main-db" },
      "config": { "table": "auth_users" }
    },
    "check_empty": {
      "type": "control.if",
      "config": { "condition": "{{ nodes.count_users.count == 0 }}" }
    },
    "create_admin": {
      "type": "auth.create_user",
      "services": { "auth": "auth", "database": "main-db" },
      "config": { "email": "{{ input.email }}", "password": "{{ input.password }}", "roles": ["admin"] }
    },
    "respond": {
      "type": "response.json",
      "config": { "status": 201, "body": { "id": "{{ nodes.create_admin.id }}", "email": "{{ nodes.create_admin.email }}" } }
    },
    "respond_bad_token": {
      "type": "response.error",
      "config": { "status": 403, "code": "SETUP_DISABLED", "message": "setup unavailable" }
    },
    "respond_not_empty": {
      "type": "response.error",
      "config": { "status": 403, "code": "SETUP_DISABLED", "message": "setup unavailable" }
    },
    "respond_exists": {
      "type": "response.error",
      "config": { "status": 403, "code": "SETUP_DISABLED", "message": "setup unavailable" }
    }
  },
  "edges": [
    { "from": "check_token", "output": "then", "to": "count_users" },
    { "from": "check_token", "output": "else", "to": "respond_bad_token" },
    { "from": "count_users", "to": "check_empty" },
    { "from": "check_empty", "output": "then", "to": "create_admin" },
    { "from": "check_empty", "output": "else", "to": "respond_not_empty" },
    { "from": "create_admin", "to": "respond" },
    { "from": "create_admin", "output": "exists", "to": "respond_exists" }
  ]
}
```

- [ ] **Step 4: Write `routes/setup.json`**

```json
{
  "id": "setup",
  "method": "POST",
  "path": "/setup",
  "summary": "One-time admin bootstrap",
  "tags": ["auth"],
  "middleware": ["limiter"],
  "body": {
    "schema": {
      "type": "object",
      "required": ["setup_token", "email", "password"],
      "properties": {
        "setup_token": { "type": "string", "minLength": 1 },
        "email": { "type": "string", "minLength": 3 },
        "password": { "type": "string", "minLength": 8, "maxLength": 512 }
      }
    }
  },
  "trigger": {
    "workflow": "setup",
    "input": {
      "setup_token": "{{ body.setup_token }}",
      "email": "{{ body.email }}",
      "password": "{{ body.password }}"
    }
  }
}
```

- [ ] **Step 5: Run tests to verify they pass**

Same command as Step 2. Expected: PASS (4/4).

- [ ] **Step 6: Commit**

```bash
git add projects/homebase
git commit -m "feat(homebase): one-time /setup admin bootstrap

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 3: Login, logout, me

**Files:**
- Create: `projects/homebase/tests/test-auth-login.json`, `tests/test-auth-logout.json`, `tests/test-auth-me.json`
- Create: `projects/homebase/workflows/auth.login.json`, `workflows/auth.logout.json`, `workflows/auth.me.json`
- Create: `projects/homebase/routes/auth.login.json`, `routes/auth.logout.json`, `routes/auth.me.json`

**Interfaces:**
- Consumes: Task 1 services.
- Produces: `POST /auth/login {email,password}` → `200 {user, token}` + session cookie / `401`; `POST /auth/logout` (session) → `204`; `GET /auth/me` (session) → `200 user`. Later tasks rely on `auth.session` middleware populating `auth.sub` and `auth.claims.session_id`.

- [ ] **Step 1: Write the failing tests**

`tests/test-auth-login.json`:
```json
{
  "id": "test-auth-login",
  "workflow": "auth-login",
  "tests": [
    {
      "name": "valid credentials get a session",
      "input": { "email": "admin@example.com", "password": "password123" },
      "mocks": {
        "verify": { "output": { "id": "u1", "email": "admin@example.com", "roles": ["admin"] } },
        "session": { "output": { "token": "tok", "session_id": "s1", "cookie": { "name": "noda_session", "value": "tok" } } },
        "respond": { "output": { "status": 200 } }
      },
      "expect": { "status": "success", "output": { "respond.status": 200 } }
    },
    {
      "name": "invalid credentials get 401",
      "input": { "email": "admin@example.com", "password": "wrong" },
      "mocks": {
        "verify": { "output_name": "invalid", "output": {} },
        "respond_invalid": { "output": { "status": 401 } }
      },
      "expect": { "status": "success", "output": { "respond_invalid.status": 401 } }
    }
  ]
}
```

`tests/test-auth-logout.json`:
```json
{
  "id": "test-auth-logout",
  "workflow": "auth-logout",
  "tests": [
    {
      "name": "revokes the current session",
      "input": { "session_id": "s1" },
      "mocks": {
        "revoke": { "output": { "revoked_count": 1, "clear_cookie": { "name": "noda_session", "value": "" } } },
        "respond": { "output": { "status": 204 } }
      },
      "expect": { "status": "success", "output": { "respond.status": 204 } }
    }
  ]
}
```

`tests/test-auth-me.json`:
```json
{
  "id": "test-auth-me",
  "workflow": "auth-me",
  "tests": [
    {
      "name": "returns the current user",
      "input": { "user_id": "u1" },
      "mocks": {
        "get": { "output": { "id": "u1", "email": "admin@example.com", "roles": ["admin"] } },
        "respond": { "output": { "status": 200 } }
      },
      "expect": { "status": "success", "output": { "respond.status": 200 } }
    },
    {
      "name": "missing user gets 404",
      "input": { "user_id": "ghost" },
      "mocks": {
        "get": { "output_name": "not_found", "output": {} },
        "respond_missing": { "output": { "status": 404 } }
      },
      "expect": { "status": "success", "output": { "respond_missing.status": 404 } }
    }
  ]
}
```

- [ ] **Step 2: Run to verify they fail**

```bash
DATABASE_URL='postgres://noda:noda@localhost:5432/noda?sslmode=disable' FILES_PATH=/tmp/homebase-files SETUP_TOKEN=test-setup-token PUBLIC_BASE_URL=http://localhost:3000 go run ./cmd/noda test --config projects/homebase
```
Expected: FAIL (unknown workflows `auth-login`, `auth-logout`, `auth-me`).

- [ ] **Step 3: Write workflows** (login is a verbatim copy of `examples/auth-demo/workflows/auth.login.json`)

`workflows/auth.login.json`:
```json
{
  "id": "auth-login",
  "name": "Auth: Login",
  "nodes": {
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
    },
    "respond": {
      "type": "response.json",
      "config": {
        "status": 200,
        "body": { "user": "{{ nodes.verify }}", "token": "{{ nodes.session.token }}" },
        "cookies": "{{ [nodes.session.cookie] }}"
      }
    }
  },
  "edges": [
    { "from": "verify", "to": "session" },
    { "from": "verify", "output": "invalid", "to": "respond_invalid" },
    { "from": "session", "to": "respond" }
  ]
}
```

`workflows/auth.logout.json`:
```json
{
  "id": "auth-logout",
  "name": "Auth: Logout",
  "nodes": {
    "revoke": {
      "type": "auth.revoke_session",
      "services": { "auth": "auth", "database": "main-db" },
      "config": { "session_id": "{{ input.session_id }}" }
    },
    "respond": {
      "type": "response.json",
      "config": { "status": 204, "cookies": "{{ [nodes.revoke.clear_cookie] }}" }
    }
  },
  "edges": [
    { "from": "revoke", "to": "respond" }
  ]
}
```

`workflows/auth.me.json`:
```json
{
  "id": "auth-me",
  "name": "Auth: Current user",
  "nodes": {
    "get": {
      "type": "auth.get_user",
      "services": { "auth": "auth", "database": "main-db" },
      "config": { "user_id": "{{ input.user_id }}" }
    },
    "respond": {
      "type": "response.json",
      "config": { "status": 200, "body": "{{ nodes.get }}" }
    },
    "respond_missing": {
      "type": "response.error",
      "config": { "status": 404, "code": "NOT_FOUND", "message": "user not found" }
    }
  },
  "edges": [
    { "from": "get", "to": "respond" },
    { "from": "get", "output": "not_found", "to": "respond_missing" }
  ]
}
```

- [ ] **Step 4: Write routes**

`routes/auth.login.json`:
```json
{
  "id": "auth-login",
  "method": "POST",
  "path": "/auth/login",
  "summary": "Log in with email and password",
  "tags": ["auth"],
  "middleware": ["limiter"],
  "body": {
    "schema": {
      "type": "object",
      "required": ["email", "password"],
      "properties": {
        "email": { "type": "string" },
        "password": { "type": "string" }
      }
    }
  },
  "trigger": {
    "workflow": "auth-login",
    "input": { "email": "{{ body.email }}", "password": "{{ body.password }}" }
  }
}
```

`routes/auth.logout.json`:
```json
{
  "id": "auth-logout",
  "method": "POST",
  "path": "/auth/logout",
  "summary": "Log out the current session",
  "tags": ["auth"],
  "middleware": ["auth.session"],
  "trigger": {
    "workflow": "auth-logout",
    "input": { "session_id": "{{ auth.claims.session_id }}" }
  }
}
```

`routes/auth.me.json`:
```json
{
  "id": "auth-me",
  "method": "GET",
  "path": "/auth/me",
  "summary": "Current user",
  "tags": ["auth"],
  "middleware": ["auth.session"],
  "trigger": {
    "workflow": "auth-me",
    "input": { "user_id": "{{ auth.sub }}" }
  }
}
```

- [ ] **Step 5: Run tests to verify they pass** (same command as Step 2). Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add projects/homebase
git commit -m "feat(homebase): login/logout/me with opaque sessions

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 4: Session listing & per-device revocation

**Files:**
- Create: `projects/homebase/tests/test-sessions-list.json`, `tests/test-session-revoke.json`
- Create: `projects/homebase/workflows/sessions.list.json`, `workflows/sessions.revoke.json`
- Create: `projects/homebase/routes/sessions.list.json`, `routes/sessions.revoke.json`

**Interfaces:**
- Consumes: `auth.session` middleware (`auth.sub`, `auth.claims.session_id`), `auth_sessions` table.
- Produces: `GET /auth/sessions` → `200 {sessions: [{id, created_at, last_used_at, expires_at, ip, user_agent}], current_session_id}`; `DELETE /auth/sessions/:id` → `204` / `404`.

- [ ] **Step 1: Write the failing tests**

`tests/test-sessions-list.json`:
```json
{
  "id": "test-sessions-list",
  "workflow": "sessions-list",
  "tests": [
    {
      "name": "lists active sessions",
      "input": { "user_id": "u1", "session_id": "s1" },
      "mocks": {
        "find": { "output": [ { "id": "s1", "created_at": "2026-07-07T10:00:00Z", "ip": "1.2.3.4" } ] },
        "respond": { "output": { "status": 200 } }
      },
      "expect": { "status": "success", "output": { "respond.status": 200 } }
    }
  ]
}
```

`tests/test-session-revoke.json`:
```json
{
  "id": "test-session-revoke",
  "workflow": "session-revoke",
  "tests": [
    {
      "name": "revokes an owned session",
      "input": { "session_id": "s2", "user_id": "u1" },
      "mocks": {
        "find": { "output": { "id": "s2", "user_id": "u1" } },
        "revoke": { "output": { "revoked_count": 1, "clear_cookie": { "name": "noda_session", "value": "" } } },
        "respond": { "output": { "status": 204 } }
      },
      "expect": { "status": "success", "output": { "respond.status": 204 } }
    },
    {
      "name": "unknown session gets 404",
      "input": { "session_id": "nope", "user_id": "u1" },
      "mocks": {
        "find": { "output_name": "error", "output": { "error": "not found" } },
        "respond_missing": { "output": { "status": 404 } }
      },
      "expect": { "status": "success", "output": { "respond_missing.status": 404 } }
    }
  ]
}
```

- [ ] **Step 2: Run to verify they fail** (standard test command). Expected: FAIL (unknown workflows).

- [ ] **Step 3: Write workflows**

`workflows/sessions.list.json`:
```json
{
  "id": "sessions-list",
  "name": "Sessions: List devices",
  "nodes": {
    "find": {
      "type": "db.find",
      "services": { "database": "main-db" },
      "config": {
        "table": "auth_sessions",
        "select": ["id", "created_at", "last_used_at", "expires_at", "ip", "user_agent"],
        "where_clause": {
          "query": "user_id = ? AND revoked_at IS NULL AND expires_at > now()",
          "params": ["{{ input.user_id }}"]
        },
        "order": "created_at DESC"
      }
    },
    "respond": {
      "type": "response.json",
      "config": {
        "status": 200,
        "body": { "sessions": "{{ nodes.find }}", "current_session_id": "{{ input.session_id }}" }
      }
    }
  },
  "edges": [
    { "from": "find", "to": "respond" }
  ]
}
```

`workflows/sessions.revoke.json` (the `find` guard proves ownership before revoking — `auth.revoke_session` itself does not check the user):
```json
{
  "id": "session-revoke",
  "name": "Sessions: Revoke one device",
  "nodes": {
    "find": {
      "type": "db.findOne",
      "services": { "database": "main-db" },
      "config": {
        "table": "auth_sessions",
        "select": ["id", "user_id"],
        "where": { "id": "{{ input.session_id }}", "user_id": "{{ input.user_id }}" },
        "required": true
      }
    },
    "revoke": {
      "type": "auth.revoke_session",
      "services": { "auth": "auth", "database": "main-db" },
      "config": { "session_id": "{{ input.session_id }}" }
    },
    "respond": {
      "type": "response.json",
      "config": { "status": 204 }
    },
    "respond_missing": {
      "type": "response.error",
      "config": { "status": 404, "code": "NOT_FOUND", "message": "session not found" }
    }
  },
  "edges": [
    { "from": "find", "to": "revoke" },
    { "from": "find", "output": "error", "to": "respond_missing" },
    { "from": "revoke", "to": "respond" }
  ]
}
```

- [ ] **Step 4: Write routes**

`routes/sessions.list.json`:
```json
{
  "id": "sessions-list",
  "method": "GET",
  "path": "/auth/sessions",
  "summary": "List active sessions (devices)",
  "tags": ["auth"],
  "middleware": ["auth.session"],
  "trigger": {
    "workflow": "sessions-list",
    "input": { "user_id": "{{ auth.sub }}", "session_id": "{{ auth.claims.session_id }}" }
  }
}
```

`routes/sessions.revoke.json`:
```json
{
  "id": "session-revoke",
  "method": "DELETE",
  "path": "/auth/sessions/:id",
  "summary": "Revoke one session (device)",
  "tags": ["auth"],
  "middleware": ["auth.session"],
  "trigger": {
    "workflow": "session-revoke",
    "input": { "session_id": "{{ params.id }}", "user_id": "{{ auth.sub }}" }
  }
}
```

- [ ] **Step 5: Run tests to verify they pass.** Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add projects/homebase
git commit -m "feat(homebase): session listing and per-device revocation

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 5: Text drops — create, list, search

**Files:**
- Create: `projects/homebase/tests/test-drop-create.json`, `tests/test-drops-list.json`
- Create: `projects/homebase/workflows/drops.create.json`, `workflows/drops.list.json`
- Create: `projects/homebase/routes/drops.create.json`, `routes/drops.list.json`

**Interfaces:**
- Consumes: `drops` table, `auth.session`.
- Produces: `POST /drops {text}` → `201 <drop row>`; `GET /drops?q=&before=` → `200 {drops: [...], next_before: <created_at of last item | null>}`. Drop row shape (used by Tasks 6–9 and E2E): `{id, text, file_name, file_key, file_size, file_mime, created_at, updated_at}` — list/get endpoints select everything EXCEPT `file_key`.

- [ ] **Step 1: Write the failing tests**

`tests/test-drop-create.json`:
```json
{
  "id": "test-drop-create",
  "workflow": "drop-create",
  "tests": [
    {
      "name": "creates a text drop",
      "input": { "text": "- [ ] buy milk" },
      "mocks": {
        "insert": { "output": { "id": "d1", "text": "- [ ] buy milk", "created_at": "2026-07-07T10:00:00Z" } },
        "respond": { "output": { "status": 201 } }
      },
      "expect": { "status": "success", "output": { "respond.status": 201 } }
    }
  ]
}
```

`tests/test-drops-list.json`:
```json
{
  "id": "test-drops-list",
  "workflow": "drops-list",
  "tests": [
    {
      "name": "lists drops newest first with cursor",
      "input": { "q": "", "before": "" },
      "mocks": {
        "find": { "output": [ { "id": "d2", "text": "newer", "created_at": "2026-07-07T11:00:00Z" }, { "id": "d1", "text": "older", "created_at": "2026-07-07T10:00:00Z" } ] },
        "respond": { "output": { "status": 200 } }
      },
      "expect": { "status": "success", "output": { "respond.status": 200 } }
    },
    {
      "name": "empty result gives null cursor",
      "input": { "q": "zzz", "before": "" },
      "mocks": {
        "find": { "output": [] },
        "respond": { "output": { "status": 200 } }
      },
      "expect": { "status": "success", "output": { "respond.status": 200 } }
    }
  ]
}
```

- [ ] **Step 2: Run to verify they fail.** Expected: FAIL (unknown workflows).

- [ ] **Step 3: Write workflows**

`workflows/drops.create.json`:
```json
{
  "id": "drop-create",
  "name": "Drops: Create text drop",
  "nodes": {
    "insert": {
      "type": "db.create",
      "services": { "database": "main-db" },
      "config": {
        "table": "drops",
        "data": {
          "id": "{{ $uuid() }}",
          "text": "{{ input.text }}"
        }
      }
    },
    "respond": {
      "type": "response.json",
      "config": { "status": 201, "body": "{{ nodes.insert }}" }
    }
  },
  "edges": [
    { "from": "insert", "to": "respond" }
  ]
}
```

`workflows/drops.list.json` — one `where_clause` covers both filters: empty `q` matches every row (`'%%'`), empty `before` disables the cursor via `NULLIF(...)::timestamptz` → `COALESCE(..., 'infinity')`:
```json
{
  "id": "drops-list",
  "name": "Drops: List & search",
  "nodes": {
    "find": {
      "type": "db.find",
      "services": { "database": "main-db" },
      "config": {
        "table": "drops",
        "select": ["id", "text", "file_name", "file_size", "file_mime", "created_at", "updated_at"],
        "where_clause": {
          "query": "(text ILIKE '%' || ? || '%' OR file_name ILIKE '%' || ? || '%') AND created_at < COALESCE(NULLIF(?, '')::timestamptz, 'infinity')",
          "params": ["{{ input.q }}", "{{ input.q }}", "{{ input.before }}"]
        },
        "order": "created_at DESC",
        "limit": 50
      }
    },
    "respond": {
      "type": "response.json",
      "config": {
        "status": 200,
        "body": {
          "drops": "{{ nodes.find }}",
          "next_before": "{{ len(nodes.find) > 0 ? nodes.find[-1].created_at : nil }}"
        }
      }
    }
  },
  "edges": [
    { "from": "find", "to": "respond" }
  ]
}
```

- [ ] **Step 4: Write routes**

`routes/drops.create.json`:
```json
{
  "id": "drop-create",
  "method": "POST",
  "path": "/drops",
  "summary": "Create a text drop",
  "tags": ["drops"],
  "middleware": ["auth.session"],
  "body": {
    "schema": {
      "type": "object",
      "required": ["text"],
      "properties": {
        "text": { "type": "string", "minLength": 1 }
      }
    }
  },
  "trigger": {
    "workflow": "drop-create",
    "input": { "text": "{{ body.text }}" }
  }
}
```

`routes/drops.list.json`:
```json
{
  "id": "drops-list",
  "method": "GET",
  "path": "/drops",
  "summary": "List and search drops, newest first",
  "tags": ["drops"],
  "middleware": ["auth.session"],
  "trigger": {
    "workflow": "drops-list",
    "input": {
      "q": "{{ query.q ?? '' }}",
      "before": "{{ query.before ?? '' }}"
    }
  }
}
```

- [ ] **Step 5: Run tests to verify they pass.** Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add projects/homebase
git commit -m "feat(homebase): text drops — create, list, ILIKE search, cursor pagination

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 6: Drop detail — get, edit text, delete

**Files:**
- Create: `projects/homebase/tests/test-drop-get.json`, `tests/test-drop-update.json`, `tests/test-drop-delete.json`
- Create: `projects/homebase/workflows/drops.get.json`, `workflows/drops.update.json`, `workflows/drops.delete.json`
- Create: `projects/homebase/routes/drops.get.json`, `routes/drops.update.json`, `routes/drops.delete.json`

**Interfaces:**
- Consumes: drop row shape from Task 5; `files` storage service.
- Produces: `GET /drops/:id` → `200 <drop sans file_key>` / `404`; `PATCH /drops/:id {text}` → `200 <updated drop>` / `404`; `DELETE /drops/:id` → `204` / `404` (row first, then bytes; failed byte-delete logs and still 204s).

- [ ] **Step 1: Write the failing tests**

`tests/test-drop-get.json`:
```json
{
  "id": "test-drop-get",
  "workflow": "drop-get",
  "tests": [
    {
      "name": "returns a drop",
      "input": { "id": "d1" },
      "mocks": {
        "get": { "output": { "id": "d1", "text": "hello", "created_at": "2026-07-07T10:00:00Z" } },
        "respond": { "output": { "status": 200 } }
      },
      "expect": { "status": "success", "output": { "respond.status": 200 } }
    },
    {
      "name": "unknown drop gets 404",
      "input": { "id": "nope" },
      "mocks": {
        "get": { "output_name": "error", "output": { "error": "not found" } },
        "respond_missing": { "output": { "status": 404 } }
      },
      "expect": { "status": "success", "output": { "respond_missing.status": 404 } }
    }
  ]
}
```

`tests/test-drop-update.json`:
```json
{
  "id": "test-drop-update",
  "workflow": "drop-update",
  "tests": [
    {
      "name": "updates text (tick a checkbox)",
      "input": { "id": "d1", "text": "- [x] buy milk" },
      "mocks": {
        "update": { "output": { "rows_affected": 1 } },
        "fetch": { "output": { "id": "d1", "text": "- [x] buy milk" } },
        "respond": { "output": { "status": 200 } }
      },
      "expect": { "status": "success", "output": { "respond.status": 200 } }
    },
    {
      "name": "unknown drop gets 404",
      "input": { "id": "nope", "text": "x" },
      "mocks": {
        "update": { "output": { "rows_affected": 0 } },
        "respond_missing": { "output": { "status": 404 } }
      },
      "expect": { "status": "success", "output": { "respond_missing.status": 404 } }
    }
  ]
}
```

`tests/test-drop-delete.json`:
```json
{
  "id": "test-drop-delete",
  "workflow": "drop-delete",
  "tests": [
    {
      "name": "deletes a text-only drop (no file to remove)",
      "input": { "id": "d1" },
      "mocks": {
        "get": { "output": { "id": "d1", "file_key": null } },
        "del_row": { "output": { "rows_affected": 1 } },
        "respond_no_file": { "output": { "status": 204 } }
      },
      "expect": { "status": "success", "output": { "respond_no_file.status": 204 } }
    },
    {
      "name": "deletes a file drop including bytes",
      "input": { "id": "d2" },
      "mocks": {
        "get": { "output": { "id": "d2", "file_key": "drops/abc" } },
        "del_row": { "output": { "rows_affected": 1 } },
        "del_file": { "output": {} },
        "respond_file_deleted": { "output": { "status": 204 } }
      },
      "expect": { "status": "success", "output": { "respond_file_deleted.status": 204 } }
    },
    {
      "name": "failed byte-delete still returns 204 (orphan logged)",
      "input": { "id": "d3" },
      "mocks": {
        "get": { "output": { "id": "d3", "file_key": "drops/gone" } },
        "del_row": { "output": { "rows_affected": 1 } },
        "del_file": { "output_name": "error", "output": { "error": "file not found" } },
        "respond_orphan": { "output": { "status": 204 } }
      },
      "expect": { "status": "success", "output": { "respond_orphan.status": 204 } }
    },
    {
      "name": "unknown drop gets 404",
      "input": { "id": "nope" },
      "mocks": {
        "get": { "output_name": "error", "output": { "error": "not found" } },
        "respond_missing": { "output": { "status": 404 } }
      },
      "expect": { "status": "success", "output": { "respond_missing.status": 404 } }
    }
  ]
}
```

- [ ] **Step 2: Run to verify they fail.** Expected: FAIL (unknown workflows).

- [ ] **Step 3: Write workflows**

`workflows/drops.get.json`:
```json
{
  "id": "drop-get",
  "name": "Drops: Get one",
  "nodes": {
    "get": {
      "type": "db.findOne",
      "services": { "database": "main-db" },
      "config": {
        "table": "drops",
        "select": ["id", "text", "file_name", "file_size", "file_mime", "created_at", "updated_at"],
        "where": { "id": "{{ input.id }}" },
        "required": true
      }
    },
    "respond": {
      "type": "response.json",
      "config": { "status": 200, "body": "{{ nodes.get }}" }
    },
    "respond_missing": {
      "type": "response.error",
      "config": { "status": 404, "code": "NOT_FOUND", "message": "drop not found" }
    }
  },
  "edges": [
    { "from": "get", "to": "respond" },
    { "from": "get", "output": "error", "to": "respond_missing" }
  ]
}
```

`workflows/drops.update.json`:
```json
{
  "id": "drop-update",
  "name": "Drops: Edit text",
  "nodes": {
    "update": {
      "type": "db.update",
      "services": { "database": "main-db" },
      "config": {
        "table": "drops",
        "data": { "text": "{{ input.text }}", "updated_at": "{{ now() }}" },
        "where": { "id": "{{ input.id }}" }
      }
    },
    "check": {
      "type": "control.if",
      "config": { "condition": "{{ nodes.update.rows_affected > 0 }}" }
    },
    "fetch": {
      "type": "db.findOne",
      "services": { "database": "main-db" },
      "config": {
        "table": "drops",
        "select": ["id", "text", "file_name", "file_size", "file_mime", "created_at", "updated_at"],
        "where": { "id": "{{ input.id }}" },
        "required": true
      }
    },
    "respond": {
      "type": "response.json",
      "config": { "status": 200, "body": "{{ nodes.fetch }}" }
    },
    "respond_missing": {
      "type": "response.error",
      "config": { "status": 404, "code": "NOT_FOUND", "message": "drop not found" }
    }
  },
  "edges": [
    { "from": "update", "to": "check" },
    { "from": "check", "output": "then", "to": "fetch" },
    { "from": "check", "output": "else", "to": "respond_missing" },
    { "from": "fetch", "to": "respond" }
  ]
}
```

`workflows/drops.delete.json` — DB row first (source of truth), then bytes; byte-delete failure logs and still 204s:
```json
{
  "id": "drop-delete",
  "name": "Drops: Delete",
  "nodes": {
    "get": {
      "type": "db.findOne",
      "services": { "database": "main-db" },
      "config": {
        "table": "drops",
        "select": ["id", "file_key"],
        "where": { "id": "{{ input.id }}" },
        "required": true
      }
    },
    "del_row": {
      "type": "db.delete",
      "services": { "database": "main-db" },
      "config": { "table": "drops", "where": { "id": "{{ input.id }}" } }
    },
    "has_file": {
      "type": "control.if",
      "config": { "condition": "{{ nodes.get.file_key != nil }}" }
    },
    "del_file": {
      "type": "storage.delete",
      "services": { "storage": "files" },
      "config": { "path": "{{ nodes.get.file_key }}" }
    },
    "log_orphan": {
      "type": "util.log",
      "config": {
        "level": "warn",
        "message": "orphaned file bytes after drop delete: {{ nodes.get.file_key }}"
      }
    },
    "respond_file_deleted": {
      "type": "response.json",
      "config": { "status": 204 }
    },
    "respond_orphan": {
      "type": "response.json",
      "config": { "status": 204 }
    },
    "respond_no_file": {
      "type": "response.json",
      "config": { "status": 204 }
    },
    "respond_missing": {
      "type": "response.error",
      "config": { "status": 404, "code": "NOT_FOUND", "message": "drop not found" }
    }
  },
  "edges": [
    { "from": "get", "to": "del_row" },
    { "from": "get", "output": "error", "to": "respond_missing" },
    { "from": "del_row", "to": "has_file" },
    { "from": "has_file", "output": "then", "to": "del_file" },
    { "from": "has_file", "output": "else", "to": "respond_no_file" },
    { "from": "del_file", "to": "respond_file_deleted" },
    { "from": "del_file", "output": "error", "to": "log_orphan" },
    { "from": "log_orphan", "to": "respond_orphan" }
  ]
}
```

- [ ] **Step 4: Write routes**

`routes/drops.get.json`:
```json
{
  "id": "drop-get",
  "method": "GET",
  "path": "/drops/:id",
  "summary": "Get one drop",
  "tags": ["drops"],
  "middleware": ["auth.session"],
  "trigger": {
    "workflow": "drop-get",
    "input": { "id": "{{ params.id }}" }
  }
}
```

`routes/drops.update.json`:
```json
{
  "id": "drop-update",
  "method": "PATCH",
  "path": "/drops/:id",
  "summary": "Replace a drop's text",
  "tags": ["drops"],
  "middleware": ["auth.session"],
  "body": {
    "schema": {
      "type": "object",
      "required": ["text"],
      "properties": {
        "text": { "type": "string", "minLength": 1 }
      }
    }
  },
  "trigger": {
    "workflow": "drop-update",
    "input": { "id": "{{ params.id }}", "text": "{{ body.text }}" }
  }
}
```

`routes/drops.delete.json`:
```json
{
  "id": "drop-delete",
  "method": "DELETE",
  "path": "/drops/:id",
  "summary": "Delete a drop (and its file bytes)",
  "tags": ["drops"],
  "middleware": ["auth.session"],
  "trigger": {
    "workflow": "drop-delete",
    "input": { "id": "{{ params.id }}" }
  }
}
```

- [ ] **Step 5: Run tests to verify they pass.** Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add projects/homebase
git commit -m "feat(homebase): drop get/edit/delete with byte cleanup

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 7: File drops — upload & download

**Files:**
- Create: `projects/homebase/tests/test-drop-upload.json`, `tests/test-drop-download.json`
- Create: `projects/homebase/workflows/drops.upload.json`, `workflows/drops.download.json`
- Create: `projects/homebase/routes/drops.upload.json`, `routes/drops.download.json`

**Interfaces:**
- Consumes: `files` storage service; drop row shape.
- Produces: `POST /drops/upload` (multipart field `file`, optional form field `text`) → `201 <drop row>`; `GET /drops/:id/file` → binary body with `Content-Type` + `Content-Disposition` / `404` (no file or no drop) / `500 STORAGE_ERROR` (bytes missing).

**Notes for the implementer:**
- `upload.handle` streams to storage BEFORE the insert (spec's write-ordering). On insert failure the just-written bytes are cleaned up.
- The filename is stored sanitized (CR, LF, `"` stripped) because `response.file` rejects those characters in `Content-Disposition` at download time.
- The escape dance in the `file_name` expression is JSON-level: `'\"'` is expr `'"'`, `"\\r"` is expr `"\r"`.
- `security.headers` (helmet) on the download route supplies `X-Content-Type-Options: nosniff`.

- [ ] **Step 1: Write the failing tests**

`tests/test-drop-upload.json`:
```json
{
  "id": "test-drop-upload",
  "workflow": "drop-upload",
  "tests": [
    {
      "name": "uploads a file with a comment",
      "input": { "text": "the report" },
      "mocks": {
        "upload": { "output": { "path": "drops/abc", "size": 1024, "content_type": "application/pdf", "filename": "report.pdf" } },
        "insert": { "output": { "id": "d1", "text": "the report", "file_name": "report.pdf", "file_size": 1024, "file_mime": "application/pdf" } },
        "respond": { "output": { "status": 201 } }
      },
      "expect": { "status": "success", "output": { "respond.status": 201 } }
    },
    {
      "name": "rejected upload gets 400",
      "input": { "text": "" },
      "mocks": {
        "upload": { "output_name": "error", "output": { "error": "file too large" } },
        "respond_invalid": { "output": { "status": 400 } }
      },
      "expect": { "status": "success", "output": { "respond_invalid.status": 400 } }
    },
    {
      "name": "insert failure cleans up the written bytes",
      "input": { "text": "" },
      "mocks": {
        "upload": { "output": { "path": "drops/abc", "size": 1024, "content_type": "application/pdf", "filename": "report.pdf" } },
        "insert": { "output_name": "error", "output": { "error": "db down" } },
        "cleanup": { "output": {} },
        "respond_insert_failed": { "output": { "status": 500 } }
      },
      "expect": { "status": "success", "output": { "respond_insert_failed.status": 500 } }
    }
  ]
}
```

`tests/test-drop-download.json`:
```json
{
  "id": "test-drop-download",
  "workflow": "drop-download",
  "tests": [
    {
      "name": "downloads file bytes",
      "input": { "id": "d1" },
      "mocks": {
        "get": { "output": { "id": "d1", "file_key": "drops/abc", "file_name": "report.pdf", "file_mime": "application/pdf" } },
        "read": { "output": { "data": "bytes", "size": 1024, "content_type": "application/pdf" } },
        "respond_file": { "output": { "status": 200 } }
      },
      "expect": { "status": "success", "output": { "respond_file.status": 200 } }
    },
    {
      "name": "text-only drop gets 404",
      "input": { "id": "d2" },
      "mocks": {
        "get": { "output": { "id": "d2", "file_key": null } },
        "respond_no_file": { "output": { "status": 404 } }
      },
      "expect": { "status": "success", "output": { "respond_no_file.status": 404 } }
    },
    {
      "name": "missing bytes get 500",
      "input": { "id": "d3" },
      "mocks": {
        "get": { "output": { "id": "d3", "file_key": "drops/gone", "file_name": "x", "file_mime": "text/plain" } },
        "read": { "output_name": "error", "output": { "error": "not found" } },
        "respond_bytes_missing": { "output": { "status": 500 } }
      },
      "expect": { "status": "success", "output": { "respond_bytes_missing.status": 500 } }
    },
    {
      "name": "unknown drop gets 404",
      "input": { "id": "nope" },
      "mocks": {
        "get": { "output_name": "error", "output": { "error": "not found" } },
        "respond_missing": { "output": { "status": 404 } }
      },
      "expect": { "status": "success", "output": { "respond_missing.status": 404 } }
    }
  ]
}
```

- [ ] **Step 2: Run to verify they fail.** Expected: FAIL (unknown workflows).

- [ ] **Step 3: Write workflows**

`workflows/drops.upload.json`:
```json
{
  "id": "drop-upload",
  "name": "Drops: Upload file",
  "nodes": {
    "upload": {
      "type": "upload.handle",
      "services": { "destination": "files" },
      "config": {
        "max_size": 1073741824,
        "allowed_types": ["*"],
        "path": "drops/{{ $uuid() }}",
        "field": "file"
      }
    },
    "insert": {
      "type": "db.create",
      "services": { "database": "main-db" },
      "config": {
        "table": "drops",
        "data": {
          "id": "{{ $uuid() }}",
          "text": "{{ input.text != '' ? input.text : nil }}",
          "file_name": "{{ replace(replace(replace(nodes.upload.filename, '\"', ''), \"\\r\", ''), \"\\n\", '') }}",
          "file_key": "{{ nodes.upload.path }}",
          "file_size": "{{ nodes.upload.size }}",
          "file_mime": "{{ nodes.upload.content_type }}"
        }
      }
    },
    "cleanup": {
      "type": "storage.delete",
      "services": { "storage": "files" },
      "config": { "path": "{{ nodes.upload.path }}" }
    },
    "respond": {
      "type": "response.json",
      "config": { "status": 201, "body": "{{ nodes.insert }}" }
    },
    "respond_invalid": {
      "type": "response.error",
      "config": { "status": 400, "code": "UPLOAD_INVALID", "message": "upload rejected" }
    },
    "respond_insert_failed": {
      "type": "response.error",
      "config": { "status": 500, "code": "CREATE_FAILED", "message": "could not store drop" }
    },
    "respond_cleanup_failed": {
      "type": "response.error",
      "config": { "status": 500, "code": "CREATE_FAILED", "message": "could not store drop" }
    }
  },
  "edges": [
    { "from": "upload", "to": "insert" },
    { "from": "upload", "output": "error", "to": "respond_invalid" },
    { "from": "insert", "to": "respond" },
    { "from": "insert", "output": "error", "to": "cleanup" },
    { "from": "cleanup", "to": "respond_insert_failed" },
    { "from": "cleanup", "output": "error", "to": "respond_cleanup_failed" }
  ]
}
```

`workflows/drops.download.json`:
```json
{
  "id": "drop-download",
  "name": "Drops: Download file",
  "nodes": {
    "get": {
      "type": "db.findOne",
      "services": { "database": "main-db" },
      "config": {
        "table": "drops",
        "select": ["id", "file_key", "file_name", "file_mime"],
        "where": { "id": "{{ input.id }}" },
        "required": true
      }
    },
    "has_file": {
      "type": "control.if",
      "config": { "condition": "{{ nodes.get.file_key != nil }}" }
    },
    "read": {
      "type": "storage.read",
      "services": { "storage": "files" },
      "config": { "path": "{{ nodes.get.file_key }}" }
    },
    "respond_file": {
      "type": "response.file",
      "config": {
        "status": 200,
        "data": "{{ nodes.read.data }}",
        "content_type": "{{ nodes.get.file_mime ?? 'application/octet-stream' }}",
        "filename": "{{ nodes.get.file_name ?? 'file' }}"
      }
    },
    "respond_no_file": {
      "type": "response.error",
      "config": { "status": 404, "code": "NOT_FOUND", "message": "drop has no file" }
    },
    "respond_bytes_missing": {
      "type": "response.error",
      "config": { "status": 500, "code": "STORAGE_ERROR", "message": "file bytes missing" }
    },
    "respond_missing": {
      "type": "response.error",
      "config": { "status": 404, "code": "NOT_FOUND", "message": "drop not found" }
    }
  },
  "edges": [
    { "from": "get", "to": "has_file" },
    { "from": "get", "output": "error", "to": "respond_missing" },
    { "from": "has_file", "output": "then", "to": "read" },
    { "from": "has_file", "output": "else", "to": "respond_no_file" },
    { "from": "read", "to": "respond_file" },
    { "from": "read", "output": "error", "to": "respond_bytes_missing" }
  ]
}
```

- [ ] **Step 4: Write routes**

`routes/drops.upload.json`:
```json
{
  "id": "drop-upload",
  "method": "POST",
  "path": "/drops/upload",
  "summary": "Upload a file drop (multipart; optional text comment)",
  "tags": ["drops"],
  "middleware": ["auth.session"],
  "trigger": {
    "workflow": "drop-upload",
    "files": ["file"],
    "input": { "text": "{{ (body ?? {}).text ?? '' }}" }
  }
}
```

`routes/drops.download.json`:
```json
{
  "id": "drop-download",
  "method": "GET",
  "path": "/drops/:id/file",
  "summary": "Download a drop's file",
  "tags": ["drops"],
  "middleware": ["auth.session", "security.headers"],
  "trigger": {
    "workflow": "drop-download",
    "input": { "id": "{{ params.id }}" }
  }
}
```

- [ ] **Step 5: Run tests to verify they pass.** Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add projects/homebase
git commit -m "feat(homebase): file drops — streaming upload and binary download

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 8: Share links — create, list, revoke (owner side)

**Files:**
- Create: `projects/homebase/tests/test-share-create.json`, `tests/test-shares-list.json`, `tests/test-share-revoke.json`
- Create: `projects/homebase/workflows/shares.create.json`, `workflows/shares.list.json`, `workflows/shares.revoke.json`
- Create: `projects/homebase/routes/shares.create.json`, `routes/shares.list.json`, `routes/shares.revoke.json`

**Interfaces:**
- Consumes: `share_links` + `drops` tables; `secrets.PUBLIC_BASE_URL`.
- Produces: `POST /drops/:id/share {expires_in?}` → `201 {id, token, url, expires_at}`; `GET /shares` → `200 {shares: [...]}`; `DELETE /shares/:id` → `204`/`404`. Token format: 64 hex chars (two dash-stripped UUIDv4s, ~244 bits) — Task 9 matches on it.

- [ ] **Step 1: Write the failing tests**

`tests/test-share-create.json`:
```json
{
  "id": "test-share-create",
  "workflow": "share-create",
  "tests": [
    {
      "name": "creates a permanent share link",
      "input": { "id": "d1", "expires_in": "" },
      "mocks": {
        "get": { "output": { "id": "d1" } },
        "insert": { "output": { "id": "sh1", "drop_id": "d1", "token": "tok", "expires_at": null } },
        "respond": { "output": { "status": 201 } }
      },
      "expect": { "status": "success", "output": { "respond.status": 201 } }
    },
    {
      "name": "creates an expiring share link",
      "input": { "id": "d1", "expires_in": "168h" },
      "mocks": {
        "get": { "output": { "id": "d1" } },
        "insert": { "output": { "id": "sh2", "drop_id": "d1", "token": "tok", "expires_at": "2026-07-14T10:00:00Z" } },
        "respond": { "output": { "status": 201 } }
      },
      "expect": { "status": "success", "output": { "respond.status": 201 } }
    },
    {
      "name": "unknown drop gets 404",
      "input": { "id": "nope", "expires_in": "" },
      "mocks": {
        "get": { "output_name": "error", "output": { "error": "not found" } },
        "respond_missing": { "output": { "status": 404 } }
      },
      "expect": { "status": "success", "output": { "respond_missing.status": 404 } }
    }
  ]
}
```

`tests/test-shares-list.json`:
```json
{
  "id": "test-shares-list",
  "workflow": "shares-list",
  "tests": [
    {
      "name": "lists active links with drop context",
      "input": {},
      "mocks": {
        "find": { "output": [ { "id": "sh1", "token": "tok", "drop_id": "d1", "file_name": null, "text": "hello", "created_at": "2026-07-07T10:00:00Z", "expires_at": null } ] },
        "respond": { "output": { "status": 200 } }
      },
      "expect": { "status": "success", "output": { "respond.status": 200 } }
    }
  ]
}
```

`tests/test-share-revoke.json`:
```json
{
  "id": "test-share-revoke",
  "workflow": "share-revoke",
  "tests": [
    {
      "name": "revokes a link",
      "input": { "share_id": "sh1" },
      "mocks": {
        "del": { "output": { "rows_affected": 1 } },
        "respond": { "output": { "status": 204 } }
      },
      "expect": { "status": "success", "output": { "respond.status": 204 } }
    },
    {
      "name": "unknown link gets 404",
      "input": { "share_id": "nope" },
      "mocks": {
        "del": { "output": { "rows_affected": 0 } },
        "respond_missing": { "output": { "status": 404 } }
      },
      "expect": { "status": "success", "output": { "respond_missing.status": 404 } }
    }
  ]
}
```

- [ ] **Step 2: Run to verify they fail.** Expected: FAIL (unknown workflows).

- [ ] **Step 3: Write workflows**

`workflows/shares.create.json`:
```json
{
  "id": "share-create",
  "name": "Shares: Create link",
  "nodes": {
    "get": {
      "type": "db.findOne",
      "services": { "database": "main-db" },
      "config": {
        "table": "drops",
        "select": ["id"],
        "where": { "id": "{{ input.id }}" },
        "required": true
      }
    },
    "gen": {
      "type": "transform.set",
      "config": {
        "fields": {
          "token": "{{ replace($uuid(), '-', '') + replace($uuid(), '-', '') }}"
        }
      }
    },
    "insert": {
      "type": "db.create",
      "services": { "database": "main-db" },
      "config": {
        "table": "share_links",
        "data": {
          "id": "{{ $uuid() }}",
          "drop_id": "{{ input.id }}",
          "token": "{{ nodes.gen.token }}",
          "expires_at": "{{ input.expires_in != '' ? now() + duration(input.expires_in) : nil }}"
        }
      }
    },
    "respond": {
      "type": "response.json",
      "config": {
        "status": 201,
        "body": {
          "id": "{{ nodes.insert.id }}",
          "token": "{{ nodes.gen.token }}",
          "url": "{{ secrets.PUBLIC_BASE_URL + '/s/' + nodes.gen.token }}",
          "expires_at": "{{ nodes.insert.expires_at }}"
        }
      }
    },
    "respond_missing": {
      "type": "response.error",
      "config": { "status": 404, "code": "NOT_FOUND", "message": "drop not found" }
    }
  },
  "edges": [
    { "from": "get", "to": "gen" },
    { "from": "get", "output": "error", "to": "respond_missing" },
    { "from": "gen", "to": "insert" },
    { "from": "insert", "to": "respond" }
  ]
}
```

`workflows/shares.list.json`:
```json
{
  "id": "shares-list",
  "name": "Shares: List active links",
  "nodes": {
    "find": {
      "type": "db.find",
      "services": { "database": "main-db" },
      "config": {
        "table": "share_links",
        "select": ["share_links.id", "share_links.token", "share_links.expires_at", "share_links.created_at", "share_links.drop_id", "drops.file_name", "drops.text"],
        "joins": [
          { "type": "INNER", "table": "drops", "on": "drops.id = share_links.drop_id" }
        ],
        "where_clause": {
          "query": "share_links.expires_at IS NULL OR share_links.expires_at > now()",
          "params": []
        },
        "order": "share_links.created_at DESC"
      }
    },
    "respond": {
      "type": "response.json",
      "config": { "status": 200, "body": { "shares": "{{ nodes.find }}" } }
    }
  },
  "edges": [
    { "from": "find", "to": "respond" }
  ]
}
```

`workflows/shares.revoke.json`:
```json
{
  "id": "share-revoke",
  "name": "Shares: Revoke link",
  "nodes": {
    "del": {
      "type": "db.delete",
      "services": { "database": "main-db" },
      "config": { "table": "share_links", "where": { "id": "{{ input.share_id }}" } }
    },
    "check": {
      "type": "control.if",
      "config": { "condition": "{{ nodes.del.rows_affected > 0 }}" }
    },
    "respond": {
      "type": "response.json",
      "config": { "status": 204 }
    },
    "respond_missing": {
      "type": "response.error",
      "config": { "status": 404, "code": "NOT_FOUND", "message": "share not found" }
    }
  },
  "edges": [
    { "from": "del", "to": "check" },
    { "from": "check", "output": "then", "to": "respond" },
    { "from": "check", "output": "else", "to": "respond_missing" }
  ]
}
```

- [ ] **Step 4: Write routes**

`routes/shares.create.json`:
```json
{
  "id": "share-create",
  "method": "POST",
  "path": "/drops/:id/share",
  "summary": "Create a share link for a drop",
  "tags": ["shares"],
  "middleware": ["auth.session"],
  "body": {
    "schema": {
      "type": "object",
      "properties": {
        "expires_in": { "type": "string", "pattern": "^[0-9]+(s|m|h)$" }
      }
    }
  },
  "trigger": {
    "workflow": "share-create",
    "input": {
      "id": "{{ params.id }}",
      "expires_in": "{{ (body ?? {}).expires_in ?? '' }}"
    }
  }
}
```

`routes/shares.list.json`:
```json
{
  "id": "shares-list",
  "method": "GET",
  "path": "/shares",
  "summary": "List active share links",
  "tags": ["shares"],
  "middleware": ["auth.session"],
  "trigger": {
    "workflow": "shares-list",
    "input": {}
  }
}
```

`routes/shares.revoke.json`:
```json
{
  "id": "share-revoke",
  "method": "DELETE",
  "path": "/shares/:id",
  "summary": "Revoke a share link",
  "tags": ["shares"],
  "middleware": ["auth.session"],
  "trigger": {
    "workflow": "share-revoke",
    "input": { "share_id": "{{ params.id }}" }
  }
}
```

- [ ] **Step 5: Run tests to verify they pass.** Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add projects/homebase
git commit -m "feat(homebase): share links — create with optional expiry, list, revoke

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 9: Public share access — `/s/:token` and `/s/:token/file`

**Files:**
- Create: `projects/homebase/tests/test-share-get.json`, `tests/test-share-download.json`
- Create: `projects/homebase/workflows/shares.get.json`, `workflows/shares.download.json`
- Create: `projects/homebase/routes/shares.get.json`, `routes/shares.download.json`

**Interfaces:**
- Consumes: token format from Task 8; `files` storage.
- Produces (NO auth): `GET /s/:token` → `200 {text, file_name, file_size, file_mime, created_at, has_file}`; `GET /s/:token/file` → binary. EVERY failure (unknown/expired/revoked token, dangling drop, no file, missing bytes) is the identical `404 NOT_FOUND "not found"` — the uniform-404 rule.

- [ ] **Step 1: Write the failing tests**

`tests/test-share-get.json`:
```json
{
  "id": "test-share-get",
  "workflow": "share-get",
  "tests": [
    {
      "name": "valid token returns the drop",
      "input": { "token": "tok" },
      "mocks": {
        "find": { "output": { "id": "sh1", "drop_id": "d1" } },
        "get_drop": { "output": { "text": "hello", "file_name": null, "file_size": null, "file_mime": null, "created_at": "2026-07-07T10:00:00Z" } },
        "respond": { "output": { "status": 200 } }
      },
      "expect": { "status": "success", "output": { "respond.status": 200 } }
    },
    {
      "name": "unknown or expired token gets uniform 404",
      "input": { "token": "nope" },
      "mocks": {
        "find": { "output": null },
        "respond_no_link": { "output": { "status": 404 } }
      },
      "expect": { "status": "success", "output": { "respond_no_link.status": 404 } }
    }
  ]
}
```

`tests/test-share-download.json`:
```json
{
  "id": "test-share-download",
  "workflow": "share-download",
  "tests": [
    {
      "name": "valid token downloads the file",
      "input": { "token": "tok" },
      "mocks": {
        "find": { "output": { "id": "sh1", "drop_id": "d1" } },
        "get_drop": { "output": { "file_key": "drops/abc", "file_name": "report.pdf", "file_mime": "application/pdf" } },
        "read": { "output": { "data": "bytes", "size": 5, "content_type": "application/pdf" } },
        "respond_file": { "output": { "status": 200 } }
      },
      "expect": { "status": "success", "output": { "respond_file.status": 200 } }
    },
    {
      "name": "unknown token gets uniform 404",
      "input": { "token": "nope" },
      "mocks": {
        "find": { "output": null },
        "respond_no_link": { "output": { "status": 404 } }
      },
      "expect": { "status": "success", "output": { "respond_no_link.status": 404 } }
    },
    {
      "name": "text-only drop behind a link gets uniform 404",
      "input": { "token": "tok" },
      "mocks": {
        "find": { "output": { "id": "sh1", "drop_id": "d1" } },
        "get_drop": { "output": { "file_key": null } },
        "respond_no_file": { "output": { "status": 404 } }
      },
      "expect": { "status": "success", "output": { "respond_no_file.status": 404 } }
    },
    {
      "name": "missing bytes get uniform 404",
      "input": { "token": "tok" },
      "mocks": {
        "find": { "output": { "id": "sh1", "drop_id": "d1" } },
        "get_drop": { "output": { "file_key": "drops/gone", "file_name": "x", "file_mime": "text/plain" } },
        "read": { "output_name": "error", "output": { "error": "not found" } },
        "respond_no_bytes": { "output": { "status": 404 } }
      },
      "expect": { "status": "success", "output": { "respond_no_bytes.status": 404 } }
    }
  ]
}
```

- [ ] **Step 2: Run to verify they fail.** Expected: FAIL (unknown workflows).

- [ ] **Step 3: Write workflows**

`workflows/shares.get.json`:
```json
{
  "id": "share-get",
  "name": "Public: View shared drop",
  "nodes": {
    "find": {
      "type": "db.findOne",
      "services": { "database": "main-db" },
      "config": {
        "table": "share_links",
        "select": ["id", "drop_id"],
        "where_clause": {
          "query": "token = ? AND (expires_at IS NULL OR expires_at > now())",
          "params": ["{{ input.token }}"]
        },
        "required": false
      }
    },
    "check": {
      "type": "control.if",
      "config": { "condition": "{{ nodes.find != nil }}" }
    },
    "get_drop": {
      "type": "db.findOne",
      "services": { "database": "main-db" },
      "config": {
        "table": "drops",
        "select": ["text", "file_name", "file_size", "file_mime", "created_at"],
        "where": { "id": "{{ nodes.find.drop_id }}" },
        "required": true
      }
    },
    "respond": {
      "type": "response.json",
      "config": {
        "status": 200,
        "body": {
          "text": "{{ nodes.get_drop.text }}",
          "file_name": "{{ nodes.get_drop.file_name }}",
          "file_size": "{{ nodes.get_drop.file_size }}",
          "file_mime": "{{ nodes.get_drop.file_mime }}",
          "created_at": "{{ nodes.get_drop.created_at }}",
          "has_file": "{{ nodes.get_drop.file_name != nil }}"
        }
      }
    },
    "respond_no_link": {
      "type": "response.error",
      "config": { "status": 404, "code": "NOT_FOUND", "message": "not found" }
    },
    "respond_gone": {
      "type": "response.error",
      "config": { "status": 404, "code": "NOT_FOUND", "message": "not found" }
    }
  },
  "edges": [
    { "from": "find", "to": "check" },
    { "from": "check", "output": "then", "to": "get_drop" },
    { "from": "check", "output": "else", "to": "respond_no_link" },
    { "from": "get_drop", "to": "respond" },
    { "from": "get_drop", "output": "error", "to": "respond_gone" }
  ]
}
```

`workflows/shares.download.json`:
```json
{
  "id": "share-download",
  "name": "Public: Download shared file",
  "nodes": {
    "find": {
      "type": "db.findOne",
      "services": { "database": "main-db" },
      "config": {
        "table": "share_links",
        "select": ["id", "drop_id"],
        "where_clause": {
          "query": "token = ? AND (expires_at IS NULL OR expires_at > now())",
          "params": ["{{ input.token }}"]
        },
        "required": false
      }
    },
    "check": {
      "type": "control.if",
      "config": { "condition": "{{ nodes.find != nil }}" }
    },
    "get_drop": {
      "type": "db.findOne",
      "services": { "database": "main-db" },
      "config": {
        "table": "drops",
        "select": ["file_key", "file_name", "file_mime"],
        "where": { "id": "{{ nodes.find.drop_id }}" },
        "required": true
      }
    },
    "has_file": {
      "type": "control.if",
      "config": { "condition": "{{ nodes.get_drop.file_key != nil }}" }
    },
    "read": {
      "type": "storage.read",
      "services": { "storage": "files" },
      "config": { "path": "{{ nodes.get_drop.file_key }}" }
    },
    "respond_file": {
      "type": "response.file",
      "config": {
        "status": 200,
        "data": "{{ nodes.read.data }}",
        "content_type": "{{ nodes.get_drop.file_mime ?? 'application/octet-stream' }}",
        "filename": "{{ nodes.get_drop.file_name ?? 'file' }}"
      }
    },
    "respond_no_link": {
      "type": "response.error",
      "config": { "status": 404, "code": "NOT_FOUND", "message": "not found" }
    },
    "respond_gone": {
      "type": "response.error",
      "config": { "status": 404, "code": "NOT_FOUND", "message": "not found" }
    },
    "respond_no_file": {
      "type": "response.error",
      "config": { "status": 404, "code": "NOT_FOUND", "message": "not found" }
    },
    "respond_no_bytes": {
      "type": "response.error",
      "config": { "status": 404, "code": "NOT_FOUND", "message": "not found" }
    }
  },
  "edges": [
    { "from": "find", "to": "check" },
    { "from": "check", "output": "then", "to": "get_drop" },
    { "from": "check", "output": "else", "to": "respond_no_link" },
    { "from": "get_drop", "to": "has_file" },
    { "from": "get_drop", "output": "error", "to": "respond_gone" },
    { "from": "has_file", "output": "then", "to": "read" },
    { "from": "has_file", "output": "else", "to": "respond_no_file" },
    { "from": "read", "to": "respond_file" },
    { "from": "read", "output": "error", "to": "respond_no_bytes" }
  ]
}
```

- [ ] **Step 4: Write routes**

`routes/shares.get.json`:
```json
{
  "id": "share-get",
  "method": "GET",
  "path": "/s/:token",
  "summary": "View a shared drop (no auth)",
  "tags": ["public"],
  "middleware": ["limiter", "security.headers"],
  "trigger": {
    "workflow": "share-get",
    "input": { "token": "{{ params.token }}" }
  }
}
```

`routes/shares.download.json`:
```json
{
  "id": "share-download",
  "method": "GET",
  "path": "/s/:token/file",
  "summary": "Download a shared file (no auth)",
  "tags": ["public"],
  "middleware": ["limiter", "security.headers"],
  "trigger": {
    "workflow": "share-download",
    "input": { "token": "{{ params.token }}" }
  }
}
```

- [ ] **Step 5: Run tests to verify they pass.** Expected: PASS (all suites so far).

- [ ] **Step 6: Commit**

```bash
git add projects/homebase
git commit -m "feat(homebase): public share endpoints with uniform 404s

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 10: E2E suite against the compose stack + README

**Files:**
- Create: `projects/homebase/e2e/e2e_test.go`
- Create: `projects/homebase/e2e/run.sh` (chmod +x)
- Modify: `projects/homebase/README.md` (replace the stub)

**Interfaces:**
- Consumes: every endpoint contract from Tasks 2–9, the compose stack from Task 1.
- Produces: the acceptance gate — `./projects/homebase/e2e/run.sh` exits 0.

**Notes for the implementer:**
- The test is plain stdlib (repo module already covers `projects/`), behind the `e2e` build tag so `go test ./...` never runs it.
- "Bytes verifiably gone" after delete is asserted via the API (download → 404 because the row is gone); the volume itself isn't inspected.
- Rate limiter is 30 req/min per IP on limited routes; the E2E makes fewer than 30 requests against them, so no 429s. Don't add a limiter-tripping test — it would flake the suite that follows it.

- [ ] **Step 1: Write `e2e/e2e_test.go`**

```go
//go:build e2e

// Package e2e drives the full Homebase lifecycle against a running
// docker-compose stack (see run.sh). It is the acceptance gate for the
// foundation spec: docs/superpowers/specs/2026-07-07-homebase-foundation-design.md
package e2e

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

var (
	baseURL    = envOr("BASE_URL", "http://localhost:3000")
	setupToken = envOr("SETUP_TOKEN", "e2e-setup-token")
)

const (
	adminEmail    = "admin@example.com"
	adminPassword = "correct horse battery staple"
)

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// client is a tiny API client bound to one session token ("one machine").
type client struct {
	t     *testing.T
	token string
}

func (c *client) do(method, path string, body io.Reader, contentType string) *http.Response {
	c.t.Helper()
	req, err := http.NewRequest(method, baseURL+path, body)
	if err != nil {
		c.t.Fatalf("%s %s: %v", method, path, err)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		c.t.Fatalf("%s %s: %v", method, path, err)
	}
	return resp
}

func (c *client) doJSON(method, path string, payload any) *http.Response {
	c.t.Helper()
	var body io.Reader
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			c.t.Fatalf("marshal: %v", err)
		}
		body = bytes.NewReader(b)
	}
	return c.do(method, path, body, "application/json")
}

func decode(t *testing.T, resp *http.Response) map[string]any {
	t.Helper()
	defer resp.Body.Close()
	var m map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&m); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	return m
}

func wantStatus(t *testing.T, resp *http.Response, want int) {
	t.Helper()
	if resp.StatusCode != want {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("status = %d, want %d; body: %s", resp.StatusCode, want, b)
	}
}

func drainAndClose(resp *http.Response) {
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
}

func login(t *testing.T) *client {
	t.Helper()
	anon := &client{t: t}
	resp := anon.doJSON("POST", "/auth/login", map[string]string{
		"email": adminEmail, "password": adminPassword,
	})
	wantStatus(t, resp, 200)
	body := decode(t, resp)
	token, _ := body["token"].(string)
	if token == "" {
		t.Fatal("login returned no token")
	}
	return &client{t: t, token: token}
}

// TestHomebaseLifecycle is one ordered walk through the whole API. Subtests
// share state (the drops/shares created earlier) and must run in order.
func TestHomebaseLifecycle(t *testing.T) {
	anon := &client{t: t}

	// --- setup ---
	t.Run("setup with wrong token is 403", func(t *testing.T) {
		resp := anon.doJSON("POST", "/setup", map[string]string{
			"setup_token": "definitely-wrong", "email": adminEmail, "password": adminPassword,
		})
		wantStatus(t, resp, 403)
		drainAndClose(resp)
	})

	t.Run("setup creates the admin", func(t *testing.T) {
		resp := anon.doJSON("POST", "/setup", map[string]string{
			"setup_token": setupToken, "email": adminEmail, "password": adminPassword,
		})
		wantStatus(t, resp, 201)
		drainAndClose(resp)
	})

	t.Run("second setup is 403 even with the right token", func(t *testing.T) {
		resp := anon.doJSON("POST", "/setup", map[string]string{
			"setup_token": setupToken, "email": "evil@example.com", "password": "hunter2hunter2",
		})
		wantStatus(t, resp, 403)
		drainAndClose(resp)
	})

	// --- auth ---
	t.Run("unauthenticated request is 401", func(t *testing.T) {
		resp := anon.do("GET", "/drops", nil, "")
		wantStatus(t, resp, 401)
		drainAndClose(resp)
	})

	machineA := login(t) // "laptop"
	machineB := login(t) // "desktop"

	t.Run("me returns the admin", func(t *testing.T) {
		resp := machineA.do("GET", "/auth/me", nil, "")
		wantStatus(t, resp, 200)
		body := decode(t, resp)
		if body["email"] != adminEmail {
			t.Fatalf("me.email = %v", body["email"])
		}
	})

	// --- text drops & todos ---
	var todoDropID string
	t.Run("create a todo text drop", func(t *testing.T) {
		resp := machineA.doJSON("POST", "/drops", map[string]string{"text": "- [ ] buy milk"})
		wantStatus(t, resp, 201)
		body := decode(t, resp)
		todoDropID, _ = body["id"].(string)
		if todoDropID == "" {
			t.Fatal("no drop id")
		}
	})

	t.Run("the other machine sees the drop", func(t *testing.T) {
		resp := machineB.do("GET", "/drops", nil, "")
		wantStatus(t, resp, 200)
		body := decode(t, resp)
		drops, _ := body["drops"].([]any)
		if len(drops) != 1 {
			t.Fatalf("drops = %d, want 1", len(drops))
		}
	})

	t.Run("tick the checkbox via PATCH", func(t *testing.T) {
		resp := machineB.doJSON("PATCH", "/drops/"+todoDropID, map[string]string{"text": "- [x] buy milk"})
		wantStatus(t, resp, 200)
		body := decode(t, resp)
		if body["text"] != "- [x] buy milk" {
			t.Fatalf("text = %v", body["text"])
		}
	})

	t.Run("search finds by content", func(t *testing.T) {
		resp := machineA.do("GET", "/drops?q=milk", nil, "")
		wantStatus(t, resp, 200)
		body := decode(t, resp)
		if drops, _ := body["drops"].([]any); len(drops) != 1 {
			t.Fatalf("q=milk found %d drops, want 1", len(drops))
		}
		resp = machineA.do("GET", "/drops?q=zzz-not-there", nil, "")
		wantStatus(t, resp, 200)
		body = decode(t, resp)
		if drops, _ := body["drops"].([]any); len(drops) != 0 {
			t.Fatalf("q=zzz found %d drops, want 0", len(drops))
		}
	})

	// --- file drops ---
	fileContent := []byte("e2e file payload " + time.Now().Format(time.RFC3339Nano))
	var fileDropID string
	t.Run("upload a file drop with a comment", func(t *testing.T) {
		var buf bytes.Buffer
		w := multipart.NewWriter(&buf)
		if err := w.WriteField("text", "the report"); err != nil {
			t.Fatal(err)
		}
		fw, err := w.CreateFormFile("file", "report.txt")
		if err != nil {
			t.Fatal(err)
		}
		fw.Write(fileContent)
		w.Close()

		resp := machineA.do("POST", "/drops/upload", &buf, w.FormDataContentType())
		wantStatus(t, resp, 201)
		body := decode(t, resp)
		fileDropID, _ = body["id"].(string)
		if fileDropID == "" {
			t.Fatal("no drop id")
		}
		if body["file_name"] != "report.txt" {
			t.Fatalf("file_name = %v", body["file_name"])
		}
	})

	t.Run("download the file from the other machine", func(t *testing.T) {
		resp := machineB.do("GET", "/drops/"+fileDropID+"/file", nil, "")
		wantStatus(t, resp, 200)
		defer resp.Body.Close()
		got, _ := io.ReadAll(resp.Body)
		if !bytes.Equal(got, fileContent) {
			t.Fatalf("downloaded bytes differ: got %q", got)
		}
		if cd := resp.Header.Get("Content-Disposition"); !strings.Contains(cd, "report.txt") {
			t.Fatalf("Content-Disposition = %q", cd)
		}
	})

	// --- sharing ---
	var shareURL, shareID string
	t.Run("create a share link", func(t *testing.T) {
		resp := machineA.doJSON("POST", "/drops/"+fileDropID+"/share", nil)
		wantStatus(t, resp, 201)
		body := decode(t, resp)
		token, _ := body["token"].(string)
		shareID, _ = body["id"].(string)
		if token == "" || shareID == "" {
			t.Fatalf("share missing token/id: %v", body)
		}
		shareURL = "/s/" + token
	})

	t.Run("friend fetches the share unauthenticated", func(t *testing.T) {
		resp := anon.do("GET", shareURL, nil, "")
		wantStatus(t, resp, 200)
		body := decode(t, resp)
		if body["file_name"] != "report.txt" || body["has_file"] != true {
			t.Fatalf("share body = %v", body)
		}
		resp = anon.do("GET", shareURL+"/file", nil, "")
		wantStatus(t, resp, 200)
		defer resp.Body.Close()
		got, _ := io.ReadAll(resp.Body)
		if !bytes.Equal(got, fileContent) {
			t.Fatal("shared download differs")
		}
	})

	t.Run("expiring link dies", func(t *testing.T) {
		resp := machineA.doJSON("POST", "/drops/"+todoDropID+"/share", map[string]string{"expires_in": "1s"})
		wantStatus(t, resp, 201)
		body := decode(t, resp)
		tok, _ := body["token"].(string)
		time.Sleep(1500 * time.Millisecond)
		resp = anon.do("GET", "/s/"+tok, nil, "")
		wantStatus(t, resp, 404)
		drainAndClose(resp)
	})

	t.Run("revoked link dies", func(t *testing.T) {
		resp := machineA.do("DELETE", "/shares/"+shareID, nil, "")
		wantStatus(t, resp, 204)
		drainAndClose(resp)
		resp = anon.do("GET", shareURL, nil, "")
		wantStatus(t, resp, 404)
		drainAndClose(resp)
	})

	t.Run("unknown share token is the same 404", func(t *testing.T) {
		resp := anon.do("GET", "/s/"+strings.Repeat("f", 64), nil, "")
		wantStatus(t, resp, 404)
		drainAndClose(resp)
	})

	// --- per-device session revocation ---
	t.Run("revoke machine B's session from machine A", func(t *testing.T) {
		resp := machineB.do("GET", "/auth/sessions", nil, "")
		wantStatus(t, resp, 200)
		body := decode(t, resp)
		bSession, _ := body["current_session_id"].(string)
		if bSession == "" {
			t.Fatal("no current_session_id")
		}
		sessions, _ := body["sessions"].([]any)
		if len(sessions) < 2 {
			t.Fatalf("sessions = %d, want >= 2", len(sessions))
		}

		resp = machineA.do("DELETE", "/auth/sessions/"+bSession, nil, "")
		wantStatus(t, resp, 204)
		drainAndClose(resp)

		resp = machineB.do("GET", "/drops", nil, "")
		wantStatus(t, resp, 401)
		drainAndClose(resp)
	})

	// --- deletion ---
	t.Run("delete the file drop; share links and file access die with it", func(t *testing.T) {
		// a fresh share link that must die with the drop (cascade)
		resp := machineA.doJSON("POST", "/drops/"+fileDropID+"/share", nil)
		wantStatus(t, resp, 201)
		body := decode(t, resp)
		tok, _ := body["token"].(string)

		resp = machineA.do("DELETE", "/drops/"+fileDropID, nil, "")
		wantStatus(t, resp, 204)
		drainAndClose(resp)

		resp = machineA.do("GET", "/drops/"+fileDropID, nil, "")
		wantStatus(t, resp, 404)
		drainAndClose(resp)
		resp = machineA.do("GET", "/drops/"+fileDropID+"/file", nil, "")
		wantStatus(t, resp, 404)
		drainAndClose(resp)
		resp = anon.do("GET", "/s/"+tok, nil, "")
		wantStatus(t, resp, 404)
		drainAndClose(resp)
	})

	// --- logout ---
	t.Run("logout kills machine A's session", func(t *testing.T) {
		resp := machineA.do("POST", "/auth/logout", nil, "")
		wantStatus(t, resp, 204)
		drainAndClose(resp)
		resp = machineA.do("GET", "/drops", nil, "")
		wantStatus(t, resp, 401)
		drainAndClose(resp)
	})
}

func TestMain(m *testing.M) {
	// Wait for the stack (run.sh also waits; this is belt & braces).
	deadline := time.Now().Add(60 * time.Second)
	for {
		resp, err := http.Get(baseURL + "/health/ready")
		if err == nil {
			drainAndClose(resp)
			if resp.StatusCode == 200 {
				break
			}
		}
		if time.Now().After(deadline) {
			fmt.Fprintln(os.Stderr, "homebase stack not ready at", baseURL)
			os.Exit(1)
		}
		time.Sleep(time.Second)
	}
	os.Exit(m.Run())
}
```

- [ ] **Step 2: Write `e2e/run.sh`**

```bash
#!/usr/bin/env bash
# Homebase E2E: boots the compose stack from scratch, runs the Go suite, tears down.
# Usage: ./projects/homebase/e2e/run.sh   (from anywhere)
set -euo pipefail
cd "$(dirname "$0")/.."

export SETUP_TOKEN="${SETUP_TOKEN:-e2e-setup-token}"
export PUBLIC_BASE_URL="${PUBLIC_BASE_URL:-http://localhost:3000}"

docker compose down -v --remove-orphans 2>/dev/null || true
docker compose up -d --build
trap 'docker compose down -v --remove-orphans' EXIT

echo "waiting for noda ..."
for _ in $(seq 1 60); do
  if curl -fso /dev/null http://localhost:3000/health/ready; then
    break
  fi
  sleep 1
done

cd ../..
SETUP_TOKEN="$SETUP_TOKEN" go test -tags e2e -count=1 -v ./projects/homebase/e2e/
```

Then: `chmod +x projects/homebase/e2e/run.sh`

- [ ] **Step 3: Run the suite**

```bash
./projects/homebase/e2e/run.sh
```
Expected: all subtests PASS, stack torn down. If a subtest fails, fix the config (not the test) unless the test contradicts the spec.

- [ ] **Step 4: Replace `README.md`**

```markdown
# Homebase

Personal private-cloud API built on [Noda](../../README.md) — config only, no
application code. Replaces a paste-everything Discord channel: one
chronological **drops** feed (markdown text and/or files), simple search,
revocable share links for friends, per-device sessions.

Spec: `docs/superpowers/specs/2026-07-07-homebase-foundation-design.md`

## Deploy

```bash
cp .env.example .env       # set SETUP_TOKEN, PUBLIC_BASE_URL (and DOMAIN for the edge)
docker compose up -d --build            # api on :3000 (migrations run automatically)
docker compose --profile edge up -d     # additionally: Caddy with TLS on :443
```

One-time bootstrap (creates the only account):

```bash
curl -X POST "$PUBLIC_BASE_URL/setup" -H 'Content-Type: application/json' \
  -d '{"setup_token":"<SETUP_TOKEN>","email":"you@example.com","password":"..."}'
```

## Use

```bash
# log a machine in (store the token on that machine)
TOKEN=$(curl -s -X POST "$BASE/auth/login" -H 'Content-Type: application/json' \
  -d '{"email":"you@example.com","password":"..."}' | jq -r .token)
AUTH="Authorization: Bearer $TOKEN"

curl -H "$AUTH" -X POST "$BASE/drops" -d '{"text":"- [ ] buy milk"}' -H 'Content-Type: application/json'
curl -H "$AUTH" -F file=@report.pdf -F text="the report" "$BASE/drops/upload"
curl -H "$AUTH" "$BASE/drops?q=milk"                 # search
curl -H "$AUTH" -X PATCH "$BASE/drops/<id>" -d '{"text":"- [x] buy milk"}' -H 'Content-Type: application/json'
curl -H "$AUTH" -OJ "$BASE/drops/<id>/file"          # download
curl -H "$AUTH" -X POST "$BASE/drops/<id>/share" -d '{"expires_in":"168h"}' -H 'Content-Type: application/json'
# → {"url": ".../s/<token>", ...}; friends need nothing but that URL
curl -H "$AUTH" "$BASE/auth/sessions"                # devices
curl -H "$AUTH" -X DELETE "$BASE/auth/sessions/<id>" # lost-laptop kill switch
```

| Endpoint | Auth | Purpose |
|---|---|---|
| `POST /setup` | setup token | one-time admin bootstrap |
| `POST /auth/login`, `POST /auth/logout`, `GET /auth/me` | — / session | sessions |
| `GET /auth/sessions`, `DELETE /auth/sessions/:id` | session | device list / revoke |
| `POST /drops`, `POST /drops/upload` | session | text drop / file drop |
| `GET /drops?q=&before=`, `GET /drops/:id`, `GET /drops/:id/file` | session | list+search / detail / download |
| `PATCH /drops/:id`, `DELETE /drops/:id` | session | edit text / delete |
| `POST /drops/:id/share`, `GET /shares`, `DELETE /shares/:id` | session | share links |
| `GET /s/:token`, `GET /s/:token/file` | none | friend access |

## Tests

```bash
# workflow tests (no containers)
DATABASE_URL='postgres://x' FILES_PATH=/tmp/hb SETUP_TOKEN=test-setup-token \
  PUBLIC_BASE_URL=http://localhost:3000 go run ./cmd/noda test --config projects/homebase

# full E2E against the compose stack
./projects/homebase/e2e/run.sh
```
```

(Adjust the workflow-test env line to the full `DATABASE_URL` used elsewhere if `postgres://x` fails config validation — the URL is only parsed, not dialed.)

- [ ] **Step 5: Re-run workflow tests + validate one last time** (standard commands). Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add projects/homebase
git commit -m "test(homebase): full-lifecycle E2E suite against compose stack + README

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 11: Final verification & branch finish

**Files:** none new.

- [ ] **Step 1: Full verification from a clean state**

```bash
DATABASE_URL='postgres://noda:noda@localhost:5432/noda?sslmode=disable' FILES_PATH=/tmp/homebase-files SETUP_TOKEN=test-setup-token PUBLIC_BASE_URL=http://localhost:3000 go run ./cmd/noda validate --config projects/homebase
DATABASE_URL='postgres://noda:noda@localhost:5432/noda?sslmode=disable' FILES_PATH=/tmp/homebase-files SETUP_TOKEN=test-setup-token PUBLIC_BASE_URL=http://localhost:3000 go run ./cmd/noda test --config projects/homebase
./projects/homebase/e2e/run.sh
go build ./... && go vet ./...
```
Expected: all green. `go build/vet` guard the e2e file's build tag hygiene.

- [ ] **Step 2: Use the superpowers:finishing-a-development-branch skill** to decide merge/PR (expected outcome: PR from `homebase-foundation` to `main`, `--auto` merge after the 4 functional CI checks, matching repo convention).

---

## Self-Review (done at plan-writing time)

- **Spec coverage:** every spec endpoint has a task (setup T2; login/logout/me T3; sessions T4; drops create/list/search T5; get/patch/delete T6; upload/download T7; share owner-side T8; public T9). Error-handling rules (uniform 404, same-403, write ordering, orphan logging) are implemented in T2/T6/T7/T9. Security items: rate limiter (T1 config + route middleware), nosniff via `security.headers` (T7/T9 routes), server-side `file_key` (T7 `drops/{{ $uuid() }}`), sanitized `Content-Disposition` (T7 insert + `response.file`'s built-in rejection), body limit (T1). Testing layers both present (per-task workflow tests; T10 E2E gate).
- **Deviations from spec (deliberate, minor):** no separate `schemas/` directory — schemas are inlined in route files, following `examples/auth-demo`; `GET /drops` limit is fixed at 50 (the `limit` config field is not expression-capable; a client-tunable limit is YAGNI).
- **Type consistency:** node ids referenced in tests match workflow definitions; `input.*` names match route `trigger.input` keys; `current_session_id`/`sessions` (T4) match the E2E's usage; share token vocabulary (`token`, `url`, `id`) consistent T8→T9→E2E.
- **Placeholder scan:** clean — every step carries full file content or an exact command with expected outcome.
