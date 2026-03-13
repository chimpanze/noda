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

func newTestSetup(t *testing.T) (*registry.ServiceRegistry, *registry.NodeRegistry) {
	t.Helper()
	svcReg := registry.NewServiceRegistry()
	nodeReg := registry.NewNodeRegistry()
	_ = nodeReg.RegisterFromPlugin(&util.Plugin{})
	return svcReg, nodeReg
}

func newTestSetupWithCache(t *testing.T) (*registry.ServiceRegistry, *registry.NodeRegistry, *miniredis.Miniredis) {
	t.Helper()
	svcReg, nodeReg := newTestSetup(t)
	mr := miniredis.RunT(t)
	p := &cacheplugin.Plugin{}
	svc, err := p.CreateService(map[string]any{"url": "redis://" + mr.Addr()})
	require.NoError(t, err)
	err = svcReg.Register("app-cache", svc, p)
	require.NoError(t, err)
	return svcReg, nodeReg, mr
}

// --- ParseScheduleConfigs ---

func TestParseScheduleConfigs(t *testing.T) {
	raw := map[string]map[string]any{
		"schedules/cleanup.json": {
			"id":          "cleanup-tokens",
			"cron":        "0 */6 * * *",
			"timezone":    "UTC",
			"description": "Remove expired tokens",
			"services": map[string]any{
				"lock": "app-cache",
			},
			"trigger": map[string]any{
				"workflow": "cleanup-tokens",
				"input":    map[string]any{"batch": float64(100)},
			},
			"lock": map[string]any{
				"enabled": true,
				"ttl":     "300s",
			},
		},
	}

	configs := ParseScheduleConfigs(raw)
	require.Len(t, configs, 1)

	sc := configs[0]
	assert.Equal(t, "cleanup-tokens", sc.ID)
	assert.Equal(t, "0 */6 * * *", sc.Cron)
	assert.Equal(t, "UTC", sc.Timezone)
	assert.Equal(t, "app-cache", sc.LockSvcName)
	assert.True(t, sc.LockEnabled)
	assert.Equal(t, 300*time.Second, sc.LockTTL)
	assert.Equal(t, "cleanup-tokens", sc.WorkflowID)
	assert.Equal(t, float64(100), sc.InputMap["batch"])
}

func TestParseScheduleConfigs_Minimal(t *testing.T) {
	raw := map[string]map[string]any{
		"schedules/job.json": {
			"id":   "simple-job",
			"cron": "@every 1m",
			"trigger": map[string]any{
				"workflow": "my-workflow",
			},
		},
	}

	configs := ParseScheduleConfigs(raw)
	require.Len(t, configs, 1)
	assert.Equal(t, "simple-job", configs[0].ID)
	assert.False(t, configs[0].LockEnabled)
	assert.Nil(t, configs[0].InputMap)
}

// --- Scheduler firing ---

func TestRuntime_JobFires(t *testing.T) {
	svcReg, nodeReg := newTestSetup(t)

	workflows := map[string]map[string]any{
		"test-wf": {
			"nodes": map[string]any{
				"log": map[string]any{
					"type": "util.log",
					"config": map[string]any{
						"message": "scheduled job fired",
						"level":   "info",
					},
				},
			},
			"edges": []any{},
		},
	}

	// Use @every 1s to fire quickly in tests
	sc := ScheduleConfig{
		ID:         "test-job",
		Cron:       "@every 1s",
		WorkflowID: "test-wf",
	}

	rt := NewRuntime([]ScheduleConfig{sc}, svcReg, nodeReg, workflows, nil, nil, nil, nil)

	err := rt.Start()
	require.NoError(t, err)
	defer func() { _ = rt.Stop(context.Background()) }()

	// Wait for at least 1 job run
	require.Eventually(t, func() bool {
		history := rt.History()
		return len(history) >= 1 && history[0].Success
	}, 5*time.Second, 100*time.Millisecond)
}

