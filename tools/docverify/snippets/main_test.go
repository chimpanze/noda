package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExtractJSONBlocks(t *testing.T) {
	md := "intro\n```json\n{\"a\": 1}\n```\ntext\n```json\n{bad\n```\n"
	blocks := extractJSONBlocks("test.md", md)
	if len(blocks) != 2 {
		t.Fatalf("expected 2 blocks, got %d", len(blocks))
	}
	if blocks[0].Line != 3 {
		t.Errorf("expected first block at line 3, got %d", blocks[0].Line)
	}
	if !strings.Contains(blocks[0].Content, `"a": 1`) {
		t.Errorf("unexpected content: %q", blocks[0].Content)
	}
}

func TestCheckBlock(t *testing.T) {
	cases := []struct {
		name, content, wantVerdict string
	}{
		{"valid object", `{"a": 1}`, "PARSE-OK"},
		{"invalid json", `{bad`, "PARSE-FAIL"},
		{"json with comments strips", "// note\n{\"a\": 1}", "PARSE-OK"},
		{"valid expr", `{"key": "{{ input.user_id }}"}`, "EXPR-OK"},
		{"invalid expr", `{"key": "{{ len( }}"}`, "EXPR-FAIL"},
	}
	for _, c := range cases {
		got := checkBlock(c.content)
		if got.Verdict != c.wantVerdict {
			t.Errorf("%s: got %s (%s), want %s", c.name, got.Verdict, got.Detail, c.wantVerdict)
		}
	}
}

func TestIsDeliberateEllipsis(t *testing.T) {
	cases := []struct {
		name, content string
		want          bool
	}{
		{"object elision", `{"type": "db.query", "config": { ... }}`, true},
		{"array elision", `{"nodes": [ ... ]}`, true},
		{"bare value elision", `{"a": ..., "b": 1}`, true},
		{"real syntax error", `{"type": "db.query", "config": {,}}`, false},
		{"trailing comma is not an elision", `{"type": "db.query", "config": {"a": 1,}}`, false},
		{"no ellipsis at all", `{bad`, false},
	}
	for _, c := range cases {
		if got := isDeliberateEllipsis(c.content); got != c.want {
			t.Errorf("%s: isDeliberateEllipsis(%q) = %v, want %v", c.name, c.content, got, c.want)
		}
	}
}

func TestLooksStructural(t *testing.T) {
	cases := []struct {
		content string
		want    bool
	}{
		{`{"nodes": { ... }}`, true},
		{`{"type": "db.query"}`, true},
		{`"mocks": { "insert_user": { "output": {} } }`, false},
		{`{ "exists": true }`, false},
	}
	for _, c := range cases {
		if got := looksStructural(c.content); got != c.want {
			t.Errorf("looksStructural(%q) = %v, want %v", c.content, got, c.want)
		}
	}
}

func TestNodeFragmentWorkflow(t *testing.T) {
	t.Run("bare node object", func(t *testing.T) {
		got := nodeFragmentWorkflow(map[string]any{"type": "db.query", "config": map[string]any{}})
		nodes, ok := got["nodes"].(map[string]any)
		if !ok || len(nodes) != 1 {
			t.Fatalf("expected one synthetic node, got %#v", got)
		}
	})
	t.Run("multi node id map", func(t *testing.T) {
		got := nodeFragmentWorkflow(map[string]any{
			"a": map[string]any{"type": "db.query"},
			"b": map[string]any{"type": "util.log"},
		})
		nodes, ok := got["nodes"].(map[string]any)
		if !ok || len(nodes) != 2 {
			t.Fatalf("expected two nodes, got %#v", got)
		}
	})
	t.Run("json schema property map is not a node map", func(t *testing.T) {
		// {"type": "object", ...} and {"email": {"type": "string"}} both look
		// node-ish; the dot in a real node type is what tells them apart.
		if got := nodeFragmentWorkflow(map[string]any{"type": "object", "properties": map[string]any{}}); got != nil {
			t.Errorf("JSON Schema object treated as a node: %#v", got)
		}
		if got := nodeFragmentWorkflow(map[string]any{"email": map[string]any{"type": "string"}}); got != nil {
			t.Errorf("JSON Schema property map treated as a node map: %#v", got)
		}
	})
	t.Run("mixed map is not a node map", func(t *testing.T) {
		if got := nodeFragmentWorkflow(map[string]any{
			"a":       map[string]any{"type": "db.query"},
			"timeout": "30s",
		}); got != nil {
			t.Errorf("mixed map treated as a node map: %#v", got)
		}
	})
	t.Run("empty map", func(t *testing.T) {
		if got := nodeFragmentWorkflow(map[string]any{}); got != nil {
			t.Errorf("empty map treated as a node map: %#v", got)
		}
	})
}

func TestExtractJSONBlocksIgnoreDirective(t *testing.T) {
	md := "<!-- docverify:ignore hypothetical plugin -->\n```json\n{\"type\": \"my.query\"}\n```\n" +
		"```json\n{\"type\": \"db.query\"}\n```\n"
	blocks := extractJSONBlocks("test.md", md)
	if len(blocks) != 2 {
		t.Fatalf("expected 2 blocks, got %d", len(blocks))
	}
	if !blocks[0].Ignore {
		t.Error("block preceded by docverify:ignore should be ignored")
	}
	if blocks[1].Ignore {
		t.Error("the directive must not leak to the next block")
	}
	// A bare HTML comment is not a directive.
	if extractJSONBlocks("t.md", "<!-- just a note -->\n```json\n{}\n```\n")[0].Ignore {
		t.Error("a non-directive comment must not exempt a block")
	}
	// A directive with no reason text is not a directive.
	if extractJSONBlocks("t.md", "<!-- docverify:ignore -->\n```json\n{}\n```\n")[0].Ignore {
		t.Error("docverify:ignore without a reason must not exempt a block")
	}
}

// TestUnparseableConfigShapedBlockIsAViolation pins the anti-loophole rule: a
// snippet must not be able to escape the gate by being invalid JSON.
func TestUnparseableConfigShapedBlockIsAViolation(t *testing.T) {
	dir := t.TempDir()
	// Every in-scope dir must exist: markdownFiles is deliberately strict, so
	// a docsDirs entry that no longer exists fails loudly rather than
	// shrinking coverage silently.
	for _, d := range docsDirs {
		if err := os.MkdirAll(filepath.Join(dir, d), 0o750); err != nil {
			t.Fatal(err)
		}
	}
	sub := filepath.Join(dir, "04-guides", "nested")
	if err := os.MkdirAll(sub, 0o750); err != nil {
		t.Fatal(err)
	}
	md := "# x\n\n```json\n{\n  \"type\": \"db.query\",\n  \"config\": {,}\n}\n```\n"
	path := filepath.Join(sub, "broken.md")
	if err := os.WriteFile(path, []byte(md), 0o600); err != nil {
		t.Fatal(err)
	}

	// markdownFiles must find it: the walk is recursive.
	files, err := markdownFiles(dir)
	if err != nil {
		t.Fatalf("markdownFiles: %v", err)
	}
	if len(files) != 1 || files[0] != path {
		t.Fatalf("recursive walk did not find the nested doc: %v", files)
	}

	violations, _, err := nodeSchemaViolations(files)
	if err != nil {
		t.Fatalf("nodeSchemaViolations: %v", err)
	}
	if len(violations) != 1 || !strings.Contains(violations[0], "unparseable config-shaped block") {
		t.Fatalf("expected one unparseable-block violation, got %v", violations)
	}
}
