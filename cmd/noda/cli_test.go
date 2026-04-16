package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestScaffoldProject(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "myapp")

	err := scaffoldProject(dir)
	require.NoError(t, err)

	// Verify directories
	for _, d := range []string{"routes", "workflows", "schemas", "tests", "migrations", ".claude"} {
		info, err := os.Stat(filepath.Join(dir, d))
		require.NoError(t, err, "directory %s should exist", d)
		assert.True(t, info.IsDir())
	}

	// Verify files exist and are non-empty
	files := []string{
		"noda.json",
		".env.example",
		"docker-compose.yml",
		"routes/api.json",
		"workflows/hello.json",
		"schemas/greeting.json",
		"tests/hello.test.json",
		"README.md",
		"CLAUDE.md",
		".claude/settings.json",
	}
	for _, f := range files {
		data, err := os.ReadFile(filepath.Join(dir, f))
		require.NoError(t, err, "file %s should exist", f)
		assert.NotEmpty(t, data, "file %s should not be empty", f)
	}
}

func TestScaffoldProject_NodaJSONValid(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "myapp")
	require.NoError(t, scaffoldProject(dir))

	// noda.json should be valid JSON
	data, err := os.ReadFile(filepath.Join(dir, "noda.json"))
	require.NoError(t, err)
	assert.Contains(t, string(data), `"server"`)
	assert.Contains(t, string(data), `"port"`)
	assert.Contains(t, string(data), `"services"`)
}

func TestScaffoldProject_DockerComposeValid(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "myapp")
	require.NoError(t, scaffoldProject(dir))

	data, err := os.ReadFile(filepath.Join(dir, "docker-compose.yml"))
	require.NoError(t, err)
	assert.Contains(t, string(data), "postgres")
	assert.Contains(t, string(data), "redis")
}

func TestScaffoldProject_SampleWorkflow(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "myapp")
	require.NoError(t, scaffoldProject(dir))

	data, err := os.ReadFile(filepath.Join(dir, "workflows/hello.json"))
	require.NoError(t, err)
	assert.Contains(t, string(data), `"nodes"`)
	assert.Contains(t, string(data), `"edges"`)
	assert.Contains(t, string(data), `transform.set`)
}

func TestScaffoldProject_ReadmeContainsName(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "cool-api")
	require.NoError(t, scaffoldProject(dir))

	data, err := os.ReadFile(filepath.Join(dir, "README.md"))
	require.NoError(t, err)
	assert.Contains(t, string(data), "cool-api")

	data, err = os.ReadFile(filepath.Join(dir, "CLAUDE.md"))
	require.NoError(t, err)
	assert.Contains(t, string(data), "cool-api")
}

func TestScaffoldProject_AIAssistance(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "myapp")
	require.NoError(t, scaffoldProject(dir))

	// CLAUDE.md should exist and contain project guidance
	data, err := os.ReadFile(filepath.Join(dir, "CLAUDE.md"))
	require.NoError(t, err)
	assert.Contains(t, string(data), "Noda project")
	assert.Contains(t, string(data), "MCP Tools")

	// .claude/settings.json should exist with MCP config
	data, err = os.ReadFile(filepath.Join(dir, ".claude", "settings.json"))
	require.NoError(t, err)
	assert.Contains(t, string(data), `"noda"`)
	assert.Contains(t, string(data), `"command"`)
	assert.Contains(t, string(data), `"mcp"`)
}

func TestScaffoldProject_NodaJSONHasDefaultGlobalMiddleware(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "myapp")
	require.NoError(t, scaffoldProject(dir))

	data, err := os.ReadFile(filepath.Join(dir, "noda.json"))
	require.NoError(t, err)

	var cfg map[string]any
	require.NoError(t, json.Unmarshal(data, &cfg))

	gm, ok := cfg["global_middleware"].([]any)
	require.True(t, ok, "noda.json should declare global_middleware as an array, got %T", cfg["global_middleware"])

	names := make([]string, 0, len(gm))
	for _, v := range gm {
		s, ok := v.(string)
		require.True(t, ok, "global_middleware entry %v should be a string", v)
		names = append(names, s)
	}

	assert.Equal(t, []string{"recover", "requestid", "logger"}, names)
}

func TestScaffoldProject_DuplicateIsIdempotent(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "myapp")
	require.NoError(t, scaffoldProject(dir))
	// Second call should overwrite without error
	require.NoError(t, scaffoldProject(dir))
}
