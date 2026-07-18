package server

import (
	"testing"

	"github.com/chimpanze/noda/internal/config"
	"github.com/chimpanze/noda/internal/registry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSetup_BuildsSubWorkflowRunnerWithoutInjectedCache covers #359: when no
// workflow cache is injected via WithWorkflowCache, NewServer's runner-wiring
// block is skipped (s.workflows is nil at that point), and Setup() must wire
// subWorkflowRunner from the cache it self-builds instead.
func TestSetup_BuildsSubWorkflowRunnerWithoutInjectedCache(t *testing.T) {
	rc := &config.ResolvedConfig{
		Root: map[string]any{},
		Workflows: map[string]map[string]any{
			"workflows/child.json": {
				"id":    "child",
				"nodes": map[string]any{"log": map[string]any{"type": "util.log", "config": map[string]any{"message": "hi"}}},
				"edges": []any{},
			},
		},
	}
	srv, err := NewServer(rc, registry.NewServiceRegistry(), buildTestNodeRegistry())
	require.NoError(t, err)
	require.Nil(t, srv.subWorkflowRunner, "precondition: no runner before Setup without injected cache")
	require.NoError(t, srv.Setup())
	assert.NotNil(t, srv.subWorkflowRunner, "Setup must wire the sub-workflow runner from its self-built cache (#359)")
}
