# Noda — Public API Interfaces (`pkg/api/`)

**Version**: 0.4.0
**Status**: Planning

This document defines the stable public interfaces that live in `pkg/api/`. Plugin authors import this package to build plugins and nodes for Noda. Everything in this package is a contract — once published, breaking changes require a major version bump.

---

## 1. Overview

The public API consists of seven components:

- **Plugin** — the package-level registration interface. Declares a prefix, provides nodes, manages service instances.
- **Node Descriptor** — static metadata about a node type. Registered once at startup. Contains the node's name, service dependencies, and config schema.
- **Node Executor** — the runtime interface. Created per node instance with its config. Declares its output ports and handles execution.
- **Execution Context** — the read-only environment provided to nodes at execution time.
- **Response Types** — standardized structure for HTTP responses (`HTTPResponse`). Returned by `response.*` nodes, used by the trigger layer to unblock the HTTP handler. WebSocket and SSE nodes interact with the connection manager directly via their injected service.
- **Common Service Interfaces** — shared interfaces (`StorageService`, `CacheService`, `ConnectionService`) that enable cross-plugin service usage without tight coupling. Database services have no common interface — only the database plugin's own nodes access GORM directly.
- **Standard Errors** — typed errors for consistent error handling across all nodes and plugins.

---

## 2. Plugin Interface

A plugin is a self-contained package that owns a namespace prefix and provides nodes and service instances. Three types of plugins exist, all implementing the same interface:

- **External plugins** (postgres, cache, storage, image, http, email) — manage connections to external systems
- **Core node plugins** — provide built-in workflow nodes, no services of their own:
  - `core.control` → `control` prefix (if, switch, loop)
  - `core.workflow` → `workflow` prefix (run, output)
  - `core.transform` → `transform` prefix (set, map, filter, merge, delete, validate)
  - `core.response` → `response` prefix (json, redirect, error)
  - `core.util` → `util` prefix (log, uuid, delay, timestamp)
  - `core.event` → `event` prefix (emit) — has service deps to stream/pubsub, but no services of its own
  - `core.upload` → `upload` prefix (handle) — has service deps to storage, but no services of its own
  - `core.ws` → `ws` prefix (send) — has service deps to connection manager instances
  - `core.sse` → `sse` prefix (send) — has service deps to connection manager instances
  - `core.wasm` → `wasm` prefix (send, query) — has service deps to Wasm runtime instances
- **Internal service plugins** (stream, pubsub) — expose internal runtime services to nodes via the service registry. Connection manager endpoints and Wasm runtime instances also register as internal services automatically from their respective config sections.

```
Plugin
  Name()                        → string
  Prefix()                      → string
  Nodes()                       → []NodeRegistration
  HasServices()                 → bool
  CreateService(config)         → ServiceInstance, error
  HealthCheck(service)          → error
  Shutdown(service)             → error
```

**Name()** — the plugin's human-readable name. Used in logs, error messages, and the CLI. Examples: `"postgres"`, `"cache"`, `"stream"`, `"pubsub"`, `"core.control"`, `"internal.ws"`.

**Prefix()** — the namespace this plugin owns. All nodes from this plugin are registered under this prefix. Examples: `"db"`, `"cache"`, `"control"`, `"ws"`. Only one plugin can own a given prefix — duplicate prefix registration is a startup error.

**Nodes()** — returns a list of node registrations. Each registration pairs a node descriptor with an executor factory (a function that creates new executor instances). May return an empty list for plugins that only provide services (e.g., the stream plugin provides infrastructure for workers but may not expose workflow nodes directly).

**HasServices()** — returns whether this plugin manages service instances. Core node plugins (control, transform) return false. External and internal service plugins return true. When false, `CreateService`, `HealthCheck`, and `Shutdown` are never called.

**CreateService(config)** — called once per service instance declared in the config. Receives the instance-specific config (database URL, connection pool settings, etc.) and returns an initialized service instance. Only called when `HasServices()` returns true.

**HealthCheck(service)** — called periodically and at startup to verify a service instance is alive. Receives the service instance created by `CreateService`. Returns an error if the service is unreachable.

**Shutdown(service)** — called during graceful shutdown. Closes connections, flushes buffers, releases resources. Called once per service instance.

---

## 3. Node Descriptor

The descriptor is the static metadata about a node type. It's registered once at startup and used by the engine for validation, by the visual editor for rendering, and by the config validator for schema checking.

