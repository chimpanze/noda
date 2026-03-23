package engine

import (
	"fmt"
	"time"
)

// ParseWorkflowFromMap converts a raw workflow JSON map into a WorkflowConfig.
// This is the canonical parser used by the server, worker, scheduler, and testing packages.
func ParseWorkflowFromMap(id string, raw map[string]any) (WorkflowConfig, error) {
	wf := WorkflowConfig{
		ID:      id,
		Timeout: MapStrVal(raw, "timeout"),
		Nodes:   make(map[string]NodeConfig),
	}

	nodesRaw, _ := raw["nodes"].(map[string]any)
	for nodeID, nodeRaw := range nodesRaw {
		nm, ok := nodeRaw.(map[string]any)
		if !ok {
			return wf, fmt.Errorf("node %q: invalid format", nodeID)
		}
		nc := NodeConfig{
			Type: MapStrVal(nm, "type"),
			As:   MapStrVal(nm, "as"),
		}
		if cfg, ok := nm["config"].(map[string]any); ok {
			nc.Config = cfg
		}
		if svc, ok := nm["services"].(map[string]any); ok {
			nc.Services = make(map[string]string)
			for k, v := range svc {
				nc.Services[k] = fmt.Sprintf("%v", v)
			}
		}
		wf.Nodes[nodeID] = nc
	}

	edgesRaw, _ := raw["edges"].([]any)
	for _, edgeRaw := range edgesRaw {
		em, ok := edgeRaw.(map[string]any)
		if !ok {
			continue
		}
		ec := EdgeConfig{
			From:   MapStrVal(em, "from"),
			To:     MapStrVal(em, "to"),
			Output: MapStrVal(em, "output"),
		}
		if retryRaw, ok := em["retry"].(map[string]any); ok {
			rc := &RetryConfig{
				Backoff: MapStrVal(retryRaw, "backoff"),
				Delay:   MapStrVal(retryRaw, "delay"),
			}
			if a, ok := retryRaw["attempts"].(float64); ok {
				rc.Attempts = int(a)
			}
			if err := validateRetryConfig(rc, ec.From, ec.To); err != nil {
				return wf, err
			}
			ec.Retry = rc
		}
		wf.Edges = append(wf.Edges, ec)
	}

	return wf, nil
}

// MapStrVal extracts a string value from a map, returning "" if not found or not a string.
func MapStrVal(m map[string]any, key string) string {
	v, _ := m[key].(string)
	return v
}

// validateRetryConfig checks that retry configuration is valid at parse time,
// rather than silently defaulting at runtime.
func validateRetryConfig(rc *RetryConfig, from, to string) error {
	edgeDesc := fmt.Sprintf("edge %s → %s", from, to)
	if rc.Attempts < 1 {
		return fmt.Errorf("%s: retry attempts must be >= 1, got %d", edgeDesc, rc.Attempts)
	}
	if rc.Delay == "" {
		return fmt.Errorf("%s: retry delay is required", edgeDesc)
	}
	if _, err := time.ParseDuration(rc.Delay); err != nil {
		return fmt.Errorf("%s: invalid retry delay %q: %w", edgeDesc, rc.Delay, err)
	}
	if rc.Backoff == "" {
		rc.Backoff = "fixed"
	} else if rc.Backoff != "fixed" && rc.Backoff != "exponential" {
		return fmt.Errorf("%s: retry backoff must be \"fixed\" or \"exponential\", got %q", edgeDesc, rc.Backoff)
	}
	return nil
}
