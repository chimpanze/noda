# Design: End-to-End Verification of External-Service Nodes

**Date:** 2026-06-25
**Status:** Approved (pending spec review)
**Goal:** Extend the non-external node e2e verification (see
`2026-06-22-non-external-node-e2e-design.md`) to every Noda node that *does*
require an external service, exercising each through the real engine against a
real containerized backend, fixing any bugs found, so the product is
production-ready for those nodes too.

## Relationship to the prior round

The 2026-06-22 work proved every node with **no** external dependency works
end-to-end (Tier A pure-compute via `noda test`, Tier B in-process services via
`engine.ExecuteGraph`). It explicitly deferred external-service nodes as an
out-of-scope follow-up:

> - End-to-end coverage of external-service nodes (needs Redis/Postgres/etc.
>   fixtures or testcontainers).
> - `event.emit` once an internal/no-op delivery mode exists (or via embedded Redis).

This spec is that follow-up. It reuses the **same engine-e2e pattern** —
`plugins/core/upload/engine_e2e_test.go` is the template — changing only one
thing: the service in the registry is backed by a real container instead of an
in-memory fake.

## Scope

### Findings from the code that shape scope

- `stream`, `pubsub`, and `storage` plugins register **no nodes**
  (`Nodes() []api.NodeRegistration { return nil }`). They are *service-only*
  plugins. There are no `stream.*` / `pubsub.*` / `storage.*` node types to test
  directly.
- The only consumer of the `stream` and `pubsub` services is the core node
  `event.emit` (`plugins/core/event/emit.go`), which selects between them via a
  `mode` config (`"stream"` | `"pubsub"`). Covering `event.emit` against real
  Redis is therefore how the stream/pubsub services get end-to-end coverage.
- The `storage` plugin supports only `local` and `memory` backends
  (`plugins/storage/plugin.go`: *"unknown backend … (supported: local,
  memory)"*). There is **no S3/MinIO backend**. Its only consumer node,
  `upload.handle`, is already covered end-to-end by the 2026-06-22 Tier B test
  with in-memory afero. Storage needs **no container** and **no new test** here.

### In scope

Nodes whose execution requires a real external service, driven through
`engine.Compile` + `engine.ExecuteGraph` against a container:

| Node(s) | External service | Container image |
|--------|------------------|-----------------|
| `db.create`, `db.find`, `db.findOne`, `db.count`, `db.update`, `db.upsert`, `db.delete` (and `db.query`/`db.exec` where raw SQL is enabled) | Postgres (gorm, `url` config) | `postgres:17` |
| `cache.set`, `cache.del`, `cache.exists` | Redis (`url` via `internal/plugin/redis.go`) | `redis:7` |
| `event.emit` (mode `stream`) | Redis Streams (`stream` service) | `redis:7` (shared) |
| `event.emit` (mode `pubsub`) | Redis PubSub (`pubsub` service) | `redis:7` (shared) |
| `email.send` | SMTP (`host`/`port`) | `axllent/mailpit` |

### Out of scope

- `image.*` — uses libvips (`bimg`) locally; no external **service**.
- `http.*` — outbound HTTP; testable with an in-process `httptest` server, not a
  container. (Candidate for a later round; excluded here per scope decision.)
- `livekit.*` — LiveKit server; heavier setup, deferred.
- `oidc` / auth middleware — not node plugins.
- `storage` — no nodes, no external backend, already covered (see above).
- Any node already covered by the 2026-06-22 round.
- The React editor, docs, and CI for anything beyond the new integration job.

## How Noda runs these nodes (the pattern we reuse)

From `plugins/core/upload/engine_e2e_test.go`:

1. Build a `registry.ServiceRegistry`, register the real service under the name
   the node's `Services` map references.
2. Build a `registry.NodeRegistry`, `RegisterFromPlugin(&Plugin{})` for the
   plugin(s) under test.
3. Construct an `engine.WorkflowConfig` (the smallest graph that exercises the
   node), `engine.Compile(wf, nodeReg)`.
4. `engine.NewExecutionContext(engine.WithInput(...))`, then
   `engine.ExecuteGraph(ctx, graph, execCtx, svcReg, nodeReg)`.
5. Assert the **output** via `execCtx.GetOutput(nodeID)` **and** the **effect**
   directly against the backend (row in Postgres, key in Redis, message in
   Mailpit).
6. Assert the **error path** (missing service, bad config, backend rejection).

For external nodes the only change is step 1: the service comes from
`plugin.CreateService(map[string]any{"url": <container URL>})` (or the SMTP
host/port for email), so the node talks to a real backend.

## Approach

### 1. Container provisioning: `testcontainers-go`

- Add `testcontainers-go` as a **test-only** dependency (the `postgres` and
  `redis` modules, plus a generic container for Mailpit).
- New package `internal/testing/containers/` exposing helpers used by the
  integration test files:
  - `StartPostgres(t testing.TB) (url string)` — returns a gorm-compatible
    `postgres://…` connection URL.
  - `StartRedis(t testing.TB) (url string)` — returns a `redis://…` URL parseable
    by `redis.ParseURL` (matches `internal/plugin/redis.go`).
  - `StartMailpit(t testing.TB) (smtpHost string, smtpPort int, apiBaseURL string)`
    — SMTP endpoint for the email service config, HTTP API base for assertions.
  - Each helper registers `t.Cleanup` to terminate the container.
