package engine

import (
	"fmt"
	"log/slog"
	"sync"
)

// WorkflowCache stores pre-compiled workflow graphs for reuse across requests.
// Compiled graphs are immutable — expressions within node configs are resolved
// at runtime per execution, so the cached graph is safe to share.
type WorkflowCache struct {
	mu     sync.RWMutex
	graphs map[string]*CompiledGraph
}

// buildGraphs parses+compiles all workflows and indexes each by its file key
// and (if different) its logical "id" field, rejecting any id collision.
func buildGraphs(workflows map[string]map[string]any, resolver NodeOutputResolver) (map[string]*CompiledGraph, error) {
	graphs := make(map[string]*CompiledGraph, len(workflows))
	source := make(map[string]string) // index key → file key that declared it
	put := func(key, fileKey string, g *CompiledGraph) error {
		if prev, ok := source[key]; ok {
			return fmt.Errorf("duplicate workflow id %q (declared by %q and %q)", key, prev, fileKey)
		}
		source[key] = fileKey
		graphs[key] = g
		return nil
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
		if err := put(id, id, graph); err != nil {
			return nil, err
		}
		if jsonID, ok := raw["id"].(string); ok && jsonID != id {
			if err := put(jsonID, id, graph); err != nil {
				return nil, err
			}
		}
	}
	return graphs, nil
}

// NewWorkflowCache creates a cache and pre-compiles all workflows from the given
// raw workflow map. Returns an error if any workflow fails to parse or compile.
func NewWorkflowCache(workflows map[string]map[string]any, resolver NodeOutputResolver) (*WorkflowCache, error) {
	graphs, err := buildGraphs(workflows, resolver)
	if err != nil {
		return nil, err
	}

	slog.Info("workflows compiled", "count", len(graphs))

	return &WorkflowCache{graphs: graphs}, nil
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
	newGraphs, err := buildGraphs(workflows, resolver)
	if err != nil {
		return err
	}

	c.mu.Lock()
	c.graphs = newGraphs
	c.mu.Unlock()
	return nil
}
