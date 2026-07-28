package http

import (
	"testing"

	"github.com/chimpanze/noda/internal/registry"
	"github.com/stretchr/testify/assert"
)

func TestConfigSchemasMatchExecutors(t *testing.T) {
	tests := []struct {
		nodeType     string
		schema       map[string]any
		minimalValid map[string]any // smallest config the executor accepts (from docs example)
		emptyValid   bool           // does the executor run with config {}?
		invalid      map[string]any // one config the executor would reject/misuse
	}{
		{"http.get", (&getDescriptor{}).ConfigSchema(),
			map[string]any{"url": "{{ input.url }}", "body": "{{ input.payload }}"}, false,
			map[string]any{"url": "/x", "headers": "not-an-object"}},
		{"http.post", (&postDescriptor{}).ConfigSchema(),
			map[string]any{"url": "{{ input.url }}"}, false,
			map[string]any{"headers": map[string]any{"X": "y"}}},
		{"http.request", (&requestDescriptor{}).ConfigSchema(),
			map[string]any{"method": "GET", "url": "{{ input.url }}"}, false,
			map[string]any{"url": "/x"}},
	}
	for _, tt := range tests {
		t.Run(tt.nodeType, func(t *testing.T) {
			assert.Empty(t, registry.CheckSchemaVocabulary(tt.schema))
			assert.Empty(t, registry.ValidateNodeConfig(tt.schema, tt.minimalValid), "minimal valid config must pass")
			emptyErrs := registry.ValidateNodeConfig(tt.schema, map[string]any{})
			if tt.emptyValid {
				assert.Empty(t, emptyErrs, "executor accepts {}, schema must too")
			} else {
				assert.NotEmpty(t, emptyErrs, "executor rejects {}, schema must too")
			}
			assert.NotEmpty(t, registry.ValidateNodeConfig(tt.schema, tt.invalid))
		})
	}
}

// The query field must accept BOTH shapes the docs show: a literal object and
// a whole-map expression string. The oneOf is what makes the second legal for
// the editor's stricter ajv validation; the Go walker short-circuits any
// string containing "{{" before it reaches the schema at all.
func TestQueryFieldAcceptsObjectAndExpression(t *testing.T) {
	schemas := map[string]map[string]any{
		"http.get":     (&getDescriptor{}).ConfigSchema(),
		"http.post":    (&postDescriptor{}).ConfigSchema(),
		"http.request": (&requestDescriptor{}).ConfigSchema(),
	}
	for nodeType, schema := range schemas {
		t.Run(nodeType, func(t *testing.T) {
			assert.Empty(t, registry.CheckSchemaVocabulary(schema))

			base := map[string]any{"url": "/items"}
			if nodeType == "http.request" {
				base["method"] = "GET"
			}

			objForm := map[string]any{}
			exprForm := map[string]any{}
			for k, v := range base {
				objForm[k] = v
				exprForm[k] = v
			}
			objForm["query"] = map[string]any{"limit": "10"}
			exprForm["query"] = "{{ input.q }}"

			assert.Empty(t, registry.ValidateNodeConfig(schema, objForm),
				"a literal object must validate")
			assert.Empty(t, registry.ValidateNodeConfig(schema, exprForm),
				"a whole-map expression must validate")
		})
	}
}
