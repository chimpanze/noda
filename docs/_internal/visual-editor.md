# Noda — Visual Editor

**Version**: 0.4.0
**Status**: Complete

The visual editor is Noda's primary authoring tool. It is a web application that reads and writes Noda's JSON config files, providing a graphical interface for building workflows, managing routes, configuring services, and debugging executions in real time.

---

## 1. Core Principles

**The editor is not the runtime.** The editor produces JSON config files. Noda consumes them. The editor can be closed, replaced, or never used — the config files are the source of truth. The editor is a development tool, not a production dependency.

**Everything the editor does maps to a JSON file.** There is no editor-only state. Every action in the editor (adding a node, drawing an edge, configuring a route) results in a change to a JSON config file. If you edit the JSON by hand, the editor reflects it. If the editor writes JSON, it's valid Noda config.

**The editor runs inside `noda dev`.** When a developer runs `noda dev`, Noda starts all runtimes with hot reload AND serves the editor at `http://localhost:<port>/editor`. Zero setup — the editor is part of the dev experience. It communicates with the running Noda instance for live tracing, validation, and file sync.

**Adapt, don't build from scratch.** The workflow canvas is built on **React Flow** (`@xyflow/react`) — a mature, battle-tested library for node-based editors. Node configuration forms use **shadcn/ui** components. Auto-layout uses **ELKjs**. The editor is a composition of proven libraries, not a custom rendering engine.

---

## 2. Technology Stack

| Concern | Technology | Reason |
|---|---|---|
| Framework | React + TypeScript | React Flow requires React, large ecosystem |
| Canvas | React Flow (`@xyflow/react`) | Node-based UI, zoom/pan, custom nodes/edges, minimap |
| UI Components | shadcn/ui + Tailwind CSS | Consistent, customizable, accessible |
| State Management | Zustand | Simple, performant, recommended by React Flow |
| Auto-Layout | ELKjs | Graph layout engine, handles complex DAGs |
| Form Generation | React JSON Schema Form (`@rjsf/core`) | Generate node config forms from JSON Schema |
| Code Editor | Monaco Editor | Expression editing with autocomplete |
| WebSocket | Native WebSocket API | Live trace streaming |
| File Sync | HTTP API to Noda dev server | Read/write config files |
| Icons | Lucide React | Consistent icon set |

---

## 3. Architecture

The editor runs as a single-page application served by Noda's dev server.

```
┌──────────────────────────────────────────────┐
│                   Browser                     │
│                                               │
│  ┌─────────────┐  ┌──────────────────────┐   │
│  │  Navigation  │  │    Active View       │   │
│  │  - Workflows │  │  (Canvas / Config)   │   │
│  │  - Routes    │  │                      │   │
│  │  - Workers   │  │                      │   │
│  │  - Schedules │  │                      │   │
│  │  - Schemas   │  │                      │   │
│  │  - Services  │  │                      │   │
│  │  - Wasm      │  │                      │   │
│  │  - Tests     │  │                      │   │
│  └─────────────┘  └──────────────────────┘   │
│                                               │
│  ┌──────────────────────────────────────────┐ │
│  │        Live Trace / Debug Panel          │ │
│  └──────────────────────────────────────────┘ │
└───────────────────────┬──────────────────────┘
                        │ HTTP + WebSocket
                        │
┌───────────────────────▼──────────────────────┐
│              Noda Dev Server                  │
│                                               │
│  Editor API ──→ Config Files (read/write)    │
│  Trace WS   ──→ Workflow Engine (live trace) │
│  Validate   ──→ Config Validator             │
│  Node Meta  ──→ Plugin Registry (schemas)    │
└──────────────────────────────────────────────┘
```

### 3.1 Editor API

The Noda dev server exposes an HTTP API for the editor:

| Endpoint | Method | Purpose |
|---|---|---|
| `/api/editor/files` | GET | List all config files by type |
| `/api/editor/files/:path` | GET | Read a config file |
| `/api/editor/files/:path` | PUT | Write a config file |
| `/api/editor/files/:path` | DELETE | Delete a config file |
| `/api/editor/validate` | POST | Validate a config file against its schema |
| `/api/editor/validate/all` | POST | Validate all config files, return all errors |
| `/api/editor/nodes` | GET | List all registered node types with their descriptors |
| `/api/editor/nodes/:type/schema` | GET | Get a node type's config JSON Schema |
| `/api/editor/nodes/:type/outputs` | POST | Compute outputs for a node given its config (calls the factory) |
| `/api/editor/services` | GET | List all configured service instances with their types |
| `/api/editor/plugins` | GET | List all loaded plugins with their prefixes |
| `/api/editor/schemas` | GET | List all shared schema definitions |
| `/api/editor/expressions/validate` | POST | Validate an expression string |
| `/api/editor/expressions/context` | GET | Get available expression context variables for a given workflow/node |

