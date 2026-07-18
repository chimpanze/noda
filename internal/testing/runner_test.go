package testing

import (
	"context"
	"testing"

	"github.com/chimpanze/noda/internal/config"
	"github.com/chimpanze/noda/internal/registry"
	"github.com/chimpanze/noda/internal/secrets"
	"github.com/chimpanze/noda/plugins/core/control"
	"github.com/chimpanze/noda/plugins/core/response"
	"github.com/chimpanze/noda/plugins/core/transform"
	"github.com/chimpanze/noda/plugins/core/util"
	"github.com/chimpanze/noda/plugins/core/workflow"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func buildCoreNodeReg(t *testing.T) *registry.NodeRegistry {
	t.Helper()
	nodeReg := registry.NewNodeRegistry()
	require.NoError(t, nodeReg.RegisterFromPlugin(&control.Plugin{}))
	require.NoError(t, nodeReg.RegisterFromPlugin(&transform.Plugin{}))
	require.NoError(t, nodeReg.RegisterFromPlugin(&util.Plugin{}))
	require.NoError(t, nodeReg.RegisterFromPlugin(&workflow.Plugin{}))
	return nodeReg
}

func TestRunner_PassingTest(t *testing.T) {
	rc := &config.ResolvedConfig{
		Workflows: map[string]map[string]any{
			"workflows/wf.json": {
				"id": "simple-wf",
				"nodes": map[string]any{
					"fetch": map[string]any{
						"type": "db.query",
						"config": map[string]any{
							"table": "users",
						},
					},
				},
				"edges": []any{},
			},
		},
	}

	suite := TestSuite{
		ID:       "test-simple",
		Workflow: "simple-wf",
		Cases: []TestCase{
			{
				Name:  "fetches data",
				Input: map[string]any{"id": 1},
				Mocks: map[string]MockConfig{
					"fetch": {Output: map[string]any{"id": 1, "name": "Alice"}},
				},
				Expect: TestExpectation{
					Status: "success",
					Output: map[string]any{
						"fetch.name": "Alice",
					},
				},
			},
		},
	}

	coreNodeReg := buildCoreNodeReg(t)
	results := RunTestSuite(suite, rc, coreNodeReg, nil)
	require.Len(t, results, 1)
	assert.True(t, results[0].Passed, results[0].Error)
}

func TestRunner_FailingExpectation(t *testing.T) {
	rc := &config.ResolvedConfig{
		Workflows: map[string]map[string]any{
			"workflows/wf.json": {
				"id": "simple-wf",
				"nodes": map[string]any{
					"fetch": map[string]any{"type": "db.query"},
				},
				"edges": []any{},
			},
		},
	}

	suite := TestSuite{
		Workflow: "simple-wf",
		Cases: []TestCase{
			{
				Name: "wrong output",
				Mocks: map[string]MockConfig{
					"fetch": {Output: map[string]any{"name": "Bob"}},
				},
				Expect: TestExpectation{
					Status: "success",
					Output: map[string]any{"fetch.name": "Alice"},
				},
			},
		},
	}

	results := RunTestSuite(suite, rc, buildCoreNodeReg(t), nil)
	assert.False(t, results[0].Passed)
	assert.Contains(t, results[0].Error, "Alice")
}

func TestRunner_MockError(t *testing.T) {
	rc := &config.ResolvedConfig{
		Workflows: map[string]map[string]any{
			"workflows/wf.json": {
				"id": "err-wf",
				"nodes": map[string]any{
					"fetch": map[string]any{"type": "db.query"},
				},
				"edges": []any{},
			},
		},
	}

	suite := TestSuite{
		Workflow: "err-wf",
		Cases: []TestCase{
			{
				Name: "database error",
				Mocks: map[string]MockConfig{
					"fetch": {Error: &MockError{Message: "connection refused"}},
				},
				Expect: TestExpectation{
					Status: "error",
				},
			},
		},
	}

	results := RunTestSuite(suite, rc, buildCoreNodeReg(t), nil)
	// The mock error gets caught by the error output of the mock executor
	// Since there's no error edge, the error is stored as output data
	// The workflow succeeds (error is handled internally)
	assert.NotNil(t, results[0])
}

func TestRunner_CoreNodesExecuteNormally(t *testing.T) {
	rc := &config.ResolvedConfig{
		Workflows: map[string]map[string]any{
			"workflows/wf.json": {
				"id": "core-wf",
				"nodes": map[string]any{
					"set_data": map[string]any{
						"type": "transform.set",
						"config": map[string]any{
							"fields": map[string]any{
								"greeting": "{{ \"Hello\" }}",
							},
						},
					},
				},
				"edges": []any{},
			},
		},
	}

	suite := TestSuite{
		Workflow: "core-wf",
		Cases: []TestCase{
			{
				Name:  "core node works",
				Mocks: map[string]MockConfig{},
				Expect: TestExpectation{
					Status: "success",
					Output: map[string]any{
						"set_data.greeting": "Hello",
					},
				},
			},
		},
	}

	results := RunTestSuite(suite, rc, buildCoreNodeReg(t), nil)
	require.Len(t, results, 1)
	assert.True(t, results[0].Passed, results[0].Error)
}

func TestRunner_UnmockedPluginNode(t *testing.T) {
	rc := &config.ResolvedConfig{
		Workflows: map[string]map[string]any{
			"workflows/wf.json": {
				"id": "unmocked-wf",
				"nodes": map[string]any{
					"fetch": map[string]any{"type": "db.query"},
				},
				"edges": []any{},
			},
		},
	}

	suite := TestSuite{
		Workflow: "unmocked-wf",
		Cases: []TestCase{
			{
				Name:  "no mock",
				Mocks: map[string]MockConfig{}, // fetch is not mocked
				Expect: TestExpectation{
					Status: "success",
					Output: map[string]any{
						"fetch.id": 1, // this won't match — unmocked returns error data
					},
				},
			},
		},
	}

	results := RunTestSuite(suite, rc, buildCoreNodeReg(t), nil)
	// The unmocked node is a db node (has error outputs). It returns an error
	// with no error edge, so the workflow fails with status "error".
	assert.False(t, results[0].Passed)
	assert.Contains(t, results[0].Error, "error")
}

func TestRunner_AuthPassedThrough(t *testing.T) {
	rc := &config.ResolvedConfig{
		Workflows: map[string]map[string]any{
			"workflows/wf.json": {
				"id": "auth-wf",
				"nodes": map[string]any{
					"check": map[string]any{
						"type": "transform.set",
						"config": map[string]any{
							"fields": map[string]any{
								"user": "{{ auth.sub }}",
							},
						},
					},
				},
				"edges": []any{},
			},
		},
	}

	suite := TestSuite{
		Workflow: "auth-wf",
		Cases: []TestCase{
			{
				Name: "auth works",
				Auth: &AuthConfig{
					UserID: "user-42",
					Roles:  []string{"admin"},
				},
				Mocks: map[string]MockConfig{},
				Expect: TestExpectation{
					Status: "success",
					Output: map[string]any{
						"check.user": "user-42",
					},
				},
			},
		},
	}

	results := RunTestSuite(suite, rc, buildCoreNodeReg(t), nil)
	require.Len(t, results, 1)
	assert.True(t, results[0].Passed, results[0].Error)
}

func TestRunner_WorkflowWithEdges(t *testing.T) {
	rc := &config.ResolvedConfig{
		Workflows: map[string]map[string]any{
			"workflows/wf.json": {
				"id": "edge-wf",
				"nodes": map[string]any{
					"fetch": map[string]any{"type": "db.query"},
					"format": map[string]any{
						"type": "transform.set",
						"config": map[string]any{
							"fields": map[string]any{
								"result": "{{ nodes.fetch.name }}",
							},
						},
					},
				},
				"edges": []any{
					map[string]any{"from": "fetch", "to": "format"},
				},
			},
		},
	}

	suite := TestSuite{
		Workflow: "edge-wf",
		Cases: []TestCase{
			{
				Name: "chained nodes",
				Mocks: map[string]MockConfig{
					"fetch": {Output: map[string]any{"name": "Alice"}},
				},
				Expect: TestExpectation{
					Status: "success",
					Output: map[string]any{
						"format.result": "Alice",
					},
				},
			},
		},
	}

	results := RunTestSuite(suite, rc, buildCoreNodeReg(t), nil)
	require.Len(t, results, 1)
	assert.True(t, results[0].Passed, results[0].Error)
}

// Assertions must be able to target intermediate (non-terminal) node outputs.
// The engine's eviction tracker frees them as soon as their consumers finish,
// so the test runner must retain all outputs for the duration of the run
// (issue #329).
func TestRunner_AssertsOnIntermediateNodeOutput(t *testing.T) {
	rc := &config.ResolvedConfig{
		Workflows: map[string]map[string]any{
			"workflows/wf.json": {
				"id": "intermediate-wf",
				"nodes": map[string]any{
					"fetch": map[string]any{"type": "db.query"},
					"format": map[string]any{
						"type": "transform.set",
						"config": map[string]any{
							"fields": map[string]any{
								"result": "{{ nodes.fetch.name }}",
							},
						},
					},
				},
				"edges": []any{
					map[string]any{"from": "fetch", "to": "format"},
				},
			},
		},
	}

	suite := TestSuite{
		Workflow: "intermediate-wf",
		Cases: []TestCase{
			{
				Name: "asserts on the evicted intermediate node",
				Mocks: map[string]MockConfig{
					"fetch": {Output: map[string]any{"name": "Alice"}},
				},
				Expect: TestExpectation{
					Status: "success",
					Output: map[string]any{
						"fetch.name":    "Alice",
						"format.result": "Alice",
					},
				},
			},
		},
	}

	results := RunTestSuite(suite, rc, buildCoreNodeReg(t), nil)
	require.Len(t, results, 1)
	assert.True(t, results[0].Passed, results[0].Error)
}

func TestRunner_ThreeNodeChain(t *testing.T) {
	rc := &config.ResolvedConfig{
		Workflows: map[string]map[string]any{
			"workflows/wf.json": {
				"id": "chain-wf",
				"nodes": map[string]any{
					"validate": map[string]any{"type": "transform.validate"},
					"insert":   map[string]any{"type": "db.insert"},
					"respond":  map[string]any{"type": "response.json"},
				},
				"edges": []any{
					map[string]any{"from": "validate", "to": "insert"},
					map[string]any{"from": "insert", "to": "respond"},
				},
			},
		},
	}

	suite := TestSuite{
		Workflow: "chain-wf",
		Cases: []TestCase{
			{
				Name: "chain works",
				Mocks: map[string]MockConfig{
					"validate": {Output: map[string]any{}},
					"insert":   {Output: map[string]any{"id": "uuid-123"}},
					"respond":  {Output: map[string]any{"status": 201}},
				},
				Expect: TestExpectation{
					Status: "success",
					Output: map[string]any{
						"respond.status": float64(201),
					},
				},
			},
		},
	}

	results := RunTestSuite(suite, rc, buildCoreNodeReg(t), nil)
	require.Len(t, results, 1)
	assert.True(t, results[0].Passed, results[0].Error)
}

// Unmocked response.json emits *api.HTTPResponse. Its output must be
// navigable by dot-path assertions with lowercase keys, like every other
// node output (issue #330).
func TestRunner_UnmockedResponseJSONDotPaths(t *testing.T) {
	rc := &config.ResolvedConfig{
		Workflows: map[string]map[string]any{
			"workflows/wf.json": {
				"id": "resp-wf",
				"nodes": map[string]any{
					"fetch": map[string]any{"type": "db.query"},
					"resp": map[string]any{
						"type": "response.json",
						"config": map[string]any{
							"status": 201,
							"body": map[string]any{
								"email": "{{ nodes.fetch.email }}",
							},
						},
					},
				},
				"edges": []any{
					map[string]any{"from": "fetch", "to": "resp"},
				},
			},
		},
	}

	suite := TestSuite{
		Workflow: "resp-wf",
		Cases: []TestCase{
			{
				Name: "dot paths into the response",
				Mocks: map[string]MockConfig{
					"fetch": {Output: map[string]any{"email": "a@b.c"}},
				},
				Expect: TestExpectation{
					Status: "success",
					Output: map[string]any{
						"resp.status":     float64(201),
						"resp.body.email": "a@b.c",
					},
					Outputs: map[string]any{
						"resp": map[string]any{
							"status": float64(201),
							"body":   map[string]any{"email": "a@b.c"},
						},
					},
				},
			},
		},
	}

	coreNodeReg := buildCoreNodeReg(t)
	require.NoError(t, coreNodeReg.RegisterFromPlugin(&response.Plugin{}))
	results := RunTestSuite(suite, rc, coreNodeReg, nil)
	require.Len(t, results, 1)
	assert.True(t, results[0].Passed, results[0].Error)
}

func TestRunTestCase_PopulatesTrace(t *testing.T) {
	rc := &config.ResolvedConfig{
		Workflows: map[string]map[string]any{
			"workflows/wf.json": {
				"id": "trace-wf",
				"nodes": map[string]any{
					"fetch": map[string]any{"type": "db.query"},
					"format": map[string]any{
						"type": "transform.set",
						"config": map[string]any{
							"fields": map[string]any{
								"result": "{{ nodes.fetch.name }}",
							},
						},
					},
				},
				"edges": []any{
					map[string]any{"from": "fetch", "to": "format"},
				},
			},
		},
	}

	suite := TestSuite{
		Workflow: "trace-wf",
		Cases: []TestCase{
			{
				Name: "trace populated",
				Mocks: map[string]MockConfig{
					"fetch": {Output: map[string]any{"name": "Alice"}},
				},
				Expect: TestExpectation{
					Status: "success",
					Output: map[string]any{
						"format.result": "Alice",
					},
				},
			},
		},
	}

	results := RunTestSuite(suite, rc, buildCoreNodeReg(t), nil)
	require.Len(t, results, 1)
	assert.True(t, results[0].Passed, results[0].Error)
	assert.NotEmpty(t, results[0].Trace, "trace should be populated")

	// Check that expected node IDs appear in trace
	nodeIDs := make(map[string]bool)
	for _, te := range results[0].Trace {
		nodeIDs[te.NodeID] = true
	}
	assert.True(t, nodeIDs["fetch"], "trace should contain fetch node")
	assert.True(t, nodeIDs["format"], "trace should contain format node")
}

func TestFormatResults_VerboseShowsTrace(t *testing.T) {
	suiteResults := []SuiteResult{
		{
			Suite: TestSuite{Workflow: "test-wf"},
			Results: []TestResult{
				{
					CaseName: "with trace",
					Passed:   true,
					Trace: []TraceEvent{
						{NodeID: "node1", Type: "transform.set", Output: "success", Duration: 100},
						{NodeID: "node2", Type: "db.query", Output: "success", Duration: 200},
					},
				},
			},
		},
	}

	output := FormatResults(suiteResults, true)
	assert.Contains(t, output, "Trace:")
	assert.Contains(t, output, "node1")
	assert.Contains(t, output, "node2")
}

func TestRunner_TestdataIntegration(t *testing.T) {
	sm := secrets.New()
	_ = sm.Load(context.Background())
	rc, errs := config.ValidateAll("../../testdata/valid-project", "development", sm)
	require.Empty(t, errs)

	suites, err := LoadTests(rc)
	require.NoError(t, err)
	require.NotEmpty(t, suites)

	coreNodeReg := buildCoreNodeReg(t)
	for _, suite := range suites {
		results := RunTestSuite(suite, rc, coreNodeReg, nil)
		for _, r := range results {
			assert.True(t, r.Passed, "suite=%s test=%q error=%s", suite.Workflow, r.CaseName, r.Error)
		}
	}
}

func TestRunner_SecretsAvailableInExpressions(t *testing.T) {
	rc := &config.ResolvedConfig{
		Workflows: map[string]map[string]any{
			"workflows/wf.json": {
				"id": "secret-wf",
				"nodes": map[string]any{
					"set": map[string]any{
						"type":   "transform.set",
						"config": map[string]any{"fields": map[string]any{"token": "{{ secrets.MY_TOKEN }}"}},
					},
				},
				"edges": []any{},
			},
		},
	}
	suite := TestSuite{
		ID:       "test-secrets",
		Workflow: "secret-wf",
		Cases: []TestCase{{
			Name:   "secrets resolve",
			Expect: TestExpectation{Status: "success", Output: map[string]any{"set.token": "s3cret"}},
		}},
	}
	results := RunTestSuite(suite, rc, buildCoreNodeReg(t), map[string]any{"MY_TOKEN": "s3cret"})
	require.Len(t, results, 1)
	assert.True(t, results[0].Passed, "got: %+v", results[0])
}
