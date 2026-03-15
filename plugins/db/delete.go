package db

import (
	"context"
	"fmt"

	"github.com/chimpanze/noda/internal/plugin"
	"github.com/chimpanze/noda/pkg/api"
	"gorm.io/gorm"
)

type deleteDescriptor struct{}

func (d *deleteDescriptor) Name() string        { return "delete" }
func (d *deleteDescriptor) Description() string { return "Deletes rows matching a condition" }
func (d *deleteDescriptor) ServiceDeps() map[string]api.ServiceDep {
	return map[string]api.ServiceDep{
		"database": {Prefix: "db", Required: true},
	}
}
func (d *deleteDescriptor) ConfigSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"table": map[string]any{"type": "string", "description": "Table name"},
			"where": map[string]any{"type": "object", "description": "Equality conditions for row matching"},
		},
		"required": []any{"table", "where"},
	}
}
func (d *deleteDescriptor) OutputDescriptions() map[string]string {
	return map[string]string{
		"success": "Object with rows_affected count",
		"error":   "Database error details",
	}
}

type deleteExecutor struct{}

func newDeleteExecutor(_ map[string]any) api.NodeExecutor {
	return &deleteExecutor{}
}

func (e *deleteExecutor) Outputs() []string { return api.DefaultOutputs() }

func (e *deleteExecutor) Execute(ctx context.Context, nCtx api.ExecutionContext, config map[string]any, services map[string]any) (string, any, error) {
	db, err := plugin.GetService[*gorm.DB](services, "database")
	if err != nil {
		return "", nil, err
	}

	table, err := plugin.ResolveString(nCtx, config, "table")
	if err != nil {
		return "", nil, fmt.Errorf("db.delete: %w", err)
	}

	if err := ValidateIdentifier(table); err != nil {
		return "", nil, fmt.Errorf("db.delete: %w", err)
	}

	where, err := plugin.ResolveMap(nCtx, config, "where")
	if err != nil {
		return "", nil, fmt.Errorf("db.delete: %w", err)
	}

	tx := db.WithContext(ctx).Table(table).Where(where).Delete(nil)
	if tx.Error != nil {
		return "", nil, fmt.Errorf("db.delete: %w", tx.Error)
	}

	return api.OutputSuccess, map[string]any{
		"rows_affected": tx.RowsAffected,
	}, nil
}
