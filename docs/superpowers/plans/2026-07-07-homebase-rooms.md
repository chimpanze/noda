# Homebase Rooms Implementation Plan (Cycle 2)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add LiveKit-backed meetings and screen streaming to `projects/homebase/`: room create/list/delete, owner tokens, reusable revocable guest links, public join endpoint — all config, E2E-tested against a dev-mode LiveKit container.

**Architecture:** Stateless rooms — LiveKit owns room lifecycle (`empty_timeout: 600`), room type is encoded in the name (`hb-meet-<slug>` / `hb-stream-<slug>`); Postgres gains one table (`room_links`) for guest links. New `lk` service in `noda.json` (plugin `livekit`). No Redis.

**Tech Stack:** Noda config (existing cycle-1 project), LiveKit Cloud in production / `livekit/livekit-server --dev` container in E2E, Postgres, Go stdlib for E2E additions.

**Spec:** `docs/superpowers/specs/2026-07-07-homebase-rooms-design.md`

## Global Constraints

- **Branch:** `homebase-rooms` (exists, off main; spec committed there). Config only — no Go code except the E2E test file.
- **Room names:** `hb-meet-<slug>` / `hb-stream-<slug>`, slug = first 8 chars of a dash-stripped UUIDv4. Path params validated against `^hb-(meet|stream)-[a-z0-9]{8}$` before any LiveKit call.
- **Presets:** meeting → `empty_timeout: 600`, `max_participants: 10`; stream → `empty_timeout: 600`, `max_participants: 50`.
- **Grants (exact):**
  - meeting owner AND meeting guest: `{"canPublish": true, "canSubscribe": true, "canPublishData": true}`; owner additionally `"roomAdmin": true`.
  - stream owner: `{"canPublish": true, "canPublishSources": ["SCREEN_SHARE", "SCREEN_SHARE_AUDIO", "MICROPHONE"], "canSubscribe": true, "canPublishData": true, "roomAdmin": true}` — **enum names MUST be uppercase**; `plugins/livekit/helpers.go:applyGrants` silently drops unknown strings, which would lock the owner out of publishing.
  - stream guest: `{"canPublish": false, "canSubscribe": true}`.
  - Owner token TTL `"12h"`; guest tokens use the node default (6h) — omit `ttl`.
  - Owner identity `owner`; guest identity `guest-<display>-<4 rand>`.
- **Uniform 404 on public `/j/:token`:** unknown/expired link, dead room, AND LiveKit-unreachable all return `404` / `NOT_FOUND` / `"not found"`, each via its OWN response node. Owner routes fail loudly on LiveKit errors (unwired error edges), except where a 404 is specified.
- **No multi-in-edge joins** (engine treats them as wait-all): every branch terminates in its own response node.
- **Workflow-test facts (from cycle 1):** `noda test` cannot evaluate `secrets.*` → mock every response node whose body references `secrets.LIVEKIT_URL`/`secrets.PUBLIC_BASE_URL`; mock all plugin nodes (`lk.*`, `db.*`) on paths that execute them; `control.if`/`transform.set` run for real.
- **Verification env prefix** (config load resolves `$env`, so every validate/test command needs):
  `DATABASE_URL='postgres://noda:noda@localhost:5432/noda?sslmode=disable' FILES_PATH=/tmp/homebase-files SETUP_TOKEN=test-setup-token PUBLIC_BASE_URL=http://localhost:3000 LIVEKIT_URL=ws://localhost:7880 LIVEKIT_API_KEY=devkey LIVEKIT_API_SECRET=secret`
- **Commits:** one per task, `feat(homebase): ...`, Claude co-author trailer.

---

### Task 1: `lk` service, env contract, migration, compose wiring

**Files:**
- Modify: `projects/homebase/noda.json`
- Modify: `projects/homebase/.env.example`
- Modify: `projects/homebase/docker-compose.yml`
- Create: `projects/homebase/migrations/20260707000003_room_links.up.sql`
- Create: `projects/homebase/migrations/20260707000003_room_links.down.sql`

**Interfaces:**
- Produces: service `lk` (plugin `livekit`); table `room_links(id, room_name, room_type, token, expires_at, created_at)`; env contract `LIVEKIT_URL`, `LIVEKIT_API_KEY`, `LIVEKIT_API_SECRET` required by compose (`:?`) and by config load.

- [ ] **Step 1: Add the `lk` service to `noda.json`**

In `projects/homebase/noda.json`, add to `"services"` (alongside `main-db`/`auth`/`files`):

```json
    "lk": {
      "plugin": "livekit",
      "config": {
        "url": "{{ $env('LIVEKIT_URL') }}",
        "api_key": "{{ $env('LIVEKIT_API_KEY') }}",
        "api_secret": "{{ $env('LIVEKIT_API_SECRET') }}"
      }
    }
```

- [ ] **Step 2: Extend `.env.example`**

Append to the `# --- required on the server ---` section:

```bash
# LiveKit Cloud credentials (https://cloud.livekit.io → project → Keys)
LIVEKIT_URL=wss://your-project.livekit.cloud
LIVEKIT_API_KEY=your-api-key
LIVEKIT_API_SECRET=your-api-secret
```

- [ ] **Step 3: Wire env through compose**

In `projects/homebase/docker-compose.yml`, add to the `noda` service `environment` block:

```yaml
      LIVEKIT_URL: ${LIVEKIT_URL:?set LIVEKIT_URL in .env}
      LIVEKIT_API_KEY: ${LIVEKIT_API_KEY:?set LIVEKIT_API_KEY in .env}
      LIVEKIT_API_SECRET: ${LIVEKIT_API_SECRET:?set LIVEKIT_API_SECRET in .env}
```

AND the same three lines to the `migrate` service `environment` block (`noda migrate` loads the full config, which now resolves these at load time).

- [ ] **Step 4: Write the migration**

`migrations/20260707000003_room_links.up.sql`:
```sql
CREATE TABLE room_links (
  id         TEXT PRIMARY KEY,
  room_name  TEXT NOT NULL,
  room_type  TEXT NOT NULL,
  token      TEXT NOT NULL UNIQUE,
  expires_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_room_links_room ON room_links(room_name);
```

`migrations/20260707000003_room_links.down.sql`:
```sql
DROP TABLE IF EXISTS room_links;
```

- [ ] **Step 5: Validate**

Run (repo root; note the LIVEKIT vars in the prefix):
```bash
DATABASE_URL='postgres://noda:noda@localhost:5432/noda?sslmode=disable' FILES_PATH=/tmp/homebase-files SETUP_TOKEN=test-setup-token PUBLIC_BASE_URL=http://localhost:3000 LIVEKIT_URL=ws://localhost:7880 LIVEKIT_API_KEY=devkey LIVEKIT_API_SECRET=secret go run ./cmd/noda validate --config projects/homebase
```
Expected: validation succeeds. Also run the workflow tests (same prefix, `test` instead of `validate`): all 42 cycle-1 tests still pass.

- [ ] **Step 6: Commit**

