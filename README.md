# Noda

A configuration-driven API runtime for Go. Build production-grade REST APIs, background workers, scheduled jobs, real-time connections, and stateful services entirely through JSON configuration, a visual workflow editor, and Wasm modules.

## Features

- **Workflow Engine** — DAG-based execution with parallel nodes, branching, loops, retries, and sub-workflows
- **HTTP Server** — Fiber v3 with JWT auth, CORS, rate limiting, OpenAPI generation
- **Database** — PostgreSQL via GORM with transactions and migrations
- **Caching** — Redis-backed key-value with TTL
- **Events & Workers** — Redis Streams consumer groups with dead letter queues
- **Scheduler** — Cron jobs with distributed locking
- **Storage** — Local and S3 file handling with uploads and image processing
- **Real-Time** — WebSocket and SSE with channel routing and cross-instance sync
- **Authorization** — Casbin RBAC/ABAC with multi-tenant policies
- **Wasm Runtime** — Extism-based tick loop with full host API for custom logic
- **Visual Editor** — React Flow canvas with live tracing, auto-layout, and expression autocomplete
- **Observability** — OpenTelemetry tracing, health checks, structured logging
- **Testing** — Workflow test runner with mocks and verbose trace output

## Prerequisites

- Go 1.25+
- Docker and Docker Compose
- libvips (for image processing, included in Docker image)

## Quick Start

```bash
# Scaffold a new project
noda init my-api
cd my-api

# Start with Docker Compose (PostgreSQL + Redis included)
docker compose up --build

# Or run locally
noda dev
```

See the [Getting Started Guide](docs/01-getting-started/quick-start.md) for a full tutorial.

## Development

```bash
make build          # Build binary to dist/noda
make test           # Run tests with race detector
make test-coverage  # Generate coverage report
make lint           # Run golangci-lint
make fmt            # Format code
make dev            # Start with Docker Compose
make clean          # Remove build artifacts
```

## Project Structure

```
cmd/noda/           CLI entry point
pkg/api/            Public interfaces (plugin author contract)
internal/           Core runtime packages
plugins/            Plugin implementations (db, cache, storage, stream, pubsub, http, email, image)
editor/             Visual editor (React + TypeScript)
examples/           Example projects
testdata/           Test fixtures
docs/               Documentation
```

## Documentation

### Guides

| Document | Description |
|---|---|
| [Getting Started](docs/01-getting-started/quick-start.md) | Installation, quick start, tutorial |
| [Config Reference](docs/02-config/overview.md) | All config file formats and fields |
| [Node Reference](docs/03-nodes/_index.md) | All 72 node types with examples |
| [Plugin Author Guide](docs/04-guides/plugin-development.md) | Building custom plugins |
| [Wasm Developer Guide](docs/04-guides/wasm-development.md) | Building Wasm modules |
| [Deployment Guide](docs/04-guides/deployment.md) | Production deployment, scaling, observability |

### Architecture

| Document | Description |
|---|---|
| [Architecture Plan](docs/_internal/architecture-plan.md) | Full system design — runtimes, workflow engine, plugins, config format |
| [Public API Interfaces](docs/_internal/interfaces.md) | Go interfaces for plugin authors |
| [Wasm Host API](docs/_internal/wasm-host-api.md) | Contract for Wasm module developers |
| [Config Conventions](docs/_internal/config-conventions.md) | Field naming rules, value formats, structural patterns |
| [Node Reference](docs/03-nodes/_index.md) | All 72 nodes — config, outputs, behavior |
| [Visual Editor](docs/_internal/visual-editor.md) | Editor design — React Flow, tech stack, features |

### Examples

| Example | Description |
|---|---|
| [REST API](examples/rest-api/) | CRUD task management with auth and validation |
| [SaaS Backend](examples/saas-backend/) | Multi-tenant with webhooks, workers, uploads, email |
| [Real-Time Collaboration](examples/realtime-collab/) | WebSocket-based live editing |
| [Discord Bot](examples/discord-bot/) | Wasm module with gateway connection |
| [Wasm Counter](examples/wasm-counter/) | Simple stateful Wasm module |
| [Init Example](examples/init-example/) | Scaffolded starter project from `noda init` |
| [Video Rooms](examples/video-rooms/) | Video conferencing with LiveKit integration |

### Use Cases

| Use Case | Description |
|---|---|
| [Simple REST API](docs/05-examples/rest-api.md) | CRUD, auth, validation, OpenAPI |
| [SaaS Backend](docs/05-examples/saas-backend.md) | Multi-tenant, webhooks, workers, uploads, email |
| [Real-Time Collaboration](docs/05-examples/realtime-collab.md) | WebSocket, presence, live editing |
| [Discord Bot](docs/05-examples/discord-bot.md) | Wasm runtime, gateway connection, async HTTP |
| [Multiplayer Game](docs/05-examples/multiplayer-game.md) | Wasm 20Hz tick loop, state broadcasting |
| [Video Conferencing](docs/05-examples/video-conferencing.md) | LiveKit rooms, tokens, recording, webhooks |

## Tech Stack

**Runtime:** Go — Fiber v3, GORM, go-redis, Casbin, Expr, Extism (Wasm), Afero, bimg, Cobra, OpenTelemetry

**Editor:** React + TypeScript — React Flow, shadcn/ui, Zustand, ELKjs, Monaco Editor

## License

MIT — see [LICENSE](LICENSE)
