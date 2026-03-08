package config

import (
	"fmt"
	"strings"
)

// FormatErrors formats validation errors for CLI display, grouped by file.
// Returns empty string when there are no errors.
func FormatErrors(errs []ValidationError) string {
	if len(errs) == 0 {
		return ""
	}

	// Group by file path
	grouped := make(map[string][]ValidationError)
	var fileOrder []string
	for _, e := range errs {
		if _, seen := grouped[e.FilePath]; !seen {
			fileOrder = append(fileOrder, e.FilePath)
		}
		grouped[e.FilePath] = append(grouped[e.FilePath], e)
	}

	var b strings.Builder
	for _, filePath := range fileOrder {
		fileErrs := grouped[filePath]
		fmt.Fprintf(&b, "\n%s\n", filePath)
		for _, e := range fileErrs {
			if e.JSONPath != "" {
				fmt.Fprintf(&b, "  %s: %s\n", e.JSONPath, e.Message)
			} else {
				fmt.Fprintf(&b, "  %s\n", e.Message)
			}
		}
	}

	fileCount := len(grouped)
	fmt.Fprintf(&b, "\n%d error(s) in %d file(s)\n", len(errs), fileCount)

	return b.String()
}
