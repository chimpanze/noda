# Milestone 8: HTTP Server Runtime — Task Breakdown

**Depends on:** Milestone 4 (workflow engine), Milestone 5 (control nodes), Milestone 6 (transform + utility)
**Result:** Fiber HTTP server runs, routes load from config, middleware chains apply, trigger mapping populates `$.input`, response nodes send HTTP responses, and the workflow continues async after the response. OpenAPI spec generated from routes.

---

## Task 8.1: Fiber Server Initialization

**Description:** Initialize Fiber v3 from root config.

**Subtasks:**

- [x] Create `internal/server/server.go`
- [x] Implement `NewServer(config *ResolvedConfig) (*Server, error)`:
  - Create Fiber app with configured settings (port, read/write timeouts, body limit)
  - Configure Fiber error handler to return standardized error format
  - Store references to workflow engine, service registry, expression engine
- [x] Implement `Start()` and `Stop()` methods
- [x] Port configurable from root config or flag (default: 3000)

**Tests:**
- [x] Server starts and listens on configured port
- [x] Server stops gracefully
- [x] Custom port from config works

**Acceptance criteria:** Fiber server starts and stops from config.

---

## Task 8.2: Middleware Loading

**Description:** Load and configure all Fiber middleware from config.

**Subtasks:**

- [x] Create `internal/server/middleware.go`
- [x] Implement middleware registry mapping middleware names to Fiber middleware constructors:
  - `auth.jwt` → gofiber/contrib/jwt middleware with config from `security.jwt`
  - `security.cors` → Fiber CORS middleware
  - `security.headers` → Fiber Helmet middleware
  - `security.csrf` → Fiber CSRF middleware
  - `limiter` → Fiber Rate Limiter
  - `logger` → Fiber Logger
  - `requestid` → Fiber RequestID
  - `recover` → Fiber Recover
  - `timeout` → Fiber Timeout
  - `compress` → Fiber Compress
  - `etag` → Fiber ETag
  - `cache` → Fiber Cache (HTTP response caching)
- [x] JWT middleware: validate token, extract claims, populate `$.auth` on request context
- [x] Each middleware reads its config from the root config's `security` or `middleware` section

**Tests:**
- [x] JWT middleware rejects invalid tokens (401)
- [x] JWT middleware passes valid tokens, claims accessible
- [x] CORS headers applied
- [x] Rate limiter rejects excess requests (429)
- [x] Request ID generated and present in response
- [x] Recovery catches panics and returns 500

**Acceptance criteria:** All middleware configurable from JSON, applies correctly.

---

## Task 8.3: Middleware Presets and Route Groups

**Description:** Resolve middleware presets and apply group-level middleware to routes.

**Subtasks:**

- [x] Create `internal/server/presets.go`
- [x] Implement `ResolveMiddleware(route, groups, presets)`:
  - If route has `middleware_preset` → expand to middleware list from presets config
  - If route is under a route group → inherit group's middleware/preset
  - Route-level middleware extends (not replaces) group middleware
  - Final middleware chain: global → group → route-specific
- [x] Validate at startup: preset names referenced in routes/groups must exist in `middleware_presets`

**Tests:**
- [x] Route with preset gets expanded middleware
- [x] Route in group inherits group middleware
- [x] Route middleware extends group middleware
- [x] Unknown preset name → startup error

**Acceptance criteria:** Middleware chains compose correctly from presets and groups.

---

## Task 8.4: Route Registration

**Description:** Translate route configs into Fiber handlers.

**Subtasks:**

- [x] Create `internal/server/routes.go`
- [x] Implement `RegisterRoutes(app *fiber.App, routes []RouteConfig, ...)`:
  - For each route: register a Fiber handler at `method + path`
  - Apply resolved middleware chain
  - Handler: extract trigger data → run trigger mapping → execute workflow → handle response
- [x] Path parameter mapping: Fiber's `:param` syntax matches the route config format
- [x] Method mapping: `GET`, `POST`, `PUT`, `PATCH`, `DELETE`
- [x] Tag-based grouping for OpenAPI

