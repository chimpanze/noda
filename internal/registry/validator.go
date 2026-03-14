package registry

import (
	"fmt"
	"strings"

	"github.com/chimpanze/noda/internal/config"
	"github.com/chimpanze/noda/internal/expr"
)

// ValidateStartup checks all plugin, service, and node references in the config.
// The deferred parameter lists services that will be created later (connection endpoints,
// wasm runtimes) — these are treated as valid references during slot validation.
func ValidateStartup(rc *config.ResolvedConfig, plugins *PluginRegistry, services *ServiceRegistry, nodes *NodeRegistry, compiler *expr.Compiler, deferred map[string]DeferredService) []error {
	var errs []error

	for wfName, wf := range rc.Workflows {
		wfNodes, ok := wf["nodes"].(map[string]any)
		if !ok {
			continue
		}

		for nodeID, raw := range wfNodes {
			node, ok := raw.(map[string]any)
			if !ok {
				continue
			}

			nodeType, _ := node["type"].(string)
			if nodeType == "" {
				continue
			}

			// 1. Check node type prefix is registered
			prefix := extractPrefix(nodeType)
			if _, found := plugins.Get(prefix); !found {
				errs = append(errs, fmt.Errorf("workflow %q, node %q: unknown node type prefix %q (type: %s)", wfName, nodeID, prefix, nodeType))
				continue
			}

			// 2. Check node type is registered
			desc, found := nodes.GetDescriptor(nodeType)
			if !found {
				errs = append(errs, fmt.Errorf("workflow %q, node %q: unknown node type %q", wfName, nodeID, nodeType))
				continue
			}

			// 3. Validate service slots
			serviceDeps := desc.ServiceDeps()
			nodeServices, _ := node["services"].(map[string]any)

			for slot, dep := range serviceDeps {
				svcName, hasSlot := nodeServices[slot]
				svcNameStr, _ := svcName.(string)

				if !hasSlot || svcNameStr == "" {
					if dep.Required {
						errs = append(errs, fmt.Errorf("workflow %q, node %q: missing required service slot %q", wfName, nodeID, slot))
					}
					continue
				}

				// Check service exists in registry or deferred set
				svcPrefix, exists := services.GetPrefix(svcNameStr)
				if !exists {
					if ds, ok := deferred[svcNameStr]; ok {
						svcPrefix = ds.Prefix
						exists = true
					}
				}
				if !exists {
					errs = append(errs, fmt.Errorf("workflow %q, node %q: service %q not found (slot: %s)", wfName, nodeID, svcNameStr, slot))
					continue
				}

				// Check prefix matches
				if svcPrefix != dep.Prefix {
					errs = append(errs, fmt.Errorf("workflow %q, node %q: service %q has prefix %q, but slot %q requires prefix %q", wfName, nodeID, svcNameStr, svcPrefix, slot, dep.Prefix))
				}
			}
		}
	}

	// 4. Pre-compile all expressions in workflow node configs to catch syntax errors
	for wfName, wf := range rc.Workflows {
		wfNodes, ok := wf["nodes"].(map[string]any)
		if !ok {
			continue
		}
		for nodeID, raw := range wfNodes {
			node, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			if cfg, ok := node["config"].(map[string]any); ok {
				for _, exprErr := range expr.ValidateExpressions(compiler, cfg) {
					errs = append(errs, fmt.Errorf("workflow %q, node %q: %w", wfName, nodeID, exprErr))
				}
			}
		}
	}

	return errs
}

// ValidateStartupDryRun checks node types and pre-compiles expressions without
// requiring live services. Used by the validate command for offline validation.
func ValidateStartupDryRun(rc *config.ResolvedConfig, plugins *PluginRegistry, nodes *NodeRegistry, compiler *expr.Compiler, deferred map[string]DeferredService) []error {
	var errs []error

	// Collect configured service names for reference checking
	configuredServices := make(map[string]string) // name → prefix
	if servicesMap, ok := rc.Root["services"].(map[string]any); ok {
		for name, raw := range servicesMap {
			if cfg, ok := raw.(map[string]any); ok {
				if pluginName, ok := cfg["plugin"].(string); ok {
					// Resolve plugin name to its prefix (e.g. "postgres" → "db")
					if p, found := plugins.GetByName(pluginName); found {
						configuredServices[name] = p.Prefix()
					} else {
						configuredServices[name] = pluginName
					}
				}
			}
		}
	}

	// Include deferred services (connection endpoints, wasm runtimes)
	for name, ds := range deferred {
		configuredServices[name] = ds.Prefix
	}

	for wfName, wf := range rc.Workflows {
		wfNodes, ok := wf["nodes"].(map[string]any)
		if !ok {
			continue
		}

		for nodeID, raw := range wfNodes {
			node, ok := raw.(map[string]any)
			if !ok {
				continue
			}

			nodeType, _ := node["type"].(string)
			if nodeType == "" {
				continue
			}

			// 1. Check node type prefix is registered
			prefix := extractPrefix(nodeType)
			if _, found := plugins.Get(prefix); !found {
				errs = append(errs, fmt.Errorf("workflow %q, node %q: unknown node type prefix %q (type: %s)", wfName, nodeID, prefix, nodeType))
				continue
			}

			// 2. Check node type is registered
			desc, found := nodes.GetDescriptor(nodeType)
			if !found {
				errs = append(errs, fmt.Errorf("workflow %q, node %q: unknown node type %q", wfName, nodeID, nodeType))
				continue
			}

			// 3. Validate service slot references exist in config (not live check)
			serviceDeps := desc.ServiceDeps()
			nodeServices, _ := node["services"].(map[string]any)
			for slot, dep := range serviceDeps {
				svcName, hasSlot := nodeServices[slot]
				svcNameStr, _ := svcName.(string)
				if !hasSlot || svcNameStr == "" {
					if dep.Required {
						errs = append(errs, fmt.Errorf("workflow %q, node %q: missing required service slot %q", wfName, nodeID, slot))
					}
					continue
				}
				svcPrefix, exists := configuredServices[svcNameStr]
				if !exists {
					errs = append(errs, fmt.Errorf("workflow %q, node %q: service %q not found in config (slot: %s)", wfName, nodeID, svcNameStr, slot))
					continue
				}
				if svcPrefix != dep.Prefix {
					errs = append(errs, fmt.Errorf("workflow %q, node %q: service %q has prefix %q, but slot %q requires prefix %q",
						wfName, nodeID, svcNameStr, svcPrefix, slot, dep.Prefix))
				}
			}
		}
	}

	// Pre-compile all expressions
	for wfName, wf := range rc.Workflows {
		wfNodes, ok := wf["nodes"].(map[string]any)
		if !ok {
			continue
		}
		for nodeID, raw := range wfNodes {
			node, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			if cfg, ok := node["config"].(map[string]any); ok {
				for _, exprErr := range expr.ValidateExpressions(compiler, cfg) {
					errs = append(errs, fmt.Errorf("workflow %q, node %q: %w", wfName, nodeID, exprErr))
				}
			}
		}
	}

	return errs
}

func extractPrefix(nodeType string) string {
	if idx := strings.IndexByte(nodeType, '.'); idx >= 0 {
		return nodeType[:idx]
	}
	return nodeType
}