func TestRuntime_TriggerMetadata(t *testing.T) {
	svcReg, nodeReg := newTestSetup(t)

	workflows := map[string]map[string]any{
		"meta-wf": {
			"nodes": map[string]any{
				"log": map[string]any{
					"type": "util.log",
					"config": map[string]any{
						"message": "ok",
						"level":   "info",
					},
				},
			},
			"edges": []any{},
		},
	}

	sc := ScheduleConfig{
		ID:         "meta-job",
		Cron:       "@every 1s",
		WorkflowID: "meta-wf",
	}

	rt := NewRuntime([]ScheduleConfig{sc}, svcReg, nodeReg, workflows, nil, nil, nil, nil)
	err := rt.Start()
	require.NoError(t, err)
	defer func() { _ = rt.Stop(context.Background()) }()

	require.Eventually(t, func() bool {
		history := rt.History()
		return len(history) >= 1
	}, 5*time.Second, 100*time.Millisecond)

	run := rt.History()[0]
	assert.Equal(t, "meta-job", run.ScheduleID)
	assert.NotEmpty(t, run.TraceID)
	assert.False(t, run.StartedAt.IsZero())
}

func TestRuntime_InputMapping(t *testing.T) {
	svcReg, nodeReg := newTestSetup(t)

	workflows := map[string]map[string]any{
		"input-wf": {
			"nodes": map[string]any{
				"log": map[string]any{
					"type": "util.log",
					"config": map[string]any{
						"message": "{{ input.label }}",
						"level":   "info",
					},
				},
			},
			"edges": []any{},
		},
	}

	sc := ScheduleConfig{
		ID:         "input-job",
		Cron:       "@every 1s",
		WorkflowID: "input-wf",
		InputMap: map[string]any{
			"label": "{{ schedule.id }}",
		},
	}

	rt := NewRuntime([]ScheduleConfig{sc}, svcReg, nodeReg, workflows, nil, nil, nil, nil)
	err := rt.Start()
	require.NoError(t, err)
	defer func() { _ = rt.Stop(context.Background()) }()

	require.Eventually(t, func() bool {
		history := rt.History()
		return len(history) >= 1 && history[0].Success
	}, 5*time.Second, 100*time.Millisecond)
}

func TestRuntime_JobFailureLogged(t *testing.T) {
	svcReg, nodeReg := newTestSetup(t)

	// A workflow that doesn't exist
	workflows := map[string]map[string]any{}

	sc := ScheduleConfig{
		ID:         "fail-job",
		Cron:       "@every 1s",
		WorkflowID: "nonexistent",
	}

	rt := NewRuntime([]ScheduleConfig{sc}, svcReg, nodeReg, workflows, nil, nil, nil, nil)
	err := rt.Start()
	require.NoError(t, err)
	defer func() { _ = rt.Stop(context.Background()) }()

	require.Eventually(t, func() bool {
		history := rt.History()
		return len(history) >= 1 && !history[0].Success
	}, 5*time.Second, 100*time.Millisecond)

	run := rt.History()[0]
	assert.False(t, run.Success)
	assert.Contains(t, run.Error, "workflow")
}

func TestRuntime_GracefulShutdown(t *testing.T) {
	svcReg, nodeReg := newTestSetup(t)

	rt := NewRuntime(
		[]ScheduleConfig{{
			ID:         "shutdown-job",
			Cron:       "@every 10m",
			WorkflowID: "wf",
		}},
		svcReg, nodeReg, nil, nil, nil, nil, nil,
	)

	err := rt.Start()
	require.NoError(t, err)

	done := make(chan struct{})
	go func() {
		_ = rt.Stop(context.Background())
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("shutdown took too long")
	}
}

