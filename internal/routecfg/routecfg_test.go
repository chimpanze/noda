package routecfg

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Pure helper tests ---

func TestNormalizeRoutes_SingleRoute(t *testing.T) {
	data := map[string]any{"method": "GET", "path": "/test"}
	routes := NormalizeRoutes(data)
	require.Len(t, routes, 1)
	assert.Equal(t, "GET", routes[0]["method"])
}

func TestNormalizeRoutes_GroupedRoutes(t *testing.T) {
	data := map[string]any{
		"getUsers": map[string]any{"method": "GET", "path": "/users"},
		"addUser":  map[string]any{"method": "POST", "path": "/users"},
		"meta":     "not a route",
	}
	routes := NormalizeRoutes(data)
	assert.Len(t, routes, 2)
}

func TestNormalizeRoutes_Empty(t *testing.T) {
	routes := NormalizeRoutes(map[string]any{})
	assert.Empty(t, routes)
}

// --- Middleware: ExtractMiddlewareConfig branches ---

func TestExtractMiddlewareConfig_MiddlewareSection(t *testing.T) {
	root := map[string]any{
		"middleware": map[string]any{
			"limiter": map[string]any{"max": float64(100)},
		},
	}
	cfg := ExtractMiddlewareConfig("limiter", root)
	require.NotNil(t, cfg)
	assert.Equal(t, float64(100), cfg["max"])
}

func TestExtractMiddlewareConfig_SecuritySection(t *testing.T) {
	root := map[string]any{
		"security": map[string]any{
			"cors": map[string]any{"allow_origins": "*"},
		},
	}
	cfg := ExtractMiddlewareConfig("security.cors", root)
	require.NotNil(t, cfg)
	assert.Equal(t, "*", cfg["allow_origins"])
}

func TestExtractMiddlewareConfig_AuthJWT(t *testing.T) {
	root := map[string]any{
		"security": map[string]any{
			"jwt": map[string]any{"secret": "s3cr3t"},
		},
	}
	cfg := ExtractMiddlewareConfig("auth.jwt", root)
	require.NotNil(t, cfg)
	assert.Equal(t, "s3cr3t", cfg["secret"])
}

func TestExtractMiddlewareConfig_CasbinEnforce(t *testing.T) {
	root := map[string]any{
		"security": map[string]any{
			"casbin": map[string]any{"model": "test.conf"},
		},
	}
	cfg := ExtractMiddlewareConfig("casbin.enforce", root)
	require.NotNil(t, cfg)
	assert.Equal(t, "test.conf", cfg["model"])
}

func TestExtractMiddlewareConfig_NilRoot(t *testing.T) {
	cfg := ExtractMiddlewareConfig("limiter", nil)
	assert.Nil(t, cfg)
}

func TestExtractMiddlewareConfig_NoMatch(t *testing.T) {
	root := map[string]any{
		"middleware": map[string]any{
			"other": map[string]any{},
		},
	}
	cfg := ExtractMiddlewareConfig("limiter", root)
	assert.Nil(t, cfg)
}
