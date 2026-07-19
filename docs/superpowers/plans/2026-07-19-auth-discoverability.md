# Auth-Plugin Discoverability + Validation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Close #374–#379 and #381 — plugin service configs become declared (`ServiceConfigSchema`), validated (dry-run parity on all five surfaces), and discoverable (MCP tool + docs); the auth example teaches the built-in plugin; edge outputs validate; scaffolds ship working secrets.

**Architecture:** Per spec `docs/superpowers/specs/2026-07-19-auth-discoverability-design.md`. One PR on branch `auth-discoverability` (current). The `api.Plugin` interface change (Task 1) ripples through every plugin (Task 2), then enforcement (3), parity port of the engine's edge check (4), MCP tool (5), example rewrite (6), scaffold secret (7), docs + assembly (8).

**Tech Stack:** Go, santhosh-tekuri/jsonschema/v6 (existing dep), mark3labs/mcp-go (existing), testify.

## Global Constraints

- TDD; conventional commits; every commit ends `Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>`.
- Verification per task: `go build ./... && go vet ./...` + named package tests; before PR: full `go test ./internal/... ./plugins/... ./cmd/... ./pkg/... -count=1`, `go vet -tags=integration ./...`, and `gofmt -l .` empty outside `editor/`.
- Service config schemas are STRUCTURAL only: required keys, types, enums, descriptions — no `minLength`/content constraints (values are often `$env()`-resolved and may be empty at validate time).
- Schemas use the same map-literal conventions as node `ConfigSchema` implementations (see any `plugins/*/plugin.go` node descriptor): `map[string]any{"type": "object", "properties": ..., "required": []any{...}, "additionalProperties": ...}`.
- Error wording for new validation: `service %q (plugin %q): <schema message>` and `workflow %q: edge %q -> %q: output %q not among declared outputs [...]`.
- CHANGELOG entries land in the task that causes the change, under `## [Unreleased]` (read neighboring entries for style).
- Do not touch `docs/03-nodes/*` cookbook `## Runnable example` link lines (coverage gate).

---

### Task 1: `ServiceConfigSchema` contract + audit test + auth plugin schema

**Files:**
- Modify: `pkg/api/plugin.go` (interface), `plugins/auth/plugin.go`
- Test: `internal/registry/service_schema_audit_test.go` (create), `plugins/auth/plugin_test.go`

**Interfaces:**
- Produces: `api.Plugin.ServiceConfigSchema() map[string]any` — nil for service-less plugins; `type: "object"` schema for service-bearing ones. Every later task depends on this exact method name.
- NOTE: this task intentionally BREAKS the build for all other Plugin implementers; Task 2 fixes them. Commit Tasks 1+2 together only if the audit test cannot run otherwise — preferred: implement Task 1, leave the build red at the interface level, complete Task 2, then run both tasks' tests and commit as two commits back-to-back (test commit gates allow this because the audit test compiles only after Task 2).

