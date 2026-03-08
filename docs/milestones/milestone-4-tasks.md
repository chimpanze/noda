# Milestone 4: Workflow Engine — Core Execution — Task Breakdown

**Depends on:** Milestone 1 (config loading), Milestone 2 (expression engine), Milestone 3 (plugin/service registry)
**Result:** The graph executor runs workflows — nodes execute in dependency order, parallel branches run concurrently, joins converge correctly, retries work on error edges, trace IDs propagate through execution.

---

## Task 4.1: Graph Compiler

**Description:** Parse workflow nodes and edges into an executable DAG with computed metadata.

**Subtasks:**

- [ ] Create `internal/engine/compiler.go`
- [ ] Implement `Compile(workflow WorkflowConfig) (*CompiledGraph, error)`
- [ ] `CompiledGraph` contains:
  - Adjacency map: for each node, which nodes it leads to (grouped by output name)
  - Reverse adjacency: for each node, which nodes lead into it
  - Entry nodes: nodes with no inbound edges
  - Terminal nodes: nodes with no outbound `success` edges
  - Dependency count per node: how many inbound edges must fire before the node runs
  - Node metadata: type, compiled config expressions, service slot references
- [ ] Cycle detection: walk the graph, detect back edges. If found, produce error listing the cycle path.
- [ ] Validate edge references: every `from` and `to` must reference existing node IDs. Every `output` must be a valid output for the source node type.
- [ ] Compute join types per node:
  - If all inbound edges come from independent parallel branches → AND-join (wait for all)
  - If inbound edges come from mutually exclusive branches (different outputs of a conditional) → OR-join (wait for whichever fires)
  - Determine this by tracing each inbound edge back to find the common ancestor and checking if it's a conditional split

**Tests:**
- [ ] Linear workflow: A → B → C compiles correctly
- [ ] Parallel workflow: A → C, B → C identifies C as AND-join
- [ ] Conditional: if → then → merge, if → else → merge identifies merge as OR-join
- [ ] Cycle detection catches A → B → A
- [ ] Invalid edge (references non-existent node) produces error
- [ ] Invalid output name on edge produces error
- [ ] Entry nodes correctly identified
- [ ] Terminal nodes correctly identified

**Acceptance criteria:** Workflows compile into executable graphs with correct join semantics.

---

## Task 4.2: Execution Context Implementation

**Description:** Implement the runtime execution context that nodes interact with.

**Subtasks:**

- [ ] Create `internal/engine/context.go`
- [ ] Implement `ExecutionContextImpl` satisfying `api.ExecutionContext`:
  - `Input()` — returns `$.input`
  - `Auth()` — returns `*api.AuthData` or nil
  - `Trigger()` — returns `api.TriggerData`
  - `Resolve(expression string)` — delegates to expression engine's resolver
  - `Log(level, message, fields)` — delegates to structured logger with trace ID context
- [ ] Node output storage:
  - `SetOutput(nodeID string, data any)` — store a node's output
  - `GetOutput(nodeID string) (any, bool)` — retrieve a node's output
  - Outputs accessible in expressions as `{{ nodeID }}` or `{{ nodeID.field }}`
- [ ] `as` alias support: if a node has `"as": "user"`, its output is stored under `"user"` instead of the node ID
- [ ] Thread-safe: parallel nodes write outputs concurrently
- [ ] Trace ID: generated at execution start, available via `Trigger().TraceID`

**Tests:**
- [ ] Input/Auth/Trigger return correct values
- [ ] SetOutput → GetOutput round-trip
- [ ] `as` alias overrides node ID
- [ ] Resolve accesses node outputs in expressions
- [ ] Concurrent output writes don't race (run with `-race`)
- [ ] Trace ID is present and unique per execution

**Acceptance criteria:** Context stores outputs thread-safely and supports expression resolution.

---

## Task 4.3: Node Dispatcher

**Description:** Execute a single node: resolve services, call Execute, store output, determine next edge.

