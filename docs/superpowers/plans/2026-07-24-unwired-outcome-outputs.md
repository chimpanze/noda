# Unwired Outcome Outputs Fail Loudly (#442) — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A node output that reports an operation outcome (`exists` / `invalid` / `not_found`) can no longer be silently unwired: validation/boot rejects it, the 202 fallback can't mask a contract-declaring route, and every shipped/generated workflow wires its outcome outputs.

**Architecture:** New optional `api.OutcomeOutputsProvider` interface on `NodeDescriptor` (mirrors `NodeOutputSchemaProvider`); a new check in `registry.ValidateStartupDryRun` (single insertion point — CLI validate, boot, dev-mode, editor, MCP, and `TestShippedProjectsValidate` all funnel through it); a nil-check guard in `awaitWorkflowResponse`'s `writeAccepted`; mechanical wiring of 20 shipped sites + the CRUD generator.

**Tech Stack:** Go, testify, existing registry/server unit-test patterns. No new dependencies.

**Spec:** `docs/superpowers/specs/2026-07-24-unwired-outcome-outputs-design.md` (approved). Read it first.

## Global Constraints

- Severity decision (user): unwired outcome output is a **hard validation error**, no warning mode, no config escape hatch.
- The 8 declaring node types and their outcome outputs, exactly: `db.create`/`db.update`/`db.upsert`/`auth.create_user` → `["exists"]`; `auth.get_user` → `["not_found"]`; `auth.set_password`/`auth.verify_credentials`/`auth.consume_token` → `["invalid"]`. Control-flow outputs (`then`/`else`/`default`/`done`) must NOT be declared.
- Validation error message format (exact, used in code and docs):
  `workflow %q, node %q (%s): outcome output %q has no outbound edge — a fired outcome output with no edge silently ends the path; wire it (e.g. to an error response, or to the same target as "success" if the distinction does not matter)`
- Work in a worktree branched off **origin/main after a fetch** (never local main): `git fetch origin && git worktree add .worktrees/unwired-outcome-outputs -b fix/unwired-outcome-outputs origin/main`. Every commit runs from the worktree root — verify with `git rev-parse --show-toplevel` before committing.
- Pre-commit gate for every task, run from the **worktree root**: `gofmt -l .` (must print nothing), `go vet ./...`, `go vet -tags integration ./...`, and the task's tests. CI's golangci-lint fails on formatting alone.
- gopls diagnostics inside `.worktrees/` are noise; trust `go build`/`go test` only.
- Commit messages end with: `Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>`
- JSON example files: preserve existing formatting style; edit as text (2-space indent, key order preserved) — do NOT round-trip through `json.dump`/jq (loses formatting; bit a previous tranche).

---

### Task 1: `api.OutcomeOutputsProvider` contract + 8 descriptor implementations + audit test

**Files:**
- Modify: `pkg/api/node.go` (after `NodeOutputSchemaProvider`, ~line 24)
- Modify: `plugins/db/create.go`, `plugins/db/update.go`, `plugins/db/upsert.go`
- Modify: `plugins/auth/create_user.go`, `plugins/auth/get_user.go`, `plugins/auth/set_password.go`, `plugins/auth/verify_credentials.go`, `plugins/auth/one_time_tokens.go`
- Test: `internal/registry/outcome_outputs_audit_test.go` (new)

**Interfaces:**
- Produces: `api.OutcomeOutputsProvider` with method `OutcomeOutputs() []string` — Tasks 2's validator type-asserts `desc.(api.OutcomeOutputsProvider)`.

- [ ] **Step 1: Write the failing audit test**

Create `internal/registry/outcome_outputs_audit_test.go`. External `registry_test` package so it can import `plugins/all` without an import cycle — same rationale as `service_schema_audit_test.go` (read its header comment and mirror it):

