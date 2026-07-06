# Worker/Scheduler Hardening (Tranche E1) — Design

Date: 2026-07-06
Source: `REVIEW-FINDINGS-2026-07-05.md` — worker-sched-1, worker-sched-2, worker-sched-4, worker-sched-5 (4 Medium).
Branch/worktree (planned): `feat/worker-scheduler-hardening` in `.worktrees/worker-scheduler-hardening`, off `main`.

## Why

The review's "worker/scheduler & lifecycle" tranche was split (user decision): the **Redis-distributed worker/scheduler** findings ship here (E1); the **in-process lifecycle/devmode/registry** findings (platform-1/2/3/4/5/6, incl. the High SIGTERM-during-boot) become tranche E2. These four defects cause silent misbehavior of the async runtime: workflows capped below their configured timeout, duplicate execution under reaper contention, sub-minute schedules skipping fires, and scheduled jobs self-overlapping.

**Decisions (user-approved):** split E into E1 (this) + E2; overlap policy is **skip-if-still-running**; one PR for E1.

## Findings in scope

| ID | Sev | Summary |
|---|---|---|
| worker-sched-1 | Med | Shared worker middleware chain uses one construction-time timeout; `TimeoutMiddleware` silently overrides per-worker `timeout` config (min_idle is derived from the per-worker value, so they disagree) |
| worker-sched-2 | Med (concurrency) | Reaper `XAutoClaim` claims a 16-message page but processes at worker concurrency → claimed-but-unprocessed messages exceed `min_idle` and get stolen by another instance → duplicate execution |
| worker-sched-4 | Med | Scheduler distributed-lock key truncated to the minute while `WithSeconds()` is enabled → sub-minute schedules skip all but one fire per minute |
| worker-sched-5 | Med | No same-instance overlap guard — a job slower than its interval self-overlaps even with locking |

## Verified facts

- `internal/worker/middleware.go`: `MessageContext` fields are `WorkerID/MessageID/TraceID/Topic/Group/Logger` (no timeout). `TimeoutMiddleware.Wrap` uses `m.Timeout` (else 30s). `DefaultMiddleware(timeout)`/`ResolveMiddleware(names, timeout)` build the chain with one timeout, called once in `cmd/noda/main.go:980,983` (comment: "All workers share a single middleware chain").
- `internal/worker/runtime.go:405-408`: the shared chain is `Wrap`ped with a fresh `&MessageContext{...}` per message. `processMessage` also applies `context.WithTimeout(procCtx, timeout)` where `timeout := w.Timeout; if 0 { timeout = defaultMessageTimeout }` (`defaultMessageTimeout = 5m`).
- `internal/worker/runtime.go reapOnce`: `XAutoClaim{..., MinIdle: w.Retry.MinIdle, Count: 16}`, then `sem := make(chan struct{}, concurrency)` (`concurrency := max(w.Concurrency, 1)`). The outer `for` loop pages until the XAutoClaim cursor returns `"0"`.
- `internal/scheduler/runtime.go`: `cron.New(cron.WithSeconds(), ...)` (line 108); `runJob` computes `lockKey := fmt.Sprintf("noda:schedule:%s:%d", sc.ID, now.Truncate(time.Minute).Unix())` (line 229); `lockTTL` forced ≥ `timeout+30s`; jobs registered via `r.cron.AddFunc(spec, func(){ r.runJob(sc) })` (line 119); `Runtime` struct holds `schedules []ScheduleConfig`; constructed in `NewRuntime` (line 71).

## Design

### Unit 1 — Per-worker timeout via MessageContext (worker-sched-1)

Add a `Timeout time.Duration` field to `MessageContext` (`middleware.go`). Where the message context is built (`runtime.go:405-408`), set `Timeout:` to the per-worker resolved timeout (compute it the same way `processMessage` does: `w.Timeout`, falling back to `defaultMessageTimeout`). In `TimeoutMiddleware.Wrap`, prefer the per-message value:

```go
	timeout := msgCtx.Timeout
	if timeout == 0 {
		timeout = m.Timeout
	}
	if timeout == 0 {
		timeout = 30 * time.Second
	}
```

The shared middleware chain now enforces each worker's own timeout, matching the per-message `context.WithTimeout` and the `min_idle` derivation. No signature change to the middleware chain (still shared); the per-worker value flows through the per-message `MessageContext`.

