# Noda — Use Case: Simple REST API

**Version**: 0.4.0

A task management API with CRUD operations, JWT authentication, request validation, and OpenAPI documentation. This is the baseline use case — what most developers will build first.

---

## What We're Building

A REST API for managing tasks:

- `POST /api/tasks` — create a task
- `GET /api/tasks` — list tasks (with pagination)
- `GET /api/tasks/:id` — get a single task
- `PUT /api/tasks/:id` — update a task
- `DELETE /api/tasks/:id` — delete a task

Authenticated via JWT. Input validated against JSON Schema. Responses follow the standardized format.

---

## Services Required

| Instance | Plugin | Purpose |
|---|---|---|
| `main-db` | `postgres` | Task storage |

Minimal setup — one database, no Redis, no storage, no workers.

---

## Config Structure

```
noda.json                — ports, JWT config, service declarations
schemas/
  Task.json              — task schema ($ref'd by routes and workflows)
routes/
  tasks.json             — all 5 route definitions
workflows/
  create-task.json
  list-tasks.json
  get-task.json
  update-task.json
  delete-task.json
```

---

## Key Workflows

### Create Task

**Trigger:** `POST /api/tasks` → workflow `create-task`

**Input mapping:** `{ "title": "{{ body.title }}", "description": "{{ body.description }}" }`

**Nodes:**

1. `db.create` — insert into tasks table with `user_id` from `{{ auth.sub }}`
2. `response.json` — return 201 with created task

Request body validation happens automatically at the route level via `body.schema` — invalid requests receive a `422` response before the workflow runs.

**Error path:** Validation failure → automatic 422 (route-level). DB failure → `response.error` (500).

**Features exercised:** Trigger mapping, expression resolution, auth context (`$.auth`), JSON Schema validation, database write, standardized error responses.

### List Tasks with Pagination

**Trigger:** `GET /api/tasks` → workflow `list-tasks`

**Input mapping:** `{ "page": "{{ query.page ?? 1 }}", "limit": "{{ query.limit ?? 20 }}" }`

**Nodes:**

1. `db.query` (as `count`) — `SELECT COUNT(*) FROM tasks WHERE user_id = $1`
2. `db.query` (as `rows`) — `SELECT * FROM tasks WHERE user_id = $1 ORDER BY created_at DESC LIMIT $2 OFFSET $3`
3. `response.json` — return 200 with `{ "data": "{{ rows }}", "pagination": { "page": "{{ input.page }}", "limit": "{{ input.limit }}", "total": "{{ count[0].count }}" } }`

Both queries run in parallel (no dependency between them). The response node waits for both (AND-join).

**Features exercised:** Implicit parallelism, expression defaults (`??` operator), parameterized queries, response composition from multiple upstream nodes.

### Get Single Task

**Trigger:** `GET /api/tasks/:id` → workflow `get-task`

**Nodes:**

1. `db.query` — `SELECT * FROM tasks WHERE id = $1 AND user_id = $2`
2. `control.if` — condition: `{{ len(db-query) == 0 }}`
   - `then` → `response.error` (404, NOT_FOUND)
   - `else` → `response.json` (200, task data)

**Features exercised:** Conditional branching, `control.if` with `then`/`else`, different response nodes per branch.

---

## Architecture Features Validated

| Feature | How it's used |
|---|---|
| Trigger mapping | Every route maps request data to `$.input` |
| JWT authentication | All routes use `auth.jwt` middleware, workflows access `auth.sub` |
| JSON Schema validation | Route-level `body.schema` validates requests automatically before the workflow runs |
| Implicit parallelism | List endpoint runs count and fetch concurrently |
| Flow convergence (AND-join) | Response node waits for both parallel queries |
| Conditional branching | Get endpoint handles "not found" case |
| Standardized errors | All error paths produce the same error format |
| Expression engine | Default values, string interpolation, path access |
| OpenAPI generation | Routes + schemas → auto-generated API docs |
| `$ref` schemas | Task schema shared between routes and validation |

---

## What's NOT Needed

No Redis, no workers, no WebSockets, no Wasm, no storage, no image processing, no events. A single PostgreSQL database and the HTTP server runtime.
