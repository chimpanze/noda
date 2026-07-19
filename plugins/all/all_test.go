package all_test

import (
	"testing"

	"github.com/chimpanze/noda/plugins/all"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAllIsCorePlusServiceOnly pins All() = Core() + ServiceOnly() with no
// duplicate names, so the three consumers (cmd/noda runtime, internal/mcp,
// the ServiceConfigSchema audit) can never drift apart again (#384).
func TestAllIsCorePlusServiceOnly(t *testing.T) {
	core := all.Core()
	svcOnly := all.ServiceOnly()
	everything := all.All()
	require.Len(t, everything, len(core)+len(svcOnly))

	seen := map[string]bool{}
	for _, p := range everything {
		require.NotEmpty(t, p.Name())
		assert.False(t, seen[p.Name()], "duplicate plugin name %q", p.Name())
		seen[p.Name()] = true
	}
	// Spot-check the two categories.
	assert.True(t, seen["postgres"], "core plugin postgres (db) missing")
	for _, name := range []string{"stream", "pubsub", "storage"} {
		assert.True(t, seen[name], "service-only plugin %q missing", name)
	}
}