```bash
git add projects/homebase
git commit -m "feat(homebase): livekit service, room_links migration, env wiring

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 2: `POST /rooms` + `GET /rooms`

**Files:**
- Create: `projects/homebase/tests/test-room-create.json`, `tests/test-rooms-list.json`
- Create: `projects/homebase/workflows/rooms.create.json`, `workflows/rooms.list.json`
- Create: `projects/homebase/routes/rooms.create.json`, `routes/rooms.list.json`

**Interfaces:**
- Consumes: `lk` service, `room_links` table (Task 1); `auth.session` middleware (cycle 1).
- Produces: `POST /rooms {type, expires_in?}` → `201 {room, type, livekit_url, guest_token, guest_url, expires_at}`; `GET /rooms` → `200 {rooms: [...], links: [...]}`. Later tasks and E2E rely on these body keys and on room names shaped `hb-(meet|stream)-<slug8>`.

- [ ] **Step 1: Write the failing tests**

`tests/test-room-create.json`:
```json
{
  "id": "test-room-create",
  "workflow": "room-create",
  "tests": [
    {
      "name": "creates a meeting room with guest link",
      "input": { "type": "meeting", "expires_in": "" },
      "mocks": {
        "create": { "output": { "name": "hb-meet-a1b2c3d4", "sid": "RM_x", "num_participants": 0 } },
        "insert": { "output": { "id": "l1", "room_name": "hb-meet-a1b2c3d4", "room_type": "meeting", "token": "tok", "expires_at": null } },
        "respond": { "output": { "status": 201 } }
      },
      "expect": { "status": "success", "output": { "respond.status": 201 } }
    },
    {
      "name": "creates a stream room with expiring link",
      "input": { "type": "stream", "expires_in": "24h" },
      "mocks": {
        "create": { "output": { "name": "hb-stream-a1b2c3d4", "sid": "RM_y", "num_participants": 0 } },
        "insert": { "output": { "id": "l2", "room_name": "hb-stream-a1b2c3d4", "room_type": "stream", "token": "tok2", "expires_at": "2026-07-08T18:00:00Z" } },
        "respond": { "output": { "status": 201 } }
      },
      "expect": { "status": "success", "output": { "respond.status": 201 } }
    },
    {
      "name": "link insert failure deletes the just-created room",
      "input": { "type": "meeting", "expires_in": "" },
      "mocks": {
        "create": { "output": { "name": "hb-meet-a1b2c3d4" } },
        "insert": { "output_name": "error", "output": { "error": "db down" } },
        "cleanup": { "output": { "deleted": true } },
        "respond_insert_failed": { "output": { "status": 500 } }
      },
      "expect": { "status": "success", "output": { "respond_insert_failed.status": 500 } }
    }
  ]
}
```

`tests/test-rooms-list.json`:
```json
{
  "id": "test-rooms-list",
  "workflow": "rooms-list",
  "tests": [
    {
      "name": "lists hb rooms and active links",
      "input": {},
      "mocks": {
        "list": { "output": { "rooms": [ { "name": "hb-meet-a1b2c3d4", "num_participants": 2 }, { "name": "other-app-room", "num_participants": 1 } ] } },
        "links": { "output": [ { "id": "l1", "room_name": "hb-meet-a1b2c3d4", "room_type": "meeting", "token": "tok", "expires_at": null } ] },
        "respond": { "output": { "status": 200 } }
      },
      "expect": { "status": "success", "output": { "respond.status": 200 } }
    }
  ]
}
```

- [ ] **Step 2: Run to verify they fail**

```bash
DATABASE_URL='postgres://noda:noda@localhost:5432/noda?sslmode=disable' FILES_PATH=/tmp/homebase-files SETUP_TOKEN=test-setup-token PUBLIC_BASE_URL=http://localhost:3000 LIVEKIT_URL=ws://localhost:7880 LIVEKIT_API_KEY=devkey LIVEKIT_API_SECRET=secret go run ./cmd/noda test --config projects/homebase
```
Expected: FAIL (unknown workflows `room-create` / `rooms-list`).

- [ ] **Step 3: Write workflows**

`workflows/rooms.create.json` — room first, then link row; insert failure deletes the room (cycle-1 upload ordering, mirrored):
```json
{
  "id": "room-create",
  "name": "Rooms: Create with guest link",
  "nodes": {
    "gen": {
      "type": "transform.set",
      "config": {
        "fields": {
          "slug": "{{ replace($uuid(), '-', '')[0:8] }}",
          "link_token": "{{ replace($uuid(), '-', '') + replace($uuid(), '-', '') }}"
        }
      }
    },
    "namer": {
      "type": "transform.set",
      "config": {
        "fields": {
          "room_name": "{{ 'hb-' + (input.type == 'meeting' ? 'meet' : 'stream') + '-' + nodes.gen.slug }}"
        }
      }
    },
    "create": {
      "type": "lk.roomCreate",
      "services": { "livekit": "lk" },
      "config": {
        "name": "{{ nodes.namer.room_name }}",
        "empty_timeout": 600,
        "max_participants": "{{ input.type == 'meeting' ? 10 : 50 }}"
      }
    },
    "insert": {
      "type": "db.create",
      "services": { "database": "main-db" },
      "config": {
        "table": "room_links",
        "data": {
          "id": "{{ $uuid() }}",
          "room_name": "{{ nodes.namer.room_name }}",
          "room_type": "{{ input.type }}",
          "token": "{{ nodes.gen.link_token }}",
          "expires_at": "{{ input.expires_in != '' ? now() + duration(input.expires_in) : nil }}"
        }
      }
    },
    "cleanup": {
      "type": "lk.roomDelete",
      "services": { "livekit": "lk" },
      "config": { "room": "{{ nodes.namer.room_name }}" }
    },
    "respond": {
      "type": "response.json",
      "config": {
        "status": 201,
        "body": {
          "room": "{{ nodes.namer.room_name }}",
          "type": "{{ input.type }}",
          "livekit_url": "{{ secrets.LIVEKIT_URL }}",
          "guest_token": "{{ nodes.gen.link_token }}",
          "guest_url": "{{ secrets.PUBLIC_BASE_URL + '/j/' + nodes.gen.link_token }}",
          "expires_at": "{{ nodes.insert.expires_at }}"
        }
      }
    },
    "respond_insert_failed": {
      "type": "response.error",
      "config": { "status": 500, "code": "CREATE_FAILED", "message": "could not create room link" }
    },
    "respond_cleanup_failed": {
      "type": "response.error",
      "config": { "status": 500, "code": "CREATE_FAILED", "message": "could not create room link" }
    }
  },
  "edges": [
    { "from": "gen", "to": "namer" },
    { "from": "namer", "to": "create" },
    { "from": "create", "to": "insert" },
    { "from": "insert", "to": "respond" },
    { "from": "insert", "output": "error", "to": "cleanup" },
    { "from": "cleanup", "to": "respond_insert_failed" },
    { "from": "cleanup", "output": "error", "to": "respond_cleanup_failed" }
  ]
}
```

`workflows/rooms.list.json`:
```json
{
  "id": "rooms-list",
  "name": "Rooms: List active rooms and links",
  "nodes": {
    "list": {
      "type": "lk.roomList",
      "services": { "livekit": "lk" },
      "config": {}
    },
    "links": {
      "type": "db.find",
      "services": { "database": "main-db" },
      "config": {
        "table": "room_links",
        "select": ["id", "room_name", "room_type", "token", "expires_at", "created_at"],
        "where_clause": {
          "query": "expires_at IS NULL OR expires_at > now()",
          "params": []
        },
        "order": "created_at DESC"
      }
    },
    "respond": {
      "type": "response.json",
      "config": {
        "status": 200,
        "body": {
          "rooms": "{{ filter(nodes.list.rooms, {startsWith(.name, 'hb-')}) }}",
          "links": "{{ nodes.links }}"
        }
      }
    }
  },
  "edges": [
    { "from": "list", "to": "links" },
    { "from": "links", "to": "respond" }
  ]
}
```

- [ ] **Step 4: Write routes**

`routes/rooms.create.json`:
```json
{
  "id": "room-create",
  "method": "POST",
  "path": "/rooms",
  "summary": "Create a meeting or stream room with a guest link",
  "tags": ["rooms"],
  "middleware": ["auth.session"],
  "body": {
    "schema": {
      "type": "object",
      "required": ["type"],
      "properties": {
        "type": { "type": "string", "enum": ["meeting", "stream"] },
        "expires_in": { "type": "string", "pattern": "^[0-9]+(s|m|h)$" }
      }
    }
  },
  "trigger": {
    "workflow": "room-create",
    "input": {
      "type": "{{ body.type }}",
      "expires_in": "{{ (body ?? {}).expires_in ?? '' }}"
    }
  }
}
```

`routes/rooms.list.json`:
```json
{
  "id": "rooms-list",
  "method": "GET",
  "path": "/rooms",
  "summary": "List active rooms and guest links",
  "tags": ["rooms"],
  "middleware": ["auth.session"],
  "trigger": {
    "workflow": "rooms-list",
    "input": {}
  }
}
```

- [ ] **Step 5: Run tests to verify they pass** (same command as Step 2). Expected: PASS (42 prior + 4 new = 46).

- [ ] **Step 6: Commit**

```bash
git add projects/homebase
git commit -m "feat(homebase): room create with guest link + rooms list

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 3: `DELETE /rooms/:name` + `POST /rooms/:name/token`

