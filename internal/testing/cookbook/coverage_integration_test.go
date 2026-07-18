//go:build integration

package cookbook

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chimpanze/noda/internal/registry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCookbookCoverage is the project's completion gate: every node type the
// full plugin set registers must (a) appear in at least one cookbook project
// workflow and (b) have a docs page linking its cookbook family. New nodes
// fail CI here until they get an example.
func TestCookbookCoverage(t *testing.T) {
	reg := registry.NewNodeRegistry()
	for _, p := range cookbookPlugins() {
		require.NoError(t, reg.RegisterFromPlugin(p))
	}
	types := reg.AllTypes()
	require.GreaterOrEqual(t, len(types), 81, "full plugin set should register all node types")

	// Collect every "type" used across cookbook workflows AND workers.
	used := map[string]bool{}
	for _, pattern := range []string{
		"../../../examples/node-cookbook/*/workflows/*.json",
		"../../../examples/node-cookbook/*/workers/*.json",
	} {
		files, err := filepath.Glob(pattern)
		require.NoError(t, err)
		for _, f := range files {
			raw, err := os.ReadFile(f)
			require.NoError(t, err)
			var doc map[string]any
			require.NoError(t, json.Unmarshal(raw, &doc), f)
			collectTypes(doc, used)
		}
	}

	var missingExample, missingDocLink []string
	for _, typ := range types {
		if !used[typ] {
			missingExample = append(missingExample, typ)
		}
		docPath := filepath.Join("../../../docs/03-nodes", typ+".md")
		raw, err := os.ReadFile(docPath)
		if err != nil {
			missingDocLink = append(missingDocLink, typ+" (no doc page)")
			continue
		}
		if !strings.Contains(string(raw), "node-cookbook/") {
			missingDocLink = append(missingDocLink, typ)
		}
	}
	assert.Empty(t, missingExample, "node types with no cookbook example")
	assert.Empty(t, missingDocLink, "node docs without a cookbook link")
}

// collectTypes walks a decoded workflow/worker JSON doc gathering node types.
func collectTypes(v any, out map[string]bool) {
	switch t := v.(type) {
	case map[string]any:
		if typ, ok := t["type"].(string); ok && strings.Contains(typ, ".") {
			out[typ] = true
		}
		for _, val := range t {
			collectTypes(val, out)
		}
	case []any:
		for _, val := range t {
			collectTypes(val, out)
		}
	}
}
