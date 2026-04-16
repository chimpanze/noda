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
Input:  { "config": { ... }, "services": [...], "connections": [...] }
Output: { "ok": true } or { "error": "reason" }
```

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

A minimal module that echoes WebSocket messages back to the sender.

```go
package main

import (
    "encoding/json"
    "github.com/extism/go-pdk"
)

//export initialize
func initialize() int32 {
    // Read input
    input := pdk.Input()
    var config map[string]any
    json.Unmarshal(input, &config)

    output, _ := json.Marshal(map[string]any{"ok": true})
    pdk.Output(output)
    return 0
}

//export tick
func tick() int32 {
    input := pdk.Input()
    var tickData map[string]any
    json.Unmarshal(input, &tickData)

    // Echo back any client messages
    if messages, ok := tickData["client_messages"].([]any); ok {
        for _, msg := range messages {
            m := msg.(map[string]any)
            channel := m["channel"].(string)
            data := m["data"]

            // Send back via host function
            payload, _ := json.Marshal(map[string]any{
                "channel": channel,
                "data":    data,
            })
            pdk.Call("noda_call", []byte(`{"service":"ws","operation":"send","payload":`+string(payload)+`}`))
        }
    }

    output, _ := json.Marshal(map[string]any{"ok": true})
    pdk.Output(output)
    return 0
}

//export query
func query() int32 {
    input := pdk.Input()
    var queryData map[string]any
    json.Unmarshal(input, &queryData)

    // Return current state
    response, _ := json.Marshal(map[string]any{
        "status": "running",
        "connections": 0,
    })
    pdk.Output(response)
    return 0
}

func main() {}
```

## Example: Stateful Counter

A module that maintains an in-memory counter, persisted to cache.

```go
package main

import (
    "encoding/json"
    "github.com/extism/go-pdk"
)

var counter int64

//export initialize
func initialize() int32 {
    // Load counter from cache
    payload, _ := json.Marshal(map[string]any{
        "service":   "cache",
        "operation": "get",
        "payload":   map[string]any{"key": "counter"},
    })
    result := pdk.Call("noda_call", payload)

    var res map[string]any
    json.Unmarshal(result, &res)
    if val, ok := res["value"].(float64); ok {
        counter = int64(val)
    }

    output, _ := json.Marshal(map[string]any{"ok": true})
    pdk.Output(output)
    return 0
}

//export tick
func tick() int32 {
    input := pdk.Input()
    var tickData map[string]any
    json.Unmarshal(input, &tickData)

    // Process commands
    if commands, ok := tickData["commands"].([]any); ok {
        for _, cmd := range commands {
            c := cmd.(map[string]any)
            switch c["action"] {
            case "increment":
                counter++
            case "decrement":
                counter--
            case "reset":
                counter = 0
            }
        }

        // Persist to cache
        payload, _ := json.Marshal(map[string]any{
            "service":   "cache",
            "operation": "set",
            "payload":   map[string]any{"key": "counter", "value": counter, "ttl": 0},
        })
        pdk.Call("noda_call_async", payload)
    }

    output, _ := json.Marshal(map[string]any{"ok": true})
    pdk.Output(output)
    return 0
}

//export query
func query() int32 {
    response, _ := json.Marshal(map[string]any{"value": counter})
    pdk.Output(response)
    return 0
}

func main() {}
```

## Building Wasm Modules

### Go

```bash
tinygo build -o module.wasm -target wasi ./main.go
```

Or with the Noda PDK:

```bash
cd pdk && make build
```

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
