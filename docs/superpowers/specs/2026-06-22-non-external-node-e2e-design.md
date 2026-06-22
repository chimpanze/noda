# Design: End-to-End Verification of Non-External Nodes

**Date:** 2026-06-22
**Status:** Approved (pending spec review)
**Goal:** Make every Noda node that has *no connection to an external service* verifiably work end-to-end, fixing any bugs found, so the product is production-ready for those nodes.

## Scope

### In scope

Nodes that can execute without any external service (no Redis, no Postgres, no SMTP, no LiveKit, no OIDC provider, no outbound HTTP). Two tiers:

**Tier A — pure compute, executed for real by `noda test` (config → engine → node):**

| Plugin | Nodes |
|--------|-------|
| control | `control.if`, `control.switch`, `control.loop` |
| transform | `transform.set`, `transform.map`, `transform.filter`, `transform.merge`, `transform.delete`, `transform.validate` |
| response | `response.json`, `response.redirect`, `response.error` |
| util | `util.log`, `util.uuid`, `util.delay`, `util.timestamp`, `util.jwt_sign` |
| workflow | `workflow.run`, `workflow.output` |

**Tier B — needs an in-process service with no external dependency, so Go integration tests:**

| Plugin | Nodes | In-process service |
|--------|-------|--------------------|
| ws | `ws.send` | `connmgr` connection manager |
| sse | `sse.send` | `connmgr` connection manager |
| wasm | `wasm.send`, `wasm.query` | embedded Wasm runtime (mock `PluginInstance`) |
| upload | `upload.handle` | in-memory afero storage service |

### Out of scope

- **`event.emit`** — unconditionally publishes to Redis stream/pubsub; it has no internal-only mode, so by the "no external service" rule it is excluded. (Confirmed with user 2026-06-22.)
- All external-service plugins: `db.*`, `cache.*`, `stream.*`, `pubsub.*`, `storage.*` (the standalone Redis/Postgres-backed plugins), `image.*`, `http.*`, `email.*`, `lk.*` (LiveKit), `oidc.*`.
- The React editor, docs, and any node not listed above.

## Background: how Noda runs workflows under test

- `internal/testing/runner.go` `RunTestSuite` → `runTestCase` parses a workflow from `ResolvedConfig`, builds a node registry where **core nodes execute for real** and **non-core plugin nodes are mocked** (or fail clearly if unmocked), compiles with `engine.Compile`, runs `engine.ExecuteGraph`, then matches a `TestExpectation`.
- The `noda test` CLI (`cmd/noda/main.go`) discovers `*.test.json` / `test-*.json` files under a project's `tests/` dir, builds the core node registry via `corePlugins()`, and runs each suite.
- JSON test fixture shape (`internal/testing/types.go`): `{ id, workflow, tests: [{ name, input, auth?, mocks?, expect: { status, output (dot-path map), outputs (partial deep-match), error_node } }] }`.

This means **Tier A nodes are genuinely exercised end-to-end** by `noda test` — no mocking of the node under test.

### Known gap to fix

`runTestCase` calls `engine.ExecuteGraph` **without** injecting a `SubWorkflowRunner` (`runner.go:127-128`). The engine reads it via `ExecutionContext.SubWorkflowRunner()` (`internal/engine/context.go:136-143`) for `control.loop` and `workflow.run`. So today those two nodes cannot run through `noda test`. This is a real production gap and is in scope to fix (confirmed with user 2026-06-22).

## Approach

Use Noda's own test machinery rather than a parallel harness.

### 1. Fix the test runner (production fix)

In `internal/testing/runner.go`, build a `engine.SubWorkflowRunnerImpl` from the resolved config + the test node registry + service registry, and inject it via `engine.WithSubWorkflowRunner(...)` when constructing the execution context. Sub-workflows invoked by `control.loop`/`workflow.run` must resolve against the same test registry (so their plugin nodes are mocked the same way) and the same `ResolvedConfig` (so sub-workflow IDs resolve).

Add a focused unit test in `internal/testing` proving a workflow that uses `control.loop` over a sub-workflow, and one that uses `workflow.run`, execute and produce expected outputs.

