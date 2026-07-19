package email

import (
	"testing"

	"github.com/chimpanze/noda/internal/registry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestServiceConfigSchema_RequiredMatchesCreateService pins schema<->code
// agreement: a config missing "host" must fail BOTH schema validation and
// CreateService.
func TestServiceConfigSchema_RequiredMatchesCreateService(t *testing.T) {
	p := &Plugin{}
	schema := p.ServiceConfigSchema()
	require.Empty(t, registry.CheckSchemaVocabulary(schema))
	required, _ := schema["required"].([]any)
	require.Equal(t, []any{"host"}, required)

	cfg := map[string]any{}
	assert.NotEmpty(t, registry.ValidateNodeConfig(schema, cfg), "schema must reject config missing \"host\"")
	_, err := p.CreateService(cfg)
	assert.Error(t, err, "CreateService must reject config missing \"host\"")
}
