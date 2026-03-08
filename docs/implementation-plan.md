# Noda — Implementation Plan

**Version**: 0.4.0
**Status**: Planning

This document defines the implementation structure for Noda. Each milestone produces a testable, working system — not isolated components. Milestones build on each other in dependency order: nothing is built until its foundations are solid and tested.

---

## Guiding Principles

**Test from the bottom up.** Every package has unit tests. Every integration has integration tests. Every milestone ends with an end-to-end test that exercises the full feature through the public surface (HTTP request in, response out).

**One thing works before the next starts.** No parallel milestone development. Each milestone is complete — tested, reviewed, documented — before the next begins. This prevents the "everything is 80% done, nothing works" problem.

**Interfaces first, implementations second.** `pkg/api/` (the public interfaces) is defined and frozen before any plugin is written. This forces clean boundaries and prevents coupling.

**Real config files from day one.** Every milestone is validated against actual JSON config files, not hard-coded test structures. If the config format is wrong, we find out early.

**Docker Compose is always green.** From the first milestone onward, `docker compose up` starts a working system. The developer experience is never broken.

---

## Milestone 0: Project Skeleton

**What:** Go module, directory structure, CI pipeline, Docker setup.

**Deliverables:**
- Go module initialized with all dependency declarations
- Directory structure matching the architecture plan
- `Dockerfile` with multi-stage build (builder + runtime with libvips)
- `docker-compose.yml` with PostgreSQL and Redis
- CI pipeline: lint (golangci-lint), test, build
- Makefile with standard targets: `build`, `test`, `lint`, `dev`
- `pkg/api/` — all public interfaces defined (Plugin, NodeDescriptor, NodeExecutor, ExecutionContext, HTTPResponse, service interfaces, standard errors). No implementations yet — just the Go interfaces, types, and error definitions from the interfaces document.

**Tests:** CI pipeline runs, Docker builds succeed, `pkg/api/` compiles.

**Why first:** Everything depends on the interfaces and the build pipeline. Defining `pkg/api/` now means every subsequent milestone codes against a stable contract.

---

## Milestone 1: Config Loading

**What:** Load, validate, and merge JSON config files with environment overlays.

**Deliverables:**
- Config loader: discover files by directory convention (`routes/`, `workflows/`, `schemas/`, etc.)
- Environment overlay merging (`noda.json` + `noda.{env}.json`)
- `$ref` resolution for shared schemas
- `$env()` resolution for environment variables
- JSON Schema validation for every config type (route, workflow, worker, schedule, connections)
- Error reporting: file path, line reference, clear messages for every validation failure
- CLI skeleton with Cobra: `noda validate` command that loads and validates all config

**Tests:**
- Unit: config merging, `$ref` resolution, `$env()` resolution, overlay precedence
- Integration: load a complete sample project, validate all files, verify error messages for intentionally broken configs
- End-to-end: `noda validate` exits 0 for valid config, exits 1 with clear errors for invalid config

**Why now:** Everything reads config. The loader must be bulletproof before anything consumes it.

---

## Milestone 2: Expression Engine

**What:** Compile and evaluate `{{ }}` expressions using the Expr library.

**Deliverables:**
- Expression parser: extract `{{ }}` delimiters, pass content to Expr
- Expression compiler: parse all expressions at load time, cache compiled programs
- Expression evaluator: evaluate compiled programs against a context map
- Context builder: construct the evaluation context from `$.input`, `$.auth`, `$.trigger`, and node outputs
- String interpolation: `"Hello {{ input.name }}"` resolves to `"Hello Alice"`
- Custom function registration: `$uuid()`, `now()`, `len()`, `lower()`, `upper()`
- Static vs expression field detection (for startup validation of static-only fields)

**Tests:**
- Unit: parsing, compilation, evaluation with various context shapes, type coercion, error cases (undefined variables, type mismatches), string interpolation
- Unit: custom function registration and invocation
- Unit: static field validation (reject expressions where only literals are allowed)

**Why now:** The workflow engine, trigger mapping, and every node config depends on expression resolution.

---

## Milestone 3: Plugin System and Service Registry

**What:** Plugin loading, service instance creation, service registry, startup validation.

**Deliverables:**
- Plugin registry: register plugins by prefix, detect duplicate prefixes
- Node registry: register node descriptors and executor factories per plugin
- Service instance lifecycle: `CreateService(config)` → store in registry → `HealthCheck()` → `Shutdown()`
- Service registry: lookup by instance name, validate prefix matches on slots
- Startup validator:
  1. Load plugins, collect prefixes
  2. Initialize service instances
  3. Scan workflows, collect node types
  4. Verify every prefix has a plugin
  5. Verify every service reference has an instance
  6. Verify every node's service slots match the correct prefix
  7. Report all errors before starting