The descriptor is purely static — it contains no config-dependent information. Dynamic properties like outputs live on the executor (see Section 4).

```
NodeDescriptor
  Name()                        → string
  ServiceDeps()                 → map[slotName]ServiceDep
  ConfigSchema()                → JSONSchema

ServiceDep
  Prefix                        → string
  Required                      → bool
```

**Name()** — the node's short name within its plugin's prefix. Examples: `"query"`, `"if"`, `"resize"`. The engine combines this with the plugin prefix to form the full type: `"db.query"`, `"control.if"`, `"image.resize"`.

**ServiceDeps()** — a map of slot names to service dependency definitions. Each slot declares a required plugin prefix and whether it's optional or required. This declares what services — both external (database, cache) and internal (connection manager, Wasm runtime) — the node needs. Examples:

| Node | ServiceDeps |
|---|---|
| `db.query` | `{ "database": { prefix: "db", required: true } }` |
| `image.resize` | `{ "source": { prefix: "storage", required: true }, "target": { prefix: "storage", required: true } }` |
| `upload.handle` | `{ "destination": { prefix: "storage", required: true } }` |
| `ws.send` | `{ "connections": { prefix: "ws", required: true } }` — instance name is the endpoint key from connections config |
| `event.emit` | `{ "stream": { prefix: "stream", required: false }, "pubsub": { prefix: "pubsub", required: false } }` |
| `wasm.send` | `{ "runtime": { prefix: "wasm", required: true } }` |
| `control.if` | `{}` (no service dependencies) |

Slot names are chosen by the node author and are meaningful — `"source"` and `"target"` for image processing, `"database"` for queries, `"connections"` for WebSocket delivery. The workflow config maps these slot names to actual service instance names.

**Optional slots** are validated conditionally. For `event.emit`, the node's config specifies a static `mode` field (`"stream"` or `"pubsub"` — not an expression). At startup, the engine validates that the slot matching the configured mode is filled. Unfilled optional slots are allowed as long as the node's logic doesn't require them.

**Internal services** (connection manager, Wasm runtime manager) register in the service registry alongside plugin services. Core nodes reference them through the same ServiceDeps mechanism — no special cases.

At startup, the engine validates that:
1. Every required slot is filled in the workflow config
2. Optional slots that are filled reference valid service instances
3. Each referenced service instance comes from a plugin whose prefix matches the slot's requirement

**ConfigSchema()** — a JSON Schema defining the valid configuration for this node. Used at startup to validate all node configs in all workflows. Used by the visual editor to generate configuration forms and provide validation feedback.

---

## 4. Node Executor

The executor is the runtime interface. A new instance is created per node during workflow compilation via the factory, which receives the node's raw config. The executor determines its outputs from the config it was created with, and handles execution at runtime.

```
NodeExecutor
  Outputs()                             → []string
  Execute(ctx, nCtx, config, services)  → outputName, data, error
```

**Outputs()** — returns the list of named output ports for this specific node instance. Determined from the config the executor was created with.

For static nodes (most nodes), the outputs are always the same regardless of config:

| Node | Outputs |
|---|---|
| `db.query` | `["success", "error"]` |
| `transform.set` | `["success", "error"]` |
| `response.json` | `["success", "error"]` |
| `control.if` | `["then", "else", "error"]` |
| `control.loop` | `["done", "error"]` |

For dynamic nodes, the outputs depend on the config. Case names and output names must be static string literals — expressions are not allowed because outputs must be known at startup:

| Node | Config | Outputs |
|---|---|---|
| `control.switch` | `cases: ["admin", "user"]` | `["admin", "user", "default", "error"]` |
| `workflow.run` | `workflow: "check-inventory"` | collected from sub-workflow's `workflow.output` nodes, plus `"error"` |

The `workflow.run` factory reads the referenced sub-workflow at compilation time, collects all `workflow.output` node names, and returns those plus `"error"` as its outputs. This means the sub-workflow must exist and be valid at startup.

All dynamic-output nodes follow the same contract: **exactly one output fires per execution.** `control.if` fires one of `then`/`else`. `control.switch` fires one case. `workflow.run` fires whichever `workflow.output` was reached inside the sub-workflow. The engine enforces this for sub-workflows at startup by validating that all `workflow.output` nodes are on mutually exclusive branches.

