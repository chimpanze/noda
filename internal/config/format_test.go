package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFormatErrors_Empty(t *testing.T) {
	result := FormatErrors(nil)
	assert.Empty(t, result)
}

func TestFormatErrors_SingleError(t *testing.T) {
	errs := []ValidationError{
		{FilePath: "routes/tasks.json", JSONPath: "/trigger/workflow", Message: "required field missing"},
	}

	result := FormatErrors(errs)
	assert.Contains(t, result, "routes/tasks.json")
	assert.Contains(t, result, "/trigger/workflow")
	assert.Contains(t, result, "required field missing")
	assert.Contains(t, result, "1 error(s) in 1 file(s)")
}

func TestFormatErrors_GroupedByFile(t *testing.T) {
	errs := []ValidationError{
		{FilePath: "routes/a.json", JSONPath: "/id", Message: "missing"},
		{FilePath: "routes/b.json", JSONPath: "/method", Message: "invalid"},
		{FilePath: "routes/a.json", JSONPath: "/path", Message: "missing"},
	}

	result := FormatErrors(errs)
	assert.Contains(t, result, "3 error(s) in 2 file(s)")
}

func TestFormatErrors_Summary(t *testing.T) {
	errs := []ValidationError{
		{FilePath: "a.json", Message: "err1"},
		{FilePath: "b.json", Message: "err2"},
		{FilePath: "c.json", Message: "err3"},
	}

	result := FormatErrors(errs)
	assert.Contains(t, result, "3 error(s) in 3 file(s)")
}

func TestFormatErrors_NoJSONPath(t *testing.T) {
	errs := []ValidationError{
		{FilePath: "noda.json", Message: "general error"},
	}

	result := FormatErrors(errs)
	assert.Contains(t, result, "general error")
	assert.NotContains(t, result, ": :")
}
