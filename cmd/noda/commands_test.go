package main

import (
	"os"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testRootCmd creates a root command with persistent flags, matching main().
func testRootCmd(sub *cobra.Command) *cobra.Command {
	root := &cobra.Command{Use: "noda", SilenceUsage: true}
	root.PersistentFlags().String("env", "", "runtime environment")
	root.PersistentFlags().String("config", ".", "path to config directory")
	root.AddCommand(sub)
	return root
}

// --- Command construction and flag parsing ---

func TestNewValidateCmd_FlagParsing(t *testing.T) {
	cmd := newValidateCmd()
	assert.Equal(t, "validate", cmd.Use)

	// Should accept --verbose flag
	require.NoError(t, cmd.Flags().Parse([]string{"--verbose"}))
	verbose, err := cmd.Flags().GetBool("verbose")
	require.NoError(t, err)
	assert.True(t, verbose)
}

func TestNewTestCmd_FlagParsing(t *testing.T) {
	cmd := newTestCmd()
	assert.Equal(t, "test", cmd.Use)

	// Should accept --verbose and --workflow flags
	require.NoError(t, cmd.Flags().Parse([]string{"--verbose", "--workflow", "hello"}))

	verbose, err := cmd.Flags().GetBool("verbose")
	require.NoError(t, err)
	assert.True(t, verbose)

	wf, err := cmd.Flags().GetString("workflow")
	require.NoError(t, err)
	assert.Equal(t, "hello", wf)
}

func TestNewStartCmd_FlagParsing(t *testing.T) {
	cmd := newStartCmd()
	assert.Equal(t, "start", cmd.Use)
}

func TestNewDevCmd_FlagParsing(t *testing.T) {
	cmd := newDevCmd()
	assert.Equal(t, "dev", cmd.Use)
}

func TestNewGenerateCmd_Exists(t *testing.T) {
	cmd := newGenerateCmd()
	assert.Equal(t, "generate", cmd.Use)
}

func TestNewMigrateCmd_HasSubcommands(t *testing.T) {
	cmd := newMigrateCmd()
	assert.Equal(t, "migrate", cmd.Use)
	assert.NotEmpty(t, cmd.Commands(), "migrate should have subcommands")
}

func TestNewScheduleCmd_Exists(t *testing.T) {
	cmd := newScheduleCmd()
	assert.Equal(t, "schedule", cmd.Use)
}

// --- Integration: validate and test against testdata ---

func TestValidateCmd_RunsAgainstTestdata(t *testing.T) {
	root := testRootCmd(newValidateCmd())
	root.SetArgs([]string{"validate", "--config", "../../testdata/minimal-project"})

	err := root.Execute()
	assert.NoError(t, err)
}

func TestTestCmd_RunsAgainstTestdata(t *testing.T) {
	root := testRootCmd(newTestCmd())
	root.SetArgs([]string{"test", "--config", "../../testdata/valid-project"})

	err := root.Execute()
	assert.NoError(t, err)
}

func TestValidateCmd_InvalidProject(t *testing.T) {
	root := testRootCmd(newValidateCmd())
	root.SetArgs([]string{"validate", "--config", "../../testdata/invalid-project"})

	err := root.Execute()
	assert.Error(t, err)
}

func TestValidateCmd_Verbose(t *testing.T) {
	root := testRootCmd(newValidateCmd())
	root.SetArgs([]string{"validate", "--verbose", "--config", "../../testdata/minimal-project"})

	err := root.Execute()
	assert.NoError(t, err)
}

func TestValidateCmd_NonexistentDir(t *testing.T) {
	root := testRootCmd(newValidateCmd())
	root.SetArgs([]string{"validate", "--config", "/nonexistent/path"})

	err := root.Execute()
	assert.Error(t, err)
}

func TestGenerateOpenAPICmd_RunsAgainstTestdata(t *testing.T) {
	root := testRootCmd(newGenerateCmd())
	root.SetArgs([]string{"generate", "openapi", "--config", "../../testdata/minimal-project"})

	err := root.Execute()
	assert.NoError(t, err)
}

func TestGenerateOpenAPICmd_OutputToFile(t *testing.T) {
	outFile := t.TempDir() + "/openapi.json"
	root := testRootCmd(newGenerateCmd())
	root.SetArgs([]string{"generate", "openapi", "--config", "../../testdata/minimal-project", "--output", outFile})

	err := root.Execute()
	require.NoError(t, err)

	// Verify file was created
	data, err := os.ReadFile(outFile)
	require.NoError(t, err)
	assert.Contains(t, string(data), "openapi")
}

func TestTestCmd_WithWorkflowFilter(t *testing.T) {
	root := testRootCmd(newTestCmd())
	root.SetArgs([]string{"test", "--config", "../../testdata/valid-project", "--workflow", "nonexistent"})

	err := root.Execute()
	// Should succeed even if no tests match the filter
	assert.NoError(t, err)
}

func TestTestCmd_VerboseMode(t *testing.T) {
	root := testRootCmd(newTestCmd())
	root.SetArgs([]string{"test", "--verbose", "--config", "../../testdata/valid-project"})

	err := root.Execute()
	assert.NoError(t, err)
}

// --- init command ---

func TestInitCmd_Scaffolds(t *testing.T) {
	dir := t.TempDir() + "/test-app"
	root := testRootCmd(newInitCmd())
	root.SetArgs([]string{"init", dir})

	err := root.Execute()
	require.NoError(t, err)

	// Verify key files exist
	_, err = os.Stat(dir + "/noda.json")
	assert.NoError(t, err)
	_, err = os.Stat(dir + "/routes/api.json")
	assert.NoError(t, err)
}

// --- plugin command ---

func TestPluginListCmd(t *testing.T) {
	root := testRootCmd(newPluginCmd())
	root.SetArgs([]string{"plugin", "list"})

	err := root.Execute()
	assert.NoError(t, err)
}

// --- completion command ---

func TestCompletionCmd_Bash(t *testing.T) {
	root := testRootCmd(newCompletionCmd())
	root.SetArgs([]string{"completion", "bash"})

	err := root.Execute()
	assert.NoError(t, err)
}

func TestCompletionCmd_Zsh(t *testing.T) {
	root := testRootCmd(newCompletionCmd())
	root.SetArgs([]string{"completion", "zsh"})

	err := root.Execute()
	assert.NoError(t, err)
}

func TestCompletionCmd_Fish(t *testing.T) {
	root := testRootCmd(newCompletionCmd())
	root.SetArgs([]string{"completion", "fish"})

	err := root.Execute()
	assert.NoError(t, err)
}