**Files:**
- Create: `projects/homebase/tests/test-room-delete.json`, `tests/test-room-token.json`
- Create: `projects/homebase/workflows/rooms.delete.json`, `workflows/rooms.token.json`
- Create: `projects/homebase/routes/rooms.delete.json`, `routes/rooms.token.json`

**Interfaces:**
- Consumes: room-name shape from Task 2; grants from Global Constraints.
- Produces: `DELETE /rooms/:name` → `204`/`404`; `POST /rooms/:name/token` → `200 {livekit_url, token, meet_url}` / `404`. E2E decodes the returned JWT and expects `video.room == name`, `video.roomAdmin == true`.

- [ ] **Step 1: Write the failing tests**

`tests/test-room-delete.json`:
```json
{
  "id": "test-room-delete",
  "workflow": "room-delete",
  "tests": [
    {
      "name": "deletes a room and its links",
      "input": { "name": "hb-meet-a1b2c3d4" },
      "mocks": {
        "del": { "output": { "deleted": true } },
        "del_links": { "output": { "rows_affected": 1 } },
        "respond": { "output": { "status": 204 } }
      },
      "expect": { "status": "success", "output": { "respond.status": 204 } }
    },
    {
      "name": "unknown room gets 404",
      "input": { "name": "hb-meet-ffffffff" },
      "mocks": {
        "del": { "output_name": "error", "output": { "error": "room not found" } },
        "respond_missing": { "output": { "status": 404 } }
      },
      "expect": { "status": "success", "output": { "respond_missing.status": 404 } }
    },
    {
      "name": "malformed room name gets 404",
      "input": { "name": "not-a-room" },
      "mocks": {
        "respond_badname": { "output": { "status": 404 } }
      },
      "expect": { "status": "success", "output": { "respond_badname.status": 404 } }
    }
  ]
}
```

`tests/test-room-token.json`:
```json
{
  "id": "test-room-token",
  "workflow": "room-token",
  "tests": [
    {
      "name": "mints meeting owner token",
      "input": { "name": "hb-meet-a1b2c3d4" },
      "mocks": {
        "exists": { "output": { "rooms": [ { "name": "hb-meet-a1b2c3d4" } ] } },
        "token_meet": { "output": { "token": "jwt-meet", "identity": "owner", "room": "hb-meet-a1b2c3d4" } },
        "respond_meet": { "output": { "status": 200 } }
      },
      "expect": { "status": "success", "output": { "respond_meet.status": 200 } }
    },
    {
      "name": "mints stream owner token",
      "input": { "name": "hb-stream-a1b2c3d4" },
      "mocks": {
        "exists": { "output": { "rooms": [ { "name": "hb-stream-a1b2c3d4" } ] } },
        "token_stream": { "output": { "token": "jwt-stream", "identity": "owner", "room": "hb-stream-a1b2c3d4" } },
        "respond_stream": { "output": { "status": 200 } }
      },
      "expect": { "status": "success", "output": { "respond_stream.status": 200 } }
    },
    {
      "name": "dead room gets 404",
      "input": { "name": "hb-meet-ffffffff" },
      "mocks": {
        "exists": { "output": { "rooms": [] } },
        "respond_missing": { "output": { "status": 404 } }
      },
      "expect": { "status": "success", "output": { "respond_missing.status": 404 } }
    },
    {
      "name": "malformed room name gets 404",
      "input": { "name": "'; DROP TABLE rooms;--" },
      "mocks": {
        "respond_badname": { "output": { "status": 404 } }
      },
      "expect": { "status": "success", "output": { "respond_badname.status": 404 } }
    }
  ]
}
```

- [ ] **Step 2: Run to verify they fail** (standard command). Expected: FAIL (unknown workflows).

- [ ] **Step 3: Write workflows**

`workflows/rooms.delete.json`:
```json
{
  "id": "room-delete",
  "name": "Rooms: End room and remove links",
  "nodes": {
    "checkname": {
      "type": "control.if",
      "config": { "condition": "{{ matches(input.name, '^hb-(meet|stream)-[a-z0-9]{8}$') }}" }
    },
    "del": {
      "type": "lk.roomDelete",
      "services": { "livekit": "lk" },
      "config": { "room": "{{ input.name }}" }
    },
    "del_links": {
      "type": "db.delete",
      "services": { "database": "main-db" },
      "config": { "table": "room_links", "where": { "room_name": "{{ input.name }}" } }
    },
    "respond": {
      "type": "response.json",
      "config": { "status": 204 }
    },
    "respond_missing": {
      "type": "response.error",
      "config": { "status": 404, "code": "NOT_FOUND", "message": "room not found" }
    },
    "respond_badname": {
      "type": "response.error",
      "config": { "status": 404, "code": "NOT_FOUND", "message": "room not found" }
    }
  },
  "edges": [
    { "from": "checkname", "output": "then", "to": "del" },
    { "from": "checkname", "output": "else", "to": "respond_badname" },
    { "from": "del", "to": "del_links" },
    { "from": "del", "output": "error", "to": "respond_missing" },
    { "from": "del_links", "to": "respond" }
  ]
}
```

