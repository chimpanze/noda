package server

import (
	"testing"

	"github.com/chimpanze/noda/internal/expr"
	"github.com/chimpanze/noda/internal/pathutil"
	"github.com/chimpanze/noda/internal/registry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Pure helper tests ---

func TestNormalizeRoutes_SingleRoute(t *testing.T) {
	data := map[string]any{"method": "GET", "path": "/test"}
	routes := normalizeRoutes(data)
	require.Len(t, routes, 1)
	assert.Equal(t, "GET", routes[0]["method"])
}

func TestNormalizeRoutes_GroupedRoutes(t *testing.T) {
	data := map[string]any{
		"getUsers": map[string]any{"method": "GET", "path": "/users"},
		"addUser":  map[string]any{"method": "POST", "path": "/users"},
		"meta":     "not a route",
	}
	routes := normalizeRoutes(data)
	assert.Len(t, routes, 2)
}

func TestNormalizeRoutes_Empty(t *testing.T) {
	routes := normalizeRoutes(map[string]any{})
	assert.Empty(t, routes)
}

func TestConvertPath_WithParams(t *testing.T) {
	assert.Equal(t, "/users/{id}", convertPath("/users/:id"))
	assert.Equal(t, "/users/{id}/posts/{postId}", convertPath("/users/:id/posts/:postId"))
}

func TestConvertPath_NoParams(t *testing.T) {
	assert.Equal(t, "/static/path", convertPath("/static/path"))
}

func TestExtractPathParams_WithParams(t *testing.T) {
	params := extractPathParams("/users/:id/posts/:postId")
	require.Len(t, params, 2)

	p1 := params[0].(map[string]any)
	assert.Equal(t, "id", p1["name"])
	assert.Equal(t, "path", p1["in"])
	assert.Equal(t, true, p1["required"])

	p2 := params[1].(map[string]any)
	assert.Equal(t, "postId", p2["name"])
}

func TestExtractPathParams_NoParams(t *testing.T) {
	params := extractPathParams("/users/list")
	assert.Nil(t, params)
}

func TestFindVarRefs_String(t *testing.T) {
	refs := findVarRefs("use {{ $var('myVar') }} and {{ $var('other') }}")
	assert.Contains(t, refs, "myVar")
	assert.Contains(t, refs, "other")
}

func TestFindVarRefs_Nested(t *testing.T) {
	data := map[string]any{
		"key": "{{ $var('topLevel') }}",
		"nested": map[string]any{
			"inner": "{{ $var('deep') }}",
		},
		"list": []any{"{{ $var('inList') }}"},
	}
	refs := findVarRefs(data)
	assert.Contains(t, refs, "topLevel")
	assert.Contains(t, refs, "deep")
	assert.Contains(t, refs, "inList")
}

func TestFindVarRefs_NoRefs(t *testing.T) {
	refs := findVarRefs("no references here")
	assert.Empty(t, refs)
}

func TestFindVarRefs_NonStringType(t *testing.T) {
	refs := findVarRefs(42)
	assert.Empty(t, refs)
}

func TestFindEnvRefs_String(t *testing.T) {
	refs := findEnvRefs("use $env(DB_URL) and $env(API_KEY)")
	assert.Contains(t, refs, "DB_URL")
	assert.Contains(t, refs, "API_KEY")
}

func TestFindEnvRefs_Nested(t *testing.T) {
	data := map[string]any{
		"url": "$env(DATABASE_URL)",
		"nested": map[string]any{
			"secret": "$env(SECRET_KEY)",
		},
		"list": []any{"$env(LIST_VAR)"},
	}
	refs := findEnvRefs(data)
	assert.Contains(t, refs, "DATABASE_URL")
	assert.Contains(t, refs, "SECRET_KEY")
	assert.Contains(t, refs, "LIST_VAR")
}

func TestFindEnvRefs_NoRefs(t *testing.T) {
	refs := findEnvRefs("no env refs")
	assert.Empty(t, refs)
}

func TestFindEnvRefs_EmptyName(t *testing.T) {
	// $env() with empty name after trimming shouldn't match
	refs := findEnvRefs("$env(  )")
	assert.Empty(t, refs)
}

func TestFindEnvRefs_NonStringType(t *testing.T) {
	refs := findEnvRefs(42)
	assert.Empty(t, refs)
}

// --- Editor API constructor tests ---

func TestNewEditorAPI_Constructor(t *testing.T) {
	root, err := pathutil.NewRoot(t.TempDir())
	require.NoError(t, err)
	plugins := registry.NewPluginRegistry()
	nodes := registry.NewNodeRegistry()
	services := registry.NewServiceRegistry()
	compiler := expr.NewCompiler()

	api := NewEditorAPI(root, "", nil, plugins, nodes, services, compiler, nil)
	require.NotNil(t, api)
	assert.NotEmpty(t, api.root.String())
}

func TestNewEditorAPIReadOnly_Constructor(t *testing.T) {
	root, err := pathutil.NewRoot(t.TempDir())
	require.NoError(t, err)
	plugins := registry.NewPluginRegistry()
	nodes := registry.NewNodeRegistry()
	services := registry.NewServiceRegistry()
	compiler := expr.NewCompiler()

	api := NewEditorAPIReadOnly(root, "", nil, plugins, nodes, services, compiler, nil)
	require.NotNil(t, api)
	assert.Nil(t, api.reloader)
}

// --- ServerOption tests ---

func TestWithCompiler_Option(t *testing.T) {
	s := &Server{}
	c := expr.NewCompiler()
	opt := WithCompiler(c)
	opt(s)
	assert.Equal(t, c, s.compiler)
}

func TestConnManagers_Nil(t *testing.T) {
	s := &Server{}
	assert.Nil(t, s.ConnManagers())
}
