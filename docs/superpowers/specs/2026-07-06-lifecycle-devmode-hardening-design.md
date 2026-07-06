# Lifecycle/Devmode/Registry Hardening (Tranche E2) — Design

Date: 2026-07-06
Source: `REVIEW-FINDINGS-2026-07-05.md` — platform-1 (High), platform-2, platform-3, platform-4, platform-5, platform-6 (5 Medium).
Branch/worktree (planned): `feat/lifecycle-devmode-hardening` in `.worktrees/lifecycle-devmode-hardening`, off `main`.

## Why

Tranche E2 of the worker/scheduler+lifecycle split (E1 shipped as PR #282). These six defects are in-process concurrency/resource bugs in the runtime's startup/shutdown and dev-mode hot-reload paths: a SIGTERM during boot is swallowed (only a forced second signal exits), dev-mode reloads race and can install a stale config or fire after shutdown, the file watcher misses new subdirectories and file deletions, and a timed-out service creation leaks its resource.

**Context:** E was split (user decision) into E1 (worker/scheduler, shipped) and E2 (this, in-process lifecycle/devmode/registry).

## Findings in scope

| ID | Sev | Summary |
|---|---|---|
| platform-1 | High | SIGTERM during `StartAll` swallowed: `StopAll` no-ops while `l.started==0`, and `StartAll` sets `started=n` after the stop ran → shutdown-during-boot lost; only a forced 2nd-signal `os.Exit` remains |
| platform-2 | Med | Concurrent `HandleChange` invocations (editor save + watcher debounce) can install a stale config last |
| platform-3 | Med | In-flight reload not awaited at shutdown: debounce `AfterFunc` untracked, `SetShuttingDown` checked only at reload entry → `onReload` can run after services close |
| platform-4 | Med | Timed-out `CreateService` cleanup probes `Close() error`, which no service instance implements → should call `plugin.Shutdown` → late-completing DB pools leak |
| platform-5 | Med | Watcher never watches subdirectories created after startup (fsnotify non-recursive; dir-Create events dropped by the `.json` filter) |
| platform-6 | Med | Deleting a config file never triggers a reload (`Remove` excluded from the watcher op mask) |

## Verified facts

- `internal/lifecycle/lifecycle.go`: `Lifecycle{mu sync.Mutex, components []Component, started int, logger, rollbackDeadline}`. `StartAll` sets `l.started = i` only on a component start *failure*, and `l.started = n` after all succeed. `StopAll` reads `started := l.started`, copies `components[:started]`, sets `started=0`, and returns early when `started==0`.
- `internal/devmode/reload.go`: `Reloader{configDir, envFlag, logger, hub, shuttingDown atomic.Bool, mu sync.RWMutex, config, onReload}`. `HandleChange(path)` checks `shuttingDown` at entry, validates (unlocked), then takes `r.mu.Lock()` for the swap + `onReload`. `SetShuttingDown()` sets the flag. Called at `internal/lifecycle/adapters.go:121` (`c.reloader.SetShuttingDown()`).
- `internal/devmode/watcher.go`: `Watcher{watcher *fsnotify.Watcher, logger, onChange, debounce, done, wg}`. `WatchDir` walks + `watcher.Add`s dirs (skips hidden) at startup. `loop()` filters `event.Op&(fsnotify.Write|fsnotify.Create|fsnotify.Rename) == 0` then `filepath.Ext != ".json"`, then debounces via `time.AfterFunc(w.debounce, …)` calling `w.onChange(path)`. `Stop(ctx)` closes `done`, `watcher.Close()`, waits `wg`; the loop stops the pending timer on `<-w.done`.
- `internal/registry/lifecycle.go`: in the per-service loop, `plugin, found := plugins.GetByName(...)`. On `CreateService` timeout, the cleanup goroutine does `if closer, ok := res.instance.(interface{ Close() error }); ok { closer.Close() }`. `pkg/api/plugin.go:11`: `Plugin.Shutdown(service any) error` (composite delegates, `plugins.go:89`).

## Design

### Unit 1 — Shutdown during boot (platform-1)

Track startup incrementally and coordinate with a shutdown flag, all under `l.mu`. **`started` is the single source of truth** for "components currently started and not yet stopped"; `StopAll` always stops exactly the current `started` (never a stale local prefix).

- Add `shuttingDown bool` to `Lifecycle`.
- `StartAll`, for each component:
  1. lock `l.mu`; if `shuttingDown`, unlock, call `l.StopAll(...)`, and return `fmt.Errorf("startup aborted: shutdown requested")`; else unlock.
  2. `err := c.Start(ctx)` (**outside** the lock — `Start` may block).
  3. on `err != nil`: call `l.StopAll(...)` and return `fmt.Errorf("starting %s: %w", ...)`.
  4. lock `l.mu`; `l.started++`; unlock.
- **After the loop:** lock `l.mu`; if `shuttingDown`, unlock, call `l.StopAll(...)`, return the abort error; else unlock, return nil. This after-loop re-check closes the last-component window (a `StopAll` that interleaves right as the final component starts).
- Remove the old `l.started = i` on-error assignment and the final `l.started = n`; the counter is maintained incrementally in step 4.
- `StopAll`: at the top, under `l.mu`, set `shuttingDown = true`, read `started := l.started`, set `l.started = 0`; then reverse-stop the `started` components (as today). It is **idempotent** — a second call reads `started == 0` and no-ops.

**Why every interleaving is safe:** `StopAll` sets `shuttingDown` and reads the *incrementally-maintained* `started` under the same mutex the increment uses. If a component's `Start` completes but a `StopAll` runs before its `started++`, then either (a) `StopAll` read the pre-increment count and stopped the prefix, and `StartAll`'s subsequent per-iteration/after-loop `shuttingDown` check fires and calls `StopAll` again — which now reads the post-increment count and stops that component; or (b) the increment lands first and the single `StopAll` stops it. In all orderings, no component is left running after a `StopAll`, and `StartAll` starts nothing new once `shuttingDown` is set. `StartAll` never holds `l.mu` across `Start`.

### Unit 2 — Reload serialization + shutdown await (platform-2 + platform-3)

One coherent design in `internal/devmode/reload.go`:
- Add `reloadMu sync.Mutex` and hold it across the **entire** `HandleChange` body (validate → swap → `onReload`). Serializing the whole operation means each reload re-reads current on-disk state and the *latest* change validates the freshest config and wins deterministically (fixing platform-2). `r.mu` remains the fast RWMutex guarding `Config()`/the swap only.
- Add `reloadWg sync.WaitGroup`; `HandleChange` does `reloadWg.Add(1)`/`defer reloadWg.Done()` (after passing the initial `shuttingDown` gate). Replace `SetShuttingDown()` with `Shutdown()`: set `shuttingDown=true`, then `reloadWg.Wait()` — blocks until any in-flight reload finishes. Update the caller `internal/lifecycle/adapters.go:121` to call `Shutdown()` (the reloader adapter's Stop path, before services are torn down).
- Re-check `shuttingDown` **after** validation and **before** `onReload` (validation is slow; shutdown may have begun meanwhile) — if set, skip the swap/callback and return.

Result: reloads run one-at-a-time (latest wins), and no `onReload` fires after shutdown has begun or is abandoned mid-flight without being awaited.

### Unit 3 — Watcher: new subdirs + deletes (platform-5 + platform-6)

In `internal/devmode/watcher.go loop()`:
- Add `fsnotify.Remove` to the op mask: `event.Op&(fsnotify.Write|fsnotify.Create|fsnotify.Rename|fsnotify.Remove) == 0`. A deleted `.json` (or dir) then triggers the debounced reload; `HandleChange` re-validates the remaining config (platform-6).
- On a `Create` event, **before** the `.json` extension filter, `os.Stat` the path; if it is a **directory**, add it (and its subtree) to the watcher and trigger a reload. Factor the recursive add into a small helper reusing `WatchDir`'s walk (or call `w.watcher.Add` on the new dir and `WatchDir` on it to cover a tree created at once). This makes watching effectively recursive for post-startup directories (platform-5). Non-dir Creates continue through the existing `.json` filter.

Keep the hidden-dir skip consistent with `WatchDir`.

### Unit 4 — Registry service-cleanup leak (platform-4)

In `internal/registry/lifecycle.go`, the timed-out-create cleanup goroutine must tear the late instance down via its plugin, not a generic `Close()` type-assertion. Capture the `plugin` into the goroutine and, on a late successful result, call `plugin.Shutdown(res.instance)` (log the outcome). Every service plugin implements `Shutdown(service any) error`, so the DB pool (and any other resource) is actually released.

```go
	go func(name string, plugin api.Plugin) {
		select {
		case res := <-resultCh:
			if res.err == nil && res.instance != nil {
				if err := plugin.Shutdown(res.instance); err != nil {
					slog.Warn("timed-out service creation completed late, shutdown failed", "name", name, "error", err)
				} else {
					slog.Warn("timed-out service creation completed late, resource shut down", "name", name)
				}
			}
		case <-ctx.Done():
			slog.Warn("timed-out service creation cleanup abandoned at shutdown", "name", name)
		}
	}(name, plugin)
```

## Testing (per finding)

- **platform-1:** a test that calls `StopAll` concurrently with (or in the middle of) `StartAll` and asserts the started components are stopped and no further components start — e.g. a component whose `Start` blocks on a signal, fire `StopAll` from another goroutine, release, and assert the remaining components never started and all started ones got `Stop`. Plus: `StartAll` after `StopAll`/`shuttingDown` returns the abort error and starts nothing.
- **platform-2:** two concurrent `HandleChange` calls serialized by `reloadMu`; assert the final `Config()` is the latest on-disk state (not an older one) — drive via a fake validate or two successive on-disk edits.
- **platform-3:** an in-flight `HandleChange` (blocked in `onReload`) with `Shutdown()` called from another goroutine — assert `Shutdown()` blocks until the reload finishes; and a `HandleChange` that begins validating, then `shuttingDown` is set, does NOT call `onReload`.
- **platform-5:** create a `.json` under a new subdirectory after `Start()`; assert `onChange` fires (the new dir is watched). platform-6: delete a watched `.json`; assert `onChange` fires. (Use a temp dir, real `fsnotify`, and the existing watcher test pattern with a short debounce.)
- **platform-4:** a fake plugin whose `CreateService` blocks past `createTimeout` then returns an instance; assert the cleanup goroutine calls the fake's `Shutdown` (recorded), not a `Close()`.

Gate: `gofmt -l .`, `go vet ./...`, `golangci-lint run`, `go test -race ./internal/lifecycle/... ./internal/devmode/... ./internal/registry/...`.

## Mechanics

- Worktree `.worktrees/lifecycle-devmode-hardening`, branch `feat/lifecycle-devmode-hardening` off `main`.
- Subagent-driven execution per task: implementer → spec-compliance reviewer → code-quality reviewer.
- Spec + plan force-added to the branch.
- CHANGELOG "Fixed" entry.
- At merge: add a "Shipped 2026-07-06" note for platform-1..6 to `REVIEW-FINDINGS-2026-07-05.md` (on review PR #262's branch).

## Out of scope

Other platform findings (platform-7..15 Low long-tail). No change to the watcher's debounce mechanism or the lifecycle rollback-deadline behavior beyond what the shutdown-flag coordination requires.
