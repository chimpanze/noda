# Visual Editor Completion Plan

## Current State Summary

**Fully functional:** Workflows view (canvas, node config, edge config, palette, drag-drop, undo/redo, auto-layout, copy/paste, tracing), Routes view (read-only listing), Services view (health monitoring), Schemas view (read-only Monaco), Tests view (display-only)

**Stub/placeholder views:** Workers, Schedules, Connections, Wasm, Migrations (all show "coming in future milestones")

**Backend API available but unused by UI:** `computeNodeOutputs`, `deleteFile`, `validateAll`, expression fields in routes

**Backend API not yet implemented:** `expressions/validate`, `expressions/context`, test execution, `file:changed` WebSocket events

---

## Phase 1: Complete CRUD for Existing Views

These views already display data but can't create, edit, or delete. The backend `PUT` and `DELETE` file endpoints already exist.

### 1.1 Routes View — Full CRUD

**Current:** Read-only table of routes grouped by file.
**Target:** Create, edit, delete routes with a form editor.

- Add "New Route" button that opens form in right panel
- Click existing route to open editable form:
  - Method (dropdown: GET/POST/PUT/PATCH/DELETE)
  - Path (text input with parameter highlighting)
  - Workflow (dropdown from `files.workflows`)
  - Input mapping (KeyValueMapField for `trigger.input`)
  - Middleware (multi-select/tag input)
  - Tags (tag input)
  - Request schemas: params, query, body (JSON Schema editor or `$ref` picker from `files.schemas`)
  - Response schemas by status code
- Save: serialize route back into its file, PUT to backend
- Delete route (with confirmation)
- "New Route File" button for creating new route config files

**Files modified:** `RoutesView.tsx` (major rewrite)
**Files created:** `RouteFormPanel.tsx`
**Store:** Add route mutation helpers to `editor.ts` or new `routes.ts` store

### 1.2 Schemas View — Edit & Save

**Current:** Read-only Monaco viewer.
**Target:** Full editing with save.

- Make Monaco editor writable (it already is, but no save button)
- Add Save button that PUTs to backend
- Add "New Schema" button to create new schema file
- Delete schema (with confirmation, warn if referenced)
- Show where each schema is referenced (which routes/validate nodes use `$ref`)

**Files modified:** `SchemasView.tsx` (moderate edit)

### 1.3 Tests View — Editing & Execution

**Current:** Display-only, run button disabled.
**Target:** Edit test cases and display results (execution still via CLI since no backend API).

- Make test case fields editable (input JSON, mocks, expected output)
- Save changes via PUT to backend
- Add "New Test Case" / "New Test File" buttons
- Delete test cases
- Show last run results if available (read from a results file or future API)

**Files modified:** `TestsView.tsx` (moderate rewrite)

---

## Phase 2: Build Missing Views

All use the same pattern: read files via `GET /_noda/files/*`, display in table, edit via form, save via `PUT /_noda/files/*`.

### 2.1 Workers View

**Config shape:** `{ workers: [{ id, stream, subscribe: { topic, group }, concurrency, middleware, trigger: { workflow, input } }] }`

- Table: ID, stream service, topic, group, concurrency, workflow
- Form panel:
  - ID (text)
  - Stream service (dropdown from services with `stream.` prefix)
  - Subscribe topic (text)
  - Consumer group (text)
  - Concurrency (number)
  - Middleware (tag input)
  - Trigger workflow (dropdown) + input mapping (key-value)
- CRUD: create/edit/delete workers within worker config files

**Files created:** `WorkersView.tsx` (rewrite stub), `WorkerFormPanel.tsx`

### 2.2 Schedules View

**Config shape:** `{ schedules: [{ id, cron, timezone, lock: { service, ttl }, trigger: { workflow, input } }] }`

- Table: ID, cron expression, human-readable description, timezone, workflow
- Form panel:
  - ID (text)
  - Cron expression (text input + human-readable preview, e.g. "Every 6 hours")
  - Timezone (dropdown of IANA timezones)
  - Lock service (dropdown) + TTL (text)
  - Trigger workflow (dropdown) + input mapping (key-value)
- Cron preview helper: parse cron string and display next 3 run times

**Files created:** `SchedulesView.tsx` (rewrite stub), `ScheduleFormPanel.tsx`, `utils/cron.ts` (human-readable cron descriptions)

### 2.3 Connections View (WebSocket/SSE)

**Config shape:** `{ connections: [{ name, type, path, middleware, channel, limits, on_connect, on_message, on_disconnect, ping }] }`

- Table: Name, type (ws/sse), path, lifecycle workflows
- Form panel:
  - Name (text)
  - Type (dropdown: websocket/sse)
  - Path (text)
  - Channel pattern (expression text)
  - Middleware (tag input)
  - Connection limits (number)
  - Lifecycle workflows: on_connect, on_message, on_disconnect (workflow dropdowns)
  - Ping/heartbeat settings (for websocket)

**Files created:** `ConnectionsView.tsx` (rewrite stub), `ConnectionFormPanel.tsx`

