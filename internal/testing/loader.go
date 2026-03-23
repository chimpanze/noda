package testing

import (
	"fmt"

	json "github.com/goccy/go-json"

	"github.com/chimpanze/noda/internal/config"
)

// testFileJSON represents the JSON structure of a test file.
type testFileJSON struct {
	ID       string         `json:"id"`
	Workflow string         `json:"workflow"`
	Tests    []testCaseJSON `json:"tests"`
}

type testCaseJSON struct {
	Name   string                    `json:"name"`
	Input  map[string]any            `json:"input"`
	Auth   *AuthConfig               `json:"auth"`
	Mocks  map[string]mockConfigJSON `json:"mocks"`
	Expect TestExpectation           `json:"expect"`
}

type mockConfigJSON struct {
	Output     any        `json:"output"`
	OutputName string     `json:"output_name"`
	Error      *MockError `json:"error"`
}

// LoadTests loads and parses test suites from the resolved config.
func LoadTests(rc *config.ResolvedConfig) ([]TestSuite, error) {
	if len(rc.Tests) == 0 {
		return nil, nil
	}

	var suites []TestSuite

	for filePath, data := range rc.Tests {
		suite, err := parseTestFile(filePath, data)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", filePath, err)
		}

		// Validate workflow reference
		if err := validateWorkflowRef(suite.Workflow, rc); err != nil {
			return nil, fmt.Errorf("%s: %w", filePath, err)
		}

		// Validate mock node IDs exist in the workflow
		wfConfig := findWorkflow(suite.Workflow, rc)
		if wfConfig != nil {
			for _, tc := range suite.Cases {
				if err := validateMockNodes(tc, wfConfig); err != nil {
					return nil, fmt.Errorf("%s: test %q: %w", filePath, tc.Name, err)
				}
			}
		}

		suites = append(suites, suite)
	}

	return suites, nil
}

func parseTestFile(filePath string, data map[string]any) (TestSuite, error) {
	// Re-marshal and unmarshal to use JSON struct tags
	bytes, err := json.Marshal(data)
	if err != nil {
		return TestSuite{}, fmt.Errorf("marshal: %w", err)
	}

	var tf testFileJSON
	if err := json.Unmarshal(bytes, &tf); err != nil {
		return TestSuite{}, fmt.Errorf("parse: %w", err)
	}

	if tf.Workflow == "" {
		return TestSuite{}, fmt.Errorf("missing 'workflow' field")
	}

	suite := TestSuite{
		ID:       tf.ID,
		Workflow: tf.Workflow,
		FilePath: filePath,
	}

	for _, tc := range tf.Tests {
		testCase := TestCase{
			Name:   tc.Name,
			Input:  tc.Input,
			Auth:   tc.Auth,
			Expect: tc.Expect,
			Mocks:  make(map[string]MockConfig),
		}
		for nodeID, mc := range tc.Mocks {
			testCase.Mocks[nodeID] = MockConfig(mc)
		}
		suite.Cases = append(suite.Cases, testCase)
	}

	return suite, nil
}

func validateWorkflowRef(workflowID string, rc *config.ResolvedConfig) error {
	for _, wf := range rc.Workflows {
		if id, _ := wf["id"].(string); id == workflowID {
			return nil
		}
	}
	return fmt.Errorf("workflow %q not found", workflowID)
}

func findWorkflow(workflowID string, rc *config.ResolvedConfig) map[string]any {
	for _, wf := range rc.Workflows {
		if id, _ := wf["id"].(string); id == workflowID {
			return wf
		}
	}
	return nil
}

func validateMockNodes(tc TestCase, wf map[string]any) error {
	nodes, _ := wf["nodes"].(map[string]any)
	if nodes == nil {
		return nil
	}

	for nodeID := range tc.Mocks {
		if _, ok := nodes[nodeID]; !ok {
			return fmt.Errorf("mock node %q does not exist in workflow", nodeID)
		}
	}
	return nil
}
