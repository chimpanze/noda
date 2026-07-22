// Package all enumerates every built-in plugin. It is the single source of
// truth consumed by the runtime (cmd/noda), the MCP server, and the
// ServiceConfigSchema audit (#384): a new plugin must be added here — and
// only here — to reach all three.
package all

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
	imageplugin "github.com/chimpanze/noda/plugins/image"
	livekitplugin "github.com/chimpanze/noda/plugins/livekit"
	pubsubplugin "github.com/chimpanze/noda/plugins/pubsub"
	storageplugin "github.com/chimpanze/noda/plugins/storage"
	streamplugin "github.com/chimpanze/noda/plugins/stream"
)

// Core returns all plugins that provide node types.
func Core() []api.Plugin {
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
		&imageplugin.Plugin{},
		&corews.Plugin{},
		&coresse.Plugin{},
		&corewasm.Plugin{},
		&coreoidc.Plugin{},
		&livekitplugin.Plugin{},
		&authplugin.Plugin{},
	}
}

// ServiceOnly returns plugins that provide services but no node types
// (stream, pubsub, storage). Registered by the full runtime, not needed
// for node registries.
func ServiceOnly() []api.Plugin {
	return []api.Plugin{
		&streamplugin.Plugin{},
		&pubsubplugin.Plugin{},
		&storageplugin.Plugin{},
	}
}

// All returns every built-in plugin (Core + ServiceOnly).
func All() []api.Plugin {
	return append(Core(), ServiceOnly()...)
}
