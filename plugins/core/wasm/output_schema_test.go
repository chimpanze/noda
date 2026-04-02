package wasm

import (
	"testing"

	"github.com/chimpanze/noda/pkg/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Compile-time interface check.
var _ api.NodeOutputSchemaProvider = (*sendDescriptor)(nil)

func TestSendDescriptor_OutputSchema(t *testing.T) {
	d := &sendDescriptor{}

	provider, ok := api.NodeDescriptor(d).(api.NodeOutputSchemaProvider)
	require.True(t, ok, "sendDescriptor must implement NodeOutputSchemaProvider")

	schema := provider.OutputSchema()
	require.NotNil(t, schema)

	assert.Equal(t, "object", schema["type"])

	props, ok := schema["properties"].(map[string]any)
	require.True(t, ok, "properties must be map[string]any")
	sentProp, ok := props["sent"].(map[string]any)
	require.True(t, ok, "properties.sent must be map[string]any")
	assert.Equal(t, "boolean", sentProp["type"])

	required, ok := schema["required"].([]any)
	require.True(t, ok, "required must be []any")
	assert.Contains(t, required, "sent")
}
