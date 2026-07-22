package dberr_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/chimpanze/noda/internal/dberr"
	"github.com/chimpanze/noda/pkg/api"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A classifiable driver error becomes a typed api error, and the node's
// context string is deliberately dropped in favour of the typed error.
func TestClassifyOr_TypedWins(t *testing.T) {
	driverErr := &pgconn.PgError{Code: "23505", Message: "nonstandard wording"}
	got := dberr.ClassifyOr(driverErr, "users", "db.create")

	var ce *api.ConflictError
	require.True(t, errors.As(got, &ce), "want ConflictError, got %v", got)
	assert.Equal(t, "users", ce.Resource)
	assert.True(t, errors.As(got, new(*pgconn.PgError)), "cause must stay recoverable")
}

// An unclassifiable error keeps today's behavior: wrapped with the node's
// context string, so existing messages and %w chains are unchanged.
func TestClassifyOr_FallsThroughWithContext(t *testing.T) {
	base := errors.New("connection reset")
	got := dberr.ClassifyOr(base, "users", "db.update")

	assert.EqualError(t, got, "db.update: connection reset")
	assert.ErrorIs(t, got, base)
}

// Class 42 is an author bug, not a caller fault, so it must fall through
// to the wrapped form and stay a 500.
func TestClassifyOr_Class42FallsThrough(t *testing.T) {
	driverErr := fmt.Errorf("boom: %w", &pgconn.PgError{Code: "42703", Message: "no column"})
	got := dberr.ClassifyOr(driverErr, "users", "db.find")

	var ce *api.ConflictError
	var ve *api.ValidationError
	assert.False(t, errors.As(got, &ce))
	assert.False(t, errors.As(got, &ve))
	assert.Contains(t, got.Error(), "db.find:")
}
