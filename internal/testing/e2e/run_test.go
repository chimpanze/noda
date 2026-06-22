package e2e

import (
	"testing"

	"github.com/chimpanze/noda/internal/config"
	"github.com/chimpanze/noda/internal/registry"
	nodatesting "github.com/chimpanze/noda/internal/testing"
	"github.com/chimpanze/noda/pkg/api"
	"github.com/chimpanze/noda/plugins/core/control"
	"github.com/chimpanze/noda/plugins/core/response"
	"github.com/chimpanze/noda/plugins/core/transform"
	"github.com/chimpanze/noda/plugins/core/util"
	workflowplugin "github.com/chimpanze/noda/plugins/core/workflow"
	"github.com/stretchr/testify/require"
)

// TestNodeE2E loads the testdata/node-e2e project and runs every test suite
// through the real engine, failing on any case failure.
func TestNodeE2E(t *testing.T) {
	const dir = "../../../testdata/node-e2e"

	sm, err := config.NewSecretsManager(dir, "")
	require.NoError(t, err)

	rc, errs := config.ValidateAll(dir, "", sm)
	require.Empty(t, errs, "config validation errors: %v", errs)

	coreReg := registry.NewNodeRegistry()
	for _, p := range []api.Plugin{
		&control.Plugin{},
		&transform.Plugin{},
		&response.Plugin{},
		&util.Plugin{},
		&workflowplugin.Plugin{},
	} {
		require.NoError(t, coreReg.RegisterFromPlugin(p))
	}

	suites, err := nodatesting.LoadTests(rc)
	require.NoError(t, err)
	require.NotEmpty(t, suites, "no test suites found in testdata/node-e2e/tests")

	for _, s := range suites {
		for _, r := range nodatesting.RunTestSuite(s, rc, coreReg) {
			if !r.Passed {
				t.Errorf("[workflow %s] case %q failed: %s", s.Workflow, r.CaseName, r.Error)
			}
		}
	}
}
