package main

import (
	"testing"

	"github.com/chimpanze/noda/internal/config"
	nodatesting "github.com/chimpanze/noda/internal/testing"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// validateProject must accept a project that `noda validate` accepts.
func TestValidateProject_AcceptsValidProject(t *testing.T) {
	dir := "../../testdata/valid-project"

	sm, err := config.NewSecretsManager(dir, "")
	require.NoError(t, err)
	rc, errs := config.ValidateAll(dir, "", sm)
	require.Empty(t, errs)

	require.NoError(t, validateProject(rc))
}

// testdata/test-cmd-invalid-project is the #444 trap in fixture form: its
// config parses, its workflow test passes, and it cannot boot. Asserting all
// three here is what makes TestTestCmd_FailsOnProjectThatDoesNotValidate
// meaningful — without the middle assertion, that test could pass for the
// boring reason that the fixture's tests were failing anyway.
func TestValidateProject_RejectsProjectThatCannotBoot(t *testing.T) {
	dir := "../../testdata/test-cmd-invalid-project"

	sm, err := config.NewSecretsManager(dir, "")
	require.NoError(t, err)

	rc, errs := config.ValidateAll(dir, "", sm)
	require.Empty(t, errs, "fixture must pass config validation — the gap only exists past that point")

	suites, err := nodatesting.LoadTests(rc)
	require.NoError(t, err)
	require.NotEmpty(t, suites)

	reg, err := buildCoreNodeRegistry()
	require.NoError(t, err)
	ran := 0
	for _, suite := range suites {
		for _, res := range nodatesting.RunTestSuite(suite, rc, reg, sm.ExpressionContext()) {
			ran++
			require.Truef(t, res.Passed,
				"fixture's own tests must pass: suite %q case %q: %s", suite.ID, res.CaseName, res.Error)
		}
	}
	require.Positive(t, ran, "fixture must actually define test cases — an empty tests array would make the assertion above vacuous")

	err = validateProject(rc)
	require.Error(t, err, "unwired outcome output must fail startup validation")
	assert.Contains(t, err.Error(), "outcome output")
	assert.Contains(t, err.Error(), "exists")
}
