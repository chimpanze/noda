package http

import (
	"testing"

	"github.com/chimpanze/noda/internal/registry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestServiceConfigSchema_EmptyRequired_ConsistentWithCreateService pins
// schema<->code agreement for http: CreateService has no unconditionally
// required config field (every key has a usable default), so an empty
// config must pass schema validation, and CreateService's own
// (unchanged-by-this-task) success behavior on an empty config is pinned
// too.
func TestServiceConfigSchema_EmptyRequired_ConsistentWithCreateService(t *testing.T) {
	p := &Plugin{}
	schema := p.ServiceConfigSchema()
	require.Empty(t, registry.CheckSchemaVocabulary(schema))
	required, _ := schema["required"].([]any)
	assert.Empty(t, required)
	assert.Empty(t, registry.ValidateNodeConfig(schema, map[string]any{}), "empty config must pass schema validation")

	svc, err := p.CreateService(map[string]any{})
	require.NoError(t, err, "CreateService must still accept an empty config")
	require.NotNil(t, svc)
}