### Unit 2 — Reaper claims only what it processes (worker-sched-2)

In `reapOnce`, change the `XAutoClaim` `Count` from the fixed `16` to the worker's processing width: `Count: int64(concurrency)`. The reaper then never holds more claimed messages than it is actively processing, so no claimed message sits past `min_idle` waiting for a semaphore slot (closing the steal-and-duplicate window). The paging `for` loop is unchanged — it still drains the whole pending set across successive claims, each page fully processed before the next `XAutoClaim`. (`concurrency` is already `max(w.Concurrency, 1)`, so `Count ≥ 1`.)

### Unit 3 — Second-granularity lock key (worker-sched-4)

Change the lock-key time bucket from minute to second:

```go
	lockKey := fmt.Sprintf("noda:schedule:%s:%d", sc.ID, now.Truncate(time.Second).Unix())
```

`WithSeconds()` allows a minimum interval of 1 second, so second-truncation gives every distinct fire a distinct lock key; two fires within the same second are not expressible by a cron spec. Minute-truncated collisions (which silently skipped sub-minute fires) are eliminated. The `lockTTL ≥ timeout+30s` behavior is unchanged.

### Unit 4 — Same-instance overlap guard (worker-sched-5)

Add a per-schedule running guard to the scheduler `Runtime`:
- New field `running map[string]*atomic.Bool` (keyed by `sc.ID`), populated in `NewRuntime` with one `&atomic.Bool{}` per configured schedule (map is read-only after construction, so no mutex needed for lookups).
- At the top of `runJob(sc)`, before any work:

```go
	guard := r.running[sc.ID]
	if guard != nil && !guard.CompareAndSwap(false, true) {
		r.logger.Warn("scheduler: skipping overlapping run", "schedule_id", sc.ID, "cron", sc.Cron)
		return
	}
	defer func() { if guard != nil { guard.Store(false) } }()
```

If the previous run on this instance is still active when the next tick fires, the tick is skipped and logged; the schedule self-heals on the next non-overlapping tick. This is the same-instance complement to the distributed lock (which guards across instances). The guard is set before the distributed-lock acquisition and released after the job completes, so it also covers the lock-wait/execution span.

## Testing (per finding)

- **worker-sched-1:** build the middleware chain with a small shared timeout, run a worker whose `MessageContext.Timeout` is larger, and assert `TimeoutMiddleware` uses the per-message (larger) value — a handler that runs longer than the shared timeout but within the per-worker timeout is NOT cut off. (Unit-test `TimeoutMiddleware.Wrap` with a `MessageContext{Timeout: …}`.)
- **worker-sched-2:** assert the reaper's `XAutoClaim` Count equals the worker concurrency (extract the arg or drive `reapOnce` against a fake/mini-Redis and assert no more than `concurrency` messages are claimed per round). At minimum, a unit test that the Count passed equals `concurrency`.
- **worker-sched-4:** two fire times 30s apart within the same minute produce distinct lock keys; two within the same second produce the same key (documented as not cron-expressible). A pure-function test of the key formula.
- **worker-sched-5:** invoke `runJob` for a schedule whose guard is already `true` (simulating an in-progress run) and assert it returns immediately (skips) and logs; a non-overlapping invocation runs. Test via the `running` guard directly.

Gate: `gofmt -l .`, `go vet ./...`, `golangci-lint run`, `go test -race ./internal/worker/... ./internal/scheduler/...`.

## Mechanics

- Worktree `.worktrees/worker-scheduler-hardening`, branch `feat/worker-scheduler-hardening` off `main`.
- Subagent-driven execution per task: implementer → spec-compliance reviewer → code-quality reviewer.
- Spec + plan force-added to the branch.
- CHANGELOG "Fixed" entry.
- At merge: add a "Shipped 2026-07-06" note for these findings to `REVIEW-FINDINGS-2026-07-05.md` (on review PR #262's branch).

## Out of scope

Lifecycle/devmode/registry findings platform-1/2/3/4/5/6 → tranche E2. Other worker/scheduler findings (worker-sched-3 already downgraded to Low non-shipped-path; worker-sched-6..13 Low long-tail) are not in this set.
