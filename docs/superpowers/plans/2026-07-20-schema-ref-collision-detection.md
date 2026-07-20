# Schema `$ref` Name Collision Detection Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make schema `$ref` names an unambiguous naming authority — collisions and undecidable file classifications fail loudly at validate time instead of binding to a nondeterministic winner — and expose the resulting registry so the OpenAPI generator unification can build on it.

**Architecture:** All work is inside the leaf package `internal/config`, almost entirely in `internal/config/refs.go`. `buildSchemaRegistry` gains per-ref-name source tracking and becomes exported; `isBareSchema` is replaced by a three-way shape-aware classifier; `ResolveRefs` returns `[]ValidationError` instead of `[]error` and stashes the registry on `RawConfig` so `ValidateAll` can surface it on `ResolvedConfig`.

**Tech Stack:** Go, `testify` (`assert`/`require`) — the existing convention in `internal/config/*_test.go`.

**Spec:** `docs/superpowers/specs/2026-07-20-schema-ref-collision-detection.md`
**Issue:** #405

## Global Constraints

- Ref name format is unchanged: `<reldir>/<key>`, e.g. `schemas/User`, `schemas/validation/Task`. Do not change `extractSchemasRelPath`.
- Every error this plan adds must be **deterministic in its text**. Anything derived from a Go map must be sorted before formatting. A nondeterministic message for a nondeterminism bug is its own defect.
- No shipped config may start failing. `examples/`, `testdata/`, and `projects/` contain 20 schema files with 0 collisions and 0 ambiguous classifications — verified. Task 5 re-checks this.
- Use `ValidationError` from `internal/config/validator.go:24` (fields `FilePath`, `JSONPath`, `Message`, `SchemaPath`). Do not introduce a new error type.
- Do not add `JSONPath` to unresolved-ref or circular-ref errors — `resolveRefsInValue` threads no JSON pointer, and adding one is out of scope.
- Run `go build ./... && go vet ./...` before every commit.

---

### Task 1: Convert `ResolveRefs` to return `[]ValidationError`

Pure refactor, no detection logic yet. This exists as its own task because it touches the pipeline signature and a reviewer could reasonably approve it while rejecting the detection design that follows.

**Files:**
- Modify: `internal/config/refs.go:13-39` (`ResolveRefs`), `:92-126` (`resolveRefsInValue`), `:128-162` (`resolveRef`)
- Modify: `internal/config/pipeline.go:88-95`
- Test: `internal/config/refs_test.go`

**Interfaces:**
- Consumes: nothing from earlier tasks.
- Produces: `func ResolveRefs(rc *RawConfig) []ValidationError`. Internal helpers become `func resolveRefsInValue(v any, registry map[string]map[string]any, filePath string, seen []string) (any, []ValidationError)` and `func resolveRef(refName string, registry map[string]map[string]any, filePath string, seen []string) (any, []ValidationError)`. Task 2 and Task 3 append to these same `[]ValidationError` slices.

- [ ] **Step 1: Write the failing test**

Add to `internal/config/refs_test.go`:

```go
func TestResolveRefs_ErrorCarriesFilePath(t *testing.T) {
	rc := &RawConfig{
		Schemas: map[string]map[string]any{},
		Routes: map[string]map[string]any{
			"routes/test.json": {
				"response": map[string]any{"$ref": "schemas/Missing"},
			},
		},
		Workflows:   map[string]map[string]any{},
		Workers:     map[string]map[string]any{},
		Schedules:   map[string]map[string]any{},
		Connections: map[string]map[string]any{},
		Tests:       map[string]map[string]any{},
		Models:      map[string]map[string]any{},
	}

	errs := ResolveRefs(rc)
	require.Len(t, errs, 1)
	assert.Equal(t, "routes/test.json", errs[0].FilePath)
	assert.Contains(t, errs[0].Message, `unresolved $ref "schemas/Missing"`)
	// The path lives in FilePath now, not doubled into the message.
	assert.NotContains(t, errs[0].Message, "routes/test.json")
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/config/ -run TestResolveRefs_ErrorCarriesFilePath -v`
Expected: compile failure — `errs[0].FilePath undefined (type error has no field or method FilePath)`.

- [ ] **Step 3: Change the three signatures and the error construction**

In `internal/config/refs.go`, change `ResolveRefs`'s declaration and error variable:

```go
func ResolveRefs(rc *RawConfig) []ValidationError {
	// Build schema registry
	registry := buildSchemaRegistry(rc.Schemas)

	var errs []ValidationError
```

The body below it is otherwise unchanged — `errs = append(errs, refErrs...)` still type-checks.

Change `resolveRefsInValue`'s declaration and its three local `var errs []error` declarations to `var errs []ValidationError`:

```go
func resolveRefsInValue(v any, registry map[string]map[string]any, filePath string, seen []string) (any, []ValidationError) {
```

Change `resolveRef` to build `ValidationError` values. Replace the circular-reference return:

```go
			return nil, []ValidationError{{
				FilePath: filePath,
				Message:  fmt.Sprintf("circular $ref detected: %s", strings.Join(cycle, " → ")),
			}}
```

and the unresolved-ref return:

```go
		return nil, []ValidationError{{
			FilePath: filePath,
			Message: fmt.Sprintf("unresolved $ref %q (known refs: %s — a schemas/ file maps each top-level key to a schema, registered as schemas/<Key>; a file that is itself a JSON Schema registers as schemas/<filename without .json>)",
				refName, knownList),
		}}
```

together with its declaration:

```go
func resolveRef(refName string, registry map[string]map[string]any, filePath string, seen []string) (any, []ValidationError) {
```

- [ ] **Step 4: Update the pipeline call site**

In `internal/config/pipeline.go`, replace lines 87-95 with:

```go
	// 6. Resolve $ref
	refErrs := ResolveRefs(raw)
	if len(refErrs) > 0 {
		return nil, refErrs
	}
```

- [ ] **Step 5: Run the full package suite**

Run: `go build ./... && go vet ./... && go test ./internal/config/ -count=1`
Expected: PASS. The 12 existing `ResolveRefs` call sites in `refs_test.go` and the one in `benchmark_test.go` compile unchanged; `assert.Empty` accepts any slice and `errs[0].Error()` is valid on an addressable slice element despite the pointer receiver on `ValidationError.Error`.

If `TestResolveRefs_UnresolvedRef` (around `refs_test.go:157-160`) fails on `assert.Contains(t, errs[0].Error(), "routes/test.json")`, that is a real signal to read, not to patch: `ValidationError.Error()` prefixes `FilePath`, so it should still pass. Investigate before changing the assertion.

- [ ] **Step 6: Commit**

```bash
git add internal/config/refs.go internal/config/pipeline.go internal/config/refs_test.go
git commit -m "refactor(config): ResolveRefs returns ValidationError with FilePath

\$ref errors were wrapped into ValidationError with only Message set, so
the file path was dropped and Error() rendered a leading \": \". Return
typed errors from ResolveRefs instead and drop the now-redundant
\"in <path>\" message suffixes.

Refs #405

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

### Task 2: Detect ref-name collisions

**Files:**
- Modify: `internal/config/refs.go:41-65` (`buildSchemaRegistry`)
- Test: `internal/config/refs_test.go`

**Interfaces:**
- Consumes: `[]ValidationError` from Task 1.
- Produces: `func buildSchemaRegistry(schemas map[string]map[string]any) (map[string]map[string]any, []ValidationError)` and the unexported type `schemaSource{FilePath, Key string}` (empty `Key` means a bare-schema file). Task 3 adds a second error source to the same return slice; Task 4 exports this function.

- [ ] **Step 1: Write the failing tests**

Add to `internal/config/refs_test.go`. The repeat loops are the point — a single build could coincidentally pick either winner, so only "errors on every one of N builds" distinguishes detection from luck.

```go
func TestBuildSchemaRegistry_CollisionKeyedVsKeyed(t *testing.T) {
	schemas := map[string]map[string]any{
		"/p/schemas/a.json": {"User": map[string]any{"marker": "FROM_A"}},
		"/p/schemas/b.json": {"User": map[string]any{"marker": "FROM_B"}},
	}

	// Map iteration is randomized; the collision must be reported on every build.
	var first string
	for i := 0; i < 200; i++ {
		_, errs := buildSchemaRegistry(schemas)
		require.Len(t, errs, 1, "collision must be detected on build %d", i)
		if i == 0 {
			first = errs[0].Error()
			continue
		}
		assert.Equal(t, first, errs[0].Error(), "error text must be deterministic (build %d)", i)
	}

	assert.Contains(t, first, `"schemas/User"`)
	assert.Contains(t, first, "/p/schemas/a.json")
	assert.Contains(t, first, "/p/schemas/b.json")
}