- **Container reuse:** start one container per test package via `TestMain` (or a
  `sync.Once`-guarded package singleton) so many cases share a backend. Cases
  isolate themselves with unique table names (Postgres), key prefixes (Redis),
  and unique recipient/subject (Mailpit) rather than per-case containers, to keep
  wall-clock and Docker load reasonable.
- testcontainers auto-pulls images and picks random host ports, so there is no
  fixed-port collision and no separate `docker compose up` step. Docker must be
  running; if the Docker daemon is unavailable the package skips with a clear
  message.

### 2. Build tag and invocation

- Every new external-e2e file carries `//go:build integration`.
- `make test-integration` runs `go test -tags=integration ./...`.
- Default `go test ./...` (no tag) is unchanged: container-free and fast.

### 3. Test files

One file per plugin, alongside the existing package tests, named
`engine_e2e_integration_test.go` and tagged `//go:build integration`:

- **`plugins/db/engine_e2e_integration_test.go`** — against real Postgres:
  - Happy: `db.create` inserts a row; a subsequent `db.find` / `db.findOne`
    returns it; `db.count` reflects it; `db.update` mutates it; `db.upsert`
    inserts-or-updates; `db.delete` removes it (verified by `db.count` and by a
    direct query). Cover the CRUD set as a small ordered round-trip plus targeted
    single-node graphs where clearer.
  - Effect asserted both via node output and a direct gorm/SQL read.
  - Error path: operation against a non-existent table; node with missing service
    dependency yields a descriptive error, not a panic.
  - `db.query`/`db.exec` covered only if/where raw SQL is enabled in the test
    config; otherwise noted as gated.
- **`plugins/cache/engine_e2e_integration_test.go`** — against real Redis:
  - Happy: `cache.set` (with and without TTL) → `cache.exists` true →
    `cache.del` → `cache.exists` false. Assert directly against Redis (key
    present/absent, TTL applied).
  - Error path: node with missing service.
- **`plugins/core/event/engine_e2e_integration_test.go`** — against real Redis:
  - `event.emit` mode `stream`: published payload readable via `XRANGE`/`XREAD`
    on the topic; returned message ID is non-empty.
  - `event.emit` mode `pubsub`: a subscriber created in the test receives the
    published payload on the channel.
  - Error path: unknown/empty `mode`; missing the corresponding service.
- **`plugins/email/engine_e2e_integration_test.go`** — against Mailpit:
  - Happy: `email.send` delivers to Mailpit; assert via Mailpit's HTTP API that a
    message arrived with the expected To, Subject, and body.
  - Error path: unroutable SMTP host / send failure surfaces as a node error.
  - Note: verify Mailpit accepts plain SMTP on its listen port without
    STARTTLS/auth given `plugins/email/service.go`'s dial logic; configure the
    service accordingly.

### 4. Shared harness location

`internal/testing/containers/` keeps the container plumbing in one place so each
plugin test stays focused on the workflow + assertions, mirroring how the
2026-06-22 round kept the driver in `internal/testing/e2e/`.

## Bug handling

User-approved fix-in-place mode, identical to the prior round. Keep a running
findings list and present a summary at the end covering, per bug: node, symptom,
root cause, fix, and the test that now guards it. Record it in
`docs/superpowers/specs/2026-06-25-external-node-e2e-findings.md`.

## Error handling & edge cases the tests must cover

- Missing required service yields a descriptive error, not a nil-deref/panic.
- Backend rejection (bad table, send failure) routes to the node's `error`
  output or fails the workflow cleanly per the node's contract.
- `cache.set` TTL is actually applied (observed via Redis).
- `event.emit` with an invalid `mode` errors clearly.
- Container/Docker unavailable → package skips with an actionable message rather
  than failing spuriously.

## CI

- New `integration` job that installs Docker (default on GitHub-hosted runners),
  installs `libvips-dev` only if needed, and runs `go test -tags=integration
  ./...`. testcontainers manages Postgres/Redis/Mailpit itself — no `services:`
  blocks.
- The existing unit job is unchanged so the default suite stays fast.

## Definition of done

- Every in-scope node (`db.*`, `cache.*`, `event.emit` stream+pubsub,
  `email.send`) has end-to-end coverage for happy path + at least one
  edge/error path, driven through `engine.ExecuteGraph` against a real container.
- `internal/testing/containers/` helpers exist and are reused across the plugin
  tests.
- All bugs found are fixed; findings summary delivered.
- `go test -tags=integration ./...` passes with Docker running.
- Default `go test ./...` is unaffected (no new dependency pulled into the
  non-integration build path beyond test files behind the build tag).
- No regression in existing tests.

## Out-of-scope follow-ups (noted, not done here)

- `http.*` end-to-end via an in-process `httptest` server.
- `livekit.*` against a LiveKit server container.
- An S3/MinIO storage backend (would also need plugin code, not just tests).
- Wiring the integration suite into the nightly/chaos schedules described in the
  broader production-trustworthiness plan.