**Tests:**
- [x] GET route responds to GET requests
- [x] POST route responds to POST requests
- [x] Path parameters extracted correctly (`:id` → `request.params.id`)
- [x] Unknown path returns 404
- [x] Wrong method returns 405

**Acceptance criteria:** Routes register and handle requests with correct middleware.

---

## Task 8.5: Trigger Mapping Layer

**Description:** Evaluate input expressions against raw request data to populate `$.input`.

**Subtasks:**

- [x] Create `internal/server/trigger.go`
- [x] Implement `MapTrigger(fiberCtx, triggerConfig) (*TriggerResult, error)`:
  - Build raw trigger context: `request.body`, `request.params`, `request.query`, `request.headers`
  - Evaluate each expression in `trigger.input` against the raw context
  - Produce `$.input` map
  - Populate `$.auth` from JWT middleware context (if present)
  - Populate `$.trigger` with `{ type: "http", timestamp, trace_id }`
- [x] After mapping, raw request data is no longer accessible to the workflow
- [x] Expression defaults: `{{ request.query.page ?? 1 }}` works for optional params

**Tests:**
- [x] Body fields mapped to input
- [x] Path parameters mapped to input
- [x] Query parameters mapped to input
- [x] Headers accessible in expressions
- [x] Default values via `??` operator
- [x] Auth data populated from JWT claims
- [x] Trigger metadata correct (type: "http")
- [x] Raw request not accessible after mapping

**Acceptance criteria:** Request data maps to `$.input` via expressions.

---

## Task 8.6: File Stream Passthrough

**Description:** Handle `files` array in trigger config for file upload routes.

**Subtasks:**

- [x] Extend trigger mapping: when `trigger.files` lists field names, pass those fields as raw file streams (not resolved as expressions)
- [x] File streams are multipart form file handles — the `upload.handle` node (M13) will consume them
- [x] Non-file fields still resolve normally

**Tests:**
- [x] File field passed through as stream
- [x] Non-file fields resolve normally alongside file fields
- [x] Missing file field → error

**Acceptance criteria:** File uploads are passed through for downstream node processing.

---

## Task 8.7: Raw Body Preservation

**Description:** Preserve unparsed request body for webhook signature verification.

**Subtasks:**

- [x] When `trigger.raw_body: true`:
  - Read the raw body bytes before any parsing
  - Store as `$.trigger.raw_body` on the execution context
  - Normal `trigger.input` mapping still works alongside raw_body
- [x] When `trigger.raw_body: false` (default): body is consumed by parsing, raw bytes not preserved

**Tests:**
- [x] `raw_body: true` → `trigger.raw_body` contains raw bytes
- [x] `raw_body: true` → `input` mapping still works
- [x] `raw_body: false` → `trigger.raw_body` not present
- [x] Raw body matches exactly what was sent (byte-for-byte)

**Acceptance criteria:** Webhook signature verification is possible with `raw_body`.

---

## Task 8.8: Response Node Plugin

**Description:** Implement `response.json`, `response.redirect`, `response.error` nodes.

**Subtasks:**

- [x] Create `plugins/core/response/plugin.go`:
  - Name: `"core.response"`, Prefix: `"response"`
  - HasServices: false
  - Nodes: `response.json`, `response.redirect`, `response.error`
- [x] `response.json`: resolve `status`, `body`, `headers`, `cookies` → produce `api.HTTPResponse`
- [x] `response.redirect`: resolve `url`, use static `status` (default 302), produce `HTTPResponse` with `Location` header
- [x] `response.error`: resolve `status`, `code`, `message`, `details` → produce `HTTPResponse` with standardized error body, inject `trace_id` automatically

**Tests:**
- [x] `response.json` produces correct HTTPResponse
- [x] `response.redirect` produces redirect with Location header
- [x] `response.error` produces standardized error body with trace_id
- [x] Headers and cookies included in response
- [x] Expression resolution works in all fields

**Acceptance criteria:** All three response nodes produce correct `HTTPResponse` objects.

---

## Task 8.9: Response Handling — Go Channel Mechanism

**Description:** Implement the response pipeline: workflow fires response node → HTTP response sent → workflow continues async.