```go
package registry_test

import (
	"testing"

	"github.com/chimpanze/noda/internal/registry"
	"github.com/chimpanze/noda/pkg/api"
	"github.com/chimpanze/noda/plugins/all"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestOutcomeOutputsAudit checks, for every node type the runtime registers
// (plugins/all — the same list cmd/noda consumes, #384):
//  1. the 8 node types with a documented outcome output declare it via
//     api.OutcomeOutputsProvider, exactly;
//  2. any provider's declared outcome outputs are a subset of the node's
//     Outputs() and never "success"/"error" (those have their own rules).
func TestOutcomeOutputsAudit(t *testing.T) {
	nodes := registry.NewNodeRegistry()
	for _, p := range all.All() {
		require.NoError(t, nodes.RegisterFromPlugin(p))
	}

	expected := map[string][]string{
		"db.create":               {"exists"},
		"db.update":               {"exists"},
		"db.upsert":               {"exists"},
		"auth.create_user":        {"exists"},
		"auth.get_user":           {"not_found"},
		"auth.set_password":       {"invalid"},
		"auth.verify_credentials": {"invalid"},
		"auth.consume_token":      {"invalid"},
	}

	for _, nodeType := range nodes.AllTypes() {
		desc, ok := nodes.GetDescriptor(nodeType)
		require.True(t, ok, nodeType)
		provider, isProvider := desc.(api.OutcomeOutputsProvider)

		if want, shouldDeclare := expected[nodeType]; shouldDeclare {
			require.True(t, isProvider, "node %q must implement OutcomeOutputsProvider", nodeType)
			assert.Equal(t, want, provider.OutcomeOutputs(), "node %q outcome outputs", nodeType)
		}
		if !isProvider {
			continue
		}
		outputs, ok := nodes.OutputsForType(nodeType)
		require.True(t, ok, nodeType)
		for _, oo := range provider.OutcomeOutputs() {
			assert.Contains(t, outputs, oo, "node %q declares outcome output %q not in Outputs()", nodeType, oo)
			assert.NotEqual(t, "success", oo, "node %q: success is not an outcome output", nodeType)
			assert.NotEqual(t, "error", oo, "node %q: error has its own unwired rule in the engine", nodeType)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/registry/ -run TestOutcomeOutputsAudit -v`
Expected: FAIL — `api.OutcomeOutputsProvider` undefined (compile error).

- [ ] **Step 3: Add the interface to `pkg/api/node.go`**

Directly below the `NodeOutputSchemaProvider` block:

```go
// OutcomeOutputsProvider is optionally implemented by NodeDescriptor to declare
// outputs that report an operation outcome the workflow must handle (e.g. a
// db.create's "exists"). A fired output with no outbound edge silently ends
// that execution path, so validation rejects a workflow that leaves a declared
// outcome output unwired (#442). Control-flow branches (control.if's "then"/
// "else", control.switch's "default", control.loop's "done") are deliberately
// not outcome outputs: leaving one unwired is a normal workflow shape.
type OutcomeOutputsProvider interface {
	OutcomeOutputs() []string
}
```

- [ ] **Step 4: Implement on the 8 descriptors**

Each of the 8 files defines a descriptor struct near the top (e.g. `createDescriptor` in `plugins/db/create.go`). Add one method per file, next to its `OutputDescriptions`. For `plugins/db/create.go`:

```go
// OutcomeOutputs declares "exists" as an outcome the workflow must wire:
// an unwired outcome output silently ends the path (#442).
func (d *createDescriptor) OutcomeOutputs() []string { return []string{"exists"} }
```

Repeat with the file's own descriptor type and output:
- `plugins/db/update.go`, `plugins/db/upsert.go`: `{"exists"}`
- `plugins/auth/create_user.go`: `{"exists"}`
- `plugins/auth/get_user.go`: `{"not_found"}`
- `plugins/auth/set_password.go`, `plugins/auth/verify_credentials.go`: `{"invalid"}`
- `plugins/auth/one_time_tokens.go`: this file holds TWO descriptors (create_token and consume_token). Only the **consume_token** descriptor gets `OutcomeOutputs() []string { return []string{"invalid"} }` — create_token has default outputs, do not touch it.

Do NOT add the method to any control node.

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./internal/registry/ -run TestOutcomeOutputsAudit -v`
Expected: PASS. Also run `go build ./...`.

- [ ] **Step 6: Commit**

```bash
git add pkg/api/node.go plugins/db plugins/auth internal/registry/outcome_outputs_audit_test.go
git commit -m "feat(api): OutcomeOutputsProvider — nodes declare must-handle outcome outputs (#442)"
```

---

### Task 2: Validation rule — unwired outcome output is a hard error

**Files:**
- Modify: `internal/registry/validator.go` (inside `ValidateStartupDryRun`, after the check-4 edge-output block that ends ~line 317, still inside the `for wfName, wf := range rc.Workflows` loop)
- Test: `internal/registry/validator_test.go`

**Interfaces:**
- Consumes: `api.OutcomeOutputsProvider` (Task 1).
- Produces: the validation error with the exact Global-Constraints message — Task 5's sweep is verified by this rule via `TestShippedProjectsValidate`.

- [ ] **Step 1: Write the failing tests**

In `internal/registry/validator_test.go`, mirror `setupEdgeValidation` (~line 544; it uses the `pluginWithNodes`, `stubDescriptor`, `stubExecutor` helpers — read them first and adapt if their fields differ):

```go
// outcomeStubDescriptor is a stubDescriptor that declares outcome outputs.
type outcomeStubDescriptor struct {
	stubDescriptor
	outcomes []string
}

func (d *outcomeStubDescriptor) OutcomeOutputs() []string { return d.outcomes }

