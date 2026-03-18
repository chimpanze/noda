package registry

import (
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/chimpanze/noda/internal/config"
	"github.com/chimpanze/noda/internal/expr"
)

// toUint converts a JSON number value to uint. Handles float64 (default JSON
// unmarshalling), json.Number, and int.
func toUint(v any) (uint, bool) {
	switch n := v.(type) {
	case float64:
		if n >= 0 {
			return uint(n), true
		}
	case json.Number:
		if i, err := n.Int64(); err == nil && i >= 0 {
			return uint(i), true
		}
	case int:
		if n >= 0 {
			return uint(n), true
		}
	}
	return 0, false
}

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

	slog.Info("node types registered", "count", len(nodes.AllTypes()))

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

	slog.Info("services initialized", "count", services.Count())

	// 4. Create shared expression compiler
	var compilerOpts []expr.CompilerOption
	if serverCfg, ok := rc.Root["server"].(map[string]any); ok {
		if budget, ok := serverCfg["expression_memory_budget"]; ok {
			if n, ok := toUint(budget); ok {
				compilerOpts = append(compilerOpts, expr.WithMemoryBudget(n))
			}
		}
		if strict, ok := serverCfg["expression_strict_mode"].(bool); ok {
			compilerOpts = append(compilerOpts, expr.WithStrictMode(strict))
		}
	}
	compiler := expr.NewCompilerWithVars(rc.Vars, compilerOpts...)

	// 5. Run startup validation
	if opt.DryRun {
		valErrs := ValidateStartupDryRun(rc, plugins, nodes, compiler, deferred)
		allErrors = append(allErrors, valErrs...)
	} else {
		valErrs := ValidateStartup(rc, plugins, services, nodes, compiler, deferred)
		allErrors = append(allErrors, valErrs...)
	}

	slog.Info("startup validation passed")

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