The visual editor calls the factory with the current config whenever the user changes node configuration, and re-renders the output ports based on what `Outputs()` returns. The engine validates at startup that every edge references a valid output name.

**Execute(ctx, nCtx, config, services):**

**Parameters:**

- **ctx** — Go's standard `context.Context`. Carries deadlines, cancellation signals, and trace metadata. Nodes must pass this to all I/O operations (database queries, HTTP requests, cache calls). The engine sets a deadline on the context based on timeout configuration. If the context is cancelled, the node should return immediately.
- **nCtx** — the Noda execution context (see Section 5). Provides access to input data, auth data, trigger metadata, expression resolution, and logging. Named `nCtx` to avoid shadowing Go's `context` package.
- **config** — the raw, unresolved node configuration as defined in the workflow JSON. Expressions (`{{ }}`) are NOT pre-evaluated. The node calls `nCtx.Resolve()` when it needs to evaluate an expression. This gives the node control over when and whether expressions are evaluated.
- **services** — a map of slot name → service instance. Keyed by the slot names declared in `ServiceDeps()`. For nodes with no service dependencies, this map is empty.

**Return values:**

- **outputName** — which output to fire. Must be one of the names returned by `Outputs()`. Examples: `"success"`, `"then"`, `"admin"`. If the executor returns an error (the third return value), this field is ignored and the engine automatically fires the `"error"` output.
- **data** — the output data to store on the execution context. Stored under the node's ID (or its `as` alias). Downstream nodes can access this via expressions.
- **error** — if non-nil, the engine fires the `"error"` output and makes the error details available to error-handling nodes. If the node has no edge on its `"error"` output, the entire workflow fails with the standardized error response.

**Execution contract:**

- The executor must pass `ctx` to all I/O operations — database queries, HTTP requests, cache reads. This enables timeout enforcement and cancellation.
- The executor must not modify the execution context directly — it returns data, and the engine writes it.
- The executor must not resolve expressions it doesn't need — lazy evaluation prevents errors in unused branches.
- The executor must not look up services itself — all services are provided via the `services` parameter.
- The executor must not spawn background work — all work completes before `Execute` returns. For async operations, the node should use `event.emit` or `wasm.send`.
- The executor should use `nCtx.Log()` for logging, not write to stdout/stderr directly.

---

## 5. Execution Context

The execution context is passed to every node executor. It is read-only from the node's perspective — the node cannot write to it directly.

```
ExecutionContext
  Input()                       → any
  Auth()                        → AuthData or nil
  Trigger()                     → TriggerData
  Resolve(expression)           → value, error
  Log(level, message, fields)
```

**Input()** — returns the `$.input` data as set by the trigger mapping layer. This is the normalized input regardless of how the workflow was triggered (HTTP, event, schedule, WebSocket).

**Auth()** — returns the authentication data (`$.auth`) if available. Returns nil for triggers that don't have authentication (scheduler, some events). Nodes must handle the nil case. The shape is:

```
AuthData
  UserID                        → string
  Roles                         → []string
  Claims                        → map[string]any (raw JWT claims)
```

**Trigger()** — returns metadata about what triggered this workflow execution:

```
TriggerData
  Type                          → string ("http", "event", "schedule", "websocket", "wasm")
  Timestamp                     → time
  TraceID                       → string (unique execution identifier)
```

**Resolve(expression)** — evaluates a `{{ }}` expression string against the current execution context. The current context includes `$.input`, `$.auth`, `$.trigger`, and all outputs from nodes that have already completed. Returns the resolved value or an error if the expression is invalid or references data that doesn't exist.

This is the mechanism for lazy expression evaluation. The node receives raw, unresolved config and calls `Resolve()` when it needs a value. This enables:
- `control.if` to resolve only the condition, not both branches
- `control.loop` to re-resolve expressions per iteration
- Nodes to skip resolving config values they don't need in a given execution path

**Log(level, message, fields)** — structured logging. The level is one of: `debug`, `info`, `warn`, `error`. Fields is a map of key-value pairs for structured context. In dev mode, log output appears in the live trace stream. In production, it's routed through the standard logging pipeline (slog → OpenTelemetry).

---

## 6. Response Types

The only response type defined in `pkg/api/` is `HTTPResponse`. WebSocket and SSE nodes interact with the connection manager directly via their injected service — they don't return a response type to the engine.

