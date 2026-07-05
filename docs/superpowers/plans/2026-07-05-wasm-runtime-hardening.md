# Wasm Runtime Hardening (Tranche A) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fix the 9 `wasm-pdk-*` findings (4 High, 5 Medium) from `REVIEW-FINDINGS-2026-07-05.md` — make guest execution interruptible and single-threaded, surface host-call errors to the PDK, honor the configured codec, and close five localized correctness bugs.

**Architecture:** The tick-loop goroutine (`internal/wasm/tick.go`) becomes the sole caller of the guest. Guest calls run synchronously via a new `PluginInstance.CallWithContext`, made interruptible by setting `extism.Manifest.Timeout`. The host↔guest wire contract changes in a clean break (`{ok,data,error}` envelope + configured codec + `interval_ms` key); in-repo example `.wasm` modules are recompiled.

**Tech Stack:** Go (go1.25), extism/go-sdk v1.7.1, wazero v1.9.0, vmihailenco/msgpack/v5, fasthttp/websocket; TinyGo for the example guest modules.

## Global Constraints

- Go module floor: **go1.25** (do not lower).
- **Clean ABI break** — no backward-compat shim; host and PDK change together; recompile example modules; note in CHANGELOG.
- Default guest memory cap when `MemoryPages` unset: **256 pages (16 MiB)**.
- Duplicate `Gateway.Connect` id: **reject** with `VALIDATION_ERROR`, do not close-old.
- Host-call envelope: success `{"ok":true,"data":<v>}`, error `{"ok":false,"error":{"code":<s>,"message":<s>}}`; `stack[0]==0` means void success only.
- Timer interval key on the wire: **`interval_ms`**.
- All `internal/wasm` + `pdk` tests run under `-race`.
- Pre-push gate: `gofmt -l .`, `go vet ./...`, `golangci-lint run`, `go test -race ./internal/wasm/... ./pdk/...`.
- Only one goroutine may call the guest during `running`; never abandon a guest call while permitting a new one.

**Worktree:** `.worktrees/wasm-runtime-hardening`, branch `feat/wasm-runtime-hardening` off `main`. Spec + this plan are force-added (`git add -f docs/superpowers/...`).

---

## File map

- `internal/wasm/module.go` — `PluginInstance` iface, `Module` struct (`shutdownCtx`, `failed`), `callWithTimeout` signature, `Stop`, `SetTimer`, `queryRequest.target`, `SendCommand`, `Query`.
- `internal/wasm/tick.go` — `tickLoop`, `executeTick`, `processQuery` (routing + failed-module stop).
- `internal/wasm/runtime.go` — `loadModuleFromBytes` (manifest Timeout + Memory default), `buildHostFunctions` (envelope + codec).
- `internal/wasm/hostapi.go` — numeric coercion, `set_timer` key, `classifyError`, async ctx capture.
- `internal/wasm/gateway.go` — `Connect` dup-id reject; `toInt64` at code/heartbeat sites.
- `internal/wasm/coerce.go` *(new)* — `toInt64`/`toFloat` helpers.
- `pdk/go/noda/host.go`, `pdk/go/noda/noda.go`, `pdk/go/noda/errors.go` *(new)* — envelope decode + `HostError`.
- `internal/wasm/wasm_test.go`, `gateway_test.go` — fakes gain `CallWithContext`; new tests.
- `examples/*/wasm/*` — recompiled. `CHANGELOG.md`, `docs/_internal/wasm-host-api.md`, `REVIEW-FINDINGS-2026-07-05.md`.

---

### Task 1: Interruptible synchronous guest calls (wasm-pdk-1 core)

**Files:**
- Modify: `internal/wasm/module.go` (interface `PluginInstance`; `Module.shutdownCtx/shutdownCancel`; `callWithTimeout`)
- Modify: `internal/wasm/runtime.go` (`loadModuleFromBytes` manifest `Timeout`)
- Test: `internal/wasm/wasm_test.go` (extend fakes; new test)

**Interfaces:**
- Produces: `PluginInstance.CallWithContext(ctx context.Context, name string, data []byte) (uint32, []byte, error)`; `(*Module).callWithTimeout(parent context.Context, name string, data []byte, timeout time.Duration) (uint32, []byte, error)`; `Module.shutdownCtx context.Context`.
- Consumes: extism `(*extism.Plugin).CallWithContext` (already exists, v1.7.1).

- [ ] **Step 1: Write the failing test** — a fake whose `CallWithContext` blocks until ctx is done; `callWithTimeout` must return an error promptly (not hang), and must NOT spawn a goroutine that outlives it.

