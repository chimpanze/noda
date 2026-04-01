package generate

import (
	"fmt"

	json "github.com/goccy/go-json"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// validOnDeleteActions lists allowed ON DELETE actions for foreign keys.
var validOnDeleteActions = map[string]bool{
	"CASCADE":     true,
	"SET NULL":    true,
	"RESTRICT":    true,
	"NO ACTION":   true,
	"SET DEFAULT": true,
}

// validateOnDelete checks that an ON DELETE value is a known SQL action.
func validateOnDelete(value string) error {
	if !validOnDeleteActions[strings.ToUpper(value)] {
		return fmt.Errorf("invalid ON DELETE action %q", value)
	}
	return nil
}

// validateDefault checks that a column default value does not contain SQL injection patterns.
// Allowed: numeric literals, quoted strings, SQL keywords (TRUE/FALSE/NULL), function calls like NOW().
func validateDefault(value string) error {
	v := strings.TrimSpace(value)
	if v == "" {
		return nil
	}
	// Reject semicolons which could terminate a statement
	if strings.Contains(v, ";") {
		return fmt.Errorf("invalid default value %q: contains semicolon", value)
	}
	// Reject comment markers
	if strings.Contains(v, "--") || strings.Contains(v, "/*") {
		return fmt.Errorf("invalid default value %q: contains SQL comment", value)
	}
	return nil
}

// quoteIdent quotes a SQL identifier to prevent injection.
func quoteIdent(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

// quoteIdentList quotes each identifier in a list.
func quoteIdentList(names []string) []string {
	quoted := make([]string, len(names))
	for i, n := range names {
		quoted[i] = quoteIdent(n)
	}
	return quoted
}

// ModelDef represents a parsed model definition.
type ModelDef struct {
	Table      string               `json:"table"`
	Columns    map[string]ColumnDef `json:"columns"`
	Relations  map[string]RelDef    `json:"relations"`
	Indexes    []IndexDef           `json:"indexes"`
	Timestamps bool                 `json:"timestamps"`
	SoftDelete bool                 `json:"soft_delete"`
}

// ColumnDef represents a column definition.
type ColumnDef struct {
	Type       string   `json:"type"`
	PrimaryKey bool     `json:"primary_key"`
	NotNull    bool     `json:"not_null"`
	Default    string   `json:"default"`
	Enum       []string `json:"enum"`
	MaxLength  int      `json:"max_length"`
	Precision  int      `json:"precision"`
	Scale      int      `json:"scale"`
}

// RelDef represents a relation definition.
type RelDef struct {
	Type       string `json:"type"`
	Table      string `json:"table"`
	ForeignKey string `json:"foreign_key"`
	OnDelete   string `json:"on_delete"`
	Junction   string `json:"junction"`
	LocalKey   string `json:"local_key"`
}

// IndexDef represents an index definition.
type IndexDef struct {
	Columns []string `json:"columns"`
	Unique  bool     `json:"unique"`
}

// Change represents a schema change between snapshots.
type Change struct {
	Type     string // "create_table", "drop_table", "add_column", "drop_column", "alter_column", "add_index", "drop_index", "add_fk", "drop_fk"
	Table    string
	Column   string
	OldCol   *ColumnDef
	NewCol   *ColumnDef
	Index    *IndexDef
	Relation *RelDef
	RelName  string
	Model    *ModelDef
}

// GenerateMigration compares current models against a snapshot and produces SQL.
// dialect should be "postgres" or "sqlite". An empty string defaults to "postgres".
func GenerateMigration(modelsDir, dialect string) (upSQL, downSQL string, err error) {
	if dialect == "" {
		dialect = "postgres"
	}

	current, err := loadModels(modelsDir)
	if err != nil {
		return "", "", fmt.Errorf("loading models: %w", err)
	}

	if len(current) == 0 {
		return "", "", fmt.Errorf("no model files found in %s", modelsDir)
	}

	old := loadSnapshot(modelsDir)

	changes := diffModels(old, current)
	if len(changes) == 0 {
		return "", "", nil
	}

	up, down := changesToSQL(changes, current, dialect)
	return up, down, nil
}

// SaveSnapshot writes the current model state as the snapshot.
func SaveSnapshot(modelsDir string) error {
	current, err := loadModels(modelsDir)
	if err != nil {
		return err
	}
	return saveSnapshot(modelsDir, current)
}

func loadModels(modelsDir string) (map[string]ModelDef, error) {
	models := make(map[string]ModelDef)

	entries, err := os.ReadDir(modelsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return models, nil
		}
		return nil, err
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") || entry.Name() == ".snapshot.json" {
			continue
		}

		data, err := os.ReadFile(filepath.Join(modelsDir, entry.Name()))
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", entry.Name(), err)
		}

		var model ModelDef
		if err := json.Unmarshal(data, &model); err != nil {
			return nil, fmt.Errorf("parsing %s: %w", entry.Name(), err)
		}

		if model.Table == "" {
			model.Table = strings.TrimSuffix(entry.Name(), ".json")
		}

		// Validate column Default values to prevent SQL injection
		for colName, col := range model.Columns {
			if col.Default != "" {
				if err := validateDefault(col.Default); err != nil {
					return nil, fmt.Errorf("%s: column %q: %w", entry.Name(), colName, err)
				}
			}
		}

		// Validate relation OnDelete values to prevent SQL injection
		for relName, rel := range model.Relations {
			if rel.OnDelete != "" {
				if err := validateOnDelete(rel.OnDelete); err != nil {
					return nil, fmt.Errorf("%s: relation %q: %w", entry.Name(), relName, err)
				}
				// Normalize to uppercase
				rel.OnDelete = strings.ToUpper(rel.OnDelete)
				model.Relations[relName] = rel
			}
		}

		models[model.Table] = model
	}

	return models, nil
}

