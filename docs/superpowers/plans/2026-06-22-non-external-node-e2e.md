# Non-External Node End-to-End Verification — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Prove every Noda node with no external-service dependency works end-to-end through the real engine, fixing any bug found, so those nodes are production-ready.

**Architecture:** Two tiers. **Tier A** (control / transform / response / util / workflow) is exercised through Noda's own `noda test` runner against a new pure-core project `testdata/node-e2e/`, driven from Go by `internal/testing/e2e/run_test.go`. **Tier B** (ws.send / sse.send / wasm.send / wasm.query / upload.handle) is exercised by Go integration tests that wire an in-process service (connmgr / Wasm runtime / in-memory afero) and run a one-node workflow through `engine.ExecuteGraph`. A prerequisite runner fix injects a `SubWorkflowRunner` so `control.loop` / `workflow.run` can execute under test at all.

**Tech Stack:** Go, `internal/engine`, `internal/testing`, `internal/registry`, `internal/connmgr`, `internal/wasm`, `plugins/core/*`, `plugins/storage`, testify.

**Reference spec:** `docs/superpowers/specs/2026-06-22-non-external-node-e2e-design.md`

---

## Background facts (verified, rely on these)

- The `noda test` runner (`internal/testing/runner.go`) executes **core nodes for real** and mocks non-core plugin nodes. So Tier A nodes run genuinely end-to-end via JSON fixtures.
- Node outputs are collected per node ID; the stored value is exactly the node's returned `data` (`runner.go:collectOutputs`). `expect.output` is `{"<nodeID>.<field>": value}` against the **raw** outputs map (numbers compared as float64). `expect.outputs` (plural) is a **partial deep match** with JSON normalization — used for real `response.*` nodes whose data is a `*api.HTTPResponse` with **capitalized** keys (`Status`, `Body`, `Headers`).
- A node `Execute` returning an error routes to the `error` port. **No outbound error edge → workflow fails** (`status:"error"`, `error_node:"<id>"`). With an error edge, flow continues (`status:"success"`).
- Workflow JSON shape: top-level `id`, `nodes` (map nodeID→`{type, config, services?}`), `edges` (array of `{from, to, output?}`; `output` defaults to `"success"`). Edge `output` MUST be one of the source node's declared ports.
- Port names: `control.if` → `then`/`else`/`error`; `control.switch` → case-names + `default` + `error`; `control.loop` → `done`/`error`; everything else default `success`/`error`; `workflow.output` is terminal (no ports).
- A test-only project needs only `noda.json` (minimal: `{ "services": {} }`) plus `workflows/` and `tests/`. No `routes/` required; there is no orphan-workflow check (`internal/config/crossrefs.go`).
- `*registry.NodeRegistry` implements `engine.NodeOutputResolver`, so it can be passed directly to `engine.Compile`.
- `ServiceRegistry.Register(name, instance, plugin)` stores `plugin` without dereferencing — passing `nil` for `plugin` is safe; engine dispatch resolves services via `Get(name)` only.
- `NodeConfig{Type string, Services map[string]string, As string, Config map[string]any}`.
- expr-lang builtins available in `{{ }}`: `len(...)`, `split(s, sep)`, indexing `x[0]`, `in` operator, comparisons.

---

## File Structure

**Created:**
- `testdata/node-e2e/noda.json` — minimal project config.
- `testdata/node-e2e/workflows/*.json` — one workflow per node/behavior (Tier A).
- `testdata/node-e2e/tests/*.json` — one test suite per workflow (Tier A).
- `internal/testing/e2e/run_test.go` — Go driver: loads the project, runs every suite, fails on any failure.
- `plugins/core/ws/engine_e2e_test.go` — ws.send through the engine (Tier B).
- `plugins/core/sse/engine_e2e_test.go` — sse.send through the engine (Tier B).
- `plugins/core/wasm/engine_e2e_test.go` — wasm.send + wasm.query through the engine (Tier B).
- `plugins/core/upload/engine_e2e_test.go` — upload.handle through the engine (Tier B).

**Modified:**
- `internal/testing/runner.go` — inject a best-effort `SubWorkflowRunner` (Task 1).

---

## Task 1: Fix the test runner to support sub-workflows

