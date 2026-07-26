// Command snippets extracts fenced json blocks from the user-facing docs,
// checks that they parse (after stripping // comment lines, which the docs
// use for annotation), that every {{ ... }} expression compiles, and — for
// blocks shaped like a workflow — that the node configs and graph structure
// are actually valid per the real runtime rules (#445).
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/chimpanze/noda/internal/config"
	"github.com/chimpanze/noda/internal/engine"
	"github.com/chimpanze/noda/internal/expr"
	"github.com/chimpanze/noda/internal/registry"
	"github.com/chimpanze/noda/plugins/all"
)

// docsDirs lists the doc subtrees that ship user-facing examples.
// docs/_internal/ (architecture docs) is deliberately excluded — it isn't
// rendered in the editor and its snippets aren't held to the same contract.
var docsDirs = []string{"01-getting-started", "02-config", "03-nodes", "04-guides", "05-examples"}

// markdownFiles returns the sorted list of doc markdown files under root that
// are in scope for snippet verification. Shared by main() and the CI gate
// test so both scan exactly the same set of files.
func markdownFiles(root string) ([]string, error) {
	var files []string
	for _, d := range docsDirs {
		matches, err := filepath.Glob(filepath.Join(root, d, "*.md"))
		if err != nil {
			return nil, err
		}
		files = append(files, matches...)
	}
	sort.Strings(files)
	return files, nil
}

type block struct {
	File    string
	Line    int // 1-based line of the block's first content line
	Content string
}

