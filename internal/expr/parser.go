// Package expr implements the Noda expression engine for parsing, compiling,
// and evaluating {{ }} delimited expressions in config values.
package expr

import (
	"fmt"
	"strings"
)

// SegmentType distinguishes literal text from expression segments.
type SegmentType int

const (
	SegmentLiteral    SegmentType = iota // Plain text
	SegmentExpression                    // {{ expr }} content (without delimiters)
)

// Segment is a portion of a parsed expression string.
type Segment struct {
	Type  SegmentType
	Value string // literal text or expression text (trimmed)
}

// ParsedExpression is the result of parsing a string that may contain {{ }} expressions.
type ParsedExpression struct {
	Raw       string    // the original input string
	Segments  []Segment // ordered literal and expression segments
	IsLiteral bool      // true if no {{ }} found
	IsSimple  bool      // true if entire string is one {{ }} (no surrounding text)
}

// Parse extracts {{ }} delimited expressions from a string and classifies the result.
func Parse(input string) (*ParsedExpression, error) {
	pe := &ParsedExpression{Raw: input}

	if !strings.Contains(input, "{{") {
		pe.IsLiteral = true
		pe.Segments = []Segment{{Type: SegmentLiteral, Value: input}}
		return pe, nil
	}

	remaining := input
	for len(remaining) > 0 {
		openIdx := strings.Index(remaining, "{{")
		if openIdx == -1 {
			// Rest is literal
			if remaining != "" {
				pe.Segments = append(pe.Segments, Segment{Type: SegmentLiteral, Value: remaining})
			}
			break
		}

		// Add literal before the {{
		if openIdx > 0 {
			pe.Segments = append(pe.Segments, Segment{Type: SegmentLiteral, Value: remaining[:openIdx]})
		}

		// Find matching }} accounting for nested braces
		exprStart := openIdx + 2
		closeIdx := findClosingBraces(remaining[exprStart:])
		if closeIdx == -1 {
			pos := len(input) - len(remaining) + openIdx
			return nil, fmt.Errorf("unclosed expression delimiter at position %d: %s", pos, input)
		}

		exprText := strings.TrimSpace(remaining[exprStart : exprStart+closeIdx])
		if exprText == "" {
			pos := len(input) - len(remaining) + openIdx
			return nil, fmt.Errorf("empty expression at position %d", pos)
		}

		pe.Segments = append(pe.Segments, Segment{Type: SegmentExpression, Value: exprText})
		remaining = remaining[exprStart+closeIdx+2:]
	}

	// Classify
	if len(pe.Segments) == 1 && pe.Segments[0].Type == SegmentExpression {
		pe.IsSimple = true
	}

	return pe, nil
}

// findClosingBraces finds the index of the closing }} in s, accounting for
// nested braces (e.g., map literals like {key: value}).
// Returns -1 if no closing }} is found.
func findClosingBraces(s string) int {
	depth := 0
	inString := false
	var stringChar byte

	for i := 0; i < len(s); i++ {
		ch := s[i]

		// Handle string literals
		if inString {
			if ch == '\\' && i+1 < len(s) {
				i++ // skip escaped char
				continue
			}
			if ch == stringChar {
				inString = false
			}
			continue
		}

		if ch == '"' || ch == '\'' {
			inString = true
			stringChar = ch
			continue
		}

		if ch == '{' {
			depth++
			continue
		}

		if ch == '}' {
			if depth > 0 {
				depth--
				continue
			}
			// At depth 0, check for }}
			if i+1 < len(s) && s[i+1] == '}' {
				return i
			}
			// Single } at depth 0 - might be syntax error, let expr handle it
			continue
		}
	}

	return -1
}