**Subtasks:**

- [ ] Create `internal/engine/dispatch.go`
- [ ] Implement `dispatchNode(ctx context.Context, node *CompiledNode, execCtx *ExecutionContextImpl, services *ServiceRegistry, nodes *NodeRegistry) (outputName string, err error)`:
  - Look up node's executor factory from node registry
  - Create executor instance with the node's raw config
  - Resolve service slots: for each slot in the node's `services`, look up the service instance from the service registry
  - Call `executor.Execute(ctx, execCtx, config, resolvedServices)`
  - Store output data on the execution context under node ID (or `as` alias)
  - Return which output fired
- [ ] If Execute returns an error AND the node has no `error` output edge → propagate as workflow failure
- [ ] If Execute returns an error AND the node has an `error` output edge → store error data, return `"error"` as output name
- [ ] `context.Context` is passed through — if cancelled, execution stops

**Tests:**
- [ ] Successful node execution stores output and returns `"success"`
- [ ] Failed node with error edge returns `"error"` and stores error data
- [ ] Failed node without error edge returns error
- [ ] Service slots resolved correctly
- [ ] Context cancellation stops execution
- [ ] `as` alias used for output storage

**Acceptance criteria:** Individual nodes execute with correct service resolution and error routing.

---

## Task 4.4: Parallel Execution Engine

**Description:** The main execution loop that runs nodes in parallel where possible and handles joins.

**Subtasks:**

- [ ] Create `internal/engine/executor.go`
- [ ] Implement `Execute(ctx context.Context, graph *CompiledGraph, execCtx *ExecutionContextImpl, ...) error`:
  - Start all entry nodes concurrently (goroutines)
  - When a node completes, determine which outbound edge to follow based on the output name
  - For each target node of the followed edge:
    - Decrement its pending dependency count (atomic)
    - If all dependencies satisfied (AND-join: count reaches 0) or the relevant branch fired (OR-join) → dispatch the node
  - Continue until all terminal nodes complete or an unhandled error occurs
- [ ] Use `sync.WaitGroup` or channel-based coordination for goroutine management
- [ ] Respect `context.Context` deadline — if context expires, cancel all running nodes
- [ ] Handle the "no response node" case — workflow completes when all reachable terminal nodes finish
- [ ] Error propagation: unhandled node error (no error edge) → workflow fails, cancel remaining nodes

**Tests:**
- [ ] Linear workflow executes in order
- [ ] Parallel branches run concurrently (verify with timing — two 50ms nodes complete in ~50ms, not ~100ms)
- [ ] AND-join waits for all parallel branches
- [ ] OR-join fires when the taken branch arrives
- [ ] Context cancellation stops all running nodes
- [ ] Unhandled error cancels remaining nodes and returns error
- [ ] Terminal nodes with no outbound success edges mark completion

**Acceptance criteria:** Workflows execute with correct parallelism and join semantics.

---

## Task 4.5: Memory Management — Output Eviction

**Description:** Evict node outputs from the execution context once no more nodes need them.

**Subtasks:**

- [ ] Create `internal/engine/eviction.go`
- [ ] At compile time: for each node output, compute the set of downstream nodes that reference it in their expressions (by analyzing compiled expressions)
- [ ] Track a reference count per output: initialized to the number of downstream dependents
- [ ] After each node execution: decrement reference counts for all upstream outputs the node consumed
- [ ] When a reference count reaches 0: remove the output from the context map, allowing GC to reclaim memory
- [ ] `as` aliases tracked the same way

**Tests:**
- [ ] Output evicted after last dependent executes
- [ ] Output NOT evicted while dependents remain
- [ ] `as` alias eviction works
- [ ] Parallel branches — eviction only after both branches consume the output
- [ ] Large output data is actually GC'd (memory profile test)

**Acceptance criteria:** Node outputs are freed as soon as no downstream node needs them.

---

## Task 4.6: Retry Logic on Error Edges

