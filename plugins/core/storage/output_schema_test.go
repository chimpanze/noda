package storage

import (
	"testing"

	"github.com/chimpanze/noda/pkg/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Compile-time interface checks.
var _ api.NodeOutputSchemaProvider = (*readDescriptor)(nil)
var _ api.NodeOutputSchemaProvider = (*writeDescriptor)(nil)

func TestReadDescriptor_OutputSchema(t *testing.T) {
	d := &readDescriptor{}

	provider, ok := api.NodeDescriptor(d).(api.NodeOutputSchemaProvider)
	require.True(t, ok, "readDescriptor must implement NodeOutputSchemaProvider")

	schema := provider.OutputSchema()
	require.NotNil(t, schema)

	assert.Equal(t, "object", schema["type"])

	props, ok := schema["properties"].(map[string]any)
	require.True(t, ok, "properties must be map[string]any")
	assert.Contains(t, props, "data")
	assert.Contains(t, props, "size")
	assert.Contains(t, props, "content_type")

	dataProp, ok := props["data"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "string", dataProp["type"])

	sizeProp, ok := props["size"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "integer", sizeProp["type"])

	ctProp, ok := props["content_type"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "string", ctProp["type"])

	required, ok := schema["required"].([]any)
	require.True(t, ok, "required must be []any")
	assert.Contains(t, required, "data")
	assert.Contains(t, required, "size")
	assert.Contains(t, required, "content_type")
}

func TestWriteDescriptor_OutputSchema(t *testing.T) {
	d := &writeDescriptor{}

	provider, ok := api.NodeDescriptor(d).(api.NodeOutputSchemaProvider)
	require.True(t, ok, "writeDescriptor must implement NodeOutputSchemaProvider")

	schema := provider.OutputSchema()
	require.NotNil(t, schema)

	assert.Equal(t, "object", schema["type"])
	// write returns a minimal schema with only type: "object"
	assert.NotContains(t, schema, "properties")
	assert.NotContains(t, schema, "required")
}
