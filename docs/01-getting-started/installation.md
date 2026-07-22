# Installation

Noda is a configuration-driven API runtime for Go. You define routes, workflows, middleware, auth, services, and real-time connections in JSON config files — no application code required for standard patterns. Custom logic runs in Wasm modules.

## Quick Install (macOS / Linux)

```bash
curl -fsSL https://raw.githubusercontent.com/chimpanze/noda/main/install.sh | sh
```

This downloads the latest release binary and installs it to `/usr/local/bin`. To install a specific version:

```bash
curl -fsSL https://raw.githubusercontent.com/chimpanze/noda/main/install.sh | VERSION=v1.0.0 sh
```

Re-run the same command to update to the latest version.

## Windows

There is no prebuilt Windows binary. Noda requires cgo and libvips, so on Windows either run it in Docker (simplest) or build from source.

**Docker:**

```
docker pull ghcr.io/chimpanze/noda:latest
```

**From source:** install [Go 1.26+](https://go.dev/dl/), Node 22+, a C toolchain (e.g. [MSYS2](https://www.msys2.org/) mingw-w64), and [libvips](https://www.libvips.org/install.html). Run the build from the MSYS2 shell — the `Makefile` uses Unix commands that are not available in PowerShell or cmd.exe:

```
git clone https://github.com/chimpanze/noda.git
cd noda
make build
```

This produces `dist/noda`. Verify it:

```
dist/noda version
```

## Docker

```bash
docker pull ghcr.io/chimpanze/noda:latest
```

## Prerequisites

- **PostgreSQL** (optional) — for database operations
- **Redis** (optional) — for caching, events, pub/sub, distributed locking
- **libvips** (required) — Noda links against the system libvips dynamically and it is **not** bundled with the prebuilt binary, so the binary will not start without it. Install it on every machine that runs Noda (e.g. `brew install vips`, `apt install libvips-dev`), whether you use the prebuilt binary or build from source. The Docker image already includes it.

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
| `noda mcp` | Start MCP server for AI agent integration (see [MCP & AI Agents](../04-guides/mcp-and-ai-agents.md)) |
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
