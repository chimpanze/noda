package mcp

import (
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
	livekitplugin "github.com/chimpanze/noda/plugins/livekit"
	pubsubplugin "github.com/chimpanze/noda/plugins/pubsub"
	storageplugin "github.com/chimpanze/noda/plugins/storage"
	streamplugin "github.com/chimpanze/noda/plugins/stream"
)

// optionalPlugins holds plugins registered via build-tagged init() functions.
var optionalPlugins []api.Plugin

// corePlugins returns all plugins that provide node types.
// This mirrors cmd/noda/main.go corePlugins() but is self-contained
// so the MCP package has no dependency on cmd/noda.
func corePlugins() []api.Plugin {
	plugins := []api.Plugin{
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
	}
	return append(plugins, optionalPlugins...)
}

// serviceOnlyPlugins returns plugins that provide services but no node
// types (stream, pubsub, storage). This mirrors cmd/noda/main.go's
// serviceOnlyPlugins() — these plugins aren't needed by the MCP node
// registry, but noda_get_service_schema must still be able to describe
// their service config blocks.
func serviceOnlyPlugins() []api.Plugin {
	return []api.Plugin{
		&streamplugin.Plugin{},
		&pubsubplugin.Plugin{},
		&storageplugin.Plugin{},
	}
}

// servicePlugins returns every plugin the MCP server knows about, for
// enumerating service config schemas (noda_get_service_schema).
func servicePlugins() []api.Plugin {
	return append(corePlugins(), serviceOnlyPlugins()...)
}
