# Wasm Runtime Hardening (Tranche A) — Design

Date: 2026-07-05
Source: `REVIEW-FINDINGS-2026-07-05.md` — the 9 `wasm-pdk-*` findings (4 High, 5 Medium).
Branch/worktree (planned): `feat/wasm-runtime-hardening` in `.worktrees/wasm-runtime-hardening`, off `main`.

## Why

The clean-slate Go review (2026-07-05) found `internal/wasm` to be the single densest defect cluster — 4 of the 10 High findings. The wasm runtime cannot actually interrupt a guest, silently corrupts guest state after a tick timeout, hides host-call errors (including permission denials) from the guest, and breaks entirely under `encoding: "msgpack"`. Five further Medium correctness bugs sit in the same surface. This tranche fixes all nine as one coherent change because they share two files' worth of surface (the host-call boundary and the module call path) and the same PDK contract.

**ABI decision (user-approved):** clean break. The host↔guest wire contract changes; the in-repo example modules are recompiled against the updated PDK; the break is noted in CHANGELOG and the PDK version. No backward-compat shim.

## Findings in scope

| ID | Sev | Summary |
|---|---|---|
| wasm-pdk-1 | High | Guest execution uninterruptible; timeouts abandon goroutines; hung `query` deadlocks shutdown |
| wasm-pdk-2 | High | Concurrent `Plugin.Call` on the same instance after a tick timeout corrupts guest memory |
| wasm-pdk-3 | High | Host-call errors written as ordinary output; PDK never sees them (permission denials consumed as data) |
| wasm-pdk-4 | High | `encoding: "msgpack"` breaks every host call (host functions hardcode `jsonCodec`) |
| wasm-pdk-5 | Med | PDK `SetTimer` sends `interval_ms`; host reads `interval` → timers never fire |
| wasm-pdk-6 | Med | `wasm.send` command misrouted to the `query` export when a module exports both |
| wasm-pdk-7 | Med | Data race on `m.lifecycleCtx` (Stop reassigns while async goroutines read) |
| wasm-pdk-8 | Med | `Gateway.Connect` with a duplicate id orphans the old connection |
| wasm-pdk-10 | Med | No default memory limit → up to 4 GiB linear memory per module |

## Verified library facts (extism go-sdk v1.7.1, wazero v1.9.0)

- `plugin.go:129-130`: `if manifest.Timeout > 0 { runtimeConfig = runtimeConfig.WithCloseOnContextDone(true) }`. Setting `manifest.Timeout` is the **only** switch that makes guest execution interruptible.
- `extism.go:453` `CallWithContext(ctx, name, data)` exists and honors ctx deadline/cancellation; with `WithCloseOnContextDone` a ctx-done terminates the guest.
- `extism.go` `SetInput`→`reset` frees prior allocations and clobbers shared kernel output/error registers; `wazero api/wasm.go:378` "Call is not goroutine-safe". These are the mechanism behind wasm-pdk-2.

## Design

### Unit 1 — Interruptible, serialized guest execution (wasm-pdk-1, wasm-pdk-2)

The tick loop (`tick.go tickLoop`) is already the single point that should own every guest call. The bug is that `callWithTimeout` (`module.go:417-442`) spawns a goroutine per call and abandons it, and `processQuery` (`tick.go:135`) calls the plugin with no timeout at all.

