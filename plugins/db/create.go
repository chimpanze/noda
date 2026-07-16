package db

import (
	"context"
	"fmt"
	"strings"

	"github.com/chimpanze/noda/internal/plugin"
	"github.com/chimpanze/noda/pkg/api"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
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
			"data":  map[string]any{"type": "object", "description": "Column values as key-value pairs", "additionalProperties": true},
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
		return "", nil, fmt.Errorf("db.create: %w", err)
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

	row, err := marshalJSONComposites(data)
	if err != nil {
		return "", nil, fmt.Errorf("db.create: %w", err)
	}

	// Use RETURNING * so that server-generated fields (e.g. id, created_at)
	// are populated back into the row map.
	tx := db.WithContext(ctx).Table(table).Clauses(clause.Returning{}).Create(row)
	if tx.Error != nil {
		errMsg := tx.Error.Error()
		if strings.Contains(errMsg, "duplicate key") || strings.Contains(errMsg, "unique constraint") {
			return "", nil, &api.ConflictError{
				Resource: table,
				Reason:   "unique constraint violation",
			}
		}
		return "", nil, fmt.Errorf("db.create: %w", tx.Error)
	}

	// clause.Returning repopulates row from the DB, where jsonb columns come
	// back as raw bytes or strings that would serialize incorrectly. Restore the
	// caller's original structured composite values (server-generated scalars
	// such as id and created_at remain from RETURNING).
	for k, orig := range data {
		if isJSONComposite(orig) {
			row[k] = orig
		}
	}

	return api.OutputSuccess, row, nil
}
