# Milestone 8: HTTP Server Runtime — Task Breakdown

**Depends on:** Milestone 4 (workflow engine), Milestone 5 (control nodes), Milestone 6 (transform + utility)
**Result:** Fiber HTTP server runs, routes load from config, middleware chains apply, trigger mapping populates `$.input`, response nodes send HTTP responses, and the workflow continues async after the response. OpenAPI spec generated from routes.

---

## Task 8.1: Fiber Server Initialization

**Description:** Initialize Fiber v3 from root config.

**Subtasks:**

- [ ] Create `internal/server/server.go`
- [ ] Implement `NewServer(config *ResolvedConfig) (*Server, error)`:
  - Create Fiber app with configured settings (port, read/write timeouts, body limit)
  - Configure Fiber error handler to return standardized error format
  - Store references to workflow engine, service registry, expression engine
- [ ] Implement `Start()` and `Stop()` methods
- [ ] Port configurable from root config or flag (default: 3000)

**Tests:**
- [ ] Server starts and listens on configured port
- [ ] Server stops gracefully
- [ ] Custom port from config works

**Acceptance criteria:** Fiber server starts and stops from config.

---

## Task 8.2: Middleware Loading

**Description:** Load and configure all Fiber middleware from config.

**Subtasks:**

- [ ] Create `internal/server/middleware.go`
- [ ] Implement middleware registry mapping middleware names to Fiber middleware constructors:
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
- [ ] JWT middleware: validate token, extract claims, populate `$.auth` on request context
- [ ] Each middleware reads its config from the root config's `security` or `middleware` section

**Tests:**
- [ ] JWT middleware rejects invalid tokens (401)
- [ ] JWT middleware passes valid tokens, claims accessible
- [ ] CORS headers applied
- [ ] Rate limiter rejects excess requests (429)
- [ ] Request ID generated and present in response
- [ ] Recovery catches panics and returns 500

**Acceptance criteria:** All middleware configurable from JSON, applies correctly.

---

## Task 8.3: Middleware Presets and Route Groups

**Description:** Resolve middleware presets and apply group-level middleware to routes.

**Subtasks:**

- [ ] Create `internal/server/presets.go`
- [ ] Implement `ResolveMiddleware(route, groups, presets)`:
  - If route has `middleware_preset` → expand to middleware list from presets config
  - If route is under a route group → inherit group's middleware/preset
  - Route-level middleware extends (not replaces) group middleware
  - Final middleware chain: global → group → route-specific
- [ ] Validate at startup: preset names referenced in routes/groups must exist in `middleware_presets`

**Tests:**
- [ ] Route with preset gets expanded middleware
- [ ] Route in group inherits group middleware
- [ ] Route middleware extends group middleware
- [ ] Unknown preset name → startup error

**Acceptance criteria:** Middleware chains compose correctly from presets and groups.

---

## Task 8.4: Route Registration

**Description:** Translate route configs into Fiber handlers.

**Subtasks:**

- [ ] Create `internal/server/routes.go`
- [ ] Implement `RegisterRoutes(app *fiber.App, routes []RouteConfig, ...)`:
  - For each route: register a Fiber handler at `method + path`
  - Apply resolved middleware chain
  - Handler: extract trigger data → run trigger mapping → execute workflow → handle response
- [ ] Path parameter mapping: Fiber's `:param` syntax matches the route config format
- [ ] Method mapping: `GET`, `POST`, `PUT`, `PATCH`, `DELETE`
- [ ] Tag-based grouping for OpenAPI

**Tests:**
- [ ] GET route responds to GET requests
- [ ] POST route responds to POST requests
- [ ] Path parameters extracted correctly (`:id` → `request.params.id`)
- [ ] Unknown path returns 404
- [ ] Wrong method returns 405

**Acceptance criteria:** Routes register and handle requests with correct middleware.

---

## Task 8.5: Trigger Mapping Layer

**Description:** Evaluate input expressions against raw request data to populate `$.input`.

**Subtasks:**

