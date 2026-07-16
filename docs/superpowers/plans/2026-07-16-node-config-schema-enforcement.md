# Node ConfigSchema Enforcement (#332) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Every workflow node's `config` payload is validated against its (audited, now-truthful) `ConfigSchema()` at `noda validate` and server boot; all example/testdata projects validate clean. Fixes #332.

**Architecture:** A mini JSON-Schema walker in `internal/registry/configschema.go` implements exactly the vocabulary node schemas use, with one extra rule a generic validator can't express: expression strings (containing `{{`) satisfy any type/enum. A vocabulary-guard test keeps schemas inside that vocabulary. Schemas are audited per plugin package **before** enforcement is wired into `ValidateStartup`/`ValidateStartupDryRun` (`internal/registry/validator.go`), so the branch never has a broken interregnum. See `docs/superpowers/specs/2026-07-16-node-config-schema-enforcement-design.md`.

**Tech Stack:** Go, stdlib only for the validator; testify in tests; existing `registerCorePlugins` (`cmd/noda/main.go:798`) for full-registry tests.

## Global Constraints

- Branch: `feat/node-config-schemas`, worktree `.worktrees/node-config-schemas`, cut from `origin/main` **after `git fetch`** (never local main). If the #331 tranche (`feat/trigger-input-coercion`) merged first, that's fine — the branches don't overlap.
- Before every commit: `gofmt -l .` from the **repo root** prints nothing; `go vet ./...` clean.
- Commit messages end with `Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>`.
- **Audit ground truth, in priority order:** (1) the executor's `Execute` code, (2) `docs/03-nodes/<type>.md` (source-verified 2026-07-15). Where they disagree, the executor wins and the discrepancy is noted in the task report.
- **Audit rules:** `required` = fields the executor errors/misbehaves without (a field the executor defaults is NOT required). Declared types = the literal types the executor accepts (expressions pass automatically — never widen a type to `["integer","string"]` just to admit expressions; plain `"integer"` suffices. Only keep a union where the executor genuinely accepts two literal types). Every config key the executor reads must appear in `properties`. Properties the executor never reads are removed. Nodes accepting open-ended config maps set `"additionalProperties": true`.
- gopls diagnostics inside `.worktrees/` are noise; verify with real `go build`/`go test`.

---

### Task 1: Mini-validator `ValidateNodeConfig` + `CheckSchemaVocabulary`

**Files:**
- Create: `internal/registry/configschema.go`
- Test: `internal/registry/configschema_test.go`

**Interfaces:**
- Produces:
  - `func ValidateNodeConfig(schema map[string]any, config map[string]any) []error` — errors mention the offending field path.
  - `func CheckSchemaVocabulary(schema map[string]any) []error` — rejects unsupported schema keywords.
  - Supported vocabulary: annotations `$schema,title,description,default,examples,deprecated` (ignored) + `type,enum,properties,required,items,oneOf,additionalProperties`.

- [ ] **Step 1: Write the failing tests**

