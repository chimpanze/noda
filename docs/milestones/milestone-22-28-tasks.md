# Milestone 22: Visual Editor — Foundation — Task Breakdown

**Depends on:** Milestone 20 (dev mode with hot reload and trace WebSocket)
**Result:** React app served by `noda dev`, workflow canvas renders nodes and edges from config, file sync reads/writes through editor API.

---

## Task 22.1: React Project Setup

**Description:** Initialize the editor frontend project.

**Subtasks:**

- [ ] Create `editor/` directory with Vite + React + TypeScript setup
- [ ] Install dependencies: `@xyflow/react`, `zustand`, `tailwindcss`, `lucide-react`, `shadcn/ui` base components
- [ ] Configure Tailwind CSS
- [ ] Configure Vite proxy to forward `/api/editor/*` and `/ws/trace` to Noda dev server
- [ ] Build script: `npm run build` produces static assets for embedding
- [ ] Dev script: `npm run dev` for standalone frontend development with hot module reload

**Tests:**
- [ ] `npm run build` succeeds
- [ ] Dev server starts and renders the app shell

**Acceptance criteria:** Editor frontend project scaffolded and building.

---

## Task 22.2: Editor API Endpoints

**Description:** Implement the editor HTTP API on the Noda dev server.

**Subtasks:**

- [ ] Create `internal/server/editor.go`
- [ ] Implement endpoints (only active in dev mode):
  - `GET /api/editor/files` — list all config files by type
  - `GET /api/editor/files/:path` — read a config file (raw JSON)
  - `PUT /api/editor/files/:path` — write a config file (triggers hot reload)
  - `DELETE /api/editor/files/:path` — delete a config file
  - `POST /api/editor/validate` — validate a single config file
  - `POST /api/editor/validate/all` — validate all files, return all errors
  - `GET /api/editor/nodes` — list all registered node types with descriptors
  - `GET /api/editor/nodes/:type/schema` — get a node's config JSON Schema
  - `POST /api/editor/nodes/:type/outputs` — compute outputs given a config (calls factory)
  - `GET /api/editor/services` — list configured service instances with types and health
  - `GET /api/editor/plugins` — list loaded plugins with prefixes
  - `GET /api/editor/schemas` — list shared schema definitions
