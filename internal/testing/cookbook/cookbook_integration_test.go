//go:build integration

package cookbook

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/chimpanze/noda/pkg/api"
	"github.com/chimpanze/noda/plugins/core/control"
	"github.com/chimpanze/noda/plugins/core/response"
	"github.com/chimpanze/noda/plugins/core/transform"
	"github.com/chimpanze/noda/plugins/core/util"
	workflowplugin "github.com/chimpanze/noda/plugins/core/workflow"
	"github.com/stretchr/testify/require"
)

// cookbookPlugins is the plugin set available to cookbook projects. Extended
// per-family as later tranches add service-backed families.
func cookbookPlugins() []api.Plugin {
	return []api.Plugin{
		&control.Plugin{},
		&transform.Plugin{},
		&response.Plugin{},
		&util.Plugin{},
		&workflowplugin.Plugin{},
	}
}

// TestCookbook replays every cookbook project's verify.json against the real
// in-process server.
func TestCookbook(t *testing.T) {
	dirs, err := filepath.Glob("../../../examples/node-cookbook/*")
	require.NoError(t, err)

	ran := 0
	for _, dir := range dirs {
		if _, err := os.Stat(filepath.Join(dir, "verify.json")); err != nil {
			continue
		}
		ran++
		t.Run(filepath.Base(dir), func(t *testing.T) {
			RunProject(t, dir, cookbookPlugins())
		})
	}
	require.NotZero(t, ran, "no cookbook projects found — wrong path?")
}
