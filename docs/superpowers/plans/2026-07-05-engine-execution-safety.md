# Engine Execution Safety (Tranche B) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fix the 7 `engine-*` findings (2 High, 5 Medium) from `REVIEW-FINDINGS-2026-07-05.md` — eliminate the parallel-branch process crash, stop reporting truncated executions as success, make join classification deterministic, remove the pooled-map aliasing hazard, and reject alias/ID and duplicate-workflow-ID collisions at load.

**Architecture:** All changes are in `internal/engine` (executor, compiler, exclusivity, context, cache), plus one cross-cutting behavior the HTTP server already supports (`*api.TimeoutError` → 504). No public API change.

**Tech Stack:** Go (go1.25), `pkg/api` error types, `expr-lang/expr`.

## Global Constraints

- Go module floor: **go1.25**.
- No public API break; the timeout behavior changes from returning `nil` to returning an error (a bugfix) — worth a CHANGELOG "Fixed" line.
- Workflow timeout → `&api.TimeoutError{Duration, Operation}` (server maps to **504**, `internal/server/errors.go:75`); parent-cancellation → a generic wrapped error (500).
- Starved AND-join is a **hard error** (do not silently skip).
- Join classification and output-exclusivity verdicts must be **deterministic** across process restarts (no map-iteration-order dependence).
- All `internal/engine` tests run under `-race`.
- Pre-push gate: `gofmt -l .`, `go vet ./...`, `golangci-lint run`, `go test -race ./internal/engine/... ./internal/server/...`.

**Worktree:** `.worktrees/engine-execution-safety`, branch `feat/engine-execution-safety` off `main`. Spec + this plan force-added (`git add -f docs/superpowers/...`).

## File map

- `internal/engine/executor.go` — `firstError` recorder (Task 1); completeness check at drain (Tasks 2, 3).
- `internal/engine/compiler.go` — deterministic `hasCommonConditionalAncestor` (Task 4); `validateAliases` (Task 6).
- `internal/engine/exclusivity.go` — deterministic `findOutputLeadingTo` (Task 4).
- `internal/engine/context.go` — remove `exprContextPool` (Task 5).
- `internal/engine/cache.go` — duplicate-workflow-ID rejection + shared `buildGraphs` helper (Task 7).
- Tests: `internal/engine/executor_test.go`, `compiler_test.go`, `exclusivity_test.go`, `context_test.go`, `edge_cases_test.go`.

## Test harness (already exists — reuse)

- `setupExecutorTest(t, map[string]api.NodeExecutor) (*registry.NodeRegistry, *registry.ServiceRegistry)` (`executor_test.go:45`).
- `api.NodeExecutor`: `Outputs() []string` and `Execute(ctx, nCtx api.ExecutionContext, config, services map[string]any) (outputName string, data any, err error)`.
- Build+run: `graph, err := Compile(wf, resolver)`; `execCtx := NewExecutionContext(WithWorkflowID(id))`; `ExecuteGraph(context.Background(), graph, execCtx, svcReg, nodeReg)`.
- Existing test executors: `slowExecutor{delay}`, `orderTrackingExecutor`, `mockPassExecutor`.

---

### Task 1: Crash-safe first-error recorder (engine-1)

**Files:**
- Modify: `internal/engine/executor.go` (replace `firstErr atomic.Value`)
- Test: `internal/engine/executor_test.go`

**Interfaces:**
- Produces: `type firstError struct{ mu sync.Mutex; err error }` with `set(error)` and `get() error`.

- [ ] **Step 1: Write the failing test** — two parallel branches fail with *different concrete error types*; the current `atomic.Value` panics.

