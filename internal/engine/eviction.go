package engine

import (
	"strings"
	"sync"
	"sync/atomic"
)

// EvictionTracker tracks reference counts for node outputs and evicts them
// when no downstream nodes need them.
type EvictionTracker struct {
	mu      sync.Mutex
	refs    map[string]*atomic.Int32 // output key → reference count
	execCtx *ExecutionContextImpl
}

// NewEvictionTracker creates an eviction tracker from a compiled graph.
// It analyzes which downstream nodes reference each output.
func NewEvictionTracker(graph *CompiledGraph, execCtx *ExecutionContextImpl) *EvictionTracker {
	refs := make(map[string]*atomic.Int32)

	// For each node, count how many downstream nodes could consume its output.
	// A node's output is referenced by any node reachable through its outbound edges.
	for nodeID, node := range graph.Nodes {
		outputKey := nodeID
		if node.As != "" {
			outputKey = node.As
		}

		// Count downstream consumers by analyzing which nodes reference this output
		// in their config expressions or are direct edge targets
		consumers := countConsumers(graph, nodeID, outputKey)
		if consumers > 0 {
			ref := &atomic.Int32{}
			ref.Store(int32(consumers))
			refs[outputKey] = ref
		}
	}

	return &EvictionTracker{
		refs:    refs,
		execCtx: execCtx,
	}
}

// NodeCompleted is called after a node finishes executing.
// It decrements reference counts for all upstream outputs the node consumed.
func (t *EvictionTracker) NodeCompleted(nodeID string, graph *CompiledGraph) {
	// Find all upstream nodes (direct parents)
	for _, parentID := range graph.Reverse[nodeID] {
		parentNode := graph.Nodes[parentID]
		outputKey := parentID
		if parentNode.As != "" {
			outputKey = parentNode.As
		}

		t.mu.Lock()
		ref, ok := t.refs[outputKey]
		t.mu.Unlock()

		if ok && ref.Add(-1) == 0 {
			t.execCtx.EvictOutput(outputKey)
		}
	}
}

// countConsumers counts how many direct downstream nodes a given node has.
func countConsumers(graph *CompiledGraph, nodeID, outputKey string) int {
	// Count direct edge targets plus any nodes whose config references this output
	directTargets := make(map[string]bool)
	for _, targets := range graph.Adjacency[nodeID] {
		for _, t := range targets {
			directTargets[t] = true
		}
	}

	// Also scan all downstream nodes for expression references to this output key
	for id, node := range graph.Nodes {
		if id == nodeID {
			continue
		}
		if directTargets[id] {
			continue // already counted
		}
		if configReferences(node.Config, outputKey) {
			directTargets[id] = true
		}
	}

	return len(directTargets)
}

// configReferences checks if a config map contains references to an output key.
func configReferences(config map[string]any, key string) bool {
	for _, v := range config {
		switch val := v.(type) {
		case string:
			if strings.Contains(val, key) {
				return true
			}
		case map[string]any:
			if configReferences(val, key) {
				return true
			}
		}
	}
	return false
}
