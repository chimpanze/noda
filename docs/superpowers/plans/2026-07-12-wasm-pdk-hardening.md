# Wasm/PDK Hardening Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Close the seven wasm/pdk follow-ups: Query fails fast during shutdown (#293), the outstandingCalls Add/Wait invariant becomes structural via `addMu`/`tryAddOutstanding` (#295), the PDK exports `DecodeInto` (#294), example guest modules build in CI (#296), the gateway checks the whitelist before duplicate ids (#265), #267 is resolved by upstream-check-else-document, and two test-polish items land (#268).

**Architecture:** All runtime changes live in `internal/wasm` (module.go, hostapi.go, gateway.go, runtime.go comment); the PDK change in `pdk/go/noda/codec.go` plus both example guests; CI in `.github/workflows/ci.yml` as a NEW non-blocking job. The `addMu` design: a leaf mutex guarding only the stopping-check+Add pair — acquired while holding `m.mu` is allowed (SendCommand), the reverse never happens, so no deadlock.

**Tech Stack:** Go, extism/go-sdk v1.7.1 (wazero), tinygo 0.40.1 (`-target wasi -buildmode=c-shared` — required on tinygo ≥ 0.40 so exports are callable without `_start`, per `examples/wasm-helpers/README.md:23`), GitHub Actions.

**Spec:** `docs/superpowers/specs/2026-07-12-wasm-pdk-hardening-design.md`

## Global Constraints