### 3.2 Trace WebSocket

The dev server exposes a WebSocket at `/ws/trace` for live execution tracing. Events:

| Event | Payload |
|---|---|
| `workflow:started` | `{ workflow_id, trace_id, trigger_type, input }` |
| `workflow:completed` | `{ workflow_id, trace_id, status, duration }` |
| `node:entered` | `{ workflow_id, trace_id, node_id, node_type }` |
| `node:completed` | `{ workflow_id, trace_id, node_id, output_name, data, duration }` |
| `node:failed` | `{ workflow_id, trace_id, node_id, error }` |
| `edge:followed` | `{ workflow_id, trace_id, from, output, to }` |
| `retry:attempted` | `{ workflow_id, trace_id, edge, attempt, max_attempts }` |

The editor subscribes to this WebSocket and overlays trace data onto the workflow canvas in real time.

### 3.3 File Watching

When the editor writes a config file, Noda's file watcher detects the change and triggers hot reload. The editor does not need to explicitly tell Noda to reload — the existing `fsnotify` mechanism handles it. If hot reload detects validation errors, they are surfaced through the trace WebSocket as system events.

---

## 4. Views

The editor is organized into views, each managing a specific config concern. The left sidebar provides navigation between views.

### 4.1 Workflows View

The primary view. A list of workflows on the left, the selected workflow's graph canvas on the right.

**Canvas features (via React Flow):**
- Drag-and-drop nodes from a sidebar palette
- Draw edges by dragging from output ports to input ports
- Zoom, pan, minimap, grid snapping
- Multi-select, copy/paste, undo/redo
- Auto-layout via ELKjs (manual trigger or on demand)
- Node grouping (visual only — for organizing complex workflows)

**Custom node rendering:**

Each node type has a custom React Flow node component:
- Node header: icon + type name (e.g., "db.query") + optional alias (`as`)
- Output ports: rendered dynamically from the node's `Outputs()` — one port per output name, color-coded (green for success, red for error, blue for custom)
- Status indicator: shows execution state during live tracing (idle, running, completed, failed)
- Config summary: compact inline preview of key config values (e.g., the SQL query, the condition expression)

**Custom edge rendering:**
- Default edges: smooth bezier curves
- Error edges: red/dashed
- Retry badge: if an edge has retry config, show a small badge with attempt count
- Animation during live tracing: edges pulse when data flows through them

**Node palette:**

A searchable sidebar listing all available node types, grouped by category (Control, Transform, Response, Database, Cache, etc.). Drag a node from the palette onto the canvas to add it.

The palette is populated from the `/api/editor/nodes` endpoint — it always reflects the currently loaded plugins and their nodes.

**Node configuration panel:**

When a node is selected, a right-side panel shows its configuration form. The form is auto-generated from the node's `ConfigSchema()` (JSON Schema), using React JSON Schema Form. This means:
- Every node gets a proper form without custom UI code
- Enum fields render as dropdowns
- Expression fields get a Monaco editor with autocomplete
- Required fields are visually marked
- Validation runs in real time against the schema

The `services` field renders as a set of dropdowns, filtered by the slot's required prefix. Only service instances matching the required prefix appear as options.

### 4.2 Routes View

A table listing all HTTP routes with their method, path, workflow, middleware, and tags. Clicking a route opens its configuration form.

**Route form fields:**
- Method (dropdown)
- Path (text with parameter highlighting for `:param`)
- Middleware (multi-select from available middleware presets + individual middleware)
- Trigger workflow (dropdown of available workflows)
- Input mapping (key-value pairs with expression editor per value)
- Request schemas (params, query, body) with JSON Schema editor or `$ref` picker
- Response schemas with status code mapping
- File stream fields (`files` array)
- Raw body toggle

**Route testing:**

A "Try it" panel lets developers send test requests directly from the editor. It pre-fills the request based on the route's schemas and shows the response with execution trace linked.

### 4.3 Workers View

A table listing all worker definitions. Each row shows the worker ID, stream service, topic, consumer group, concurrency, and linked workflow.

