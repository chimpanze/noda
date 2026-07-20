package config

import (
	"fmt"
	"math"
	"strings"
	"time"
)

// ValidateCrossRefs validates references between config files.
func ValidateCrossRefs(rc *RawConfig) []ValidationError {
	var errs []ValidationError

	workflowIDs := collectIDs(rc.Workflows)
	serviceMap := collectServices(rc.Root)
	presets := collectPresets(rc.Root)
	instances := collectInstances(rc.Root)
	endpoints := collectEndpoints(rc.Connections)

	// Route → Workflow
	for filePath, route := range rc.Routes {
		if trigger, ok := route["trigger"].(map[string]any); ok {
			if wfID, ok := trigger["workflow"].(string); ok {
				if !workflowIDs[wfID] {
					errs = append(errs, ValidationError{
						FilePath: filePath,
						JSONPath: "/trigger/workflow",
						Message:  fmt.Sprintf("references non-existent workflow %q", wfID),
					})
				}
			}
		}
		// Route middleware presets
		errs = append(errs, validateMiddlewareRefs(filePath, route, presets, instances)...)
	}

	// Worker → Stream service and Workflow
	for filePath, worker := range rc.Workers {
		if services, ok := worker["services"].(map[string]any); ok {
			if streamName, ok := services["stream"].(string); ok {
				if plugin, exists := serviceMap[streamName]; !exists {
					errs = append(errs, ValidationError{
						FilePath: filePath,
						JSONPath: "/services/stream",
						Message:  fmt.Sprintf("references non-existent service %q", streamName),
					})
				} else if plugin != "stream" {
					errs = append(errs, ValidationError{
						FilePath: filePath,
						JSONPath: "/services/stream",
						Message:  fmt.Sprintf("service %q has plugin %q, expected \"stream\"", streamName, plugin),
					})
				}
			}
		}
		if trigger, ok := worker["trigger"].(map[string]any); ok {
			if wfID, ok := trigger["workflow"].(string); ok {
				if !workflowIDs[wfID] {
					errs = append(errs, ValidationError{
						FilePath: filePath,
						JSONPath: "/trigger/workflow",
						Message:  fmt.Sprintf("references non-existent workflow %q", wfID),
					})
				}
			}
		}
	}

	// Schedule → lock TTL vs timeout validation
	for filePath, schedule := range rc.Schedules {
		if lock, ok := schedule["lock"].(map[string]any); ok {
			if ttlStr, ok := lock["ttl"].(string); ok {
				if timeoutStr, ok := schedule["timeout"].(string); ok {
					ttl, ttlErr := time.ParseDuration(ttlStr)
					tout, toutErr := time.ParseDuration(timeoutStr)
					if ttlErr == nil && toutErr == nil && ttl < tout {
						errs = append(errs, ValidationError{
							FilePath: filePath,
							JSONPath: "/lock/ttl",
							Message:  fmt.Sprintf("lock TTL (%s) is less than job timeout (%s) — lock may expire before the job finishes", ttlStr, timeoutStr),
						})
					}
				}
			}
		}
	}

	// Schedule → Cache service (lock) and Workflow
	for filePath, schedule := range rc.Schedules {
		if lock, ok := schedule["lock"].(map[string]any); ok {
			if enabled, ok := lock["enabled"].(bool); ok && enabled {
				if services, ok := schedule["services"].(map[string]any); ok {
					if lockName, ok := services["lock"].(string); ok {
						if plugin, exists := serviceMap[lockName]; !exists {
							errs = append(errs, ValidationError{
								FilePath: filePath,
								JSONPath: "/services/lock",
								Message:  fmt.Sprintf("references non-existent service %q", lockName),
							})
						} else if plugin != "cache" {
							errs = append(errs, ValidationError{
								FilePath: filePath,
								JSONPath: "/services/lock",
								Message:  fmt.Sprintf("service %q has plugin %q, expected \"cache\"", lockName, plugin),
							})
						}
					}
				}
			}
		}
		if trigger, ok := schedule["trigger"].(map[string]any); ok {
			if wfID, ok := trigger["workflow"].(string); ok {
				if !workflowIDs[wfID] {
					errs = append(errs, ValidationError{
						FilePath: filePath,
						JSONPath: "/trigger/workflow",
						Message:  fmt.Sprintf("references non-existent workflow %q", wfID),
					})
				}
			}
		}
	}

	// Connection → PubSub and lifecycle workflows
	for filePath, conn := range rc.Connections {
		if sync, ok := conn["sync"].(map[string]any); ok {
			if pubsubName, ok := sync["pubsub"].(string); ok {
				if plugin, exists := serviceMap[pubsubName]; !exists {
					errs = append(errs, ValidationError{
						FilePath: filePath,
						JSONPath: "/sync/pubsub",
						Message:  fmt.Sprintf("references non-existent service %q", pubsubName),
					})
				} else if plugin != "pubsub" {
					errs = append(errs, ValidationError{
						FilePath: filePath,
						JSONPath: "/sync/pubsub",
						Message:  fmt.Sprintf("service %q has plugin %q, expected \"pubsub\"", pubsubName, plugin),
					})
				}
			}
		}
		if endpoints, ok := conn["endpoints"].(map[string]any); ok {
			for epName, epVal := range endpoints {
				if ep, ok := epVal.(map[string]any); ok {
					for _, field := range []string{"on_connect", "on_message", "on_disconnect"} {
						if wfID, ok := ep[field].(string); ok {
							if !workflowIDs[wfID] {
								errs = append(errs, ValidationError{
									FilePath: filePath,
									JSONPath: fmt.Sprintf("/endpoints/%s/%s", epName, field),
									Message:  fmt.Sprintf("references non-existent workflow %q", wfID),
								})
							}
						}
					}
				}
			}
		}
	}

	// Workflow → Workflow (workflow.run and control.loop)
	wfGraph := make(map[string][]string) // workflow ID → referenced workflow IDs
	for filePath, wf := range rc.Workflows {
		wfID, _ := wf["id"].(string)
		if nodes, ok := wf["nodes"].(map[string]any); ok {
			for nodeID, nodeVal := range nodes {
				if node, ok := nodeVal.(map[string]any); ok {
					nodeType, _ := node["type"].(string)
					// ws.send/sse.send bind the "connections" service slot to a
					// connections endpoint name (the endpoint is registered as a
					// service under its own name). Validate the referenced endpoint exists.
					if nodeType == "ws.send" || nodeType == "sse.send" {
						if svcs, ok := node["services"].(map[string]any); ok {
							if epName, ok := svcs["connections"].(string); ok && !endpoints[epName] {
								msg := fmt.Sprintf("references non-existent connections endpoint %q", epName)
								if len(endpoints) == 0 {
									// #380: distinguish "typo" from "the config
									// was never discovered" — endpoints are only
									// read from the connections/ directory.
									msg += " (no connections endpoints are defined anywhere — define them in a connections/*.json file, e.g. connections/realtime.json)"
								}
								errs = append(errs, ValidationError{
									FilePath: filePath,
									JSONPath: fmt.Sprintf("/nodes/%s/services/connections", nodeID),
									Message:  msg,
								})
							}
						}
					}
					if nodeType == "workflow.run" || nodeType == "control.loop" {
						if cfg, ok := node["config"].(map[string]any); ok {
							if targetID, ok := cfg["workflow"].(string); ok {
								if !workflowIDs[targetID] {
									errs = append(errs, ValidationError{
										FilePath: filePath,
										JSONPath: fmt.Sprintf("/nodes/%s/config/workflow", nodeID),
										Message:  fmt.Sprintf("references non-existent workflow %q", targetID),
									})
								}
								if wfID != "" {
									wfGraph[wfID] = append(wfGraph[wfID], targetID)
								}
							}
						}
					}
				}
			}
		}
	}

	// Detect circular workflow references
	errs = append(errs, detectWorkflowCycles(wfGraph)...)

	// Validate duration fields in routes
	for filePath, route := range rc.Routes {
		if v, ok := route["response_timeout"].(string); ok {
			if _, err := time.ParseDuration(v); err != nil {
				errs = append(errs, ValidationError{
					FilePath: filePath,
					JSONPath: "/response_timeout",
					Message:  fmt.Sprintf("invalid duration %q: %v", v, err),
				})
			}
		}
	}

	// Validate route trigger file mappings. A "files" entry marks which
	// trigger.input keys carry multipart file streams — it does not create
	// them (internal/server/trigger.go getFileFields). A files entry with no
	// matching input key means the stream never reaches the workflow and
	// every real upload fails.
	for filePath, route := range rc.Routes {
		trigger, ok := route["trigger"].(map[string]any)
		if !ok {
			continue
		}
		files, ok := trigger["files"].([]any)
		if !ok {
			continue
		}
		input, _ := trigger["input"].(map[string]any)
		for _, f := range files {
			name, ok := f.(string)
			if !ok {
				continue
			}
			if _, present := input[name]; !present {
				errs = append(errs, ValidationError{
					FilePath: filePath,
					JSONPath: "/trigger/files",
					Message: fmt.Sprintf(
						`files entry %q has no matching trigger.input key — add "%s": "%s" to trigger.input, otherwise the multipart stream never reaches the workflow`,
						name, name, name),
				})
			}
		}
	}

	// Validate server scalar settings. These may be strings after $env()
	// resolution, so the schema admits both forms and the semantic check
	// happens here.
	if rc.Root != nil {
		if _, ok := rc.Root["server"].(map[string]any); ok {
			for _, field := range []string{"read_timeout", "write_timeout", "response_timeout", "shutdown_deadline", "health_timeout"} {
				if _, _, err := ServerDuration(rc.Root, field); err != nil {
					errs = append(errs, ValidationError{
						FilePath: "noda.json",
						JSONPath: "/server/" + field,
						Message:  err.Error(),
					})
				}
			}
			for _, f := range []struct {
				key      string
				min, max int
			}{
				{"port", 1, 65535},
				{"body_limit", 0, math.MaxInt},
				{"expression_memory_budget", 0, math.MaxInt},
			} {
				v, ok, err := ServerInt(rc.Root, f.key)
				switch {
				case err != nil:
					errs = append(errs, ValidationError{
						FilePath: "noda.json",
						JSONPath: "/server/" + f.key,
						Message:  err.Error(),
					})
				case ok && (v < f.min || v > f.max):
					errs = append(errs, ValidationError{
						FilePath: "noda.json",
						JSONPath: "/server/" + f.key,
						Message:  fmt.Sprintf("value %d out of range [%d, %d]", v, f.min, f.max),
					})
				}
			}
			if _, err := ServerTrustProxy(rc.Root); err != nil {
				errs = append(errs, ValidationError{
					FilePath: "noda.json",
					JSONPath: "/server/trust_proxy",
					Message:  err.Error(),
				})
			}
			if _, err := ServerOpenAPI(rc.Root); err != nil {
				errs = append(errs, ValidationError{
					FilePath: "noda.json",
					JSONPath: "/server/openapi",
					Message:  err.Error(),
				})
			}
		}
	}

	// Validate duration fields in workers
	for filePath, worker := range rc.Workers {
		if v, ok := worker["timeout"].(string); ok {
			if _, err := time.ParseDuration(v); err != nil {
				errs = append(errs, ValidationError{
					FilePath: filePath,
					JSONPath: "/timeout",
					Message:  fmt.Sprintf("invalid duration %q: %v", v, err),
				})
			}
		}
	}

	// Validate duration fields in connection endpoints
	for filePath, conn := range rc.Connections {
		if endpoints, ok := conn["endpoints"].(map[string]any); ok {
			for epName, epVal := range endpoints {
				if ep, ok := epVal.(map[string]any); ok {
					for _, field := range []string{"ping_interval", "heartbeat", "retry"} {
						if v, ok := ep[field].(string); ok {
							if _, err := time.ParseDuration(v); err != nil {
								errs = append(errs, ValidationError{
									FilePath: filePath,
									JSONPath: fmt.Sprintf("/endpoints/%s/%s", epName, field),
									Message:  fmt.Sprintf("invalid duration %q: %v", v, err),
								})
							}
						}
					}
				}
			}
		}
	}

	// Warn if CORS middleware is used but no allow_origins is configured
	if corsUsed(rc) {
		if rc.Root != nil {
			security, _ := rc.Root["security"].(map[string]any)
			corsCfg, _ := security["cors"].(map[string]any)
			if corsCfg == nil || corsCfg["allow_origins"] == nil || corsCfg["allow_origins"] == "" {
				errs = append(errs, ValidationError{
					FilePath: "noda.json",
					JSONPath: "/security/cors/allow_origins",
					Message:  "warning: CORS middleware is active but allow_origins is not configured; will default to localhost only",
				})
			}
		}
	}

	// Validate global_middleware entries are strings
	if rc.Root != nil {
		if mw, ok := rc.Root["global_middleware"].([]any); ok {
			for i, v := range mw {
				if _, ok := v.(string); !ok {
					errs = append(errs, ValidationError{
						FilePath: "noda.json",
						JSONPath: fmt.Sprintf("/global_middleware/%d", i),
						Message:  fmt.Sprintf("expected string, got %T", v),
					})
				}
			}
		}
	}

	return errs
}

