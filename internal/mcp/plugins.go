package mcp

import (
	"github.com/chimpanze/noda/pkg/api"
	cacheplugin "github.com/chimpanze/noda/plugins/cache"
	"github.com/chimpanze/noda/plugins/core/control"
	"github.com/chimpanze/noda/plugins/core/event"
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
)

// corePlugins returns all plugins that provide node types.
// This mirrors cmd/noda/main.go corePlugins() but is self-contained
// so the MCP package has no dependency on cmd/noda.
func corePlugins() []api.Plugin {
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
		&imageplugin.Plugin{},
		&httpplugin.Plugin{},
		&emailplugin.Plugin{},
		&corews.Plugin{},
		&coresse.Plugin{},
		&corewasm.Plugin{},
	}
}
