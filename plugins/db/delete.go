package db

import (
	"context"
	"fmt"

	"github.com/chimpanze/noda/pkg/api"
)

type deleteDescriptor struct{}

func (d *deleteDescriptor) Name() string { return "delete" }
func (d *deleteDescriptor) ServiceDeps() map[string]api.ServiceDep {
	return map[string]api.ServiceDep{
		"database": {Prefix: "db", Required: true},
	}
}
func (d *deleteDescriptor) ConfigSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"table":     map[string]any{"type": "string"},
			"condition": map[string]any{"type": "string"},
			"params":    map[string]any{"type": "array"},
		},
		"required": []any{"table", "condition"},
	}
}

type deleteExecutor struct{}

func newDeleteExecutor(_ map[string]any) api.NodeExecutor {
	return &deleteExecutor{}
}

func (e *deleteExecutor) Outputs() []string { return []string{"success", "error"} }

func (e *deleteExecutor) Execute(ctx context.Context, nCtx api.ExecutionContext, config map[string]any, services map[string]any) (string, any, error) {
	db, err := getDB(services)
	if err != nil {
		return "", nil, err
	}

	table, err := resolveString(nCtx, config, "table")
	if err != nil {
		return "", nil, fmt.Errorf("db.delete: %w", err)
	}

	condition, err := resolveString(nCtx, config, "condition")
	if err != nil {
		return "", nil, fmt.Errorf("db.delete: %w", err)
	}

	params, err := resolveParams(nCtx, config)
	if err != nil {
		return "", nil, fmt.Errorf("db.delete: %w", err)
	}

	tx := db.WithContext(ctx).Table(table).Where(condition, params...).Delete(nil)
	if tx.Error != nil {
		return "", nil, fmt.Errorf("db.delete: %w", tx.Error)
	}

	return "success", map[string]any{
		"rows_affected": tx.RowsAffected,
	}, nil
}