- [ ] **Step 1: Write the audit test** (`internal/registry/service_schema_audit_test.go`), mirroring the structure of the existing node-schema audit (find it: `grep -rn "func Test.*Audit" internal/registry/ plugins/` — the #338 audit; mirror its plugin-enumeration approach; if it enumerates via `RegisterCorePlugins`/bootstrap helpers, reuse those):

```go
// TestServiceConfigSchemaAudit: every registered plugin with HasServices()
// must declare a structural JSON Schema for its service config; plugins
// without services must return nil (#375, #376).
func TestServiceConfigSchemaAudit(t *testing.T) {
	plugins := allRegisteredPlugins(t) // reuse/adapt the node-audit's enumeration helper
	for _, p := range plugins {
		schema := p.ServiceConfigSchema()
		if !p.HasServices() {
			assert.Nil(t, schema, "plugin %q has no services and must return nil", p.Name())
			continue
		}
		require.NotNil(t, schema, "plugin %q has services and must declare a ServiceConfigSchema", p.Name())
		assert.Equal(t, "object", schema["type"], "plugin %q schema root must be type object", p.Name())
		// every schema must compile
		_, err := compileServiceSchema(p.Name(), schema) // helper added in Task 3; for now inline jsonschema compile
		require.NoError(t, err, "plugin %q schema must compile", p.Name())
	}
}
```

- [ ] **Step 2: Add the interface method** in `pkg/api/plugin.go` after `HasServices()`:

```go
	// ServiceConfigSchema returns a JSON Schema (as a Go map, same
	// conventions as NodeDescriptor.ConfigSchema) describing this plugin's
	// service `config` block. Structural only: required keys, types,
	// enums, descriptions — no value-content constraints, because values
	// are frequently $env()-resolved and may be empty at validate time.
	// Plugins without services return nil.
	ServiceConfigSchema() map[string]any
```

- [ ] **Step 3: Implement the auth schema** in `plugins/auth/plugin.go`, derived from `newService` (`plugins/auth/service.go:70-145` — read it end to end; the keys observed are `database` (required), `session.ttl` (duration string), `session.cookie.{name,path,domain,same_site,secure,http_only}`, `argon2.{...}` (enumerate its keys from :116-128), `tokens.{...}` (duration strings, keys from :130+). Include EVERY key the parser reads — the Step 4 sync test enforces the required set, and the reviewer will diff schema keys against the parser):

```go
func (p *Plugin) ServiceConfigSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"database": map[string]any{"type": "string", "description": "Name of the db service (services.*) the auth plugin stores its tables in"},
			"session": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"ttl": map[string]any{"type": "string", "description": "Session lifetime as a Go duration (default 720h)"},
					"cookie": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"name":      map[string]any{"type": "string"},
							"path":      map[string]any{"type": "string"},
							"domain":    map[string]any{"type": "string"},
							"same_site": map[string]any{"type": "string", "enum": []any{"Lax", "Strict", "None"}},
							"secure":    map[string]any{"type": "boolean"},
							"http_only": map[string]any{"type": "boolean"},
						},
					},
				},
			},
			// argon2 + tokens: fill from service.go's parser, same style
		},
		"required":             []any{"database"},
		"additionalProperties": false,
	}
}
```

(`additionalProperties: false` only where the parser ignores unknown keys silently — that's exactly when a typo'd key needs flagging. If a plugin deliberately passes config through (check each parser), use `true`.)

- [ ] **Step 4: Per-plugin sync test** (`plugins/auth/plugin_test.go`):

```go
// TestServiceConfigSchema_RequiredMatchesCreateService pins schema<->code
// agreement: a config missing each schema-required field must fail BOTH
// schema validation and CreateService.
func TestServiceConfigSchema_RequiredMatchesCreateService(t *testing.T) {
	p := &Plugin{}
	schema := p.ServiceConfigSchema()
	required, _ := schema["required"].([]any)
	require.NotEmpty(t, required)
	for _, r := range required {
		field := r.(string)
		cfg := map[string]any{"database": "db"}
		delete(cfg, field)
		_, err := p.CreateService(cfg)
		assert.Error(t, err, "CreateService must reject config missing required %q", field)
	}
}
```

- [ ] **Step 5:** Proceed to Task 2 (build is red until all implementers exist). Then: `go test ./internal/registry/ ./plugins/auth/ -run 'ServiceConfigSchema|Audit' -v` → PASS.
- [ ] **Step 6: Commit**: `feat(api): ServiceConfigSchema on Plugin — auth schema + registry audit (#375 #376)`

### Task 2: implement `ServiceConfigSchema` across every plugin

**Files:**
- Modify: `plugins/db/plugin.go`, `plugins/cache/plugin.go`, `plugins/stream/plugin.go`, `plugins/pubsub/plugin.go`, `plugins/storage/plugin.go`, `plugins/image/plugin.go`, `plugins/http/plugin.go`, `plugins/email/plugin.go`, `plugins/livekit/plugin.go`, and EVERY other `api.Plugin` implementer (find all: `grep -rln "func (p \*Plugin) HasServices" plugins/` plus `grep -rn "api.Plugin = " --include="*.go"` and any test mocks that implement the interface)
- Test: each service plugin's existing `plugin_test.go` gets the same sync test shape as Task 1 Step 4

Rules per plugin — derive from each `CreateService`/parser, structural only:
- **db** (`plugins/db/plugin.go:40-110`): `driver` (enum postgres/sqlite per the code's switch), `url` (postgres), `path` (sqlite), `max_open`/`max_idle` (integer), `conn_lifetime` (duration string). Required: whatever the code errors without — read it; likely none unconditionally (driver-dependent) → use `required: []any{}` and describe the driver-dependence in descriptions (schema stays structural; CreateService remains the arbiter of conditional requirements).
- **cache/stream/pubsub** (via `internal/plugin/redis.go:20-35`): `url` (string, required — NewRedisClient errors without), `pool_size`, `min_idle` (integers).
- **storage** (`plugins/storage/plugin.go:23-31`): `backend` (enum from the code's switch), `path`.
- **image**: read its plugin.go — if `CreateService` reads no keys, schema is `{type: object, properties: {}, additionalProperties: true}`.
- **http** (`plugins/http/plugin.go:33-70`): `timeout` (string or number — use `"type": []any{"string", "number"}`), `base_url`, `headers` (object), `allow_private_networks` (bool), `allowed_hosts` (array of string). Required: none.
- **email** (`plugins/email/plugin.go:30-50`): `host` (required per parser error), `port` (string or number per parsePort), `username`, `password`, `from`, `tls` (bool). Check which produce errors when missing → those are required.
- **livekit** (`plugins/livekit/plugin.go:50-63 + timeout parse`): `url`, `api_key`, `api_secret` all required; `timeout` (duration string) optional.
- **service-less plugins** (all `plugins/core/*`, wasm, and any others): `func (p *Plugin) ServiceConfigSchema() map[string]any { return nil }` — one line each.
- **test mocks** implementing `api.Plugin`: add the nil method.

- [ ] **Step 1:** Enumerate implementers (commands above); add the method to each per the rules; add the sync test to each service plugin's test file (loop over `required`, delete-field, assert both schema-validate failure — reuse Task 3's `compileServiceSchema` helper once it exists, or inline jsonschema — and `CreateService` error; for plugins with empty `required`, assert instead that a fully-empty config passes schema compile and that `CreateService`'s own error behavior is unchanged by this task).
- [ ] **Step 2:** `go build ./... && go vet ./...` → clean (interface satisfied everywhere). `go test ./plugins/... ./internal/registry/ -count=1` → PASS including the Task 1 audit.
- [ ] **Step 3: Commit**: `feat(plugins): ServiceConfigSchema for every plugin (#375)`

### Task 3: dry-run enforcement of service schemas (#376)

**Files:**
- Modify: `internal/registry/validator.go` (ValidateStartupDryRun, :131+)
- Create: `testdata/harness-04-auth-regression/` (minimal reproduction: noda.json with a db service + an auth service whose config LACKS `database`, one trivial workflow/route so the project is otherwise valid)
- Test: `internal/registry/validator_test.go`, `cmd/noda/validate_projects_test.go` (extend)

**Interfaces:**
- Produces: `compileServiceSchema(pluginName string, schema map[string]any) (*jsonschema.Schema, error)` (package-private, cached in a package map guarded like `internal/config/validator.go`'s `schemaCache`, :20,100-136 — copy that compile pattern: marshal the map to JSON, `jsonschema.NewCompiler()`, add resource, compile).

- [ ] **Step 1: Write failing tests** in `internal/registry/validator_test.go` (follow existing ValidateStartupDryRun test setups in that file):

```go
// auth service missing required "database" -> dry-run error naming service+plugin
// auth service with database present -> no service-schema errors
// service with unknown plugin name -> unchanged behavior (crossrefs handle it; dry-run skips)
// service config with a typo'd extra key under additionalProperties:false -> error
// service-less plugin never validated
```

Write them as real table-driven tests constructing `rc.Root["services"]` maps and asserting on returned error strings (`service "auth" (plugin "auth")` substring).

- [ ] **Step 2:** Run → FAIL (no such validation).
- [ ] **Step 3: Implement** — in `ValidateStartupDryRun`, after the configuredServices collection loop (which already resolves plugin names, :135-148), add:

```go
	// Validate each service's config against its plugin's declared schema
	// (#376): an un-bootable service config must fail validation, not boot.
	if servicesMap, ok := rc.Root["services"].(map[string]any); ok {
		for name, raw := range servicesMap {
			cfg, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			pluginName, _ := cfg["plugin"].(string)
			p, found := plugins.GetByName(pluginName)
			if !found {
				continue // unknown plugin is a crossref error already
			}
			schema := p.ServiceConfigSchema()
			if schema == nil {
				continue
			}
			compiled, err := compileServiceSchema(pluginName, schema)
			if err != nil {
				errs = append(errs, fmt.Errorf("service %q (plugin %q): invalid ServiceConfigSchema: %w", name, pluginName, err))
				continue
			}
			svcCfg, _ := cfg["config"].(map[string]any)
			if svcCfg == nil {
				svcCfg = map[string]any{}
			}
			if err := validateAgainst(compiled, svcCfg); err != nil {
				errs = append(errs, fmt.Errorf("service %q (plugin %q): %s", name, pluginName, err))
			}
		}
	}
```

(`validateAgainst` = the same normalize-and-validate helper style `validateNodeConfigSchema` uses at :245+ — reuse its JSON-roundtrip + error-flattening approach; extract shared code if the reuse is clean, don't duplicate 30 lines.)

- [ ] **Step 4:** Run Step 1 tests → PASS.
- [ ] **Step 5: Regression anchor** — create `testdata/harness-04-auth-regression/` and extend `cmd/noda/validate_projects_test.go` with:

```go
// TestValidate_Harness04AuthRegression: the 2026-07-19 harness built a
// project that validated but could not boot (auth service without
// "database"). It must now fail validation (#376).
```

asserting `noda validate`-level validation (the test file's existing pattern) returns an error containing `service "auth"`. ALSO verify every existing `examples/*` and `testdata/*` project still validates (the test file already sweeps projects — run it; fix any project the new validation legitimately flags, in this task).

- [ ] **Step 6:** `go test ./internal/registry/ ./cmd/noda/ -count=1` → PASS. CHANGELOG (`### Changed`): "Service configs are now validated against each plugin's declared schema on every surface (validate/boot/editor/MCP/hot-reload) — was: `valid: true` for configs whose plugin would refuse to boot (#376)."
- [ ] **Step 7: Commit**: `feat(registry): validate service configs against plugin schemas in the dry-run (#376)`

### Task 4: edge-output validation parity (#379)

**Files:**
- Modify: `internal/registry/validator.go`
- Test: `internal/registry/validator_test.go`

**Interfaces:**
- Consumes: `engine.Compile`'s edge semantics at `internal/engine/compiler.go:154-188` — READ IT FIRST; the dry-run check must mirror it exactly (empty output defaults to `"success"`; outputs resolved config-aware). The registry cannot import engine (cycle) — but the outputs source is the node registry itself: find how NodeRegistry exposes config-aware outputs (`grep -n "OutputsForTypeWithConfig" internal/registry/ internal/engine/`) and call that; if the method lives engine-side only, instantiate via `nodes.GetFactory(nodeType)` and call `.Outputs()` on the executor built with the node's config — which is what the engine resolver does underneath.

- [ ] **Step 1: Write failing tests** (table-driven, same file):

```go
// control.if edge with output "true" -> error listing [then else error]  (the #378 bug class)
// control.if edge with output "then" -> no error
// edge with no output key -> treated as "success" -> no error for a [success,error] node
// control.switch with cases ["opened","closed"]: edge output "opened" ok; "openedd" -> error listing cases+default+error
// edge from an unknown node type -> no edge check (existing unknown-type error already fires)
```

- [ ] **Step 2:** Run → FAIL.
- [ ] **Step 3: Implement** — inside the existing per-workflow loop in `ValidateStartupDryRun`, after node checks, iterate `wf["edges"].([]any)`; for each edge map read `from`/`output` (`MapStrVal` equivalent: plain type assertions); skip if the `from` node is missing or its type unregistered (other checks own those); resolve declared outputs config-aware (see Interfaces note); default empty output to `"success"`; on mismatch:

```go
errs = append(errs, fmt.Errorf("workflow %q: edge %q -> %q: output %q not among declared outputs [%s]",
	wfName, from, to, output, strings.Join(outputs, " ")))
```

- [ ] **Step 4:** Run tests → PASS. Then run the FULL example/testdata sweep (`go test ./cmd/noda/ -run Validate -count=1`) — any bundled config now flagged has a genuinely dead edge; fix those configs in this task (expected candidates: none known; the engine already rejected them at boot, so shipped examples should be clean — if one fails, it was never bootable and the fix is part of this issue's value).
- [ ] **Step 5:** CHANGELOG (`### Changed`): "Validation now rejects workflow edges whose `output` names an undeclared node output (boot already did; validate/editor/MCP now agree) (#379)."
- [ ] **Step 6: Commit**: `feat(registry): edge-output validation in the dry-run — validate/boot parity (#379)`

### Task 5: MCP `noda_get_service_schema` (#375)

**Files:**
- Modify: `internal/mcp/tools.go` (tool registration near `noda_get_config_schema`, :47-55; handler nearby following the existing handler style)
- Test: `internal/mcp/tools_test.go` (or the file where other tool handlers are tested — find `getConfigSchemaHandler` tests and sit next to them)

- [ ] **Step 1: Write failing tests**: handler with `{"plugin": "auth"}` returns JSON containing `"database"` and `"required"`; `{"plugin": "livekit"}` contains `"api_secret"`; `{}` (or `"all"`) returns every service-bearing plugin (assert ≥9 entries, each with `name`, `prefix`, `config_schema`); `{"plugin": "nope"}` returns a helpful error listing valid names; `{"plugin": "control"}` (service-less) returns an error saying the plugin has no services.
- [ ] **Step 2:** Run → FAIL.
- [ ] **Step 3: Implement**: register

```go
	s.AddTool(
		mcp.NewTool("noda_get_service_schema",
			mcp.WithDescription("Get the JSON Schema for a plugin's service config block (services.*.config in noda.json). Omit plugin (or pass \"all\") to list every service-bearing plugin with its schema."),
			mcp.WithString("plugin", mcp.Description("Plugin name, e.g. auth, postgres, cache, livekit. Omit or \"all\" for all.")),
		),
		getServiceSchemaHandler,
	)
```

Handler: enumerate registered plugins the same way the audit test does (the MCP server already holds/builds a plugin registry — find how existing handlers reach it), filter `HasServices()`, marshal `{name, prefix, config_schema}` list or the single entry. Update `noda_get_config_schema`'s description string to append: `For services.*.config shapes, use noda_get_service_schema.`

- [ ] **Step 4:** Run tests → PASS; `go test ./internal/mcp/ -count=1`.
- [ ] **Step 5: Commit**: `feat(mcp): noda_get_service_schema exposes plugin service config schemas (#375)`

### Task 6: auth example rewrite (#377, #378)

**Files:**
- Modify: `internal/mcp/examples.go` (auth pattern, :78-180 region)
- Test: `internal/mcp/examples_test.go`

- [ ] **Step 1: Write failing tests** (extend existing):

```go
// no control.if edge anywhere in examples.go uses output "true"/"false"
//   (grep-style test over ALL example strings: unmarshal each workflow, walk edges,
//    assert output values are never "true"/"false")
// the auth example's primary content mentions: services.auth, "plugin": "auth",
//   "database", "noda auth init", and "auth_users"
// the auth example labels the hand-rolled variant: contains "ALTERNATIVE" (or
//   chosen marker) and "incompatible"
```

- [ ] **Step 2:** Run → FAIL (current example is hand-rolled-only with true/false edges at :173-177).
- [ ] **Step 3: Rewrite** the auth example: primary = built-in plugin (auth service config with `database` pointing at the db service; a login route using `auth.verify_credentials` → `auth.create_token`/session per the REAL nodes — read `docs/03-nodes/auth.*.md` for accurate configs and output names (`success`/`invalid`/`not_found` etc.); note in the example text: "Run `noda auth init` to scaffold the flows and `auth_users` migrations this plugin requires"). Alternative = current hand-rolled JWT content, prefixed with the label block: "ALTERNATIVE — hand-rolled JWT with your own users table. Incompatible with the auth plugin's `auth_users` tables: choose one pattern (#377)." Fix every `"output": "true"` → `"then"`, `"false"` → `"else"` in whatever hand-rolled content remains.
- [ ] **Step 4:** Validate embedded example configs are actually valid: if `examples_test.go` already round-trips them through validation, ensure it still passes; if not, add a test that runs each example workflow JSON through `json.Unmarshal` at minimum.
- [ ] **Step 5:** `go test ./internal/mcp/ -count=1` → PASS.
- [ ] **Step 6: Commit**: `fix(mcp): auth example leads with the built-in plugin; then/else edges; pattern-incompatibility warning (#377 #378)`

### Task 7: scaffold secret generation (#381)

**Files:**
- Create: `internal/scaffold/secret.go` + `secret_test.go`
- Modify: `cmd/noda/init.go`, `internal/mcp/tools.go` (:792-795 + `scaffoldEnvExample` :986-995)
- Test: `cmd/noda/init_test.go` (extend), `internal/mcp` scaffold test (find the existing scaffold handler test)

**Interfaces:**
- Produces: `scaffold.GenerateJWTSecret() (string, error)` — 64 hex chars (32 random bytes, crypto/rand).

- [ ] **Step 1: Write failing tests** (`internal/scaffold/secret_test.go`):

```go
func TestGenerateJWTSecret(t *testing.T) {
	a, err := GenerateJWTSecret()
	require.NoError(t, err)
	b, err := GenerateJWTSecret()
	require.NoError(t, err)
	assert.Len(t, a, 64)
	assert.NotEqual(t, a, b, "secrets must be unique per call")
	_, err = hex.DecodeString(a)
	assert.NoError(t, err)
}
```

Plus scaffold-level failing tests: cmd init writes a `.env` whose `JWT_SECRET=` value is 64 hex chars and differs between two inits; `.env.example` contains the literal minimum note (`at least 32 bytes`); MCP scaffold same two properties (extend the existing scaffold handler test that inspects written files).

- [ ] **Step 2:** Run → FAIL.
- [ ] **Step 3: Implement** `internal/scaffold/secret.go`:

```go
// Package scaffold holds helpers shared by the CLI and MCP project scaffolds.
package scaffold

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
)

// GenerateJWTSecret returns a fresh 32-byte hex secret (64 chars) —
// noda's auth.jwt middleware requires >=32 bytes (#381).
func GenerateJWTSecret() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generating jwt secret: %w", err)
	}
	return hex.EncodeToString(b), nil
}
```

Wire-up:
- `cmd/noda/init.go`: after extracting templates, generate a secret and write `.env` from `.env.example` content with the `JWT_SECRET=` line replaced by the generated value (init currently only ships `.env.example` and prints `cp .env.example .env` at :131 — write the real `.env` too and update that hint to say a ready `.env` was generated). Update `cmd/noda/templates/.env.example`'s secret line to `JWT_SECRET=replace-with-at-least-32-bytes` plus a comment line `# auth.jwt requires a secret of at least 32 bytes; noda init generates one in .env`.
- `internal/mcp/tools.go`: at :792-795 the scaffold writes `.env` and `.env.example` from the same const — keep `.env.example` = `scaffoldEnvExample` (with its `JWT_SECRET` line changed to the placeholder + comment, same wording as the template), and build the `.env` content by replacing the placeholder line with the generated secret.
- Run `go test ./cmd/noda/ -run 'AuthFixture|Init' -v`: if `TestAuthFixtureMatchesTemplates` pins any changed bytes, update the fixture per that test's documented regen recipe.

- [ ] **Step 4:** Run all Step 1 tests → PASS. CHANGELOG (`### Changed`): "`noda init` and `noda_scaffold_project` now generate a unique 32-byte `JWT_SECRET` into `.env` — was: a shared 25-byte placeholder that failed auth.jwt's own minimum at boot (#381)."
- [ ] **Step 5: Commit**: `feat(scaffold): generate a compliant per-project JWT secret (#381)`

### Task 8: docs (#374, #375, #377) + assembly

**Files:**
- Modify: `docs/04-guides/authentication.md`, `docs/01-getting-started/services.md`, the expressions doc (locate: `ls docs/01-getting-started/` → the expressions page), `CHANGELOG.md`

- [ ] **Step 1: #374** — in the expressions doc's context-variables section AND `authentication.md`: document `auth.sub` (user id, string), `auth.roles` ([]string), `auth.claims.*` (raw claim map); populated by `auth.jwt` middleware and session middleware on authenticated requests; empty/absent otherwise (guard with `auth.sub ?? ''`). Verify names against `internal/server/trigger.go:134-146` (authMap keys: `sub`, `roles`, `claims`) and the engine context — document exactly what exists, nothing more.
- [ ] **Step 2: #375** — `docs/01-getting-started/services.md`: add an auth-plugin service section/table matching the livekit table style: `database` (required), `session.ttl`, `session.cookie.*`, `argon2.*`, `tokens.*` — same fields as the Task 1 schema, plus one line pointing at `noda_get_service_schema` for the machine-readable version.
- [ ] **Step 3: #377** — `authentication.md`: a "Choosing an auth pattern" section: built-in plugin (auth service + `noda auth init` + `auth_users` tables + scaffolded flows) vs hand-rolled JWT (own `users` table, full control); the two are table-incompatible — pick one; mixing symptoms (queries against the wrong table return empty).
- [ ] **Step 4:** Full verification (Global Constraints commands). Re-run the 04-auth regression test + audit + MCP tests one more time: `go test ./internal/registry/ ./internal/mcp/ ./cmd/noda/ -count=1`.
- [ ] **Step 5:** CHANGELOG `### Added`: "`ServiceConfigSchema` on `api.Plugin` + `noda_get_service_schema` MCP tool — plugin service configs are declared, validated, and discoverable (#374 #375)." (Interface-change note for external plugin authors, PubSubService-entry style.)
- [ ] **Step 6: Commit**: `docs(auth): auth.* expression context, service config reference, pattern guide (#374 #375 #377)`
- [ ] **Step 7: PR**: push `auth-discoverability`; `gh pr create` titled `feat: auth-plugin discoverability + validation — service config schemas on every surface` with body summarizing per-issue fixes and `Fixes #374` … `Fixes #379`, `Fixes #381` (NOT #380/#373) + the standard footer; `gh pr merge --squash --auto`.
