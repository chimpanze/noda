package expr

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsStatic_StaticString(t *testing.T) {
	assert.True(t, isStatic("hello"))
	assert.True(t, isStatic(""))
	assert.True(t, isStatic("some value with { braces }"))
}

func TestIsStatic_ExpressionString(t *testing.T) {
	assert.False(t, isStatic("{{ input.name }}"))
	assert.False(t, isStatic("Hello {{ input.name }}"))
}

func TestValidateStaticFields_AllStatic(t *testing.T) {
	config := map[string]any{
		"mode":     "sequential",
		"workflow": "my-workflow",
	}
	errs := validateStaticFields(config, []string{"mode", "workflow"})
	assert.Empty(t, errs)
}

func TestValidateStaticFields_ExpressionInStaticField(t *testing.T) {
	config := map[string]any{
		"mode":     "{{ input.mode }}",
		"workflow": "my-workflow",
	}
	errs := validateStaticFields(config, []string{"mode", "workflow"})
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
	errs := validateStaticFields(config, []string{"mode", "cases"})
	assert.Len(t, errs, 2)
}

func TestValidateStaticFields_NestedField(t *testing.T) {
	config := map[string]any{
		"match": map[string]any{
			"type": "{{ input.type }}",
		},
	}
	errs := validateStaticFields(config, []string{"match.type"})
	assert.Len(t, errs, 1)
	assert.Contains(t, errs[0].Error(), "match.type")
}

func TestValidateStaticFields_MissingField(t *testing.T) {
	config := map[string]any{
		"mode": "sequential",
	}
	errs := validateStaticFields(config, []string{"mode", "nonexistent"})
	assert.Empty(t, errs) // missing fields are fine
}

func TestValidateStaticFields_NonStringField(t *testing.T) {
	config := map[string]any{
		"count": 42,
	}
	errs := validateStaticFields(config, []string{"count"})
	assert.Empty(t, errs) // non-string fields skip
}
