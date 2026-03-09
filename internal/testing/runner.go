package testing

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/chimpanze/noda/internal/config"
	"github.com/chimpanze/noda/internal/engine"
	"github.com/chimpanze/noda/internal/registry"
	"github.com/chimpanze/noda/pkg/api"
)

// isCoreNodeType returns true if a node type is registered in the core node registry.
func isCoreNodeType(nodeType string, coreReg *registry.NodeRegistry) bool {
	_, ok := coreReg.GetFactory(nodeType)
	return ok
}

// RunTestSuite executes all test cases in a suite and returns results.
func RunTestSuite(
	suite TestSuite,
	rc *config.ResolvedConfig,
	coreNodeReg *registry.NodeRegistry,
) []TestResult {
	var results []TestResult

	for _, tc := range suite.Cases {
		result := runTestCase(tc, suite.Workflow, rc, coreNodeReg)
		results = append(results, result)
	}

	return results
}

func runTestCase(
	tc TestCase,
	workflowID string,
	rc *config.ResolvedConfig,
	coreNodeReg *registry.NodeRegistry,
) TestResult {
	start := time.Now()

	// Parse workflow config
	wfConfig, err := parseWorkflowConfig(workflowID, rc)
	if err != nil {
		return TestResult{
			CaseName: tc.Name,
			Passed:   false,
			Expected: tc.Expect,
			Error:    fmt.Sprintf("parse workflow: %s", err),
			Duration: time.Since(start),
		}
	}

	// Build test-specific node registry with mocks
	testNodeReg, resolver, err := buildTestRegistry(wfConfig, tc.Mocks, coreNodeReg)
	if err != nil {
		return TestResult{
			CaseName: tc.Name,
			Passed:   false,
			Expected: tc.Expect,
			Error:    fmt.Sprintf("build test registry: %s", err),
			Duration: time.Since(start),
		}
	}

	// Compile workflow
	graph, err := engine.Compile(wfConfig, resolver)
	if err != nil {
		return TestResult{
			CaseName: tc.Name,
			Passed:   false,
			Expected: tc.Expect,
			Error:    fmt.Sprintf("compile workflow: %s", err),
			Duration: time.Since(start),
		}
	}

	// Build execution context
	opts := []engine.ExecutionContextOption{
		engine.WithWorkflowID(workflowID),
	}
	if tc.Input != nil {
		opts = append(opts, engine.WithInput(tc.Input))
	}
	if tc.Auth != nil {
		opts = append(opts, engine.WithAuth(&api.AuthData{
			UserID: tc.Auth.UserID,
			Roles:  tc.Auth.Roles,
			Claims: tc.Auth.Claims,
		}))
	}
	execCtx := engine.NewExecutionContext(opts...)

	// Execute
	svcReg := registry.NewServiceRegistry()
	execErr := engine.ExecuteGraph(context.Background(), graph, execCtx, svcReg, testNodeReg)

	// Build actual result
	actual := TestActualResult{
		Outputs: collectOutputs(graph, execCtx),
	}
	if execErr != nil {
		actual.Status = "error"
		actual.ErrorMsg = execErr.Error()
		// Try to extract error node from error message
		actual.ErrorNode = extractErrorNode(execErr.Error())
	} else {
		actual.Status = "success"
	}

	// Match expectations
	passed, mismatch := MatchExpectation(tc.Expect, actual)

	result := TestResult{
		CaseName: tc.Name,
		Passed:   passed,
		Expected: tc.Expect,
		Actual:   actual,
		Duration: time.Since(start),
	}
	if !passed {
		result.Error = mismatch
	}

	return result
}

// parseWorkflowConfig finds a workflow by ID and converts it to engine.WorkflowConfig.
func parseWorkflowConfig(workflowID string, rc *config.ResolvedConfig) (engine.WorkflowConfig, error) {
	wfData := findWorkflow(workflowID, rc)
	if wfData == nil {
		return engine.WorkflowConfig{}, fmt.Errorf("workflow %q not found", workflowID)
	}
	return engine.ParseWorkflowFromMap(workflowID, wfData)
}