**Worker form fields:**
- Stream service (dropdown)
- Subscribe topic and consumer group
- Concurrency
- Middleware
- Trigger workflow + input mapping
- Dead letter configuration

### 4.4 Schedules View

A table listing all scheduled jobs. Each row shows the schedule ID, cron expression (with human-readable description), timezone, lock config, and linked workflow.

**Schedule form fields:**
- Cron expression (with a visual cron builder)
- Timezone (dropdown)
- Lock service and TTL
- Trigger workflow + input mapping

### 4.5 Connections View

Configuration for WebSocket and SSE endpoints. Shows the sync pubsub config and a list of endpoints.

**Endpoint form fields:**
- Type (websocket/sse)
- Path
- Middleware
- Channel pattern
- Connection limits
- Lifecycle workflow references (on_connect, on_message, on_disconnect)
- Ping/heartbeat settings

### 4.6 Services View

A list of all configured service instances grouped by plugin type. Each service shows its instance name, plugin, health status (live from the running dev server), and config.

**Service form fields:**
- Instance name
- Plugin type (dropdown)
- Plugin-specific config (connection URL, credentials, backend settings)

Adding a new service: select plugin type → fill config → the service is immediately available for node configuration in workflows.

### 4.7 Schemas View

A list of all shared JSON Schema definitions in the `schemas/` directory. Each schema can be edited with a JSON editor or a visual schema builder.

Schemas are referenced via `$ref` from routes and `transform.validate` nodes. The editor resolves and displays these references inline where used.

### 4.8 Wasm Runtimes View

A list of Wasm module configurations. Each shows the module name, tick rate, encoding, services, connections, outbound whitelist, and status (running/stopped in dev mode).

**Runtime form fields:**
- Module file path
- Tick rate
- Encoding (json/msgpack)
- Service access list (multi-select from available services)
- Connection endpoint access list
- Outbound whitelist (HTTP hosts, WebSocket hosts)
- Custom config (JSON editor)

### 4.9 Tests View

A list of workflow test files. Each test file shows its test cases with pass/fail status from the last run.

**Test form:**
- Select workflow
- Define test cases: input data, node mocks (output or error), expected outcome
- Run tests directly from the editor
- View execution trace for failed tests
- Diff view for expected vs actual output

### 4.10 Migrations View

A list of migration files with their status (applied/pending). Buttons to run up/down/create from the editor.

---

## 5. Live Execution Tracing

The defining feature of the editor. When `noda dev` is running, the editor shows workflow executions in real time on the canvas.

### 5.1 How It Works

1. Developer fires an HTTP request (from the Route "Try it" panel, curl, or an actual client)
2. Noda starts a workflow execution and streams trace events over the WebSocket
3. The editor receives events and overlays them on the currently open workflow canvas:
   - **Node highlighting:** Nodes light up as they execute — blue while running, green on success, red on failure
   - **Edge animation:** Edges animate with a flowing pulse when data passes through them
   - **Data inspection:** Click a completed node to see its input and output data in the debug panel
   - **Error display:** Failed nodes show the error details inline
   - **Timing:** Each node shows execution duration
   - **Retry visualization:** Retry edges show attempt count and backoff timing

### 5.2 Trace History

The debug panel at the bottom of the screen shows a list of recent executions. Each execution is a row showing:
- Trace ID
- Trigger type and source (which route, which event, which schedule)
- Workflow ID
- Status (running, success, error)
- Duration
- Timestamp

Clicking an execution replays the trace on the canvas — nodes light up in sequence, showing the execution path. This works even after the execution completes — the trace data is stored in memory during the dev session.

### 5.3 Sub-Workflow Tracing

