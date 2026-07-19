package registry

import (
	"fmt"
	"sort"
	"strings"

	"github.com/chimpanze/noda/internal/config"
	"github.com/chimpanze/noda/internal/expr"
	"github.com/chimpanze/noda/pkg/api"
)

// staticFieldsByNodeType maps node types to config fields that must be static
// (literal values, not expressions). These fields are evaluated at compile time
// or used for structural decisions that cannot change per-request.
var staticFieldsByNodeType = map[string][]string{
	"event.emit":      {"mode"},
	"control.switch":  {"cases"},
	"workflow.run":    {"workflow", "transaction"},
	"control.loop":    {"workflow"},
	"workflow.output": {"name"},
	"http.request":    {"method"},
	// match.type is nested; single-segment static lookup never matched it and strict root keys reject a top-level "type"
	"transform.merge": {"mode"},
}

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

			errs = append(errs, validateNodeConfigSchema(wfName, nodeID, nodeType, desc, node)...)

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

	// 4. Pre-compile all expressions and validate static fields in workflow node configs
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
			if cfg, ok := node["config"].(map[string]any); ok {
				for _, exprErr := range expr.ValidateExpressions(compiler, cfg) {
					errs = append(errs, fmt.Errorf("workflow %q, node %q: %w", wfName, nodeID, exprErr))
				}
				if fields, ok := staticFieldsByNodeType[nodeType]; ok {
					for _, sfErr := range expr.ValidateStaticFields(cfg, fields) {
						errs = append(errs, fmt.Errorf("workflow %q, node %q: %w", wfName, nodeID, sfErr))
					}
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

	// Validate each service's config against its plugin's declared schema
	// (#376): an un-bootable service config must fail validation, not boot.
	if servicesMap, ok := rc.Root["services"].(map[string]any); ok {
		for name, raw := range servicesMap {
			cfg, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			pluginName, _ := cfg["plugin"].(string)
			p, found := plugins.GetByName(pluginName)
			if !found {
				// #385: crossrefs only catch this service when a node
				// references it; an unreferenced entry would otherwise pass
				// validate and fail at boot (lifecycle.go "unknown plugin").
				errs = append(errs, &ServiceConfigError{Service: name, Plugin: pluginName,
					Err: fmt.Errorf("unknown plugin %q (known plugins: %s)", pluginName, strings.Join(pluginNames(plugins), ", "))})
				continue
			}
			schema := p.ServiceConfigSchema()
			if schema == nil {
				continue
			}
			compiled, err := compileServiceSchema(pluginName, schema)
			if err != nil {
				errs = append(errs, &ServiceConfigError{Service: name, Plugin: pluginName, Err: fmt.Errorf("invalid ServiceConfigSchema: %w", err)})
				continue
			}
			svcCfg, _ := cfg["config"].(map[string]any)
			if svcCfg == nil {
				svcCfg = map[string]any{}
			}
			if err := validateAgainst(compiled, svcCfg); err != nil {
				errs = append(errs, &ServiceConfigError{Service: name, Plugin: pluginName, Err: err})
			}
		}
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

			errs = append(errs, validateNodeConfigSchema(wfName, nodeID, nodeType, desc, node)...)

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

		// 4. Validate edge outputs against declared node outputs (#379): the
		// compiler already rejects undeclared edge outputs at boot
		// (internal/engine/compiler.go), so the dry-run check mirrors that
		// logic to give validate/editor/MCP the same guarantee before deploy.
		// Empty output defaults to "success"; outputs are resolved
		// config-aware (control.switch's outputs depend on its "cases"
		// config), matching the engine resolver exactly.
		if edgesRaw, ok := wf["edges"].([]any); ok {
			// #386: resolve each source node's outputs once, not once per
			// edge — engine.Compile computes per node and reuses; mirror it.
			type outputsResult struct {
				outputs []string
				ok      bool
			}
			outputsByNode := make(map[string]outputsResult)
			for _, rawEdge := range edgesRaw {
				edgeMap, ok := rawEdge.(map[string]any)
				if !ok {
					continue
				}
				from, _ := edgeMap["from"].(string)
				to, _ := edgeMap["to"].(string)
				output, _ := edgeMap["output"].(string)

				fromNodeRaw, ok := wfNodes[from]
				if !ok {
					errs = append(errs, fmt.Errorf("workflow %q: edge references unknown source node %q", wfName, from))
					continue
				}
				if _, ok := wfNodes[to]; !ok {
					errs = append(errs, fmt.Errorf("workflow %q: edge references unknown target node %q", wfName, to))
					continue
				}
				fromNode, ok := fromNodeRaw.(map[string]any)
				if !ok {
					continue
				}
				fromType, _ := fromNode["type"].(string)
				if fromType == "" {
					continue
				}
				if _, found := nodes.GetDescriptor(fromType); !found {
					continue // unregistered node type: owned by check 2 above
				}

				res, cached := outputsByNode[from]
				if !cached {
					fromCfg, _ := fromNode["config"].(map[string]any)
					res.outputs, res.ok = nodes.OutputsForTypeWithConfig(fromType, fromCfg)
					outputsByNode[from] = res
				}
				if !res.ok {
					continue
				}
				outputs := res.outputs

				wantOutput := output
				if wantOutput == "" {
					wantOutput = "success"
				}
				if !containsStr(outputs, wantOutput) {
					errs = append(errs, fmt.Errorf("workflow %q: edge %q -> %q: output %q not among declared outputs [%s]",
						wfName, from, to, wantOutput, strings.Join(outputs, " ")))
				}
			}
		}
	}

	// Pre-compile all expressions and validate static fields
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
			if cfg, ok := node["config"].(map[string]any); ok {
				for _, exprErr := range expr.ValidateExpressions(compiler, cfg) {
					errs = append(errs, fmt.Errorf("workflow %q, node %q: %w", wfName, nodeID, exprErr))
				}
				if fields, ok := staticFieldsByNodeType[nodeType]; ok {
					for _, sfErr := range expr.ValidateStaticFields(cfg, fields) {
						errs = append(errs, fmt.Errorf("workflow %q, node %q: %w", wfName, nodeID, sfErr))
					}
				}
			}
		}
	}

	return errs
}

// validateNodeConfigSchema checks the node's config payload against the
// descriptor's ConfigSchema. A node without a config key validates as {}
// so required-field violations surface.
func validateNodeConfigSchema(wfName, nodeID, nodeType string, desc api.NodeDescriptor, node map[string]any) []error {
	schema := desc.ConfigSchema()
	if schema == nil {
		return nil
	}
	cfg, _ := node["config"].(map[string]any)
	if cfg == nil {
		cfg = map[string]any{}
	}
	var errs []error
	for _, scErr := range ValidateNodeConfig(schema, cfg) {
		errs = append(errs, fmt.Errorf("workflow %q, node %q (%s): %w", wfName, nodeID, nodeType, scErr))
	}
	return errs
}

func extractPrefix(nodeType string) string {
	if idx := strings.IndexByte(nodeType, '.'); idx >= 0 {
		return nodeType[:idx]
	}
	return nodeType
}

// pluginNames returns the sorted names of all registered plugins, for
// unknown-plugin error messages.
func pluginNames(plugins *PluginRegistry) []string {
	all := plugins.All()
	names := make([]string, 0, len(all))
	for _, p := range all {
		names = append(names, p.Name())
	}
	sort.Strings(names)
	return names
}

// containsStr reports whether s is present in slice.
func containsStr(slice []string, s string) bool {
	for _, v := range slice {
		if v == s {
			return true
		}
	}
	return false
}
