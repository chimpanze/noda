# Homebase — Foundation Design (Cycle 1: Auth + Drops Feed)

**Date:** 2026-07-07
**Status:** Approved design, pre-implementation
**Location:** `projects/homebase/`

## What Homebase is

A personal "private cloud" API built entirely on Noda config (no application
code). It replaces the owner's current workflow of pasting text, images, and
files into a single private Discord channel. Later cycles add screen
streaming and meetings (both LiveKit rooms); this cycle builds the foundation:
auth and the drops feed.

The full app, by cycle:

1. **Foundation (this spec):** project skeleton, single-admin auth, drops
   feed (text + files), share links, E2E suite.
2. **Streaming + meetings:** LiveKit rooms with different presets (screen
   share broadcast vs. multi-party meeting), guest join links. Adds Redis.

## Decisions already made

- **User model: owner only.** One admin account. Friends never log in; they
  access shared drops via secret links (and later join meetings/streams via
  guest tokens). Schema does not need to anticipate friend accounts.
- **Deployment: public VPS** running Docker Compose, Caddy for TLS.
- **Data model: one unified feed ("drops"),** not separate notes/files/todo
  modules. A drop is markdown text, a file, or both. Todo lists are just
  markdown checkboxes inside text drops; editing a drop's text is how a box
  gets ticked. Simple substring search only — explicitly *not* a knowledge
  base (no tags, folders, or full-text ranking).
- **Share links: revocable, optional expiry.** Long random token in the URL;
  owner can list and revoke; optional per-link expiry.
- **Auth: Noda auth plugin (Approach A).** Opaque, per-device, individually
  revocable sessions. Registration routes are not mounted at all. Chosen over
  an adventure-stream-style admin JWT (no revocation) and static API keys
  (no rotation story).

## Architecture

Pure Noda JSON config. Project layout:

```
projects/homebase/
  noda.json            services: main-db (Postgres), auth, files (storage → local disk)
  migrations/          auth tables + drops + share_links
  routes/              one JSON file per endpoint
  workflows/           one workflow per endpoint
  schemas/             request validation schemas
  tests/               workflow tests (noda test)
  e2e/                 Go E2E suite against the real compose stack
  docker-compose.yml   noda + postgres + caddy; named volume for file bytes
  .env.example
  Caddyfile
  README.md
```

Services:

- **`main-db` (db plugin, Postgres):** users/sessions (auth plugin's standard
  migration), `drops`, `share_links`.
- **`auth` (auth plugin):** bound to `main-db`.
- **`files` (storage plugin, local FS):** file bytes on a compose volume,
  keyed by server-generated UUID. Afero underneath, so S3 later is a config
  change.
- **No Redis this cycle.** The streaming/meetings cycle adds it.
- **Caddy** terminates TLS and redirects HTTP→HTTPS (same pattern as
  `projects/adventure-stream`).

### First-run bootstrap

`POST /setup`, guarded by a `SETUP_TOKEN` env var, creates the admin account
iff zero users exist; afterwards it always refuses. Fresh deploy is:
`docker compose up`, one curl to `/setup`, done. No registration route ever
exists.

## Data model

Beyond the auth plugin's standard tables:

**`drops`**

| column | type | notes |
|---|---|---|
| id | uuid pk | |
| text | text null | markdown; checkboxes/strikethrough live here |
| file_name | text null | original filename, metadata only |
| file_key | text null | server-generated UUID key in storage |
| file_size | bigint null | |
| file_mime | text null | |
| created_at / updated_at | timestamptz | feed ordering |

Check constraint: text or file present (`text IS NOT NULL OR file_key IS NOT
NULL`). File columns are set/cleared together.

**`share_links`**

| column | type | notes |
|---|---|---|
| id | uuid pk | |
| drop_id | uuid fk → drops ON DELETE CASCADE | |
| token | text unique | UUIDv4-derived random token (~122 bits entropy) |
| expires_at | timestamptz null | null = valid until revoked |
| created_at | timestamptz | |