```go
package registry

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func schemaFor(props map[string]any, required ...any) map[string]any {
	return map[string]any{"type": "object", "properties": props, "required": required}
}

func TestValidateNodeConfig(t *testing.T) {
	intProp := map[string]any{"type": "integer"}
	strProp := map[string]any{"type": "string"}

	tests := []struct {
		name    string
		schema  map[string]any
		config  map[string]any
		wantErr string // substring; "" = valid
	}{
		{"missing required", schemaFor(map[string]any{"status": intProp}, "status"),
			map[string]any{}, `missing required config field "status"`},
		{"required satisfied by expression", schemaFor(map[string]any{"status": intProp}, "status"),
			map[string]any{"status": "{{ input.code }}"}, ""},
		{"wrong type", schemaFor(map[string]any{"status": intProp}, "status"),
			map[string]any{"status": "created"}, `"status"`},
		{"integer accepts integral float64", schemaFor(map[string]any{"status": intProp}),
			map[string]any{"status": float64(200)}, ""},
		{"integer rejects fractional", schemaFor(map[string]any{"status": intProp}),
			map[string]any{"status": 2.5}, `"status"`},
		{"union type", schemaFor(map[string]any{"v": map[string]any{"type": []any{"integer", "string"}}}),
			map[string]any{"v": "abc"}, ""},
		{"unknown top-level key", schemaFor(map[string]any{"name": strProp}),
			map[string]any{"nmae": "x"}, `unknown config field "nmae"`},
		{"unknown key allowed with additionalProperties", map[string]any{
			"type": "object", "properties": map[string]any{}, "additionalProperties": true},
			map[string]any{"anything": 1}, ""},
		{"enum violation", schemaFor(map[string]any{"mode": map[string]any{"type": "string", "enum": []any{"a", "b"}}}),
			map[string]any{"mode": "c"}, `"mode"`},
		{"enum satisfied by expression", schemaFor(map[string]any{"mode": map[string]any{"type": "string", "enum": []any{"a", "b"}}}),
			map[string]any{"mode": "{{ input.m }}"}, ""},
		{"items validated", schemaFor(map[string]any{"tags": map[string]any{"type": "array", "items": strProp}}),
			map[string]any{"tags": []any{"ok", 5}}, `tags[1]`},
		{"nested object properties", schemaFor(map[string]any{"opts": map[string]any{
			"type": "object", "properties": map[string]any{"n": intProp}, "required": []any{"n"}}}),
			map[string]any{"opts": map[string]any{}}, `opts`},
		{"nested unknown key tolerated without additionalProperties:false", schemaFor(map[string]any{"opts": map[string]any{
			"type": "object", "properties": map[string]any{"n": intProp}}}),
			map[string]any{"opts": map[string]any{"extra": 1}}, ""},
		{"oneOf any branch", map[string]any{"oneOf": []any{
			schemaFor(map[string]any{"a": strProp}, "a"),
			schemaFor(map[string]any{"b": strProp}, "b"),
		}}, map[string]any{"b": "x"}, ""},
		{"oneOf no branch", map[string]any{"oneOf": []any{
			schemaFor(map[string]any{"a": strProp}, "a"),
			schemaFor(map[string]any{"b": strProp}, "b"),
		}}, map[string]any{"c": "x"}, "does not match"},
		{"empty schema accepts anything", map[string]any{}, map[string]any{"x": 1}, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := ValidateNodeConfig(tt.schema, tt.config)
			if tt.wantErr == "" {
				assert.Empty(t, errs)
				return
			}
			if assert.NotEmpty(t, errs) {
				found := false
				for _, e := range errs {
					if strings.Contains(e.Error(), tt.wantErr) {
						found = true
					}
				}
				assert.True(t, found, "want error containing %q, got %v", tt.wantErr, errs)
			}
		})
	}
}

func TestCheckSchemaVocabulary(t *testing.T) {
	ok := map[string]any{"type": "object", "properties": map[string]any{
		// "pattern" here is a FIELD NAME (key of properties), not a keyword — must pass
		"pattern": map[string]any{"type": "string", "description": "d"},
	}, "required": []any{"pattern"}}
	assert.Empty(t, CheckSchemaVocabulary(ok))

	bad := map[string]any{"type": "object", "properties": map[string]any{
		"n": map[string]any{"type": "integer", "minimum": float64(1)},
	}}
	errs := CheckSchemaVocabulary(bad)
	if assert.NotEmpty(t, errs) {
		assert.Contains(t, errs[0].Error(), "minimum")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/registry/ -run 'TestValidateNodeConfig|TestCheckSchemaVocabulary' -v`
Expected: FAIL — `ValidateNodeConfig` undefined.

- [ ] **Step 3: Implement the validator**

Create `internal/registry/configschema.go`:

