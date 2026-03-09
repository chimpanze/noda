package registry

import (
	"fmt"

	"github.com/chimpanze/noda/internal/config"
	"github.com/chimpanze/noda/internal/expr"
)

// BootstrapResult holds all registries after startup initialization.
type BootstrapResult struct {
	Plugins  *PluginRegistry
	Services *ServiceRegistry
	Nodes    *NodeRegistry
	Compiler *expr.Compiler
}

// Bootstrap initializes the full plugin/service/node pipeline from a resolved config.
// It registers all built-in plugins, creates services, registers internal services,
// and runs startup validation.
func Bootstrap(rc *config.ResolvedConfig, plugins *PluginRegistry) (*BootstrapResult, []error) {
	var allErrors []error

	// 1. Register nodes from all plugins
	nodes := NewNodeRegistry()
	for _, p := range plugins.All() {
		if err := nodes.RegisterFromPlugin(p); err != nil {
			allErrors = append(allErrors, fmt.Errorf("node registration: %w", err))
		}
	}

	// 2. Initialize services from root config
	services := NewServiceRegistry()
	if servicesMap, ok := rc.Root["services"].(map[string]any); ok {
		var svcErrs []error
		services, svcErrs = InitializeServices(servicesMap, plugins)
		allErrors = append(allErrors, svcErrs...)
	}

	// 3. Register internal services (ws, sse, wasm placeholders)
	internalErrs := RegisterInternalServices(rc, services)
	allErrors = append(allErrors, internalErrs...)

	// 4. Create shared expression compiler
	compiler := expr.NewCompilerWithFunctions()

	// 5. Run startup validation (pre-compiles expressions into shared compiler cache)
	valErrs := ValidateStartup(rc, plugins, services, nodes, compiler)
	allErrors = append(allErrors, valErrs...)

	if len(allErrors) > 0 {
		return nil, allErrors
	}

	return &BootstrapResult{
		Plugins:  plugins,
		Services: services,
		Nodes:    nodes,
		Compiler: compiler,
	}, nil
}