func collectIDs(configs map[string]map[string]any) map[string]bool {
	ids := make(map[string]bool)
	for _, data := range configs {
		if id, ok := data["id"].(string); ok {
			ids[id] = true
		}
	}
	return ids
}

// collectEndpoints gathers every connections endpoint name across all
// connections files. Each endpoint is registered at runtime as a service
// under its own name, so ws.send/sse.send bind their "connections" slot to it.
func collectEndpoints(connections map[string]map[string]any) map[string]bool {
	result := make(map[string]bool)
	for _, conn := range connections {
		if eps, ok := conn["endpoints"].(map[string]any); ok {
			for name := range eps {
				result[name] = true
			}
		}
	}
	return result
}

func collectServices(root map[string]any) map[string]string {
	result := make(map[string]string)
	if root == nil {
		return result
	}
	services, ok := root["services"].(map[string]any)
	if !ok {
		return result
	}
	for name, svc := range services {
		if svcMap, ok := svc.(map[string]any); ok {
			if plugin, ok := svcMap["plugin"].(string); ok {
				result[name] = plugin
			}
		}
	}
	return result
}

func collectPresets(root map[string]any) map[string]bool {
	result := make(map[string]bool)
	if root == nil {
		return result
	}
	presets, ok := root["middleware_presets"].(map[string]any)
	if !ok {
		return result
	}
	for name := range presets {
		result[name] = true
	}
	return result
}

