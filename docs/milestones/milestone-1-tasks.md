# Milestone 1: Config Loading — Task Breakdown

**Depends on:** Milestone 0 (project skeleton, `pkg/api/` interfaces)
**Result:** `noda validate` loads, merges, resolves, and validates all JSON config files. Every error reports the file path and a clear message.

---

## Task 1.1: Config File Discovery

**Description:** Implement the file discovery system that scans the project directory and categorizes config files by convention.

**Subtasks:**

- [ ] Create `internal/config/discovery.go` with a `Discover(rootPath string) (*DiscoveredFiles, error)` function
- [ ] Define `DiscoveredFiles` struct containing categorized file paths:
  - `Root` — path to `noda.json` (required, error if missing)
  - `Overlay` — path to `noda.{env}.json` (optional)
  - `Schemas` — paths to `schemas/**/*.json`
  - `Routes` — paths to `routes/**/*.json`
  - `Workflows` — paths to `workflows/**/*.json`
  - `Workers` — paths to `workers/**/*.json`
  - `Schedules` — paths to `schedules/**/*.json`
  - `Connections` — paths to `connections/**/*.json`
  - `Tests` — paths to `tests/**/*.json`
- [ ] Scan directories recursively — `routes/v1/users.json` is discovered
- [ ] Filter to `.json` files only — non-JSON files are ignored with a warning log
- [ ] Accept `env` parameter to determine which overlay file to look for
- [ ] Missing `noda.json` produces a clear error with the expected path
- [ ] Missing config directories are fine (empty list, no error)

**Tests:**
- [ ] Discover all files from a fully populated test project
- [ ] Discover from minimal project (only `noda.json`)
- [ ] Nested files in subdirectories are found
- [ ] Non-JSON files are ignored
- [ ] Missing `noda.json` returns error
- [ ] Overlay file matched by environment name

**Acceptance criteria:** Given a project directory and environment name, returns all categorized config file paths.

---

## Task 1.2: JSON File Loading

**Description:** Load each discovered JSON file into raw Go maps, collecting all errors.

**Subtasks:**

- [ ] Create `internal/config/loader.go` with `LoadAll(discovered *DiscoveredFiles) (*RawConfig, []error)` function
- [ ] Define `RawConfig` struct:
  ```go
  type RawConfig struct {
      Root        map[string]any
      Overlay     map[string]any              // nil if no overlay
      Schemas     map[string]map[string]any   // keyed by file path
      Routes      map[string]map[string]any
      Workflows   map[string]map[string]any
      Workers     map[string]map[string]any
      Schedules   map[string]map[string]any
      Connections map[string]map[string]any
      Tests       map[string]map[string]any
  }
  ```
- [ ] Implement `loadJSONFile(path string) (map[string]any, error)` — reads and parses a single file
- [ ] On parse error: include file path and the JSON parser's position/message in the error
- [ ] Continue loading all files even if some fail — collect all errors, return them together
- [ ] Handle edge cases: empty files (`{}`), BOM markers, UTF-8 encoding

**Tests:**
- [ ] Valid JSON files load correctly
- [ ] Invalid JSON returns error with file path and position
- [ ] Multiple broken files produce multiple errors (not just the first)
- [ ] Empty JSON object `{}` is valid
- [ ] Large files load without issue

**Acceptance criteria:** All discovered files are loaded into `RawConfig`. Errors include file paths.

---

## Task 1.3: Environment Overlay Merging

**Description:** Deep-merge the environment overlay into the base `noda.json` config.

**Subtasks:**

- [ ] Create `internal/config/merge.go` with `MergeOverlay(base, overlay map[string]any) map[string]any`
- [ ] Implement deep merge rules:
  - Scalar values: overlay replaces base
  - Objects (maps): merge recursively — overlay keys override, base-only keys preserved
  - Arrays: overlay replaces base entirely (arrays are atomic, no element-level merge)
  - `null` in overlay: removes the key from the result
- [ ] The merge produces a new map (does not mutate base or overlay)
- [ ] If overlay is nil (no overlay file), return base unchanged

**Tests:**
- [ ] Simple scalar override
- [ ] Nested object merge — overlay adds new keys, overrides existing, preserves untouched
- [ ] Array replacement — overlay array completely replaces base array
- [ ] Null removes key from result
- [ ] Deeply nested merge (3+ levels)
- [ ] No overlay — base returned unchanged
- [ ] Empty overlay `{}` — base returned unchanged