- [ ] Create `internal/server/trigger.go`
- [ ] Implement `MapTrigger(fiberCtx, triggerConfig) (*TriggerResult, error)`:
  - Build raw trigger context: `request.body`, `request.params`, `request.query`, `request.headers`
  - Evaluate each expression in `trigger.input` against the raw context
  - Produce `$.input` map
  - Populate `$.auth` from JWT middleware context (if present)
  - Populate `$.trigger` with `{ type: "http", timestamp, trace_id }`
- [ ] After mapping, raw request data is no longer accessible to the workflow
- [ ] Expression defaults: `{{ request.query.page ?? 1 }}` works for optional params

**Tests:**
- [ ] Body fields mapped to input
- [ ] Path parameters mapped to input
- [ ] Query parameters mapped to input
- [ ] Headers accessible in expressions
- [ ] Default values via `??` operator
- [ ] Auth data populated from JWT claims
- [ ] Trigger metadata correct (type: "http")
- [ ] Raw request not accessible after mapping

**Acceptance criteria:** Request data maps to `$.input` via expressions.

---

## Task 8.6: File Stream Passthrough

**Description:** Handle `files` array in trigger config for file upload routes.

**Subtasks:**

- [ ] Extend trigger mapping: when `trigger.files` lists field names, pass those fields as raw file streams (not resolved as expressions)
- [ ] File streams are multipart form file handles — the `upload.handle` node (M13) will consume them
- [ ] Non-file fields still resolve normally

**Tests:**
- [ ] File field passed through as stream
- [ ] Non-file fields resolve normally alongside file fields
- [ ] Missing file field → error

**Acceptance criteria:** File uploads are passed through for downstream node processing.

---

## Task 8.7: Raw Body Preservation

**Description:** Preserve unparsed request body for webhook signature verification.

**Subtasks:**

- [ ] When `trigger.raw_body: true`:
  - Read the raw body bytes before any parsing
  - Store as `$.trigger.raw_body` on the execution context
  - Normal `trigger.input` mapping still works alongside raw_body
- [ ] When `trigger.raw_body: false` (default): body is consumed by parsing, raw bytes not preserved

**Tests:**
- [ ] `raw_body: true` → `trigger.raw_body` contains raw bytes
- [ ] `raw_body: true` → `input` mapping still works
- [ ] `raw_body: false` → `trigger.raw_body` not present
- [ ] Raw body matches exactly what was sent (byte-for-byte)

**Acceptance criteria:** Webhook signature verification is possible with `raw_body`.

---

## Task 8.8: Response Node Plugin

**Description:** Implement `response.json`, `response.redirect`, `response.error` nodes.

**Subtasks:**

- [ ] Create `plugins/core/response/plugin.go`:
  - Name: `"core.response"`, Prefix: `"response"`
  - HasServices: false
  - Nodes: `response.json`, `response.redirect`, `response.error`
- [ ] `response.json`: resolve `status`, `body`, `headers`, `cookies` → produce `api.HTTPResponse`
- [ ] `response.redirect`: resolve `url`, use static `status` (default 302), produce `HTTPResponse` with `Location` header
- [ ] `response.error`: resolve `status`, `code`, `message`, `details` → produce `HTTPResponse` with standardized error body, inject `trace_id` automatically

**Tests:**
- [ ] `response.json` produces correct HTTPResponse
- [ ] `response.redirect` produces redirect with Location header
- [ ] `response.error` produces standardized error body with trace_id
- [ ] Headers and cookies included in response
- [ ] Expression resolution works in all fields

**Acceptance criteria:** All three response nodes produce correct `HTTPResponse` objects.

---

## Task 8.9: Response Handling — Go Channel Mechanism

**Description:** Implement the response pipeline: workflow fires response node → HTTP response sent → workflow continues async.

**Subtasks:**

