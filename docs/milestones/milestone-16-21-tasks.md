# Milestone 16: WebSocket and SSE — Task Breakdown

**Depends on:** Milestone 8 (HTTP server), Milestone 11 (events — PubSub for cross-instance sync)
**Result:** WebSocket and SSE endpoints work with lifecycle workflows, channel-based messaging, wildcard routing, and cross-instance delivery via Redis routing table. Use Case 3 (Real-Time Collaboration) works.

---

## Task 16.1: Connection Manager Core

**Description:** Track open WebSocket and SSE connections with channel subscriptions.

**Subtasks:**

- [x] Create `internal/connmgr/manager.go`
- [x] Implement `ConnectionManager`:
  - Track connections: `connectionID → { conn, channels, userID, endpoint }`
  - Channel subscriptions: `channel → []connectionID`
  - Register/unregister connections
  - Send to channel: find all connections subscribed to the channel, deliver message
  - Wildcard matching: `user.*` matches `user.123`, `user.456`. `*` matches everything.
- [x] Thread-safe: connections added/removed concurrently
- [x] Implement `api.ConnectionService` on the manager instances (per endpoint)

**Tests:**
- [x] Register connection, send to channel, message delivered
- [x] Unregister connection, send to channel, no delivery
- [x] Wildcard matching: `user.*` delivers to all user channels
- [x] `*` delivers to all connections on endpoint
- [x] Concurrent register/unregister safe

**Acceptance criteria:** Connection tracking with channel-based delivery and wildcards.

---

## Task 16.2: WebSocket Endpoint Registration

**Description:** Register WebSocket endpoints from connection config.

**Subtasks:**

- [x] Create `internal/connmgr/websocket.go`
- [x] For each WebSocket endpoint in config:
  - Register a Fiber WebSocket handler at the configured path
  - Apply middleware (auth, etc.)
  - On connection: extract channel from pattern expression, register with connection manager
  - Message loop: read messages, trigger `on_message` workflow
  - On disconnect: trigger `on_disconnect` workflow, unregister
  - On connect: trigger `on_connect` workflow
- [x] Channel pattern: resolve expression at connect time (e.g., `doc.{{ request.params.doc_id }}`)
- [x] Ping/pong keepalive at configured interval
- [x] Max message size enforcement

**Tests:**
- [x] WebSocket connects at configured path
- [x] Channel pattern resolves from path params
- [ ] Auth middleware applied before connection
- [x] Messages received and delivered to on_message workflow
- [x] Ping/pong keeps connection alive
- [ ] Oversized messages rejected

**Acceptance criteria:** WebSocket endpoints work with auth and lifecycle workflows.

---

## Task 16.3: SSE Endpoint Registration

**Description:** Register SSE endpoints from connection config.

**Subtasks:**

- [x] Create `internal/connmgr/sse.go`
- [x] For each SSE endpoint in config:
  - Register a Fiber handler that establishes SSE stream
  - Apply middleware, resolve channel pattern
  - `on_connect` workflow triggered
  - Heartbeat at configured interval
  - Retry header set from config
  - On disconnect: `on_disconnect` workflow
- [x] SSE events include: `event` type, `data`, `id` fields

**Tests:**
- [x] SSE connection established
- [x] Events delivered with correct event/data/id fields
- [x] Heartbeat keeps connection alive
- [ ] Auth middleware applied

**Acceptance criteria:** SSE endpoints work with event delivery.

---

## Task 16.4: `ws.send` and `sse.send` Nodes

**Description:** Workflow nodes that push messages to connected clients.

**Subtasks:**

- [x] Create `plugins/core/ws/plugin.go` and `plugins/core/sse/plugin.go`
- [x] `ws.send`: ServiceDeps `{ "connections": { prefix: "ws" } }`, resolve `channel` and `data`, call `service.Send(channel, data)`
- [x] `sse.send`: ServiceDeps `{ "connections": { prefix: "sse" } }`, resolve `channel`, `data`, optional `event` and `id`, call `service.SendSSE(channel, event, data, id)`
- [x] Both: buffered non-blocking delivery, return `success` immediately

**Tests:**
- [x] ws.send delivers to connected WebSocket clients
- [x] sse.send delivers SSE event with correct fields
- [x] Wildcard channel delivery works from nodes
- [x] No connected clients → success (no error)

