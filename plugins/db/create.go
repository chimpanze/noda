package db

import (
	"context"
	"fmt"
	"strings"

	"github.com/chimpanze/noda/internal/plugin"
	"github.com/chimpanze/noda/pkg/api"
	"gorm.io/gorm"
)

type createDescriptor struct{}

func (d *createDescriptor) Name() string        { return "create" }
func (d *createDescriptor) Description() string { return "Inserts a row into a table" }
func (d *createDescriptor) ServiceDeps() map[string]api.ServiceDep {
	return map[string]api.ServiceDep{
		"database": {Prefix: "db", Required: true},
	}
}
func (d *createDescriptor) ConfigSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"table": map[string]any{"type": "string", "description": "Table name"},
			"data":  map[string]any{"type": "object", "description": "Column values as key-value pairs"},
		},
		"required": []any{"table", "data"},
	}
}
func (d *createDescriptor) OutputDescriptions() map[string]string {
	return map[string]string{
		"success": "The created row object including generated fields (id, created_at, etc.)",
		"error":   "Database error details (e.g. unique constraint violation)",
	}
}

type createExecutor struct{}

func newCreateExecutor(_ map[string]any) api.NodeExecutor {
	return &createExecutor{}
}

func (e *createExecutor) Outputs() []string { return api.DefaultOutputs() }

func (e *createExecutor) Execute(ctx context.Context, nCtx api.ExecutionContext, config map[string]any, services map[string]any) (string, any, error) {
	db, err := plugin.GetService[*gorm.DB](services, "database")
	if err != nil {
		return "", nil, err
	}

	table, err := plugin.ResolveString(nCtx, config, "table")
	if err != nil {
		return "", nil, fmt.Errorf("db.create: %w", err)
	}

	if err := ValidateIdentifier(table); err != nil {
		return "", nil, fmt.Errorf("db.create: %w", err)
	}

	data, err := plugin.ResolveMap(nCtx, config, "data")
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

	return api.OutputSuccess, data, nil
}
