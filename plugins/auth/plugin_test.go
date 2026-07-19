package auth

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestServiceConfigSchema_RequiredMatchesCreateService pins schema<->code
// agreement: a config missing each schema-required field must fail BOTH
// schema validation and CreateService.
func TestServiceConfigSchema_RequiredMatchesCreateService(t *testing.T) {
	p := &Plugin{}
	schema := p.ServiceConfigSchema()
	required, _ := schema["required"].([]any)
	require.NotEmpty(t, required)
	for _, r := range required {
		field := r.(string)
		cfg := map[string]any{"database": "db"}
		delete(cfg, field)
		_, err := p.CreateService(cfg)
		assert.Error(t, err, "CreateService must reject config missing required %q", field)
	}
}