// setupOutcomeValidation registers a "db"-like plugin with one node that
// declares an outcome output ("create": exists) and one that doesn't
// ("findOne"), so the unwired-outcome-output check (#442) can be exercised
// without pulling in plugins/db.
func setupOutcomeValidation(t *testing.T) (*PluginRegistry, *NodeRegistry) {
	t.Helper()

	plugins := NewPluginRegistry()
	dbPlugin := pluginWithNodes("test-db", "db", []api.NodeRegistration{
		{
			Descriptor: &outcomeStubDescriptor{stubDescriptor: stubDescriptor{name: "create"}, outcomes: []string{"exists"}},
			Factory: func(map[string]any) api.NodeExecutor {
				return &stubExecutor{outputs: []string{"success", "exists", "error"}}
			},
		},
		{
			Descriptor: &stubDescriptor{name: "findOne"},
			Factory: func(map[string]any) api.NodeExecutor {
				return &stubExecutor{outputs: []string{"success", "error"}}
			},
		},
	})
	require.NoError(t, plugins.Register(dbPlugin))

	nodes := NewNodeRegistry()
	require.NoError(t, nodes.RegisterFromPlugin(dbPlugin))
	return plugins, nodes
}

func TestValidateStartupDryRun_OutcomeOutput_UnwiredRejected(t *testing.T) {
	plugins, nodes := setupOutcomeValidation(t)

	rc := &config.ResolvedConfig{
		Workflows: map[string]map[string]any{
			"wf1": {
				"nodes": map[string]any{
					"insert": edgeWorkflowNode("db.create", nil),
					"next":   edgeWorkflowNode("db.findOne", nil),
				},
				"edges": []any{edge("insert", "next", "success")},
			},
		},
	}

	errs := ValidateStartupDryRun(rc, plugins, nodes, expr.NewCompilerWithFunctions(), nil)
	require.Len(t, errs, 1)
	assert.Contains(t, errs[0].Error(), `workflow "wf1", node "insert" (db.create): outcome output "exists" has no outbound edge`)
}

func TestValidateStartupDryRun_OutcomeOutput_WiredAccepted(t *testing.T) {
	plugins, nodes := setupOutcomeValidation(t)

	rc := &config.ResolvedConfig{
		Workflows: map[string]map[string]any{
			"wf1": {
				"nodes": map[string]any{
					"insert": edgeWorkflowNode("db.create", nil),
					"next":   edgeWorkflowNode("db.findOne", nil),
				},
				"edges": []any{
					edge("insert", "next", "success"),
					edge("insert", "next", "exists"),
				},
			},
		},
	}

	errs := ValidateStartupDryRun(rc, plugins, nodes, expr.NewCompilerWithFunctions(), nil)
	assert.Empty(t, errs)
}

func TestValidateStartupDryRun_OutcomeOutput_NoEdgesAtAllRejected(t *testing.T) {
	plugins, nodes := setupOutcomeValidation(t)

	// A workflow with no "edges" key at all still fails: the outcome output
	// is unwired regardless of whether the edge list exists.
	rc := &config.ResolvedConfig{
		Workflows: map[string]map[string]any{
			"wf1": {
				"nodes": map[string]any{
					"insert": edgeWorkflowNode("db.create", nil),
				},
			},
		},
	}

	errs := ValidateStartupDryRun(rc, plugins, nodes, expr.NewCompilerWithFunctions(), nil)
	require.Len(t, errs, 1)
	assert.Contains(t, errs[0].Error(), `outcome output "exists" has no outbound edge`)
}

