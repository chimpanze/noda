package cache

import (
	"testing"

	"github.com/chimpanze/noda/pkg/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Compile-time interface checks.
var _ api.NodeOutputSchemaProvider = (*setDescriptor)(nil)
var _ api.NodeOutputSchemaProvider = (*getDescriptor)(nil)
var _ api.NodeOutputSchemaProvider = (*delDescriptor)(nil)

func TestSetDescriptor_OutputSchema(t *testing.T) {
	d := &setDescriptor{}

	// Verify interface satisfaction at runtime.
	provider, ok := api.NodeDescriptor(d).(api.NodeOutputSchemaProvider)
	require.True(t, ok, "setDescriptor must implement NodeOutputSchemaProvider")

	schema := provider.OutputSchema()
	require.NotNil(t, schema)

	assert.Equal(t, "object", schema["type"])

	props, ok := schema["properties"].(map[string]any)
	require.True(t, ok, "properties must be map[string]any")
	okProp, ok := props["ok"].(map[string]any)
	require.True(t, ok, "properties.ok must be map[string]any")
	assert.Equal(t, "boolean", okProp["type"])

	required, ok := schema["required"].([]any)
	require.True(t, ok, "required must be []any")
	assert.Contains(t, required, "ok")
}

func TestGetDescriptor_OutputSchema(t *testing.T) {
	d := &getDescriptor{}

	provider, ok := api.NodeDescriptor(d).(api.NodeOutputSchemaProvider)
	require.True(t, ok, "getDescriptor must implement NodeOutputSchemaProvider")

	schema := provider.OutputSchema()
	require.NotNil(t, schema)

	assert.Equal(t, "object", schema["type"])

	props, ok := schema["properties"].(map[string]any)
	require.True(t, ok, "properties must be map[string]any")
	assert.Contains(t, props, "value")
	// value property is an empty map (no type constraint)
	valueProp, ok := props["value"].(map[string]any)
	require.True(t, ok, "properties.value must be map[string]any")
	assert.Empty(t, valueProp)

	required, ok := schema["required"].([]any)
	require.True(t, ok, "required must be []any")
	assert.Contains(t, required, "value")
}

func TestDelDescriptor_OutputSchema(t *testing.T) {
	d := &delDescriptor{}

	provider, ok := api.NodeDescriptor(d).(api.NodeOutputSchemaProvider)
	require.True(t, ok, "delDescriptor must implement NodeOutputSchemaProvider")

	schema := provider.OutputSchema()
	require.NotNil(t, schema)

	assert.Equal(t, "object", schema["type"])

	props, ok := schema["properties"].(map[string]any)
	require.True(t, ok, "properties must be map[string]any")
	okProp, ok := props["ok"].(map[string]any)
	require.True(t, ok, "properties.ok must be map[string]any")
	assert.Equal(t, "boolean", okProp["type"])

	required, ok := schema["required"].([]any)
	require.True(t, ok, "required must be []any")
	assert.Contains(t, required, "ok")
}