### 2.4 Wasm Runtimes View

**Config shape:** `{ wasm: [{ name, module, tick_rate, encoding, services, connections, outbound, config }] }`

- Table: Module name, tick rate, encoding, status
- Form panel:
  - Name (text)
  - Module file path (text)
  - Tick rate (number, Hz)
  - Encoding (dropdown: json/msgpack)
  - Service access (multi-select from services)
  - Connection access (multi-select from connections)
  - Outbound whitelist: HTTP hosts, WS hosts (string array)
  - Custom config (JSON Monaco editor)

**Files created:** `WasmView.tsx` (rewrite stub), `WasmFormPanel.tsx`

### 2.5 Migrations View

**Note:** Depends on backend support for listing/running migrations via API, which doesn't exist yet. Implement as read-only initially.

- List migration files from `files` (if the backend exposes them)
- Show filename and status (applied/pending) if backend provides migration state
- "Create New Migration" prompt for name, create file via PUT
- Run Up / Run Down buttons (disabled until backend API exists)

**Files created:** `MigrationsView.tsx` (rewrite stub)

---

## Phase 3: Canvas & Interaction Enhancements

### 3.1 Context Menus

- **Right-click node:** Duplicate, Delete, View Schema, Copy
- **Right-click edge:** Add/Remove Retry, Delete, Insert Node (splits edge)
- **Right-click canvas:** Add Node (opens search), Paste, Auto-Layout

**Files created:** `CanvasContextMenu.tsx`
**Files modified:** `WorkflowCanvas.tsx`

### 3.2 Quick-Add Search Dialog

- Double-click canvas to open search dialog at click position
- Type to filter node types (same data as palette)
- Enter/click to add node at that position
- Keyboard-driven: arrow keys to navigate, Enter to select

**Files created:** `QuickAddDialog.tsx`
**Files modified:** `WorkflowCanvas.tsx`

### 3.3 Multi-Select

- Shift+click or drag-select to select multiple nodes
- Move selected group together
- Delete selected group
- Copy/paste selected group (already partially supported in clipboard store)
- Store: `selectedNodeIds: Set<string>` instead of single `selectedNodeId`

**Files modified:** `editor.ts`, `WorkflowCanvas.tsx`, `clipboard.ts`

### 3.4 Edge Animation During Trace

- When trace events flow, animate edges (CSS pulse/dash animation)
- Color edges based on execution state (green flowing for success, red for error)
- Show data flowing direction

**Files modified:** `NodaEdge.tsx`, add CSS animations

### 3.5 Node ID Collision Fix

- Reset `nextNodeCounter` when switching workflows
- Or use a smarter ID generator that checks existing IDs in the workflow

**Files modified:** `WorkflowCanvas.tsx`

---

## Phase 4: Expression Editor Intelligence

### 4.1 Backend: Expression Validation Endpoint

Add `POST /_noda/expressions/validate` to the Go server that compiles an expression and returns errors.

**Files modified:** `internal/server/editor.go`, `internal/server/routes.go`

### 4.2 Backend: Expression Context Endpoint

Add `GET /_noda/expressions/context?workflow=X&node=Y` that returns available variables for a node based on its position in the graph (upstream node outputs, trigger input, loop variables).

**Files modified:** `internal/server/editor.go`, `internal/server/routes.go`

### 4.3 Monaco Autocomplete Provider

- Register a completion provider for expression fields
- Fetch context from the new endpoint
- Suggest: `input.*`, upstream node outputs (`{{ node-id.field }}`), built-in functions (`len()`, `lower()`, `$uuid()`)
- Show inline documentation on hover

**Files created:** `editor/src/utils/expressionLanguage.ts`
**Files modified:** `ExpressionWidget.tsx`

### 4.4 Real-Time Expression Validation

- Debounced call to `/expressions/validate` as user types
- Red underline on syntax errors in Monaco
- Error message in tooltip

**Files modified:** `ExpressionWidget.tsx`

---

## Phase 5: File Sync & Conflict Detection

### 5.1 Backend: File Change WebSocket Events

Extend the trace WebSocket (or add a separate channel) to emit `file:changed` events when files change on disk outside the editor.

**Files modified:** `internal/server/editor.go`, `internal/trace/websocket.go`

### 5.2 Editor: File Change Handling

- Listen for `file:changed` events
- If the changed file is currently open and has no unsaved edits: auto-reload
- If the changed file has unsaved edits: show conflict dialog (keep mine / load theirs / diff)

**Files modified:** `traceClient.ts`, `editor.ts`
**Files created:** `ConflictDialog.tsx`

### 5.3 Auto-Save with Dirty Tracking

- Current: 300ms debounced save (already works)
- Add: visual dirty indicator per file in sidebar (dot or asterisk)
- Add: "Unsaved changes" warning before navigating away (beforeunload)

**Files modified:** `WorkflowList.tsx`, `App.tsx`

---

## Phase 6: Advanced Canvas Features

### 6.1 Multi-Tab Workflows

