# Wasm Module Developer Guide

This guide covers building Wasm modules that run inside Noda's Wasm runtime.

## Overview

Noda's Wasm runtime is built on [Extism](https://extism.org/) (using Wazero). Wasm modules run in a sandboxed tick loop and can interact with Noda services (storage, cache, WebSocket, HTTP, etc.) through host functions.

Use cases for Wasm modules:
- **Pure-function helpers** — formatting, masking, validation, or any small pure function the expression engine doesn't provide (phone E.164, PII redaction, custom checksums). See [Query-only modules](#query-only-modules) below.
- **Game servers** — tick-based game loops at 20+ Hz
- **Bot integrations** — Discord, Slack, or custom protocol gateways
- **Stateful services** — in-memory state with external service access
- **Custom protocols** — any logic that doesn't fit the workflow model

### Wasm vs. Feature Request — which should I reach for?

- **Generic and reusable** (`padStart`, a new crypto primitive, a commonly-needed string function) → open a feature request. A built-in is faster than shipping a Wasm module per project.
- **Domain-specific** (phone formatting with your country's quirks, redaction tuned to your PII fields, your company's signing protocol) → Wasm. Don't wait for Noda to learn your business rules.

## Query-only modules

When you just need a pure function for a workflow expression, export only `initialize` and `query`. No `tick`, no state, no `tick_rate` in config.

```json
{
  "wasm_runtimes": {
    "helpers": { "module": "wasm/helpers.wasm" }
  }
}
```

```go
//go:wasmexport initialize
func initialize() int32 {
    if _, err := noda.GetInitInput(); err != nil { return noda.Fail(err) }
    return 0
}

//go:wasmexport query
func query() int32 {
    raw := pdk.Input()
    // ... parse raw, compute result ...
    return noda.Output(map[string]any{"value": result})
}
```

Noda detects the missing `tick` export and skips the tick loop entirely. `tick_rate` and `tick_timeout` are ignored for query-only modules.

See [`examples/wasm-helpers/`](../../examples/wasm-helpers) for a complete runnable example with phone-E.164 and masking helpers.

## Module Exports

A Wasm module must export these functions:

### `initialize` (required)

Called once at startup. Receives a JSON/MessagePack payload with the module's config and manifest.

```
Input:  {
  "encoding": "json",              // or "msgpack" — how all subsequent payloads are encoded
  "config":   { ... },             // opaque config from wasm_runtimes.<name>.config
  "services": {                    // map keyed by service name
    "redis": { "type": "cache", "operations": ["get", "set", "del", "exists"] }
  }
}
Output: { "ok": true } or { "error": "reason" }
```

(There is no `connections` field — connection endpoints are granted via `wasm_runtimes.<name>.connections` in `noda.json` and used through host calls, not announced in the initialize payload.)

### `tick` (required)

Called at the configured `tick_rate` (default: 20 Hz). Receives accumulated events since the last tick. Each tick call must complete within `tick_timeout` (default: 10x tick budget) or it will be terminated with an error.

```
Input: {
  "dt": 0.05,                    // Delta time in seconds
  "timestamp": 1700000000000,    // Unix milliseconds
  "client_messages": [...],      // Messages from WebSocket clients
  "incoming_ws": [...],          // Inbound WebSocket data (outbound connections)
  "connection_events": [...],    // Connect/disconnect events
  "commands": [...],             // From wasm.send nodes
  "responses": [...],            // Async host call results
  "timers": [...]                // Fired timer labels
}
Output: { "ok": true }
```

### Typed access to `Data` fields (Go PDK)

`client_messages[].data`, `commands[].data`, and `incoming_ws[].data` arrive at
the module already decoded by the codec (JSON or MessagePack) into `any`. If
you're using the Go PDK (`github.com/nodafw/noda-pdk-go/noda`), don't
`json.Marshal`/`Unmarshal` these by hand — that hardcodes JSON and silently
breaks under a MessagePack encoding config. Use `noda.DecodeInto`, which
re-encodes with the module's active codec:

```go
var op CounterOp
if err := noda.DecodeInto(cmd.Data, &op); err != nil {
    // handle malformed command
}
```

### `command` (optional)

Called immediately when a `wasm.send` node targets this module. Runs outside the tick loop for low-latency fire-and-forget operations.

```
Input:  { ... }  // Command data from wasm.send
Output: { "ok": true }
```

### `query` (optional)

Called synchronously when a `wasm.query` node targets this module. Must return within the configured timeout.

```
Input:  { ... }      // Query data from wasm.query
Output: { ... }      // Response data returned to the workflow
```

### `shutdown` (optional)

Called during graceful shutdown. Use it to persist state or clean up.

```
Input:  {}
Output: { "ok": true }
```

## Host API

Wasm modules call Noda services through host functions.

### `noda_call` (synchronous)

Makes a synchronous call to a Noda service. Blocks until the result is available.