- [ ] Create `internal/server/response.go`
- [ ] In the Fiber handler:
  - Create a Go channel for HTTPResponse
  - Start workflow execution in a goroutine
  - The handler waits on the channel with a timeout
  - When any node produces an `HTTPResponse` (detected by type assertion on output data), send it down the channel
  - Handler receives it, writes to `fiber.Ctx`, returns (releases the Fiber context)
  - Workflow continues executing remaining nodes on the engine's goroutine pool
- [ ] No response node case: if workflow completes without producing an HTTPResponse → return 202 Accepted
- [ ] Error case: if workflow fails before response node → return standardized error with HTTP status from error mapping
- [ ] Background nodes never touch `fiber.Ctx` — they only access the Noda execution context

**Tests:**
- [ ] Response sent when response node fires
- [ ] Workflow continues after response (verify with a logging node after response)
- [ ] No response node → 202 Accepted
- [ ] Workflow failure → error response with correct status code
- [ ] Timeout on response channel → 504 Gateway Timeout
- [ ] Concurrent requests handled correctly

**Acceptance criteria:** HTTP responses send immediately on response node, workflow continues async.

---

## Task 8.10: HTTP Error Mapping

**Description:** Map standard errors to HTTP status codes.

**Subtasks:**

- [ ] Create `internal/server/errors.go`
- [ ] Implement `MapErrorToHTTP(err error) (int, api.ErrorData)`:
  - `ValidationError` → 422
  - `NotFoundError` → 404
  - `ConflictError` → 409
  - `ServiceUnavailableError` → 503
  - `TimeoutError` → 504
  - Untyped error → 500
- [ ] Produce standardized error body: `{ "error": { "code", "message", "details", "trace_id" } }`
- [ ] Used when workflow fails without reaching a response node

**Tests:**
- [ ] Each error type maps to correct status code
- [ ] Error body follows standardized format
- [ ] Trace ID included in error response
- [ ] Unknown error type → 500

**Acceptance criteria:** All workflow errors produce correct HTTP status codes.

---

## Task 8.11: OpenAPI Generation

**Description:** Generate OpenAPI 3.1 spec from route configs.

**Subtasks:**

- [ ] Create `internal/server/openapi.go`
- [ ] Implement `GenerateOpenAPI(config *ResolvedConfig) (*openapi3.T, error)` using `getkin/kin-openapi`
- [ ] Map:
  - Routes → operations (path + method)
  - Route `body.schema` → request body
  - Route `params`/`query` → parameters
  - Route `response` → response definitions
  - `$ref` schemas → component schemas
  - `tags` → operation tags
  - JWT middleware → security scheme
- [ ] Serve at `/docs` (Swagger or Scalar UI) and `/openapi.json` (raw spec)
- [ ] CLI: `noda generate openapi` exports the spec to a file

**Tests:**
- [ ] Generated spec is valid OpenAPI 3.1
- [ ] Routes produce correct operations
- [ ] Schemas map to components
- [ ] Security schemes generated from JWT config
- [ ] `/openapi.json` endpoint returns the spec

**Acceptance criteria:** OpenAPI spec auto-generated from route configs.

---

## Task 8.12: Integration and End-to-End Tests

**Description:** Full HTTP request → workflow → response tests.

**Subtasks:**

- [ ] Test: GET request → workflow with transform.set → response.json with 200
- [ ] Test: POST request → trigger mapping extracts body fields → workflow → response.json with 201
- [ ] Test: Path parameters → available in trigger mapping expressions
- [ ] Test: JWT middleware → `$.auth` available in workflow expressions
- [ ] Test: Parallel workflow → early response → background node still executes
- [ ] Test: No response node → 202 Accepted
- [ ] Test: Workflow error → standardized error response with correct status
- [ ] Test: Middleware chain order (auth before handler)
- [ ] Test: Route groups with preset middleware
- [ ] Test: `raw_body: true` → raw bytes preserved alongside parsed input
- [ ] Test: OpenAPI spec endpoint returns valid spec
- [ ] Use Case 1 walkthrough: create a minimal task API (CRUD) and test all routes

**Acceptance criteria:** HTTP server handles real workflows end-to-end. First demo-able moment.
