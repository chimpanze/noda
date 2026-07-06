# Worker/Scheduler Hardening (Tranche E1) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fix the 4 worker/scheduler findings from `REVIEW-FINDINGS-2026-07-05.md` (worker-sched-1/2/4/5, all Medium) — per-worker timeout honored, reaper stops over-claiming, sub-minute schedules stop colliding, scheduled jobs stop self-overlapping.

**Architecture:** Targeted fixes in `internal/worker` (middleware + reaper) and `internal/scheduler` (lock key + overlap guard). No public API break.

**Tech Stack:** Go (go1.25), go-redis/v9, robfig/cron/v3, alicebob/miniredis (tests).

## Global Constraints

- Go module floor: **go1.25**.
- Per-worker `timeout` config must be honored by the shared middleware chain (via `MessageContext`), consistent with the per-message context timeout and the `min_idle` derivation.
- Reaper must not hold more claimed messages than it is actively processing (`Count == concurrency`).
- Scheduler distributed-lock key must be distinct per sub-minute fire (second granularity).
- Overlap policy: **skip if still running** (per-schedule, same-instance).
- Touched packages' tests run under `-race`.
- Pre-push gate: `gofmt -l .`, `go vet ./...`, `golangci-lint run`, `go test -race ./internal/worker/... ./internal/scheduler/...`.

**Worktree:** `.worktrees/worker-scheduler-hardening`, branch `feat/worker-scheduler-hardening` off `main`. Spec + this plan force-added.

## File map

- `internal/worker/middleware.go` — `MessageContext.Timeout` field; `TimeoutMiddleware.Wrap` prefers it (Task 1).
- `internal/worker/runtime.go` — set `MessageContext.Timeout` at the build site (~405); reaper `Count` (~639) (Tasks 1, 2).
- `internal/scheduler/runtime.go` — `scheduleLockKey` helper + second granularity; `running` guard (Tasks 3, 4).
- Tests: `internal/worker/runtime_test.go`, `internal/scheduler/*_test.go`.

---

### Task 1: Per-worker timeout via MessageContext (worker-sched-1)

**Files:**
- Modify: `internal/worker/middleware.go` (`MessageContext`, `TimeoutMiddleware.Wrap`), `internal/worker/runtime.go` (build site ~405-408)
- Test: `internal/worker/runtime_test.go` (or middleware_test.go)

**Interfaces:**
- Produces: `MessageContext.Timeout time.Duration`.

- [ ] **Step 1: Write the failing test**

```go
// A per-message Timeout larger than the middleware's construction-time timeout
// must win — a handler that runs longer than m.Timeout but within msgCtx.Timeout
// is NOT cut off.
func TestTimeoutMiddleware_UsesPerMessageTimeout(t *testing.T) {
	m := &TimeoutMiddleware{Timeout: 20 * time.Millisecond} // shared/default (small)
	msgCtx := &MessageContext{WorkerID: "w", Logger: slog.Default(), Timeout: 200 * time.Millisecond}
	handler := m.Wrap(func(ctx context.Context) error {
		select {
		case <-time.After(80 * time.Millisecond): // > 20ms, < 200ms
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}, msgCtx)
	require.NoError(t, handler(context.Background()), "per-message timeout (200ms) must govern, not the 20ms default")
}

func TestTimeoutMiddleware_FallsBackWhenNoPerMessageTimeout(t *testing.T) {
	m := &TimeoutMiddleware{Timeout: 20 * time.Millisecond}
	msgCtx := &MessageContext{WorkerID: "w", Logger: slog.Default()} // Timeout == 0
	handler := m.Wrap(func(ctx context.Context) error {
		<-ctx.Done()
		return ctx.Err()
	}, msgCtx)
	require.Error(t, handler(context.Background()), "with no per-message timeout, the 20ms default still applies")
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/worker/ -run TestTimeoutMiddleware_UsesPerMessageTimeout -race`
Expected: FAIL — `MessageContext` has no `Timeout` field (compile error), and once added, `Wrap` still uses `m.Timeout` (20ms) so the 80ms handler is cut off.

- [ ] **Step 3: Add the field and prefer it**

