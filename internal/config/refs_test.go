package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveRefs_Simple(t *testing.T) {
	rc := &RawConfig{
		Schemas: map[string]map[string]any{
			"project/schemas/User.json": {
				"User": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"name": map[string]any{"type": "string"},
					},
				},
			},
		},
		Routes: map[string]map[string]any{
			"routes/users.json": {
				"response": map[string]any{
					"$ref": "schemas/User",
				},
			},
		},
		Workflows:   map[string]map[string]any{},
		Workers:     map[string]map[string]any{},
		Schedules:   map[string]map[string]any{},
		Connections: map[string]map[string]any{},
		Tests:       map[string]map[string]any{},
	}

	errs := ResolveRefs(rc)
	assert.Empty(t, errs)

	response := rc.Routes["routes/users.json"]["response"].(map[string]any)
	assert.Equal(t, "object", response["type"])
	assert.NotNil(t, response["properties"])
}

func TestResolveRefs_InWorkflow(t *testing.T) {
	rc := &RawConfig{
		Schemas: map[string]map[string]any{
			"project/schemas/Task.json": {
				"Task": map[string]any{
					"type": "object",
				},
			},
		},
		Routes: map[string]map[string]any{},
		Workflows: map[string]map[string]any{
			"workflows/create.json": {
				"nodes": map[string]any{
					"validate": map[string]any{
						"type":   "transform.validate",
						"config": map[string]any{"schema": map[string]any{"$ref": "schemas/Task"}},
					},
				},
			},
		},
		Workers:     map[string]map[string]any{},
		Schedules:   map[string]map[string]any{},
		Connections: map[string]map[string]any{},
		Tests:       map[string]map[string]any{},
	}

	errs := ResolveRefs(rc)
	assert.Empty(t, errs)

	schema := rc.Workflows["workflows/create.json"]["nodes"].(map[string]any)["validate"].(map[string]any)["config"].(map[string]any)["schema"].(map[string]any)
	assert.Equal(t, "object", schema["type"])
}

func TestResolveRefs_NestedRef(t *testing.T) {
	rc := &RawConfig{
		Schemas: map[string]map[string]any{
			"project/schemas/common.json": {
				"Pagination": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"page": map[string]any{"type": "integer"},
					},
				},
				"PaginatedList": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"pagination": map[string]any{"$ref": "schemas/Pagination"},
						"items":      map[string]any{"type": "array"},
					},
				},
			},
		},
		Routes: map[string]map[string]any{
			"routes/list.json": {
				"response": map[string]any{"$ref": "schemas/PaginatedList"},
			},
		},
		Workflows:   map[string]map[string]any{},
		Workers:     map[string]map[string]any{},
		Schedules:   map[string]map[string]any{},
		Connections: map[string]map[string]any{},
		Tests:       map[string]map[string]any{},
	}

	errs := ResolveRefs(rc)
	assert.Empty(t, errs)

	response := rc.Routes["routes/list.json"]["response"].(map[string]any)
	props := response["properties"].(map[string]any)
	pagination := props["pagination"].(map[string]any)
	assert.Equal(t, "object", pagination["type"])
}

func TestResolveRefs_CircularRef(t *testing.T) {
	rc := &RawConfig{
		Schemas: map[string]map[string]any{
			"project/schemas/a.json": {
				"A": map[string]any{"child": map[string]any{"$ref": "schemas/B"}},
				"B": map[string]any{"child": map[string]any{"$ref": "schemas/A"}},
			},
		},
		Routes: map[string]map[string]any{
			"routes/test.json": {
				"schema": map[string]any{"$ref": "schemas/A"},
			},
		},
		Workflows:   map[string]map[string]any{},
		Workers:     map[string]map[string]any{},
		Schedules:   map[string]map[string]any{},
		Connections: map[string]map[string]any{},
		Tests:       map[string]map[string]any{},
	}

	errs := ResolveRefs(rc)
	require.NotEmpty(t, errs)
	assert.Contains(t, errs[0].Error(), "circular")
}

func TestResolveRefs_MissingRef(t *testing.T) {
	rc := &RawConfig{
		Schemas: map[string]map[string]any{},
		Routes: map[string]map[string]any{
			"routes/test.json": {
				"schema": map[string]any{"$ref": "schemas/NonExistent"},
			},
		},
		Workflows:   map[string]map[string]any{},
		Workers:     map[string]map[string]any{},
		Schedules:   map[string]map[string]any{},
		Connections: map[string]map[string]any{},
		Tests:       map[string]map[string]any{},
	}

	errs := ResolveRefs(rc)
	require.Len(t, errs, 1)
	assert.Contains(t, errs[0].Error(), "schemas/NonExistent")
	assert.Contains(t, errs[0].Error(), "routes/test.json")
}

