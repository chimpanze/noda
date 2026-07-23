//go:build integration

package realworld

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Hurl 8.0.1's `--report-json <dir>` writes `<dir>/report.json`: a JSON
// ARRAY of per-file objects, inspected directly against the installed
// binary (see internal/testing/realworld's dev notes / task-10 report — run
// `hurl --test --report-json <dir> some.hurl` and read the file to
// reproduce). Each element is shaped like:
//
//	{
//	  "filename": "articles.hurl",
//	  "success": false,
//	  "time": 617,
//	  "cookies": [...],
//	  "entries": [
//	    {
//	      "index": 1,        // 1-based position of this entry within the file — STABLE
//	      "line": 1,         // source line of the entry's request — STABLE
//	      "curl_cmd": "...", // NOT stable: includes substituted variables (uid, tokens)
//	      "asserts": [
//	        {"line": 2, "success": true},
//	        {"line": 2, "success": false, "message": "Assert status code\n --> ...\n   |\n   | GET ...\n 7 | HTTP 200\n   |      ^^^ actual value is <404>\n   |"}
//	      ],
//	      "captures": [...],
//	      "calls": [...]
//	    }
//	  ]
//	}
//
// There is no per-entry "success" field; an entry is derived as failing iff
// any of its asserts has success:false. Hurl STOPS running a file's
// remaining entries after the first entry failure (verified empirically: a
// 3-entry file with entry 2 failing reports only 2 entries, never 3) — each
// .hurl file is a stateful chain, so this is expected and does not need
// special-casing: entries after a failure simply never appear in the
// report, and re-running after a fix lets later entries execute (and
// potentially reveal further failures) for the first time.
//
// "index" and "line" are structural properties of the .hurl SOURCE file and
// do not depend on any per-run variable (uid, captured tokens, timestamps),
// so they are STABLE across runs and are used as the failing-set key
// granularity, the finest stable identifier this report schema exposes:
//
//	"<basename> :: entry <index> (line <line>)"
//
// The file-level "success" field is a belt-and-suspenders cross-check, not
// the primary signal: a file can fail for a reason that never produces a
// failing assert (a capture error, a runtime/network error before any
// assert runs) — the assert-derived logic above would silently count that
// as passing. If a file reports success:false but none of its entries
// produced a failing-assert key, a synthetic key is added so the failure
// can't be swallowed:
//
//	"<basename> :: file-level failure (no failing assert)"
type hurlReportFile struct {
	Filename string            `json:"filename"`
	Success  bool              `json:"success"`
	Entries  []hurlReportEntry `json:"entries"`
}

type hurlReportEntry struct {
	Index   int                `json:"index"`
	Line    int                `json:"line"`
	Asserts []hurlReportAssert `json:"asserts"`
}

type hurlReportAssert struct {
	Line    int    `json:"line"`
	Success bool   `json:"success"`
	Message string `json:"message"`
}

// parseHurlReport reads <reportDir>/report.json and returns the set of
// failing entry keys, the count of entries that passed, and (keyed by
// failing entry key) Hurl's captured assertion-failure message(s) so the
// caller can log them once it knows which keys are documented gaps vs
// genuine regressions (see conformance_integration_test.go) — this function
// deliberately does not log, since it doesn't have the baseline.
func parseHurlReport(t *testing.T, reportDir string) (failing map[string]bool, passCount int, messages map[string]string) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(reportDir, "report.json"))
	if err != nil {
		t.Fatalf("read hurl report: %v", err)
	}
	var files []hurlReportFile
	if err := json.Unmarshal(data, &files); err != nil {
		t.Fatalf("parse hurl report (schema drift? see hurlreport.go doc comment): %v", err)
	}

	failing = map[string]bool{}
	messages = map[string]string{}
	for _, f := range files {
		base := filepath.Base(f.Filename)
		fileHasFailingAssert := false
		for _, e := range f.Entries {
			key := fmt.Sprintf("%s :: entry %d (line %d)", base, e.Index, e.Line)
			var msgs []string
			for _, a := range e.Asserts {
				if !a.Success {
					if a.Message != "" {
						msgs = append(msgs, a.Message)
					} else {
						msgs = append(msgs, fmt.Sprintf("assert at line %d failed (no message)", a.Line))
					}
				}
			}
			if len(msgs) > 0 {
				failing[key] = true
				fileHasFailingAssert = true
				messages[key] = strings.Join(msgs, "\n---\n")
			} else {
				passCount++
			}
		}
		// Cross-check against the file-level success field: a file can fail
		// for a reason that produces no failing assert (capture error,
		// runtime/network error before any assert runs). Without this, such
		// a failure is silently swallowed and the entry miscounted as
		// passing — a false GREEN. See the doc comment above.
		if !f.Success && !fileHasFailingAssert {
			key := fmt.Sprintf("%s :: file-level failure (no failing assert)", base)
			failing[key] = true
			messages[key] = "Hurl reported success:false for this file but no entry had a failing assert (capture error or runtime error?) — inspect the file's entries/captures directly."
		}
	}
	return failing, passCount, messages
}
