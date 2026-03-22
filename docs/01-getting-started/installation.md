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
| `noda start` | Start the production server |
| `noda dev` | Start in dev mode with hot reload |
| `noda validate` | Validate all config files |
| `noda test` | Run workflow tests |
| `noda migrate create [name]` | Create a new migration |
| `noda migrate up` | Apply all pending migrations |
| `noda migrate down` | Roll back the last migration |
| `noda migrate status` | Show migration status |
| `noda generate openapi` | Generate OpenAPI 3.1 specification |
| `noda schedule status` | Show configured scheduled jobs |
| `noda plugin list` | List all registered plugins and node counts |
| `noda mcp` | Start MCP server for AI agent integration |
| `noda version` | Print version and build info |
| `noda completion <shell>` | Generate shell completions |

### Global Flags

| Flag | Description |
|------|-------------|
| `--config <path>` | Path to config directory (default: `.`) |
| `--env <name>` | Runtime environment (loads overlay) |

### Common Command Flags

| Command | Flag | Description |
|---------|------|-------------|
| `noda start` | `--server` | Start HTTP server only |
| `noda start` | `--workers` | Start worker runtime only |
| `noda start` | `--scheduler` | Start scheduler only |
| `noda start` | `--wasm` | Start Wasm runtimes only |
| `noda start` | `--all` | Start all runtimes (default) |
| `noda validate` | `--verbose` | Show detailed validation info |
| `noda test` | `--verbose` | Show execution traces for all tests |
| `noda test` | `--workflow <id>` | Run tests only for specified workflow |
| `noda generate openapi` | `--output <file>` | Output file path (default: stdout) |
| `noda migrate *` | `--service <name>` | Database service name (default: `db`) |
