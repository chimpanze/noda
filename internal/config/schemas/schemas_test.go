package schemas

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEmbeddedSchemas(t *testing.T) {
	schemaFiles := []string{
		"root.json",
		"route.json",
		"workflow.json",
		"worker.json",
		"schedule.json",
		"connections.json",
		"test.json",
	}

	for _, name := range schemaFiles {
		t.Run(name, func(t *testing.T) {
			data, err := FS.ReadFile(name)
			require.NoError(t, err)
			assert.NotEmpty(t, data)

			// Verify valid JSON
			var schema map[string]any
			err = json.Unmarshal(data, &schema)
			require.NoError(t, err)
			assert.Equal(t, "object", schema["type"])
		})
	}
}

func TestRouteSchema_RequiredFields(t *testing.T) {
	data, err := FS.ReadFile("route.json")
	require.NoError(t, err)

	var schema map[string]any
	require.NoError(t, json.Unmarshal(data, &schema))

	required := schema["required"].([]any)
	assert.Contains(t, required, "id")
	assert.Contains(t, required, "method")
	assert.Contains(t, required, "path")
	assert.Contains(t, required, "trigger")
}

func TestWorkflowSchema_RequiredFields(t *testing.T) {
	data, err := FS.ReadFile("workflow.json")
	require.NoError(t, err)

	var schema map[string]any
	require.NoError(t, json.Unmarshal(data, &schema))

	required := schema["required"].([]any)
	assert.Contains(t, required, "id")
	assert.Contains(t, required, "nodes")
	assert.Contains(t, required, "edges")
}

func TestWorkerSchema_RequiredFields(t *testing.T) {
	data, err := FS.ReadFile("worker.json")
	require.NoError(t, err)

	var schema map[string]any
	require.NoError(t, json.Unmarshal(data, &schema))

	required := schema["required"].([]any)
	assert.Contains(t, required, "id")
	assert.Contains(t, required, "services")
	assert.Contains(t, required, "subscribe")
	assert.Contains(t, required, "trigger")
}