func loadSnapshot(modelsDir string) map[string]ModelDef {
	data, err := os.ReadFile(filepath.Join(modelsDir, ".snapshot.json"))
	if err != nil {
		return make(map[string]ModelDef)
	}

	var snapshot map[string]ModelDef
	if err := json.Unmarshal(data, &snapshot); err != nil {
		return make(map[string]ModelDef)
	}
	return snapshot
}

func saveSnapshot(modelsDir string, models map[string]ModelDef) error {
	data, err := json.MarshalIndent(models, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(modelsDir, ".snapshot.json"), data, 0o600) //nolint:gosec // snapshot file, not world-readable
}

func diffModels(old, new map[string]ModelDef) []Change {
	var changes []Change

	// New tables
	for table, model := range new {
		if _, exists := old[table]; !exists {
			m := model
			changes = append(changes, Change{Type: "create_table", Table: table, Model: &m})
		}
	}

	// Dropped tables
	for table, model := range old {
		if _, exists := new[table]; !exists {
			m := model
			changes = append(changes, Change{Type: "drop_table", Table: table, Model: &m})
		}
	}

	// Altered tables
	for table, newModel := range new {
		oldModel, exists := old[table]
		if !exists {
			continue
		}

		// New columns
		for col, def := range newModel.Columns {
			if _, exists := oldModel.Columns[col]; !exists {
				d := def
				changes = append(changes, Change{Type: "add_column", Table: table, Column: col, NewCol: &d})
			}
		}

		// Dropped columns
		for col, def := range oldModel.Columns {
			if _, exists := newModel.Columns[col]; !exists {
				d := def
				changes = append(changes, Change{Type: "drop_column", Table: table, Column: col, OldCol: &d})
			}
		}

		// Altered columns
		for col, newDef := range newModel.Columns {
			oldDef, exists := oldModel.Columns[col]
			if !exists {
				continue
			}
			if columnChanged(oldDef, newDef) {
				o, n := oldDef, newDef
				changes = append(changes, Change{Type: "alter_column", Table: table, Column: col, OldCol: &o, NewCol: &n})
			}
		}

		// Timestamps changes
		if newModel.Timestamps && !oldModel.Timestamps {
			changes = append(changes, Change{Type: "add_column", Table: table, Column: "created_at", NewCol: &ColumnDef{Type: "timestamp", NotNull: true, Default: "NOW()"}})
			changes = append(changes, Change{Type: "add_column", Table: table, Column: "updated_at", NewCol: &ColumnDef{Type: "timestamp", NotNull: true, Default: "NOW()"}})
		}
		if !newModel.Timestamps && oldModel.Timestamps {
			changes = append(changes, Change{Type: "drop_column", Table: table, Column: "created_at"})
			changes = append(changes, Change{Type: "drop_column", Table: table, Column: "updated_at"})
		}

		// Soft delete changes
		if newModel.SoftDelete && !oldModel.SoftDelete {
			changes = append(changes, Change{Type: "add_column", Table: table, Column: "deleted_at", NewCol: &ColumnDef{Type: "timestamp"}})
		}
		if !newModel.SoftDelete && oldModel.SoftDelete {
			changes = append(changes, Change{Type: "drop_column", Table: table, Column: "deleted_at"})
		}

		// Index changes
		oldIndexSet := indexSet(oldModel.Indexes)
		newIndexSet := indexSet(newModel.Indexes)

		for key, idx := range newIndexSet {
			if _, exists := oldIndexSet[key]; !exists {
				i := idx
				changes = append(changes, Change{Type: "add_index", Table: table, Index: &i})
			}
		}
		for key, idx := range oldIndexSet {
			if _, exists := newIndexSet[key]; !exists {
				i := idx
				changes = append(changes, Change{Type: "drop_index", Table: table, Index: &i})
			}
		}

		// Relation (FK) changes
		for name, rel := range newModel.Relations {
			if rel.Type != "belongsTo" {
				continue
			}
			oldRel, exists := oldModel.Relations[name]
			if !exists || oldRel.Type != "belongsTo" || oldRel.Table != rel.Table || oldRel.ForeignKey != rel.ForeignKey || oldRel.OnDelete != rel.OnDelete {
				r := rel
				changes = append(changes, Change{Type: "add_fk", Table: table, RelName: name, Relation: &r})
			}
		}
		for name, rel := range oldModel.Relations {
			if rel.Type != "belongsTo" {
				continue
			}
			if _, exists := newModel.Relations[name]; !exists {
				r := rel
				changes = append(changes, Change{Type: "drop_fk", Table: table, RelName: name, Relation: &r})
			}
		}
	}

	return changes
}