```
noda_call(service, operation, payload) -> result
```

### `noda_call_async` (asynchronous)

Makes an asynchronous call. The result arrives in the next tick's `responses` array, tagged with the provided label.

```
noda_call_async(service, operation, payload, label)
```

### Available Services and Operations

#### Storage

| Operation | Payload | Result |
|-----------|---------|--------|
| `read` | `{ "path": "..." }` | `{ "data": bytes, "size": int, "content_type": "..." }` |
| `write` | `{ "path": "...", "data": bytes, "content_type": "..." }` | `{ "ok": true }` |
| `delete` | `{ "path": "..." }` | `{ "ok": true }` |
| `list` | `{ "prefix": "..." }` | `{ "paths": [...] }` |

#### Cache

| Operation | Payload | Result |
|-----------|---------|--------|
| `get` | `{ "key": "..." }` | `{ "value": any }` |
| `set` | `{ "key": "...", "value": any, "ttl": int }` | `{ "ok": true }` |
| `del` | `{ "key": "..." }` | `{ "ok": true }` |
| `exists` | `{ "key": "..." }` | `{ "exists": bool }` |

#### WebSocket (send to clients)

| Operation | Payload | Result |
|-----------|---------|--------|
| `send` | `{ "channel": "...", "data": any }` | `{ "ok": true }` |

#### SSE (send to clients)

| Operation | Payload | Result |
|-----------|---------|--------|
| `send` | `{ "channel": "...", "data": any, "event": "...", "id": "..." }` | `{ "ok": true }` |

#### Stream (Redis Streams)

| Operation | Payload | Result |
|-----------|---------|--------|
| `publish` | `{ "topic": "...", "payload": { ... } }` | `{ "message_id": "..." }` |

#### PubSub (Redis Pub/Sub)

| Operation | Payload | Result |
|-----------|---------|--------|
| `publish` | `{ "topic": "...", "payload": { ... } }` | `{ "ok": true }` |

#### HTTP (outbound)

| Operation | Payload | Result |
|-----------|---------|--------|
| `request` | `{ "method": "...", "url": "...", "headers": {}, "body": any }` | `{ "status": int, "headers": {}, "body": any }` |

#### WebSocket (outbound connections)

| Operation | Payload | Result |
|-----------|---------|--------|
| `connect` | `{ "url": "..." }` | `{ "connection_id": "..." }` |
| `send` | `{ "connection_id": "...", "data": any }` | `{ "ok": true }` |
| `close` | `{ "connection_id": "..." }` | `{ "ok": true }` |

#### Timers

| Operation | Payload | Result |
|-----------|---------|--------|
| `set` | `{ "label": "...", "delay_ms": int }` | `{ "ok": true }` |
| `cancel` | `{ "label": "..." }` | `{ "ok": true }` |

## Configuration

Wasm runtimes are configured in `noda.json`:

```json
{
  "wasm_runtimes": {
    "my-module": {
      "module": "wasm/my-module.wasm",
      "tick_rate": 20,
      "encoding": "json",
      "services": ["redis", "postgres", "files"],
      "connections": ["game-ws"],
      "allow_outbound": {
        "http": ["api.example.com"],
        "ws": ["gateway.discord.gg"]
      },
      "config": {
        "max_players": 100,
        "game_mode": "deathmatch"
      }
    }
  }
}
```

| Field | Description |
|-------|-------------|
| `module` | Path to the `.wasm` file |
| `tick_rate` | Ticks per second (default: 20) |
| `encoding` | `"json"` or `"msgpack"` for host communication |
| `services` | Service instances the module can access |
| `connections` | Connection endpoints the module can send to |
| `allow_outbound` | Whitelisted hosts for outbound HTTP and WebSocket |
| `tick_timeout` | Max duration for a single tick call (e.g. `"5s"`). Default: 10x tick budget. A tick that exceeds this is killed with an error |
| `config` | Opaque config object passed to `initialize` |

## Example: Echo Module

A minimal module that echoes WebSocket messages back to their channel, written against the Noda PDK (never hand-roll the JSON envelopes — `noda.DecodeInto`/`noda.Call` stay correct when the runtime is configured for MessagePack):

```go
package main

import (
    "github.com/nodafw/noda-pdk-go/noda"
)

//go:wasmexport initialize
func initialize() int32 {
    if _, err := noda.GetInitInput(); err != nil {
        return noda.Fail(err)
    }
    return 0
}

//go:wasmexport tick
func tick() int32 {
    input, err := noda.GetTickInput()
    if err != nil {
        return 0
    }

    // Echo every client message back to its channel.
    // "game-ws" is the connection endpoint name granted via
    // wasm_runtimes.<name>.connections in noda.json.
    for _, msg := range input.ClientMessages {
        _, _ = noda.Call("game-ws", "send", map[string]any{
            "channel": msg.Channel,
            "data":    msg.Data,
        })
    }
    return 0
}

//go:wasmexport query
func query() int32 {
    return noda.Output(map[string]any{"status": "running"})
}

func main() {}
```

