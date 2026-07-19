# Auth-Plugin Discoverability + Validation Tranche — Design

**Date:** 2026-07-19
**Scope:** 7 issues from the 2026-07-19 AI-usability harness re-run: #374, #375, #376 (centerpiece), #377, #378, #379, #381. (#373 `$ref` resolution and #380 connections docs are explicitly out — separate follow-ups.)
**Delivery:** one PR, branch `auth-discoverability` off main.

## Problem

The harness's 04-auth builder produced a project that `noda_validate_config` called `valid: true` but that cannot boot: the auth service's required `database` field is invisible to every part of the surface — no schema declares it, no doc lists it, no validator checks it. Five sibling gaps share the root cause (the auth plugin's contract lives only inside its `CreateService` code) or were enabled by missing validation (the bundled example's `true`/`false` edges that nothing flags).

## Decisions (user-approved)

1. **`ServiceConfigSchema() map[string]any` added to `api.Plugin`** (breaking pre-1.0 interface change, same precedent as the `PubSubService.Subscribe` widening) — not a hardcoded validation table.
2. **#379 rides along** (edge-output validation); #380 does not.
3. **Scaffolds generate a random 32-byte secret per project** — not a compliant static placeholder.
4. **`noda_get_examples('auth')` leads with the built-in auth plugin**, keeping the hand-rolled JWT variant as a clearly-labeled, schema-incompatible alternative.

## Components

### 1. Plugin contract (#375, #376 foundation)

`pkg/api/plugin.go`:

```go
type Plugin interface {
	Name() string
	Prefix() string
	Nodes() []NodeRegistration
	HasServices() bool
	// ServiceConfigSchema returns a JSON Schema (as a Go map, same
	// conventions as NodeDescriptor.ConfigSchema) describing this plugin's
	// service `config` block. Plugins without services return nil.
	ServiceConfigSchema() map[string]any
	CreateService(config map[string]any) (any, error)
	HealthCheck(service any) error
	Shutdown(service any) error
}
```

Every in-repo plugin implements it, **derived from what its `CreateService` actually parses** (auth: required `database` + optional `session.ttl`, `session.cookie.*`, argon/token-TTL/template fields; livekit: required `url`/`api_key`/`api_secret` + optional `timeout`; db/cache/stream/pubsub: required `url` + their option fields; storage/image/http/email per their parsers; core node plugins and other service-less plugins return nil). Schemas are **structural only** (required keys, types, enums, descriptions) — no value-content constraints (`minLength` etc.), because values are frequently `$env()`-resolved and may be legitimately empty at validate time.

Guardrails:
- **Audit test** in `internal/registry` (mirror of the #338 all-81 node ConfigSchema audit): every registered plugin with `HasServices() == true` must return a non-nil `type: "object"` schema; service-less plugins must return nil.
- **Per-plugin sync tests**: for each schema-required field, a config missing that field must fail BOTH schema validation and `CreateService` — pinning schema↔code agreement in each direction.

### 2. Dry-run enforcement (#376)

`registry.ValidateStartupDryRun` gains a step: for every `services.*` entry whose `plugin` resolves in the plugin registry, compile the plugin's `ServiceConfigSchema` (santhosh-tekuri/jsonschema, existing dep; compile once per plugin, cached) and validate the entry's `config` against it. Errors read `service "auth" (plugin "auth"): missing property 'database'`.

Because boot, `noda validate`, the editor endpoints, MCP `noda_validate_config`, and dev-mode hot reload (via `SetDryRun`, #349) all share `ValidateStartupDryRun`, the fix lands on every surface at once. Unknown plugin names are already crossref errors — unchanged.

**Regression anchor:** a testdata project reproducing the harness's 04-auth config (auth service without `database`) must go from `valid: true` to rejected on all surfaces (asserted at least for validate + MCP).

### 3. MCP exposure (#375)

New tool `noda_get_service_schema` in `internal/mcp`:
- input `plugin` (string, optional): a plugin name, or omitted/`all` for every service-bearing plugin;
- output: `[{name, prefix, config_schema}]`.
`noda_get_config_schema`'s root description gains a pointer: "for `services.*.config` shapes, use `noda_get_service_schema`".

### 4. Edge-output validation (#379)

`ValidateStartupDryRun`'s workflow loop additionally validates every edge: the edge's `output` (default `success` when absent — match engine behavior) must be among the source node's declared outputs, obtained by instantiating the node's executor via its registered factory with the node's actual config (factories are cheap; this is how dispatch works) and calling `Outputs()`. Node types whose outgoing ports cannot be statically enumerated are exempted via an explicit, commented list (at minimum `workflow.output`, which is terminal; the plan finalizes the list by checking every `Outputs()` implementation). Error: `workflow "w", edge "check" → "grant": output "true" not among declared outputs [then else error]`.

Behavior change: configs with bogus edge outputs used to validate and boot (the edge just never fired); they now fail validation → CHANGELOG entry.

### 5. Auth example rewrite (#377, #378)

`internal/mcp/examples.go` auth pattern:
- **Primary**: built-in auth plugin — `services.auth` (`plugin: "auth"`, `config.database` naming the db service), a note that `noda auth init` scaffolds the flows and the `auth_users` migrations the plugin's nodes require, session middleware usage, `auth.*` nodes.
- **Alternative** (kept, explicitly labeled): hand-rolled JWT with its own `users` table, with a warning that it is schema-incompatible with the plugin's `auth_users` tables — pick one pattern (#377).
- All `control.if` edges use `then`/`else` (#378); `internal/mcp/examples_test.go` updated and extended to assert no `"output": "true"|"false"` remains.

### 6. Scaffold secret (#381)

A shared helper (crypto/rand, 64 hex chars = 32 bytes) used by both scaffold paths:
- `cmd/noda init`: writes `.env` with the generated secret; `.env.example` keeps a placeholder plus a comment stating noda's ≥32-byte minimum.
- MCP `noda_scaffold_project` (tools.go): same behavior.
Tests: generated secret ≥64 hex chars, two scaffolds produce different secrets, `.env.example` documents the minimum. The `cmd/noda` auth fixture drift guard is checked/updated if it pins template bytes.

### 7. Docs (#374, #375, #377)

- **#374**: document the `auth.*` expression context — `auth.sub`, `auth.roles`, `auth.claims.*`, where it's populated (JWT/session middleware) and where it's empty — in the expressions doc and the auth guide.
- **#375**: auth service config fields documented in the services reference (same table style as the livekit `timeout` row added in #371).
- **#377**: a "choosing an auth pattern" note in the auth guide (built-in plugin vs hand-rolled; incompatible tables; when to use which).
`noda://` resources serve from `docs/`, so MCP inherits every docs fix.

## Testing summary

Unit: plugin schema audit + per-plugin sync tests; dry-run service-schema validation (missing required, wrong type, unknown-plugin unchanged, service-less plugin ignored); edge-output validation (static outputs, control.switch dynamic cases, exempt list, default-output edges); MCP tool test; scaffold secret tests; examples test (no true/false edges, primary pattern is the plugin).
Integration: the 04-auth regression anchor; `noda validate` on all `examples/` and `testdata/` projects stays green (any config the new validators flag gets fixed in the same PR — expected: none for service schemas, possibly some for edge outputs).
CHANGELOG: interface change (`ServiceConfigSchema`), two validation behavior changes (#376, #379), scaffold secret change (#381).
