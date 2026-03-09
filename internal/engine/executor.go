package engine

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/chimpanze/noda/internal/registry"
	"github.com/chimpanze/noda/internal/trace"
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

	// Start OTel workflow span
	var workflowErr error
	ctx, workflowSpan := trace.StartWorkflowSpan(ctx, execCtx.Tracer(), graph.WorkflowID, execCtx.Trigger().TraceID, execCtx.Trigger().Type)
	defer func() { trace.EndWorkflowSpan(workflowSpan, workflowErr) }()

	startTime := time.Now()
	execCtx.EmitTrace("workflow:started", "", "", "", "", nil)
	execCtx.Log("info", "workflow started", map[string]any{
		"trigger_type": execCtx.Trigger().Type,
	})

	// Track pending dependency counts per node.
	// CONCURRENCY SAFETY: The map structure is populated here before any goroutines
	// launch, then only the atomic values within are modified concurrently. The map
	// keys are never added or removed after this point, so no mutex is needed.
	pending := make(map[string]*atomic.Int32)
	for id, count := range graph.DepCount {
		p := &atomic.Int32{}
		p.Store(int32(count))
		pending[id] = p
	}

	// For OR-join nodes, track whether they've already been dispatched.
	// Same concurrency invariant as pending: map is read-only after init.
	dispatched := make(map[string]*atomic.Bool)
	for id := range graph.Nodes {
		dispatched[id] = &atomic.Bool{}
	}

	// Track output eviction for memory management
	evictionTracker := NewEvictionTracker(graph, execCtx)

	// Use context with cancel for error propagation
	execCtx2, cancel := context.WithCancel(ctx)
	defer cancel()

	var (
		wg       sync.WaitGroup
		firstErr atomic.Value
	)

	// dispatchIfReady launches a goroutine to execute a node.
	// CONCURRENCY SAFETY: wg.Add(1) is called synchronously before the goroutine
	// is spawned, so wg.Wait() cannot return prematurely.
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

			// If a node produced an error output but no error edges exist,
			// the workflow must fail — silent swallowing is a bug.
			if output == "error" {
				errorTargets := graph.Adjacency[nodeID]["error"]
				if len(errorTargets) == 0 {
					errData, _ := execCtx.GetOutput(nodeID)
					nodeErr := fmt.Errorf("node %q failed with no error edge: %v", nodeID, errData)
					execCtx.Log("warn", "node error with no error edge", map[string]any{
						"node_id": nodeID,
					})
					firstErr.CompareAndSwap(nil, nodeErr)
					cancel()
					return
				}
			}

			// Per-edge retry on error output: each error edge can specify its own
			// retry config. We try each edge's retry policy in order. If any retry
			// succeeds, the node is considered successful and we follow the success
			// output instead. If all retries fail, we follow the error edges.
			if output == "error" {
				errorTargets := graph.Adjacency[nodeID]["error"]
				for _, target := range errorTargets {
					edge, ok := graph.GetEdge(nodeID, "error", target)
					if !ok || edge.Retry == nil {
						continue
					}
					retryOutput, retryErr := retryNode(execCtx2, node, execCtx, services, nodes, edge.Retry)
					if retryErr != nil {
						firstErr.CompareAndSwap(nil, retryErr)
						cancel()
						return
					}
					if retryOutput != "error" {
						output = retryOutput
						break
					}
				}
			}

			// Evict upstream outputs that are no longer needed
			evictionTracker.NodeCompleted(nodeID, graph)

			// Follow outbound edges for the fired output
			targets := graph.Adjacency[nodeID][output]
			for _, targetID := range targets {
				execCtx.EmitTrace("edge:followed", "", "", output, "", map[string]any{
					"from": nodeID,
					"to":   targetID,
				})
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
					dispatchIfReady(targetID)
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
		workflowErr = errVal.(error)
		execCtx.EmitTrace("workflow:failed", "", "", "", workflowErr.Error(), nil)
		return workflowErr
	}

	execCtx.EmitTrace("workflow:completed", "", "", "", "", map[string]any{"duration": duration.String()})
	execCtx.Log("info", "workflow completed", map[string]any{
		"status":   "success",
		"duration": duration.String(),
	})

	return nil
}