- A single test plugin (in-memory key-value store) to validate the full lifecycle

**Tests:**
- Unit: plugin registration, duplicate prefix detection, service creation, registry lookup
- Integration: full startup validation with valid config, verify rejection of missing plugins, missing services, prefix mismatches
- End-to-end: `noda validate` catches service reference errors

**Why now:** The workflow engine dispatches to nodes via the plugin/service system. This must work before any real node executes.

---

## Milestone 4: Workflow Engine — Core Execution

**What:** The graph executor. Nodes run, data flows, parallel execution works.

**Deliverables:**
- Graph compiler: parse nodes + edges into an executable DAG, detect cycles, determine execution order
- Entry node detection: nodes with no inbound edges
- Parallel execution: nodes with no dependencies run concurrently (goroutines)
- AND-join: node waits for all inbound edges from parallel branches
- OR-join: node waits for whichever branch was taken from a conditional split (determined at compile time)
- Execution context: stores node outputs, supports `Resolve()` for lazy expression evaluation
- Node dispatch: look up executor from registry, call `Execute(ctx, nCtx, config, services)`, store output, follow edges
- Memory management: evict node outputs from context after last dependent executes
- `context.Context` propagation: deadlines, cancellation through the entire execution
- Basic outputs: `success` and `error` routing
- Retry logic on error edges: when an error edge has `retry` config, re-execute the source node up to `attempts` times with configurable backoff (`fixed` or `exponential`) before following the error edge. Retries respect the `context.Context` deadline — if the deadline expires during backoff, cancel immediately and follow the error edge.
- Trace ID generation: every workflow execution gets a unique trace ID, propagated through the context and available in `$.trigger.trace_id`. Basic structured logging with trace ID context (full OpenTelemetry integration comes later in M19).

**Tests:**
- Unit: graph compilation, cycle detection, execution order, parallel detection, join type resolution
- Unit: context creation, expression resolution against context, output eviction
- Unit: retry logic — success on retry N, all retries exhausted, context deadline cancels backoff, exponential vs fixed delay
- Integration: execute a workflow with mock nodes — linear, parallel, converging
- Integration: verify parallel nodes actually run concurrently (timing assertions)
- Integration: verify context memory is freed after last dependent
- Integration: retry on error edge — node fails twice, succeeds on third attempt, success output fires
- Integration: trace ID present in all log entries for a single execution

**Why now:** This is the core. Every runtime (HTTP, worker, scheduler, Wasm) dispatches to the workflow engine.

---

## Milestone 5: Core Control Nodes

**What:** `control.if`, `control.switch`, `control.loop`, `workflow.run`, `workflow.output`.

**Deliverables:**
- `control.if` — evaluate condition, fire `then`/`else`/`error`
- `control.switch` — evaluate expression, match against static cases, fire matched case / `default` / `error`
- `control.loop` — iterate collection, invoke sub-workflow per item sequentially, collect results, fire `done`/`error`
- `workflow.run` — invoke sub-workflow, dynamic outputs from `workflow.output` nodes, `error` output
- `workflow.output` — terminal node, declare name + return data
- `workflow.output` mutual exclusivity validation at startup
- `workflow.run` dynamic output collection at startup (read sub-workflow, collect `workflow.output` names)
- `control.loop` input mapping with `$item` and `$index`

**Tests:**
- Unit: each node in isolation with mock contexts
- Integration: workflows exercising branching, switching, looping, sub-workflow invocation
- Integration: mutual exclusivity validation — verify rejection of invalid sub-workflows
- Integration: nested sub-workflows (workflow.run → workflow.run → workflow.output)
- Integration: loop with failing iteration (verify error fires, remaining skipped)

**Why now:** Control flow nodes are needed for any non-trivial workflow. They also validate the engine's join logic and sub-workflow mechanics.

---

## Milestone 6: Transform and Utility Nodes

**What:** `transform.*` and `util.*` — pure data manipulation and utility nodes.

**Deliverables:**
- `transform.set` — field mapping
- `transform.map` — per-item expression over collection
- `transform.filter` — predicate filter over collection
- `transform.merge` — append, match (inner/outer/enrich), position modes
- `transform.delete` — remove fields
- `transform.validate` — JSON Schema validation with field-level errors
- `util.log` — structured logging through slog
- `util.uuid` — UUID v4 generation
- `util.delay` — wait with context deadline respect
- `util.timestamp` — current time in iso8601/unix/unix_ms

