//go:build integration

package cache

import (
	"context"
	"testing"

	"github.com/chimpanze/noda/internal/engine"
	"github.com/chimpanze/noda/internal/plugin"
	"github.com/chimpanze/noda/internal/registry"
	"github.com/chimpanze/noda/internal/testing/containers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupCache(t *testing.T) (*registry.ServiceRegistry, *registry.NodeRegistry, any) {
	t.Helper()
	url := containers.StartRedis(t)
	svc, err := (&Plugin{}).CreateService(map[string]any{"url": url})
	require.NoError(t, err)
	svcReg := registry.NewServiceRegistry()
	require.NoError(t, svcReg.Register("cache", svc, nil))
	nodeReg := registry.NewNodeRegistry()
	require.NoError(t, nodeReg.RegisterFromPlugin(&Plugin{}))
	return svcReg, nodeReg, svc
}

func runWF(t *testing.T, svcReg *registry.ServiceRegistry, nodeReg *registry.NodeRegistry,
	wf engine.WorkflowConfig) *engine.ExecutionContextImpl {
	t.Helper()
	graph, err := engine.Compile(wf, nodeReg)
	require.NoError(t, err)
	execCtx := engine.NewExecutionContext(engine.WithInput(nil))
	require.NoError(t, engine.ExecuteGraph(context.Background(), graph, execCtx, svcReg, nodeReg))
	return execCtx
}

func TestCacheSetExistsDel_Engine(t *testing.T) {
	svcReg, nodeReg, svc := setupCache(t)
	rc := svc.(plugin.RedisClientProvider).Client()
	ctx := context.Background()

	// --- cache.set ---
	setWF := engine.WorkflowConfig{
		ID: "cache-set",
		Nodes: map[string]engine.NodeConfig{
			"s": {
				Type:     "cache.set",
				Services: map[string]string{"cache": "cache"},
				Config:   map[string]any{"key": "greeting", "value": "hello", "ttl": 60},
			},
		},
	}
	runWF(t, svcReg, nodeReg, setWF)

	// The service JSON-encodes values, so the raw Redis value is `"hello"` (with quotes).
	got, err := rc.Get(ctx, "greeting").Result()
	require.NoError(t, err)
	assert.Equal(t, `"hello"`, got)

	ttl, err := rc.TTL(ctx, "greeting").Result()
	require.NoError(t, err)
	assert.Positive(t, ttl)

	// --- cache.exists (key present) ---
	existsWF := engine.WorkflowConfig{
		ID: "cache-exists",
		Nodes: map[string]engine.NodeConfig{
			"e": {
				Type:     "cache.exists",
				Services: map[string]string{"cache": "cache"},
				Config:   map[string]any{"key": "greeting"},
			},
		},
	}
	ectx := runWF(t, svcReg, nodeReg, existsWF)
	eout, ok := ectx.GetOutput("e")
	require.True(t, ok)
	// cache.exists returns map[string]any{"exists": bool}
	eoutMap, ok := eout.(map[string]any)
	require.True(t, ok, "cache.exists output should be map[string]any, got %T: %v", eout, eout)
	assert.Equal(t, true, eoutMap["exists"])

	// --- cache.del ---
	delWF := engine.WorkflowConfig{
		ID: "cache-del",
		Nodes: map[string]engine.NodeConfig{
			"d": {
				Type:     "cache.del",
				Services: map[string]string{"cache": "cache"},
				Config:   map[string]any{"key": "greeting"},
			},
		},
	}
	runWF(t, svcReg, nodeReg, delWF)

	n, err := rc.Exists(ctx, "greeting").Result()
	require.NoError(t, err)
	assert.Equal(t, int64(0), n)
}

func TestCacheSet_MissingService_Engine(t *testing.T) {
	// Register no service so the required dep is unmet.
	svcReg := registry.NewServiceRegistry()
	nodeReg := registry.NewNodeRegistry()
	require.NoError(t, nodeReg.RegisterFromPlugin(&Plugin{}))

	wf := engine.WorkflowConfig{
		ID: "cache-noservice",
		Nodes: map[string]engine.NodeConfig{
			"s": {
				Type:     "cache.set",
				Services: map[string]string{"cache": "missing"},
				Config:   map[string]any{"key": "k", "value": "v"},
			},
		},
	}
	graph, err := engine.Compile(wf, nodeReg)
	require.NoError(t, err)
	execCtx := engine.NewExecutionContext(engine.WithInput(nil))
	err = engine.ExecuteGraph(context.Background(), graph, execCtx, svcReg, nodeReg)
	require.Error(t, err)
}
