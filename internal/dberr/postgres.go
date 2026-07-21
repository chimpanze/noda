package dberr

import (
	"errors"

	"github.com/jackc/pgx/v5/pgconn"
)

// Postgres SQLSTATE codes with a demonstrable caller cause. Codes absent
// from every table below are deliberately unmapped and stay a 500 —
// notably all of class 42 (undefined column/table/function), which is an
// author bug rather than a caller fault.
//
// The values are the client-facing reason strings. They must stay free of
// driver text: internal/server renders ValidationError.Message to prod
// clients unconditionally.
var (
	pgConflict = map[string]string{
		"23505": "unique constraint violation",
		"23503": "foreign key constraint violation",
		"23P01": "exclusion constraint violation",
	}

	pgValidation = map[string]string{
		"23502": "not-null constraint violation",
		"23514": "check constraint violation",
		"22P02": "invalid input syntax",
		"22003": "numeric value out of range",
		"22007": "invalid datetime format",
		"22008": "datetime field overflow",
		"22001": "value too long for column",
		"22023": "invalid parameter value",
	}

	pgUnavailable = map[string]string{
		"40001": "serialization failure",
		"40P01": "deadlock detected",
		"53300": "too many connections",
		"08000": "connection exception",
		"08003": "connection does not exist",
		"08006": "connection failure",
	}
)

const pgQueryCanceled = "57014"

// pgError extracts a *pgconn.PgError from anywhere in err's chain.
func pgError(err error) (*pgconn.PgError, bool) {
	var pge *pgconn.PgError
	if errors.As(err, &pge) {
		return pge, true
	}
	return nil, false
}

// pgField returns a best-effort column name for a ValidationError.
// Postgres frequently leaves ColumnName empty (it is populated for
// not-null violations but not for cast failures), and no constraint name
// is used here to avoid exposing internal schema naming. An empty field
// is acceptable.
func pgField(pge *pgconn.PgError) string {
	return pge.ColumnName
}