**Tests:**
- Unit: each transform node with various input shapes, edge cases (empty arrays, missing fields, type mismatches)
- Unit: merge modes exhaustively — append, inner join, outer join, enrich, position
- Unit: validate against complex schemas, verify field-level error details
- Integration: workflow combining multiple transforms in sequence
- Integration: util.delay respects context cancellation

**Why now:** These are dependency-free (no services needed) and heavily used in every workflow. Good candidates for building confidence in the node execution model.

---

## Milestone 7: Workflow Testing Framework

**What:** `noda test` command, mock nodes, test runner.

**Deliverables:**
- Test file loading from `tests/` directory
- Mock node replacement: replace plugin nodes with mocks returning configured outputs/errors
- `workflow.run` mocking: specify which output name fires
- Core nodes execute normally (control flow, transforms)
- Test runner: execute each test case, compare expected vs actual
- Verbose mode: full execution trace for failed tests
- CLI: `noda test`, `noda test --workflow <id>`, `noda test --verbose`

**Tests:**
- Unit: mock replacement logic, expected output matching
- Integration: run tests against workflows using control.if, control.switch, control.loop, transforms
- Integration: mock sub-workflows in workflow.run, verify correct output fires
- End-to-end: `noda test` command exits 0 for passing, exits 1 for failing with clear output

**Why now:** Testing only depends on the workflow engine and core nodes (M4, M5, M6). Moving it here means every subsequent milestone — all the plugin work, all the runtimes — can write workflow tests immediately. Developers have `noda test` available from the start of real application development, not after 11 milestones of untestable plugin work.

---

## Milestone 8: HTTP Server Runtime

**What:** Fiber HTTP server, route loading, middleware, trigger mapping, response handling.

**Deliverables:**
- Fiber v3 server initialization from config
- Route registration: translate route configs into Fiber handlers
- Middleware loading: JWT auth, CORS, rate limiter, helmet, request ID, recovery, logger, timeout, compression, ETag
- Middleware presets and route groups
- Trigger mapping layer: evaluate input expressions against request data, populate `$.input`
- `$.auth` population from JWT middleware
- `$.trigger` metadata (type: "http", timestamp, trace_id)
- Response handling: Go channel mechanism for `HTTPResponse` — Fiber handler waits, `response.*` node fires, response sent, workflow continues async
- No `response.*` node → 202 Accepted, workflow runs in background
- Error responses: standardized error format with correct HTTP status codes
- HTTP error mapping: translate standard errors to HTTP status codes — `ValidationError` → 422, `NotFoundError` → 404, `ConflictError` → 409, `ServiceUnavailableError` → 503, `TimeoutError` → 504, untyped errors → 500. This is the HTTP-specific implementation of the error mapping strategy. Worker and scheduler trigger types implement their own mapping in their respective milestones.
- `response.json`, `response.redirect`, `response.error` nodes
- File stream passthrough (`files` array in trigger config)
- `raw_body` preservation
- OpenAPI generation from route configs (`noda generate openapi`)

**Tests:**
- Unit: trigger mapping with various request shapes, `$ref` resolution in route schemas
- Unit: middleware preset resolution, group inheritance
- Integration: full HTTP request → trigger mapping → workflow execution → response cycle
- Integration: parallel workflow with early response — verify response sent before workflow completes
- Integration: no response node → 202 Accepted
- Integration: error mapping — ValidationError → 422, NotFoundError → 404, etc.
- Integration: file upload trigger with `files` array
- Integration: `raw_body` preservation for webhook verification
- End-to-end: start Fiber server, send HTTP requests, verify responses
- End-to-end: OpenAPI spec generation matches route configs

**Why now:** This is the first time Noda is actually useful — you can send an HTTP request and get a response from a workflow. This milestone is the first "demo-able" moment.

---

## Milestone 9: Database Plugin

**What:** PostgreSQL plugin with GORM, all `db.*` nodes, transactions, migrations.

**Deliverables:**
- PostgreSQL plugin: register under `db` prefix, create GORM connections as service instances
- `db.query` — parameterized SELECT, return `[]map[string]any`
- `db.exec` — parameterized INSERT/UPDATE/DELETE, return rows_affected
- `db.create` — insert record, return created row with generated fields
- `db.update` — update by condition, return rows_affected
- `db.delete` — delete by condition, return rows_affected
- `workflow.run` with `transaction: true` — wrap sub-workflow in GORM transaction, swap connection for nested `db.*` nodes
- Health check: ping database
- Graceful shutdown: close connection pool
- Migration CLI: `noda migrate create/up/down/status` with timestamped SQL file pairs

