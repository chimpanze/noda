# Deployment Guide

This guide covers deploying Noda to production environments.

## Docker Deployment

### Single Instance with Docker Compose

```yaml
services:
  noda:
    image: ghcr.io/your-org/noda:latest
    ports:
      - "3000:3000"
      - "3001:3001"    # Dev trace WebSocket (remove in production)
    volumes:
      - ./config:/app/config
      - ./wasm:/app/wasm
    environment:
      - NODA_ENV=production
      - DATABASE_URL=postgres://noda:noda@postgres:5432/noda?sslmode=disable
      - REDIS_URL=redis:6379
      - JWT_SECRET=your-secret-here
    depends_on:
      postgres:
        condition: service_healthy
      redis:
        condition: service_healthy
    healthcheck:
      test: ["CMD", "curl", "-f", "http://localhost:3000/health"]
      interval: 10s
      timeout: 5s
      retries: 3

  postgres:
    image: postgres:16-alpine
    environment:
      POSTGRES_USER: noda
      POSTGRES_PASSWORD: noda
      POSTGRES_DB: noda
    volumes:
      - postgres_data:/var/lib/postgresql/data
    ports:
      - "5432:5432"
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U noda"]
      interval: 5s
      timeout: 5s
      retries: 5

  redis:
    image: redis:7-alpine
    ports:
      - "6379:6379"
    volumes:
      - redis_data:/data
    healthcheck:
      test: ["CMD", "redis-cli", "ping"]
      interval: 5s
      timeout: 5s
      retries: 5

volumes:
  postgres_data:
  redis_data:
```

### Dockerfile

Use the repository's `Dockerfile` — it is a four-stage build:

1. A Node stage builds the embedded editor assets.
2. A `golang:1.26-bookworm` builder compiles the binary, controlled by a `VARIANT` build-arg: `full` (cgo, libvips → `image.*` nodes work) or `slim` (static, no image processing).
3. The `full` runtime is `debian:bookworm-slim` with libvips; the `slim` runtime is `gcr.io/distroless/static-debian12`.
4. Both run as a non-root user and declare a `HEALTHCHECK` against `/health/live`.

```bash
docker build --build-arg VARIANT=slim -t my-noda-app .
# or VARIANT=full if you use image.* nodes
```

Prebuilt images are published to GHCR by the tag-triggered `docker.yml` workflow. Release binaries (Linux amd64/arm64, macOS, Windows) come from the `release.yml` matrix on `v*` tags — the `.goreleaser.yaml` in the repo is **not** the active release path.

## Environment Variables

All environment variables can be referenced in config via `$env()`:

```json
{ "url": "{{ $env('DATABASE_URL') }}" }
```

### Core Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `NODA_ENV` | Environment name (loads overlay config; the `--env` flag wins when set) | `development` |

There is no env-var override for the config directory or the port: the config directory comes from the `--config` flag (default `.`), and the port from `server.port` in `noda.json` (which may itself read an env var via `"port": "{{ $env('PORT') }}"`).

### Service Variables (typical)

| Variable | Description |
|----------|-------------|
| `DATABASE_URL` | PostgreSQL connection string |
| `REDIS_URL` | Redis address (host:port) |
| `JWT_SECRET` | JWT signing secret |
| `SMTP_HOST` | SMTP server host |
| `SMTP_PORT` | SMTP server port |
| `SMTP_USER` | SMTP username |
| `SMTP_PASS` | SMTP password |
| `S3_ENDPOINT` | S3-compatible storage endpoint |
| `S3_ACCESS_KEY` | S3 access key |
| `S3_SECRET_KEY` | S3 secret key |

### .env File

Noda auto-loads `.env` files from the config directory. Variables defined there are available via `$env()`.

## Multiple Instance Deployment

Noda supports horizontal scaling. Key considerations:

### Load Balancing

Place a load balancer (nginx, Traefik, AWS ALB) in front of multiple Noda instances:

```yaml
services:
  nginx:
    image: nginx:alpine
    ports:
      - "80:80"
    volumes:
      - ./nginx.conf:/etc/nginx/nginx.conf

  noda-1:
    image: ghcr.io/your-org/noda:latest
    environment: &noda-env
      DATABASE_URL: postgres://noda:noda@postgres:5432/noda?sslmode=disable
      REDIS_URL: redis:6379

  noda-2:
    image: ghcr.io/your-org/noda:latest
    environment: *noda-env

  noda-3:
    image: ghcr.io/your-org/noda:latest
    environment: *noda-env
```

### WebSocket Sticky Sessions

If using WebSocket connections, configure sticky sessions on the load balancer so clients stay connected to the same instance. Noda uses Redis PubSub for cross-instance WebSocket message distribution, so messages reach all connected clients regardless of which instance they're on.

