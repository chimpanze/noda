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
	Deps  []string          `json:"deps"`
	Seed  map[string]string `json:"seed,omitempty"`
	Steps []Step            `json:"steps"`
}

// Step is one ordered action: an HTTP request/expect pair, or a mail assertion.
type Step struct {
	Name    string            `json:"name"`
	Request RequestSpec       `json:"request,omitempty"`
	Expect  ExpectSpec        `json:"expect,omitempty"`
	Capture map[string]string `json:"capture,omitempty"`
	Mail    *MailExpect       `json:"mail,omitempty"`
}

// RequestSpec describes the HTTP request to send.
type RequestSpec struct {
	Method    string            `json:"method,omitempty"`
	Path      string            `json:"path,omitempty"`
	Headers   map[string]string `json:"headers,omitempty"`
	Body      any               `json:"body,omitempty"`
	Multipart *MultipartSpec    `json:"multipart,omitempty"`
}

// MultipartSpec builds a multipart/form-data request body.
type MultipartSpec struct {
	Fields map[string]string `json:"fields,omitempty"`
	Files  []FilePart        `json:"files,omitempty"`
}

// FilePart is one file in a multipart request. Exactly one of Content /
// ContentBase64 must be set. Field defaults to "file".
type FilePart struct {
	Field         string `json:"field,omitempty"`
	Filename      string `json:"filename"`
	ContentType   string `json:"content_type,omitempty"`
	Content       string `json:"content,omitempty"`
	ContentBase64 string `json:"content_base64,omitempty"`
}

// MailExpect asserts a message arrived in the Mailpit inbox.
type MailExpect struct {
	To        string `json:"to"`
	Subject   string `json:"subject"`
	BodyRegex string `json:"body_regex,omitempty"`
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
		isMail := st.Mail != nil
		isRequest := st.Request.Method != "" || st.Request.Path != "" || st.Request.Body != nil || st.Request.Multipart != nil
		switch {
		case isMail && isRequest:
			return nil, fmt.Errorf("cookbook: %s: step %q: mail and request are mutually exclusive", path, st.Name)
		case isMail:
			if st.Mail.To == "" || st.Mail.Subject == "" {
				return nil, fmt.Errorf("cookbook: %s: step %q: mail requires to and subject", path, st.Name)
			}
			if st.Expect.Status != 0 || len(st.Capture) > 0 {
				return nil, fmt.Errorf("cookbook: %s: step %q: mail steps take no expect/capture", path, st.Name)
			}
		case isRequest:
			if st.Request.Method == "" {
				return nil, fmt.Errorf("cookbook: %s: step %q: missing request.method", path, st.Name)
			}
			if st.Request.Path == "" {
				return nil, fmt.Errorf("cookbook: %s: step %q: missing request.path", path, st.Name)
			}
			if st.Expect.Status == 0 {
				return nil, fmt.Errorf("cookbook: %s: step %q: missing expect.status", path, st.Name)
			}
			if st.Request.Multipart != nil && st.Request.Body != nil {
				return nil, fmt.Errorf("cookbook: %s: step %q: multipart and body are mutually exclusive", path, st.Name)
			}
			if st.Request.Multipart != nil {
				for _, f := range st.Request.Multipart.Files {
					has := 0
					if f.Content != "" {
						has++
					}
					if f.ContentBase64 != "" {
						has++
					}
					if has != 1 {
						return nil, fmt.Errorf("cookbook: %s: step %q: file %q needs exactly one of content/content_base64", path, st.Name, f.Filename)
					}
				}
			}
		default:
			return nil, fmt.Errorf("cookbook: %s: step %q: needs a request or a mail block", path, st.Name)
		}
	}
	return &s, nil
}