```go
package registry

import (
	"fmt"
	"math"
	"reflect"
	"strings"
)

// Node ConfigSchemas are validated with a small purpose-built walker instead
// of a generic JSON Schema library for one reason: an expression string
// ("{{ … }}") must satisfy ANY declared type or enum, because its runtime
// value is unknowable at validation time. CheckSchemaVocabulary keeps schemas
// within the keyword set this walker implements, so the two cannot drift.

// annotationKeywords carry no constraints and are ignored by validation.
var annotationKeywords = map[string]bool{
	"$schema": true, "title": true, "description": true,
	"default": true, "examples": true, "deprecated": true,
}

// constraintKeywords are the schema keywords ValidateNodeConfig implements.
var constraintKeywords = map[string]bool{
	"type": true, "enum": true, "properties": true, "required": true,
	"items": true, "oneOf": true, "additionalProperties": true,
}

// CheckSchemaVocabulary returns an error for every keyword in the schema tree
// that ValidateNodeConfig does not implement. Keys of "properties" maps are
// field names, not keywords.
func CheckSchemaVocabulary(schema map[string]any) []error {
	var errs []error
	checkVocab(schema, "", &errs)
	return errs
}

func checkVocab(schema map[string]any, path string, errs *[]error) {
	for k, v := range schema {
		switch {
		case annotationKeywords[k]:
			// ignore
		case k == "properties":
			if props, ok := v.(map[string]any); ok {
				for name, sub := range props {
					if subMap, ok := sub.(map[string]any); ok {
						checkVocab(subMap, joinPath(path, name), errs)
					}
				}
			}
		case k == "items":
			if subMap, ok := v.(map[string]any); ok {
				checkVocab(subMap, path+"[]", errs)
			}
		case k == "oneOf":
			if branches, ok := v.([]any); ok {
				for i, b := range branches {
					if subMap, ok := b.(map[string]any); ok {
						checkVocab(subMap, fmt.Sprintf("%s(oneOf %d)", path, i), errs)
					}
				}
			}
		case constraintKeywords[k]:
			// implemented, nothing nested to walk
		default:
			*errs = append(*errs, fmt.Errorf("schema keyword %q at %q is not supported by node config validation", k, path))
		}
	}
}

// ValidateNodeConfig checks a node's config payload against its ConfigSchema.
// Expression strings satisfy any type or enum. Unknown keys at the top level
// of the config are errors unless the schema sets "additionalProperties": true;
// nested objects reject unknown keys only with an explicit
// "additionalProperties": false.
func ValidateNodeConfig(schema map[string]any, config map[string]any) []error {
	var errs []error
	validateValue(schema, config, "", true, &errs)
	return errs
}

func validateValue(schema map[string]any, value any, path string, rootStrict bool, errs *[]error) {
	if s, ok := value.(string); ok && strings.Contains(s, "{{") {
		return // expression: runtime type unknowable, satisfies anything
	}

	if branches, ok := schema["oneOf"].([]any); ok {
		for _, b := range branches {
			bm, ok := b.(map[string]any)
			if !ok {
				continue
			}
			var branchErrs []error
			validateValue(bm, value, path, rootStrict, &branchErrs)
			if len(branchErrs) == 0 {
				return
			}
		}
		*errs = append(*errs, fmt.Errorf("config%s does not match any allowed variant", atPath(path)))
		return
	}

	if enum, ok := schema["enum"].([]any); ok {
		matched := false
		for _, member := range enum {
			if looseEqual(value, member) {
				matched = true
				break
			}
		}
		if !matched {
			*errs = append(*errs, fmt.Errorf("config field %q: value %v not in allowed values %v", path, value, enum))
			return
		}
	}

	if !typeAllows(schema["type"], value) {
		*errs = append(*errs, fmt.Errorf("config field %q: expected %s, got %s", path, typeNames(schema["type"]), goTypeName(value)))
		return
	}

	if obj, ok := value.(map[string]any); ok {
		props, _ := schema["properties"].(map[string]any)

		if req, ok := schema["required"].([]any); ok {
			for _, r := range req {
				name, ok := r.(string)
				if !ok {
					continue
				}
				if _, present := obj[name]; !present {
					*errs = append(*errs, fmt.Errorf("missing required config field %q%s", name, atPath(path)))
				}
			}
		}

		strict := false
		switch ap := schema["additionalProperties"].(type) {
		case bool:
			strict = !ap
		default:
			// Unset: strict at the config root (catches typo'd field names),
			// permissive on nested objects.
			strict = rootStrict && path == "" && props != nil
		}

		for name, v := range obj {
			sub, declared := props[name].(map[string]any)
			if !declared {
				if strict {
					*errs = append(*errs, fmt.Errorf("unknown config field %q%s", name, atPath(path)))
				}
				continue
			}
			validateValue(sub, v, joinPath(path, name), false, errs)
		}
	}

	if arr, ok := value.([]any); ok {
		if items, ok := schema["items"].(map[string]any); ok {
			for i, el := range arr {
				validateValue(items, el, fmt.Sprintf("%s[%d]", path, i), false, errs)
			}
		}
	}
}

func typeAllows(declared any, value any) bool {
	switch t := declared.(type) {
	case nil:
		return true
	case string:
		return matchesType(value, t)
	case []any:
		for _, one := range t {
			if s, ok := one.(string); ok && matchesType(value, s) {
				return true
			}
		}
		return false
	default:
		return true
	}
}

func matchesType(v any, t string) bool {
	switch t {
	case "object":
		_, ok := v.(map[string]any)
		return ok
	case "array":
		_, ok := v.([]any)
		return ok
	case "string":
		_, ok := v.(string)
		return ok
	case "boolean":
		_, ok := v.(bool)
		return ok
	case "null":
		return v == nil
	case "number":
		_, ok := toFloat(v)
		return ok
	case "integer":
		f, ok := toFloat(v)
		return ok && f == math.Trunc(f)
	default:
		return false
	}
}

func toFloat(v any) (float64, bool) {
	switch n := v.(type) {
	case int:
		return float64(n), true
	case int32:
		return float64(n), true
	case int64:
		return float64(n), true
	case float32:
		return float64(n), true
	case float64:
		return n, true
	default:
		return 0, false
	}
}

func looseEqual(a, b any) bool {
	if fa, ok := toFloat(a); ok {
		if fb, ok := toFloat(b); ok {
			return fa == fb
		}
		return false
	}
	return reflect.DeepEqual(a, b)
}

func typeNames(declared any) string {
	switch t := declared.(type) {
	case string:
		return t
	case []any:
		parts := make([]string, 0, len(t))
		for _, one := range t {
			if s, ok := one.(string); ok {
				parts = append(parts, s)
			}
		}
		return strings.Join(parts, " or ")
	default:
		return "value"
	}
}

func goTypeName(v any) string {
	switch v.(type) {
	case nil:
		return "null"
	case map[string]any:
		return "object"
	case []any:
		return "array"
	case string:
		return "string"
	case bool:
		return "boolean"
	case int, int32, int64, float32, float64:
		return "number"
	default:
		return fmt.Sprintf("%T", v)
	}
}

func joinPath(path, name string) string {
	if path == "" {
		return name
	}
	return path + "." + name
}

func atPath(path string) string {
	if path == "" {
		return ""
	}
	return fmt.Sprintf(" (in %q)", path)
}
```