**Subtasks:**

- [x] Create `internal/server/response.go`
- [x] In the Fiber handler:
  - Create a Go channel for HTTPResponse
  - Start workflow execution in a goroutine
  - The handler waits on the channel with a timeout
  - When any node produces an `HTTPResponse` (detected by type assertion on output data), send it down the channel
  - Handler receives it, writes to `fiber.Ctx`, returns (releases the Fiber context)
  - Workflow continues executing remaining nodes on the engine's goroutine pool
- [x] No response node case: if workflow completes without producing an HTTPResponse → return 202 Accepted
- [x] Error case: if workflow fails before response node → return standardized error with HTTP status from error mapping
- [x] Background nodes never touch `fiber.Ctx` — they only access the Noda execution context

**Tests:**
- [x] Response sent when response node fires
- [x] Workflow continues after response (verify with a logging node after response)
- [x] No response node → 202 Accepted
- [x] Workflow failure → error response with correct status code
- [x] Timeout on response channel → 504 Gateway Timeout
- [x] Concurrent requests handled correctly

**Acceptance criteria:** HTTP responses send immediately on response node, workflow continues async.

---

## Task 8.10: HTTP Error Mapping

**Description:** Map standard errors to HTTP status codes.

**Subtasks:**

- [x] Create `internal/server/errors.go`
- [x] Implement `MapErrorToHTTP(err error) (int, api.ErrorData)`:
  - `ValidationError` → 422
  - `NotFoundError` → 404
  - `ConflictError` → 409
  - `ServiceUnavailableError` → 503
  - `TimeoutError` → 504
  - Untyped error → 500
- [x] Produce standardized error body: `{ "error": { "code", "message", "details", "trace_id" } }`
- [x] Used when workflow fails without reaching a response node

**Tests:**
- [x] Each error type maps to correct status code
- [x] Error body follows standardized format
- [x] Trace ID included in error response
- [x] Unknown error type → 500

**Acceptance criteria:** All workflow errors produce correct HTTP status codes.

---

## Task 8.11: OpenAPI Generation

**Description:** Generate OpenAPI 3.1 spec from route configs.

**Subtasks:**

- [x] Create `internal/server/openapi.go`
- [x] Implement `GenerateOpenAPI(config *ResolvedConfig) (*openapi3.T, error)` using `getkin/kin-openapi`
- [x] Map:
  - Routes → operations (path + method)
  - Route `body.schema` → request body
  - Route `params`/`query` → parameters
  - Route `response` → response definitions
  - `$ref` schemas → component schemas
  - `tags` → operation tags
  - JWT middleware → security scheme
- [x] Serve at `/docs` (Swagger or Scalar UI) and `/openapi.json` (raw spec)
- [x] CLI: `noda generate openapi` exports the spec to a file

**Tests:**
- [x] Generated spec is valid OpenAPI 3.1
- [x] Routes produce correct operations
- [x] Schemas map to components
- [x] Security schemes generated from JWT config
- [x] `/openapi.json` endpoint returns the spec

**Acceptance criteria:** OpenAPI spec auto-generated from route configs.

---

## Task 8.12: Integration and End-to-End Tests

**Description:** Full HTTP request → workflow → response tests.

**Subtasks:**

- [x] Test: GET request → workflow with transform.set → response.json with 200
- [x] Test: POST request → trigger mapping extracts body fields → workflow → response.json with 201
- [x] Test: Path parameters → available in trigger mapping expressions
- [x] Test: JWT middleware → `$.auth` available in workflow expressions
- [x] Test: Parallel workflow → early response → background node still executes
- [x] Test: No response node → 202 Accepted
- [x] Test: Workflow error → standardized error response with correct status
- [x] Test: Middleware chain order (auth before handler)
- [x] Test: Route groups with preset middleware
- [x] Test: `raw_body: true` → raw bytes preserved alongside parsed input
- [x] Test: OpenAPI spec endpoint returns valid spec
- [x] Use Case 1 walkthrough: create a minimal task API (CRUD) and test all routes

**Acceptance criteria:** HTTP server handles real workflows end-to-end. First demo-able moment.
