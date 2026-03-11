package registry

import (
	"fmt"

	"github.com/chimpanze/noda/internal/config"
)

// DeferredService describes a service that will be created later (e.g., by the server
// for connection endpoints, or by the wasm runtime). Used during startup validation
// to verify service slot references without registering placeholder instances.
type DeferredService struct {
	Prefix string
}

// CollectDeferredServices scans the config for services that will be created at
// runtime (connection endpoints, wasm runtimes) and returns them as a map of
// name → DeferredService. These are NOT registered in the ServiceRegistry.
func CollectDeferredServices(rc *config.ResolvedConfig) (map[string]DeferredService, []error) {
	deferred := make(map[string]DeferredService)
	var errs []error

	// Connection endpoints
	for filePath, conn := range rc.Connections {
		endpoints, ok := conn["endpoints"].(map[string]any)
		if !ok {
			continue
		}
		for epName, epVal := range endpoints {
			ep, ok := epVal.(map[string]any)
			if !ok {
				continue
			}
			epType, _ := ep["type"].(string)
			switch epType {
			case "websocket":
				deferred[epName] = DeferredService{Prefix: "ws"}
			case "sse":
				deferred[epName] = DeferredService{Prefix: "sse"}
			default:
				errs = append(errs, fmt.Errorf("connection %q endpoint %q: unknown type %q", filePath, epName, epType))
			}
		}
	}

	// Wasm runtimes
	if wasmRuntimes, ok := rc.Root["wasm_runtimes"].(map[string]any); ok {
		for name := range wasmRuntimes {
			deferred[name] = DeferredService{Prefix: "wasm"}
		}
	}

	return deferred, errs
}
