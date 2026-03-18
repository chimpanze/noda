package secrets

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDotEnvProvider_BasicLoad(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".env"), []byte("TEST_KEY=hello\nTEST_OTHER=world\n"), 0644))

	p := &DotEnvProvider{ConfigDir: dir}
	vals, err := p.Load(context.Background())
	require.NoError(t, err)

	assert.Equal(t, "hello", vals["TEST_KEY"])
	assert.Equal(t, "world", vals["TEST_OTHER"])
}

func TestDotEnvProvider_WithEnvironment(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".env"), []byte("BASE=base_val\nSHARED=from_base\n"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".env.production"), []byte("PROD_ONLY=prod_val\nSHARED=from_prod\n"), 0644))

	p := &DotEnvProvider{ConfigDir: dir, Env: "production"}
	vals, err := p.Load(context.Background())
	require.NoError(t, err)

	assert.Equal(t, "base_val", vals["BASE"])
	assert.Equal(t, "prod_val", vals["PROD_ONLY"])
	assert.Equal(t, "from_prod", vals["SHARED"], "env-specific file should override base")
}

func TestDotEnvProvider_MissingFiles(t *testing.T) {
	dir := t.TempDir()
	p := &DotEnvProvider{ConfigDir: dir}
	vals, err := p.Load(context.Background())
	require.NoError(t, err)
	assert.Empty(t, vals)
}

func TestDotEnvProvider_DoesNotPollute(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".env"), []byte("SECRETS_TEST_NOPOLLUTE=should_not_be_in_env\n"), 0644))

	p := &DotEnvProvider{ConfigDir: dir}
	vals, err := p.Load(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "should_not_be_in_env", vals["SECRETS_TEST_NOPOLLUTE"])

	// Must NOT be in process environment
	_, found := os.LookupEnv("SECRETS_TEST_NOPOLLUTE")
	assert.False(t, found, "DotEnvProvider should use Read() not Load() — must not pollute os.Environ()")
}

func TestProcessEnvProvider(t *testing.T) {
	t.Setenv("SECRETS_TEST_PROC_ENV", "from_process")

	p := &ProcessEnvProvider{}
	vals, err := p.Load(context.Background())
	require.NoError(t, err)

	assert.Equal(t, "from_process", vals["SECRETS_TEST_PROC_ENV"])
	// Should include standard env vars
	_, hasPath := vals["PATH"]
	assert.True(t, hasPath, "ProcessEnvProvider should include all os.Environ() values")
}