func TestBuildSchemaRegistry_CollisionBareVsKeyed(t *testing.T) {
	schemas := map[string]map[string]any{
		"/p/schemas/User.json":  {"type": "object"},
		"/p/schemas/other.json": {"User": map[string]any{"marker": "KEYED"}},
	}

	for i := 0; i < 200; i++ {
		_, errs := buildSchemaRegistry(schemas)
		require.Len(t, errs, 1, "collision must be detected on build %d", i)
	}

	_, errs := buildSchemaRegistry(schemas)
	assert.Contains(t, errs[0].Error(), "/p/schemas/User.json (whole file)")
	assert.Contains(t, errs[0].Error(), `/p/schemas/other.json (key "User")`)
}

func TestBuildSchemaRegistry_NoCollisionAcrossDirectories(t *testing.T) {
	registry, errs := buildSchemaRegistry(map[string]map[string]any{
		"/p/schemas/billing/Invoice.json": {"Invoice": map[string]any{"marker": "billing"}},
		"/p/schemas/orders/Invoice.json":  {"Invoice": map[string]any{"marker": "orders"}},
	})

	assert.Empty(t, errs)
	assert.Equal(t, "billing", registry["schemas/billing/Invoice"]["marker"])
	assert.Equal(t, "orders", registry["schemas/orders/Invoice"]["marker"])
}