In `middleware.go`, add to `MessageContext`:

```go
type MessageContext struct {
	WorkerID  string
	MessageID string
	TraceID   string
	Topic     string
	Group     string
	Timeout   time.Duration // per-worker processing timeout; 0 = use middleware default
	Logger    *slog.Logger
}
```

In `TimeoutMiddleware.Wrap`, replace the timeout resolution:

```go
		timeout := msgCtx.Timeout
		if timeout == 0 {
			timeout = m.Timeout
		}
		if timeout == 0 {
			timeout = 30 * time.Second
		}
```

In `runtime.go` at the `MessageContext` build site (~405-408), set the per-worker timeout (compute it the same way `processMessage` does):

```go
	msgTimeout := w.Timeout
	if msgTimeout == 0 {
		msgTimeout = defaultMessageTimeout
	}
	for i := len(r.middleware) - 1; i >= 0; i-- {
		handler = r.middleware[i].Wrap(handler, &MessageContext{
			WorkerID: w.ID, MessageID: msg.ID, TraceID: traceID,
			Topic: w.Topic, Group: w.Group, Timeout: msgTimeout, Logger: r.logger,
		})
	}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/worker/ -run TestTimeoutMiddleware -race`
Expected: PASS.

- [ ] **Step 5: Full worker suite**

Run: `go test ./internal/worker/... -race`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/worker/middleware.go internal/worker/runtime.go internal/worker/runtime_test.go
git commit -m "fix(worker): honor per-worker timeout in the shared middleware chain (worker-sched-1)"
```

---

### Task 2: Reaper claims only what it processes (worker-sched-2)

**Files:**
- Modify: `internal/worker/runtime.go` (`reapOnce` XAutoClaim `Count`, line ~639)
- Test: `internal/worker/runtime_test.go`

- [ ] **Step 1: Write the failing/regression test** — with the smaller claim count, the reaper still drains a multi-message backlog.

```go
// With Count == concurrency the reaper pages through the pending set; assert a
// backlog of several idle messages is fully reclaimed and processed.
func TestReapOnce_DrainsBacklogAtConcurrency(t *testing.T) {
	client, svcReg, nodeReg, mr := newTestSetup(t)
	// Build a worker with concurrency 2 and a topic/group; publish N messages,
	// register them pending under a dead consumer, advance past min_idle, then
	// reapOnce and assert all N are processed/acked. (Mirror the existing reaper
	// test setup around runtime_test.go:861-905 — same topic/group/min_idle pattern.)
	// ... setup: w := WorkerConfig{..., Concurrency: 2, Retry: RetryConfig{MinIdle: ...}}
	// ... publish 4 messages, XReadGroup as a dead consumer to make them pending,
	// ... mr.FastForward(past min_idle)
	r := &Runtime{ /* … as existing reaper tests construct … */ }
	require.NoError(t, r.reapOnce(context.Background(), w, client))
	// assert all 4 acked (XPending count == 0), i.e. the smaller Count still drains.
}
```

(Follow the exact construction used by the existing reaper tests at `runtime_test.go:861-905` — reuse `newTestSetup`, the same `WorkerConfig`/`Retry`/`min_idle` and `mr.FastForward` pattern. The point of THIS test is that reducing `Count` to `concurrency` doesn't break multi-message draining.)

- [ ] **Step 2: Run test to verify it passes against current code, then make the change**

Run: `go test ./internal/worker/ -run TestReapOnce_DrainsBacklogAtConcurrency -race`
Expected: PASS against current `Count: 16` (draining works either way). This is a regression guard for the `Count` reduction; the over-claim/steal window itself is timing-dependent and not deterministically unit-reproducible, so we guard the draining behavior.

- [ ] **Step 3: Reduce the claim count to concurrency**

In `reapOnce` (`runtime.go:639`), change:

```go
			Count:    int64(concurrency),
