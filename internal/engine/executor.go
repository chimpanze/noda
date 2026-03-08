package engine

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/chimpanze/noda/internal/registry"
)

// ExecuteGraph runs a compiled workflow graph to completion.
func ExecuteGraph(
	ctx context.Context,
	graph *CompiledGraph,
	execCtx *ExecutionContextImpl,
	services *registry.ServiceRegistry,
	nodes *registry.NodeRegistry,
) error {
	if len(graph.EntryNodes) == 0 {
		return fmt.Errorf("workflow %q has no entry nodes", graph.WorkflowID)
	}

	startTime := time.Now()
	execCtx.Log("info", "workflow started", map[string]any{
		"trigger_type": execCtx.Trigger().Type,
	})

	// Track pending dependency counts per node (atomic for thread safety)
	pending := make(map[string]*atomic.Int32)
	for id, count := range graph.DepCount {
		p := &atomic.Int32{}
		p.Store(int32(count))
		pending[id] = p
	}

	// For OR-join nodes, track whether they've already been dispatched
	dispatched := make(map[string]*atomic.Bool)
	for id := range graph.Nodes {
		dispatched[id] = &atomic.Bool{}
	}

	// Track terminal node completion
	terminalCount := int32(len(graph.TerminalNodes))
	terminalsCompleted := &atomic.Int32{}

	// Use context with cancel for error propagation
	execCtx2, cancel := context.WithCancel(ctx)
	defer cancel()

	var (
		wg       sync.WaitGroup
		firstErr atomic.Value
	)

	// dispatchIfReady checks if a node's dependencies are met and dispatches it.
	var dispatchIfReady func(nodeID string)
	dispatchIfReady = func(nodeID string) {
		node := graph.Nodes[nodeID]

		wg.Add(1)
		go func() {
			defer wg.Done()

			// Check context
			if execCtx2.Err() != nil {
				return
			}

			nodeStart := time.Now()
			execCtx.Log("debug", "node started", map[string]any{
				"node_id":   node.ID,
				"node_type": node.Type,
			})

			output, err := dispatchNode(execCtx2, node, execCtx, services, nodes)
			if err != nil {
				execCtx.Log("warn", "node failed", map[string]any{
					"node_id": node.ID,
					"error":   err.Error(),
				})
				firstErr.CompareAndSwap(nil, err)
				cancel()
				return
			}

			execCtx.Log("debug", "node completed", map[string]any{
				"node_id":  node.ID,
				"output":   output,
				"duration": time.Since(nodeStart).String(),
			})

			// Check for retry on error edges
			if output == "error" {
				targets := graph.Adjacency[nodeID]["error"]
				for _, target := range targets {
					edge, ok := graph.GetEdge(nodeID, "error", target)
					if ok && edge.Retry != nil {
						// Retry the node
						retryOutput, retryErr := retryNode(execCtx2, node, execCtx, services, nodes, edge.Retry)
						if retryErr != nil {
							firstErr.CompareAndSwap(nil, retryErr)
							cancel()
							return
						}
						if retryOutput != "error" {
							// Retry succeeded — use the success output instead
							output = retryOutput
							break
						}
					}
				}
			}

			// Follow outbound edges for the fired output
			targets := graph.Adjacency[nodeID][output]
			for _, targetID := range targets {
				targetNode := graph.Nodes[targetID]
				joinType := graph.JoinTypes[targetID]

				switch joinType {
				case JoinOR:
					// OR-join: dispatch on first arrival
					if dispatched[targetID].CompareAndSwap(false, true) {
						dispatchIfReady(targetID)
					}
				case JoinAND:
					// AND-join: decrement counter, dispatch when all arrive
					if pending[targetID].Add(-1) == 0 {
						dispatchIfReady(targetID)
					}
				default:
					// Single inbound edge
					_ = targetNode
					dispatchIfReady(targetID)
				}
			}

			// Check if this is a terminal node
			if isTerminal(graph, nodeID) {
				if terminalsCompleted.Add(1) >= terminalCount {
					// All terminals done — but we let WaitGroup handle completion
				}
			}
		}()
	}

	// Start all entry nodes
	for _, entryID := range graph.EntryNodes {
		dispatched[entryID].Store(true)
		dispatchIfReady(entryID)
	}

	wg.Wait()

	duration := time.Since(startTime)

	if errVal := firstErr.Load(); errVal != nil {
		execCtx.Log("info", "workflow failed", map[string]any{
			"duration": duration.String(),
		})
		return errVal.(error)
	}

	execCtx.Log("info", "workflow completed", map[string]any{
		"status":   "success",
		"duration": duration.String(),
	})

	return nil
}

func isTerminal(g *CompiledGraph, nodeID string) bool {
	for _, id := range g.TerminalNodes {
		if id == nodeID {
			return true
		}
	}
	return false
}
