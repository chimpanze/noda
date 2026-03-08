package testing

import (
	"context"
	"fmt"

	"github.com/chimpanze/noda/pkg/api"
)

// MockExecutor implements api.NodeExecutor with configurable behavior.
type MockExecutor struct {
	config  MockConfig
	outputs []string
}

// NewMockExecutor creates a mock executor from config.
func NewMockExecutor(mc MockConfig) *MockExecutor {
	outputs := []string{"success", "error"}
	if mc.OutputName != "" && mc.OutputName != "success" {
		outputs = []string{mc.OutputName, "error"}
	}
	return &MockExecutor{
		config:  mc,
		outputs: outputs,
	}
}

func (e *MockExecutor) Outputs() []string { return e.outputs }

func (e *MockExecutor) Execute(_ context.Context, _ api.ExecutionContext, _ map[string]any, _ map[string]any) (string, any, error) {
	if e.config.Error != nil {
		return "", nil, fmt.Errorf("%s", e.config.Error.Message)
	}

	outputName := "success"
	if e.config.OutputName != "" {
		outputName = e.config.OutputName
	}

	return outputName, e.config.Output, nil
}

// UnmockedExecutor fails with a clear message when a plugin node is not mocked.
type UnmockedExecutor struct {
	nodeID   string
	nodeType string
}

func NewUnmockedExecutor(nodeID, nodeType string) *UnmockedExecutor {
	return &UnmockedExecutor{nodeID: nodeID, nodeType: nodeType}
}

func (e *UnmockedExecutor) Outputs() []string { return []string{"success", "error"} }

func (e *UnmockedExecutor) Execute(_ context.Context, _ api.ExecutionContext, _ map[string]any, _ map[string]any) (string, any, error) {
	return "", nil, fmt.Errorf("node %q (type %q) has no mock — add a mock or use a core node", e.nodeID, e.nodeType)
}
