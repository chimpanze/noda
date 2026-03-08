package testing

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestFormatResults_AllPassing(t *testing.T) {
	results := []SuiteResult{
		{
			Suite: TestSuite{Workflow: "create-task"},
			Results: []TestResult{
				{CaseName: "creates task", Passed: true, Duration: 5 * time.Millisecond},
				{CaseName: "validates input", Passed: true, Duration: 3 * time.Millisecond},
			},
		},
	}

	output := FormatResults(results, false)
	assert.Contains(t, output, "create-task")
	assert.Contains(t, output, "✓ creates task")
	assert.Contains(t, output, "✓ validates input")
	assert.Contains(t, output, "2 passed, 0 failed, 2 total")
}

func TestFormatResults_WithFailure(t *testing.T) {
	results := []SuiteResult{
		{
			Suite: TestSuite{Workflow: "create-task"},
			Results: []TestResult{
				{CaseName: "creates task", Passed: true, Duration: 5 * time.Millisecond},
				{CaseName: "bad input", Passed: false, Error: "expected status \"error\", got \"success\"", Duration: 2 * time.Millisecond},
			},
		},
	}

	output := FormatResults(results, false)
	assert.Contains(t, output, "✓ creates task")
	assert.Contains(t, output, "✗ bad input")
	assert.Contains(t, output, "expected status")
	assert.Contains(t, output, "1 passed, 1 failed, 2 total")
}

func TestFormatResults_Summary(t *testing.T) {
	results := []SuiteResult{
		{
			Suite:   TestSuite{Workflow: "wf1"},
			Results: []TestResult{{Passed: true}},
		},
		{
			Suite:   TestSuite{Workflow: "wf2"},
			Results: []TestResult{{Passed: false, Error: "fail"}},
		},
	}

	output := FormatResults(results, false)
	assert.Contains(t, output, "1 passed, 1 failed, 2 total")
}

func TestFormatResults_Verbose(t *testing.T) {
	results := []SuiteResult{
		{
			Suite: TestSuite{Workflow: "wf1"},
			Results: []TestResult{
				{
					CaseName: "test1",
					Passed:   true,
					Duration: 5 * time.Millisecond,
					Trace: []TraceEvent{
						{NodeID: "validate", Type: "transform.validate", Output: "success", Duration: 1 * time.Millisecond},
						{NodeID: "insert", Type: "db.insert", Output: "success", Duration: 2 * time.Millisecond},
					},
				},
			},
		},
	}

	output := FormatResults(results, true)
	assert.Contains(t, output, "Trace:")
	assert.Contains(t, output, "validate")
	assert.Contains(t, output, "insert")
}

func TestFormatResults_NonVerboseNoTrace(t *testing.T) {
	results := []SuiteResult{
		{
			Suite: TestSuite{Workflow: "wf1"},
			Results: []TestResult{
				{
					CaseName: "test1",
					Passed:   true,
					Duration: 5 * time.Millisecond,
					Trace:    []TraceEvent{{NodeID: "n1"}},
				},
			},
		},
	}

	output := FormatResults(results, false)
	assert.NotContains(t, output, "Trace:")
}
