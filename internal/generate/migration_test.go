package generate

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestQuoteIdent_Simple(t *testing.T) {
	got := quoteIdent("users")
	if got != `"users"` {
		t.Errorf("quoteIdent(users) = %q, want %q", got, `"users"`)
	}
}

func TestQuoteIdent_EmbeddedQuotes(t *testing.T) {
	got := quoteIdent(`my"table`)
	if got != `"my""table"` {
		t.Errorf("quoteIdent(my\"table) = %q, want %q", got, `"my""table"`)
	}
}

func TestQuoteIdent_ReservedWord(t *testing.T) {
	got := quoteIdent("select")
	if got != `"select"` {
		t.Errorf("quoteIdent(select) = %q, want %q", got, `"select"`)
	}
}

func TestGenerateMigration_QuotedIdentifiers(t *testing.T) {
	// Use a table name with a reserved word to verify quoting
	old := map[string]ModelDef{}
	new := map[string]ModelDef{
		"order": {
			Table: "order",
			Columns: map[string]ColumnDef{
				"id":     {Type: "uuid", PrimaryKey: true},
				"select": {Type: "text"},
			},
		},
	}

	changes := diffModels(old, new)
	up, down := changesToSQL(changes, new)

	if !strings.Contains(up, `"order"`) {
		t.Errorf("expected quoted table name in up SQL, got:\n%s", up)
	}
	if !strings.Contains(up, `"select"`) {
		t.Errorf("expected quoted column name in up SQL, got:\n%s", up)
	}
	if !strings.Contains(down, `"order"`) {
		t.Errorf("expected quoted table name in down SQL, got:\n%s", down)
	}
}

func TestPgType(t *testing.T) {
	tests := []struct {
		col  ColumnDef
		want string
	}{
		{ColumnDef{Type: "uuid"}, "UUID"},
		{ColumnDef{Type: "text"}, "TEXT"},
		{ColumnDef{Type: "varchar"}, "VARCHAR(255)"},
		{ColumnDef{Type: "varchar", MaxLength: 100}, "VARCHAR(100)"},
		{ColumnDef{Type: "integer"}, "INTEGER"},
		{ColumnDef{Type: "int"}, "INTEGER"},
		{ColumnDef{Type: "bigint"}, "BIGINT"},
		{ColumnDef{Type: "boolean"}, "BOOLEAN"},
		{ColumnDef{Type: "bool"}, "BOOLEAN"},
		{ColumnDef{Type: "decimal"}, "NUMERIC(10,2)"},
		{ColumnDef{Type: "decimal", Precision: 8, Scale: 4}, "NUMERIC(8,4)"},
		{ColumnDef{Type: "timestamp"}, "TIMESTAMPTZ"},
		{ColumnDef{Type: "json"}, "JSONB"},
		{ColumnDef{Type: "jsonb"}, "JSONB"},
		{ColumnDef{Type: "serial"}, "SERIAL"},
	}

	for _, tt := range tests {
		got := pgType(tt.col)
		if got != tt.want {
			t.Errorf("pgType(%q) = %q, want %q", tt.col.Type, got, tt.want)
		}
	}
}

func TestDiffModels_NewTable(t *testing.T) {
	old := map[string]ModelDef{}
	new := map[string]ModelDef{
		"users": {
			Table: "users",
			Columns: map[string]ColumnDef{
				"id":   {Type: "uuid", PrimaryKey: true},
				"name": {Type: "text", NotNull: true},
			},
		},
	}

	changes := diffModels(old, new)
	if len(changes) != 1 {
		t.Fatalf("expected 1 change, got %d", len(changes))
	}
	if changes[0].Type != "create_table" {
		t.Errorf("expected create_table, got %s", changes[0].Type)
	}
}

func TestDiffModels_DropTable(t *testing.T) {
	old := map[string]ModelDef{
		"users": {Table: "users", Columns: map[string]ColumnDef{"id": {Type: "uuid"}}},
	}
	new := map[string]ModelDef{}

	changes := diffModels(old, new)
	if len(changes) != 1 {
		t.Fatalf("expected 1 change, got %d", len(changes))
	}
	if changes[0].Type != "drop_table" {
		t.Errorf("expected drop_table, got %s", changes[0].Type)
	}
}

func TestDiffModels_AddColumn(t *testing.T) {
	old := map[string]ModelDef{
		"users": {Table: "users", Columns: map[string]ColumnDef{
			"id": {Type: "uuid"},
		}},
	}
	new := map[string]ModelDef{
		"users": {Table: "users", Columns: map[string]ColumnDef{
			"id":    {Type: "uuid"},
			"email": {Type: "text", NotNull: true},
		}},
	}

	changes := diffModels(old, new)
	if len(changes) != 1 {
		t.Fatalf("expected 1 change, got %d", len(changes))
	}
	if changes[0].Type != "add_column" || changes[0].Column != "email" {
		t.Errorf("expected add_column email, got %s %s", changes[0].Type, changes[0].Column)
	}
}

func TestDiffModels_AlterColumn(t *testing.T) {
	old := map[string]ModelDef{
		"users": {Table: "users", Columns: map[string]ColumnDef{
			"name": {Type: "text"},
		}},
	}
	new := map[string]ModelDef{
		"users": {Table: "users", Columns: map[string]ColumnDef{
			"name": {Type: "varchar", MaxLength: 100},
		}},
	}

	changes := diffModels(old, new)
	if len(changes) != 1 {
		t.Fatalf("expected 1 change, got %d", len(changes))
	}
	if changes[0].Type != "alter_column" {
		t.Errorf("expected alter_column, got %s", changes[0].Type)
	}
}