**Acceptance criteria:** Workflow nodes push to real-time connections.

---

## Task 16.5: Lifecycle Workflows

**Description:** Wire connection lifecycle events to workflow execution.

**Subtasks:**

- [x] `on_connect`: trigger referenced workflow with input from connection metadata (user_id, channel, endpoint, params)
- [x] `on_message`: trigger referenced workflow with message data + connection metadata. `$.trigger.type = "websocket"`.
- [x] `on_disconnect`: trigger referenced workflow with connection metadata
- [x] Workflows execute asynchronously — connection handling doesn't block on workflow completion (except on_connect which can reject the connection)

**Tests:**
- [x] Connect → on_connect workflow fires with correct metadata
- [x] Message → on_message workflow fires with message data
- [x] Disconnect → on_disconnect workflow fires
- [ ] Auth data available in lifecycle workflow via `$.auth`

**Acceptance criteria:** Connection lifecycle triggers workflows.

---

## Task 16.6: Redis Routing Table and Cross-Instance Delivery

**Description:** Track which instance holds which connections for targeted delivery.

**Subtasks:**

- [ ] Create `internal/connmgr/routing.go`
- [ ] On client connect: `HSET noda:routing:{channel} {instance_id} 1` with TTL
- [ ] On client disconnect: `HDEL noda:routing:{channel} {instance_id}`
- [ ] On send to channel:
  - Look up routing table for the channel
  - If local connections exist → deliver locally
  - If remote instances exist → publish via PubSub to those specific instances
- [ ] Wildcard sends: scan routing table for matching channels, group by instance, publish once per instance
- [ ] TTL refresh on ping interval to handle crash cleanup
- [ ] Subscribe to PubSub for incoming cross-instance messages, deliver to local connections

**Tests:**
- [ ] Routing table updated on connect/disconnect
- [ ] Local delivery works without PubSub
- [ ] Cross-instance delivery via PubSub
- [ ] Crashed instance entries expire via TTL
- [ ] Wildcard routing across instances

**Acceptance criteria:** Messages route correctly across multiple Noda instances.

---

## Task 16.7: End-to-End Tests — Use Case 3

**Subtasks:**

- [x] Test: Two WebSocket clients on same channel, one sends edit, both receive broadcast
- [x] Test: REST endpoint pushes to WebSocket channel (HTTP → ws.send)
- [x] Test: Connection lifecycle workflows fire correctly (connect, message, disconnect)
- [ ] Test: Cross-instance delivery (spin up two Noda instances in test)
- [x] Test: SSE subscription receives events from workflow

**Acceptance criteria:** Use Case 3 (Real-Time Collaboration) works end-to-end.

---

---

# Milestone 17: Casbin Authorization — Task Breakdown

**Depends on:** Milestone 8 (HTTP server), Milestone 11 (PubSub for policy sync)
**Result:** RBAC/ABAC enforcement on routes with multi-tenant support and cross-instance policy sync.

---

## Task 17.1: Casbin Middleware

**Description:** Load Casbin model and policies from config, enforce on routes.

**Subtasks:**

- [ ] Create `internal/server/casbin.go`
- [ ] Load Casbin model definition from root config `security.casbin.model`
- [ ] Load policies from root config `security.casbin.policies` or policy file path
- [ ] Create Fiber middleware: extract subject from `$.auth`, object from request path, action from HTTP method
- [ ] Enforce: permitted → continue, denied → 403 with standardized error
- [ ] Support multi-tenant RBAC: `{subject, tenant, object, action}` model where tenant comes from path parameter (e.g., workspace_id)

**Tests:**
- [ ] Permitted request passes
- [ ] Denied request returns 403
- [ ] Multi-tenant: user allowed in workspace A, denied in workspace B
- [ ] Admin role has broader access than member role

**Acceptance criteria:** Casbin enforces authorization on routes.

---

## Task 17.2: Policy Sync Across Instances

**Description:** Propagate policy changes across instances via Redis PubSub.

**Subtasks:**

- [ ] Implement Casbin watcher using Redis PubSub
- [ ] When policies are updated on one instance → publish update notification
- [ ] Other instances receive notification → reload policies
- [ ] Policy updates can come from: config file change (hot reload) or API endpoint (future)

