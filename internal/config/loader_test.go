package config

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadAll_ValidFiles(t *testing.T) {
	dir := setupTestProject(t, map[string]string{
		"noda.json":                  `{"services": {}}`,
		"routes/tasks.json":          `{"id": "tasks"}`,
		"workflows/create-task.json": `{"id": "create-task", "nodes": {}}`,
	})

	d, err := Discover(dir, "")
	require.NoError(t, err)

	rc, errs := LoadAll(d)
	assert.Empty(t, errs)
	assert.NotNil(t, rc.Root)
	assert.Contains(t, rc.Root, "services")
	assert.Len(t, rc.Routes, 1)
	assert.Len(t, rc.Workflows, 1)
}

func TestLoadAll_InvalidJSON(t *testing.T) {
	dir := setupTestProject(t, map[string]string{
		"noda.json":       `{"services": {}}`,
		"routes/bad.json": `{invalid json`,
	})

	d, err := Discover(dir, "")
	require.NoError(t, err)

	rc, errs := LoadAll(d)
	require.Len(t, errs, 1)
	assert.Contains(t, errs[0].Error(), "bad.json")
	assert.Contains(t, errs[0].Error(), "invalid JSON")
	// Root should still be loaded
	assert.NotNil(t, rc.Root)
}

func TestLoadAll_MultipleBrokenFiles(t *testing.T) {
	dir := setupTestProject(t, map[string]string{
		"noda.json":         `{bad}`,
		"routes/tasks.json": `{also bad}`,
		"routes/users.json": `{"valid": true}`,
	})

	d, err := Discover(dir, "")
	require.NoError(t, err)

	_, errs := LoadAll(d)
	assert.Len(t, errs, 2) // both broken files reported
}

func TestLoadAll_EmptyJSON(t *testing.T) {
	dir := setupTestProject(t, map[string]string{
		"noda.json":         `{}`,
		"routes/empty.json": `{}`,
	})

	d, err := Discover(dir, "")
	require.NoError(t, err)

	rc, errs := LoadAll(d)
	assert.Empty(t, errs)
	assert.NotNil(t, rc.Root)
	assert.Len(t, rc.Routes, 1)
}

func TestLoadAll_LargeFile(t *testing.T) {
	// Generate a large JSON object
	largeJSON := `{"items": [`
	for i := 0; i < 1000; i++ {
		if i > 0 {
			largeJSON += ","
		}
		largeJSON += `{"id": ` + string(rune('0'+i%10)) + `}`
	}
	largeJSON += `]}`

	dir := setupTestProject(t, map[string]string{
		"noda.json": largeJSON,
	})

	d, err := Discover(dir, "")
	require.NoError(t, err)

	rc, errs := LoadAll(d)
	assert.Empty(t, errs)
	assert.NotNil(t, rc.Root)
}

func TestLoadAll_WithOverlay(t *testing.T) {
	dir := setupTestProject(t, map[string]string{
		"noda.json":             `{"port": 3000}`,
		"noda.development.json": `{"port": 8080}`,
	})

	d, err := Discover(dir, "development")
	require.NoError(t, err)

	rc, errs := LoadAll(d)
	assert.Empty(t, errs)
	assert.NotNil(t, rc.Root)
	assert.NotNil(t, rc.Overlay)
	assert.Equal(t, float64(8080), rc.Overlay["port"])
}

func TestLoadAll_BOMStripped(t *testing.T) {
	dir := setupTestProject(t, map[string]string{
		"noda.json": "\xEF\xBB\xBF" + `{"bom": true}`,
	})

	d, err := Discover(dir, "")
	require.NoError(t, err)

	rc, errs := LoadAll(d)
	assert.Empty(t, errs)
	assert.Equal(t, true, rc.Root["bom"])
}

func TestLoadAll_WithVarsFile(t *testing.T) {
	dir := setupTestProject(t, map[string]string{
		"noda.json": `{}`,
		"vars.json": `{"api_key": "secret123", "base_url": "https://example.com"}`,
	})

	d, err := Discover(dir, "")
	require.NoError(t, err)

	rc, errs := LoadAll(d)
	assert.Empty(t, errs)
	assert.Equal(t, "secret123", rc.Vars["api_key"])
	assert.Equal(t, "https://example.com", rc.Vars["base_url"])
}

func TestLoadAll_VarsNonStringValue(t *testing.T) {
	dir := setupTestProject(t, map[string]string{
		"noda.json": `{}`,
		"vars.json": `{"port": 3000}`,
	})

	d, err := Discover(dir, "")
	require.NoError(t, err)

	_, errs := LoadAll(d)
	require.Len(t, errs, 1)
	assert.Contains(t, errs[0].Error(), "must be a string")
}

func TestLoadAll_ErrorIncludesFilePath(t *testing.T) {
	dir := setupTestProject(t, map[string]string{
		"noda.json":       `{}`,
		"routes/bad.json": `{invalid}`,
	})

	d, err := Discover(dir, "")
	require.NoError(t, err)

	_, errs := LoadAll(d)
	require.Len(t, errs, 1)
	assert.Contains(t, errs[0].Error(), filepath.Join(dir, "routes", "bad.json"))
}
