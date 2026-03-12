package db

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/chimpanze/noda/internal/plugin"
	"github.com/chimpanze/noda/pkg/api"
	"gorm.io/gorm"
)

type queryDescriptor struct{}

func (d *queryDescriptor) Name() string { return "query" }
func (d *queryDescriptor) Description() string {
	return "Executes a SELECT query and returns result rows"
}
func (d *queryDescriptor) ServiceDeps() map[string]api.ServiceDep {
	return map[string]api.ServiceDep{
		"database": {Prefix: "db", Required: true},
	}
}
func (d *queryDescriptor) ConfigSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"query":  map[string]any{"type": "string", "description": "SQL SELECT statement"},
			"params": map[string]any{"type": "array", "description": "Positional query parameters ($1, $2, ...)"},
		},
		"required": []any{"query"},
	}
}

type queryExecutor struct{}

func newQueryExecutor(_ map[string]any) api.NodeExecutor {
	return &queryExecutor{}
}

func (e *queryExecutor) Outputs() []string { return api.DefaultOutputs() }

func (e *queryExecutor) Execute(ctx context.Context, nCtx api.ExecutionContext, config map[string]any, services map[string]any) (string, any, error) {
	db, err := plugin.GetService[*gorm.DB](services, "database")
	if err != nil {
		return "", nil, err
	}

	query, err := plugin.ResolveString(nCtx, config, "query")
	if err != nil {
		return "", nil, fmt.Errorf("db.query: %w", err)
	}

	params, err := plugin.ResolveOptionalArray(nCtx, config, "params")
	if err != nil {
		return "", nil, fmt.Errorf("db.query: %w", err)
	}

	slog.Warn("executing raw SQL query", "query_prefix", query[:min(len(query), 50)])

	var results []map[string]any
	tx := db.WithContext(ctx).Raw(query, params...).Scan(&results)
	if tx.Error != nil {
		return "", nil, fmt.Errorf("db.query: %w", tx.Error)
	}

	if results == nil {
		results = []map[string]any{}
	}

	return api.OutputSuccess, results, nil
}
