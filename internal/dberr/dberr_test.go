package dberr_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/chimpanze/noda/internal/dberr"
	"github.com/chimpanze/noda/pkg/api"
	"github.com/jackc/pgx/v5/pgconn"
	sqlite3 "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func pgErr(code string) error {
	return fmt.Errorf("db.find: %w", &pgconn.PgError{Code: code, Message: "driver detail"})
}

func sqliteErr(ext sqlite3.ErrNoExtended) error {
	return fmt.Errorf("db.find: %w", sqlite3.Error{Code: sqlite3.ErrConstraint, ExtendedCode: ext})
}

func TestClassifyPostgres(t *testing.T) {
	tests := []struct {
		code string
		want any
	}{
		{"23505", &api.ConflictError{}},
		{"23503", &api.ConflictError{}},
		{"23P01", &api.ConflictError{}},
		{"23502", &api.ValidationError{}},
		{"23514", &api.ValidationError{}},
		{"22P02", &api.ValidationError{}},
		{"22003", &api.ValidationError{}},
		{"22007", &api.ValidationError{}},
		{"22008", &api.ValidationError{}},
		{"22001", &api.ValidationError{}},
		{"22023", &api.ValidationError{}},
		{"40001", &api.ServiceUnavailableError{}},
		{"40P01", &api.ServiceUnavailableError{}},
		{"53300", &api.ServiceUnavailableError{}},
		{"08000", &api.ServiceUnavailableError{}},
		{"08003", &api.ServiceUnavailableError{}},
		{"08006", &api.ServiceUnavailableError{}},
		{"57014", &api.TimeoutError{}},
	}
	for _, tc := range tests {
		t.Run(tc.code, func(t *testing.T) {
			got := dberr.Classify(pgErr(tc.code), "users")
			require.NotNil(t, got, "code %s should classify", tc.code)
			assert.IsType(t, tc.want, got)
			var pge *pgconn.PgError
			assert.True(t, errors.As(got, &pge), "cause must stay recoverable")
			assert.Equal(t, tc.code, pge.Code)
		})
	}
}

// Class 42 is an author bug, not a caller fault: it must stay a 500.
func TestClassifyLeavesClass42Unmapped(t *testing.T) {
	for _, code := range []string{"42703", "42P01", "42883", "42601"} {
		assert.Nil(t, dberr.Classify(pgErr(code), "users"), "code %s must not classify", code)
	}
}

func TestClassifyUnknownAndNil(t *testing.T) {
	assert.Nil(t, dberr.Classify(nil, "users"))
	assert.Nil(t, dberr.Classify(errors.New("plain error"), "users"))
	assert.Nil(t, dberr.Classify(pgErr("XX000"), "users"))
}

func TestClassifySQLite(t *testing.T) {
	tests := []struct {
		name string
		ext  sqlite3.ErrNoExtended
		want any
	}{
		{"unique", sqlite3.ErrConstraintUnique, &api.ConflictError{}},
		{"primarykey", sqlite3.ErrConstraintPrimaryKey, &api.ConflictError{}},
		{"foreignkey", sqlite3.ErrConstraintForeignKey, &api.ConflictError{}},
		{"notnull", sqlite3.ErrConstraintNotNull, &api.ValidationError{}},
		{"check", sqlite3.ErrConstraintCheck, &api.ValidationError{}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := dberr.Classify(sqliteErr(tc.ext), "users")
			require.NotNil(t, got)
			assert.IsType(t, tc.want, got)
		})
	}
}

// SQLite collapses every schema error into Code=1, which must stay a 500.
func TestClassifySQLiteLogicErrorUnmapped(t *testing.T) {
	err := fmt.Errorf("db.find: %w", sqlite3.Error{Code: sqlite3.ErrError})
	assert.Nil(t, dberr.Classify(err, "users"))
}

func TestConflictCarriesResource(t *testing.T) {
	got := dberr.Classify(pgErr("23505"), "orders")
	var ce *api.ConflictError
	require.True(t, errors.As(got, &ce))
	assert.Equal(t, "orders", ce.Resource)
}

// ValidationError.Message reaches prod clients unconditionally, so it must
// be a fixed safe phrase and never echo driver text.
func TestValidationMessageHasNoDriverText(t *testing.T) {
	got := dberr.Classify(pgErr("22P02"), "users")
	var ve *api.ValidationError
	require.True(t, errors.As(got, &ve))
	assert.NotContains(t, ve.Message, "driver detail")
	assert.Nil(t, ve.Value, "Value must not carry driver-derived data")
}

func TestIsUniqueViolation(t *testing.T) {
	assert.True(t, dberr.IsUniqueViolation(pgErr("23505")))
	assert.True(t, dberr.IsUniqueViolation(sqliteErr(sqlite3.ErrConstraintUnique)))
	assert.True(t, dberr.IsUniqueViolation(sqliteErr(sqlite3.ErrConstraintPrimaryKey)))
	assert.False(t, dberr.IsUniqueViolation(pgErr("23503")))
	assert.False(t, dberr.IsUniqueViolation(sqliteErr(sqlite3.ErrConstraintNotNull)))
	assert.False(t, dberr.IsUniqueViolation(nil))
	assert.False(t, dberr.IsUniqueViolation(errors.New("plain")))
}
