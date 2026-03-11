package registry

import (
	"testing"

	"github.com/chimpanze/noda/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCollectDeferredServices_WebSocket(t *testing.T) {
	rc := &config.ResolvedConfig{
		Connections: map[string]map[string]any{
			"connections/realtime.json": {
				"endpoints": map[string]any{
					"live": map[string]any{"type": "websocket", "path": "/ws"},
				},
			},
		},
	}

	deferred, errs := CollectDeferredServices(rc)
	assert.Empty(t, errs)
	require.Contains(t, deferred, "live")
	assert.Equal(t, "ws", deferred["live"].Prefix)
}

func TestCollectDeferredServices_SSE(t *testing.T) {
	rc := &config.ResolvedConfig{
		Connections: map[string]map[string]any{
			"connections/events.json": {
				"endpoints": map[string]any{
					"updates": map[string]any{"type": "sse", "path": "/events"},
				},
			},
		},
	}

	deferred, errs := CollectDeferredServices(rc)
	assert.Empty(t, errs)
	require.Contains(t, deferred, "updates")
	assert.Equal(t, "sse", deferred["updates"].Prefix)
}

func TestCollectDeferredServices_Wasm(t *testing.T) {
	rc := &config.ResolvedConfig{
		Root: map[string]any{
			"wasm_runtimes": map[string]any{
				"game-engine": map[string]any{"module": "game.wasm"},
			},
		},
		Connections: map[string]map[string]any{},
	}

	deferred, errs := CollectDeferredServices(rc)
	assert.Empty(t, errs)
	require.Contains(t, deferred, "game-engine")
	assert.Equal(t, "wasm", deferred["game-engine"].Prefix)
}

func TestCollectDeferredServices_UnknownConnectionType(t *testing.T) {
	rc := &config.ResolvedConfig{
		Connections: map[string]map[string]any{
			"connections/bad.json": {
				"endpoints": map[string]any{
					"bad": map[string]any{"type": "unknown"},
				},
			},
		},
	}

	_, errs := CollectDeferredServices(rc)
	require.Len(t, errs, 1)
	assert.Contains(t, errs[0].Error(), "unknown type")
}

func TestCollectDeferredServices_MultipleEndpoints(t *testing.T) {
	rc := &config.ResolvedConfig{
		Connections: map[string]map[string]any{
			"connections/realtime.json": {
				"endpoints": map[string]any{
					"chat":   map[string]any{"type": "websocket", "path": "/ws/chat"},
					"events": map[string]any{"type": "sse", "path": "/events"},
				},
			},
		},
	}

	deferred, errs := CollectDeferredServices(rc)
	assert.Empty(t, errs)
	assert.Len(t, deferred, 2)
	assert.Equal(t, "ws", deferred["chat"].Prefix)
	assert.Equal(t, "sse", deferred["events"].Prefix)
}
