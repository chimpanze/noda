package http

import (
	"testing"

	"github.com/chimpanze/noda/pkg/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Compile-time interface check.
var _ api.NodeOutputSchemaProvider = (*requestDescriptor)(nil)

func TestRequestDescriptor_OutputSchema(t *testing.T) {
	d := &requestDescriptor{}

	provider, ok := api.NodeDescriptor(d).(api.NodeOutputSchemaProvider)
	require.True(t, ok, "requestDescriptor must implement NodeOutputSchemaProvider")

	schema := provider.OutputSchema()
	require.NotNil(t, schema)

	assert.Equal(t, "object", schema["type"])

	props, ok := schema["properties"].(map[string]any)
	require.True(t, ok, "properties must be map[string]any")
	assert.Contains(t, props, "status")
	assert.Contains(t, props, "headers")
	assert.Contains(t, props, "body")

	statusProp, ok := props["status"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "integer", statusProp["type"])

	headersProp, ok := props["headers"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "object", headersProp["type"])

	// body has no type constraint
	bodyProp, ok := props["body"].(map[string]any)
	require.True(t, ok)
	assert.Empty(t, bodyProp)

	required, ok := schema["required"].([]any)
	require.True(t, ok, "required must be []any")
	assert.Contains(t, required, "status")
	assert.Contains(t, required, "headers")
	assert.Contains(t, required, "body")
}
