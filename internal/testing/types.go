package testing

import "time"

// TestSuite represents a collection of test cases for a single workflow.
type TestSuite struct {
	ID       string
	Workflow string // referenced workflow ID
	FilePath string
	Cases    []TestCase
}

// TestCase represents a single test scenario.
type TestCase struct {
	Name   string
	Input  map[string]any
	Auth   *AuthConfig
	Mocks  map[string]MockConfig // keyed by node ID
	Expect TestExpectation
}

// AuthConfig holds auth data for a test case.
type AuthConfig struct {
	UserID string         `json:"user_id"`
	Roles  []string       `json:"roles"`
	Claims map[string]any `json:"claims"`
}

// MockConfig configures a mock node executor.
type MockConfig struct {
	Output     any        `json:"output"`     // data to return on success
	OutputName string     `json:"output_name"` // which output to fire (default: "success")
	Error      *MockError `json:"error"`      // if set, mock fails
}

// MockError configures a mock error.
type MockError struct {
	Message string `json:"message"`
}

// TestExpectation describes the expected result of a test case.
type TestExpectation struct {
	Status    string         `json:"status"`    // "success" or "error"
	Output    map[string]any `json:"output"`    // dot-path → expected value
	ErrorNode string         `json:"error_node"` // if status="error", which node should fail
}

// TestResult captures the outcome of running a single test case.
type TestResult struct {
	CaseName string
	Passed   bool
	Expected TestExpectation
	Actual   TestActualResult
	Error    string // failure reason if not passed
	Trace    []TraceEvent
	Duration time.Duration
}

// TestActualResult captures what actually happened during execution.
type TestActualResult struct {
	Status    string         // "success" or "error"
	Outputs   map[string]any // all node outputs
	ErrorNode string         // node that failed (if any)
	ErrorMsg  string         // error message (if any)
}

// TraceEvent records a single execution event.
type TraceEvent struct {
	NodeID   string
	Type     string
	Output   string
	Duration time.Duration
}

// SuiteResult groups test results for a single suite.
type SuiteResult struct {
	Suite   TestSuite
	Results []TestResult
}
