# Noda — Future Vision: Client Generation

**Version**: 0.4.0
**Status**: Future Vision

This document explores a future capability: generating typed API clients and ready-to-use UI components directly from Noda's config. Because Noda knows the full picture — routes, schemas, validation rules, auth requirements, WebSocket channels, error formats — it can generate richer clients than a standard OpenAPI code generator.

---

## 1. The Insight

Most API client generators work from an OpenAPI spec and produce typed fetch wrappers. That's useful but limited. Noda has significantly more metadata than a typical OpenAPI spec:

- **Full JSON Schema** on every input field (not just types — min/max, patterns, formats, enums, required fields)
- **Auth requirements** per route (which middleware, what token format)
- **WebSocket channel schemas** with message types and channel patterns
- **SSE event types** with structured payloads
- **Standardized error format** with typed error codes
- **Pagination conventions** (page/limit/total structure)
- **File upload config** (max size, allowed types, field names)

This metadata is enough to generate not just a typed client, but functional UI components with correct validation, error handling, and real-time subscriptions.

---

## 2. Two Layers

### 2.1 SDK Layer — TypeScript API Client

A zero-dependency TypeScript client generated from the OpenAPI spec plus Noda's extended metadata.

```typescript
// Generated from routes + schemas
const noda = new NodaClient({ baseUrl: "https://api.example.com", token: "..." });

// Fully typed — input and output types from JSON Schema
const task = await noda.tasks.create({ title: "Ship feature", description: "..." });
// task: { id: string, title: string, status: "pending", created_at: string }

// Pagination typed
const list = await noda.tasks.list({ page: 1, limit: 20 });
// list: { data: Task[], pagination: { page: number, limit: number, total: number } }

// Standardized errors typed
try {
  await noda.tasks.create({ title: "" });
} catch (e) {
  if (e instanceof NodaValidationError) {
    e.details; // [{ field: "title", message: "minLength: 2" }]
  }
}

// WebSocket typed
const ws = noda.connect("collab", { doc_id: "abc-123" });
ws.on("edit", (data: EditOperation) => { ... });
ws.on("cursor", (data: CursorUpdate) => { ... });
ws.send({ type: "edit", operations: [...] });

// SSE typed
const events = noda.subscribe("live-feed", { topic: "scores" });
events.on("score-update", (data: ScoreEvent) => { ... });
```

The SDK handles auth header injection, error parsing into typed errors, pagination helpers, WebSocket reconnection, and SSE EventSource management. No UI, no framework dependency — pure TypeScript.

### 2.2 Component Layer — Lit Web Components

A set of pre-built UI components generated from the same metadata. Built with **Lit** — lightweight (~5KB), compiles to standard Web Components, works in React, Vue, Svelte, Angular, and plain HTML.

```html
<!-- Works in any framework or plain HTML -->
<noda-task-form
  @submit="${handleSubmit}"
  @error="${handleError}">
</noda-task-form>

<noda-task-list
  page="1"
  limit="20"
  @page-change="${handlePage}">
</noda-task-list>

<noda-ws-status
  endpoint="collab"
  doc-id="abc-123">
</noda-ws-status>
```

---

## 3. Why Lit / Web Components

The decision to use Lit over React or Vue components is deliberate:

**Noda doesn't know the developer's frontend.** A Noda backend might serve a React app, a Vue app, a Svelte app, a mobile app, or a plain HTML page. Framework-specific components would force a choice or require generating multiple versions.

Web Components are a browser standard. A `<noda-task-form>` element works everywhere:
- React: `<noda-task-form onSubmit={handler} />`
- Vue: `<noda-task-form @submit="handler" />`
- Svelte: `<noda-task-form on:submit={handler} />`
- Plain HTML: `<noda-task-form></noda-task-form>` + `element.addEventListener("submit", handler)`

**Lit specifically because:**
- Compiles to standard Web Components — no runtime framework lock-in
- ~5KB production size — negligible overhead
- Reactive properties — data binding via attributes and properties
- CSS custom properties + `::part()` selectors — external styling without forking
- Shadow DOM optional — developers can opt out for easier styling

---

## 4. What Gets Generated

### 4.1 Per-Route Components

For each route, Noda generates components based on the HTTP method and schema:

**POST/PUT routes (forms):**
- `<noda-{resource}-form>` — form with inputs for every field in the request body schema
- Input types derived from JSON Schema: `string` → text input, `string+format:email` → email input, `integer` → number input, `enum` → select dropdown, `boolean` → checkbox, `string+format:date-time` → date picker
- Client-side validation from JSON Schema (required, minLength, pattern, min/max)
- Loading state during submission
- Error display mapped to fields (from standardized error `details` array)
- Emits `submit` event with validated data, `error` event on failure

**GET routes (lists/details):**
- `<noda-{resource}-list>` — table/list with columns from response schema
- Built-in pagination controls if the response includes the pagination structure
- Loading and empty states
- Row click events for navigation
- `<noda-{resource}-detail>` — field display from response schema

