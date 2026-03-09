package config

import "fmt"

// ValidateCrossRefs validates references between config files.
func ValidateCrossRefs(rc *RawConfig) []ValidationError {
	var errs []ValidationError

	workflowIDs := collectIDs(rc.Workflows)
	serviceMap := collectServices(rc.Root)
	presets := collectPresets(rc.Root)

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
		errs = append(errs, validateMiddlewareRefs(filePath, route, presets)...)
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

func validateMiddlewareRefs(filePath string, route map[string]any, presets map[string]bool) []ValidationError {
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
				for cur != next {
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

func formatCycle(ids []string) string {
	result := ""
	for i, id := range ids {
		if i > 0 {
			result += " → "
		}
		result += id
	}
	return result
}