- [ ] Static file serving: serve editor build output at `/editor`
- [ ] Embed editor assets in the binary (using Go's `embed` package) or serve from filesystem in dev

**Tests:**
- [ ] Each endpoint returns correct data shape
- [ ] File write triggers hot reload
- [ ] Node list matches registered plugins
- [ ] Validation endpoint returns errors for invalid config

**Acceptance criteria:** Editor API fully functional for all editor operations.

---

## Task 22.3: App Shell and Navigation

**Description:** Editor layout with sidebar navigation.

**Subtasks:**

- [ ] Create app shell component: left sidebar + main content area + bottom panel (collapsed by default)
- [ ] Sidebar navigation items: Workflows, Routes, Workers, Schedules, Connections, Services, Schemas, Wasm, Tests, Migrations
- [ ] Each nav item fetches its file list from the editor API
- [ ] Active view state managed in Zustand store
- [ ] Resizable panels (sidebar, bottom panel) with drag handles
- [ ] Panel sizes persisted in localStorage

**Tests:**
- [ ] Navigation renders all items
- [ ] Clicking nav item switches the active view
- [ ] Panel resizing works

**Acceptance criteria:** App shell with working navigation.

---

## Task 22.4: Zustand State Store

**Description:** Central state management for the editor.

**Subtasks:**

- [ ] Create store with slices:
  - `files` — loaded file contents, dirty state per file
  - `activeView` — which view is shown (workflows, routes, etc.)
  - `activeWorkflow` — currently open workflow ID
  - `selectedNode` — currently selected node ID (null if none)
  - `selectedEdge` — currently selected edge ID
  - `traces` — recent execution traces
  - `validation` — current validation errors
- [ ] Actions: loadFile, saveFile, selectNode, deselectAll, addTrace, setValidation
- [ ] Middleware: undo/redo support (for graph editing in M24)

**Tests:**
- [ ] State updates correctly for each action
- [ ] Dirty state tracked per file

**Acceptance criteria:** State store manages all editor state.

---

## Task 22.5: Workflow List View

**Description:** List all workflows with click-to-open.

**Subtasks:**

- [ ] Fetch workflow files from editor API
- [ ] Display as a list: workflow ID, name, node count, edge count
- [ ] Click workflow → loads into canvas, sets as active workflow
- [ ] Search/filter by name

**Tests:**
- [ ] Workflows listed from API
- [ ] Click opens workflow on canvas

**Acceptance criteria:** Workflows are browsable and selectable.

---

## Task 22.6: Workflow Canvas — Basic Rendering

**Description:** Render workflow nodes and edges on the React Flow canvas.

**Subtasks:**

- [ ] Integrate React Flow with the Zustand store
- [ ] Map Noda workflow nodes to React Flow nodes:
  - Position from node's `position` field
  - Node type determines the custom component (see Task 22.7)
  - Node ID matches Noda's node ID
- [ ] Map Noda edges to React Flow edges:
  - Source: `from` node ID, source handle: `output` name
  - Target: `to` node ID
  - Edge type: normal or error (based on `output` value)
- [ ] Canvas features: zoom, pan, minimap, background grid
- [ ] Load workflow JSON → render on canvas

**Tests:**
- [ ] Workflow with 5 nodes renders correctly
- [ ] Edges connect correct ports
- [ ] Zoom and pan work
- [ ] Minimap shows overview

**Acceptance criteria:** Workflows render visually on the canvas.

---

## Task 22.7: Custom Node Components

**Description:** Render each node type with appropriate visual style.

**Subtasks:**

- [ ] Create base `NodaNode` component with:
  - Header: icon + node type (e.g., `db.query`) + alias (if `as` is set)
  - Output ports: one handle per output, labeled, color-coded (green=success, red=error, blue=custom)
  - Input port: single input handle on the left
  - Compact config preview: show key config value inline (e.g., SQL query text, condition expression)
- [ ] Color-code by category:
  - Control: purple, Workflow: blue, Transform: yellow, Response: green
  - Database: orange, Cache: cyan, Storage: teal, Image: pink
  - HTTP: indigo, Email: red, Event: amber, WebSocket/SSE: violet
  - Upload: brown, Wasm: emerald, Utility: gray
- [ ] Icon per category using Lucide React
- [ ] Selected state: highlighted border
- [ ] Node selection → update Zustand store → right panel shows config (raw JSON for now)

**Tests:**
- [ ] Each node category renders with correct color and icon
- [ ] Output ports match the node's `Outputs()`
- [ ] Selection state visually distinct
- [ ] Config preview shows key field

**Acceptance criteria:** Nodes are visually distinct and informative.

---

## Task 22.8: Custom Edge Components

**Description:** Render edges with visual distinction by type.

**Subtasks:**

- [ ] Normal edges: solid dark bezier curves
- [ ] Error edges (`output: "error"`): dashed red curves
- [ ] Retry badge: if edge has `retry` config, show a small badge with attempt count
- [ ] Edge labels: show output name on hover or always for non-default outputs
- [ ] Selected edge state: thicker/highlighted

**Tests:**
- [ ] Error edges visually distinct from normal
- [ ] Retry badge shows on retry edges
- [ ] Edge selection works

**Acceptance criteria:** Edges are visually informative.

---

## Task 22.9: File Sync

**Description:** Read from and write to Noda config files through the editor API.

**Subtasks:**

- [ ] On workflow open: `GET /api/editor/files/{path}` → parse → render on canvas
- [ ] On change (future milestones): serialize canvas state to workflow JSON → `PUT /api/editor/files/{path}`
- [ ] File change detection: listen for `file:changed` events on trace WebSocket → re-read file if changed externally
- [ ] Conflict handling: if file changed on disk while editor has unsaved changes → show diff dialog (implement dialog in M27)
- [ ] JSON formatting: 2-space indent, sorted keys

**Tests:**
- [ ] Open workflow reads correct file content
- [ ] External file change detected via WebSocket event
- [ ] JSON formatting consistent

**Acceptance criteria:** Editor reads and writes config files through the API.

---

---

# Milestone 23: Visual Editor — Node Configuration — Task Breakdown

**Depends on:** Milestone 22 (editor foundation)
**Result:** Selecting a node shows an auto-generated config form. Expression fields have Monaco with autocomplete. Service slots show filtered dropdowns.

---

## Task 23.1: React JSON Schema Form Integration

**Description:** Auto-generate config forms from node JSON Schemas.

**Subtasks:**

- [ ] Install and configure `@rjsf/core` with `@rjsf/utils` and a shadcn/ui theme adapter
- [ ] When a node is selected: fetch its config schema from `/api/editor/nodes/:type/schema`
- [ ] Render form in the right panel from the JSON Schema
- [ ] Map schema types to form widgets:
  - `string` → text input
  - `integer`/`number` → number input
  - `boolean` → checkbox
  - `enum` → select dropdown
  - `array` → array field with add/remove
  - `object` → nested fieldset
- [ ] Required fields: visual indicator (asterisk or red border)
- [ ] Default values pre-filled from schema

**Tests:**
- [ ] Form generates from a sample node schema
- [ ] Enum fields render as dropdowns
- [ ] Required fields marked
- [ ] Form data matches expected JSON structure

**Acceptance criteria:** Node config forms auto-generate from JSON Schema.

---

## Task 23.2: Expression Field Widget

**Description:** Custom form widget for expression fields using Monaco Editor.

**Subtasks:**

- [ ] Install `@monaco-editor/react`
- [ ] Create custom RJSF widget for expression-typed fields:
  - Detect expression fields: string fields that can contain `{{ }}`
  - Render Monaco inline editor (single-line for short expressions, multi-line for complex)
  - Syntax highlighting for Expr language inside `{{ }}`
  - Literal text outside `{{ }}` in normal color
- [ ] Real-time validation: parse the expression, show red underline on syntax errors
- [ ] Toggle: switch between Monaco (expression mode) and plain text input

**Tests:**
- [ ] Expression field renders Monaco editor
- [ ] Syntax highlighting works for Expr syntax
- [ ] Invalid expression shows error indicator
- [ ] Plain text values work without `{{ }}`

**Acceptance criteria:** Expression editing with syntax highlighting and validation.

---

## Task 23.3: Expression Autocomplete

**Description:** Context-aware autocomplete for expressions based on graph position.

**Subtasks:**

- [ ] Implement context analysis: given a node's position in the graph, compute available variables:
  - `input.*` — always available, fields from trigger mapping
  - `auth.*` — available when route has auth middleware
  - `trigger.*` — always available
  - Upstream node outputs: for each completed upstream node, its output fields
  - `$item` and `$index` — available inside `control.loop` input mapping
- [ ] Feed available variables to Monaco's autocomplete provider
- [ ] Autocomplete triggers on `.` and on `{{ ` 
- [ ] Show type hints where possible (from upstream node output schemas)
- [ ] Autocomplete for built-in functions: `len()`, `lower()`, `upper()`, `now()`, `$uuid()`
- [ ] Fetch available context from `/api/editor/expressions/context` endpoint

**Tests:**
- [ ] Autocomplete suggests upstream node outputs
- [ ] Autocomplete suggests `input.*` fields
- [ ] Functions appear in autocomplete
- [ ] `$item` available in loop context

**Acceptance criteria:** Expression autocomplete is context-aware and helpful.

---

## Task 23.4: Service Slot Dropdowns

**Description:** Service slot fields show filtered dropdowns of available instances.

**Subtasks:**

- [ ] For each service slot in the node's `ServiceDeps`:
  - Fetch available services from `/api/editor/services`
  - Filter to services matching the slot's required prefix
  - Render as a dropdown with service instance names
- [ ] Show slot name, required prefix, and currently selected instance
- [ ] Required slots: validation error if empty
- [ ] Optional slots: can be left empty

**Tests:**
- [ ] Dropdown shows only services matching the required prefix
- [ ] Selected service persists in config
- [ ] Required empty slot shows validation error

**Acceptance criteria:** Service slots are easy to configure with filtered options.

---

## Task 23.5: Config Save and Preview

**Description:** Save config changes and show resulting JSON.

**Subtasks:**

- [ ] Config preview panel: show the raw JSON that will be written, alongside the form
- [ ] On form change: update the workflow JSON in the Zustand store, mark file as dirty
- [ ] Auto-save with debounce (300ms after last change): write to disk via editor API
- [ ] Save indicator: show "saving..." / "saved" / "error" status
- [ ] Dirty file indicator on workflow tab

**Tests:**
- [ ] Form change → JSON preview updates
- [ ] Auto-save writes to disk after debounce
- [ ] Save error displayed to user

**Acceptance criteria:** Config changes persist automatically with visual feedback.

---

---

# Milestone 24: Visual Editor — Graph Editing — Task Breakdown

**Depends on:** Milestone 22 (foundation), Milestone 23 (node config)
**Result:** Build complete workflows visually — drag nodes from palette, draw edges, delete, copy/paste, undo/redo, auto-layout.

---

## Task 24.1: Node Palette

**Description:** Searchable sidebar of all available node types.

**Subtasks:**

- [ ] Fetch node types from `/api/editor/nodes`
- [ ] Group by category (Control, Transform, Database, etc.)
- [ ] Collapsible category sections
- [ ] Search field: filter by node name or category
- [ ] Each palette item shows: icon, type name, brief description
- [ ] Drag source: palette items are draggable

**Tests:**
- [ ] All registered node types appear in palette
- [ ] Search filters correctly
- [ ] Categories group correctly

**Acceptance criteria:** All node types browsable and draggable from palette.

---

## Task 24.2: Drag-and-Drop Node Addition

**Description:** Drag from palette to canvas to add a node.

**Subtasks:**

- [ ] Implement React Flow `onDrop` handler
- [ ] On drop: create a new node with unique ID, set position from drop coordinates
- [ ] New node gets default config (empty, or schema defaults)
- [ ] Auto-select the new node → config panel opens
- [ ] Update workflow JSON in store

**Tests:**
- [ ] Drag node to canvas → node appears at drop position
- [ ] New node has unique ID
- [ ] Config panel opens for new node

**Acceptance criteria:** Nodes added by drag and drop.

---

## Task 24.3: Edge Drawing

**Description:** Draw edges by dragging from output port to target node.

**Subtasks:**

- [ ] Configure React Flow edge connection handling
- [ ] Connection starts from output handle (labeled port on right side of node)
- [ ] Connection ends on input handle (left side of target node)
- [ ] On connect: create edge in workflow JSON with `from`, `output` (from source handle), `to`
- [ ] Validate: prevent connecting to self, prevent duplicate edges
- [ ] Visual feedback during drag: preview edge follows cursor

**Tests:**
- [ ] Edge drawn from output to input
- [ ] Edge appears in workflow edges array
- [ ] Self-connection prevented
- [ ] Duplicate edge prevented

**Acceptance criteria:** Edges drawn visually and persisted.

---

## Task 24.4: Delete, Copy/Paste, Multi-Select

**Description:** Standard graph editing operations.

**Subtasks:**

- [ ] Delete: select node/edge → press Delete → removed from workflow JSON, connected edges cleaned up
- [ ] Multi-select: Shift+click or drag selection box
- [ ] Copy/paste: Ctrl+C selected nodes → Ctrl+V creates duplicates with new IDs, offset position, reconnect internal edges
- [ ] Batch delete: delete multiple selected nodes/edges at once

**Tests:**
- [ ] Delete node removes it and its edges
- [ ] Delete edge removes only the edge
- [ ] Copy/paste creates duplicates with new IDs
- [ ] Internal edges between copied nodes preserved

**Acceptance criteria:** Standard editing operations work.

---

## Task 24.5: Undo/Redo

**Description:** Undo and redo graph changes.

**Subtasks:**

- [ ] Implement undo/redo using Zustand middleware (or a history stack)
- [ ] Track changes: node add/remove, edge add/remove, node config change, node position change
- [ ] Ctrl+Z → undo last change
- [ ] Ctrl+Y (or Ctrl+Shift+Z) → redo
- [ ] History depth: 50 changes

**Tests:**
- [ ] Add node → undo → node removed
- [ ] Delete node → undo → node restored with edges
- [ ] Config change → undo → config reverted
- [ ] Redo restores undone change

**Acceptance criteria:** Undo/redo works for all graph operations.

---

## Task 24.6: Auto-Layout

**Description:** Automatically arrange nodes using ELKjs.

**Subtasks:**

- [ ] Install `elkjs`
- [ ] Implement layout function: convert React Flow graph to ELK graph, run layout, apply positions back
- [ ] Layout direction: left-to-right (entry nodes on left, terminal on right)
- [ ] Trigger: Ctrl+Shift+F or toolbar button
- [ ] Preserve layout after manual node positioning (don't auto-layout on every change)

**Tests:**
- [ ] Layout produces non-overlapping node positions
- [ ] Left-to-right flow direction
- [ ] Complex graph (10+ nodes) layouts correctly

**Acceptance criteria:** Auto-layout produces clean, readable graphs.

---

## Task 24.7: Quick-Add and Context Menus

**Description:** Quick interaction shortcuts.

**Subtasks:**

- [ ] Double-click canvas → open search dialog, type node name, add to canvas at click position
- [ ] Right-click node → context menu: Duplicate, Delete, Open Sub-Workflow (for workflow.run), View Schema
- [ ] Right-click edge → context menu: Add Retry (opens retry config), Delete, Insert Node (splits edge)
- [ ] Right-click canvas → context menu: Add Node (opens palette search), Paste, Auto-Layout

**Tests:**
- [ ] Quick-add creates node at cursor position
- [ ] Context menu actions execute correctly
- [ ] Insert node on edge splits the edge correctly

**Acceptance criteria:** Quick interactions speed up workflow building.

---

## Task 24.8: Keyboard Shortcuts

**Description:** Implement all keyboard shortcuts from the editor spec.

**Subtasks:**

- [ ] Ctrl+S → save current file
- [ ] Ctrl+Z / Ctrl+Y → undo / redo
- [ ] Ctrl+Shift+F → auto-layout
- [ ] Ctrl+A → select all nodes
- [ ] Delete → remove selected
- [ ] Ctrl+C / Ctrl+V → copy / paste
- [ ] Ctrl+K → command palette (search everything)
- [ ] Escape → deselect / close panel
- [ ] Tab → cycle focus between canvas, config panel, debug panel
- [ ] Shortcut help: `?` shows shortcut list

**Tests:**
- [ ] Each shortcut triggers its action
- [ ] Shortcuts don't conflict with browser defaults

**Acceptance criteria:** All keyboard shortcuts functional.

---

## Task 24.9: End-to-End Test

**Subtasks:**

- [ ] Build a complete workflow from scratch using only the editor: add nodes from palette, draw edges, configure each node, save, run `noda validate` → passes

**Acceptance criteria:** A workflow built entirely in the editor is valid Noda config.

---

---

# Milestone 25: Visual Editor — Live Tracing — Task Breakdown

**Depends on:** Milestone 22 (editor foundation), Milestone 20 (trace WebSocket)
**Result:** Workflow executions visualized in real time on the canvas — nodes light up, edges animate, data is inspectable.

---

## Task 25.1: Trace WebSocket Client

**Description:** Connect to the trace WebSocket and parse events.

**Subtasks:**

- [ ] Create WebSocket client that connects to `/ws/trace`
- [ ] Parse incoming events into typed structures
- [ ] Store traces in Zustand store: list of executions, each with their events
- [ ] Auto-reconnect on disconnect
- [ ] Connection status indicator in the UI

**Tests:**
- [ ] Client connects and receives events
- [ ] Events parsed into store correctly
- [ ] Reconnection works after disconnect

**Acceptance criteria:** Trace events flow into the editor state.

---

## Task 25.2: Node Highlighting

**Description:** Highlight nodes based on execution state.

**Subtasks:**

- [ ] On `node:entered` event: set node to "running" state (blue pulse/glow)
- [ ] On `node:completed` event: set node to "completed" state (green)
- [ ] On `node:failed` event: set node to "failed" state (red)
- [ ] Animation: smooth transition between states
- [ ] Clear highlights when a new execution starts or manually cleared
- [ ] Only highlight nodes in the currently viewed workflow

**Tests:**
- [ ] Running node shows blue indicator
- [ ] Completed node shows green
- [ ] Failed node shows red
- [ ] Highlights clear on new execution

**Acceptance criteria:** Node execution state visible in real time.

---

## Task 25.3: Edge Animation

**Description:** Animate edges when data flows through them.

**Subtasks:**

- [ ] On `edge:followed` event: animate the corresponding edge with a flowing pulse effect
- [ ] Pulse travels from source to target over ~500ms
- [ ] Different colors: green pulse for success, red for error
- [ ] Animation runs once per event (not looping)

**Tests:**
- [ ] Edge animates when data flows
- [ ] Correct edge animated (matching from/to/output)
- [ ] Error edge animation visually distinct

**Acceptance criteria:** Data flow visible through edge animation.

---

## Task 25.4: Data Inspection Panel

**Description:** Click a completed node to see its input/output data.

**Subtasks:**

- [ ] Bottom debug panel with tabs: Execution List, Node Detail
- [ ] Node Detail: when clicking a completed node during/after a trace:
  - Show input data (what the node received)
  - Show output data (what the node produced)
  - Show execution duration
  - Show which output fired
  - For failed nodes: show error details
- [ ] Data displayed as formatted JSON with syntax highlighting
- [ ] Collapsible sections for large data

**Tests:**
- [ ] Completed node click shows correct input/output
- [ ] Failed node shows error details
- [ ] Duration displayed correctly

**Acceptance criteria:** Full data inspection for any executed node.

---

## Task 25.5: Execution History and Replay

**Description:** List recent executions and replay past traces.

**Subtasks:**

- [ ] Execution list in debug panel: trace ID, trigger type, workflow, status, duration, timestamp
- [ ] Click an execution → replay the trace on canvas:
  - Nodes highlight in sequence with timing
  - Edges animate in order
  - Replay speed: 1x, 2x, 5x, step-by-step
- [ ] Replay controls: play, pause, step forward, speed selector
- [ ] Store last N executions in memory (configurable, default 50)

**Tests:**
- [ ] Execution list shows recent executions
- [ ] Replay highlights nodes in correct order
- [ ] Speed control works
- [ ] Step-by-step mode advances one node at a time

**Acceptance criteria:** Past executions can be replayed and inspected.

---

## Task 25.6: Sub-Workflow Tracing

**Description:** Navigate into sub-workflow executions.

**Subtasks:**

- [ ] When a `workflow.run` node executes in a trace, it contains sub-workflow events
- [ ] Click the `workflow.run` node during trace → option to "Open Sub-Workflow Trace"
- [ ] Opens the sub-workflow in a new tab with its trace events highlighted
- [ ] Breadcrumbs: show navigation path (parent workflow → sub-workflow)
- [ ] Back navigation returns to parent with `workflow.run` node still highlighted

**Tests:**
- [ ] Sub-workflow trace navigable from parent
- [ ] Breadcrumbs show correct path
- [ ] Back navigation works

**Acceptance criteria:** Sub-workflow executions fully traceable.

---

---

# Milestone 26: Visual Editor — Remaining Views — Task Breakdown

**Depends on:** Milestone 22 (foundation), Milestone 23 (node config forms)
**Result:** All non-workflow config views functional — routes, workers, schedules, connections, services, schemas, wasm, tests, migrations.

---

## Task 26.1: Routes View

**Subtasks:**

- [ ] Table listing all routes: method, path, workflow, middleware, tags
- [ ] Click route → form for editing route config
- [ ] Form fields: method (dropdown), path (text), middleware (multi-select), trigger mapping (key-value with expression editor), schemas (JSON editor or $ref picker), response schemas
- [ ] "Try it" panel: compose a test request based on route schema, send via fetch, show response with linked trace ID

**Acceptance criteria:** Routes browsable, editable, and testable.

---

## Task 26.2: Workers View

**Subtasks:**

- [ ] Table listing workers: ID, stream service, topic, group, concurrency, workflow
- [ ] Click → form for editing worker config
- [ ] All fields editable including dead letter config

**Acceptance criteria:** Workers browsable and editable.

---

## Task 26.3: Schedules View

**Subtasks:**

- [ ] Table listing schedules: ID, cron expression (with human-readable description), timezone, workflow, lock status
- [ ] Click → form for editing schedule config
- [ ] Visual cron builder: dropdown selectors for minute/hour/day/month/weekday that compose into a cron expression
- [ ] Cron preview: "Every 6 hours" or "Every Monday at 9:00 AM UTC"

**Acceptance criteria:** Schedules browsable and editable with visual cron builder.

---

## Task 26.4: Connections View

**Subtasks:**

- [ ] Show sync pubsub config
- [ ] Table of endpoints: name, type (ws/sse), path, middleware, lifecycle workflows
- [ ] Click → form for editing endpoint config
- [ ] Channel pattern editor with expression support

**Acceptance criteria:** Connection endpoints browsable and editable.

---

## Task 26.5: Services View

**Subtasks:**

- [ ] List all services grouped by plugin type
- [ ] Each service shows: name, plugin, health status (live from API)
- [ ] Click → form for editing service config (connection URL, pool settings)
- [ ] Add service: select plugin type → fill config
- [ ] Remove service: with confirmation (warns about referencing workflows)
- [ ] Health status auto-refresh every 30 seconds

**Acceptance criteria:** Services manageable with live health status.

---

## Task 26.6: Schemas View

**Subtasks:**

- [ ] List all shared schemas
- [ ] Click → JSON editor for the schema definition
- [ ] Show where each schema is referenced (which routes, which validate nodes)
- [ ] Add/remove schemas

**Acceptance criteria:** Schemas editable with reference tracking.

---

## Task 26.7: Wasm Runtimes View

**Subtasks:**

- [ ] List all Wasm runtime configs
- [ ] Each shows: name, module path, tick rate, encoding, status (running/stopped)
- [ ] Click → form: module path, tick rate, encoding dropdown, service multi-select, connection multi-select, outbound whitelist, custom config JSON editor

**Acceptance criteria:** Wasm runtimes configurable from editor.

---

## Task 26.8: Tests View

**Subtasks:**

- [ ] List test suites with pass/fail from last run
- [ ] Click suite → list of test cases with individual pass/fail
- [ ] Test case editor: input JSON, mock definitions, expectations
- [ ] "Run" button: execute tests via `noda test` API, show results
- [ ] Failed test: show execution trace with diff (expected vs actual)

**Acceptance criteria:** Tests runnable and inspectable from editor.

---

## Task 26.9: Migrations View

**Subtasks:**

- [ ] List migrations: filename, status (applied/pending), timestamp
- [ ] Buttons: Run Up, Run Down, Create New
- [ ] Create: prompt for name, generate files, open in editor (or external editor link)

**Acceptance criteria:** Migrations manageable from editor.

---

## Task 26.10: Project Scaffold Wizard

**Subtasks:**

- [ ] First-run detection: if project has no `noda.json`, show wizard
- [ ] Wizard steps: project name → select services → configure connections → create first route → generate docker-compose
- [ ] On complete: write all config files, reload editor

**Acceptance criteria:** New projects can be bootstrapped from the editor.

---

---

# Milestone 27: Validation and Polish — Task Breakdown

**Depends on:** Milestones 22-26 (all editor milestones)
**Result:** Real-time validation feedback throughout the editor, cross-file validation, dark mode, auto-save with conflict detection.

---

## Task 27.1: Graph Validation in Editor

**Subtasks:**

- [ ] Detect cycles — highlight edges forming the cycle in red
- [ ] Missing service references — red badge on affected nodes
- [ ] Invalid edge output names — red badge on affected edges
- [ ] Unreachable nodes (no path from any entry node) — dimmed/grayed
- [ ] `workflow.output` mutual exclusivity violations — error on conflicting nodes
- [ ] Validation runs on every graph change (debounced)

**Acceptance criteria:** Graph errors visible immediately on the canvas.

---

## Task 27.2: Cross-File Validation Panel

**Subtasks:**

- [ ] Unified validation panel: show all errors from all files in one list
- [ ] Errors grouped by file with click-to-navigate (opens the relevant view/form)
- [ ] Background validation: runs automatically, updates on file changes
- [ ] "Validate All" button for manual trigger
- [ ] Error count badge in sidebar navigation items

**Acceptance criteria:** All validation errors visible in one place.

---

## Task 27.3: Expression Validation Inline

**Subtasks:**

- [ ] Red underline on invalid expressions in Monaco editor
- [ ] Error tooltip on hover: show the parse error
- [ ] Warning for references to non-existent node outputs (based on graph position analysis)

**Acceptance criteria:** Expression errors visible while typing.

---

## Task 27.4: Auto-Save and Conflict Detection

**Subtasks:**

- [ ] Auto-save with 300ms debounce after last change
- [ ] Save indicator in toolbar: saving / saved / error
- [ ] File conflict detection: if trace WebSocket reports `file:changed` for a file with unsaved editor changes:
  - Show diff dialog: "File changed on disk. Keep your version / Load disk version / View diff"
  - Three-way comparison if possible
- [ ] Manual save: Ctrl+S forces immediate save

**Acceptance criteria:** Changes persist automatically with conflict protection.

---

## Task 27.5: Dark Mode

**Subtasks:**

- [ ] Implement light and dark themes
- [ ] All node colors, edge colors, and UI components adapt
- [ ] React Flow canvas background adapts
- [ ] Monaco editor theme adapts
- [ ] Theme toggle in toolbar
- [ ] Preference stored in localStorage
- [ ] Respect system preference on first load

**Acceptance criteria:** Full dark mode support.

---

## Task 27.6: Multi-Tab Workflows

**Subtasks:**

- [ ] Open multiple workflows in tabs (like browser tabs)
- [ ] Tab shows workflow name + dirty indicator
- [ ] Click `workflow.run` node → open sub-workflow in new tab
- [ ] Ctrl+W closes current tab
- [ ] Tab state preserved during session

**Acceptance criteria:** Multiple workflows open simultaneously.

---

---

# Milestone 28: Documentation and Examples — Task Breakdown

**Depends on:** All previous milestones
**Result:** Complete user-facing documentation, example projects for all five use cases, plugin and Wasm authoring guides.

---

## Task 28.1: Getting Started Guide

**Subtasks:**

- [ ] Prerequisites: Go, Docker, Docker Compose
- [ ] Installation: `go install` or Docker
- [ ] First project: `noda init`, `docker compose up`, `noda dev`
- [ ] First route and workflow: step-by-step with screenshots
- [ ] First test: write and run a `noda test`
- [ ] Deploy: `noda start` in production

**Acceptance criteria:** New developer goes from zero to running project.

---

## Task 28.2: Concept Guides

**Subtasks:**

- [ ] Workflows: graph model, nodes, edges, execution
- [ ] Nodes: types, config, service deps, outputs
- [ ] Services: plugins, instances, explicit references
- [ ] Triggers: HTTP, events, schedules, WebSocket
- [ ] Expressions: syntax, context variables, functions
- [ ] Testing: test files, mocks, running tests
- [ ] Real-time: WebSocket, SSE, channels, lifecycle
- [ ] Events: streams vs pubsub, workers, dead letter

**Acceptance criteria:** Each concept explained with examples.

---

## Task 28.3: Plugin Authoring Guide

**Subtasks:**

- [ ] Step-by-step: create a plugin, register nodes, manage services
- [ ] Example: build a simple notification plugin
- [ ] Interface reference: Plugin, NodeDescriptor, NodeExecutor, service interfaces
- [ ] Testing: how to test your plugin
- [ ] Publishing: how to share a plugin

**Acceptance criteria:** Developer can build and test a custom plugin.

---

## Task 28.4: Wasm Module Authoring Guide

**Subtasks:**

- [ ] Language setup: Rust with Extism PDK, TinyGo with Extism PDK
- [ ] Module structure: exports, initialize, tick, query, shutdown
- [ ] Host API: noda_call, noda_call_async, all operations
- [ ] Testing: using the Noda test harness
- [ ] Building: compile to .wasm, configure in noda.json
- [ ] Debugging: tick budget, logging, trace integration

**Acceptance criteria:** Developer can build, test, and deploy a Wasm module.

---

## Task 28.5: API Reference

**Subtasks:**

- [ ] All nodes: config fields, outputs, service deps, behavior (from core nodes catalog)
- [ ] All config fields: every field in every config type
- [ ] CLI commands: full reference with all flags
- [ ] Expression functions: all built-in and registerable functions
- [ ] Error codes: all standard error types and their HTTP mappings
- [ ] Auto-generated where possible (from JSON Schemas and node descriptors)

**Acceptance criteria:** Complete reference for all Noda APIs.

---

## Task 28.6: Example Projects

**Subtasks:**

- [ ] Use Case 1: Simple REST API — task management CRUD, complete config files, README
- [ ] Use Case 2: SaaS Backend — multi-tenant, webhooks, workers, email, file uploads
- [ ] Use Case 3: Real-Time Collaboration — WebSocket, presence, live editing
- [ ] Use Case 4: Discord Bot — Wasm module (Rust or TinyGo), gateway connection, REST responses
- [ ] Use Case 5: Multiplayer Game — Wasm module, tick loop, player input, state broadcasting
- [ ] Each example: complete config files, README with setup instructions, `noda test` files, docker-compose

**Tests:**
- [ ] Each example project passes `noda validate`
- [ ] Each example project passes `noda test`
- [ ] Each example runs with `docker compose up && noda dev`

**Acceptance criteria:** Five working example projects covering the full feature set.

---

## Task 28.7: Documentation Website

**Subtasks:**

- [ ] Set up documentation site (Docusaurus, VitePress, or similar)
- [ ] Organize content: Getting Started, Concepts, Guides, Reference, Examples
- [ ] Search functionality
- [ ] Code examples with syntax highlighting
- [ ] Versioned documentation (for future releases)
- [ ] Deploy to GitHub Pages or similar

**Acceptance criteria:** Documentation publicly accessible and searchable.