### Distributed Locking

Noda uses Redis-based distributed locking for:

- **Scheduler** — ensures cron jobs run on exactly one instance
- **Workers** — Redis Streams consumer groups automatically distribute messages across instances

No additional configuration is needed — this works out of the box when multiple instances share the same Redis.

### Worker Scaling

Workers use Redis Streams consumer groups. When you scale to N instances, messages are automatically distributed across all instances. Each worker config's `concurrency` setting controls per-instance parallelism:

```json
{
  "concurrency": 5
}
```

With 3 instances and `concurrency: 5`, you get up to 15 concurrent message processors for that worker.

## Health Check Endpoints

Noda exposes health check endpoints:

| Endpoint | Purpose | Success | Failure |
|----------|---------|---------|---------|
| `GET /health/live` | Liveness probe — is the process running? | `200 OK` | — |
| `GET /health/ready` | Readiness probe — are all services initialized? | `200 OK` | `503` |
| `GET /health` | Deep check — pings all registered services | `200 OK` | `503` |

### Kubernetes Probes

```yaml
livenessProbe:
  httpGet:
    path: /health/live
    port: 3000
  initialDelaySeconds: 5
  periodSeconds: 10

readinessProbe:
  httpGet:
    path: /health/ready
    port: 3000
  initialDelaySeconds: 10
  periodSeconds: 5
```

## Observability

### OpenTelemetry

Noda has built-in OpenTelemetry support for traces. Tracing is **off** until `enabled: true` is set:

```json
{
  "observability": {
    "tracing": {
      "enabled": true,
      "exporter": "otlp",
      "endpoint": "http://jaeger:4318",
      "sampling_rate": 1.0
    }
  }
}
```

Recognized fields are `enabled`, `exporter`, `endpoint`, `insecure`, and `sampling_rate`. The service name in traces is always `noda` — there is no `service_name` config field, and `OTEL_SERVICE_NAME` is not read.

### What Gets Traced

- Every HTTP request gets a trace with the request's trace ID
- Each workflow execution is a span
- Each node execution is a child span carrying `node.id`, `node.type`, the fired output name, and duration (node config and input values appear only in dev-mode trace events, not on OTel spans)
- Worker message processing is traced end-to-end
- Scheduled job executions are traced

### Jaeger / Tempo Setup

Add Jaeger to your Docker Compose for local trace viewing:

```yaml
services:
  jaeger:
    image: jaegertracing/all-in-one:latest
    ports:
      - "16686:16686"   # UI
      - "4318:4318"     # OTLP HTTP
    environment:
      - COLLECTOR_OTLP_ENABLED=true
```

### Prometheus Metrics

Noda exposes Prometheus-compatible metrics at `/metrics` (when enabled in config):

```json
{
  "observability": {
    "metrics": {
      "enabled": true,
      "path": "/metrics"
    }
  }
}
```

Available metrics:

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `http_request_duration_seconds` | Histogram | method, route, status | HTTP request latency (seconds) |
| `http_requests_total` | Counter | method, route, status | Total HTTP requests |
| `http_errors_total` | Counter | method, route, status, error_type | Total HTTP errors |
| `workflow_duration_seconds` | Histogram | workflow_id, status | Workflow execution latency (seconds) |
| `workflow_executions_total` | Counter | workflow_id, status | Total workflow executions |
| `workflow_errors_total` | Counter | workflow_id, error_type | Total failed workflows |
| `node_duration_seconds` | Histogram | node_type, status | Node execution latency (seconds) |
| `node_errors_total` | Counter | node_type, error_type | Total node errors |
| `connections_active` | UpDownCounter | type (ws/sse) | Current WebSocket/SSE connections |
| `panics_recovered_total` | Counter | source | Recovered panics |

## Database Migrations

Run migrations before starting the server:

```bash
noda migrate --config ./config
```

Or in Docker Compose, run as an init container:

```yaml
services:
  migrate:
    image: ghcr.io/your-org/noda:latest
    command: ["migrate", "--config", "/app/config"]
    volumes:
      - ./config:/app/config
    environment:
      - DATABASE_URL=postgres://noda:noda@postgres:5432/noda?sslmode=disable
    depends_on:
      postgres:
        condition: service_healthy

  noda:
    depends_on:
      migrate:
        condition: service_completed_successfully
```

Migration files are timestamped SQL files in `migrations/`:

```
migrations/
├── 20240101000000_create_tasks.up.sql
├── 20240101000000_create_tasks.down.sql
├── 20240102000000_add_user_id.up.sql
└── 20240102000000_add_user_id.down.sql
```

## Redis Configuration

Redis-backed services (cache, stream, pubsub) accept a `url` field using the standard Redis URL format, plus optional connection pool settings:

```json
{
  "services": {
    "redis": {
      "plugin": "cache",
      "config": {
        "url": "{{ $env('REDIS_URL') }}",
        "pool_size": 20,
        "min_idle": 5
      }
    }
  }
}
```

| Field | Description | Default |
|-------|-------------|---------|
| `url` | Redis URL (`redis://user:pass@host:6379/0`) | required |
| `pool_size` | Maximum connections in the pool | go-redis default (10 per CPU) |
| `min_idle` | Minimum idle connections kept open | go-redis default (0) |

For production workloads, tune `pool_size` based on your concurrency needs. A good starting point is 2x your expected concurrent database operations. Set `min_idle` to reduce connection establishment latency under load.

## Production Checklist

- [ ] Set `NODA_ENV=production` to load production config overlay
- [ ] Use strong, unique `JWT_SECRET`
- [ ] Configure PostgreSQL with connection pooling (PgBouncer)
- [ ] Configure Redis with persistence (AOF or RDB)
- [ ] Set appropriate `body_limit` for your use case
- [ ] Configure CORS for your frontend domain
- [ ] Enable rate limiting on public endpoints
- [ ] Set up health check monitoring
- [ ] Configure OTLP tracing export
- [ ] Run `noda migrate` before deploying new versions
- [ ] Use volume mounts for persistent storage (uploads, files)
- [ ] Remove dev trace port (3001) from production
- [ ] Set up log aggregation (stdout/stderr to your logging platform)
- [ ] Configure backup strategy for PostgreSQL and Redis
- [ ] Use TLS termination at the load balancer
- [ ] Apply rate limiting to authentication endpoints (see Security Hardening below)
- [ ] Enable CSRF protection on state-changing endpoints if serving browser clients

## Security Hardening

### Rate Limiting on Auth Endpoints

Authentication endpoints (login, token refresh, registration) are prime targets for brute-force and credential-stuffing attacks. Apply strict rate limiting to these routes using middleware presets:

```json
{
  "middleware_presets": {
    "auth_rate_limited": ["limiter.strict", "auth.jwt"]
  },
  "routes": [
    {
      "path": "/auth/login",
      "method": "POST",
      "middleware": "auth_rate_limited",
      "workflow": "auth.login"
    }
  ]
}
```

If you use `auth.jwt` or `auth.oidc` middleware without any `limiter` middleware on the same routes, consider adding one. The `limiter.strict` preset defaults to 10 requests per minute per IP.

### CSRF Protection

For applications that serve browser clients with cookie-based sessions, enable CSRF protection on state-changing endpoints (POST, PUT, DELETE):

```json
{
  "middleware_presets": {
    "protected": ["security.csrf", "auth.jwt"]
  }
}
```

CSRF protection is not enabled by default because API-only deployments (mobile apps, service-to-service) use token-based auth where CSRF is not applicable. If your API is consumed exclusively by non-browser clients, CSRF middleware is unnecessary.

## Outbound network policy

> **⚠️ Breaking change for existing deployments.** Workflows that
> previously used the `http` plugin to reach `localhost`,
> `127.0.0.1`, RFC1918 (`10.x`, `172.16-31.x`, `192.168.x`), or
> internal cluster IPs (`100.64.x`) will now fail with
> `netguard: destination denied by policy`. To re-enable, set
> `allow_private_networks: true` on the affected service config,
> or list specific hostnames in `allowed_hosts`. See the HTTP
> Client Service options in `docs/02-config/noda-json.md`.

The `http` plugin enforces an outbound-network policy on every request to
prevent SSRF. By default, requests resolved to any of these IP ranges are
denied:

- `127.0.0.0/8`, `::1/128` — loopback
- `169.254.0.0/16`, `fe80::/10` — link-local
- `10.0.0.0/8`, `172.16.0.0/12`, `192.168.0.0/16` — RFC1918 private
- `fc00::/7` — IPv6 unique-local
- `100.64.0.0/10` — RFC 6598 carrier-grade NAT
- `0.0.0.0`

Two cloud-metadata IPs are **always** denied, even when the policy below
opens private ranges:

- `169.254.169.254` — AWS, GCP, Azure, DigitalOcean, Oracle, IBM, OpenStack
- `100.100.100.200` — Alibaba

To call services on a private network, set `allow_private_networks: true`
on the HTTP service config, or list the specific hostnames in
`allowed_hosts`. See `docs/02-config/noda-json.md` for the full schema.

DNS rebinding is defeated by checking the resolved IP and dialing it
directly; the hostname is never re-resolved between check and dial.

`HTTP_PROXY` / `HTTPS_PROXY` / `NO_PROXY` environment variables are
intentionally not honoured. The runtime preserves the invariant that
"the IP we check is the IP we dial"; a forward proxy would shift the
SSRF check to the proxy itself.