```

(`concurrency` is already computed as `max(w.Concurrency, 1)` earlier in `reapOnce`, so `Count >= 1`.)

- [ ] **Step 4: Run test to verify it still passes**

Run: `go test ./internal/worker/ -run TestReapOnce_DrainsBacklogAtConcurrency -race`
Expected: PASS (the reaper still drains the backlog, now claiming `concurrency` per page).

- [ ] **Step 5: Full worker suite**

Run: `go test ./internal/worker/... -race`
Expected: PASS (existing reaper tests at 861-938 still green).

- [ ] **Step 6: Commit**

```bash
git add internal/worker/runtime.go internal/worker/runtime_test.go
git commit -m "fix(worker): reaper claims Count==concurrency to close the steal window (worker-sched-2)"
```

---

### Task 3: Second-granularity scheduler lock key (worker-sched-4)

**Files:**
- Modify: `internal/scheduler/runtime.go` (`scheduleLockKey` helper + `runJob` ~229)
- Test: `internal/scheduler/runtime_test.go` (create if absent)

**Interfaces:**
- Produces: `scheduleLockKey(id string, t time.Time) string`.

- [ ] **Step 1: Write the failing test**

```go
func TestScheduleLockKey_DistinctPerSubMinuteFire(t *testing.T) {
	base := time.Date(2026, 7, 6, 12, 0, 0, 0, time.UTC)
	k0 := scheduleLockKey("s1", base)
	k30 := scheduleLockKey("s1", base.Add(30*time.Second)) // second fire in the same minute
	require.NotEqual(t, k0, k30, "sub-minute fires must get distinct lock keys")
	// same second -> same key (not cron-expressible to fire twice in one second)
	require.Equal(t, k0, scheduleLockKey("s1", base.Add(500*time.Millisecond)))
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/scheduler/ -run TestScheduleLockKey_DistinctPerSubMinuteFire`
Expected: FAIL — `scheduleLockKey` undefined; and the current inline minute-truncation would make `k0 == k30`.

- [ ] **Step 3: Extract the helper with second granularity and use it**

In `runtime.go`, add:

```go
// scheduleLockKey builds the distributed-lock key for a single fire of a
// schedule. The time is truncated to the second (not the minute) so that
// sub-minute schedules (cron WithSeconds allows a 1s minimum interval) get a
// distinct key per fire rather than colliding within the minute.
func scheduleLockKey(id string, t time.Time) string {
	return fmt.Sprintf("noda:schedule:%s:%d", id, t.Truncate(time.Second).Unix())
}
```

Replace the inline construction in `runJob` (line ~229):

```go
		lockKey := scheduleLockKey(sc.ID, now)
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/scheduler/ -run TestScheduleLockKey_DistinctPerSubMinuteFire`
Expected: PASS.

- [ ] **Step 5: Full scheduler suite**

Run: `go test ./internal/scheduler/... -race`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/scheduler/runtime.go internal/scheduler/runtime_test.go
git commit -m "fix(scheduler): second-granularity distributed-lock key for sub-minute fires (worker-sched-4)"
```

---

### Task 4: Same-instance overlap guard (worker-sched-5)

**Files:**
- Modify: `internal/scheduler/runtime.go` (`Runtime.running`, `NewRuntime`, `runJob`)
- Test: `internal/scheduler/runtime_test.go`

**Interfaces:**
- Produces: `Runtime.running map[string]*atomic.Bool` (keyed by schedule ID; read-only after `NewRuntime`).

- [ ] **Step 1: Write the failing test**

```go
func TestRunJob_SkipsOverlappingRun(t *testing.T) {
	sc := ScheduleConfig{ID: "s1", Cron: "* * * * * *", WorkflowID: "wf"}
	rt := NewRuntime([]ScheduleConfig{sc}, nil, nil, nil, nil, nil, nil, nil, nil)
	// Simulate a run already in progress on this instance.
	rt.running["s1"].Store(true)

	before := len(rt.jobHistory())
	rt.runJob(sc) // must skip immediately: no lock, no workflow, no history entry
	require.Equal(t, before, len(rt.jobHistory()), "overlapping run must be skipped (no new history)")
	require.True(t, rt.running["s1"].Load(), "the guard owned by the in-progress run stays set")
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/scheduler/ -run TestRunJob_SkipsOverlappingRun -race`
Expected: FAIL — `rt.running` is nil (no field); once added, `runJob` doesn't check it and proceeds (records history / panics without services).

- [ ] **Step 3: Add the running map and the guard**

Add `"sync/atomic"` import. Add to `Runtime` struct:

```go
	running map[string]*atomic.Bool // per-schedule same-instance overlap guard (keyed by schedule ID)
```

In `NewRuntime`, build it (one entry per schedule) in the returned struct literal:

```go
	running := make(map[string]*atomic.Bool, len(schedules))
	for _, sc := range schedules {
		running[sc.ID] = &atomic.Bool{}
	}
	return &Runtime{
		// … existing fields …
		running: running,
	}
```

At the very top of `runJob(sc)` (before `start := time.Now()` and the timeout/lock work):

```go
	if guard := r.running[sc.ID]; guard != nil {
		if !guard.CompareAndSwap(false, true) {
			r.logger.Warn("scheduler: skipping overlapping run", "schedule_id", sc.ID, "cron", sc.Cron)
			return
		}
		defer guard.Store(false)
	}
```

The guard is acquired before the distributed-lock acquisition and released (via `defer`) after the job completes, so it covers the whole run (lock wait + execution). Cross-instance overlap is still handled by the distributed lock; this closes the same-instance case.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/scheduler/ -run TestRunJob_SkipsOverlappingRun -race`
Expected: PASS.

- [ ] **Step 5: Full scheduler suite**

Run: `go test ./internal/scheduler/... -race`
Expected: PASS (existing e2e tests still green — a single non-overlapping fire acquires and releases the guard normally).

- [ ] **Step 6: Commit**

```bash
git add internal/scheduler/runtime.go internal/scheduler/runtime_test.go
git commit -m "fix(scheduler): skip overlapping same-instance runs (worker-sched-5)"
```

---

### Task 5: CHANGELOG + full gate

**Files:**
- Modify: `CHANGELOG.md`

- [ ] **Step 1: CHANGELOG entry**

Add under `### Fixed`: "Worker/scheduler hardening: a worker's configured `timeout` is honored by the middleware chain (no longer capped by a shared default); the pending-message reaper claims only as many messages as it processes concurrently (closing a duplicate-execution window under contention); sub-minute schedules with distributed locking get a per-second lock key (no longer skip fires within a minute); a scheduled job that runs longer than its interval skips overlapping same-instance runs instead of self-overlapping."

- [ ] **Step 2: Full gate**

Run: `gofmt -l . && go vet ./... && golangci-lint run && go test -race ./internal/worker/... ./internal/scheduler/...`
Expected: clean, all pass. Fix any lint issues introduced by this branch; leave pre-existing/unrelated ones (note them).

- [ ] **Step 3: Commit**

```bash
git add CHANGELOG.md
git commit -m "docs(worker): changelog for worker/scheduler hardening"
```

---

## Self-review notes

- **Spec coverage:** worker-sched-1 → Task 1; worker-sched-2 → Task 2; worker-sched-4 → Task 3; worker-sched-5 → Task 4; changelog/gate → Task 5. All four covered.
- **Type consistency:** `MessageContext.Timeout` (Task 1) set at the build site and read in `TimeoutMiddleware.Wrap`. `scheduleLockKey(id, t)` (Task 3) used in `runJob`. `Runtime.running` (Task 4) built in `NewRuntime`, read in `runJob`.
- **Test-harness notes:** worker tests use `miniredis` via `newTestSetup` and call `reapOnce` directly (runtime_test.go:26, 899). Scheduler tests use `NewRuntime(...)` + `jobHistory()` (e2e_test.go:61). Task 2's test mirrors the existing reaper test (861-905); Task 4's test drives `runJob` on a lock-less/service-less schedule so the skip path returns before touching services.
- **Risk note:** Task 2's fix is a one-line correctness change; the over-claim/steal window is timing-dependent, so its test is a draining-regression guard (the precise `Count == concurrency` is a code invariant). Task 4's guard is acquired/released around the whole `runJob` including the lock wait.
- **Deferred (out of scope):** lifecycle/devmode/registry platform-1/2/3/4/5/6 → tranche E2; worker-sched-3 (Low, non-shipped path) and the worker-sched-6..13 Low long-tail.