When a workflow calls a sub-workflow via `workflow.run`, the trace includes events from the sub-workflow. The editor can:
- Show the sub-workflow inline (expand the `workflow.run` node to show the sub-workflow's graph)
- Navigate to the sub-workflow (click to open it in a new canvas tab, with the trace continuing)
- Collapse back to the parent (the `workflow.run` node shows a summary: which output fired, duration)

### 5.4 Wasm Module Tracing

Wasm interactions appear in the trace:
- `wasm.send` nodes show the data sent to the module
- `wasm.query` nodes show the query, response, and timing
- If a Wasm module triggers a workflow via `noda_call("", "trigger_workflow", ...)`, that workflow's trace is linked — you can click through from the Wasm-triggered execution to the parent module context

---

## 6. Expression Editor

Expressions are central to Noda. The editor provides a rich editing experience for `{{ }}` expressions.

### 6.1 Monaco Integration

Expression fields in node config forms use Monaco editor (the engine behind VS Code) with:
- Syntax highlighting for Expr language
- Autocomplete for available context variables (`input.*`, `auth.*`, `trigger.*`, node outputs)
- Autocomplete for built-in functions (`len()`, `lower()`, `upper()`, `now()`, `$uuid()`)
- Autocomplete for plugin-registered custom functions
- Real-time validation — red underline if the expression is invalid
- Inline documentation on hover

### 6.2 Context Awareness

The expression editor knows which variables are available based on the node's position in the graph:

- A node that comes after `fetch-user` can autocomplete `{{ fetch-user.name }}`
- A node inside a `control.loop` can autocomplete `{{ $item }}` and `{{ $index }}`
- A node in a sub-workflow can autocomplete `{{ input.* }}` based on the parent's input mapping

This context is computed by the editor from the graph structure — it doesn't need to run the workflow.

### 6.3 Expression Preview

In the debug panel, when a trace is active, expression fields show their resolved values inline. You see both the expression (`{{ input.name }}`) and what it resolved to (`"Alice"`) for the last execution. This helps debug expression errors.

---

## 7. Validation

The editor validates continuously, providing immediate feedback without running the workflow.

### 7.1 Config Schema Validation

Every node's config is validated against its JSON Schema in real time as the user types. Errors appear inline in the form.

### 7.2 Graph Validation

The editor validates the graph structure:
- Every edge references valid output names
- No cycles (except retry edges, which are on error edges and handled by the engine)
- All required service slots are filled
- All service references point to existing service instances
- All `workflow.output` nodes in sub-workflows have unique names
- All `workflow.output` nodes are on mutually exclusive branches
- All `workflow.run` references point to existing workflows
- Expression references only existing node outputs (based on graph position)

Graph validation errors are shown as red badges on affected nodes and edges, with explanations in a validation panel.

### 7.3 Cross-File Validation

The editor validates references across files:
- Routes reference existing workflows
- Workers reference existing stream services and workflows
- Schedules reference existing cache services and workflows
- `$ref` schema references resolve to existing schema files
- Middleware presets reference valid middleware names

The "Validate All" button (or automatic background validation) runs the full cross-file validation and shows all errors in a unified panel.

---

## 8. Workflow Canvas Details

### 8.1 Node Types and Visual Rendering

Each node category has a distinct visual style:

| Category | Color | Icon style |
|---|---|---|
| Control (if, switch, loop) | Purple | Diamond / branch icon |
| Workflow (run, output) | Blue | Sub-graph icon |
| Transform (set, map, filter, merge) | Yellow | Transform / arrow icon |
| Response (json, redirect, error) | Green | Return / send icon |
| Utility (log, uuid, delay, timestamp) | Gray | Tool icon |
| Database (query, exec, create, update, delete) | Orange | Database icon |
| Cache (get, set, del, exists) | Cyan | Lightning icon |
| Storage (read, write, delete, list) | Teal | File icon |
| Image (resize, crop, watermark, convert, thumbnail) | Pink | Image icon |
| HTTP (request, get, post) | Indigo | Globe icon |
| Email (send) | Red | Mail icon |
| Event (emit) | Amber | Broadcast icon |
| WebSocket/SSE (send) | Violet | Signal icon |
| Upload (handle) | Brown | Upload icon |
| Wasm (send, query) | Emerald | Chip icon |

### 8.2 Output Port Rendering

Output ports on the right side of a node. Each port is labeled and color-coded:

- `success` — green
- `error` — red
- `then` / `else` — blue / orange
- `done` — green
- Custom outputs (switch cases, workflow.run outputs) — distinct colors per port
- `default` — gray

### 8.3 Edge Types

| Edge type | Visual | When |
|---|---|---|
| Normal (success) | Solid, dark | Default edge |
| Error | Dashed, red | `output: "error"` |
| Conditional | Solid, colored | From `then`/`else`/case ports |
| Retry | Error edge + retry badge | Has `retry` config |

### 8.4 Canvas Actions

- **Double-click canvas** → open quick-add menu (search and add a node)
- **Double-click node** → open node config panel
- **Right-click node** → context menu (duplicate, delete, add to sub-workflow, view schema)
- **Right-click edge** → context menu (add retry, delete, add node between)
- **Ctrl+Z / Ctrl+Y** → undo/redo
- **Ctrl+C / Ctrl+V** → copy/paste nodes (with edge reconnection)
- **Ctrl+A** → select all
- **Delete** → remove selected nodes/edges
- **Ctrl+S** → save (write to file)
- **Ctrl+Shift+F** → auto-layout

---

## 9. Keyboard Shortcuts and Navigation

| Shortcut | Action |
|---|---|
| `Ctrl+K` | Command palette — search everything (nodes, workflows, routes, services) |
| `Ctrl+S` | Save current file |
| `Ctrl+Z` / `Ctrl+Y` | Undo / redo |
| `Ctrl+Shift+F` | Auto-layout current workflow |
| `Ctrl+Enter` | Run test (in test view) / Send request (in route "Try it" panel) |
| `Ctrl+P` | Quick file switcher |
| `Tab` | Cycle between canvas, config panel, and debug panel |
| `Escape` | Close panel / deselect |
| `F5` | Force reload config from disk |

---

## 10. File Sync Protocol

The editor and Noda dev server maintain file consistency:

### 10.1 Editor → Disk

When the user makes a change in the editor (add a node, change config, draw an edge), the editor:
1. Updates its in-memory state immediately (optimistic UI)
2. Serializes the changed file to JSON
3. Sends a PUT to `/api/editor/files/:path`
4. Noda writes the file to disk
5. File watcher triggers hot reload
6. If hot reload finds validation errors, they arrive via the trace WebSocket

### 10.2 Disk → Editor

When a file changes on disk (from git pull, manual edit, or another tool):
1. Noda's file watcher detects the change
2. Hot reload processes the change
3. Noda sends a `file:changed` event over the trace WebSocket with the file path
4. The editor re-reads the file via GET `/api/editor/files/:path`
5. The editor updates its in-memory state

If the editor has unsaved changes to the same file (conflict), it shows a diff dialog: "File changed on disk. Keep your version, load disk version, or merge?"

### 10.3 JSON Formatting

The editor writes JSON with consistent formatting:
- 2-space indentation
- Keys sorted alphabetically within objects (for git-diff readability)
- No trailing commas
- UTF-8 encoding

---

## 11. Project Scaffolding

When a developer runs `noda init [name]`, the CLI scaffolds a project. The editor enhances this with a first-run wizard:

1. **Project name and description**
2. **Services** — which services do you need? (PostgreSQL, Redis, S3, email). For each, provide connection details or use defaults for Docker Compose.
3. **Authentication** — JWT config (secret, algorithm, token lookup)
4. **First route** — create a simple route and workflow to get started
5. **Docker Compose** — generate `docker-compose.yml` based on selected services

The wizard produces valid config files. The developer immediately has a working project with `noda dev`.

---

## 12. Sub-Workflow Editing

Sub-workflows are regular workflows. The editor provides navigation between them:

- **Inline expansion:** Double-click a `workflow.run` node to expand it inline on the canvas, showing the sub-workflow's graph nested inside. Useful for understanding the full flow.
- **Navigate into:** Right-click → "Open Sub-Workflow" to navigate to the sub-workflow as a separate canvas tab. Breadcrumbs show the navigation path.
- **Create from selection:** Select a group of nodes → right-click → "Extract to Sub-Workflow." The editor creates a new workflow file, replaces the selection with a `workflow.run` node, and wires the input/output mapping automatically.
- **Output port sync:** When a sub-workflow's `workflow.output` nodes change, the parent's `workflow.run` node updates its output ports automatically.

---

## 13. Multi-Tab Workflow

The editor supports opening multiple workflows in tabs (like browser tabs or VS Code tabs):

- Each tab is a separate workflow canvas
- Tabs show the workflow name and a dirty indicator (unsaved changes)
- Click a `workflow.run` node → opens the sub-workflow in a new tab
- Click a workflow in the sidebar → opens or focuses the existing tab
- Ctrl+W closes the current tab

---

## 14. Dark Mode

The editor supports light and dark themes. Theme preference is stored in the browser's localStorage. All node colors, edge colors, and UI components adapt to the theme.

---

## 15. Responsive Layout

The editor has three main panels that can be resized:

- **Left sidebar** — navigation + file list (collapsible)
- **Center** — canvas or form view (always visible)
- **Right panel** — node config (collapsible, slides in when a node is selected)
- **Bottom panel** — debug/trace panel (collapsible, resizable height)

Panels remember their size between sessions (stored in localStorage).