`workflows/rooms.token.json` — meeting/stream branch on the name prefix; two token nodes, two response nodes (no joins):
```json
{
  "id": "room-token",
  "name": "Rooms: Owner token",
  "nodes": {
    "checkname": {
      "type": "control.if",
      "config": { "condition": "{{ matches(input.name, '^hb-(meet|stream)-[a-z0-9]{8}$') }}" }
    },
    "exists": {
      "type": "lk.roomList",
      "services": { "livekit": "lk" },
      "config": { "names": ["{{ input.name }}"] }
    },
    "check_found": {
      "type": "control.if",
      "config": { "condition": "{{ len(nodes.exists.rooms) > 0 }}" }
    },
    "is_meet": {
      "type": "control.if",
      "config": { "condition": "{{ startsWith(input.name, 'hb-meet-') }}" }
    },
    "token_meet": {
      "type": "lk.token",
      "services": { "livekit": "lk" },
      "config": {
        "identity": "owner",
        "room": "{{ input.name }}",
        "name": "owner",
        "ttl": "12h",
        "grants": { "canPublish": true, "canSubscribe": true, "canPublishData": true, "roomAdmin": true }
      }
    },
    "token_stream": {
      "type": "lk.token",
      "services": { "livekit": "lk" },
      "config": {
        "identity": "owner",
        "room": "{{ input.name }}",
        "name": "owner",
        "ttl": "12h",
        "grants": { "canPublish": true, "canPublishSources": ["SCREEN_SHARE", "SCREEN_SHARE_AUDIO", "MICROPHONE"], "canSubscribe": true, "canPublishData": true, "roomAdmin": true }
      }
    },
    "respond_meet": {
      "type": "response.json",
      "config": {
        "status": 200,
        "body": {
          "livekit_url": "{{ secrets.LIVEKIT_URL }}",
          "token": "{{ nodes.token_meet.token }}",
          "meet_url": "{{ 'https://meet.livekit.io/custom?liveKitUrl=' + replace(replace(secrets.LIVEKIT_URL, ':', '%3A'), '/', '%2F') + '&token=' + nodes.token_meet.token }}"
        }
      }
    },
    "respond_stream": {
      "type": "response.json",
      "config": {
        "status": 200,
        "body": {
          "livekit_url": "{{ secrets.LIVEKIT_URL }}",
          "token": "{{ nodes.token_stream.token }}",
          "meet_url": "{{ 'https://meet.livekit.io/custom?liveKitUrl=' + replace(replace(secrets.LIVEKIT_URL, ':', '%3A'), '/', '%2F') + '&token=' + nodes.token_stream.token }}"
        }
      }
    },
    "respond_missing": {
      "type": "response.error",
      "config": { "status": 404, "code": "NOT_FOUND", "message": "room not found" }
    },
    "respond_badname": {
      "type": "response.error",
      "config": { "status": 404, "code": "NOT_FOUND", "message": "room not found" }
    }
  },
  "edges": [
    { "from": "checkname", "output": "then", "to": "exists" },
    { "from": "checkname", "output": "else", "to": "respond_badname" },
    { "from": "exists", "to": "check_found" },
    { "from": "check_found", "output": "then", "to": "is_meet" },
    { "from": "check_found", "output": "else", "to": "respond_missing" },
    { "from": "is_meet", "output": "then", "to": "token_meet" },
    { "from": "is_meet", "output": "else", "to": "token_stream" },
    { "from": "token_meet", "to": "respond_meet" },
    { "from": "token_stream", "to": "respond_stream" }
  ]
}
```

- [ ] **Step 4: Write routes**

`routes/rooms.delete.json`:
```json
{
  "id": "room-delete",
  "method": "DELETE",
  "path": "/rooms/:name",
  "summary": "End a room (disconnects everyone) and remove its guest links",
  "tags": ["rooms"],
  "middleware": ["auth.session"],
  "trigger": {
    "workflow": "room-delete",
    "input": { "name": "{{ params.name }}" }
  }
}
```

`routes/rooms.token.json`:
```json
{
  "id": "room-token",
  "method": "POST",
  "path": "/rooms/:name/token",
  "summary": "Mint the owner's LiveKit token for a room",
  "tags": ["rooms"],
  "middleware": ["auth.session"],
  "trigger": {
    "workflow": "room-token",
    "input": { "name": "{{ params.name }}" }
  }
}
```

- [ ] **Step 5: Run tests to verify they pass.** Expected: PASS (46 prior + 7 new = 53).

- [ ] **Step 6: Commit**

