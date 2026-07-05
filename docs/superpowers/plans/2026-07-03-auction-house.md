# Auction House (Bidhub) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build `projects/auction-house/` — a self-contained live auction API on Noda that exercises most subsystems and logs every rough edge to `FINDINGS.md`.

**Architecture:** Pure Noda config project (no Go application code except a Wasm module and test tooling). Postgres is the source of truth; bid acceptance is a compare-and-set `db.exec` inside a `workflow.run` transaction; Redis Streams + workers deliver emails; a 10-second cron closes auctions; WS + SSE broadcast prices; a Wasm module computes proxy bids; a mock PSP (a route in the same app) exercises outbound HTTP + signed webhooks.

**Tech Stack:** Noda (this repo's CLI), Postgres 16, Redis 7, Mailpit, tinygo (Wasm), bash/curl/jq + a small Go smoke tool for WS/SSE.

**Spec:** `docs/superpowers/specs/2026-07-03-auction-house-design.md`

## Global Constraints

- **Honest build:** consult only `docs/` (NOT `docs/_internal/`), the `noda` CLI, MCP tools (`noda_get_node_schema`, `noda_validate_config`, …), and `examples/`. Never read `internal/` or `plugins/` source to figure out behavior. If you must peek to get unstuck, log a FINDINGS entry *first* — the need to peek is itself a finding.
- **Friction log:** every rough edge goes in `projects/auction-house/FINDINGS.md` as it happens: `[F-##] severity (bug|doc|dx|gap) — expected vs. actual — where`. Severity: blocker / major / minor / paper-cut. Never silently work around something.
- **Plan JSON is a grounded draft, not gospel.** Every config block below was checked against MCP schemas/examples on 2026-07-03, but if `noda validate`, a node doc (`docs/03-nodes/<type>.md`), or runtime behavior disagrees, follow reality — and if the plan was *reasonably* wrong (schema ambiguous, doc missing), that's a FINDINGS entry.
- **Working dir:** all `noda`/compose commands run from `projects/auction-house/`. Install the CLI once: `go install ./cmd/noda` from repo root (assumes `$GOPATH/bin` on PATH; verify with `noda --help`).
- **Branch:** all work on `feat/auction-house` (create from `main` at Task 1; worktree optional per superpowers:using-git-worktrees).
- **Gates:** `noda validate --verbose` must pass before every commit; `noda test` must pass from Task 3 on.
- **Env (`.env`):** `DATABASE_URL=postgres://noda:noda@localhost:5432/auction?sslmode=disable`, `REDIS_URL=redis://localhost:6379/0`, `JWT_SECRET=dev-secret-change-me`, `SMTP_HOST=localhost`, `SMTP_PORT=1025`, `SMTP_FROM=auctions@bidhub.local`, `PSP_SECRET=psp-dev-secret`, `BASE_URL=http://localhost:8080`.
- **IDs/names used throughout (interface contract):** services `main-db, app-cache, main-stream, realtime, files, mailer, psp-client`; stream topics `bid.placed, auction.closed, payment.settled`; DLQ topic `notify-dlq`; WS endpoint `auction-ws` (channel `auction.<listing_id>`); SSE endpoint `ticker` (channel `ticker`); wasm runtime `proxy-bidder`; admin UUID `00000000-0000-0000-0000-000000000001`.
- Expressions: `{{ ... }}`; helpers verified to exist: `$uuid()`, `now()`, `bcrypt_hash/verify`, `hmac(data,key,'sha256')`, `toFloat`, `len`, `$env('X')`. Validate anything fancier with `noda_validate_expression` before relying on it.

---

### Task 1: Scaffold, infrastructure, root config, boot gate

**Files:**
- Create: `projects/auction-house/{noda.json, docker-compose.yml, .env.example, .env, .gitignore, FINDINGS.md, README.md}`
- Create (via scaffold): `projects/auction-house/{routes,workflows,schemas,tests,migrations}/`

**Interfaces:**
- Produces: service names, middleware presets (`authenticated`, `admin`), `limiter:bids` instance, route group `/api/admin`, casbin model — all later tasks reference these.

- [ ] **Step 1: Branch + scaffold.** `git checkout -b feat/auction-house`. Run MCP `noda_scaffold_project` with path `/Users/marten/GolandProjects/noda/projects/auction-house` (or `noda init projects/auction-house` if that's what docs/01-getting-started says). Inspect what it generated; delete sample routes/workflows we won't use. Log friction if scaffold output contradicts docs.

- [ ] **Step 2: Seed FINDINGS.md** with header + the six candidates already found during planning (verify each is real before final triage):

```markdown
# FINDINGS — auction-house dogfooding build

Format: [F-##] severity (bug|doc|dx|gap) — expected vs. actual — where

- [F-01] minor (doc) — MCP noda_get_examples "scheduled-job" schedule JSON uses top-level workflow/input, but the schedule config schema requires trigger.{workflow,input}.
- [F-02] minor (doc) — examples/saas-backend worker uses retry.{max_attempts,dlq}; worker schema defines dead_letter.{topic,after}. Which is canonical for new configs is undocumented.
- [F-03] minor (doc) — examples/init-example test uses expect.outputs.<node>.Body; the test config schema defines only expect.output.
- [F-04] paper-cut (doc) — docs/02-config/schedules.md declares 6-field cron but its first example ("0 2 * * *") is 5-field.
- [F-05] minor (doc) — MCP auth example util.jwt_sign uses expires_in and omits secret; node schema says expiry + required secret.
- [F-06] minor (doc) — response.file node exists (noda_list_nodes) but has no page in docs/03-nodes/.
```

- [ ] **Step 3: Write `docker-compose.yml`:**

```yaml
services:
  postgres:
    image: postgres:16-alpine
    environment:
      POSTGRES_USER: noda
      POSTGRES_PASSWORD: noda
      POSTGRES_DB: auction
    ports: ["5432:5432"]
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U noda"]
      interval: 2s
      retries: 20
  redis:
    image: redis:7-alpine
    ports: ["6379:6379"]
  mailpit:
    image: axllent/mailpit
    ports: ["1025:1025", "8025:8025"]
```

- [ ] **Step 4: Write `.env` + `.env.example`** with the Global Constraints values (`.env.example` with placeholder secrets). `.gitignore`: `.env`, `data/`, `*.wasm` binaries stay committed? — commit the built `.wasm` (examples do), ignore only `.env` and `data/`.

- [ ] **Step 5: Write `noda.json`:**

```json
{
  "server": { "port": 8080 },
  "secrets": { "providers": [ { "type": "dotenv" }, { "type": "env" } ] },
  "security": {
    "jwt": {
      "secret": "{{ $env('JWT_SECRET') }}",
      "algorithm": "HS256",
      "token_lookup": "header:Authorization"
    },
    "casbin": {
      "model": "[request_definition]\nr = sub, obj, act\n\n[policy_definition]\np = sub, obj, act\n\n[role_definition]\ng = _, _\n\n[policy_effect]\ne = some(where (p.eft == allow))\n\n[matchers]\nm = g(r.sub, p.sub) && keyMatch2(r.obj, p.obj) && (r.act == p.act || p.act == \"*\")",
      "policies": [ [ "p", "admin", "/api/admin/*", "*" ] ],
      "role_links": [ [ "g", "00000000-0000-0000-0000-000000000001", "admin" ] ]
    }
  },
  "middleware_instances": {
    "limiter:bids": {
      "type": "limiter",
      "config": { "max": 100, "expiration": "1m", "storage": "redis", "redis_url": "{{ $env('REDIS_URL') }}" }
    }
  },
  "middleware_presets": {
    "authenticated": [ "auth.jwt" ],
    "admin": [ "auth.jwt", "casbin.enforce" ]
  },
  "route_groups": {
    "/api/admin": { "middleware_preset": "admin" }
  },
  "services": {
    "main-db":    { "plugin": "postgres", "config": { "url": "{{ $env('DATABASE_URL') }}" } },
    "app-cache":  { "plugin": "cache",    "config": { "url": "{{ $env('REDIS_URL') }}" } },
    "main-stream":{ "plugin": "stream",   "config": { "url": "{{ $env('REDIS_URL') }}" } },
    "realtime":   { "plugin": "pubsub",   "config": { "url": "{{ $env('REDIS_URL') }}" } },
    "files":      { "plugin": "storage",  "config": { "backend": "local", "path": "./data/files" } },
    "mailer":     { "plugin": "email",    "config": { "host": "{{ $env('SMTP_HOST') }}", "port": "{{ $env('SMTP_PORT') }}", "from": "{{ $env('SMTP_FROM') }}" } },
    "psp-client": { "plugin": "http",     "config": {} }
  }
}
```

Note: dynamic role assignment isn't possible — `role_links` are static config, so the admin user must be a seeded fixed UUID (Task 2). If that's confirmed (check `docs/02-config/noda-json.md` security section), log it: `[F-##] major (gap) — no way to assign casbin roles from data (DB/JWT claim); admin identity must be hardcoded in config`.

- [ ] **Step 6: Boot gate.** `docker compose up -d`, then `noda validate --verbose` (fix until clean), then `noda dev` — server must boot and stay up. `curl -s localhost:8080/` (404 is fine, connection refused is not). Ctrl-C.

- [ ] **Step 7: Commit** `feat(auction-house): scaffold, infra, root config`.

---

### Task 2: Migrations & seed admin

**Files:**
- Create: `projects/auction-house/migrations/20260703100000_init.up.sql` + `.down.sql`
- Create: `projects/auction-house/migrations/20260703100001_seed_admin.up.sql` + `.down.sql`

**Interfaces:**
- Produces: tables `users, listings, bids, proxy_bids, watches, orders, listing_photos, audit_log`; admin user `admin@bidhub.local` / password `admin-password-123` with UUID `00000000-0000-0000-0000-000000000001`.

- [ ] **Step 1: Read the migrations doc** (`docs/` — the crud MCP example references `noda://docs/migrations`; find the user-facing page, e.g. via `grep -rl "migrate" docs/ --include='*.md' | grep -v _internal`). Confirm file naming and `noda migrate` verbs. If only `docs/_internal/` documents the CLI, that's `[F-##] major (doc) — migration CLI has no user-facing documentation`.

- [ ] **Step 2: Write `20260703100000_init.up.sql`:**

```sql
CREATE TABLE users (
  id            UUID PRIMARY KEY,
  email         TEXT NOT NULL UNIQUE,
  name          TEXT NOT NULL,
  password_hash TEXT NOT NULL,
  role          TEXT NOT NULL DEFAULT 'user',
  suspended     BOOLEAN NOT NULL DEFAULT FALSE,
  created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE listings (
  id             UUID PRIMARY KEY,
  seller_id      UUID NOT NULL REFERENCES users(id),
  title          TEXT NOT NULL,
  description    TEXT NOT NULL DEFAULT '',
  status         TEXT NOT NULL DEFAULT 'draft',
  starting_price NUMERIC(12,2) NOT NULL,
  bid_increment  NUMERIC(12,2) NOT NULL,
  current_price  NUMERIC(12,2),
  bid_count      INTEGER NOT NULL DEFAULT 0,
  ends_at        TIMESTAMPTZ NOT NULL,
  winner_id      UUID REFERENCES users(id),
  closed_at      TIMESTAMPTZ,
  created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_listings_status_ends ON listings(status, ends_at);

CREATE TABLE bids (
  id         UUID PRIMARY KEY,
  listing_id UUID NOT NULL REFERENCES listings(id),
  bidder_id  UUID NOT NULL REFERENCES users(id),
  amount     NUMERIC(12,2) NOT NULL,
  proxy      BOOLEAN NOT NULL DEFAULT FALSE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (listing_id, amount)
);
CREATE INDEX idx_bids_listing_amount ON bids(listing_id, amount DESC);

CREATE TABLE proxy_bids (
  id         UUID PRIMARY KEY,
  listing_id UUID NOT NULL REFERENCES listings(id),
  user_id    UUID NOT NULL REFERENCES users(id),
  max_amount NUMERIC(12,2) NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (listing_id, user_id)
);

CREATE TABLE watches (
  user_id    UUID NOT NULL REFERENCES users(id),
  listing_id UUID NOT NULL REFERENCES listings(id),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (user_id, listing_id)
);

CREATE TABLE orders (
  id         UUID PRIMARY KEY,
  listing_id UUID NOT NULL UNIQUE REFERENCES listings(id),
  winner_id  UUID NOT NULL REFERENCES users(id),
  amount     NUMERIC(12,2) NOT NULL,
  status     TEXT NOT NULL DEFAULT 'pending',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  paid_at    TIMESTAMPTZ
);

CREATE TABLE listing_photos (
  id         UUID PRIMARY KEY,
  listing_id UUID NOT NULL REFERENCES listings(id),
  path       TEXT NOT NULL,
  thumb_path TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE audit_log (
  id         UUID PRIMARY KEY,
  event_type TEXT NOT NULL,
  payload    JSONB NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

`UNIQUE (listing_id, amount)` on bids is a deliberate DB backstop for the concurrency probes — the CAS should prevent duplicates, and if the constraint ever fires we learn the CAS isn't sufficient.

`.down.sql`: `DROP TABLE audit_log, listing_photos, orders, watches, proxy_bids, bids, listings, users;` (one per line, reverse order).

- [ ] **Step 3: Seed admin.** Generate the bcrypt hash from the repo root:

```bash
cat > /tmp/hash.go <<'EOF'
package main
import ("fmt"; "golang.org/x/crypto/bcrypt")
func main() { h, _ := bcrypt.GenerateFromPassword([]byte("admin-password-123"), 10); fmt.Println(string(h)) }
EOF
go run /tmp/hash.go
```

`20260703100001_seed_admin.up.sql` (paste the hash):

```sql
INSERT INTO users (id, email, name, password_hash, role)
VALUES ('00000000-0000-0000-0000-000000000001', 'admin@bidhub.local', 'Admin', '<PASTE_BCRYPT_HASH>', 'admin');
```

`.down.sql`: `DELETE FROM users WHERE id = '00000000-0000-0000-0000-000000000001';`

- [ ] **Step 4: Run + verify.** `noda migrate up`, then `noda migrate status` (both applied). Verify: `docker compose exec postgres psql -U noda auction -c '\dt'` shows 8 tables. Run `noda migrate down && noda migrate up` once to prove down-migrations work.

- [ ] **Step 5: Commit** `feat(auction-house): schema migrations + seeded admin`.

---

### Task 3: Auth — register & login (first TDD cycle)

**Files:**
- Create: `routes/register.json`, `routes/login.json`, `workflows/register.json`, `workflows/login.json`, `schemas/register.json`, `tests/register.test.json`, `tests/login.test.json`
(all paths relative to `projects/auction-house/`)

**Interfaces:**
- Produces: `POST /api/auth/register` → 201 `{id,email,name}`; `POST /api/auth/login` → 200 `{token}` with JWT claims `{sub: <user uuid>, email, role}`. All later authenticated calls use `Authorization: Bearer <token>`; workflows read the caller as `auth.sub`.

- [ ] **Step 1: Write the failing tests first.** `tests/register.test.json`:

```json
{
  "id": "register-test",
  "workflow": "register",
  "tests": [
    {
      "name": "creates user and strips hash",
      "input": { "email": "a@b.test", "name": "Alice", "password": "longenough1" },
      "mocks": {
        "create": { "output_name": "success", "output": { "id": "u-1", "email": "a@b.test", "name": "Alice", "role": "user" } }
      },
      "expect": { "status": "success" }
    },
    {
      "name": "rejects invalid email",
      "input": { "email": "not-an-email", "name": "Alice", "password": "longenough1" },
      "expect": { "status": "success", "outputs": { "invalid": { "Status": 400 } } }
    }
  ]
}
```

`tests/login.test.json`:

```json
{
  "id": "login-test",
  "workflow": "login",
  "tests": [
    {
      "name": "unknown email returns 401",
      "input": { "email": "ghost@b.test", "password": "x" },
      "mocks": { "lookup": { "output_name": "success", "output": null } },
      "expect": { "status": "success", "outputs": { "unauthorized": { "Status": 401 } } }
    },
    {
      "name": "suspended user rejected",
      "input": { "email": "a@b.test", "password": "pw" },
      "mocks": { "lookup": { "output_name": "success", "output": { "id": "u-1", "suspended": true, "password_hash": "" } } },
      "expect": { "status": "success", "outputs": { "unauthorized": { "Status": 401 } } }
    }
  ]
}
```

The exact `expect.outputs` shape is uncertain (F-03): consult `docs/02-config/tests.md` first and use its form. If the documented form fails against a real run, log friction.

- [ ] **Step 2: Run `noda test`** — expect FAIL (workflow `register` not found).

- [ ] **Step 3: Implement.** `schemas/register.json`:

```json
{
  "type": "object",
  "required": ["email", "name", "password"],
  "additionalProperties": false,
  "properties": {
    "email": { "type": "string", "format": "email" },
    "name": { "type": "string", "minLength": 1, "maxLength": 80 },
    "password": { "type": "string", "minLength": 10 }
  }
}
```

`routes/register.json`:

```json
{
  "id": "register",
  "method": "POST",
  "path": "/api/auth/register",
  "trigger": {
    "workflow": "register",
    "input": {
      "email": "{{ request.body.email }}",
      "name": "{{ request.body.name }}",
      "password": "{{ request.body.password }}"
    }
  }
}
```

`workflows/register.json`:

```json
{
  "id": "register",
  "nodes": {
    "validate": { "type": "transform.validate", "config": { "schema": "$ref(schemas/register.json)" } },
    "hash": { "type": "transform.set", "config": { "fields": { "password_hash": "{{ bcrypt_hash(input.password) }}" } } },
    "create": {
      "type": "db.create", "services": { "database": "main-db" },
      "config": { "table": "users", "data": {
        "id": "{{ $uuid() }}", "email": "{{ lower(input.email) }}", "name": "{{ input.name }}",
        "password_hash": "{{ nodes.hash.password_hash }}", "role": "user"
      } }
    },
    "respond": { "type": "response.json", "config": { "status": 201, "body": {
      "id": "{{ nodes.create.id }}", "email": "{{ nodes.create.email }}", "name": "{{ nodes.create.name }}" } } },
    "invalid": { "type": "response.error", "config": { "status": 400, "message": "Invalid registration data" } },
    "conflict": { "type": "response.error", "config": { "status": 409, "message": "Email already registered" } }
  },
  "edges": [
    { "from": "validate", "to": "hash", "output": "success" },
    { "from": "validate", "to": "invalid", "output": "error" },
    { "from": "hash", "to": "create", "output": "success" },
    { "from": "create", "to": "respond", "output": "success" },
    { "from": "create", "to": "conflict", "output": "error" }
  ]
}
```

`routes/login.json` — same shape, path `/api/auth/login`, workflow `login`, input `{email, password}`.

`workflows/login.json`:

```json
{
  "id": "login",
  "nodes": {
    "lookup": { "type": "db.findOne", "services": { "database": "main-db" },
      "config": { "table": "users", "where": { "email": "{{ lower(input.email) }}" }, "required": false } },
    "usable": { "type": "control.if",
      "config": { "condition": "{{ nodes.lookup != nil && !nodes.lookup.suspended && bcrypt_verify(input.password, nodes.lookup.password_hash) }}" } },
    "sign": { "type": "util.jwt_sign", "config": {
      "claims": { "sub": "{{ nodes.lookup.id }}", "email": "{{ nodes.lookup.email }}", "role": "{{ nodes.lookup.role }}" },
      "secret": "{{ $env('JWT_SECRET') }}", "expiry": "24h" } },
    "respond": { "type": "response.json", "config": { "status": 200, "body": { "token": "{{ nodes.sign.token }}" } } },
    "unauthorized": { "type": "response.error", "config": { "status": 401, "message": "Invalid credentials" } }
  },
  "edges": [
    { "from": "lookup", "to": "usable", "output": "success" },
    { "from": "usable", "to": "sign", "output": "then" },
    { "from": "usable", "to": "unauthorized", "output": "else" },
    { "from": "sign", "to": "respond", "output": "success" }
  ]
}
```

Note `control.if` outputs are `then`/`else` per `noda_list_nodes` — the MCP auth example uses `true`/`false` edges. Whichever `noda validate` accepts wins; log the loser as a doc finding.

- [ ] **Step 4: Run `noda test`** — expect PASS. Then live check: `noda dev` in background, register + login with curl, confirm 201/200 and decode the JWT (`cut -d. -f2 | base64 -d`) has `sub`/`role`.

- [ ] **Step 5: Commit** `feat(auction-house): auth register/login with tests`.

---

### Task 4: Listings CRUD + admin moderation (casbin)

**Files:**
- Create: `routes/{create-listing,activate-listing,list-listings,get-listing,admin-suspend-listing,admin-suspend-user}.json`, matching `workflows/*.json`, `schemas/listing.json`, `tests/create-listing.test.json`

**Interfaces:**
- Consumes: auth from Task 3.
- Produces: `POST /api/auctions` (draft), `POST /api/auctions/:listing_id/activate`, `GET /api/auctions`, `GET /api/auctions/:listing_id`; `POST /api/admin/listings/:listing_id/suspend`, `POST /api/admin/users/:user_id/suspend`. Listing statuses: `draft|active|closed|cancelled|suspended`.

- [ ] **Step 1: Failing test** `tests/create-listing.test.json`: valid input with mocked `create` → success; `ends_at` in the past → expect the 400 node (`invalid`). Run `noda test` → FAIL.

- [ ] **Step 2: Implement.** `schemas/listing.json`:

```json
{
  "type": "object",
  "required": ["title", "starting_price", "bid_increment", "ends_at"],
  "additionalProperties": false,
  "properties": {
    "title": { "type": "string", "minLength": 1, "maxLength": 120 },
    "description": { "type": "string", "maxLength": 2000 },
    "starting_price": { "type": "number", "exclusiveMinimum": 0 },
    "bid_increment": { "type": "number", "exclusiveMinimum": 0 },
    "ends_at": { "type": "string", "format": "date-time" }
  }
}
```

`routes/create-listing.json` (preset `authenticated`; input maps body fields + `seller_id: {{ auth.sub }}`) → `workflows/create-listing.json`: `validate` ($ref schema) → `future` (`control.if` ends_at in the future — try `{{ input.ends_at > now() }}`; validate the expression with `noda_validate_expression` first; if date comparison on the string fails, delegate to SQL later and log a dx finding) → `create` (`db.create` listings, `status: "draft"`, `id: $uuid()`) → 201. Error edges → 400.

`activate-listing`: route POST `/api/auctions/:listing_id/activate` (authenticated), workflow: `load` (`db.findOne` listings, required true) → `owns` (`control.if` `nodes.load.seller_id == input.user_id && nodes.load.status == 'draft'`) → `activate` (`db.update` status active where id) → `announce` (`sse.send` services `{connections:"ticker"}` channel `ticker`, data `{type:"listing.activated", listing_id, title, ends_at}`) → 200. else → 403. (SSE endpoint arrives in Task 6 — add the `announce` node then if validation requires the endpoint to exist; note ordering.) Actually: **defer the `announce` node to Task 6** to keep this task self-contained.

`list-listings`: GET `/api/auctions` public. `db.find` listings `where {status:"active"}`, `order "ends_at asc"`, `limit 50` → `response.json` with the array.

`get-listing`: GET `/api/auctions/:listing_id` public. `load` (findOne required) → `bids` (`db.find` bids where listing_id, order `"amount desc"`, limit 10) → `photos` (`db.find` listing_photos where listing_id) → `respond` 200 `{listing: nodes.load, bids: nodes.bids, photos: nodes.photos}`.

`admin-suspend-listing`: POST `/api/admin/listings/:listing_id/suspend` (group middleware handles auth) → `db.update` listings `{status:"suspended"}` where id → 200. `admin-suspend-user`: `db.update` users `{suspended:true}` where id → 200.

- [ ] **Step 3: `noda test` passes; live casbin check.** `noda dev`; as a normal user hit an admin route → expect 403; as `admin@bidhub.local` → 200. If casbin lets normal users through or errors opaquely, FINDINGS.

- [ ] **Step 4: Commit** `feat(auction-house): listings CRUD + admin moderation`.

---

### Task 5: Photo upload, thumbnails, file serving

**Files:**
- Create: `routes/{upload-photo,get-photo-thumb}.json`, `workflows/{upload-photo,get-photo-thumb}.json`

**Interfaces:**
- Consumes: `files` storage service, listings from Task 4.
- Produces: `POST /api/auctions/:listing_id/photos` (multipart field `photo`) → 201 `{photo_id, path, thumb_path}`; `GET /photos/:photo_id/thumb` → image bytes. Rows in `listing_photos`.

- [ ] **Step 1: Read `docs/03-nodes/upload.handle.md` and `image.thumbnail.md`** — the exact output field for the stored path is unverified. Adjust the JSON below to match; log friction if the doc doesn't say what `success` output contains.

- [ ] **Step 2: Implement `workflows/upload-photo.json`:** `load` listing (findOne required) → `owns` (control.if seller + status draft-or-active) → `cap` (`db.count` listing_photos where listing_id; control.if `< 3`) → `upload` (`upload.handle`, services `{destination:"files"}`, config `{field:"photo", max_size: 5242880, allowed_types:["image/jpeg","image/png"], path:"photos/{{ input.listing_id }}"}`) → `thumb` (`image.thumbnail`, services `{source:"files","target":"files"}`, config `{input:"photos/{{ input.listing_id }}/{{ nodes.upload.filename }}", output:"thumbs/{{ input.listing_id }}/{{ nodes.upload.filename }}", width:320, height:240}`) → `record` (db.create listing_photos with both paths) → 201. Errors: upload error → 400, cap else → 409, owns else → 403.

- [ ] **Step 3: `get-photo-thumb`:** GET `/photos/:photo_id/thumb` public → `load` (findOne listing_photos required) → `read` (`storage.read` services `{storage:"files"}` config `{path:"{{ nodes.load.thumb_path }}"}`) → `serve` (`response.file` config `{data:"{{ nodes.read }}", content_type:"image/jpeg"}`). 404 on load error via `response.error`.

- [ ] **Step 4: Live verify** (needs libvips installed — if `noda dev` fails to load the image plugin locally, log it and note whether docs warn about the libvips prerequisite): upload a real PNG with curl `-F photo=@...` (generate: `python3 -c "import base64,sys;sys.stdout.buffer.write(base64.b64decode('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg=='))" > /tmp/px.png` — a 1×1 PNG; thumbnailing a 1×1 to 320×240 may error, which is itself informative; also test with a real photo). GET the thumb URL → 200 image bytes.

- [ ] **Step 5: Commit** `feat(auction-house): photo upload + thumbnails + serving`.

---

### Task 6: Realtime plumbing — WS endpoint, SSE ticker, watch

**Files:**
- Create: `connections/realtime.json`, `routes/watch-listing.json`, `workflows/watch-listing.json`
- Modify: `workflows/activate-listing.json` (add ticker announce)

**Interfaces:**
- Produces: WS `/ws/auctions/:listing_id` (endpoint `auction-ws`, channel `auction.<listing_id>`, JWT-authed); SSE `/sse/ticker` (endpoint `ticker`, channel `ticker`, public); `POST /api/auctions/:listing_id/watch`. Later tasks send via `ws.send`/`sse.send` with those service/channel names.

- [ ] **Step 1: Read `docs/02-config/connections.md`.** Confirm: separate `connections/*.json` file vs `connections` key in noda.json; whether SSE endpoints support `channels.pattern` without params; how WS auth passes the JWT (header vs query param `?token=`). Log doc gaps.

- [ ] **Step 2: `connections/realtime.json`:**

```json
{
  "sync": { "pubsub": "realtime" },
  "endpoints": {
    "auction-ws": {
      "type": "websocket",
      "path": "/ws/auctions/:listing_id",
      "middleware": ["auth.jwt"],
      "channels": { "pattern": "auction.{{ request.params.listing_id }}", "max_per_channel": 200 },
      "ping_interval": "30s"
    },
    "ticker": {
      "type": "sse",
      "path": "/sse/ticker",
      "channels": { "pattern": "ticker" }
    }
  }
}
```

- [ ] **Step 3: `watch-listing`:** route POST `/api/auctions/:listing_id/watch` (authenticated) → workflow: `load` listing (required, must be active) → `save` (`db.upsert` watches `{user_id: input.user_id, listing_id: input.listing_id}` — check `db.upsert` schema for conflict-target config) → `respond` 200 `{watching:true}`.

- [ ] **Step 4: Add `announce` to `activate-listing`** (deferred from Task 4): `sse.send` services `{connections:"ticker"}`, config `{channel:"ticker", "event":"listing.activated", "data":{"listing_id":"{{ input.listing_id }}","title":"{{ nodes.load.title }}","ends_at":"{{ nodes.load.ends_at }}"}}` wired between `activate` and `respond`.

- [ ] **Step 5: Smoke check by hand:** `noda dev`; `curl -N localhost:8080/sse/ticker` in one terminal; activate a listing in another → the SSE event must appear. For WS, quick check with the Task 12 smoke tool later; here just confirm the endpoint upgrades (curl gives 400/426 rather than 404).

- [ ] **Step 6: `noda validate` + `noda test` + commit** `feat(auction-house): WS/SSE endpoints + watch`.

---

### Task 7: Bidding core — CAS transaction, anti-snipe, broadcast

**Files:**
- Create: `routes/place-bid.json`, `workflows/{place-bid,place-bid-tx,broadcast-bid}.json`, `tests/place-bid-tx.test.json`

**Interfaces:**
- Consumes: listings/bids tables, `auction-ws`/`ticker` endpoints, `main-stream`.
- Produces: `POST /api/auctions/:listing_id/bids` `{amount}` → 201 `{accepted:true, amount, ends_at}` or 422. Sub-workflow `place-bid-tx` input `{listing_id, user_id, amount, proxy}` → output `{accepted, reason?, amount?, ends_at?}` — Task 9 reuses it for proxy bids. `broadcast-bid` input `{listing_id, user_id, amount, ends_at, proxy}` emits stream topic `bid.placed` (payload = that input) + WS + SSE — Tasks 8/9 depend on this payload shape.

- [ ] **Step 1: Failing tests** `tests/place-bid-tx.test.json` (run `noda test` → FAIL before implementing):

```json
{
  "id": "place-bid-tx-test",
  "workflow": "place-bid-tx",
  "tests": [
    {
      "name": "rejects bid below increment",
      "input": { "listing_id": "l-1", "user_id": "u-2", "amount": 102, "proxy": false },
      "mocks": { "listing": { "output_name": "success", "output": { "id": "l-1", "seller_id": "u-1", "status": "active", "starting_price": 50, "bid_increment": 5, "current_price": 100 } } },
      "expect": { "status": "success", "output": { "accepted": false } }
    },
    {
      "name": "rejects own listing",
      "input": { "listing_id": "l-1", "user_id": "u-1", "amount": 200, "proxy": false },
      "mocks": { "listing": { "output_name": "success", "output": { "id": "l-1", "seller_id": "u-1", "status": "active", "starting_price": 50, "bid_increment": 5, "current_price": 100 } } },
      "expect": { "status": "success", "output": { "accepted": false } }
    },
    {
      "name": "accepts valid bid",
      "input": { "listing_id": "l-1", "user_id": "u-2", "amount": 105, "proxy": false },
      "mocks": {
        "listing": { "output_name": "success", "output": { "id": "l-1", "seller_id": "u-1", "status": "active", "starting_price": 50, "bid_increment": 5, "current_price": 100 } },
        "cas": { "output_name": "success", "output": { "rows_affected": 1 } },
        "insert": { "output_name": "success", "output": { "id": "b-1", "amount": 105 } },
        "snipe_guard": { "output_name": "success", "output": { "rows_affected": 0 } },
        "reload": { "output_name": "success", "output": { "id": "l-1", "ends_at": "2026-07-04T00:00:00Z" } }
      },
      "expect": { "status": "success", "output": { "accepted": true } }
    },
    {
      "name": "lost CAS race reports outbid",
      "input": { "listing_id": "l-1", "user_id": "u-2", "amount": 105, "proxy": false },
      "mocks": {
        "listing": { "output_name": "success", "output": { "id": "l-1", "seller_id": "u-1", "status": "active", "starting_price": 50, "bid_increment": 5, "current_price": 100 } },
        "cas": { "output_name": "success", "output": { "rows_affected": 0 } }
      },
      "expect": { "status": "success", "output": { "accepted": false } }
    }
  ]
}
```

- [ ] **Step 2: `workflows/place-bid-tx.json`** — the heart of the project:

```json
{
  "id": "place-bid-tx",
  "nodes": {
    "listing": { "type": "db.findOne", "services": { "database": "main-db" },
      "config": { "table": "listings", "where": { "id": "{{ input.listing_id }}" }, "required": true } },
    "guard": { "type": "control.if", "config": {
      "condition": "{{ nodes.listing.status == 'active' && nodes.listing.seller_id != input.user_id && toFloat(input.amount) >= (nodes.listing.current_price == nil ? toFloat(nodes.listing.starting_price) : toFloat(nodes.listing.current_price) + toFloat(nodes.listing.bid_increment)) }}" } },
    "cas": { "type": "db.exec", "services": { "database": "main-db" }, "config": {
      "query": "UPDATE listings SET current_price = $1, bid_count = bid_count + 1 WHERE id = $2 AND status = 'active' AND ends_at > now() AND (current_price IS NULL OR current_price + bid_increment <= $1)",
      "params": [ "{{ toFloat(input.amount) }}", "{{ input.listing_id }}" ] } },
    "won_cas": { "type": "control.if", "config": { "condition": "{{ nodes.cas.rows_affected == 1 }}" } },
    "insert": { "type": "db.create", "services": { "database": "main-db" }, "config": {
      "table": "bids", "data": { "id": "{{ $uuid() }}", "listing_id": "{{ input.listing_id }}",
        "bidder_id": "{{ input.user_id }}", "amount": "{{ toFloat(input.amount) }}", "proxy": "{{ input.proxy }}" } } },
    "snipe_guard": { "type": "db.exec", "services": { "database": "main-db" }, "config": {
      "query": "UPDATE listings SET ends_at = now() + interval '2 minutes' WHERE id = $1 AND ends_at < now() + interval '2 minutes'",
      "params": [ "{{ input.listing_id }}" ] } },
    "reload": { "type": "db.findOne", "services": { "database": "main-db" },
      "config": { "table": "listings", "where": { "id": "{{ input.listing_id }}" }, "required": true } },
    "accepted": { "type": "workflow.output", "config": { "data": {
      "accepted": true, "amount": "{{ toFloat(input.amount) }}", "ends_at": "{{ nodes.reload.ends_at }}", "extended": "{{ nodes.snipe_guard.rows_affected == 1 }}" } } },
    "rejected_guard": { "type": "workflow.output", "config": { "data": { "accepted": false, "reason": "bid does not meet requirements" } } },
    "rejected_race": { "type": "workflow.output", "config": { "data": { "accepted": false, "reason": "outbid or auction closed" } } }
  },
  "edges": [
    { "from": "listing", "to": "guard", "output": "success" },
    { "from": "guard", "to": "cas", "output": "then" },
    { "from": "guard", "to": "rejected_guard", "output": "else" },
    { "from": "cas", "to": "won_cas", "output": "success" },
    { "from": "won_cas", "to": "insert", "output": "then" },
    { "from": "won_cas", "to": "rejected_race", "output": "else" },
    { "from": "insert", "to": "snipe_guard", "output": "success" },
    { "from": "snipe_guard", "to": "reload", "output": "success" },
    { "from": "reload", "to": "accepted", "output": "success" }
  ]
}
```

Check `workflow.output`'s schema first (`noda_get_node_schema workflow.output`) — if its config field isn't `data`, adjust. Anti-snipe and expiry checks are all SQL (`now()` in Postgres), deliberately avoiding expr date math.

- [ ] **Step 3: `workflows/broadcast-bid.json`:** three nodes in a chain, input `{listing_id, user_id, amount, ends_at, proxy}` — `emit` (`event.emit`, services `{stream:"main-stream"}`, config `{mode:"stream", topic:"bid.placed", payload: {listing_id, bidder_id: user_id, amount, ends_at, proxy — all as {{ input.* }} }}`) → `push_ws` (`ws.send`, services `{connections:"auction-ws"}`, config `{channel:"auction.{{ input.listing_id }}", data:{type:"bid", amount:"{{ input.amount }}", ends_at:"{{ input.ends_at }}"}}`) → `push_sse` (`sse.send`, services `{connections:"ticker"}`, config `{channel:"ticker", event:"bid", data:{listing_id:"{{ input.listing_id }}", amount:"{{ input.amount }}"}}`) → `done` (`workflow.output`, `{sent:true}`).

- [ ] **Step 4: `routes/place-bid.json`:**

```json
{
  "id": "place-bid",
  "method": "POST",
  "path": "/api/auctions/:listing_id/bids",
  "middleware": [ "auth.jwt", "limiter:bids" ],
  "body": { "validate": true, "schema": { "type": "object", "required": ["amount"],
    "properties": { "amount": { "type": "number", "exclusiveMinimum": 0 } } } },
  "trigger": { "workflow": "place-bid", "input": {
    "listing_id": "{{ request.params.listing_id }}", "user_id": "{{ auth.sub }}", "amount": "{{ request.body.amount }}" } }
}
```

`workflows/place-bid.json`: `tx` (`workflow.run` config `{workflow:"place-bid-tx", "transaction": true, "input": {listing_id, user_id, amount, proxy: false}}`, services `{database:"main-db"}`) → `ok` (control.if `nodes.tx.accepted`) → then: `broadcast` (`workflow.run` broadcast-bid with tx outputs) → `respond` 201 `{accepted:true, amount, ends_at, extended}`; else: `rejected` (`response.error` 422 `"{{ nodes.tx.reason }}"` — if response.error message doesn't interpolate, use static text + log finding). `tx` error edge → `response.error` 404.

- [ ] **Step 5: `noda test`** → all place-bid-tx cases PASS. Live: create+activate listing, bid below increment → 422; valid bid → 201; check `curl -N /sse/ticker` sees the bid event; second bid at same amount → 422.

- [ ] **Step 6: Commit** `feat(auction-house): CAS bidding with anti-snipe + broadcast`.

---

### Task 8: Events, notification worker, DLQ

**Files:**
- Create: `workers/notify-bid.json`, `workflows/notify-bid-handler.json`, `routes/poison-event.json`, `workflows/poison-event.json`

**Interfaces:**
- Consumes: topic `bid.placed` payload `{listing_id, bidder_id, amount, ends_at, proxy}` from `broadcast-bid`.
- Produces: outbid emails via Mailpit; DLQ stream `notify-dlq`; `POST /api/admin/poison-event` (admin) emits a poison `bid.placed` with `amount: -1`.

- [ ] **Step 1: Read `docs/02-config/workers.md`** — resolve F-02 (canonical `dead_letter` vs `retry`). Use the documented canonical form below (assumed `dead_letter`).

- [ ] **Step 2: `workers/notify-bid.json`:**

```json
{
  "id": "notify-bid",
  "services": { "stream": "main-stream" },
  "subscribe": { "topic": "bid.placed", "group": "notify" },
  "concurrency": 4,
  "timeout": "30s",
  "dead_letter": { "topic": "notify-dlq", "after": 3 },
  "trigger": { "workflow": "notify-bid-handler", "input": {
    "listing_id": "{{ message.payload.listing_id }}",
    "bidder_id": "{{ message.payload.bidder_id }}",
    "amount": "{{ message.payload.amount }}" } }
}
```

- [ ] **Step 3: `workflows/notify-bid-handler.json`:** `poison` (control.if `toFloat(input.amount) < 0`) → then: `explode` (`db.findOne` table `poison_probe`, `required: true` — table doesn't exist, node errors, workflow fails, worker retries 3× then DLQs; if a nonexistent-table error turns out to be non-retryable poison-message disposition instead, that's the #244 semantics — observe and log what actually happens) ; else: `top2` (`db.find` bids where listing_id, order `"amount desc"`, limit 2) → `has_outbid` (control.if `len(nodes.top2) == 2 && nodes.top2[1].bidder_id != input.bidder_id`) → then: `loser` (db.findOne users where id = `{{ nodes.top2[1].bidder_id }}`) → `listing` (db.findOne listings) → `mail` (`email.send`, services `{mailer:"mailer"}`, config `{to:"{{ nodes.loser.email }}", subject:"You've been outbid on {{ nodes.listing.title }}", body:"New high bid: {{ input.amount }}. Beat it at {{ $env('BASE_URL') }}/api/auctions/{{ input.listing_id }}"}`) → `done` (workflow.output `{notified:true}`); else-paths → workflow.output `{notified:false}`.

- [ ] **Step 4: Poison route** (admin group): `routes/poison-event.json` POST `/api/admin/poison-event` → `workflows/poison-event.json`: single `event.emit` `{mode:"stream", topic:"bid.placed", payload:{listing_id:"poison", bidder_id:"poison", amount:-1}}` → 202.

- [ ] **Step 5: Live verify.** `noda dev`; run a real bid war between two users → Mailpit API (`curl localhost:8025/api/v1/messages`) shows the outbid email. Fire poison event → within ~30s `docker compose exec redis redis-cli XLEN notify-dlq` ≥ 1 (confirm actual DLQ stream key naming — inspect with `redis-cli KEYS '*dlq*'`; log naming discoverability).

- [ ] **Step 6: Commit** `feat(auction-house): outbid notifications + DLQ poison path`.

---

### Task 9: Wasm proxy-bidding engine

**Files:**
- Create: `wasm/proxy-bidder/{main.go, go.mod}`, `wasm/proxy-bidder.wasm` (built), `routes/set-proxy-bid.json`, `workflows/{set-proxy-bid,run-proxy-round,apply-proxy-bid}.json`, `tests/run-proxy-round.test.json`
- Modify: `noda.json` (add `wasm_runtimes`), `workflows/place-bid.json` (proxy round after accept)

**Interfaces:**
- Consumes: `place-bid-tx`, `broadcast-bid` (Task 7).
- Produces: `POST /api/auctions/:listing_id/proxy-bid` `{max_amount}`; wasm runtime `proxy-bidder`; query contract: input `{current_price: number|null, starting_price, bid_increment, seller_id, high_bidder_id: string|null, proxy_bids: [{user_id, max_amount}]}` → output `{counter_bids: [{user_id, amount}]}` (ordered sequence of bids to apply).

- [ ] **Step 1: Read `docs/04-guides/wasm-development.md`** (query-only module section) and `examples/wasm-helpers/`. Confirm export names (`initialize`, `query`), `pdk.Input()`/`noda.Output()`, build target.

- [ ] **Step 2: Write the engine.** `wasm/proxy-bidder/go.mod` (copy the wasm-counter pattern, module `proxy-bidder`, `replace github.com/nodafw/noda-pdk-go => ../../../../pdk/go`). `main.go`:

```go
package main

import (
	"encoding/json"

	pdk "github.com/extism/go-pdk"
	"github.com/nodafw/noda-pdk-go/noda"
)

type proxyBid struct {
	UserID    string  `json:"user_id"`
	MaxAmount float64 `json:"max_amount"`
}

type queryIn struct {
	CurrentPrice  *float64   `json:"current_price"`
	StartingPrice float64    `json:"starting_price"`
	BidIncrement  float64    `json:"bid_increment"`
	SellerID      string     `json:"seller_id"`
	HighBidderID  *string    `json:"high_bidder_id"`
	ProxyBids     []proxyBid `json:"proxy_bids"`
}

type counterBid struct {
	UserID string  `json:"user_id"`
	Amount float64 `json:"amount"`
}

//go:wasmexport initialize
func initialize() int32 {
	if _, err := noda.GetInitInput(); err != nil {
		return noda.Fail(err)
	}
	return 0
}

//go:wasmexport query
func query() int32 {
	var in queryIn
	if err := json.Unmarshal(pdk.Input(), &in); err != nil {
		return noda.Fail(err)
	}

	price := in.StartingPrice
	priceIsBid := false // starting price alone is not a bid
	if in.CurrentPrice != nil {
		price = *in.CurrentPrice
		priceIsBid = true
	}
	leader := ""
	if in.HighBidderID != nil {
		leader = *in.HighBidderID
	}

	var out []counterBid
	// English-auction proxy battle: repeat until no proxy can outbid the leader.
	for i := 0; i < 100; i++ {
		next := price + in.BidIncrement
		if !priceIsBid {
			next = price // first bid may match starting price
		}
		var best *proxyBid
		for j := range in.ProxyBids {
			p := &in.ProxyBids[j]
			if p.UserID == leader || p.UserID == in.SellerID || p.MaxAmount < next {
				continue
			}
			if best == nil || p.MaxAmount > best.MaxAmount || (p.MaxAmount == best.MaxAmount && p.UserID < best.UserID) {
				best = p
			}
		}
		if best == nil {
			break
		}
		price, priceIsBid, leader = next, true, best.UserID
		out = append(out, counterBid{UserID: best.UserID, Amount: next})
	}
	if out == nil {
		out = []counterBid{}
	}
	return noda.Output(map[string]any{"counter_bids": out})
}

func main() {}
```

- [ ] **Step 3: Build:** `cd wasm/proxy-bidder && go mod tidy && tinygo build -o ../proxy-bidder.wasm -target wasi ./main.go` (fallback if tinygo unavailable: `GOOS=wasip1 GOARCH=wasm go build -o ../proxy-bidder.wasm .` — log which worked and whether docs cover it). Add to `noda.json`:

```json
"wasm_runtimes": {
  "proxy-bidder": { "module": "wasm/proxy-bidder.wasm", "encoding": "json" }
}
```

- [ ] **Step 4: Failing test** `tests/run-proxy-round.test.json`: mock `listing`, `top_bid`, `proxies` (two proxy bids maxes 20/15, current 10, increment 1), mock `engine` node output `{counter_bids:[{user_id:"u-3",amount:11}]}`, mock the loop's `apply` iteration… — if `control.loop` iterations can't be mocked in the test runner, test only up to `engine` and log a dx finding about loop testability. Run `noda test` → FAIL, then implement Step 5 until PASS.

- [ ] **Step 5: Workflows.** `run-proxy-round` (input `{listing_id}`): `listing` (findOne required) → `top_bid` (db.find bids order `"amount desc"` limit 1) → `proxies` (db.find proxy_bids where listing_id) → `any` (control.if `len(nodes.proxies) > 0 && nodes.listing.status == 'active'`) → then: `engine` (`wasm.query`, services `{runtime:"proxy-bidder"}`, config `{data:{current_price:"{{ nodes.listing.current_price }}", starting_price:"{{ nodes.listing.starting_price }}", bid_increment:"{{ nodes.listing.bid_increment }}", seller_id:"{{ nodes.listing.seller_id }}", high_bidder_id:"{{ len(nodes.top_bid) > 0 ? nodes.top_bid[0].bidder_id : nil }}", proxy_bids:"{{ nodes.proxies }}"}, timeout:"5s"}`) → `apply` (`control.loop` config `{collection:"{{ nodes.engine.counter_bids }}", workflow:"apply-proxy-bid", input:{listing_id:"{{ input.listing_id }}", user_id:"{{ $item.user_id }}", amount:"{{ $item.amount }}"}}`) → `out` (workflow.output `{counter_bids:"{{ nodes.engine.counter_bids }}"}`); else → workflow.output `{counter_bids:[]}`.

`apply-proxy-bid` (input `{listing_id, user_id, amount}`): `tx` (workflow.run place-bid-tx, transaction, `proxy: true`) → `ok` (control.if accepted) → then `broadcast` (workflow.run broadcast-bid, `proxy: true`) → output; else output `{accepted:false}`.

`set-proxy-bid`: route POST `/api/auctions/:listing_id/proxy-bid` (authenticated, body schema `{max_amount: number > 0}`) → workflow: `listing` (findOne required) → `eligible` (control.if active && not seller) → `save` (db.upsert proxy_bids on (listing_id,user_id) set max_amount) → `round` (workflow.run run-proxy-round) → `respond` 200 `{max_amount, counter_bids: nodes.round.counter_bids}`; else 403.

Wire into `place-bid`: after `broadcast`, add `round` (workflow.run run-proxy-round `{listing_id}`) before `respond`.

- [ ] **Step 6: Live proxy battle.** alice lists (start 10, inc 1); bob sets proxy max 20; carol sets proxy max 15 → expect auto-battle ending with bob leading at 16 (carol's 15 exhausted: sequence 10(bob)…—verify exact sequence semantics against the engine; the invariant that matters: final leader bob, final price ≤ 16, bids strictly increasing). GET listing shows proxy bids flagged `proxy:true`. Outbid emails observed in Mailpit.

- [ ] **Step 7: `noda test` + commit** `feat(auction-house): wasm proxy-bidding engine`.

---

### Task 10: Scheduler — auction closing + orders

**Files:**
- Create: `schedules/close-auctions.json`, `workflows/{close-auctions,close-one-auction}.json`, `workers/close-notify.json`, `workflows/close-notify-handler.json`, `tests/close-one-auction.test.json`

**Interfaces:**
- Consumes: listings/bids/orders tables; `auction-ws`/`ticker`; `main-stream`.
- Produces: topic `auction.closed` payload `{listing_id, seller_id, winner_id: string|null, amount: number|null, order_id: string|null, title}`; orders rows (`status: pending`). Task 11 consumes orders; Task 13's e2e waits on this close path.

- [ ] **Step 1: Failing tests** for `close-one-auction` (input `{listing_id}`): (a) with bids → CAS-close mocked `rows_affected:1`, winner from mocked top bid, order created, emit reached — expect success; (b) no bids → closed with null winner, no order node hit; (c) already closed (CAS 0 rows) → output `{closed:false}` and no emit. `noda test` → FAIL.

- [ ] **Step 2: `workflows/close-one-auction.json`:** `listing` (findOne required) → `top` (db.find bids order `"amount desc"` limit 1) → `cas_close` (db.exec `UPDATE listings SET status='closed', closed_at=now(), winner_id=$2 WHERE id=$1 AND status='active' AND ends_at <= now()` params `[input.listing_id, "{{ len(nodes.top) > 0 ? nodes.top[0].bidder_id : nil }}"]`) → `won` (control.if rows_affected == 1) → else output `{closed:false}`; then → `has_winner` (control.if `len(nodes.top) > 0`) → then `order` (db.create orders `{id:$uuid(), listing_id, winner_id: top[0].bidder_id, amount: top[0].amount, status:"pending"}`) ; both branches converge on `emit` (event.emit topic `auction.closed`, payload with `order_id: "{{ len(nodes.top) > 0 ? nodes.order.id : nil }}"` — a node reference that may be unexecuted on the no-winner path; if the engine rejects referencing a skipped node, split into two emit nodes and log a dx finding) → `push_ws` (ws.send channel `auction.{{ input.listing_id }}` data `{type:"auction.closed", winner_id, amount}`) → `push_sse` (sse.send ticker `event:"auction.closed"`) → output `{closed:true}`.

- [ ] **Step 3: `workflows/close-auctions.json`:** `expired` (db.find listings config `{where_clause:{query:"status = ? AND ends_at <= now()", params:["active"]}, limit:100}` — verify `where_clause` param shape in `docs/03-nodes/db.find.md`) → `each` (control.loop → close-one-auction `{listing_id:"{{ $item.id }}"}`) → output `{closed:"{{ len(nodes.expired) }}"}`.

`schedules/close-auctions.json`:

```json
{
  "id": "close-auctions",
  "cron": "*/10 * * * * *",
  "timezone": "UTC",
  "timeout": "30s",
  "lock": { "enabled": true, "ttl": "30s" },
  "services": { "lock": "app-cache" },
  "trigger": { "workflow": "close-auctions" }
}
```

(Backend-review memory notes a known sub-minute scheduler finding — if `*/10` fires only once a minute or not at all, that's the confirmation; log observed behavior precisely.)

- [ ] **Step 4: `workers/close-notify.json`** — topic `auction.closed`, group `notify`, dead_letter `notify-dlq` after 3, trigger `close-notify-handler` with payload fields. Handler: `has_winner` (control.if winner_id != nil) → then: `winner` (findOne users) + `mail_winner` (email.send "You won {{ title }} at {{ amount }} — pay at {{ $env('BASE_URL') }}/api/orders/{{ order_id }}/pay"); both paths: `seller` (findOne users where id=seller_id) → `mail_seller` (email.send "Your auction {{ title }} ended"). Output `{notified:true}`.

- [ ] **Step 5: Live verify:** listing ending in ~20s, one bid, `noda dev`; within ~30s status flips to `closed`, order exists, both emails in Mailpit, SSE ticker showed `auction.closed`.

- [ ] **Step 6: `noda test` + commit** `feat(auction-house): scheduled closing + orders + notifications`.

---

### Task 11: Payment — pay, mock PSP, signed webhook, settle, audit

**Files:**
- Create: `routes/{pay-order,mock-psp-charge,psp-webhook}.json`, `workflows/{pay-order,mock-psp-charge,psp-webhook}.json`, `workers/audit.json`, `workflows/audit-handler.json`, `tests/psp-webhook.test.json`

**Interfaces:**
- Consumes: orders (Task 10), `psp-client` http service, `PSP_SECRET`.
- Produces: `POST /api/orders/:order_id/pay` → 202; `POST /mock/psp/charge` (public); `POST /webhooks/psp` (public, HMAC-verified: `signature = hmac(order_id, PSP_SECRET, 'sha256')`); topic `payment.settled` `{order_id, amount}`; audit_log rows.

- [ ] **Step 1: Failing test** `tests/psp-webhook.test.json`: (a) valid signature (precompute: `echo -n '<order_id>' | openssl dgst -sha256 -hmac 'psp-dev-secret' | awk '{print $2}'` with order_id `o-1`; ensure the test runner loads `.env` so `PSP_SECRET` matches — if it doesn't load env, that's a finding; then mock `mark_paid` rows_affected 1 + `emit`) → expect the 200 node; (b) bad signature → expect the 401 node and NO `mark_paid` (assert via expect shape available; if unassertable, log dx finding). `noda test` → FAIL.

- [ ] **Step 2: `pay-order`:** route POST `/api/orders/:order_id/pay` (authenticated) → workflow: `order` (findOne required) → `payable` (control.if `nodes.order.winner_id == input.user_id && nodes.order.status == 'pending'`) → then `charge` (`http.post`, services `{client:"psp-client"}`, config `{url:"{{ $env('BASE_URL') }}/mock/psp/charge", body:{order_id:"{{ input.order_id }}", amount:"{{ nodes.order.amount }}", callback_url:"{{ $env('BASE_URL') }}/webhooks/psp"}, timeout:"10s"}`) → `respond` 202 `{status:"processing"}`; else 403; order error edge → 404.

- [ ] **Step 3: `mock-psp-charge`** (public POST `/mock/psp/charge`): `sign` (transform.set `{signature: "{{ hmac(input.order_id, $env('PSP_SECRET'), 'sha256') }}"}`) → `callback` (http.post `{url:"{{ input.callback_url }}", body:{order_id:"{{ input.order_id }}", amount:"{{ input.amount }}", status:"settled", signature:"{{ nodes.sign.signature }}"}}`) → `respond` 200 `{accepted:true}`. (Synchronous nested call chain pay→charge→webhook is deliberate — it stress-tests reentrant request handling; if it deadlocks, majorly important finding.)

- [ ] **Step 4: `psp-webhook`** (public POST `/webhooks/psp`): `verify` (control.if `input.signature == hmac(input.order_id, $env('PSP_SECRET'), 'sha256')`) → else `reject` (response.error 401 "bad signature"); then `mark_paid` (db.exec `UPDATE orders SET status='paid', paid_at=now() WHERE id=$1 AND status='pending'` params [order_id]) → `settled` (control.if rows_affected==1) → then `emit` (event.emit topic `payment.settled` `{order_id, amount}`) → `ok` (response.json 200 `{ok:true}`); idempotent-replay else-branch → `ok` too.

- [ ] **Step 5: `workers/audit.json`** — topic `payment.settled`, group `audit`, trigger `audit-handler` → single `db.create` audit_log `{id:$uuid(), event_type:"payment.settled", payload:{order_id:"{{ input.order_id }}", amount:"{{ input.amount }}"}}` → output. (JSONB from a map — if the db plugin can't write a map into JSONB, log finding; fallback: store as text.)

- [ ] **Step 6: Live end-to-end payment:** win an auction (Task 10 flow), pay → order becomes `paid`, audit_log row exists. Tamper test: `curl -d '{"order_id":"x","signature":"junk","amount":1}' /webhooks/psp` → 401.

- [ ] **Step 7: `noda test` + commit** `feat(auction-house): mock PSP payment + signed webhook + audit`.

---

### Task 12: Daily digest + admin trigger

**Files:**
- Create: `schedules/daily-digest.json`, `workflows/{digest,digest-user}.json`, `routes/run-digest.json`, `workflows/run-digest.json`, `tests/digest-user.test.json`

**Interfaces:**
- Consumes: watches, listings, mailer.
- Produces: `POST /api/admin/run-digest` (admin) → 202; cron `0 0 8 * * *` UTC.

- [ ] **Step 1: Failing test** `tests/digest-user.test.json`: input `{user_id:"u-1", email:"a@b.test"}`, mock `items` (db.query → two rows `{title, status, current_price}`), mock `mail` success → expect success. `noda test` → FAIL.

- [ ] **Step 2: `digest-user`:** `items` (`db.query`, config `{query:"SELECT l.title, l.status, l.current_price, l.ends_at FROM listings l JOIN watches w ON w.listing_id = l.id WHERE w.user_id = $1 AND (l.status = 'active' OR l.closed_at > now() - interval '1 day')", params:["{{ input.user_id }}"]}` — verify db.query supports `params` via its node doc) → `any` (control.if `len(nodes.items) > 0`) → then `lines` (transform.map over items → `"{{ $item.title }} [{{ $item.status }}] — {{ $item.current_price }}"` — check transform.map's item variable name in its doc) → `mail` (email.send to input.email, subject "Your Bidhub digest", body `{{ join(nodes.lines, "\n") }}`) → output `{sent:true}`; else output `{sent:false}`.

`digest`: `recipients` (db.query `SELECT DISTINCT u.id, u.email FROM users u JOIN watches w ON w.user_id = u.id WHERE u.suspended = false`) → `each` (control.loop → digest-user `{user_id:"{{ $item.id }}", email:"{{ $item.email }}"}`) → output `{count:"{{ len(nodes.recipients) }}"}`.

`schedules/daily-digest.json`: id `daily-digest`, cron `0 0 8 * * *`, timezone UTC, lock via app-cache, trigger `digest`.

`run-digest`: POST `/api/admin/run-digest` → workflow `run-digest`: `run` (workflow.run digest) → 202 `{count}`.

- [ ] **Step 3: Live:** watch a listing as bob, `POST /api/admin/run-digest` as admin → digest email in Mailpit.

- [ ] **Step 4: `noda test` + commit** `feat(auction-house): daily digest`.

---

### Task 13: WS/SSE smoke tool + full-lifecycle e2e script

**Files:**
- Create: `tests/smoke/{main.go, go.mod}`, `tests/e2e.sh`, `tests/lib.sh`

**Interfaces:**
- Consumes: every endpoint built so far.
- Produces: `tests/e2e.sh` — exits 0 only if the entire auction lifecycle works against `docker compose up` + `noda start`.

- [ ] **Step 1: Smoke tool** `tests/smoke/go.mod`: module `smoke`, go 1.25, `require github.com/gorilla/websocket v1.5.3`. `main.go` (complete):

```go
// smoke connects WS + SSE, fires one bid over HTTP, and requires both
// realtime frames to arrive. Exit 0 = both seen.
package main

import (
	"bufio"
	"bytes"
	"flag"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

func main() {
	base := flag.String("base", "http://localhost:8080", "API base URL")
	token := flag.String("token", "", "bearer token of the bidder")
	listing := flag.String("listing", "", "listing UUID")
	amount := flag.String("amount", "", "bid amount")
	flag.Parse()
	if *token == "" || *listing == "" || *amount == "" {
		fmt.Fprintln(os.Stderr, "usage: smoke -token T -listing L -amount N")
		os.Exit(2)
	}

	wsURL := strings.Replace(*base, "http", "ws", 1) + "/ws/auctions/" + *listing
	hdr := http.Header{"Authorization": {"Bearer " + *token}}
	ws, _, err := websocket.DefaultDialer.Dial(wsURL, hdr)
	if err != nil {
		fmt.Fprintln(os.Stderr, "WS dial:", err)
		os.Exit(1)
	}
	defer ws.Close()

	wsGot, sseGot := make(chan string, 1), make(chan string, 1)
	go func() {
		for {
			_, msg, err := ws.ReadMessage()
			if err != nil {
				return
			}
			if bytes.Contains(msg, []byte(*amount)) {
				wsGot <- string(msg)
				return
			}
		}
	}()
	go func() {
		resp, err := http.Get(*base + "/sse/ticker")
		if err != nil {
			return
		}
		defer resp.Body.Close()
		sc := bufio.NewScanner(resp.Body)
		for sc.Scan() {
			if strings.Contains(sc.Text(), *listing) && strings.Contains(sc.Text(), *amount) {
				sseGot <- sc.Text()
				return
			}
		}
	}()

	time.Sleep(500 * time.Millisecond) // let subscriptions settle
	req, _ := http.NewRequest("POST", *base+"/api/auctions/"+*listing+"/bids",
		strings.NewReader(`{"amount": `+*amount+`}`))
	req.Header.Set("Authorization", "Bearer "+*token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil || resp.StatusCode != 201 {
		fmt.Fprintln(os.Stderr, "bid failed:", err, resp)
		os.Exit(1)
	}

	deadline := time.After(10 * time.Second)
	for wsGot != nil || sseGot != nil {
		select {
		case m := <-wsGot:
			fmt.Println("WS ok:", m)
			wsGot = nil
		case m := <-sseGot:
			fmt.Println("SSE ok:", m)
			sseGot = nil
		case <-deadline:
			fmt.Fprintln(os.Stderr, "timeout: ws seen =", wsGot == nil, "sse seen =", sseGot == nil)
			os.Exit(1)
		}
	}
}
```

- [ ] **Step 2: `tests/lib.sh`** — helpers: `api()` (curl wrapper printing status+body, `-H "Authorization: Bearer $1"`), `login()` (register-or-login, echoes token), `wait_for()` (poll a command until true or N seconds), `mailpit_count()` (`curl -s localhost:8025/api/v1/search?query="$1" | jq .messages_count`), `sql()` (`docker compose exec -T postgres psql -U noda auction -tAc "$1"`).

- [ ] **Step 3: `tests/e2e.sh`** — `set -euo pipefail`, steps in order, each echoing PASS/FAIL:
1. `docker compose up -d --wait`; `noda migrate up`; start `noda start &` (record PID, trap cleanup); wait_for `GET /api/auctions` = 200.
2. Register+login alice, bob, carol; login admin (`admin@bidhub.local` / `admin-password-123`).
3. Alice creates listing (start 10, inc 1, `ends_at = now+90s` via `date -u -v+90S +%Y-%m-%dT%H:%M:%SZ`); uploads 1×1 PNG *and* a real ≥320px PNG (generate with python3/PIL if available, else committed fixture `tests/fixtures/photo.png`); activate. Assert thumb GET returns 200 and non-empty body.
4. Bob bids 10 via **smoke tool** (`go run ./tests/smoke -token $BOB -listing $L -amount 10`) — asserts WS + SSE delivery.
5. Carol sets proxy max 20; bob sets proxy max 15 → assert final leader carol at ≤16 (`sql "SELECT bidder_id FROM bids WHERE listing_id='$L' ORDER BY amount DESC LIMIT 1"`), bids strictly increasing, outbid email for bob in Mailpit.
6. Rate limit: fire 105 rapid bids of amount 1 (all would 422) → assert at least one 429.
7. Anti-snipe: wait until <60s remain, alice… (alice can't bid) — bob bids current+1 → assert `ends_at` moved (GET listing before/after).
8. Wait_for listing status `closed` (poll ≤130s, 10s cron + extension) → assert exactly one order, winner = top bidder, winner+seller emails in Mailpit.
9. Winner pays (`POST /api/orders/$ORDER/pay`) → wait_for order status `paid`; assert audit_log row; tampered webhook → 401.
10. Watch + digest: bob watches another listing; admin `POST /api/admin/run-digest` → digest email in Mailpit.
11. Poison event via admin route → wait_for `redis-cli XLEN` on the DLQ stream ≥ 1.
12. Casbin negative: bob calls an admin route → 403.
13. Print `ALL E2E PASSED`.

- [ ] **Step 4: Run it.** Iterate until green. Every surprise → FINDINGS.

- [ ] **Step 5: Commit** `test(auction-house): smoke tool + full-lifecycle e2e`.

---

### Task 14: Concurrency probes

**Files:**
- Create: `tests/probes/parallel-bids.sh`, `tests/probes/close-race.sh`

**Interfaces:**
- Consumes: running stack from Task 13 (`lib.sh` helpers).
- Produces: probe scripts exiting 0 iff DB invariants hold.

- [ ] **Step 1: `parallel-bids.sh`:** fresh listing (start 10, inc 1, ends +5m); two bidders' tokens; fire 30 bids **concurrently** at amounts 11..40 shuffled (`seq 11 40 | sort -R | xargs -P 30 -I{} curl -s -o /dev/null -w "%{http_code}\n" ... -d '{"amount": {}}'`). Then assert via `sql`:
  - `SELECT COUNT(*) FROM (SELECT amount FROM bids WHERE listing_id='$L' GROUP BY amount HAVING COUNT(*)>1) d` → **0** (no duplicate amounts);
  - `SELECT (SELECT MAX(amount) FROM bids WHERE listing_id='$L') = (SELECT current_price FROM listings WHERE id='$L')` → **t**;
  - `SELECT (SELECT COUNT(*) FROM bids WHERE listing_id='$L') = (SELECT bid_count FROM listings WHERE id='$L')` → **t**;
  - accepted bids (201 count) == bids rows.
  Also: sequence of accepted amounts sorted by created_at must be strictly increasing: `SELECT bool_and(ok) FROM (SELECT amount > lag(amount) OVER (ORDER BY created_at, amount) OR lag(amount) OVER (ORDER BY created_at, amount) IS NULL AS ok FROM bids WHERE listing_id='$L') s` → **t**. Any violation = the CAS isn't airtight → detailed FINDINGS entry with the violating rows.

- [ ] **Step 2: `close-race.sh`:** listing with `ends_at = now()+8s`, no anti-snipe interference (place first bid immediately so later ones don't extend—or accept extension and bound the loop): loop bids every 150ms for 25s while the close cron fires. Assert: `SELECT COUNT(*) FROM bids b JOIN listings l ON l.id=b.listing_id WHERE l.id='$L' AND b.created_at > l.closed_at` → **0** (no bid after close); exactly one order; order.amount == max bid. Note: anti-snipe extends `ends_at` on late bids, so the auction closes only after bids stop for 2 minutes — cap the wait accordingly or use increment large enough that late bids fail the guard. Design the timing so the probe terminates <3 min.

- [ ] **Step 3: Run both 5× in a row** (races are probabilistic): `for i in $(seq 5); do ./tests/probes/parallel-bids.sh || exit 1; done`. Log outcomes either way — a clean pass is also a result worth recording.

- [ ] **Step 4: Commit** `test(auction-house): concurrency probes`.

---

### Task 15: Findings triage, README, wrap-up

**Files:**
- Modify: `projects/auction-house/FINDINGS.md`, `projects/auction-house/README.md`
- Create: GitHub issues for confirmed bugs/gaps

**Interfaces:** none — this is the deliverable.

- [ ] **Step 1: Re-verify every FINDINGS entry** — reproduce each; reclassify user-error entries (keep them, marked `user-error`, with what would have prevented the confusion).

- [ ] **Step 2: Triage:** for each confirmed `bug`/`gap`: search existing issues (`gh issue list --search`), then `gh issue create` with repro steps, expected/actual, and `Found while dogfooding projects/auction-house (spec: docs/superpowers/specs/2026-07-03-auction-house-design.md)`. Mark each FINDINGS entry with its issue number or `→ doc-fix candidate`. Do NOT fix runtime bugs in this branch — the project must keep demonstrating them until fixed separately.

- [ ] **Step 3: README.md:** what it is (dogfooding project), quick start (compose up, migrate, dev, e2e.sh), the coverage matrix from the spec with ✅ per subsystem, link to FINDINGS.md.

- [ ] **Step 4: Final full gate:** `noda validate --verbose` && `noda test` && `tests/e2e.sh` && both probes — all green (or failing ONLY on documented, issue-linked findings; note exact status in README).

- [ ] **Step 5: Commit** `docs(auction-house): findings triage + README`, then follow superpowers:finishing-a-development-branch (PR to main).

---

## Self-Review Notes

- **Spec coverage:** accounts/roles→T3/T4; listings→T4; photos→T5; live bidding+rate limit→T7; anti-snipe→T7; watch/WS/SSE→T6; proxy Wasm→T9; closing scheduler→T10; notifications/DLQ→T8/T10; payment/webhook→T11; digest+admin→T12/T4; test runner→T3/4/7/10/11/12; e2e→T13; concurrency→T14; findings protocol→T1/T15; migrations→T2; dev mode→used throughout; expressions→everywhere. All spec sections have tasks.
- **Known uncertainties are flagged inline** (upload.handle output path field, `expect.outputs` test shape, `control.if` edge labels, `where_clause` shape, db.query params, transform.map item var, SSE channel patterns, workflow.output config field) — each has a named doc to consult and a validation gate; by the honest-build rules these consults are mandatory first steps, not optional.
- **Type consistency:** `place-bid-tx` input `{listing_id, user_id, amount, proxy}` and output `{accepted, reason?, amount?, ends_at?, extended?}` used identically in T7 (place-bid), T9 (apply-proxy-bid); `broadcast-bid` payload matches `notify-bid` worker input mapping (T8); `auction.closed` payload fields match `close-notify-handler` (T10); topics/channels/service names match Global Constraints everywhere.