Changes:
1. **`PluginInstance` interface** (`module.go:25-29`): add `CallWithContext(ctx context.Context, name string, data []byte) (uint32, []byte, error)`. The real `*extism.Plugin` already satisfies it; test fakes implement it (and may honor ctx to simulate a hung guest).
2. **Manifest** (`runtime.go loadModuleFromBytes`): set `manifest.Timeout` to a ceiling in milliseconds — `max(TickTimeout, wasmCallTimeout)` — so extism enables `WithCloseOnContextDone`.
3. **`callWithTimeout` → synchronous interruptible call**: give it an explicit parent-context parameter (`callWithTimeout(parent context.Context, name string, data []byte, timeout time.Duration)`); build `ctx, cancel := context.WithTimeout(parent, timeout)` and call `m.Plugin.CallWithContext(ctx, name, data)` **inline** (no goroutine). Tick/query pass the module's stable `shutdownCtx` (Unit 7) so `Stop` can cancel an in-flight call; `Stop`'s own post-`tickDone` `shutdown` export call passes a fresh `context.Background()`-rooted context (the loop has already exited). Remove the per-call goroutine and the `outstandingCalls.Add/Done` around it (async host calls keep their own tracking).
4. **`processQuery`** routes through the same timed call path (deadline = `wasmCallTimeout`), so a hung `query`/`command` can no longer block the tick loop or `Stop`'s `<-tickDone`.
5. **Closed-module handling**: a timed-out call terminates the guest and closes the module. `executeTick`/`processQuery` detect the closed/failed module (error string check + a `failed atomic.Bool` set on timeout), log once, stop the tick loop, and cause the module to report unhealthy. `Query`/`SendCommand` after failure return a clear error. This replaces the current corrupt-and-continue behavior.

Result: exactly one goroutine ever calls the plugin during `running`; a runaway guest is terminated rather than abandoned; shutdown never deadlocks on a hung export.

### Unit 2 — Host-call error envelope (wasm-pdk-3), clean ABI break

Define one envelope emitted by the host and decoded by the PDK for `noda_call`:

- Success: `{"ok": true, "data": <result>}` — `data` omitted for void operations.
- Error: `{"ok": false, "error": {"code": <string>, "message": <string>}}`.

Host (`runtime.go buildHostFunctions`):
- `noda_call` wraps every return (dispatch success, dispatch error, ReadBytes/unmarshal/marshal failure) in the envelope. `stack[0]==0` is reserved strictly for genuine void success (no bytes to return). Error codes come from the dispatcher error prefix (`PERMISSION_DENIED`, `VALIDATION_ERROR`, `SERVICE_UNAVAILABLE`, `NOT_FOUND`, else `INTERNAL_ERROR`) via a small `classifyError` helper.
- `noda_call_async` is unchanged on the wire (results already flow through the typed `AsyncResponse` with `Status`/`Error`), but shares the codec fix.

PDK (`pdk/go/noda/host.go`, `noda.go`):
- `call()` decodes the envelope: `ok:false` → return `&HostError{Code, Message}` (new typed error); `ok:true` → return `data` bytes; `stack[0]==0` → `(nil, nil)` void success.
- `Call`/`CallInto` propagate the error. `CallInto` unmarshals `data` only on success.
- The `pdk.GetError()` comment/contract (unimplementable from an extism host function) is removed from code and `docs/_internal/wasm-host-api.md` §4.3.

### Unit 3 — Codec fix (wasm-pdk-4)

- Both host functions use `dispatcher.module.Codec` instead of `&jsonCodec{}`. The module reference is set before any export runs (`runtime.go:83-85`).
- Add a numeric coercion helper `toInt64(any) (int64, bool)` / `toFloat(any) (float64, bool)` handling `float64`, `int64`, `uint64`, `int`, `json.Number`. Apply at the `float64` assertion sites: `hostapi.go` set_timer `interval_ms` (line ~187), cache `ttl` (line ~263); `gateway.go` ws `code` (line ~167), `heartbeat_interval` (~210). Fixes silent failures for msgpack modules whose numbers decode as `int64`/`uint64`.
- The Unit 2 envelope is marshalled with the module codec too.

### Unit 4 — Localized correctness fixes

