# CLAUDE.md — Noda Project Guide

## What is Noda

Noda is a configuration-driven API runtime for Go. JSON config files define routes, workflows, middleware, auth, services, and real-time connections. A visual editor generates the config. No application code required for standard patterns. Custom logic runs in Wasm modules.

## Project Status

**Planning phase.** No code written yet. Architecture fully designed across multiple documents. Implementation plan defined with 29 milestones and comprehensive task breakdowns.

## Repository Structure

```
CLAUDE.md                              ← you are here
README.md                              ← project overview
docs/
  implementation-plan.md               ← 29 milestones with dependencies and deliverables
  architecture/
    architecture-plan.md               ← full system design (1777 lines, 32 sections)
    interfaces.md                      ← Go public API interfaces for plugin authors
    wasm-host-api.md                   ← Wasm module developer contract
    config-conventions.md              ← config field naming rules and patterns
    core-nodes.md                      ← all 46 node specs (config, outputs, behavior)
    visual-editor.md                   ← editor vision (React Flow, tech stack, features)
    future-client-generation.md        ← future vision: SDK + Lit web components
  use-cases/
    01-rest-api.md                     ← simple CRUD API
    02-saas-backend.md                 ← multi-tenant with webhooks, workers, uploads
    03-realtime-collab.md              ← WebSocket live editing
    04-discord-bot.md                  ← Wasm module with gateway connection
    05-multiplayer-game.md             ← Wasm 20Hz game loop
  milestones/
    milestone-0-tasks.md               ← project skeleton (14 tasks)
    milestone-1-tasks.md               ← config loading (13 tasks)
    milestone-2-tasks.md               ← expression engine (7 tasks)
    milestone-3-tasks.md               ← plugin system (8 tasks)
    milestone-4-tasks.md               ← workflow engine (8 tasks)
    milestone-5-tasks.md               ← control nodes (9 tasks)
    milestone-6-tasks.md               ← transform + utility nodes (12 tasks)
    milestone-7-tasks.md               ← testing framework (7 tasks)
    milestone-8-tasks.md               ← HTTP server (12 tasks)
    milestone-9-tasks.md               ← database plugin (8 tasks)
    milestone-10-15-tasks.md           ← cache, events, scheduler, storage, image, HTTP/email
    milestone-16-21-tasks.md           ← WebSocket, Casbin, Wasm, observability, dev mode, CLI
    milestone-22-28-tasks.md           ← visual editor (5 milestones), validation, documentation
```

## Technology Stack

- **Language:** Go
- **HTTP:** gofiber/fiber/v3
- **ORM:** gorm.io/gorm (map[string]any, no struct definitions)
- **Redis:** redis/go-redis/v9
- **JWT:** gofiber/contrib/jwt + golang-jwt/jwt/v5
- **Authorization:** casbin/casbin/v2
- **Expressions:** expr-lang/expr
- **JSON Schema:** santhosh-tekuri/jsonschema/v6
- **Storage:** spf13/afero
- **Image:** h2non/bimg (libvips)
- **Cron:** robfig/cron/v3
- **Wasm:** extism/go-sdk (wazero)
- **CLI:** spf13/cobra
- **Observability:** go.opentelemetry.io/otel
- **Editor:** React + TypeScript, @xyflow/react (React Flow), shadcn/ui, Zustand, ELKjs, Monaco

## Milestone Dependency Order

```
M0  Project Skeleton          → no deps
M1  Config Loading            → M0
M2  Expression Engine         → M0
M3  Plugin System             → M0, M1
M4  Workflow Engine           → M1, M2, M3
M5  Core Control Nodes        → M4
M6  Transform + Utility       → M4
M7  Testing Framework         → M5, M6
M8  HTTP Server               → M4, M5, M6
M9  Database Plugin           → M3, M8
M10 Cache Plugin              → M3, M8
M11 Events + Workers          → M3, M4, M8
M12 Scheduler                 → M10
M13 Storage + Upload          → M3, M8
M14 Image Processing          → M13
M15 HTTP Client + Email       → M3, M8
M16 WebSocket + SSE           → M8, M11
M17 Casbin Auth               → M8, M11
M18 Wasm Runtime              → M10, M13, M15, M16
M19 Observability             → M4, M8
M20 Dev Mode + Hot Reload     → M1, M8, M19
M21 CLI Completion            → all
M22 Editor Foundation         → M20
M23 Editor Node Config        → M22
M24 Editor Graph Editing      → M22, M23
M25 Editor Live Tracing       → M22, M20
M26 Editor Remaining Views    → M22, M23
M27 Editor Validation/Polish  → M22-M26
M28 Documentation             → all
```

