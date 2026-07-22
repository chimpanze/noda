package registry_test

// TestServiceConfigSchemaAudit enumerates every plugin registered by the
// runtime (plugins/all, the same list cmd/noda and internal/mcp consume,
// #384) and checks that api.Plugin.ServiceConfigSchema agrees with
// HasServices: plugins without services return nil, plugins with services
// declare a structural "type": "object" schema that stays within the
// vocabulary registry.CheckSchemaVocabulary/ValidateNodeConfig implement —
// the same walker used for NodeDescriptor.ConfigSchema, per the
// ServiceConfigSchema doc comment's "same conventions" note (#375, #376).
//
// The list always includes the image plugin: there is one build
// configuration and libvips is always present (#425). Consuming the
// runtime's own list is the point — the audit cannot drift from what
// actually gets registered.
//
// This lives in an external (_test) package, rather than package registry,
// so it can import plugins/all (whose plugin packages themselves import
// internal/registry) without creating a production import cycle.

import (
	"testing"

	"github.com/chimpanze/noda/internal/registry"
	"github.com/chimpanze/noda/plugins/all"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestServiceConfigSchemaAudit(t *testing.T) {
	plugins := all.All()
	for _, p := range plugins {
		t.Run(p.Name(), func(t *testing.T) {
			schema := p.ServiceConfigSchema()
			if !p.HasServices() {
				assert.Nil(t, schema, "plugin %q has no services and must return nil", p.Name())
				return
			}
			require.NotNil(t, schema, "plugin %q has services and must declare a ServiceConfigSchema", p.Name())
			assert.Equal(t, "object", schema["type"], "plugin %q schema root must be type object", p.Name())
			// registry.CompileServiceSchema is the real (#376) dry-run helper
			// (ValidateStartupDryRun calls its unexported twin,
			// compileServiceSchema, directly) — compiling here with the real
			// santhosh-tekuri/jsonschema library catches malformed schemas
			// (unknown keywords, wrong-shaped "required"/"type" values, etc.)
			// the same way the per-node schema_audit_test.go files use
			// registry.CheckSchemaVocabulary for node ConfigSchemas.
			_, err := registry.CompileServiceSchema(p.Name(), schema)
			require.NoError(t, err, "plugin %q schema must compile", p.Name())
		})
	}
}
