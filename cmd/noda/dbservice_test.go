package main

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIsDatabaseService(t *testing.T) {
	require.True(t, isDatabaseService(map[string]any{"plugin": "db"}))
	require.True(t, isDatabaseService(map[string]any{"plugin": "postgres"}))
	require.False(t, isDatabaseService(map[string]any{"plugin": "cache"}))
	require.False(t, isDatabaseService(map[string]any{}))
}

func TestFindServicesByPlugin_AcceptsPostgresForDB(t *testing.T) {
	services := map[string]any{"maindb": map[string]any{"plugin": "postgres", "config": map[string]any{"driver": "postgres"}}}
	names, _ := findServicesByPlugin(services, "db")
	require.Equal(t, []string{"maindb"}, names)
}

func TestPostgresServiceNames_AcceptsDBPlugin(t *testing.T) {
	services := map[string]any{"maindb": map[string]any{"plugin": "db", "config": map[string]any{"url": "postgres://x"}}}
	require.Equal(t, []string{"maindb"}, postgresServiceNames(services))
}
