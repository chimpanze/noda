package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func makeBaseRC() *RawConfig {
	return &RawConfig{
		Root: map[string]any{
			"services": map[string]any{
				"main-db":    map[string]any{"plugin": "postgres"},
				"app-cache":  map[string]any{"plugin": "cache"},
				"main-stream": map[string]any{"plugin": "stream"},
				"realtime":   map[string]any{"plugin": "pubsub"},
			},
		},
		Schemas: map[string]map[string]any{},
		Routes: map[string]map[string]any{
			"routes/tasks.json": {
				"id":     "list-tasks",
				"method": "GET",
				"path":   "/api/tasks",
				"trigger": map[string]any{
					"workflow": "list-tasks",
				},
			},
		},
		Workflows: map[string]map[string]any{
			"workflows/list-tasks.json": {
				"id":    "list-tasks",
				"nodes": map[string]any{},
				"edges": []any{},
			},
			"workflows/create-task.json": {
				"id":    "create-task",
				"nodes": map[string]any{},
				"edges": []any{},
			},
		},
		Workers: map[string]map[string]any{
			"workers/notifications.json": {
				"id": "notifications",
				"services": map[string]any{
					"stream": "main-stream",
				},
				"subscribe": map[string]any{"topic": "tasks", "group": "workers"},
				"trigger": map[string]any{
					"workflow": "create-task",
				},
			},
		},
		Schedules:   map[string]map[string]any{},
		Connections: map[string]map[string]any{},
		Tests:       map[string]map[string]any{},
	}
}

func TestCrossRefs_AllValid(t *testing.T) {
	rc := makeBaseRC()
	errs := ValidateCrossRefs(rc)
	assert.Empty(t, errs)
}

func TestCrossRefs_RouteNonExistentWorkflow(t *testing.T) {
	rc := makeBaseRC()
	rc.Routes["routes/tasks.json"]["trigger"] = map[string]any{
		"workflow": "non-existent",
	}

	errs := ValidateCrossRefs(rc)
	require.Len(t, errs, 1)
	assert.Contains(t, errs[0].Message, "non-existent")
	assert.Equal(t, "routes/tasks.json", errs[0].FilePath)
}

func TestCrossRefs_WorkerNonExistentStream(t *testing.T) {
	rc := makeBaseRC()
	rc.Workers["workers/notifications.json"]["services"] = map[string]any{
		"stream": "missing-stream",
	}

	errs := ValidateCrossRefs(rc)
	require.Len(t, errs, 1)
	assert.Contains(t, errs[0].Message, "missing-stream")
}

func TestCrossRefs_WorkerWrongServiceType(t *testing.T) {
	rc := makeBaseRC()
	rc.Workers["workers/notifications.json"]["services"] = map[string]any{
		"stream": "app-cache", // cache, not stream
	}

	errs := ValidateCrossRefs(rc)
	require.Len(t, errs, 1)
	assert.Contains(t, errs[0].Message, "cache")
	assert.Contains(t, errs[0].Message, "expected \"stream\"")
}

func TestCrossRefs_ScheduleNonExistentLockService(t *testing.T) {
	rc := makeBaseRC()
	rc.Schedules["schedules/cleanup.json"] = map[string]any{
		"id":   "cleanup",
		"cron": "0 * * * *",
		"services": map[string]any{
			"lock": "non-existent-cache",
		},
		"trigger": map[string]any{
			"workflow": "list-tasks",
		},
		"lock": map[string]any{
			"enabled": true,
		},
	}

	errs := ValidateCrossRefs(rc)
	require.Len(t, errs, 1)
	assert.Contains(t, errs[0].Message, "non-existent-cache")
}

func TestCrossRefs_ConnectionLifecycleNonExistentWorkflow(t *testing.T) {
	rc := makeBaseRC()
	rc.Connections["connections/realtime.json"] = map[string]any{
		"sync": map[string]any{
			"pubsub": "realtime",
		},
		"endpoints": map[string]any{
			"chat": map[string]any{
				"type":          "websocket",
				"path":          "/ws/chat",
				"on_connect":    "non-existent-connect",
				"on_disconnect": "list-tasks", // valid
			},
		},
	}

	errs := ValidateCrossRefs(rc)
	require.Len(t, errs, 1)
	assert.Contains(t, errs[0].Message, "non-existent-connect")
}

func TestCrossRefs_WorkflowRunNonExistentWorkflow(t *testing.T) {
	rc := makeBaseRC()
	rc.Workflows["workflows/create-task.json"]["nodes"] = map[string]any{
		"run-sub": map[string]any{
			"type": "workflow.run",
			"config": map[string]any{
				"workflow": "non-existent-sub",
			},
		},
	}

	errs := ValidateCrossRefs(rc)
	require.Len(t, errs, 1)
	assert.Contains(t, errs[0].Message, "non-existent-sub")
}

func TestCrossRefs_MultipleErrors(t *testing.T) {
	rc := makeBaseRC()
	rc.Routes["routes/tasks.json"]["trigger"] = map[string]any{
		"workflow": "missing1",
	}
	rc.Workers["workers/notifications.json"]["trigger"] = map[string]any{
		"workflow": "missing2",
	}

	errs := ValidateCrossRefs(rc)
	assert.Len(t, errs, 2)
}
