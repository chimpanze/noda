package cookbook

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// LookupPath resolves a dot-separated path in a decoded JSON document.
// Numeric segments index arrays. Returns (nil, false) when any segment is
// missing or of the wrong shape.
func LookupPath(doc any, path string) (any, bool) {
	cur := doc
	for _, seg := range strings.Split(path, ".") {
		switch v := cur.(type) {
		case map[string]any:
			next, ok := v[seg]
			if !ok {
				return nil, false
			}
			cur = next
		case []any:
			idx, err := strconv.Atoi(seg)
			if err != nil || idx < 0 || idx >= len(v) {
				return nil, false
			}
			cur = v[idx]
		default:
			return nil, false
		}
	}
	return cur, true
}

// CheckAssertion evaluates one body assertion against a decoded JSON document.
func CheckAssertion(doc any, a BodyAssertion) error {
	matchers := 0
	if a.Equals != nil {
		matchers++
	}
	if a.Regex != "" {
		matchers++
	}
	if a.Exists != nil {
		matchers++
	}
	if a.Type != "" {
		matchers++
	}
	if matchers != 1 {
		return fmt.Errorf("assertion on %q: exactly one of equals/regex/exists/type required, got %d", a.Path, matchers)
	}

	val, found := LookupPath(doc, a.Path)

	if a.Exists != nil {
		if *a.Exists != found {
			return fmt.Errorf("assertion on %q: exists=%v but found=%v", a.Path, *a.Exists, found)
		}
		return nil
	}
	if !found {
		return fmt.Errorf("assertion on %q: path not found", a.Path)
	}

	switch {
	case a.Equals != nil:
		// Normalize both sides through JSON so 3 (int) equals 3.0 (float64).
		want, err := json.Marshal(a.Equals)
		if err != nil {
			return fmt.Errorf("assertion on %q: marshal expected: %w", a.Path, err)
		}
		got, err := json.Marshal(val)
		if err != nil {
			return fmt.Errorf("assertion on %q: marshal actual: %w", a.Path, err)
		}
		if string(want) != string(got) {
			return fmt.Errorf("assertion on %q: expected %s, got %s", a.Path, want, got)
		}
	case a.Regex != "":
		s, ok := val.(string)
		if !ok {
			return fmt.Errorf("assertion on %q: regex needs a string, got %T", a.Path, val)
		}
		re, err := regexp.Compile(a.Regex)
		if err != nil {
			return fmt.Errorf("assertion on %q: bad regex: %w", a.Path, err)
		}
		if !re.MatchString(s) {
			return fmt.Errorf("assertion on %q: %q does not match /%s/", a.Path, s, a.Regex)
		}
	case a.Type != "":
		if err := checkType(val, a.Type); err != nil {
			return fmt.Errorf("assertion on %q: %w", a.Path, err)
		}
	}
	return nil
}

func checkType(val any, want string) error {
	ok := false
	switch want {
	case "string":
		_, ok = val.(string)
	case "number":
		_, ok = val.(float64)
	case "boolean":
		_, ok = val.(bool)
	case "object":
		_, ok = val.(map[string]any)
	case "array":
		_, ok = val.([]any)
	case "null":
		ok = val == nil
	default:
		return fmt.Errorf("unknown type matcher %q", want)
	}
	if !ok {
		return fmt.Errorf("expected type %s, got %T", want, val)
	}
	return nil
}

var varRef = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)\}`)

// Substitute replaces ${name} references with captured variable values.
// Unknown names are left intact so failures show what was missing.
func Substitute(s string, vars map[string]string) string {
	return varRef.ReplaceAllStringFunc(s, func(m string) string {
		name := varRef.FindStringSubmatch(m)[1]
		if v, ok := vars[name]; ok {
			return v
		}
		return m
	})
}

// Capture extracts values from a decoded JSON response body into vars.
// Spec values must be "body.<path>".
func Capture(doc any, spec map[string]string, vars map[string]string) error {
	for name, src := range spec {
		path, ok := strings.CutPrefix(src, "body.")
		if !ok {
			return fmt.Errorf("capture %q: source %q must start with \"body.\"", name, src)
		}
		val, found := LookupPath(doc, path)
		if !found {
			return fmt.Errorf("capture %q: path %q not found in response body", name, src)
		}
		switch v := val.(type) {
		case string:
			vars[name] = v
		case float64:
			vars[name] = strconv.FormatFloat(v, 'f', -1, 64)
		case bool:
			vars[name] = strconv.FormatBool(v)
		default:
			return fmt.Errorf("capture %q: value at %q is %T; only scalars can be captured", name, src, val)
		}
	}
	return nil
}