### 6.1 HTTPResponse

Returned by `response.json`, `response.redirect`, and `response.error` nodes.

```
HTTPResponse
  Status                        → int
  Headers                       → map[string]string
  Cookies                       → []Cookie
  Body                          → any

Cookie
  Name                          → string
  Value                         → string
  Path                          → string
  Domain                        → string
  MaxAge                        → int (seconds)
  Secure                        → bool
  HTTPOnly                      → bool
  SameSite                      → string ("Strict", "Lax", "None")
```

**Status** — HTTP status code. Examples: `200`, `201`, `302`, `404`, `500`.

**Headers** — arbitrary response headers. Used for cache control, content type overrides, redirects (via `Location`), and custom headers.

**Cookies** — structured cookie definitions. Cleaner than setting cookies via raw headers, and the visual editor can render a proper form for each field.

**Body** — the response payload. For `response.json`, serialized as JSON. For `response.redirect`, typically empty (the `Location` header + status 302 does the work). For `response.error`, follows the standardized error shape (see Section 8.1).

### 6.2 WebSocket and SSE Nodes

`ws.send` and `sse.send` nodes do not return a response type. Instead, they call the connection manager service directly during `Execute()` via their injected service dependency. For example, `ws.send` calls `services["connections"].Send(channel, data)` on the connection manager instance. This keeps the engine agnostic to real-time protocols and makes the nodes self-contained.

### 6.3 How the Trigger Layer Uses HTTPResponse

For HTTP-triggered workflows, the trigger layer watches for any node that returns an `HTTPResponse`. As soon as one fires, the HTTP response is written to the client immediately via a Go channel — the workflow continues executing any remaining nodes asynchronously. If the workflow has no `response.*` node, the trigger layer returns `202 Accepted` immediately and the workflow runs entirely in the background.

For non-matching triggers (e.g., an `HTTPResponse` in a worker-triggered workflow), the response data is just stored as regular node output — nothing breaks, nothing special happens.

### 6.4 Response Nodes Are Not Special

Response nodes are regular nodes that happen to return an `HTTPResponse`. They have the standard `success` and `error` outputs. They follow the same interface as every other node. The engine doesn't treat them differently — the trigger layer does.

This means:
- A workflow can have multiple response nodes in different branches (success path returns 200, error path returns 404)
- A response node can appear anywhere in the graph, not just at the end
- A single workflow can contain both `response.json` and `ws.send` — an HTTP request that also pushes a real-time update to connected clients

---

## 7. Service Instance

Service instances are created by plugins and passed to node executors. The engine stores them and manages their lifecycle through the Plugin interface.

### 7.1 Opaque Instances (Same Plugin)

When a node uses a service from its own plugin (e.g., `db.query` using a postgres service), the executor receives the concrete type directly. Since the node and service come from the same package, the type is known and safe to use.

### 7.2 Common Service Interfaces (Cross Plugin)

When a node uses a service from a **different** plugin (e.g., `image.resize` using a storage service), the executor can't know the concrete type — that would couple the two plugins. Instead, `pkg/api/` defines common service interfaces that both the providing plugin and the consuming plugin depend on.

```
StorageService
  Read(ctx, path)               → data, error
  Write(ctx, path, data)        → error
  Delete(ctx, path)             → error
  List(ctx, prefix)             → []string, error

CacheService
  Get(ctx, key)                 → value, error
  Set(ctx, key, value, ttl)     → error
  Del(ctx, key)                 → error
  Exists(ctx, key)              → bool, error

ConnectionService
  Send(ctx, channel, data)      → error
  SendSSE(ctx, channel, event, data, id) → error
```

All methods accept Go's `context.Context` as the first parameter for timeout and cancellation propagation.

The storage plugin implements `StorageService` on its service instances. The image plugin declares `ServiceDeps()` with `{ "source": { prefix: "storage", required: true } }` and receives a `StorageService` interface — without ever importing the storage plugin package.

Connection endpoints implement `ConnectionService`. The `ws.send` node calls `services["connections"].Send(channel, data)` directly during `Execute()`. The `sse.send` node calls `SendSSE(channel, event, data, id)`. Both support wildcard channel patterns (`user.*`, `*`). This keeps the engine agnostic to real-time delivery — the nodes handle it themselves.

