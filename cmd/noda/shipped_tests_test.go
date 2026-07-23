package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/chimpanze/noda/internal/config"
	nodatesting "github.com/chimpanze/noda/internal/testing"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// runProjectTestSuites runs every workflow test suite a project ships through
// the real `noda test` runner and asserts each case passes. It mirrors exactly
// what the test command does (main.go newTestCmd), so a green run here means a
// user running `noda test` in that directory sees green too.
func runProjectTestSuites(t *testing.T, dir string) {
	t.Helper()

	sm, err := config.NewSecretsManager(dir, "")
	require.NoError(t, err)

	rc, errs := config.ValidateAll(dir, "", sm)
	require.Empty(t, errs, "project must pass config validation before its tests can run")

	suites, err := nodatesting.LoadTests(rc)
	require.NoError(t, err)
	require.NotEmpty(t, suites, "expected at least one test suite in %s/tests", dir)

	reg, err := buildCoreNodeRegistry()
	require.NoError(t, err)

	for _, suite := range suites {
		for _, res := range nodatesting.RunTestSuite(suite, rc, reg, sm.ExpressionContext()) {
			assert.Truef(t, res.Passed, "suite %q case %q failed: %s", suite.ID, res.CaseName, res.Error)
		}
	}
}

// The project `noda init` scaffolds must pass its own shipped tests. Users run
// `noda init` then `noda test` as their first two commands; a red result there
// is the worst possible first impression.
func TestScaffoldedProjectPassesItsOwnTests(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "myapp")
	require.NoError(t, scaffoldProject(dir, true))

	runProjectTestSuites(t, dir)
}

// An example that references $env() is unrunnable without knowing which
// variables to set — `noda test` and `noda dev` both stop at config
// validation. Every such example must ship a .env.example naming them.
func TestExamplesDocumentTheirEnvVars(t *testing.T) {
	dirs, err := filepath.Glob("../../examples/*")
	require.NoError(t, err)

	for _, dir := range dirs {
		if _, err := os.Stat(filepath.Join(dir, "noda.json")); err != nil {
			continue
		}
		needed := envForDir[filepath.Base(dir)]
		if len(needed) == 0 {
			continue
		}
		t.Run(filepath.Base(dir), func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join(dir, ".env.example"))
			require.NoErrorf(t, err, "example references $env() but ships no .env.example")
			for _, name := range needed {
				assert.Containsf(t, string(data), name+"=",
					".env.example must document %s", name)
			}
		})
	}
}

// Every shipped example that ships a tests/ directory must pass those tests.
// TestShippedProjectsValidate only proves an example validates and dry-run
// boots; it never executes the workflow test suites, which is how stale
// expectations survived in the examples.
func TestShippedExamplesPassTheirTests(t *testing.T) {
	dirs, err := filepath.Glob("../../examples/*")
	require.NoError(t, err)
	cookbookDirs, err := filepath.Glob("../../examples/node-cookbook/*")
	require.NoError(t, err)
	dirs = append(dirs, cookbookDirs...)

	for _, dir := range dirs {
		if _, err := os.Stat(filepath.Join(dir, "noda.json")); err != nil {
			continue
		}
		if _, err := os.Stat(filepath.Join(dir, "tests")); err != nil {
			continue
		}
		t.Run(filepath.Base(dir), func(t *testing.T) {
			for _, name := range envForDir[filepath.Base(dir)] {
				t.Setenv(name, "dummy")
			}
			runProjectTestSuites(t, dir)
		})
	}
}