**Tests:**
- Unit: SQL generation for each node type, parameter binding
- Integration (requires PostgreSQL): full CRUD cycle — create, query, update, delete
- Integration: transaction — success path commits, failure path rolls back
- Integration: multiple service instances (main-db, analytics-db) in same workflow
- Integration: migration create/up/down/status lifecycle
- End-to-end: HTTP request → workflow with db nodes → database write → response with created data

**Why now:** Database is the first real external service. It validates the full plugin lifecycle and the transaction sub-workflow pattern. After this milestone, you can build the simple REST API use case (Use Case 1).

---

## Milestone 10: Cache Plugin

**What:** Redis-backed cache plugin, all `cache.*` nodes.

**Deliverables:**
- Cache plugin: register under `cache` prefix, create go-redis connections
- `cache.get` — read key, return value, NotFoundError if missing
- `cache.set` — write key with optional TTL
- `cache.del` — delete key
- `cache.exists` — check existence
- `CacheService` interface implementation (for cross-plugin use — scheduler locking, Wasm host API)
- Health check: ping Redis
- Graceful shutdown: close connections

**Tests:**
- Unit: each node with mock Redis
- Integration (requires Redis): full get/set/del/exists cycle, TTL expiration
- Integration: CacheService interface usage from a different plugin context
- End-to-end: HTTP request → workflow with cache nodes → cached response

---

## Milestone 11: Event System and Workers

**What:** Stream plugin, PubSub plugin, event.emit node, worker runtime.

**Deliverables:**
- Stream plugin (`stream` prefix): Redis Streams wrapper, consumer groups
- PubSub plugin (`pubsub` prefix): Redis PubSub wrapper
- `event.emit` node: stream and pubsub modes, static mode validation
- Worker runtime: consume from Redis Streams, execute workflows per message
- Worker config: topic, consumer group, concurrency, middleware, trigger mapping
- Worker middleware: logging, timeout, error handling. Note: worker middleware is a separate system from Fiber's HTTP middleware — it wraps message processing, not HTTP handlers. The config pattern is the same (middleware names in an array), but the implementation is independent.
- Worker error mapping: failed workflows log errors with full context (trace ID, node ID, error details). Messages are redelivered on failure. After exhausting retries, messages move to the dead letter topic. No HTTP response is generated.
- Dead letter queue: move failed messages after N attempts
- Message acknowledgment: ack on success, nack + redelivery on failure

**Tests:**
- Unit: event.emit serialization for both modes
- Integration (requires Redis): emit to stream → worker consumes → workflow executes
- Integration: worker concurrency — multiple messages processed concurrently
- Integration: dead letter — message fails N times, lands in dead letter topic
- Integration: worker middleware — timeout kills long workflows
- Integration: worker error mapping — verify error logged with trace ID, message redelivered
- End-to-end: HTTP request → workflow emits event → worker picks up → worker workflow executes

**Why now:** Events and workers complete the async processing story. After this milestone, you can build the SaaS backend use case (Use Case 2: webhook ingestion → event → worker → notification).

---

## Milestone 12: Scheduler Runtime

**What:** Cron-based scheduler with distributed locking.

**Deliverables:**
- Scheduler runtime: load schedule configs, register cron jobs via `robfig/cron/v3`
- Distributed locking: atomic set-if-not-exists on cache service, TTL-based lock
- Trigger mapping: schedule metadata → `$.input`
- `$.trigger` with type: "schedule"
- Timezone support per job
- Scheduler error mapping: failed workflows are logged with trace ID and schedule metadata. The scheduler records the failure in the job execution history. No HTTP response, no dead letter — just logging and history.
- CLI: schedule status display

**Tests:**
- Integration: schedule fires at correct time (accelerated clock or short intervals)
- Integration: distributed lock — only one instance executes per interval
- Integration: lock expiry — if instance crashes, lock expires and next instance takes over
- Integration: error mapping — verify failure logged with schedule context
- End-to-end: scheduled workflow executes and produces observable side effects

---

## Milestone 13: Storage and Upload

**What:** Storage plugin (Afero), all `storage.*` nodes, `upload.handle` node.

