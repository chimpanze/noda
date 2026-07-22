// Package dberr classifies SQL driver errors into the typed api error
// set, so that caller-triggerable query failures surface as 4xx rather
// than a blanket 500.
//
// Classification is by driver error code (Postgres SQLSTATE, SQLite
// extended result code), never by message text: message wording differs
// between drivers and versions, and matching on it previously caused
// SQLite unique violations to be missed entirely.
package dberr

import (
	"github.com/chimpanze/noda/pkg/api"
)

// Classify maps a SQL driver error to a typed api error, or returns nil
// if the error has no known caller-facing meaning.
//
// Returning nil rather than the original error is deliberate: the caller
// keeps ownership of its own error-wrapping context, and an unmapped
// error is never silently reshaped. Callers should fall through to their
// existing fmt.Errorf path when nil is returned.
//
// resource names the table or entity involved; it is used for
// ConflictError.Resource and is safe to send to clients.
func Classify(err error, resource string) error {
	if err == nil {
		return nil
	}

	if pge, ok := pgError(err); ok {
		code := pge.Code
		if reason, hit := pgConflict[code]; hit {
			return &api.ConflictError{Resource: resource, Reason: reason, Cause: err}
		}
		if reason, hit := pgValidation[code]; hit {
			return &api.ValidationError{Field: pgField(pge), Message: reason, Cause: err}
		}
		if _, hit := pgUnavailable[code]; hit {
			return &api.ServiceUnavailableError{Service: "database", Cause: err}
		}
		if code == pgQueryCanceled {
			return &api.TimeoutError{Operation: "database query", Cause: err}
		}
		return nil
	}

	if se, ok := sqliteError(err); ok {
		if reason, hit := sqliteConflict[se.ExtendedCode]; hit {
			return &api.ConflictError{Resource: resource, Reason: reason, Cause: err}
		}
		if reason, hit := sqliteValidation[se.ExtendedCode]; hit {
			// SQLite exposes no structured column name; Field stays empty.
			return &api.ValidationError{Message: reason, Cause: err}
		}
		return nil
	}

	return nil
}

// IsUniqueViolation reports whether err is a unique-constraint violation
// on either driver. It exists for callers that branch on this specific
// condition rather than returning a typed error — plugins/auth returns an
// "exists" node output instead of a ConflictError.
func IsUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	if pge, ok := pgError(err); ok {
		return pge.Code == "23505"
	}
	if se, ok := sqliteError(err); ok {
		return se.ExtendedCode == sqlite3ConstraintUnique ||
			se.ExtendedCode == sqlite3ConstraintPrimaryKey ||
			se.ExtendedCode == sqlite3ConstraintRowID
	}
	return false
}
