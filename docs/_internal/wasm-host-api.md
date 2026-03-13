# Noda — Wasm Host API

**Version**: 0.4.0
**Status**: Planning

This document defines the contract between Noda and Wasm modules running in the Wasm runtime. It is intended for developers writing Wasm modules in any supported language (Rust, Go, C, AssemblyScript, JavaScript, etc.).

---

## 1. Overview

Noda's Wasm runtime is built on **Extism** — a WebAssembly framework that handles memory management, data passing, and host function linking. Module authors use the **Extism PDK** (Plugin Development Kit) for their language to read inputs, return outputs, and call host functions.

Noda communicates with Wasm modules using a **tick-based execution model**. Instead of the module running its own infinite loop, Noda calls the module's exported functions at regular intervals and in response to events. Between ticks, the module's Wasm instance stays alive — all linear memory, heap allocations, global variables, and static data persist across calls.

All data crossing the Wasm boundary is serialized bytes. The encoding format is **configurable per module** — JSON (default, human-readable, universal) or MessagePack (binary, compact, faster serialization for high-frequency modules). The Extism PDK handles serialization in each language — module authors work with native types regardless of the wire format.

---

## 2. Encoding

The encoding format is set per module in the runtime config:

```json
{
  "wasm_runtimes": {
    "game": {
      "module": "game.wasm",
      "tick_rate": 60,
      "encoding": "msgpack"
    }
  }
}
```

| Encoding | Value | Use case |
|---|---|---|
| JSON | `"json"` (default) | Development, debugging, low-frequency modules |
| MessagePack | `"msgpack"` | High-frequency modules (30+ Hz), large tick payloads |

The encoding applies to all data crossing the boundary: tick inputs, host function payloads and responses, query inputs and outputs. The module's `initialize` input includes the active encoding so the PDK can configure itself.

The Noda PDK (a thin wrapper around the Extism PDK, provided per language) handles encoding/decoding transparently. Module authors call `noda.call("cache", "get", payload)` and receive native types — they never manually serialize or parse wire bytes.

JSON and MessagePack share the same data model (maps, arrays, strings, numbers, booleans, null), so the schema for every operation is identical regardless of encoding. Switching encoding is a config change, not a code change.

---

## 3. Module Lifecycle

A Wasm module goes through the following lifecycle, managed entirely by Noda:

```
load → initialize → [tick / command / query]... → shutdown → unload
```

### 3.1 Load

Noda loads the `.wasm` binary via Extism, binds the host functions, and creates the module instance. The module's linear memory is initialized.

### 3.2 Initialize

Noda calls the module's `initialize` export once. The input contains the module's configuration, the active encoding, and a manifest of available services and operations:

```json
{
  "encoding": "json",
  "config": {
    "tick_rate": 20,
    "max_players": 100
  },
  "services": {
    "game-storage": {
      "type": "storage",
      "operations": ["read", "write", "delete", "list"]
    },
    "app-cache": {
      "type": "cache",
      "operations": ["get", "set", "del", "exists"]
    },
    "game-ws": {
      "type": "ws",
      "operations": ["send"]
    },
    "main-stream": {
      "type": "stream",
      "operations": ["emit"]
    }
  },
  "outbound": {
    "http": ["api.example.com"],
    "ws": ["stream.example.com"]
  },
  "system": ["log", "trigger_workflow", "set_timer", "clear_timer"]
}
```

- **encoding** — the active wire format (`"json"` or `"msgpack"`). The PDK uses this to configure serialization.
- **config** — opaque user-defined configuration from `wasm_runtimes` in `noda.json`. Noda resolves `$env()` references before passing — the module receives actual values, never variable references.
- **services** — which service instances the module can access, their type, and available operations.
- **outbound** — which external hosts the module can reach via HTTP or WebSocket.
- **system** — which system-level operations are available.

The module should use this phase to allocate its internal state, initialize data structures, and optionally load persisted state via `noda_call("game-storage", "read", ...)`.

**Return:** A status object or empty on success. A non-zero return code indicates initialization failure — Noda logs the error and does not start ticking.

### 3.3 Tick

Noda calls the module's `tick` export at the configured `tick_rate` (in Hz). The tick input contains events accumulated since the last tick:

```json
{
  "dt": 50,
  "timestamp": 1709900000000,
  "client_messages": [
    {
      "endpoint": "game-ws",
      "channel": "game.lobby-1",
      "user_id": "abc-123",
      "data": { "action": "move", "x": 10, "y": 5 }
    }
  ],
  "incoming_ws": [
    {
      "connection": "exchange-feed",
      "data": { "type": "price_update", "symbol": "BTC", "price": 65000 }
    }
  ],
  "connection_events": [
    { "endpoint": "game-ws", "channel": "game.lobby-1", "user_id": "abc-123", "event": "connect" },
    { "endpoint": "game-ws", "channel": "game.lobby-1", "user_id": "def-456", "event": "disconnect" },
    { "connection": "discord-gateway", "event": "reconnected" }
  ],
  "commands": [
    { "source": "workflow", "data": { "type": "admin_broadcast", "message": "Server restart in 5min" } }
  ],
  "responses": {
    "place-order": {
      "status": "ok",
      "data": { "order_id": "ord-789", "filled": true }
    },
    "fetch-llm": {
      "status": "error",
      "error": { "code": "TIMEOUT", "message": "Request timed out after 10s" }
    }
  },
  "timers": ["save-state"]
}
```

- **dt** — milliseconds since the last tick. Module uses this for time-based calculations (physics, cooldowns, intervals).
- **timestamp** — current Unix timestamp in milliseconds.
- **client_messages** — messages received from clients connected to Noda WebSocket/SSE endpoints mapped to this module. Each message includes the `endpoint` name, `channel`, authenticated `user_id` (from the endpoint's auth middleware, null if unauthenticated), and the `data` payload.
- **incoming_ws** — messages received on outbound WebSocket connections managed by Noda (e.g., Discord gateway, exchange feed). Keyed by the connection `id` set during `ws_connect`.
- **connection_events** — lifecycle events for both client connections (from Noda endpoints) and outbound connections (managed by Noda). Client events include `endpoint`, `channel`, and `user_id`. Outbound events include `connection` (the id). The `event` field is `"connect"`, `"disconnect"`, or `"reconnected"`. The module uses `"reconnected"` to handle protocol-specific recovery (e.g., Discord RESUME vs IDENTIFY).
- **commands** — data sent by workflows via the `wasm.send` node. Accumulated between ticks and delivered in batch.
- **responses** — results from `noda_call_async` calls made in previous ticks, keyed by the label provided when the async call was made. Each entry has a `status` (`"ok"` or `"error"`) and either `data` or `error`.
- **timers** — named timers that have fired since the last tick (see Section 6.3).

All fields are optional — Noda omits empty arrays/objects from the tick input to minimize payload size.

During tick execution, the module processes events, updates internal state, and makes `noda_call` or `noda_call_async` host function calls as needed.

**Return:** A status object or empty. A non-zero return code is logged as an error.

**Tick budget:** If a tick exceeds its time budget (`1000 / tick_rate` milliseconds), Noda logs a warning with the actual duration. This helps module authors detect performance problems during development. Consecutive budget overruns are logged at increasing severity. Noda does not kill slow ticks — the next tick is simply delayed and receives a larger `dt`.

### 3.4 Query

Noda calls the module's `query` export when a workflow executes a `wasm.query` node. This is a synchronous request-response: the workflow blocks until the module returns.

```json
{
  "type": "get_leaderboard",
  "limit": 10
}
```

The input is the `data` field from the `wasm.query` node's config (with expressions already resolved by the workflow engine).

**Return:** Response data. This becomes the output of the `wasm.query` node in the parent workflow. A non-zero return code causes the `wasm.query` node to fail with an error.

Queries must be fast — the `wasm.query` node has a mandatory `timeout` in its config, and the workflow's `context.Context` deadline applies. The module should read from its in-memory state, not perform expensive I/O during a query.

**Serialization:** Queries are serialized with respect to ticks — a query is never called during a tick. If a query arrives while a tick is running, it waits. If multiple queries queue up, they are dispatched in order.

### 3.5 Command

Noda calls the module's `command` export when a workflow executes a `wasm.send` node. This is async from the workflow's perspective — the workflow does not wait for a response.

If the module exports `command`, Noda calls it immediately (between ticks). If the module does not export `command`, the data is buffered and delivered in the next tick's `commands` array.

Immediate delivery is useful when the module needs to react without waiting for the next tick (e.g., an admin action that must take effect instantly).

### 3.6 Shutdown

Noda calls the module's `shutdown` export during graceful shutdown. The module should:

1. Persist any critical state via `noda_call("game-storage", "write", ...)`
2. Release internal resources
3. Return promptly

A shutdown deadline applies (from Noda's graceful shutdown config). After the deadline, the module instance is force-terminated.

---

## 4. Host API — `noda_call` and `noda_call_async`

The module communicates with Noda through two host functions registered with Extism:

### 4.1 `noda_call` — Synchronous

```
noda_call(service, operation, payload) → response
```

- **service** — the service instance name (e.g., `"game-storage"`, `"app-cache"`, `"game-ws"`). Empty string `""` for system-level operations.
- **operation** — the operation to perform (e.g., `"read"`, `"get"`, `"send"`, `"log"`).
- **payload** — serialized bytes containing the operation's input parameters.
- **response** — serialized bytes containing the operation's result. Empty for void operations.

The call blocks the module until Noda completes the operation and returns. Use for fast operations only: cache reads (<1ms), WebSocket sends (buffered, non-blocking), logging, event emission.

### 4.2 `noda_call_async` — Asynchronous

```
noda_call_async(service, operation, payload, label)
```

- **service**, **operation**, **payload** — same as `noda_call`.
- **label** — a string chosen by the module to identify this call. Must be unique among currently pending async calls. If the module reuses a label that is still pending, Noda returns a `VALIDATION_ERROR` immediately — the previous pending call is not affected. The module should use descriptive, unique labels (e.g., include a counter or context: `"order-123"`, `"save-tick-500"`).

The call returns immediately — Noda performs the operation on its own goroutine. The result is delivered in the next tick's `responses` field, keyed by the label:

```json
{
  "responses": {
    "my-label": {
      "status": "ok",
      "data": { ... }
    }
  }
}
```

Or on failure:

```json
{
  "responses": {
    "my-label": {
      "status": "error",
      "error": { "code": "TIMEOUT", "message": "..." }
    }
  }
}
```

Use for slow operations: outbound HTTP requests, large storage reads/writes, any I/O that could exceed the tick budget.

**Guideline:** If the operation might take more than 5ms, use `noda_call_async`. Cache gets and WebSocket sends are safe to call synchronously. HTTP requests and large storage operations should always be async.

### 4.3 Error Handling

`noda_call` returns a response on success. On failure, Noda returns an error through Extism's error mechanism (accessible via `pdk.GetError()` or equivalent in each PDK). The error is a structured object:

```json
{
  "code": "NOT_FOUND",
  "message": "Key 'leaderboard' does not exist",
  "operation": "cache.get"
}
```

Error codes match Noda's standard error codes: `NOT_FOUND`, `VALIDATION_ERROR`, `SERVICE_UNAVAILABLE`, `TIMEOUT`, `PERMISSION_DENIED`, `INTERNAL_ERROR`.

`PERMISSION_DENIED` is returned when the module calls a service or operation not listed in its manifest.

For `noda_call_async`, errors are delivered in the tick's `responses` field with `"status": "error"` — the module never receives an error from the async call itself (since it returns immediately).

---

## 5. Service Operations

Each service type exposes a set of operations. The module can only call operations on services listed in its manifest (provided during `initialize`).

### 5.1 Storage Operations (`storage` type)

**read** — Read a file or object from storage.
```json
// payload
{ "path": "worlds/world-1.json" }
// response
{ "data": "...", "size": 1024, "content_type": "application/json" }
```

**write** — Write a file or object to storage.
```json
// payload
{ "path": "worlds/world-1.json", "data": "...", "content_type": "application/json" }
// response
{}
```

**delete** — Delete a file or object.
```json
// payload
{ "path": "worlds/world-1.json" }
// response
{}
```

**list** — List files/objects under a prefix.
```json
// payload
{ "prefix": "worlds/" }
// response
{ "paths": ["worlds/world-1.json", "worlds/world-2.json"] }
```

Storage operations can be called sync or async. Small reads (config files, state snapshots <1MB) are fine synchronously. Large reads/writes should use `noda_call_async`.

### 5.2 Cache Operations (`cache` type)

**get** — Read a cached value.
```json
// payload
{ "key": "leaderboard" }
// response
{ "value": { "scores": [...] } }
```
Returns error with code `NOT_FOUND` if the key does not exist.

**set** — Write a cached value with optional TTL.
```json
// payload
{ "key": "leaderboard", "value": { "scores": [...] }, "ttl": 300 }
// response
{}
```
`ttl` is in seconds. Omit for no expiration.

**del** — Delete a cached value.
```json
// payload
{ "key": "leaderboard" }
// response
{}
```

**exists** — Check if a key exists.
```json
// payload
{ "key": "leaderboard" }
// response
{ "exists": true }
```

Cache operations are fast (<1ms typically) and safe to call synchronously.

### 5.3 Connection Operations (`ws` and `sse` types)

**send** — Send a message to connected clients on a Noda WebSocket/SSE endpoint.
```json
// payload
{ "channel": "game.lobby-1", "data": { "type": "state_update", "entities": [...] } }
// response
{}
```

For SSE endpoints, additional fields are available:
```json
// payload
{ "channel": "feed.live", "event": "score-update", "data": { ... }, "id": "evt-123" }
// response
{}
```

Channel patterns support wildcards: `user.*`, `game.*`, `*`.

Connection sends are buffered by Noda and delivered asynchronously — they do not block the tick. Always call synchronously.

### 5.4 Stream and PubSub Operations (`stream` and `pubsub` types)

**emit** — Publish an event to a stream or pubsub service.
```json
// payload
{ "topic": "player.scored", "payload": { "player_id": "p1", "score": 100 } }
// response
{}
```

Stream emits are durable (consumed by workers with acknowledgment). PubSub emits are real-time fan-out (all current subscribers receive it, no persistence). The service type determines the behavior — the payload is identical for both.

Event emission is fast and safe to call synchronously.

### 5.5 HTTP Operations (outbound)

**request** — Make an outbound HTTP request to a whitelisted host.
```json
// payload
{
  "method": "POST",
  "url": "https://discord.com/api/v10/channels/12345/messages",
  "headers": { "Authorization": "Bot ..." },
  "body": { "content": "Hello from Noda!" }
}
// response
{
  "status": 200,
  "headers": { "content-type": "application/json" },
  "body": { "id": "msg-abc-123" }
}
```

The URL's host must be in the module's `allow_outbound.http` whitelist. Requests to non-whitelisted hosts return `PERMISSION_DENIED`.

**HTTP requests should almost always use `noda_call_async`.** External API calls typically take 50-500ms, which exceeds most tick budgets. The pattern:

```
// During tick — fire async
noda_call_async("", "http_request", payload, "send-discord-msg")

// Next tick — check response
if responses["send-discord-msg"].status == "ok" {
    // message sent successfully
}
```

Synchronous HTTP calls are allowed but will block the tick and trigger budget warnings.

---

## 6. System Operations

System operations use an empty string `""` as the service name.

### 6.1 Logging

```json
// noda_call("", "log", payload)
{ "level": "info", "message": "Tick complete", "fields": { "entities": 42, "dt": 50 } }
```

Levels: `debug`, `info`, `warn`, `error`. Log output goes through Noda's structured logging system with the module name as context. Always synchronous, non-blocking.

### 6.2 Trigger Workflow

```json
// noda_call("", "trigger_workflow", payload)
{ "workflow": "ban-user", "input": { "user_id": "abc", "reason": "cheating" } }
// response
{ "trace_id": "tr-abc-123" }
```

Invokes the workflow engine directly (in-process, not via Redis). The workflow runs asynchronously — `noda_call` returns immediately with a trace ID. The workflow execution has `TriggerData.Type = "wasm"`. The `input` object becomes `$.input` in the workflow directly — no trigger mapping expressions.

Always synchronous (the call itself returns immediately; the workflow runs in the background).

### 6.3 Timers

Modules can register named timers that fire on a schedule:

```json
// noda_call("", "set_timer", payload)
{ "name": "save-state", "interval": 30000 }
// response
{}
```

```json
// noda_call("", "clear_timer", payload)
{ "name": "save-state" }
// response
{}
```

`interval` is in milliseconds. When a timer fires, its name appears in the `timers` array of the next tick input. Timers are not precise — they fire on the next tick after the interval elapses.

### 6.4 Emit Event

Event emission is not a system operation — it uses regular service calls to stream or pubsub service instances:

```json
// noda_call("main-stream", "emit", payload)
{
  "topic": "player.scored",
  "payload": { "player_id": "p1", "score": 100 }
}
// response
{}
```

The service name references a stream or pubsub instance from the module's manifest. The operation is `"emit"` for both service types.

---

## 7. Outbound WebSocket Connections

For modules that need persistent outbound connections (e.g., Discord gateway, exchange data feeds), Noda manages the connection lifecycle.

### 7.1 Connection Management

During `initialize`, the module requests outbound connections:

```json
// noda_call("", "ws_connect", payload)
{
  "id": "discord-gateway",
  "url": "wss://gateway.discord.gg/?v=10&encoding=json",
  "headers": { "Authorization": "Bot ..." }
}
// response
{ "status": "connected" }
```

The URL's host must be in the module's `allow_outbound.ws` whitelist.

**Noda manages the connection**, including:
- TCP/TLS establishment and reconnection on failure
- WebSocket ping/pong keepalive
- Protocol-level heartbeats (configurable per connection)
- Buffering incoming messages for delivery in the next tick
- Reconnection with configurable backoff

The module receives clean application-level messages in the tick's `incoming_ws` array.

**Connection lifecycle events** are delivered in the tick's `connection_events` array:

```json
{
  "connection_events": [
    { "connection": "discord-gateway", "event": "reconnected" },
    { "connection": "exchange-feed", "event": "disconnected", "reason": "server closed" }
  ]
}
```

Events: `"reconnected"` (Noda re-established the connection after a drop), `"disconnected"` (connection lost, reconnection will be attempted if configured). The module uses `"reconnected"` to handle protocol-specific recovery — for example, sending RESUME vs IDENTIFY after a Discord reconnection.

### 7.2 Sending on Outbound Connections

```json
// noda_call("", "ws_send", payload)
{ "id": "discord-gateway", "data": { "op": 2, "d": { "token": "...", "intents": 513 } } }
// response
{}
```

### 7.3 Connection Configuration

```json
// noda_call("", "ws_configure", payload)
{
  "id": "discord-gateway",
  "heartbeat_interval": 41250,
  "heartbeat_payload": { "op": 1, "d": null },
  "reconnect": {
    "enabled": true,
    "max_attempts": 10,
    "backoff": "exponential",
    "initial_delay": 1000
  }
}
// response
{}
```

When `heartbeat_interval` and `heartbeat_payload` are set, Noda sends the heartbeat automatically between ticks — the module doesn't need to track heartbeat timing.

### 7.4 Closing Connections

```json
// noda_call("", "ws_close", payload)
{ "id": "discord-gateway", "code": 1000, "reason": "shutting down" }
// response
{}
```

---

## 8. Module Exports Summary

The module must export these functions via the Extism PDK:

| Export | Called by | Input | Output | Required |
|---|---|---|---|---|
| `initialize` | Noda at startup | Config + service manifest | Status | Yes |
| `tick` | Noda at tick_rate Hz | Events + delta time | Status | Yes |
| `command` | `wasm.send` workflow node | Arbitrary data from node config | Status | No |
| `query` | `wasm.query` workflow node | Arbitrary data from node config | Response data | No |
| `shutdown` | Noda at graceful stop | Empty | Empty | Yes |

**`command` vs tick delivery:** Data sent via `wasm.send` from a workflow can be delivered in two ways. If the module exports `command`, Noda calls it immediately (between ticks). If the module does not export `command`, the data is buffered and delivered in the next tick's `commands` array.

**`query`** is always called immediately and synchronously — it cannot be deferred to a tick because the calling workflow is waiting for the response. Queries are serialized with respect to ticks and other queries.

---

## 9. Extism-Specific Details

### 9.1 PDK Usage

Module authors use the Noda PDK for their language (a thin wrapper around the Extism PDK). The Noda PDK provides:

- Automatic serialization/deserialization based on the configured encoding (JSON or MessagePack)
- Typed wrappers around `noda_call` and `noda_call_async`
- Tick input parsing into native structures
- Error handling

The raw Extism PDK functions are also available for advanced use:

- `pdk.Input()` — read the raw input bytes for the current call
- `pdk.Output()` — write raw output bytes
- `pdk.Var.Get/Set` — module-scope persistent variables (survive across calls, managed by Extism)

### 9.2 Memory

Wasm linear memory persists across all calls (`initialize`, `tick`, `command`, `query`, `shutdown`). The module instance is never re-created between calls. Global variables, heap allocations, and static data all survive.

Extism manages the memory protocol for passing data between host and guest. Module authors never deal with pointer arithmetic, allocation, or deallocation — the PDK handles it.

### 9.3 Threading and Call Serialization

Extism modules are single-threaded. Noda guarantees that calls to a module instance are serialized — `tick`, `command`, and `query` are never called concurrently on the same instance.

Call priority: if a `query` arrives during a tick, it waits for the tick to complete. If multiple queries queue up, they are dispatched in order. `command` calls (if the export exists) are dispatched between ticks when no other call is active.

### 9.4 State Persistence and Crash Recovery

Wasm linear memory persists across calls but **does not survive process crashes or restarts**. If a Noda instance crashes, all in-memory state in all Wasm modules on that instance is lost.

Not all modules need persistent state — a stateless bot that only reacts to incoming messages and calls APIs loses nothing on crash. For stateful modules (game servers, trading bots with open positions), the module author is responsible for persisting state at appropriate intervals using `noda_call` or `noda_call_async` to a storage or cache service.

The recommended pattern is a timer-based snapshot:

1. During `initialize`, set a timer: `noda_call("", "set_timer", { "name": "save-state", "interval": 30000 })`
2. During `tick`, check `timers` for `"save-state"` and write state to storage via `noda_call_async`
3. During `initialize` on restart, load the last snapshot from storage

The interval controls the trade-off: shorter intervals mean less data loss but more I/O. The module author chooses based on their use case — a chat bot might save every 60 seconds, a game server every 5 seconds, and a trading bot after every significant state change.

Noda does not provide automatic WAL (Write-Ahead Log) or snapshotting. This is a deliberate simplicity trade-off — automated state management would require the framework to understand the module's internal data model, which violates the principle that Noda provides infrastructure while the module owns its logic.

---

## 10. Testing Wasm Modules

Wasm modules depend on Noda's tick loop and host API, which makes testing outside of Noda non-trivial. The Noda PDK for each language includes a **test harness** that simulates the Noda runtime locally.

The test harness provides:

- **Mock `noda_call` / `noda_call_async`** — register expected calls and their return values. Assert that the module made the expected calls with the expected arguments.
- **Tick simulation** — construct tick input objects and call the module's `tick` export directly. Chain multiple ticks to test sequences.
- **Query simulation** — call the module's `query` export and assert the response.
- **Connection event simulation** — inject connect/disconnect/reconnect events into tick inputs.
- **Async response injection** — place responses in the tick's `responses` field to simulate completed async calls.

Example test flow (pseudocode):

```
harness = NodaTestHarness::new("game.wasm")
harness.mock("game-storage", "read", returns: { "data": saved_state })
harness.call_initialize(config)

harness.tick({ "dt": 50, "client_messages": [
  { "endpoint": "game-ws", "channel": "lobby", "user_id": "p1", "data": { "action": "move", "x": 10 } }
]})

assert harness.calls_made("game-ws", "send").count == 1
assert harness.calls_made("game-ws", "send")[0].payload["channel"] == "lobby"

result = harness.query({ "type": "get_player", "player_id": "p1" })
assert result["x"] == 10
```

The test harness runs the actual Wasm binary via Extism, so it tests real module code — not mocks of the module. Only the Noda side (services, connections) is mocked.

---

## 11. Use Case Patterns

### 11.1 Game Server

A game module stores its world state in Wasm linear memory (persists across ticks). The tick cycle:

1. Process `connection_events` — add/remove players on connect/disconnect
2. Process `client_messages` — apply player inputs (movement, actions)
3. Run simulation — physics, AI, collision detection using `dt` for time-stepping
4. Broadcast — `noda_call("game-ws", "send", ...)` with state delta to connected players
5. Periodic save — when `"save-state"` appears in `timers`, use `noda_call_async` to write state to storage without blocking the tick

Queries (from `wasm.query` workflow nodes) read directly from in-memory state — leaderboards, player info, world stats.

### 11.2 Discord / Slack / Telegram Bot

The module connects to the platform's gateway WebSocket during `initialize` via `noda_call("", "ws_connect", ...)`. Noda handles heartbeats and reconnection. The tick cycle:

1. Check `connection_events` for `"reconnected"` — send platform-specific session resume
2. Process `incoming_ws` — handle gateway events (messages, reactions, member updates)
3. Respond via `noda_call_async("", "http_request", ...)` — platform REST APIs for sending messages (always async, typically 100-500ms)
4. Check `responses` — handle results of async HTTP calls from previous ticks
5. For complex operations (moderation, data lookups) — `noda_call("", "trigger_workflow", ...)` to invoke Noda workflows

Key detail: bots send messages via the platform's HTTP REST API, not the gateway WebSocket. These HTTP calls must use `noda_call_async` to avoid blocking the tick.

### 11.3 Trading Bot

The module connects to an exchange's WebSocket feed for real-time market data during `initialize`. The tick cycle:

1. Process `incoming_ws` — price updates, order book changes, trade executions
2. Run trading logic — strategy evaluation, signal generation, risk checks (all in-memory)
3. Place orders via `noda_call_async("", "http_request", ...)` — exchange REST API
4. Check `responses` — order confirmations/rejections from previous ticks
5. Log and emit events — `noda_call("main-stream", "emit", ...)` for trade history consumed by analytics workflows

All exchange HTTP calls must be async. A trading bot at 10Hz has a 100ms tick budget — a synchronous HTTP call to an exchange would consume the entire budget or more.

### 11.4 AI Agent

The module runs an observe-decide-act loop each tick:

1. Gather observations — `client_messages`, `incoming_ws`, `responses` from previous actions
2. Decide — call an LLM via `noda_call_async("", "http_request", ...)` with the prompt
3. Check `responses` — when the LLM response arrives (may take several ticks), parse the action
4. Act — trigger workflows, send messages, write to storage based on the LLM's decision
5. Persist memory — periodic `noda_call_async` to storage for the agent's conversation history

LLM calls take 1-30 seconds. The agent fires the request async and continues ticking. When the response arrives (possibly many ticks later), the agent processes it. The agent remains responsive to commands and queries between LLM calls.

---

## 12. Configuration Reference

The module is configured in `noda.json` under `wasm_runtimes`:

```json
{
  "wasm_runtimes": {
    "game": {
      "module": "game.wasm",
      "tick_rate": 20,
      "encoding": "msgpack",
      "services": ["app-cache", "game-storage"],
      "connections": ["game-ws"],
      "allow_outbound": {
        "http": [],
        "ws": []
      },
      "config": {
        "max_players": 100
      }
    }
  }
}
```

| Field | Type | Required | Description |
|---|---|---|---|
| `module` | string | Yes | Path to the `.wasm` binary file |
| `tick_rate` | int | Yes | Ticks per second (Hz). Range: 1-120 |
| `encoding` | string | No | Wire format: `"json"` (default) or `"msgpack"` |
| `services` | []string | Yes | Service instance names the module can access |
| `connections` | []string | No | Noda WebSocket/SSE endpoint names the module can push to |
| `allow_outbound.http` | []string | No | Whitelisted hosts for outbound HTTP |
| `allow_outbound.ws` | []string | No | Whitelisted hosts for outbound WebSocket |
| `config` | object | No | Opaque config passed to `initialize`. `$env()` references are resolved by Noda. |

---

## 13. Security Model

- **Service isolation** — the module can only access services listed in its config. `noda_call` to any other service returns `PERMISSION_DENIED`.
- **Network isolation** — outbound HTTP and WebSocket connections are only allowed to whitelisted hosts. No wildcards.
- **Memory isolation** — each module instance has its own linear memory space, enforced by the Wasm sandbox. Instances cannot access each other's memory.
- **No filesystem access** — all storage goes through `noda_call` to a storage service. No direct filesystem operations.
- **No database access** — modules cannot access the database directly. For database operations, modules trigger workflows that contain database nodes. This keeps the database boundary clean and maintains GORM as the single database interface.
- **Call serialization** — `tick`, `command`, and `query` are never called concurrently on the same instance, preventing data races.
- **Resource limits** — Extism supports memory limits and execution timeouts per module. Noda configures these from the manifest.
- **Secret isolation** — environment variables are resolved by Noda in the config before passing to the module. Modules cannot read arbitrary environment variables — only values explicitly included in their config.