// buildTestRegistry creates a NodeRegistry with core types and mock overrides.
// It modifies the WorkflowConfig to use synthetic types for mocked/unmocked nodes.
func buildTestRegistry(
	wf engine.WorkflowConfig,
	mocks map[string]MockConfig,
	coreNodeReg *registry.NodeRegistry,
) (*registry.NodeRegistry, engine.NodeOutputResolver, error) {
	testReg := registry.NewNodeRegistry()

	// Copy all core node factories
	for _, nodeType := range coreNodeReg.AllTypes() {
		factory, _ := coreNodeReg.GetFactory(nodeType)
		testReg.RegisterFactory(nodeType, factory)
	}

	// Track synthetic type outputs for the resolver
	syntheticOutputs := make(map[string][]string)

	// Process each node in the workflow
	for nodeID, node := range wf.Nodes {
		// Explicit mocks always take priority, even for core types
		if mc, hasMock := mocks[nodeID]; hasMock {
			// Create synthetic type for this mock
			syntheticType := fmt.Sprintf("__mock__.%s", nodeID)
			mockCfg := mc // capture
			testReg.RegisterFactory(syntheticType, func(_ map[string]any) api.NodeExecutor {
				return NewMockExecutor(mockCfg)
			})

			outputs := []string{"success", "error"}
			if mc.OutputName != "" && mc.OutputName != "success" {
				outputs = []string{mc.OutputName, "error"}
			}
			syntheticOutputs[syntheticType] = outputs

			// Replace node type in workflow config
			node.Type = syntheticType
			node.Services = nil // mocks don't use services
			wf.Nodes[nodeID] = node
		} else if isCoreNodeType(node.Type, coreNodeReg) {
			continue // core types without explicit mocks use real factories
		} else {
			// Unmocked plugin node — create error factory
			syntheticType := fmt.Sprintf("__unmocked__.%s", nodeID)
			nID, nType := nodeID, node.Type // capture
			testReg.RegisterFactory(syntheticType, func(_ map[string]any) api.NodeExecutor {
				return NewUnmockedExecutor(nID, nType)
			})
			syntheticOutputs[syntheticType] = []string{"success", "error"}

			node.Type = syntheticType
			node.Services = nil
			wf.Nodes[nodeID] = node
		}
	}

	resolver := &testOutputResolver{
		coreReg:          coreNodeReg,
		syntheticOutputs: syntheticOutputs,
	}

	return testReg, resolver, nil
}

// testOutputResolver provides output names for both core and synthetic types.
type testOutputResolver struct {
	coreReg          *registry.NodeRegistry
	syntheticOutputs map[string][]string
}

func (r *testOutputResolver) OutputsForType(nodeType string) ([]string, bool) {
	// Check synthetic types first
	if outputs, ok := r.syntheticOutputs[nodeType]; ok {
		return outputs, true
	}

	// Check core registry
	desc, ok := r.coreReg.GetDescriptor(nodeType)
	if ok {
		// Use descriptor to determine outputs by creating a temporary executor
		factory, _ := r.coreReg.GetFactory(nodeType)
		if factory != nil {
			exec := factory(nil)
			return exec.Outputs(), true
		}
		_ = desc
	}

	// Default
	return []string{"success", "error"}, true
}

func collectOutputs(graph *engine.CompiledGraph, execCtx *engine.ExecutionContextImpl) map[string]any {
	outputs := make(map[string]any)
	for nodeID := range graph.Nodes {
		if data, ok := execCtx.GetOutput(nodeID); ok {
			outputs[nodeID] = data
		}
	}
	return outputs
}

func extractErrorNode(errMsg string) string {
	// Error messages from dispatchNode are formatted as: node "X": ...
	if strings.HasPrefix(errMsg, "node \"") {
		end := strings.Index(errMsg[6:], "\"")
		if end > 0 {
			return errMsg[6 : 6+end]
		}
	}
	return ""
}

// ParseWorkflowFromConfig is exported for testing.
func ParseWorkflowFromConfig(workflowID string, rc *config.ResolvedConfig) (engine.WorkflowConfig, error) {
	return parseWorkflowConfig(workflowID, rc)
}

// WorkflowConfigFromJSON converts raw workflow JSON data to engine.WorkflowConfig.
func WorkflowConfigFromJSON(data map[string]any) (engine.WorkflowConfig, error) {
	id, _ := data["id"].(string)
	return engine.ParseWorkflowFromMap(id, data)
}
