package config

import (
	"context"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/chimpanze/noda/internal/secrets"
)

func testdataPath(name string) string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "..", "..", "testdata", name)
}

func TestIntegration_ValidProject(t *testing.T) {
	dir := testdataPath("valid-project")

	sm := secrets.New()
	_ = sm.Load(context.Background())
	rc, errs := ValidateAll(dir, "development", sm)
	if len(errs) > 0 {
		t.Logf("Errors:\n%s", FormatErrors(errs))
	}
	assert.Empty(t, errs)
	require.NotNil(t, rc)
	assert.Equal(t, "development", rc.Environment)
	assert.NotNil(t, rc.Root)
	assert.True(t, len(rc.Routes) >= 3, "should have at least 3 routes")
	assert.True(t, len(rc.Workflows) >= 3, "should have at least 3 workflows")
	assert.Len(t, rc.Workers, 1)
	assert.Len(t, rc.Schedules, 1)
	assert.Len(t, rc.Connections, 1)
	assert.True(t, len(rc.Tests) >= 1, "should have at least 1 test")
}

func TestIntegration_InvalidProject(t *testing.T) {
	dir := testdataPath("invalid-project")

	sm := secrets.New()
	_ = sm.Load(context.Background())
	_, errs := ValidateAll(dir, "development", sm)
	require.NotEmpty(t, errs, "invalid project should produce errors")
	// The broken-syntax.json should cause a JSON parse error
	hasParseError := false
	for _, e := range errs {
		if contains(e.Message, "invalid JSON") || contains(e.Message, "broken-syntax") {
			hasParseError = true
		}
	}
	assert.True(t, hasParseError, "should have JSON parse error for broken-syntax.json")
}

func TestIntegration_MinimalProject(t *testing.T) {
	dir := testdataPath("minimal-project")

	sm := secrets.New()
	_ = sm.Load(context.Background())
	rc, errs := ValidateAll(dir, "development", sm)
	if len(errs) > 0 {
		t.Logf("Errors:\n%s", FormatErrors(errs))
	}
	assert.Empty(t, errs)
	require.NotNil(t, rc)
	assert.Len(t, rc.Routes, 1)
	assert.Len(t, rc.Workflows, 1)
}