**Tests:**
- [ ] Policy change on instance A → instance B reflects change
- [ ] Multiple instances stay in sync

**Acceptance criteria:** Authorization policies synchronized across instances.

---

---

# Milestone 18: Wasm Runtime — Task Breakdown

**Depends on:** Milestone 10 (cache), Milestone 13 (storage), Milestone 15 (HTTP client), Milestone 16 (WebSocket/SSE)
**Result:** Extism-based Wasm runtime with tick loop, sync/async host API, all service operations, outbound WebSocket management, and workflow integration. Use Cases 4 (Discord Bot) and 5 (Multiplayer Game) work.

---

## Task 18.1: Extism Module Loading

**Description:** Load Wasm modules via Extism and manage their lifecycle.

**Subtasks:**

- [ ] Create `internal/wasm/runtime.go`
- [ ] Implement `LoadModule(config WasmRuntimeConfig) (*Module, error)`:
  - Load `.wasm` binary from configured path
  - Create Extism plugin instance with host function bindings
  - Configure memory limits from config
- [ ] Module struct tracks: Extism plugin, config, service permissions, encoding format, timers, pending async calls

**Tests:**
- [ ] Module loads from valid .wasm file
- [ ] Invalid .wasm file → error
- [ ] Memory limits applied

**Acceptance criteria:** Wasm modules load via Extism.

---

## Task 18.2: Host Function — `noda_call`

**Description:** Synchronous host function for service dispatch.

**Subtasks:**

- [ ] Register `noda_call` as Extism host function
- [ ] Parse arguments: service name, operation, payload (JSON or MessagePack based on module encoding)
- [ ] Permission check: verify service is in module's allowed services list. Return `PERMISSION_DENIED` if not.
- [ ] Dispatch to service:
  - Storage operations: delegate to `StorageService`
  - Cache operations: delegate to `CacheService`
  - Connection operations: delegate to `ConnectionService`
  - Stream/PubSub emit: delegate to stream/pubsub service
  - System operations (service=""): log, trigger_workflow, set_timer, clear_timer
- [ ] Serialize response and return to module
- [ ] Error handling: service errors → Extism error mechanism

**Tests:**
- [ ] Cache get/set through noda_call
- [ ] Storage read/write through noda_call
- [ ] WS send through noda_call
- [ ] Unauthorized service → PERMISSION_DENIED
- [ ] System operations: log, trigger_workflow

**Acceptance criteria:** Synchronous host calls dispatch to all service types.

---

## Task 18.3: Host Function — `noda_call_async`

**Description:** Asynchronous host function with label-based response delivery.

**Subtasks:**

- [ ] Register `noda_call_async` as Extism host function
- [ ] Parse arguments: service, operation, payload, label
- [ ] Validate label uniqueness among pending async calls → `VALIDATION_ERROR` on duplicate
- [ ] Launch goroutine to execute the operation
- [ ] Return immediately to the module (non-blocking)
- [ ] When operation completes: store result (ok + data, or error) keyed by label
- [ ] Results delivered in the next tick's `responses` field

**Tests:**
- [ ] Async HTTP request → response appears in next tick
- [ ] Async storage write → response appears in next tick
- [ ] Duplicate label → VALIDATION_ERROR
- [ ] Async error → error response in tick

**Acceptance criteria:** Async calls execute on background goroutines with labeled response delivery.

---

## Task 18.4: Tick Loop

**Description:** Call module's `tick` export at configured Hz with accumulated events.

**Subtasks:**

- [ ] Create `internal/wasm/tick.go`
- [ ] Implement tick loop:
  - Calculate interval from `tick_rate` (1000/Hz milliseconds)
  - Accumulate events between ticks: client_messages, incoming_ws, connection_events, commands
  - On tick: construct tick input (dt, timestamp, accumulated events, async responses, fired timers)
  - Serialize tick input (JSON or MessagePack)
  - Call module's `tick` export via Extism
  - Clear accumulated events and delivered responses
  - Track timing for tick budget warnings
- [ ] Tick budget monitoring: if tick exceeds budget, log warning. Consecutive overruns → increasing severity.
- [ ] Tick is never killed — next tick delayed, receives larger `dt`

**Tests:**
- [ ] Tick fires at configured rate
- [ ] dt reflects actual time between ticks
- [ ] Events accumulated and delivered correctly
- [ ] Async responses delivered in correct tick
- [ ] Budget overrun logged

