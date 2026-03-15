package engine

import (
	"sync/atomic"
)

// EvictionTracker tracks reference counts for node outputs and evicts them
// when no downstream nodes need them.
type EvictionTracker struct {
	refs      map[string]*atomic.Int32 // output key → reference count
	consumers map[string][]string      // nodeID → output keys it consumes
	execCtx   *ExecutionContextImpl
}

// NewEvictionTracker creates an eviction tracker from a compiled graph.
// It analyzes which downstream nodes reference each output.
// Terminal node outputs are never evicted (they are the workflow's results).
func NewEvictionTracker(graph *CompiledGraph, execCtx *ExecutionContextImpl) *EvictionTracker {
	refs := make(map[string]*atomic.Int32)
	consumers := make(map[string][]string) // nodeID → output keys it references

	// Build set of terminal node IDs for quick lookup
	terminalSet := make(map[string]bool, len(graph.TerminalNodes))
	for _, id := range graph.TerminalNodes {
		terminalSet[id] = true
	}

	// Build output key map: nodeID → outputKey
	outputKeys := make(map[string]string, len(graph.Nodes))
	for nodeID, node := range graph.Nodes {
		key := nodeID
		if node.As != "" {
			key = node.As
		}
		outputKeys[nodeID] = key
	}

	// Pre-compute reverse config-ref index: outputKey → set of nodes referencing it.
	// This avoids O(n^2) iteration when building consumer sets below.
	reverseConfigRefs := make(map[string][]string)
	for id, node := range graph.Nodes {
		for ref := range node.ConfigRefs {
			reverseConfigRefs[ref] = append(reverseConfigRefs[ref], id)
		}
	}

	// For each non-terminal node, find all consumers and build the reverse map.
	for nodeID := range graph.Nodes {
		if terminalSet[nodeID] {
			continue
		}
		outputKey := outputKeys[nodeID]

		// Find all nodes that consume this output (direct edges + expression refs)
		consumerSet := make(map[string]bool)
		for _, targets := range graph.Adjacency[nodeID] {
			for _, t := range targets {
				consumerSet[t] = true
			}
		}
		for _, id := range reverseConfigRefs[outputKey] {
			if id != nodeID {
				consumerSet[id] = true
			}
		}

		if len(consumerSet) > 0 {
			ref := &atomic.Int32{}
			ref.Store(int32(len(consumerSet)))
			refs[outputKey] = ref

			// Register this output key as consumed by each consumer node
			for consumerID := range consumerSet {
				consumers[consumerID] = append(consumers[consumerID], outputKey)
			}
		}
	}

	return &EvictionTracker{
		refs:      refs,
		consumers: consumers,
		execCtx:   execCtx,
	}
}

// NodeCompleted is called after a node finishes executing.
// It decrements reference counts for all outputs the node consumed
// (both via direct edges and expression references).
func (t *EvictionTracker) NodeCompleted(nodeID string) {
	for _, outputKey := range t.consumers[nodeID] {
		if ref, ok := t.refs[outputKey]; ok {
			if ref.Add(-1) == 0 {
				t.execCtx.EvictOutput(outputKey)
			}
		}
	}
}
