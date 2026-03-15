package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveEnvVars_Simple(t *testing.T) {
	t.Setenv("DB_HOST", "localhost")

	config := map[string]any{
		"url": "{{ $env('DB_HOST') }}",
	}

	result, errs := resolveEnvVars(config)
	assert.Empty(t, errs)
	assert.Equal(t, "localhost", result["url"])
}

func TestResolveEnvVars_MultipleInOneString(t *testing.T) {
	t.Setenv("DB_HOST", "localhost")
	t.Setenv("DB_PORT", "5432")

	config := map[string]any{
		"url": "postgres://{{ $env('DB_HOST') }}:{{ $env('DB_PORT') }}/main",
	}

	result, errs := resolveEnvVars(config)
	assert.Empty(t, errs)
	assert.Equal(t, "postgres://localhost:5432/main", result["url"])
}

func TestResolveEnvVars_Nested(t *testing.T) {
	t.Setenv("SECRET", "abc123")

	config := map[string]any{
		"services": map[string]any{
			"db": map[string]any{
				"config": map[string]any{
					"password": "{{ $env('SECRET') }}",
				},
			},
		},
	}

	result, errs := resolveEnvVars(config)
	assert.Empty(t, errs)
	pw := result["services"].(map[string]any)["db"].(map[string]any)["config"].(map[string]any)["password"]
	assert.Equal(t, "abc123", pw)
}

func TestResolveEnvVars_MissingVar(t *testing.T) {
	config := map[string]any{
		"services": map[string]any{
			"db": map[string]any{
				"url": "{{ $env('MISSING_VAR') }}",
			},
		},
	}

	_, errs := resolveEnvVars(config)
	require.Len(t, errs, 1)
	assert.Contains(t, errs[0].Error(), "MISSING_VAR")
	assert.Contains(t, errs[0].Error(), "services.db.url")
}

func TestResolveEnvVars_MultipleMissingVars(t *testing.T) {
	config := map[string]any{
		"a": "{{ $env('MISSING_A') }}",
		"b": "{{ $env('MISSING_B') }}",
	}

	_, errs := resolveEnvVars(config)
	assert.Len(t, errs, 2)
}

func TestResolveEnvVars_NonStringValuesUntouched(t *testing.T) {
	config := map[string]any{
		"port":    float64(3000),
		"debug":   true,
		"nothing": nil,
	}

	result, errs := resolveEnvVars(config)
	assert.Empty(t, errs)
	assert.Equal(t, float64(3000), result["port"])
	assert.Equal(t, true, result["debug"])
}

func TestResolveEnvVars_InArray(t *testing.T) {
	t.Setenv("ORIGIN", "https://example.com")

	config := map[string]any{
		"origins": []any{"{{ $env('ORIGIN') }}", "http://localhost"},
	}

	result, errs := resolveEnvVars(config)
	assert.Empty(t, errs)
	assert.Equal(t, []any{"https://example.com", "http://localhost"}, result["origins"])
}

func TestResolveEnvVarsSelective_OnlyResolvesRoot(t *testing.T) {
	t.Setenv("DB_URL", "postgres://localhost/test")

	rc := &RawConfig{
		Root: map[string]any{
			"services": map[string]any{
				"db": map[string]any{
					"url": "{{ $env('DB_URL') }}",
				},
			},
		},
		Routes: map[string]map[string]any{
			"routes/tasks.json": {
				"expr": "{{ $env('SHOULD_NOT_RESOLVE') }}",
			},
		},
	}

	errs := resolveEnvVarsSelective(rc)
	assert.Empty(t, errs)

	// Root should be resolved
	url := rc.Root["services"].(map[string]any)["db"].(map[string]any)["url"]
	assert.Equal(t, "postgres://localhost/test", url)

	// Routes should be untouched
	assert.Equal(t, "{{ $env('SHOULD_NOT_RESOLVE') }}", rc.Routes["routes/tasks.json"]["expr"])
}
