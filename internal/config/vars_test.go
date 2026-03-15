package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveVars_Simple(t *testing.T) {
	vars := map[string]string{"MAIN_DB": "main-db"}

	config := map[string]any{
		"service": "{{ $var('MAIN_DB') }}",
	}

	result, errs := resolveVars(config, vars)
	assert.Empty(t, errs)
	assert.Equal(t, "main-db", result["service"])
}

func TestResolveVars_MultipleInOneString(t *testing.T) {
	vars := map[string]string{
		"HOST": "localhost",
		"PORT": "5432",
	}

	config := map[string]any{
		"url": "postgres://{{ $var('HOST') }}:{{ $var('PORT') }}/main",
	}

	result, errs := resolveVars(config, vars)
	assert.Empty(t, errs)
	assert.Equal(t, "postgres://localhost:5432/main", result["url"])
}

func TestResolveVars_Nested(t *testing.T) {
	vars := map[string]string{"TABLE": "tasks"}

	config := map[string]any{
		"nodes": map[string]any{
			"query": map[string]any{
				"config": map[string]any{
					"table": "{{ $var('TABLE') }}",
				},
			},
		},
	}

	result, errs := resolveVars(config, vars)
	assert.Empty(t, errs)
	table := result["nodes"].(map[string]any)["query"].(map[string]any)["config"].(map[string]any)["table"]
	assert.Equal(t, "tasks", table)
}

func TestResolveVars_InArray(t *testing.T) {
	vars := map[string]string{"TOPIC": "member.invited"}

	config := map[string]any{
		"topics": []any{"{{ $var('TOPIC') }}", "other.topic"},
	}

	result, errs := resolveVars(config, vars)
	assert.Empty(t, errs)
	assert.Equal(t, []any{"member.invited", "other.topic"}, result["topics"])
}

func TestResolveVars_UnknownVariable(t *testing.T) {
	vars := map[string]string{}

	config := map[string]any{
		"nodes": map[string]any{
			"emit": map[string]any{
				"topic": "{{ $var('MISSING') }}",
			},
		},
	}

	_, errs := resolveVars(config, vars)
	require.Len(t, errs, 1)
	assert.Contains(t, errs[0].Error(), "MISSING")
	assert.Contains(t, errs[0].Error(), "nodes.emit.topic")
}

func TestResolveVars_MultipleUnknown(t *testing.T) {
	vars := map[string]string{}

	config := map[string]any{
		"a": "{{ $var('MISSING_A') }}",
		"b": "{{ $var('MISSING_B') }}",
	}

	_, errs := resolveVars(config, vars)
	assert.Len(t, errs, 2)
}

func TestResolveVars_NonStringValuesUntouched(t *testing.T) {
	vars := map[string]string{"X": "y"}

	config := map[string]any{
		"port":    float64(3000),
		"debug":   true,
		"nothing": nil,
	}

	result, errs := resolveVars(config, vars)
	assert.Empty(t, errs)
	assert.Equal(t, float64(3000), result["port"])
	assert.Equal(t, true, result["debug"])
}

func TestResolveVars_EmptyVarsIsNoOp(t *testing.T) {
	config := map[string]any{
		"key": "value",
	}

	result, errs := resolveVars(config, map[string]string{})
	assert.Empty(t, errs)
	assert.Equal(t, "value", result["key"])
}

func TestResolveVarsAll_AcrossSections(t *testing.T) {
	vars := map[string]string{
		"TOPIC":   "member.invited",
		"MAIN_DB": "main-db",
	}

	rc := &RawConfig{
		Vars: vars,
		Root: map[string]any{
			"name": "test-app",
		},
		Workers: map[string]map[string]any{
			"workers/notify.json": {
				"subscribe": map[string]any{
					"topic": "{{ $var('TOPIC') }}",
				},
			},
		},
		Workflows: map[string]map[string]any{
			"workflows/invite.json": {
				"nodes": map[string]any{
					"emit": map[string]any{
						"config": map[string]any{
							"topic": "{{ $var('TOPIC') }}",
						},
					},
					"query": map[string]any{
						"services": map[string]any{
							"db": "{{ $var('MAIN_DB') }}",
						},
					},
				},
			},
		},
		Routes:      make(map[string]map[string]any),
		Schedules:   make(map[string]map[string]any),
		Connections: make(map[string]map[string]any),
		Tests:       make(map[string]map[string]any),
		Models:      make(map[string]map[string]any),
		Schemas:     make(map[string]map[string]any),
	}

	errs := resolveVarsAll(rc)
	assert.Empty(t, errs)

	// Check worker topic resolved
	workerTopic := rc.Workers["workers/notify.json"]["subscribe"].(map[string]any)["topic"]
	assert.Equal(t, "member.invited", workerTopic)

	// Check workflow nodes resolved
	wfNodes := rc.Workflows["workflows/invite.json"]["nodes"].(map[string]any)
	emitTopic := wfNodes["emit"].(map[string]any)["config"].(map[string]any)["topic"]
	assert.Equal(t, "member.invited", emitTopic)
	dbService := wfNodes["query"].(map[string]any)["services"].(map[string]any)["db"]
	assert.Equal(t, "main-db", dbService)
}

func TestResolveVarsAll_EmptyVarsNoOp(t *testing.T) {
	rc := &RawConfig{
		Vars: nil,
		Root: map[string]any{"name": "test"},
	}

	errs := resolveVarsAll(rc)
	assert.Empty(t, errs)
	assert.Equal(t, "test", rc.Root["name"])
}