```go
// executor_test.go
type errExecutor struct {
	output string
	data   any
	err    error
}

func (e *errExecutor) Outputs() []string { return []string{"success", "error"} }
func (e *errExecutor) Execute(_ context.Context, _ api.ExecutionContext, _ map[string]any, _ map[string]any) (string, any, error) {
	return e.output, e.data, e.err
}

func TestExecuteGraph_ParallelMixedErrorTypes_NoPanic(t *testing.T) {
	// Branch "a" returns a Go error → dispatch wraps it in *engine.NodeExecutionError.
	// Branch "b" emits output "error" with no error edge → executor builds *errors.errorString.
	// Two distinct concrete error types race to record firstErr.
	nodeReg, svcReg := setupExecutorTest(t, map[string]api.NodeExecutor{
		"a": &errExecutor{err: errors.New("branch a failed")},
		"b": &errExecutor{output: "error"},
	})
	wf := WorkflowConfig{
		ID: "mixed",
		Nodes: map[string]NodeConfig{
			"a": {Type: "test.a"},
			"b": {Type: "test.b"},
		},
		// two entry nodes → run in parallel; "b" has no error edge
	}
	graph, err := Compile(wf, nil)
	require.NoError(t, err)

	// Run repeatedly to make the concurrent record deterministic.
	for i := 0; i < 20; i++ {
		execCtx := NewExecutionContext(WithWorkflowID("mixed"))
		gerr := ExecuteGraph(context.Background(), graph, execCtx, svcReg, nodeReg)
		require.Error(t, gerr) // a failure is expected; a PANIC (process crash) is not
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/engine/ -run TestExecuteGraph_ParallelMixedErrorTypes_NoPanic -race -count=1`
Expected: FAIL — `panic: sync/atomic: compare and swap of inconsistently typed value into Value` (crashes the test binary).

- [ ] **Step 3: Replace `atomic.Value` with the mutex recorder**

In `executor.go`, add near the top of the file (package level):

```go
// firstError records the first error seen across parallel node goroutines.
// It replaces a sync/atomic.Value, which panics when errors of different
// concrete types are stored (even on a losing CompareAndSwap).
type firstError struct {
	mu  sync.Mutex
	err error
}

func (f *firstError) set(err error) {
	f.mu.Lock()
	if f.err == nil {
		f.err = err
	}
	f.mu.Unlock()
}

func (f *firstError) get() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.err
}
```

In `ExecuteGraph`, change the declaration:

```go
	var (
		wg       sync.WaitGroup
		firstErr firstError
	)
```

