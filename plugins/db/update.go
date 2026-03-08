package db

import (
	"context"
	"fmt"

	"github.com/chimpanze/noda/pkg/api"
)

type updateDescriptor struct{}

func (d *updateDescriptor) Name() string { return "update" }
func (d *updateDescriptor) ServiceDeps() map[string]api.ServiceDep {
	return map[string]api.ServiceDep{
		"database": {Prefix: "db", Required: true},
	}
}
func (d *updateDescriptor) ConfigSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"table":     map[string]any{"type": "string"},
			"data":      map[string]any{"type": "object"},
			"condition": map[string]any{"type": "string"},
			"params":    map[string]any{"type": "array"},
		},
		"required": []any{"table", "data", "condition"},
	}
}

type updateExecutor struct{}

func newUpdateExecutor(_ map[string]any) api.NodeExecutor {
	return &updateExecutor{}
}

func (e *updateExecutor) Outputs() []string { return []string{"success", "error"} }

func (e *updateExecutor) Execute(ctx context.Context, nCtx api.ExecutionContext, config map[string]any, services map[string]any) (string, any, error) {
	db, err := getDB(services)
	if err != nil {
		return "", nil, err
	}

	table, err := resolveString(nCtx, config, "table")
	if err != nil {
		return "", nil, fmt.Errorf("db.update: %w", err)
	}

	data, err := resolveMap(nCtx, config, "data")
	if err != nil {
		return "", nil, fmt.Errorf("db.update: %w", err)
	}

	condition, err := resolveString(nCtx, config, "condition")
	if err != nil {
		return "", nil, fmt.Errorf("db.update: %w", err)
	}

	params, err := resolveParams(nCtx, config)
	if err != nil {
		return "", nil, fmt.Errorf("db.update: %w", err)
	}

	tx := db.WithContext(ctx).Table(table).Where(condition, params...).Updates(data)
	if tx.Error != nil {
		return "", nil, fmt.Errorf("db.update: %w", tx.Error)
	}

	return "success", map[string]any{
		"rows_affected": tx.RowsAffected,
	}, nil
}
