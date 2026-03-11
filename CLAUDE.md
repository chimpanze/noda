# CLAUDE.md — Noda Project Guide

## What is Noda

Noda is a configuration-driven API runtime for Go. JSON config files define routes, workflows, middleware, auth, services, and real-time connections. A visual editor generates the config. No application code required for standard patterns. Custom logic runs in Wasm modules.

## Project Status

**Complete.** All 29 milestones implemented. 80.8% test coverage. All 5 use cases working.

## Repository Structure

```
CLAUDE.md                              ← you are here
README.md                              ← project overview
cmd/noda/                              ← CLI entry point
pkg/api/                               ← public interfaces (plugin author contract)
internal/
  config/                              ← config loading, merging, validation
  engine/                              ← workflow engine
  expr/                                ← expression parser, compiler, evaluator
  registry/                            ← plugin, service, node registries
  server/                              ← Fiber HTTP server
  worker/                              ← worker runtime
  scheduler/                           ← scheduler runtime
  connmgr/                             ← WebSocket/SSE connection manager
  wasm/                                ← Wasm runtime (Extism)
  trace/                               ← tracing, dev mode trace WebSocket
  testing/                             ← workflow test runner
  migrate/                             ← database migration management
  devmode/                             ← dev mode with hot reload
plugins/
  db/                                  ← PostgreSQL plugin
  cache/                               ← Redis cache plugin
  stream/                              ← Redis Streams plugin
  pubsub/                              ← Redis PubSub plugin
  storage/                             ← Afero storage plugin
  image/                               ← bimg image plugin
  http/                                ← outbound HTTP plugin
  email/                               ← email plugin
  core/                                ← core node plugins
    control/                           ← control.if, control.switch, control.loop
    workflow/                          ← workflow.run, workflow.output
    transform/                         ← transform.set, transform.map, etc.
    response/                          ← response.json, response.redirect, response.error
    util/                              ← util.log, util.uuid, util.delay, util.timestamp
    event/                             ← event.emit
    upload/                            ← upload.handle
    ws/                                ← ws.send
    sse/                               ← sse.send
    wasm/                              ← wasm.send, wasm.query
editor/                                ← React frontend (Vite + React Flow)
editorfs/                              ← embedded editor assets
pdk/                                   ← Wasm plugin development kit
examples/                              ← 6 example projects
testdata/                              ← test fixtures
docs/
  getting-started.md                   ← installation, quick start, tutorial
  config-reference.md                  ← all config file formats and fields
  node-reference.md                    ← all 46 node types with examples
  plugin-author-guide.md               ← building custom plugins
  wasm-developer-guide.md              ← building Wasm modules
  deployment-guide.md                  ← production deployment guide
  architecture/
    architecture-plan.md               ← full system design
    interfaces.md                      ← Go public API interfaces
    wasm-host-api.md                   ← Wasm module developer contract
    config-conventions.md              ← config field naming rules and patterns
    core-nodes.md                      ← all 46 node specs
    visual-editor.md                   ← editor design
    future-client-generation.md        ← future vision: SDK + Lit web components
  use-cases/
    01-rest-api.md                     ← simple CRUD API
    02-saas-backend.md                 ← multi-tenant with webhooks, workers, uploads
    03-realtime-collab.md              ← WebSocket live editing
    04-discord-bot.md                  ← Wasm module with gateway connection
    05-multiplayer-game.md             ← Wasm 20Hz game loop
  milestones/                          ← historical task breakdowns (all complete)
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

## Working with This Project

### Documentation

Start with `docs/getting-started.md` for usage. See `docs/architecture/architecture-plan.md` for the full system design. The other architecture docs define specific contracts: `interfaces.md` for the Go plugin API, `wasm-host-api.md` for the Wasm boundary, `config-conventions.md` for JSON config patterns, `core-nodes.md` for all node specifications.

### Development Guidelines

- **Test from the bottom up.** Every package has unit tests. Every integration has integration tests.
- **Interfaces first.** `pkg/api/` interfaces are stable. All implementations code against them.
- **Real config files.** Test against actual JSON config files in `testdata/`, not hard-coded structures.
- **Docker Compose always green.** `docker compose up` starts a working system.
