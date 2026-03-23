package db

import (
	"fmt"
	"log/slog"
	"strings"

	"github.com/chimpanze/noda/internal/plugin"
	"github.com/chimpanze/noda/pkg/api"
	"gorm.io/gorm"
)

type whereClause struct {
	Query  string
	Params []any
}

type joinSpec struct {
	Query  string
	Params []any
}

// resolveWhereClause parses the optional "where_clause" config field.
// Expected format: {"query": "col > ?", "params": [...]}
func resolveWhereClause(nCtx api.ExecutionContext, config map[string]any) (*whereClause, error) {
	raw, ok := config["where_clause"]
	if !ok {
		return nil, nil
	}
	m, ok := raw.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("where_clause must be an object")
	}

	queryRaw, ok := m["query"]
	if !ok {
		return nil, fmt.Errorf("where_clause: missing required field \"query\"")
	}
	queryStr, ok := queryRaw.(string)
	if !ok {
		return nil, fmt.Errorf("where_clause.query must be a string")
	}
	resolved, err := nCtx.Resolve(queryStr)
	if err != nil {
		return nil, fmt.Errorf("where_clause: resolve query: %w", err)
	}
	query, ok := resolved.(string)
	if !ok {
		return nil, fmt.Errorf("where_clause.query resolved to %T, expected string", resolved)
	}
	if err := ValidateSQLFragment(query); err != nil {
		return nil, fmt.Errorf("where_clause: %w", err)
	}

	params, err := plugin.ResolveOptionalArray(nCtx, m, "params")
	if err != nil {
		return nil, fmt.Errorf("where_clause: %w", err)
	}

	return &whereClause{Query: query, Params: params}, nil
}

// resolveJoins parses the optional "joins" config field.
// Each join: {"type": "LEFT", "table": "users", "on": "tasks.user_id = users.id", "params": [...]}
func resolveJoins(nCtx api.ExecutionContext, config map[string]any) ([]joinSpec, error) {
	raw, ok := config["joins"]
	if !ok {
		return nil, nil
	}
	arr, ok := raw.([]any)
	if !ok {
		return nil, fmt.Errorf("joins must be an array")
	}

	joins := make([]joinSpec, 0, len(arr))
	for i, item := range arr {
		m, ok := item.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("joins[%d] must be an object", i)
		}

		joinType := "INNER"
		if t, ok := m["type"].(string); ok {
			joinType = strings.ToUpper(t)
		}
		if err := ValidateJoinType(joinType); err != nil {
			return nil, fmt.Errorf("joins[%d]: %w", i, err)
		}

		tableRaw, ok := m["table"]
		if !ok {
			return nil, fmt.Errorf("joins[%d]: missing required field \"table\"", i)
		}
		table, ok := tableRaw.(string)
		if !ok {
			return nil, fmt.Errorf("joins[%d].table must be a string", i)
		}
		if err := ValidateIdentifier(table); err != nil {
			return nil, fmt.Errorf("joins[%d]: %w", i, err)
		}

		onRaw, ok := m["on"]
		if !ok {
			return nil, fmt.Errorf("joins[%d]: missing required field \"on\"", i)
		}
		on, ok := onRaw.(string)
		if !ok {
			return nil, fmt.Errorf("joins[%d].on must be a string", i)
		}
		if err := ValidateSQLFragment(on); err != nil {
			return nil, fmt.Errorf("joins[%d]: %w", i, err)
		}

		params, err := plugin.ResolveOptionalArray(nCtx, m, "params")
		if err != nil {
			return nil, fmt.Errorf("joins[%d]: %w", i, err)
		}

		query := fmt.Sprintf("%s JOIN %s ON %s", joinType, table, on)
		joins = append(joins, joinSpec{Query: query, Params: params})
	}

	return joins, nil
}

// resolveSelectColumns parses the optional "select" config field.
func resolveSelectColumns(nCtx api.ExecutionContext, config map[string]any) ([]string, error) {
	raw, ok := config["select"]
	if !ok {
		return nil, nil
	}
	arr, ok := raw.([]any)
	if !ok {
		return nil, fmt.Errorf("select must be an array")
	}

	cols := make([]string, 0, len(arr))
	for i, item := range arr {
		s, ok := item.(string)
		if !ok {
			return nil, fmt.Errorf("select[%d] must be a string", i)
		}
		resolved, err := nCtx.Resolve(s)
		if err != nil {
			return nil, fmt.Errorf("resolve select[%d]: %w", i, err)
		}
		str, ok := resolved.(string)
		if !ok {
			return nil, fmt.Errorf("select[%d] resolved to %T, expected string", i, resolved)
		}
		if err := ValidateIdentifier(str); err != nil {
			return nil, fmt.Errorf("select[%d]: %w", i, err)
		}
		cols = append(cols, str)
	}
	return cols, nil
}

