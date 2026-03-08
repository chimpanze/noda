package db

import (
	"context"
	"fmt"

	"github.com/chimpanze/noda/pkg/api"
)

type queryDescriptor struct{}

func (d *queryDescriptor) Name() string { return "query" }
func (d *queryDescriptor) ServiceDeps() map[string]api.ServiceDep {
	return map[string]api.ServiceDep{
		"database": {Prefix: "db", Required: true},
	}
}
func (d *queryDescriptor) ConfigSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"query":  map[string]any{"type": "string"},
			"params": map[string]any{"type": "array"},
		},
		"required": []any{"query"},
	}
}

type queryExecutor struct{}

func newQueryExecutor(_ map[string]any) api.NodeExecutor {
	return &queryExecutor{}
}

func (e *queryExecutor) Outputs() []string { return []string{"success", "error"} }

func (e *queryExecutor) Execute(ctx context.Context, nCtx api.ExecutionContext, config map[string]any, services map[string]any) (string, any, error) {
	db, err := getDB(services)
	if err != nil {
		return "", nil, err
	}

	query, err := resolveString(nCtx, config, "query")
	if err != nil {
		return "", nil, fmt.Errorf("db.query: %w", err)
	}

	params, err := resolveParams(nCtx, config)
	if err != nil {
		return "", nil, fmt.Errorf("db.query: %w", err)
	}

	var results []map[string]any
	tx := db.WithContext(ctx).Raw(query, params...).Scan(&results)
	if tx.Error != nil {
		return "", nil, fmt.Errorf("db.query: %w", tx.Error)
	}

	if results == nil {
		results = []map[string]any{}
	}

	return "success", results, nil
}
