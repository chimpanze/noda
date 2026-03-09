package registry

import (
	"fmt"
	"strings"

	"github.com/chimpanze/noda/internal/config"
	"github.com/chimpanze/noda/internal/expr"
)

// ValidateStartup checks all plugin, service, and node references in the config.
func ValidateStartup(rc *config.ResolvedConfig, plugins *PluginRegistry, services *ServiceRegistry, nodes *NodeRegistry) []error {
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

				// Check service exists
				svcPrefix, exists := services.GetPrefix(svcNameStr)
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
	compiler := expr.NewCompilerWithFunctions()
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