**Acceptance criteria:** Tick loop runs at configured Hz with correct event delivery.

---

## Task 18.5: Timer Management

**Description:** Named timers that fire on tick boundaries.

**Subtasks:**

- [ ] Implement `set_timer(name, interval_ms)` and `clear_timer(name)` system operations
- [ ] Track timers per module: name → next fire time
- [ ] On each tick: check which timers have elapsed, include their names in `timers` array
- [ ] Timers fire on tick boundaries (not precise to interval — fire on the next tick after interval elapses)

**Tests:**
- [ ] Set timer → fires after interval
- [ ] Clear timer → stops firing
- [ ] Timer fires on tick boundary (not between ticks)
- [ ] Multiple timers with different intervals

**Acceptance criteria:** Named timers work with tick-aligned delivery.

---

## Task 18.6: Outbound WebSocket Management

**Description:** Manage outbound WebSocket connections for modules.

**Subtasks:**

- [ ] Implement system operations: `ws_connect`, `ws_configure`, `ws_send`, `ws_close`
- [ ] `ws_connect`: establish WebSocket connection to whitelisted host, buffer incoming messages
- [ ] `ws_configure`: set heartbeat interval/payload, reconnection settings
- [ ] `ws_send`: send message on established connection
- [ ] `ws_close`: close connection with code and reason
- [ ] Network isolation: URL host must be in `allow_outbound.ws` whitelist
- [ ] Reconnection: automatic with configurable backoff
- [ ] Heartbeat: automatic based on ws_configure settings
- [ ] Incoming messages buffered and delivered in tick's `incoming_ws`
- [ ] Connection events delivered in tick's `connection_events`

**Tests:**
- [ ] Connect to mock WebSocket server
- [ ] Send and receive messages
- [ ] Auto-heartbeat at configured interval
- [ ] Reconnection on disconnect
- [ ] Connection event delivery (reconnected, disconnected)
- [ ] Non-whitelisted host → PERMISSION_DENIED

**Acceptance criteria:** Outbound WebSocket lifecycle fully managed by Noda.

---

## Task 18.7: Query and Command Dispatch

**Description:** Handle `wasm.query` and `wasm.send` from workflows.

**Subtasks:**

- [ ] `wasm.query` node: serialize query data, call module's `query` export (serialized with respect to ticks), return response. Enforce timeout from config.
- [ ] `wasm.send` node: if module exports `command` → call immediately between ticks. Otherwise buffer for next tick's `commands` array.
- [ ] Call serialization: queries wait for tick to complete, commands dispatched between ticks

**Tests:**
- [ ] wasm.query → module returns data → workflow receives it
- [ ] wasm.query timeout → TimeoutError
- [ ] wasm.send with command export → immediate delivery
- [ ] wasm.send without command export → buffered for tick
- [ ] Query during tick → waits for tick to complete

**Acceptance criteria:** Workflow-to-Wasm communication works in both directions.

---

## Task 18.8: JSON and MessagePack Encoding

**Description:** Configurable serialization format per module.

**Subtasks:**

- [ ] Implement encoding abstraction: `Serialize(data any) ([]byte, error)` and `Deserialize(bytes []byte, target any) error`
- [ ] JSON encoder/decoder (default)
- [ ] MessagePack encoder/decoder using `vmihailenco/msgpack/v5`
- [ ] Encoding set per module from config, applied to all data crossing the boundary
- [ ] Include `encoding` field in initialize manifest

**Tests:**
- [ ] JSON encoding round-trip
- [ ] MessagePack encoding round-trip
- [ ] Same test module works with both encodings (behavioral parity)
- [ ] Encoding field present in initialize manifest

**Acceptance criteria:** Both serialization formats work identically.

---

## Task 18.9: Test Wasm Module

**Description:** Build a simple test module (TinyGo or Rust) that exercises all host API operations.

**Subtasks:**

- [ ] Create `testdata/wasm/test-module/` with source code
- [ ] Module implements: `initialize`, `tick`, `query`, `command`, `shutdown`
- [ ] On initialize: read config, call noda_call to read from storage
- [ ] On tick: process client_messages, set/get cache, send ws messages, check async responses, fire timers
- [ ] On query: return in-memory state
- [ ] On command: update in-memory state
- [ ] On shutdown: write state to storage
- [ ] Build `.wasm` binary as part of test setup