- [ ] **Step 4: Run tests**

Run: `go test ./internal/registry/ -count=1`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
gofmt -l . && go vet ./internal/registry/
git add internal/registry/configschema.go internal/registry/configschema_test.go
git commit -m "feat(registry): node ConfigSchema mini-validator with expression-aware typing (#332)"
```

---

### Task 2: Vocabulary guard over every registered node

**Files:**
- Create: `cmd/noda/nodeschema_test.go`

**Interfaces:**
- Consumes: `registry.CheckSchemaVocabulary`, `registerCorePlugins` (`cmd/noda/main.go:798`), `registry.NewNodeRegistry` / `RegisterFromPlugin` / `AllTypes` / `GetDescriptor` (`internal/registry/nodes.go`).
- Produces: `buildFullNodeRegistry(t)` helper (available to later `cmd/noda` tests; Task 13's gate intentionally uses the raw `noda validate` pipeline instead).

- [ ] **Step 1: Write the test**

```go
package main

import (
	"testing"

	"github.com/chimpanze/noda/internal/registry"
	"github.com/stretchr/testify/require"
)

// buildFullNodeRegistry registers every built-in plugin's nodes, exactly as
// server boot and `noda validate` do.
func buildFullNodeRegistry(t *testing.T) *registry.NodeRegistry {
	t.Helper()
	plugins := registry.NewPluginRegistry()
	require.NoError(t, registerCorePlugins(plugins))
	nodes := registry.NewNodeRegistry()
	for _, p := range plugins.All() {
		require.NoError(t, nodes.RegisterFromPlugin(p))
	}
	return nodes
}

