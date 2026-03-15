package db

import (
	"context"
	"fmt"

	"github.com/chimpanze/noda/internal/plugin"
	"github.com/chimpanze/noda/pkg/api"
	"gorm.io/gorm"
)

type findOneDescriptor struct{}

func (d *findOneDescriptor) Name() string        { return "findOne" }
func (d *findOneDescriptor) Description() string { return "Single row SELECT returning one row object" }
func (d *findOneDescriptor) ServiceDeps() map[string]api.ServiceDep {
	return map[string]api.ServiceDep{
		"database": {Prefix: "db", Required: true},
	}
}
func (d *findOneDescriptor) ConfigSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"table":        map[string]any{"type": "string", "description": "Table name"},
			"select":       map[string]any{"type": "array", "description": "Column names to select (default: all)"},
			"where":        map[string]any{"type": "object", "description": "Equality conditions as key-value pairs"},
			"where_clause": map[string]any{"type": "object", "description": "Raw WHERE with query and params"},
			"joins":        map[string]any{"type": "array", "description": "JOIN clauses"},
			"order":        map[string]any{"type": "string", "description": "ORDER BY clause"},
			"required":     map[string]any{"type": "boolean", "description": "Return NotFoundError when no row matches (default: true)"},
		},
		"required": []any{"table"},
	}
}
func (d *findOneDescriptor) OutputDescriptions() map[string]string {
	return map[string]string{
		"success": "Single row object or nil if not found",
		"error":   "Database error details",
	}
}

type findOneExecutor struct{}

func newFindOneExecutor(_ map[string]any) api.NodeExecutor {
	return &findOneExecutor{}
}

func (e *findOneExecutor) Outputs() []string { return api.DefaultOutputs() }

func (e *findOneExecutor) Execute(ctx context.Context, nCtx api.ExecutionContext, config map[string]any, services map[string]any) (string, any, error) {
	db, err := plugin.GetService[*gorm.DB](services, "database")
	if err != nil {
		return "", nil, err
	}

	table, err := plugin.ResolveString(nCtx, config, "table")
	if err != nil {
		return "", nil, fmt.Errorf("db.findOne: %w", err)
	}

	if err := ValidateIdentifier(table); err != nil {
		return "", nil, fmt.Errorf("db.findOne: %w", err)
	}

	// Default required to true
	required := true
	if v, ok := config["required"]; ok {
		if b, ok := v.(bool); ok {
			required = b
		}
	}

	tx := db.WithContext(ctx).Table(table)

	tx, err = applyWhere(tx, nCtx, config)
	if err != nil {
		return "", nil, fmt.Errorf("db.findOne: %w", err)
	}

	tx, err = applyQueryOptions(tx, nCtx, config)
	if err != nil {
		return "", nil, fmt.Errorf("db.findOne: %w", err)
	}

	tx = tx.Limit(1)

	var results []map[string]any
	tx = tx.Scan(&results)
	if tx.Error != nil {
		return "", nil, fmt.Errorf("db.findOne: %w", tx.Error)
	}

	if len(results) == 0 {
		if required {
			return "", nil, &api.NotFoundError{Resource: table}
		}
		return api.OutputSuccess, nil, nil
	}

	return api.OutputSuccess, results[0], nil
}
