//go:build integration

package realworld

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/chimpanze/noda/internal/testing/containers"
	"github.com/chimpanze/noda/internal/testing/cookbook"
	"github.com/chimpanze/noda/plugins/all"
)

// TestRealWorldConformance boots examples/realworld against a real Postgres
// testcontainer, runs the vendored upstream RealWorld/Conduit Hurl suite
// (examples/realworld/harness/hurl) against it, and asserts the resulting
// failing-set matches harness/known-failing.json exactly — no more (a
// regression), no less (a documented gap that's since been fixed and needs
// its baseline entry removed).
func TestRealWorldConformance(t *testing.T) {
	if _, err := exec.LookPath("hurl"); err != nil {
		t.Skip("hurl not installed (brew install hurl / see Task 12 for CI)")
	}
	projectDir, err := filepath.Abs("../../../examples/realworld")
	require.NoError(t, err)

	dbURL := containers.StartPostgres(t)
	t.Setenv("DATABASE_URL", dbURL)
	t.Setenv("JWT_SECRET", "test-secret-please-change-0123456789abcdef")

	baseURL, stop := cookbook.BootListen(t, projectDir, all.All())
	defer stop()

	failing, passCount := runHurl(t, projectDir, baseURL)
	baseline := loadBaseline(t, filepath.Join(projectDir, "harness", "known-failing.json"))

	newFailures := diff(failing, baseline) // failing but not documented
	fixedGaps := diff(baseline, failing)   // documented but now passing

	require.Empty(t, newFailures, "NEW failures (regressions) — fix the app or document as a gap:\n%s", strings.Join(newFailures, "\n"))
	require.Empty(t, fixedGaps, "baseline gaps that now PASS — remove from known-failing.json + close the issue:\n%s", strings.Join(fixedGaps, "\n"))

	t.Logf("conformance: %d passed, %d known-gap, 0 unexpected", passCount, len(baseline))
}

// runHurl runs the entire vendored Hurl suite as a single `hurl --test`
// invocation (Hurl shares no state across files by design, but --jobs 1
// keeps ordering deterministic and output easy to correlate) against
// baseURL, with a uid unique to this run since the suite builds
// emails/usernames from it and those hit unique constraints across reruns
// against the same (fresh, per-run) database.
func runHurl(t *testing.T, projectDir, baseURL string) (failing map[string]bool, passCount int) {
	t.Helper()
	hurlDir := filepath.Join(projectDir, "harness", "hurl")
	files, err := filepath.Glob(filepath.Join(hurlDir, "*.hurl"))
	require.NoError(t, err)
	require.NotEmpty(t, files)
	sort.Strings(files) // deterministic order across OSes/filesystems

	reportDir := t.TempDir()
	uid := strconv.FormatInt(time.Now().UnixNano(), 10)

	args := []string{
		"--test", "--jobs", "1",
		"--variable", "host=" + baseURL,
		"--variable", "uid=" + uid,
		"--report-json", reportDir,
	}
	args = append(args, files...)
	cmd := exec.Command("hurl", args...)
	cmd.Stderr = os.Stderr
	// Hurl exits non-zero when any test fails — expected here. Parse the
	// report regardless of exit status.
	_ = cmd.Run()

	return parseHurlReport(t, reportDir)
}

// loadBaseline reads harness/known-failing.json's "gaps" array into a set.
func loadBaseline(t *testing.T, path string) map[string]bool {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	var f struct {
		Gaps []string `json:"gaps"`
	}
	require.NoError(t, json.Unmarshal(data, &f))
	m := make(map[string]bool, len(f.Gaps))
	for _, g := range f.Gaps {
		m[g] = true
	}
	return m
}

// diff returns the keys present in a but not in b, sorted.
func diff(a, b map[string]bool) []string {
	var out []string
	for k := range a {
		if !b[k] {
			out = append(out, k)
		}
	}
	sort.Strings(out)
	return out
}
