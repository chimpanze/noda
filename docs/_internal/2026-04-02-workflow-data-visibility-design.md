# Workflow Data Visibility & Test Generation

**Date:** 2026-04-02
**Status:** Approved

## Problem

When building workflows in the editor, there's no visibility into the data shapes flowing between nodes. You can't see what a node produces until you run it and dig into the trace panel. This makes it hard to write correct expressions referencing upstream nodes and hard to write tests since you have to manually reconstruct data at each step.

## Solution

Three complementary features that reinforce each other:

1. **Output Schema System** — know data shapes while building
2. **Canvas Data Visualization** — see actual data flowing through nodes after a run
3. **Test Generation from Trace Runs** — capture runs as test fixtures with one click

## Non-Goals

- No step-through debugger (no pause/resume/breakpoint engine changes)
- No backend schema storage (schemas live in editor localStorage)
- No schema validation at runtime (schemas are informational, not enforced)
- No Wasm output introspection (runtime-learned schemas cover them after first run)
- No schema export/import (schemas rebuild from trace runs)

---

## 1. Output Schema System

Three schema sources, one unified interface. The editor asks "what's the output shape of this node?" and gets a schema back regardless of source.

### 1.1 Static Schemas

Node descriptors gain an optional `OutputSchema() map[string]any` method returning JSON Schema. Only implemented for fixed-output nodes where the shape never changes.

Target nodes (~8-10):
- `cache.set` → `{ok: bool}`
- `cache.get` → `{value: any}`
- `cache.delete` → `{ok: bool}`
- `util.uuid` → `{value: string}`
- `util.timestamp` → `{value: string}`
- `storage.read` → `{data: bytes, size: int, content_type: string}`
- `storage.write` → `{ok: bool, path: string}`
- `http.request` → `{status: int, headers: object, body: any}`
- `wasm.send` → `{sent: bool}`
- `response.json` → `api.HTTPResponse` struct

These are part of the node implementation and change only if the node's output structure changes — which would break consumers anyway.

### 1.2 Config-Derived Schemas

A schema derivation function per node type that inspects the user's config and returns a partial schema. Runs in the editor (TypeScript), reading the node's config JSON. No backend call needed.

Examples:
- `transform.set` with `{"fields": {"name": "...", "total": "..."}}` → schema `{type: "object", properties: {name: {}, total: {}}}`
- `control.switch` with `{"cases": ["a", "b"]}` → output ports `a`, `b`, `default`
- `db.create` with known input fields → same keys in output

### 1.3 Runtime-Learned Schemas

When trace events arrive with `data`, the editor infers a JSON Schema from the concrete value and persists it. Updated on every trace run. Shown with a visual indicator that it's "learned" not "declared."

### 1.4 Priority

Static > config-derived > runtime-learned. If a static schema exists, that's authoritative. Config-derived fills in what it can. Runtime-learned fills the remaining gaps.

### 1.5 Unified API

A single editor function:

```typescript
getNodeOutputSchema(nodeId: string): OutputSchema | null
```

Checks all three sources in priority order and returns the best available schema. The canvas and TracePanel both call this.

The `OutputSchema` type includes the source for UI differentiation:

```typescript
interface OutputSchema {
  schema: JSONSchema;
  source: "static" | "config-derived" | "runtime-learned";
  stale: boolean;
}
```

---

## 2. Canvas Data Visualization

### 2.1 Data Badges on Nodes

After a trace run, each completed node shows a compact data badge directly on the canvas:
- Simple values: the value itself (`42`, `"active"`, `true`)
- Objects: key count and first 2-3 keys (`{name, email, +3}`)
- Arrays: length and element hint (`[5 rows]`)
- Click the badge to expand full data in a popover

### 2.2 Data Badges on Edges

The edge between two nodes shows what data flowed through it. Same compact format. Answers "what did node B actually receive from node A?" at a glance.

### 2.3 Schema Hints in Build Mode

Before running anything, nodes show their known output shape (from static or config-derived schemas) as a dimmed badge. After a run, the badge upgrades to show real data. Runtime-learned schemas appear with a dotted border to distinguish them from declared schemas.

### 2.4 Inline Data Diff

When re-running a workflow and the output shape of a node changes compared to the learned schema, the badge highlights the change (new keys, removed keys, type changes). Helps catch unexpected changes quickly.

### 2.5 Existing Panel Preserved

The TracePanel node detail view remains for full JSON inspection, copy-to-clipboard, etc. Badges are the quick view; the panel is the deep view.

---

## 3. Test Generation from Trace Runs

### 3.1 Export Flow

An "Export as Test" button in the TracePanel execution view. Generates a test JSON file from captured trace data.

### 3.2 Captured Data

