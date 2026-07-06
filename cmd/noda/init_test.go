package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestScaffoldProject_RefusesOverwrite(t *testing.T) {
	dir := t.TempDir()
	proj := filepath.Join(dir, "app")
	require.NoError(t, os.MkdirAll(proj, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(proj, "noda.json"), []byte("{}"), 0644))

	err := scaffoldProject(proj, false)
	require.Error(t, err)
	require.Contains(t, err.Error(), "noda.json")
	// existing file untouched, no partial scaffold
	b, _ := os.ReadFile(filepath.Join(proj, "noda.json"))
	require.Equal(t, "{}", string(b))
	// no partial scaffold: other template output files must not have been written
	_, err = os.Stat(filepath.Join(proj, "docker-compose.yml"))
	require.True(t, os.IsNotExist(err), "docker-compose.yml should not have been written")
}

func TestScaffoldProject_ForceOverwrites(t *testing.T) {
	dir := t.TempDir()
	proj := filepath.Join(dir, "app")
	require.NoError(t, os.MkdirAll(proj, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(proj, "noda.json"), []byte("{}"), 0644))
	require.NoError(t, scaffoldProject(proj, true))
	b, _ := os.ReadFile(filepath.Join(proj, "noda.json"))
	require.NotEqual(t, "{}", string(b)) // overwritten with the template
}
