package util

import (
	"testing"

	"github.com/chimpanze/noda/pkg/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Compile-time interface checks.
var _ api.NodeOutputSchemaProvider = (*uuidDescriptor)(nil)
var _ api.NodeOutputSchemaProvider = (*timestampDescriptor)(nil)

func TestUUIDDescriptor_OutputSchema(t *testing.T) {
	d := &uuidDescriptor{}

	provider, ok := api.NodeDescriptor(d).(api.NodeOutputSchemaProvider)
	require.True(t, ok, "uuidDescriptor must implement NodeOutputSchemaProvider")

	schema := provider.OutputSchema()
	require.NotNil(t, schema)

	assert.Equal(t, "string", schema["type"])
	assert.Equal(t, "UUID v4 string", schema["description"])
}

func TestTimestampDescriptor_OutputSchema(t *testing.T) {
	d := &timestampDescriptor{}

	provider, ok := api.NodeDescriptor(d).(api.NodeOutputSchemaProvider)
	require.True(t, ok, "timestampDescriptor must implement NodeOutputSchemaProvider")

	schema := provider.OutputSchema()
	require.NotNil(t, schema)

	oneOf, ok := schema["oneOf"].([]any)
	require.True(t, ok, "oneOf must be []any")
	require.Len(t, oneOf, 2)

	// First option: string (ISO 8601)
	first, ok := oneOf[0].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "string", first["type"])

	// Second option: integer (unix timestamp)
	second, ok := oneOf[1].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "integer", second["type"])
}