```bash
git add projects/homebase
git commit -m "feat(homebase): room delete and owner token endpoints

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 4: Guest link rotate + revoke

**Files:**
- Create: `projects/homebase/tests/test-room-link-rotate.json`, `tests/test-room-link-revoke.json`
- Create: `projects/homebase/workflows/rooms.link-rotate.json`, `workflows/rooms.link-revoke.json`
- Create: `projects/homebase/routes/rooms.link-rotate.json`, `routes/rooms.link-revoke.json`

**Interfaces:**
- Consumes: room-name validation pattern, token format, `room_links` table.
- Produces: `POST /rooms/:name/link {expires_in?}` → `201 {guest_token, guest_url, expires_at}` / `404`; `DELETE /rooms/:name/link` → `204` (idempotent) / `404` on malformed name. Rotation invalidates all previous links for the room (delete-then-insert).

- [ ] **Step 1: Write the failing tests**

`tests/test-room-link-rotate.json`:
```json
{
  "id": "test-room-link-rotate",
  "workflow": "room-link-rotate",
  "tests": [
    {
      "name": "rotates the guest link",
      "input": { "name": "hb-meet-a1b2c3d4", "expires_in": "" },
      "mocks": {
        "exists": { "output": { "rooms": [ { "name": "hb-meet-a1b2c3d4" } ] } },
        "del_old": { "output": { "rows_affected": 1 } },
        "insert": { "output": { "id": "l2", "room_name": "hb-meet-a1b2c3d4", "room_type": "meeting", "token": "newtok", "expires_at": null } },
        "respond": { "output": { "status": 201 } }
      },
      "expect": { "status": "success", "output": { "respond.status": 201 } }
    },
    {
      "name": "dead room gets 404",
      "input": { "name": "hb-meet-ffffffff", "expires_in": "" },
      "mocks": {
        "exists": { "output": { "rooms": [] } },
        "respond_missing": { "output": { "status": 404 } }
      },
      "expect": { "status": "success", "output": { "respond_missing.status": 404 } }
    },
    {
      "name": "malformed name gets 404",
      "input": { "name": "zzz", "expires_in": "" },
      "mocks": {
        "respond_badname": { "output": { "status": 404 } }
      },
      "expect": { "status": "success", "output": { "respond_badname.status": 404 } }
    }
  ]
}
```

`tests/test-room-link-revoke.json`:
```json
{
  "id": "test-room-link-revoke",
  "workflow": "room-link-revoke",
  "tests": [
    {
      "name": "revokes existing links",
      "input": { "name": "hb-stream-a1b2c3d4" },
      "mocks": {
        "del": { "output": { "rows_affected": 1 } },
        "respond": { "output": { "status": 204 } }
      },
      "expect": { "status": "success", "output": { "respond.status": 204 } }
    },
    {
      "name": "idempotent when no link exists",
      "input": { "name": "hb-stream-a1b2c3d4" },
      "mocks": {
        "del": { "output": { "rows_affected": 0 } },
        "respond": { "output": { "status": 204 } }
      },
      "expect": { "status": "success", "output": { "respond.status": 204 } }
    },
    {
      "name": "malformed name gets 404",
      "input": { "name": "zzz" },
      "mocks": {
        "respond_badname": { "output": { "status": 404 } }
      },
      "expect": { "status": "success", "output": { "respond_badname.status": 404 } }
    }
  ]
}
```

- [ ] **Step 2: Run to verify they fail.** Expected: FAIL (unknown workflows).

- [ ] **Step 3: Write workflows**

`workflows/rooms.link-rotate.json` — room must exist; delete-then-insert so old links die atomically enough for one owner:
```json
{
  "id": "room-link-rotate",
  "name": "Rooms: Rotate guest link",
  "nodes": {
    "checkname": {
      "type": "control.if",
      "config": { "condition": "{{ matches(input.name, '^hb-(meet|stream)-[a-z0-9]{8}$') }}" }
    },
    "exists": {
      "type": "lk.roomList",
      "services": { "livekit": "lk" },
      "config": { "names": ["{{ input.name }}"] }
    },
    "check_found": {
      "type": "control.if",
      "config": { "condition": "{{ len(nodes.exists.rooms) > 0 }}" }
    },
    "del_old": {
      "type": "db.delete",
      "services": { "database": "main-db" },
      "config": { "table": "room_links", "where": { "room_name": "{{ input.name }}" } }
    },
    "gen": {
      "type": "transform.set",
      "config": {
        "fields": {
          "link_token": "{{ replace($uuid(), '-', '') + replace($uuid(), '-', '') }}"
        }
      }
    },
    "insert": {
      "type": "db.create",
      "services": { "database": "main-db" },
      "config": {
        "table": "room_links",
        "data": {
          "id": "{{ $uuid() }}",
          "room_name": "{{ input.name }}",
          "room_type": "{{ startsWith(input.name, 'hb-meet-') ? 'meeting' : 'stream' }}",
          "token": "{{ nodes.gen.link_token }}",
          "expires_at": "{{ input.expires_in != '' ? now() + duration(input.expires_in) : nil }}"
        }
      }
    },
    "respond": {
      "type": "response.json",
      "config": {
        "status": 201,
        "body": {
          "guest_token": "{{ nodes.gen.link_token }}",
          "guest_url": "{{ secrets.PUBLIC_BASE_URL + '/j/' + nodes.gen.link_token }}",
          "expires_at": "{{ nodes.insert.expires_at }}"
        }
      }
    },
    "respond_missing": {
      "type": "response.error",
      "config": { "status": 404, "code": "NOT_FOUND", "message": "room not found" }
    },
    "respond_badname": {
      "type": "response.error",
      "config": { "status": 404, "code": "NOT_FOUND", "message": "room not found" }
    }
  },
  "edges": [
    { "from": "checkname", "output": "then", "to": "exists" },
    { "from": "checkname", "output": "else", "to": "respond_badname" },
    { "from": "exists", "to": "check_found" },
    { "from": "check_found", "output": "then", "to": "del_old" },
    { "from": "check_found", "output": "else", "to": "respond_missing" },
    { "from": "del_old", "to": "gen" },
    { "from": "gen", "to": "insert" },
    { "from": "insert", "to": "respond" }
  ]
}
```

`workflows/rooms.link-revoke.json`:
```json
{
  "id": "room-link-revoke",
  "name": "Rooms: Revoke guest link",
  "nodes": {
    "checkname": {
      "type": "control.if",
      "config": { "condition": "{{ matches(input.name, '^hb-(meet|stream)-[a-z0-9]{8}$') }}" }
    },
    "del": {
      "type": "db.delete",
      "services": { "database": "main-db" },
      "config": { "table": "room_links", "where": { "room_name": "{{ input.name }}" } }
    },
    "respond": {
      "type": "response.json",
      "config": { "status": 204 }
    },
    "respond_badname": {
      "type": "response.error",
      "config": { "status": 404, "code": "NOT_FOUND", "message": "room not found" }
    }
  },
  "edges": [
    { "from": "checkname", "output": "then", "to": "del" },
    { "from": "checkname", "output": "else", "to": "respond_badname" },
    { "from": "del", "to": "respond" }
  ]
}
```

- [ ] **Step 4: Write routes**

`routes/rooms.link-rotate.json`:
```json
{
  "id": "room-link-rotate",
  "method": "POST",
  "path": "/rooms/:name/link",
  "summary": "Rotate the room's guest link (invalidates the old one)",
  "tags": ["rooms"],
  "middleware": ["auth.session"],
  "body": {
    "schema": {
      "type": ["object", "null"],
      "properties": {
        "expires_in": { "type": "string", "pattern": "^[0-9]+(s|m|h)$" }
      }
    }
  },
  "trigger": {
    "workflow": "room-link-rotate",
    "input": {
      "name": "{{ params.name }}",
      "expires_in": "{{ (body ?? {}).expires_in ?? '' }}"
    }
  }
}
```

(The `["object", "null"]` type mirrors the cycle-1 fix on `shares.create.json`: a no-body POST parses as null and must pass an all-optional schema.)

`routes/rooms.link-revoke.json`:
```json
{
  "id": "room-link-revoke",
  "method": "DELETE",
  "path": "/rooms/:name/link",
  "summary": "Revoke the room's guest link (room stays up)",
  "tags": ["rooms"],
  "middleware": ["auth.session"],
  "trigger": {
    "workflow": "room-link-revoke",
    "input": { "name": "{{ params.name }}" }
  }
}
```

- [ ] **Step 5: Run tests to verify they pass.** Expected: PASS (53 prior + 6 new = 59).

- [ ] **Step 6: Commit**

```bash
git add projects/homebase
git commit -m "feat(homebase): guest link rotate and revoke

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 5: Public join — `GET /j/:token`

**Files:**
- Create: `projects/homebase/tests/test-room-join.json`
- Create: `projects/homebase/workflows/rooms.join.json`
- Create: `projects/homebase/routes/rooms.join.json`

**Interfaces:**
- Consumes: `room_links` rows (Task 2/4), guest grants (Global Constraints).
- Produces: `GET /j/:token?name=Alice` → `200 {livekit_url, token, meet_url, room, type}`; ALL failures → identical `404 NOT_FOUND "not found"`. E2E decodes the JWT: `video.room` matches, meeting guest has `canPublish: true`, stream guest has `canPublish: false`.

- [ ] **Step 1: Write the failing test** — `tests/test-room-join.json`:

```json
{
  "id": "test-room-join",
  "workflow": "room-join",
  "tests": [
    {
      "name": "meeting guest gets full-publish token",
      "input": { "token": "tok", "name": "Alice" },
      "mocks": {
        "find": { "output": { "id": "l1", "room_name": "hb-meet-a1b2c3d4", "room_type": "meeting" } },
        "exists": { "output": { "rooms": [ { "name": "hb-meet-a1b2c3d4" } ] } },
        "token_meet": { "output": { "token": "jwt-guest-meet", "identity": "guest-Alice-ab12", "room": "hb-meet-a1b2c3d4" } },
        "respond_meet": { "output": { "status": 200 } }
      },
      "expect": { "status": "success", "output": { "respond_meet.status": 200 } }
    },
    {
      "name": "stream guest gets subscribe-only token",
      "input": { "token": "tok2", "name": "" },
      "mocks": {
        "find": { "output": { "id": "l2", "room_name": "hb-stream-a1b2c3d4", "room_type": "stream" } },
        "exists": { "output": { "rooms": [ { "name": "hb-stream-a1b2c3d4" } ] } },
        "token_stream": { "output": { "token": "jwt-guest-stream", "identity": "guest-guest-ab12", "room": "hb-stream-a1b2c3d4" } },
        "respond_stream": { "output": { "status": 200 } }
      },
      "expect": { "status": "success", "output": { "respond_stream.status": 200 } }
    },
    {
      "name": "unknown or expired link gets uniform 404",
      "input": { "token": "nope", "name": "" },
      "mocks": {
        "find": { "output": null },
        "respond_no_link": { "output": { "status": 404 } }
      },
      "expect": { "status": "success", "output": { "respond_no_link.status": 404 } }
    },
    {
      "name": "dead room gets uniform 404",
      "input": { "token": "tok", "name": "" },
      "mocks": {
        "find": { "output": { "id": "l1", "room_name": "hb-meet-a1b2c3d4", "room_type": "meeting" } },
        "exists": { "output": { "rooms": [] } },
        "respond_room_gone": { "output": { "status": 404 } }
      },
      "expect": { "status": "success", "output": { "respond_room_gone.status": 404 } }
    },
    {
      "name": "livekit unreachable gets uniform 404",
      "input": { "token": "tok", "name": "" },
      "mocks": {
        "find": { "output": { "id": "l1", "room_name": "hb-meet-a1b2c3d4", "room_type": "meeting" } },
        "exists": { "output_name": "error", "output": { "error": "connection refused" } },
        "respond_lk_down": { "output": { "status": 404 } }
      },
      "expect": { "status": "success", "output": { "respond_lk_down.status": 404 } }
    }
  ]
}
```