- Open multiple workflows in tabs above the canvas
- Tab shows workflow name + dirty dot
- Click `workflow.run` node to open sub-workflow in new tab
- Ctrl+W closes current tab
- Tab bar component with overflow scroll

**Files created:** `WorkflowTabs.tsx`
**Files modified:** `App.tsx`, `editor.ts` (track open tabs)

### 6.2 Sub-Workflow Tracing

- When viewing a trace that includes `workflow.run`, show a "View Sub-Trace" button on the node
- Clicking opens the sub-workflow with its trace events highlighted
- Breadcrumb navigation: Parent Workflow > Sub-Workflow

**Files created:** `TraceBreadcrumb.tsx`
**Files modified:** `NodaNode.tsx`, `TracePanel.tsx`

### 6.3 Trace Replay

- Replay controls: Play, Pause, Step, Speed (1x/2x/5x)
- Nodes highlight in sequence with timing from trace events
- Edges animate when data flows through them
- Timeline slider to scrub through execution

**Files created:** `ReplayControls.tsx`, `stores/replay.ts`
**Files modified:** `WorkflowCanvas.tsx`

### 6.4 "Try It" Panel for Routes

- In Routes View: button to send a test HTTP request
- Compose: method, path, headers, body
- Send via `fetch()` to the running Noda instance
- Show response + link to the execution trace

**Files created:** `TryItPanel.tsx`
**Files modified:** `RoutesView.tsx`

---

## Phase 7: Polish & UX

### 7.1 Dark Mode

- Theme toggle in toolbar
- CSS variables or Tailwind `dark:` classes
- React Flow, Monaco, and all components adapt
- Persist preference in localStorage
- Respect `prefers-color-scheme` on first load

**Files created:** `hooks/useTheme.ts`
**Files modified:** `tailwind.config`, all components with `dark:` variants

### 7.2 Graph Validation Visualizations

- Unreachable nodes: dimmed/grayed opacity
- Cycle detection: highlight cycle edges in red
- Missing service refs: warning badge on node
- Invalid expression refs: warning badge on node

**Files created:** `utils/graphValidation.ts`
**Files modified:** `NodaNode.tsx`, `NodaEdge.tsx`

### 7.3 Resizable Panels

- Drag handles between panels (sidebar, config panel, trace panel)
- Persist sizes in localStorage
- Collapse/expand buttons

**Files created:** `ResizablePanel.tsx` (or use a library)
**Files modified:** `App.tsx`

### 7.4 Command Palette (Ctrl+K)

- Global search across: nodes, workflows, routes, services, schemas
- Actions: open workflow, add node, navigate to view, run auto-layout
- Keyboard-driven with fuzzy matching

**Files created:** `CommandPalette.tsx`
**Files modified:** `useKeyboardShortcuts.ts`

### 7.5 Project Scaffolding Wizard

- First-run detection (no workflows exist)
- Steps: project name, select services, first route, generate files
- Produces valid config files

**Files created:** `ProjectWizard.tsx`

---

## Suggested Implementation Order

| Order | Item | Effort | Impact |
|-------|------|--------|--------|
| 1 | 1.1 Routes CRUD | Medium | High — most-used config editing |
| 2 | 1.2 Schemas Edit & Save | Small | Medium — unblocks route schema refs |
| 3 | 2.1 Workers View | Medium | High — core missing view |
| 4 | 2.2 Schedules View | Medium | High — core missing view |
| 5 | 2.3 Connections View | Medium | High — core missing view |
| 6 | 2.4 Wasm View | Medium | Medium — niche use case |
| 7 | 1.3 Tests Editing | Medium | Medium — CLI still needed for execution |
| 8 | 3.1 Context Menus | Small | High — major UX improvement |
| 9 | 3.2 Quick-Add Dialog | Small | High — faster workflow building |
| 10 | 3.5 Node ID Fix | Tiny | Medium — prevents bugs |
| 11 | 3.3 Multi-Select | Medium | Medium — power user feature |
| 12 | 5.3 Auto-Save Indicators | Small | Medium — prevents data loss confusion |
| 13 | 4.1-4.4 Expression Intelligence | Large | High — key differentiator |
| 14 | 3.4 Edge Animation | Small | Medium — visual polish |
| 15 | 6.1 Multi-Tab | Large | High — essential for sub-workflows |
| 16 | 6.4 Try It Panel | Medium | High — developer workflow |
| 17 | 5.1-5.2 File Sync | Medium | Medium — multi-tool workflows |
| 18 | 7.4 Command Palette | Medium | High — power user productivity |
| 19 | 7.1 Dark Mode | Large | Medium — cosmetic |
| 20 | 7.2 Graph Validation Viz | Medium | Medium — correctness aid |
| 21 | 7.3 Resizable Panels | Small | Small — comfort |
| 22 | 6.2-6.3 Sub-Workflow Tracing & Replay | Large | Medium — advanced debugging |
| 23 | 2.5 Migrations View | Small | Small — depends on backend |
| 24 | 7.5 Project Wizard | Medium | Medium — onboarding |