func TestValidateStartupDryRun_OutcomeOutput_ControlFlowExempt(t *testing.T) {
	plugins, nodes := setupEdgeValidation(t)

	// control.if with only "then" wired is a legitimate shape ("do nothing
	// when false") — stubDescriptor implements no OutcomeOutputsProvider,
	// exactly like the real control descriptors.
	rc := &config.ResolvedConfig{
		Workflows: map[string]map[string]any{
			"wf1": {
				"nodes": map[string]any{
					"decide": edgeWorkflowNode("control.if", nil),
					"next":   edgeWorkflowNode("control.if", nil),
				},
				"edges": []any{edge("decide", "next", "then")},
			},
		},
	}

	errs := ValidateStartupDryRun(rc, plugins, nodes, expr.NewCompilerWithFunctions(), nil)
	assert.Empty(t, errs)
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/registry/ -run 'TestValidateStartupDryRun_OutcomeOutput' -v`
Expected: `outcomeStubDescriptor` compiles (or adapt to the real stub helpers), then `UnwiredRejected` and `NoEdgesAtAllRejected` FAIL (0 errors returned); `WiredAccepted`/`ControlFlowExempt` pass vacuously.

- [ ] **Step 3: Implement the check in `validator.go`**

Insert after the check-4 edge-output block (after its closing `}`, before the workflow loop's closing brace at ~line 318), inside the same `for wfName, wf := range rc.Workflows` loop:

```go
		// 5. Outcome outputs must be wired (#442): a fired output with no
		// outbound edge silently ends the path — ExecuteGraph returns nil, the
		// workflow reports success, HTTP falls back to 202. The engine already
		// fails loudly on an unwired "error" output; this extends the same
		// contract to outputs a descriptor declares as operation outcomes
		// (db.create's "exists", auth.get_user's "not_found", ...). Built here
		// rather than at runtime so validate/editor/MCP reject it before deploy.
		wiredOutputs := make(map[string]map[string]bool) // nodeID → wired output set
		if edgesRaw, ok := wf["edges"].([]any); ok {
			for _, rawEdge := range edgesRaw {
				edgeMap, ok := rawEdge.(map[string]any)
				if !ok {
					continue
				}
				from, _ := edgeMap["from"].(string)
				output, _ := edgeMap["output"].(string)
				if output == "" {
					output = "success"
				}
				if wiredOutputs[from] == nil {
					wiredOutputs[from] = make(map[string]bool)
				}
				wiredOutputs[from][output] = true
			}
		}
		for nodeID, raw := range wfNodes {
			node, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			nodeType, _ := node["type"].(string)
			desc, found := nodes.GetDescriptor(nodeType)
			if !found {
				continue // unregistered node type: owned by check 2 above
			}
			provider, ok := desc.(api.OutcomeOutputsProvider)
			if !ok {
				continue
			}
			for _, out := range provider.OutcomeOutputs() {
				if !wiredOutputs[nodeID][out] {
					errs = append(errs, fmt.Errorf("workflow %q, node %q (%s): outcome output %q has no outbound edge — a fired outcome output with no edge silently ends the path; wire it (e.g. to an error response, or to the same target as \"success\" if the distinction does not matter)",
						wfName, nodeID, nodeType, out))
				}
			}
		}
```

Check the file's imports already include `pkg/api` (they do — `api.NodeDescriptor` is referenced); add if not.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/registry/ -v -run 'TestValidateStartupDryRun'`
Expected: ALL PASS (new tests plus every pre-existing dry-run test — none of the existing fixtures use the 8 real node types unwired, but if one fails, fix the fixture by wiring the output, not by weakening the check).

- [ ] **Step 5: Run the full registry package + shipped-projects gate**

Run: `go test ./internal/registry/ && go test ./cmd/noda/ -run TestShippedProjectsValidate`
Expected: registry PASSES; **`TestShippedProjectsValidate` FAILS** listing exactly the 19 example sites from the spec's table (testdata/valid-project is not part of that gate; other cmd/noda tests using testdata may also now fail). This failure is EXPECTED and stays red until Task 5. Record the exact failing list in the commit message body.

- [ ] **Step 6: Commit**

```bash
git add internal/registry/validator.go internal/registry/validator_test.go
git commit -m "feat(validate): reject unwired outcome outputs at validate/boot (#442)

TestShippedProjectsValidate now fails on the 19 known example sites;
fixed in the examples-sweep commit."
```

---

### Task 3: `writeAccepted` contract guard — no 202 on routes that declare responses

**Files:**
- Modify: `internal/server/routes.go` (`awaitWorkflowResponse`, the `writeAccepted` closure at ~line 455)
- Test: `internal/server/routes_test.go`

**Interfaces:**
- Consumes: `respValidator *responseValidator` — already a parameter; it is **nil** unless the route has a `response` block with ≥1 status schema and validation not disabled (see route build at routes.go:119-135).

- [ ] **Step 1: Write the failing test**

In `internal/server/routes_test.go`, find `TestRoute_NoResponseNode_202Accepted` (~line 253) and copy its full setup (server construction, route registration, request execution) into a new test. The ONLY differences: the route config gains a `response` block, and the assertions flip:

```go
func TestRoute_NoResponseNode_DeclaredResponses_500(t *testing.T) {
	// Same harness as TestRoute_NoResponseNode_202Accepted, with one change:
	// the route declares a response schema. A workflow that then produces no
	// response contradicts the route's own contract — that must be a 500,
	// not a 202 that reads as success (#442).
	//
	// Add to the route config map used by the copied setup:
	//   "response": map[string]any{
	//       "200": map[string]any{
	//           "schema": map[string]any{"type": "object"},
	//       },
	//   },

	// ... copied setup ...

	// Assertions (replacing the 202 ones):
	// assert.Equal(t, 500, resp.StatusCode)
	// result decoded from body:
	// assert.Equal(t, "INTERNAL_ERROR", errBody["error"].(map[string]any)["code"])
	// assert.NotEmpty(t, errBody["error"].(map[string]any)["trace_id"])
}
```

