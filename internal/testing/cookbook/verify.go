// Package cookbook loads and executes examples/node-cookbook verify.json
// suites against a real in-process Noda server.
package cookbook

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
)

// Suite is one cookbook project's verification file (verify.json).
type Suite struct {
	Deps  []string `json:"deps"`
	Steps []Step   `json:"steps"`
}

// Step is one ordered request/expect pair.
type Step struct {
	Name    string            `json:"name"`
	Request RequestSpec       `json:"request"`
	Expect  ExpectSpec        `json:"expect"`
	Capture map[string]string `json:"capture,omitempty"`
}

// RequestSpec describes the HTTP request to send.
type RequestSpec struct {
	Method  string            `json:"method"`
	Path    string            `json:"path"`
	Headers map[string]string `json:"headers,omitempty"`
	Body    any               `json:"body,omitempty"`
}

// ExpectSpec describes the assertions on the response.
type ExpectSpec struct {
	Status   int               `json:"status"`
	Headers  map[string]string `json:"headers,omitempty"`
	Body     []BodyAssertion   `json:"body,omitempty"`
	BodyText *string           `json:"body_text,omitempty"`
}

// BodyAssertion checks one path in the JSON response body. Exactly one of
// Equals, Regex, Exists, Type must be set.
// Assert a JSON null via Type: "null" — an explicit "equals": null reads as matcher-unset.
type BodyAssertion struct {
	Path   string `json:"path"`
	Equals any    `json:"equals,omitempty"`
	Regex  string `json:"regex,omitempty"`
	Exists *bool  `json:"exists,omitempty"`
	Type   string `json:"type,omitempty"`
}

// LoadSuite reads and validates a verify.json file.
func LoadSuite(path string) (*Suite, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("cookbook: reading %s: %w", path, err)
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	var s Suite
	if err := dec.Decode(&s); err != nil {
		return nil, fmt.Errorf("cookbook: parsing %s: %w", path, err)
	}
	if len(s.Steps) == 0 {
		return nil, fmt.Errorf("cookbook: %s: no steps", path)
	}
	for i, st := range s.Steps {
		if st.Name == "" {
			return nil, fmt.Errorf("cookbook: %s: step %d: missing name", path, i)
		}
		if st.Request.Method == "" {
			return nil, fmt.Errorf("cookbook: %s: step %q: missing request.method", path, st.Name)
		}
		if st.Request.Path == "" {
			return nil, fmt.Errorf("cookbook: %s: step %q: missing request.path", path, st.Name)
		}
		if st.Expect.Status == 0 {
			return nil, fmt.Errorf("cookbook: %s: step %q: missing expect.status", path, st.Name)
		}
	}
	return &s, nil
}
