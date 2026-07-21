// Package cookbook loads and executes examples/node-cookbook verify.json
// suites against a real in-process Noda server.
package cookbook

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"time"
)

// Suite is one cookbook project's verification file (verify.json).
type Suite struct {
	Listen bool              `json:"listen,omitempty"`
	Deps   []string          `json:"deps"`
	Seed   map[string]string `json:"seed,omitempty"`
	Steps  []Step            `json:"steps"`
}

// Step is one ordered action: an HTTP request/expect pair, or a mail assertion, or a WebSocket/SSE interaction.
type Step struct {
	Name string `json:"name"`
	// omitzero, not omitempty: omitempty has no effect on a non-pointer
	// struct field. These are only ever unmarshalled today, so this is a
	// no-op in practice — it is correct rather than misleading if a Suite
	// is ever marshalled.
	Request RequestSpec       `json:"request,omitzero"`
	Expect  ExpectSpec        `json:"expect,omitzero"`
	Capture map[string]string `json:"capture,omitempty"`
	Mail    *MailExpect       `json:"mail,omitempty"`
	WS      *WSStep           `json:"ws,omitempty"`
	SSE     *SSEStep          `json:"sse,omitempty"`
}

// RequestSpec describes the HTTP request to send.
type RequestSpec struct {
	Method       string            `json:"method,omitempty"`
	Path         string            `json:"path,omitempty"`
	Headers      map[string]string `json:"headers,omitempty"`
	Body         any               `json:"body,omitempty"`
	Multipart    *MultipartSpec    `json:"multipart,omitempty"`
	RetryTimeout string            `json:"retry_timeout,omitempty"`
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

// WSStep is one WebSocket action on a named client: connect, send, or expect.
type WSStep struct {
	Client  string          `json:"client"`
	Connect string          `json:"connect,omitempty"`
	Send    any             `json:"send,omitempty"`
	Expect  []BodyAssertion `json:"expect,omitempty"`
}

// SSEStep is one SSE action on a named client: connect or expect.
type SSEStep struct {
	Client  string          `json:"client"`
	Connect string          `json:"connect,omitempty"`
	Expect  []BodyAssertion `json:"expect,omitempty"`
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
// ${var} substitution applies to Equals string values only (not Regex/BodyText).
type BodyAssertion struct {
	Path   string `json:"path"`
	Equals any    `json:"equals,omitempty"`
	Regex  string `json:"regex,omitempty"`
	Exists *bool  `json:"exists,omitempty"`
	Type   string `json:"type,omitempty"`
}

// hasTopLevelExpectOrCapture reports whether a step sets any top-level
// expect assertion or capture block (invalid on ws/sse steps).
func hasTopLevelExpectOrCapture(st Step) bool {
	return st.Expect.Status != 0 || len(st.Expect.Body) > 0 || st.Expect.BodyText != nil || len(st.Expect.Headers) > 0 || len(st.Capture) > 0
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
		isRequest := st.Request.Method != "" || st.Request.Path != "" || st.Request.Body != nil || st.Request.Multipart != nil || st.Request.RetryTimeout != ""
		isWS := st.WS != nil
		isSSE := st.SSE != nil

		// Count how many kinds are present
		kindCount := 0
		if isMail {
			kindCount++
		}
		if isRequest {
			kindCount++
		}
		if isWS {
			kindCount++
		}
		if isSSE {
			kindCount++
		}

		// More than one kind is an error
		if kindCount > 1 {
			return nil, fmt.Errorf("cookbook: %s: step %q: request/mail/ws/sse are mutually exclusive", path, st.Name)
		}

		// ws/sse steps only allowed when Suite.Listen is true
		if (isWS || isSSE) && !s.Listen {
			return nil, fmt.Errorf("cookbook: %s: step %q: ws/sse steps require suite.listen=true", path, st.Name)
		}

		switch {
		case isMail:
			if st.Mail.To == "" || st.Mail.Subject == "" {
				return nil, fmt.Errorf("cookbook: %s: step %q: mail requires to and subject", path, st.Name)
			}
			if st.Expect.Status != 0 || len(st.Capture) > 0 {
				return nil, fmt.Errorf("cookbook: %s: step %q: mail steps take no expect/capture", path, st.Name)
			}
			// Mail steps reject body/headers/body_text assertions
			if len(st.Expect.Body) > 0 || len(st.Expect.Headers) > 0 || st.Expect.BodyText != nil {
				return nil, fmt.Errorf("cookbook: %s: step %q: mail steps do not support body/headers/body_text assertions", path, st.Name)
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
			// Validate retry_timeout format if present
			if st.Request.RetryTimeout != "" {
				if _, err := time.ParseDuration(st.Request.RetryTimeout); err != nil {
					return nil, fmt.Errorf("cookbook: %s: step %q: invalid retry_timeout %q: %w", path, st.Name, st.Request.RetryTimeout, err)
				}
			}
		case isWS:
			if hasTopLevelExpectOrCapture(st) {
				return nil, fmt.Errorf("cookbook: %s: step %q: ws/sse steps take no top-level expect/capture (assertions go inside the ws/sse block)", path, st.Name)
			}
			if st.WS.Client == "" {
				return nil, fmt.Errorf("cookbook: %s: step %q: ws requires client", path, st.Name)
			}
			// Exactly one of Connect/Send/Expect
			actionCount := 0
			if st.WS.Connect != "" {
				actionCount++
			}
			if st.WS.Send != nil {
				actionCount++
			}
			if len(st.WS.Expect) > 0 {
				actionCount++
			}
			if actionCount != 1 {
				return nil, fmt.Errorf("cookbook: %s: step %q: ws requires exactly one of connect/send/expect", path, st.Name)
			}
		case isSSE:
			if hasTopLevelExpectOrCapture(st) {
				return nil, fmt.Errorf("cookbook: %s: step %q: ws/sse steps take no top-level expect/capture (assertions go inside the ws/sse block)", path, st.Name)
			}
			if st.SSE.Client == "" {
				return nil, fmt.Errorf("cookbook: %s: step %q: sse requires client", path, st.Name)
			}
			// Exactly one of Connect/Expect
			actionCount := 0
			if st.SSE.Connect != "" {
				actionCount++
			}
			if len(st.SSE.Expect) > 0 {
				actionCount++
			}
			if actionCount != 1 {
				return nil, fmt.Errorf("cookbook: %s: step %q: sse requires exactly one of connect/expect", path, st.Name)
			}
		default:
			return nil, fmt.Errorf("cookbook: %s: step %q: needs a request, mail, ws, or sse block", path, st.Name)
		}
	}
	return &s, nil
}