- Lock ordering rule (document it on the new field): `addMu` is a LEAF — it may be acquired while holding `m.mu` (SendCommand does), never held across guest calls, channel ops, or `m.mu` acquisition.
- `tryAddOutstanding()` becomes the ONLY way to Add to `outstandingCalls`; grep proof required (`grep -n "outstandingCalls.Add" internal/wasm/` must show only the helper).
- Stop's existing semantics must hold: `stopping` still observable under `m.mu` for `AddAsyncResult`/late-write dropping (keep the store inside the current `m.mu` block, additionally wrapped in `addMu` — see Task 2).
- Stopping error text: `module %q stopping` (Query and any stopping-path errors use it consistently).
- The race test must FAIL against unguarded code (polarity): removing the guard reintroduces Add-vs-Wait misuse, which `-race`/WaitGroup panics catch.
- Gateway error prefixes are contract: `PERMISSION_DENIED: host not in allow_outbound.ws whitelist` and `VALIDATION_ERROR: connection id %q already in use` — texts unchanged, only their precedence flips.
- CI job is NON-BLOCKING: a new separate job NOT added to any required-checks config; it must still fail red on breakage.
- `gofmt -l .` after Task 5 must print NOTHING (the wasm-helpers drive-by removes the repo's one standing hit).
- extism stays at v1.7.1 unless Task 4's upstream check finds a host-side SetError equivalent (then bump per spec decision 2).
- Conventional commits with trailer `Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>`; per-task gate: `gofmt -l internal/ pdk/ examples/` (nothing new), `go vet ./...`, task tests green.

## Worktree setup (before Task 1)

```bash
cd /Users/marten/GolandProjects/noda
git fetch origin main   # local main goes stale — branch from origin/main explicitly
git worktree add .worktrees/wasm-pdk-hardening -b feat/wasm-pdk-hardening origin/main
cd .worktrees/wasm-pdk-hardening
mkdir -p docs/superpowers/specs docs/superpowers/plans
cp ../../docs/superpowers/specs/2026-07-12-wasm-pdk-hardening-design.md docs/superpowers/specs/
cp ../../docs/superpowers/plans/2026-07-12-wasm-pdk-hardening.md docs/superpowers/plans/
git add -f docs/superpowers/specs/2026-07-12-wasm-pdk-hardening-design.md docs/superpowers/plans/2026-07-12-wasm-pdk-hardening.md
git commit -m "docs: spec + plan for wasm/pdk hardening tranche (#293-#296, #265, #267, #268)"
```

---

### Task 1: Query stopping guard (#293) + failed-flag assertion (#268.1)

**Files:**
- Modify: `internal/wasm/module.go:264-304` (Query)
- Test: `internal/wasm/wasm_test.go` (new test near the other Module tests; extend `TestModule_Tick_HangingTickKilledByTimeout` at :3233)

**Interfaces:**
- Consumes: existing test helpers `newMockPlugin()`, `newSlowMockPlugin()`, `NewHostDispatcher`, `testLogger()`, `ModuleConfig` (all visible in wasm_test.go).
- Produces: stopping-error text `module %q stopping` that Task 2's SendCommand path may share.

- [ ] **Step 1: Write the failing tests**

Add to `internal/wasm/wasm_test.go` (mirror the setup of `TestModule_Tick_HangingTickKilledByTimeout`):

```go
// #293: a Query racing or arriving after Stop must fail fast with a
// stopping error, not burn its full timeout against a drained queryCh.
func TestModule_Query_FailsFastAfterStop(t *testing.T) {
	plugin := newMockPlugin()
	svcReg := registry.NewServiceRegistry()
	dispatcher := NewHostDispatcher(svcReg, nil, testLogger())
	m, err := NewModule("test", plugin, ModuleConfig{Name: "test", TickRate: 10}, dispatcher, testLogger())
	require.NoError(t, err)
	require.NoError(t, m.Initialize(context.Background()))
	m.Start()
	require.NoError(t, m.Stop(context.Background()))

	start := time.Now()
	_, err = m.Query(context.Background(), map[string]any{"probe": true}, 30*time.Second)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "stopping")
	assert.Less(t, time.Since(start), time.Second, "must fail fast, not burn the 30s timeout")
}
```

And in `TestModule_Tick_HangingTickKilledByTimeout`, after the existing `calls := plugin.getCalls("tick")` block, add:

```go
	// #268: a tick killed by timeout/interrupt is fatal — the narrowed
	// markFailed behavior must leave the module marked failed.
	assert.True(t, m.failed.Load(), "hung-tick interrupt must mark the module failed")
```

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/wasm/ -run 'TestModule_Query_FailsFastAfterStop' -v -timeout 60s`
Expected: FAIL — the query burns toward its timeout (the `Less` assertion trips after the queryCh enqueue blocks or the await select hangs).
Run: `go test ./internal/wasm/ -run 'TestModule_Tick_HangingTickKilledByTimeout' -v`
Expected: PASS or FAIL — if the new failed-flag assertion FAILS, stop and report (that would mean "interrupt is fatal" isn't actually implemented — a real finding, not a test bug).

- [ ] **Step 3: Implement the guard**

In `Query` (module.go:264): after the existing `m.failed` check add

```go
	if m.stopping.Load() {
		return nil, fmt.Errorf("module %q stopping", m.Name)
	}
```

Add `case <-m.stopCh:` returning the same error to BOTH selects:

```go
	select {
	case m.queryCh <- req:
	case <-m.stopCh:
		return nil, fmt.Errorf("module %q stopping", m.Name)
	case <-ctx.Done():
		return nil, ctx.Err()
	}
```

and in the await select (alongside `resp := <-req.result`, `timer.C`, `ctx.Done()`):

```go
	case <-m.stopCh:
		return nil, fmt.Errorf("module %q stopping", m.Name)
```

- [ ] **Step 4: Run to verify pass**

Run: `go test ./internal/wasm/ -run 'TestModule_Query|TestModule_Tick_Hanging' -v -timeout 60s`
Expected: PASS. Then the package: `go test ./internal/wasm/` → PASS.

- [ ] **Step 5: Gate and commit**

```bash
gofmt -l internal/ && go vet ./internal/wasm/
git add internal/wasm/module.go internal/wasm/wasm_test.go
git commit -m "fix(wasm): Query fails fast during shutdown; pin failed-flag on hung-tick kill (#293, #268)"
```

---

### Task 2: addMu / tryAddOutstanding encapsulation (#295)

**Files:**
- Modify: `internal/wasm/module.go` (field ~:67, Stop ~:196-215, SendCommand ~:307-335)
- Modify: `internal/wasm/hostapi.go:114-120, 183-190` (both Add sites)
- Test: `internal/wasm/wasm_test.go` (new race-style test)

**Interfaces:**
- Consumes: stopping-error text from Task 1.
- Produces: `func (m *Module) tryAddOutstanding() bool` — the only permitted Add path (grep-enforced).

- [ ] **Step 1: Write the failing/racing test**

```go
// #295: tryAddOutstanding is the only Add path; Stop's stopping store under
// the same addMu guarantees no Add races Wait-at-zero. Run under -race —
// with the guard removed (raw Add), this hits WaitGroup Add-vs-Wait misuse.
func TestModule_TryAddOutstanding_NoAddAfterStop(t *testing.T) {
	plugin := newMockPlugin()
	svcReg := registry.NewServiceRegistry()
	dispatcher := NewHostDispatcher(svcReg, nil, testLogger())
	m, err := NewModule("test", plugin, ModuleConfig{Name: "test", TickRate: 10}, dispatcher, testLogger())
	require.NoError(t, err)
	require.NoError(t, m.Initialize(context.Background()))
	m.Start()

	stop := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				if m.tryAddOutstanding() {
					time.Sleep(50 * time.Microsecond) // hold the counter briefly
					m.outstandingCalls.Done()
				}
			}
		}()
	}
	time.Sleep(10 * time.Millisecond) // let the hammer run
	require.NoError(t, m.Stop(context.Background()))
	assert.False(t, m.tryAddOutstanding(), "no Add may succeed after Stop")
	close(stop)
	wg.Wait()
}
```

Run: `go test ./internal/wasm/ -run TestModule_TryAddOutstanding -v` → FAIL with `undefined: tryAddOutstanding` (compile RED).

- [ ] **Step 2: Implement**

`module.go` — field (next to `outstandingCalls sync.WaitGroup` at :67):

```go
	// addMu guards the stopping-check+Add pair (tryAddOutstanding) against
	// Stop's stopping store, so no Add can race Wait-at-zero. LEAF LOCK:
	// may be acquired while holding m.mu (SendCommand does); never hold it
	// across guest calls, channel ops, or m.mu acquisition.
	addMu sync.Mutex
