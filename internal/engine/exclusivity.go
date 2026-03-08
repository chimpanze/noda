package engine

import "fmt"

// ValidateOutputExclusivity checks that all workflow.output nodes in a graph
// are on mutually exclusive branches (i.e., only one can fire per execution).
func ValidateOutputExclusivity(graph *CompiledGraph) error {
	// Find all workflow.output nodes
	var outputNodes []string
	outputNames := make(map[string]string) // name → nodeID

	for id, node := range graph.Nodes {
		if node.Type == "workflow.output" {
			name, _ := node.Config["name"].(string)
			if name == "" {
				continue
			}

			// Check for duplicate names
			if existingID, exists := outputNames[name]; exists {
				return fmt.Errorf("duplicate workflow.output name %q: nodes %q and %q", name, existingID, id)
			}
			outputNames[name] = id
			outputNodes = append(outputNodes, id)
		}
	}

	if len(outputNodes) <= 1 {
		return nil // single or no output is always valid
	}

	// Check each pair of output nodes for mutual exclusivity
	for i := 0; i < len(outputNodes); i++ {
		for j := i + 1; j < len(outputNodes); j++ {
			if !areMutuallyExclusive(graph, outputNodes[i], outputNodes[j]) {
				nameI, _ := graph.Nodes[outputNodes[i]].Config["name"].(string)
				nameJ, _ := graph.Nodes[outputNodes[j]].Config["name"].(string)
				return fmt.Errorf("workflow.output nodes %q (%s) and %q (%s) are not mutually exclusive — both can fire in a single execution",
					outputNodes[i], nameI, outputNodes[j], nameJ)
			}
		}
	}

	return nil
}

// areMutuallyExclusive checks if two nodes are on mutually exclusive branches
// by finding their common conditional ancestor.
func areMutuallyExclusive(graph *CompiledGraph, nodeA, nodeB string) bool {
	// Get all ancestors for both nodes
	ancestorsA := traceAncestorsWithPaths(graph, nodeA)
	ancestorsB := traceAncestorsWithPaths(graph, nodeB)

	// Find common ancestors
	for ancestor := range ancestorsA {
		if _, ok := ancestorsB[ancestor]; !ok {
			continue
		}

		// Check if the ancestor is a conditional node (has multiple output types)
		outputs := graph.Adjacency[ancestor]
		if len(outputs) <= 1 {
			continue
		}

		// Check if A and B are reached through different outputs of this ancestor
		outputForA := findOutputLeadingTo(graph, ancestor, nodeA)
		outputForB := findOutputLeadingTo(graph, ancestor, nodeB)

		if outputForA != "" && outputForB != "" && outputForA != outputForB {
			return true
		}
	}

	return false
}

// traceAncestorsWithPaths returns all ancestors of a node.
func traceAncestorsWithPaths(graph *CompiledGraph, node string) map[string]bool {
	visited := make(map[string]bool)
	queue := []string{node}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		for _, parent := range graph.Reverse[cur] {
			if !visited[parent] {
				visited[parent] = true
				queue = append(queue, parent)
			}
		}
	}
	return visited
}

// findOutputLeadingTo finds which output of ancestor eventually leads to target.
func findOutputLeadingTo(graph *CompiledGraph, ancestor, target string) string {
	for outputName, targets := range graph.Adjacency[ancestor] {
		visited := make(map[string]bool)
		queue := make([]string, len(targets))
		copy(queue, targets)
		for _, t := range targets {
			visited[t] = true
		}

		for len(queue) > 0 {
			cur := queue[0]
			queue = queue[1:]
			if cur == target {
				return outputName
			}
			for _, nextTargets := range graph.Adjacency[cur] {
				for _, next := range nextTargets {
					if !visited[next] {
						visited[next] = true
						queue = append(queue, next)
					}
				}
			}
		}
	}
	return ""
}

