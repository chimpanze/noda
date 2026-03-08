# Noda

A configuration-driven API runtime for Go. Build production-grade REST APIs, background workers, scheduled jobs, real-time connections, and stateful services entirely through JSON configuration, a visual workflow editor, and Wasm modules.

## Status

**Planning phase.** Architecture fully designed. Implementation not yet started.

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

TBD
