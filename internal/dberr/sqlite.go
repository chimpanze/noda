package dberr

import (
	"errors"

	sqlite3 "github.com/mattn/go-sqlite3"
)

// SQLite extended result codes. SQLite maps cleanly only for the
// constraint family: it has no analogue for the Postgres data-exception
// class (dynamic typing silently accepts a string in an integer column),
// and it collapses every schema error into ErrError (Code=1), which stays
// unmapped and therefore a 500.
//
// Note that SQLite splits what Postgres reports as a single 23505 across
// two extended codes. Both must map to ConflictError, or a primary-key
// conflict would be 409 on Postgres and 500 on SQLite.
var (
	sqliteConflict = map[sqlite3.ErrNoExtended]string{
		sqlite3.ErrConstraintUnique:     "unique constraint violation",
		sqlite3.ErrConstraintPrimaryKey: "unique constraint violation",
		sqlite3.ErrConstraintForeignKey: "foreign key constraint violation",
	}

	sqliteValidation = map[sqlite3.ErrNoExtended]string{
		sqlite3.ErrConstraintNotNull: "not-null constraint violation",
		sqlite3.ErrConstraintCheck:   "check constraint violation",
	}
)

// sqliteError extracts a sqlite3.Error from anywhere in err's chain.
// Note it is a value type, not a pointer, unlike pgconn.PgError.
func sqliteError(err error) (sqlite3.Error, bool) {
	return errors.AsType[sqlite3.Error](err)
}

// sqlite3.ErrConstraintUnique and sqlite3.ErrConstraintPrimaryKey are
// package vars (computed via ErrConstraint.Extend at init), not constant
// expressions, so these must be var, not const.
var (
	sqlite3ConstraintUnique     = sqlite3.ErrConstraintUnique
	sqlite3ConstraintPrimaryKey = sqlite3.ErrConstraintPrimaryKey
)
