package db

import (
	"context"
	"fmt"

	"github.com/chimpanze/noda/internal/plugin"
	"github.com/chimpanze/noda/pkg/api"
	"gorm.io/gorm"
)

type countDescriptor struct{}

func (d *countDescriptor) Name() string        { return "count" }
func (d *countDescriptor) Description() string { return "Counts rows matching conditions" }
func (d *countDescriptor) ServiceDeps() map[string]api.ServiceDep {
	return map[string]api.ServiceDep{
		"database": {Prefix: "db", Required: true},
	}
}
func (d *countDescriptor) ConfigSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"table":        map[string]any{"type": "string", "description": "Table name"},
			"where":        map[string]any{"type": "object", "description": "Equality conditions as key-value pairs"},
			"where_clause": map[string]any{"type": "object", "description": "Raw WHERE with query and params"},
			"joins":        map[string]any{"type": "array", "description": "JOIN clauses"},
		},
		"required": []any{"table"},
	}
}

type countExecutor struct{}

func newCountExecutor(_ map[string]any) api.NodeExecutor {
	return &countExecutor{}
}

func (e *countExecutor) Outputs() []string { return api.DefaultOutputs() }

func (e *countExecutor) Execute(ctx context.Context, nCtx api.ExecutionContext, config map[string]any, services map[string]any) (string, any, error) {
	db, err := plugin.GetService[*gorm.DB](services, "database")
	if err != nil {
		return "", nil, err
	}

	table, err := plugin.ResolveString(nCtx, config, "table")
	if err != nil {
		return "", nil, fmt.Errorf("db.count: %w", err)
	}

	if err := ValidateIdentifier(table); err != nil {
		return "", nil, fmt.Errorf("db.count: %w", err)
	}

	tx := db.WithContext(ctx).Table(table)

	tx, err = applyWhere(tx, nCtx, config)
	if err != nil {
		return "", nil, fmt.Errorf("db.count: %w", err)
	}

	// Apply joins if present
	joins, err := resolveJoins(nCtx, config)
	if err != nil {
		return "", nil, fmt.Errorf("db.count: %w", err)
	}
	for _, j := range joins {
		if len(j.Params) > 0 {
			tx = tx.Joins(j.Query, j.Params...)
		} else {
			tx = tx.Joins(j.Query)
		}
	}

	var count int64
	tx = tx.Count(&count)
	if tx.Error != nil {
		return "", nil, fmt.Errorf("db.count: %w", tx.Error)
	}

	return api.OutputSuccess, map[string]any{
		"count": count,
	}, nil
}
