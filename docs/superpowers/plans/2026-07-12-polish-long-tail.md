# Polish Long Tail Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Close the final nine backlog issues: engine test coverage + honest sub-workflow timeout duration (#272, #273), the `request.raw_body` alias gap (#275), scaffold/generate cosmetics (#276, #277), the connmgr wildcard chokepoint (#279), scheduler overlap-skip history (#284), worker timeout consolidation (#285), and a ctx-bounded devmode Shutdown (#287).

**Architecture:** Nine independent small fixes across six packages, grouped into five implementation tasks by subsystem. The two touchy ones: #285 makes `processMessage`'s outer `WithTimeout` the single timeout owner while the middleware keeps only its panic-to-error shield (config name `worker.timeout` MUST keep working — it's user-facing config surface); #279 moves the wildcard guard into `Manager.Send`/`SendSSE` and deletes the three call-site copies.

**Tech Stack:** Go, testify; no new dependencies.

**Spec:** `docs/superpowers/specs/2026-07-12-polish-long-tail-design.md`

## Global Constraints

- Config-surface compatibility: the worker middleware name `worker.timeout` (in `ResolveMiddleware` and the middleware's `Name()`) stays valid — only the TYPE renames and drops its timer. A config listing `worker.timeout` must keep working (now meaning "panic shield; timeout owned by the runtime").
- Canonical wildcard error at the chokepoint: `channel must be a literal name, not a pattern` (the ws/sse plugins' text). The hostapi sites emitted `VALIDATION_ERROR: channel must be a literal name, not a pattern` — since hostapi's dispatch wraps connmgr errors, verify what the guest sees and update any test asserting the exact old prefix.
- `matchConnections` is used ONLY by `Manager.Send` and `Manager.SendSSE` (verified 2026-07-12) — the guard on those two methods covers every wildcard path.
- `JobRun` gains `SkipReason string` (values `"lock"` and `"overlap"`) — additive; the `Skipped` comment updates. Any `/jobHistory` JSON consumers see a new field only.
- Engine: `*api.TimeoutError` is public API (`pkg/api`); its `Error()` may gain a Duration==0 branch but the type shape is unchanged.
- Behavior changes for CHANGELOG (Task 5): #279 (enforcement point moved — same outcome for all current callers), #284 (history now records overlap skips), #285 (single timeout layer), #287 (Shutdown bounded by ctx).
- Local gate per task: `gofmt -l .` prints nothing, `go vet ./...`, task tests green; `-race` for connmgr/worker/devmode test runs. Conventional commits with trailer `Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>`.
- PR #314 may merge mid-flight; Task 6 rebases onto latest main (expected overlap only CHANGELOG — entry union).

## Worktree setup (before Task 1)

```bash
cd /Users/marten/GolandProjects/noda
git fetch origin main
git worktree add .worktrees/polish-long-tail -b feat/polish-long-tail origin/main
cd .worktrees/polish-long-tail
mkdir -p docs/superpowers/specs docs/superpowers/plans
cp ../../docs/superpowers/specs/2026-07-12-polish-long-tail-design.md docs/superpowers/specs/
cp ../../docs/superpowers/plans/2026-07-12-polish-long-tail.md docs/superpowers/plans/
git add -f docs/superpowers/specs/2026-07-12-polish-long-tail-design.md docs/superpowers/plans/2026-07-12-polish-long-tail.md
git commit -m "docs: spec + plan for polish long tail tranche (final 9 issues)"
```

---

### Task 1: Engine — aborted-branch test (#272) + honest sub-workflow timeout duration (#273)

**Files:**
- Modify: `internal/engine/executor.go:253-259` (the drain-check result block)
- Modify: `pkg/api/` (the file defining `TimeoutError` — locate via `grep -rn "type TimeoutError" pkg/api/`) — `Error()` gains a Duration==0 branch
- Test: `internal/engine/executor_test.go` (mirror `TestExecuteGraph_Timeout_ReturnsTimeoutError` at :499)

**Interfaces:**
- Produces: no API change; `TimeoutError.Duration` now carries the parent-derived budget when `graph.Timeout == 0`.

- [ ] **Step 1: Write the failing tests**

Mirror the graph/plugin construction of `TestExecuteGraph_Timeout_ReturnsTimeoutError` (executor_test.go:499) EXACTLY — same helpers, same blocking-node setup — for two new tests:

```go
// #272: a non-deadline cancellation must surface as the wrapped "aborted"
// error, not a TimeoutError and not success.
func TestExecuteGraph_ParentCancel_ReturnsAbortedError(t *testing.T) {
	// same setup as TestExecuteGraph_Timeout_ReturnsTimeoutError, but:
	ctx, cancel := context.WithCancel(context.Background())
	go func() { time.Sleep(50 * time.Millisecond); cancel() }()
	// ... ExecuteGraph(ctx, ...) with a graph whose node blocks longer ...
	require.Error(t, gerr)
	var toErr *api.TimeoutError
	require.False(t, errors.As(gerr, &toErr), "plain cancel must not map to TimeoutError")
	assert.Contains(t, gerr.Error(), "aborted")
	assert.ErrorIs(t, gerr, context.Canceled)
}

// #273: a parent deadline propagating into a graph with no own timeout must
// report the budget the child actually had, not "after 0s".
func TestExecuteGraph_InheritedDeadline_ReportsBudget(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	// ... graph with Timeout: 0 and a node blocking > 100ms ...
	var toErr *api.TimeoutError
	require.ErrorAs(t, gerr, &toErr)
	assert.Greater(t, toErr.Duration, time.Duration(0), "must carry the inherited budget")
	assert.LessOrEqual(t, toErr.Duration, 150*time.Millisecond, "budget ≈ the parent's 100ms deadline")
	assert.NotContains(t, gerr.Error(), "after 0s")
}
```

(The comments marked `...` are where you copy the sibling test's construction verbatim; the plan cannot reproduce the harness here — the sibling test at :499 is the single source.)

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/engine/ -run 'TestExecuteGraph_ParentCancel|TestExecuteGraph_InheritedDeadline' -v`
Expected: the #273 test FAILS (`Duration` is 0 today). The #272 test may PASS already (the branch exists, it's merely untested) — that's fine, it's a coverage pin; verify it exercises the intended branch by temporarily breaking the branch (`resultErr = nil`) and watching it fail, then restore (note the polarity result).

- [ ] **Step 3: Implement #273**

In `executor.go`, replace the deadline branch of the drain check (:255-256):

```go
		if errors.Is(execCtx2.Err(), context.DeadlineExceeded) {
			// graph.Timeout == 0 means the deadline was inherited from the
			// parent (sub-workflow case): report the budget the child
			// actually had instead of "after 0s" (#273).
			budget := graph.Timeout
			if budget == 0 {
				if dl, ok := execCtx2.Deadline(); ok {
					budget = dl.Sub(startTime)
				}
			}
			resultErr = &api.TimeoutError{Duration: budget, Operation: "workflow " + graph.WorkflowID}
		} else {
```

(`startTime` already exists in scope — it feeds `duration := time.Since(startTime)`. Confirm `execCtx2` is the context carrying the deadline; if the deadline lives on the original `ctx`, use that one — check how `execCtx2` is derived.)

In `pkg/api` `TimeoutError.Error()`: if the current format always prints the duration, add the zero branch:

```go
func (e *TimeoutError) Error() string {
	if e.Duration <= 0 {
		return fmt.Sprintf("timeout: %s", e.Operation)
	}
	return fmt.Sprintf("timeout after %s: %s", e.Duration, e.Operation)
}
```

(Match the EXISTING format string exactly — read the current implementation first and only add the zero branch; other tests assert on the message.)

- [ ] **Step 4: Run to verify pass**

Run: `go test ./internal/engine/ ./pkg/api/... -count=1`
Expected: PASS, including all pre-existing timeout-message assertions.

- [ ] **Step 5: Gate and commit**

```bash
gofmt -l . && go vet ./internal/engine/ ./pkg/api/...
git add internal/engine/ pkg/api/
git commit -m "fix(engine): inherited-deadline timeouts report the real budget; pin aborted-branch coverage (#272, #273)"
```

---

### Task 2: Server raw_body alias (#275) + scaffold/generate cosmetics (#276, #277)

**Files:**
- Modify: `internal/server/trigger.go:45-48` (raw_body block)
- Modify: `internal/generate/crud.go:127, 413` (drop `columns` param)
- Modify: `cmd/noda/init.go:67-68` (sort conflicts)
- Modify: `internal/mcp/tools.go:113` area (tool description)
- Test: `internal/server/trigger_test.go` (or the file testing buildTriggerInput — find where `raw_body` is tested and extend); `cmd/noda`'s init conflict test if one exists (extend), else add the sort assertion where conflicts are tested

**Interfaces:** none produced.

- [ ] **Step 1: raw_body mirror (#275)**

`trigger.go:46-48` becomes:

```go
	if rawBody, ok := triggerConfig["raw_body"].(bool); ok && rawBody {
		rawCtx["raw_body"] = string(c.Body())
		// Mirror onto the request.* alias so {{ request.raw_body }} and
		// {{ raw_body }} agree (#275).
		if req, ok := rawCtx["request"].(map[string]any); ok {
			req["raw_body"] = rawCtx["raw_body"]
		}
	}
```

Test (extend the existing raw_body test — find it via `grep -rn "raw_body" internal/server/*_test.go`): assert `rawCtx["request"].(map[string]any)["raw_body"]` equals the top-level value when the flag is on, and that the key is ABSENT from the request map when the flag is off. If `docs/02-config` enumerates the `request.*` alias fields (grep `request.` in docs/02-config/), add `raw_body` to that list with a note it appears only when the trigger sets `raw_body: true`.

- [ ] **Step 2: Drop the unused param (#276)**

`crud.go:413`: `func generateListWorkflow(table string, columns []colInfo, opts CRUDOptions)` → `func generateListWorkflow(table string, opts CRUDOptions)`; caller at :127 updated. Run `go build ./internal/generate/` — if any other caller exists the compiler finds it.

- [ ] **Step 3: Sort conflicts + MCP description (#277)**

`cmd/noda/init.go` — before the error at :68:

```go
		if len(conflicts) > 0 {
			sort.Strings(conflicts)
			return fmt.Errorf("refusing to overwrite existing files (use --force): %s", strings.Join(conflicts, ", "))
		}
```

(add the `sort` import). `internal/mcp/tools.go` `noda_scaffold_project` description: append the sentence `Fails if the target path already contains files (no overwrite).` to the existing description string (read it first; keep its style).

- [ ] **Step 4: Verify**

Run: `go test ./internal/server/ ./internal/generate/ ./cmd/noda/ ./internal/mcp/ -count=1`
Expected: PASS. If an existing init-conflict test asserts unsorted order, fix its expectation (that IS the change); if none asserts order, add a two-file non-lexical-creation-order case asserting sorted output.

- [ ] **Step 5: Gate and commit**

```bash
gofmt -l . && go vet ./...
git add internal/server/ internal/generate/crud.go cmd/noda/init.go internal/mcp/tools.go docs/
git commit -m "fix(server): request.raw_body alias; chore(scaffold): sorted conflicts, MCP desc, drop unused param (#275, #276, #277)"
```

---

### Task 3: Connmgr wildcard chokepoint (#279)

**Files:**
- Modify: `internal/connmgr/manager.go:147-150, 168-171` (Send/SendSSE entry guards)
- Modify: `plugins/core/ws/send.go:57-59` (remove guard)
- Modify: `plugins/core/sse/send.go:57-59` (remove guard)
- Modify: `internal/wasm/hostapi.go:341-343, 351-353` (remove both guards)
- Test: `internal/connmgr/manager_test.go` (new chokepoint test); existing ws/sse/hostapi wildcard tests updated only if they assert the exact old error prefix

**Interfaces:**
- Produces: `Manager.Send`/`Manager.SendSSE` return `fmt.Errorf("channel must be a literal name, not a pattern")` for any channel containing `*`.

- [ ] **Step 1: Write the failing chokepoint test**

```go
// #279: the wildcard guard lives at the chokepoint, not the callers — a
// future caller of Send/SendSSE cannot reopen the wildcard-send hole.
func TestManager_SendRejectsWildcardChannels(t *testing.T) {
	m := NewManager(...) // mirror the construction the file's other tests use
	for _, ch := range []string{"*", "user.*", "*.events"} {
		err := m.Send(context.Background(), ch, "x")
		require.Error(t, err, "Send(%q)", ch)
		assert.Contains(t, err.Error(), "literal name")
		err = m.SendSSE(context.Background(), ch, "ev", "x", "")
		require.Error(t, err, "SendSSE(%q)", ch)
		assert.Contains(t, err.Error(), "literal name")
	}
}
```

Run: `go test ./internal/connmgr/ -run TestManager_SendRejects -v` → FAIL (wildcards currently match connections / return nil).

- [ ] **Step 2: Implement**

At the top of BOTH `Manager.Send` (manager.go:147) and `Manager.SendSSE` (:168):

```go
	if strings.Contains(channel, "*") {
		return fmt.Errorf("channel must be a literal name, not a pattern")
	}
```

(add imports as needed). Then REMOVE the three call-site guards:
- `plugins/core/ws/send.go:57-59` (the `strings.Contains(channel, "*")` block; drop the now-unused `strings` import if nothing else uses it)
- `plugins/core/sse/send.go:57-59` (same)
- `internal/wasm/hostapi.go:341-343` AND `:351-353` (the two `VALIDATION_ERROR:` blocks in dispatchConnection)

hostapi note: the old guest-visible error was `VALIDATION_ERROR: channel must be a literal name, not a pattern`; post-change the guest sees the connmgr error passed through `svc.Send(...)`'s return — check how dispatchConnection wraps/classifies that error (`classifyError` maps by prefix) and update any wasm test asserting the exact old prefix. If the classification changes from VALIDATION_ERROR to INTERNAL_ERROR for guests, restore the prefix AT THE HOSTAPI WRAP (wrap the returned error: `fmt.Errorf("VALIDATION_ERROR: %w", err)` on the send paths) rather than reinstating a duplicate guard — the guard logic stays single-sourced, only the guest-facing classification is preserved.

- [ ] **Step 3: Verify**

Run: `go test ./internal/connmgr/ -race && go test ./plugins/core/ws/ ./plugins/core/sse/ ./internal/wasm/ -count=1`
Expected: PASS — existing call-site wildcard tests now pass THROUGH the chokepoint (they call the plugin which calls Manager); if any fail on exact error text, adjust per the hostapi note above.

- [ ] **Step 4: Gate and commit**

```bash
gofmt -l . && go vet ./...
git add internal/connmgr/ plugins/core/ws/send.go plugins/core/sse/send.go internal/wasm/hostapi.go
git commit -m "refactor(connmgr): wildcard-send guard at the Manager.Send/SendSSE chokepoint (#279)"
```

---

### Task 4: Scheduler overlap-skip history (#284) + worker timeout consolidation (#285)

**Files:**
- Modify: `internal/scheduler/runtime.go:42-51` (JobRun), `:226-232` (overlap site), `:315-321` (lock-skip site gains SkipReason)
- Modify: `internal/worker/middleware.go:68-100, 138-160` (type rename + timer removal + chain construction)
- Test: `internal/scheduler/` (flip the no-entry overlap test; find it via `grep -rn "overlap" internal/scheduler/*_test.go`); `internal/worker/` (existing timeout + panic tests must pass through the single layer)

**Interfaces:**
- Produces: `JobRun.SkipReason string` (`"lock"` | `"overlap"`); worker middleware type `PanicShieldMiddleware` (config name `worker.timeout` still resolves to it).

- [ ] **Step 1: Scheduler failing test**

Find the existing same-instance overlap test (asserts NO new history entry after a skipped overlapping run). Flip it:

```go
	// #284: the overlap skip now records history like the lock skip does.
	history := r.History(sc.ID) // use the actual accessor the test already uses
	require.Len(t, history, 2)  // the running entry + the skip (adjust to the test's shape)
	skip := history[len(history)-1]
	assert.True(t, skip.Skipped)
	assert.Equal(t, "overlap", skip.SkipReason)
```

Run it → FAIL (no entry recorded today).

- [ ] **Step 2: Implement #284**

`JobRun` (:50): `Skipped bool // true if the run was skipped (see SkipReason)` plus new field `SkipReason string // "lock" (distributed lock not acquired) or "overlap" (previous same-instance run still active)`.

Overlap site (:226-231) — record before returning:

```go
		if !guard.CompareAndSwap(false, true) {
			r.logger.Warn("scheduler: skipping overlapping run", "schedule_id", sc.ID, "cron", sc.Cron)
			r.recordRun(JobRun{
				ScheduleID: sc.ID,
				StartedAt:  time.Now(),
				Skipped:    true,
				SkipReason: "overlap",
			})
			return
		}
```

(TraceID: the overlap site has no traceID yet — either generate one (`uuid.New().String()`) for parity with other records or leave empty; check what `/jobHistory` renders and pick the one that doesn't produce a confusing blank; note the choice.) Lock-skip site (:315-321): add `SkipReason: "lock"`.

- [ ] **Step 3: Worker failing check (#285)**

Grep first: `grep -rn "MessageContext{" internal/worker/ | grep -v _test` and `grep -rn "\.Timeout" internal/worker/*.go | grep -v _test` — confirm nothing feeds a per-message timeout different from `w.Timeout` through `MessageContext.Timeout` (runtime.go:409 constructs it; see what Timeout value it carries). If something DOES feed a different value, STOP and report — the consolidation premise breaks and the controller must decide.

- [ ] **Step 4: Implement #285**

`middleware.go`: rename `TimeoutMiddleware` → `PanicShieldMiddleware`; delete the `Timeout` field and the `timeout` resolution + `context.WithTimeout` block in `Wrap` (the child goroutine + panic conversion + `select { case err := <-done: ...; case <-ctx.Done(): ... }` structure stays — read the full current Wrap body and remove ONLY the timer):

```go
// PanicShieldMiddleware runs the handler in a child goroutine and converts
// panics to errors (recover() cannot cross goroutines, so the outer
// RecoverMiddleware can't catch them). It no longer applies its own
// timeout: processMessage's context owns the per-message deadline (#285).
type PanicShieldMiddleware struct{}

func (m *PanicShieldMiddleware) Name() string { return "worker.timeout" } // config-name compat
```

`DefaultMiddleware` (:139-145) and `ResolveMiddleware` (:148-160): `&TimeoutMiddleware{Timeout: timeout}` → `&PanicShieldMiddleware{}`; if the `timeout time.Duration` parameter of either function becomes unused, remove it AND update their callers (compiler-guided). KEEP the `case "worker.timeout":` string. Check whether `MessageContext.Timeout` has any remaining reader after this; if the field becomes fully dead, remove it and its writers (compiler-guided) — if something else reads it, leave it and note why.

- [ ] **Step 5: Verify**

Run: `go test ./internal/scheduler/ ./internal/worker/ -race -count=1`
Expected: PASS — worker timeout tests still pass (timeouts now fire via the outer processMessage ctx which the shield's `ctx.Done()` select honors); panic tests unchanged.

- [ ] **Step 6: Gate and commit**

```bash
gofmt -l . && go vet ./...
git add internal/scheduler/ internal/worker/
git commit -m "fix(scheduler): record overlap skips in job history; refactor(worker): single timeout layer, panic shield keeps config name (#284, #285)"
```

---

### Task 5: Devmode ctx-bounded Shutdown (#287) + CHANGELOG

**Files:**
- Modify: `internal/devmode/reload.go:56-71` (Shutdown signature + barrier)
- Modify: `internal/lifecycle/adapters.go` (the `watcherComponent.Stop(ctx)` call site — find via `grep -n "Shutdown()" internal/lifecycle/adapters.go`)
- Modify: `CHANGELOG.md` ([Unreleased])
- Test: `internal/devmode/` (both polarities)

**Interfaces:**
- Produces: `func (r *Reloader) Shutdown(ctx context.Context)` (signature change; lifecycle adapter is the only caller — verify via grep and update all).

- [ ] **Step 1: Write the failing test**

```go
// #287: Shutdown must respect the ctx budget instead of draining reloadMu
// unboundedly when a reload is stuck.
func TestReloaderShutdown_BoundedByContext(t *testing.T) {
	r := NewReloader(...) // mirror the file's existing test construction
	release := make(chan struct{})
	// occupy reloadMu via a reload handler that blocks until released —
	// use whatever hook the existing tests use to make HandleChange slow;
	// if none exists, lock r.reloadMu directly from the test (same package).
	r.reloadMu.Lock()
	go func() { <-release; r.reloadMu.Unlock() }()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	start := time.Now()
	r.Shutdown(ctx)
	assert.Less(t, time.Since(start), time.Second, "Shutdown must return on ctx expiry")
	close(release)
}

func TestReloaderShutdown_WaitsForBarrierWhenUnpressured(t *testing.T) {
	r := NewReloader(...)
	r.reloadMu.Lock()
	go func() { time.Sleep(30 * time.Millisecond); r.reloadMu.Unlock() }()
	start := time.Now()
	r.Shutdown(context.Background()) // no deadline: waits for the barrier
	assert.GreaterOrEqual(t, time.Since(start), 25*time.Millisecond)
}
```

Run: `go test ./internal/devmode/ -run TestReloaderShutdown -v` → compile FAIL (Shutdown takes no ctx).

- [ ] **Step 2: Implement**

```go
// Shutdown flips the stopping flag and awaits the in-flight-reload barrier,
// bounded by ctx. On ctx expiry it returns without the barrier: the
// shuttingDown flag makes the in-flight reload's post-lock re-check bail
// before firing onReload, so nothing fires into the closing system (#287).
func (r *Reloader) Shutdown(ctx context.Context) {
	r.shuttingDown.Store(true)
	acquired := make(chan struct{})
	go func() {
		r.reloadMu.Lock()
		r.reloadMu.Unlock() //nolint:staticcheck // intentional barrier: drain to await in-flight reload
		close(acquired)
	}()
	select {
	case <-acquired:
	case <-ctx.Done():
		r.logger.Warn("devmode: shutdown proceeding without reload barrier (in-flight reload still running)")
	}
}
```

(Use the Reloader's actual logger field name; if it has none, use `slog.Warn`.) Update the doc comment above (:56-66) to describe the bounded behavior. `internal/lifecycle/adapters.go`: `reloader.Shutdown()` → `reloader.Shutdown(ctx)`. Grep for any other `Shutdown()` caller and update.

- [ ] **Step 3: CHANGELOG** ([Unreleased], fold into existing subsections)

- **Fixed:** sub-workflow timeouts inherited from a parent deadline report the child's actual budget instead of "timeout after 0s" (#273)
- **Fixed:** `{{ request.raw_body }}` now mirrors `{{ raw_body }}` on the request alias (#275)
- **Fixed:** dev-mode shutdown no longer waits unboundedly for a stuck in-flight reload — bounded by the lifecycle stop budget (#287)
- **Changed:** wildcard-send rejection is enforced at the connection manager (`Send`/`SendSSE`) instead of per caller — same behavior for all current callers, future callers covered (#279)
- **Changed:** `/jobHistory` records same-instance overlap skips (`skipped` with a new `skip_reason` distinguishing them from lock skips) (#284)
- **Changed:** the worker's per-message timeout is applied once (runtime-owned); the `worker.timeout` middleware keeps its config name but is now the panic-to-error shield only (#285)

(Adjust the `skip_reason` casing to however JobRun serializes.)

- [ ] **Step 4: Verify, gate, commit**

```bash
go test ./internal/devmode/ -race -count=1 && go test ./internal/lifecycle/ -count=1
gofmt -l . && go vet ./...
git add internal/devmode/ internal/lifecycle/ CHANGELOG.md
git commit -m "fix(devmode): ctx-bounded Reloader.Shutdown; CHANGELOG for polish tranche (#287)"
```

---

### Task 6: Rebase, full verification, review, PR

- [ ] **Step 1: Rebase** (`git fetch origin main && git rebase origin/main`; CHANGELOG conflicts → entry union)

- [ ] **Step 2: Full verification**

```bash
go build ./... && go vet ./... && gofmt -l .
go test ./...
go test ./internal/connmgr/ ./internal/worker/ ./internal/devmode/ -race -count=1
```

Expected: all green, gofmt output empty.

- [ ] **Step 3: Whole-branch review**, then PR:

```bash
git push -u origin feat/polish-long-tail
gh pr create --title "chore: polish long tail — the final nine backlog issues" \
  --body "$(cat <<'EOF'
Tranche 5 of the open-issue backlog (spec + plan on branch under docs/superpowers/). Closing these EMPTIES the issue backlog.

- engine: inherited-deadline sub-workflow timeouts report the child's real budget, not "after 0s"; the non-deadline aborted branch is now directly tested (#272, #273)
- server: `{{ request.raw_body }}` mirrors `{{ raw_body }}` (#275)
- scaffold/generate: unused param dropped, deterministic sorted conflict lists, MCP scaffold description mentions refuse-on-conflict (#276, #277)
- connmgr: wildcard-send guard enforced at the Manager.Send/SendSSE chokepoint, three call-site copies removed (#279)
- scheduler: same-instance overlap skips recorded in /jobHistory with a skip_reason distinguishing them from lock skips (#284)
- worker: single per-message timeout layer (runtime-owned); `worker.timeout` middleware keeps its config name as the panic-to-error shield (#285)
- devmode: Reloader.Shutdown bounded by the lifecycle stop budget (#287)

Closes #272
Closes #273
Closes #275
Closes #276
Closes #277
Closes #279
Closes #284
Closes #285
Closes #287

🤖 Generated with [Claude Code](https://claude.com/claude-code)
EOF
)"
```

Wait for the required CI checks (plus the new wasm-guests job staying green).
