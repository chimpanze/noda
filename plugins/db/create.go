package db

import (
	"context"
	"fmt"
	"strings"

	"github.com/chimpanze/noda/pkg/api"
)

type createDescriptor struct{}

func (d *createDescriptor) Name() string { return "create" }
func (d *createDescriptor) ServiceDeps() map[string]api.ServiceDep {
	return map[string]api.ServiceDep{
		"database": {Prefix: "db", Required: true},
	}
}
func (d *createDescriptor) ConfigSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"table": map[string]any{"type": "string"},
			"data":  map[string]any{"type": "object"},
		},
		"required": []any{"table", "data"},
	}
}

type createExecutor struct{}

func newCreateExecutor(_ map[string]any) api.NodeExecutor {
	return &createExecutor{}
}

func (e *createExecutor) Outputs() []string { return []string{"success", "error"} }

func (e *createExecutor) Execute(ctx context.Context, nCtx api.ExecutionContext, config map[string]any, services map[string]any) (string, any, error) {
	db, err := getDB(services)
	if err != nil {
		return "", nil, err
	}

	table, err := resolveString(nCtx, config, "table")
	if err != nil {
		return "", nil, fmt.Errorf("db.create: %w", err)
	}

	data, err := resolveMap(nCtx, config, "data")
	if err != nil {
		return "", nil, fmt.Errorf("db.create: %w", err)
	}

	tx := db.WithContext(ctx).Table(table).Create(data)
	if tx.Error != nil {
		errMsg := tx.Error.Error()
		if strings.Contains(errMsg, "duplicate key") || strings.Contains(errMsg, "unique constraint") {
			return "", nil, &api.ConflictError{
				Resource: table,
				Reason:   errMsg,
			}
		}
		return "", nil, fmt.Errorf("db.create: %w", tx.Error)
	}

	return "success", data, nil
}
