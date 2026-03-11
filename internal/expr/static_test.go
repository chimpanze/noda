package expr

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsStatic_StaticString(t *testing.T) {
	assert.True(t, IsStatic("hello"))
	assert.True(t, IsStatic(""))
	assert.True(t, IsStatic("some value with { braces }"))
}

func TestIsStatic_ExpressionString(t *testing.T) {
	assert.False(t, IsStatic("{{ input.name }}"))
	assert.False(t, IsStatic("Hello {{ input.name }}"))
}

func TestValidateStaticFields_AllStatic(t *testing.T) {
	config := map[string]any{
		"mode":     "sequential",
		"workflow": "my-workflow",
	}
	errs := ValidateStaticFields(config, []string{"mode", "workflow"})
	assert.Empty(t, errs)
}

func TestValidateStaticFields_ExpressionInStaticField(t *testing.T) {
	config := map[string]any{
		"mode":     "{{ input.mode }}",
		"workflow": "my-workflow",
	}
	errs := ValidateStaticFields(config, []string{"mode", "workflow"})
	assert.Len(t, errs, 1)
	assert.Contains(t, errs[0].Error(), "mode")
	assert.Contains(t, errs[0].Error(), "static value")
}

func TestValidateStaticFields_MixedFields(t *testing.T) {
	config := map[string]any{
		"mode":  "{{ input.mode }}",
		"cases": "{{ input.cases }}",
		"title": "{{ input.title }}", // not in static fields list
	}
	errs := ValidateStaticFields(config, []string{"mode", "cases"})
	assert.Len(t, errs, 2)
}

func TestValidateStaticFields_NestedField(t *testing.T) {
	config := map[string]any{
		"match": map[string]any{
			"type": "{{ input.type }}",
		},
	}
	errs := ValidateStaticFields(config, []string{"match.type"})
	assert.Len(t, errs, 1)
	assert.Contains(t, errs[0].Error(), "match.type")
}

func TestValidateStaticFields_MissingField(t *testing.T) {
	config := map[string]any{
		"mode": "sequential",
	}
	errs := ValidateStaticFields(config, []string{"mode", "nonexistent"})
	assert.Empty(t, errs) // missing fields are fine
}

func TestValidateStaticFields_NonStringField(t *testing.T) {
	config := map[string]any{
		"count": 42,
	}
	errs := ValidateStaticFields(config, []string{"count"})
	assert.Empty(t, errs) // non-string fields skip
}
