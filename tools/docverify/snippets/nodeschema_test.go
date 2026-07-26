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

	violations, scanned, err := nodeSchemaViolations(files)
	if err != nil {
		t.Fatalf("nodeSchemaViolations: %v", err)
	}

	// A future refactor that breaks block extraction (e.g. changes the
	// ```json fence convention or the "nodes" detection) must not silently
	// scan zero blocks and report a spuriously clean gate.
	if scanned == 0 {
		t.Fatal("scanned 0 workflow-shaped blocks — block extraction is broken, not the docs")
	}
	t.Logf("scanned %d workflow-shaped blocks across %d files", scanned, len(files))

	if len(violations) > 0 {
		t.Errorf("%d doc snippet(s) failed node/graph validation:", len(violations))
		for _, v := range violations {
			t.Errorf("  %s", v)
		}
	}
}
