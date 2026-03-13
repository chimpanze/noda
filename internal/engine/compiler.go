package engine

import (
	"fmt"
	"strings"

	"github.com/chimpanze/noda/internal/expr"
)

// JoinType indicates how a node handles multiple inbound edges.
type JoinType int

const (
	JoinNone JoinType = iota // single inbound edge or entry node
	JoinAND                  // wait for ALL inbound edges (parallel branches)
	JoinOR                   // wait for whichever fires (conditional branches)
)

// EdgeConfig represents a connection between two nodes.
type EdgeConfig struct {
	From   string       `json:"from"`
	To     string       `json:"to"`
	Output string       `json:"output"` // defaults to "success"
	Retry  *RetryConfig `json:"retry,omitempty"`
}

// RetryConfig defines retry behavior on error edges.
type RetryConfig struct {
	Attempts int    `json:"attempts"`
	Backoff  string `json:"backoff"` // "fixed" or "exponential"
	Delay    string `json:"delay"`   // duration string, e.g. "1s"
}

// NodeConfig represents a node in a workflow.
type NodeConfig struct {
	Type     string            `json:"type"`
	Services map[string]string `json:"services,omitempty"`
	As       string            `json:"as,omitempty"`
	Config   map[string]any    `json:"config,omitempty"`
}

// WorkflowConfig represents a parsed workflow definition.
type WorkflowConfig struct {
	ID    string
	Nodes map[string]NodeConfig
	Edges []EdgeConfig
}

// CompiledNode holds compiled metadata for a single node.
type CompiledNode struct {
	ID       string
	Type     string
	As       string // output alias
	Config   map[string]any
	Services map[string]string
	Outputs  []string // valid output names from the node descriptor

	// ConfigRefs holds identifiers referenced in config expressions,
	// pre-computed at compile time for use by eviction tracking.
	ConfigRefs map[string]bool
}

// CompiledEdge represents a compiled edge with resolved retry config.
type CompiledEdge struct {
	From   string
	To     string
	Output string
	Retry  *RetryConfig
}

// CompiledGraph is the executable representation of a workflow.
type CompiledGraph struct {
	WorkflowID string
	Nodes      map[string]*CompiledNode

	// Adjacency: nodeID → output → []targetNodeID
	Adjacency map[string]map[string][]string

	// Edges: from:output:to → edge
	Edges map[string]*CompiledEdge

	// Reverse adjacency: nodeID → []sourceNodeID
	Reverse map[string][]string

	// Entry nodes: nodes with no inbound edges
	EntryNodes []string

	// Terminal nodes: nodes with no outbound success edges (error-only edges don't count)
	TerminalNodes []string

	// Dependency count: how many inbound edges before a node can run
	DepCount map[string]int

	// Join type per node
	JoinTypes map[string]JoinType
}

// NodeOutputResolver resolves the valid outputs for a node type.
// In production this delegates to the NodeRegistry; tests can provide stubs.
type NodeOutputResolver interface {
	OutputsForType(nodeType string) ([]string, bool)
}

// ConfigAwareResolver is an optional interface that resolvers can implement
// to resolve outputs using the node's config. This is needed for nodes like
// control.switch whose outputs depend on their "cases" config.
type ConfigAwareResolver interface {
	OutputsForTypeWithConfig(nodeType string, config map[string]any) ([]string, bool)
}

// DefaultOutputResolver returns ["success", "error"] for all types.
type DefaultOutputResolver struct{}

func (d *DefaultOutputResolver) OutputsForType(string) ([]string, bool) {
	return []string{"success", "error"}, true
}