### 2. Tier A: `testdata/node-e2e/` project + Go driver

Create a self-contained Noda project `testdata/node-e2e/`:

```
testdata/node-e2e/
  noda.json                 # minimal config: routes/workflows registry
  workflows/
    control-if.json
    control-switch.json
    control-loop.json
    transform-set.json
    ... (one workflow per node/behavior)
  tests/
    control-if.test.json
    ... (a suite per workflow)
```

Each workflow is the smallest graph that exercises the target node and routes its result into a `workflow.output` (or `response.json`) so the assertion has a stable address. Each suite asserts:

- **happy path** — node produces correct output for representative input;
- **at least one edge/error path** — e.g. `control.if` false branch, `transform.validate` failure, `control.switch` default, `transform.filter` empty result, expression error surfacing on the `error` output.

A new Go test `internal/testing/node_e2e_test.go` loads `testdata/node-e2e/` (via the same config-loading path the CLI uses), runs every suite through `RunTestSuite`, and fails if any case fails. This keeps it in `go test ./...` and makes `noda test testdata/node-e2e` green too.

Per-node coverage targets for Tier A:

- `control.if`: then/else branches.
- `control.switch`: matched case + default.
- `control.loop`: iterate a collection, accumulate via sub-workflow, empty collection.
- `transform.set`: set static + expression-derived fields.
- `transform.map`: map over array.
- `transform.filter`: keep/drop, empty result.
- `transform.merge`: merge two objects, precedence.
- `transform.delete`: remove key(s).
- `transform.validate`: pass + fail (error output).
- `response.json`: status + body.
- `response.redirect`: status + location.
- `response.error`: status + message.
- `util.uuid`: shape/format assertion (non-empty, v4 pattern).
- `util.timestamp`: format assertion.
- `util.delay`: completes; small duration.
- `util.log`: passes input through; no crash.
- `util.jwt_sign`: signs; verifiable structure (three segments / decodable claims).
- `workflow.run`: calls sub-workflow, returns its output.
- `workflow.output`: terminal output mapping.

### 3. Tier B: service-wired Go integration tests

For each Tier B node, a Go integration test in the plugin package that:

1. wires the real in-process service (no external dependency):
   - ws/sse: `connmgr.NewManager()` + `connmgr.NewEndpointService(...)` with a registered `Conn` capturing delivered bytes;
   - wasm: `wasmrt.WasmService` with a mock `PluginInstance`;
   - upload: an in-memory afero (`afero.NewMemMapFs()`) storage service + a synthesized multipart request in the execution context;
2. compiles a small workflow with `engine.Compile` and runs `engine.ExecuteGraph` (full engine path, not just the executor in isolation);
3. asserts the **effect**: ws/sse message delivered to the right channel; wasm command queued / query response returned; upload file written to storage with correct metadata;
4. asserts the **error path**: missing channel/service, wasm call error, upload size/type rejection.

Where an existing `*_test.go` already covers the executor in isolation, extend it to drive through `engine.ExecuteGraph` so the test is genuinely end-to-end rather than a direct `Execute()` call.

## Bug handling

Fix each bug in place as it is found (user-approved mode). Keep a running list and present a summary at the end covering: node, symptom, root cause, fix, test that now guards it.

## Error handling & edge cases the tests must cover

- Expression evaluation errors route to the node's `error` output (not a panic).
- Missing required config is a clear error, not a nil-deref.
- `control.loop` with an empty collection produces an empty/zero result, not a hang.
- `transform.*` on absent paths behaves per documented semantics.
- Tier B: missing required service yields a descriptive error.

## Definition of done

- Every in-scope node has end-to-end coverage (Tier A JSON suite or Tier B Go integration test) for happy path + at least one edge/error path.
- The runner sub-workflow fix is implemented and unit-tested.
- All bugs found are fixed; summary delivered.
- `go test ./...` passes.
- `noda test testdata/node-e2e` passes.
- No regression in existing tests.

## Out-of-scope follow-ups (noted, not done here)

- End-to-end coverage of external-service nodes (needs Redis/Postgres/etc. fixtures or testcontainers).
- `event.emit` once an internal/no-op delivery mode exists (or via embedded Redis).
