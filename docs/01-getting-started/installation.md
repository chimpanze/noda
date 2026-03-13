# Installation

Noda is a configuration-driven API runtime for Go. You define routes, workflows, middleware, auth, services, and real-time connections in JSON config files — no application code required for standard patterns. Custom logic runs in Wasm modules.

## Docker (recommended)

```bash
docker pull ghcr.io/your-org/noda:latest
```

## Go Install

```bash
go install github.com/your-org/noda/cmd/noda@latest
```

## Binary Download

Download the latest release from the [releases page](https://github.com/your-org/noda/releases) for your platform.

## Prerequisites

- **PostgreSQL** (optional) — for database operations
- **Redis** (optional) — for caching, events, pub/sub, distributed locking
- **libvips** (optional) — for image processing (`image.*` nodes)

## CLI Reference

| Command | Description |
|---------|-------------|
| `noda init [name]` | Scaffold a new project |
| `noda start` | Start the server |
| `noda dev` | Start in dev mode with hot reload |
| `noda validate` | Validate all config files |
| `noda test` | Run workflow tests |
| `noda migrate` | Run database migrations |
| `noda generate` | Generate config scaffolds |
| `noda plugin list` | List available plugins |
| `noda plugin info <name>` | Show plugin details |
| `noda completion <shell>` | Generate shell completions |
