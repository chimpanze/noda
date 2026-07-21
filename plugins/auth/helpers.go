package auth

import (
	"encoding/json"
	"errors"
	"strings"
	"unicode/utf8"
)

var (
	errPasswordTooShort = errors.New("auth: password must be at least 8 characters")
	errPasswordTooLong  = errors.New("auth: password must be at most 512 characters")
)

func normalizeEmail(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

func parseRoles(v any) []string {
	var raw string
	switch t := v.(type) {
	case string:
		raw = t
	case []byte:
		raw = string(t)
	default:
		return []string{}
	}
	var roles []string
	if err := json.Unmarshal([]byte(raw), &roles); err != nil {
		return []string{}
	}
	return roles
}

func parseJSONMap(v any) map[string]any {
	var raw string
	switch t := v.(type) {
	case string:
		raw = t
	case []byte:
		raw = string(t)
	default:
		return map[string]any{}
	}
	out := map[string]any{}
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return map[string]any{}
	}
	return out
}

// userView returns a copy of a raw auth_users row safe for workflow output:
// password_hash removed, roles/metadata decoded.
func userView(row map[string]any) map[string]any {
	out := make(map[string]any, len(row))
	for k, v := range row {
		if k == "password_hash" {
			continue
		}
		out[k] = v
	}
	out["roles"] = parseRoles(row["roles"])
	out["metadata"] = parseJSONMap(row["metadata"])
	return out
}

// validatePassword counts runes, matching the code-point semantics of the
// scaffolded routes' JSON-Schema minLength/maxLength — the two layers must
// agree or a schema-passing password fails here after side effects ran.
func validatePassword(pw string) error {
	n := utf8.RuneCountInString(pw)
	if n < 8 {
		return errPasswordTooShort
	}
	if n > 512 {
		return errPasswordTooLong
	}
	return nil
}
