package engine

import (
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/chimpanze/noda/internal/expr"
)

// JoinType indicates how a node handles multiple inbound edges.
type JoinType int

const (
	JoinNone  JoinType = iota // single inbound edge or entry node
	JoinAND                   // wait for ALL inbound edges (parallel branches)
	JoinOR                    // wait for whichever fires (conditional branches)
	JoinMixed                 // both: some legs are mutually exclusive, others concurrent
)

func (j JoinType) String() string {
	switch j {
	case JoinNone:
		return "none"
	case JoinAND:
		return "AND-join"
	case JoinOR:
		return "OR-join"
	case JoinMixed:
		return "mixed join"
	default:
		return "unknown"
	}
}

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
	ID      string
	Timeout string `json:"timeout,omitempty"`
	Nodes   map[string]NodeConfig
	Edges   []EdgeConfig
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
	Timeout    time.Duration
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

	// Join type per node. Derived from JoinGroups; kept as a readable summary
	// for diagnostics and error messages.
	JoinTypes map[string]JoinType

	// JoinGroups partitions each node's inbound source nodes into
	// mutually-exclusive groups: nodeID → sourceNodeID → group index.
	// Sources in the same group descend from different outputs of a common
	// conditional ancestor, so exactly one of them delivers an arrival.
	// Sources in different groups run concurrently and all deliver.
	JoinGroups map[string]map[string]int

	// JoinGroupCount is the number of distinct groups in JoinGroups[nodeID] —
	// i.e. how many arrivals the node must collect before it runs.
	JoinGroupCount map[string]int
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

		JoinGroups:     make(map[string]map[string]int),
		JoinGroupCount: make(map[string]int),
	}

	// Parse workflow timeout
	if wf.Timeout != "" {
		d, err := time.ParseDuration(wf.Timeout)
		if err != nil {
			return nil, fmt.Errorf("invalid workflow timeout %q: %w", wf.Timeout, err)
		}
		g.Timeout = d
	}

	// Compile nodes
	for id, nc := range wf.Nodes {
		if nc.Type == "" {
			return nil, fmt.Errorf("node %q has empty type", id)
		}
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

	// Identify terminal nodes (no non-error outbound edges).
	// A node is terminal if it has no outbound edges other than "error" edges.
	// This correctly handles nodes with outputs like "then"/"else", "done", etc.
	for id := range g.Nodes {
		hasNonErrorEdge := false
		for output, targets := range g.Adjacency[id] {
			if output != "error" && len(targets) > 0 {
				hasNonErrorEdge = true
				break
			}
		}
		if !hasNonErrorEdge {
			g.TerminalNodes = append(g.TerminalNodes, id)
		}
	}

	// Cycle detection
	if err := detectCycle(g); err != nil {
		return nil, err
	}

	// Compute join types
	computeJoinTypes(g)

	// Validate aliases
	if err := validateAliases(g); err != nil {
		return nil, err
	}

	// Validate output exclusivity (compile-time check)
	if err := ValidateOutputExclusivity(g); err != nil {
		return nil, err
	}

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

// validateAliases rejects an "as" alias that equals a node ID or duplicates
// another alias — both would silently overwrite outputs at runtime.
func validateAliases(g *CompiledGraph) error {
	seen := make(map[string]string) // alias → nodeID that declared it
	for id, n := range g.Nodes {
		if n.As == "" {
			continue
		}
		if _, isNodeID := g.Nodes[n.As]; isNodeID {
			return fmt.Errorf("workflow %q: node %q alias %q collides with an existing node ID", g.WorkflowID, id, n.As)
		}
		if prev, dup := seen[n.As]; dup {
			return fmt.Errorf("workflow %q: alias %q declared by both %q and %q", g.WorkflowID, n.As, prev, id)
		}
		seen[n.As] = id
	}
	return nil
}

// computeJoinTypes partitions every node's inbound legs into mutually-exclusive
// groups and records how many arrivals the node must collect before it runs.
// This generalizes the older AND/OR split: a parallel join is N groups of one
// leg, a conditional join is one group of N legs, and a join fed by both kinds
// at once (JoinMixed) falls out of the same computation rather than having to
// be forced into one bucket or the other.
//
// This runs at compile time and the result is cached, so the pairwise
// exclusivity check is acceptable for typical workflow sizes.
func computeJoinTypes(g *CompiledGraph) {
	for id := range g.Nodes {
		inbound := g.Reverse[id]
		if len(inbound) == 0 {
			g.JoinTypes[id] = JoinNone
			g.JoinGroupCount[id] = 0
			continue
		}

		// Distinct sources: two edges from the same node (e.g. a conditional
		// wiring both "then" and "else" straight to this node) are one leg,
		// because only one of them can ever fire.
		sources := slices.Clone(inbound)
		slices.Sort(sources)
		sources = slices.Compact(sources)

		// Union sources that are mutually exclusive. Each resulting class is
		// a group that delivers exactly one arrival.
		uf := newUnionFind(sources)
		for i := 0; i < len(sources); i++ {
			for k := i + 1; k < len(sources); k++ {
				if mutuallyExclusive(g, id, sources[i], sources[k]) {
					uf.union(sources[i], sources[k])
				}
			}
		}

		groups := make(map[string]int, len(sources))
		index := make(map[string]int)
		for _, src := range sources {
			root := uf.find(src)
			idx, ok := index[root]
			if !ok {
				idx = len(index)
				index[root] = idx
			}
			groups[src] = idx
		}

		g.JoinGroups[id] = groups
		g.JoinGroupCount[id] = len(index)

		switch {
		case len(sources) == 1:
			g.JoinTypes[id] = JoinNone
		case len(index) == 1:
			g.JoinTypes[id] = JoinOR
		case len(index) == len(sources):
			g.JoinTypes[id] = JoinAND
		default:
			g.JoinTypes[id] = JoinMixed
		}
	}
}

// unionFind is a tiny disjoint-set over node IDs.
type unionFind struct{ parent map[string]string }

func newUnionFind(ids []string) *unionFind {
	p := make(map[string]string, len(ids))
	for _, id := range ids {
		p[id] = id
	}
	return &unionFind{parent: p}
}

func (u *unionFind) find(x string) string {
	for u.parent[x] != x {
		u.parent[x] = u.parent[u.parent[x]] // path halving
		x = u.parent[x]
	}
	return x
}

func (u *unionFind) union(a, b string) {
	ra, rb := u.find(a), u.find(b)
	if ra != rb {
		u.parent[rb] = ra
	}
}

// mutuallyExclusive reports whether two inbound sources of joinID can never
// both deliver an arrival, because they descend from different outputs of a
// common conditional ancestor. A node fires exactly one of its outputs per
// execution, so legs under different outputs are exclusive.
func mutuallyExclusive(g *CompiledGraph, joinID, srcA, srcB string) bool {
	ancA := traceAncestors(g, srcA)
	ancA[srcA] = true
	ancB := traceAncestors(g, srcB)
	ancB[srcB] = true

	for ancestor := range ancA {
		if !ancB[ancestor] {
			continue
		}
		// Only a node with more than one distinct output can split control.
		if len(g.Adjacency[ancestor]) <= 1 {
			continue
		}
		outsA := descendedOutputs(g, ancestor, joinID, srcA)
		outsB := descendedOutputs(g, ancestor, joinID, srcB)
		if len(outsA) == 0 || len(outsB) == 0 {
			continue
		}
		// Disjoint output sets → the two legs are on different branches.
		// Overlapping sets mean at least one leg is reachable from an output
		// the other also uses, so we cannot prove exclusivity; stay conservative
		// and treat them as concurrent.
		disjoint := true
		for o := range outsA {
			if outsB[o] {
				disjoint = false
				break
			}
		}
		if disjoint {
			return true
		}
	}
	return false
}

// descendedOutputs returns the set of ancestor outputs that src's leg into
// joinID descends from.
func descendedOutputs(g *CompiledGraph, ancestor, joinID, src string) map[string]bool {
	outputs := g.Adjacency[ancestor]
	names := make([]string, 0, len(outputs))
	for n := range outputs {
		names = append(names, n)
	}
	sort.Strings(names)

	result := make(map[string]bool)
	for _, outputName := range names {
		// A leg that is a direct edge from the conditional to the join
		// descends from the output that edge carries. It is not reachable
		// from that output (a node is not reachable from itself), so the
		// reachability scan below would miss it and collapse the diamond
		// into a join that never fires (#433).
		if src == ancestor {
			if slices.Contains(outputs[outputName], joinID) {
				result[outputName] = true
			}
			continue
		}
		if reachableFrom(g, outputs[outputName], src) {
			result[outputName] = true
		}
	}
	return result
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
	return slices.Contains(slice, s)
}
