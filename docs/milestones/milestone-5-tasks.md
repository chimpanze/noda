# Milestone 5: Core Control Nodes — Task Breakdown

**Depends on:** Milestone 4 (workflow engine)
**Result:** Conditional branching, multi-way switching, loops over collections, sub-workflow invocation, and sub-workflow output declaration all work. The engine validates `workflow.output` mutual exclusivity at startup.

---

## Task 5.1: Core Control Node Plugin Shell

**Description:** Create the core control node plugin that registers under the `control` prefix and the workflow plugin under the `workflow` prefix.

**Subtasks:**

- [ ] Create `plugins/core/control/plugin.go`:
  - Name: `"core.control"`, Prefix: `"control"`
  - HasServices: false
  - Nodes: registers `control.if`, `control.switch`, `control.loop`
- [ ] Create `plugins/core/workflow/plugin.go`:
  - Name: `"core.workflow"`, Prefix: `"workflow"`
  - HasServices: false
  - Nodes: registers `workflow.run`, `workflow.output`
- [ ] Both plugins implement `api.Plugin` with no-op service methods
- [ ] Register both plugins in the plugin registry during startup

**Tests:**
- [ ] Plugins register successfully with correct prefixes
- [ ] Node types are retrievable: `control.if`, `control.switch`, `control.loop`, `workflow.run`, `workflow.output`

**Acceptance criteria:** Core control and workflow plugins register and expose their node types.

---

## Task 5.2: `control.if` Node

**Description:** Evaluate a condition expression and route to `then` or `else`.

**Subtasks:**

- [ ] Create `plugins/core/control/if.go`
- [ ] Implement descriptor:
  - Name: `"if"`
  - ServiceDeps: empty
  - ConfigSchema: `{ "condition": { "type": "string" } }` (required)
- [ ] Implement executor:
  - Outputs: `["then", "else", "error"]`
  - Execute: resolve `condition` expression via `nCtx.Resolve()`, evaluate truthiness
  - Truthy → return `"then"` with the resolved value
  - Falsy → return `"else"` with the resolved value
  - Expression error → return error (engine routes to `"error"` output)
- [ ] Truthiness rules: `false`, `nil`, `0`, `""`, empty array → falsy. Everything else → truthy.

**Tests:**
- [ ] `true` condition → `then` output
- [ ] `false` condition → `else` output
- [ ] Expression `{{ input.role == "admin" }}` evaluates correctly
- [ ] `nil` result → `else`
- [ ] `0` → `else`, `1` → `then`
- [ ] Empty string → `else`, non-empty → `then`
- [ ] Empty array → `else`, non-empty → `then`
- [ ] Invalid expression → error output
- [ ] Output data is the resolved condition value

**Acceptance criteria:** Conditional branching works with all Go truthiness edge cases.

---

## Task 5.3: `control.switch` Node

**Description:** Evaluate an expression and match against static case names.

**Subtasks:**

- [ ] Create `plugins/core/control/switch.go`
- [ ] Implement descriptor:
  - Name: `"switch"`
  - ServiceDeps: empty
  - ConfigSchema: `expression` (required string), `cases` (required array of strings, static)
- [ ] Implement factory: reads `cases` from config at compile time, returns executor with those cases as outputs
- [ ] Implement executor:
  - Outputs: `[...cases, "default", "error"]` — dynamic from config
  - Execute: resolve `expression`, convert result to string, match against case names
  - Match found → return case name as output
  - No match → return `"default"`
  - Expression error → return error
- [ ] Validate at startup: `cases` must be static strings (no expressions), validated by the expression engine's static field detection

**Tests:**
- [ ] Expression matches a case → correct output fires
- [ ] No match → `default` fires
- [ ] Expression error → error fires
- [ ] Cases are strings — integer expression result is converted to string for matching
- [ ] Empty cases array → everything goes to default
- [ ] Factory produces correct outputs from config
- [ ] Static validation rejects expressions in `cases` field

**Acceptance criteria:** Multi-way branching works with dynamic output ports.

---

## Task 5.4: `workflow.output` Node

**Description:** Terminal node that declares a named output and return data for sub-workflows.

**Subtasks:**

- [ ] Create `plugins/core/workflow/output.go`
- [ ] Implement descriptor:
  - Name: `"output"`
  - ServiceDeps: empty
  - ConfigSchema: `name` (required string, static), `data` (optional, expression)
- [ ] Implement executor:
  - Outputs: `[]` — empty, terminal node (no outbound edges)
  - Execute: resolve `data` expression if present, return it. The engine uses the `name` field from config to determine which output the parent `workflow.run` fires.
- [ ] The `name` field must be static — validated at startup
- [ ] The node must have no outbound edges — validated at compile time

**Tests:**
- [ ] Output returns resolved data
- [ ] Output with no data returns nil
- [ ] Name is accessible from config for parent workflow routing
- [ ] Static validation rejects expression in `name` field
- [ ] Compile-time validation rejects outbound edges

**Acceptance criteria:** Sub-workflow outputs declare their name and data correctly.

---

## Task 5.5: `workflow.output` Mutual Exclusivity Validation

**Description:** Validate at startup that all `workflow.output` nodes in a sub-workflow are on mutually exclusive branches.

**Subtasks:**

- [ ] Create `internal/engine/exclusivity.go`
- [ ] Implement `ValidateOutputExclusivity(graph *CompiledGraph) error`:
  - Find all `workflow.output` nodes in the graph
  - For each pair: trace backwards to find their common ancestor
  - If the common ancestor is a conditional node (`control.if`, `control.switch`) and the two outputs descend from different branches → mutually exclusive (valid)
  - If the two outputs can both be reached in a single execution (e.g., both on parallel branches) → error