**Deliverables:**
- Storage plugin (`storage` prefix): Afero wrapper, local and S3 backends
- `storage.read`, `storage.write`, `storage.delete`, `storage.list`
- `StorageService` interface implementation (for cross-plugin use)
- `upload.handle` node: multipart parsing, size/type validation, streaming to storage
- Multiple named storage instances

**Tests:**
- Unit: each storage node with in-memory Afero backend
- Integration: local filesystem backend — write, read, list, delete
- Integration: upload.handle — valid file accepted, oversized rejected, wrong type rejected
- Integration: two storage instances in same workflow (source + destination)
- End-to-end: HTTP file upload → upload.handle → storage.read verifies file exists

---

## Milestone 14: Image Processing

**What:** Image plugin with bimg/libvips.

**Deliverables:**
- Image plugin (`image` prefix): bimg wrapper
- `image.resize`, `image.crop`, `image.watermark`, `image.convert`, `image.thumbnail`
- Source and target storage service slots
- Quality and format configuration

**Tests:**
- Integration: resize produces correct dimensions (verify with image metadata)
- Integration: format conversion (PNG → WEBP, verify output format)
- Integration: thumbnail generation with smart crop
- End-to-end: upload image → resize → store thumbnail → read thumbnail via API

---

## Milestone 15: HTTP Client and Email

**What:** Outbound HTTP plugin and email plugin.

**Deliverables:**
- HTTP plugin (`http` prefix): Go's net/http wrapper with timeout
- `http.request`, `http.get`, `http.post`
- Email plugin (`email` prefix): SMTP client
- `email.send` with text/html content types

**Tests:**
- Integration: HTTP request to a mock server, verify request shape, handle responses
- Integration: HTTP timeout — verify TimeoutError when server is slow
- Integration: email.send to a mock SMTP server (like MailHog), verify delivery
- End-to-end: webhook received → workflow makes outbound HTTP call → response used in workflow

---

## Milestone 16: WebSocket and SSE

**What:** Connection manager, ws/sse endpoints, routing table, cross-instance sync.

**Deliverables:**
- Connection manager: track open connections, channel subscriptions
- WebSocket endpoint registration from config
- SSE endpoint registration from config
- Channel-based message routing with wildcard support
- `ws.send` and `sse.send` nodes
- Lifecycle workflows: on_connect, on_message, on_disconnect
- Auth middleware on WebSocket/SSE endpoints
- Redis routing table: channel → instance_id mapping with TTL
- Cross-instance delivery via PubSub
- Ping/pong keepalive
- `ConnectionService` interface implementation

**Tests:**
- Integration: WebSocket connect → on_connect workflow fires
- Integration: WebSocket message → on_message workflow fires → ws.send broadcasts
- Integration: WebSocket disconnect → on_disconnect workflow fires
- Integration: SSE subscribe → sse.send delivers event
- Integration: wildcard channels — `user.*` delivers to all user channels
- Integration: cross-instance delivery via routing table (two Noda instances in test)
- End-to-end: full real-time collaboration scenario — two clients, edits broadcast between them

**Why now:** After this milestone, the real-time collaboration use case (Use Case 3) works.

---

## Milestone 17: Casbin Authorization

**What:** Casbin integration for RBAC/ABAC.

**Deliverables:**
- Casbin middleware: load model and policies from config
- `{subject, object, action}` enforcement on routes
- Workspace-scoped policies (multi-tenant RBAC)
- Policy sync across instances via Redis PubSub watcher

**Tests:**
- Integration: permitted request passes, forbidden request returns 403
- Integration: multi-tenant — user A can access workspace 1, not workspace 2
- Integration: policy update propagation across instances

---

## Milestone 18: Wasm Runtime

**What:** Extism-based Wasm host, tick loop, host API, all Wasm integration.

**Deliverables:**
- Extism module loading and initialization
- Tick loop: call module's `tick` export at configured Hz
- `noda_call` host function: synchronous dispatch to services
- `noda_call_async` host function: async dispatch with label-based response delivery
- Tick input construction: dt, timestamp, client_messages, incoming_ws, connection_events, commands, responses, timers
- Query dispatch: call module's `query` export, serialized with respect to ticks
- Command dispatch: call module's `command` export (immediate) or buffer for next tick
- Shutdown lifecycle: call module's `shutdown` export with deadline
- Timer management: set_timer, clear_timer, fire on tick
- Outbound WebSocket management: ws_connect, ws_configure, ws_send, ws_close, reconnection, heartbeats
- JSON and MessagePack encoding (configurable per module)
- Service isolation: permission check on every `noda_call`
- Network isolation: whitelist check on HTTP and WebSocket outbound
- `wasm.send` and `wasm.query` workflow nodes
- `trigger_workflow` system operation
- Tick budget monitoring and warning logs
- `$env()` resolution in Wasm config before passing to module

