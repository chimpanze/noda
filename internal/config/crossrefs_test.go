package config

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func makeBaseRC() *RawConfig {
	return &RawConfig{
		Root: map[string]any{
			"services": map[string]any{
				"main-db":     map[string]any{"plugin": "postgres"},
				"app-cache":   map[string]any{"plugin": "cache"},
				"main-stream": map[string]any{"plugin": "stream"},
				"realtime":    map[string]any{"plugin": "pubsub"},
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

func TestFormatCycle(t *testing.T) {
	tests := []struct {
		name     string
		ids      []string
		expected string
	}{
		{
			name:     "nil slice",
			ids:      nil,
			expected: "",
		},
		{
			name:     "empty slice",
			ids:      []string{},
			expected: "",
		},
		{
			name:     "single element",
			ids:      []string{"A"},
			expected: "A",
		},
		{
			name:     "two elements A->B->A",
			ids:      []string{"A", "B", "A"},
			expected: "A → B → A",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatCycle(tt.ids)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestDetectWorkflowCycles(t *testing.T) {
	t.Run("no cycle", func(t *testing.T) {
		graph := map[string][]string{
			"A": {"B"},
			"B": {"C"},
		}
		errs := detectWorkflowCycles(graph)
		assert.Empty(t, errs)
	})

	t.Run("A->B->A cycle", func(t *testing.T) {
		graph := map[string][]string{
			"A": {"B"},
			"B": {"A"},
		}
		errs := detectWorkflowCycles(graph)
		require.Len(t, errs, 1)
		assert.Contains(t, errs[0].Message, "circular workflow reference")
		assert.Contains(t, errs[0].Message, "A")
		assert.Contains(t, errs[0].Message, "B")
	})

	t.Run("empty graph", func(t *testing.T) {
		errs := detectWorkflowCycles(map[string][]string{})
		assert.Empty(t, errs)
	})

	t.Run("self-referencing cycle", func(t *testing.T) {
		graph := map[string][]string{
			"A": {"A"},
		}
		errs := detectWorkflowCycles(graph)
		require.Len(t, errs, 1)
		assert.Contains(t, errs[0].Message, "circular workflow reference")
		assert.Contains(t, errs[0].Message, "A")
	})
}

func TestValidateCrossRefs_WorkerTimeout(t *testing.T) {
	rc := &RawConfig{
		Workers: map[string]map[string]any{
			"workers/bad.json": {"timeout": "notaduration"},
		},
	}
	errs := ValidateCrossRefs(rc)
	found := false
	for _, e := range errs {
		if strings.Contains(e.Message, "invalid duration") && e.JSONPath == "/timeout" {
			found = true
		}
	}
	assert.True(t, found, "expected invalid duration error for worker timeout")
}

func TestValidateCrossRefs_ConnectionEndpointDuration(t *testing.T) {
	rc := &RawConfig{
		Connections: map[string]map[string]any{
			"connections/ws.json": {
				"endpoints": map[string]any{
					"game": map[string]any{
						"ping_interval": "bad",
						"heartbeat":     "10s",
					},
				},
			},
		},
	}
	errs := ValidateCrossRefs(rc)
	found := false
	for _, e := range errs {
		if strings.Contains(e.Message, "invalid duration") && strings.Contains(e.JSONPath, "ping_interval") {
			found = true
		}
	}
	assert.True(t, found, "expected invalid duration error for ping_interval")
}