**Tests:**
- [ ] Full lifecycle test with the test module
- [ ] All noda_call operations exercised
- [ ] All noda_call_async operations exercised
- [ ] All tick input fields delivered correctly

**Acceptance criteria:** Test module validates the entire Wasm Host API.

---

## Task 18.10: End-to-End Tests

**Subtasks:**

- [ ] Test: HTTP request → workflow → wasm.query → response with Wasm-computed data
- [ ] Test: wasm.send from workflow → module receives command
- [ ] Test: Module triggers workflow via noda_call → workflow executes
- [ ] Test: Module connects to outbound WebSocket → receives messages in tick

**Acceptance criteria:** Wasm runtime fully integrated with Noda. Use Cases 4 and 5 are buildable.

---

---

# Milestone 19: Observability — Task Breakdown

**Depends on:** Milestone 4 (workflow engine), Milestone 8 (HTTP server)
**Result:** Full OpenTelemetry tracing, health check endpoints, production-grade structured logging.

---

## Task 19.1: OpenTelemetry Tracing

**Description:** Emit OTel spans for workflow and node execution.

**Subtasks:**

- [ ] Initialize OTel tracer provider from config (exporter: stdout in dev, OTLP in production)
- [ ] Create span per workflow execution (root span)
- [ ] Create child span per node execution
- [ ] Span attributes: workflow_id, node_id, node_type, trigger_type, status
- [ ] Error recording on spans
- [ ] Trace ID from M4 used as OTel trace ID (correlation)

**Tests:**
- [ ] Spans emitted for workflow execution
- [ ] Child spans for each node
- [ ] Trace ID correlates across spans
- [ ] Error spans recorded on failure

**Acceptance criteria:** OTel traces exported for all workflow executions.

---

## Task 19.2: Health Check Endpoints

**Description:** Health, readiness, and liveness endpoints.

**Subtasks:**

- [ ] `/health` — checks all service instances (DB, Redis, Wasm modules), returns aggregate status
- [ ] `/health/ready` — returns 200 when all services initialized, 503 otherwise
- [ ] `/health/live` — returns 200 if process is running
- [ ] Service health detail: `{ "status": "healthy", "services": { "main-db": "ok", "app-cache": "ok" } }`

**Tests:**
- [ ] Healthy system → 200 with all services OK
- [ ] One unhealthy service → 503 with details
- [ ] Readiness probe during startup → 503, after init → 200
- [ ] Liveness always 200 if process runs

**Acceptance criteria:** Container orchestrators can probe health.

---

## Task 19.3: Dev Mode Trace WebSocket

**Description:** Stream full execution events over WebSocket for the visual editor.

**Subtasks:**

- [ ] Create `internal/trace/websocket.go`
- [ ] Serve WebSocket at `/ws/trace` in dev mode
- [ ] Emit events: workflow:started, workflow:completed, node:entered, node:completed, node:failed, edge:followed, retry:attempted
- [ ] Each event includes full data (input/output for nodes) — dev mode only, never in production
- [ ] Multiple editor connections can subscribe simultaneously
- [ ] Event format matches the visual editor document specification

**Tests:**
- [ ] WebSocket connects and receives events
- [ ] Events match execution flow
- [ ] Multiple clients receive same events

**Acceptance criteria:** Visual editor can consume live execution traces.

---

---

# Milestone 20: Dev Mode and Hot Reload — Task Breakdown

**Depends on:** Milestone 1 (config loading), Milestone 8 (HTTP server), Milestone 19 (observability)
**Result:** `noda dev` starts all runtimes with hot reload. Config file changes apply without restart. Graceful shutdown works correctly.

---

## Task 20.1: `noda dev` Command

**Description:** Development mode that starts all runtimes together.

**Subtasks:**

- [ ] Implement `noda dev` command:
  - Run full config validation
  - Initialize all plugins and services
  - Start HTTP server, worker runtime, scheduler, Wasm runtime, connection manager
  - Serve editor placeholder at `/editor`
  - Start trace WebSocket server
  - Start file watcher
  - Block until shutdown signal
- [ ] Dev-only features: full trace streaming, verbose logging, editor serving

