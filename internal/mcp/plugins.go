package mcp

import (
	"github.com/chimpanze/noda/pkg/api"
	"github.com/chimpanze/noda/plugins/all"
)

// corePlugins returns all plugins that provide node types. The list lives
// in plugins/all (#384) so the MCP surface can never drift from the runtime.
func corePlugins() []api.Plugin { return all.Core() }

// servicePlugins returns every plugin the MCP server knows about, for
// enumerating service config schemas (noda_get_service_schema). Includes
// the service-only plugins (stream, pubsub, storage), which aren't needed
// by the MCP node registry but must still be describable.
func servicePlugins() []api.Plugin { return all.All() }