A simple test Wasm module (in Rust or TinyGo) that exercises all host API operations.

**Tests:**
- Unit: tick input serialization (JSON + MessagePack), service permission checks
- Integration: load test module → initialize → tick → query → shutdown lifecycle
- Integration: noda_call sync — cache get/set, ws send, log
- Integration: noda_call_async — fire HTTP request, verify response in next tick
- Integration: timer — set timer, verify fires after interval
- Integration: outbound WebSocket — connect to mock server, receive messages in tick
- Integration: wasm.query from workflow — workflow sends query, receives response
- Integration: wasm.send from workflow — data delivered via command or tick
- Integration: trigger_workflow from Wasm — module triggers workflow, workflow executes
- Integration: MessagePack encoding — same test module with msgpack, verify identical behavior
- Integration: permission denied — module calls unauthorized service
- End-to-end: HTTP request → workflow → wasm.query → response with Wasm-computed data

**Why now:** Wasm is the most complex runtime. Building it after everything else means all the services it depends on (cache, storage, WebSocket, outbound HTTP, events) are stable and tested. After this milestone, the Discord bot (Use Case 4) and multiplayer game (Use Case 5) use cases work.

---

## Milestone 19: Observability

**What:** Full OpenTelemetry integration, health checks, production logging.

Note: Basic trace ID generation and structured logging are established in M4. This milestone adds full OpenTelemetry export, distributed tracing across services, and production health infrastructure.

**Deliverables:**
- OpenTelemetry tracing: spans per workflow execution, per node execution
- Trace ID propagation through all components (extending the basic trace ID from M4)
- OpenTelemetry export to standard collectors (Jaeger, Grafana Tempo, etc.)
- Structured logging via slog with full trace context and span correlation
- Health check endpoint: `/health` with service status (database, Redis, Wasm modules)
- Readiness probe: all services initialized and healthy
- Liveness probe: process is alive
- Dev mode trace WebSocket: full execution events for the visual editor (builds on top of the basic trace infrastructure)

**Tests:**
- Integration: verify spans are emitted for workflow execution
- Integration: trace ID present in all log entries for a given execution
- Integration: health endpoint reflects actual service status
- End-to-end: execute workflow, verify trace is exported to collector

---

## Milestone 20: Dev Mode and Hot Reload

**What:** `noda dev` command with file watching, hot reload, trace streaming, and graceful shutdown.

**Deliverables:**
- `noda dev` command: starts all runtimes in dev mode
- File watching via fsnotify: detect config file changes
- Hot reload: reload changed config files, re-validate, re-compile expressions, update routes/workflows
- Error surfacing: validation errors from hot reload delivered via trace WebSocket
- Trace WebSocket server: stream execution events for the visual editor
- Static file serving for the editor (placeholder until the editor is built)
- Graceful reload: in-flight requests complete with old config, new requests use new config
- Graceful shutdown for dev mode: ordered shutdown sequence — stop accepting → drain in-flight work → shutdown Wasm modules → close service connections → flush telemetry → exit. Triggered by SIGTERM/SIGINT. Configurable deadline (default: 30s).

**Tests:**
- Integration: change a workflow file → verify new workflow is active without restart
- Integration: introduce a validation error → verify error surfaced via WebSocket
- Integration: in-flight request completes with old config during reload
- Integration: graceful shutdown — in-flight request completes before process exits
- End-to-end: `noda dev` starts, file change triggers reload, next request uses new config

---

## Milestone 21: CLI Completion

**What:** All remaining CLI commands, production start, and polish.

**Deliverables:**
- `noda init [name]` — project scaffolding with directory structure, sample config, Docker Compose
- `noda start` with runtime flags (`--server`, `--workers`, `--scheduler`, `--wasm`, `--all`)
- Production graceful shutdown for `noda start` — same ordered sequence as dev mode, integrated with process managers and container orchestrators
- `noda generate openapi` — finalized
- `noda generate mcp` — MCP server definition export
- `noda plugin add/remove/list`
- `noda version`
- `--env` flag on all commands
- Help text and usage documentation for every command
- Shell completion (bash, zsh, fish)

**Tests:**
- End-to-end: `noda init` produces a valid project that passes `noda validate`
- End-to-end: `noda start --server` starts HTTP server only
- End-to-end: `noda start` graceful shutdown completes cleanly
- End-to-end: each CLI command produces expected output