func TestDiffModels_Timestamps(t *testing.T) {
	old := map[string]ModelDef{
		"users": {Table: "users", Columns: map[string]ColumnDef{"id": {Type: "uuid"}}, Timestamps: false},
	}
	new := map[string]ModelDef{
		"users": {Table: "users", Columns: map[string]ColumnDef{"id": {Type: "uuid"}}, Timestamps: true},
	}

	changes := diffModels(old, new)
	if len(changes) != 2 {
		t.Fatalf("expected 2 changes (created_at, updated_at), got %d", len(changes))
	}
}

func TestCreateTableSQL(t *testing.T) {
	model := &ModelDef{
		Table: "tasks",
		Columns: map[string]ColumnDef{
			"id":     {Type: "uuid", PrimaryKey: true, Default: "gen_random_uuid()"},
			"title":  {Type: "text", NotNull: true},
			"status": {Type: "text", NotNull: true, Default: "'todo'"},
		},
		Timestamps: true,
		Indexes: []IndexDef{
			{Columns: []string{"status"}},
		},
	}

	up, down := createTableSQL(model)

	if !strings.Contains(up, `CREATE TABLE "tasks"`) {
		t.Errorf("expected CREATE TABLE \"tasks\" in up SQL, got:\n%s", up)
	}
	if !strings.Contains(up, `PRIMARY KEY ("id")`) {
		t.Errorf("expected quoted PRIMARY KEY in up SQL, got:\n%s", up)
	}
	if !strings.Contains(up, `"created_at" TIMESTAMPTZ NOT NULL DEFAULT NOW()`) {
		t.Errorf("expected quoted created_at in up SQL, got:\n%s", up)
	}
	if !strings.Contains(up, `CREATE INDEX "idx_tasks_status" ON "tasks" ("status")`) {
		t.Errorf("expected quoted index in up SQL, got:\n%s", up)
	}
	if !strings.Contains(down, `DROP TABLE IF EXISTS "tasks" CASCADE`) {
		t.Errorf("expected quoted DROP TABLE in down SQL, got:\n%s", down)
	}
}

func TestCreateTableSQL_WithFK(t *testing.T) {
	model := &ModelDef{
		Table: "tasks",
		Columns: map[string]ColumnDef{
			"id":           {Type: "uuid", PrimaryKey: true},
			"workspace_id": {Type: "uuid", NotNull: true},
		},
		Relations: map[string]RelDef{
			"workspace": {Type: "belongsTo", Table: "workspaces", ForeignKey: "workspace_id", OnDelete: "CASCADE"},
		},
	}

	up, _ := createTableSQL(model)

	if !strings.Contains(up, `FOREIGN KEY ("workspace_id") REFERENCES "workspaces" ("id") ON DELETE CASCADE`) {
		t.Errorf("expected quoted FK constraint in up SQL, got:\n%s", up)
	}
}

func TestCreateTableSQL_ManyToMany(t *testing.T) {
	model := &ModelDef{
		Table: "tasks",
		Columns: map[string]ColumnDef{
			"id": {Type: "uuid", PrimaryKey: true},
		},
		Relations: map[string]RelDef{
			"tags": {Type: "manyToMany", Table: "tags", Junction: "task_tags", LocalKey: "task_id", ForeignKey: "tag_id"},
		},
	}

	up, _ := createTableSQL(model)

	if !strings.Contains(up, `CREATE TABLE IF NOT EXISTS "task_tags"`) {
		t.Errorf("expected quoted junction table in up SQL, got:\n%s", up)
	}
}

func TestGenerateMigration_Integration(t *testing.T) {
	dir := t.TempDir()
	modelsDir := filepath.Join(dir, "models")
	migrationsDir := filepath.Join(dir, "migrations")
	if err := os.MkdirAll(modelsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	model := ModelDef{
		Table: "users",
		Columns: map[string]ColumnDef{
			"id":    {Type: "uuid", PrimaryKey: true, Default: "gen_random_uuid()"},
			"email": {Type: "text", NotNull: true},
		},
		Timestamps: true,
	}

	data, _ := json.MarshalIndent(model, "", "  ")
	if err := os.WriteFile(filepath.Join(modelsDir, "users.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}

	up, down, err := GenerateMigration(modelsDir, migrationsDir)
	if err != nil {
		t.Fatal(err)
	}
	if up == "" {
		t.Fatal("expected non-empty up SQL")
	}
	if down == "" {
		t.Fatal("expected non-empty down SQL")
	}
	if !strings.Contains(up, `CREATE TABLE "users"`) {
		t.Error("expected CREATE TABLE \"users\" in up SQL")
	}

	// Save snapshot and regenerate — should be empty
	if err := SaveSnapshot(modelsDir); err != nil {
		t.Fatal(err)
	}

	up2, _, err := GenerateMigration(modelsDir, migrationsDir)
	if err != nil {
		t.Fatal(err)
	}
	if up2 != "" {
		t.Errorf("expected empty up SQL after snapshot, got: %s", up2)
	}
}

func TestTopoSortCreates(t *testing.T) {
	creates := []Change{
		{Type: "create_table", Table: "tasks", Model: &ModelDef{
			Table: "tasks",
			Relations: map[string]RelDef{
				"workspace": {Type: "belongsTo", Table: "workspaces"},
			},
		}},
		{Type: "create_table", Table: "workspaces", Model: &ModelDef{Table: "workspaces"}},
	}

	sorted := topoSortCreates(creates)
	if len(sorted) != 2 {
		t.Fatalf("expected 2, got %d", len(sorted))
	}
	// workspaces should come before tasks
	if sorted[0].Table != "workspaces" {
		t.Errorf("expected workspaces first, got %s", sorted[0].Table)
	}
	if sorted[1].Table != "tasks" {
		t.Errorf("expected tasks second, got %s", sorted[1].Table)
	}
}
