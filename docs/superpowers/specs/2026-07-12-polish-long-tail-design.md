# Polish Long Tail — Design

**Date:** 2026-07-12
**Issues:** #272, #273 (engine), #275 (server), #276, #277 (scaffold/generate), #279 (connmgr), #284 (scheduler), #285 (worker), #287 (devmode)
**Branch:** `feat/polish-long-tail`

## Problem

The final nine open issues — all small, all follow-ups deferred from earlier
review tranches. Closing them empties the issue backlog.

- **#272:** `ExecuteGraph`'s drain check has two context-expiry branches:
  deadline → `*api.TimeoutError` (tested) and any other cancellation →
  `workflow %q aborted: %w` (untested).
- **#273:** a parent timeout propagating into a sub-workflow with no own
  `timeout` synthesizes `*api.TimeoutError{Duration: 0}` — "timeout after
  0s" is misleading (504 mapping is correct).
- **#275:** `buildRawRequestContext` puts `raw_body` at top level only;
  `{{ request.raw_body }}` is nil while `{{ raw_body }}` works.
- **#276:** `internal/generate/crud.go generateListWorkflow(table, columns,
  opts)` never uses `columns`; no linter catches it.
- **#277:** `cmd/noda/init.go`'s refuse-on-overwrite conflict list is
  unsorted (MCP handler sorts); the `noda_scaffold_project` MCP tool
  description doesn't mention refuse-on-conflict.
- **#279:** the wildcard-send guard is replicated at three
  `api.ConnectionService.Send`/`SendSSE` call sites; `Manager.Send`/
  `SendSSE` still honor `*` — a future fourth caller reopens the hole.
- **#284:** the same-instance overlap skip in `runJob` only Warn-logs; the
  distributed-lock skip records `JobRun{Skipped: true}` — `/jobHistory`
  under-counts overlaps.
- **#285:** the per-message timeout is applied twice (processMessage outer
  `WithTimeout` + `TimeoutMiddleware`), both resolving to the same value;
  the middleware also carries the panic→error child-goroutine shield.
- **#287:** `Reloader.Shutdown()` drains `reloadMu` unboundedly — a reload
  stuck in `config.ValidateAll` blows the per-component Stop(ctx) budget.

## Decisions (user-approved 2026-07-12)

1. **#285: the outer processMessage `WithTimeout` owns the timeout.**
   `TimeoutMiddleware` drops its own timer, keeps the panic-to-error
   child-goroutine shield, and is renamed to reflect that (e.g.
   `PanicShieldMiddleware`; final name per file conventions). Disposition's
   procCtx-exhaustion logic unchanged.
2. Whole design as presented (engine remaining-deadline for #273; alias
   mirror for #275; drop the param for #276; chokepoint-only guard for
   #279 with call-site copies removed; skip-entry recorded for #284;
   ctx-bounded barrier for #287).

## Design

### 1. Engine (#272, #273) — `internal/engine/executor.go`

- **#272 test:** run a graph whose node blocks; cancel the parent context
  (plain `context.CancelFunc`, not a deadline); assert the returned error
  wraps `context.Canceled` and matches `workflow %q aborted`. Direct mirror
  of the existing deadline-branch test.
- **#273:** where the child's `*api.TimeoutError` is synthesized with
  `graph.Timeout == 0` (executor.go:254 area): capture the effective
  deadline (`ctx.Deadline()`) at synthesis; if present, set
  `Duration: time.Until(deadline)` measured at graph START (capture the
  start-relative budget: `deadline.Sub(startTime)` — the plan pins the
  exact capture point so the reported duration is the budget the child
  actually had, not a near-zero remainder). If no deadline is retrievable,
  the error message omits the duration ("timeout: workflow X") instead of
  printing "after 0s". `*api.TimeoutError`'s type/504 mapping unchanged.
  Test: parent-timeout-into-child asserts a nonzero, plausible Duration.

### 2. Server (#275) — `internal/server/trigger.go`

`buildRawRequestContext` mirrors `raw_body` onto the `request` alias map
(same value, both locations). If `docs/02-config` (or wherever the alias
contract lives) enumerates the `request.*` fields, add `raw_body` there.
Test: unit test asserting `request.raw_body == raw_body` in the built
context.

### 3. Scaffold/generate (#276, #277)

- `internal/generate/crud.go`: drop the `columns []colInfo` parameter from
  `generateListWorkflow`; update its callers. No behavior change; existing
  generate tests must pass unchanged.
- `cmd/noda/init.go`: `sort.Strings` (or slices.Sort) the conflict list
  before printing — deterministic refuse-on-overwrite messages. Test:
  fixture with files created in non-lexical order asserts sorted output
  (or extend an existing conflict test).
- `internal/mcp/tools.go`: `noda_scaffold_project` description gains "fails
  if the target path contains existing files (no overwrite)" phrasing.

### 4. Connmgr chokepoint (#279) — `internal/connmgr/manager.go` + 3 call sites

- `Manager.Send` and `Manager.SendSSE` reject wildcard channels at entry:
  channel containing `*` → one canonical error. The three call sites may
  emit differing texts today — the plan reads all three, picks the
  ws/sse plugins' text as canonical, and updates any test that asserted a
  divergent (e.g. hostapi) variant. Callers that wrap the error keep their
  wrapping.
- The three call-site guards are REMOVED: `plugins/core/ws/send.go:57`,
  `plugins/core/sse/send.go:57`, `internal/wasm/hostapi.go:341` and `:351`
  (both dispatchConnection sites).
- `matchConnections`/`Broadcast`-style internal wildcard use (if any path
  other than Send/SendSSE uses it legitimately) is untouched — the guard
  lands only on the two public methods. The plan verifies `matchConnections`
  has no other callers (the issue states it's used only by these two
  methods; re-verify).
- Tests: existing call-site wildcard tests keep passing (now exercising the
  chokepoint through the caller); one new Manager-level test pins both
  methods rejecting `*` and `prefix.*`.

### 5. Scheduler (#284) — `internal/scheduler/runtime.go`

The same-instance overlap skip in `runJob` records a `JobRun` with
`Skipped: true` and a reason distinguishing it from the lock skip. Field
mechanics: `JobRun.Skipped`'s comment currently says "lock was not
acquired" — the struct gains a `SkipReason string` field (values e.g.
`"lock"` / `"overlap"`), set at both skip sites; the comment updates. The
plan reads `recordRun` and the `/jobHistory` serialization to keep the
addition backward compatible (new field additive in JSON). The existing
test asserting NO new history entry for overlap flips to assert the
recorded skip — an intentional test-behavior change the issue requests.
CHANGELOG notes the history now includes overlap skips.

### 6. Worker (#285) — `internal/worker/middleware.go` + `runtime.go`

- `TimeoutMiddleware` → renamed (e.g. `PanicShieldMiddleware`): drops the
  `Timeout` field and its `WithTimeout`; keeps running `next` in a child
  goroutine converting panics to errors; its select keeps honoring
  ctx.Done() (which now expires via processMessage's outer timeout).
- `processMessage`'s outer `WithTimeout` is the single timeout owner
  (already resolves `w.Timeout` / default).
- Pre-check the plan pins: nothing else feeds a DIFFERENT per-message value
  through `MessageContext.Timeout` — if something does, that semantic must
  move to the outer resolution (grep `MessageContext.Timeout` writers).
- Middleware-chain construction (`middleware.go:143,155`) updates for the
  rename/removed field.
- Existing worker timeout tests must still pass (they now exercise the
  single outer layer); panic-recovery tests unchanged.

### 7. Devmode (#287) — `internal/devmode/reload.go` + `internal/lifecycle/adapters.go`

- `Reloader.Shutdown()` → `Shutdown(ctx context.Context)`: sets
  `shuttingDown`, then awaits the `reloadMu` barrier in a goroutine with
  `select { case <-acquired: case <-ctx.Done(): }`. On ctx expiry: Warn log
  ("shutdown proceeding without reload barrier") and return — the stopping
  flag already makes any in-flight reload's post-lock re-check bail before
  firing `onReload`, so nothing fires into the closing system; the reload
  goroutine finishes and releases on its own.
- `watcherComponent.Stop(ctx)` passes its ctx through.
- Preserve the existing `//nolint:staticcheck` barrier idiom inside the
  goroutine (Lock/Unlock pair).
