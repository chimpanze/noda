# Engine Execution Safety (Tranche B) — Design

Date: 2026-07-05
Source: `REVIEW-FINDINGS-2026-07-05.md` — 7 `engine-*` findings (2 High, 5 Medium).
Branch/worktree (planned): `feat/engine-execution-safety` in `.worktrees/engine-execution-safety`, off `main`.

## Why

The clean-slate Go review (2026-07-05) found the workflow engine (`internal/engine`) carries the two highest-blast-radius correctness defects outside the wasm cluster: a parallel-branch error path that can **crash the whole process**, and a timeout path that **reports a truncated execution as success**. Five further Medium correctness bugs sit in the same executor/compiler/context/cache surface (nondeterministic join classification, a pooled-map aliasing hazard, silent output overwrites via alias/ID collision, and silent workflow overwrites via duplicate IDs). This tranche fixes all seven as one coherent change — they share the execution model and several are related failure modes of the same "don't silently drop work / don't be nondeterministic" theme.

**Decisions (user-approved):** starved AND-join → **hard error** at drain (do not silently skip); all 7 in **one PR**. No public API break (adds internal validation + one existing error type on a new path).

## Findings in scope

| ID | Sev | Summary |
|---|---|---|
| engine-1 | High | `firstErr atomic.Value.CompareAndSwap` with mixed concrete error types panics → process crash |
| engine-2 | High | Workflow timeout/cancellation returns `nil` error → truncated run reported as success |
| engine-3 | Med | AND-join whose legs can't all fire silently skips the join and succeeds |
| engine-4 | Med | Join classification + output-exclusivity validation nondeterministic (map iteration + first-match break) |
| engine-5 | Med | `$env` aliases the pooled expression-context map → corruption / fatal concurrent map access |
| engine-6 | Med | Node alias (`as`) may equal another node's ID → outputs silently overwrite |
| engine-7 | Med | Workflow cache double-indexing by JSON `id` silently overwrites workflows |

## Verified facts

- `internal/server/errors.go:75-83`: `MapErrorToHTTP` maps `*api.TimeoutError` (via `errors.As`) → HTTP **504**. `api.TimeoutError{Duration time.Duration, Operation string}` (`pkg/api/errors.go:34`).
- `internal/engine/executor.go`: `firstErr atomic.Value` (75), CAS at 116/155/174; success is decided solely by `firstErr.Load()` at 226 with **no** `execCtx2.Err()` check before `return nil` (259). AND-join fires on `pending[targetID].Add(-1) == 0` (205); OR-join dispatch already tracked via the `dispatched map[string]*atomic.Bool` (55-58).
- `internal/engine/context.go`: `exprContextPool` (20) reused by `buildExprContext` (354) and returned by `Resolve`/`ResolveWithVars` (209/221). `SetOutput`/`GetOutput` (264/276) key by alias-or-ID; `RegisterAlias` (290).
- `internal/engine/compiler.go`: `computeJoinTypes` (313) → `hasCommonConditionalAncestor` first-match `break` over `range outputs` (382-386); `Compile` (122) already runs `detectCycle`/`computeJoinTypes`/`ValidateOutputExclusivity` (243-253) — the place to add alias/ID validation.
- `internal/engine/exclusivity.go`: `findOutputLeadingTo` first-match `break`... over `range graph.Adjacency[ancestor]` (80).
- `internal/engine/cache.go`: `NewWorkflowCache` (35-37) and `Invalidate` (67-69) index by `raw["id"]` with no duplicate check.

## Design

### Unit 1 — Crash-safe first-error (engine-1)

Replace `firstErr atomic.Value` with a small mutex-guarded recorder in `executor.go`:

```go
type firstError struct {
    mu  sync.Mutex
    err error
}
func (f *firstError) set(err error) { f.mu.Lock(); if f.err == nil { f.err = err }; f.mu.Unlock() }
func (f *firstError) get() error    { f.mu.Lock(); defer f.mu.Unlock(); return f.err }
```

Replace the three `firstErr.CompareAndSwap(nil, X)` calls with `firstErr.set(X)` and `firstErr.Load()` with `firstErr.get()`. First-error-wins semantics preserved; no type-mismatch panic possible. (`atomic.Pointer[error]` is the alternative; the mutex is chosen for clarity on this cold error path.)

### Unit 2 — Completeness check at drain (engine-2 + engine-3)

Add an incompleteness check in `ExecuteGraph` between `wg.Wait()` (222) and the success return (259), evaluated only when `firstErr.get() == nil`:

1. **Context expired (engine-2):** if `execCtx2.Err() != nil`:
   - `errors.Is(execCtx2.Err(), context.DeadlineExceeded)` → return `&api.TimeoutError{Duration: graph.Timeout, Operation: "workflow " + graph.WorkflowID}` (→ 504).
   - otherwise (parent cancelled) → return `fmt.Errorf("workflow %q aborted: %w", graph.WorkflowID, execCtx2.Err())` (→ 500).
2. **Starved AND-join (engine-3):** after the wait, scan every node whose `graph.JoinTypes[id] == JoinAND`. Its inbound leg count is `total := graph.DepCount[id]`; the number of legs that never arrived is `remaining := pending[id].Load()`; so legs received = `total - remaining`. A node is **starved** iff `0 < remaining < total` — it received at least one leg but not all, and because an AND-join fires only when `pending` reaches 0, `remaining > 0` means it definitively never fired. `remaining == total` (zero legs arrived) is a normal unreached branch, not an error; `remaining == 0` fired normally. On the first starved node found, return `fmt.Errorf("workflow %q incomplete: AND-join %q received %d of %d legs and never fired", graph.WorkflowID, id, total-remaining, total)` (→ 500).

