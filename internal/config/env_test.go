package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDetectEnvironment_FlagPriority(t *testing.T) {
	t.Setenv("NODA_ENV", "production")

	env, err := DetectEnvironment("staging")
	require.NoError(t, err)
	assert.Equal(t, "staging", env)
}

func TestDetectEnvironment_EnvVar(t *testing.T) {
	t.Setenv("NODA_ENV", "production")

	env, err := DetectEnvironment("")
	require.NoError(t, err)
	assert.Equal(t, "production", env)
}

func TestDetectEnvironment_Default(t *testing.T) {
	t.Setenv("NODA_ENV", "")

	env, err := DetectEnvironment("")
	require.NoError(t, err)
	assert.Equal(t, "development", env)
}

func TestDetectEnvironment_InvalidName(t *testing.T) {
	_, err := DetectEnvironment("../../etc/passwd")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid environment name")

	_, err = DetectEnvironment("dev.local")
	assert.Error(t, err)

	_, err = DetectEnvironment("dev local")
	assert.Error(t, err)
}

func TestDetectEnvironment_ValidNames(t *testing.T) {
	for _, name := range []string{"development", "production", "staging", "pre-prod", "test-123"} {
		env, err := DetectEnvironment(name)
		require.NoError(t, err)
		assert.Equal(t, name, env)
	}
}
