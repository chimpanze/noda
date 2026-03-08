package registry

import (
	"fmt"

	"github.com/chimpanze/noda/internal/config"
	"github.com/chimpanze/noda/pkg/api"
)

// internalPlugin is a placeholder plugin for internal services (ws, sse, wasm).
type internalPlugin struct {
	prefix string
}

func (p *internalPlugin) Name() string                                     { return "internal-" + p.prefix }
func (p *internalPlugin) Prefix() string                                   { return p.prefix }
func (p *internalPlugin) Nodes() []api.NodeRegistration                    { return nil }
func (p *internalPlugin) HasServices() bool                                { return true }
func (p *internalPlugin) CreateService(config map[string]any) (any, error) { return nil, nil }
func (p *internalPlugin) HealthCheck(service any) error                    { return nil }
func (p *internalPlugin) Shutdown(service any) error                       { return nil }

var (
	wsPlugin   = &internalPlugin{prefix: "ws"}
	ssePlugin  = &internalPlugin{prefix: "sse"}
	wasmPlugin = &internalPlugin{prefix: "wasm"}
)

// RegisterInternalServices registers placeholder services for connection endpoints and Wasm runtimes.
func RegisterInternalServices(rc *config.ResolvedConfig, registry *ServiceRegistry) []error {
	var errs []error

	// Register connection endpoints
	for name, conn := range rc.Connections {
		connType, _ := conn["type"].(string)
		var plugin api.Plugin
		switch connType {
		case "websocket":
			plugin = wsPlugin
		case "sse":
			plugin = ssePlugin
		default:
			errs = append(errs, fmt.Errorf("connection %q: unknown type %q", name, connType))
			continue
		}
		if err := registry.Register(name, nil, plugin); err != nil {
			errs = append(errs, fmt.Errorf("connection %q: %w", name, err))
		}
	}

	// Register Wasm runtimes from root config
	if wasmRuntimes, ok := rc.Root["wasm"].(map[string]any); ok {
		for name := range wasmRuntimes {
			if err := registry.Register(name, nil, wasmPlugin); err != nil {
				errs = append(errs, fmt.Errorf("wasm runtime %q: %w", name, err))
			}
		}
	}

	return errs
}