// Compile converts a workflow config into an executable graph.
func Compile(wf WorkflowConfig, resolver NodeOutputResolver) (*CompiledGraph, error) {
	if resolver == nil {
		resolver = &DefaultOutputResolver{}
	}

	g := &CompiledGraph{
		WorkflowID: wf.ID,
		Nodes:      make(map[string]*CompiledNode),
		Adjacency:  make(map[string]map[string][]string),
		Edges:      make(map[string]*CompiledEdge),
		Reverse:    make(map[string][]string),
		DepCount:   make(map[string]int),
		JoinTypes:  make(map[string]JoinType),
	}

	// Compile nodes
	for id, nc := range wf.Nodes {
		var outputs []string
		if car, ok := resolver.(ConfigAwareResolver); ok {
			outputs, _ = car.OutputsForTypeWithConfig(nc.Type, nc.Config)
		} else {
			outputs, _ = resolver.OutputsForType(nc.Type)
		}
		g.Nodes[id] = &CompiledNode{
			ID:         id,
			Type:       nc.Type,
			As:         nc.As,
			Config:     nc.Config,
			Services:   nc.Services,
			Outputs:    outputs,
			ConfigRefs: extractConfigRefs(nc.Config),
		}
		g.Adjacency[id] = make(map[string][]string)
	}

	// Validate alias uniqueness
	aliases := make(map[string]string) // alias -> nodeID
	for id, node := range wf.Nodes {
		if node.As != "" {
			if existingID, exists := aliases[node.As]; exists {
				return nil, fmt.Errorf("duplicate alias %q: used by both node %q and %q", node.As, existingID, id)
			}
			aliases[node.As] = id
		}
	}

	// Compile edges
	for _, edge := range wf.Edges {
		if _, ok := g.Nodes[edge.From]; !ok {
			return nil, fmt.Errorf("edge references unknown source node %q", edge.From)
		}
		if _, ok := g.Nodes[edge.To]; !ok {
			return nil, fmt.Errorf("edge references unknown target node %q", edge.To)
		}

		output := edge.Output
		if output == "" {
			output = "success"
		}

		// Validate output name
		node := g.Nodes[edge.From]
		if !containsString(node.Outputs, output) {
			return nil, fmt.Errorf("node %q has no output %q (valid: %s)", edge.From, output, strings.Join(node.Outputs, ", "))
		}

		// Validate retry only on error edges
		if edge.Retry != nil && output != "error" {
			return nil, fmt.Errorf("retry config only valid on error edges, found on %q output of node %q", output, edge.From)
		}

		g.Adjacency[edge.From][output] = append(g.Adjacency[edge.From][output], edge.To)
		g.Reverse[edge.To] = append(g.Reverse[edge.To], edge.From)
		g.DepCount[edge.To]++

		edgeKey := fmt.Sprintf("%s:%s:%s", edge.From, output, edge.To)
		retry := edge.Retry
		g.Edges[edgeKey] = &CompiledEdge{
			From:   edge.From,
			To:     edge.To,
			Output: output,
			Retry:  retry,
		}
	}

	// Identify entry nodes (no inbound edges)
	for id := range g.Nodes {
		if len(g.Reverse[id]) == 0 {
			g.EntryNodes = append(g.EntryNodes, id)
		}
	}

	// Identify terminal nodes (no outbound success edges).
	// Nodes with only error edges are still terminal — error edges represent
	// exceptional flow, so the node's output must be preserved for inspection.
	for id := range g.Nodes {
		if len(g.Adjacency[id]["success"]) == 0 {
			g.TerminalNodes = append(g.TerminalNodes, id)
		}
	}

	// Cycle detection
	if err := detectCycle(g); err != nil {
		return nil, err
	}

	// Compute join types
	computeJoinTypes(g)

	return g, nil
}

// detectCycle uses DFS to detect cycles in the graph.
func detectCycle(g *CompiledGraph) error {
	const (
		white = 0 // unvisited
		gray  = 1 // in current DFS path
		black = 2 // fully processed
	)

	color := make(map[string]int)
	parent := make(map[string]string)

	var dfs func(node string) error
	dfs = func(node string) error {
		color[node] = gray
		for _, targets := range g.Adjacency[node] {
			for _, target := range targets {
				if color[target] == gray {
					// Build cycle path
					cycle := []string{target, node}
					cur := node
					for cur != target {
						cur = parent[cur]
						cycle = append(cycle, cur)
					}
					// Reverse for readability
					for i, j := 0, len(cycle)-1; i < j; i, j = i+1, j-1 {
						cycle[i], cycle[j] = cycle[j], cycle[i]
					}
					return fmt.Errorf("cycle detected: %s", strings.Join(cycle, " → "))
				}
				if color[target] == white {
					parent[target] = node
					if err := dfs(target); err != nil {
						return err
					}
				}
			}
		}
		color[node] = black
		return nil
	}

	for id := range g.Nodes {
		if color[id] == white {
			if err := dfs(id); err != nil {
				return err
			}
		}
	}
	return nil
}

// computeJoinTypes determines AND-join vs OR-join for each node.
// This runs at compile time and the result is cached, so the O(n^2) worst case
// from hasCommonConditionalAncestor is acceptable for typical workflow sizes.
func computeJoinTypes(g *CompiledGraph) {
	for id := range g.Nodes {
		inbound := g.Reverse[id]
		if len(inbound) <= 1 {
			g.JoinTypes[id] = JoinNone
			continue
		}

		// Check if all inbound edges come from different outputs of the same node
		// (conditional split → OR-join)
		if allFromSameNode(inbound) {
			g.JoinTypes[id] = JoinOR
		} else {
			// Check if inbound edges trace back to a common conditional ancestor
			if hasCommonConditionalAncestor(g, id, inbound) {
				g.JoinTypes[id] = JoinOR
			} else {
				g.JoinTypes[id] = JoinAND
			}
		}
	}
}

// allFromSameNode checks if all source nodes are the same node.
func allFromSameNode(sources []string) bool {
	if len(sources) == 0 {
		return false
	}
	first := sources[0]
	for _, s := range sources[1:] {
		if s != first {
			return false
		}
	}
	return true
}

