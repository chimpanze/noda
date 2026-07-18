package testing

import (
	"testing"

	"github.com/chimpanze/noda/internal/config"
	"github.com/chimpanze/noda/internal/registry"
	"github.com/chimpanze/noda/pkg/api"
	"github.com/chimpanze/noda/plugins/core/transform"
	workflowplugin "github.com/chimpanze/noda/plugins/core/workflow"
	"github.com/stretchr/testify/require"
)

func coreRegForSubWf(t *testing.T) *registry.NodeRegistry {
	t.Helper()
	reg := registry.NewNodeRegistry()
	for _, p := range []api.Plugin{&transform.Plugin{}, &workflowplugin.Plugin{}} {
		require.NoError(t, reg.RegisterFromPlugin(p))
	}
	return reg
}

func TestRunner_WorkflowRunSubWorkflow(t *testing.T) {
	rc := &config.ResolvedConfig{
		Workflows: map[string]map[string]any{
			"callee": {
				"id": "callee",
				"nodes": map[string]any{
					"calc": map[string]any{
						"type":   "transform.set",
						"config": map[string]any{"fields": map[string]any{"result": "{{ input.x * 10 }}"}},
					},
					"out": map[string]any{
						"type":   "workflow.output",
						"config": map[string]any{"name": "result", "data": "{{ nodes.calc }}"},
					},
				},
				"edges": []any{map[string]any{"from": "calc", "to": "out"}},
			},
			"caller": {
				"id": "caller",
				"nodes": map[string]any{
					"call": map[string]any{
						"type":   "workflow.run",
						"config": map[string]any{"workflow": "callee", "input": map[string]any{"x": "{{ input.x }}"}},
					},
				},
				"edges": []any{},
			},
		},
	}

	suite := TestSuite{
		Workflow: "caller",
		Cases: []TestCase{{
			Name:  "calls sub-workflow",
			Input: map[string]any{"x": float64(3)},
			Expect: TestExpectation{
				Status: "success",
				Output: map[string]any{"call.result": float64(30)},
			},
		}},
	}

	results := RunTestSuite(suite, rc, coreRegForSubWf(t), nil)
	require.Len(t, results, 1)
	require.True(t, results[0].Passed, "expected pass, got: %s", results[0].Error)
}
