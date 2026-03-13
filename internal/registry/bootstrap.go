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

// BootstrapOptions configures Bootstrap behavior.
type BootstrapOptions struct {
	// DryRun skips service creation (no database connections, no external calls).
	// Used by the validate command to check config without requiring live services.
	DryRun bool
}

// Bootstrap initializes the full plugin/service/node pipeline from a resolved config.
// It registers all built-in plugins, creates services, registers internal services,
// and runs startup validation.
func Bootstrap(rc *config.ResolvedConfig, plugins *PluginRegistry, opts ...BootstrapOptions) (*BootstrapResult, []error) {
	var opt BootstrapOptions
	if len(opts) > 0 {
		opt = opts[0]
	}

	var allErrors []error

	// 1. Register nodes from all plugins
	nodes := NewNodeRegistry()
	for _, p := range plugins.All() {
		if err := nodes.RegisterFromPlugin(p); err != nil {
			allErrors = append(allErrors, fmt.Errorf("node registration: %w", err))
		}
	}

	// 2. Collect deferred services (connection endpoints, wasm runtimes)
	// These will be created later by the server/wasm runtime, but we need
	// their names and prefixes for startup validation.
	deferred, deferredErrs := CollectDeferredServices(rc)
	allErrors = append(allErrors, deferredErrs...)

	// 3. Initialize services from root config (skip in dry-run mode)
	services := NewServiceRegistry()
	if !opt.DryRun {
		if servicesMap, ok := rc.Root["services"].(map[string]any); ok {
			var svcErrs []error
			services, svcErrs = InitializeServices(servicesMap, plugins)
			allErrors = append(allErrors, svcErrs...)
		}
	}

	// 4. Create shared expression compiler
	compiler := expr.NewCompilerWithVars(rc.Vars)

	// 5. Run startup validation
	if opt.DryRun {
		valErrs := ValidateStartupDryRun(rc, plugins, nodes, compiler, deferred)
		allErrors = append(allErrors, valErrs...)
	} else {
		valErrs := ValidateStartup(rc, plugins, services, nodes, compiler, deferred)
		allErrors = append(allErrors, valErrs...)
	}

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
