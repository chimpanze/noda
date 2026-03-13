package db

import (
	"fmt"
	"regexp"
	"strings"
)

var (
	identifierRe = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_.]*$`)
	orderItemRe  = regexp.MustCompile(`(?i)^[a-zA-Z_][a-zA-Z0-9_.]*(\s+(ASC|DESC))?$`)

	validJoinTypes = map[string]bool{
		"INNER": true,
		"LEFT":  true,
		"RIGHT": true,
		"FULL":  true,
		"CROSS": true,
	}
)

// ValidateIdentifier rejects empty or non-matching SQL identifiers.
func ValidateIdentifier(s string) error {
	if s == "" {
		return fmt.Errorf("identifier must not be empty")
	}
	if !identifierRe.MatchString(s) {
		return fmt.Errorf("invalid identifier: %q", s)
	}
	return nil
}

// ValidateOrderClause splits on commas and validates each part against orderItemRe.
func ValidateOrderClause(s string) error {
	if s == "" {
		return fmt.Errorf("order clause must not be empty")
	}
	parts := strings.Split(s, ",")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			return fmt.Errorf("order clause contains empty item")
		}
		if !orderItemRe.MatchString(part) {
			return fmt.Errorf("invalid order item: %q", part)
		}
	}
	return nil
}

// ValidateJoinType checks that the join type is one of the allowed SQL join types.
func ValidateJoinType(s string) error {
	if !validJoinTypes[strings.ToUpper(s)] {
		return fmt.Errorf("invalid join type: %q", s)
	}
	return nil
}

// blockedSQLKeywords are dangerous SQL keywords that should never appear in user-provided fragments.
var blockedSQLKeywords = []string{
	"DROP", "DELETE", "INSERT", "UPDATE", "ALTER", "CREATE",
	"EXEC", "UNION", "SELECT", "GRANT", "REVOKE", "TRUNCATE",
}

// wordBoundaryRe matches non-word characters for whole-word keyword detection.
var wordBoundaryRe = regexp.MustCompile(`\W+`)

// containsWord checks if s contains kw as a whole word (not as a substring of another word).
func containsWord(s, kw string) bool {
	words := wordBoundaryRe.Split(s, -1)
	for _, w := range words {
		if w == kw {
			return true
		}
	}
	return false
}

// ValidateSQLFragment rejects strings containing dangerous SQL patterns.
func ValidateSQLFragment(s string) error {
	if strings.Contains(s, ";") {
		return fmt.Errorf("SQL fragment must not contain semicolons")
	}
	if strings.Contains(s, "--") {
		return fmt.Errorf("SQL fragment must not contain line comments")
	}
	if strings.Contains(s, "/*") {
		return fmt.Errorf("SQL fragment must not contain block comments")
	}
	upper := strings.ToUpper(s)
	for _, kw := range blockedSQLKeywords {
		if containsWord(upper, kw) {
			return fmt.Errorf("SQL fragment contains blocked keyword %q", kw)
		}
	}
	return nil
}