Also verify the existing `TestRoute_NoResponseNode_202Accepted` keeps passing untouched — it is the no-`response`-block case and its behavior must not change.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/server/ -run 'TestRoute_NoResponseNode' -v`
Expected: new test FAILS (got 202, want 500); old test PASSES.

- [ ] **Step 3: Implement the guard**

Replace the `writeAccepted` closure body in `awaitWorkflowResponse`:

```go
	writeAccepted := func() error {
		// No response node fired. For a fire-and-forget route that's the
		// contract: 202. But a route that declares response schemas and
		// produced none is contradicting its own contract — surface it as a
		// server error rather than a 2xx that reads as success (#442).
		if respValidator != nil {
			s.logger.Error("route declares response schemas but the workflow produced no response",
				"route", routeID, "trace_id", traceID)
			return writeErrorResponse(c, 500, ErrorResponse{
				Error: api.ErrorData{
					Code:    "INTERNAL_ERROR",
					Message: "route declares responses but the workflow produced none",
					TraceID: traceID,
				},
			})
		}
		return c.Status(fiber.StatusAccepted).JSON(map[string]any{
			"status":   "accepted",
			"trace_id": traceID,
		})
	}
```

Notes: `writeErrorResponse`/`ErrorResponse`/`api.ErrorData` are exactly what the 504 branch below already uses — mirror it. `"INTERNAL_ERROR"` is the #417 `api.ErrorCode` vocabulary word for a 500 (check `pkg/api` for an exported constant and use it if one exists; do NOT invent a new code — the machine-readable code regressing to something ad-hoc was a documented trap in the #418 work). Both `writeAccepted` call sites (workflow-done ~line 483 and completed-at-deadline ~line 497) go through this closure — no other change needed.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/server/ -run 'TestRoute_NoResponseNode' -v` then the whole package `go test ./internal/server/`
Expected: both targeted tests PASS. If any other server test fails, it is a test that declares response schemas AND relies on the 202 fallback — inspect it: if it models a real fire-and-forget route, remove its `response` block; if it models the #442 bug, update its expectation to 500. Do not weaken the guard.

- [ ] **Step 5: Commit**

```bash
git add internal/server/routes.go internal/server/routes_test.go
git commit -m "fix(server): 500, not 202, when a route declaring responses produces none (#442)"
```

---

### Task 4: CRUD generator wires `exists`

**Files:**
- Modify: `internal/generate/crud.go` (`generateCreateWorkflow` ~line 397-410, `generateUpdateWorkflow` ~line 546-560)
- Test: `internal/generate/crud_test.go`

**Interfaces:**
- Consumes: nothing new. Produces workflows that must pass Task 2's validation.

- [ ] **Step 1: Write the failing test**

