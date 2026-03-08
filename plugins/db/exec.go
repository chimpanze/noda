package db

import (
	"context"
	"fmt"

	"github.com/chimpanze/noda/pkg/api"
)

type execDescriptor struct{}

func (d *execDescriptor) Name() string { return "exec" }
func (d *execDescriptor) ServiceDeps() map[string]api.ServiceDep {
	return map[string]api.ServiceDep{
		"database": {Prefix: "db", Required: true},
	}
}
func (d *execDescriptor) ConfigSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"query":  map[string]any{"type": "string"},
			"params": map[string]any{"type": "array"},
		},
		"required": []any{"query"},
	}
}

type execExecutor struct{}

func newExecExecutor(_ map[string]any) api.NodeExecutor {
	return &execExecutor{}
}

func (e *execExecutor) Outputs() []string { return []string{"success", "error"} }

func (e *execExecutor) Execute(ctx context.Context, nCtx api.ExecutionContext, config map[string]any, services map[string]any) (string, any, error) {
	db, err := getDB(services)
	if err != nil {
		return "", nil, err
	}

	query, err := resolveString(nCtx, config, "query")
	if err != nil {
		return "", nil, fmt.Errorf("db.exec: %w", err)
	}

	params, err := resolveParams(nCtx, config)
	if err != nil {
		return "", nil, fmt.Errorf("db.exec: %w", err)
	}

	tx := db.WithContext(ctx).Exec(query, params...)
	if tx.Error != nil {
		return "", nil, fmt.Errorf("db.exec: %w", tx.Error)
	}

	return "success", map[string]any{
		"rows_affected": tx.RowsAffected,
	}, nil
}