## Example: Stateful Counter

A module that maintains an in-memory counter, persisted to cache. (This is a condensed version of [`examples/wasm-counter/`](../../examples/wasm-counter) — see that directory for the full runnable module.)

```go
package main

import (
    "github.com/nodafw/noda-pdk-go/noda"
)

var counter int64

//go:wasmexport initialize
func initialize() int32 {
    if _, err := noda.GetInitInput(); err != nil {
        return noda.Fail(err)
    }

    // Load the persisted counter from cache ("redis" = granted service name).
    var res struct {
        Value float64 `json:"value"`
    }
    if err := noda.CallInto("redis", "get", map[string]any{"key": "counter"}, &res); err == nil {
        counter = int64(res.Value)
    }
    return 0
}

//go:wasmexport tick
func tick() int32 {
    input, err := noda.GetTickInput()
    if err != nil {
        return 0
    }

    for _, cmd := range input.Commands {
        var op struct {
            Action string `json:"action"`
        }
        if err := noda.DecodeInto(cmd.Data, &op); err != nil {
            continue
        }
        switch op.Action {
        case "increment":
            counter++
        case "decrement":
            counter--
        case "reset":
            counter = 0
        }
    }

    // Persist asynchronously; the result arrives in a later tick's Responses.
    noda.CallAsync("redis", "set", "persist-counter",
        map[string]any{"key": "counter", "value": counter, "ttl": 0})
    return 0
}

//go:wasmexport query
func query() int32 {
    return noda.Output(map[string]any{"value": counter})
}

func main() {}
```

## Building Wasm Modules

### Go

```bash
tinygo build -o module.wasm -target wasi -buildmode=c-shared .
```

`-buildmode=c-shared` is required on tinygo ≥ 0.40 so exports are callable directly by Extism without running `_start` first — without it, `initialize` panics with `wasmExportCheckRun`. See `pdk/README.md` and [`examples/wasm-helpers/`](../../examples/wasm-helpers) for complete build recipes.

### Rust

```bash
cargo build --target wasm32-wasi --release
```

### AssemblyScript

```bash
asc main.ts --target release --outFile module.wasm
```

## Interacting with Wasm Modules from Workflows

### Fire-and-forget with `wasm.send`

```json
{
  "type": "wasm.send",
  "services": { "runtime": "my-module" },
  "config": {
    "data": {
      "action": "increment"
    }
  }
}
```

### Synchronous query with `wasm.query`

```json
{
  "type": "wasm.query",
  "services": { "runtime": "my-module" },
  "config": {
    "data": { "query": "get_counter" },
    "timeout": "2s"
  }
}
```

## Security

Wasm modules run in a sandboxed Wazero runtime with multiple layers of access control:

### Sandbox Isolation
- No filesystem access — use the storage service instead
- No direct network access — all outbound communication goes through host functions

### Service Access Control
- Only services listed in the module's `services` config array are accessible via `noda_call`
- Calling a service not in the list returns `PERMISSION_DENIED`
- Similarly, only `connections` listed in the config can be used for WebSocket/SSE sends

### Outbound Network Whitelisting
- Outbound HTTP requests are restricted to hosts listed in `allow_outbound.http`
- Outbound WebSocket connections are restricted to hosts listed in `allow_outbound.ws`
- Host matching compares the parsed URL hostname against the whitelist entries
- Requests to non-whitelisted hosts return `PERMISSION_DENIED`

### Input Validation
- All host API calls validate required fields — missing or empty required parameters return `VALIDATION_ERROR`
- For example, cache `get`/`set`/`del` require a non-empty `key`, timers require a non-empty `name`, and `trigger_workflow` requires a non-empty `workflow` ID

### Lifecycle Context
- Async operations (`noda_call_async`, `trigger_workflow`) are bound to the module's lifecycle context, not a detached background context — they are cancelled if the module shuts down

## Resource limits

### Runaway compute

Noda's only enforcement against a Wasm module's runaway computation is
a wall-clock timeout (`wasmCallTimeout`, currently 30 seconds per call,
plus a per-tick `TickTimeout` configurable per module). There is **no
instruction-level metering** ("fuel"), even though some Wasm runtimes
support it.

This is a deliberate limitation of the underlying runtime: Noda uses
Extism on top of wazero, and wazero does not support fuel metering as
a design choice (the maintainers prefer context-cancellation-based
deadlines). Extism v1.7.1 likewise exposes only a wall-clock `Timeout`
field on its manifest.

If your module needs to perform a long synchronous computation, raise
its `TickTimeout` and split the work across multiple ticks rather than
doing it in a single long call. If a module misbehaves and exhausts
CPU, the only process-wide signal is the 30-second timeout; tighter
bounds require either upstream fuel support in Extism/wazero or
running each module in a separate process.