func TestResolveRefs_ReportsCollision(t *testing.T) {
	rc := &RawConfig{
		Schemas: map[string]map[string]any{
			"/p/schemas/a.json": {"User": map[string]any{"type": "object"}},
			"/p/schemas/b.json": {"User": map[string]any{"type": "string"}},
		},
		Routes:      map[string]map[string]any{},
		Workflows:   map[string]map[string]any{},
		Workers:     map[string]map[string]any{},
		Schedules:   map[string]map[string]any{},
		Connections: map[string]map[string]any{},
		Tests:       map[string]map[string]any{},
		Models:      map[string]map[string]any{},
	}

	errs := ResolveRefs(rc)
	require.Len(t, errs, 1)
	assert.Equal(t, "/p/schemas/a.json", errs[0].FilePath)
	assert.Equal(t, "/User", errs[0].JSONPath)
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/config/ -run 'TestBuildSchemaRegistry_|TestResolveRefs_ReportsCollision' -v`
Expected: compile failure — `assignment mismatch: 2 variables but buildSchemaRegistry returns 1 value`.

- [ ] **Step 3: Implement source tracking and collision reporting**

In `internal/config/refs.go`, replace `buildSchemaRegistry` (lines 41-65) with:

```go
// schemaSource records which file contributed a ref name, and how. An empty
// Key means the file is itself a JSON Schema document and registered whole.
type schemaSource struct {
	FilePath string
	Key      string
}

func (s schemaSource) describe() string {
	if s.Key == "" {
		return s.FilePath + " (whole file)"
	}
	return fmt.Sprintf("%s (key %q)", s.FilePath, s.Key)
}

func buildSchemaRegistry(schemas map[string]map[string]any) (map[string]map[string]any, []ValidationError) {
	registry := make(map[string]map[string]any)
	sources := make(map[string][]schemaSource)

	for filePath, content := range schemas {
		relDir := extractSchemasRelPath(filePath)

		// A file that is itself a JSON Schema document registers whole
		// under schemas/<filename-without-extension> (#373); otherwise
		// each top-level key is a named schema definition.
		if isBareSchema(content) {
			base := strings.TrimSuffix(filepath.Base(filePath), filepath.Ext(filePath))
			refName := relDir + "/" + base
			registry[refName] = content
			sources[refName] = append(sources[refName], schemaSource{FilePath: filePath})
			continue
		}

		for key, val := range content {
			if schema, ok := val.(map[string]any); ok {
				refName := relDir + "/" + key
				registry[refName] = schema
				sources[refName] = append(sources[refName], schemaSource{FilePath: filePath, Key: key})
			}
		}
	}

	return registry, collisionErrors(sources)
}

// collisionErrors reports every ref name claimed by more than one source (#405).
// Without this the registry silently keeps whichever definition Go's randomized
// map iteration happened to write last, so the same config can validate against
// a different schema on the next boot.
//
// Everything here is sorted: the input is derived from a map, and a
// nondeterministic message would defeat the purpose.
func collisionErrors(sources map[string][]schemaSource) []ValidationError {
	names := make([]string, 0, len(sources))
	for name, srcs := range sources {
		if len(srcs) > 1 {
			names = append(names, name)
		}
	}
	sort.Strings(names)

	var errs []ValidationError
	for _, name := range names {
		srcs := sources[name]
		sort.Slice(srcs, func(i, j int) bool {
			if srcs[i].FilePath != srcs[j].FilePath {
				return srcs[i].FilePath < srcs[j].FilePath
			}
			return srcs[i].Key < srcs[j].Key
		})

		described := make([]string, len(srcs))
		for i, s := range srcs {
			described[i] = s.describe()
		}

		jsonPath := ""
		if srcs[0].Key != "" {
			jsonPath = "/" + srcs[0].Key
		}

		errs = append(errs, ValidationError{
			FilePath: srcs[0].FilePath,
			JSONPath: jsonPath,
			Message: fmt.Sprintf(
				"duplicate schema ref %q defined %d times: %s — ref names must be unique; rename one definition, or move one file into a schemas/ subdirectory (the directory is part of the ref name)",
				name, len(srcs), strings.Join(described, ", ")),
		})
	}

	return errs
}
```

`sort` and `fmt` are already imported by this file.

- [ ] **Step 4: Update the `ResolveRefs` call site**

In `internal/config/refs.go`, change the first two statements of `ResolveRefs`:

```go
	// Build schema registry
	registry, errs := buildSchemaRegistry(rc.Schemas)
```

and delete the now-duplicate `var errs []ValidationError` declaration below it.

- [ ] **Step 5: Run the tests to verify they pass**

Run: `go build ./... && go vet ./... && go test ./internal/config/ -count=1`
Expected: PASS, all tests.

- [ ] **Step 6: Commit**

```bash
git add internal/config/refs.go internal/config/refs_test.go
git commit -m "fix(config): reject duplicate schema \$ref names

buildSchemaRegistry had no duplicate detection, so two definitions
mapping to the same ref name silently collapsed to one — and Go's
randomized map iteration picked a different winner between runs of the
same config, meaning a route could validate against a different schema
on each boot. Track contributing sources per ref name and fail
validation instead, with deterministically sorted error text.

Fixes #405

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

### Task 3: Shape-aware bare-schema classification

**Files:**
- Modify: `internal/config/refs.go:67-77` (`isBareSchema`) and `buildSchemaRegistry`
- Test: `internal/config/refs_test.go`

**Interfaces:**
- Consumes: `buildSchemaRegistry`'s `(registry, []ValidationError)` shape from Task 2.
- Produces: `func classifySchemaFile(content map[string]any) schemaFileKind` returning one of the constants `schemaFileBare`, `schemaFileKeyed`, `schemaFileAmbiguous`. `isBareSchema` is deleted.

- [ ] **Step 1: Write the failing tests**

Add to `internal/config/refs_test.go`:

```go
func TestClassifySchemaFile(t *testing.T) {
	tests := []struct {
		name    string
		content map[string]any
		want    schemaFileKind
	}{
		{"type as string is the keyword", map[string]any{"type": "object"}, schemaFileBare},
		{"type as array is the keyword", map[string]any{"type": []any{"string", "null"}}, schemaFileBare},
		{"$schema string", map[string]any{"$schema": "https://json-schema.org/draft/2020-12/schema"}, schemaFileBare},
		{"$ref string", map[string]any{"$ref": "schemas/Other"}, schemaFileBare},
		{"oneOf array", map[string]any{"oneOf": []any{}}, schemaFileBare},
		{"enum array", map[string]any{"enum": []any{"a"}}, schemaFileBare},
		{"bare with type and properties", map[string]any{"type": "object", "properties": map[string]any{}}, schemaFileBare},

		{"capitalized definition names", map[string]any{"User": map[string]any{}}, schemaFileKeyed},
		{
			"type as object is a definition name",
			map[string]any{"type": map[string]any{"type": "string"}, "Other": map[string]any{}},
			schemaFileKeyed,
		},
		{
			"oneOf as object is a definition name",
			map[string]any{"oneOf": map[string]any{"type": "string"}},
			schemaFileKeyed,
		},

		{"properties alone is undecidable", map[string]any{"properties": map[string]any{}}, schemaFileAmbiguous},
		{"items alone is undecidable", map[string]any{"items": map[string]any{}}, schemaFileAmbiguous},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, classifySchemaFile(tt.content))
		})
	}
}

func TestBuildSchemaRegistry_LowercaseKeywordDefinitionNames(t *testing.T) {
	// Previously misclassified as a bare schema, silently losing both definitions.
	registry, errs := buildSchemaRegistry(map[string]map[string]any{
		"/p/schemas/domain.json": {
			"type":  map[string]any{"type": "string"},
			"items": map[string]any{"type": "array"},
		},
	})

	assert.Empty(t, errs)
	assert.Len(t, registry, 2)
	assert.Contains(t, registry, "schemas/type")
	assert.Contains(t, registry, "schemas/items")
	assert.NotContains(t, registry, "schemas/domain")
}

func TestBuildSchemaRegistry_AmbiguousFileIsAnError(t *testing.T) {
	registry, errs := buildSchemaRegistry(map[string]map[string]any{
		"/p/schemas/thing.json": {"properties": map[string]any{"name": map[string]any{}}},
	})

	require.Len(t, errs, 1)
	assert.Equal(t, "/p/schemas/thing.json", errs[0].FilePath)
	assert.Contains(t, errs[0].Message, "cannot tell")
	assert.Contains(t, errs[0].Message, `"type"`)
	assert.Empty(t, registry, "an unclassifiable file must not register anything")
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/config/ -run 'TestClassifySchemaFile|TestBuildSchemaRegistry_Lowercase|TestBuildSchemaRegistry_Ambiguous' -v`
Expected: compile failure — `undefined: schemaFileKind`, `undefined: classifySchemaFile`.

- [ ] **Step 3: Replace `isBareSchema` with the classifier**

In `internal/config/refs.go`, delete `isBareSchema` (lines 67-77) and put in its place:

```go
// schemaFileKind is how a schemas/ file's top level should be read.
type schemaFileKind int

const (
	// schemaFileKeyed: each top-level key is a named schema definition.
	schemaFileKeyed schemaFileKind = iota
	// schemaFileBare: the file is itself a JSON Schema document (#373).
	schemaFileBare
	// schemaFileAmbiguous: the two readings cannot be told apart.
	schemaFileAmbiguous
)

// bareSchemaKeywords maps a JSON Schema keyword to a predicate reporting
// whether the value has the shape that keyword actually takes in a schema
// document. Presence alone is not enough: a top-level "type" whose value is an
// object is a schema *named* "type", not the type keyword, and reading it as a
// bare schema silently discards every definition in the file (#405).
var bareSchemaKeywords = map[string]func(any) bool{
	"$schema": isJSONString,
	"$ref":    isJSONString,
	"type":    func(v any) bool { return isJSONString(v) || isJSONArray(v) },
	"enum":    isJSONArray,
	"oneOf":   isJSONArray,
	"anyOf":   isJSONArray,
	"allOf":   isJSONArray,
}

// ambiguousSchemaKeywords take object values both as schema keywords and as
// definition names, so shape cannot separate the two readings.
var ambiguousSchemaKeywords = []string{"properties", "items"}

func isJSONString(v any) bool {
	_, ok := v.(string)
	return ok
}

func isJSONArray(v any) bool {
	_, ok := v.([]any)
	return ok
}

// classifySchemaFile decides how to read a schemas/ file's top level.
// Iteration order over bareSchemaKeywords does not matter: the result is a
// boolean OR over independent checks.
func classifySchemaFile(content map[string]any) schemaFileKind {
	for kw, hasKeywordShape := range bareSchemaKeywords {
		if v, ok := content[kw]; ok && hasKeywordShape(v) {
			return schemaFileBare
		}
	}

	for _, kw := range ambiguousSchemaKeywords {
		if _, ok := content[kw]; ok {
			return schemaFileAmbiguous
		}
	}

	return schemaFileKeyed
}
```

- [ ] **Step 4: Wire the classifier into `buildSchemaRegistry`**

In `buildSchemaRegistry`, replace the `if isBareSchema(content) { ... }` block with a switch, and collect ambiguity errors alongside the collision errors:

```go
func buildSchemaRegistry(schemas map[string]map[string]any) (map[string]map[string]any, []ValidationError) {
	registry := make(map[string]map[string]any)
	sources := make(map[string][]schemaSource)
	var ambiguous []ValidationError

	for filePath, content := range schemas {
		relDir := extractSchemasRelPath(filePath)

		switch classifySchemaFile(content) {
		case schemaFileAmbiguous:
			ambiguous = append(ambiguous, ValidationError{
				FilePath: filePath,
				Message: `cannot tell whether this file is a JSON Schema document or a map of schema definitions — ` +
					`it has a top-level "properties" or "items" but no "type"/"$schema"/"$ref"/"enum"/"oneOf"/"anyOf"/"allOf". ` +
					`Add "type" to make it a schema document, or rename the definition to make it a named-definitions file`,
			})

		case schemaFileBare:
			// A file that is itself a JSON Schema document registers whole
			// under schemas/<filename-without-extension> (#373).
			base := strings.TrimSuffix(filepath.Base(filePath), filepath.Ext(filePath))
			refName := relDir + "/" + base
			registry[refName] = content
			sources[refName] = append(sources[refName], schemaSource{FilePath: filePath})

		default: // schemaFileKeyed — each top-level key is a named schema definition.
			for key, val := range content {
				if schema, ok := val.(map[string]any); ok {
					refName := relDir + "/" + key
					registry[refName] = schema
					sources[refName] = append(sources[refName], schemaSource{FilePath: filePath, Key: key})
				}
			}
		}
	}

	// Sorted by FilePath so the message order does not depend on map iteration.
	sort.Slice(ambiguous, func(i, j int) bool { return ambiguous[i].FilePath < ambiguous[j].FilePath })

	return registry, append(ambiguous, collisionErrors(sources)...)
}
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `go build ./... && go vet ./... && go test ./internal/config/ -count=1`
Expected: PASS, all tests.

If an existing test that feeds a bare schema now fails, check whether its fixture relies on presence-only detection (for example `{"properties": {...}}` with no `"type"`). That fixture is exactly the ambiguous case — give it a `"type": "object"` rather than weakening the classifier.

- [ ] **Step 6: Commit**

```bash
git add internal/config/refs.go internal/config/refs_test.go
git commit -m "fix(config): classify schema files by keyword shape, not presence

isBareSchema checked only whether a JSON Schema keyword was present, so a
named-definitions file with a lowercase definition name like \"type\" or
\"items\" was read as a bare schema and every definition in it silently
vanished. Check the value shape instead — a \"type\" holding an object is a
definition name, not the keyword — and reject the genuinely undecidable
case (top-level \"properties\"/\"items\" with no other evidence) rather than
guessing.

Refs #405

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

### Task 4: Export the registry through the pipeline

This is what actually unblocks the OpenAPI generator unification: `ResolvedConfig.Schemas` is keyed by **file path**, which is the source of the dangling-`$ref` bug in `internal/server/openapi.go:37` vs `:246`.

**Files:**
- Modify: `internal/config/refs.go` (rename `buildSchemaRegistry` → `BuildSchemaRegistry`, set the field in `ResolveRefs`)
- Modify: `internal/config/loader.go:11-24` (`RawConfig`)
- Modify: `internal/config/pipeline.go:16-29` (`ResolvedConfig`), `:126-139` (construction)
- Test: `internal/config/refs_test.go`, `internal/config/pipeline_test.go`

**Interfaces:**
- Consumes: `buildSchemaRegistry` from Tasks 2-3.
- Produces: `func BuildSchemaRegistry(schemas map[string]map[string]any) (map[string]map[string]any, []ValidationError)`; `RawConfig.SchemaRegistry map[string]map[string]any`; `ResolvedConfig.SchemaRegistry map[string]map[string]any`. Keys are ref names (`schemas/User`, `schemas/validation/Task`) — the same strings a config's `$ref` uses.

- [ ] **Step 1: Write the failing test**

Add to `internal/config/refs_test.go`:

```go
func TestResolveRefs_PublishesRegistry(t *testing.T) {
	rc := &RawConfig{
		Schemas: map[string]map[string]any{
			"/p/schemas/User.json":            {"User": map[string]any{"type": "object"}},
			"/p/schemas/validation/Task.json": {"Task": map[string]any{"type": "object"}},
		},
		Routes:      map[string]map[string]any{},
		Workflows:   map[string]map[string]any{},
		Workers:     map[string]map[string]any{},
		Schedules:   map[string]map[string]any{},
		Connections: map[string]map[string]any{},
		Tests:       map[string]map[string]any{},
		Models:      map[string]map[string]any{},
	}

	errs := ResolveRefs(rc)
	require.Empty(t, errs)

	// Keyed by ref name — the same string a config's "$ref" uses — not by file path.
	assert.Len(t, rc.SchemaRegistry, 2)
	assert.Contains(t, rc.SchemaRegistry, "schemas/User")
	assert.Contains(t, rc.SchemaRegistry, "schemas/validation/Task")
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/config/ -run TestResolveRefs_PublishesRegistry -v`
Expected: compile failure — `rc.SchemaRegistry undefined (type *RawConfig has no field or method SchemaRegistry)`.

- [ ] **Step 3: Add the fields and export the builder**

In `internal/config/loader.go`, add to `RawConfig` after the `Schemas` field:

```go
	Schemas     map[string]map[string]any // keyed by file path
	// SchemaRegistry maps $ref names ("schemas/User") to schema definitions.
	// Populated by ResolveRefs; the authoritative schema naming (#405).
	SchemaRegistry map[string]map[string]any
```

In `internal/config/refs.go`, rename the function and update its doc comment:

```go
// BuildSchemaRegistry maps every schema $ref name to its definition, and
// reports files that cannot be classified and ref names claimed more than
// once. Ref names are "<reldir>/<key>", e.g. "schemas/User" or
// "schemas/validation/Task" — the exact strings configs write in "$ref".
func BuildSchemaRegistry(schemas map[string]map[string]any) (map[string]map[string]any, []ValidationError) {
```

Update the call in `ResolveRefs` to use the exported name and publish the result:

```go
	// Build schema registry
	registry, errs := BuildSchemaRegistry(rc.Schemas)
	rc.SchemaRegistry = registry
```

Update the four `buildSchemaRegistry(` call sites added in Tasks 2 and 3 in `internal/config/refs_test.go` to `BuildSchemaRegistry(`.

- [ ] **Step 4: Surface it on `ResolvedConfig`**

In `internal/config/pipeline.go`, add to the `ResolvedConfig` struct after `Schemas`:

```go
	Schemas     map[string]map[string]any
	// SchemaRegistry maps $ref names ("schemas/User") to schema definitions.
	// Schemas above is keyed by file path; consumers that need to resolve a
	// "$ref" string want this one.
	SchemaRegistry map[string]map[string]any
```

and to the returned literal, after `Schemas: raw.Schemas,`:

```go
		SchemaRegistry: raw.SchemaRegistry,
```

- [ ] **Step 5: Add the end-to-end pipeline assertion**

Add to `internal/config/pipeline_test.go`:

```go
func TestValidateAll_PopulatesSchemaRegistry(t *testing.T) {
	rc, errs := ValidateAll("testdata/valid-project", "", secrets.NewManager())
	require.Empty(t, errs)
	require.NotNil(t, rc)

	for name := range rc.SchemaRegistry {
		assert.True(t, strings.HasPrefix(name, "schemas/"),
			"registry key %q should be a $ref name, not a file path", name)
	}
}
```

Before running, open `internal/config/pipeline_test.go` and match the existing tests' project fixture path and `secrets` manager construction — reuse whatever an existing passing `ValidateAll` test in that file uses rather than the placeholder above, and add `strings` to the imports if it is not already there.

- [ ] **Step 6: Run the tests to verify they pass**

Run: `go build ./... && go vet ./... && go test ./internal/config/ -count=1`
Expected: PASS, all tests.

- [ ] **Step 7: Commit**

```bash
git add internal/config/refs.go internal/config/loader.go internal/config/pipeline.go internal/config/refs_test.go internal/config/pipeline_test.go
git commit -m "feat(config): expose the schema ref registry on ResolvedConfig

The \$ref registry was built once inside ResolveRefs and discarded, leaving
ResolvedConfig.Schemas keyed by file path — the reason the OpenAPI
generator registers components under a path and emits refs by basename.
Export BuildSchemaRegistry and carry its result through to
ResolvedConfig.SchemaRegistry, keyed by ref name.

Refs #405

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

### Task 5: Corpus guard, docs, and CHANGELOG

**Files:**
- Modify: `docs/02-config/schemas.md:33-38`
- Modify: `CHANGELOG.md` (the `## [Unreleased]` section)

**Interfaces:**
- Consumes: everything above. Produces nothing consumed by later tasks.

- [ ] **Step 1: Verify no shipped config regressed**

The whole change is only safe if every config in the repo still loads. Run:

```bash
go test ./... -count=1 2>&1 | grep -v '^ok\|no test files' | head -40
```

Expected: no output (no failures). If a config under `examples/`, `testdata/`, or `projects/` now reports a duplicate ref or an ambiguous file, **stop and report it** — the pre-implementation scan found 20 schema files with 0 collisions and 0 ambiguous classifications, so a hit here means the implementation diverges from the spec, not that the fixture is wrong.

- [ ] **Step 2: Update the schemas doc**

In `docs/02-config/schemas.md`, replace lines 35-38 with:

```markdown
- **Named-definitions file** (shown above): each top-level key is a schema, registered as `schemas/<Key>`. `schemas/Task.json` containing keys `Task` and `CreateTask` registers `schemas/Task` and `schemas/CreateTask`. The filename itself does not matter.
- **Bare schema file**: a file that is itself a JSON Schema document registers under its filename -- `schemas/greeting.json` registers `schemas/greeting`. A file counts as a bare schema when a top-level JSON Schema keyword carries the value shape that keyword really takes: `type` as a string or array, `$schema`/`$ref` as a string, or `enum`/`oneOf`/`anyOf`/`allOf` as an array. A top-level `type` holding an *object* is a schema named `type`, not the keyword.

For files in subdirectories the directory path is part of the ref: `schemas/validation/User.json` with key `CreateUser` registers `schemas/validation/CreateUser`; a bare `schemas/validation/greeting.json` registers `schemas/validation/greeting`.

**Ref names must be unique.** Two definitions that register the same name -- two files in one directory sharing a top-level key, or a bare `schemas/User.json` alongside another file's `User` key -- are rejected by `noda validate` and at boot. Rename one definition, or move one file into a subdirectory, since the directory is part of the name.

**Give a bare schema a `type`.** A file whose top level has `properties` or `items` and none of the keywords listed above is ambiguous -- it could be read either way -- and is rejected rather than guessed at. Adding `"type": "object"` resolves it.
```

- [ ] **Step 3: Update the CHANGELOG**

Under `## [Unreleased]`, add to the existing `### Fixed` section (create it directly after `### Added` if it does not exist):

```markdown
- Schema `$ref` name collisions are rejected instead of silently resolving to a nondeterministic winner — two definitions registering the same ref name (e.g. two files in one directory sharing a top-level key) previously collapsed to whichever one Go's randomized map iteration wrote last, so a route could validate against a different schema on each boot (#405)
- A named-definitions schema file whose definition names collide with lowercase JSON Schema keywords (`{"type": {...}}`) is no longer misread as a bare schema document, which silently discarded every definition in the file (#405)
- A schema file with a top-level `properties`/`items` and no `type`/`$schema`/`$ref`/`enum`/`oneOf`/`anyOf`/`allOf` is now rejected as ambiguous rather than assumed to be a bare schema — add `"type"` to disambiguate (#405)
```

- [ ] **Step 4: Verify the docs build and nothing else broke**

Run: `make test 2>&1 | tail -20`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add docs/02-config/schemas.md CHANGELOG.md
git commit -m "docs: schema ref names must be unique and unambiguous

Refs #405

Co-Authored-By: Claude Opus 4.8 <noreply@anthropic.com>"
```

---

## Self-Review Notes

**Spec coverage:** collision detection → Task 2; shape-aware classification and the ambiguous case → Task 3; `ResolveRefs` returning `[]ValidationError` → Task 1; registry export → Task 4; corpus guard and the four documented behavior changes → Task 5. The spec's "deliberately out of scope" `JSONPath` note is captured in Global Constraints.

**Naming consistency across tasks:** `buildSchemaRegistry` is unexported in Tasks 2-3 and renamed to `BuildSchemaRegistry` in Task 4 — Task 4 Step 3 explicitly updates the four test call sites the earlier tasks added. `schemaSource`, `collisionErrors`, `classifySchemaFile`, `schemaFileKind` and its three constants, `isJSONString`, `isJSONArray`, `SchemaRegistry` are each defined once and used with the same spelling everywhere.

**Known soft spot:** Task 4 Step 5 depends on a fixture path in `pipeline_test.go` that the implementer must confirm rather than copy blindly — flagged inline.
