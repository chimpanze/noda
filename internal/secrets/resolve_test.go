package secrets

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestManager(vals map[string]string) *Manager {
	m := New(&staticProvider{name: "test", values: vals})
	_ = m.Load(context.Background())
	return m
}

func TestResolve_Simple(t *testing.T) {
	m := newTestManager(map[string]string{"DB_HOST": "localhost"})

	result, errs := m.Resolve(map[string]any{
		"url": "{{ $env('DB_HOST') }}",
	})
	assert.Empty(t, errs)
	assert.Equal(t, "localhost", result["url"])
}

func TestResolve_MultipleInOneString(t *testing.T) {
	m := newTestManager(map[string]string{"DB_HOST": "localhost", "DB_PORT": "5432"})

	result, errs := m.Resolve(map[string]any{
		"url": "postgres://{{ $env('DB_HOST') }}:{{ $env('DB_PORT') }}/main",
	})
	assert.Empty(t, errs)
	assert.Equal(t, "postgres://localhost:5432/main", result["url"])
}

func TestResolve_Nested(t *testing.T) {
	m := newTestManager(map[string]string{"SECRET": "abc123"})

	result, errs := m.Resolve(map[string]any{
		"services": map[string]any{
			"db": map[string]any{
				"config": map[string]any{
					"password": "{{ $env('SECRET') }}",
				},
			},
		},
	})
	assert.Empty(t, errs)
	pw := result["services"].(map[string]any)["db"].(map[string]any)["config"].(map[string]any)["password"]
	assert.Equal(t, "abc123", pw)
}

func TestResolve_MissingVar(t *testing.T) {
	m := newTestManager(map[string]string{})

	_, errs := m.Resolve(map[string]any{
		"services": map[string]any{
			"db": map[string]any{
				"url": "{{ $env('MISSING_VAR') }}",
			},
		},
	})
	require.Len(t, errs, 1)
	assert.Contains(t, errs[0].Error(), "MISSING_VAR")
	assert.Contains(t, errs[0].Error(), "services.db.url")
}

func TestResolve_MultipleMissingVars(t *testing.T) {
	m := newTestManager(map[string]string{})

	_, errs := m.Resolve(map[string]any{
		"a": "{{ $env('MISSING_A') }}",
		"b": "{{ $env('MISSING_B') }}",
	})
	assert.Len(t, errs, 2)
}

func TestResolve_NonStringValuesUntouched(t *testing.T) {
	m := newTestManager(map[string]string{})

	result, errs := m.Resolve(map[string]any{
		"port":    float64(3000),
		"debug":   true,
		"nothing": nil,
	})
	assert.Empty(t, errs)
	assert.Equal(t, float64(3000), result["port"])
	assert.Equal(t, true, result["debug"])
}

func TestResolve_InArray(t *testing.T) {
	m := newTestManager(map[string]string{"ORIGIN": "https://example.com"})

	result, errs := m.Resolve(map[string]any{
		"origins": []any{"{{ $env('ORIGIN') }}", "http://localhost"},
	})
	assert.Empty(t, errs)
	assert.Equal(t, []any{"https://example.com", "http://localhost"}, result["origins"])
}

func TestEnvPattern(t *testing.T) {
	p := envPatternRe()
	assert.NotNil(t, p)
	assert.True(t, p.MatchString("{{ $env('KEY') }}"))
	assert.False(t, p.MatchString("{{ $var('KEY') }}"))
}