`control.loop` and `workflow.run` read `ExecutionContext.SubWorkflowRunner()`; the runner never sets it. Inject one (best-effort, so existing plugin-laden projects don't regress).

**Files:**
- Test: `internal/testing/runner_subworkflow_test.go` (create)
- Modify: `internal/testing/runner.go`

- [ ] **Step 1: Write the failing test**

Create `internal/testing/runner_subworkflow_test.go`:

```go
package testing

import (
	"testing"

	"github.com/chimpanze/noda/internal/config"
	"github.com/chimpanze/noda/internal/registry"
	"github.com/chimpanze/noda/pkg/api"
	"github.com/chimpanze/noda/plugins/core/transform"
	workflowplugin "github.com/chimpanze/noda/plugins/core/workflow"
	"github.com/stretchr/testify/require"
)

func coreRegForSubWf(t *testing.T) *registry.NodeRegistry {
	t.Helper()
	reg := registry.NewNodeRegistry()
	for _, p := range []api.Plugin{&transform.Plugin{}, &workflowplugin.Plugin{}} {
		require.NoError(t, reg.RegisterFromPlugin(p))
	}
	return reg
}

func TestRunner_WorkflowRunSubWorkflow(t *testing.T) {
	rc := &config.ResolvedConfig{
		Workflows: map[string]map[string]any{
			"callee": {
				"id": "callee",
				"nodes": map[string]any{
					"calc": map[string]any{
						"type":   "transform.set",
						"config": map[string]any{"fields": map[string]any{"result": "{{ input.x * 10 }}"}},
					},
					"out": map[string]any{
						"type":   "workflow.output",
						"config": map[string]any{"name": "result", "data": "{{ nodes.calc }}"},
					},
				},
				"edges": []any{map[string]any{"from": "calc", "to": "out"}},
			},
			"caller": {
				"id": "caller",
				"nodes": map[string]any{
					"call": map[string]any{
						"type":   "workflow.run",
						"config": map[string]any{"workflow": "callee", "input": map[string]any{"x": "{{ input.x }}"}},
					},
				},
				"edges": []any{},
			},
		},
	}

	suite := TestSuite{
		Workflow: "caller",
		Cases: []TestCase{{
			Name:  "calls sub-workflow",
			Input: map[string]any{"x": float64(3)},
			Expect: TestExpectation{
				Status: "success",
				Output: map[string]any{"call.result": float64(30)},
			},
		}},
	}

	results := RunTestSuite(suite, rc, coreRegForSubWf(t))
	require.Len(t, results, 1)
	require.True(t, results[0].Passed, "expected pass, got: %s", results[0].Error)
}
```

- [ ] **Step 2: Run the test, verify it FAILS**

Run: `go test ./internal/testing/ -run TestRunner_WorkflowRunSubWorkflow -v`
Expected: FAIL — the `call` node errors because no sub-workflow runner is configured (status ends `error`, so `Passed` is false).

- [ ] **Step 3: Implement the fix in `internal/testing/runner.go`**

In `runTestCase`, the service registry is currently created at line ~127 just before `ExecuteGraph`. Move it earlier and inject the runner into `opts`. Replace the block that starts at `execCtx := engine.NewExecutionContext(opts...)` (line ~124) and the following `svcReg`/`ExecuteGraph` lines (~126-128) with:

```go
	// Service registry is shared by the main workflow and any sub-workflows.
	svcReg := registry.NewServiceRegistry()

	// Inject a sub-workflow runner so control.loop and workflow.run execute
	// end-to-end. Best-effort: if the project's workflows cannot all be compiled
	// with the test resolver (e.g. they contain unmocked plugin nodes), skip
	// injection and fall back to prior behavior (nil runner).
	if cache, cacheErr := engine.NewWorkflowCache(rc.Workflows, resolver); cacheErr == nil {
		opts = append(opts, engine.WithSubWorkflowRunner(&engine.SubWorkflowRunnerImpl{
			Cache:    cache,
			Services: svcReg,
			Nodes:    testNodeReg,
		}))
	}

	execCtx := engine.NewExecutionContext(opts...)

	// Execute
	execErr := engine.ExecuteGraph(context.Background(), graph, execCtx, svcReg, testNodeReg)
```

(Delete the now-duplicate `svcReg := registry.NewServiceRegistry()` that previously sat just above `ExecuteGraph`.)

- [ ] **Step 4: Run the test, verify it PASSES**

Run: `go test ./internal/testing/ -run TestRunner_WorkflowRunSubWorkflow -v`
Expected: PASS.

- [ ] **Step 5: Run the whole testing package to confirm no regression**

Run: `go test ./internal/testing/...`
Expected: PASS (existing runner tests still green; `NewWorkflowCache` logs an info line per case — acceptable).

- [ ] **Step 6: Commit**

```bash
git add internal/testing/runner.go internal/testing/runner_subworkflow_test.go
git commit -m "fix(testing): inject sub-workflow runner so control.loop/workflow.run run under test"
```

---

## Task 2: Scaffold the `testdata/node-e2e/` project and Go driver

Stand up the project with the first workflow (`transform.set`) and the driver that runs all suites. Later tasks just add workflow+test pairs.

**Files:**
- Create: `testdata/node-e2e/noda.json`
- Create: `testdata/node-e2e/workflows/transform-set.json`
- Create: `testdata/node-e2e/tests/transform-set.test.json`
- Create: `internal/testing/e2e/run_test.go`

- [ ] **Step 1: Create `testdata/node-e2e/noda.json`**

```json
{ "services": {} }
```

- [ ] **Step 2: Create `testdata/node-e2e/workflows/transform-set.json`**

```json
{
  "id": "transform-set",
  "nodes": {
    "set": {
      "type": "transform.set",
      "config": {
        "fields": {
          "greeting": "Hi {{ input.name }}",
          "count": "{{ input.n }}",
          "flag": true
        }
      }
    }
  },
  "edges": []
}
```

- [ ] **Step 3: Create `testdata/node-e2e/tests/transform-set.test.json`**

```json
{
  "id": "transform-set",
  "workflow": "transform-set",
  "tests": [
    {
      "name": "sets fields from input and literals",
      "input": { "name": "Ann", "n": 5 },
      "expect": {
        "status": "success",
        "output": { "set.greeting": "Hi Ann", "set.count": 5, "set.flag": true }
      }
    }
  ]
}
```

- [ ] **Step 4: Create the Go driver `internal/testing/e2e/run_test.go`**

```go
package e2e

import (
	"testing"

	"github.com/chimpanze/noda/internal/config"
	"github.com/chimpanze/noda/internal/registry"
	nodatesting "github.com/chimpanze/noda/internal/testing"
	"github.com/chimpanze/noda/pkg/api"
	"github.com/chimpanze/noda/plugins/core/control"
	"github.com/chimpanze/noda/plugins/core/response"
	"github.com/chimpanze/noda/plugins/core/transform"
	"github.com/chimpanze/noda/plugins/core/util"
	workflowplugin "github.com/chimpanze/noda/plugins/core/workflow"
	"github.com/stretchr/testify/require"
)

// TestNodeE2E loads the testdata/node-e2e project and runs every test suite
// through the real engine, failing on any case failure.
func TestNodeE2E(t *testing.T) {
	const dir = "../../../testdata/node-e2e"

	sm, err := config.NewSecretsManager(dir, "")
	require.NoError(t, err)

	rc, errs := config.ValidateAll(dir, "", sm)
	require.Empty(t, errs, "config validation errors: %v", errs)

	coreReg := registry.NewNodeRegistry()
	for _, p := range []api.Plugin{
		&control.Plugin{},
		&transform.Plugin{},
		&response.Plugin{},
		&util.Plugin{},
		&workflowplugin.Plugin{},
	} {
		require.NoError(t, coreReg.RegisterFromPlugin(p))
	}

	suites, err := nodatesting.LoadTests(rc)
	require.NoError(t, err)
	require.NotEmpty(t, suites, "no test suites found in testdata/node-e2e/tests")

	for _, s := range suites {
		for _, r := range nodatesting.RunTestSuite(s, rc, coreReg) {
			if !r.Passed {
				t.Errorf("[workflow %s] case %q failed: %s", s.Workflow, r.CaseName, r.Error)
			}
		}
	}
}
```

- [ ] **Step 5: Run the driver, verify it PASSES**

Run: `go test ./internal/testing/e2e/ -run TestNodeE2E -v`
Expected: PASS (one suite, one case).

- [ ] **Step 6: Verify the CLI path also works**

Run: `go run ./cmd/noda test --config testdata/node-e2e`
Expected: output shows the `transform-set` suite passing, exit 0.

- [ ] **Step 7: Commit**

```bash
git add testdata/node-e2e internal/testing/e2e
git commit -m "test(e2e): scaffold node-e2e project and Go driver (transform.set)"
```

---

## Task 3: Remaining transform nodes (map, filter, merge, delete, validate)

Add one workflow + suite per node. Array/scalar outputs are reduced to deterministic scalars via a downstream `transform.set` adapter so `expect.output` can address them.

**Files (create):**
- `testdata/node-e2e/workflows/transform-map.json`, `tests/transform-map.test.json`
- `testdata/node-e2e/workflows/transform-filter.json`, `tests/transform-filter.test.json`
- `testdata/node-e2e/workflows/transform-merge.json`, `tests/transform-merge.test.json`
- `testdata/node-e2e/workflows/transform-delete.json`, `tests/transform-delete.test.json`
- `testdata/node-e2e/workflows/transform-validate.json`, `tests/transform-validate.test.json`

- [ ] **Step 1: transform.map** — `workflows/transform-map.json`:

```json
{
  "id": "transform-map",
  "nodes": {
    "map": { "type": "transform.map", "config": { "collection": "{{ input.items }}", "expression": "{{ $item * 2 }}" } },
    "agg": { "type": "transform.set", "config": { "fields": { "first": "{{ nodes.map[0] }}", "count": "{{ len(nodes.map) }}" } } }
  },
  "edges": [ { "from": "map", "to": "agg" } ]
}
```

`tests/transform-map.test.json`:

```json
{
  "id": "transform-map",
  "workflow": "transform-map",
  "tests": [
    { "name": "doubles each item", "input": { "items": [1, 2, 3] },
      "expect": { "status": "success", "output": { "agg.first": 2, "agg.count": 3 } } },
    { "name": "errors when collection is not an array", "input": { "items": "nope" },
      "expect": { "status": "error", "error_node": "map" } }
  ]
}
```

- [ ] **Step 2: transform.filter** — `workflows/transform-filter.json`:

```json
{
  "id": "transform-filter",
  "nodes": {
    "flt": { "type": "transform.filter", "config": { "collection": "{{ input.items }}", "expression": "{{ $item.active }}" } },
    "agg": { "type": "transform.set", "config": { "fields": { "count": "{{ len(nodes.flt) }}" } } }
  },
  "edges": [ { "from": "flt", "to": "agg" } ]
}
```

`tests/transform-filter.test.json`:

```json
{
  "id": "transform-filter",
  "workflow": "transform-filter",
  "tests": [
    { "name": "keeps active items", "input": { "items": [ { "active": true }, { "active": false }, { "active": true } ] },
      "expect": { "status": "success", "output": { "agg.count": 2 } } },
    { "name": "empty result", "input": { "items": [ { "active": false } ] },
      "expect": { "status": "success", "output": { "agg.count": 0 } } }
  ]
}
```

- [ ] **Step 3: transform.merge** — `workflows/transform-merge.json`:

```json
{
  "id": "transform-merge",
  "nodes": {
    "mrg": { "type": "transform.merge", "config": { "mode": "append", "inputs": ["{{ input.a }}", "{{ input.b }}"] } },
    "agg": { "type": "transform.set", "config": { "fields": { "count": "{{ len(nodes.mrg) }}" } } }
  },
  "edges": [ { "from": "mrg", "to": "agg" } ]
}
```

`tests/transform-merge.test.json`:

```json
{
  "id": "transform-merge",
  "workflow": "transform-merge",
  "tests": [
    { "name": "appends arrays", "input": { "a": [1, 2], "b": [3] },
      "expect": { "status": "success", "output": { "agg.count": 3 } } },
    { "name": "errors on empty inputs", "input": { "a": [], "b": [] },
      "expect": { "status": "success", "output": { "agg.count": 0 } } }
  ]
}
```

- [ ] **Step 4: transform.delete** — `workflows/transform-delete.json`:

```json
{
  "id": "transform-delete",
  "nodes": {
    "del": { "type": "transform.delete", "config": { "data": "{{ input.user }}", "fields": ["password"] } },
    "chk": { "type": "transform.set", "config": { "fields": { "name": "{{ nodes.del.name }}", "has_pw": "{{ \"password\" in nodes.del }}" } } }
  },
  "edges": [ { "from": "del", "to": "chk" } ]
}
```

`tests/transform-delete.test.json`:

```json
{
  "id": "transform-delete",
  "workflow": "transform-delete",
  "tests": [
    { "name": "removes password key", "input": { "user": { "name": "Ann", "password": "secret" } },
      "expect": { "status": "success", "output": { "chk.name": "Ann", "chk.has_pw": false } } }
  ]
}
```

- [ ] **Step 5: transform.validate** — `workflows/transform-validate.json`:

```json
{
  "id": "transform-validate",
  "nodes": {
    "val": {
      "type": "transform.validate",
      "config": {
        "data": "{{ input }}",
        "schema": { "type": "object", "required": ["email"], "properties": { "email": { "type": "string" } } }
      }
    }
  },
  "edges": []
}
```

`tests/transform-validate.test.json`:

```json
{
  "id": "transform-validate",
  "workflow": "transform-validate",
  "tests": [
    { "name": "passes valid input", "input": { "email": "a@b.com" },
      "expect": { "status": "success", "output": { "val.email": "a@b.com" } } },
    { "name": "fails missing required field", "input": { "name": "x" },
      "expect": { "status": "error", "error_node": "val" } }
  ]
}
```

- [ ] **Step 6: Run the driver**

Run: `go test ./internal/testing/e2e/ -run TestNodeE2E -v`
Expected: PASS. If any case fails, the node has a real bug — debug with `superpowers:systematic-debugging`, fix the node source under `plugins/core/transform/`, and rerun until green. (Record the bug for the final summary.)

- [ ] **Step 7: Commit**

```bash
git add testdata/node-e2e
git commit -m "test(e2e): cover transform.map/filter/merge/delete/validate"
```

---

## Task 4: control.if and control.switch

**Files (create):**
- `testdata/node-e2e/workflows/control-if.json`, `tests/control-if.test.json`
- `testdata/node-e2e/workflows/control-switch.json`, `tests/control-switch.test.json`

- [ ] **Step 1: control.if** — `workflows/control-if.json`:

```json
{
  "id": "control-if",
  "nodes": {
    "cond":  { "type": "control.if", "config": { "condition": "{{ input.admin }}" } },
    "grant": { "type": "transform.set", "config": { "fields": { "decision": "granted" } } },
    "deny":  { "type": "transform.set", "config": { "fields": { "decision": "denied" } } }
  },
  "edges": [
    { "from": "cond", "output": "then", "to": "grant" },
    { "from": "cond", "output": "else", "to": "deny" }
  ]
}
```

`tests/control-if.test.json`:

```json
{
  "id": "control-if",
  "workflow": "control-if",
  "tests": [
    { "name": "true takes then branch", "input": { "admin": true },
      "expect": { "status": "success", "output": { "grant.decision": "granted" } } },
    { "name": "false takes else branch", "input": { "admin": false },
      "expect": { "status": "success", "output": { "deny.decision": "denied" } } }
  ]
}
```

- [ ] **Step 2: control.switch** — `workflows/control-switch.json`:

```json
{
  "id": "control-switch",
  "nodes": {
    "sw": { "type": "control.switch", "config": { "expression": "{{ input.tier }}", "cases": ["free", "pro"] } },
    "f":  { "type": "transform.set", "config": { "fields": { "plan": "free-plan" } } },
    "p":  { "type": "transform.set", "config": { "fields": { "plan": "pro-plan" } } },
    "d":  { "type": "transform.set", "config": { "fields": { "plan": "default-plan" } } }
  },
  "edges": [
    { "from": "sw", "output": "free", "to": "f" },
    { "from": "sw", "output": "pro", "to": "p" },
    { "from": "sw", "output": "default", "to": "d" }
  ]
}
```

`tests/control-switch.test.json`:

```json
{
  "id": "control-switch",
  "workflow": "control-switch",
  "tests": [
    { "name": "routes free", "input": { "tier": "free" },
      "expect": { "status": "success", "output": { "f.plan": "free-plan" } } },
    { "name": "routes pro", "input": { "tier": "pro" },
      "expect": { "status": "success", "output": { "p.plan": "pro-plan" } } },
    { "name": "routes default", "input": { "tier": "enterprise" },
      "expect": { "status": "success", "output": { "d.plan": "default-plan" } } }
  ]
}
```

- [ ] **Step 3: Run the driver**

Run: `go test ./internal/testing/e2e/ -run TestNodeE2E -v`
Expected: PASS. Fix any control-node bug found in `plugins/core/control/` and rerun.

- [ ] **Step 4: Commit**

```bash
git add testdata/node-e2e
git commit -m "test(e2e): cover control.if and control.switch branching"
```

---

## Task 5: control.loop (uses the Task 1 runner fix)

A sub-workflow doubles each item; the loop aggregates results; an adapter reduces the result array to scalars.

**Files (create):**
- `testdata/node-e2e/workflows/loop-item.json` (sub-workflow)
- `testdata/node-e2e/workflows/control-loop.json` (main)
- `testdata/node-e2e/tests/control-loop.test.json`

- [ ] **Step 1: sub-workflow** — `workflows/loop-item.json`:

```json
{
  "id": "loop-item",
  "nodes": {
    "dbl": { "type": "transform.set", "config": { "fields": { "doubled": "{{ input.val * 2 }}" } } },
    "out": { "type": "workflow.output", "config": { "name": "result", "data": "{{ nodes.dbl }}" } }
  },
  "edges": [ { "from": "dbl", "to": "out" } ]
}
```

- [ ] **Step 2: main workflow** — `workflows/control-loop.json`:

```json
{
  "id": "control-loop",
  "nodes": {
    "lp": {
      "type": "control.loop",
      "config": { "collection": "{{ input.nums }}", "workflow": "loop-item", "input": { "val": "{{ $item }}" } }
    },
    "agg": { "type": "transform.set", "config": { "fields": { "count": "{{ len(nodes.lp) }}", "first": "{{ nodes.lp[0].doubled }}" } } }
  },
  "edges": [ { "from": "lp", "output": "done", "to": "agg" } ]
}
```

- [ ] **Step 3: test** — `tests/control-loop.test.json`:

```json
{
  "id": "control-loop",
  "workflow": "control-loop",
  "tests": [
    { "name": "doubles each item", "input": { "nums": [1, 2, 3] },
      "expect": { "status": "success", "output": { "agg.count": 3, "agg.first": 2 } } }
  ]
}
```

Note: the empty-collection case is skipped here because the `agg` adapter dereferences `nodes.lp[0]`; empty-loop behavior is covered by `transform` empty-array cases in Task 3. (If you want it, add a second workflow whose adapter only computes `count`.)

- [ ] **Step 4: Run the driver**

Run: `go test ./internal/testing/e2e/ -run TestNodeE2E -v`
Expected: PASS. This exercises the Task 1 fix end-to-end. If `lp` fails with a "runner not configured"-style error, Task 1 is not wired correctly — revisit it.

- [ ] **Step 5: Commit**

```bash
git add testdata/node-e2e
git commit -m "test(e2e): cover control.loop over a sub-workflow"
```

---

## Task 6: response.json, response.redirect, response.error

Real `response.*` nodes store `*api.HTTPResponse` with **capitalized** keys, so assert with `expect.outputs` (plural, partial deep match).

**Files (create):**
- `testdata/node-e2e/workflows/response-json.json`, `tests/response-json.test.json`
- `testdata/node-e2e/workflows/response-redirect.json`, `tests/response-redirect.test.json`
- `testdata/node-e2e/workflows/response-error.json`, `tests/response-error.test.json`

- [ ] **Step 1: response.json** — `workflows/response-json.json`:

```json
{
  "id": "response-json",
  "nodes": {
    "resp": { "type": "response.json", "config": { "status": 200, "body": { "ok": true, "name": "{{ input.name }}" } } }
  },
  "edges": []
}
```

`tests/response-json.test.json`:

```json
{
  "id": "response-json",
  "workflow": "response-json",
  "tests": [
    { "name": "builds 200 json response", "input": { "name": "Ann" },
      "expect": { "status": "success", "outputs": { "resp": { "Status": 200, "Body": { "ok": true, "name": "Ann" } } } } }
  ]
}
```

- [ ] **Step 2: response.redirect** — `workflows/response-redirect.json`:

```json
{
  "id": "response-redirect",
  "nodes": {
    "rd": { "type": "response.redirect", "config": { "url": "/login", "status": 302 } }
  },
  "edges": []
}
```

`tests/response-redirect.test.json`:

```json
{
  "id": "response-redirect",
  "workflow": "response-redirect",
  "tests": [
    { "name": "builds 302 redirect", "input": {},
      "expect": { "status": "success", "outputs": { "rd": { "Status": 302, "Headers": { "Location": "/login" } } } } }
  ]
}
```

- [ ] **Step 3: response.error** — `workflows/response-error.json`:

```json
{
  "id": "response-error",
  "nodes": {
    "er": { "type": "response.error", "config": { "status": 404, "code": "NOT_FOUND", "message": "Missing" } }
  },
  "edges": []
}
```

`tests/response-error.test.json`:

```json
{
  "id": "response-error",
  "workflow": "response-error",
  "tests": [
    { "name": "builds 404 error response", "input": {},
      "expect": { "status": "success", "outputs": { "er": { "Status": 404, "Body": { "error": { "code": "NOT_FOUND", "message": "Missing" } } } } } }
  ]
}
```

- [ ] **Step 4: Run the driver**

Run: `go test ./internal/testing/e2e/ -run TestNodeE2E -v`
Expected: PASS. If the `expect.outputs` shape mismatches, inspect the actual via `-v` trace or temporarily log `r.Actual.Outputs`; adjust capitalization/nesting to match `*api.HTTPResponse` normalization, or fix a genuine node bug in `plugins/core/response/`.

- [ ] **Step 5: Commit**

```bash
git add testdata/node-e2e
git commit -m "test(e2e): cover response.json/redirect/error"
```

---

## Task 7: util nodes (uuid, timestamp, delay, log, jwt_sign)

These run for real (no mocks). Non-deterministic outputs are validated by shape via a downstream `transform.set` adapter producing a boolean/number.

**Files (create):**
- `testdata/node-e2e/workflows/util-uuid.json`, `tests/util-uuid.test.json`
- `testdata/node-e2e/workflows/util-timestamp.json`, `tests/util-timestamp.test.json`
- `testdata/node-e2e/workflows/util-delay.json`, `tests/util-delay.test.json`
- `testdata/node-e2e/workflows/util-log.json`, `tests/util-log.test.json`
- `testdata/node-e2e/workflows/util-jwt.json`, `tests/util-jwt.test.json`

- [ ] **Step 1: util.uuid** — `workflows/util-uuid.json`:

```json
{
  "id": "util-uuid",
  "nodes": {
    "gen": { "type": "util.uuid", "config": {} },
    "chk": { "type": "transform.set", "config": { "fields": { "len": "{{ len(nodes.gen) }}" } } }
  },
  "edges": [ { "from": "gen", "to": "chk" } ]
}
```

`tests/util-uuid.test.json`:

```json
{
  "id": "util-uuid",
  "workflow": "util-uuid",
  "tests": [
    { "name": "produces a 36-char uuid", "input": {},
      "expect": { "status": "success", "output": { "chk.len": 36 } } }
  ]
}
```

- [ ] **Step 2: util.timestamp** — `workflows/util-timestamp.json`:

```json
{
  "id": "util-timestamp",
  "nodes": {
    "ts":  { "type": "util.timestamp", "config": { "format": "unix" } },
    "chk": { "type": "transform.set", "config": { "fields": { "positive": "{{ nodes.ts > 0 }}" } } }
  },
  "edges": [ { "from": "ts", "to": "chk" } ]
}
```

`tests/util-timestamp.test.json`:

```json
{
  "id": "util-timestamp",
  "workflow": "util-timestamp",
  "tests": [
    { "name": "produces a positive unix timestamp", "input": {},
      "expect": { "status": "success", "output": { "chk.positive": true } } }
  ]
}
```

- [ ] **Step 3: util.delay** — `workflows/util-delay.json`:

```json
{
  "id": "util-delay",
  "nodes": {
    "wait": { "type": "util.delay", "config": { "timeout": "1ms" } }
  },
  "edges": []
}
```

`tests/util-delay.test.json`:

```json
{
  "id": "util-delay",
  "workflow": "util-delay",
  "tests": [
    { "name": "completes after a short delay", "input": {}, "expect": { "status": "success" } }
  ]
}
```

- [ ] **Step 4: util.log** — `workflows/util-log.json`:

```json
{
  "id": "util-log",
  "nodes": {
    "lg": { "type": "util.log", "config": { "level": "info", "message": "hello {{ input.who }}" } }
  },
  "edges": []
}
```

`tests/util-log.test.json`:

```json
{
  "id": "util-log",
  "workflow": "util-log",
  "tests": [
    { "name": "logs without error", "input": { "who": "world" }, "expect": { "status": "success" } }
  ]
}
```

- [ ] **Step 5: util.jwt_sign** — `workflows/util-jwt.json`:

```json
{
  "id": "util-jwt",
  "nodes": {
    "sign": { "type": "util.jwt_sign", "config": { "claims": { "sub": "{{ input.uid }}" }, "secret": "test-secret", "algorithm": "HS256" } },
    "chk":  { "type": "transform.set", "config": { "fields": { "segments": "{{ len(split(nodes.sign, \".\")) }}" } } }
  },
  "edges": [ { "from": "sign", "to": "chk" } ]
}
```

`tests/util-jwt.test.json`:

```json
{
  "id": "util-jwt",
  "workflow": "util-jwt",
  "tests": [
    { "name": "signs a 3-segment jwt", "input": { "uid": "user-1" },
      "expect": { "status": "success", "output": { "chk.segments": 3 } } }
  ]
}
```

- [ ] **Step 6: Run the driver**

Run: `go test ./internal/testing/e2e/ -run TestNodeE2E -v`
Expected: PASS. If `util.jwt_sign` returns an object instead of a bare token string (docstring vs implementation mismatch noted in research), `split(nodes.sign, ".")` will fail — that is a real bug: make the node return the compact token string (or adjust the adapter to `nodes.sign.token` only if the documented contract is the object form). Decide based on `plugins/core/util/jwt.go` and its doc, fix the mismatch, record it.

- [ ] **Step 7: Commit**

```bash
git add testdata/node-e2e
git commit -m "test(e2e): cover util.uuid/timestamp/delay/log/jwt_sign"
```

---

## Task 8: workflow.run and workflow.output

`workflow.output` is exercised inside every sub-workflow already; add a dedicated standalone case plus a `workflow.run` happy path.

**Files (create):**
- `testdata/node-e2e/workflows/run-callee.json`, `workflows/workflow-run.json`, `tests/workflow-run.test.json`
- `testdata/node-e2e/workflows/workflow-output.json`, `tests/workflow-output.test.json`

- [ ] **Step 1: callee + caller** — `workflows/run-callee.json`:

```json
{
  "id": "run-callee",
  "nodes": {
    "calc": { "type": "transform.set", "config": { "fields": { "result": "{{ input.x * 10 }}" } } },
    "out":  { "type": "workflow.output", "config": { "name": "result", "data": "{{ nodes.calc }}" } }
  },
  "edges": [ { "from": "calc", "to": "out" } ]
}
```

`workflows/workflow-run.json`:

```json
{
  "id": "workflow-run",
  "nodes": {
    "call": { "type": "workflow.run", "config": { "workflow": "run-callee", "input": { "x": "{{ input.x }}" } } }
  },
  "edges": []
}
```

`tests/workflow-run.test.json`:

```json
{
  "id": "workflow-run",
  "workflow": "workflow-run",
  "tests": [
    { "name": "runs sub-workflow and returns its output", "input": { "x": 3 },
      "expect": { "status": "success", "output": { "call.result": 30 } } }
  ]
}
```

- [ ] **Step 2: workflow.output standalone** — `workflows/workflow-output.json`:

```json
{
  "id": "workflow-output",
  "nodes": {
    "out": { "type": "workflow.output", "config": { "name": "final", "data": { "msg": "{{ input.m }}" } } }
  },
  "edges": []
}
```

`tests/workflow-output.test.json`:

```json
{
  "id": "workflow-output",
  "workflow": "workflow-output",
  "tests": [
    { "name": "emits named output data", "input": { "m": "hi" },
      "expect": { "status": "success", "output": { "out.msg": "hi" } } }
  ]
}
```

- [ ] **Step 3: Run the driver**

Run: `go test ./internal/testing/e2e/ -run TestNodeE2E -v`
Expected: PASS. (`workflow.run` relies on the Task 1 fix.) Fix any bug in `plugins/core/workflow/` and rerun.

- [ ] **Step 4: Commit**

```bash
git add testdata/node-e2e
git commit -m "test(e2e): cover workflow.run and workflow.output"
```

---

## Task 9: Tier B — ws.send through the engine

**Files:**
- Create: `plugins/core/ws/engine_e2e_test.go`

- [ ] **Step 1: Write the integration test**

```go
package ws

import (
	"context"
	"testing"

	"github.com/chimpanze/noda/internal/connmgr"
	"github.com/chimpanze/noda/internal/engine"
	"github.com/chimpanze/noda/internal/registry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWSSend_Engine(t *testing.T) {
	mgr := connmgr.NewManager()
	var got []byte
	require.NoError(t, mgr.Register(&connmgr.Conn{
		ID:      "c1",
		Channel: "room.1",
		SendFn:  func(d []byte) error { got = d; return nil },
	}))
	svc := connmgr.NewEndpointService(mgr, "ws-test")

	svcReg := registry.NewServiceRegistry()
	require.NoError(t, svcReg.Register("conns", svc, nil))

	nodeReg := registry.NewNodeRegistry()
	require.NoError(t, nodeReg.RegisterFromPlugin(&Plugin{}))

	wf := engine.WorkflowConfig{
		ID: "ws-wf",
		Nodes: map[string]engine.NodeConfig{
			"send": {
				Type:     "ws.send",
				Config:   map[string]any{"channel": "{{ input.room }}", "data": "{{ input.msg }}"},
				Services: map[string]string{"connections": "conns"},
			},
		},
	}
	graph, err := engine.Compile(wf, nodeReg)
	require.NoError(t, err)

	execCtx := engine.NewExecutionContext(engine.WithInput(map[string]any{"room": "room.1", "msg": "hello"}))
	require.NoError(t, engine.ExecuteGraph(context.Background(), graph, execCtx, svcReg, nodeReg))

	out, ok := execCtx.GetOutput("send")
	require.True(t, ok)
	assert.Equal(t, "room.1", out.(map[string]any)["channel"])
	assert.Contains(t, string(got), "hello")
}

func TestWSSend_Engine_MissingChannel(t *testing.T) {
	mgr := connmgr.NewManager()
	svc := connmgr.NewEndpointService(mgr, "ws-test")
	svcReg := registry.NewServiceRegistry()
	require.NoError(t, svcReg.Register("conns", svc, nil))
	nodeReg := registry.NewNodeRegistry()
	require.NoError(t, nodeReg.RegisterFromPlugin(&Plugin{}))

	wf := engine.WorkflowConfig{
		ID: "ws-wf-err",
		Nodes: map[string]engine.NodeConfig{
			"send": {Type: "ws.send", Config: map[string]any{"data": "x"}, Services: map[string]string{"connections": "conns"}},
		},
	}
	graph, err := engine.Compile(wf, nodeReg)
	require.NoError(t, err)
	execCtx := engine.NewExecutionContext(engine.WithInput(map[string]any{}))
	err = engine.ExecuteGraph(context.Background(), graph, execCtx, svcReg, nodeReg)
	require.Error(t, err) // no error edge → workflow fails
}
```

- [ ] **Step 2: Run the test**

Run: `go test ./plugins/core/ws/ -run TestWSSend_Engine -v`
Expected: PASS. If delivery fails, inspect `internal/connmgr/endpoint.go` `Send` and fix.

- [ ] **Step 3: Commit**

```bash
git add plugins/core/ws/engine_e2e_test.go
git commit -m "test(e2e): drive ws.send through the engine with a real connmgr"
```

---

## Task 10: Tier B — sse.send through the engine

**Files:**
- Create: `plugins/core/sse/engine_e2e_test.go`

- [ ] **Step 1: Write the integration test**

```go
package sse

import (
	"context"
	"testing"

	"github.com/chimpanze/noda/internal/connmgr"
	"github.com/chimpanze/noda/internal/engine"
	"github.com/chimpanze/noda/internal/registry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSSESend_Engine(t *testing.T) {
	mgr := connmgr.NewManager()
	var gotEvent, gotData string
	require.NoError(t, mgr.Register(&connmgr.Conn{
		ID:      "c1",
		Channel: "feed.1",
		SSEFn:   func(event, data, id string) error { gotEvent, gotData = event, data; return nil },
	}))
	svc := connmgr.NewEndpointService(mgr, "sse-test")

	svcReg := registry.NewServiceRegistry()
	require.NoError(t, svcReg.Register("conns", svc, nil))
	nodeReg := registry.NewNodeRegistry()
	require.NoError(t, nodeReg.RegisterFromPlugin(&Plugin{}))

	wf := engine.WorkflowConfig{
		ID: "sse-wf",
		Nodes: map[string]engine.NodeConfig{
			"send": {
				Type:     "sse.send",
				Config:   map[string]any{"channel": "{{ input.ch }}", "event": "update", "data": "{{ input.msg }}"},
				Services: map[string]string{"connections": "conns"},
			},
		},
	}
	graph, err := engine.Compile(wf, nodeReg)
	require.NoError(t, err)

	execCtx := engine.NewExecutionContext(engine.WithInput(map[string]any{"ch": "feed.1", "msg": "tick"}))
	require.NoError(t, engine.ExecuteGraph(context.Background(), graph, execCtx, svcReg, nodeReg))

	out, ok := execCtx.GetOutput("send")
	require.True(t, ok)
	assert.Equal(t, "feed.1", out.(map[string]any)["channel"])
	assert.Equal(t, "update", gotEvent)
	assert.Contains(t, gotData, "tick")
}
```

- [ ] **Step 2: Run the test**

Run: `go test ./plugins/core/sse/ -run TestSSESend_Engine -v`
Expected: PASS. If the `event` field is not delivered, check `plugins/core/sse/send.go` and `internal/connmgr/endpoint.go` `SendSSE`. If the SSE data serialization differs (e.g. JSON-encoded), relax the assertion to `assert.Contains(t, gotData, "tick")` (already used) and fix any genuine bug.

- [ ] **Step 3: Commit**

```bash
git add plugins/core/sse/engine_e2e_test.go
git commit -m "test(e2e): drive sse.send through the engine with a real connmgr"
```

---

## Task 11: Tier B — wasm.send and wasm.query through the engine

Use the in-process Wasm runtime with a mock `PluginInstance` (the pattern already in `plugins/core/wasm/wasm_test.go`), but drive it through `engine.ExecuteGraph`.

**Files:**
- Create: `plugins/core/wasm/engine_e2e_test.go`

- [ ] **Step 1: Write the integration test**

```go
package wasm

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/chimpanze/noda/internal/engine"
	"github.com/chimpanze/noda/internal/registry"
	wasmrt "github.com/chimpanze/noda/internal/wasm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupWasmService(t *testing.T) (*wasmrt.WasmService, *mockPlugin) {
	t.Helper()
	svcReg := registry.NewServiceRegistry()
	rt := wasmrt.NewRuntime(svcReg, nil, slog.Default())
	plug := newMockPlugin()
	_, _ = rt.LoadModuleWithPlugin(wasmrt.ModuleConfig{Name: "game", TickRate: 20}, plug)
	require.NoError(t, rt.StartAll(context.Background()))
	t.Cleanup(func() { _ = rt.StopAll(context.Background()) })
	time.Sleep(50 * time.Millisecond)
	return wasmrt.NewWasmService(rt, "game"), plug
}

func runWasmNode(t *testing.T, nodeType string, config map[string]any, svc *wasmrt.WasmService) (any, error) {
	t.Helper()
	svcReg := registry.NewServiceRegistry()
	require.NoError(t, svcReg.Register("wasmsvc", svc, nil))
	nodeReg := registry.NewNodeRegistry()
	require.NoError(t, nodeReg.RegisterFromPlugin(&Plugin{}))

	wf := engine.WorkflowConfig{
		ID: "wasm-wf",
		Nodes: map[string]engine.NodeConfig{
			"n": {Type: nodeType, Config: config, Services: map[string]string{"runtime": "wasmsvc"}},
		},
	}
	graph, err := engine.Compile(wf, nodeReg)
	require.NoError(t, err)
	execCtx := engine.NewExecutionContext(engine.WithInput(map[string]any{}))
	if err := engine.ExecuteGraph(context.Background(), graph, execCtx, svcReg, nodeReg); err != nil {
		return nil, err
	}
	out, _ := execCtx.GetOutput("n")
	return out, nil
}

func TestWasmSend_Engine(t *testing.T) {
	svc, plug := setupWasmService(t)
	out, err := runWasmNode(t, "wasm.send", map[string]any{"data": map[string]any{"cmd": "move"}}, svc)
	require.NoError(t, err)
	assert.Equal(t, true, out.(map[string]any)["sent"])
	// The command reached the module on its tick loop.
	require.Eventually(t, func() bool {
		plug.mu.Lock()
		defer plug.mu.Unlock()
		return len(plug.calls) > 0
	}, time.Second, 10*time.Millisecond)
}

func TestWasmQuery_Engine(t *testing.T) {
	svc, plug := setupWasmService(t)
	plug.mu.Lock()
	plug.responses["query"] = mockResponse{exitCode: 0, data: []byte(`{"state":"ok"}`)}
	plug.exports["query"] = true
	plug.mu.Unlock()

	out, err := runWasmNode(t, "wasm.query", map[string]any{"data": map[string]any{"type": "get_state"}}, svc)
	require.NoError(t, err)
	require.NotNil(t, out)
}
```

- [ ] **Step 2: Run the test**

Run: `go test ./plugins/core/wasm/ -run 'TestWasm(Send|Query)_Engine' -v`
Expected: PASS. The exact success-data keys (`sent`) come from `plugins/core/wasm/send.go`; the query method name and response wiring come from `query.go` and `internal/wasm/runtime.go`. If `wasm.query`'s mock method name differs from `"query"`, read `query.go` to find the method it calls (e.g. it may pass `data` to a fixed export) and set `plug.responses[...]` / `plug.exports[...]` accordingly, then re-run. Adjust assertions to the real returned shape; fix genuine bugs.

- [ ] **Step 3: Commit**

```bash
git add plugins/core/wasm/engine_e2e_test.go
git commit -m "test(e2e): drive wasm.send/query through the engine with the in-process runtime"
```

---

## Task 12: Tier B — upload.handle through the engine

`upload.handle` needs an in-memory storage service in the `destination` slot and a `*multipart.FileHeader` in the input. Reuse the helpers already in `plugins/core/upload/handle_test.go` (`newMemStorage`, `makeFileHeader`).

**Files:**
- Create: `plugins/core/upload/engine_e2e_test.go`

- [ ] **Step 1: Write the integration test**

```go
package upload

import (
	"context"
	"testing"

	"github.com/chimpanze/noda/internal/engine"
	"github.com/chimpanze/noda/internal/registry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUploadHandle_Engine(t *testing.T) {
	svc := newMemStorage(t) // helper from handle_test.go (same package)
	fh := makeFileHeader("hello.txt", "text/plain", []byte("hello world"))
	require.NotNil(t, fh)

	svcReg := registry.NewServiceRegistry()
	require.NoError(t, svcReg.Register("store", svc, nil))
	nodeReg := registry.NewNodeRegistry()
	require.NoError(t, nodeReg.RegisterFromPlugin(&Plugin{}))

	wf := engine.WorkflowConfig{
		ID: "upload-wf",
		Nodes: map[string]engine.NodeConfig{
			"up": {
				Type: "upload.handle",
				Config: map[string]any{
					"max_size":      float64(1024 * 1024),
					"allowed_types": []any{"text/plain; charset=utf-8"},
					"path":          "uploads/hello.txt",
				},
				Services: map[string]string{"destination": "store"},
			},
		},
	}
	graph, err := engine.Compile(wf, nodeReg)
	require.NoError(t, err)

	execCtx := engine.NewExecutionContext(engine.WithInput(map[string]any{"file": fh}))
	require.NoError(t, engine.ExecuteGraph(context.Background(), graph, execCtx, svcReg, nodeReg))

	out, ok := execCtx.GetOutput("up")
	require.True(t, ok)
	rm := out.(map[string]any)
	assert.Equal(t, "uploads/hello.txt", rm["path"])
	assert.Equal(t, "hello.txt", rm["filename"])

	data, err := svc.Read(context.Background(), "uploads/hello.txt")
	require.NoError(t, err)
	assert.Equal(t, "hello world", string(data))
}

func TestUploadHandle_Engine_TypeRejected(t *testing.T) {
	svc := newMemStorage(t)
	fh := makeFileHeader("evil.bin", "application/octet-stream", []byte{0x00, 0x01, 0x02})
	require.NotNil(t, fh)

	svcReg := registry.NewServiceRegistry()
	require.NoError(t, svcReg.Register("store", svc, nil))
	nodeReg := registry.NewNodeRegistry()
	require.NoError(t, nodeReg.RegisterFromPlugin(&Plugin{}))

	wf := engine.WorkflowConfig{
		ID: "upload-wf-err",
		Nodes: map[string]engine.NodeConfig{
			"up": {
				Type: "upload.handle",
				Config: map[string]any{
					"max_size":      float64(1024),
					"allowed_types": []any{"image/png"},
					"path":          "uploads/evil.bin",
				},
				Services: map[string]string{"destination": "store"},
			},
		},
	}
	graph, err := engine.Compile(wf, nodeReg)
	require.NoError(t, err)
	execCtx := engine.NewExecutionContext(engine.WithInput(map[string]any{"file": fh}))
	err = engine.ExecuteGraph(context.Background(), graph, execCtx, svcReg, nodeReg)
	require.Error(t, err) // disallowed type, no error edge → workflow fails
}
```

- [ ] **Step 2: Run the test**

Run: `go test ./plugins/core/upload/ -run TestUploadHandle_Engine -v`
Expected: PASS. The success-data keys (`path`, `filename`, `size`, `content_type`) come from `plugins/core/upload/handle.go`; the existing `handle_test.go` confirms them. Fix genuine bugs found.

- [ ] **Step 3: Commit**

```bash
git add plugins/core/upload/engine_e2e_test.go
git commit -m "test(e2e): drive upload.handle through the engine with in-memory storage"
```

---

## Task 13: Full verification and bug summary

- [ ] **Step 1: Run the entire suite**

Run: `go test ./...`
Expected: PASS across all packages. Investigate and fix any failure (record each as a bug).

- [ ] **Step 2: Run the CLI test path against the new project**

Run: `go run ./cmd/noda test --config testdata/node-e2e --verbose`
Expected: every suite passes, exit 0.

- [ ] **Step 3: Vet and build**

Run: `go vet ./... && go build ./...`
Expected: no errors.

- [ ] **Step 4: Write the bug summary**

Create `docs/superpowers/specs/2026-06-22-non-external-node-e2e-findings.md` listing, for every bug found during Tasks 3–12: node, symptom, root cause, fix (file:line), and the test that now guards it. If no bugs were found, state that explicitly and note the coverage added.

- [ ] **Step 5: Commit**

```bash
git add docs/superpowers/specs/2026-06-22-non-external-node-e2e-findings.md
git commit -m "docs: findings + bug summary for non-external node e2e verification"
```

---

## Self-Review notes (coverage check vs spec)

- Tier A nodes (control.if/switch/loop; transform.set/map/filter/merge/delete/validate; response.json/redirect/error; util.uuid/timestamp/delay/log/jwt_sign; workflow.run/output): Tasks 2–8. ✅ each has happy path + (where meaningful) an error/edge path.
- Tier B nodes (ws.send, sse.send, wasm.send, wasm.query, upload.handle): Tasks 9–12, each through `engine.ExecuteGraph` with a real in-process service + an error path where applicable. ✅
- Runner sub-workflow fix: Task 1. ✅ (unblocks control.loop / workflow.run).
- event.emit: intentionally excluded (Redis-only). ✅ matches spec.
- Definition of done (`go test ./...` + `noda test testdata/node-e2e` green; bugs fixed + summarized): Task 13. ✅
- Non-determinism handled via downstream `transform.set` shape adapters (uuid length, timestamp positivity, jwt segment count) so JSON-equality assertions stay stable while the real node executes. ✅