---

## Milestone 22: Visual Editor — Foundation

**What:** Editor application shell, file sync, basic canvas.

**Deliverables:**
- React + TypeScript project setup with Vite
- React Flow integration: canvas with zoom, pan, minimap, grid
- Editor API on Noda dev server: all `/api/editor/*` endpoints
- File sync: read config files from Noda, write changes back
- Sidebar navigation: workflows, routes, workers, schedules, connections, services, schemas, wasm, tests
- Workflow list view: list all workflows from config
- Basic canvas rendering: load a workflow, display nodes and edges
- Custom node components: render node type, service slots, output ports
- Custom edge components: normal, error (dashed red)
- Node selection → right panel shows raw JSON config (placeholder for forms)

**Tests:**
- Unit: React components render correctly
- Integration: editor API returns correct file listings and content
- Integration: file write from editor → Noda hot reloads successfully
- End-to-end: open editor, see workflow list, click workflow, see graph on canvas

---

## Milestone 23: Visual Editor — Node Configuration

**What:** Auto-generated config forms, expression editor, service slot dropdowns.

**Deliverables:**
- React JSON Schema Form integration: generate forms from node ConfigSchema
- Expression field rendering: Monaco editor with `{{ }}` syntax highlighting
- Expression autocomplete: context-aware variable suggestions based on graph position
- Expression validation: real-time error feedback
- Service slot dropdowns: filtered by required prefix from available service instances
- Enum fields as dropdowns (mode, method, format, etc.)
- Required field indicators
- Config preview: show the resulting JSON alongside the form
- Save: write updated workflow config on change

**Tests:**
- Unit: form generation from various JSON Schemas
- Unit: expression autocomplete suggests correct context variables
- Integration: edit node config in form → JSON file updates correctly
- End-to-end: open workflow, click node, edit config, save, verify JSON on disk

---

## Milestone 24: Visual Editor — Graph Editing

**What:** Full canvas interaction — add nodes, draw edges, delete, undo/redo.

**Deliverables:**
- Node palette: searchable sidebar of all node types, grouped by category
- Drag-and-drop from palette to canvas
- Edge drawing: drag from output port to target node
- Edge validation: prevent invalid connections (wrong output names)
- Delete nodes and edges
- Copy/paste with edge reconnection
- Undo/redo (Zustand middleware)
- Auto-layout via ELKjs
- Quick-add: double-click canvas to search and add a node
- Context menus: right-click node/edge for actions
- Keyboard shortcuts: Ctrl+Z, Ctrl+S, Ctrl+Shift+F, Delete, etc.
- Multi-select and batch operations

**Tests:**
- Unit: undo/redo state management
- Integration: add node → verify in workflow JSON, delete node → removed from JSON
- Integration: draw edge → verify in edges array, auto-layout produces valid positions
- End-to-end: build a complete workflow from scratch using only the editor, verify it validates

---

## Milestone 25: Visual Editor — Live Tracing

**What:** Real-time execution visualization on the canvas.

**Deliverables:**
- Trace WebSocket client: connect to `/ws/trace`, parse events
- Node highlighting: blue (running), green (completed), red (failed)
- Edge animation: pulse effect when data flows
- Data inspection: click completed node to see input/output in debug panel
- Error display: failed nodes show error details
- Timing: execution duration per node
- Retry visualization: attempt count on error edges
- Debug panel: execution history list, click to replay trace
- Trace replay: select past execution, animate the graph
- Sub-workflow tracing: navigate into workflow.run nodes

**Tests:**
- Integration: fire HTTP request → verify canvas updates in real time
- Integration: trace replay shows correct execution order
- End-to-end: full development cycle — edit workflow, send request, watch execution, inspect data, fix issue, repeat

---

## Milestone 26: Visual Editor — Remaining Views

**What:** All non-workflow views.

**Deliverables:**
- Routes view: table + forms for route config
- Route "Try it" panel: send test requests, see responses with linked trace
- Workers view: table + forms
- Schedules view: table + forms with visual cron builder
- Connections view: endpoint config forms
- Services view: list with health status, add/remove/edit
- Schemas view: JSON Schema editor
- Wasm runtimes view: config forms with service/connection pickers
- Tests view: test case editor, run tests, view results with trace
- Migrations view: status table, run up/down/create
- Project scaffold wizard (first-run experience)

**Tests:**
- Integration: each view correctly reads and writes its config type
- End-to-end: create a route in the routes view, create a workflow in the workflow view, connect them, send a request via "Try it", see trace