Deliberate simplification: share tokens are stored plaintext. An attacker who
can read this Postgres already has the files; hashing adds ceremony without
protection here.

## API surface

All endpoints return the standard Noda error envelope on failure.

### Bootstrap & auth

| endpoint | auth | behavior |
|---|---|---|
| `POST /setup` | setup token in body | `{setup_token, email, password}`; creates admin iff zero users, else 403 |
| `POST /auth/login` | none | `{email, password}` → session token |
| `POST /auth/logout` | session | revokes the calling session |
| `GET /auth/me` | session | current user |
| `GET /auth/sessions` | session | list active sessions (devices) — plain `db.find` on the sessions table |
| `DELETE /auth/sessions/:id` | session | revoke one session (lost-laptop case) |

### Drops (session required)

| endpoint | behavior |
|---|---|
| `POST /drops` | JSON `{text}` for text-only, or multipart (`file` + optional `text` field) via `upload.handle` |
| `GET /drops?limit=50&before=<cursor>&q=<term>` | newest first, cursor pagination on created_at; `q` is ILIKE on text and file_name |
| `GET /drops/:id` | metadata + text |
| `GET /drops/:id/file` | file bytes; 404 if the drop has no file |
| `PATCH /drops/:id` | replace `text` (checkbox ticking) |
| `DELETE /drops/:id` | delete row (share links cascade), then file bytes |

### Sharing

| endpoint | auth | behavior |
|---|---|---|
| `POST /drops/:id/share` | session | optional `{expires_in: "168h"}` → `{url, token}` |
| `GET /shares` | session | active links with drop info |
| `DELETE /shares/:id` | session | revoke |
| `GET /s/:token` | none | drop text + file metadata |
| `GET /s/:token/file` | none | file bytes |

## Error handling

- Schema validation → 400; missing/invalid session → 401; unknown id → 404.
- Public `/s/*` routes return a **uniform 404** for unknown, expired, and
  revoked tokens — nothing to enumerate.
- `/setup` returns the **same 403 body** for a wrong setup token and for
  "already initialized" — no oracle for instance state.
- **Upload ordering:** write bytes to storage first, then insert the DB row;
  on insert failure, delete the just-written bytes.
- **Delete ordering:** DB row first (source of truth), then bytes. A failed
  byte-delete leaves a harmless orphan plus a log line, never a dangling row.

## Security

- `file_key` is a server-generated UUID; the user filename never appears in a
  path — metadata only, sanitized in `Content-Disposition`.
- Downloads: stored MIME + `X-Content-Type-Options: nosniff`.
- Rate limiting (middleware limiter, as in `examples/auth-demo`) on
  `/auth/login`, `/setup`, and `/s/*`.
- Body-size limit from env; default 1 GB.
- Password hashing, session verification, constant-time comparisons: all
  inside the hardened auth plugin — nothing re-implemented.
- Caddy: TLS, HTTP→HTTPS redirect.

## Testing

Two layers; the E2E run is the acceptance gate.

1. **Workflow tests** (`tests/`, `noda test`): per-workflow happy path plus
   interesting failures — validation errors, drop-not-found, expired share,
   second `/setup` refused. Fast, no containers.
2. **E2E suite** (`e2e/`, Go test behind a build tag, against the real
   `docker compose` stack): one full lifecycle —
   setup → login from two "machines" → post text drop → post file drop (real
   multipart) → list + search → PATCH to tick a checkbox → create share link
   → fetch it unauthenticated → 1-second-expiry link dies → revoke a link →
   404 → revoke machine 2's session, its token now rejected → delete drop →
   file bytes and share links verifiably gone.

Green E2E against the compose stack means the same compose file works on the
VPS.

## Out of scope (this cycle)

- Frontend of any kind.
- Friend accounts, tags, folders, full-text search, reminders.
- Streaming, meetings, LiveKit, Redis (next cycle).
- S3 storage (config-level switch later if wanted).
