# SaaS Backend Example

A multi-tenant project management backend demonstrating Noda's full plugin ecosystem: RBAC with Casbin, webhook integrations (GitHub, Stripe), event-driven workers, file uploads with image thumbnails, and email notifications.

## Features Exercised

- **Multi-tenant RBAC** — Casbin with workspace-scoped policies (owner, admin, member, viewer)
- **Middleware presets + route groups** — workspace auth applied to all nested routes
- **Webhook ingestion** — GitHub with HMAC signature verification (`hmac_verify` + `raw_body`); Stripe events accepted unverified (Stripe's `t=...,v1=...` scheme needs timestamp parsing — out of scope for this example)
- **Event-driven workers** — email notifications, thumbnail generation, GitHub issue sync
- **File uploads** — `upload.handle` with storage service, size/type validation
- **Image processing** — thumbnail generation in worker with source/target storage
- **Email service** — invitation notification delivery
- **Control flow** — `control.if` for conditional logic, `control.switch` for event routing
- **Dead letter queues** — failed worker messages go to DLQ after 3 retries

## Endpoints

| Method | Path | Description |
|--------|------|-------------|
| POST | `/api/workspaces` | Create a workspace |
| GET | `/api/workspaces` | List user's workspaces |
| POST | `/api/workspaces/:workspace_id/projects` | Create a project |
| GET | `/api/workspaces/:workspace_id/projects` | List projects |
| POST | `/api/workspaces/:workspace_id/tasks` | Create a task |
| GET | `/api/workspaces/:workspace_id/tasks` | List tasks (filterable by project_id) |
| PUT | `/api/workspaces/:workspace_id/tasks/:task_id` | Update a task |
| POST | `/api/workspaces/:workspace_id/members` | Invite a member |
| POST | `/api/workspaces/:workspace_id/projects/:project_id/attachments` | Upload attachment |
| POST | `/webhooks/github` | GitHub webhook receiver |
| POST | `/webhooks/stripe` | Stripe webhook receiver |

## Architecture

```
HTTP Request
  │
  ├── /api/workspaces/:workspace_id/* ──→ [auth.jwt] → [casbin.enforce] → workflow
  ├── /api/workspaces ──────────────────→ [auth.jwt] → workflow
  └── /webhooks/* ──────────────────────→ workflow (no auth)
                                              │
                                              ├── event.emit ──→ Redis Stream
                                              │                      │
                                              │                 Workers:
                                              │                   ├── send-notification (member.invited)
                                              │                   ├── generate-thumbnail (attachment.uploaded)
                                              │                   └── sync-github-issue (github.issue)
                                              │
                                              └── response.json ──→ HTTP Response
```

## Database Schema

```sql
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

CREATE TABLE workspaces (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    name TEXT NOT NULL,
    slug TEXT NOT NULL UNIQUE,
    owner_id TEXT NOT NULL,
    stripe_customer_id TEXT,
    subscription_status TEXT DEFAULT 'trial',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE workspace_members (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    workspace_id UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    user_id TEXT,
    email TEXT NOT NULL,
    role TEXT NOT NULL CHECK (role IN ('owner', 'admin', 'member', 'viewer')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE projects (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    workspace_id UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    description TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE tasks (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    workspace_id UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    title TEXT NOT NULL,
    description TEXT,
    status TEXT NOT NULL DEFAULT 'todo' CHECK (status IN ('todo', 'in_progress', 'done')),
    assignee_id TEXT,
    github_issue_id TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE attachments (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    workspace_id UUID NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    filename TEXT NOT NULL,
    path TEXT NOT NULL,
    content_type TEXT NOT NULL,
    size INTEGER NOT NULL,
    thumbnail_path TEXT,
    uploaded_by TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
```

## Docker Setup

```bash
cp .env.example .env   # required — validate/test/start fail without it
docker compose up -d
```

Then create the database tables:

```bash
docker compose exec postgres psql -U noda -d noda -f /dev/stdin <<'SQL'
-- Paste the SQL schema above
SQL
```

## Environment Variables

| Variable | Description | Example |
|----------|-------------|---------|
| `DATABASE_URL` | PostgreSQL connection string | `postgres://noda:noda@postgres:5432/noda?sslmode=disable` |
| `REDIS_URL` | Redis connection string | `redis://redis:6379` |
| `JWT_SECRET` | JWT signing secret | `your-secret-key` |
| `SMTP_HOST` | SMTP server host | `smtp.example.com` |
| `SMTP_PORT` | SMTP server port | `587` |
| `SMTP_FROM` | Sender email address | `noreply@example.com` |
| `GITHUB_WEBHOOK_SECRET` | GitHub webhook HMAC secret | `dev-webhook-secret` |

## Testing

Validate all configuration files:

```bash
go run ./cmd/noda validate --config examples/saas-backend
```

Run workflow tests:

```bash
go run ./cmd/noda test --config examples/saas-backend --verbose
```

## Try the GitHub webhook

```bash
export GITHUB_WEBHOOK_SECRET=dev-webhook-secret   # must match .env
payload='{"action":"opened","issue":{"id":12345,"title":"Bug report","state":"open"}}'
sig=$(printf '%s' "$payload" | openssl dgst -sha256 -hmac "$GITHUB_WEBHOOK_SECRET" | awk '{print $NF}')

curl -i -X POST http://localhost:3000/webhooks/github \
  -H 'Content-Type: application/json' \
  -H 'x-github-event: issues' \
  -H "x-hub-signature-256: sha256=$sig" \
  -d "$payload"
# → 200 {"status":"ok"}; a wrong/missing signature → 401 INVALID_SIGNATURE
```

## Workers

Each worker subscribes to a single Redis Stream topic and processes events asynchronously:

| Worker | Topic | Purpose |
|--------|-------|---------|
| `send-notification` | `member.invited` | Sends invitation emails |
| `generate-thumbnail` | `attachment.uploaded` | Generates 200x200 thumbnails for images |
| `sync-github-issue` | `github.issue` | Creates/closes tasks from GitHub issues |

To scale to additional notification topics (e.g., `task.assigned`, `payment.received`), add separate worker files for each topic — the worker runtime supports one topic per worker.
