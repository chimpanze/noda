package engine

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// cyclicWorkflows returns a workflow map whose graph contains a cycle, keyed
// the way config.ValidateAll keys rc.Workflows: by source file path.
func cyclicWorkflows() map[string]map[string]any {
	return map[string]map[string]any{
		"/proj/workflows/loop.json": {
			"id": "loop",
			"nodes": map[string]any{
				"a": map[string]any{"type": "util.log", "config": map[string]any{"level": "info", "message": "a"}},
				"b": map[string]any{"type": "util.log", "config": map[string]any{"level": "info", "message": "b"}},
			},
			"edges": []any{
				map[string]any{"from": "a", "to": "b"},
				map[string]any{"from": "b", "to": "a"},
			},
		},
	}
}

// testResolver reports the outputs of the node types used in this file's
// fixtures. Compile only asks for outputs, so nothing else is needed.
type testResolver struct{}

func (testResolver) OutputsForType(nodeType string) ([]string, bool) {
	switch nodeType {
	case "util.log":
		return []string{"success", "error"}, true
	default:
		return nil, false
	}
}

// The file key must be recoverable from the error without parsing its text.
// internal/startup uses it to tell the editor which file to mark.
func TestNewWorkflowCache_CompileFailureCarriesSourceFile(t *testing.T) {
	_, err := NewWorkflowCache(cyclicWorkflows(), testResolver{})
	require.Error(t, err)

	var compileErr *WorkflowCompileError
	require.ErrorAs(t, err, &compileErr)
	assert.Equal(t, "/proj/workflows/loop.json", compileErr.File)
}

// The text is load-bearing: it is what `noda start` prints today, and what
// `noda validate` prints under its "workflows validation failed:" heading now
// that the workflow phase is shared. Adding the type must not change a single
// byte of it.
func TestWorkflowCompileError_TextIsUnchanged(t *testing.T) {
	_, err := NewWorkflowCache(cyclicWorkflows(), testResolver{})
	require.Error(t, err)

	assert.Equal(t,
		`compile workflow "/proj/workflows/loop.json": cycle detected: a → b → a`,
		err.Error())
}

func TestWorkflowCompileError_Unwraps(t *testing.T) {
	inner := errors.New("boom")
	e := &WorkflowCompileError{File: "f.json", Err: inner}
	assert.Same(t, inner, errors.Unwrap(e))
	assert.Equal(t, "boom", e.Error())
}

// Two broken workflows must always report the same one first. Ranging the map
// directly meant identical input produced different errors between runs (#450).
func TestNewWorkflowCache_ReportsBrokenWorkflowsDeterministically(t *testing.T) {
	workflows := cyclicWorkflows()
	workflows["/proj/workflows/aaa-loop.json"] = workflows["/proj/workflows/loop.json"]

	for range 20 {
		_, err := NewWorkflowCache(workflows, testResolver{})
		require.Error(t, err)

		var compileErr *WorkflowCompileError
		require.ErrorAs(t, err, &compileErr)
		assert.Equal(t, "/proj/workflows/aaa-loop.json", compileErr.File,
			"the lexically first broken workflow must always be the one reported")
	}
}
