// Package mcp implements a Model Context Protocol server for Noda.
// It exposes node metadata, config schemas, validation, and project
// management tools over stdio so AI agents can discover and build
// Noda projects interactively.
package mcp

import (
	"log/slog"

	"github.com/chimpanze/noda/internal/registry"
	"github.com/chimpanze/noda/pkg/api"
	"github.com/mark3labs/mcp-go/server"
)

// NewServer creates a configured MCP server with all Noda tools and resources.
func NewServer(version string) *server.MCPServer {
	s := server.NewMCPServer(
		"Noda",
		version,
		server.WithToolCapabilities(true),
		server.WithResourceCapabilities(true, true),
	)

	// Build node registry from plugins (metadata only, no runtime services).
	nodeReg := buildNodeRegistry()

	registerTools(s, nodeReg)
	registerResources(s)

	return s
}

// buildNodeRegistry creates a node registry with all plugins registered.
// No services are started — only descriptors and factories are loaded.
func buildNodeRegistry() *registry.NodeRegistry {
	nodeReg := registry.NewNodeRegistry()
	for _, p := range allPlugins() {
		if err := nodeReg.RegisterFromPlugin(p); err != nil {
			slog.Warn("failed to register MCP plugin nodes", "plugin", p.Name(), "error", err)
		}
	}
	return nodeReg
}

// allPlugins returns every plugin that provides nodes.
func allPlugins() []api.Plugin {
	return corePlugins()
}
