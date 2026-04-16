# Noda PDK

The Noda Plugin Development Kit — a thin wrapper around Extism that lets you ship small Wasm modules Noda can invoke from workflows.

## When to reach for Wasm

Use Wasm when you need logic Noda's built-in nodes and expression engine don't cover **and** the logic is either domain-specific (phone formatting, custom validation, proprietary protocols) or stateful (game loops, in-memory aggregation).

If the thing you need is a small pure function likely useful to many projects (`padStart`, `substr`, new crypto helpers), open a feature request instead — it's faster to add a built-in than to ship a Wasm module per project.

## Pure-function ("query-only") module

The shortest module: one `initialize` (required) + one `query` (called from `wasm.query` nodes). No `tick`, no state. Config omits `tick_rate`.

```go
package main

import (
    "github.com/extism/go-pdk"
    "github.com/nodafw/noda-pdk-go/noda"
)

//go:wasmexport initialize
func initialize() int32 {
    if _, err := noda.GetInitInput(); err != nil {
        return noda.Fail(err)
    }
    return 0
}

//go:wasmexport query
func query() int32 {
    raw := pdk.Input() // raw request bytes from the wasm.query node
    return noda.Output(map[string]any{"hello": string(raw)})
}

func main() {}
```

Build:

```bash
tinygo build -o module.wasm -target wasi -buildmode=c-shared .
```

(`-buildmode=c-shared` is required on tinygo ≥ 0.40 so Extism can invoke exports without first running `_start`.)

Wire into `noda.json`:

```json
{
  "wasm_runtimes": {
    "helpers": { "module": "wasm/module.wasm" }
  }
}
```

Call from a workflow:

```json
{
  "type": "wasm.query",
  "services": { "runtime": "helpers" },
  "config": {
    "data": { "name": "world" },
    "timeout": "2s"
  }
}
```

## Stateful / tick-based modules

If you need to run on a timer (game tick, periodic aggregation), export `tick` and set `tick_rate` in `noda.json`. See [`../examples/wasm-counter`](../examples/wasm-counter) for the reference pattern.

## Package layout

| File | Purpose |
|---|---|
| `go/noda/noda.go` | `Call`, `CallInto`, `CallAsync`, `GetInitInput`, `GetTickInput`, `Output`, `Fail` |
| `go/noda/system.go` | `LogInfo`, `LogWarn`, `TriggerWorkflow`, `SetTimer`, `CancelTimer` |
| `go/noda/types.go` | `InitInput`, `TickInput`, `Command`, `AsyncResponse` |
| `go/noda/codec.go` | JSON / MessagePack encoding |
| `go/noda/ws.go` | WebSocket send helpers |
| `go/noda/host.go` | Raw Extism host imports (internal) |

## Further reading

- Full guide: [`docs/04-guides/wasm-development.md`](../docs/04-guides/wasm-development.md)
- Query-only example: [`examples/wasm-helpers`](../examples/wasm-helpers)
- Stateful example: [`examples/wasm-counter`](../examples/wasm-counter)
