# Changelog

All notable changes to Noda will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Security
- Edge & trace hardening: DB conflict/unavailable error bodies no longer leak driver/constraint detail in production (detail gated behind dev mode); trace redaction now covers slice-typed node data (e.g. `db.query` rows) and `stream_key`; the dev `/ws/trace` endpoint rejects cross-origin connections; `response.redirect` rejects `/\`-authority open redirects; `ws.send`/`sse.send` (and the Wasm host connection API) reject wildcard channels — **broadcasting via a wildcard send is no longer supported; subscribe connections to a shared literal channel instead**; `image.resize`/`crop`/`thumbnail` cap output dimensions.

### Added
- `auth.jwt` optional claim validation: `audience`, `issuer`, and `require_expiry` (all default off — when unset, behavior is unchanged)
- Prometheus metrics endpoint (`/metrics`) with OTel metrics API
- Trace sampling rate configuration (`observability.tracing.sampling_rate`)
- Circuit breaker support for outbound HTTP services (`circuit_breaker` config)
- Per-workflow timeout configuration (`timeout` field in workflow definitions)
- Redis-backed distributed rate limiting (`storage: "redis"` in limiter config)
- JWT support for RSA (RS256/384/512) and ECDSA (ES256/384/512) algorithms
- Request ID propagation from HTTP middleware through workflow execution and logs
- Secrets redaction in trace events and log output
- Idempotency middleware with Redis backend
- Health check timeout (default 5s) to prevent hung service checks
- `gosec` security linter with tuned exclusion rules
- CHANGELOG.md

### Changed
- Route-group middleware now resolves **deterministically**: overlapping group prefixes (e.g. `/api` and `/api/admin`) **merge** their middleware (outermost-first, deduped) instead of one winning at random, and prefix matching is path-segment aware (`/api` no longer matches `/api-docs`). **Upgrade note:** a config that nested groups with a cross-group ordering conflict (e.g. a parent group placing `casbin.enforce` before a child group's `auth.jwt`) previously booted non-deterministically but now fails fast at route registration with a clear ordering error — reorder the affected groups to fix.
- Dockerfile: non-root user, HEALTHCHECK directive, embedded editor build, version metadata via ldflags
- WebSocket/SSE connections are now gracefully closed during shutdown
- Worker reaper polls at `retry.min_idle / 2` (30s floor) instead of a fixed 30s, and fetches delivery counts for each reclaimed page in one `XPENDING` call instead of one per failed message — fewer idle Redis scans, same redelivery semantics

### Fixed
- Worker process no longer crashes when a message handler panics inside the timeout middleware's goroutine; the panic is recovered and surfaced as an error
- Worker consumers survive a panic in pre-handler setup (deserialization, input mapping, middleware construction) instead of permanently losing a consumer goroutine
- Wasm gateway reconnect no longer resurrects a torn-down outbound WebSocket when a close races with an in-flight reconnect; also fixed a data race between the heartbeat loop and reconnect's reassignment of the connection stop channel
- WebSocket broadcast no longer head-of-line-blocks: each connection has a bounded outbound queue with a write deadline, so one slow client can't stall the whole channel
- Data race in workflow test runner trace callback (concurrent map access from parallel nodes)
- Health endpoint documentation now matches actual paths (`/health/live`, `/health/ready`, `/health`)
- Deployment docs corrected for `sampling_rate` config field name
- Worker now reclaims idle pending messages via XAutoClaim, so failed messages are actually redelivered and dead-letter (`dead_letter.after`) and retry limits are enforced (previously pending messages were never re-processed).
- Worker pre-handler panics are now retried and dead-lettered/dropped through the normal disposition instead of being stranded in the pending-entries list (#243); panic errors now include a stack trace.
- New worker `retry` config (`min_idle`, `max_attempts`); without a `dead_letter` topic a repeatedly-failing message is dropped with a loud error after `max_attempts`.
- Worker ack/dead-letter disposition runs on a fresh context, so a message that fails by exhausting the handler timeout is still counted, dead-lettered, or dropped instead of retrying forever.
- Worker `dead_letter.after` defaults to `retry.max_attempts` when omitted, so a topic-only `dead_letter` config dead-letters poison messages instead of silently dropping them; an empty `dead_letter.topic` disables dead-lettering with an ERROR log instead of publishing to a stream named `""`.
- Legacy worker `retry.dlq` (documented pre-reclaim) is honored as an alias for `dead_letter.topic` instead of being silently ignored.
- Worker `min_idle` is clamped to `timeout` + 30s margin (not exactly `timeout`), so the reaper cannot reclaim a message whose consumer is still finishing or acknowledging.
- Worker reaper processes reclaimed messages with the worker's `concurrency` instead of serially, so one slow poison message no longer head-of-line-blocks redelivery.

### Removed
- Stream plugin consume-side API (`Subscribe`, `Ack`, `PendingCount`): unused by the platform (workers consume streams directly) and its hardcoded 60s reclaim conflicted with the worker reaper's timeout-clamped policy. `Publish` and the `pkg/api.StreamService` contract are unchanged.

## [1.0.0] - 2026-03-18

### Added
- Configuration-driven API runtime with JSON config files
- 72 built-in node types across 15 categories
- Visual editor (React + React Flow) with embedded serving
- Workflow engine with parallel execution, retry, and eviction
- Expression language with 50+ built-in functions
- Plugin system: PostgreSQL, Redis (cache/streams/pubsub), HTTP, email, storage, image processing
- WebSocket and SSE real-time connection management
- Worker runtime with Redis Streams consumer groups and dead letter queue
- Scheduler runtime with cron expressions and distributed locking
- Wasm module runtime via Extism (wazero)
- JWT, OIDC, Casbin RBAC, CORS, CSRF, rate limiting, helmet middleware
- OpenTelemetry distributed tracing with OTLP export
- Database migration management with up/down SQL files
- OpenAPI 3.1.0 spec generation from config
- Graceful lifecycle management with ordered startup/shutdown
- Dev mode with hot reload and live trace WebSocket
- Load testing with k6 scenarios
- 80.8% test coverage across all packages
- LiveKit integration for real-time audio/video
- MCP server integration