func TestResolveRefs_MultipleDefinitionsFromOneFile(t *testing.T) {
	rc := &RawConfig{
		Schemas: map[string]map[string]any{
			"project/schemas/models.json": {
				"User": map[string]any{"type": "user"},
				"Task": map[string]any{"type": "task"},
			},
		},
		Routes: map[string]map[string]any{
			"routes/a.json": {"schema": map[string]any{"$ref": "schemas/User"}},
			"routes/b.json": {"schema": map[string]any{"$ref": "schemas/Task"}},
		},
		Workflows:   map[string]map[string]any{},
		Workers:     map[string]map[string]any{},
		Schedules:   map[string]map[string]any{},
		Connections: map[string]map[string]any{},
		Tests:       map[string]map[string]any{},
	}

	errs := ResolveRefs(rc)
	assert.Empty(t, errs)

	assert.Equal(t, "user", rc.Routes["routes/a.json"]["schema"].(map[string]any)["type"])
	assert.Equal(t, "task", rc.Routes["routes/b.json"]["schema"].(map[string]any)["type"])
}

func TestResolveRefs_SubfolderSchema(t *testing.T) {
	rc := &RawConfig{
		Schemas: map[string]map[string]any{
			"project/schemas/validation/CreateTask.json": {
				"CreateTask": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"title": map[string]any{"type": "string"},
					},
				},
			},
			"project/schemas/models/Task.json": {
				"Task": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"id":    map[string]any{"type": "string"},
						"title": map[string]any{"type": "string"},
					},
				},
			},
			// Flat schema still works
			"project/schemas/Common.json": {
				"Pagination": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"page": map[string]any{"type": "integer"},
					},
				},
			},
		},
		Routes: map[string]map[string]any{
			"routes/create.json": {
				"body":     map[string]any{"$ref": "schemas/validation/CreateTask"},
				"response": map[string]any{"$ref": "schemas/models/Task"},
			},
			"routes/list.json": {
				"pagination": map[string]any{"$ref": "schemas/Pagination"},
			},
		},
		Workflows:   map[string]map[string]any{},
		Workers:     map[string]map[string]any{},
		Schedules:   map[string]map[string]any{},
		Connections: map[string]map[string]any{},
		Tests:       map[string]map[string]any{},
	}

	errs := ResolveRefs(rc)
	assert.Empty(t, errs)

	// Subfolder ref: schemas/validation/CreateTask
	body := rc.Routes["routes/create.json"]["body"].(map[string]any)
	assert.Equal(t, "object", body["type"])
	assert.NotNil(t, body["properties"])

	// Subfolder ref: schemas/models/Task
	response := rc.Routes["routes/create.json"]["response"].(map[string]any)
	assert.Equal(t, "object", response["type"])

	// Flat ref still works: schemas/Pagination
	pagination := rc.Routes["routes/list.json"]["pagination"].(map[string]any)
	assert.Equal(t, "object", pagination["type"])
}

