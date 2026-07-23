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
// failing entry keys plus the count of entries that passed. For every
// failing entry it also logs (via t.Logf) the key and Hurl's captured
// assertion-failure message(s), so a run's `-v` output carries what Task 11
// needs to write accurate FINDINGS rows.
func parseHurlReport(t *testing.T, reportDir string) (failing map[string]bool, passCount int) {
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
	for _, f := range files {
		base := filepath.Base(f.Filename)
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
				t.Logf("FAIL %s:\n%s", key, strings.Join(msgs, "\n---\n"))
			} else {
				passCount++
			}
		}
	}
	return failing, passCount
}