- Test: a reload handler artificially blocked (hook or slow validate stub),
  `Shutdown` with a ~50ms ctx returns promptly; without ctx pressure it
  still waits for the barrier (both polarities).

## Testing

Unit tests per item in their home packages; `-race` runs for the connmgr,
worker, and devmode packages; full `go test ./...` at the end. CHANGELOG
[Unreleased]: Fixed (#273 misleading duration, #275 alias gap, #287
unbounded shutdown wait), Changed (#279 chokepoint enforcement — behavior
identical for all current callers; #284 jobHistory records overlap skips;
#285 single timeout layer + middleware rename).

## Execution shape

Standing conventions: worktree `.worktrees/polish-long-tail`, branch
`feat/polish-long-tail` off ORIGIN main (fetch first); spec + plan
`git add -f`'d onto the branch; subagent-driven. Five implementation tasks:
(1) engine #272+#273, (2) server #275 + scaffold #276+#277,
(3) connmgr #279, (4) scheduler #284 + worker #285,
(5) devmode #287 + CHANGELOG; then (6) rebase (post-#314), full
verification, whole-branch review, PR. PR closes all nine issues — the
backlog is empty afterwards.

## Out of scope

- Any new wildcard capability or channel-matching change beyond the guard
  placement (#279).
- Persisting scheduler history anywhere new (#284 is additive to the
  existing in-memory/endpoint surface).
- Worker retry/disposition semantics (#285 touches only the timeout layer).