Replace the three `firstErr.CompareAndSwap(nil, X)` calls (currently ~116, ~155, ~174) with `firstErr.set(X)`, and the `firstErr.Load()` read (currently ~226) with `firstErr.get()`. In the failure block, `workflowErr = errVal.(error)` becomes `workflowErr = errVal` (it's already an `error`). Keep the `sync/atomic` import (still used by `pending`/`dispatched`/`depth`).

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/engine/ -run TestExecuteGraph_ParallelMixedErrorTypes_NoPanic -race -count=1`
Expected: PASS (error returned, no panic).

- [ ] **Step 5: Full engine suite**

Run: `go test ./internal/engine/... -race`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/engine/executor.go internal/engine/executor_test.go
git commit -m "fix(engine): crash-safe first-error recorder (engine-1)"
```

---

### Task 2: Timeout/cancellation returns an error, not nil (engine-2)

**Files:**
- Modify: `internal/engine/executor.go` (drain region after `wg.Wait()`)
- Test: `internal/engine/executor_test.go`

**Interfaces:**
- Consumes: `firstError.get()` (Task 1).
- Produces: a non-nil `error` return from `ExecuteGraph` on context expiry — `*api.TimeoutError` on deadline, a wrapped error on cancel.

- [ ] **Step 1: Write the failing test**

```go
func TestExecuteGraph_Timeout_ReturnsTimeoutError(t *testing.T) {
	nodeReg, svcReg := setupExecutorTest(t, map[string]api.NodeExecutor{
		"slow": &slowExecutor{delay: 200 * time.Millisecond},
		"next": &mockPassExecutor{},
	})
	wf := WorkflowConfig{
		ID:      "tmo",
		Timeout: "50ms",
		Nodes:   map[string]NodeConfig{"slow": {Type: "test.slow"}, "next": {Type: "test.next"}},
		Edges:   []EdgeConfig{{From: "slow", To: "next"}},
	}
	graph, err := Compile(wf, nil)
	require.NoError(t, err)
	execCtx := NewExecutionContext(WithWorkflowID("tmo"))
	gerr := ExecuteGraph(context.Background(), graph, execCtx, svcReg, nodeReg)

	var toErr *api.TimeoutError
	require.ErrorAs(t, gerr, &toErr, "timeout must return *api.TimeoutError, not nil")
	_, ran := execCtx.GetOutput("next")
	require.False(t, ran, "downstream node must not be reported as run after timeout")
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/engine/ -run TestExecuteGraph_Timeout_ReturnsTimeoutError -race -count=1`
Expected: FAIL — `gerr` is `nil` (ErrorAs fails), because success is decided solely by `firstErr`.

- [ ] **Step 3: Add the context-expiry check at drain**

Add imports to `executor.go`: `"errors"` and `"github.com/chimpanze/noda/pkg/api"`.

Replace the drain/return region (from `wg.Wait()` through the `if errVal := firstErr.Load()...` block) so the error is computed once:

```go
	wg.Wait()

	duration := time.Since(startTime)

	// Determine the workflow result: a recorded node error takes precedence;
	// otherwise a truncated execution (context expired) is itself a failure —
	// never report success for work that did not complete.
	resultErr := firstErr.get()
	if resultErr == nil && execCtx2.Err() != nil {
		if errors.Is(execCtx2.Err(), context.DeadlineExceeded) {
			resultErr = &api.TimeoutError{Duration: graph.Timeout, Operation: "workflow " + graph.WorkflowID}
		} else {
			resultErr = fmt.Errorf("workflow %q aborted: %w", graph.WorkflowID, execCtx2.Err())
		}
	}

	if resultErr != nil {
		execCtx.Log("info", "workflow failed", map[string]any{"duration": duration.String()})
		workflowErr = resultErr
		if m := execCtx.Metrics(); m != nil {
			wfAttrs := metric.WithAttributes(attribute.String("workflow_id", graph.WorkflowID))
			m.WorkflowDuration.Record(ctx, duration.Seconds(), wfAttrs)
			m.WorkflowsTotal.Add(ctx, 1, wfAttrs)
			m.WorkflowErrors.Add(ctx, 1, wfAttrs)
		}
		execCtx.EmitTrace(string(trace.EventWorkflowFailed), "", "", "", workflowErr.Error(), nil)
		return workflowErr
	}
```

(Leave the success-metrics/trace/`return nil` block below unchanged.)

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/engine/ -run TestExecuteGraph_Timeout_ReturnsTimeoutError -race -count=1`
Expected: PASS.

- [ ] **Step 5: Full engine + server suites (cross-cutting 504 mapping)**

Run: `go test -race ./internal/engine/... ./internal/server/...`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/engine/executor.go internal/engine/executor_test.go
git commit -m "fix(engine): return TimeoutError on workflow timeout, not nil success (engine-2)"
```

---

### Task 3: Starved AND-join is a hard error (engine-3)

**Files:**
- Modify: `internal/engine/executor.go` (extend the drain check from Task 2)
- Test: `internal/engine/executor_test.go`

**Interfaces:**
- Consumes: the `resultErr` computation (Task 2), `graph.DepCount`, `graph.JoinTypes`, `pending map[string]*atomic.Int32`, `JoinAND`.

- [ ] **Step 1: Write the failing test** — a conditional leg that doesn't fire starves an AND-join.

```go
// condExecutor emits a chosen output so one downstream leg is never reached.
type condExecutor struct{ out string }

func (c *condExecutor) Outputs() []string { return []string{"go", "skip"} }
func (c *condExecutor) Execute(_ context.Context, _ api.ExecutionContext, _ map[string]any, _ map[string]any) (string, any, error) {
	return c.out, nil, nil
}

// twoOutResolver reports that "test.cond" has outputs go/skip; others success/error.
type twoOutResolver struct{}

func (twoOutResolver) OutputsForType(nodeType string) ([]string, bool) {
	if nodeType == "test.cond" {
		return []string{"go", "skip"}, true
	}
	return []string{"success", "error"}, true
}

func TestExecuteGraph_StarvedANDJoin_Errors(t *testing.T) {
	nodeReg, svcReg := setupExecutorTest(t, map[string]api.NodeExecutor{
		"a":    &mockPassExecutor{},
		"cond": &condExecutor{out: "skip"}, // "go" branch (→ b → j) never fires
		"b":    &mockPassExecutor{},
		"j":    &mockPassExecutor{},
	})
	wf := WorkflowConfig{
		ID: "starve",
		Nodes: map[string]NodeConfig{
			"a": {Type: "test.a"}, "cond": {Type: "test.cond"}, "b": {Type: "test.b"}, "j": {Type: "test.j"},
		},
		Edges: []EdgeConfig{
			{From: "a", To: "j"},                 // leg 1 fires
			{From: "cond", To: "b", Output: "go"},// only via "go", which is not emitted
			{From: "b", To: "j"},                 // leg 2 never arrives
		},
	}
	graph, err := Compile(wf, twoOutResolver{})
	require.NoError(t, err)
	require.Equal(t, JoinAND, graph.JoinTypes["j"], "precondition: j is an AND-join")

	execCtx := NewExecutionContext(WithWorkflowID("starve"))
	gerr := ExecuteGraph(context.Background(), graph, execCtx, svcReg, nodeReg)
	require.Error(t, gerr)
	require.Contains(t, gerr.Error(), "AND-join \"j\"")
}

func TestExecuteGraph_ANDJoin_AllLegsFire_OK(t *testing.T) {
	nodeReg, svcReg := setupExecutorTest(t, map[string]api.NodeExecutor{
		"a": &mockPassExecutor{}, "cond": &condExecutor{out: "go"}, "b": &mockPassExecutor{}, "j": &mockPassExecutor{},
	})
	wf := WorkflowConfig{
		ID:    "ok",
		Nodes: map[string]NodeConfig{"a": {Type: "test.a"}, "cond": {Type: "test.cond"}, "b": {Type: "test.b"}, "j": {Type: "test.j"}},
		Edges: []EdgeConfig{{From: "a", To: "j"}, {From: "cond", To: "b", Output: "go"}, {From: "b", To: "j"}},
	}
	graph, err := Compile(wf, twoOutResolver{})
	require.NoError(t, err)
	execCtx := NewExecutionContext(WithWorkflowID("ok"))
	require.NoError(t, ExecuteGraph(context.Background(), graph, execCtx, svcReg, nodeReg))
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/engine/ -run TestExecuteGraph_StarvedANDJoin -race -count=1`
Expected: FAIL — `gerr` is nil (starved join silently skipped, success returned).

- [ ] **Step 3: Add the starved-AND-join scan**

In `executor.go`, extend the drain check: after the context-expiry block (Task 2) and still under `if resultErr == nil`, scan AND-joins:

```go
	if resultErr == nil {
		for id, jt := range graph.JoinTypes {
			if jt != JoinAND {
				continue
			}
			total := graph.DepCount[id]
			remaining := int(pending[id].Load())
			// Received at least one leg (remaining < total) but never fired
			// (an AND-join fires only when remaining reaches 0). remaining == total
			// means zero legs arrived — a normal unreached branch, not an error.
			if remaining > 0 && remaining < total {
				resultErr = fmt.Errorf("workflow %q incomplete: AND-join %q received %d of %d legs and never fired",
					graph.WorkflowID, id, total-remaining, total)
				break
			}
		}
	}
```

This must sit **before** the `if resultErr != nil { ... return }` block from Task 2 (so the starved error flows through the same failure path). The `pending` reads are race-free — all goroutines have joined at `wg.Wait()`.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/engine/ -run 'TestExecuteGraph_StarvedANDJoin_Errors|TestExecuteGraph_ANDJoin_AllLegsFire_OK' -race -count=1`
Expected: PASS both.

- [ ] **Step 5: Full engine suite**

Run: `go test ./internal/engine/... -race`
Expected: PASS (watch for pre-existing fixtures that intentionally starve an AND-join — if any legitimately relied on the old silent-skip, surface it as DONE_WITH_CONCERNS rather than weakening the check).

- [ ] **Step 6: Commit**

```bash
git add internal/engine/executor.go internal/engine/executor_test.go
git commit -m "fix(engine): starved AND-join is a hard error (engine-3)"
```

---

### Task 4: Deterministic join classification & exclusivity (engine-4)

**Files:**
- Modify: `internal/engine/compiler.go` (`hasCommonConditionalAncestor`), `internal/engine/exclusivity.go` (`findOutputLeadingTo`)
- Test: `internal/engine/compiler_test.go`

**Interfaces:** no signature changes; behavior becomes order-independent.

- [ ] **Step 1: Write the failing/characterization test** — classification is stable across many compiles.

```go
func TestComputeJoinTypes_Deterministic(t *testing.T) {
	// A node reachable through two outputs of a conditional ancestor is the
	// ambiguous case the old first-match break decided by map order.
	wf := WorkflowConfig{
		ID: "det",
		Nodes: map[string]NodeConfig{
			"cond": {Type: "test.cond"}, "x": {Type: "test.x"}, "y": {Type: "test.y"}, "j": {Type: "test.j"},
		},
		Edges: []EdgeConfig{
			{From: "cond", To: "x", Output: "go"},
			{From: "cond", To: "y", Output: "skip"},
			{From: "x", To: "j"},
			{From: "y", To: "j"},
		},
	}
	var first JoinType
	for i := 0; i < 50; i++ {
		g, err := Compile(wf, twoOutResolver{})
		require.NoError(t, err)
		if i == 0 {
			first = g.JoinTypes["j"]
		} else {
			require.Equal(t, first, g.JoinTypes["j"], "join classification must be deterministic across compiles")
		}
	}
	require.Equal(t, JoinOR, first, "j reached via two different conditional outputs → OR-join")
}
```

(`twoOutResolver` is defined in Task 3's test file; if Task 4 is implemented before Task 3, add it here instead.)

- [ ] **Step 2: Run test to verify it fails or is flaky**

Run: `go test ./internal/engine/ -run TestComputeJoinTypes_Deterministic -count=1`
Expected: FAIL or intermittently FAIL — first-match `break` over a map yields order-dependent classification.

- [ ] **Step 3: Make both first-match loops deterministic**

In `compiler.go`, add `"sort"` to imports. In `hasCommonConditionalAncestor`, replace the inner first-match loop (currently ~381-387) so each inbound `src` records **all** outputs whose subgraph reaches it, iterating output names in sorted order:

```go
			reachedThrough := make(map[string]map[string]bool) // src → set of output names
			for _, src := range inbound {
				reachedThrough[src] = make(map[string]bool)
				names := make([]string, 0, len(outputs))
				for n := range outputs {
					names = append(names, n)
				}
				sort.Strings(names)
				for _, outputName := range names {
					if reachableFrom(g, outputs[outputName], src) {
						reachedThrough[src][outputName] = true
					}
				}
			}
			// OR-join iff the inbound sources are reached through different outputs.
			allOutputs := make(map[string]bool)
			for _, set := range reachedThrough {
				for o := range set {
					allOutputs[o] = true
				}
			}
			if len(allOutputs) > 1 {
				return true
			}
```

In `exclusivity.go`, `findOutputLeadingTo` (~79-104): iterate `graph.Adjacency[ancestor]` output names in sorted order so the returned output for a target is deterministic:

```go
func findOutputLeadingTo(graph *CompiledGraph, ancestor, target string) string {
	names := make([]string, 0, len(graph.Adjacency[ancestor]))
	for n := range graph.Adjacency[ancestor] {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, outputName := range names {
		targets := graph.Adjacency[ancestor][outputName]
		visited := make(map[string]bool)
		queue := make([]string, len(targets))
		copy(queue, targets)
		for _, t := range targets {
			visited[t] = true
		}
		for len(queue) > 0 {
			cur := queue[0]
			queue = queue[1:]
			if cur == target {
				return outputName
			}
			for _, nextTargets := range graph.Adjacency[cur] {
				for _, next := range nextTargets {
					if !visited[next] {
						visited[next] = true
						queue = append(queue, next)
					}
				}
			}
		}
	}
	return ""
}
```

Add `"sort"` to `exclusivity.go` imports.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/engine/ -run TestComputeJoinTypes_Deterministic -count=5`
Expected: PASS every run.

- [ ] **Step 5: Full engine suite**

Run: `go test ./internal/engine/... -race`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/engine/compiler.go internal/engine/exclusivity.go internal/engine/compiler_test.go
git commit -m "fix(engine): deterministic join classification and exclusivity (engine-4)"
```

---

### Task 5: Drop the expr-context pool (engine-5)

**Files:**
- Modify: `internal/engine/context.go` (remove `exprContextPool`, `returnExprContext`; fresh alloc in `buildExprContext`; drop `defer returnExprContext` in `Resolve`/`ResolveWithVars`)
- Test: `internal/engine/context_test.go`

**Interfaces:** `buildExprContext()` still returns `map[string]any`; callers no longer return it to a pool.

- [ ] **Step 1: Write the failing test** — a captured `$env` map must not be corrupted by concurrent Resolves.

```go
func TestResolve_EnvCaptureNotCorruptedByConcurrentResolves(t *testing.T) {
	c := NewExecutionContext(WithInput(map[string]any{"marker": "orig"}))
	// Capture the whole environment map via $env (expr-lang exposes it by reference).
	captured, err := c.Resolve("{{ $env }}")
	require.NoError(t, err)
	capturedMap, ok := captured.(map[string]any)
	require.True(t, ok)

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = c.Resolve("{{ input.marker }}")
		}()
	}
	// Read the captured map concurrently with the Resolves above.
	for i := 0; i < 50; i++ {
		_ = capturedMap["input"]
	}
	wg.Wait()

	// Under the pool, capturedMap was recycled/cleared by a concurrent Resolve.
	require.Contains(t, capturedMap, "input", "captured $env map must remain intact")
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/engine/ -run TestResolve_EnvCaptureNotCorruptedByConcurrentResolves -race -count=1`
Expected: FAIL — `-race` reports a data race on the pooled map, and/or `capturedMap` no longer contains "input".

- [ ] **Step 3: Remove the pool**

In `context.go`: delete the `exprContextPool` var (lines ~19-24) and the `returnExprContext` func (lines ~389-391). In `buildExprContext`, replace the pool `Get` + clear-keys with a fresh allocation:

```go
func (c *ExecutionContextImpl) buildExprContext() map[string]any {
	ctx := make(map[string]any, 8)
	ctx["input"] = c.input
	// ... rest unchanged ...
	return ctx
}
```

In `Resolve` and `ResolveWithVars`, remove the `defer returnExprContext(context)` line.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/engine/ -run TestResolve_EnvCaptureNotCorruptedByConcurrentResolves -race -count=1`
Expected: PASS (no race, map intact).

- [ ] **Step 5: Full engine suite**

Run: `go test ./internal/engine/... -race`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/engine/context.go internal/engine/context_test.go
git commit -m "fix(engine): drop pooled expr-context map to end \$env aliasing hazard (engine-5)"
```

---

### Task 6: Alias/ID collision validation (engine-6)

**Files:**
- Modify: `internal/engine/compiler.go` (`validateAliases`, called from `Compile`)
- Test: `internal/engine/compiler_test.go`

**Interfaces:**
- Produces: `validateAliases(g *CompiledGraph) error`.

- [ ] **Step 1: Write the failing test**

```go
func TestCompile_AliasCollidesWithNodeID(t *testing.T) {
	wf := WorkflowConfig{
		ID:    "c1",
		Nodes: map[string]NodeConfig{"x": {Type: "test.x"}, "y": {Type: "test.y", As: "x"}},
		Edges: []EdgeConfig{{From: "x", To: "y"}},
	}
	_, err := Compile(wf, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "collides")
}

func TestCompile_DuplicateAlias(t *testing.T) {
	wf := WorkflowConfig{
		ID:    "c2",
		Nodes: map[string]NodeConfig{"a": {Type: "test.a", As: "dup"}, "b": {Type: "test.b", As: "dup"}},
		Edges: []EdgeConfig{{From: "a", To: "b"}},
	}
	_, err := Compile(wf, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "dup")
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/engine/ -run 'TestCompile_AliasCollidesWithNodeID|TestCompile_DuplicateAlias' -count=1`
Expected: FAIL — `Compile` currently returns no error for either.

- [ ] **Step 3: Add `validateAliases` and call it in `Compile`**

In `compiler.go`:

```go
// validateAliases rejects an "as" alias that equals a node ID or duplicates
// another alias — both would silently overwrite outputs at runtime.
func validateAliases(g *CompiledGraph) error {
	seen := make(map[string]string) // alias → nodeID that declared it
	for id, n := range g.Nodes {
		if n.As == "" {
			continue
		}
		if _, isNodeID := g.Nodes[n.As]; isNodeID {
			return fmt.Errorf("workflow %q: node %q alias %q collides with an existing node ID", g.WorkflowID, id, n.As)
		}
		if prev, dup := seen[n.As]; dup {
			return fmt.Errorf("workflow %q: alias %q declared by both %q and %q", g.WorkflowID, n.As, prev, id)
		}
		seen[n.As] = id
	}
	return nil
}
```

In `Compile`, after `computeJoinTypes(g)` and before/after `ValidateOutputExclusivity` (near line 250):

```go
	if err := validateAliases(g); err != nil {
		return nil, err
	}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/engine/ -run 'TestCompile_AliasCollidesWithNodeID|TestCompile_DuplicateAlias' -count=1`
Expected: PASS.

- [ ] **Step 5: Full engine suite**

Run: `go test ./internal/engine/... -race`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/engine/compiler.go internal/engine/compiler_test.go
git commit -m "fix(engine): reject alias/node-ID collisions at compile time (engine-6)"
```

---

### Task 7: Reject duplicate workflow IDs (engine-7)

**Files:**
- Modify: `internal/engine/cache.go` (shared `buildGraphs` helper used by `NewWorkflowCache` and `Invalidate`)
- Test: `internal/engine/edge_cases_test.go`

**Interfaces:**
- Produces: `buildGraphs(workflows map[string]map[string]any, resolver NodeOutputResolver) (map[string]*CompiledGraph, error)`.

- [ ] **Step 1: Write the failing test**

```go
func TestNewWorkflowCache_DuplicateLogicalID(t *testing.T) {
	workflows := map[string]map[string]any{
		"fileA": {"id": "shared", "nodes": map[string]any{"n": map[string]any{"type": "test.n"}}},
		"fileB": {"id": "shared", "nodes": map[string]any{"n": map[string]any{"type": "test.n"}}},
	}
	_, err := NewWorkflowCache(workflows, &DefaultOutputResolver{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "duplicate workflow id")
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/engine/ -run TestNewWorkflowCache_DuplicateLogicalID -count=1`
Expected: FAIL — the second `graphs["shared"] = graph` silently overwrites; no error.

- [ ] **Step 3: Extract `buildGraphs` with duplicate detection; use it in both constructors**

In `cache.go`, add:

```go
// buildGraphs parses+compiles all workflows and indexes each by its file key
// and (if different) its logical "id" field, rejecting any id collision.
func buildGraphs(workflows map[string]map[string]any, resolver NodeOutputResolver) (map[string]*CompiledGraph, error) {
	graphs := make(map[string]*CompiledGraph, len(workflows))
	source := make(map[string]string) // index key → file key that declared it
	put := func(key, fileKey string, g *CompiledGraph) error {
		if prev, ok := source[key]; ok {
			return fmt.Errorf("duplicate workflow id %q (declared by %q and %q)", key, prev, fileKey)
		}
		source[key] = fileKey
		graphs[key] = g
		return nil
	}
	for id, raw := range workflows {
		wfConfig, err := ParseWorkflowFromMap(id, raw)
		if err != nil {
			return nil, fmt.Errorf("parse workflow %q: %w", id, err)
		}
		graph, err := Compile(wfConfig, resolver)
		if err != nil {
			return nil, fmt.Errorf("compile workflow %q: %w", id, err)
		}
		if err := put(id, id, graph); err != nil {
			return nil, err
		}
		if jsonID, ok := raw["id"].(string); ok && jsonID != id {
			if err := put(jsonID, id, graph); err != nil {
				return nil, err
			}
		}
	}
	return graphs, nil
}
```

Rewrite `NewWorkflowCache` to use it:

```go
func NewWorkflowCache(workflows map[string]map[string]any, resolver NodeOutputResolver) (*WorkflowCache, error) {
	graphs, err := buildGraphs(workflows, resolver)
	if err != nil {
		return nil, err
	}
	slog.Info("workflows compiled", "count", len(graphs))
	return &WorkflowCache{graphs: graphs}, nil
}
```

Rewrite `Invalidate` to use it:

```go
func (c *WorkflowCache) Invalidate(workflows map[string]map[string]any, resolver NodeOutputResolver) error {
	newGraphs, err := buildGraphs(workflows, resolver)
	if err != nil {
		return err
	}
	c.mu.Lock()
	c.graphs = newGraphs
	c.mu.Unlock()
	return nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/engine/ -run TestNewWorkflowCache_DuplicateLogicalID -count=1`
Expected: PASS.

- [ ] **Step 5: Full engine suite**

Run: `go test ./internal/engine/... -race`
Expected: PASS (existing cache/edge_cases tests still green).

- [ ] **Step 6: Commit**

```bash
git add internal/engine/cache.go internal/engine/edge_cases_test.go
git commit -m "fix(engine): reject duplicate workflow ids at cache build (engine-7)"
```

---

### Task 8: CHANGELOG + full gate

**Files:**
- Modify: `CHANGELOG.md`

- [ ] **Step 1: CHANGELOG entry**

Add under a `### Fixed` heading: "Engine execution safety: a workflow that times out now returns a `504`/error instead of silently reporting success on a truncated run; parallel branches failing with different error types no longer crash the process; starved AND-joins fail loudly; join classification is deterministic; alias/node-ID and duplicate workflow-ID collisions are rejected at load."

- [ ] **Step 2: Full gate**

Run: `gofmt -l . && go vet ./... && golangci-lint run && go test -race ./internal/engine/... ./internal/server/...`
Expected: clean, all pass.

- [ ] **Step 3: Commit**

```bash
git add CHANGELOG.md
git commit -m "docs(engine): changelog for execution-safety fixes"
```

---

## Self-review notes

- **Spec coverage:** engine-1 → Task 1; engine-2 → Task 2; engine-3 → Task 3; engine-4 → Task 4; engine-5 → Task 5; engine-6 → Task 6; engine-7 → Task 7; changelog/gate → Task 8. All seven covered.
- **Type consistency:** `firstError.set/get` (Task 1) used in Tasks 2/3. `resultErr` introduced in Task 2, extended in Task 3. `pending map[string]*atomic.Int32`, `graph.DepCount map[string]int`, `graph.JoinTypes`, `JoinAND` used as defined in `compiler.go`. `twoOutResolver`/`condExecutor` defined in Task 3's test, reused by Task 4 (noted inline). `buildGraphs` signature (Task 7) matches both call sites.
- **Ordering:** Task 2 and Task 3 edit the same drain region; Task 3 assumes Task 2's `resultErr` variable exists (stated). Task 4's test reuses Task 3's `twoOutResolver` (noted). Run tasks in order.
- **Deferred (out of scope, per spec):** engine-8/9/10/11 (Low); blocking `$env` from capturing secrets.
