package dberr

import (
	"errors"

	sqlite3 "github.com/mattn/go-sqlite3"
)

// SQLite extended result codes.
//
// SQLite's dynamic typing accepts a string in an ordinary INTEGER column
// without error, so for ordinary columns there is no analogue of the
// Postgres data-exception class. There are two exceptions, both mapped
// below: an INTEGER PRIMARY KEY (rowid alias) rejects a non-integer with
// ErrMismatch, and a STRICT table rejects a wrong-typed value with
// SQLITE_CONSTRAINT_DATATYPE.
//
// SQLite still collapses every schema error into ErrError (Code=1), which
// stays unmapped and therefore a 500 — the same treatment Postgres class
// 42 gets.
//
// Note that SQLite splits what Postgres reports as a single 23505 across
// three extended codes (unique, primary key, rowid). All map to
// ConflictError, or the same conflict would differ by driver.

// sqliteConstraintDataType is SQLITE_CONSTRAINT_DATATYPE (3091), raised
// when a STRICT table rejects a value of the wrong type.
//
// It is constructed rather than named because go-sqlite3 v1.14.22 defines
// ErrConstraint.Extend(n) constants only up to ErrConstraintRowID
// (Extend(10)) — STRICT tables arrived in SQLite 3.37, after that list was
// written. TestSQLiteDataTypeCodeValue pins the value.
var sqliteConstraintDataType = sqlite3.ErrConstraint.Extend(12)

var (
	sqliteConflict = map[sqlite3.ErrNoExtended]string{
		sqlite3.ErrConstraintUnique:     "unique constraint violation",
		sqlite3.ErrConstraintPrimaryKey: "unique constraint violation",
		sqlite3.ErrConstraintRowID:      "unique constraint violation",
		sqlite3.ErrConstraintForeignKey: "foreign key constraint violation",
	}

	sqliteValidation = map[sqlite3.ErrNoExtended]string{
		sqlite3.ErrConstraintNotNull: "not-null constraint violation",
		sqlite3.ErrConstraintCheck:   "check constraint violation",
		sqliteConstraintDataType:     "invalid input syntax",
		// ErrMismatch is a PRIMARY result code with no extended variant.
		// SQLite reports ExtendedCode == Code == 20 in that case, so it
		// keys into this ExtendedCode-based map directly and needs no
		// separate se.Code branch.
		sqlite3.ErrNoExtended(sqlite3.ErrMismatch): "invalid input syntax",
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
	sqlite3ConstraintRowID      = sqlite3.ErrConstraintRowID
)