```go
// in wasm_test.go
type blockingPlugin struct {
	mockPlugin
	started chan struct{}
}

func (b *blockingPlugin) CallWithContext(ctx context.Context, name string, data []byte) (uint32, []byte, error) {
	close(b.started)
	<-ctx.Done() // simulate a guest that only stops when the context is cancelled
	return 0, nil, ctx.Err()
}

func TestCallWithTimeout_InterruptibleAndSynchronous(t *testing.T) {
	bp := &blockingPlugin{mockPlugin: *newMockPlugin(), started: make(chan struct{})}
	rt := NewRuntime(registry.NewServiceRegistry(), nil, testLogger())
	m, err := rt.LoadModuleWithPlugin(ModuleConfig{Name: "b", TickTimeout: 20 * time.Millisecond}, bp)
	require.NoError(t, err)

	start := time.Now()
	_, _, callErr := m.callWithTimeout(m.shutdownCtx, "tick", []byte("{}"), 20*time.Millisecond)
	require.Error(t, callErr)
	require.Less(t, time.Since(start), 500*time.Millisecond, "must return at the deadline, not hang")
	<-bp.started // proves the call actually ran inline
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/wasm/ -run TestCallWithTimeout_InterruptibleAndSynchronous -race`
Expected: FAIL — `blockingPlugin` does not satisfy `PluginInstance` (no method set) / `callWithTimeout` signature mismatch (compile error).

- [ ] **Step 3: Add `CallWithContext` to the interface and update fakes**

In `module.go`, change the interface:

```go
// PluginInstance abstracts the Extism plugin for testability.
type PluginInstance interface {
	CallWithContext(ctx context.Context, name string, data []byte) (uint32, []byte, error)
	FunctionExists(name string) bool
	Close(ctx context.Context) error
}
```

In `wasm_test.go`, give `mockPlugin` the method (delegating to its existing recorded-response logic) and drop the old `Call`:

```go
func (m *mockPlugin) CallWithContext(_ context.Context, name string, data []byte) (uint32, []byte, error) {
	m.mu.Lock()
	m.calls = append(m.calls, mockCall{Name: name, Data: data})
	resp, ok := m.responses[name]
	m.mu.Unlock()
	if ok {
		return resp.exitCode, resp.data, resp.err
	}
	return 0, nil, nil
}
```

Update `slowMockPlugin` (wasm_test.go:~3011) the same way (rename `Call` → `CallWithContext`, add the `ctx` param, ignore it or honor it as the test needs).

- [ ] **Step 4: Add `shutdownCtx` and rewrite `callWithTimeout` synchronously**

In `NewModule` (module.go), replace the `lifecycleCtx` pair with a stable shutdown context:

```go
m.shutdownCtx, m.shutdownCancel = context.WithCancel(context.Background())
```

Struct fields (module.go): replace `lifecycleCtx/lifecycleCancel` with:

```go
	shutdownCtx    context.Context
	shutdownCancel context.CancelFunc
```

Rewrite `callWithTimeout`:

```go
// callWithTimeout calls a guest export synchronously with a per-call deadline.
// It runs inline on the caller's goroutine (the tick loop during running),
// so only one goroutine ever touches the plugin. With extism manifest.Timeout
// set, a context deadline actually terminates the guest.
func (m *Module) callWithTimeout(parent context.Context, name string, data []byte, timeout time.Duration) (uint32, []byte, error) {
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()
	return m.Plugin.CallWithContext(ctx, name, data)
}
```

Update the three existing callers to pass a context:
- `Initialize` (module.go:155): `m.callWithTimeout(m.shutdownCtx, "initialize", data, wasmCallTimeout)`
- `executeTick` (tick.go:90): `m.callWithTimeout(m.shutdownCtx, "tick", data, m.Config.TickTimeout)`
- `Stop` shutdown call (module.go:208): use a fresh context — `ctx2, cancel2 := context.WithTimeout(context.Background(), wasmCallTimeout); defer cancel2(); m.callWithTimeout(ctx2, "shutdown", data, wasmCallTimeout)` and delete the lifecycle-reset dance (module.go:203-211).

In `loadModuleFromBytes` (runtime.go), set the manifest timeout so extism enables interruptibility:

```go
	timeoutMs := cfg.TickTimeout
	if timeoutMs < wasmCallTimeout {
		timeoutMs = wasmCallTimeout
	}
	manifest.Timeout = uint64(timeoutMs / time.Millisecond)
```

- [ ] **Step 5: Run the test to verify it passes**

Run: `go test ./internal/wasm/ -run TestCallWithTimeout_InterruptibleAndSynchronous -race`
Expected: PASS.

- [ ] **Step 6: Full wasm package compiles/passes**

Run: `go build ./... && go test ./internal/wasm/ -race`
Expected: PASS (fix any remaining `Call(` → `CallWithContext(` references the compiler flags, e.g. `SendCommand`/`processQuery` still call `m.Plugin.Call` — those are addressed in Tasks 2/7 but must at least compile now; temporarily route them through `m.Plugin.CallWithContext(context.Background(), …)` and leave a `// TODO(task2/7)` — replaced below).

- [ ] **Step 7: Commit**

```bash
git add internal/wasm/module.go internal/wasm/runtime.go internal/wasm/wasm_test.go
git commit -m "fix(wasm): interruptible synchronous guest calls (wasm-pdk-1)"
```

---

### Task 2: No concurrent guest calls; failed-module stop (wasm-pdk-2, wasm-pdk-1 shutdown)