**Database services have no common interface.** The database plugin uses GORM internally, and its full API surface is too rich and ORM-specific to abstract into a minimal interface. Only the database plugin's own nodes (`db.query`, `db.exec`, etc.) access the database directly. If another plugin needs to write data to a database, it should emit an event or trigger a sub-workflow that contains the database nodes — keeping the database boundary clean.

These interfaces live in `pkg/api/` alongside everything else. They are intentionally minimal — they cover the operations that cross-plugin nodes commonly need. A plugin's own nodes can always use the full concrete type for richer functionality.

Plugins that want their services to be usable by other plugins' nodes **must** implement the appropriate common interface. The engine validates this at startup — if a node's service slot requires prefix `"storage"` and the referenced instance doesn't implement `StorageService`, that's a startup error.

---

## 8. Standard Errors

Standard error types provide consistent error handling across all nodes and plugins. Nodes return these from `Execute()`, and the engine translates them into standardized error responses.

```
ServiceUnavailableError
  Service: string               — which service instance failed
  Cause: error                  — underlying error

ValidationError
  Field: string                 — which field failed validation
  Message: string               — human-readable explanation
  Value: any                    — the invalid value

TimeoutError
  Duration: duration            — how long before timeout
  Operation: string             — what was being done

NotFoundError
  Resource: string              — what type of resource
  ID: string                    — the identifier that wasn't found

ConflictError
  Resource: string              — what type of resource
  Reason: string                — why there's a conflict
```

All standard errors implement Go's `error` interface. The engine uses type assertion to identify them.

### 8.1 Error Output Data

When a node fails and the engine fires the `error` output, the error data is made available on the execution context under the failing node's ID (or `as` alias). The shape is standardized:

```
ErrorData
  Code                          → string ("VALIDATION_ERROR", "NOT_FOUND", etc.)
  Message                       → string (human-readable)
  NodeID                        → string (which node failed)
  NodeType                      → string (the failing node's type)
  Details                       → any (error-type-specific data)
```

Error-handling nodes receive this as the output of the failed node and can inspect it, log it, transform it, or build an appropriate response.

### 8.2 Error Mapping by Trigger Type

How errors are surfaced depends on what triggered the workflow:

**HTTP triggers** — standard errors map to HTTP status codes and the standardized JSON error response:

| Error Type | HTTP Status | Error Code |
|---|---|---|
| `ValidationError` | 422 | `VALIDATION_ERROR` |
| `NotFoundError` | 404 | `NOT_FOUND` |
| `ConflictError` | 409 | `CONFLICT` |
| `ServiceUnavailableError` | 503 | `SERVICE_UNAVAILABLE` |
| `TimeoutError` | 504 | `TIMEOUT` |
| Any other error | 500 | `INTERNAL_ERROR` |

**Event/Worker triggers** — errors are logged with full context (trace ID, node ID, error details). If retries are exhausted, the message is sent to the dead letter topic. No HTTP response is generated.

**Scheduler triggers** — errors are logged. The scheduler records the failure for the job execution history. No HTTP response is generated.

**WebSocket triggers** — errors are logged. Optionally, an error message can be pushed back to the client connection if the workflow includes a `ws.send` in its error path.

Plugins and nodes can define additional error types for their domain, but the standard errors cover the common cases and should be preferred.

---

## 9. Node Registration

A node registration pairs a descriptor with an executor factory:

```
NodeRegistration
  Descriptor                    → NodeDescriptor
  Factory(config)               → NodeExecutor
```

The factory receives the raw node config from the workflow JSON and creates a new executor instance. This is called during workflow compilation — once per node in the graph. The executor inspects the config to determine its outputs (e.g., a switch node reads its cases) and stores any per-node state it needs.

The visual editor calls the same factory when the user edits a node's config, using the returned executor's `Outputs()` to render connection ports in real time.

---

## 10. Summary — What Plugin Authors Implement

To build a Noda plugin, you implement:

1. **The Plugin interface** — declare your name, prefix, and nodes. Implement service lifecycle.
2. **A NodeDescriptor per node** — declare each node's name, service slots, and config schema.
3. **A NodeExecutor per node** — implement a factory that receives config and returns an executor. The executor declares its outputs and implements `Execute`, which receives Go's `context.Context` (`ctx`), the Noda execution context (`nCtx`), config, and services, and returns which output to fire with what data.

Everything else — graph execution, expression evaluation, service resolution, startup validation, error routing, tracing — is handled by the engine.
