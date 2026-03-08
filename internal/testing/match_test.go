package testing

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMatch_ExactValue(t *testing.T) {
	expected := TestExpectation{
		Status: "success",
		Output: map[string]any{"name": "Alice"},
	}
	actual := TestActualResult{
		Status: "success",
		Outputs: map[string]any{
			"name": "Alice",
		},
	}

	ok, msg := MatchExpectation(expected, actual)
	assert.True(t, ok, msg)
}

func TestMatch_WrongValue(t *testing.T) {
	expected := TestExpectation{
		Status: "success",
		Output: map[string]any{"name": "Alice"},
	}
	actual := TestActualResult{
		Status: "success",
		Outputs: map[string]any{
			"name": "Bob",
		},
	}

	ok, msg := MatchExpectation(expected, actual)
	assert.False(t, ok)
	assert.Contains(t, msg, "Alice")
	assert.Contains(t, msg, "Bob")
}

func TestMatch_MissingPath(t *testing.T) {
	expected := TestExpectation{
		Status: "success",
		Output: map[string]any{"name": "Alice"},
	}
	actual := TestActualResult{
		Status:  "success",
		Outputs: map[string]any{},
	}

	ok, msg := MatchExpectation(expected, actual)
	assert.False(t, ok)
	assert.Contains(t, msg, "not found")
}

func TestMatch_NestedPath(t *testing.T) {
	expected := TestExpectation{
		Status: "success",
		Output: map[string]any{
			"respond.status": float64(201),
		},
	}
	actual := TestActualResult{
		Status: "success",
		Outputs: map[string]any{
			"respond": map[string]any{"status": float64(201)},
		},
	}

	ok, msg := MatchExpectation(expected, actual)
	assert.True(t, ok, msg)
}

func TestMatch_PartialMatching(t *testing.T) {
	expected := TestExpectation{
		Status: "success",
		Output: map[string]any{"name": "Alice"},
	}
	actual := TestActualResult{
		Status: "success",
		Outputs: map[string]any{
			"name":  "Alice",
			"age":   30,
			"email": "alice@example.com",
		},
	}

	ok, msg := MatchExpectation(expected, actual)
	assert.True(t, ok, msg)
}

func TestMatch_ArrayComparison(t *testing.T) {
	expected := TestExpectation{
		Status: "success",
		Output: map[string]any{
			"items": []any{1.0, 2.0, 3.0},
		},
	}
	actual := TestActualResult{
		Status: "success",
		Outputs: map[string]any{
			"items": []any{1.0, 2.0, 3.0},
		},
	}

	ok, msg := MatchExpectation(expected, actual)
	assert.True(t, ok, msg)
}

func TestMatch_StatusMismatch(t *testing.T) {
	expected := TestExpectation{Status: "success"}
	actual := TestActualResult{Status: "error"}

	ok, msg := MatchExpectation(expected, actual)
	assert.False(t, ok)
	assert.Contains(t, msg, "status")
}

func TestMatch_ErrorNode(t *testing.T) {
	expected := TestExpectation{
		Status:    "error",
		ErrorNode: "validate",
	}
	actual := TestActualResult{
		Status:    "error",
		ErrorNode: "validate",
	}

	ok, msg := MatchExpectation(expected, actual)
	assert.True(t, ok, msg)
}

func TestMatch_ErrorNodeMismatch(t *testing.T) {
	expected := TestExpectation{
		Status:    "error",
		ErrorNode: "validate",
	}
	actual := TestActualResult{
		Status:    "error",
		ErrorNode: "insert",
	}

	ok, msg := MatchExpectation(expected, actual)
	assert.False(t, ok)
	assert.Contains(t, msg, "validate")
}

func TestMatch_NumericComparison(t *testing.T) {
	// JSON numbers are float64, but expected might be int
	expected := TestExpectation{
		Status: "success",
		Output: map[string]any{
			"respond.status": float64(201),
		},
	}
	actual := TestActualResult{
		Status: "success",
		Outputs: map[string]any{
			"respond": map[string]any{"status": 201},
		},
	}

	ok, msg := MatchExpectation(expected, actual)
	assert.True(t, ok, msg)
}
