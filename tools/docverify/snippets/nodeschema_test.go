package main

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// docsRoot resolves the docs/ directory relative to this test file rather
// than the process's working directory, since go test's cwd is the package
// directory (tools/docverify/snippets), not the repo root.
func docsRoot(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed to resolve test file path")
	}
	root := filepath.Join(filepath.Dir(thisFile), "..", "..", "..", "docs")
	if info, err := os.Stat(root); err != nil || !info.IsDir() {
		t.Fatalf("docs directory not found at %q (resolved from test file location): %v", root, err)
	}
	return root
}

// TestNodeSchemaGate is the CI gate for #445: every fenced json workflow
// block in the user-facing docs must pass both registry.ValidateStartupDryRun
// (node ConfigSchemas) and engine.Compile (graph-level rules). See the
// package-level comment on nodeSchemaViolations for why both checks are
// required — a ConfigSchema-only gate missed the retry-on-success-edge bug
// that this gate exists to catch.
func TestNodeSchemaGate(t *testing.T) {
	root := docsRoot(t)
	files, err := markdownFiles(root)
	if err != nil {
		t.Fatalf("markdownFiles(%q): %v", root, err)
	}
	if len(files) == 0 {
		t.Fatalf("no markdown files found under %q — docs root resolution is broken", root)
	}

	violations, counts, err := nodeSchemaViolations(files)
	if err != nil {
		t.Fatalf("nodeSchemaViolations: %v", err)
	}

	// Coverage floors. A refactor that breaks block extraction (a change to
	// the ```json fence convention, the "nodes" detection, or the node
	// fragment shapes) must fail here rather than silently scan a handful of
	// blocks and report a spuriously clean gate. "scanned > 0" is far too weak
	// for that: extraction that only survived for unindented ```json fences
	// would drop coverage by two orders of magnitude and stay green.
	//
	// As of 2026-07-26 the gate scans 243 blocks: 48 workflow-shaped and 195
	// node fragments, across 117 files. The floors are set just under those,
	// so ordinary doc churn does not trip them but a structural regression
	// does. Raise them when the docs grow.
	const (
		minFiles     = 100
		minWorkflow  = 40
		minFragments = 180
	)
	if len(files) < minFiles {
		t.Errorf("scanned %d markdown files, want >= %d — file discovery is broken, not the docs", len(files), minFiles)
	}
	if counts.Workflow < minWorkflow {
		t.Errorf("scanned %d workflow-shaped blocks, want >= %d — block extraction is broken, not the docs", counts.Workflow, minWorkflow)
	}
	if counts.Fragment < minFragments {
		t.Errorf("scanned %d node-fragment blocks, want >= %d — fragment detection is broken, not the docs", counts.Fragment, minFragments)
	}
	t.Logf("scanned %d blocks (%d workflow-shaped, %d node fragments, %d ignored) across %d files",
		counts.Total(), counts.Workflow, counts.Fragment, counts.Ignored, len(files))

	if len(violations) > 0 {
		t.Errorf("%d doc snippet(s) failed node/graph validation:", len(violations))
		for _, v := range violations {
			t.Errorf("  %s", v)
		}
	}
}