// Every node ConfigSchema must stay within the vocabulary the config
// validator implements — otherwise a constraint would be silently ignored.
func TestNodeConfigSchemas_SupportedVocabulary(t *testing.T) {
	nodes := buildFullNodeRegistry(t)
	types := nodes.AllTypes()
	require.NotEmpty(t, types)
	for _, nodeType := range types {
		desc, ok := nodes.GetDescriptor(nodeType)
		require.True(t, ok, nodeType)
		schema := desc.ConfigSchema()
		if schema == nil {
			continue
		}
		for _, err := range registry.CheckSchemaVocabulary(schema) {
			t.Errorf("%s: %v", nodeType, err)
		}
	}
}
```

- [ ] **Step 2: Run it and triage**

Run: `go test ./cmd/noda/ -run TestNodeConfigSchemas_SupportedVocabulary -v`
Expected: PASS (research found only supported keywords in node schemas — `oneOf` in `util.timestamp`; `format`/`pattern` appear only as *field names*). If it fails: either the keyword is trivially replaceable in that node's schema (replace it) or it's load-bearing (extend the validator + vocabulary + a test case in Task 1's table — same commit).

- [ ] **Step 3: Commit**

```bash
gofmt -l .
git add cmd/noda/nodeschema_test.go
git commit -m "test(cmd): vocabulary guard for node ConfigSchemas (#332)"
```

---

### Tasks 3–11: Per-package schema audits

Nine audit tasks, identical procedure, different file sets. **Common procedure for each (referred to below as THE AUDIT PROCEDURE):**

1. For every listed node file: read the descriptor's `ConfigSchema()` and the executor's `Execute` (and factory) in the same file; read `docs/03-nodes/<node-type>.md`.
2. Correct the schema per the Global Constraints audit rules (required = executor-errors-without; literal types only; every read key declared; unread keys removed; open maps get `additionalProperties: true`).
3. Create `plugins/<pkg>/schema_audit_test.go` (package-internal test) with this exact shape — one entry per node in the package:

```go
package <pkg>

import (
	"testing"

	"github.com/chimpanze/noda/internal/registry"
	"github.com/stretchr/testify/assert"
)