Detection uses only `graph.DepCount` and the `pending` counters that already exist — no new tracking map is added. The `pending` map keys are read-only after init and the `Load()` happens after `wg.Wait()` (all goroutines joined), so the reads are race-free.

This makes all three consumers correct: HTTP returns 504/500 (not 202), the worker does not ack a truncated event as done, the scheduler logs failure.

### Unit 3 — Deterministic join classification & exclusivity (engine-4)

At the two first-match sites, iterate output names in **sorted** order and collect the full set of outputs a source is reachable through, rather than breaking on the first:

- `compiler.go hasCommonConditionalAncestor` (376-397): for each inbound `src`, collect **all** output names of the conditional ancestor whose subgraph reaches `src` (sorted iteration; no `break`). The "reached through different outputs → OR" test then compares the complete `reachedThrough` set. A `src` reachable through multiple outputs is recorded deterministically (e.g. all of them), so the OR/AND verdict no longer depends on map order.
- `exclusivity.go findOutputLeadingTo` (79-104): iterate `graph.Adjacency[ancestor]` output names in sorted order; on multiple reaching outputs, return a deterministic choice (first in sorted order) — the caller only needs *whether A and B come through different outputs*, which sorted iteration makes stable.

Result: identical classification and exclusivity verdicts across process restarts and map-seed changes. No behavioral change for unambiguous graphs; ambiguous graphs classify consistently.

### Unit 4 — Drop the expr-context pool (engine-5)

Remove `exprContextPool` and `returnExprContext`; `buildExprContext` allocates the outer map fresh (`make(map[string]any, 8)`); `Resolve`/`ResolveWithVars` drop the `defer returnExprContext(...)`. This eliminates the escape/recycle hazard entirely: a node output capturing `$env` (which returns the context map by reference) now holds a private, non-recycled map, and no map is shared across concurrent `Resolve` calls. The path already allocates `nodesMap` fresh each call, so the pool saved only the 8-key outer map — a negligible, correctness-costing optimization.

*Out of scope:* blocking `$env` so `secrets` can't be captured into a node output is a separate hardening concern (needs an expr AST patcher) and is not this finding.

### Unit 5 — Alias/ID collision validation (engine-6)

Add a compile-time check in `Compile` (alongside the existing validations near compiler.go:250): reject a workflow where (a) any node's `as` alias equals any node's ID, or (b) two nodes declare the same alias. Error: `fmt.Errorf("workflow %q: alias %q collides with node ID / duplicate alias", ...)`. Fails at load rather than silently overwriting outputs at runtime.

### Unit 6 — Reject duplicate workflow IDs (engine-7)

In `NewWorkflowCache` and `Invalidate`, before indexing by `jsonID`, check whether that key is already populated by a *different* workflow; if so return `fmt.Errorf("duplicate workflow id %q (declared by file keys %q and %q)", ...)`. Applies to both the file-key index and the `raw["id"]` index. Config error → reject at load; keeps the existing "index by both file key and logical id" behavior for the non-colliding case.

## Testing (TDD, per finding)

- **engine-1:** a test that drives two parallel branches failing with *different* concrete error types (a `*NodeExecutionError` and a plain `fmt.Errorf`); asserts no panic and a deterministic first-error. Run under `-race`. (RED: panics against current `atomic.Value`.)
- **engine-2:** a workflow with `timeout` and a slow node; assert `ExecuteGraph` returns a `*api.TimeoutError` (via `errors.As`), not nil, and that a downstream node's output is absent (proving it didn't silently "succeed"). Plus a parent-cancel variant → non-nil generic error.
- **engine-3:** a graph with an AND-join fed by a conditional such that only one leg fires; assert the incomplete/starved error (and that it is NOT raised when the join legitimately received zero legs).
- **engine-4:** classify a graph many times (and/or with shuffled adjacency insertion) asserting a stable JoinType and a stable `ValidateOutputExclusivity` verdict; a fixture where the old first-match could flip.
- **engine-5:** `-race` test: a node whose expression is `{{ $env }}` (captured as output) run concurrently with other Resolves; assert no data race and no cross-contamination of the captured value.
- **engine-6:** `Compile` returns an error for a workflow with an alias equal to another node's ID, and for duplicate aliases.
- **engine-7:** `NewWorkflowCache`/`Invalidate` return an error for two workflows sharing an `id`.

All engine tests run under `-race`. Gate: `gofmt -l .`, `go vet ./...`, `golangci-lint run`, `go test -race ./internal/engine/... ./internal/server/...` (server included because the timeout→504 mapping is cross-cutting).

## Mechanics

- Worktree `.worktrees/engine-execution-safety`, branch `feat/engine-execution-safety` off `main`.
- Subagent-driven execution per task: implementer → spec-compliance reviewer → code-quality reviewer.
- Spec + plan force-added to the branch (`git add -f docs/superpowers/...`).
- No CHANGELOG ABI note (no public API change); the timeout behavior is a bugfix (nil→error) worth a CHANGELOG "Fixed" line.
- At merge: add a "Shipped 2026-07-05" note for engine-1..7 to `REVIEW-FINDINGS-2026-07-05.md` (that file is on the review PR #262 branch, handled at merge time).

## Out of scope

Other engine findings from the review (engine-8 dynamic-ref eviction, engine-9 currentNode log attribution, engine-10 depth-copy race, engine-11 retry cancellation) are Low and deferred to a later tranche. Blocking `$env` from capturing secrets is a separate hardening item, not part of engine-5.