- **wasm-pdk-5 (timer key):** host reads `interval_ms` (align with PDK `system.go:38`). `hostapi.go` set_timer reads `payload["interval_ms"]` via `toInt64`. Keep the `> 0` validation.
- **wasm-pdk-6 (command routing):** add a `target string` (or `export string`) field to `queryRequest`. `Query` sets `"query"`; `SendCommand`'s command-export path sets `"command"`. `processQuery` calls `req.target` instead of inferring from `FunctionExists`. Removes the "both exports present" misroute.
- **wasm-pdk-7 (lifecycleCtx race):** introduce a single `shutdownCtx`/`shutdownCancel` created in `NewModule` and never reassigned; `Stop` cancels it. The temporary "reset lifecycle context for shutdown call" dance in `Stop` (module.go:203-211) is removed — the synchronous shutdown call (Unit 1) uses a fresh `context.WithTimeout(context.Background(), wasmCallTimeout)`. Async host goroutines (`hostapi.go` CallAsync, trigger_workflow) capture the stable `shutdownCtx`. No field is written under a race.
- **wasm-pdk-8 (gateway dup id):** `Gateway.Connect` checks `g.conns[id]` under the write lock before dialing; if present, return `VALIDATION_ERROR: connection id %q already in use` and do not dial. The guest must `ws_close` first to reuse an id. No orphaned readLoop.
- **wasm-pdk-10 (memory cap):** in `loadModuleFromBytes`, when `cfg.MemoryPages <= 0` set `manifest.Memory = &extism.ManifestMemory{MaxPages: defaultMemoryPages}` with `defaultMemoryPages = 256` (16 MiB). Explicit config still overrides. Document the default.

## Testing (TDD, per finding)

All new tests are unit-level with fake `PluginInstance`s; no external services. Each fix starts red:

- **Interruptibility:** a fake whose `CallWithContext` blocks until ctx is done; assert `callWithTimeout` returns a timeout error promptly and the module marks itself failed and stops ticking. A hung-`query` variant asserts `Stop` returns within the shutdown budget (no `<-tickDone` deadlock).
- **No concurrency:** a fake that records overlapping calls (atomic counter, fails the test if >1 concurrent); drive ticks + queries + a timeout and assert the counter never exceeds 1.
- **Error envelope:** host-level test that a `PERMISSION_DENIED` dispatch produces an `ok:false` envelope; PDK-level round-trip test (envelope bytes → `HostError`).
- **Codec:** a module configured `encoding: msgpack` whose host call succeeds end-to-end; numeric-coercion unit tests for int64/uint64/float64/json.Number.
- **Timer:** end-to-end set_timer via the host reads `interval_ms` and the timer appears in a later tick's `Timers`.
- **Command routing:** module exporting both `query` and `command`; assert a command hits `command`.
- **lifecycleCtx:** `-race` test driving concurrent async calls during `Stop`.
- **Gateway dup id:** second `Connect` with a live id returns the validation error and leaves the first connection intact.
- **Memory cap:** unset `MemoryPages` yields a manifest with `MaxPages == 256`.

Recompile the PDK-based example modules under `examples/` and confirm they still load and tick.

## Mechanics

- Worktree `.worktrees/wasm-runtime-hardening`, branch `feat/wasm-runtime-hardening` off `main`.
- Subagent-driven per unit: implementer → spec-compliance reviewer → code-quality reviewer, per working conventions.
- Pre-push gate: `gofmt -l`, `go vet`, `golangci-lint run`, `go test ./internal/wasm/... ./pdk/...` with `-race`.
- Spec + plan force-added to the branch (`git add -f docs/superpowers/...`).
- CHANGELOG entry documenting the PDK ABI break (envelope + `interval_ms` already matched PDK; codec now honored).
- `REVIEW-FINDINGS-2026-07-05.md` gets a "Shipped 2026-07-05" note for the nine items when merged.

## Out of scope

wasm-pdk-9 (heartbeat lost on reconnect), -11 (invalid JSON on quote in error string — largely obviated by the envelope), -12 (PDK reconnect config), -13 (module hash verification) are Low and left for a follow-up unless trivially adjacent during implementation.
