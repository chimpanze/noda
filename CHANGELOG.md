# Changelog

All notable changes to Noda will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
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
- Dockerfile: non-root user, HEALTHCHECK directive, embedded editor build, version metadata via ldflags
- WebSocket/SSE connections are now gracefully closed during shutdown

### Fixed
- Data race in workflow test runner trace callback (concurrent map access from parallel nodes)
- Health endpoint documentation now matches actual paths (`/health/live`, `/health/ready`, `/health`)
- Deployment docs corrected for `sampling_rate` config field name

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