func TestExtractSchemasRelPath(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"/project/schemas/Task.json", "schemas"},
		{"/project/schemas/validation/CreateTask.json", "schemas/validation"},
		{"/project/schemas/models/db/User.json", "schemas/models/db"},
		{"schemas/Task.json", "schemas"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := extractSchemasRelPath(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestResolveRefs_BareSchemaFileRegistersUnderFilename pins #373: a schema
// file that is itself a JSON Schema document (has "type"/"properties"/...
// at top level, rather than name→schema wrapper keys) registers under
// schemas/<filename-without-extension>.
func TestResolveRefs_BareSchemaFileRegistersUnderFilename(t *testing.T) {
	rc := &RawConfig{
		Schemas: map[string]map[string]any{
			"project/schemas/greeting.json": {
				"type": "object",
				"properties": map[string]any{
					"message": map[string]any{"type": "string"},
				},
				"required": []any{"message"},
			},
		},
		Routes: map[string]map[string]any{
			"routes/greet.json": {
				"body": map[string]any{"schema": map[string]any{"$ref": "schemas/greeting"}},
			},
		},
	}

	errs := ResolveRefs(rc)
	require.Empty(t, errs)

	body := rc.Routes["routes/greet.json"]["body"].(map[string]any)
	schema := body["schema"].(map[string]any)
	assert.Equal(t, "object", schema["type"])
	props := schema["properties"].(map[string]any)
	assert.Contains(t, props, "message")
}

// TestResolveRefs_BareSchemaFileSubdir: a bare schema file in a
// subdirectory registers under its directory path + filename (#373).
func TestResolveRefs_BareSchemaFileSubdir(t *testing.T) {
	rc := &RawConfig{
		Schemas: map[string]map[string]any{
			"project/schemas/validation/greeting.json": {
				"type": "object",
			},
		},
		Routes: map[string]map[string]any{
			"routes/a.json": {"schema": map[string]any{"$ref": "schemas/validation/greeting"}},
		},
	}

	errs := ResolveRefs(rc)
	require.Empty(t, errs)
}

// TestResolveRefs_BareSchemaFileDoesNotRegisterKeywordKeys: a bare schema
// file must NOT leak wrapper-style refs from its own keyword keys
// (e.g. "schemas/properties") (#373).
func TestResolveRefs_BareSchemaFileDoesNotRegisterKeywordKeys(t *testing.T) {
	rc := &RawConfig{
		Schemas: map[string]map[string]any{
			"project/schemas/greeting.json": {
				"type":       "object",
				"properties": map[string]any{"message": map[string]any{"type": "string"}},
			},
		},
		Routes: map[string]map[string]any{
			"routes/a.json": {"schema": map[string]any{"$ref": "schemas/properties"}},
		},
	}

	errs := ResolveRefs(rc)
	require.Len(t, errs, 1)
	assert.Contains(t, errs[0].Error(), "unresolved $ref")
}

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

// TestResolveRefs_UnresolvedErrorListsKnownRefs pins #373: the
// unresolved-$ref error must teach the resolution rule and list what IS
// registered, so a near-miss ref is self-diagnosing.
func TestResolveRefs_UnresolvedErrorListsKnownRefs(t *testing.T) {
	rc := &RawConfig{
		Schemas: map[string]map[string]any{
			"project/schemas/User.json": {
				"User": map[string]any{"type": "object"},
			},
		},
		Routes: map[string]map[string]any{
			"routes/a.json": {"schema": map[string]any{"$ref": "schemas/user"}},
		},
	}

	errs := ResolveRefs(rc)
	require.Len(t, errs, 1)
	msg := errs[0].Error()
	assert.Contains(t, msg, `unresolved $ref "schemas/user"`)
	assert.Contains(t, msg, "schemas/User")  // the known-refs list
	assert.Contains(t, msg, "top-level key") // the convention hint
}

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
	registry, errs := BuildSchemaRegistry(map[string]map[string]any{
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
	registry, errs := BuildSchemaRegistry(map[string]map[string]any{
		"/p/schemas/thing.json": {"properties": map[string]any{"name": map[string]any{}}},
	})

	require.Len(t, errs, 1)
	assert.Equal(t, "/p/schemas/thing.json", errs[0].FilePath)
	assert.Contains(t, errs[0].Message, "cannot tell")
	assert.Contains(t, errs[0].Message, `"type"`)
	assert.Empty(t, registry, "an unclassifiable file must not register anything")
}

func TestBuildSchemaRegistry_CollisionKeyedVsKeyed(t *testing.T) {
	schemas := map[string]map[string]any{
		"/p/schemas/a.json": {"User": map[string]any{"marker": "FROM_A"}},
		"/p/schemas/b.json": {"User": map[string]any{"marker": "FROM_B"}},
	}

	// Map iteration is randomized; the collision must be reported on every build.
	var first string
	for i := range 200 {
		_, errs := BuildSchemaRegistry(schemas)
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

	// Map iteration is randomized; the collision must be reported on every build.
	var first string
	for i := range 200 {
		_, errs := BuildSchemaRegistry(schemas)
		require.Len(t, errs, 1, "collision must be detected on build %d", i)
		if i == 0 {
			first = errs[0].Error()
			continue
		}
		assert.Equal(t, first, errs[0].Error(), "error text must be deterministic (build %d)", i)
	}

	assert.Contains(t, first, "/p/schemas/User.json (whole file)")
	assert.Contains(t, first, `/p/schemas/other.json (key "User")`)
}

func TestBuildSchemaRegistry_NoCollisionAcrossDirectories(t *testing.T) {
	registry, errs := BuildSchemaRegistry(map[string]map[string]any{
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
