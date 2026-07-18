# Backlog Pass 2026-07-18 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Close 13 open issues in three PRs: small fixes (#354 #355 #356 #357 #358 #359 #365), behavior changes (#349 #350 #361 #364), and the cross-instance connection sync bridge + livekit timeouts (#363 #368).

**Architecture:** Per spec `docs/superpowers/specs/2026-07-18-backlog-pass-design.md`. PR A lands mechanical fixes on branch `backlog-2026-07-18` (current, already carries the spec commit). PR B (`backlog-2026-07-18-b`) and PR C (`backlog-2026-07-18-c`) branch off `origin/main` in this same worktree after A is pushed; CHANGELOG conflicts at merge time are resolved as entry unions (established convention).

**Tech Stack:** Go, fiber v3, go-redis v9, expr-lang, testify. Docs are markdown under `docs/`.

## Global Constraints

- TDD: write the failing test first, watch it fail, then fix. Frequent commits, conventional prefixes (`fix:`, `feat:`, `docs:`, `test:`).
- Every commit message ends with `Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>`.
- Verification for each Go task: `go build ./... && go vet ./...` plus the named package tests. Full `go test ./internal/... ./plugins/... ./cmd/... ./pkg/...` before each PR. Integration tests needing live services auto-skip locally; CI runs them.
- CHANGELOG entries go under `## [Unreleased]` in `CHANGELOG.md`; behavior changes get an explicit "was X, now Y" sentence.
- PR bodies list `Fixes #N` per closed issue and end with the `🤖 Generated with [Claude Code](https://claude.com/claude-code)` footer. Create with `gh pr create`, then `gh pr merge --squash --auto`.
- Do not remove or reword the `**Example:**` cookbook links in `docs/03-nodes/*.md` — `TestCookbookCoverage` gates on them.
- `docs/superpowers/` is gitignored; `git add -f` for spec/plan commits.

---

# PR A — small fixes (branch `backlog-2026-07-18`, current)

### Task 1: #354 strict mode admits transport namespaces

**Files:**
- Modify: `internal/expr/compiler.go:66-74` (knownContextEnv)
- Test: `internal/expr/compiler_test.go`

**Interfaces:** none new — `knownContextEnv` is package-private; strict mode gated by existing `WithStrictMode(true)`.

- [ ] **Step 1: Write the failing test** (append to `internal/expr/compiler_test.go`):

```go
func TestStrictMode_TransportNamespaces(t *testing.T) {
	c := NewCompiler(WithStrictMode(true))
	// Every namespace a trigger mapping can reference: HTTP route transport
	// (trigger.go buildRawRequestContext + raw_body), worker message
	// (worker/runtime.go:380), scheduler metadata (scheduler/runtime.go:342).
	valid := []string{
		"{{ body.name }}",
		"{{ query.page }}",
		"{{ params.id }}",
		"{{ headers['x-api-key'] }}",
		"{{ request.headers['x-github-event'] }}",
		"{{ raw_body }}",
		"{{ method }}",
		"{{ path }}",
		"{{ message.payload.id }}",
		"{{ schedule.cron }}",
	}
	for _, e := range valid {
		if _, err := c.Compile(e); err != nil {
			t.Errorf("strict mode rejected valid trigger-mapping expression %s: %v", e, err)
		}
	}
	if _, err := c.Compile("{{ bodyy.name }}"); err == nil {
		t.Error("strict mode should still reject unknown top-level names")
	}
}
```

- [ ] **Step 2: Run to verify failure**: `go test ./internal/expr/ -run TestStrictMode_TransportNamespaces -v` — expect FAIL ("unknown name body" style compile errors).
- [ ] **Step 3: Implement** — extend `knownContextEnv` in `internal/expr/compiler.go`:

```go
var knownContextEnv = map[string]any{
	"input":   map[string]any{},
	"auth":    map[string]any{},
	"trigger": map[string]any{},
	"nodes":   map[string]any{},
	"secrets": map[string]any{},
	"$item":   map[string]any{}, // loop/map/filter iteration value (control.loop, transform.map/filter)
	"$index":  0,                // loop/map/filter iteration index
	// Transport namespaces available to trigger input mappings (#354):
	// HTTP routes (internal/server/trigger.go buildRawRequestContext),
	// workers ("message", internal/worker/runtime.go), schedulers
	// ("schedule", internal/scheduler/runtime.go).
	"body":     map[string]any{},
	"query":    map[string]any{},
	"params":   map[string]any{},
	"headers":  map[string]any{},
	"request":  map[string]any{},
	"raw_body": "",
	"method":   "",
	"path":     "",
	"message":  map[string]any{},
	"schedule": map[string]any{},
}
```

- [ ] **Step 4: Run to verify pass**: same command — expect PASS; then `go test ./internal/expr/`.
- [ ] **Step 5: Commit**: `fix(expr): strict mode admits transport namespaces used by trigger mappings (#354)`

### Task 2: #356 headers-patcher location + hmac_verify uppercase prefix

**Files:**
- Modify: `internal/expr/headers.go:26-28`, `internal/expr/functions.go:201-204`
- Test: `internal/expr/headers_test.go`, `internal/expr/functions_test.go`