## Use Case Checkpoints

After M9: Use Case 1 (REST API) works.
After M11: Use Case 2 (SaaS Backend) core works.
After M15: Use Case 2 complete.
After M16: Use Case 3 (Real-Time Collaboration) works.
After M18: Use Cases 4 (Discord Bot) and 5 (Multiplayer Game) work.

## Working with This Project

### Reading Architecture

Start with `docs/architecture/architecture-plan.md` for the full system overview. The other architecture docs define specific contracts: `interfaces.md` for the Go plugin API, `wasm-host-api.md` for the Wasm boundary, `config-conventions.md` for JSON config patterns, `core-nodes.md` for all node specifications.

### Reading Tasks

Each milestone file in `docs/milestones/` contains numbered tasks with checkbox subtasks and acceptance criteria. Tasks within a milestone should be completed in order. Every task specifies what to build, how to test it, and what "done" looks like.

### Implementation Guidelines

- **Test from the bottom up.** Every package has unit tests. Every integration has integration tests. Every milestone ends with end-to-end tests.
- **Interfaces first.** `pkg/api/` interfaces are defined in M0 and stable. All implementations code against them.
- **Real config files.** Test against actual JSON config files in `testdata/`, not hard-coded structures.
- **No parallel milestones.** Complete and test each milestone before starting the next.
- **Docker Compose always green.** From M0 onward, `docker compose up` starts a working system.

### Target Directory Structure (after M0)

```
cmd/noda/                  — CLI entry point
pkg/api/                   — public interfaces (plugin author contract)
internal/config/           — config loading, merging, validation
internal/engine/           — workflow engine
internal/expr/             — expression parser, compiler, evaluator
internal/registry/         — plugin, service, node registries
internal/server/           — Fiber HTTP server
internal/worker/           — worker runtime
internal/scheduler/        — scheduler runtime
internal/connmgr/          — WebSocket/SSE connection manager
internal/wasm/             — Wasm runtime (Extism)
internal/trace/            — tracing, dev mode trace WebSocket
internal/testing/          — workflow test runner
internal/migrate/          — database migration management
plugins/db/                — PostgreSQL plugin
plugins/cache/             — Redis cache plugin
plugins/stream/            — Redis Streams plugin
plugins/pubsub/            — Redis PubSub plugin
plugins/storage/           — Afero storage plugin
plugins/image/             — bimg image plugin
plugins/http/              — outbound HTTP plugin
plugins/email/             — email plugin
plugins/core/              — core node plugins
  plugins/core/control/    — control.if, control.switch, control.loop
  plugins/core/workflow/   — workflow.run, workflow.output
  plugins/core/transform/  — transform.set, transform.map, etc.
  plugins/core/response/   — response.json, response.redirect, response.error
  plugins/core/util/       — util.log, util.uuid, util.delay, util.timestamp
  plugins/core/event/      — event.emit
  plugins/core/upload/     — upload.handle
  plugins/core/ws/         — ws.send
  plugins/core/sse/        — sse.send
  plugins/core/wasm/       — wasm.send, wasm.query
editor/                    — React frontend (Vite + React Flow)
testdata/                  — test fixtures
docs/                      — architecture and planning docs
```

## Creating GitHub Issues from Milestones

The milestone task files in `docs/milestones/` are structured for conversion to GitHub issues:

- Each `## Task X.Y: Title` becomes a GitHub issue
- The task description becomes the issue body
- Checkbox subtasks (`- [ ]`) become the issue's task list
- The **Tests** section lists test requirements
- **Acceptance criteria** defines "done"
- Each issue should be assigned to its milestone (e.g., "M0: Project Skeleton")

Recommended labels:
- `type:feature`, `type:test`, `type:infrastructure`, `type:documentation`
- `component:engine`, `component:config`, `component:plugin`, `component:server`, `component:worker`, `component:scheduler`, `component:wasm`, `component:connmgr`, `component:editor`, `component:cli`
- `priority:critical` (blocks other milestones), `priority:normal`

Each milestone should also be created as a GitHub Milestone with its description from `docs/implementation-plan.md`.