- `input` — the workflow's input data (captured from the `workflow:started` event's `data` field; requires adding input data to this event if not already present)
- `auth` — the auth context if present (captured from `workflow:started` event; requires adding auth context to this event if not already present)
- `mocks` — every non-core node's output becomes a mock entry (`nodeId → {output, output_name}`)
- `expect` — defaults to `{status: "success"}` plus the final node's output as assertions

### 3.3 Core Nodes Stay Real

`transform.set`, `control.if`, `control.switch`, and other core nodes execute normally in tests. The generator only mocks plugin nodes (DB, cache, HTTP, storage, etc.) that have external dependencies.

### 3.4 Edit Before Saving

After generation, a modal shows the test JSON in an editable view:
- Rename the test case
- Remove or adjust mock values
- Add/modify expected output assertions
- Remove fields you don't care about (partial matching is supported)

### 3.5 Save Location

Saves to the project's test directory alongside the workflow config, following the existing `tests/*.json` convention. If a test file for this workflow already exists, the new test case is appended to the `tests` array.

### 3.6 Round-Trip Verification

After saving, the test can be run immediately from the editor to verify it passes. This closes the loop: build → run → see data → export test → run test.

---

## 4. Schema Inference Details

### 4.1 Inference Algorithm

When a trace event's `data` field arrives, derive a JSON Schema:
- Primitives: `42` → `{type: "number"}`, `"hello"` → `{type: "string"}`
- Objects: recurse into each key → `{type: "object", properties: {...}, required: [...]}`
- Arrays: infer element schema from first element, or union if mixed → `{type: "array", items: {...}}`
- Null: `{type: "null"}` — upgraded on next run if a real value appears

### 4.2 Schema Merging

When a new trace run produces data for a node that already has a learned schema, merge rather than replace:
- New keys get added
- Existing keys widen their type if needed (`string` + `number` → `anyOf`)
- Keys seen in previous runs but absent in this run stay (with `required` updated)

This builds a more complete picture over multiple runs with different inputs.

### 4.3 Storage

Learned schemas stored in localStorage as `noda:schema:{projectHash}:{workflowId}:{nodeId}`. A typical workflow's schemas are a few KB. Cleared when the user resets or when the workflow structure changes (nodes added/removed).

### 4.4 Staleness Detection

If a node's type or config changes in the editor, its learned schema is marked stale (shown dimmed). Next trace run refreshes it.

---

## Backend Changes

### Go: `pkg/api/node.go`

Add optional `OutputSchema() map[string]any` to `NodeDescriptor` interface (or as a separate interface to avoid breaking existing implementations):

```go
type NodeOutputSchemaProvider interface {
    OutputSchema() map[string]any // JSON Schema for the node's output
}
```

Nodes that implement this interface provide static schemas. The editor fetches these via a new lightweight API endpoint.

### Go: New endpoint `GET /editor/schemas`

Returns a map of `nodeType → outputSchema` for all registered nodes that implement `NodeOutputSchemaProvider`. Called once on editor load.

### Go: Minor trace system change

The `workflow:started` event needs to include `input` and `auth` in its `data` field so the editor can capture them for test generation. Currently node-level events carry `data`, but the workflow-level start event may not include the original input. This is a small addition to the event emission in the engine.

### Go: No changes to test runner

The existing test file format supports everything the generator needs. No modifications needed.

## Frontend Changes

### New: `editor/src/lib/schemaInference.ts`

- `inferSchema(data: unknown): JSONSchema` — derive schema from concrete value
- `mergeSchemas(existing: JSONSchema, incoming: JSONSchema): JSONSchema` — merge two schemas
- `schemaToCompactLabel(schema: JSONSchema): string` — generate badge text from schema

### New: `editor/src/stores/schema.ts`

Zustand store managing all three schema sources:
- Loads static schemas from `/editor/schemas` on init
- Computes config-derived schemas when node config changes
- Updates runtime-learned schemas from trace events
- Exposes `getNodeOutputSchema(nodeId)` unified API
- Manages staleness detection

### New: `editor/src/components/canvas/DataBadge.tsx`

Compact data/schema badge component rendered on nodes and edges. Handles all display modes (real data, schema hint, stale indicator, diff highlight).

### New: `editor/src/components/panels/TestExportModal.tsx`

Modal for reviewing and editing generated test JSON before saving. Includes a "Run Test" button for immediate verification.

### Modified: `editor/src/components/panels/TracePanel.tsx`

- Add "Export as Test" button per execution
- Wire up data badges to canvas after trace runs

### Modified: Canvas edge/node rendering

- Render `DataBadge` components on nodes (below the node) and edges (midpoint)
- Show schema hints in build mode, real data after trace runs