func columnChanged(old, new ColumnDef) bool {
	return old.Type != new.Type ||
		old.NotNull != new.NotNull ||
		old.Default != new.Default ||
		old.PrimaryKey != new.PrimaryKey ||
		old.MaxLength != new.MaxLength ||
		old.Precision != new.Precision ||
		old.Scale != new.Scale ||
		!enumEqual(old.Enum, new.Enum)
}

func enumEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func indexSet(indexes []IndexDef) map[string]IndexDef {
	set := make(map[string]IndexDef)
	for _, idx := range indexes {
		key := strings.Join(idx.Columns, ",")
		if idx.Unique {
			key += ":unique"
		}
		set[key] = idx
	}
	return set
}

// pgType maps model column types to PostgreSQL types.
func pgType(col ColumnDef) string {
	return sqlType(col, "postgres")
}

// sqlType maps model column types to SQL types for the given dialect.
func sqlType(col ColumnDef, dialect string) string {
	if dialect == "sqlite" {
		return sqliteType(col)
	}
	return postgresType(col)
}

func postgresType(col ColumnDef) string {
	switch col.Type {
	case "uuid":
		return "UUID"
	case "text":
		return "TEXT"
	case "varchar":
		if col.MaxLength > 0 {
			return fmt.Sprintf("VARCHAR(%d)", col.MaxLength)
		}
		return "VARCHAR(255)"
	case "integer", "int":
		return "INTEGER"
	case "bigint":
		return "BIGINT"
	case "boolean", "bool":
		return "BOOLEAN"
	case "decimal":
		p, s := col.Precision, col.Scale
		if p == 0 {
			p = 10
		}
		if s == 0 {
			s = 2
		}
		return fmt.Sprintf("NUMERIC(%d,%d)", p, s)
	case "timestamp":
		return "TIMESTAMPTZ"
	case "json", "jsonb":
		return "JSONB"
	case "serial":
		return "SERIAL"
	default:
		return strings.ToUpper(col.Type)
	}
}