type result struct {
	Verdict string // PARSE-OK, PARSE-FAIL, EXPR-OK (parse+expr ok), EXPR-FAIL
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
			inBlock, start, buf = true, i+2, nil
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
	for line := range strings.SplitSeq(s, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "//") {
			continue
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}

// oneLine flattens a detail string so the report stays one line per snippet.
func oneLine(s string) string {
	return strings.ReplaceAll(s, "\n", " ")
}

func checkBlock(content string) result {
	clean := stripComments(content)
	var v any
	if err := json.Unmarshal([]byte(clean), &v); err != nil {
		return result{"PARSE-FAIL", oneLine(err.Error())}
	}
	compiler := expr.NewCompilerWithFunctions()
	for _, m := range exprRe.FindAllStringSubmatch(content, -1) {
		if _, err := compiler.Compile("{{" + m[1] + "}}"); err != nil {
			return result{"EXPR-FAIL", oneLine(fmt.Sprintf("expr %q: %v", strings.TrimSpace(m[1]), err))}
		}
	}
	if len(exprRe.FindAllString(content, -1)) > 0 {
		return result{"EXPR-OK", ""}
	}
	return result{"PARSE-OK", ""}
}

func main() {
	root := "docs"
	out := ".verification/snippets/report.md"
	files, err := markdownFiles(root)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	var lines []string
	counts := map[string]int{}
	for _, f := range files {
		data, err := os.ReadFile(f) //nolint:gosec // G304: f comes from a fixed glob under docs/
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

	violations, scanned, err := nodeSchemaViolations(files)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	lines = append(lines, "", fmt.Sprintf("--- node-schema check: %d workflow-shaped blocks scanned, %d violations ---", scanned, len(violations)))
	lines = append(lines, violations...)

	if err := os.MkdirAll(filepath.Dir(out), 0o750); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := os.WriteFile(out, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Printf("%d snippets: %v — report at %s\n", len(lines), counts, out)
	fmt.Printf("node-schema check: %d workflow-shaped blocks, %d violations\n", scanned, len(violations))
	for _, v := range violations {
		fmt.Println(v)
	}
}

// buildValidationRegistries constructs the plugin and node registries used to
// validate doc snippets. It mirrors cmd/noda/main.go's corePlugins/
// buildCoreNodeRegistry/registerCorePlugins so the gate exercises the exact
// same node set the real CLI validates against — service-only plugins
// (stream, pubsub, storage) are omitted because they contribute no node
// types and doc snippets are never live-service-checked (see the
// "not found in config (slot:" filter below).
func buildValidationRegistries() (*registry.PluginRegistry, *registry.NodeRegistry, error) {
	plugins := registry.NewPluginRegistry()
	nodes := registry.NewNodeRegistry()
	for _, p := range all.Core() {
		if err := plugins.Register(p); err != nil {
			return nil, nil, fmt.Errorf("register plugin %q: %w", p.Name(), err)
		}
		if err := nodes.RegisterFromPlugin(p); err != nil {
			return nil, nil, fmt.Errorf("register nodes from %q: %w", p.Name(), err)
		}
	}
	return plugins, nodes, nil
}

// nodeSchemaViolations scans files for fenced json blocks shaped like a
// workflow (top-level "nodes" object) and validates each one against the
// real runtime rules: registry.ValidateStartupDryRun (node ConfigSchemas,
// service slot shapes, edge outputs, outcome-output wiring) AND
// engine.Compile (graph-level rules: retry-only-on-error-edges, unknown edge
// targets, cycles, duplicate "as" aliases). Both are required — see the
// package doc comment and #445: a snippet that puts `retry` on a `success`
// edge passes ValidateStartupDryRun (which never calls engine.Compile) but
// hard-fails at real `noda start` boot, so a ConfigSchema-only gate would
// have certified it as good.
//
// It returns one "file:line: message" string per violation, plus the number
// of workflow-shaped blocks it examined (so callers can detect a
// block-extraction regression that would otherwise silently scan zero
// blocks and report a spuriously clean gate).
func nodeSchemaViolations(files []string) ([]string, int, error) {
	plugins, nodes, err := buildValidationRegistries()
	if err != nil {
		return nil, 0, err
	}
	compiler := expr.NewCompilerWithFunctions()

	var violations []string
	scanned := 0
	for _, f := range files {
		data, err := os.ReadFile(f) //nolint:gosec // G304: f comes from a fixed glob under docs/
		if err != nil {
			return nil, scanned, err
		}
		for _, b := range extractJSONBlocks(f, string(data)) {
			clean := stripComments(b.Content)
			var parsed map[string]any
			if err := json.Unmarshal([]byte(clean), &parsed); err != nil {
				// Not valid JSON at all — already reported as PARSE-FAIL by
				// checkBlock above; nothing new to say here.
				continue
			}
			if _, ok := parsed["nodes"].(map[string]any); !ok {
				continue // not workflow-shaped: e.g. a route/service config example
			}
			scanned++

			rc := &config.ResolvedConfig{
				Root:      map[string]any{},
				Workflows: map[string]map[string]any{"snippet": parsed},
			}
			deferred, deferredErrs := registry.CollectDeferredServices(rc)
			for _, derr := range deferredErrs {
				violations = append(violations, fmt.Sprintf("%s:%d %v", b.File, b.Line, derr))
			}

			for _, verr := range registry.ValidateStartupDryRun(rc, plugins, nodes, compiler, deferred) {
				msg := verr.Error()
				// A doc snippet is validated in isolation — it has no
				// top-level "services" block, because the surrounding prose,
				// not the fenced block, is what declares services in a real
				// project. Every node with a service slot therefore always
				// fails "not found in config" here. That is a harness
				// artifact of validating a fragment out of context, not a
				// doc defect, so it is filtered narrowly by this exact
				// message — do NOT broaden this to a generic "service"
				// filter, or real slot/prefix mismatches will go silent too.
				if strings.Contains(msg, "not found in config (slot:") {
					continue
				}
				violations = append(violations, fmt.Sprintf("%s:%d %s", b.File, b.Line, msg))
			}

			wf, err := engine.ParseWorkflowFromMap("snippet", parsed)
			if err != nil {
				violations = append(violations, fmt.Sprintf("%s:%d compile: %v", b.File, b.Line, err))
				continue
			}
			if _, err := engine.Compile(wf, nodes); err != nil {
				violations = append(violations, fmt.Sprintf("%s:%d compile: %v", b.File, b.Line, err))
			}
		}
	}
	return violations, scanned, nil
}