---

## Milestone 27: Validation and Polish

**What:** Cross-file validation, graph validation, real-time feedback.

**Deliverables:**
- Graph validation in editor: cycles, missing service references, invalid edge output names, unreachable nodes
- `workflow.output` mutual exclusivity validation with visual feedback
- Cross-file validation: routes → workflows, workers → streams, schedules → cache services
- Validation panel: unified error list across all files
- Red badges on invalid nodes/edges
- Expression validation: underline errors in Monaco
- Auto-save with debounce
- Conflict detection: file changed on disk while editor has unsaved changes
- Dark mode

**Tests:**
- Integration: introduce various validation errors, verify editor displays correct messages
- Integration: fix errors, verify editor clears messages
- End-to-end: complete development cycle with validation feedback at every step

---

## Milestone 28: Documentation and Examples

**What:** User-facing documentation and example projects.

**Deliverables:**
- Getting started guide: install, init, dev, first route, first workflow
- Concept guides: workflows, nodes, services, triggers, expressions, testing
- Plugin authoring guide: how to build a custom plugin
- Wasm module authoring guide: how to build a Wasm module with the Noda PDK
- API reference: all nodes, all config fields, all CLI commands
- Example projects:
  - Simple REST API (Use Case 1)
  - SaaS backend (Use Case 2)
  - Real-time collaboration (Use Case 3)
  - Discord bot with Wasm module (Use Case 4)
  - Multiplayer game with Wasm module (Use Case 5)
- Video walkthroughs for key workflows

---

## Milestone Summary

| # | Milestone | Builds on | Key result |
|---|---|---|---|
| 0 | Project Skeleton | — | Interfaces defined, CI green |
| 1 | Config Loading | 0 | `noda validate` works |
| 2 | Expression Engine | 0 | Expressions compile and evaluate |
| 3 | Plugin System | 0, 1 | Service registry, startup validation |
| 4 | Workflow Engine | 1, 2, 3 | Graph execution with parallelism, retries, trace IDs |
| 5 | Core Control Nodes | 4 | Branching, loops, sub-workflows |
| 6 | Transform + Utility | 4 | Data manipulation, validation |
| 7 | Testing Framework | 5, 6 | `noda test` works — available for all subsequent milestones |
| 8 | HTTP Server | 4, 5, 6 | First HTTP request → response |
| 9 | Database Plugin | 3, 8 | Full CRUD + transactions. Use Case 1 works. |
| 10 | Cache Plugin | 3, 8 | Redis cache operations |
| 11 | Events + Workers | 3, 4, 8 | Async event processing. Use Case 2 works. |
| 12 | Scheduler | 10 | Cron jobs with distributed locks |
| 13 | Storage + Upload | 3, 8 | File handling |
| 14 | Image Processing | 13 | Image manipulation pipeline |
| 15 | HTTP Client + Email | 3, 8 | Outbound integrations |
| 16 | WebSocket + SSE | 8, 11 | Real-time connections. Use Case 3 works. |
| 17 | Casbin Auth | 8, 11 | RBAC/ABAC with policy sync |
| 18 | Wasm Runtime | 10, 13, 15, 16 | Tick loop, host API. Use Cases 4+5 work. |
| 19 | Observability | 4, 8 | Full OTel tracing, health checks |
| 20 | Dev Mode | 1, 8, 19 | Hot reload, trace streaming, graceful shutdown |
| 21 | CLI Completion | all | All commands polished, production start |
| 22 | Editor Foundation | 20 | Canvas renders workflows |
| 23 | Editor Node Config | 22 | Forms, expressions, services |
| 24 | Editor Graph Editing | 22, 23 | Build workflows visually |
| 25 | Editor Live Tracing | 22, 20 | Watch executions in real time |
| 26 | Editor Remaining Views | 22, 23 | Routes, workers, services, etc. |
| 27 | Validation + Polish | 22-26 | Real-time feedback, dark mode |
| 28 | Documentation | all | Guides, examples, references |

### Use Case Validation Checkpoints

| After Milestone | Use Case Validated |
|---|---|
| M9 | Use Case 1: Simple REST API |
| M11 | Use Case 2: SaaS Backend (core — HTTP, DB, cache, events, workers) |
| M15 | Use Case 2: SaaS Backend (complete — adds storage, thumbnails, scheduler, email, outbound HTTP) |
| M16 | Use Case 3: Real-Time Collaboration |
| M18 | Use Case 4: Discord Bot |
| M18 | Use Case 5: Multiplayer Game |
