package registry

import (
	"testing"

	"github.com/chimpanze/noda/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRegisterInternalServices_WebSocket(t *testing.T) {
	rc := &config.ResolvedConfig{
		Connections: map[string]map[string]any{
			"live": {"type": "websocket", "path": "/ws"},
		},
	}

	registry := NewServiceRegistry()
	errs := RegisterInternalServices(rc, registry)
	assert.Empty(t, errs)

	prefix, ok := registry.GetPrefix("live")
	assert.True(t, ok)
	assert.Equal(t, "ws", prefix)
}

func TestRegisterInternalServices_SSE(t *testing.T) {
	rc := &config.ResolvedConfig{
		Connections: map[string]map[string]any{
			"updates": {"type": "sse", "path": "/events"},
		},
	}

	registry := NewServiceRegistry()
	errs := RegisterInternalServices(rc, registry)
	assert.Empty(t, errs)

	prefix, ok := registry.GetPrefix("updates")
	assert.True(t, ok)
	assert.Equal(t, "sse", prefix)
}

func TestRegisterInternalServices_Wasm(t *testing.T) {
	rc := &config.ResolvedConfig{
		Root: map[string]any{
			"wasm": map[string]any{
				"game-engine": map[string]any{"module": "game.wasm"},
			},
		},
		Connections: map[string]map[string]any{},
	}

	registry := NewServiceRegistry()
	errs := RegisterInternalServices(rc, registry)
	assert.Empty(t, errs)

	prefix, ok := registry.GetPrefix("game-engine")
	assert.True(t, ok)
	assert.Equal(t, "wasm", prefix)
}

func TestRegisterInternalServices_ConflictWithExternal(t *testing.T) {
	rc := &config.ResolvedConfig{
		Connections: map[string]map[string]any{
			"main-db": {"type": "websocket"},
		},
	}

	registry := NewServiceRegistry()
	// Pre-register an external service with same name
	require.NoError(t, registry.Register("main-db", "inst", &stubPlugin{name: "db", prefix: "db"}))

	errs := RegisterInternalServices(rc, registry)
	require.Len(t, errs, 1)
	assert.Contains(t, errs[0].Error(), "duplicate")
}

func TestRegisterInternalServices_UnknownConnectionType(t *testing.T) {
	rc := &config.ResolvedConfig{
		Connections: map[string]map[string]any{
			"bad": {"type": "unknown"},
		},
	}

	registry := NewServiceRegistry()
	errs := RegisterInternalServices(rc, registry)
	require.Len(t, errs, 1)
	assert.Contains(t, errs[0].Error(), "unknown type")
}