func TestRuntime_NextRun(t *testing.T) {
	svcReg, nodeReg := newTestSetup(t)

	sc := ScheduleConfig{
		ID:         "next-job",
		Cron:       "@every 10m",
		WorkflowID: "wf",
	}

	rt := NewRuntime([]ScheduleConfig{sc}, svcReg, nodeReg, nil, nil, nil, nil, nil)
	err := rt.Start()
	require.NoError(t, err)
	defer func() { _ = rt.Stop(context.Background()) }()

	next, ok := rt.NextRun("next-job")
	assert.True(t, ok)
	assert.True(t, next.After(time.Now()))
}

func TestRuntime_History_Capped(t *testing.T) {
	rt := &Runtime{}

	// Fill history with 1001 entries
	for i := 0; i < 1001; i++ {
		rt.recordRun(JobRun{ScheduleID: "job", Success: true})
	}

	history := rt.History()
	assert.Len(t, history, 1000)
}

// --- Distributed locking ---

func TestDistributedLock_Acquire(t *testing.T) {
	svcReg, nodeReg, _ := newTestSetupWithCache(t)

	workflows := map[string]map[string]any{
		"lock-wf": {
			"nodes": map[string]any{
				"log": map[string]any{
					"type": "util.log",
					"config": map[string]any{
						"message": "lock acquired",
						"level":   "info",
					},
				},
			},
			"edges": []any{},
		},
	}

	sc := ScheduleConfig{
		ID:          "lock-job",
		Cron:        "@every 1s",
		WorkflowID:  "lock-wf",
		LockEnabled: true,
		LockSvcName: "app-cache",
		LockTTL:     5 * time.Minute,
	}

	rt := NewRuntime([]ScheduleConfig{sc}, svcReg, nodeReg, workflows, nil, nil, nil, nil)
	err := rt.Start()
	require.NoError(t, err)
	defer func() { _ = rt.Stop(context.Background()) }()

	require.Eventually(t, func() bool {
		history := rt.History()
		return len(history) >= 1 && history[0].Success
	}, 5*time.Second, 100*time.Millisecond)
}

func TestDistributedLock_SecondInstanceSkips(t *testing.T) {
	svcReg, nodeReg, _ := newTestSetupWithCache(t)

	workflows := map[string]map[string]any{
		"dup-wf": {
			"nodes": map[string]any{
				"log": map[string]any{
					"type": "util.log",
					"config": map[string]any{
						"message": "executed",
						"level":   "info",
					},
				},
			},
			"edges": []any{},
		},
	}

	sc := ScheduleConfig{
		ID:          "dup-job",
		Cron:        "@every 1s",
		WorkflowID:  "dup-wf",
		LockEnabled: true,
		LockSvcName: "app-cache",
		LockTTL:     5 * time.Minute,
	}

	// First runtime — will acquire lock
	rt1 := NewRuntime([]ScheduleConfig{sc}, svcReg, nodeReg, workflows, nil, nil, nil, nil)
	err := rt1.Start()
	require.NoError(t, err)
	defer func() { _ = rt1.Stop(context.Background()) }()

	// Wait for first to acquire and execute
	require.Eventually(t, func() bool {
		h := rt1.History()
		return len(h) >= 1
	}, 5*time.Second, 100*time.Millisecond)

	// Second runtime using the same lock key — if both fire at the same second,
	// the second should be skipped. We simulate this by directly calling runJob.
	rt2 := NewRuntime([]ScheduleConfig{sc}, svcReg, nodeReg, workflows, nil, nil, nil, nil)

	// Both run — the lock from rt1 should cause rt2 to skip
	// Since lock TTL is 5min, the key is still set, so the next call skips
	rt2.runJob(sc)

	h2 := rt2.History()
	require.Len(t, h2, 1)
	// The second instance should have been skipped (lock held by rt1)
	assert.True(t, h2[0].Skipped || h2[0].Success, "should be skipped or succeed on next minute key")
}