**Files:**
- Modify: `internal/wasm/module.go` (`Module.failed atomic.Bool`; `Query`/`SendCommand` guards)
- Modify: `internal/wasm/tick.go` (`processQuery` via `callWithTimeout`; detect failed module; stop loop)
- Test: `internal/wasm/wasm_test.go`

**Interfaces:**
- Consumes: `callWithTimeout` (Task 1).
- Produces: `Module.failed atomic.Bool`; `(*Module).markFailed(reason string)`.

- [ ] **Step 1: Write the failing test** — concurrency invariant + hung-query no longer deadlocks Stop.

```go
// concurrencyPlugin fails the test if two CallWithContext run at once.
type concurrencyPlugin struct {
	mockPlugin
	inFlight atomic.Int32
	maxSeen  atomic.Int32
	hangQuery bool
}

func (c *concurrencyPlugin) CallWithContext(ctx context.Context, name string, data []byte) (uint32, []byte, error) {
	n := c.inFlight.Add(1)
	if n > c.maxSeen.Load() {
		c.maxSeen.Store(n)
	}
	defer c.inFlight.Add(-1)
	if c.hangQuery && name == "query" {
		<-ctx.Done()
		return 0, nil, ctx.Err()
	}
	time.Sleep(time.Millisecond)
	return 0, nil, nil
}

func TestGuestCalls_NeverConcurrent_AndHungQueryDoesNotDeadlockStop(t *testing.T) {
	cp := &concurrencyPlugin{mockPlugin: *newMockPlugin(), hangQuery: true}
	cp.exports["query"] = true
	rt := NewRuntime(registry.NewServiceRegistry(), nil, testLogger())
	m, _ := rt.LoadModuleWithPlugin(ModuleConfig{Name: "c", TickRate: 60, TickTimeout: 10 * time.Millisecond}, cp)
	m.Start()
	// fire a query that hangs; it must time out, not wedge the loop
	go func() { _, _ = m.Query(context.Background(), map[string]any{}, 20*time.Millisecond) }()
	time.Sleep(50 * time.Millisecond)

	done := make(chan error, 1)
	go func() { done <- m.Stop(context.Background()) }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Stop deadlocked on a hung query")
	}
	require.LessOrEqual(t, cp.maxSeen.Load(), int32(1), "guest calls overlapped")
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/wasm/ -run TestGuestCalls_NeverConcurrent -race`
Expected: FAIL — `processQuery` calls `m.Plugin.Call`/untimed path (from Task 1 TODO), so the hung query blocks the tick loop and `Stop` deadlocks on `<-m.tickDone`.

- [ ] **Step 3: Route `processQuery` through `callWithTimeout`; add failed-module handling**

Add to `Module` (module.go): `failed atomic.Bool` and:

```go
func (m *Module) markFailed(reason string) {
	if m.failed.CompareAndSwap(false, true) {
		m.Logger.Error("wasm module failed; stopping tick loop", "module", m.Name, "reason", reason)
	}
}
```

Rewrite `processQuery` (tick.go) to use the timed call and honor `req.target` placeholder (`"query"` for now; Task 7 sets it):

```go
func (m *Module) processQuery(req queryRequest) {
	exitCode, output, err := m.callWithTimeout(m.shutdownCtx, req.target, req.data, wasmCallTimeout)
	if err != nil {
		m.markFailed("query call: " + err.Error())
		req.result <- queryResponse{err: fmt.Errorf("%s call failed: %w", req.target, err)}
		return
	}
	if exitCode != 0 {
		req.result <- queryResponse{err: fmt.Errorf("%s returned exit code %d", req.target, exitCode)}
		return
	}
	req.result <- queryResponse{data: output}
}
```

(Add `target` to `queryRequest` now with default `"query"`; full routing is Task 7.)

In `executeTick` (tick.go), after the tick call errors, mark failed and let the loop exit:

```go
	if err != nil {
		m.markFailed("tick call: " + err.Error())
		return
	}
```

In `tickLoop` (tick.go), exit when failed:

```go
	for {
		if m.failed.Load() {
			return
		}
		select {
		case <-m.stopCh:
			return
		case <-tickerC:
			m.executeTick()
		case req := <-m.queryCh:
			m.processQuery(req)
		}
	}
```

Guard `Query`/`SendCommand` (module.go) to fail fast on a dead module:

```go
	if m.failed.Load() {
		return nil, fmt.Errorf("module %q has failed", m.Name)
	}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./internal/wasm/ -run TestGuestCalls_NeverConcurrent -race`
Expected: PASS.

- [ ] **Step 5: Full package + race**

