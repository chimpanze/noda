package engine

import (
	"strings"
	"sync/atomic"

	"github.com/chimpanze/noda/internal/expr"
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
		for id, node := range graph.Nodes {
			if id == nodeID || consumerSet[id] {
				continue
			}
			if configReferences(node.Config, outputKey) {
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
func (t *EvictionTracker) NodeCompleted(nodeID string, graph *CompiledGraph) {
	for _, outputKey := range t.consumers[nodeID] {
		if ref, ok := t.refs[outputKey]; ok {
			if ref.Add(-1) == 0 {
				t.execCtx.EvictOutput(outputKey)
			}
		}
	}
}

// configReferences checks if a config map contains expression references to an output key.
// Uses the expr parser to correctly handle string literals and nested braces.
func configReferences(config map[string]any, key string) bool {
	for _, v := range config {
		switch val := v.(type) {
		case string:
			if stringReferencesKey(val, key) {
				return true
			}
		case map[string]any:
			if configReferences(val, key) {
				return true
			}
		case []any:
			for _, item := range val {
				if s, ok := item.(string); ok && stringReferencesKey(s, key) {
					return true
				}
				if m, ok := item.(map[string]any); ok && configReferences(m, key) {
					return true
				}
			}
		}
	}
	return false
}

// stringReferencesKey checks if a string contains a reference to the given key
// within {{ }} expression segments. Uses the expr parser for correct delimiting
// (handles string literals, nested braces, etc.).
func stringReferencesKey(s, key string) bool {
	parsed, err := expr.Parse(s)
	if err != nil || parsed.IsLiteral {
		return false
	}

	for _, seg := range parsed.Segments {
		if seg.Type != expr.SegmentExpression {
			continue
		}
		if exprContainsIdentifier(seg.Value, key) {
			return true
		}
	}
	return false
}

// exprContainsIdentifier checks if an expression string contains the given key
// as a whole identifier (not as a substring of another identifier).
func exprContainsIdentifier(expression, key string) bool {
	idx := strings.Index(expression, key)
	for idx != -1 {
		afterKey := idx + len(key)
		// Check character after is not part of an identifier
		if afterKey >= len(expression) || !isIdentChar(expression[afterKey]) {
			// Check character before is not part of an identifier
			if idx == 0 || !isIdentChar(expression[idx-1]) {
				return true
			}
		}
		// Continue searching
		next := strings.Index(expression[idx+1:], key)
		if next == -1 {
			break
		}
		idx = idx + 1 + next
	}
	return false
}

// isIdentChar returns true if c is valid in a node ID (alphanumeric, underscore, hyphen).
func isIdentChar(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_' || c == '-'
}