func collectInstances(root map[string]any) map[string]bool {
	result := make(map[string]bool)
	if root == nil {
		return result
	}
	instances, ok := root["middleware_instances"].(map[string]any)
	if !ok {
		return result
	}
	for name := range instances {
		result[name] = true
	}
	return result
}

func validateMiddlewareRefs(filePath string, route map[string]any, presets map[string]bool, instances map[string]bool) []ValidationError {
	var errs []ValidationError
	mw, ok := route["middleware"].([]any)
	if !ok {
		return nil
	}
	for i, item := range mw {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if preset, ok := m["preset"].(string); ok {
			if !presets[preset] {
				errs = append(errs, ValidationError{
					FilePath: filePath,
					JSONPath: fmt.Sprintf("/middleware/%d/preset", i),
					Message:  fmt.Sprintf("references non-existent middleware preset %q", preset),
				})
			}
		}
		if name, ok := m["use"].(string); ok {
			if idx := strings.Index(name, ":"); idx >= 0 {
				// Instance reference like "auth.jwt:prod"
				if !instances[name] {
					errs = append(errs, ValidationError{
						FilePath: filePath,
						JSONPath: fmt.Sprintf("/middleware/%d/use", i),
						Message:  fmt.Sprintf("references non-existent middleware instance %q", name),
					})
				}
			}
		}
	}
	return errs
}