func sqliteType(col ColumnDef) string {
	switch col.Type {
	case "uuid":
		return "TEXT"
	case "text":
		return "TEXT"
	case "varchar":
		return "TEXT"
	case "integer", "int":
		return "INTEGER"
	case "bigint":
		return "INTEGER"
	case "boolean", "bool":
		return "INTEGER"
	case "decimal":
		return "REAL"
	case "timestamp":
		return "TEXT"
	case "json", "jsonb":
		return "TEXT"
	case "serial":
		return "INTEGER"
	default:
		return "TEXT"
	}
}

// defaultNow returns the dialect-specific default value for the current timestamp.
func defaultNow(dialect string) string {
	if dialect == "sqlite" {
		return "(datetime('now'))"
	}
	return "NOW()"
}

// timestampType returns the dialect-specific timestamp column type.
func timestampType(dialect string) string {
	if dialect == "sqlite" {
		return "TEXT"
	}
	return "TIMESTAMPTZ"
}

// fkColumnType returns the dialect-specific type for a foreign key column (used in junction tables).
func fkColumnType(dialect string) string {
	if dialect == "sqlite" {
		return "TEXT"
	}
	return "UUID"
}

func changesToSQL(changes []Change, allModels map[string]ModelDef, dialect string) (string, string) {
	var upParts, downParts []string

	// Sort changes: create_table first, drop_table last, FK constraints after tables
	sort.SliceStable(changes, func(i, j int) bool {
		order := map[string]int{
			"create_table": 0,
			"add_column":   1,
			"alter_column": 2,
			"add_index":    3,
			"add_fk":       4,
			"drop_fk":      5,
			"drop_index":   6,
			"drop_column":  7,
			"drop_table":   8,
		}
		return order[changes[i].Type] < order[changes[j].Type]
	})

	// Topological sort for create_table by FK dependencies
	var createChanges []Change
	var otherChanges []Change
	for _, ch := range changes {
		if ch.Type == "create_table" {
			createChanges = append(createChanges, ch)
		} else {
			otherChanges = append(otherChanges, ch)
		}
	}
	createChanges = topoSortCreates(createChanges)
	changes = append(createChanges, otherChanges...)

	for _, ch := range changes {
		switch ch.Type {
		case "create_table":
			up, down := createTableSQL(ch.Model, dialect)
			upParts = append(upParts, up)
			downParts = append([]string{down}, downParts...)

		case "drop_table":
			if dialect == "sqlite" {
				upParts = append(upParts, fmt.Sprintf("DROP TABLE IF EXISTS %s;", quoteIdent(ch.Table)))
			} else {
				upParts = append(upParts, fmt.Sprintf("DROP TABLE IF EXISTS %s CASCADE;", quoteIdent(ch.Table)))
			}
			if ch.Model != nil {
				recreate, _ := createTableSQL(ch.Model, dialect)
				downParts = append(downParts, recreate)
			}

		case "add_column":
			colSQL := fmt.Sprintf("%s %s", quoteIdent(ch.Column), sqlType(*ch.NewCol, dialect))
			if ch.NewCol.NotNull {
				colSQL += " NOT NULL"
			}
			if ch.NewCol.Default != "" {
				def := ch.NewCol.Default
				if dialect == "sqlite" && strings.EqualFold(def, "NOW()") {
					def = defaultNow("sqlite")
				}
				colSQL += " DEFAULT " + def
			}
			upParts = append(upParts, fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s;", quoteIdent(ch.Table), colSQL))
			downParts = append([]string{fmt.Sprintf("ALTER TABLE %s DROP COLUMN %s;", quoteIdent(ch.Table), quoteIdent(ch.Column))}, downParts...)

		case "drop_column":
			upParts = append(upParts, fmt.Sprintf("ALTER TABLE %s DROP COLUMN %s;", quoteIdent(ch.Table), quoteIdent(ch.Column)))
			if ch.OldCol != nil {
				colSQL := fmt.Sprintf("%s %s", quoteIdent(ch.Column), sqlType(*ch.OldCol, dialect))
				if ch.OldCol.NotNull {
					colSQL += " NOT NULL"
				}
				if ch.OldCol.Default != "" {
					colSQL += " DEFAULT " + ch.OldCol.Default
				}
				downParts = append(downParts, fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s;", quoteIdent(ch.Table), colSQL))
			}

		case "alter_column":
			if dialect == "sqlite" {
				upParts = append(upParts, fmt.Sprintf("-- ALTER COLUMN not supported in SQLite; recreate table %s manually", quoteIdent(ch.Table)))
				downParts = append([]string{fmt.Sprintf("-- ALTER COLUMN not supported in SQLite; recreate table %s manually", quoteIdent(ch.Table))}, downParts...)
			} else {
				if ch.OldCol.Type != ch.NewCol.Type {
					upParts = append(upParts, fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s TYPE %s;", quoteIdent(ch.Table), quoteIdent(ch.Column), sqlType(*ch.NewCol, dialect)))
					downParts = append([]string{fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s TYPE %s;", quoteIdent(ch.Table), quoteIdent(ch.Column), sqlType(*ch.OldCol, dialect))}, downParts...)
				}
				if ch.OldCol.NotNull != ch.NewCol.NotNull {
					if ch.NewCol.NotNull {
						upParts = append(upParts, fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s SET NOT NULL;", quoteIdent(ch.Table), quoteIdent(ch.Column)))
						downParts = append([]string{fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s DROP NOT NULL;", quoteIdent(ch.Table), quoteIdent(ch.Column))}, downParts...)
					} else {
						upParts = append(upParts, fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s DROP NOT NULL;", quoteIdent(ch.Table), quoteIdent(ch.Column)))
						downParts = append([]string{fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s SET NOT NULL;", quoteIdent(ch.Table), quoteIdent(ch.Column))}, downParts...)
					}
				}
				if ch.OldCol.Default != ch.NewCol.Default {
					if ch.NewCol.Default != "" {
						upParts = append(upParts, fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s SET DEFAULT %s;", quoteIdent(ch.Table), quoteIdent(ch.Column), ch.NewCol.Default))
					} else {
						upParts = append(upParts, fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s DROP DEFAULT;", quoteIdent(ch.Table), quoteIdent(ch.Column)))
					}
					if ch.OldCol.Default != "" {
						downParts = append([]string{fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s SET DEFAULT %s;", quoteIdent(ch.Table), quoteIdent(ch.Column), ch.OldCol.Default)}, downParts...)
					} else {
						downParts = append([]string{fmt.Sprintf("ALTER TABLE %s ALTER COLUMN %s DROP DEFAULT;", quoteIdent(ch.Table), quoteIdent(ch.Column))}, downParts...)
					}
				}
			}

		case "add_index":
			idxName := fmt.Sprintf("idx_%s_%s", ch.Table, strings.Join(ch.Index.Columns, "_"))
			unique := ""
			if ch.Index.Unique {
				unique = "UNIQUE "
			}
			upParts = append(upParts, fmt.Sprintf("CREATE %sINDEX %s ON %s (%s);", unique, quoteIdent(idxName), quoteIdent(ch.Table), strings.Join(quoteIdentList(ch.Index.Columns), ", ")))
			downParts = append([]string{fmt.Sprintf("DROP INDEX IF EXISTS %s;", quoteIdent(idxName))}, downParts...)

		case "drop_index":
			idxName := fmt.Sprintf("idx_%s_%s", ch.Table, strings.Join(ch.Index.Columns, "_"))
			upParts = append(upParts, fmt.Sprintf("DROP INDEX IF EXISTS %s;", quoteIdent(idxName)))
			unique := ""
			if ch.Index.Unique {
				unique = "UNIQUE "
			}
			downParts = append(downParts, fmt.Sprintf("CREATE %sINDEX %s ON %s (%s);", unique, quoteIdent(idxName), quoteIdent(ch.Table), strings.Join(quoteIdentList(ch.Index.Columns), ", ")))

		case "add_fk":
			if dialect == "sqlite" {
				// SQLite does not support ALTER TABLE ADD CONSTRAINT; FKs are inline in CREATE TABLE
				upParts = append(upParts, fmt.Sprintf("-- FK %s on %s: SQLite requires foreign keys inline in CREATE TABLE", ch.RelName, quoteIdent(ch.Table)))
			} else {
				fkName := fmt.Sprintf("fk_%s_%s", ch.Table, ch.RelName)
				onDelete := ""
				if ch.Relation.OnDelete != "" {
					onDelete = " ON DELETE " + ch.Relation.OnDelete
				}
				upParts = append(upParts, fmt.Sprintf("ALTER TABLE %s ADD CONSTRAINT %s FOREIGN KEY (%s) REFERENCES %s (%s)%s;",
					quoteIdent(ch.Table), quoteIdent(fkName), quoteIdent(ch.Relation.ForeignKey), quoteIdent(ch.Relation.Table), quoteIdent("id"), onDelete))
				downParts = append([]string{fmt.Sprintf("ALTER TABLE %s DROP CONSTRAINT IF EXISTS %s;", quoteIdent(ch.Table), quoteIdent(fkName))}, downParts...)
			}

		case "drop_fk":
			if dialect == "sqlite" {
				upParts = append(upParts, fmt.Sprintf("-- DROP FK %s on %s: SQLite requires table recreation", ch.RelName, quoteIdent(ch.Table)))
			} else {
				fkName := fmt.Sprintf("fk_%s_%s", ch.Table, ch.RelName)
				upParts = append(upParts, fmt.Sprintf("ALTER TABLE %s DROP CONSTRAINT IF EXISTS %s;", quoteIdent(ch.Table), quoteIdent(fkName)))
				onDelete := ""
				if ch.Relation.OnDelete != "" {
					onDelete = " ON DELETE " + ch.Relation.OnDelete
				}
				downParts = append(downParts, fmt.Sprintf("ALTER TABLE %s ADD CONSTRAINT %s FOREIGN KEY (%s) REFERENCES %s (%s)%s;",
					quoteIdent(ch.Table), quoteIdent(fkName), quoteIdent(ch.Relation.ForeignKey), quoteIdent(ch.Relation.Table), quoteIdent("id"), onDelete))
			}
		}
	}

	return strings.Join(upParts, "\n"), strings.Join(downParts, "\n")
}

func createTableSQL(model *ModelDef, dialect string) (string, string) {
	var lines []string
	var pkCols []string

	// Build a map of FK columns for inline REFERENCES (SQLite)
	fkRefs := make(map[string]RelDef)
	if dialect == "sqlite" {
		for _, rel := range model.Relations {
			if rel.Type == "belongsTo" {
				fkRefs[rel.ForeignKey] = rel
			}
		}
	}

	// Sort columns for deterministic output
	colNames := make([]string, 0, len(model.Columns))
	for name := range model.Columns {
		colNames = append(colNames, name)
	}
	sort.Strings(colNames)

	for _, name := range colNames {
		col := model.Columns[name]
		line := fmt.Sprintf("  %s %s", quoteIdent(name), sqlType(col, dialect))
		if col.PrimaryKey {
			pkCols = append(pkCols, name)
		}
		if col.NotNull || col.PrimaryKey {
			line += " NOT NULL"
		}
		if col.Default != "" {
			line += " DEFAULT " + col.Default
		}
		// SQLite: inline FK references
		if rel, ok := fkRefs[name]; ok {
			onDelete := ""
			if rel.OnDelete != "" {
				onDelete = " ON DELETE " + rel.OnDelete
			}
			line += fmt.Sprintf(" REFERENCES %s (%s)%s", quoteIdent(rel.Table), quoteIdent("id"), onDelete)
		}
		lines = append(lines, line)
	}

	// Timestamps
	if model.Timestamps {
		tsType := timestampType(dialect)
		tsDefault := defaultNow(dialect)
		lines = append(lines, fmt.Sprintf("  %s %s NOT NULL DEFAULT %s", quoteIdent("created_at"), tsType, tsDefault))
		lines = append(lines, fmt.Sprintf("  %s %s NOT NULL DEFAULT %s", quoteIdent("updated_at"), tsType, tsDefault))
	}

	// Soft delete
	if model.SoftDelete {
		lines = append(lines, fmt.Sprintf("  %s %s", quoteIdent("deleted_at"), timestampType(dialect)))
	}

	// Primary key
	if len(pkCols) > 0 {
		lines = append(lines, fmt.Sprintf("  PRIMARY KEY (%s)", strings.Join(quoteIdentList(pkCols), ", ")))
	}

	up := fmt.Sprintf("CREATE TABLE %s (\n%s\n);", quoteIdent(model.Table), strings.Join(lines, ",\n"))

	// Indexes
	for _, idx := range model.Indexes {
		idxName := fmt.Sprintf("idx_%s_%s", model.Table, strings.Join(idx.Columns, "_"))
		unique := ""
		if idx.Unique {
			unique = "UNIQUE "
		}
		up += fmt.Sprintf("\nCREATE %sINDEX %s ON %s (%s);", unique, quoteIdent(idxName), quoteIdent(model.Table), strings.Join(quoteIdentList(idx.Columns), ", "))
	}

	// FK constraints from belongsTo relations (PostgreSQL only — SQLite uses inline REFERENCES above)
	if dialect != "sqlite" {
		relNames := make([]string, 0, len(model.Relations))
		for name := range model.Relations {
			relNames = append(relNames, name)
		}
		sort.Strings(relNames)

		for _, name := range relNames {
			rel := model.Relations[name]
			if rel.Type != "belongsTo" {
				continue
			}
			fkName := fmt.Sprintf("fk_%s_%s", model.Table, name)
			onDelete := ""
			if rel.OnDelete != "" {
				onDelete = " ON DELETE " + rel.OnDelete
			}
			up += fmt.Sprintf("\nALTER TABLE %s ADD CONSTRAINT %s FOREIGN KEY (%s) REFERENCES %s (%s)%s;",
				quoteIdent(model.Table), quoteIdent(fkName), quoteIdent(rel.ForeignKey), quoteIdent(rel.Table), quoteIdent("id"), onDelete)
		}
	}

	// Junction tables for manyToMany
	relNames := make([]string, 0, len(model.Relations))
	for name := range model.Relations {
		relNames = append(relNames, name)
	}
	sort.Strings(relNames)

	for _, name := range relNames {
		rel := model.Relations[name]
		if rel.Type != "manyToMany" || rel.Junction == "" {
			continue
		}
		fkType := fkColumnType(dialect)
		up += fmt.Sprintf("\nCREATE TABLE IF NOT EXISTS %s (\n  %s %s NOT NULL REFERENCES %s (%s) ON DELETE CASCADE,\n  %s %s NOT NULL REFERENCES %s (%s) ON DELETE CASCADE,\n  PRIMARY KEY (%s, %s)\n);",
			quoteIdent(rel.Junction), quoteIdent(rel.LocalKey), fkType, quoteIdent(model.Table), quoteIdent("id"),
			quoteIdent(rel.ForeignKey), fkType, quoteIdent(rel.Table), quoteIdent("id"),
			quoteIdent(rel.LocalKey), quoteIdent(rel.ForeignKey))
	}

	var down string
	if dialect == "sqlite" {
		down = fmt.Sprintf("DROP TABLE IF EXISTS %s;", quoteIdent(model.Table))
	} else {
		down = fmt.Sprintf("DROP TABLE IF EXISTS %s CASCADE;", quoteIdent(model.Table))
	}

	return up, down
}

func topoSortCreates(creates []Change) []Change {
	if len(creates) <= 1 {
		return creates
	}

	// Build dependency graph
	tableModel := make(map[string]*Change)
	for i := range creates {
		tableModel[creates[i].Table] = &creates[i]
	}

	deps := make(map[string][]string) // table -> tables it depends on
	for _, ch := range creates {
		if ch.Model == nil {
			continue
		}
		for _, rel := range ch.Model.Relations {
			if rel.Type == "belongsTo" {
				deps[ch.Table] = append(deps[ch.Table], rel.Table)
			}
		}
	}

	// DFS topological sort with cycle detection
	var sorted []Change
	visited := make(map[string]bool)
	visiting := make(map[string]bool)

	var visit func(table string)
	visit = func(table string) {
		if visited[table] {
			return
		}
		if visiting[table] {
			return // cycle, skip
		}
		visiting[table] = true
		for _, dep := range deps[table] {
			if tableModel[dep] != nil {
				visit(dep)
			}
		}
		visiting[table] = false
		visited[table] = true
		if ch := tableModel[table]; ch != nil {
			sorted = append(sorted, *ch)
		}
	}

	for _, ch := range creates {
		visit(ch.Table)
	}

	return sorted
}
