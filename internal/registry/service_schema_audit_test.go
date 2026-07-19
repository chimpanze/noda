package registry_test

// TestServiceConfigSchemaAudit enumerates every plugin registered by the
// runtime (cmd/noda's corePlugins + serviceOnlyPlugins) and checks that
// api.Plugin.ServiceConfigSchema agrees with HasServices: plugins without
// services return nil, plugins with services declare a structural
// "type": "object" schema that stays within the vocabulary
// registry.CheckSchemaVocabulary/ValidateNodeConfig implement — the same
// walker used for NodeDescriptor.ConfigSchema, per the ServiceConfigSchema
// doc comment's "same conventions" note (#375, #376).
//
// This lives in an external (_test) package, rather than package registry,
// so it can import every plugin package (which themselves import
// internal/registry) without creating a production import cycle.

import (
	"testing"

	"github.com/chimpanze/noda/internal/registry"
	"github.com/chimpanze/noda/pkg/api"
	authplugin "github.com/chimpanze/noda/plugins/auth"
	cacheplugin "github.com/chimpanze/noda/plugins/cache"
	"github.com/chimpanze/noda/plugins/core/control"
	"github.com/chimpanze/noda/plugins/core/event"
	coreoidc "github.com/chimpanze/noda/plugins/core/oidc"
	"github.com/chimpanze/noda/plugins/core/response"
	coresse "github.com/chimpanze/noda/plugins/core/sse"
	corestorage "github.com/chimpanze/noda/plugins/core/storage"
	"github.com/chimpanze/noda/plugins/core/transform"
	"github.com/chimpanze/noda/plugins/core/upload"
	"github.com/chimpanze/noda/plugins/core/util"
	corewasm "github.com/chimpanze/noda/plugins/core/wasm"
	"github.com/chimpanze/noda/plugins/core/workflow"
	corews "github.com/chimpanze/noda/plugins/core/ws"
	dbplugin "github.com/chimpanze/noda/plugins/db"
	emailplugin "github.com/chimpanze/noda/plugins/email"
	httpplugin "github.com/chimpanze/noda/plugins/http"
	imageplugin "github.com/chimpanze/noda/plugins/image"
	livekitplugin "github.com/chimpanze/noda/plugins/livekit"
	pubsubplugin "github.com/chimpanze/noda/plugins/pubsub"
	storageplugin "github.com/chimpanze/noda/plugins/storage"
	streamplugin "github.com/chimpanze/noda/plugins/stream"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// allRegisteredPlugins mirrors cmd/noda/main.go's corePlugins() +
// serviceOnlyPlugins() lists (image included unconditionally here since the
// //go:build !noimage tag only gates its runtime registration, not the
// package itself).
func allRegisteredPlugins() []api.Plugin {
	return []api.Plugin{
		&control.Plugin{},
		&transform.Plugin{},
		&util.Plugin{},
		&workflow.Plugin{},
		&response.Plugin{},
		&dbplugin.Plugin{},
		&cacheplugin.Plugin{},
		&event.Plugin{},
		&corestorage.Plugin{},
		&upload.Plugin{},
		&httpplugin.Plugin{},
		&emailplugin.Plugin{},
		&corews.Plugin{},
		&coresse.Plugin{},
		&corewasm.Plugin{},
		&coreoidc.Plugin{},
		&livekitplugin.Plugin{},
		&authplugin.Plugin{},
		&imageplugin.Plugin{},
		&streamplugin.Plugin{},
		&pubsubplugin.Plugin{},
		&storageplugin.Plugin{},
	}
}

func TestServiceConfigSchemaAudit(t *testing.T) {
	plugins := allRegisteredPlugins()
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
