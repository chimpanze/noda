# Getting Started with Noda

Noda is a configuration-driven API runtime for Go. You define routes, workflows, middleware, auth, services, and real-time connections in JSON config files — no application code required for standard patterns. Custom logic runs in Wasm modules.

## Installation

### Docker (recommended)

```bash
docker pull ghcr.io/your-org/noda:latest
```

### Go Install

```bash
go install github.com/your-org/noda/cmd/noda@latest
```

### Binary Download

Download the latest release from the [releases page](https://github.com/your-org/noda/releases) for your platform.

### Prerequisites

- **PostgreSQL** (optional) — for database operations
- **Redis** (optional) — for caching, events, pub/sub, distributed locking
- **libvips** (optional) — for image processing (`image.*` nodes)

## Quick Start

### 1. Scaffold a New Project

```bash
noda init my-api
cd my-api
```

This creates a project with the following structure:

```
my-api/
├── noda.json              # Root config: services, security, middleware
├── vars.json              # Shared variables (optional)
├── routes/                # HTTP route definitions
├── workflows/             # Workflow DAGs
├── schemas/               # JSON Schema definitions
└── tests/                 # Workflow test suites
```

### 2. Define a Route

Create `routes/hello.json`:

```json
{
  "id": "hello",
  "method": "GET",
  "path": "/api/hello",
  "summary": "Say hello",
  "trigger": {
    "workflow": "hello",
    "input": {
      "name": "{{ query.name }}"
    }
  }
}
```

### 3. Define a Workflow

Create `workflows/hello.json`:

```json
{
  "id": "hello",
  "name": "Hello Workflow",
  "nodes": {
    "respond": {
      "type": "response.json",
      "config": {
        "status": 200,
        "body": {
          "message": "Hello, {{ input.name ?? 'world' }}!"
        }
      }
    }
  },
  "edges": []
}
```

### 4. Start the Server

```bash
noda start
```

### 5. Make a Request

```bash
curl http://localhost:3000/api/hello?name=Noda
# {"message": "Hello, Noda!"}
```

## Core Concepts

### Config-Driven Model

Everything in Noda is defined through JSON configuration files. There is no application code to write for standard patterns — routes, workflows, middleware, auth, services, and real-time connections are all configured declaratively.

The config directory contains:

| Directory | Purpose |
|-----------|---------|
| `noda.json` | Root config: server settings, services, security, middleware presets |
| `routes/` | HTTP route definitions mapping URLs to workflows |
| `workflows/` | Workflow DAGs — the core logic |
| `workers/` | Event-driven worker subscriptions |
| `schedules/` | Cron job definitions |
| `connections/` | WebSocket and SSE endpoint definitions |
| `schemas/` | JSON Schema definitions for validation |
| `tests/` | Workflow test suites |
| `migrations/` | SQL migration files |
| `wasm/` | Wasm modules |

### Workflows

Workflows are directed acyclic graphs (DAGs) of **nodes** connected by **edges**. Each node performs one operation — query a database, transform data, make a decision, send a response. Edges define the execution flow between nodes.

```json
{
  "id": "get-user",
  "nodes": {
    "fetch": {
      "type": "db.query",
      "services": { "database": "postgres" },
      "config": {
        "query": "SELECT * FROM users WHERE id = $1",
        "params": ["{{ input.id }}"]
      }
    },
    "check": {
      "type": "control.if",
      "config": {
        "condition": "{{ len(nodes.fetch) > 0 }}"
      }
    },
    "respond": {
      "type": "response.json",
      "config": {
        "status": 200,
        "body": "{{ nodes.fetch[0] }}"
      }
    },
    "not-found": {
      "type": "response.error",
      "config": {
        "status": 404,
        "code": "NOT_FOUND",
        "message": "User not found"
      }
    }
  },
  "edges": [
    { "from": "fetch", "to": "check" },
    { "from": "check", "output": "then", "to": "respond" },
    { "from": "check", "output": "else", "to": "not-found" }
  ]
}
```

Nodes execute in topological order. Nodes with no dependency between them run in parallel automatically.

### Plugins and Services

Plugins provide **node types** (operations) and **services** (connections to external systems). Services are configured in `noda.json` and referenced by nodes via service slots.

```json
{
  "services": {
    "postgres": {
      "plugin": "db",
      "config": {
        "driver": "postgres",
        "dsn": "{{ $env('DATABASE_URL') }}"
      }
    },
    "redis": {
      "plugin": "cache",
      "config": {
        "addr": "{{ $env('REDIS_URL') }}"
      }
    }
  }
}
```

A node references a service through its `services` field:

```json
{
  "type": "db.query",
  "services": { "database": "postgres" },
  "config": { ... }
}
```

### Expressions

Noda uses `{{ }}` expression syntax powered by [expr-lang/expr](https://expr-lang.org/). Expressions can access:

- **`input`** — data passed to the workflow from the trigger
- **`auth`** — authentication data (user_id, roles, claims)
- **`trigger`** — trigger metadata (type, timestamp, trace_id)
- **`nodes.<id>`** — output data from previously executed nodes
- **`$item`**, **`$index`** — loop iteration variables (inside `control.loop`)

Built-in functions: `len()`, `lower()`, `upper()`, `now()`, `$uuid()`, `$env()`.

### Expressions in Config Fields

Most config fields accept expressions:

```json
{
  "body": "{{ nodes.fetch[0] }}",
  "message": "Hello, {{ input.name }}!",
  "condition": "{{ auth.roles contains 'admin' }}"
}
```

Some fields are **static** (never expressions): `mode`, `cases`, `workflow`, `method`, `type`, `backoff`.

## Tutorial: Build a Task API

This tutorial walks through building a complete CRUD API for managing tasks.

### 1. Initialize the Project

```bash
noda init task-api
cd task-api
```

### 2. Configure Services

Edit `noda.json`:

```json
{
  "server": {
    "port": 3000
  },
  "services": {
    "postgres": {
      "plugin": "db",
      "config": {
        "driver": "postgres",
        "dsn": "{{ $env('DATABASE_URL') }}"
      }
    }
  }
}
```

### 3. Define the Task Schema

Create `schemas/Task.json`:

```json
{
  "Task": {
    "type": "object",
    "properties": {
      "id": { "type": "integer" },
      "title": { "type": "string" },
      "completed": { "type": "boolean" },
      "created_at": { "type": "string", "format": "date-time" }
    }
  },
  "CreateTask": {
    "type": "object",
    "properties": {
      "title": { "type": "string", "minLength": 1 }
    },
    "required": ["title"]
  }
}
```

### 4. Create the "List Tasks" Route and Workflow

`routes/list-tasks.json`:

```json
{
  "id": "list-tasks",
  "method": "GET",
  "path": "/api/tasks",
  "summary": "List all tasks",
  "trigger": {
    "workflow": "list-tasks",
    "input": {}
  }
}
```

`workflows/list-tasks.json`:

```json
{
  "id": "list-tasks",
  "name": "List Tasks",
  "nodes": {
    "fetch": {
      "type": "db.query",
      "services": { "database": "postgres" },
      "config": {
        "query": "SELECT * FROM tasks ORDER BY created_at DESC"
      }
    },
    "respond": {
      "type": "response.json",
      "config": {
        "status": 200,
        "body": "{{ nodes.fetch }}"
      }
    }
  },
  "edges": [
    { "from": "fetch", "to": "respond" }
  ]
}
```

### 5. Create the "Create Task" Route and Workflow

`routes/create-task.json`:

```json
{
  "id": "create-task",
  "method": "POST",
  "path": "/api/tasks",
  "summary": "Create a task",
  "body": {
    "schema": { "$ref": "schemas/Task#CreateTask" }
  },
  "trigger": {
    "workflow": "create-task",
    "input": {
      "title": "{{ body.title }}"
    }
  }
}
```

`workflows/create-task.json`:

Since the route defines `body.schema`, the request body is validated automatically before the workflow runs. Invalid requests get a `422` response without reaching the workflow. No `transform.validate` node needed.

```json
{
  "id": "create-task",
  "name": "Create Task",
  "nodes": {
    "insert": {
      "type": "db.create",
      "services": { "database": "postgres" },
      "config": {
        "table": "tasks",
        "data": {
          "title": "{{ input.title }}",
          "completed": false
        }
      }
    },
    "respond": {
      "type": "response.json",
      "config": {
        "status": 201,
        "body": "{{ nodes.insert }}"
      }
    }
  },
  "edges": [
    { "from": "insert", "to": "respond" }
  ]
}
```

### 6. Write a Test

`tests/test-create-task.json`:

```json
{
  "id": "test-create-task",
  "workflow": "create-task",
  "tests": [
    {
      "name": "creates a task successfully",
      "input": { "title": "Buy groceries" },
      "mocks": {
        "insert": {
          "output": { "id": 1, "title": "Buy groceries", "completed": false }
        }
      },
      "expect": {
        "status": "success"
      }
    }
  ]
}
```

### 7. Validate and Test

```bash
noda validate
noda test --verbose
```

### 8. Start with Docker Compose

Create a `docker-compose.yml`:

```yaml
services:
  noda:
    image: ghcr.io/your-org/noda:latest
    ports:
      - "3000:3000"
    volumes:
      - .:/app/config
    environment:
      - DATABASE_URL=postgres://noda:noda@postgres:5432/noda?sslmode=disable
    depends_on:
      - postgres

  postgres:
    image: postgres:16-alpine
    environment:
      POSTGRES_USER: noda
      POSTGRES_PASSWORD: noda
      POSTGRES_DB: noda
    ports:
      - "5432:5432"
```

```bash
docker compose up
```

Your API is now running at `http://localhost:3000`.

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

## Next Steps

- [Config Reference](config-reference.md) — all config file formats and fields
- [Node Reference](node-reference.md) — all 46 node types with examples
- [Plugin Author Guide](plugin-author-guide.md) — build custom plugins
- [Wasm Developer Guide](wasm-developer-guide.md) — build Wasm modules
- [Deployment Guide](deployment-guide.md) — production deployment
