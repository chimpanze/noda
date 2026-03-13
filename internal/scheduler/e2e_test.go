package scheduler

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/chimpanze/noda/internal/registry"
	cacheplugin "github.com/chimpanze/noda/plugins/cache"
	"github.com/chimpanze/noda/plugins/core/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// buildE2ESetup creates a registry with util nodes and an optional cache service.
func buildE2ESetup(t *testing.T, withCache bool) (*registry.ServiceRegistry, *registry.NodeRegistry) {
	t.Helper()
	svcReg := registry.NewServiceRegistry()
	nodeReg := registry.NewNodeRegistry()
	_ = nodeReg.RegisterFromPlugin(&util.Plugin{})

	if withCache {
		mr := miniredis.RunT(t)
		cp := &cacheplugin.Plugin{}
		rawSvc, err := cp.CreateService(map[string]any{"url": "redis://" + mr.Addr()})
		require.NoError(t, err)
		err = svcReg.Register("lock-cache", rawSvc, cp)
		require.NoError(t, err)
	}

	return svcReg, nodeReg
}

// TestE2E_ScheduledWorkflowExecutes verifies that a scheduled workflow fires and produces
// an observable side effect (recorded in history).
func TestE2E_ScheduledWorkflowExecutes(t *testing.T) {
	svcReg, nodeReg := buildE2ESetup(t, false)

	workflows := map[string]map[string]any{
		"side-effect-wf": {
			"nodes": map[string]any{
				"log": map[string]any{
					"type": "util.log",
					"config": map[string]any{
						"message": "e2e scheduled job ran",
						"level":   "info",
					},
				},
			},
			"edges": []any{},
		},
	}

	sc := ScheduleConfig{
		ID:         "e2e-job",
		Cron:       "@every 1s",
		WorkflowID: "side-effect-wf",
	}

	rt := NewRuntime([]ScheduleConfig{sc}, svcReg, nodeReg, workflows, nil, nil, nil, nil)
	require.NoError(t, rt.Start())
	defer rt.Stop(context.Background())

	// Wait for the job to fire and complete successfully
	require.Eventually(t, func() bool {
		history := rt.History()
		for _, h := range history {
			if h.ScheduleID == "e2e-job" && h.Success {
				return true
			}
		}
		return false
	}, 5*time.Second, 100*time.Millisecond)

	history := rt.History()
	require.NotEmpty(t, history)
	assert.Equal(t, "e2e-job", history[0].ScheduleID)
	assert.True(t, history[0].Success)
	assert.NotEmpty(t, history[0].TraceID)
	assert.False(t, history[0].Skipped)
}

// TestE2E_TwoInstances_OnlyOneExecutes simulates two scheduler instances sharing
// the same Redis lock service — only one should execute the job per tick.
func TestE2E_TwoInstances_OnlyOneExecutes(t *testing.T) {
	mr := miniredis.RunT(t)
	cp := &cacheplugin.Plugin{}

	// Instance A
	rawSvcA, err := cp.CreateService(map[string]any{"url": "redis://" + mr.Addr()})
	require.NoError(t, err)
	svcRegA := registry.NewServiceRegistry()
	require.NoError(t, svcRegA.Register("lock-cache", rawSvcA, cp))
	nodeRegA := registry.NewNodeRegistry()
	_ = nodeRegA.RegisterFromPlugin(&util.Plugin{})

	// Instance B — same Redis instance (simulates second process)
	rawSvcB, err := cp.CreateService(map[string]any{"url": "redis://" + mr.Addr()})
	require.NoError(t, err)
	svcRegB := registry.NewServiceRegistry()
	require.NoError(t, svcRegB.Register("lock-cache", rawSvcB, cp))
	nodeRegB := registry.NewNodeRegistry()
	_ = nodeRegB.RegisterFromPlugin(&util.Plugin{})

	workflows := map[string]map[string]any{
		"locked-wf": {
			"nodes": map[string]any{
				"log": map[string]any{
					"type": "util.log",
					"config": map[string]any{
						"message": "locked workflow ran",
						"level":   "info",
					},
				},
			},
			"edges": []any{},
		},
	}

	sc := ScheduleConfig{
		ID:          "locked-job",
		Cron:        "@every 1s",
		WorkflowID:  "locked-wf",
		LockEnabled: true,
		LockSvcName: "lock-cache",
		LockTTL:     30 * time.Second,
	}

	rtA := NewRuntime([]ScheduleConfig{sc}, svcRegA, nodeRegA, workflows, nil, nil, nil, nil)
	rtB := NewRuntime([]ScheduleConfig{sc}, svcRegB, nodeRegB, workflows, nil, nil, nil, nil)

	require.NoError(t, rtA.Start())
	defer rtA.Stop(context.Background())
	require.NoError(t, rtB.Start())
	defer rtB.Stop(context.Background())

	// Wait until at least one execution happened across both instances
	require.Eventually(t, func() bool {
		histA := rtA.History()
		histB := rtB.History()
		return len(histA)+len(histB) >= 1
	}, 5*time.Second, 100*time.Millisecond)

	// Allow a bit more time for a second tick
	time.Sleep(1500 * time.Millisecond)

	histA := rtA.History()
	histB := rtB.History()

	// Count executions (not skipped) across both instances
	executed := 0
	skipped := 0
	for _, h := range append(histA, histB...) {
		if h.Success {
			executed++
		}
		if h.Skipped {
			skipped++
		}
	}

	// At least one execution, and at least one skip (the other instance was blocked)
	assert.GreaterOrEqual(t, executed, 1, "at least one instance should have executed")
	assert.GreaterOrEqual(t, skipped, 1, "at least one instance should have been skipped by the lock")
}

// TestE2E_SchedulerGracefulShutdown verifies Stop() waits for in-flight jobs and returns.
func TestE2E_SchedulerGracefulShutdown(t *testing.T) {
	svcReg, nodeReg := buildE2ESetup(t, false)

	workflows := map[string]map[string]any{
		"shutdown-wf": {
			"nodes": map[string]any{
				"log": map[string]any{
					"type": "util.log",
					"config": map[string]any{
						"message": "shutdown test",
						"level":   "info",
					},
				},
			},
			"edges": []any{},
		},
	}

	sc := ScheduleConfig{
		ID:         "shutdown-job",
		Cron:       "@every 1s",
		WorkflowID: "shutdown-wf",
	}

	rt := NewRuntime([]ScheduleConfig{sc}, svcReg, nodeReg, workflows, nil, nil, nil, nil)
	require.NoError(t, rt.Start())

	// Let one job fire
	time.Sleep(1500 * time.Millisecond)

	done := make(chan struct{})
	go func() {
		rt.Stop(context.Background())
		close(done)
	}()

	select {
	case <-done:
		// OK
	case <-time.After(5 * time.Second):
		t.Fatal("Stop() did not return within 5 seconds")
	}
}
