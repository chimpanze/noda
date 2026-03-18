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

type upsertDescriptor struct{}

func (d *upsertDescriptor) Name() string        { return "upsert" }
func (d *upsertDescriptor) Description() string { return "Inserts a row or updates it on conflict" }
func (d *upsertDescriptor) ServiceDeps() map[string]api.ServiceDep {
	return map[string]api.ServiceDep{
		"database": {Prefix: "db", Required: true},
	}
}
func (d *upsertDescriptor) ConfigSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"table":    map[string]any{"type": "string", "description": "Table name"},
			"data":     map[string]any{"type": "object", "description": "Column values as key-value pairs"},
			"conflict": map[string]any{"description": "Conflict column(s) for ON CONFLICT"},
			"update":   map[string]any{"description": "Columns to update on conflict"},
		},
		"required": []any{"table", "data", "conflict"},
	}
}
func (d *upsertDescriptor) OutputDescriptions() map[string]string {
	return map[string]string{
		"success": "The upserted row object",
		"error":   "Database error details",
	}
}

type upsertExecutor struct{}

func newUpsertExecutor(_ map[string]any) api.NodeExecutor {
	return &upsertExecutor{}
}

func (e *upsertExecutor) Outputs() []string { return api.DefaultOutputs() }

func (e *upsertExecutor) Execute(ctx context.Context, nCtx api.ExecutionContext, config map[string]any, services map[string]any) (string, any, error) {
	db, err := plugin.GetService[*gorm.DB](services, "database")
	if err != nil {
		return "", nil, err
	}

	table, err := plugin.ResolveString(nCtx, config, "table")
	if err != nil {
		return "", nil, fmt.Errorf("db.upsert: %w", err)
	}

	if err := ValidateIdentifier(table); err != nil {
		return "", nil, fmt.Errorf("db.upsert: %w", err)
	}

	data, err := plugin.ResolveMap(nCtx, config, "data")
	if err != nil {
		return "", nil, fmt.Errorf("db.upsert: %w", err)
	}

	// Resolve conflict columns
	conflictCols, err := resolveConflictColumns(config)
	if err != nil {
		return "", nil, fmt.Errorf("db.upsert: %w", err)
	}

	onConflict := clause.OnConflict{
		Columns: conflictCols,
	}

	// Resolve update specification
	if err := resolveUpdateSpec(config, data, conflictCols, &onConflict); err != nil {
		return "", nil, fmt.Errorf("db.upsert: %w", err)
	}

	tx := db.WithContext(ctx).Table(table).Clauses(onConflict).Create(data)
	if tx.Error != nil {
		errMsg := tx.Error.Error()
		if strings.Contains(errMsg, "duplicate key") || strings.Contains(errMsg, "unique constraint") {
			return "", nil, &api.ConflictError{
				Resource: table,
				Reason:   errMsg,
			}
		}
		return "", nil, fmt.Errorf("db.upsert: %w", tx.Error)
	}

	return api.OutputSuccess, data, nil
}

func resolveConflictColumns(config map[string]any) ([]clause.Column, error) {
	raw, ok := config["conflict"]
	if !ok {
		return nil, fmt.Errorf("missing required field \"conflict\"")
	}

	switch v := raw.(type) {
	case string:
		if err := ValidateIdentifier(v); err != nil {
			return nil, fmt.Errorf("conflict column: %w", err)
		}
		return []clause.Column{{Name: v}}, nil
	case []any:
		cols := make([]clause.Column, 0, len(v))
		for i, item := range v {
			s, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("conflict[%d] must be a string", i)
			}
			if err := ValidateIdentifier(s); err != nil {
				return nil, fmt.Errorf("conflict[%d]: %w", i, err)
			}
			cols = append(cols, clause.Column{Name: s})
		}
		return cols, nil
	default:
		return nil, fmt.Errorf("conflict must be a string or array of strings")
	}
}

func resolveUpdateSpec(config map[string]any, data map[string]any, conflictCols []clause.Column, onConflict *clause.OnConflict) error {
	raw, ok := config["update"]
	if !ok {
		// Update all non-conflict columns
		conflictNames := make(map[string]bool, len(conflictCols))
		for _, c := range conflictCols {
			conflictNames[c.Name] = true
		}
		var updateCols []string
		for k := range data {
			if !conflictNames[k] {
				updateCols = append(updateCols, k)
			}
		}
		onConflict.DoUpdates = clause.AssignmentColumns(updateCols)
		return nil
	}

	switch v := raw.(type) {
	case []any:
		cols := make([]string, 0, len(v))
		for i, item := range v {
			s, ok := item.(string)
			if !ok {
				return fmt.Errorf("update[%d] must be a string", i)
			}
			if err := ValidateIdentifier(s); err != nil {
				return fmt.Errorf("update[%d]: %w", i, err)
			}
			cols = append(cols, s)
		}
		onConflict.DoUpdates = clause.AssignmentColumns(cols)
	case map[string]any:
		assignments := make([]clause.Assignment, 0, len(v))
		for col, val := range v {
			if err := ValidateIdentifier(col); err != nil {
				return fmt.Errorf("update column: %w", err)
			}
			assignments = append(assignments, clause.Assignment{
				Column: clause.Column{Name: col},
				Value:  val,
			})
		}
		onConflict.DoUpdates = assignments
	default:
		return fmt.Errorf("update must be an array or object")
	}
	return nil
}