**Description:** Implement retry behavior on error edges before following them.

**Subtasks:**

- [ ] Create `internal/engine/retry.go`
- [ ] When an edge has `retry` config and a node dispatches to it:
  - Before following the error edge, re-execute the source node
  - Up to `retry.attempts` times
  - With delay between attempts:
    - `"fixed"` backoff: constant delay (e.g., `"1s"`)
    - `"exponential"` backoff: delay doubles each attempt (1s, 2s, 4s, ...)
  - Parse `retry.delay` as duration string
- [ ] If the node succeeds on a retry: fire its `success` output instead, don't follow the error edge
- [ ] If all retries exhausted: follow the error edge as normal
- [ ] Context deadline: if `context.Context` expires during a backoff sleep, cancel immediately and follow the error edge. No more retry attempts.
- [ ] Log each retry attempt with attempt number, delay, and node ID

**Tests:**
- [ ] Node fails once, succeeds on retry → success output fires
- [ ] Node fails all retries → error edge followed
- [ ] Fixed backoff: delays are constant
- [ ] Exponential backoff: delays double
- [ ] Context deadline cancels retry during backoff sleep
- [ ] Context deadline cancels retry during node re-execution
- [ ] Retry config only valid on error edges (validated at compile time)
- [ ] No retry config → error edge followed immediately

**Acceptance criteria:** Retries work with both backoff strategies and respect context deadlines.

---

## Task 4.7: Trace ID and Basic Logging

**Description:** Generate trace IDs and provide structured logging through execution.

**Subtasks:**

- [ ] Generate a UUID trace ID at the start of every workflow execution
- [ ] Store trace ID in execution context, accessible via `Trigger().TraceID`
- [ ] All log entries during execution include `trace_id` field
- [ ] Implement `nCtx.Log(level, message, fields)`:
  - Delegates to Go's `slog` package
  - Automatically adds: `trace_id`, `workflow_id`, `node_id` (if in node context)
  - Levels: `debug`, `info`, `warn`, `error`
- [ ] Log key execution events automatically:
  - Workflow started (info): workflow ID, trigger type, trace ID
  - Node started (debug): node ID, node type
  - Node completed (debug): node ID, duration
  - Node failed (warn): node ID, error
  - Retry attempted (info): node ID, attempt number, delay
  - Workflow completed (info): workflow ID, status, duration

**Tests:**
- [ ] Trace ID is unique per execution
- [ ] Trace ID appears in all log output for an execution
- [ ] Log levels filter correctly
- [ ] Automatic execution events are logged
- [ ] nCtx.Log includes node context when called from a node

**Acceptance criteria:** Every execution has a trace ID. All logs are structured with execution context.

---

## Task 4.8: Integration Tests

**Description:** Full workflow execution tests with mock nodes.

**Subtasks:**

- [ ] Create a set of mock node executors for testing:
  - `mock.pass` — always succeeds, returns config data as output
  - `mock.fail` — always fails with a configurable error
  - `mock.slow` — succeeds after a configurable delay
  - `mock.conditional` — succeeds or fails based on input expression
- [ ] Test: linear workflow (3 nodes in sequence) — verify execution order and data flow
- [ ] Test: parallel workflow (2 entry nodes → 1 join node) — verify concurrent execution
- [ ] Test: diamond workflow (A → B, A → C, B → D, C → D) — verify AND-join
- [ ] Test: conditional split with OR-join — verify only taken branch triggers join
- [ ] Test: error path — node fails, error edge followed, error node receives error data
- [ ] Test: retry — node fails, retry config on error edge, node succeeds on second attempt
- [ ] Test: context timeout — slow node exceeds deadline, workflow fails
- [ ] Test: output eviction — verify outputs are removed after last dependent
- [ ] Test: complex graph — 10+ nodes with mixed parallel, conditional, and error paths

**Acceptance criteria:** Workflow engine handles all graph patterns correctly.