**Acceptance criteria:** Merged config correctly combines base and overlay at all nesting levels.

---

## Task 1.4: Environment Detection

**Description:** Determine the active environment from flag, env var, or default.

**Subtasks:**

- [ ] Create `internal/config/env.go` with `DetectEnvironment(flagValue string) string`
- [ ] Priority: CLI `--env` flag (if non-empty) → `NODA_ENV` environment variable (if set) → default `"development"`
- [ ] Validate: environment name must be alphanumeric + hyphens (no path separators, no dots)

**Tests:**
- [ ] Flag value takes priority over env var
- [ ] Env var used when flag is empty
- [ ] Default `"development"` when both are empty
- [ ] Invalid environment name rejected

**Acceptance criteria:** Environment is reliably detected with correct priority.

---

## Task 1.5: `$env()` Resolution

**Description:** Replace `$env()` references in config strings with actual environment variable values.

**Subtasks:**

- [ ] Create `internal/config/envvars.go` with `ResolveEnvVars(config map[string]any) (map[string]any, []error)`
- [ ] Recursively walk the config map. For every string value, find `{{ $env('VAR_NAME') }}` patterns and replace with the env var value
- [ ] A string can contain multiple `$env()` references mixed with literal text: `"postgres://{{ $env('DB_HOST') }}:{{ $env('DB_PORT') }}/main"`
- [ ] Missing environment variable: collect error with variable name and the config path (e.g., `services.main-db.config.url`)
- [ ] Collect ALL missing var errors before returning (don't stop at first)
- [ ] `$env()` is resolved in root config AND `wasm_runtimes.*.config` sections
- [ ] `$env()` is NOT resolved in routes, workflows, workers, schedules, connections (those use runtime expressions)

**Tests:**
- [ ] Simple `$env()` replacement
- [ ] Multiple `$env()` in one string
- [ ] `$env()` inside nested objects
- [ ] Missing env var produces error with config path
- [ ] Multiple missing vars all reported
- [ ] `$env()` in wasm_runtimes config is resolved
- [ ] `$env()` preserved in non-root configs (not resolved here)

**Acceptance criteria:** All `$env()` references in root config are replaced. Missing vars produce clear errors.

---

## Task 1.6: `$ref` Resolution for Shared Schemas

**Description:** Resolve `$ref` references that point to shared schema definitions.

**Subtasks:**

- [ ] Create `internal/config/refs.go` with `ResolveRefs(config *RawConfig) []error`
- [ ] Build a schema registry from loaded schema files. A schema file can contain multiple top-level keys — `schemas/User.json` with `{ "User": {...}, "Pagination": {...} }` produces refs `schemas/User` and `schemas/Pagination`
- [ ] Walk all config structures (routes, workflows, etc.). When `{ "$ref": "schemas/User" }` is found, replace the entire object with the referenced schema content
- [ ] Handle nested `$ref`: a schema can reference another schema. Resolve recursively.
- [ ] Detect circular `$ref` chains — produce an error listing the cycle, don't infinite loop
- [ ] Missing `$ref` target: error with the ref path and the file/location where it's used
- [ ] Collect all resolution errors

**Tests:**
- [ ] Simple `$ref` replacement
- [ ] `$ref` in route response schema
- [ ] `$ref` in workflow node config (e.g., transform.validate)
- [ ] Nested `$ref` — schema A references schema B
- [ ] Circular `$ref` detected and reported
- [ ] Missing schema produces error with context
- [ ] Multiple definitions from one schema file

**Acceptance criteria:** All `$ref` references are inlined. Circular and missing refs are reported.

---

## Task 1.7: JSON Schema Definitions for Config Types

**Description:** Define validation schemas for every config file type, embedded in the binary.

**Subtasks:**

- [ ] Create `internal/config/schemas/` directory for embedded schema files
- [ ] Define root config schema (`root.json`):
  - `services` map, `security` object, `middleware_presets` map, `route_groups` map, `wasm_runtimes` map, `connections` object
- [ ] Define route schema (`route.json`):
  - Required: `id`, `method`, `path`, `trigger.workflow`
  - Optional: `summary`, `tags`, `middleware`, `params`, `query`, `body`, `response`, `trigger.input`, `trigger.files`, `trigger.raw_body`
- [ ] Define workflow schema (`workflow.json`):
  - Required: `id`, `nodes`, `edges`
  - Node shape: `type` required, `services`/`as`/`position`/`config` optional
  - Edge shape: `from`/`to` required, `output`/`retry` optional
- [ ] Define worker schema (`worker.json`):
  - Required: `id`, `services.stream`, `subscribe.topic`, `subscribe.group`, `trigger.workflow`
- [ ] Define schedule schema (`schedule.json`):
  - Required: `id`, `cron`, `trigger.workflow`
  - Optional: `services.lock`, `timezone`, `lock.enabled`, `lock.ttl`
- [ ] Define connections schema (`connections.json`):
  - `sync.pubsub` required, `endpoints` map with endpoint config shape
- [ ] Define test schema (`test.json`):
  - Required: `id`, `workflow`, `tests` array with `name`, `input`, `mocks`, `expect`
- [ ] Define wasm runtime schema (as part of root schema):
  - Required: `module`, `tick_rate`, `services`
  - Optional: `encoding`, `connections`, `allow_outbound`, `config`
- [ ] Embed all schemas using Go's `embed` package

**Tests:**
- [ ] Each schema validates a known-good config sample
- [ ] Each schema rejects a config with missing required fields
- [ ] Each schema rejects a config with wrong types
- [ ] Schemas are successfully embedded and loadable at runtime

**Acceptance criteria:** All config types have JSON Schemas. Schemas are embedded in the binary.

---

## Task 1.8: Config Validator

**Description:** Validate all loaded config files against their JSON Schemas.

**Subtasks:**

- [ ] Create `internal/config/validator.go` with `Validate(config *RawConfig) []ValidationError`
- [ ] Define `ValidationError` struct: `FilePath`, `JSONPath`, `Message`, `SchemaPath`
- [ ] Use `santhosh-tekuri/jsonschema/v6` to compile each embedded schema and validate each config file
- [ ] Validate in order: root → routes → workflows → workers → schedules → connections → tests
- [ ] Collect ALL errors across ALL files before returning
- [ ] Each error includes:
  - File path (e.g., `routes/tasks.json`)
  - JSON path within file (e.g., `.trigger.input.email`)
  - Human-readable message (e.g., `required field 'workflow' is missing`)

**Tests:**
- [ ] Valid sample project passes with zero errors
- [ ] Missing required field produces error with correct file and JSON path
- [ ] Wrong type produces error
- [ ] Multiple errors across multiple files all collected
- [ ] Unknown/extra fields do not produce errors (forward compatibility)

**Acceptance criteria:** All config files are validated. Errors are detailed and actionable.

---

## Task 1.9: Cross-File Reference Validation

**Description:** Validate that references between config files are consistent.

**Subtasks:**

- [ ] Create `internal/config/crossrefs.go` with `ValidateCrossRefs(config *RawConfig) []ValidationError`
- [ ] Route → Workflow: every `trigger.workflow` must reference an existing workflow `id`
- [ ] Worker → Stream service: `services.stream` must reference a service in root config with stream plugin
- [ ] Worker → Workflow: `trigger.workflow` must reference an existing workflow `id`
- [ ] Schedule → Cache service: `services.lock` must reference a cache service (when `lock.enabled`)
- [ ] Schedule → Workflow: `trigger.workflow` must reference an existing workflow `id`
- [ ] Connection → PubSub: `sync.pubsub` must reference a pubsub service
- [ ] Connection lifecycle → Workflow: `on_connect`/`on_message`/`on_disconnect` must reference existing workflow IDs
- [ ] Workflow → Workflow: `workflow.run` and `control.loop` node configs' `workflow` field must reference existing workflow IDs
- [ ] Middleware presets: names used in routes/groups should be defined in `middleware_presets`
- [ ] Collect all reference errors

**Tests:**
- [ ] Route references non-existent workflow → error
- [ ] Worker references non-existent stream service → error
- [ ] Worker references service that exists but wrong type → error
- [ ] Schedule references non-existent lock service → error
- [ ] Connection lifecycle references non-existent workflow → error
- [ ] workflow.run references non-existent sub-workflow → error
- [ ] All valid references pass
- [ ] Multiple errors collected together

**Acceptance criteria:** All cross-file references are validated. Broken references produce specific errors.

---

## Task 1.10: Error Formatting

**Description:** Format validation errors for CLI display.

**Subtasks:**

- [ ] Create `internal/config/format.go` with `FormatErrors(errors []ValidationError) string`
- [ ] Group errors by file path
- [ ] For each file: show file path header, then indented errors with JSON path and message
- [ ] Detect terminal color support — use red for file paths, yellow for JSON paths if supported
- [ ] Summary line at end: "X errors in Y files"
- [ ] When zero errors: return empty string (caller handles success message)

**Tests:**
- [ ] Single error formats correctly
- [ ] Multiple errors grouped by file
- [ ] Summary line is accurate
- [ ] Non-color output is readable

**Acceptance criteria:** Error output is clear, grouped, and actionable.

---

## Task 1.11: Full Pipeline — `ValidateAll()`

**Description:** Wire the full config loading pipeline into a single function.

**Subtasks:**

- [ ] Create `internal/config/pipeline.go` with `ValidateAll(rootPath string, env string) (*ResolvedConfig, []ValidationError)`
- [ ] Pipeline steps:
  1. Detect environment
  2. Discover files
  3. Load all JSON files
  4. Merge overlay into root
  5. Resolve `$env()` in root config
  6. Resolve `$ref` in all configs
  7. Validate all configs against schemas
  8. Validate cross-file references
  9. Return resolved config or errors
- [ ] Define `ResolvedConfig` — the fully resolved, validated config structure that downstream systems consume
- [ ] Any step that produces errors short-circuits remaining validation (no point validating schemas if JSON parsing failed)

**Tests:**
- [ ] Full pipeline on valid project → success, resolved config returned
- [ ] Full pipeline on project with broken JSON → JSON errors only (no schema validation attempted)
- [ ] Full pipeline on project with schema errors → schema errors returned
- [ ] Full pipeline on project with cross-ref errors → cross-ref errors returned
- [ ] Full pipeline with missing env vars → env var errors returned

**Acceptance criteria:** Single function runs the entire config loading pipeline.

---

## Task 1.12: Wire `noda validate` Command

**Description:** Connect the config pipeline to the CLI validate command.

**Subtasks:**

- [ ] Update `cmd/noda/` to replace the placeholder `validate` command with the real implementation
- [ ] `noda validate`:
  - Reads `--config` flag for project root (default: `.`)
  - Reads `--env` flag for environment
  - Calls `ValidateAll()`
  - On success: prints "✓ All config files valid (X files checked)", exits 0
  - On failure: prints formatted errors, exits 1
- [ ] `noda validate --verbose`:
  - Also prints: environment detected, overlay file used, file counts per category

**Tests:**
- [ ] `noda validate` on valid project → exit 0, success message
- [ ] `noda validate` on invalid project → exit 1, error output
- [ ] `noda validate --verbose` shows additional detail
- [ ] `noda validate --env production` loads production overlay
- [ ] `noda validate --config /path/to/project` uses specified root

**Acceptance criteria:** `noda validate` is fully functional end-to-end.

---

## Task 1.13: Sample Test Projects

**Description:** Create realistic test fixture projects for integration and end-to-end tests.

**Subtasks:**

- [ ] Create `testdata/valid-project/` with:
  - `noda.json` — services (two postgres, one cache, one stream, one pubsub), JWT config, middleware presets, route groups
  - `noda.development.json` — overlay with dev database URL
  - `schemas/User.json` — User and Pagination definitions
  - `schemas/Task.json` — Task definition
  - `routes/tasks.json` — 5 CRUD routes referencing task workflows
  - `workflows/create-task.json` — workflow with nodes and edges
  - `workflows/list-tasks.json` — workflow with parallel nodes
  - `workflows/get-task.json` — workflow with control.if
  - `workers/notifications.json` — worker referencing stream service
  - `schedules/cleanup.json` — schedule with lock config
  - `connections/realtime.json` — WebSocket endpoint
  - `tests/test-create-task.json` — test definition
- [ ] Create `testdata/invalid-project/` with intentionally broken configs:
  - Missing required fields
  - Invalid JSON syntax in one file
  - Missing `$ref` target
  - Cross-reference errors (route → non-existent workflow)
  - Wrong service type (worker references cache instead of stream)
  - Circular `$ref`
- [ ] Create `testdata/minimal-project/` — just `noda.json` with one route and one workflow

**Tests:**
- [ ] Valid project passes full validation
- [ ] Invalid project produces all expected errors (check specific messages)
- [ ] Minimal project passes validation

**Acceptance criteria:** Test fixtures cover the full range of valid, invalid, and edge-case configs.