Add to `internal/generate/crud_test.go` (mirror `TestGenerateCRUD_AllOperations`'s way of invoking the generator and digging into the result — read it first; `getWorkflow`-style helpers may exist):

```go
func TestGenerateCRUD_WiresExistsOutcome(t *testing.T) {
	// #442: generated create/update workflows must wire db.create/db.update's
	// "exists" outcome output — validation rejects unwired outcome outputs.
	result := generateForTest(t) // use the same construction TestGenerateCRUD_AllOperations uses

	for _, wf := range []string{"create", "update"} {
		workflow := workflowByPrefix(t, result, wf) // however AllOperations locates one
		nodes := workflow["nodes"].(map[string]any)
		conflict, ok := nodes["conflict"].(map[string]any)
		require.True(t, ok, "%s workflow must have a conflict response node", wf)
		assert.Equal(t, "response.error", conflict["type"])
		cfg := conflict["config"].(map[string]any)
		assert.Equal(t, 409, cfg["status"])
		assert.Equal(t, "CONFLICT", cfg["code"])

		srcNode := map[string]string{"create": "create", "update": "update"}[wf]
		found := false
		for _, e := range workflow["edges"].([]any) {
			em := e.(map[string]any)
			if em["from"] == srcNode && em["output"] == "exists" && em["to"] == "conflict" {
				found = true
			}
		}
		assert.True(t, found, "%s workflow must wire %s.exists -> conflict", wf, srcNode)
	}
}
```

(Adapt the two helper call sites to whatever `TestGenerateCRUD_AllOperations` actually does — the assertions are the contract.)

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/generate/ -run TestGenerateCRUD_WiresExistsOutcome -v`
Expected: FAIL — no "conflict" node.

- [ ] **Step 3: Implement**

In `generateCreateWorkflow`, after the `nodes["respond"]` block (~line 403) and its success edge:

```go
	nodes["conflict"] = map[string]any{
		"type": "response.error",
		"config": map[string]any{
			"status":  409,
			"code":    "CONFLICT",
			"message": singular + " already exists",
		},
	}
	edges = append(edges, map[string]any{"from": "create", "to": "conflict", "output": "exists"})
```

In `generateUpdateWorkflow`, after its edges block (~line 554):

```go
	nodes["conflict"] = map[string]any{
		"type": "response.error",
		"config": map[string]any{
			"status":  409,
			"code":    "CONFLICT",
			"message": singular + " already exists",
		},
	}
	edges = append(edges, map[string]any{"from": "update", "to": "conflict", "output": "exists"})
```

(`generateDeleteWorkflow` and the read paths use `db.delete`/`db.find*` — no outcome outputs, do not touch.)

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/generate/`
Expected: PASS, including all pre-existing generator tests (some assert exact node/edge sets — extend their expectations if they enumerate exhaustively; the new node is correct output, not test noise).

- [ ] **Step 5: Commit**

```bash
git add internal/generate/crud.go internal/generate/crud_test.go
git commit -m "fix(generate): CRUD create/update workflows wire exists to a 409 (#442)"
```

---

### Task 5: Sweep — wire the 20 shipped sites + cookbook 409 round-trip

**Files:**
- Modify: the 20 JSON workflow files from the spec's table (grouped below)
- Modify: `examples/node-cookbook/db/verify.json`
- Test: `go test ./cmd/noda/ -run TestShippedProjectsValidate` (goes green again) + full `go test ./...`

**Interfaces:**
- Consumes: Task 2's validation rule defines "done" — zero unwired-outcome-output errors across all shipped projects.

Three wiring patterns. Add nodes/edges by hand-editing the JSON (match each file's indentation; never reformat unrelated lines).

**Pattern A — HTTP route → 409 conflict response.** Add to `"nodes"`:

```json
"respond_conflict": {
  "type": "response.error",
  "config": { "status": 409, "code": "CONFLICT", "message": "a row with one of these unique values already exists" }
}
```

Add to `"edges"` (source node name per site): `{ "from": "<node>", "output": "exists", "to": "respond_conflict" }`

**Pattern B — worker/WS-triggered → warn log (terminal).** Add to `"nodes"`:

```json
"log_conflict": {
  "type": "util.log",
  "config": { "level": "warn", "message": "unique constraint conflict — row not written" }
}
```

Add to `"edges"`: `{ "from": "<node>", "output": "exists", "to": "log_conflict" }` (the log node's own `success` staying unwired is fine — it is not an outcome output).

**Pattern C — site-specific (3 files), detailed below.**

- [ ] **Step 1: Apply Pattern A (13 sites, 12 files)**

| File | source node |
|---|---|
| examples/node-cookbook/db/workflows/create.json | insert |
| examples/node-cookbook/db/workflows/update.json | change |
| examples/node-cookbook/db/workflows/upsert.json | save |
| examples/realtime-collab/workflows/create-document.json | insert |
| examples/realtime-collab/workflows/update-document.json | update |
| examples/rest-api/workflows/create-task.json | insert |
| examples/saas-backend/workflows/create-project.json | insert |
| examples/saas-backend/workflows/create-task.json | insert |
| examples/saas-backend/workflows/create-workspace.json | insert_workspace |
| examples/saas-backend/workflows/create-workspace.json | insert_member |
| examples/saas-backend/workflows/invite-member.json | insert |
| examples/saas-backend/workflows/upload-attachment.json | insert |
| testdata/valid-project/workflows/create-task.json | insert |

create-workspace.json gets ONE `respond_conflict` node and TWO edges (both sources). If a target file already responds to errors with `response.json` (not `response.error`), keep Pattern A's `response.error` anyway — it produces the standard error envelope.

- [ ] **Step 2: Apply Pattern B (3 sites)**

| File | source node | trigger |
|---|---|---|
| examples/realtime-collab/workflows/ws-on-message.json | save_edit | WS on_message |
| examples/saas-backend/workflows/sync-github-issue.json | create_task AND close_task | worker |
| examples/saas-backend/workflows/generate-thumbnail.json | update_record | worker |

sync-github-issue.json gets ONE `log_conflict` node and TWO edges.

- [ ] **Step 3: Apply Pattern C (3 files)**

**examples/saas-backend/workflows/handle-stripe-webhook.json** (HTTP webhook — Stripe must get a 2xx ack or it retries forever; log the conflict, then ack):

```json
"log_conflict": {
  "type": "util.log",
  "config": { "level": "warn", "message": "stripe webhook: subscription update hit a unique constraint — row not written" }
},
"ack_conflict": {
  "type": "response.json",
  "config": { "status": 200, "body": { "received": true } }
}
```

Edges: `{ "from": "update_subscription", "output": "exists", "to": "log_conflict" }`, `{ "from": "log_conflict", "to": "ack_conflict" }`

**examples/realworld/workflows/update-user.json**: the workflow already has `respond_password_invalid` (the spec-shaped error response its password-policy `control.if` uses). One new edge, no new node:

```json
{ "from": "set_password", "output": "invalid", "to": "respond_password_invalid" }
```

**examples/auth-demo/workflows/auth.reset-password.json**: `consume.invalid` is already wired; the unwired output is `set_password.invalid` (fires on password-policy violation — the token is already validated by then, so this creates no token-validity oracle; compare `cmd/noda/auth_templates/workflows/auth.reset-password.json.tmpl`, which wires it to a 400). Match the file's local response style:

```json
"respond_password_invalid": {
  "type": "response.json",
  "config": { "status": 400, "body": { "error": "password does not meet requirements" } }
}
```

Edge: `{ "from": "set_password", "output": "invalid", "to": "respond_password_invalid" }`

- [ ] **Step 4: Add the cookbook 409 round-trip**

In `examples/node-cookbook/db/verify.json`, insert a new step directly after `"name": "create inserts a row"` (before "second row for aggregates"; the books table has a unique index on `title` — the upsert step depends on it):

```json
{
  "name": "duplicate insert answers 409",
  "request": { "method": "POST", "path": "/api/books", "body": { "title": "Dune", "author": "Herbert", "year": 1965 } },
  "expect": { "status": 409, "body": [ { "path": "error.code", "equals": "CONFLICT" } ] }
},
```

Sanity-check the later steps still hold: the duplicate row is NOT written, so "find filters by author" (1 Herbert row) and "count counts by author" (count 1) are unchanged. Confirm the `error.code` body path matches `response.error`'s envelope by reading `plugins/core/response/error.go`'s body construction before committing; adjust the path if the envelope nests differently.

- [ ] **Step 5: Verify everything is green**

Run, in order:
1. `go test ./cmd/noda/ -run TestShippedProjectsValidate` — PASS (the Task 2 red goes green; this is the sweep's completeness proof)
2. `go test ./...` — PASS (workflow tests in examples' `tests/` mock node outputs; new never-executed nodes must not disturb them — if one fails, read it, don't guess)
3. `go vet -tags integration ./...`
4. With Docker available: `go test -tags integration ./internal/testing/cookbook/ -run 'TestCookbook/db' -v` — PASS including the new 409 step. (If the package path differs, `grep -rn "func TestCookbook" --include='*_test.go' .` and use that path.)

- [ ] **Step 6: Commit**

```bash
git add examples testdata
git commit -m "fix(examples): wire every outcome output — duplicates 409, invalid tokens 400, conflicts logged (#442)"
```

---

### Task 6: Docs + CHANGELOG

**Files:**
- Modify: `docs/03-nodes/db.create.md`, `db.update.md`, `db.upsert.md`, `auth.create_user.md`, `auth.get_user.md`, `auth.set_password.md`, `auth.verify_credentials.md`, `auth.consume_token.md`
- Modify: `docs/02-config/workflows.md`
- Modify: the plugin-development guide in `docs/04-guides/` (find it: `ls docs/04-guides/`)
- Modify: `CHANGELOG.md` (`[Unreleased]`)

- [ ] **Step 1: Node pages**

In each of the 8 node pages, in the outputs section (each page documents its outputs — find the section, don't append at the bottom), add one line after the outcome output's description (adjust the output name per node):

> `exists` is an **outcome output**: validation rejects a workflow that leaves it without an outbound edge, because a fired output with no edge would silently end the path (#442). Wire it to an error response, or to the same target as `success` if the distinction does not matter.

- [ ] **Step 2: `docs/02-config/workflows.md`**

In the edges section, add a subsection:

```markdown
### Outcome outputs must be wired

Some outputs report an operation outcome rather than a control-flow branch:
`exists` on `db.create`/`db.update`/`db.upsert`/`auth.create_user`, `not_found`
on `auth.get_user`, and `invalid` on `auth.set_password`/`auth.verify_credentials`/
`auth.consume_token`. A fired output with no outbound edge silently ends that
execution path, so `noda validate` (and boot) reject a workflow that leaves an
outcome output unwired:

​```
workflow "create-user", node "insert" (db.create): outcome output "exists" has no
outbound edge — a fired outcome output with no edge silently ends the path; wire
it (e.g. to an error response, or to the same target as "success" if the
distinction does not matter)
​```

Control-flow branches are exempt: leaving `control.if`'s `else`,
`control.switch`'s `default`, or `control.loop`'s `done` unwired is a normal
workflow shape.
```

(Remove the zero-width characters around the inner code fence when pasting.)

- [ ] **Step 3: Plugin-development guide**

Where the guide documents `NodeDescriptor` (near `OutputDescriptions`/`NodeOutputSchemaProvider`), add:

```markdown
### Declaring outcome outputs

If a node has an output that reports an operation outcome the workflow must
handle (a "not found", a "duplicate", an "invalid input" port), implement the
optional `api.OutcomeOutputsProvider` on the descriptor:

​```go
func (d *myDescriptor) OutcomeOutputs() []string { return []string{"not_found"} }
​```

Validation then rejects any workflow that leaves that output unwired. Do not
declare control-flow branches (an `if`'s `else`, a switch's `default`) — leaving
those unwired is a legitimate workflow shape.
```

- [ ] **Step 4: CHANGELOG**

Under `[Unreleased]`:

In `### Added`:

```markdown
- `api.OutcomeOutputsProvider` — a node descriptor can declare outcome outputs (`db.*`'s `exists`, `auth.get_user`'s `not_found`, the auth `invalid` ports) that a workflow must wire. **BREAKING:** `noda validate` and boot now reject a workflow that leaves a declared outcome output unwired, because a fired output with no outbound edge silently ends that path — the workflow reports success and an HTTP route answers `202 Accepted` while the operation did nothing (#442). This is the boot-time generalization of the engine's existing unwired-`error` rule, and closes the regression window opened by #436 (duplicate inserts silently 202'd in any workflow that wired neither `error` nor `exists`). Migration: add one edge per flagged site — to an error response, or to the same target as `success` if the distinction does not matter. Control-flow branches (`then`/`else`/`default`/`done`) are exempt.
```

In `### Fixed` (create the subsection if absent):

```markdown
- A route that declares response schemas but whose workflow produces no response now answers `500 INTERNAL_ERROR` instead of a `202 Accepted` that reads as success (#442). Fire-and-forget routes without a `response` block keep the 202.
- Every shipped example and the CRUD generator now wire their outcome outputs: duplicate inserts answer 409 `CONFLICT` (previously silently 202 since #436), auth-demo's reset-password answers 400 on a policy-invalid password, and worker/WebSocket conflict paths log a warning instead of dead-ending (#442).
```

- [ ] **Step 5: Verify docs claims against the runtime**

Run `go test ./cmd/noda/ -run TestShippedProjectsValidate && go test ./internal/registry/ ./internal/server/` once more (docs commits shouldn't change behavior — this catches accidental JSON edits). If the repo's docverify snippet validator runs in CI (`tools/docverify`), run it per its README to confirm the new fenced snippets pass.

- [ ] **Step 6: Commit**

```bash
git add docs/03-nodes docs/02-config docs/04-guides CHANGELOG.md
git commit -m "docs: outcome outputs must be wired — node pages, config guide, plugin guide, CHANGELOG (#442)"
```

---

### Task 7: Whole-branch verification + PR

- [ ] **Step 1: Full local gate from the worktree root**

```bash
gofmt -l .                       # must print nothing
go vet ./...
go vet -tags integration ./...
go test ./...
golangci-lint run                # CI parity; formatting-only failures are real failures
```

- [ ] **Step 2: Integration passes that touch changed surfaces (Docker running)**

```bash
go test -tags integration ./internal/testing/cookbook/ -run 'TestCookbook/db' -v
go test -tags integration ./... 2>&1 | tail -20   # full pass if time allows; livekit flake (#396) is rerun-noise
```

The realworld Hurl harness runs in CI with the example's containers; if `hurl` is installed locally, run it per `examples/realworld/harness/` docs — expect `known-failing.json` entries to stay as-is (the update-user `invalid` wiring is exercised by masked entries only, per #435's coverage note). If any known-failing entry unexpectedly goes green, move it out of `known-failing.json` in this PR and say so in the PR body.

- [ ] **Step 3: Add the spec + plan to the branch (convention: point-in-time records, force-added past the gitignore)**

```bash
git add -f docs/superpowers/specs/2026-07-24-unwired-outcome-outputs-design.md docs/superpowers/plans/2026-07-24-unwired-outcome-outputs.md
git commit -m "docs(superpowers): spec + plan for the #442 tranche"
```

- [ ] **Step 4: Push and open the PR**

```bash
git push -u origin fix/unwired-outcome-outputs
gh pr create --title "fix!: unwired outcome outputs fail loudly at validate/boot (#442)" --body "..."
```

PR body must cover: closes #442; the BREAKING validation change + migration line; the writeAccepted 500 change; the #436 regression this closes (409s restored at every example site); the 20-site sweep table; note that `benchmark` is NOT a required check. End the body with the standard generation footer.

---

## Self-Review (done at planning time)

- **Spec coverage:** §1→Task 1, §2→Task 2, §3→Task 3, §4 generator→Task 4, §4 sweep→Task 5, §5 docs/CHANGELOG/tests→Tasks 1-6, verification→Task 7. Anti-enumeration guardrail → Task 5 Pattern C (auth-demo). No gaps.
- **Known intentional red between tasks:** `TestShippedProjectsValidate` is red from Task 2 Step 5 until Task 5 Step 5 — commits in between are still made (the red is recorded in the Task 2 commit body). If the executor requires every commit green, reorder: run Task 5 Steps 1-3 (sweep) immediately after Task 2 Step 4, then continue — the plan's task boundaries still hold for review purposes.
- **Type consistency:** `OutcomeOutputs() []string` used identically in Tasks 1/2/6; `respond_conflict`/`log_conflict` node names consistent across Task 5; error message string identical in Task 2 code, Task 2 test, and Task 6 docs.
