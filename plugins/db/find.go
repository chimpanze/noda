package db

import (
	"context"
	"fmt"

	"github.com/chimpanze/noda/internal/plugin"
	"github.com/chimpanze/noda/pkg/api"
	"gorm.io/gorm"
)

type findDescriptor struct{}

func (d *findDescriptor) Name() string { return "find" }
func (d *findDescriptor) Description() string {
	return "Structured SELECT returning an array of row objects"
}
func (d *findDescriptor) ServiceDeps() map[string]api.ServiceDep {
	return map[string]api.ServiceDep{
		"database": {Prefix: "db", Required: true},
	}
}
func (d *findDescriptor) ConfigSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"table":        map[string]any{"type": "string", "description": "Table name"},
			"select":       map[string]any{"type": "array", "description": "Column names to select (default: all)"},
			"where":        map[string]any{"type": "object", "description": "Equality conditions as key-value pairs"},
			"where_clause": map[string]any{"type": "object", "description": "Raw WHERE with query and params"},
			"joins":        map[string]any{"type": "array", "description": "JOIN clauses"},
			"order":        map[string]any{"type": "string", "description": "ORDER BY clause"},
			"group":        map[string]any{"type": "string", "description": "GROUP BY clause"},
			"having":       map[string]any{"type": "string", "description": "HAVING clause"},
			"limit":        map[string]any{"type": "integer", "description": "Maximum rows to return"},
			"offset":       map[string]any{"type": "integer", "description": "Rows to skip"},
		},
		"required": []any{"table"},
	}
}

type findExecutor struct{}

func newFindExecutor(_ map[string]any) api.NodeExecutor {
	return &findExecutor{}
}

func (e *findExecutor) Outputs() []string { return api.DefaultOutputs() }

func (e *findExecutor) Execute(ctx context.Context, nCtx api.ExecutionContext, config map[string]any, services map[string]any) (string, any, error) {
	db, err := plugin.GetService[*gorm.DB](services, "database")
	if err != nil {
		return "", nil, err
	}

	table, err := plugin.ResolveString(nCtx, config, "table")
	if err != nil {
		return "", nil, fmt.Errorf("db.find: %w", err)
	}

	if err := ValidateIdentifier(table); err != nil {
		return "", nil, fmt.Errorf("db.find: %w", err)
	}

	tx := db.WithContext(ctx).Table(table)

	tx, err = applyWhere(tx, nCtx, config)
	if err != nil {
		return "", nil, fmt.Errorf("db.find: %w", err)
	}

	tx, err = applyQueryOptions(tx, nCtx, config)
	if err != nil {
		return "", nil, fmt.Errorf("db.find: %w", err)
	}

	var results []map[string]any
	tx = tx.Scan(&results)
	if tx.Error != nil {
		return "", nil, fmt.Errorf("db.find: %w", tx.Error)
	}

	if results == nil {
		results = []map[string]any{}
	}

	return api.OutputSuccess, results, nil
}
