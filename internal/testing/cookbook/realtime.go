package cookbook

import (
	"bufio"
	"encoding/json"
	"fmt"
	"strings"
)

// nextSSEData reads the next data: event payload from an SSE stream,
// skipping comments (heartbeats) and blank lines.
func nextSSEData(r *bufio.Reader) (string, error) {
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return "", fmt.Errorf("sse read: %w", err)
		}
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, ":") {
			continue
		}
		if data, ok := strings.CutPrefix(line, "data:"); ok {
			return strings.TrimSpace(data), nil
		}
	}
}

// matchMessage reports whether a JSON message satisfies all assertions.
// A failed assertion is a non-match (skip); undecodable JSON is an error.
func matchMessage(raw []byte, assertions []BodyAssertion) (bool, error) {
	var doc any
	if err := json.Unmarshal(raw, &doc); err != nil {
		return false, fmt.Errorf("message is not JSON (%.200s): %w", raw, err)
	}
	for _, a := range assertions {
		if err := CheckAssertion(doc, a); err != nil {
			return false, nil
		}
	}
	return true, nil
}
