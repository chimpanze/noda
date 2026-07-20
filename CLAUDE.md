# CLAUDE.md — Noda Project Guide

## What is Noda

Noda is a configuration-driven API runtime for Go. JSON config files define routes, workflows, middleware, auth, services, and real-time connections. A visual editor generates the config. No application code required for standard patterns. Custom logic runs in Wasm modules.

## Project Status

**Complete.** All 29 milestones implemented. 81.5% test coverage (2026-07-20; CI gates
at ≥75% — `make test-coverage` prints the current figure). All 6 use cases working.

All 81 node types have a runnable, CI-verified example in `examples/node-cookbook`; the
`TestCookbookCoverage` gate (run with `-tags integration`) keeps that at 81/81.

## Repository Structure

```
CLAUDE.md                              ← you are here
README.md                              ← project overview
cmd/noda/                              ← CLI entry point
pkg/api/                               ← public interfaces (plugin author contract)
internal/
  bounded/                             ← generic bounded queue with explicit drop policy
  breaker/                             ← circuit breaker for service calls
  config/                              ← config loading, merging, validation
  connmgr/                             ← WebSocket/SSE connection manager
  devmode/                             ← dev mode with hot reload
  editor/                              ← visual editor dev-mode API (/_noda endpoints)
  engine/                              ← workflow engine
  expr/                                ← expression parser, compiler, evaluator
  generate/                            ← CRUD + migration scaffolding generators
  lifecycle/                           ← startup/shutdown lifecycle manager
  mcp/                                 ← Model Context Protocol server (noda mcp)
  metrics/                             ← metrics subsystem + HTTP middleware
  migrate/                             ← database migration management
  netguard/                            ← outbound-network policy checks
  pathutil/                            ← validated roots for safe path resolution
  plugin/                              ← shared plugin config resolution helpers
  registry/                            ← plugin, service, node registries
  routecfg/                            ← route/middleware config helpers (leaf)
  scaffold/                            ← project scaffold helpers (CLI + MCP)
  scheduler/                           ← scheduler runtime
  secrets/                             ← .env loading and {{ $env(...) }} resolution
  server/                              ← Fiber HTTP server
  testing/                             ← workflow test runner
  trace/                               ← tracing, dev mode trace WebSocket
  wasm/                                ← Wasm runtime (Extism)
  worker/                              ← worker runtime
plugins/
  all/                                 ← single canonical plugin list (registers every plugin)
  auth/                                ← auth plugin (users, sessions, tokens)
  db/                                  ← PostgreSQL plugin
  cache/                               ← Redis cache plugin
  stream/                              ← Redis Streams plugin
  pubsub/                              ← Redis PubSub plugin
  storage/                             ← Afero storage plugin
  image/                               ← bimg image plugin
  http/                                ← outbound HTTP plugin
  email/                               ← email plugin
  livekit/                             ← LiveKit WebRTC plugin (lk.* nodes, snake_case)
  core/                                ← core node plugins
    control/                           ← control.if, control.switch, control.loop
    workflow/                          ← workflow.run, workflow.output
    transform/                         ← transform.set, transform.map, etc.
    response/                          ← response.json, response.redirect, response.error
    util/                              ← util.log, util.uuid, util.delay, util.timestamp
    event/                             ← event.emit
    oidc/                              ← OIDC login/callback nodes
    storage/                           ← storage.read, storage.write, storage.delete, storage.list
    upload/                            ← upload.handle
    ws/                                ← ws.send
    sse/                               ← sse.send
    wasm/                              ← wasm.send, wasm.query
editor/                                ← React frontend (Vite + React Flow)
editorfs/                              ← embedded editor assets
pdk/                                   ← Wasm plugin development kit
examples/                              ← 10 example projects (incl. node-cookbook)
testdata/                              ← test fixtures
tools/ai-usability/                    ← AI-agent usability harness + findings
docs/
  01-getting-started/                  ← installation, quick start, expressions, data flow
  02-config/                           ← all config file formats and fields (12 files)
  03-nodes/                            ← all 81 node types (one file per node)
  04-guides/                           ← deployment, plugin dev, wasm dev
  05-examples/                         ← 6 use case walkthroughs
  _internal/                           ← architecture docs (excluded from editor)
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