func TestConfigSchemasMatchExecutors(t *testing.T) {
	tests := []struct {
		nodeType     string
		schema       map[string]any
		minimalValid map[string]any // smallest config the executor accepts (from docs example)
		emptyValid   bool           // does the executor run with config {}?
		invalid      map[string]any // one config the executor would reject/misuse
	}{
		// one entry per node, e.g.:
		// {"response.json", (&jsonDescriptor{}).ConfigSchema(),
		//  map[string]any{"body": "{{ input }}"}, false,
		//  map[string]any{"status": true, "body": "x"}},
	}
	for _, tt := range tests {
		t.Run(tt.nodeType, func(t *testing.T) {
			assert.Empty(t, registry.CheckSchemaVocabulary(tt.schema))
			assert.Empty(t, registry.ValidateNodeConfig(tt.schema, tt.minimalValid), "minimal valid config must pass")
			emptyErrs := registry.ValidateNodeConfig(tt.schema, map[string]any{})
			if tt.emptyValid {
				assert.Empty(t, emptyErrs, "executor accepts {}, schema must too")
			} else {
				assert.NotEmpty(t, emptyErrs, "executor rejects {}, schema must too")
			}
			assert.NotEmpty(t, registry.ValidateNodeConfig(tt.schema, tt.invalid))
		})
	}
}
```

4. Each task's report lists, per node: fields whose `required`/type changed and why (one line each), citing executor line numbers.
5. Gate per task: `gofmt -l .` empty, `go test ./plugins/<pkgs>/... ./internal/registry/ -count=1` passes. Commit `fix(<pkg>): align node ConfigSchemas with executors (#332)`.

**Known seed finding (Task 5):** `response.json` declares `"required": ["status", "body"]` (`plugins/core/response/json.go:42`) but the executor defaults status to 200 — `required` must become `["body"]` (verify `body` too: check whether the executor errors on absent body or emits null).

### Task 3: Audit control + workflow + event + upload

**Files:** Modify: `plugins/core/control/if.go`, `plugins/core/control/loop.go`, `plugins/core/control/switch.go`, `plugins/core/workflow/output.go`, `plugins/core/workflow/run.go`, `plugins/core/event/emit.go`, `plugins/core/upload/handle.go`. Create: `plugins/core/control/schema_audit_test.go`, `plugins/core/workflow/schema_audit_test.go`, `plugins/core/event/schema_audit_test.go`, `plugins/core/upload/schema_audit_test.go`.

- [ ] Apply THE AUDIT PROCEDURE to the 7 nodes; commit.

### Task 4: Audit transform

**Files:** Modify: `plugins/core/transform/{delete,filter,map,merge,set,validate}.go`. Create: `plugins/core/transform/schema_audit_test.go`.

- [ ] Apply THE AUDIT PROCEDURE to the 6 nodes (note: `transform.set`'s values map is open-ended → `additionalProperties: true` on that sub-object); commit.

### Task 5: Audit response + ws + sse

**Files:** Modify: `plugins/core/response/{error,file,json,redirect}.go`, `plugins/core/ws/send.go`, `plugins/core/sse/send.go`. Create: `plugins/core/response/schema_audit_test.go`, `plugins/core/ws/schema_audit_test.go`, `plugins/core/sse/schema_audit_test.go`.

- [ ] Apply THE AUDIT PROCEDURE to the 6 nodes (includes the seed finding above); commit.

### Task 6: Audit util + wasm + oidc

**Files:** Modify: `plugins/core/util/{delay,jwt,log,timestamp,uuid}.go`, `plugins/core/wasm/{query,send}.go`, `plugins/core/oidc/{auth_url,exchange,refresh}.go`. Create: `plugins/core/util/schema_audit_test.go`, `plugins/core/wasm/schema_audit_test.go`, `plugins/core/oidc/schema_audit_test.go`.

- [ ] Apply THE AUDIT PROCEDURE to the 10 nodes (`util.timestamp` uses `oneOf` — keep it, the validator supports it); commit.

### Task 7: Audit storage + cache

**Files:** Modify: `plugins/core/storage/{delete,list,read,write}.go`, `plugins/cache/{del,exists,get,set}.go`. Create: `plugins/core/storage/schema_audit_test.go`, `plugins/cache/schema_audit_test.go`.

- [ ] Apply THE AUDIT PROCEDURE to the 8 nodes; commit.

### Task 8: Audit db + email

**Files:** Modify: `plugins/db/{count,create,delete,exec,find,find_one,query,update,upsert}.go`, `plugins/email/send.go`. Create: `plugins/db/schema_audit_test.go`, `plugins/email/schema_audit_test.go`.

- [ ] Apply THE AUDIT PROCEDURE to the 10 nodes (db nodes take open filter/values maps — mark those sub-objects `additionalProperties: true`); commit.

### Task 9: Audit http + image

**Files:** Modify: `plugins/http/{get,post,request}.go`, `plugins/image/{convert,crop,resize,thumbnail,watermark}.go`. Create: `plugins/http/schema_audit_test.go`, `plugins/image/schema_audit_test.go`.

- [ ] Apply THE AUDIT PROCEDURE to the 8 nodes (image package may need a build-tag check: if its tests only build with libvips, put the audit test in the same build-tag scope as the nodes); commit.

### Task 10: Audit auth

**Files:** Modify: `plugins/auth/{create_session,create_user,get_user,one_time_tokens,revoke_session,set_password,verify_credentials}.go`. Create: `plugins/auth/schema_audit_test.go`.

- [ ] Apply THE AUDIT PROCEDURE to the 7 node files (`one_time_tokens.go` may define more than one node type — one table entry per registered node type, not per file); commit.

### Task 11: Audit livekit

**Files:** Modify: `plugins/livekit/{egress_list,egress_start_room_composite,egress_start_track,egress_stop,ingress_create,ingress_delete,ingress_list,mute_track,participant_get,participant_list,participant_remove,participant_update,room_create,room_delete,room_list,room_update_metadata,send_data,token}.go`. Create: `plugins/livekit/schema_audit_test.go`.

- [ ] Apply THE AUDIT PROCEDURE to the 18-19 nodes; commit.

---

### Task 12: Wire enforcement into startup validation

**Files:**
- Modify: `internal/registry/validator.go` (both `ValidateStartup` and `ValidateStartupDryRun`)
- Test: `internal/registry/validator_test.go`

**Interfaces:**
- Consumes: `ValidateNodeConfig` (Task 1); descriptor lookup already present in both walks.
- Produces: validation errors of the form `workflow %q, node %q (%s): <ValidateNodeConfig error>`.

- [ ] **Step 1: Write the failing tests**

In `internal/registry/validator_test.go`, following the file's existing fixture idiom (it already builds `*config.ResolvedConfig` maps and a test plugin — see `testplugin_test.go`), add two cases:

```go
func TestValidateStartupDryRun_NodeConfigSchemaEnforced(t *testing.T) {
	// Build rc with one workflow whose node has a config violating the test
	// plugin's ConfigSchema (missing a required field), using the same
	// helpers the surrounding tests use. Assert one error containing
	// `missing required config field` and the workflow/node names.
}

func TestValidateStartupDryRun_MissingConfigTreatedAsEmpty(t *testing.T) {
	// Node with NO "config" key at all, schema with a required field →
	// expect the same missing-required error.
}
```

Give the test plugin's descriptor a schema with one required field if it doesn't have one (check `testplugin_test.go`; extend it there if needed — its executor must actually require the field, keeping the audit invariant true even for test plugins).

- [ ] **Step 2: Run to verify they fail**

Run: `go test ./internal/registry/ -run 'NodeConfigSchema|MissingConfigTreated' -v`
Expected: FAIL — no schema errors produced yet.

- [ ] **Step 3: Implement**

In `internal/registry/validator.go`, add a shared helper and call it from **both** functions' node walks, immediately after the `desc, found := nodes.GetDescriptor(nodeType)` guard succeeds (step "2." in each):

```go
// validateNodeConfigSchema checks the node's config payload against the
// descriptor's ConfigSchema. A node without a config key validates as {}
// so required-field violations surface.
func validateNodeConfigSchema(wfName, nodeID, nodeType string, desc api.NodeDescriptor, node map[string]any) []error {
	schema := desc.ConfigSchema()
	if schema == nil {
		return nil
	}
	cfg, _ := node["config"].(map[string]any)
	if cfg == nil {
		cfg = map[string]any{}
	}
	var errs []error
	for _, scErr := range ValidateNodeConfig(schema, cfg) {
		errs = append(errs, fmt.Errorf("workflow %q, node %q (%s): %w", wfName, nodeID, nodeType, scErr))
	}
	return errs
}
```

(Add the `pkg/api` import.) Call sites in both walks:

```go
				errs = append(errs, validateNodeConfigSchema(wfName, nodeID, nodeType, desc, node)...)
```

- [ ] **Step 4: Run the registry tests, then the whole repo**

Run: `go test ./internal/registry/ -count=1` → PASS.
Run: `go build ./... && go test ./... -count=1`
Expected: possible fallout in packages whose test fixtures use invalid node configs (engine, server, testing, devmode, `testdata/*` fixtures). For each failure: the fixture config violates a now-audited schema → **fix the fixture** (it encodes the bug class #332 exists to catch). If instead a *schema* is wrong (executor genuinely accepts the fixture), fix the schema + its `schema_audit_test.go` entry. Record every fixture change in the commit message.

- [ ] **Step 5: Commit**

```bash
gofmt -l .
git add internal/registry/ <changed fixtures/tests>
git commit -m "feat(registry): enforce node ConfigSchemas at startup validation (#332)"
```

---

### Task 13: Examples + testdata gate

**Files:**
- Create: `cmd/noda/validate_projects_test.go`

**Interfaces:**
- Consumes: `config.NewSecretsManager`, `config.ValidateAll`, `registry.NewPluginRegistry`, `registerCorePlugins`, `registry.Bootstrap` with `BootstrapOptions{DryRun: true}` — the exact `noda validate` pipeline (`cmd/noda/main.go:128-190`).

- [ ] **Step 1: Write the gate test**

```go
package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/chimpanze/noda/internal/config"
	"github.com/chimpanze/noda/internal/registry"
	"github.com/stretchr/testify/require"
)

// Every shipped example and full-project fixture must pass the exact
// pipeline `noda validate` runs — including node ConfigSchema enforcement.
func TestShippedProjectsValidate(t *testing.T) {
	exampleDirs, err := filepath.Glob("../../examples/*")
	require.NoError(t, err)
	dirs := append(exampleDirs,
		"../../testdata/auth",
		"../../testdata/valid-project",
		"../../testdata/node-e2e",
		"../../testdata/livekit-example",
		"../../testdata/minimal-project",
	)

	for _, dir := range dirs {
		if _, err := os.Stat(filepath.Join(dir, "noda.json")); err != nil {
			continue
		}
		t.Run(filepath.Base(dir), func(t *testing.T) {
			sm, err := config.NewSecretsManager(dir, "")
			require.NoError(t, err)
			rc, errs := config.ValidateAll(dir, "", sm)
			require.Empty(t, errs)

			plugins := registry.NewPluginRegistry()
			require.NoError(t, registerCorePlugins(plugins))
			_, bootErrs := registry.Bootstrap(context.Background(), rc, plugins,
				registry.BootstrapOptions{DryRun: true})
			require.Empty(t, bootErrs)
		})
	}
}

func TestInvalidProjectStillFails(t *testing.T) {
	dir := "../../testdata/invalid-project"
	sm, err := config.NewSecretsManager(dir, "")
	require.NoError(t, err)
	_, errs := config.ValidateAll(dir, "", sm)
	require.NotEmpty(t, errs, "invalid-project must keep failing validation")
}
```

- [ ] **Step 2: Run and triage failures**

Run: `go test ./cmd/noda/ -run 'TestShippedProjectsValidate|TestInvalidProjectStillFails' -v`
Triage each subtest failure into exactly one bucket:
1. **Unresolved `$env()`** — pre-existing env requirement, not a schema finding: add the variable with a dummy value via `t.Setenv` in a per-dir map inside the test (document each with a comment naming the example).
2. **Node config schema violation in an example** — the example is wrong (#332's bug class): fix the example's JSON config to satisfy the executor, matching the corresponding docs walkthrough in `docs/05-examples/`.
3. **Schema violation where the executor genuinely accepts the config** — the audit missed something: fix the schema + the package's `schema_audit_test.go`.

- [ ] **Step 3: Run the full suite once green**

Run: `go test ./... -count=1` → PASS. `gofmt -l .` → empty.

- [ ] **Step 4: Commit**

```bash
git add cmd/noda/validate_projects_test.go <changed example configs>
git commit -m "test(cmd): gate shipped examples/fixtures through validate pipeline (#332)"
```

---

### Task 14: Docs, CHANGELOG, PR

**Files:**
- Modify: `docs/02-config/workflows.md` (node section: state that `config` is validated against the node's schema; expressions satisfy any type; unknown top-level fields rejected)
- Modify: `docs/02-config/overview.md` (if it describes what `noda validate` checks, add node-config schemas to the list; skip if it doesn't)
- Modify: `CHANGELOG.md` `[Unreleased]`

- [ ] **Step 1: Write docs + CHANGELOG**

CHANGELOG entries (merge into existing subsections if present):

```markdown
### Changed
- `noda validate` and server startup now validate every workflow node's `config` against the node's ConfigSchema: missing required fields, wrong types, and unknown top-level fields are errors. Expression values (`{{ … }}`) satisfy any declared type (#332).
- Node ConfigSchemas audited against executor behavior across all plugins; `required` lists and types now reflect what executors actually accept (improves editor forms and MCP guidance).
```

- [ ] **Step 2: Final verification**

Run: `go build ./... && go test ./... -count=1 && gofmt -l .` (empty output for gofmt), plus `golangci-lint run ./...` if installed.

- [ ] **Step 3: Commit records and open PR**

```bash
git add docs/02-config/ CHANGELOG.md
git commit -m "docs: node config validation semantics (#332)"
git add -f docs/superpowers/specs/2026-07-16-node-config-schema-enforcement-design.md docs/superpowers/plans/2026-07-16-node-config-schema-enforcement.md
git commit -m "docs(superpowers): node-config-schema-enforcement spec + plan records"
git push -u origin feat/node-config-schemas
gh pr create --title "feat: audit + enforce node ConfigSchemas at validation time" --body "Fixes #332. ..."
```

PR body must include: the audit-rule summary, per-package audit reports (condensed), the behavior-changes list, and end with `🤖 Generated with [Claude Code](https://claude.com/claude-code)`.