// hasCommonConditionalAncestor traces inbound edges back to find if they share
// a common conditional ancestor (meaning they're mutually exclusive branches).
func hasCommonConditionalAncestor(g *CompiledGraph, nodeID string, inbound []string) bool {
	// For each inbound source, trace ancestors
	ancestorSets := make([]map[string]bool, len(inbound))
	for i, src := range inbound {
		ancestorSets[i] = traceAncestors(g, src)
		ancestorSets[i][src] = true
	}

	// Find common ancestors
	common := make(map[string]bool)
	for ancestor := range ancestorSets[0] {
		isCommon := true
		for _, set := range ancestorSets[1:] {
			if !set[ancestor] {
				isCommon = false
				break
			}
		}
		if isCommon {
			common[ancestor] = true
		}
	}

	// Check if any common ancestor is a conditional (has multiple distinct output edges)
	for ancestor := range common {
		outputs := g.Adjacency[ancestor]
		if len(outputs) > 1 {
			// Check if the inbound nodes are reached through different outputs
			reachedThrough := make(map[string]string) // inbound src → output name
			for _, src := range inbound {
				for outputName, targets := range outputs {
					if reachableFrom(g, targets, src) {
						reachedThrough[src] = outputName
						break
					}
				}
			}
			// If inbound nodes come through different outputs, it's OR-join
			outputsSeen := make(map[string]bool)
			for _, output := range reachedThrough {
				outputsSeen[output] = true
			}
			if len(outputsSeen) > 1 {
				return true
			}
		}
	}

	return false
}

// traceAncestors returns all ancestors of a node via BFS.
func traceAncestors(g *CompiledGraph, node string) map[string]bool {
	visited := make(map[string]bool)
	queue := []string{node}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		for _, parent := range g.Reverse[cur] {
			if !visited[parent] {
				visited[parent] = true
				queue = append(queue, parent)
			}
		}
	}
	return visited
}

// reachableFrom checks if target is reachable from any of the start nodes.
func reachableFrom(g *CompiledGraph, starts []string, target string) bool {
	visited := make(map[string]bool)
	queue := make([]string, len(starts))
	copy(queue, starts)
	for _, s := range starts {
		visited[s] = true
	}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		if cur == target {
			return true
		}
		for _, targets := range g.Adjacency[cur] {
			for _, t := range targets {
				if !visited[t] {
					visited[t] = true
					queue = append(queue, t)
				}
			}
		}
	}
	return false
}

// GetEdge returns the compiled edge for a given from:output:to combination.
func (g *CompiledGraph) GetEdge(from, output, to string) (*CompiledEdge, bool) {
	key := fmt.Sprintf("%s:%s:%s", from, output, to)
	e, ok := g.Edges[key]
	return e, ok
}

// extractConfigRefs extracts all identifiers referenced in {{ }} expressions
// within a config map. Pre-computed at compile time so the eviction tracker
// doesn't need to re-parse expressions at runtime.
func extractConfigRefs(config map[string]any) map[string]bool {
	refs := make(map[string]bool)
	walkConfigStrings(config, func(s string) {
		parsed, err := expr.Parse(s)
		if err != nil || parsed.IsLiteral {
			return
		}
		for _, seg := range parsed.Segments {
			if seg.Type != expr.SegmentExpression {
				continue
			}
			// Extract identifiers from the expression
			for _, ident := range extractIdentifiers(seg.Value) {
				refs[ident] = true
			}
		}
	})
	return refs
}

// walkConfigStrings recursively visits all string values in a config map.
func walkConfigStrings(config map[string]any, fn func(string)) {
	for _, v := range config {
		switch val := v.(type) {
		case string:
			fn(val)
		case map[string]any:
			walkConfigStrings(val, fn)
		case []any:
			for _, item := range val {
				if s, ok := item.(string); ok {
					fn(s)
				}
				if m, ok := item.(map[string]any); ok {
					walkConfigStrings(m, fn)
				}
			}
		}
	}
}

// extractIdentifiers returns node output identifiers referenced in an expression.
// It looks for "nodes.X" patterns and returns X (the node ID or alias).
// Also returns top-level identifiers for backward compatibility with non-node refs.
func extractIdentifiers(expression string) []string {
	var idents []string
	i := 0
	for i < len(expression) {
		if isIdentChar(expression[i]) {
			start := i
			for i < len(expression) && (isIdentChar(expression[i]) || expression[i] == '.') {
				i++
			}
			ident := expression[start:i]
			// Check for "nodes.X" pattern — extract X as the referenced output
			if strings.HasPrefix(ident, "nodes.") {
				parts := strings.SplitN(ident, ".", 3)
				if len(parts) >= 2 {
					idents = append(idents, parts[1])
				}
			} else {
				// Take only the root identifier (before any dot)
				if dot := strings.IndexByte(ident, '.'); dot != -1 {
					ident = ident[:dot]
				}
				idents = append(idents, ident)
			}
		} else {
			i++
		}
	}
	return idents
}

// isIdentChar returns true if c is valid in a node ID (alphanumeric, underscore, hyphen).
func isIdentChar(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_' || c == '-'
}

func containsString(slice []string, s string) bool {
	for _, v := range slice {
		if v == s {
			return true
		}
	}
	return false
}