- [ ] **Step 2: Run to verify it fails.** Expected: FAIL (unknown workflow `room-join`).

- [ ] **Step 3: Write `workflows/rooms.join.json`**

Display name: kept only if it matches a conservative charset, else `guest` — this both sanitizes and length-caps in one expression.

```json
{
  "id": "room-join",
  "name": "Public: Join room via guest link",
  "nodes": {
    "find": {
      "type": "db.findOne",
      "services": { "database": "main-db" },
      "config": {
        "table": "room_links",
        "select": ["id", "room_name", "room_type"],
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
    "exists": {
      "type": "lk.roomList",
      "services": { "livekit": "lk" },
      "config": { "names": ["{{ nodes.find.room_name }}"] }
    },
    "check_room": {
      "type": "control.if",
      "config": { "condition": "{{ len(nodes.exists.rooms) > 0 }}" }
    },
    "namegen": {
      "type": "transform.set",
      "config": {
        "fields": {
          "display": "{{ matches(trim(input.name), '^[A-Za-z0-9 _.-]{1,32}$') ? trim(input.name) : 'guest' }}",
          "suffix": "{{ replace($uuid(), '-', '')[0:4] }}"
        }
      }
    },
    "is_meeting": {
      "type": "control.if",
      "config": { "condition": "{{ nodes.find.room_type == 'meeting' }}" }
    },
    "token_meet": {
      "type": "lk.token",
      "services": { "livekit": "lk" },
      "config": {
        "identity": "{{ 'guest-' + nodes.namegen.display + '-' + nodes.namegen.suffix }}",
        "room": "{{ nodes.find.room_name }}",
        "name": "{{ nodes.namegen.display }}",
        "grants": { "canPublish": true, "canSubscribe": true, "canPublishData": true }
      }
    },
    "token_stream": {
      "type": "lk.token",
      "services": { "livekit": "lk" },
      "config": {
        "identity": "{{ 'guest-' + nodes.namegen.display + '-' + nodes.namegen.suffix }}",
        "room": "{{ nodes.find.room_name }}",
        "name": "{{ nodes.namegen.display }}",
        "grants": { "canPublish": false, "canSubscribe": true }
      }
    },
    "respond_meet": {
      "type": "response.json",
      "config": {
        "status": 200,
        "body": {
          "livekit_url": "{{ secrets.LIVEKIT_URL }}",
          "token": "{{ nodes.token_meet.token }}",
          "meet_url": "{{ 'https://meet.livekit.io/custom?liveKitUrl=' + replace(replace(secrets.LIVEKIT_URL, ':', '%3A'), '/', '%2F') + '&token=' + nodes.token_meet.token }}",
          "room": "{{ nodes.find.room_name }}",
          "type": "meeting"
        }
      }
    },
    "respond_stream": {
      "type": "response.json",
      "config": {
        "status": 200,
        "body": {
          "livekit_url": "{{ secrets.LIVEKIT_URL }}",
          "token": "{{ nodes.token_stream.token }}",
          "meet_url": "{{ 'https://meet.livekit.io/custom?liveKitUrl=' + replace(replace(secrets.LIVEKIT_URL, ':', '%3A'), '/', '%2F') + '&token=' + nodes.token_stream.token }}",
          "room": "{{ nodes.find.room_name }}",
          "type": "stream"
        }
      }
    },
    "respond_no_link": {
      "type": "response.error",
      "config": { "status": 404, "code": "NOT_FOUND", "message": "not found" }
    },
    "respond_room_gone": {
      "type": "response.error",
      "config": { "status": 404, "code": "NOT_FOUND", "message": "not found" }
    },
    "respond_lk_down": {
      "type": "response.error",
      "config": { "status": 404, "code": "NOT_FOUND", "message": "not found" }
    }
  },
  "edges": [
    { "from": "find", "to": "check" },
    { "from": "check", "output": "then", "to": "exists" },
    { "from": "check", "output": "else", "to": "respond_no_link" },
    { "from": "exists", "to": "check_room" },
    { "from": "exists", "output": "error", "to": "respond_lk_down" },
    { "from": "check_room", "output": "then", "to": "namegen" },
    { "from": "check_room", "output": "else", "to": "respond_room_gone" },
    { "from": "namegen", "to": "is_meeting" },
    { "from": "is_meeting", "output": "then", "to": "token_meet" },
    { "from": "is_meeting", "output": "else", "to": "token_stream" },
    { "from": "token_meet", "to": "respond_meet" },
    { "from": "token_stream", "to": "respond_stream" }
  ]
}
```

- [ ] **Step 4: Write `routes/rooms.join.json`**

```json
{
  "id": "room-join",
  "method": "GET",
  "path": "/j/:token",
  "summary": "Join a room via guest link (no auth)",
  "tags": ["public"],
  "middleware": ["limiter", "security.headers"],
  "trigger": {
    "workflow": "room-join",
    "input": {
      "token": "{{ params.token }}",
      "name": "{{ query.name ?? '' }}"
    }
  }
}
```

- [ ] **Step 5: Run tests to verify they pass.** Expected: PASS (59 prior + 5 new = 64).

- [ ] **Step 6: Commit**

```bash
git add projects/homebase
git commit -m "feat(homebase): public guest join endpoint with uniform 404s

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 6: E2E against dev-mode LiveKit + README

**Files:**
- Create: `projects/homebase/e2e/docker-compose.e2e.yml`
- Modify: `projects/homebase/e2e/run.sh`
- Modify: `projects/homebase/e2e/e2e_test.go` (append a new test function + one import)
- Modify: `projects/homebase/README.md`

**Interfaces:**
- Consumes: every endpoint from Tasks 2–5; cycle-1 e2e helpers (`client`, `login`, `decode`, `wantStatus`, `drainAndClose`) — all in the same package.
- Produces: the cycle-2 acceptance gate — `./projects/homebase/e2e/run.sh` green including `TestRoomsLifecycle`.

**Notes for the implementer:**
- The dev LiveKit server uses its built-in dev credentials `devkey` / `secret` (`--dev`); the vendored `livekit/protocol` enforces no minimum secret length for token signing, so these work end-to-end.
- Compose interpolates `${LIVEKIT_*:?}` in the base file at parse time, so `run.sh` exports dev defaults BEFORE any compose call.

- [ ] **Step 1: Write `e2e/docker-compose.e2e.yml`**

```yaml
# E2E-only override: adds a dev-mode LiveKit server and points noda at it.
# Used exclusively by run.sh (never in production).
services:
  livekit:
    image: livekit/livekit-server:v1.9.0
    command: ["--dev", "--bind", "0.0.0.0"]

  noda:
    depends_on:
      livekit:
        condition: service_started