Run: `go test ./internal/wasm/ -race`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/wasm/module.go internal/wasm/tick.go internal/wasm/wasm_test.go
git commit -m "fix(wasm): serialize guest calls, stop failed module (wasm-pdk-2)"
```

---

### Task 3: Host-call error envelope — host side (wasm-pdk-3 host)

**Files:**
- Modify: `internal/wasm/runtime.go` (`buildHostFunctions` `noda_call`)
- Modify: `internal/wasm/hostapi.go` (`classifyError` helper)
- Test: `internal/wasm/wasm_test.go`

**Interfaces:**
- Produces: wire envelope `{"ok":bool,"data":any,"error":{"code":string,"message":string}}`; `classifyError(err error) string`.

- [ ] **Step 1: Write the failing test** — a denied host call must yield an `ok:false` envelope, not raw data.

```go
func TestHostCall_ErrorEnvelope(t *testing.T) {
	code := classifyError(fmt.Errorf("PERMISSION_DENIED: service \"x\" not allowed"))
	require.Equal(t, "PERMISSION_DENIED", code)
	require.Equal(t, "INTERNAL_ERROR", classifyError(fmt.Errorf("boom")))
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/wasm/ -run TestHostCall_ErrorEnvelope`
Expected: FAIL — `classifyError` undefined.

- [ ] **Step 3: Add `classifyError` and wrap `noda_call` returns in the envelope**

In `hostapi.go`:

```go
// classifyError maps a dispatcher error to a wire error code based on its prefix.
func classifyError(err error) string {
	msg := err.Error()
	for _, code := range []string{"PERMISSION_DENIED", "VALIDATION_ERROR", "SERVICE_UNAVAILABLE", "NOT_FOUND"} {
		if strings.HasPrefix(msg, code) {
			return code
		}
	}
	return "INTERNAL_ERROR"
}
```

In `runtime.go` `noda_call`, replace the body so every path emits the envelope (codec is still `jsonCodec` here; Task 5 swaps it):

```go
			codec := &jsonCodec{}
			writeEnvelope := func(env map[string]any) {
				out, mErr := codec.Marshal(env)
				if mErr != nil { stack[0] = 0; return }
				off, wErr := p.WriteBytes(out)
				if wErr != nil { stack[0] = 0; return }
				stack[0] = off
			}
			input, err := p.ReadBytes(stack[0])
			if err != nil {
				writeEnvelope(map[string]any{"ok": false, "error": map[string]any{"code": "INTERNAL_ERROR", "message": "read input: " + err.Error()}})
				return
			}
			var req HostCallRequest
			if err := codec.Unmarshal(input, &req); err != nil {
				writeEnvelope(map[string]any{"ok": false, "error": map[string]any{"code": "VALIDATION_ERROR", "message": "invalid request: " + err.Error()}})
				return
			}
			result, err := dispatcher.Call(ctx, req)
			if err != nil {
				writeEnvelope(map[string]any{"ok": false, "error": map[string]any{"code": classifyError(err), "message": err.Error()}})
				return
			}
			if result == nil {
				stack[0] = 0 // void success
				return
			}
			writeEnvelope(map[string]any{"ok": true, "data": result})
```

Add `"strings"` to `hostapi.go` imports.

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./internal/wasm/ -run TestHostCall_ErrorEnvelope`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/wasm/runtime.go internal/wasm/hostapi.go internal/wasm/wasm_test.go
git commit -m "fix(wasm): host-call error envelope, host side (wasm-pdk-3)"
```

---

### Task 4: Host-call error envelope — PDK side (wasm-pdk-3 pdk)

**Files:**
- Create: `pdk/go/noda/errors.go`
- Modify: `pdk/go/noda/host.go`, `pdk/go/noda/noda.go`
- Test: `pdk/go/noda/noda_test.go` *(create if absent)*

**Interfaces:**
- Produces: `type HostError struct { Code, Message string }` with `Error() string`; `call([]byte) ([]byte, error)` now decodes the envelope.
- Consumes: envelope from Task 3.

- [ ] **Step 1: Write the failing test**

```go
// pdk/go/noda/noda_test.go
func TestDecodeEnvelope_Error(t *testing.T) {
	data := []byte(`{"ok":false,"error":{"code":"PERMISSION_DENIED","message":"nope"}}`)
	_, err := decodeEnvelope(data)
	var he *HostError
	if !errors.As(err, &he) || he.Code != "PERMISSION_DENIED" {
		t.Fatalf("want HostError PERMISSION_DENIED, got %v", err)
	}
}

func TestDecodeEnvelope_Success(t *testing.T) {
	data, err := decodeEnvelope([]byte(`{"ok":true,"data":{"value":1}}`))
	if err != nil { t.Fatal(err) }
	if data == nil { t.Fatal("want data bytes") }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./pdk/go/noda/ -run TestDecodeEnvelope`
Expected: FAIL — `decodeEnvelope`/`HostError` undefined.

- [ ] **Step 3: Implement `HostError` and envelope decode**

`pdk/go/noda/errors.go`:

```go
package noda

import "fmt"

// HostError is a structured error returned by a Noda host call.
type HostError struct {
	Code    string
	Message string
}

func (e *HostError) Error() string { return fmt.Sprintf("%s: %s", e.Code, e.Message) }
```

In `host.go`, add `decodeEnvelope` and use it in `call`:

```go
type envelope struct {
	OK    bool            `json:"ok" msgpack:"ok"`
	Data  json.RawMessage `json:"data,omitempty" msgpack:"data,omitempty"`
	Error *struct {
		Code    string `json:"code" msgpack:"code"`
		Message string `json:"message" msgpack:"message"`
	} `json:"error,omitempty" msgpack:"error,omitempty"`
}

// decodeEnvelope parses a host response envelope; returns the raw data bytes on success.
func decodeEnvelope(raw []byte) ([]byte, error) {
	var env envelope
	if err := activeCodec.Unmarshal(raw, &env); err != nil {
		return nil, err
	}
	if !env.OK {
		if env.Error != nil {
			return nil, &HostError{Code: env.Error.Code, Message: env.Error.Message}
		}
		return nil, &HostError{Code: "INTERNAL_ERROR", Message: "host call failed"}
	}
	if len(env.Data) == 0 {
		return nil, nil
	}
	return env.Data, nil
}
```

Update `call` (host.go):

```go
func call(data []byte) ([]byte, error) {
	mem := pdk.AllocateBytes(data)
	defer mem.Free()
	resultOffset := hostCall(mem.Offset())
	if resultOffset == 0 {
		return nil, nil // void success
	}
	rmem := pdk.FindMemory(resultOffset)
	return decodeEnvelope(rmem.ReadBytes())
}
```

Note: `json.RawMessage` works only for the JSON codec; for msgpack the PDK must decode `Data` via `activeCodec`. Simplify by making `Data` a `msgpack.RawMessage`-agnostic `[]byte` is not portable, so decode in two steps: unmarshal envelope into `map[string]any` when codec is msgpack. **Implement `decodeEnvelope` codec-agnostically** by unmarshalling into a struct whose `Data` is `json.RawMessage` for JSON and re-marshalling for msgpack — since `CallInto` re-unmarshals `data` anyway, return the *re-marshalled* `data` using `activeCodec`:

```go
type envelopeAny struct {
	OK    bool `json:"ok" msgpack:"ok"`
	Data  any  `json:"data,omitempty" msgpack:"data,omitempty"`
	Error *struct{ Code, Message string } `json:"error,omitempty" msgpack:"error,omitempty"`
}

func decodeEnvelope(raw []byte) ([]byte, error) {
	var env envelopeAny
	if err := activeCodec.Unmarshal(raw, &env); err != nil {
		return nil, err
	}
	if !env.OK {
		if env.Error != nil {
			return nil, &HostError{Code: env.Error.Code, Message: env.Error.Message}
		}
		return nil, &HostError{Code: "INTERNAL_ERROR", Message: "host call failed"}
	}
	if env.Data == nil {
		return nil, nil
	}
	return activeCodec.Marshal(env.Data)
}
```

Use the single `envelopeAny` version (delete the `json.RawMessage` variant). Add imports as needed.

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./pdk/go/noda/ -run TestDecodeEnvelope`
Expected: PASS.

- [ ] **Step 5: Remove the dead `pdk.GetError()` contract mention**

Delete the "PDK reads this via pdk.GetError()" comment already removed on the host; ensure no PDK code references `GetError`. (`grep -rn GetError pdk/` → no hits.)

- [ ] **Step 6: Commit**

```bash
git add pdk/go/noda/errors.go pdk/go/noda/host.go pdk/go/noda/noda.go pdk/go/noda/noda_test.go
git commit -m "fix(wasm/pdk): decode host-call error envelope into HostError (wasm-pdk-3)"
```

---

### Task 5: Honor configured codec + numeric coercion (wasm-pdk-4)

**Files:**
- Create: `internal/wasm/coerce.go`
- Modify: `internal/wasm/runtime.go` (both host fns use `dispatcher.module.Codec`)
- Modify: `internal/wasm/hostapi.go`, `internal/wasm/gateway.go` (coercion at number sites)
- Test: `internal/wasm/wasm_test.go`

**Interfaces:**
- Produces: `toInt64(v any) (int64, bool)`, `toFloat(v any) (float64, bool)`.

- [ ] **Step 1: Write the failing test**

```go
func TestToInt64_Coercion(t *testing.T) {
	for _, v := range []any{float64(42), int64(42), uint64(42), int(42), json.Number("42")} {
		got, ok := toInt64(v)
		require.True(t, ok)
		require.Equal(t, int64(42), got)
	}
	_, ok := toInt64("nope")
	require.False(t, ok)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/wasm/ -run TestToInt64_Coercion`
Expected: FAIL — `toInt64` undefined.

- [ ] **Step 3: Implement `coerce.go`**

```go
package wasm

import "encoding/json"

func toInt64(v any) (int64, bool) {
	switch n := v.(type) {
	case float64:
		return int64(n), true
	case float32:
		return int64(n), true
	case int64:
		return n, true
	case int:
		return int64(n), true
	case uint64:
		return int64(n), true
	case json.Number:
		i, err := n.Int64()
		return i, err == nil
	default:
		return 0, false
	}
}

func toFloat(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int64:
		return float64(n), true
	case int:
		return float64(n), true
	case uint64:
		return float64(n), true
	case json.Number:
		f, err := n.Float64()
		return f, err == nil
	default:
		return 0, false
	}
}
```

- [ ] **Step 4: Swap host-function codec and apply coercion**

In `runtime.go`, both `noda_call` and `noda_call_async`: replace `codec := &jsonCodec{}` with `codec := dispatcher.module.Codec` (the `writeEnvelope` closure from Task 3 then marshals with the module codec).

In `hostapi.go` cache `ttl` (line ~263):

```go
			ttl := 0
			if v, ok := toInt64(payload["ttl"]); ok {
				ttl = int(v)
			}
```

In `gateway.go` `CloseConn` code (line ~167) and `Configure` heartbeat: use `toInt64`/`toFloat` instead of the `.(float64)` assertions.

- [ ] **Step 5: Run tests + a msgpack round-trip**

Add a msgpack module host-call test asserting a `cache.set` with a msgpack-encoded `ttl` succeeds. Run:
`go test ./internal/wasm/ -run 'TestToInt64_Coercion|TestHostCall_Msgpack' -race`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/wasm/coerce.go internal/wasm/runtime.go internal/wasm/hostapi.go internal/wasm/gateway.go internal/wasm/wasm_test.go
git commit -m "fix(wasm): honor configured codec in host calls + numeric coercion (wasm-pdk-4)"
```

---

### Task 6: Timer key alignment (wasm-pdk-5)

**Files:**
- Modify: `internal/wasm/hostapi.go` (`set_timer` reads `interval_ms`)
- Test: `internal/wasm/wasm_test.go`

- [ ] **Step 1: Write the failing test**

```go
func TestSetTimer_ReadsIntervalMs(t *testing.T) {
	rt := NewRuntime(registry.NewServiceRegistry(), nil, testLogger())
	m, _ := rt.LoadModuleWithPlugin(ModuleConfig{Name: "t"}, newMockPlugin())
	d := &HostDispatcher{module: m, logger: testLogger()}
	_, err := d.handleSystemOp(context.Background(), HostCallRequest{
		Operation: "set_timer",
		Payload:   map[string]any{"name": "beat", "interval_ms": float64(100)},
	})
	require.NoError(t, err)
	m.mu.Lock()
	_, exists := m.timers["beat"]
	m.mu.Unlock()
	require.True(t, exists, "timer should be registered from interval_ms")
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/wasm/ -run TestSetTimer_ReadsIntervalMs`
Expected: FAIL — host reads `payload["interval"]`, timer not registered.

- [ ] **Step 3: Read `interval_ms` via `toInt64`**

In `hostapi.go` `set_timer` (lines ~186-193):

```go
		intervalMs, ok := toInt64(payload["interval_ms"])
		if !ok || intervalMs <= 0 {
			return nil, fmt.Errorf("VALIDATION_ERROR: interval_ms must be a positive number")
		}
		d.module.SetTimer(name, intervalMs)
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./internal/wasm/ -run TestSetTimer_ReadsIntervalMs`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/wasm/hostapi.go internal/wasm/wasm_test.go
git commit -m "fix(wasm): set_timer reads interval_ms to match PDK (wasm-pdk-5)"
```

---

### Task 7: Command/query routing (wasm-pdk-6)

**Files:**
- Modify: `internal/wasm/module.go` (`queryRequest.target`; `Query`, `SendCommand` set it)
- Modify: `internal/wasm/tick.go` (`processQuery` uses `req.target` — already done in Task 2)
- Test: `internal/wasm/wasm_test.go`

**Interfaces:**
- Produces: `queryRequest{ data []byte; target string; result chan queryResponse }`.

- [ ] **Step 1: Write the failing test** — a module exporting both `query` and `command`; a command must hit `command`.

```go
func TestSendCommand_RoutesToCommandExport(t *testing.T) {
	mp := newMockPlugin()
	mp.exports["query"] = true
	mp.exports["command"] = true
	rt := NewRuntime(registry.NewServiceRegistry(), nil, testLogger())
	m, _ := rt.LoadModuleWithPlugin(ModuleConfig{Name: "cmd", TickRate: 60}, mp)
	m.Start()
	defer m.Stop(context.Background())
	m.SendCommand(map[string]any{"hello": "world"})
	require.Eventually(t, func() bool { return len(mp.getCalls("command")) > 0 }, time.Second, 5*time.Millisecond)
	require.Empty(t, mp.getCalls("query"), "command must not be routed to query")
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/wasm/ -run TestSendCommand_RoutesToCommandExport -race`
Expected: FAIL — `processQuery` calls `"query"` because both exports exist.

- [ ] **Step 3: Add `target` and set it at both call sites**

`queryRequest` (module.go):

```go
type queryRequest struct {
	data   []byte
	target string // "query" or "command"
	result chan queryResponse
}
```

`Query` (module.go:251): `req := queryRequest{data: data, target: "query", result: make(chan queryResponse, 1)}`.

`SendCommand` command path (module.go:298): `req := queryRequest{data: cmdData, target: "command", result: make(chan queryResponse, 1)}`.

(`processQuery` already calls `m.callWithTimeout(m.shutdownCtx, req.target, …)` from Task 2. Remove the old `FunctionExists`-based `funcName` inference.)

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./internal/wasm/ -run TestSendCommand_RoutesToCommandExport -race`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/wasm/module.go internal/wasm/tick.go internal/wasm/wasm_test.go
git commit -m "fix(wasm): route commands to command export explicitly (wasm-pdk-6)"
```

---

### Task 8: Stable shutdown context (wasm-pdk-7)

**Files:**
- Modify: `internal/wasm/module.go` (remove `lifecycleCtx` reassignment — done in Task 1), `internal/wasm/hostapi.go` (async goroutines capture `shutdownCtx`)
- Test: `internal/wasm/wasm_test.go`

- [ ] **Step 1: Write the failing test** — `-race`, concurrent async calls during `Stop` must not race on the context field.

```go
func TestNoRaceOnShutdownCtx(t *testing.T) {
	mp := newMockPlugin()
	rt := NewRuntime(registry.NewServiceRegistry(), nil, testLogger())
	m, _ := rt.LoadModuleWithPlugin(ModuleConfig{Name: "r"}, mp)
	d := m.dispatcher
	m.Start()
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_ = d.CallAsync(context.Background(), HostCallRequest{Service: "", Operation: "log", Label: fmt.Sprintf("l%d", i), Payload: map[string]any{"message": "x"}})
		}(i)
	}
	_ = m.Stop(context.Background())
	wg.Wait()
}
```

- [ ] **Step 2: Run test to verify it fails (or races)**

Run: `go test ./internal/wasm/ -run TestNoRaceOnShutdownCtx -race`
Expected: FAIL/RACE if any `m.lifecycleCtx` reassignment remains.

- [ ] **Step 3: Ensure all async goroutines read `shutdownCtx`**

In `hostapi.go`: `CallAsync` goroutine (line ~108) and `trigger_workflow` goroutine (line ~176) capture `d.module.shutdownCtx` (renamed from `lifecycleCtx`). Confirm no reassignment of `shutdownCtx` exists anywhere (Task 1 removed the Stop reset). `grep -n lifecycleCtx internal/wasm/*.go` → no hits.

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./internal/wasm/ -run TestNoRaceOnShutdownCtx -race`
Expected: PASS (no race).

- [ ] **Step 5: Commit**

```bash
git add internal/wasm/module.go internal/wasm/hostapi.go internal/wasm/wasm_test.go
git commit -m "fix(wasm): stable shutdown context, no reassignment race (wasm-pdk-7)"
```

---

### Task 9: Gateway duplicate-id rejection (wasm-pdk-8)

**Files:**
- Modify: `internal/wasm/gateway.go` (`Connect`)
- Test: `internal/wasm/gateway_test.go`

- [ ] **Step 1: Write the failing test**

```go
func TestGatewayConnect_RejectsDuplicateID(t *testing.T) {
	g := NewGateway(&Module{Name: "m", Codec: &jsonCodec{}}, testLogger())
	g.conns["c1"] = &gatewayConn{id: "c1", stopCh: make(chan struct{})} // simulate a live conn
	_, err := g.Connect(context.Background(), map[string]any{"id": "c1", "url": "ws://example/x"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "already in use")
	require.NotNil(t, g.conns["c1"], "existing connection must be left intact")
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/wasm/ -run TestGatewayConnect_RejectsDuplicateID`
Expected: FAIL — `Connect` currently dials and overwrites `g.conns["c1"]`.

- [ ] **Step 3: Reject a duplicate id before dialing**

In `Connect` (gateway.go), after resolving `id`/`wsURL` and the whitelist check, before dialing:

```go
	g.mu.Lock()
	if _, exists := g.conns[id]; exists {
		g.mu.Unlock()
		return nil, fmt.Errorf("VALIDATION_ERROR: connection id %q already in use", id)
	}
	g.mu.Unlock()
```

(Keep the existing lock/insert at line 114-116 as-is for the actual insert after a successful dial.)

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./internal/wasm/ -run TestGatewayConnect_RejectsDuplicateID`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/wasm/gateway.go internal/wasm/gateway_test.go
git commit -m "fix(wasm): reject duplicate gateway connection id (wasm-pdk-8)"
```

---

### Task 10: Default memory cap (wasm-pdk-10)

**Files:**
- Modify: `internal/wasm/runtime.go` (`loadModuleFromBytes`; `defaultMemoryPages` const)
- Test: `internal/wasm/wasm_test.go`

- [ ] **Step 1: Write the failing test** — assert the manifest carries a bounded default when `MemoryPages` is unset. Extract a small helper `buildManifest(cfg, wasmBytes) extism.Manifest` so it's unit-testable.

```go
func TestBuildManifest_DefaultMemoryCap(t *testing.T) {
	man := buildManifest(ModuleConfig{}, []byte{0x00})
	require.NotNil(t, man.Memory)
	require.Equal(t, uint32(defaultMemoryPages), man.Memory.MaxPages)

	man2 := buildManifest(ModuleConfig{MemoryPages: 512}, []byte{0x00})
	require.Equal(t, uint32(512), man2.Memory.MaxPages)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/wasm/ -run TestBuildManifest_DefaultMemoryCap`
Expected: FAIL — `buildManifest`/`defaultMemoryPages` undefined.

- [ ] **Step 3: Extract `buildManifest` with a default cap**

In `runtime.go`:

```go
// defaultMemoryPages caps guest linear memory when MemoryPages is unset.
// 256 pages * 64 KiB = 16 MiB (wazero's default is unbounded up to 4 GiB).
const defaultMemoryPages uint32 = 256

func buildManifest(cfg ModuleConfig, wasmBytes []byte) extism.Manifest {
	manifest := extism.Manifest{
		Wasm:         []extism.Wasm{extism.WasmData{Data: wasmBytes}},
		AllowedHosts: cfg.AllowHTTP,
	}
	pages := cfg.MemoryPages
	if pages == 0 {
		pages = defaultMemoryPages
	}
	manifest.Memory = &extism.ManifestMemory{MaxPages: pages}

	timeoutMs := cfg.TickTimeout
	if timeoutMs < wasmCallTimeout {
		timeoutMs = wasmCallTimeout
	}
	manifest.Timeout = uint64(timeoutMs / time.Millisecond)
	return manifest
}
```

Replace the inline manifest construction in `loadModuleFromBytes` with `manifest := buildManifest(cfg, wasmBytes)` (folds in the Task 1 `Timeout` too). Add `"time"` import if needed.

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./internal/wasm/ -run TestBuildManifest_DefaultMemoryCap`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/wasm/runtime.go internal/wasm/wasm_test.go
git commit -m "fix(wasm): default 16 MiB guest memory cap (wasm-pdk-10)"
```

---

### Task 11: Recompile example modules, docs, CHANGELOG, findings note

**Files:**
- Modify: `examples/wasm-counter/wasm/*`, `examples/wasm-helpers/wasm/*`, `examples/discord-bot/wasm/*` (rebuild `.wasm`)
- Modify: `docs/_internal/wasm-host-api.md` (§4.3 error mechanism; timer key), `CHANGELOG.md`, `REVIEW-FINDINGS-2026-07-05.md`

- [ ] **Step 1: Rebuild the example guest modules against the updated PDK**

Run (per example dir with a build script/Makefile target; use the existing TinyGo invocation):
`cd examples/wasm-counter/wasm && tinygo build -o counter.wasm -target wasi .` (repeat for helpers, bot — match each dir's existing build command).
Expected: modules compile against the new PDK envelope/error API.

- [ ] **Step 2: Load-and-tick smoke check**

Run the example's own test if present, else a minimal load: `go test ./internal/wasm/ -run TestExampleModulesLoad` (add a small test that `LoadModule`s each example `.wasm` and ticks once).
Expected: PASS — modules load with the new manifest (memory cap + timeout) and the host envelope.

- [ ] **Step 3: Update docs**

In `docs/_internal/wasm-host-api.md`: replace the §4.3 "pdk.GetError()" error paragraph with the envelope contract (`{ok,data,error}`, `HostError`); change the timer field to `interval_ms`; document the 16 MiB default memory cap and that a timed-out guest call terminates and fails the module.

- [ ] **Step 4: CHANGELOG + findings note**

Add a `CHANGELOG.md` entry: "Wasm runtime hardening (tranche A) — **BREAKING (guest ABI):** host calls now return a `{ok,data,error}` envelope decoded by the PDK into `HostError`; rebuild guest modules against the updated PDK. Guest execution is now interruptible; default 16 MiB memory cap." Append a "Shipped 2026-07-05" note under the nine items in `REVIEW-FINDINGS-2026-07-05.md`.

- [ ] **Step 5: Full gate**

Run: `gofmt -l . && go vet ./... && golangci-lint run && go test -race ./internal/wasm/... ./pdk/...`
Expected: clean, all pass.

- [ ] **Step 6: Commit**

```bash
git add examples docs/_internal/wasm-host-api.md CHANGELOG.md REVIEW-FINDINGS-2026-07-05.md internal/wasm/wasm_test.go
git commit -m "docs(wasm): recompile examples, document ABI break, mark findings shipped"
```

---

## Self-review notes

- **Spec coverage:** wasm-pdk-1 → Tasks 1+2; -2 → Task 2; -3 → Tasks 3+4; -4 → Task 5; -5 → Task 6; -6 → Task 7; -7 → Tasks 1+8; -8 → Task 9; -10 → Task 10; ABI/docs/examples → Task 11. All nine covered.
- **Type consistency:** `queryRequest.target` introduced in Task 2 (default `"query"`), set in Task 7 — consistent. `callWithTimeout(parent, name, data, timeout)` used identically in Tasks 1/2. `shutdownCtx` naming consistent across Tasks 1/2/8. `toInt64`/`toFloat` defined in Task 5, reused in Task 6. Envelope shape identical host (Task 3) and PDK (Task 4).
- **Ordering constraint:** Task 1 introduces a temporary `CallWithContext(context.Background(), …)` shim in `processQuery`/`SendCommand` that Tasks 2/7 replace; noted inline so no dangling TODO ships (Task 7 removes the last inference).
- **Deferred (out of scope, per spec):** wasm-pdk-9/11/12/13.
