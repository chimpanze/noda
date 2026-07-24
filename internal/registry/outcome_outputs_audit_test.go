package registry_test

import (
	"testing"

	"github.com/chimpanze/noda/internal/registry"
	"github.com/chimpanze/noda/pkg/api"
	"github.com/chimpanze/noda/plugins/all"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestOutcomeOutputsAudit checks, for every node type the runtime registers
// (plugins/all — the same list cmd/noda consumes, #384):
//  1. the 8 node types with a documented outcome output declare it via
//     api.OutcomeOutputsProvider, exactly;
//  2. any provider's declared outcome outputs are a subset of the node's
//     Outputs() and never "success"/"error" (those have their own rules).
func TestOutcomeOutputsAudit(t *testing.T) {
	nodes := registry.NewNodeRegistry()
	for _, p := range all.All() {
		require.NoError(t, nodes.RegisterFromPlugin(p))
	}

	expected := map[string][]string{
		"db.create":               {"exists"},
		"db.update":               {"exists"},
		"db.upsert":               {"exists"},
		"auth.create_user":        {"exists"},
		"auth.get_user":           {"not_found"},
		"auth.set_password":       {"invalid"},
		"auth.verify_credentials": {"invalid"},
		"auth.consume_token":      {"invalid"},
	}

	for _, nodeType := range nodes.AllTypes() {
		desc, ok := nodes.GetDescriptor(nodeType)
		require.True(t, ok, nodeType)
		provider, isProvider := desc.(api.OutcomeOutputsProvider)

		if want, shouldDeclare := expected[nodeType]; shouldDeclare {
			require.True(t, isProvider, "node %q must implement OutcomeOutputsProvider", nodeType)
			assert.Equal(t, want, provider.OutcomeOutputs(), "node %q outcome outputs", nodeType)
		}
		if !isProvider {
			continue
		}
		outputs, ok := nodes.OutputsForType(nodeType)
		require.True(t, ok, nodeType)
		for _, oo := range provider.OutcomeOutputs() {
			assert.Contains(t, outputs, oo, "node %q declares outcome output %q not in Outputs()", nodeType, oo)
			assert.NotEqual(t, "success", oo, "node %q: success is not an outcome output", nodeType)
			assert.NotEqual(t, "error", oo, "node %q: error has its own unwired rule in the engine", nodeType)
		}
	}
}
