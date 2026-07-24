package engine

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/chimpanze/noda/internal/registry"
	"github.com/chimpanze/noda/internal/trace"
	"github.com/chimpanze/noda/pkg/api"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// firstError records the first error seen across parallel node goroutines.
// It replaces a sync/atomic.Value, which panics when errors of different
// concrete types are stored (even on a losing CompareAndSwap).
type firstError struct {
	mu  sync.Mutex
	err error
}

func (f *firstError) set(err error) {
	f.mu.Lock()
	if f.err == nil {
		f.err = err
	}
	f.mu.Unlock()
}

func (f *firstError) get() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.err
}

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
	execCtx.EmitTrace(string(trace.EventWorkflowStarted), "", "", "", "", map[string]any{
		"input": execCtx.Input(),
		"auth":  execCtx.AuthMap(),
	})
	execCtx.Log("info", "workflow started", map[string]any{
		"trigger_type": execCtx.Trigger().Type,
	})

	// Track join arrivals per node, counted once per mutually-exclusive group.
	// A node runs when every one of its groups has delivered an arrival, which
	// covers all join shapes uniformly: a parallel join is N groups of one leg,
	// a conditional join is one group of N legs, and a mixed join is somewhere
	// between (e.g. an always-firing leg plus an either/or pair → 2 groups).
	//
	// CONCURRENCY SAFETY: Both maps (and the slices within groupSeen) are fully
	// populated here before any goroutine launches, then only the atomic values
	// inside are mutated concurrently. Keys are never added or removed after
	// this point, so no mutex is needed.
	groupSeen := make(map[string][]*atomic.Bool, len(graph.Nodes))
	arrived := make(map[string]*atomic.Int32, len(graph.Nodes))
	for id := range graph.Nodes {
		seen := make([]*atomic.Bool, graph.JoinGroupCount[id])
		for i := range seen {
			seen[i] = &atomic.Bool{}
		}
		groupSeen[id] = seen
		arrived[id] = &atomic.Int32{}
	}

	// Track output eviction for memory management
	evictionTracker := NewEvictionTracker(graph, execCtx)

	// Use context with cancel (or timeout) for error propagation
	var execCtx2 context.Context
	var cancel context.CancelFunc
	if graph.Timeout > 0 {
		execCtx2, cancel = context.WithTimeout(ctx, graph.Timeout)
	} else {
		execCtx2, cancel = context.WithCancel(ctx)
	}
	defer cancel()

	var (
		wg       sync.WaitGroup
		firstErr firstError
	)

	// dispatchIfReady launches a goroutine to execute a node.
	// CONCURRENCY SAFETY: wg.Go increments the counter synchronously in the
	// calling goroutine before spawning, so wg.Wait() cannot return
	// prematurely — including for the recursive dispatches below, which run
	// inside an already-counted goroutine.
	var dispatchIfReady func(nodeID string)
	dispatchIfReady = func(nodeID string) {
		node := graph.Nodes[nodeID]

		wg.Go(func() {
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
			nodeDuration := time.Since(nodeStart).Seconds()

			if err != nil {
				// Record node error metrics
				if m := execCtx.Metrics(); m != nil {
					nodeAttrs := metric.WithAttributes(
						attribute.String("node_type", node.Type),
					)
					m.NodeDuration.Record(execCtx2, nodeDuration, nodeAttrs)
					m.NodeErrors.Add(execCtx2, 1, nodeAttrs)
				}
				execCtx.Log("warn", "node failed", map[string]any{
					"node_id": node.ID,
					"error":   err.Error(),
				})
				firstErr.set(err)
				cancel()
				return
			}

			// Record node success metrics
			if m := execCtx.Metrics(); m != nil {
				status := "success"
				if output == "error" {
					status = "error"
				}
				nodeAttrs := metric.WithAttributes(
					attribute.String("node_type", node.Type),
					attribute.String("status", status),
				)
				m.NodeDuration.Record(execCtx2, nodeDuration, nodeAttrs)
				if output == "error" {
					m.NodeErrors.Add(execCtx2, 1, metric.WithAttributes(
						attribute.String("node_type", node.Type),
					))
				}
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
					var nodeErr error
					if orig := execCtx.NodeError(nodeID); orig != nil {
						// %w keeps errors.As working so MapErrorToHTTP can type it.
						nodeErr = fmt.Errorf("node %q failed with no error edge: %w", nodeID, orig)
					} else {
						nodeErr = fmt.Errorf("node %q failed with no error edge: %v", nodeID, errData)
					}
					execCtx.Log("warn", "node error with no error edge", map[string]any{
						"node_id": nodeID,
						"error":   nodeErr.Error(),
					})
					firstErr.set(nodeErr)
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
						firstErr.set(retryErr)
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
			evictionTracker.NodeCompleted(nodeID)

			// Follow outbound edges for the fired output
			targets := graph.Adjacency[nodeID][output]
			for _, targetID := range targets {
				execCtx.EmitTrace(string(trace.EventEdgeFollowed), "", "", output, "", map[string]any{
					"from": nodeID,
					"to":   targetID,
				})
				// Count this arrival against the group its source belongs to.
				// Repeat arrivals within a group (only possible via redundant
				// edges) collapse, so the counter reaches the group total
				// exactly once and the node dispatches exactly once.
				group := graph.JoinGroups[targetID][nodeID]
				if !groupSeen[targetID][group].CompareAndSwap(false, true) {
					continue
				}
				if int(arrived[targetID].Add(1)) == graph.JoinGroupCount[targetID] {
					dispatchIfReady(targetID)
				}
			}
		})
	}

	// Start all entry nodes
	for _, entryID := range graph.EntryNodes {
		dispatchIfReady(entryID)
	}

	wg.Wait()

	duration := time.Since(startTime)

	// Determine the workflow result: a recorded node error takes precedence;
	// otherwise a truncated execution (context expired) is itself a failure —
	// never report success for work that did not complete.
	resultErr := firstErr.get()
	if resultErr == nil && execCtx2.Err() != nil {
		if errors.Is(execCtx2.Err(), context.DeadlineExceeded) {
			// graph.Timeout == 0 means the deadline was inherited from the
			// parent (sub-workflow case): report the budget the child
			// actually had instead of "after 0s" (#273).
			budget := graph.Timeout
			if budget == 0 {
				if dl, ok := execCtx2.Deadline(); ok {
					budget = dl.Sub(startTime)
				}
			}
			resultErr = &api.TimeoutError{Duration: budget, Operation: "workflow " + graph.WorkflowID}
		} else {
			resultErr = fmt.Errorf("workflow %q aborted: %w", graph.WorkflowID, execCtx2.Err())
		}
	}

	if resultErr == nil {
		// Deterministic order: a graph can contain more than one starved join
		// and the reported one must not depend on map iteration order.
		ids := make([]string, 0, len(graph.JoinGroupCount))
		for id := range graph.JoinGroupCount {
			ids = append(ids, id)
		}
		sort.Strings(ids)

		for _, id := range ids {
			total := graph.JoinGroupCount[id]
			// A single-group node fires on its one arrival, so it can never be
			// starved: either it ran or its whole branch was unreached.
			if total <= 1 {
				continue
			}
			got := int(arrived[id].Load())
			// Received at least one group but never fired. got == 0 means the
			// node was simply never reached — a normal unreached branch, not
			// an error.
			if got > 0 && got < total {
				resultErr = fmt.Errorf("workflow %q incomplete: %s %q received %d of %d branches and never fired",
					graph.WorkflowID, graph.JoinTypes[id], id, got, total)
				break
			}
		}
	}

	if resultErr != nil {
		execCtx.Log("info", "workflow failed", map[string]any{
			"duration": duration.String(),
		})
		workflowErr = resultErr
		// Record workflow error metrics
		if m := execCtx.Metrics(); m != nil {
			wfAttrs := metric.WithAttributes(
				attribute.String("workflow_id", graph.WorkflowID),
			)
			m.WorkflowDuration.Record(ctx, duration.Seconds(), wfAttrs)
			m.WorkflowsTotal.Add(ctx, 1, wfAttrs)
			m.WorkflowErrors.Add(ctx, 1, wfAttrs)
		}
		execCtx.EmitTrace(string(trace.EventWorkflowFailed), "", "", "", workflowErr.Error(), nil)
		return workflowErr
	}

	// Record workflow success metrics
	if m := execCtx.Metrics(); m != nil {
		wfAttrs := metric.WithAttributes(
			attribute.String("workflow_id", graph.WorkflowID),
		)
		m.WorkflowDuration.Record(ctx, duration.Seconds(), wfAttrs)
		m.WorkflowsTotal.Add(ctx, 1, wfAttrs)
	}

	execCtx.EmitTrace(string(trace.EventWorkflowCompleted), "", "", "", "", map[string]any{"duration": duration.String()})
	execCtx.Log("info", "workflow completed", map[string]any{
		"status":   "success",
		"duration": duration.String(),
	})

	return nil
}
