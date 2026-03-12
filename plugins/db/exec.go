package db

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/chimpanze/noda/internal/plugin"
	"github.com/chimpanze/noda/pkg/api"
	"gorm.io/gorm"
)

type execDescriptor struct{}

func (d *execDescriptor) Name() string { return "exec" }
func (d *execDescriptor) Description() string {
	return "Executes an INSERT, UPDATE, or DELETE statement"
}
func (d *execDescriptor) ServiceDeps() map[string]api.ServiceDep {
	return map[string]api.ServiceDep{
		"database": {Prefix: "db", Required: true},
	}
}
func (d *execDescriptor) ConfigSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"query":  map[string]any{"type": "string", "description": "SQL statement (INSERT, UPDATE, DELETE)"},
			"params": map[string]any{"type": "array", "description": "Positional query parameters"},
		},
		"required": []any{"query"},
	}
}

type execExecutor struct{}

func newExecExecutor(_ map[string]any) api.NodeExecutor {
	return &execExecutor{}
}

func (e *execExecutor) Outputs() []string { return api.DefaultOutputs() }

func (e *execExecutor) Execute(ctx context.Context, nCtx api.ExecutionContext, config map[string]any, services map[string]any) (string, any, error) {
	db, err := plugin.GetService[*gorm.DB](services, "database")
	if err != nil {
		return "", nil, err
	}

	query, err := plugin.ResolveString(nCtx, config, "query")
	if err != nil {
		return "", nil, fmt.Errorf("db.exec: %w", err)
	}

	params, err := plugin.ResolveOptionalArray(nCtx, config, "params")
	if err != nil {
		return "", nil, fmt.Errorf("db.exec: %w", err)
	}

	slog.Warn("executing raw SQL statement", "query_prefix", query[:min(len(query), 50)])

	tx := db.WithContext(ctx).Exec(query, params...)
	if tx.Error != nil {
		return "", nil, fmt.Errorf("db.exec: %w", tx.Error)
	}

	return api.OutputSuccess, map[string]any{
		"rows_affected": tx.RowsAffected,
	}, nil
}
