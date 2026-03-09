package engine

import (
	"fmt"
	"sync"
)

// WorkflowCache stores pre-compiled workflow graphs for reuse across requests.
// Compiled graphs are immutable — expressions within node configs are resolved
// at runtime per execution, so the cached graph is safe to share.
type WorkflowCache struct {
	mu     sync.RWMutex
	graphs map[string]*CompiledGraph
}

// NewWorkflowCache creates a cache and pre-compiles all workflows from the given
// raw workflow map. Returns an error if any workflow fails to parse or compile.
func NewWorkflowCache(workflows map[string]map[string]any, resolver NodeOutputResolver) (*WorkflowCache, error) {
	c := &WorkflowCache{
		graphs: make(map[string]*CompiledGraph, len(workflows)),
	}

	for id, raw := range workflows {
		wfConfig, err := ParseWorkflowFromMap(id, raw)
		if err != nil {
			return nil, fmt.Errorf("parse workflow %q: %w", id, err)
		}
		graph, err := Compile(wfConfig, resolver)
		if err != nil {
			return nil, fmt.Errorf("compile workflow %q: %w", id, err)
		}
		c.graphs[id] = graph
		// Also index by the workflow's "id" field so routes can reference by logical ID
		if jsonID, ok := raw["id"].(string); ok && jsonID != id {
			c.graphs[jsonID] = graph
		}
	}

	return c, nil
}

// Get returns the compiled graph for a workflow ID.
func (c *WorkflowCache) Get(workflowID string) (*CompiledGraph, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	g, ok := c.graphs[workflowID]
	return g, ok
}

// Invalidate clears and rebuilds the cache. Used by dev mode hot reload.
func (c *WorkflowCache) Invalidate(workflows map[string]map[string]any, resolver NodeOutputResolver) error {
	newGraphs := make(map[string]*CompiledGraph, len(workflows))
	for id, raw := range workflows {
		wfConfig, err := ParseWorkflowFromMap(id, raw)
		if err != nil {
			return fmt.Errorf("parse workflow %q: %w", id, err)
		}
		graph, err := Compile(wfConfig, resolver)
		if err != nil {
			return fmt.Errorf("compile workflow %q: %w", id, err)
		}
		newGraphs[id] = graph
		// Also index by the workflow's "id" field so routes can reference by logical ID
		if jsonID, ok := raw["id"].(string); ok && jsonID != id {
			newGraphs[jsonID] = graph
		}
	}

	c.mu.Lock()
	c.graphs = newGraphs
	c.mu.Unlock()
	return nil
}