- [ ] Validate unique names: no two `workflow.output` nodes can have the same `name`
- [ ] Run this validation during startup for every workflow that is referenced by a `workflow.run` or `control.loop` node

**Tests:**
- [ ] Two outputs on if/then and if/else branches → valid
- [ ] Two outputs on different switch cases → valid
- [ ] Two outputs on parallel branches → error
- [ ] Two outputs with same name → error
- [ ] Single output → always valid
- [ ] Deeply nested conditional with outputs → valid

**Acceptance criteria:** Invalid sub-workflow output configurations are caught at startup.

---

## Task 5.6: `workflow.run` Node

**Description:** Invoke a sub-workflow with input mapping and dynamic outputs.

**Subtasks:**

- [ ] Create `plugins/core/workflow/run.go`
- [ ] Implement factory:
  - Read `workflow` field (static) from config
  - Look up the referenced sub-workflow from loaded configs
  - Collect all `workflow.output` node names from the sub-workflow
  - Return executor with those names + `"error"` as outputs
- [ ] Implement executor:
  - Resolve `input` map expressions against current context
  - Create a new execution context for the sub-workflow with `$.input` from resolved input map
  - Propagate `$.auth` and `$.trigger` from parent context
  - Execute the sub-workflow using the workflow engine
  - When a `workflow.output` node fires: return its `name` as the output name, its `data` as the output data
  - If sub-workflow fails (unhandled error): return `"error"`
- [ ] Sub-workflow execution shares the parent's `context.Context` (deadline propagation)
- [ ] Trace ID propagation: sub-workflow uses the same trace ID as parent

**Tests:**
- [ ] Sub-workflow executes with correct input
- [ ] Output fires matching the sub-workflow's `workflow.output` name
- [ ] Auth and trigger propagate to sub-workflow
- [ ] Sub-workflow failure → error output
- [ ] Context deadline cancels sub-workflow
- [ ] Trace ID is consistent between parent and sub-workflow
- [ ] Nested sub-workflows work (workflow.run → workflow.run)
- [ ] Factory produces correct dynamic outputs

**Acceptance criteria:** Sub-workflow invocation works with dynamic outputs and proper context propagation.

---

## Task 5.7: `workflow.run` with `transaction: true` (Stub)

**Description:** Add the transaction flag to `workflow.run` config. Actual transaction implementation comes in Milestone 9 (Database Plugin), but the config field and service slot are prepared now.

**Subtasks:**

- [ ] Add `transaction` field to `workflow.run` config schema (boolean, optional, default false)
- [ ] Add `database` service slot to ServiceDeps (required only when `transaction: true`)
- [ ] When `transaction: true` but no database plugin loaded yet → startup validation error (expected until M9)
- [ ] When `transaction: false` or absent → no database slot required

**Tests:**
- [ ] `transaction: false` works without database slot
- [ ] `transaction: true` without database slot → startup error
- [ ] Config schema accepts `transaction` field

**Acceptance criteria:** Transaction config is recognized but not yet functional.

---

## Task 5.8: `control.loop` Node

**Description:** Iterate a sub-workflow over a collection, collecting results.

**Subtasks:**

- [ ] Create `plugins/core/control/loop.go`
- [ ] Implement descriptor:
  - Name: `"loop"`
  - ServiceDeps: empty
  - ConfigSchema: `collection` (required expression), `workflow` (required static string), `input` (required map of expressions)
- [ ] Implement executor:
  - Outputs: `["done", "error"]`
  - Resolve `collection` expression → must be an array
  - For each item in the array:
    - Create an expression context with `$item` (current element) and `$index` (zero-based index)
    - Resolve `input` map expressions against this context (plus parent context)
    - Execute the sub-workflow with the resolved input
    - Collect the sub-workflow's output data
  - Iterations run sequentially (current completes before next starts)
  - On completion: return `"done"` with array of collected results
  - On iteration failure (sub-workflow errors without error edge): return `"error"` immediately, skip remaining
- [ ] `$item` and `$index` only available within the loop's `input` expressions, not in the sub-workflow itself (the sub-workflow receives `$.input`)

**Tests:**
- [ ] Loop over 3 items → 3 iterations, collected results array has 3 entries
- [ ] `$item` resolves to current element
- [ ] `$index` resolves to zero-based index
- [ ] Sequential execution — each iteration completes before next starts
- [ ] Iteration failure → error fires, remaining skipped
- [ ] Empty collection → `done` fires with empty array
- [ ] Sub-workflow output data collected correctly
- [ ] Context deadline applies across all iterations

**Acceptance criteria:** Loops iterate collections sequentially, collecting results or failing fast.

---

## Task 5.9: Integration Tests

**Description:** End-to-end tests exercising all control flow patterns.

**Subtasks:**

- [ ] Test: workflow with `control.if` branching → correct path taken based on input
- [ ] Test: workflow with `control.switch` → correct case fires, default fires for no match
- [ ] Test: workflow with `control.loop` → iterates, collects results
- [ ] Test: workflow with `workflow.run` → sub-workflow executes, output propagates
- [ ] Test: nested sub-workflows → workflow.run calls workflow with workflow.run inside
- [ ] Test: loop with failing sub-workflow → error fires, collected results are partial
- [ ] Test: complex workflow combining if + switch + loop + sub-workflow in one graph
- [ ] Test: mutual exclusivity validation catches invalid sub-workflow at startup
- [ ] Test: `control.switch` with `workflow.run` on each case → dynamic outputs route correctly
- [ ] Write workflow test JSON files (using `noda test` format) for all the above

**Acceptance criteria:** All control flow patterns work correctly in combination.
