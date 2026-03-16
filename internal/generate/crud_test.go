package generate

import (
	"testing"
)

func TestSingularize(t *testing.T) {
	tests := []struct {
		input, want string
	}{
		{"tasks", "task"},
		{"users", "user"},
		{"categories", "category"},
		{"boxes", "box"},
		{"addresses", "address"},
		{"status", "status"},
		{"buses", "bus"},
		{"watches", "watch"},
		{"dishes", "dish"},
		{"quizzes", "quiz"},
	}

	for _, tt := range tests {
		got := singularize(tt.input)
		if got != tt.want {
			t.Errorf("singularize(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestCapitalize(t *testing.T) {
	tests := []struct {
		input, want string
	}{
		{"task", "Task"},
		{"", ""},
		{"Task", "Task"},
	}

	for _, tt := range tests {
		got := capitalize(tt.input)
		if got != tt.want {
			t.Errorf("capitalize(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestGenerateCRUD_AllOperations(t *testing.T) {
	model := map[string]any{
		"table": "tasks",
		"columns": map[string]any{
			"id":    map[string]any{"type": "uuid", "primary_key": true, "default": "gen_random_uuid()"},
			"title": map[string]any{"type": "text", "not_null": true},
		},
	}

	result := GenerateCRUD(model, CRUDOptions{})

	expectedFiles := []string{
		"routes/create-task.json",
		"routes/list-tasks.json",
		"routes/get-task.json",
		"routes/update-task.json",
		"routes/delete-task.json",
		"workflows/create-task.json",
		"workflows/list-tasks.json",
		"workflows/get-task.json",
		"workflows/update-task.json",
		"workflows/delete-task.json",
		"schemas/models/Task.json",
	}

	for _, f := range expectedFiles {
		if _, ok := result.Files[f]; !ok {
			t.Errorf("missing file: %s", f)
		}
	}
}

func TestGenerateCRUD_SelectedOps(t *testing.T) {
	model := map[string]any{
		"table": "tasks",
		"columns": map[string]any{
			"id":    map[string]any{"type": "uuid", "primary_key": true},
			"title": map[string]any{"type": "text"},
		},
	}

	result := GenerateCRUD(model, CRUDOptions{
		Operations: []string{"create", "list"},
		Artifacts:  []string{"routes"},
	})

	if len(result.Files) != 2 {
		t.Errorf("expected 2 files, got %d: %v", len(result.Files), keys(result.Files))
	}

	if _, ok := result.Files["routes/create-task.json"]; !ok {
		t.Error("missing routes/create-task.json")
	}
	if _, ok := result.Files["routes/list-tasks.json"]; !ok {
		t.Error("missing routes/list-tasks.json")
	}
}

func TestGenerateCRUD_EmptyTable(t *testing.T) {
	model := map[string]any{}
	result := GenerateCRUD(model, CRUDOptions{})
	if len(result.Files) != 0 {
		t.Errorf("expected 0 files for empty model, got %d", len(result.Files))
	}
}

func TestGenerateCRUD_WithScope(t *testing.T) {
	model := map[string]any{
		"table": "tasks",
		"columns": map[string]any{
			"id":           map[string]any{"type": "uuid", "primary_key": true},
			"workspace_id": map[string]any{"type": "uuid", "not_null": true},
			"title":        map[string]any{"type": "text"},
		},
	}

	result := GenerateCRUD(model, CRUDOptions{
		ScopeCol:   "workspace_id",
		ScopeParam: "workspace_id",
		Operations: []string{"list"},
		Artifacts:  []string{"workflows"},
	})

	wf, ok := result.Files["workflows/list-tasks.json"]
	if !ok {
		t.Fatal("missing list workflow")
	}

	nodes, _ := wf["nodes"].(map[string]any)
	findNode, _ := nodes["find"].(map[string]any)
	findConfig, _ := findNode["config"].(map[string]any)
	where, _ := findConfig["where"].(map[string]any)

	if where["workspace_id"] != "{{ input.workspace_id }}" {
		t.Errorf("expected scope in where clause, got %v", where)
	}
}

func TestGenerateCRUD_CustomBasePath(t *testing.T) {
	model := map[string]any{
		"table": "tasks",
		"columns": map[string]any{
			"id": map[string]any{"type": "uuid", "primary_key": true},
		},
	}

	result := GenerateCRUD(model, CRUDOptions{
		BasePath:   "/v2/tasks",
		Operations: []string{"get"},
		Artifacts:  []string{"routes"},
	})

	route, ok := result.Files["routes/get-task.json"]
	if !ok {
		t.Fatal("missing get route")
	}

	path, _ := route["path"].(string)
	if path != "/v2/tasks/:id" {
		t.Errorf("expected /v2/tasks/:id, got %s", path)
	}
}

func TestGenerateModelSchemas(t *testing.T) {
	columns := []colInfo{
		{Name: "id", Type: "uuid", PrimaryKey: true},
		{Name: "title", Type: "text", NotNull: true},
		{Name: "count", Type: "integer"},
		{Name: "status", Type: "text", Enum: []string{"active", "inactive"}},
	}

	schemas := generateModelSchemas("task", columns)

	createSchema, ok := schemas["CreateTask"].(map[string]any)
	if !ok {
		t.Fatal("missing CreateTask schema")
	}

	props := createSchema["properties"].(map[string]any)
	if _, hasID := props["id"]; hasID {
		t.Error("CreateTask should not include primary key 'id'")
	}

	titleProp := props["title"].(map[string]any)
	if titleProp["type"] != "string" {
		t.Errorf("expected string type for title, got %v", titleProp["type"])
	}

	statusProp := props["status"].(map[string]any)
	if statusProp["enum"] == nil {
		t.Error("expected enum for status")
	}

	countProp := props["count"].(map[string]any)
	if countProp["type"] != "integer" {
		t.Errorf("expected integer type for count, got %v", countProp["type"])
	}
}

func keys(m map[string]map[string]any) []string {
	result := make([]string, 0, len(m))
	for k := range m {
		result = append(result, k)
	}
	return result
}
