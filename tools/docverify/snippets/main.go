// Command snippets extracts fenced json blocks from the user-facing docs,
// checks that they parse (after stripping // comment lines, which the docs
// use for annotation) and that every {{ ... }} expression compiles.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/chimpanze/noda/internal/expr"
)

type block struct {
	File    string
	Line    int // 1-based line of the block's opening ```json fence
	Content string
}

type result struct {
	Verdict string // PARSE-OK (parse + any exprs ok), PARSE-FAIL, EXPR-FAIL
	Detail  string
}

var exprRe = regexp.MustCompile(`\{\{(.*?)\}\}`)

func extractJSONBlocks(file, content string) []block {
	var blocks []block
	lines := strings.Split(content, "\n")
	inBlock := false
	start := 0
	var buf []string
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !inBlock && strings.HasPrefix(trimmed, "```json") {
			inBlock, start, buf = true, i+1, nil
			continue
		}
		if inBlock && trimmed == "```" {
			blocks = append(blocks, block{File: file, Line: start, Content: strings.Join(buf, "\n")})
			inBlock = false
			continue
		}
		if inBlock {
			buf = append(buf, line)
		}
	}
	return blocks
}

// stripComments removes lines whose first non-space chars are "//" —
// the docs annotate JSON examples with such comment lines.
func stripComments(s string) string {
	var out []string
	for _, line := range strings.Split(s, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "//") {
			continue
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}

func checkBlock(content string) result {
	clean := stripComments(content)
	var v any
	if err := json.Unmarshal([]byte(clean), &v); err != nil {
		return result{"PARSE-FAIL", err.Error()}
	}
	compiler := expr.NewCompilerWithFunctions()
	for _, m := range exprRe.FindAllStringSubmatch(content, -1) {
		if _, err := compiler.Compile("{{" + m[1] + "}}"); err != nil {
			return result{"EXPR-FAIL", fmt.Sprintf("expr %q: %v", strings.TrimSpace(m[1]), err)}
		}
	}
	return result{"PARSE-OK", ""}
}

func main() {
	root := "docs"
	out := ".verification/snippets/report.md"
	dirs := []string{"01-getting-started", "02-config", "03-nodes", "04-guides", "05-examples"}
	var lines []string
	counts := map[string]int{}
	for _, d := range dirs {
		matches, err := filepath.Glob(filepath.Join(root, d, "*.md"))
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		sort.Strings(matches)
		for _, f := range matches {
			data, err := os.ReadFile(f)
			if err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
			for _, b := range extractJSONBlocks(f, string(data)) {
				r := checkBlock(b.Content)
				counts[r.Verdict]++
				lines = append(lines, fmt.Sprintf("%s:%d %s %s", b.File, b.Line, r.Verdict, r.Detail))
			}
		}
	}
	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := os.WriteFile(out, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Printf("%d snippets: %v — report at %s\n", len(lines), counts, out)
}
