package db

import (
	"context"
	"fmt"

	"github.com/chimpanze/noda/internal/plugin"
	"github.com/chimpanze/noda/pkg/api"
	"gorm.io/gorm"
)

type updateDescriptor struct{}

func (d *updateDescriptor) Name() string        { return "update" }
func (d *updateDescriptor) Description() string { return "Updates rows matching a condition" }
func (d *updateDescriptor) ServiceDeps() map[string]api.ServiceDep {
	return map[string]api.ServiceDep{
		"database": {Prefix: "db", Required: true},
	}
}
func (d *updateDescriptor) ConfigSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"table": map[string]any{"type": "string", "description": "Table name"},
			"data":  map[string]any{"type": "object", "description": "Fields to update"},
			"where": map[string]any{"type": "object", "description": "Equality conditions for row matching"},
		},
		"required": []any{"table", "data", "where"},
	}
}
func (d *updateDescriptor) OutputDescriptions() map[string]string {
	return map[string]string{
		"success": "Object with rows_affected count",
		"error":   "Database error details",
	}
}

type updateExecutor struct{}

func newUpdateExecutor(_ map[string]any) api.NodeExecutor {
	return &updateExecutor{}
}

func (e *updateExecutor) Outputs() []string { return api.DefaultOutputs() }

func (e *updateExecutor) Execute(ctx context.Context, nCtx api.ExecutionContext, config map[string]any, services map[string]any) (string, any, error) {
	db, err := plugin.GetService[*gorm.DB](services, "database")
	if err != nil {
		return "", nil, fmt.Errorf("db.update: %w", err)
	}

	table, err := plugin.ResolveString(nCtx, config, "table")
	if err != nil {
		return "", nil, fmt.Errorf("db.update: %w", err)
	}

	if err := ValidateIdentifier(table); err != nil {
		return "", nil, fmt.Errorf("db.update: %w", err)
	}

	data, err := plugin.ResolveMap(nCtx, config, "data")
	if err != nil {
		return "", nil, fmt.Errorf("db.update: %w", err)
	}

	where, err := plugin.ResolveMap(nCtx, config, "where")
	if err != nil {
		return "", nil, fmt.Errorf("db.update: %w", err)
	}

	tx := db.WithContext(ctx).Table(table).Where(where).Updates(data)
	if tx.Error != nil {
		return "", nil, fmt.Errorf("db.update: %w", tx.Error)
	}

	return api.OutputSuccess, map[string]any{
		"rows_affected": tx.RowsAffected,
	}, nil
}
