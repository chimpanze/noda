package util

import (
	"context"
	"testing"
	"time"

	"github.com/chimpanze/noda/internal/engine"
	"github.com/chimpanze/noda/internal/registry"
	"github.com/chimpanze/noda/pkg/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type utilResolver struct{}

func (r *utilResolver) OutputsForType(nodeType string) ([]string, bool) {
	return api.DefaultOutputs(), true
}

func setupUtilIntegration(t *testing.T) (*registry.NodeRegistry, *registry.ServiceRegistry) {
	t.Helper()

	nodeReg := registry.NewNodeRegistry()
	require.NoError(t, nodeReg.RegisterFromPlugin(&Plugin{}))

	return nodeReg, registry.NewServiceRegistry()
}

func TestIntegration_UUIDAndTimestamp(t *testing.T) {
	nodeReg, svcReg := setupUtilIntegration(t)
	resolver := &utilResolver{}

	wf := engine.WorkflowConfig{
		ID: "uuid-timestamp",
		Nodes: map[string]engine.NodeConfig{
			"gen_id": {Type: "util.uuid"},
			"gen_ts": {Type: "util.timestamp", Config: map[string]any{"format": "iso8601"}},
		},
		Edges: []engine.EdgeConfig{
			{From: "gen_id", To: "gen_ts"},
		},
	}

	graph, err := engine.Compile(wf, resolver)
	require.NoError(t, err)

	execCtx := engine.NewExecutionContext()
	err = engine.ExecuteGraph(context.Background(), graph, execCtx, svcReg, nodeReg)
	require.NoError(t, err)

	// gen_id is an intermediate node whose output is evicted after gen_ts completes.
	// gen_ts is the terminal node — its output is preserved.
	ts, ok := execCtx.GetOutput("gen_ts")
	assert.True(t, ok)
	_, err = time.Parse(time.RFC3339, ts.(string))
	assert.NoError(t, err)

	// Verify UUID generation works by running it standalone
	uuidWf := engine.WorkflowConfig{
		ID:    "uuid-only",
		Nodes: map[string]engine.NodeConfig{"gen_id": {Type: "util.uuid"}},
	}
	uuidGraph, err := engine.Compile(uuidWf, resolver)
	require.NoError(t, err)
	uuidCtx := engine.NewExecutionContext()
	err = engine.ExecuteGraph(context.Background(), uuidGraph, uuidCtx, svcReg, nodeReg)
	require.NoError(t, err)
	id, ok := uuidCtx.GetOutput("gen_id")
	assert.True(t, ok)
	assert.NotEmpty(t, id.(string))
}

func TestIntegration_DelayWithTimeout(t *testing.T) {
	nodeReg, svcReg := setupUtilIntegration(t)
	resolver := &utilResolver{}

	wf := engine.WorkflowConfig{
		ID: "delay-timeout",
		Nodes: map[string]engine.NodeConfig{
			"wait": {Type: "util.delay", Config: map[string]any{"timeout": "5s"}},
		},
	}

	graph, err := engine.Compile(wf, resolver)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	execCtx := engine.NewExecutionContext()
	err = engine.ExecuteGraph(ctx, graph, execCtx, svcReg, nodeReg)
	// The delay node has an "error" output, so the timeout error is caught internally
	// and routed to the "error" output (no edge from it, so workflow completes)
	require.NoError(t, err)

	// The node output should contain the error info
	data, ok := execCtx.GetOutput("wait")
	assert.True(t, ok)
	errData := data.(map[string]any)
	assert.Contains(t, errData["error"].(string), "util.delay")
}

func TestIntegration_LogNode(t *testing.T) {
	nodeReg, svcReg := setupUtilIntegration(t)
	resolver := &utilResolver{}

	wf := engine.WorkflowConfig{
		ID: "log-test",
		Nodes: map[string]engine.NodeConfig{
			"log_it": {Type: "util.log", Config: map[string]any{
				"level":   "info",
				"message": "{{ \"processed user \" + input.name }}",
			}},
		},
	}

	graph, err := engine.Compile(wf, resolver)
	require.NoError(t, err)

	execCtx := engine.NewExecutionContext(engine.WithInput(map[string]any{"name": "Alice"}))
	err = engine.ExecuteGraph(context.Background(), graph, execCtx, svcReg, nodeReg)
	require.NoError(t, err)
}

func TestIntegration_PluginRegistration(t *testing.T) {
	plugins := registry.NewPluginRegistry()
	utilPlugin := &Plugin{}
	require.NoError(t, plugins.Register(utilPlugin))

	p, ok := plugins.Get("util")
	assert.True(t, ok)
	assert.Equal(t, "core.util", p.Name())

	nodeReg := registry.NewNodeRegistry()
	require.NoError(t, nodeReg.RegisterFromPlugin(utilPlugin))

	types := nodeReg.AllTypes()
	assert.Contains(t, types, "util.log")
	assert.Contains(t, types, "util.uuid")
	assert.Contains(t, types, "util.delay")
	assert.Contains(t, types, "util.timestamp")
}