// detectWorkflowCycles uses DFS to find circular references in the workflow
// dependency graph and returns validation errors for each cycle found.
func detectWorkflowCycles(graph map[string][]string) []ValidationError {
	var errs []ValidationError

	const (
		white = 0 // unvisited
		gray  = 1 // in current path
		black = 2 // finished
	)

	color := make(map[string]int)
	parent := make(map[string]string)

	var dfs func(node string)
	dfs = func(node string) {
		color[node] = gray
		for _, next := range graph[node] {
			if color[next] == gray {
				// Cycle found — reconstruct path
				cycle := []string{next, node}
				cur := node
				for i := 0; cur != next && i < len(graph)+1; i++ {
					cur = parent[cur]
					cycle = append(cycle, cur)
				}
				// Reverse for readable order
				for i, j := 0, len(cycle)-1; i < j; i, j = i+1, j-1 {
					cycle[i], cycle[j] = cycle[j], cycle[i]
				}
				errs = append(errs, ValidationError{
					Message: fmt.Sprintf("circular workflow reference: %s", formatCycle(cycle)),
				})
				return
			}
			if color[next] == white {
				parent[next] = node
				dfs(next)
			}
		}
		color[node] = black
	}

	for node := range graph {
		if color[node] == white {
			dfs(node)
		}
	}

	return errs
}

// corsUsed returns true if security.cors appears in any middleware chain
// (global_middleware, middleware_presets, or route middleware).
func corsUsed(rc *RawConfig) bool {
	if rc.Root != nil {
		// Check global_middleware
		if mw, ok := rc.Root["global_middleware"].([]any); ok {
			for _, v := range mw {
				if s, ok := v.(string); ok && s == "security.cors" {
					return true
				}
			}
		}
		// Check middleware_presets
		if presets, ok := rc.Root["middleware_presets"].(map[string]any); ok {
			for _, v := range presets {
				if arr, ok := v.([]any); ok {
					for _, item := range arr {
						if s, ok := item.(string); ok && s == "security.cors" {
							return true
						}
					}
				}
			}
		}
	}
	// Check route middleware
	for _, route := range rc.Routes {
		if mw, ok := route["middleware"].([]any); ok {
			for _, item := range mw {
				if m, ok := item.(map[string]any); ok {
					if use, ok := m["use"].(string); ok && use == "security.cors" {
						return true
					}
				}
			}
		}
	}
	return false
}

func formatCycle(ids []string) string {
	return strings.Join(ids, " → ")
}