**DELETE routes:**
- `<noda-{resource}-delete>` — confirmation dialog + delete action

### 4.2 Real-Time Components

**WebSocket:**
- `<noda-ws-connection endpoint="name">` — manages connection lifecycle, reconnection, auth
- Emits typed events matching the channel's message schemas
- Shows connection status (connecting, connected, disconnected, reconnecting)

**SSE:**
- `<noda-sse-stream endpoint="name">` — manages EventSource, reconnection
- Emits typed events matching the SSE event types

### 4.3 Auth Components

- `<noda-auth-provider>` — wraps the app, manages JWT storage, injects auth into SDK and components
- `<noda-login-form>` — if a login route exists, generates the form
- `<noda-protected>` — conditional rendering based on auth state

### 4.4 Common Components

- `<noda-error-display>` — renders standardized error responses
- `<noda-pagination>` — pagination controls matching Noda's pagination convention
- `<noda-loading>` — loading indicator

---

## 5. Styling

Generated components are intentionally unstyled — they provide structure and behavior, not aesthetics. Developers customize appearance via:

### 5.1 CSS Custom Properties

Every component exposes CSS custom properties for common styling:

```css
noda-task-form {
  --noda-font-family: "Inter", sans-serif;
  --noda-primary-color: #3b82f6;
  --noda-error-color: #ef4444;
  --noda-border-radius: 8px;
  --noda-spacing: 16px;
  --noda-input-border: 1px solid #e5e7eb;
  --noda-input-padding: 8px 12px;
}
```

### 5.2 CSS `::part()` Selectors

Components expose named parts for fine-grained styling:

```css
noda-task-form::part(field) { margin-bottom: 1rem; }
noda-task-form::part(label) { font-weight: 600; }
noda-task-form::part(input) { border: 2px solid #ddd; }
noda-task-form::part(error) { color: red; font-size: 0.875rem; }
noda-task-form::part(submit-button) { background: #3b82f6; color: white; }
```

### 5.3 Slot-Based Composition

Components use slots for replacing entire sections:

```html
<noda-task-form>
  <div slot="header">Create a New Task</div>
  <button slot="submit">Save Task</button>
  <div slot="footer">* Required fields</div>
</noda-task-form>
```

---

## 6. CLI Integration

Client generation integrates into the Noda CLI:

```
noda generate client
├── --output [dir]          — output directory (default: ./client)
├── --sdk-only              — generate only the TypeScript SDK, no components
├── --components-only       — generate only Lit components (requires SDK)
└── --watch                 — regenerate on config changes (dev mode)
```

Output structure:

```
client/
├── sdk/
│   ├── index.ts            — NodaClient class, all methods
│   ├── types.ts            — all TypeScript types from schemas
│   ├── errors.ts           — typed error classes
│   └── websocket.ts        — WebSocket/SSE client helpers
├── components/
│   ├── task-form.ts        — <noda-task-form>
│   ├── task-list.ts        — <noda-task-list>
│   ├── task-detail.ts      — <noda-task-detail>
│   ├── auth-provider.ts    — <noda-auth-provider>
│   ├── ws-connection.ts    — <noda-ws-connection>
│   ├── error-display.ts    — <noda-error-display>
│   ├── pagination.ts       — <noda-pagination>
│   └── index.ts            — registers all custom elements
├── styles/
│   └── tokens.css          — CSS custom property defaults
└── package.json            — Lit + SDK dependencies
```

The `--watch` flag is useful during development: as the developer adds routes and schemas in the visual editor, the client regenerates automatically. The frontend always has up-to-date types and components.

---

## 7. Intended Workflow

The components are scaffolding, not a permanent framework. The expected lifecycle:

1. **Scaffold** — developer runs `noda generate client`, gets working components
2. **Prototype** — drop components into frontend, everything works with correct validation and types
3. **Customize** — restyle with CSS custom properties and `::part()` selectors
4. **Replace** — gradually replace generated components with custom ones, keeping the SDK layer
5. **Maintain** — SDK types stay in sync with the backend as routes change

The SDK layer has lasting value — it stays in use even after all components are replaced. The component layer is a fast start that gets thrown away over time, and that's fine.

---

## 8. Scope and Boundaries

**What this generates:**
- TypeScript SDK with full type safety from JSON Schema
- Lit Web Components for forms, lists, details, real-time connections
- CSS custom properties and `::part()` for styling
- Auth handling (JWT injection, protected routes)
- Error handling (standardized error format → field-level validation display)
- Pagination (matching Noda's convention)
- WebSocket and SSE clients with typed events

**What this does NOT generate:**
- Routing — the developer's frontend handles page routing
- Global state — the components are self-contained, the developer adds state management
- Layout — no grid, no navigation, no page structure
- Custom business logic — components handle CRUD, the developer adds domain logic
- Styling beyond structure — components are unstyled, the developer owns the look

**What this is NOT:**
- Not a low-code frontend builder — it's a code generator producing real, editable code
- Not a component library to npm-install — it generates files into your project that you own and modify
- Not framework-specific — Web Components work everywhere
