package db

import (
	"fmt"

	"github.com/chimpanze/noda/internal/dberr"
)

// classifyOr maps a SQL driver error to a typed api error when its cause
// is caller-facing, and otherwise wraps it with the node's context string
// exactly as before.
//
// resource is the table (or, for raw-SQL nodes, a stand-in name) used for
// ConflictError.Resource; op is the node's error prefix, e.g. "db.create".
func classifyOr(err error, resource, op string) error {
	if typed := dberr.Classify(err, resource); typed != nil {
		return typed
	}
	return fmt.Errorf("%s: %w", op, err)
}
