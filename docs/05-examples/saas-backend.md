# Noda — Use Case: SaaS Backend

**Version**: 0.4.0

A multi-tenant project management SaaS with team workspaces, role-based access, webhook integrations (GitHub, Stripe), background job processing, email notifications, and file attachments with image thumbnails. This validates the full plugin ecosystem and event-driven architecture.

---

## What We're Building

A backend for a project management tool (think lightweight Jira):

- **Multi-tenant workspaces** — each workspace has its own projects, members, and roles
- **RBAC** — owner, admin, member, viewer roles per workspace
- **Webhook ingestion** — accept GitHub webhooks (issue events) and Stripe webhooks (payment events)
- **Background processing** — send email notifications, generate reports, sync external data
- **File attachments** — upload files to projects, generate image thumbnails
- **REST API** — standard CRUD for workspaces, projects, tasks, members

---

## Services Required

| Instance | Plugin | Purpose |
|---|---|---|
| `main-db` | `postgres` | Application data |
| `app-cache` | `cache` | Session data, rate limiting state |
| `main-stream` | `stream` | Durable job queue for workers |
| `uploads` | `storage` | File attachments (S3) |
| `thumbnails` | `storage` | Generated thumbnails (S3) |
| `mailer` | `email` | Notification emails |

---

## Config Structure

```
noda.json                 — services, JWT, Casbin model, middleware presets
noda.production.json      — production URLs, secrets
schemas/
  Workspace.json
  Project.json
  Task.json
  Member.json
routes/
  workspaces.json         — CRUD + member management
  projects.json           — CRUD + file attachments
  tasks.json              — CRUD + assignment
  webhooks.json           — GitHub + Stripe ingestion
workers/
  send-notification.json
  generate-thumbnail.json
  sync-github-issue.json
workflows/
  create-workspace.json
  invite-member.json
  upload-attachment.json
  handle-github-webhook.json
  handle-stripe-webhook.json
  send-notification.json
  generate-thumbnail.json
  sync-github-issue.json
```

---

## Key Workflows

### Multi-Tenant Authorization

Casbin model uses `{subject, workspace, object, action}` — "Can user X in workspace Y perform action Z on resource W?"

The middleware chain for workspace routes:

1. `auth.jwt` — validates token, populates `$.auth`
2. `casbin.enforce` — checks the workspace-scoped policy

Route groups organize this:

```json
{
  "route_groups": {
    "/api/workspaces/:workspace_id": {
      "middleware_preset": "workspace_auth"
    }
  }
}
```

The `workspace_auth` preset includes JWT + Casbin. Every route under `/api/workspaces/:workspace_id/...` inherits it.

**Features exercised:** Casbin RBAC, middleware presets, route groups, tenant-scoped authorization.

### GitHub Webhook Ingestion

**Trigger:** `POST /webhooks/github` → workflow `handle-github-webhook`

**Route config:**
```json
{
  "trigger": {
    "workflow": "handle-github-webhook",
    "raw_body": true,
    "input": {
      "event_type": "{{ request.headers['X-GitHub-Event'] }}",
      "signature": "{{ request.headers['X-Hub-Signature-256'] }}",
      "payload": "{{ request.body }}"
    }
  }
}
```

**Nodes:**

1. `transform.validate` — verify HMAC signature using `{{ trigger.raw_body }}` against `{{ input.signature }}`
2. `control.switch` — branch on `{{ input.event_type }}`
   - `"issues"` → `event.emit` to stream (topic: `github.issue`)
   - `"pull_request"` → `event.emit` to stream (topic: `github.pr`)
   - `default` → `util.log` (ignore unknown events)
3. `response.json` — return 200 immediately

The response fires after emitting the event — the HTTP response goes back to GitHub fast. The actual processing happens in workers.

**Features exercised:** `raw_body` for signature verification, `control.switch` with multiple branches, `event.emit` to stream, early HTTP response with async continuation.

### Stripe Webhook with Payment Processing

**Trigger:** `POST /webhooks/stripe` → workflow `handle-stripe-webhook`

Same pattern as GitHub — `raw_body: true`, signature verification, then branch on event type.

For `invoice.paid`, the workflow:

1. `control.switch` on event type
2. `db.query` — look up workspace by Stripe customer ID
3. `db.update` — update subscription status
4. `event.emit` — emit `payment.received` to stream for notification worker

**Features exercised:** Webhook signature verification, database operations, event-driven notification pipeline.

### File Upload with Thumbnail Generation

**Trigger:** `POST /api/workspaces/:id/projects/:pid/attachments` → workflow `upload-attachment`

**Route config:**
```json
{
  "trigger": {
    "workflow": "upload-attachment",
    "input": {
      "workspace_id": "{{ request.params.workspace_id }}",
      "project_id": "{{ request.params.pid }}",
      "file": "{{ request.file }}"
    },
    "files": ["file"]
  }
}
```

**Nodes:**

1. `upload.handle` — validate (10MB max, image/pdf types), stream to `uploads` storage
2. `db.create` — insert attachment record with file metadata
3. `event.emit` — emit `attachment.uploaded` to stream for thumbnail worker
4. `response.json` — return 201 with attachment record

**Worker:** `generate-thumbnail` subscribes to `attachment.uploaded`:

1. `storage.read` — read original from `uploads`
2. `control.if` — is it an image? (check content_type)
   - `then` → `image.thumbnail` — generate 200x200 thumbnail, write to `thumbnails` storage, `db.update` attachment record with thumbnail path
   - `else` → skip (PDFs etc. get no thumbnail)

**Features exercised:** File stream handling (`files` array), `upload.handle` with storage service, event emission for async processing, worker consuming stream events, image processing with source/target storage, conditional logic in worker workflow.

### Email Notification Pipeline

**Worker:** `send-notification` subscribes to multiple topics: `member.invited`, `task.assigned`, `payment.received`.

**Nodes:**

1. `control.switch` on `{{ input.topic }}`
2. Per branch: `db.query` to load user/workspace data, then `transform.set` to build email content
3. `email.send` with the built template

**Features exercised:** Worker middleware (logging, timeout), topic-based routing, email service, sub-workflow reuse (the notification workflow is invoked by multiple producers).

---

## Architecture Features Validated

| Feature | How it's used |
|---|---|
| Multi-tenant RBAC | Casbin with workspace-scoped policies |
| Middleware presets + groups | Workspace auth applied to all nested routes |
| `raw_body` | Webhook signature verification (GitHub, Stripe) |
| `control.switch` | Route webhook events by type |
| Event system (stream) | Decouple HTTP responses from background processing |
| Workers | Email notifications, thumbnail generation, GitHub sync |
| Dead letter queues | Failed notifications go to dead letter topic |
| File uploads | `files` array + `upload.handle` + storage service |
| Image processing | Thumbnail generation in worker with source/target storage |
| Email service | Notification delivery |
| Multiple storage instances | `uploads` (originals) vs `thumbnails` (generated) |
| Early HTTP response | Webhook endpoints return 200 immediately, processing continues async |
| Sub-workflow reuse | Notification workflow used by multiple event sources |
| Environment overlays | Production secrets in `noda.production.json` |

---

## What's NOT Needed

No WebSockets, no Wasm, no SSE, no scheduler (could add for report generation cron). This is a classic HTTP + workers architecture.
