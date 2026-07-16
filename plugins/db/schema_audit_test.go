package db

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
		{"db.count", (&countDescriptor{}).ConfigSchema(),
			map[string]any{"table": "tasks", "where": map[string]any{"user_id": "{{ auth.sub }}"}}, false,
			map[string]any{"table": 5}},

		{"db.create", (&createDescriptor{}).ConfigSchema(),
			map[string]any{"table": "tasks", "data": map[string]any{"title": "{{ input.title }}"}}, false,
			map[string]any{"table": "tasks"}},

		{"db.delete", (&deleteDescriptor{}).ConfigSchema(),
			map[string]any{"table": "tasks", "where": map[string]any{"id": "{{ input.task_id }}"}}, false,
			map[string]any{"table": "tasks"}},

		{"db.exec", (&execDescriptor{}).ConfigSchema(),
			map[string]any{"query": "UPDATE tasks SET done = true WHERE id = $1", "params": []any{"{{ input.task_id }}"}}, false,
			map[string]any{"params": []any{1}}},

		{"db.find", (&findDescriptor{}).ConfigSchema(),
			map[string]any{"table": "tasks", "where": map[string]any{"user_id": "{{ auth.sub }}"}, "having": map[string]any{"query": "count(*) > ?", "params": []any{5}}}, false,
			map[string]any{"table": "tasks", "limit": "not-an-int"}},

		{"db.findOne", (&findOneDescriptor{}).ConfigSchema(),
			map[string]any{"table": "tasks", "where": map[string]any{"id": "{{ input.task_id }}"}, "group": "id", "limit": 10, "offset": 0}, false,
			map[string]any{"table": "tasks", "required": "yes"}},

		{"db.query", (&queryDescriptor{}).ConfigSchema(),
			map[string]any{"query": "SELECT * FROM tasks WHERE id = $1", "params": []any{"{{ input.task_id }}"}}, false,
			map[string]any{"params": []any{1}}},

		{"db.update", (&updateDescriptor{}).ConfigSchema(),
			map[string]any{"table": "tasks", "data": map[string]any{"done": true}, "where": map[string]any{"id": "{{ input.task_id }}"}}, false,
			map[string]any{"table": "tasks", "data": map[string]any{"done": true}}},

		{"db.upsert", (&upsertDescriptor{}).ConfigSchema(),
			map[string]any{"table": "user_settings", "data": map[string]any{"user_id": "{{ auth.sub }}"}, "conflict": "user_id", "update": []any{"theme"}}, false,
			map[string]any{"table": "user_settings", "data": map[string]any{"user_id": 1}, "conflict": 5}},
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