**Tests:**
- [ ] `noda dev` starts all components
- [ ] HTTP server responds to requests
- [ ] Trace WebSocket available

**Acceptance criteria:** Single command starts the full development environment.

---

## Task 20.2: File Watching and Hot Reload

**Description:** Detect config file changes and reload without restart.

**Subtasks:**

- [ ] Use `fsnotify` to watch all config directories
- [ ] On file change:
  1. Re-run config validation on changed file(s)
  2. If valid: re-compile expressions, update route registrations, update workflow definitions
  3. If invalid: surface validation errors via trace WebSocket (file:error event), keep running with previous valid config
- [ ] Debounce: wait 100ms after last change before reloading (batch rapid edits)
- [ ] Graceful reload: in-flight requests/workflows complete with old config, new requests use new config

**Tests:**
- [ ] Change workflow file → new workflow active without restart
- [ ] Change route file → new route responds
- [ ] Invalid change → error surfaced, old config still works
- [ ] Rapid edits debounced
- [ ] In-flight request completes with old config

**Acceptance criteria:** Config changes apply without restart, with error protection.

---

## Task 20.3: Graceful Shutdown

**Description:** Ordered shutdown sequence on SIGTERM/SIGINT.

**Subtasks:**

- [ ] Signal handler catches SIGTERM and SIGINT
- [ ] Shutdown sequence:
  1. Stop accepting new HTTP connections
  2. Stop worker consumers and scheduler
  3. Drain in-flight HTTP requests (configurable deadline, default 30s)
  4. Drain in-flight worker workflows
  5. Call `shutdown()` on all Wasm modules with deadline
  6. Close all WebSocket/SSE connections
  7. Close service connections (DB, Redis, storage)
  8. Flush OTel telemetry
  9. Exit
- [ ] Configurable shutdown deadline from root config
- [ ] Log each shutdown phase

**Tests:**
- [ ] In-flight request completes before shutdown
- [ ] Wasm modules get shutdown call
- [ ] Services closed after workflows drain
- [ ] Forced exit after deadline

**Acceptance criteria:** Clean, ordered shutdown preserving in-flight work.

---

---

# Milestone 21: CLI Completion — Task Breakdown

**Depends on:** All previous milestones
**Result:** All CLI commands functional, production `noda start` working, project scaffolding.

---

## Task 21.1: `noda init`

**Description:** Scaffold a new Noda project.

**Subtasks:**

- [ ] Create project directory with standard structure
- [ ] Generate `noda.json` with sensible defaults (ports, JWT placeholder)
- [ ] Generate `docker-compose.yml` with PostgreSQL and Redis
- [ ] Generate `.env.example`
- [ ] Generate sample route, workflow, and schema
- [ ] Generate README with getting started instructions
- [ ] Accept project name as argument

**Tests:**
- [ ] Scaffolded project passes `noda validate`
- [ ] Docker Compose starts successfully

**Acceptance criteria:** `noda init myapp` creates a working project.

---

## Task 21.2: `noda start` (Production Mode)

**Description:** Production server startup with runtime flag control.

**Subtasks:**

- [ ] Implement `noda start` with flags: `--server`, `--workers`, `--scheduler`, `--wasm`, `--all` (default)
- [ ] No file watching, no editor serving, no full trace streaming
- [ ] Production graceful shutdown (same sequence as dev mode)
- [ ] Environment from `--env` flag (default: infer from `NODA_ENV`)

**Tests:**
- [ ] `noda start --server` starts HTTP only
- [ ] `noda start --workers` starts workers only
- [ ] `noda start --all` starts everything
- [ ] Graceful shutdown works in production mode

**Acceptance criteria:** Production-ready server startup with selective runtime control.

---

## Task 21.3: Remaining CLI Commands

**Subtasks:**

- [ ] `noda generate openapi` — export OpenAPI spec to file (finalized from M8)
- [ ] `noda generate mcp` — export MCP server definition (stub for now)
- [ ] `noda plugin list` — list loaded plugins with prefixes and node counts
- [ ] `noda version` — print version, Go version, build info
- [ ] Shell completions: bash, zsh, fish via Cobra's built-in completion
- [ ] Help text review: every command has clear description and examples

**Tests:**
- [ ] Each command produces expected output
- [ ] Shell completions generate valid scripts

**Acceptance criteria:** All CLI commands are functional and documented.
