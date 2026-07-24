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

	failing, passCount, messages := runHurl(t, projectDir, baseURL)
	baseline := loadBaseline(t, filepath.Join(projectDir, "harness", "known-failing.json"))

	// Log every failing key now that the baseline is known, so a GREEN run
	// only prints "FAIL" for genuine (unexpected) failures — documented
	// gaps are logged as "known-gap" instead, not mislabeled as failures.
	failingKeys := make([]string, 0, len(failing))
	for k := range failing {
		failingKeys = append(failingKeys, k)
	}
	sort.Strings(failingKeys)
	for _, k := range failingKeys {
		if baseline[k] {
			t.Logf("known-gap %s:\n%s", k, messages[k])
		} else {
			t.Logf("FAIL %s:\n%s", k, messages[k])
		}
	}

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
func runHurl(t *testing.T, projectDir, baseURL string) (failing map[string]bool, passCount int, messages map[string]string) {
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
	//nolint:gosec // G204: the binary is the constant "hurl"; args are test-controlled
	// (vendored harness/hurl/*.hurl paths, a t.TempDir report dir, and the local
	// listener URL this test just started) — no external input reaches this call.
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