// applyWhere applies structured "where" and/or "where_clause" to the GORM query.
func applyWhere(tx *gorm.DB, nCtx api.ExecutionContext, config map[string]any) (*gorm.DB, error) {
	where, err := plugin.ResolveOptionalMap(nCtx, config, "where")
	if err != nil {
		return nil, err
	}
	if where != nil {
		tx = tx.Where(where)
	}

	wc, err := resolveWhereClause(nCtx, config)
	if err != nil {
		return nil, err
	}
	if wc != nil {
		tx = tx.Where(wc.Query, wc.Params...)
	}

	return tx, nil
}

// applyQueryOptions applies select, order, limit, offset, joins, group, and having.
func applyQueryOptions(tx *gorm.DB, nCtx api.ExecutionContext, config map[string]any) (*gorm.DB, error) {
	cols, err := resolveSelectColumns(nCtx, config)
	if err != nil {
		return nil, err
	}
	if len(cols) > 0 {
		tx = tx.Select(cols)
	}

	if order, ok, err := plugin.ResolveOptionalString(nCtx, config, "order"); err != nil {
		return nil, err
	} else if ok {
		if err := ValidateOrderClause(order); err != nil {
			return nil, fmt.Errorf("order: %w", err)
		}
		tx = tx.Order(order)
	}

	if limit, ok, err := plugin.ResolveOptionalInt(nCtx, config, "limit"); err != nil {
		return nil, err
	} else if ok {
		tx = tx.Limit(limit)
	}

	if offset, ok, err := plugin.ResolveOptionalInt(nCtx, config, "offset"); err != nil {
		return nil, err
	} else if ok {
		tx = tx.Offset(offset)
	}

	if group, ok, err := plugin.ResolveOptionalString(nCtx, config, "group"); err != nil {
		return nil, err
	} else if ok {
		for _, col := range strings.Split(group, ",") {
			col = strings.TrimSpace(col)
			if err := ValidateIdentifier(col); err != nil {
				return nil, fmt.Errorf("group: %w", err)
			}
		}
		tx = tx.Group(group)
	}

	// Having: support both string (legacy) and parameterized object format
	if havingRaw, ok := config["having"]; ok {
		switch h := havingRaw.(type) {
		case string:
			resolved, err := nCtx.Resolve(h)
			if err != nil {
				return nil, fmt.Errorf("having: %w", err)
			}
			havingStr, ok := resolved.(string)
			if !ok {
				return nil, fmt.Errorf("having resolved to %T, expected string", resolved)
			}
			if err := ValidateSQLFragment(havingStr); err != nil {
				return nil, fmt.Errorf("having: %w", err)
			}
			slog.Warn("string having clause is deprecated, use {\"query\": ..., \"params\": [...]} format")
			tx = tx.Having(havingStr)
		case map[string]any:
			query, _ := h["query"].(string)
			if query == "" {
				return nil, fmt.Errorf("having: missing required field \"query\"")
			}
			resolved, err := nCtx.Resolve(query)
			if err != nil {
				return nil, fmt.Errorf("having: %w", err)
			}
			queryStr, ok := resolved.(string)
			if !ok {
				return nil, fmt.Errorf("having.query resolved to %T, expected string", resolved)
			}
			if err := ValidateSQLFragment(queryStr); err != nil {
				return nil, fmt.Errorf("having: %w", err)
			}
			params, err := plugin.ResolveOptionalArray(nCtx, h, "params")
			if err != nil {
				return nil, fmt.Errorf("having: %w", err)
			}
			tx = tx.Having(queryStr, params...)
		}
	}

	joins, err := resolveJoins(nCtx, config)
	if err != nil {
		return nil, err
	}
	for _, j := range joins {
		if len(j.Params) > 0 {
			tx = tx.Joins(j.Query, j.Params...)
		} else {
			tx = tx.Joins(j.Query)
		}
	}

	return tx, nil
}
