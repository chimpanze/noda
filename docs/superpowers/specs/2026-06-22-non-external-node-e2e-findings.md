# Findings — End-to-End Verification of Non-External Nodes

**Date:** 2026-06-22
**Branch:** `feat/non-external-node-e2e`
**Spec:** `docs/superpowers/specs/2026-06-22-non-external-node-e2e-design.md`
**Plan:** `docs/superpowers/plans/2026-06-22-non-external-node-e2e.md`

## Summary

Every Noda node with no external-service dependency now has end-to-end coverage through the real engine. The effort surfaced **two genuine production bugs** (both in the sub-workflow / `workflow.output` path) and added one production-grade enhancement to the test runner. All fixes are in place and guarded by tests.

- `go build ./...` — clean
- `go vet ./...` — clean
- `go test ./...` — all packages pass
- `noda test --config testdata/node-e2e` — **26 passed, 0 failed**

## Scope verified

**Tier A — pure-core nodes, driven via `noda test` JSON fixtures (config → engine → node, executed for real).** 19 workflow suites under `testdata/node-e2e/`, run by `internal/testing/e2e/run_test.go` (`TestNodeE2E`) and by the `noda test` CLI:

- control.if, control.switch, control.loop
- transform.set, transform.map, transform.filter, transform.merge, transform.delete, transform.validate
- response.json, response.redirect, response.error
- util.uuid, util.timestamp, util.delay, util.log, util.jwt_sign
- workflow.run, workflow.output

Each has a happy path plus, where meaningful, an edge/error path (e.g. `transform.map` non-array → `status:"error"`, `transform.validate` missing required field, `control.if`/`control.switch` branch selection, empty-collection cases). Non-deterministic outputs (uuid/timestamp/jwt) are validated by shape via a downstream `transform.set` adapter (length 36, positivity, 3 JWT segments) so the real node runs while assertions stay deterministic.

**Tier B — runtime-backed nodes, driven through `engine.ExecuteGraph` with in-process services (no external dependency).** Go integration tests in each plugin package:

- ws.send → `plugins/core/ws/engine_e2e_test.go` (real `connmgr`)
- sse.send → `plugins/core/sse/engine_e2e_test.go` (real `connmgr`)
- wasm.send, wasm.query → `plugins/core/wasm/engine_e2e_test.go` (in-process Wasm runtime + mock `PluginInstance`)
- upload.handle → `plugins/core/upload/engine_e2e_test.go` (in-memory afero storage)

Each asserts the node's actual effect (message/event delivered, command queued, query response returned, file stored with correct metadata) plus an error path.

**Excluded by design:** `event.emit` — it only ever publishes to Redis (stream/pubsub) and has no internal-only mode, so it falls outside the "no external service" criterion. All other external-service nodes (db, cache, stream, pubsub, storage, image, http, email, livekit, oidc) are out of scope for this pass.

## Bugs found and fixed

### Bug 1 — Sub-workflows were broken end-to-end (`control.loop`, `workflow.run`)

**Severity:** High (core feature non-functional through the engine).
**Symptom:** Any sub-workflow that terminates in a `workflow.output` node failed when executed through the engine, so `control.loop` and `workflow.run` could not actually run a sub-workflow.
**Root cause (two compounding issues):**

1. `internal/engine/dispatch.go` — `workflow.output` is a terminal node with empty declared outputs (`Outputs() == []`). Its `Execute` returns the dynamic output *name* from config (e.g. `"result"`). The dispatch path then ran the declared-output validation, which rejected the name with `returned undeclared output "result"`.
2. `plugins/core/workflow/run.go` — `workflow.run` returned the sub-workflow's raw output name as its own port. That name is not in `workflow.run`'s declared ports (`success`/`error`), so dispatch rejected it too.

**Why it was latent:** no existing test drove `workflow.output` through real dispatch; unit tests exercised the executors in isolation or with mock runners. Baseline `go test` for `plugins/core/workflow`, `plugins/core/control`, and `internal/engine` passed without ever hitting this path.
**Fix:**
- `internal/engine/dispatch.go` — `workflow.output` returns early after recording its result (`SetWorkflowOutput` + `SetOutput` + trace), exempt from declared-output validation. Safe because the compiler forbids outgoing edges from a node with empty declared outputs, so edge-following is always a no-op for it.
- `plugins/core/workflow/run.go` — added `routeOutput`, which maps the sub-workflow's terminal output name onto a declared port: a matching declared name is honored (test-only dynamic branching via `setOutputs`); otherwise it routes through `success` with the data preserved. Applied to both the plain and transaction paths.

**Commits:** `0a34288`, `649fb4b`.
**Guarded by:** `internal/testing/runner_subworkflow_test.go` (`TestRunner_WorkflowRunSubWorkflow`), plus `tests/control-loop.test.json` and `tests/workflow-run.test.json` via `TestNodeE2E`.

### Bug 2 — `workflow.output` silently dropped non-string `data`

**Severity:** Medium (silent data loss).
**Symptom:** A `workflow.output` configured with object- or array-shaped `data` (e.g. `{ "msg": "{{ input.m }}" }`) emitted `nil` instead of the resolved structure.
**Root cause:** `plugins/core/workflow/output.go` resolved `data` only when it was a `string` expression (`if dataExpr, ok := config["data"].(string); ok`); any other shape fell through to `return name, nil, nil`. It was masked because every prior fixture used string-form `data` (`"{{ nodes.x }}"`).
**Fix:** added a recursive `resolveOutputData` helper that resolves string leaves via `nCtx.Resolve` and walks `map[string]any` / `[]any`, mirroring the structured resolution used by `transform.set` and `response.json`. String-form `data` behaves identically to before.
**Commit:** `c5bd7d8`.
**Guarded by:** `tests/workflow-output.test.json` (`out.msg == "hi"`) via `TestNodeE2E`.

## Enhancement — test runner now supports sub-workflows

`internal/testing/runner.go` previously called `engine.ExecuteGraph` without a `SubWorkflowRunner`, so `control.loop` / `workflow.run` could not be exercised by `noda test` at all. The runner now builds a best-effort `SubWorkflowRunnerImpl` (from the resolved config + test registry) and injects it; if the project's workflows can't all be compiled with the test resolver (e.g. unmocked plugin nodes), it falls back to the prior nil-runner behavior, so no existing project regresses. **Commit:** `0a34288`.

## Files added

- `testdata/node-e2e/` — `noda.json`, 21 workflow files, 19 test suites.
- `internal/testing/e2e/run_test.go` — Go driver running every suite through the engine.
- `internal/testing/runner_subworkflow_test.go` — runner sub-workflow regression test.
- `plugins/core/{ws,sse,wasm,upload}/engine_e2e_test.go` — Tier B engine integration tests.

## Residual notes

- Mocked plugin nodes inside a sub-workflow are not supported by the test runner's best-effort cache (such a sub-workflow either skips runner injection or surfaces a clear error). Not needed for any in-scope node; documented for future readers.
- `workflow.run` dynamic output branching (`setOutputs`) remains a test-only affordance; production always uses `success`/`error`. `routeOutput` is written to honor injected branches if that feature is ever wired.
