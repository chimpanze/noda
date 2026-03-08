# Noda

A configuration-driven API runtime for Go. Build production-grade REST APIs, background workers, scheduled jobs, real-time connections, and stateful services entirely through JSON configuration, a visual workflow editor, and Wasm modules.

## Prerequisites

- Go 1.25+
- Docker and Docker Compose
- libvips (for image processing, included in Docker image)

## Quick Start

```bash
# Clone and start with Docker Compose
docker compose up --build

# Or build and run locally
make build
./dist/noda version
```

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
plugins/            Plugin implementations
testdata/           Test fixtures
docs/               Architecture and planning docs
```

## Documentation

| Document | Description |
|---|---|
| [Architecture Plan](docs/architecture/architecture-plan.md) | Full system design — runtimes, workflow engine, plugins, config format |
| [Public API Interfaces](docs/architecture/interfaces.md) | Go interfaces for plugin authors |
| [Wasm Host API](docs/architecture/wasm-host-api.md) | Contract for Wasm module developers |
| [Config Conventions](docs/architecture/config-conventions.md) | Field naming rules, value formats, structural patterns |
| [Core Node Catalog](docs/architecture/core-nodes.md) | All 46 nodes — config, outputs, behavior |
| [Visual Editor](docs/architecture/visual-editor.md) | Editor vision — React Flow, tech stack, features |
| [Implementation Plan](docs/implementation-plan.md) | 29 milestones with dependencies and deliverables |

### Use Cases

| Use Case | Validates |
|---|---|
| [Simple REST API](docs/use-cases/01-rest-api.md) | CRUD, auth, validation, OpenAPI |
| [SaaS Backend](docs/use-cases/02-saas-backend.md) | Multi-tenant, webhooks, workers, uploads, email |
| [Real-Time Collaboration](docs/use-cases/03-realtime-collab.md) | WebSocket, presence, live editing |
| [Discord Bot](docs/use-cases/04-discord-bot.md) | Wasm runtime, gateway connection, async HTTP |
| [Multiplayer Game](docs/use-cases/05-multiplayer-game.md) | Wasm 20Hz tick loop, state broadcasting |

### Milestone Task Breakdowns

Detailed task breakdowns for each milestone are in [docs/milestones/](docs/milestones/).

## Tech Stack

**Runtime:** Go — Fiber v3, GORM, go-redis, Casbin, Expr, Extism (Wasm), Afero, bimg, Cobra

**Editor:** React + TypeScript — React Flow, shadcn/ui, Zustand, ELKjs, Monaco Editor

## License

MIT — see [LICENSE](LICENSE)
