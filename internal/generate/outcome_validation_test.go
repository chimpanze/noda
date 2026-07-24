package generate

import (
	"strings"
	"testing"

	"github.com/chimpanze/noda/internal/config"
	nodeexpr "github.com/chimpanze/noda/internal/expr"
	"github.com/chimpanze/noda/internal/registry"
	"github.com/chimpanze/noda/plugins/all"
	"github.com/stretchr/testify/require"
)

// TestGenerateCRUD_OutcomeOutputsWired is a closed-loop check for #442: the
// CRUD generator's output must satisfy the same outcome-output validation
// rule it will be run through at `noda validate`/boot. crud.go wires an
// "exists" edge for db.create and db.update (see the "conflict" nodes and
// edges around crud.go:388-414 and :547-574) precisely so scaffolded
// projects don't ship an unwired outcome output out of the box; this test
// feeds the generated workflows through the real validator to keep that
// true as both sides evolve.
func TestGenerateCRUD_OutcomeOutputsWired(t *testing.T) {
	model := map[string]any{
		"table": "widgets",
		"columns": map[string]any{
			"id":   map[string]any{"type": "uuid", "primary_key": true, "default": "gen_random_uuid()"},
			"name": map[string]any{"type": "text", "not_null": true},
		},
	}

	result := GenerateCRUD(model, CRUDOptions{})

	plugins := registry.NewPluginRegistry()
	nodes := registry.NewNodeRegistry()
	for _, p := range all.Core() {
		require.NoError(t, plugins.Register(p))
		require.NoError(t, nodes.RegisterFromPlugin(p))
	}

	rc := &config.ResolvedConfig{Workflows: map[string]map[string]any{}}
	for path, content := range result.Files {
		if !strings.HasPrefix(path, "workflows/") {
			continue
		}
		rc.Workflows[path] = content
	}
	require.NotEmpty(t, rc.Workflows, "generator produced no workflow files")

	errs := registry.ValidateStartupDryRun(rc, plugins, nodes, nodeexpr.NewCompilerWithFunctions(), nil)
	for _, err := range errs {
		require.NotContains(t, err.Error(), "outcome output",
			"generated workflow left an outcome output unwired: %v", err)
	}
}
