package main

import (
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