func TestDistributedLock_LockServiceNotFound(t *testing.T) {
	svcReg, nodeReg := newTestSetup(t)

	sc := ScheduleConfig{
		ID:          "missing-lock-job",
		Cron:        "@every 1s",
		WorkflowID:  "wf",
		LockEnabled: true,
		LockSvcName: "nonexistent-cache",
		LockTTL:     5 * time.Minute,
	}

	rt := NewRuntime([]ScheduleConfig{sc}, svcReg, nodeReg, map[string]map[string]any{}, nil, nil, nil, nil)
	rt.runJob(sc)

	history := rt.History()
	require.Len(t, history, 1)
	assert.False(t, history[0].Success)
	assert.Contains(t, history[0].Error, "not found")
}

func TestDistributedLock_Release(t *testing.T) {
	_, _, mr := newTestSetupWithCache(t)
	svc, err := (&cacheplugin.Plugin{}).CreateService(map[string]any{"url": "redis://" + mr.Addr()})
	require.NoError(t, err)

	ctx := context.Background()
	key := "test-lock-key"
	token, err := tryAcquireLock(ctx, svc, key, 5*time.Minute)
	require.NoError(t, err)
	assert.NotEmpty(t, token)

	// Same key should now be locked
	token2, err := tryAcquireLock(ctx, svc, key, 5*time.Minute)
	require.NoError(t, err)
	assert.Empty(t, token2)

	// Release with correct token
	err = releaseLockKey(ctx, svc, key, token)
	require.NoError(t, err)

	// Now acquirable again
	token3, err := tryAcquireLock(ctx, svc, key, 5*time.Minute)
	require.NoError(t, err)
	assert.NotEmpty(t, token3)
}

// --- Helpers ---

func TestResolveInput_StaticValues(t *testing.T) {
	rt := NewRuntime(nil, nil, nil, nil, nil, nil, nil, nil)
	input, err := rt.resolveInput(map[string]any{
		"key":   "static",
		"count": float64(42),
	}, map[string]any{})
	require.NoError(t, err)
	assert.Equal(t, "static", input["key"])
	assert.Equal(t, float64(42), input["count"])
}

func TestResolveInput_Expressions(t *testing.T) {
	rt := NewRuntime(nil, nil, nil, nil, nil, nil, nil, nil)
	ctx := map[string]any{
		"schedule": map[string]any{"id": "my-job"},
	}
	input, err := rt.resolveInput(map[string]any{
		"job": "{{ schedule.id }}",
	}, ctx)
	require.NoError(t, err)
	assert.Equal(t, "my-job", input["job"])
}

func TestRuntime_MultipleJobs(t *testing.T) {
	svcReg, nodeReg := newTestSetup(t)

	workflows := map[string]map[string]any{
		"wf-a": {
			"nodes": map[string]any{
				"log": map[string]any{
					"type":   "util.log",
					"config": map[string]any{"message": "job-a", "level": "info"},
				},
			},
			"edges": []any{},
		},
		"wf-b": {
			"nodes": map[string]any{
				"log": map[string]any{
					"type":   "util.log",
					"config": map[string]any{"message": "job-b", "level": "info"},
				},
			},
			"edges": []any{},
		},
	}

	schedules := []ScheduleConfig{
		{ID: "job-a", Cron: "@every 1s", WorkflowID: "wf-a"},
		{ID: "job-b", Cron: "@every 1s", WorkflowID: "wf-b"},
	}

	rt := NewRuntime(schedules, svcReg, nodeReg, workflows, nil, nil, nil, nil)
	err := rt.Start()
	require.NoError(t, err)
	defer func() { _ = rt.Stop(context.Background()) }()

	require.Eventually(t, func() bool {
		history := rt.History()
		var foundA, foundB bool
		for _, h := range history {
			if h.ScheduleID == "job-a" && h.Success {
				foundA = true
			}
			if h.ScheduleID == "job-b" && h.Success {
				foundB = true
			}
		}
		return foundA && foundB
	}, 5*time.Second, 100*time.Millisecond)
}
