package testing

import (
	"fmt"
	"strings"
)

// FormatResults formats test results for CLI output.
func FormatResults(suiteResults []SuiteResult, verbose bool) string {
	var b strings.Builder
	var totalPassed, totalFailed int

	for _, sr := range suiteResults {
		fmt.Fprintf(&b, "\n  Workflow: %s\n", sr.Suite.Workflow)

		for _, r := range sr.Results {
			if r.Passed {
				totalPassed++
				fmt.Fprintf(&b, "    ✓ %s (%s)\n", r.CaseName, r.Duration.Round(100*1000)) // round to 100µs
				if verbose {
					formatTrace(&b, r)
				}
			} else {
				totalFailed++
				fmt.Fprintf(&b, "    ✗ %s (%s)\n", r.CaseName, r.Duration.Round(100*1000))
				fmt.Fprintf(&b, "      %s\n", r.Error)
				if verbose {
					formatTrace(&b, r)
				}
			}
		}
	}

	total := totalPassed + totalFailed
	fmt.Fprintf(&b, "\n  %d passed, %d failed, %d total\n", totalPassed, totalFailed, total)

	return b.String()
}

func formatTrace(b *strings.Builder, r TestResult) {
	if len(r.Trace) == 0 {
		return
	}
	fmt.Fprintf(b, "      Trace:\n")
	for _, t := range r.Trace {
		fmt.Fprintf(b, "        %s (%s) → %s [%s]\n", t.NodeID, t.Type, t.Output, t.Duration)
	}
}