```

(If the `v1.9.0` tag cannot be pulled, use `livekit/livekit-server:latest` and say so in your report.)

- [ ] **Step 2: Update `e2e/run.sh`**

Replace the whole file with:

```bash
#!/usr/bin/env bash
# Homebase E2E: boots the compose stack from scratch (incl. a dev-mode
# LiveKit server), runs the Go suite, tears down.
# Usage: ./projects/homebase/e2e/run.sh   (from anywhere)
set -euo pipefail
cd "$(dirname "$0")/.."

export SETUP_TOKEN="${SETUP_TOKEN:-e2e-setup-token}"
export PUBLIC_BASE_URL="${PUBLIC_BASE_URL:-http://localhost:3000}"
# Dev-mode LiveKit credentials (the base compose file requires these vars).
export LIVEKIT_URL="${LIVEKIT_URL:-ws://livekit:7880}"
export LIVEKIT_API_KEY="${LIVEKIT_API_KEY:-devkey}"
export LIVEKIT_API_SECRET="${LIVEKIT_API_SECRET:-secret}"

COMPOSE="docker compose -f docker-compose.yml -f e2e/docker-compose.e2e.yml"

$COMPOSE down -v --remove-orphans 2>/dev/null || true
$COMPOSE up -d --build
trap '$COMPOSE down -v --remove-orphans' EXIT

echo "waiting for noda ..."
for _ in $(seq 1 60); do
  if curl -fso /dev/null http://localhost:3000/health/ready; then
    break
  fi
  sleep 1
done

(cd ../.. && SETUP_TOKEN="$SETUP_TOKEN" go test -tags e2e -count=1 -v ./projects/homebase/e2e/)
```

- [ ] **Step 3: Append the rooms lifecycle to `e2e/e2e_test.go`**

Add `"encoding/base64"` to the import block. Then append at the end of the file:

```go
// videoGrant decodes a LiveKit JWT's payload and returns its "video" claim.
func videoGrant(t *testing.T, jwt string) map[string]any {
	t.Helper()
	parts := strings.Split(jwt, ".")
	if len(parts) != 3 {
		t.Fatalf("not a JWT: %q", jwt)
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatalf("decode JWT payload: %v", err)
	}
	var claims map[string]any
	if err := json.Unmarshal(payload, &claims); err != nil {
		t.Fatalf("unmarshal JWT payload: %v", err)
	}
	video, _ := claims["video"].(map[string]any)
	if video == nil {
		t.Fatalf("JWT has no video grant: %v", claims)
	}
	return video
}

// TestRoomsLifecycle runs after TestHomebaseLifecycle (same stack, admin
// already exists) and walks the meetings/streaming API against the
// dev-mode LiveKit container.
func TestRoomsLifecycle(t *testing.T) {
	anon := &client{t: t}
	owner := login(t)

	t.Run("unauthenticated room create is 401", func(t *testing.T) {
		resp := anon.doJSON("POST", "/rooms", map[string]string{"type": "meeting"})
		wantStatus(t, resp, 401)
		drainAndClose(resp)
	})

	var meetRoom, meetGuestToken, meetLinkPath string
	t.Run("create a meeting room", func(t *testing.T) {
		resp := owner.doJSON("POST", "/rooms", map[string]string{"type": "meeting"})
		wantStatus(t, resp, 201)
		body := decode(t, resp)
		meetRoom, _ = body["room"].(string)
		meetGuestToken, _ = body["guest_token"].(string)
		if !strings.HasPrefix(meetRoom, "hb-meet-") {
			t.Fatalf("room = %q, want hb-meet-*", meetRoom)
		}
		if len(meetGuestToken) != 64 {
			t.Fatalf("guest_token length = %d, want 64", len(meetGuestToken))
		}
		if body["livekit_url"] == "" || body["type"] != "meeting" {
			t.Fatalf("bad create body: %v", body)
		}
		meetLinkPath = "/j/" + meetGuestToken
	})

	t.Run("rooms list shows the room and its link", func(t *testing.T) {
		resp := owner.do("GET", "/rooms", nil, "")
		wantStatus(t, resp, 200)
		body := decode(t, resp)
		rooms, _ := body["rooms"].([]any)
		foundRoom := false
		for _, r := range rooms {
			m, _ := r.(map[string]any)
			if m["name"] == meetRoom {
				foundRoom = true
			}
		}
		if !foundRoom {
			t.Fatalf("room %s not in list", meetRoom)
		}
		links, _ := body["links"].([]any)
		foundLink := false
		for _, l := range links {
			m, _ := l.(map[string]any)
			if m["room_name"] == meetRoom && m["token"] == meetGuestToken {
				foundLink = true
			}
		}
		if !foundLink {
			t.Fatalf("guest link for %s not in list", meetRoom)
		}
	})

	t.Run("guest joins the meeting via link", func(t *testing.T) {
		resp := anon.do("GET", meetLinkPath+"?name=Alice", nil, "")
		wantStatus(t, resp, 200)
		body := decode(t, resp)
		if body["type"] != "meeting" || body["room"] != meetRoom {
			t.Fatalf("join body: %v", body)
		}
		grant := videoGrant(t, body["token"].(string))
		if grant["room"] != meetRoom {
			t.Fatalf("grant.room = %v", grant["room"])
		}
		if grant["canPublish"] != true {
			t.Fatalf("meeting guest canPublish = %v, want true", grant["canPublish"])
		}
	})

	t.Run("owner token carries roomAdmin", func(t *testing.T) {
		resp := owner.do("POST", "/rooms/"+meetRoom+"/token", nil, "")
		wantStatus(t, resp, 200)
		body := decode(t, resp)
		grant := videoGrant(t, body["token"].(string))
		if grant["roomAdmin"] != true || grant["room"] != meetRoom {
			t.Fatalf("owner grant: %v", grant)
		}
	})

	t.Run("rotate link kills the old token", func(t *testing.T) {
		resp := owner.doJSON("POST", "/rooms/"+meetRoom+"/link", nil)
		wantStatus(t, resp, 201)
		body := decode(t, resp)
		newTok, _ := body["guest_token"].(string)
		if newTok == "" || newTok == meetGuestToken {
			t.Fatalf("rotate returned %q", newTok)
		}

		resp = anon.do("GET", meetLinkPath, nil, "")
		wantStatus(t, resp, 404)
		drainAndClose(resp)

		resp = anon.do("GET", "/j/"+newTok, nil, "")
		wantStatus(t, resp, 200)
		drainAndClose(resp)
		meetGuestToken = newTok
		meetLinkPath = "/j/" + newTok
	})

	t.Run("revoke link stops new joins", func(t *testing.T) {
		resp := owner.do("DELETE", "/rooms/"+meetRoom+"/link", nil, "")
		wantStatus(t, resp, 204)
		drainAndClose(resp)
		resp = anon.do("GET", meetLinkPath, nil, "")
		wantStatus(t, resp, 404)
		drainAndClose(resp)
	})

	var streamRoom string
	t.Run("stream guests are subscribe-only", func(t *testing.T) {
		resp := owner.doJSON("POST", "/rooms", map[string]string{"type": "stream"})
		wantStatus(t, resp, 201)
		body := decode(t, resp)
		streamRoom, _ = body["room"].(string)
		tok, _ := body["guest_token"].(string)
		if !strings.HasPrefix(streamRoom, "hb-stream-") {
			t.Fatalf("room = %q", streamRoom)
		}

		resp = anon.do("GET", "/j/"+tok+"?name=Bob", nil, "")
		wantStatus(t, resp, 200)
		joinBody := decode(t, resp)
		if joinBody["type"] != "stream" {
			t.Fatalf("type = %v", joinBody["type"])
		}
		grant := videoGrant(t, joinBody["token"].(string))
		if grant["canPublish"] != false {
			t.Fatalf("stream guest canPublish = %v, want false", grant["canPublish"])
		}
	})

	t.Run("expiring room link dies", func(t *testing.T) {
		resp := owner.doJSON("POST", "/rooms/"+streamRoom+"/link", map[string]string{"expires_in": "1s"})
		wantStatus(t, resp, 201)
		body := decode(t, resp)
		tok, _ := body["guest_token"].(string)
		time.Sleep(1500 * time.Millisecond)
		resp = anon.do("GET", "/j/"+tok, nil, "")
		wantStatus(t, resp, 404)
		drainAndClose(resp)
	})

	t.Run("unknown join token is the same 404", func(t *testing.T) {
		resp := anon.do("GET", "/j/"+strings.Repeat("e", 64), nil, "")
		wantStatus(t, resp, 404)
		drainAndClose(resp)
	})

	t.Run("delete room removes it and its links", func(t *testing.T) {
		resp := owner.doJSON("POST", "/rooms/"+streamRoom+"/link", nil)
		wantStatus(t, resp, 201)
		body := decode(t, resp)
		tok, _ := body["guest_token"].(string)

		resp = owner.do("DELETE", "/rooms/"+streamRoom, nil, "")
		wantStatus(t, resp, 204)
		drainAndClose(resp)

		resp = anon.do("GET", "/j/"+tok, nil, "")
		wantStatus(t, resp, 404)
		drainAndClose(resp)

		resp = owner.do("GET", "/rooms", nil, "")
		wantStatus(t, resp, 200)
		listBody := decode(t, resp)
		for _, r := range listBody["rooms"].([]any) {
			m, _ := r.(map[string]any)
			if m["name"] == streamRoom {
				t.Fatalf("deleted room %s still listed", streamRoom)
			}
		}
	})
}
```

- [ ] **Step 4: Update `README.md`**

Add a `## Rooms (meetings & screen streaming)` section after the drops table in Use, containing:

```markdown
## Rooms (meetings & screen streaming)

Meetings and screen streams are LiveKit rooms. Set `LIVEKIT_URL`,
`LIVEKIT_API_KEY`, `LIVEKIT_API_SECRET` in `.env` (LiveKit Cloud → project
→ Keys). Rooms auto-close 10 minutes after the last person leaves.

```bash
# create a meeting (or "stream"); share the guest_url with friends
curl -H "$AUTH" -X POST "$BASE/rooms" -d '{"type":"meeting"}' -H 'Content-Type: application/json'
# → {"room":"hb-meet-…","guest_url":"…/j/<token>","livekit_url":"wss://…",...}

curl "$BASE/j/<token>?name=Alice"          # friend: no auth → {livekit_url, token, meet_url}
curl -H "$AUTH" -X POST "$BASE/rooms/<room>/token"   # your own (publisher) token
curl -H "$AUTH" "$BASE/rooms"              # active rooms + guest links
curl -H "$AUTH" -X POST "$BASE/rooms/<room>/link"    # rotate the guest link
curl -H "$AUTH" -X DELETE "$BASE/rooms/<room>/link"  # revoke (room stays up)
curl -H "$AUTH" -X DELETE "$BASE/rooms/<room>"       # end the room for everyone
```

Meetings: everyone can publish camera/mic/screen (up to 10 people). Streams:
only you can publish (screen + screen-audio + mic, up to 50 viewers). Paste
`meet_url` into a browser, or use `livekit_url` + `token` in any LiveKit
client.

**Manual acceptance check** (E2E covers the API, not WebRTC): create a
meeting, open `meet_url` for both the owner token and a guest token in two
browser tabs, confirm audio/video flows.
```

Also add the three `LIVEKIT_*` variables to the workflow-test env line in the `## Tests` section.

- [ ] **Step 5: Verify**

```bash
go vet -tags e2e ./projects/homebase/e2e/ && go build ./...
./projects/homebase/e2e/run.sh
```
Expected: cycle-1 `TestHomebaseLifecycle` (20 subtests) AND `TestRoomsLifecycle` (11 subtests) all PASS; teardown clean. Then re-run workflow tests + validate (standard prefix): 64 passed / all valid.

- [ ] **Step 6: Commit**

```bash
git add projects/homebase
git commit -m "test(homebase): rooms E2E against dev-mode LiveKit + README rooms docs

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 7: Final verification & branch finish

**Files:** none new.

- [ ] **Step 1: Full verification from a clean state**

```bash
go build ./... && go vet ./... && go vet -tags e2e ./projects/homebase/e2e/
DATABASE_URL='postgres://noda:noda@localhost:5432/noda?sslmode=disable' FILES_PATH=/tmp/homebase-files SETUP_TOKEN=test-setup-token PUBLIC_BASE_URL=http://localhost:3000 LIVEKIT_URL=ws://localhost:7880 LIVEKIT_API_KEY=devkey LIVEKIT_API_SECRET=secret go run ./cmd/noda validate --config projects/homebase
DATABASE_URL='postgres://noda:noda@localhost:5432/noda?sslmode=disable' FILES_PATH=/tmp/homebase-files SETUP_TOKEN=test-setup-token PUBLIC_BASE_URL=http://localhost:3000 LIVEKIT_URL=ws://localhost:7880 LIVEKIT_API_KEY=devkey LIVEKIT_API_SECRET=secret go run ./cmd/noda test --config projects/homebase
./projects/homebase/e2e/run.sh
```
Expected: all green (64 workflow tests; 31 E2E subtests total).

- [ ] **Step 2: Use the superpowers:finishing-a-development-branch skill** (expected outcome per repo convention: PR from `homebase-rooms` to `main`, squash, `--auto` after the 4 functional CI checks).

---

## Self-Review (done at plan-writing time)

- **Spec coverage:** lk service/env/compose (T1), room_links (T1), POST+GET /rooms (T2), DELETE + owner token (T3), link rotate/revoke (T4), public join with uniform 404 incl. LiveKit-unreachable (T5), E2E vs dev LiveKit + README + manual-acceptance note (T6). Grants copied verbatim into Global Constraints with the uppercase-enum warning. Create-then-insert-with-cleanup ordering in T2.
- **Type consistency:** node ids in tests match workflows; `room`/`type`/`guest_token`/`guest_url`/`links`/`rooms` body keys consistent T2→T6; `room_type` values `meeting`/`stream` everywhere; E2E helpers reuse cycle-1 names (`client`, `login`, `decode`, `wantStatus`, `drainAndClose`).
- **Placeholder scan:** clean; every step has full file content or exact commands with expected output.
- **Known deviation risk:** none of the `${LIVEKIT_*:?}` base-compose vars break cycle-1 flows because run.sh exports defaults and production README documents them as required.
