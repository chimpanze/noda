# Lifecycle/Devmode/Registry Hardening (Tranche E2) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fix the 6 lifecycle/devmode/registry findings from `REVIEW-FINDINGS-2026-07-05.md` (platform-1 High + platform-2/3/4/5/6 Medium) — shutdown honored during boot, dev-mode reloads serialized and awaited at shutdown, the watcher handles new subdirs and deletions, and timed-out service creation releases its resource.

**Architecture:** In-process concurrency/resource fixes in `internal/lifecycle`, `internal/devmode`, `internal/registry`. No public API break (one dev-only method rename `SetShuttingDown`→`Shutdown`).

**Tech Stack:** Go (go1.25), fsnotify, `pkg/api` plugin interface.

## Global Constraints

- Go module floor: **go1.25** (relied on for per-iteration loop-variable capture in Task 4).
- A shutdown request during `StartAll` must stop what started and prevent starting more — no swallowed signal. `started` is the single source of truth; `StopAll` always stops the current `started` and is idempotent.
- Dev-mode reloads run one-at-a-time (latest config wins); no `onReload` fires after shutdown begins; `Shutdown()` blocks until any in-flight reload finishes.
- The watcher reacts to `.json` creates/writes/renames/**deletes**, and watches **newly created subdirectories**.
- Timed-out `CreateService` cleanup calls `plugin.Shutdown(instance)` (not a generic `Close()`).
- Touched packages' tests run under `-race`.
- Pre-push gate: `gofmt -l .`, `go vet ./...`, `golangci-lint run`, `go test -race ./internal/lifecycle/... ./internal/devmode/... ./internal/registry/...`.

**Worktree:** `.worktrees/lifecycle-devmode-hardening`, branch `feat/lifecycle-devmode-hardening` off `main`. Spec + this plan force-added.

## File map

- `internal/lifecycle/lifecycle.go` — `shuttingDown` flag, incremental `started`, after-loop re-check, idempotent `StopAll` (Task 1).
- `internal/devmode/reload.go` — `reloadMu`, `reloadWg`, `Shutdown()`, post-validation re-check (Task 2); `internal/lifecycle/adapters.go:121` caller.
- `internal/devmode/watcher.go` — `loop()` op mask + add-dir-on-Create (Task 3).
- `internal/registry/lifecycle.go` — cleanup goroutine calls `plugin.Shutdown` (Task 4).

---

### Task 1: Shutdown during boot (platform-1, High)

**Files:**
- Modify: `internal/lifecycle/lifecycle.go` (`Lifecycle` struct, `StartAll`, `StopAll`)
- Test: `internal/lifecycle/lifecycle_test.go`

**Interfaces:**
- Produces: `Lifecycle.shuttingDown bool` (guarded by `l.mu`).

- [ ] **Step 1: Write the failing test** — a `StopAll` during `StartAll` must stop the started components and prevent starting the rest.

```go
// A component whose Start blocks until released, so we can fire StopAll mid-boot.
type gateComponent struct {
	name     string
	started  *atomic.Bool
	stopped  *atomic.Bool
	startGate chan struct{} // Start blocks until closed (nil = don't block)
}

func (c *gateComponent) Name() string { return c.name }
func (c *gateComponent) Start(ctx context.Context) error {
	if c.startGate != nil {
		<-c.startGate
	}
	c.started.Store(true)
	return nil
}
func (c *gateComponent) Stop(ctx context.Context) error { c.stopped.Store(true); return nil }

func TestStartAll_ShutdownDuringBootIsHonored(t *testing.T) {
	aStarted, aStopped := &atomic.Bool{}, &atomic.Bool{}
	bStarted, bStopped := &atomic.Bool{}, &atomic.Bool{}
	gate := make(chan struct{})
	a := &gateComponent{name: "a", started: aStarted, stopped: aStopped}                 // starts immediately
	b := &gateComponent{name: "b", started: bStarted, stopped: bStopped, startGate: gate} // blocks in Start

	l := New(slog.Default()) // or however Lifecycle is constructed in existing tests
	l.Register(a)            // use the existing registration API
	l.Register(b)

	startErr := make(chan error, 1)
	go func() { startErr <- l.StartAll(context.Background()) }()

	// Wait until "a" has started and StartAll is blocked in b.Start.
	require.Eventually(t, func() bool { return aStarted.Load() }, time.Second, 5*time.Millisecond)

	// Fire shutdown while b is still starting.
	go l.StopAll(context.Background())
	// Let b finish starting.
	time.Sleep(20 * time.Millisecond)
	close(gate)

	<-startErr
	require.Eventually(t, func() bool { return aStopped.Load() }, time.Second, 5*time.Millisecond,
		"started component must be stopped by the shutdown")
	// b must also end up stopped if it started, OR never counted — in all cases not left running.
	require.Eventually(t, func() bool { return !bStarted.Load() || bStopped.Load() }, time.Second, 5*time.Millisecond,
		"a component that started must not be left running after shutdown")
}

func TestStartAll_AbortsWhenAlreadyShuttingDown(t *testing.T) {
	l := New(slog.Default())
	started := &atomic.Bool{}
	l.Register(&gateComponent{name: "x", started: started, stopped: &atomic.Bool{}})
	l.StopAll(context.Background()) // sets shuttingDown
	err := l.StartAll(context.Background())
	require.Error(t, err)
	require.False(t, started.Load(), "no component should start once shutting down")
}
```

(Use the ACTUAL Lifecycle constructor/registration API from `lifecycle_test.go` — adapt `New`/`Register`. If a fake Component already exists there, extend it with the start gate instead of adding a new type.)

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/lifecycle/ -run 'TestStartAll_ShutdownDuringBoot|TestStartAll_AbortsWhenAlreadyShuttingDown' -race`
Expected: FAIL — `StopAll` no-ops during boot (`started==0`), so `a` isn't stopped and/or `StartAll` after shutdown still starts.

- [ ] **Step 3: Implement the coordination**

In `lifecycle.go`, add `shuttingDown bool` to `Lifecycle`. Rewrite `StartAll`:

```go
func (l *Lifecycle) StartAll(ctx context.Context) error {
	l.mu.Lock()
	n := len(l.components)
	components := make([]Component, n)
	copy(components, l.components)
	l.mu.Unlock()

	for _, c := range components {
		l.mu.Lock()
		down := l.shuttingDown
		l.mu.Unlock()
		if down {
			l.StopAll(l.rollbackCtx())
			return fmt.Errorf("startup aborted: shutdown requested")
		}

		l.logger.Info("starting component", "name", c.Name())
		if err := c.Start(ctx); err != nil {
			l.logger.Error("component start failed", "name", c.Name(), "error", err)
			l.StopAll(l.rollbackCtx())
			return fmt.Errorf("starting %s: %w", c.Name(), err)
		}

		l.mu.Lock()
		l.started++
		l.mu.Unlock()
	}

	// After-loop re-check closes the last-component window.
	l.mu.Lock()
	down := l.shuttingDown
	l.mu.Unlock()
	if down {
		l.StopAll(l.rollbackCtx())
		return fmt.Errorf("startup aborted: shutdown requested")
	}
	return nil
}

// rollbackCtx returns a bounded context for stopping components during an
// aborted/failed startup.
func (l *Lifecycle) rollbackCtx() context.Context {
	ctx, cancel := context.WithTimeout(context.Background(), l.rollbackDeadline)
	// cancel is intentionally leaked to the GC-bounded timeout; StopAll returns
	// well within rollbackDeadline. (Or: return ctx, cancel and defer in caller.)
	_ = cancel
	return ctx
}
```

(If leaking `cancel` is undesirable, inline the `context.WithTimeout`+`defer cancel()` at each of the three StopAll call sites instead of the helper — the implementer picks whichever is cleaner and lint-clean.)

Rewrite `StopAll`'s header to set the flag and read the source-of-truth counter under one lock:

```go
func (l *Lifecycle) StopAll(parent context.Context) {
	l.mu.Lock()
	l.shuttingDown = true
	started := l.started
	components := make([]Component, started)
	copy(components, l.components[:started])
	l.started = 0
	l.mu.Unlock()

	if started == 0 {
		return
	}
	// ... existing reverse-stop loop unchanged ...
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/lifecycle/ -run 'TestStartAll_ShutdownDuringBoot|TestStartAll_AbortsWhenAlreadyShuttingDown' -race -count=5`
Expected: PASS (run repeated to shake out the interleaving).

- [ ] **Step 5: Full lifecycle suite**

Run: `go test ./internal/lifecycle/... -race`
Expected: PASS (existing start/stop/rollback tests still green — note: a `StopAll` now leaves `shuttingDown=true`; if any existing test reuses a Lifecycle across a StopAll then StartAll expecting success, that now returns the abort error — such a test should construct a fresh Lifecycle, which is the correct usage).

- [ ] **Step 6: Commit**

```bash
git add internal/lifecycle/lifecycle.go internal/lifecycle/lifecycle_test.go
git commit -m "fix(lifecycle): honor shutdown signal during startup (platform-1)"
```

---

### Task 2: Reload serialization + shutdown await (platform-2 + platform-3)

**Files:**
- Modify: `internal/devmode/reload.go` (`Reloader`, `HandleChange`, `SetShuttingDown`→`Shutdown`), `internal/lifecycle/adapters.go:121` (caller)
- Test: `internal/devmode/devmode_test.go`

**Interfaces:**
- Produces: `Reloader.Shutdown()` (replaces `SetShuttingDown()`); reloads serialized by `reloadMu`.

- [ ] **Step 1: Write the failing tests**

```go
// (a) Shutdown() blocks until an in-flight reload finishes, and a reload that
// begins after shutdown does not call onReload.
func TestReloader_ShutdownAwaitsInFlightReload(t *testing.T) {
	r := NewReloader(tmpValidConfigDir(t), "", initialRC(t), nil, slog.Default())
	inReload := make(chan struct{})
	releaseReload := make(chan struct{})
	var reloadRan atomic.Bool
	r.OnReload(func(*config.ResolvedConfig) {
		reloadRan.Store(true)
		close(inReload)
		<-releaseReload // hold the reload open
	})

	go r.HandleChange("x.json")           // enters onReload, blocks
	<-inReload

	shutdownReturned := make(chan struct{})
	go func() { r.Shutdown(); close(shutdownReturned) }()
	select {
	case <-shutdownReturned:
		t.Fatal("Shutdown returned before the in-flight reload finished")
	case <-time.After(50 * time.Millisecond):
	}
	close(releaseReload)                  // let the reload finish
	<-shutdownReturned                    // Shutdown now returns

	// A reload after shutdown must not fire onReload again.
	reloadRan.Store(false)
	r.HandleChange("y.json")
	require.False(t, reloadRan.Load(), "onReload must not fire after Shutdown")
}
```

(Use the existing devmode test setup for a valid config dir + initial `ResolvedConfig`; `tmpValidConfigDir`/`initialRC` are placeholders for whatever the existing tests use. If a full config dir is heavy, the platform-2 "latest wins" property can also be checked with two sequential `HandleChange` calls against a dir edited between them, asserting `r.Config()` reflects the second edit.)

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/devmode/ -run TestReloader_ShutdownAwaitsInFlightReload -race`
Expected: FAIL — `SetShuttingDown`/`Shutdown` doesn't exist as a blocking wait; `onReload` can fire after shutdown.

- [ ] **Step 3: Add reloadMu, reloadWg, Shutdown, post-validation re-check**

In `reload.go`, add fields to `Reloader`:

```go
	reloadMu sync.Mutex     // serializes the whole HandleChange (latest wins)
	reloadWg sync.WaitGroup // tracks in-flight reloads for Shutdown to await
```

Replace `SetShuttingDown` with `Shutdown`:

```go
// Shutdown marks the reloader as shutting down and blocks until any in-flight
// reload has finished, so no onReload callback fires into a closing system.
func (r *Reloader) Shutdown() {
	r.shuttingDown.Store(true)
	r.reloadWg.Wait()
}
```

Rewrite `HandleChange` to serialize, track, and re-check:

```go
func (r *Reloader) HandleChange(path string) {
	if r.shuttingDown.Load() {
		return
	}
	r.reloadWg.Add(1)
	defer r.reloadWg.Done()

	r.reloadMu.Lock()
	defer r.reloadMu.Unlock()

	// Re-check after acquiring the serialization lock (shutdown may have begun
	// while we queued behind another reload).
	if r.shuttingDown.Load() {
		return
	}

	r.logger.Info("reloading config", "trigger", path)
	// ... existing secrets + ValidateAll ... (unchanged; on error, log/emit + return) ...

	// Re-check after validation (which is slow) before swapping / firing onReload.
	if r.shuttingDown.Load() {
		return
	}

	r.mu.Lock()
	r.config = rc
	r.mu.Unlock()
	r.logger.Info("config reloaded successfully", "files", rc.FileCount)
	// ... existing hub emit ...
	if r.onReload != nil {
		r.onReload(rc)
	}
}
```

(Keep the existing `r.mu` swap narrow — lock only around `r.config = rc`, not around `onReload`, to avoid holding the read-lock during the callback. The `reloadMu` provides the serialization; `r.mu` only guards `Config()`.)

In `internal/lifecycle/adapters.go:121`, change `c.reloader.SetShuttingDown()` → `c.reloader.Shutdown()`. Ensure this runs before services are torn down (it's already in the reloader adapter's Stop path).

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/devmode/ -run TestReloader_ShutdownAwaitsInFlightReload -race`
Expected: PASS.

- [ ] **Step 5: Full devmode + lifecycle suites**

Run: `go test -race ./internal/devmode/... ./internal/lifecycle/...`
Expected: PASS (the `adapters.go` caller compiles with the renamed method).

- [ ] **Step 6: Commit**

```bash
git add internal/devmode/reload.go internal/lifecycle/adapters.go internal/devmode/devmode_test.go
git commit -m "fix(devmode): serialize reloads and await in-flight reload at shutdown (platform-2, platform-3)"
```

---

### Task 3: Watcher — new subdirs + deletes (platform-5 + platform-6)

**Files:**
- Modify: `internal/devmode/watcher.go` (`loop()`)
- Test: `internal/devmode/devmode_test.go`

- [ ] **Step 1: Write the failing tests**

```go
func TestWatcher_ReactsToNewSubdirAndDelete(t *testing.T) {
	dir := t.TempDir()
	changes := make(chan string, 8)
	w, err := NewWatcher(func(p string) { changes <- p }, slog.Default())
	require.NoError(t, err)
	w.debounce = 20 * time.Millisecond
	require.NoError(t, w.WatchDir(dir))
	w.Start()
	defer w.Stop(context.Background())

	// (platform-5) a .json created under a NEW subdirectory triggers a reload.
	sub := filepath.Join(dir, "routes")
	require.NoError(t, os.Mkdir(sub, 0755))
	time.Sleep(50 * time.Millisecond) // let the watcher pick up the new dir
	require.NoError(t, os.WriteFile(filepath.Join(sub, "r.json"), []byte("{}"), 0644))
	select {
	case <-changes:
	case <-time.After(time.Second):
		t.Fatal("no reload for a .json created under a new subdirectory")
	}

	// (platform-6) deleting a watched .json triggers a reload.
	f := filepath.Join(dir, "top.json")
	require.NoError(t, os.WriteFile(f, []byte("{}"), 0644))
	<-changes // the create
	require.NoError(t, os.Remove(f))
	select {
	case <-changes:
	case <-time.After(time.Second):
		t.Fatal("no reload on .json delete")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/devmode/ -run TestWatcher_ReactsToNewSubdirAndDelete -race`
Expected: FAIL — the new subdir isn't watched (its file create never seen) and Remove isn't in the mask.

- [ ] **Step 3: Add Remove to the mask and watch new dirs**

In `watcher.go loop()`:

```go
			// React to write/create/rename/remove.
			if event.Op&(fsnotify.Write|fsnotify.Create|fsnotify.Rename|fsnotify.Remove) == 0 {
				continue
			}

			// A newly created directory must be added to the watcher (fsnotify is
			// non-recursive), and its subtree covered in case a tree was created
			// at once. Then fall through to schedule a reload.
			if event.Op&fsnotify.Create != 0 {
				if info, err := os.Stat(event.Name); err == nil && info.IsDir() {
					if len(info.Name()) == 0 || info.Name()[0] != '.' { // skip hidden dirs, matching WatchDir
						if werr := w.WatchDir(event.Name); werr != nil {
							w.logger.Warn("watcher: failed to watch new directory", "path", event.Name, "error", werr.Error())
						}
					}
					// a dir create schedules a reload (new config may live under it)
					w.scheduleReload(event.Name, &timer, &lastPath)
					continue
				}
			}

			ext := filepath.Ext(event.Name)
			if ext != ".json" {
				continue
			}
			w.scheduleReload(event.Name, &timer, &lastPath)
```

Where `scheduleReload` factors out the existing debounce-timer logic (capture `path`, stop old timer, `time.AfterFunc`). If refactoring into a method is awkward with the local `timer`/`lastPath`, inline the same debounce block in both the dir-create and the `.json` branches instead — the essential change is: (a) `Remove` in the mask, (b) `os.Stat`+`WatchDir` on a dir Create before the `.json` filter. Keep the hidden-dir skip consistent with `WatchDir` (which uses `filepath.SkipDir`).

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/devmode/ -run TestWatcher_ReactsToNewSubdirAndDelete -race -count=3`
Expected: PASS (fsnotify timing — the 50ms sleep after Mkdir gives the watcher time to Add the dir; keep the debounce short).

- [ ] **Step 5: Full devmode suite**

Run: `go test ./internal/devmode/... -race`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/devmode/watcher.go internal/devmode/devmode_test.go
git commit -m "fix(devmode): watch new subdirs and react to config-file deletes (platform-5, platform-6)"
```

---

### Task 4: Registry service-cleanup leak (platform-4)

**Files:**
- Modify: `internal/registry/lifecycle.go` (timed-out-create cleanup goroutine)
- Test: `internal/registry/lifecycle_test.go`

- [ ] **Step 1: Write the failing test**

```go
// A plugin whose CreateService returns AFTER the create timeout, so the cleanup
// goroutine runs; assert it calls the plugin's Shutdown (not a Close()).
func TestInitializeServices_LateCreateShutDownViaPlugin(t *testing.T) {
	shutdownCalled := make(chan any, 1)
	p := &lateCreatePlugin{ // implements api.Plugin; CreateService sleeps > createTimeout then returns an instance
		createDelay:  200 * time.Millisecond,
		instance:     &struct{ id int }{id: 1},
		onShutdown:   func(inst any) error { shutdownCalled <- inst; return nil },
	}
	// register p in a PluginRegistry; call InitializeServices with a short createTimeout (e.g. 20ms).
	// ... build services config referencing p's plugin name ...
	_, errs := reg.InitializeServices(context.Background(), servicesCfg, 20*time.Millisecond)
	require.NotEmpty(t, errs) // creation timed out

	select {
	case inst := <-shutdownCalled:
		require.Equal(t, p.instance, inst, "cleanup must Shutdown the late-completing instance via the plugin")
	case <-time.After(2 * time.Second):
		t.Fatal("plugin.Shutdown was not called on the late-completing instance")
	}
}
```

(Define `lateCreatePlugin` implementing the `api.Plugin` interface — reuse the existing `testplugin_test.go` fake if it can be given a slow `CreateService` + a `Shutdown` recorder. Match `InitializeServices`'s real signature and the createTimeout parameter.)

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/registry/ -run TestInitializeServices_LateCreateShutDownViaPlugin -race`
Expected: FAIL — the cleanup does `res.instance.(interface{ Close() error })`, which the instance doesn't implement, so `Shutdown` is never called.

- [ ] **Step 3: Call plugin.Shutdown in the cleanup**

In `internal/registry/lifecycle.go`, replace the cleanup goroutine's `Close()` probe with `plugin.Shutdown`. Capture `plugin` in the closure (go1.25 per-iteration loop variables make this safe):

```go
			go func(name string) {
				select {
				case res := <-resultCh:
					if res.err == nil && res.instance != nil {
						if err := plugin.Shutdown(res.instance); err != nil {
							slog.Warn("timed-out service creation completed late, shutdown failed", "name", name, "error", err.Error())
						} else {
							slog.Warn("timed-out service creation completed late, resource shut down", "name", name)
						}
					}
				case <-ctx.Done():
					slog.Warn("timed-out service creation cleanup abandoned at shutdown", "name", name)
				}
			}(name)
```

(`plugin` is the loop's per-iteration `api.Plugin` from `plugins.GetByName`; capturing it in the closure is safe under go1.25. If the linter prefers an explicit param, add `plugin api.Plugin` to the goroutine's parameter list and pass it — requires importing `pkg/api` in the file.)

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/registry/ -run TestInitializeServices_LateCreateShutDownViaPlugin -race`
Expected: PASS.

- [ ] **Step 5: Full registry suite**

Run: `go test ./internal/registry/... -race`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/registry/lifecycle.go internal/registry/lifecycle_test.go
git commit -m "fix(registry): shut down late-completing timed-out service via plugin.Shutdown (platform-4)"
```

---

### Task 5: CHANGELOG + full gate

**Files:**
- Modify: `CHANGELOG.md`

- [ ] **Step 1: CHANGELOG entry**

Add under `### Fixed`: "Lifecycle/devmode hardening: a shutdown signal received during startup is now honored (stops what started, aborts the rest) instead of being swallowed until a second signal; dev-mode config reloads are serialized (the latest change wins) and awaited at shutdown so no reload callback fires into a closing system; the dev-mode file watcher now reacts to config files created under new subdirectories and to config-file deletions; a service whose creation times out is properly shut down via its plugin if it completes late (no leaked connection pool)."

- [ ] **Step 2: Full gate**

Run: `gofmt -l . && go vet ./... && golangci-lint run && go test -race ./internal/lifecycle/... ./internal/devmode/... ./internal/registry/...`
Expected: clean, all pass. Fix any lint issues introduced by this branch; leave pre-existing/unrelated ones (note them). If the `rollbackCtx` helper trips an errcheck/leak lint, inline `context.WithTimeout`+`defer cancel()` at the StopAll call sites instead.

- [ ] **Step 3: Commit**

```bash
git add CHANGELOG.md
git commit -m "docs(lifecycle): changelog for lifecycle/devmode/registry hardening"
```

---

## Self-review notes

- **Spec coverage:** platform-1 → Task 1; platform-2+3 → Task 2; platform-5+6 → Task 3; platform-4 → Task 4; changelog/gate → Task 5. All six covered.
- **Type consistency:** `Lifecycle.shuttingDown` (Task 1) set in `StopAll`, read in `StartAll`. `Reloader.Shutdown()` (Task 2) replaces `SetShuttingDown()` at the one caller `adapters.go:121`. `plugin.Shutdown` (Task 4) is `api.Plugin.Shutdown(any) error`.
- **Concurrency-safety notes:** Task 1 — `StopAll` is idempotent (reads current `started`, sets 0); `StartAll` never holds `l.mu` across `Start`; the after-loop re-check closes the last-component window; run tests with `-count`. Task 2 — `reloadMu` serializes the whole reload; `reloadWg` + `Shutdown()` await in-flight; re-check after validation. Task 3 — fsnotify timing needs a real temp dir + short debounce + a small sleep after Mkdir.
- **Test-harness notes:** adapt the Lifecycle constructor/registration and the devmode/registry fakes to the EXISTING helpers in `lifecycle_test.go`, `devmode_test.go`, `registry/testplugin_test.go` rather than inventing new ones where they exist.
- **Deferred (out of scope):** platform-7..15 (Low long-tail); no change to the debounce mechanism or rollback-deadline behavior beyond the shutdown-flag coordination.
