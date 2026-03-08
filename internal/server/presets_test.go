package server

import (
	"testing"

	"github.com/chimpanze/noda/internal/config"
	"github.com/chimpanze/noda/internal/registry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testServerWithConfig(root map[string]any) *Server {
	rc := &config.ResolvedConfig{
		Root:   root,
		Routes: map[string]map[string]any{},
	}
	srv, _ := NewServer(rc, registry.NewServiceRegistry(), registry.NewNodeRegistry())
	return srv
}

func TestResolveMiddlewareChain_RouteWithPreset(t *testing.T) {
	srv := testServerWithConfig(map[string]any{
		"middleware_presets": map[string]any{
			"authenticated": []any{"recover", "requestid"},
		},
	})

	route := map[string]any{
		"id":                "test-route",
		"path":              "/api/test",
		"middleware_preset": "authenticated",
	}

	handlers, err := srv.ResolveMiddlewareChain(route)
	require.NoError(t, err)
	assert.Len(t, handlers, 2)
}

func TestResolveMiddlewareChain_GroupInheritance(t *testing.T) {
	srv := testServerWithConfig(map[string]any{
		"middleware_presets": map[string]any{
			"admin": []any{"recover", "requestid"},
		},
		"route_groups": map[string]any{
			"/api/admin": map[string]any{
				"middleware_preset": "admin",
			},
		},
	})

	route := map[string]any{
		"id":   "admin-route",
		"path": "/api/admin/users",
	}

	handlers, err := srv.ResolveMiddlewareChain(route)
	require.NoError(t, err)
	assert.Len(t, handlers, 2)
}

func TestResolveMiddlewareChain_RouteExtendsGroup(t *testing.T) {
	srv := testServerWithConfig(map[string]any{
		"middleware_presets": map[string]any{
			"base": []any{"recover"},
		},
		"route_groups": map[string]any{
			"/api": map[string]any{
				"middleware_preset": "base",
			},
		},
	})

	route := map[string]any{
		"id":         "extended-route",
		"path":       "/api/tasks",
		"middleware": []any{"requestid"},
	}

	handlers, err := srv.ResolveMiddlewareChain(route)
	require.NoError(t, err)
	assert.Len(t, handlers, 2) // recover from group + requestid from route
}

func TestResolveMiddlewareChain_GlobalMiddleware(t *testing.T) {
	srv := testServerWithConfig(map[string]any{
		"global_middleware": []any{"recover", "requestid"},
	})

	route := map[string]any{
		"id":   "test-route",
		"path": "/test",
	}

	handlers, err := srv.ResolveMiddlewareChain(route)
	require.NoError(t, err)
	assert.Len(t, handlers, 2)
}

func TestResolveMiddlewareChain_Deduplication(t *testing.T) {
	srv := testServerWithConfig(map[string]any{
		"global_middleware": []any{"recover"},
		"middleware_presets": map[string]any{
			"base": []any{"recover", "requestid"},
		},
		"route_groups": map[string]any{
			"/api": map[string]any{
				"middleware_preset": "base",
			},
		},
	})

	route := map[string]any{
		"id":   "dedup-route",
		"path": "/api/test",
	}

	handlers, err := srv.ResolveMiddlewareChain(route)
	require.NoError(t, err)
	assert.Len(t, handlers, 2) // recover (deduped) + requestid
}

func TestValidatePresets_UnknownPreset(t *testing.T) {
	rc := &config.ResolvedConfig{
		Root: map[string]any{
			"middleware_presets": map[string]any{
				"known": []any{"recover"},
			},
		},
		Routes: map[string]map[string]any{
			"bad-route": {
				"middleware_preset": "unknown",
			},
		},
	}
	srv, _ := NewServer(rc, registry.NewServiceRegistry(), registry.NewNodeRegistry())

	errs := srv.ValidatePresets()
	require.Len(t, errs, 1)
	assert.Contains(t, errs[0].Error(), "unknown middleware preset")
}

func TestValidatePresets_ValidPreset(t *testing.T) {
	rc := &config.ResolvedConfig{
		Root: map[string]any{
			"middleware_presets": map[string]any{
				"known": []any{"recover"},
			},
		},
		Routes: map[string]map[string]any{
			"good-route": {
				"middleware_preset": "known",
			},
		},
	}
	srv, _ := NewServer(rc, registry.NewServiceRegistry(), registry.NewNodeRegistry())

	errs := srv.ValidatePresets()
	assert.Empty(t, errs)
}
