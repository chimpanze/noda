package testing

import (
	"testing"

	"github.com/chimpanze/noda/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadTests_ValidFile(t *testing.T) {
	rc := &config.ResolvedConfig{
		Workflows: map[string]map[string]any{
			"workflows/wf.json": {
				"id": "my-workflow",
				"nodes": map[string]any{
					"step1": map[string]any{"type": "db.query"},
				},
			},
		},
		Tests: map[string]map[string]any{
			"tests/test-wf.json": {
				"id":       "test-wf",
				"workflow": "my-workflow",
				"tests": []any{
					map[string]any{
						"name":  "basic test",
						"input": map[string]any{"key": "value"},
						"mocks": map[string]any{
							"step1": map[string]any{
								"output": map[string]any{"id": 1},
							},
						},
						"expect": map[string]any{
							"status": "success",
						},
					},
				},
			},
		},
	}

	suites, err := LoadTests(rc)
	require.NoError(t, err)
	require.Len(t, suites, 1)

	suite := suites[0]
	assert.Equal(t, "my-workflow", suite.Workflow)
	assert.Len(t, suite.Cases, 1)
	assert.Equal(t, "basic test", suite.Cases[0].Name)
	assert.Equal(t, "value", suite.Cases[0].Input["key"])
}

func TestLoadTests_ParseMocks(t *testing.T) {
	rc := &config.ResolvedConfig{
		Workflows: map[string]map[string]any{
			"workflows/wf.json": {
				"id": "my-wf",
				"nodes": map[string]any{
					"fetch": map[string]any{"type": "db.query"},
				},
			},
		},
		Tests: map[string]map[string]any{
			"tests/test.json": {
				"id":       "test",
				"workflow": "my-wf",
				"tests": []any{
					map[string]any{
						"name": "with mock error",
						"mocks": map[string]any{
							"fetch": map[string]any{
								"error": map[string]any{"message": "not found"},
							},
						},
						"expect": map[string]any{"status": "error"},
					},
				},
			},
		},
	}

	suites, err := LoadTests(rc)
	require.NoError(t, err)
	assert.NotNil(t, suites[0].Cases[0].Mocks["fetch"].Error)
	assert.Equal(t, "not found", suites[0].Cases[0].Mocks["fetch"].Error.Message)
}

func TestLoadTests_NonexistentWorkflow(t *testing.T) {
	rc := &config.ResolvedConfig{
		Workflows: map[string]map[string]any{},
		Tests: map[string]map[string]any{
			"tests/test.json": {
				"id":       "test",
				"workflow": "nonexistent",
				"tests":    []any{},
			},
		},
	}

	_, err := LoadTests(rc)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "nonexistent")
	assert.Contains(t, err.Error(), "not found")
}

func TestLoadTests_NonexistentMockNode(t *testing.T) {
	rc := &config.ResolvedConfig{
		Workflows: map[string]map[string]any{
			"workflows/wf.json": {
				"id": "my-wf",
				"nodes": map[string]any{
					"step1": map[string]any{"type": "db.query"},
				},
			},
		},
		Tests: map[string]map[string]any{
			"tests/test.json": {
				"id":       "test",
				"workflow": "my-wf",
				"tests": []any{
					map[string]any{
						"name": "bad mock",
						"mocks": map[string]any{
							"nonexistent_node": map[string]any{
								"output": map[string]any{},
							},
						},
						"expect": map[string]any{"status": "success"},
					},
				},
			},
		},
	}

	_, err := LoadTests(rc)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "nonexistent_node")
	assert.Contains(t, err.Error(), "does not exist")
}

func TestLoadTests_NoTests(t *testing.T) {
	rc := &config.ResolvedConfig{
		Tests: map[string]map[string]any{},
	}

	suites, err := LoadTests(rc)
	require.NoError(t, err)
	assert.Nil(t, suites)
}