- [ ] **Step 1: Write failing tests.** In `internal/expr/headers_test.go` (match the file's existing parse/walk helpers; if it tests via `Compile`, add this parser-level test with imports `github.com/expr-lang/expr/parser`, `github.com/expr-lang/expr/ast`):

```go
func TestHeaderKeyPatcher_PreservesSourceLocation(t *testing.T) {
	tree, err := parser.Parse(`headers['X-GitHub-Event']`)
	require.NoError(t, err)
	member := tree.Node.(*ast.MemberNode)
	origLoc := member.Property.(*ast.StringNode).Location()
	ast.Walk(&tree.Node, headerKeyPatcher{})
	patched := tree.Node.(*ast.MemberNode).Property.(*ast.StringNode)
	assert.Equal(t, "x-github-event", patched.Value)
	assert.Equal(t, origLoc, patched.Location(), "patched key must keep the original source location")
}
```

In `internal/expr/functions_test.go` (next to the existing hmac_verify tests):

```go
func TestHmacVerify_UppercaseAlgorithmPrefix(t *testing.T) {
	key, data := "secret", "payload"
	h := hmac.New(sha256.New, []byte(key))
	h.Write([]byte(data))
	sig := hex.EncodeToString(h.Sum(nil))

	r := NewFunctionRegistry()
	fn, _ := r.Get("hmac_verify")
	got, err := fn(data, key, "sha256", "SHA256="+strings.ToUpper(sig))
	require.NoError(t, err)
	assert.Equal(t, true, got, "uppercase '<ALGORITHM>=' prefix must verify")
}
```

(Adapt registry access to the file's existing accessor — the current tests already call hmac_verify somehow; mirror them.)

- [ ] **Step 2: Run to verify failure**: `go test ./internal/expr/ -run 'TestHeaderKeyPatcher_PreservesSourceLocation|TestHmacVerify_Uppercase' -v` — location test FAILs (zero location), hmac test FAILs (false).
- [ ] **Step 3: Implement.** `internal/expr/headers.go`:

```go
	if lower := strings.ToLower(str.Value); lower != str.Value {
		patched := &ast.StringNode{Value: lower}
		patched.SetLocation(str.Location())
		member.Property = patched
	}
```

`internal/expr/functions.go` hmac_verify (replace the TrimPrefix line and its comment):

```go
		// Accept GitHub-style "<algorithm>=<hex>" prefixes case-insensitively
		// (SHA256=... verifies too), and normalize hex case.
		prefix := algorithm + "="
		if len(signature) >= len(prefix) && strings.EqualFold(signature[:len(prefix)], prefix) {
			signature = signature[len(prefix):]
		}
		signature = strings.ToLower(signature)
```

- [ ] **Step 4: Run to verify pass**, then `go test ./internal/expr/`.
- [ ] **Step 5: Commit**: `fix(expr): keep source location in patched header keys; case-insensitive hmac_verify prefix (#356)`

### Task 3: #355 wire secrets into `noda test`

**Files:**
- Modify: `internal/testing/runner.go:23-36,38-42,89-141`, `cmd/noda/main.go:256`
- Modify (call sites, mechanical): `internal/testing/*_test.go` callers of `RunTestSuite`
- Test: `internal/testing/runner_test.go`

**Interfaces:**
- Produces: `RunTestSuite(suite TestSuite, rc *config.ResolvedConfig, coreNodeReg *registry.NodeRegistry, secretsCtx map[string]any) []TestResult` — new trailing param; `nil` means no secrets.

- [ ] **Step 1: Write the failing test** (append to `internal/testing/runner_test.go`, following `TestRunner_PassingTest`'s rc-literal pattern):

```go
func TestRunner_SecretsAvailableInExpressions(t *testing.T) {
	rc := &config.ResolvedConfig{
		Workflows: map[string]map[string]any{
			"workflows/wf.json": {
				"id": "secret-wf",
				"nodes": map[string]any{
					"set": map[string]any{
						"type":   "transform.set",
						"config": map[string]any{"values": map[string]any{"token": "{{ secrets.MY_TOKEN }}"}},
					},
				},
				"edges": []any{},
			},
		},
	}
	suite := TestSuite{
		ID:       "test-secrets",
		Workflow: "secret-wf",
		Cases: []TestCase{{
			Name:   "secrets resolve",
			Expect: TestExpectation{Status: "success", Output: map[string]any{"set.token": "s3cret"}},
		}},
	}
	results := RunTestSuite(suite, rc, buildCoreNodeReg(t), map[string]any{"MY_TOKEN": "s3cret"})
	require.Len(t, results, 1)
	assert.True(t, results[0].Passed, "got: %+v", results[0])
}
```

(Verify `transform.set`'s config key — if it's not `values`, use the shape from `docs/03-nodes/transform.set.md`. Expectation addressing `set.token` follows the existing intermediate-output tests.)

- [ ] **Step 2: Run to verify failure**: compile error (wrong arity) — that's the signature driving the change: `go test ./internal/testing/ -run TestRunner_Secrets -v`.
- [ ] **Step 3: Implement.** In `runner.go`: add `secretsCtx map[string]any` as the 4th param of `RunTestSuite` and 5th of `runTestCase`; pass through; in the options block (next to `WithInput`):

```go
	if secretsCtx != nil {
		opts = append(opts, engine.WithSecrets(secretsCtx))
	}
```

Sub-workflows inherit automatically (`engine/subworkflow.go:65` copies `parent.secretsContext`). Update `cmd/noda/main.go:256`:

```go
				results := nodatesting.RunTestSuite(suite, rc, coreNodeReg, sm.ExpressionContext())
```

Fix all existing test call sites by appending `, nil`.

- [ ] **Step 4: Run to verify pass**: `go test ./internal/testing/ ./cmd/noda/`.
- [ ] **Step 5: Commit**: `feat(testing): noda test evaluates secrets.* via the loaded SecretsManager (#355)`

### Task 4: #359 Setup() builds subWorkflowRunner from the self-built cache

**Files:**
- Modify: `internal/server/server.go` (Setup, after the `if s.workflows == nil` block at :178-185)
- Test: `internal/server/coverage_test.go` (or a new `server_subworkflow_test.go`)

- [ ] **Step 1: Write the failing test:**

```go
func TestSetup_BuildsSubWorkflowRunnerWithoutInjectedCache(t *testing.T) {
	rc := &config.ResolvedConfig{
		Root: map[string]any{},
		Workflows: map[string]map[string]any{
			"workflows/child.json": {
				"id":    "child",
				"nodes": map[string]any{"log": map[string]any{"type": "util.log", "config": map[string]any{"message": "hi"}}},
				"edges": []any{},
			},
		},
	}
	srv, err := NewServer(rc, registry.NewServiceRegistry(), buildTestNodeRegistry())
	require.NoError(t, err)
	require.Nil(t, srv.subWorkflowRunner, "precondition: no runner before Setup without injected cache")
	require.NoError(t, srv.Setup())
	assert.NotNil(t, srv.subWorkflowRunner, "Setup must wire the sub-workflow runner from its self-built cache (#359)")
}
```

- [ ] **Step 2: Run to verify failure**: `go test ./internal/server/ -run TestSetup_BuildsSubWorkflowRunner -v` — FAIL on the final assert.
- [ ] **Step 3: Implement** — in `Setup()` directly after the self-build block:

```go
	// The constructor only wires the sub-workflow runner when a cache was
	// injected via WithWorkflowCache; a self-built cache arrives here (#359).
	if s.subWorkflowRunner == nil && s.workflows != nil {
		s.subWorkflowRunner = &engine.SubWorkflowRunnerImpl{
			Cache:    s.workflows,
			Services: s.services,
			Nodes:    s.nodes,
		}
	}
```

- [ ] **Step 4: Run to verify pass**, then `go test ./internal/server/`.
- [ ] **Step 5: Commit**: `fix(server): wire subWorkflowRunner from Setup's self-built workflow cache (#359)`

### Task 5: #365 release wasm plugins on partial-load failure

**Files:**
- Modify: `internal/wasm/module.go` (Stop early-return + struct field), `cmd/noda/runtime.go:193-209`, `internal/testing/cookbook/runner.go:~357` (mirror)
- Test: `internal/wasm/runtime_test.go` (or wherever the existing fake `PluginInstance` lives — grep `PluginInstance` in `internal/wasm/*_test.go` and reuse it)

- [ ] **Step 1: Write the failing test** (reuse the package's existing fake plugin; it must record `Close` calls — extend it with a `closed bool` if needed):

```go
func TestStopAll_ClosesNeverStartedModules(t *testing.T) {
	rt := NewRuntime(nil, nil, slog.Default())
	fake := &fakePluginInstance{} // package's existing test fake, with Close tracking
	_, err := rt.LoadModuleWithPlugin(ModuleConfig{Name: "m1"}, fake)
	require.NoError(t, err)
	// Never StartAll — simulates a later module failing mid-loop (#365).
	require.NoError(t, rt.StopAll(context.Background()))
	assert.True(t, fake.closed, "unstarted module's plugin must be closed to free the wazero runtime")
}
```

- [ ] **Step 2: Run to verify failure**: `go test ./internal/wasm/ -run TestStopAll_ClosesNeverStarted -v` — FAIL (Stop early-returns before Close).
- [ ] **Step 3: Implement.** In `Module` struct add `closed atomic.Bool`. In `Stop` (module.go:202):

```go
	m.mu.Lock()
	if !m.running {
		m.mu.Unlock()
		// Loaded but never started: still release the Extism plugin so a
		// failed multi-module load doesn't leak wazero runtimes (#365).
		if m.closed.CompareAndSwap(false, true) {
			_ = m.Plugin.Close(ctx)
		}
		return nil
	}
```

and guard the existing `_ = m.Plugin.Close(ctx)` at the end of the running path with the same `if m.closed.CompareAndSwap(false, true)`. In `cmd/noda/runtime.go` createWasm, on load failure release what's loaded:

```go
		if _, err := rt.LoadModule(context.Background(), cfg); err != nil {
			_ = rt.StopAll(context.Background()) // release already-loaded modules (#365)
			return nil, fmt.Errorf("loading wasm module %q: %w", name, err)
		}
```

Mirror the same `StopAll`-on-failure in `internal/testing/cookbook/runner.go`'s module-load loop.

- [ ] **Step 4: Run to verify pass**: `go test ./internal/wasm/ ./cmd/noda/`.
- [ ] **Step 5: Commit**: `fix(wasm): close never-started modules; unload partial loads on mid-loop failure (#365)`

### Task 6: #358 workflow.run docs match the engine; delete dead setOutputs

**Files:**
- Modify: `plugins/core/workflow/run.go` (drop `outputs` field, `setOutputs`, simplify `routeOutput`), `docs/03-nodes/workflow.run.md`
- Test: `plugins/core/workflow/` existing tests (adjust any `setOutputs` callers)

- [ ] **Step 1: Simplify the executor** — in `run.go`: delete the `outputs []string` field, `setOutputs`, and the factory's outputs literal; replace:

```go
func newRunExecutor(_ map[string]any) api.NodeExecutor { return &RunExecutor{} }

func (e *RunExecutor) Outputs() []string { return api.DefaultOutputs() }

// routeOutput maps the sub-workflow's terminal output name onto this node's
// two declared ports: "error" stays "error", every other workflow.output
// name routes through "success" (data preserved). Dynamic per-name ports
// are not implemented — see docs/03-nodes/workflow.output.md.
func (e *RunExecutor) routeOutput(outputName string) string {
	if outputName == "error" {
		return "error"
	}
	return "success"
}
```

Fix tests that called `setOutputs` to instead assert the success-collapse (`routeOutput("custom") == "success"`, `routeOutput("error") == "error"`).

- [ ] **Step 2: Run**: `go test ./plugins/core/workflow/` — expect PASS.
- [ ] **Step 3: Fix the doc** — in `docs/03-nodes/workflow.run.md`, replace the dynamic-output-ports claim with the truth (keep the `**Example:**` link line untouched): outputs are exactly `success` and `error`; the sub-workflow's `workflow.output` name is not surfaced as a port — any name other than `error` routes through `success` with its data preserved; branch on the data, not the port; consistent with `workflow.output.md` §success-collapse and the cookbook workflow family README.
- [ ] **Step 4: Verify**: `go build ./... && go test ./plugins/core/workflow/`; confirm `grep -rn setOutputs plugins/ internal/` is empty and `grep -n "Example:" docs/03-nodes/workflow.run.md` still hits.
- [ ] **Step 5: Commit**: `docs(workflow.run): document real success/error routing; remove dead setOutputs (#358)`

### Task 7: #357 fix saas-backend sync-github-issue

**Files:**
- Modify: `examples/saas-backend/workers/sync-github-issue.json`, `examples/saas-backend/workflows/sync-github-issue.json`, `examples/saas-backend/README.md`
- Create: `examples/saas-backend/tests/test-sync-github-issue.json`

- [ ] **Step 1: Fix the worker mapping** — `issue.id` is a JSON number headed for a TEXT column; GitHub payloads never carry workspace/project ids, so drop those mappings:

```json
    "input": {
      "action": "{{ message.payload.action }}",
      "issue_id": "{{ string(message.payload.issue.id) }}",
      "title": "{{ message.payload.issue.title }}",
      "body": "{{ message.payload.issue.body }}"
    }
```

(`string()` is an expr-lang builtin; confirm with `noda validate` below.)

- [ ] **Step 2: Rework the workflow** — landing-zone routing: on `opened`, look up the oldest project; create there; skip with a log when no project exists. Replace `sync-github-issue.json`'s nodes/edges:

```json
{
  "id": "sync-github-issue",
  "name": "Sync GitHub Issue",
  "nodes": {
    "switch_action": {
      "config": { "cases": ["opened", "closed"], "expression": "{{ input.action }}" },
      "type": "control.switch"
    },
    "find_project": {
      "config": { "table": "projects", "order": "created_at asc", "limit": 1 },
      "services": { "database": "main-db" },
      "type": "db.find"
    },
    "has_project": {
      "config": { "condition": "{{ len(nodes.find_project) > 0 }}" },
      "type": "control.if"
    },
    "create_task": {
      "config": {
        "table": "tasks",
        "data": {
          "title": "{{ input.title }}",
          "description": "{{ input.body }}",
          "status": "todo",
          "github_issue_id": "{{ input.issue_id }}",
          "project_id": "{{ nodes.find_project[0].id }}",
          "workspace_id": "{{ nodes.find_project[0].workspace_id }}"
        }
      },
      "services": { "database": "main-db" },
      "type": "db.create"
    },
    "log_no_project": {
      "config": { "level": "warn", "message": "No project exists; skipping GitHub issue {{ input.issue_id }}" },
      "type": "util.log"
    },
    "close_task": {
      "config": { "table": "tasks", "data": { "status": "done" }, "where": { "github_issue_id": "{{ input.issue_id }}" } },
      "services": { "database": "main-db" },
      "type": "db.update"
    },
    "log_unknown": {
      "config": { "level": "info", "message": "Ignoring GitHub issue action: {{ input.action }}" },
      "type": "util.log"
    }
  },
  "edges": [
    { "from": "switch_action", "output": "opened", "to": "find_project" },
    { "from": "find_project", "output": "success", "to": "has_project" },
    { "from": "has_project", "output": "then", "to": "create_task" },
    { "from": "has_project", "output": "else", "to": "log_no_project" },
    { "from": "switch_action", "output": "closed", "to": "close_task" },
    { "from": "switch_action", "output": "default", "to": "log_unknown" }
  ]
}
```

- [ ] **Step 3: Add a workflow test** — `examples/saas-backend/tests/test-sync-github-issue.json`, mirroring `test-handle-github-webhook.json`'s fixture format exactly (id/workflow/cases/mocks/expect shape):

two cases — "opened issue lands in the oldest project" (mock `find_project` → `[{"id": "p1", "workspace_id": "w1"}]`, mock `create_task` → `{"id": "t1"}`, expect success) and "opened issue with no projects is skipped" (mock `find_project` → `[]`, expect success).

- [ ] **Step 4: Verify**: `go run ./cmd/noda validate --config examples/saas-backend` clean; `go run ./cmd/noda test --config examples/saas-backend` passes (db nodes are mocked; requires Task 3's secrets wiring for the webhook suite — order this task after Task 3).
- [ ] **Step 5: Document** — README section for the GitHub sync worker: landing-zone convention (oldest project), why (GitHub payloads carry no workspace routing), and the update-on-close behavior.
- [ ] **Step 6: Commit**: `fix(examples): saas-backend github sync — string issue id, landing-zone project routing (#357)`

### Task 8: PR A assembly

- [ ] **Step 1: CHANGELOG** — under `[Unreleased]` → `### Fixed`: one line each for #354, #355, #356, #357, #358 (docs), #359, #365.
- [ ] **Step 2: Full verification**: `go build ./... && go vet ./... && go test ./internal/... ./plugins/... ./cmd/... ./pkg/...` — all green (integration tests skip without services).
- [ ] **Step 3: Push and open PR**: `git push -u origin backlog-2026-07-18`; `gh pr create --title "fix: backlog pass A — strict-mode namespaces, test secrets, sub-workflow wiring, wasm partial-load, workflow.run docs, saas example" --body "<summary + Fixes #354 #355 #356 #357 #358 #359 #365 + footer>"`; `gh pr merge --squash --auto`.

---

# PR B — behavior changes (branch `backlog-2026-07-18-b` off `origin/main`)

Start: `git checkout -b backlog-2026-07-18-b origin/main`.

### Task 9: #361 unwired error ports keep typed errors

**Files:**
- Modify: `internal/engine/context.go` (nodeErrors storage), `internal/engine/dispatch.go:70-86`, `internal/engine/executor.go:171-182`
- Test: `internal/engine/executor_test.go`, `internal/server/errors_test.go` (or equivalent)

**Interfaces:**
- Produces: `(*ExecutionContextImpl).SetNodeError(nodeID string, err error)` and `(*ExecutionContextImpl).NodeError(nodeID string) error` — same mutex discipline as SetOutput/GetOutput.

- [ ] **Step 1: Write the failing test** (in `internal/engine/executor_test.go`, using the package's existing fake-registry helpers for a node type whose Execute returns `&api.ValidationError{Field: "f", Message: "bad"}` and declares `[success, error]` outputs, in a workflow with NO error edge):

```go
func TestExecuteGraph_UnwiredErrorEdgeKeepsTypedError(t *testing.T) {
	// build graph: single node "n" of the failing type, no edges
	// (follow the existing ExecuteGraph test scaffolding in this file)
	err := engineExecuteForTest(t /* graph with failing node, no error edge */)
	require.Error(t, err)
	var valErr *api.ValidationError
	assert.True(t, errors.As(err, &valErr),
		"typed node error must survive the no-error-edge wrap, got: %v", err)
}
```

- [ ] **Step 2: Run to verify failure**: `go test ./internal/engine/ -run TestExecuteGraph_UnwiredErrorEdge -v` — FAIL (plain fmt.Errorf breaks the errors.As chain).
- [ ] **Step 3: Implement.** `context.go`: add `nodeErrors map[string]error` to `ExecutionContextImpl` (init alongside outputs) plus the two accessors under the existing mutex. `dispatch.go`, in the has-error-edges branch (line ~75), before returning `"error", nil`:

```go
			execCtx.SetNodeError(node.ID, execErr) // keep the typed error for the no-edge wrap (#361)
```

`executor.go` no-edge branch:

```go
			if len(errorTargets) == 0 {
				errData, _ := execCtx.GetOutput(nodeID)
				var nodeErr error
				if orig := execCtx.NodeError(nodeID); orig != nil {
					// %w keeps errors.As working so MapErrorToHTTP can type it.
					nodeErr = fmt.Errorf("node %q failed with no error edge: %w", nodeID, orig)
				} else {
					nodeErr = fmt.Errorf("node %q failed with no error edge: %v", nodeID, errData)
				}
```

- [ ] **Step 4: Add the mapping-level test** (server package): `MapErrorToHTTP(fmt.Errorf("node \"u\" failed with no error edge: %w", &api.ValidationError{Field: "file", Message: "type"}), "t", false)` returns 422 — plus keep an eye on existing tests asserting 500 for this shape (update them: that's the intended behavior change).
- [ ] **Step 5: Run**: `go test ./internal/engine/ ./internal/server/` — PASS.
- [ ] **Step 6: CHANGELOG** (`### Changed`): "Typed node errors (ValidationError, NotFoundError, …) now map to their HTTP statuses even when no error edge is wired — was: generic 500 INTERNAL_ERROR (#361)."
- [ ] **Step 7: Commit**: `fix(engine): retain typed node errors through the no-error-edge failure path (#361)`

### Task 10: #364 http body deep-resolves nested templates

**Files:**
- Modify: `internal/plugin/resolve.go` (new helper), `plugins/http/request.go:100`
- Modify: `docs/03-nodes/http.post.md`, `docs/03-nodes/http.request.md`, `examples/node-cookbook/http/workflows/post.json` (+ that family README if it mentions the workaround)
- Test: `internal/plugin/resolve_test.go`, `plugins/http/` node tests

**Interfaces:**
- Produces: `plugin.ResolveOptionalDeepAny(nCtx api.ExecutionContext, config map[string]any, key string) (any, bool, error)` — absent key → `(nil, false, nil)`.

- [ ] **Step 1: Write failing tests.** Helper test in `internal/plugin/resolve_test.go` (mirror the existing ResolveDeepAny test's fake nCtx): nested map `{"message": "{{ input.message }}"}` resolves the inner template; absent key → `ok == false`. Node test in `plugins/http` (mirror the existing httptest-server-based tests): node config `"body": map[string]any{"message": "{{ input.message }}"}` with input `{"message": "hi"}` → test server receives JSON `{"message":"hi"}`, not the literal template.
- [ ] **Step 2: Run to verify failure**: `go test ./internal/plugin/ ./plugins/http/ -run 'DeepAny|Body' -v` — helper missing (compile fail), then node test FAILs on literal template.
- [ ] **Step 3: Implement.** `resolve.go`:

```go
// ResolveOptionalDeepAny resolves an optional config key, recursively
// resolving expression strings inside nested maps and slices.
// Returns (nil, false, nil) if the key is absent.
func ResolveOptionalDeepAny(nCtx api.ExecutionContext, config map[string]any, key string) (any, bool, error) {
	raw, ok := config[key]
	if !ok {
		return nil, false, nil
	}
	val, err := resolveDeep(nCtx, raw)
	if err != nil {
		return nil, false, fmt.Errorf("resolve %q: %w", key, err)
	}
	return val, true, nil
}
```

`plugins/http/request.go`: `bodyVal, bodyOk, bodyErr := plugin.ResolveOptionalDeepAny(nCtx, config, "body")`.

- [ ] **Step 4: Run to verify pass**: `go test ./internal/plugin/ ./plugins/http/`.
- [ ] **Step 5: Docs + cookbook.** In both http node pages: nested body templates now resolve (show `"body": {"message": "{{ input.message }}"}`); keep `**Example:**` lines. Cookbook `post.json`: replace `"body": "{{ {message: input.message} }}"` with the natural nested form — this makes the cookbook CI run the live verification. Remove the workaround note from the http family README if present.
- [ ] **Step 6: CHANGELOG** (`### Changed`): "http.post/http.request `body` now deep-resolves nested expression templates like sse.send/ws.send/event.emit — was: maps/slices passed through verbatim with `{{ … }}` text unevaluated (#364)."
- [ ] **Step 7: Commit**: `fix(http): deep-resolve nested templates in request body config (#364)`

### Task 11: #350 multipart repeated values normalize to []any

**Files:**
- Modify: `internal/server/trigger.go:175-181,199-205,215-225` (shared helper)
- Modify: `docs/03-nodes/upload.handle.md`
- Test: `internal/server/trigger_test.go`

- [ ] **Step 1: Write the failing test** (mirror the existing multipart tests in `trigger_test.go`, including the #339 uppercase-mediatype one — build a multipart body with a repeated `tags` field, exercise both the fasthttp path (`multipart/form-data`) and the manual fallback (`MULTIPART/FORM-DATA`)):

```go
// assert: parsed body["tags"] has type []any{"a", "b"} in BOTH content-type spellings
```

- [ ] **Step 2: Run to verify failure**: `go test ./internal/server/ -run Multipart -v` — FAIL with `[]string`.
- [ ] **Step 3: Implement** — add a helper in `trigger.go` and use it in all three form branches (both multipart loops and the urlencoded loop):

```go
// formValue normalizes repeated form fields to []any so downstream code
// (expressions, control.loop) sees one type regardless of content type (#350).
func formValue(v []string) any {
	if len(v) == 1 {
		return v[0]
	}
	vals := make([]any, len(v))
	for i, s := range v {
		vals[i] = s
	}
	return vals
}
```

- [ ] **Step 4: Run to verify pass**: `go test ./internal/server/`.
- [ ] **Step 5: Document the file-upload limitation** in `docs/03-nodes/upload.handle.md` (keep the `**Example:**` line): uppercase `MULTIPART/FORM-DATA` with **file** fields fails loudly (`trigger mapping: file field …`) because trigger `files` extraction uses fasthttp's `c.FormFile`, which only recognizes the lowercase media type; form values have a manual fallback, files do not; use lowercase `multipart/form-data` when uploading files.
- [ ] **Step 6: CHANGELOG** (`### Changed`): "Multipart repeated form values now normalize to `[]any` like urlencoded — was: `[]string`, which broke `control.loop` and type-switched expressions (#350)."
- [ ] **Step 7: Commit**: `fix(server): normalize multipart repeated form values to []any; document uppercase file-upload limitation (#350)`

### Task 12: #349 hot reload runs dry-run validation; validateFile scopes dry-run errors

**Files:**
- Modify: `internal/devmode/reload.go` (dryRun hook + gate), `cmd/noda/main.go` (~:441, wire hook), `internal/server/editor_validation.go` (validateFile scoping)
- Test: `internal/devmode/reload_test.go`, `internal/server/editor_validation_test.go`

**Interfaces:**
- Produces: `(*Reloader).SetDryRun(fn func(*config.ResolvedConfig) []error)` — optional; nil keeps today's behavior.

- [ ] **Step 1: Write failing reload test** (follow the existing reload_test harness — tmp config dir, NewReloader, HandleChange):

```go
func TestHandleChange_DryRunFailureKeepsOldConfig(t *testing.T) {
	// valid-on-disk config so ValidateAll passes
	r := newTestReloader(t) // package's existing helper pattern
	old := r.Config()
	r.SetDryRun(func(*config.ResolvedConfig) []error {
		return []error{fmt.Errorf("workflow \"w\", node \"n\": config schema violation")}
	})
	r.HandleChange("workflows/w.json")
	assert.Same(t, old, r.Config(), "dry-run failure must refuse the swap (#349)")
}
```

Plus the inverse: nil dryRun / no errors → config swaps (likely already covered; extend if not).

- [ ] **Step 2: Run to verify failure**: `go test ./internal/devmode/ -run DryRun -v` — compile fail (no SetDryRun), then behavior.
- [ ] **Step 3: Implement reloader** — field + setter on `Reloader`; in `HandleChange` after the `ValidateAll` error block and before the shutdown re-check:

```go
	// Hot reload must agree with boot/CLI/editor validation (#345, #349):
	// run the same dry-run startup checks and refuse the swap on failure.
	if r.dryRun != nil {
		if dryErrs := r.dryRun(rc); len(dryErrs) > 0 {
			r.logger.Warn("config reload failed dry-run validation — keeping previous config",
				"errors", len(dryErrs), "trigger", path)
			msgs := make([]string, len(dryErrs))
			for i, e := range dryErrs {
				msgs[i] = e.Error()
				r.logger.Warn("validation error", "message", e.Error())
			}
			if r.hub != nil {
				r.hub.Emit(trace.Event{
					Type:  "file:error",
					Error: strings.Join(msgs, "\n"),
					Data:  map[string]any{"file": path, "count": len(dryErrs)},
				})
			}
			return
		}
	}
```

Wire in `cmd/noda/main.go` after `NewReloader`:

```go
			reloader.SetDryRun(func(rc *config.ResolvedConfig) []error {
				deferred, errs := registry.CollectDeferredServices(rc)
				return append(errs, registry.ValidateStartupDryRun(rc, rtCtx.Plugins, rtCtx.Bootstrap.Nodes, rtCtx.Bootstrap.Compiler, deferred)...)
			})
```

- [ ] **Step 4: Write failing validateFile test** (editor_validation_test harness): project with two workflow files, one containing a node-config error that only dry-run catches; `POST validateFile` for the CLEAN file → `valid: true`; for the BROKEN file → `valid: false` with `file` set to that file's path (not `""`).
- [ ] **Step 5: Implement scoping** — in `editor_validation.go` validateFile, replace the `if len(errs) == 0` block:

```go
	if len(errs) == 0 {
		// Scope dry-run errors to the requested file's workflows (#349):
		// rc.Workflows is keyed by file path, so restrict to this file.
		scoped := *rc
		if wf, ok := rc.Workflows[absPath]; ok {
			scoped.Workflows = map[string]map[string]any{absPath: wf}
		} else {
			scoped.Workflows = nil // non-workflow file: workflow dry-run errors are unrelated here
		}
		for _, dErr := range e.startupDryRunErrors(&scoped) {
			filtered = append(filtered, map[string]any{
				"file":    absPath,
				"path":    "",
				"message": dErr.Error(),
			})
		}
	}
```

(`validateAll` keeps full-project behavior.)

- [ ] **Step 6: Run**: `go test ./internal/devmode/ ./internal/server/ ./cmd/noda/` — PASS.
- [ ] **Step 7: CHANGELOG** (`### Changed`): "Dev-mode hot reload now runs the same dry-run startup validation as boot/validate/editor and refuses the swap on failure (emits `file:error`) — was: node-config violations hot-reloaded 'successfully' (#349). Editor per-file validation scopes dry-run errors to the saved file — was: unrelated workflows' errors shown with empty file attribution."
- [ ] **Step 8: Commit**: `fix(devmode): gate hot reload on dry-run validation; scope editor per-file dry-run errors (#349)`

### Task 13: PR B assembly

- [ ] **Step 1**: Full verification (same commands as Task 8 Step 2).
- [ ] **Step 2**: `git push -u origin backlog-2026-07-18-b`; PR titled `fix: backlog pass B — typed error mapping, http deep-resolve, multipart []any, reload/validate parity` with `Fixes #349 #350 #361 #364` + footer; `gh pr merge --squash --auto`.

---

# PR C — sync bridge + livekit timeout (branch `backlog-2026-07-18-c` off `origin/main`)

Start: `git checkout -b backlog-2026-07-18-c origin/main`.

### Task 14: SyncBridge core (#363)

**Files:**
- Create: `internal/connmgr/sync.go`, `internal/connmgr/sync_test.go`

**Interfaces:**
- Produces: `connmgr.Envelope{V, Instance, Kind, Channel, Payload, Event, ID}` (JSON tags `v/instance/kind/channel/payload/event,omitempty/id,omitempty`); `NewSyncBridge(pubsub api.PubSubService, instanceID string, logger *slog.Logger) *SyncBridge`; `(*SyncBridge).Publish(ctx, endpoint string, env Envelope) error`; `(*SyncBridge).Run(ctx, endpoint string, mgr *Manager)` (blocking; returns on ctx cancel). Redis channel: `"noda:sync:" + endpoint`.

- [ ] **Step 1: Write the failing tests** with an in-memory fake bus implementing `api.PubSubService` (Publish → JSON-marshal payload, fan out to subscribed handlers as `map[string]any` after an unmarshal round-trip — mimicking `plugins/pubsub/service.go` exactly; Subscribe → register handler, block until ctx done; also a `failPublish bool` switch and an error-injection mode where Subscribe returns an error once then works). Cases:

```go
// 1. ws cross-delivery: bridgeA.Publish(kind "ws") → managerB conn's SendFn
//    receives the exact payload bytes; the publishing instance's own manager
//    receives nothing via the bridge (self-echo skipped by instance id).
// 2. sse cross-delivery: event and id fields survive; SSEFn receives them.
// 3. malformed envelope on the bus: logged, dropped, loop alive (next good
//    envelope still delivered).
// 4. unknown v or kind: dropped without delivery.
// 5. subscribe error: Run retries after backoff (set b.backoff = time.Millisecond
//    in the test) and delivers after recovery.
// 6. ctx cancel: Run returns.
```

Register conns via `mgr.Register(&Conn{ID: "c1", Channel: "room:1", SendFn: ...})`.

- [ ] **Step 2: Run to verify failure**: `go test ./internal/connmgr/ -run Sync -v` — compile fail.
- [ ] **Step 3: Implement `sync.go`:**

```go
package connmgr

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/chimpanze/noda/pkg/api"
)

// syncChannelPrefix namespaces cross-instance sync traffic in the pubsub service.
const syncChannelPrefix = "noda:sync:"

// Envelope is the versioned cross-instance sync message. Payload carries the
// pre-marshaled bytes the local manager delivered, so every instance emits
// byte-identical frames and no JSON round-trip mangling occurs.
type Envelope struct {
	V        int    `json:"v"`
	Instance string `json:"instance"`
	Kind     string `json:"kind"` // "ws" or "sse"
	Channel  string `json:"channel"`
	Payload  string `json:"payload"`
	Event    string `json:"event,omitempty"` // SSE only
	ID       string `json:"id,omitempty"`    // SSE only
}

// SyncBridge fans ConnectionService sends out to other instances through a
// pubsub service and feeds remote envelopes into the local Manager (#363).
type SyncBridge struct {
	pubsub     api.PubSubService
	instanceID string
	logger     *slog.Logger
	backoff    time.Duration // subscribe retry delay; shortened in tests
}

func NewSyncBridge(pubsub api.PubSubService, instanceID string, logger *slog.Logger) *SyncBridge {
	if logger == nil {
		logger = slog.Default()
	}
	return &SyncBridge{pubsub: pubsub, instanceID: instanceID, logger: logger, backoff: time.Second}
}

// Publish sends an envelope to the endpoint's sync channel. Errors surface to
// the caller: with sync configured, a lost publish means remote users silently
// miss messages, so the sending node fails loudly.
func (b *SyncBridge) Publish(ctx context.Context, endpoint string, env Envelope) error {
	env.V = 1
	env.Instance = b.instanceID
	return b.pubsub.Publish(ctx, syncChannelPrefix+endpoint, env)
}

// Run subscribes to the endpoint's sync channel and delivers remote envelopes
// to mgr until ctx is cancelled, reconnecting with backoff on subscribe errors.
func (b *SyncBridge) Run(ctx context.Context, endpoint string, mgr *Manager) {
	channel := syncChannelPrefix + endpoint
	for {
		err := b.pubsub.Subscribe(ctx, channel, func(payload any) error {
			b.deliver(ctx, endpoint, mgr, payload)
			return nil // a bad message must never kill the subscription
		})
		if ctx.Err() != nil {
			return
		}
		b.logger.Warn("connection sync: subscribe failed; retrying",
			"endpoint", endpoint, "error", err)
		select {
		case <-ctx.Done():
			return
		case <-time.After(b.backoff):
		}
	}
}

func (b *SyncBridge) deliver(ctx context.Context, endpoint string, mgr *Manager, payload any) {
	raw, err := json.Marshal(payload) // pubsub hands us a decoded map; round-trip into the struct
	if err != nil {
		b.logger.Warn("connection sync: envelope marshal failed", "endpoint", endpoint, "error", err)
		return
	}
	var env Envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		b.logger.Warn("connection sync: malformed envelope dropped", "endpoint", endpoint, "error", err)
		return
	}
	if env.Instance == b.instanceID {
		return // Redis echoes to the publisher; local delivery already happened
	}
	if env.V != 1 {
		b.logger.Warn("connection sync: unknown envelope version dropped", "endpoint", endpoint, "v", env.V)
		return
	}
	switch env.Kind {
	case "ws":
		if err := mgr.Send(ctx, env.Channel, env.Payload); err != nil {
			b.logger.Debug("connection sync: remote ws delivery failed", "channel", env.Channel, "error", err)
		}
	case "sse":
		if err := mgr.SendSSE(ctx, env.Channel, env.Event, env.Payload, env.ID); err != nil {
			b.logger.Debug("connection sync: remote sse delivery failed", "channel", env.Channel, "error", err)
		}
	default:
		b.logger.Warn("connection sync: unknown envelope kind dropped", "endpoint", endpoint, "kind", env.Kind)
	}
}
```

(String payloads pass through `marshalData`/`marshalDataString` unmodified, so remote frames are byte-identical.)

- [ ] **Step 4: Run to verify pass**: `go test ./internal/connmgr/ -run Sync -v`.
- [ ] **Step 5: Commit**: `feat(connmgr): SyncBridge — cross-instance envelope publish/subscribe (#363)`

### Task 15: wire the bridge — EndpointService, registerConnections, lifecycle, schema

**Files:**
- Modify: `internal/connmgr/endpoint.go`, `internal/server/connections.go:16-58`, `internal/server/server.go` (instanceID + sync context + StopRealtime), `cmd/noda/runtime.go:317`, `internal/config/schemas/connections.json:41`
- Test: `internal/connmgr/sync_test.go` (endpoint-service cases), `internal/server/` connections test, `internal/config/` schema test

**Interfaces:**
- Changes: `NewEndpointService(manager *Manager, endpoint string, bridge *SyncBridge) *EndpointService` (bridge nil = local-only). New: `(*Server).StopRealtime(ctx context.Context) error` — cancels sync subscribers, then `connManagers.Stop(ctx)`.

- [ ] **Step 1: Write failing tests.**
  - EndpointService: `Send` with bridge publishes a `kind:"ws"` envelope carrying the marshaled payload AND delivers locally; publish failure (fake bus `failPublish`) → `Send` returns an error wrapping "cross-instance sync publish"; nil bridge → local-only, no error. Same trio for `SendSSE` (`kind:"sse"`, event/id populated).
  - Schema: a connections config **without** `sync` validates cleanly (this currently fails); with `sync.pubsub` pointing at a non-pubsub service it still errors (crossrefs untouched).
- [ ] **Step 2: Run to verify failure**: `go test ./internal/connmgr/ ./internal/config/ -run 'Endpoint|Conn' -v`.
- [ ] **Step 3: Implement.** `endpoint.go`:

```go
// EndpointService wraps a Manager and implements api.ConnectionService.
// With a SyncBridge attached, sends also fan out to other instances; a
// publish failure fails the send (local delivery has already happened —
// callers wanting best-effort wire the node's error edge).
type EndpointService struct {
	manager  *Manager
	endpoint string
	bridge   *SyncBridge
}

func NewEndpointService(manager *Manager, endpoint string, bridge *SyncBridge) *EndpointService {
	return &EndpointService{manager: manager, endpoint: endpoint, bridge: bridge}
}

func (s *EndpointService) Send(ctx context.Context, channel string, data any) error {
	if err := s.manager.Send(ctx, channel, data); err != nil {
		return err
	}
	if s.bridge == nil {
		return nil
	}
	payload, err := marshalDataString(data)
	if err != nil {
		return err
	}
	if err := s.bridge.Publish(ctx, s.endpoint, Envelope{Kind: "ws", Channel: channel, Payload: payload}); err != nil {
		return fmt.Errorf("cross-instance sync publish: %w", err)
	}
	return nil
}

func (s *EndpointService) SendSSE(ctx context.Context, channel string, event string, data any, id string) error {
	if err := s.manager.SendSSE(ctx, channel, event, data, id); err != nil {
		return err
	}
	if s.bridge == nil {
		return nil
	}
	payload, err := marshalDataString(data)
	if err != nil {
		return err
	}
	if err := s.bridge.Publish(ctx, s.endpoint, Envelope{Kind: "sse", Channel: channel, Payload: payload, Event: event, ID: id}); err != nil {
		return fmt.Errorf("cross-instance sync publish: %w", err)
	}
	return nil
}
```

`server.go`: add fields `instanceID string`, `syncCtx context.Context`, `syncCancel context.CancelFunc`; in `NewServer`: `s.instanceID = uuid.New().String()` and `s.syncCtx, s.syncCancel = context.WithCancel(context.Background())`; add:

```go
// StopRealtime cancels cross-instance sync subscribers, then closes all
// connections. Registered with the lifecycle manager in place of the bare
// ManagerGroup stop.
func (s *Server) StopRealtime(ctx context.Context) error {
	s.syncCancel()
	return s.connManagers.Stop(ctx)
}
```

`connections.go` `registerConnections`, at the top of the per-config loop:

```go
		var bridge *connmgr.SyncBridge
		if syncCfg, ok := connConfig["sync"].(map[string]any); ok {
			pubsubName, _ := syncCfg["pubsub"].(string)
			if pubsubName != "" {
				raw, found := s.services.Get(pubsubName)
				if !found {
					return fmt.Errorf("connections sync: pubsub service %q not found", pubsubName)
				}
				ps, ok := raw.(api.PubSubService)
				if !ok {
					return fmt.Errorf("connections sync: service %q does not implement PubSubService", pubsubName)
				}
				bridge = connmgr.NewSyncBridge(ps, s.instanceID, s.logger)
			}
		}
```

then `svc := connmgr.NewEndpointService(mgr, name, bridge)` and, after the endpoint's handler registration succeeds:

```go
			if bridge != nil {
				go bridge.Run(s.syncCtx, name, mgr)
				s.logger.Info("cross-instance sync active", "endpoint", name)
			}
```

`cmd/noda/runtime.go:317`: register `StopRealtime` instead of the bare group — inspect `lifecycle.ConnManagerComponent`'s definition and either generalize its parameter to `interface{ Stop(context.Context) error }` (ManagerGroup already satisfies it; pass an adapter struct whose Stop calls `comps.Server.StopRealtime`) or add a sibling constructor — smallest diff wins. `connections.json`: delete the `"required": ["sync"]` line.

- [ ] **Step 4: Update existing call sites** of `NewEndpointService` (2-arg) and any connections-schema fixtures that relied on `sync` being required.
- [ ] **Step 5: Run**: `go test ./internal/connmgr/ ./internal/server/ ./internal/config/ ./cmd/noda/` — PASS.
- [ ] **Step 6: Commit**: `feat(server): wire cross-instance connection sync; make connections sync optional (#363)`

### Task 16: Redis integration test for the bridge

**Files:**
- Create: `internal/connmgr/sync_integration_test.go` (gate exactly like the existing Redis-backed integration tests — copy the env-var/skip pattern from `plugins/pubsub/plugin_test.go`)

- [ ] **Step 1: Write the test**: two `Manager`s + two `SyncBridge`s (distinct instance IDs) sharing one real `pubsub.Service`; register a conn on manager B; `NewEndpointService(mgrA, "chat", bridgeA).Send(ctx, "room:1", map[string]any{"n": 1})`; assert B's conn receives `{"n":1}` bytes within a timeout; assert A's own conn does NOT receive a duplicate.
- [ ] **Step 2: Run**: `go test ./internal/connmgr/ -run Integration -v` — skips without Redis; with local Redis (`docker compose up -d redis` or the repo's standard dev services) it must PASS.
- [ ] **Step 3: Commit**: `test(connmgr): redis integration coverage for cross-instance sync (#363)`

### Task 17: #363 docs + changelog

**Files:**
- Modify: `docs/02-config/connections.md` (field table rows 7-8 + §Cross-Instance Message Routing at :175-212), `examples/node-cookbook/realtime/README.md` (+ its `connections/rooms.json` stays as-is — it now exercises the real bridge in cookbook CI), `CHANGELOG.md`

- [ ] **Step 1: connections.md** — mark `sync` optional in the field table ("absent = local-only delivery"); rewrite §How It Works to the implemented truth: (1) local manager delivers first; (2) the send publishes a versioned envelope to `noda:sync:<endpoint>` on the configured pubsub service; (3) every instance subscribed to that endpoint receives it and delivers to its local connections, skipping its own echo; (4) **no routing table** — fan-out goes to all instances with the endpoint (delete the old claim); (5) a failed publish fails the sending node (local delivery already done) — wire the error edge for best-effort.
- [ ] **Step 2: cookbook realtime README** — replace the "Honest scope note" (unimplemented) with the implemented behavior + pointer to connections.md; keep whatever step structure the family README uses.
- [ ] **Step 3: CHANGELOG** — `### Added`: "Cross-instance WebSocket/SSE delivery via `sync.pubsub` is now implemented (#363)." `### Changed`: "`connections` `sync` block is now optional — was: schema-required while unused."
- [ ] **Step 4: Verify**: `go test ./internal/config/` (schema fixtures), `grep -rn "routing table" docs/` returns nothing.
- [ ] **Step 5: Commit**: `docs(connections): document real cross-instance sync; sync now optional (#363)`

### Task 18: #368 livekit service-level timeout

**Files:**
- Create: `plugins/livekit/timeout.go`, `plugins/livekit/timeout_test.go`
- Modify: `plugins/livekit/plugin.go` (CreateService), `plugins/livekit/service.go` (doc comment), `examples/node-cookbook/livekit/` (service config + README), the doc page carrying the livekit service config (locate: `grep -rln api_secret docs/`)

**Interfaces:**
- Produces: `callWithTimeout[T any](ctx context.Context, d time.Duration, op string, call func(context.Context) (T, error)) (T, error)`; wrapper types `timeoutRoomClient`, `timeoutEgressClient`, `timeoutIngressClient` implementing the three interfaces in `plugins/livekit/interfaces.go`. Service config gains optional `"timeout"` (Go duration string); unset = no deadline (today's behavior).

- [ ] **Step 1: Write failing tests**: (a) fake `EgressClient` whose `StartRoomCompositeEgress` blocks until ctx done → wrapped with `d=20ms`, returns within ~50ms with an error containing `timed out after` and the op name, and `errors.Is(err, context.DeadlineExceeded)`; (b) fast fake returning a twirp-style error → passes through UNCHANGED; (c) `CreateService` with `"timeout": "5s"` wraps clients, with `"timeout": "nope"` errors, without timeout leaves bare clients.
- [ ] **Step 2: Run to verify failure**: `go test ./plugins/livekit/ -run Timeout -v` — compile fail.
- [ ] **Step 3: Implement `timeout.go`**:

```go
package livekit

import (
	"context"
	"errors"
	"fmt"
	"time"

	lkproto "github.com/livekit/protocol/livekit"
)

// callWithTimeout bounds one LiveKit API call by the service-level timeout.
// A deadline expiry produces an operation-scoped error; errors returned by
// the server before the deadline pass through unchanged.
func callWithTimeout[T any](ctx context.Context, d time.Duration, op string, call func(context.Context) (T, error)) (T, error) {
	tctx, cancel := context.WithTimeout(ctx, d)
	defer cancel()
	res, err := call(tctx)
	if err != nil && errors.Is(tctx.Err(), context.DeadlineExceeded) && ctx.Err() == nil {
		var zero T
		return zero, fmt.Errorf("livekit %s: request timed out after %s: %w", op, d, err)
	}
	return res, err
}

type timeoutRoomClient struct {
	inner RoomClient
	d     time.Duration
}

func (c *timeoutRoomClient) CreateRoom(ctx context.Context, req *lkproto.CreateRoomRequest) (*lkproto.Room, error) {
	return callWithTimeout(ctx, c.d, "CreateRoom", func(ctx context.Context) (*lkproto.Room, error) { return c.inner.CreateRoom(ctx, req) })
}
```

…and the remaining 9 `RoomClient` methods, `timeoutEgressClient` (4 methods), `timeoutIngressClient` (3 methods), all in the same one-liner shape. In `plugin.go` `CreateService`, before constructing `Service`:

```go
	var timeout time.Duration
	if v, ok := config["timeout"].(string); ok && v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return nil, fmt.Errorf("livekit: invalid timeout %q: %w", v, err)
		}
		timeout = d
	}
```

and after building the three clients:

```go
	if timeout > 0 {
		return &Service{
			Room:      &timeoutRoomClient{inner: roomClient, d: timeout},
			Egress:    &timeoutEgressClient{inner: egressClient, d: timeout},
			Ingress:   &timeoutIngressClient{inner: ingressClient, d: timeout},
			APIKey:    apiKey,
			APISecret: apiSecret,
		}, nil
	}
```

- [ ] **Step 4: Run to verify pass**: `go test ./plugins/livekit/`.
- [ ] **Step 5: Cookbook + docs**: add `"timeout": "5s"` to the livekit service in `examples/node-cookbook/livekit`'s noda.json; update that family README's slow-error-path narrative (egress-unavailable now fails in ~5s with the timeout error — adjust any step assertions that match on the old error text/duration); document `timeout` in the livekit service config docs page. CHANGELOG `### Added`: "livekit service accepts an optional `timeout` (per-API-call deadline); unset keeps unbounded calls (#368)."
- [ ] **Step 6: Commit**: `feat(livekit): optional service-level request timeout (#368)`

### Task 19: PR C assembly

- [ ] **Step 1**: Full verification (Task 8 Step 2 commands); additionally run the connmgr integration test against local Redis if available.
- [ ] **Step 2**: `git push -u origin backlog-2026-07-18-c`; PR titled `feat: cross-instance connection sync bridge + livekit request timeouts` with `Fixes #363 #368` + footer; `gh pr merge --squash --auto`.
- [ ] **Step 3**: After all three PRs merge: verify `gh issue list` shows only #351 remaining; update memory notes.