```

Helper (near Stop):

```go
// tryAddOutstanding registers one outstanding host call unless the module
// is stopping. It is the ONLY permitted way to Add to outstandingCalls:
// Stop sets stopping under the same addMu before Waiting, so an Add can
// never race Wait-at-zero (the shutdown-panic class from #266). Callers
// must pair a successful return with outstandingCalls.Done().
func (m *Module) tryAddOutstanding() bool {
	m.addMu.Lock()
	defer m.addMu.Unlock()
	if m.stopping.Load() {
		return false
	}
	m.outstandingCalls.Add(1)
	return true
}
```

`Stop` — wrap the existing store (keep it inside the `m.mu` block so AddAsyncResult's mu-held reads still key off it exactly as today; the comment above it updates to name the helper):

```go
	// Set under mu so AddAsyncResult's mu-held check is race-free, AND
	// under addMu so no tryAddOutstanding can Add after this point —
	// every Add strictly happens-before the Wait below.
	m.addMu.Lock()
	m.stopping.Store(true)
	m.addMu.Unlock()
	close(m.stopCh)
```

Also update the big invariant comment above the `outstandingCalls.Wait()` block (~:229-235): the per-site serialization arguments collapse to "all Add sites go through tryAddOutstanding — see its doc comment."

`SendCommand`: KEEP the early mu-held `if m.stopping.Load()` warn+return (cheap fast path covering both branches of the function), and replace the raw Add:

```go
		if !m.tryAddOutstanding() {
			m.Logger.Warn("dropping command on stopped module", "module", m.Name)
			return
		}
		go func() {
			defer m.outstandingCalls.Done()
			...
```

`hostapi.go:117` (CallAsync launch): replace `d.module.outstandingCalls.Add(1)` with

```go
	if !d.module.tryAddOutstanding() {
		// Module is stopping; the async result would be dropped anyway.
		d.module.Logger.Debug("skipping async call on stopping module", "module", d.module.Name)
		return nil
	}
```

(the `go func() { defer d.module.outstandingCalls.Done(); ... }` body is unchanged).

`hostapi.go:185` (nested trigger_workflow): replace the Add with

```go
		if d.runner != nil {
			if !d.module.tryAddOutstanding() {
				return nil, fmt.Errorf("module %q stopping", d.module.Name)
			}
			go func() {
				defer d.module.outstandingCalls.Done()
				_ = d.runner(d.module.shutdownCtx, workflowID, input)
			}()
		}
```

(check each site's exact surrounding shape and keep return-value semantics: CallAsync's enclosing function returns `error`/`nil` per current code — match it.)

- [ ] **Step 3: Grep proof + tests**

Run: `grep -n "outstandingCalls.Add" internal/wasm/` → ONLY the line inside `tryAddOutstanding`.
Run: `go test ./internal/wasm/ -race -run 'TestModule_TryAddOutstanding|TestModule_' -timeout 120s` → PASS.
Run: `go test ./internal/wasm/ -race` (whole package once, race on) → PASS.
Polarity check (do once, then revert): change the helper body to raw `m.outstandingCalls.Add(1); return true` without the lock/check — the new test (or Stop's Wait) must fail/panic under `-race`; restore and note the result in the report.

- [ ] **Step 4: Gate and commit**

```bash
gofmt -l internal/ && go vet ./internal/wasm/
git add internal/wasm/module.go internal/wasm/hostapi.go internal/wasm/wasm_test.go
git commit -m "refactor(wasm): tryAddOutstanding/addMu makes the outstandingCalls invariant structural (#295)"
```

---

### Task 3: PDK DecodeInto + examples + guide (#294)

**Files:**
- Modify: `pdk/go/noda/codec.go` (new exported func)
- Modify: `examples/wasm-counter/wasm/counter/main.go:12-20` (delete local helper, use `noda.DecodeInto`)
- Modify: `examples/discord-bot/wasm/bot/main.go:28-36` (same)
- Modify: `docs/04-guides/wasm-development.md` (document it where the guide covers Data fields / guest authoring)
- Test: `pdk/go/noda/codec_test.go` (create if absent; check for an existing test file first and extend it)

**Interfaces:**
- Consumes: package-level `activeCodec Codec` (`pdk/go/noda/codec.go:17`).
- Produces: `func DecodeInto(v any, dst any) error` — public PDK API; the examples and Task 5's CI builds depend on it compiling under tinygo.

- [ ] **Step 1: Write the failing test**

```go
func TestDecodeInto_BothCodecs(t *testing.T) {
	type payload struct {
		Op    string `json:"op" msgpack:"op"`
		Count int    `json:"count" msgpack:"count"`
	}
	orig := activeCodec
	t.Cleanup(func() { activeCodec = orig })

	for name, c := range map[string]Codec{"json": &jsonCodec{}, "msgpack": &msgpackCodec{}} {
		t.Run(name, func(t *testing.T) {
			activeCodec = c
			// simulate what the host delivers: a codec-decoded any
			raw, err := c.Marshal(payload{Op: "incr", Count: 7})
			if err != nil {
				t.Fatal(err)
			}
			var decoded any
			if err := c.Unmarshal(raw, &decoded); err != nil {
				t.Fatal(err)
			}
			var dst payload
			if err := DecodeInto(decoded, &dst); err != nil {
				t.Fatalf("DecodeInto: %v", err)
			}
			if dst.Op != "incr" || dst.Count != 7 {
				t.Fatalf("round-trip lost data: %+v", dst)
			}
		})
	}
}
```

(Use the std `testing` package style the PDK already uses — check for testify; if the pdk module doesn't depend on it, plain `t.Fatal` as above. The msgpack subtest is what betrays a hardcoded-JSON implementation.)

Run: `go test ./pdk/go/noda/ -run TestDecodeInto -v` → FAIL `undefined: DecodeInto`. (If the pdk is its own Go module, cd into it — check for `pdk/go/noda/go.mod` or a parent module and use the invocation its existing tests use.)

- [ ] **Step 2: Implement in `codec.go`**

```go
// DecodeInto re-encodes a codec-decoded value with the module's active
// codec and unmarshals it into dst. Use it to get typed access to the
// Data-any fields the host delivers (Command.Data, ClientMessage.Data,
// IncomingWSMsg.Data):
//
//	var op CounterOp
//	if err := noda.DecodeInto(cmd.Data, &op); err != nil { ... }
func DecodeInto(v any, dst any) error {
	b, err := activeCodec.Marshal(v)
	if err != nil {
		return err
	}
	return activeCodec.Unmarshal(b, dst)
}
```

- [ ] **Step 3: Switch both examples**

In each of `examples/wasm-counter/wasm/counter/main.go` and `examples/discord-bot/wasm/bot/main.go`: delete the local `decodeInto` func (and its now-unused `encoding/json` import if nothing else uses it), replace call sites with `noda.DecodeInto(...)` (the `noda` package is already imported in both — verify the alias used).

- [ ] **Step 4: Document in the guide**

In `docs/04-guides/wasm-development.md`, where Data fields / message handling are described, add a short subsection: Data fields arrive codec-decoded as `any`; `noda.DecodeInto(cmd.Data, &typed)` is the supported typed-decode path (with the example snippet from the doc comment). Keep it to ~10 lines matching the guide's style.

- [ ] **Step 5: Verify**

```bash
go test ./pdk/... 2>/dev/null || (cd pdk/go/noda && go test ./...)   # use whichever module layout applies
(cd examples/wasm-counter/wasm/counter && tinygo build -o /tmp/counter.wasm -target wasi -buildmode=c-shared .)
(cd examples/discord-bot/wasm/bot && tinygo build -o /tmp/bot.wasm -target wasi -buildmode=c-shared .)
```

Expected: tests PASS; both modules build. (If an example README documents different build flags, use those — and note it for Task 5's CI job.)
Also: `go test ./internal/wasm/` still green (the host side is untouched but cheap to confirm).

- [ ] **Step 6: Gate and commit**

```bash
gofmt -l pdk/ examples/ | grep -v wasm-helpers && echo FMT-DIRTY || echo FMT-OK   # wasm-helpers hit is pre-existing until Task 5
go vet ./pdk/... 2>/dev/null || (cd pdk/go/noda && go vet ./...)
git add pdk/go/noda/codec.go pdk/go/noda/codec_test.go examples/wasm-counter/wasm/counter/main.go examples/discord-bot/wasm/bot/main.go docs/04-guides/wasm-development.md
git commit -m "feat(pdk): noda.DecodeInto typed-decode for Data-any fields; examples use it (#294)"
```

---

### Task 4: Gateway check order (#265) + msgpack comment (#268.2) + #267 resolution

**Files:**
- Modify: `internal/wasm/gateway.go:82-92` (Connect: whitelist before dup-id)
- Modify: `internal/wasm/gateway_test.go:20-27` (existing dup test gets a whitelisted URL; new order-pinning test)
- Modify: `internal/wasm/wasm_test.go:3528-3530` (comment only)
- Modify: `internal/wasm/runtime.go:159-165` (comment only, unless upstream check says otherwise)

**Interfaces:** none produced.

- [ ] **Step 1: Write the failing order test**

```go
// #265: permission is checked before connection-id state — a guest probing
// with a non-whitelisted URL learns nothing about existing connection ids.
func TestGatewayConnect_WhitelistCheckedBeforeDupID(t *testing.T) {
	g := NewGateway(&Module{Name: "m", Codec: &jsonCodec{}}, testLogger()) // empty AllowWS: nothing whitelisted
	g.conns["c1"] = &gatewayConn{id: "c1", stopCh: make(chan struct{})}
	_, err := g.Connect(context.Background(), map[string]any{"id": "c1", "url": "ws://evil/x"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "PERMISSION_DENIED")
	require.NotContains(t, err.Error(), "already in use")
}
```

Run: `go test ./internal/wasm/ -run TestGatewayConnect -v` → new test FAILS (gets "already in use").

- [ ] **Step 2: Reorder Connect + fix the existing test**

In `gateway.go Connect`: move the `if !g.isAllowed(wsURL) { ... PERMISSION_DENIED ... }` block ABOVE the `g.mu.Lock()` dup-id block (texts unchanged).

In `TestGatewayConnect_RejectsDuplicateID` (gateway_test.go:21): the Module literal gains a whitelist so the dup path is still reachable:

```go
	g := NewGateway(&Module{Name: "m", Codec: &jsonCodec{}, Config: ModuleConfig{AllowWS: []string{"example"}}}, testLogger())
```

(Verify the Module field is `Config ModuleConfig` — `isAllowed` reads `g.module.Config.AllowWS` at gateway.go:428; `containsHost` matches the hostname `example` for `ws://example/x`.)

- [ ] **Step 3: #268.2 comment fix**

`wasm_test.go:3528-3530`: the `TestHostCall_Msgpack` doc comment says numbers "arrive as int64 rather than JSON's float64" — the test actually exercises the full signed/unsigned width matrix (`int8`…`uint64`, see :3501). Reword to: "numbers arrive as narrow integer types (int8..uint64) rather than JSON's float64".

- [ ] **Step 4: #267 upstream check**

1. `go list -m -versions github.com/extism/go-sdk` — list releases newer than v1.7.1.
2. If newer versions exist: fetch the newest (`GOMODCACHE` download via `go mod download github.com/extism/go-sdk@<ver>` in a throwaway dir, or read the source on github) and check whether `CurrentPlugin` (or an equivalent host-call context) gained a `SetError`/host-side error mechanism. Consult source, not memory.
3. **If a mechanism exists:** STOP and report DONE_WITH_CONCERNS with the findings — the bump decision returns to the controller (it may pull breaking changes; do not bump unilaterally).
4. **If not (expected):** strengthen the comment at `runtime.go:159-165` around the `stack[0] = 0` fallback:

```go
				// WriteBytes failing leaves no way to signal an error to the
				// guest: extism go-sdk (v1.7.1) exposes no host-side SetError,
				// and offset 0 is the only void sentinel — so a real
				// PERMISSION_DENIED/NOT_FOUND collapses into apparent void
				// success here. Known-wrong failure direction, accepted until
				// upstream grows an error mechanism. See #267.
```

Record the check's outcome (versions found, what you looked at) in your report — the PR close text for #267 quotes it.

- [ ] **Step 5: Verify, gate, commit**

```bash
go test ./internal/wasm/ -run 'TestGatewayConnect|TestHostCall_Msgpack' -v
go test ./internal/wasm/
gofmt -l internal/ && go vet ./internal/wasm/
git add internal/wasm/gateway.go internal/wasm/gateway_test.go internal/wasm/wasm_test.go internal/wasm/runtime.go
git commit -m "fix(wasm): whitelist before dup-id in gateway Connect; document #267 collapse; msgpack comment (#265, #267, #268)"
```

---

### Task 5: CI tinygo guest builds + gofmt drive-by + CHANGELOG (#296)

**Files:**
- Modify: `.github/workflows/ci.yml` (new job after the existing ones)
- Modify: `examples/wasm-helpers/wasm/helpers/main.go` (gofmt only)
- Modify: `CHANGELOG.md` ([Unreleased])

**Interfaces:**
- Consumes: the build invocations proven in Task 3 (`-target wasi -buildmode=c-shared`); adjust if Task 3's report noted different flags for an example.

- [ ] **Step 1: gofmt the helpers module**

```bash
gofmt -w examples/wasm-helpers/wasm/helpers/main.go
git diff --stat examples/wasm-helpers/   # formatting-only diff
gofmt -l .                                # must print NOTHING now
(cd examples/wasm-helpers/wasm/helpers && tinygo build -o /tmp/helpers.wasm -target wasi -buildmode=c-shared .)   # still builds
```

- [ ] **Step 2: Add the CI job**

Append to `.github/workflows/ci.yml` jobs (mirror the existing `go` job's checkout/setup-go action versions exactly — read them first):

```yaml
  # Compiles every example guest module so PDK/ABI changes can't silently
  # break them again (#269/#296). Non-blocking: not in the required-checks
  # set; promote it once it proves stable.
  wasm-guests:
    name: Wasm guest modules (tinygo)
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4            # match the repo's pinned version
      - uses: actions/setup-go@v5            # match the repo's pinned version + go-version source
        with:
          go-version-file: go.mod
      - uses: acifani/setup-tinygo@v2
        with:
          tinygo-version: '0.40.1'
      - name: Build example guest modules
        run: |
          set -euo pipefail
          (cd examples/wasm-counter/wasm/counter && tinygo build -o /tmp/counter.wasm -target wasi -buildmode=c-shared .)
          (cd examples/discord-bot/wasm/bot && tinygo build -o /tmp/bot.wasm -target wasi -buildmode=c-shared .)
          (cd examples/wasm-helpers/wasm/helpers && tinygo build -o /tmp/helpers.wasm -target wasi -buildmode=c-shared .)
```

Validate the YAML locally: `python3 -c "import yaml,sys; yaml.safe_load(open('.github/workflows/ci.yml'))" && echo YAML-OK`. Also confirm all three build commands pass locally (tinygo 0.40.1 installed).

- [ ] **Step 3: CHANGELOG** ([Unreleased], match entry style; fold into existing subsections)

- **Added:** `noda.DecodeInto` in the Go PDK — typed decode for `Command.Data`/`ClientMessage.Data`/`IncomingWSMsg.Data`; both example guests use it (#294)
- **Added:** CI now compiles every example wasm guest module with tinygo, so PDK/ABI changes can't silently break them (#296)
- **Fixed:** `wasm.query` no longer burns its full timeout when the module is stopping (shutdown/devmode reload) — fails fast with a stopping error (#293)
- **Fixed:** wasm gateway checks the outbound-WS whitelist before the duplicate-connection-id check (fail closed on permission first) (#265)
- **Changed:** the wasm module's outstandingCalls invariant is now structural (`tryAddOutstanding`), not comment-enforced (#295)

- [ ] **Step 4: Gate and commit**

```bash
gofmt -l . && go vet ./... && go build ./...
git add .github/workflows/ci.yml examples/wasm-helpers/wasm/helpers/main.go CHANGELOG.md
git commit -m "ci: tinygo-build example wasm guests; gofmt wasm-helpers; CHANGELOG (#296)"
```

---

### Task 6: Rebase, full verification, review, PR

- [ ] **Step 1: Rebase onto latest main** (PR #313 may have merged; expected overlap only CHANGELOG — resolve as entry union)

```bash
git fetch origin main && git rebase origin/main
```

- [ ] **Step 2: Full verification**

```bash
go build ./... && go vet ./...
gofmt -l .                       # must print NOTHING (drive-by fixed the last hit)
go test ./...
go test ./internal/wasm/ -race -timeout 300s
```

Expected: all green.

- [ ] **Step 3: Whole-branch review** (final code-reviewer over the full branch diff), then PR:

```bash
git push -u origin feat/wasm-pdk-hardening
gh pr create --title "fix(wasm): shutdown fail-fast, structural outstandingCalls invariant, pdk DecodeInto, CI guest builds" \
  --body "$(cat <<'EOF'
Tranche 4 of the open-issue backlog (spec + plan on branch under docs/superpowers/).

- wasm.query fails fast with "module stopping" during shutdown/devmode reload instead of burning its full timeout (#293)
- outstandingCalls Add/Wait invariant is structural now: tryAddOutstanding()/addMu is the only Add path (grep-enforced), Stop stores stopping under the same lock — a future Add site can't reintroduce the shutdown panic (#295)
- PDK: noda.DecodeInto — the typed-decode path for Data-any fields; both example guests switched off their copy-pasted helpers; documented in the wasm dev guide (#294)
- CI: new non-blocking wasm-guests job tinygo-builds all three example modules, killing the silent-ABI-break class behind #269 (#296); the long-standing wasm-helpers gofmt hit is fixed in passing
- Gateway Connect checks the outbound-WS whitelist before the duplicate-id check — fail closed on permission first (#265)
- #267 (envelope WriteBytes → void-success collapse): upstream check outcome: <FILL FROM TASK 4 REPORT>; the fallback branch now documents the collapse direction. Closing as revisit-when-upstream-moves (or fixed, per outcome).
- Test polish: hung-tick kill now pins m.failed; msgpack test comment matches its assertions (#268)

Closes #293
Closes #294
Closes #295
Closes #296
Closes #265
Closes #267
Closes #268

🤖 Generated with [Claude Code](https://claude.com/claude-code)
EOF
)"
```

(Replace `<FILL FROM TASK 4 REPORT>` with the actual upstream-check result before creating the PR.)

Wait for the required functional CI checks — and confirm the new `wasm-guests` job runs green on the PR (it's non-required but must not be red).
