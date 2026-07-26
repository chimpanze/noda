// Command snippets extracts fenced json blocks from the user-facing docs,
// checks that they parse (after stripping // comment lines, which the docs
// use for annotation), that every {{ ... }} expression compiles, and — for
// blocks shaped like a workflow OR like a node config — that the node configs
// and graph structure are actually valid per the real runtime rules (#445).
package main

import (
	"encoding/json"
	"fmt"
	"io/fs"
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
//
// The walk is recursive: a non-recursive <dir>/*.md glob silently drops any
// doc filed in a subdirectory (docs/04-guides/proxy/foo.md), which is a
// coverage hole nobody would notice. It is also strict — a docsDirs entry
// that no longer exists is an error, not zero files — for the same reason.
func markdownFiles(root string) ([]string, error) {
	var files []string
	for _, d := range docsDirs {
		err := filepath.WalkDir(filepath.Join(root, d), func(path string, e fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if !e.IsDir() && strings.HasSuffix(e.Name(), ".md") {
				files = append(files, path)
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	sort.Strings(files)
	return files, nil
}

type block struct {
	File    string
	Line    int // 1-based line of the block's first content line
	Content string
	Ignore  bool // preceded by a <!-- docverify:ignore ... --> directive
}

// ignoreRe matches the opt-out directive a doc author may place on the line
// immediately before a ```json fence:
//
//	<!-- docverify:ignore a node type from a plugin the reader just wrote -->
//
// It exists for the one legitimate case the gate cannot judge: a snippet whose
// node type is deliberately hypothetical (plugin-development.md shows binding
// services to "my.query"). The reason text is mandatory so the exemption is
// greppable and reviewable; it must never be used to silence a real defect.
var ignoreRe = regexp.MustCompile(`^<!--\s*docverify:ignore\s+\S.*-->$`)

type result struct {
	Verdict string // PARSE-OK, PARSE-FAIL, EXPR-OK (parse+expr ok), EXPR-FAIL
	Detail  string
}

var exprRe = regexp.MustCompile(`\{\{(.*?)\}\}`)

func extractJSONBlocks(file, content string) []block {
	var blocks []block
	lines := strings.Split(content, "\n")
	inBlock := false
	ignore := false
	start := 0
	var buf []string
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !inBlock && strings.HasPrefix(trimmed, "```json") {
			inBlock, start, buf = true, i+2, nil
			ignore = i > 0 && ignoreRe.MatchString(strings.TrimSpace(lines[i-1]))
			continue
		}
		if inBlock && trimmed == "```" {
			blocks = append(blocks, block{File: file, Line: start, Content: strings.Join(buf, "\n"), Ignore: ignore})
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
	verdictCounts := map[string]int{}
	for _, f := range files {
		data, err := os.ReadFile(f) //nolint:gosec // G304: f comes from a fixed glob under docs/
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		for _, b := range extractJSONBlocks(f, string(data)) {
			r := checkBlock(b.Content)
			verdictCounts[r.Verdict]++
			lines = append(lines, fmt.Sprintf("%s:%d %s %s", b.File, b.Line, r.Verdict, r.Detail))
		}
	}

	violations, counts, err := nodeSchemaViolations(files)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	lines = append(lines, "", fmt.Sprintf("--- node-schema check: %d blocks scanned (%d workflow-shaped, %d node fragments), %d violations ---",
		counts.Total(), counts.Workflow, counts.Fragment, len(violations)))
	lines = append(lines, violations...)

	if err := os.MkdirAll(filepath.Dir(out), 0o750); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := os.WriteFile(out, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Printf("%d snippets: %v — report at %s\n", len(lines), verdictCounts, out)
	fmt.Printf("node-schema check: %d blocks (%d workflow-shaped, %d node fragments), %d violations\n",
		counts.Total(), counts.Workflow, counts.Fragment, len(violations))
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

// Deliberate elision placeholders. The docs abbreviate uninteresting parts of
// an example with a literal `...`, which is not JSON. These three forms cover
// every such block in docs/ today; normalising them lets the gate tell a
// deliberate fragment apart from an accidental syntax error.
var (
	ellipsisObjRe  = regexp.MustCompile(`\{\s*\.\.\.\s*\}`)
	ellipsisArrRe  = regexp.MustCompile(`\[\s*\.\.\.\s*\]`)
	ellipsisBareRe = regexp.MustCompile(`(^|[\s,:\[])\.\.\.(?:\s*)($|[,\]\}])`)
)

// isDeliberateEllipsis reports whether a block only fails to parse because of
// the docs' `...` elision convention: substituting real JSON for each `...`
// makes it parse. Anything that still fails after the substitution failed for
// some other reason — a real syntax error.
func isDeliberateEllipsis(clean string) bool {
	if !strings.Contains(clean, "...") {
		return false
	}
	s := ellipsisObjRe.ReplaceAllString(clean, "{}")
	s = ellipsisArrRe.ReplaceAllString(s, "[]")
	s = ellipsisBareRe.ReplaceAllString(s, "${1}null${2}")
	var v any
	return json.Unmarshal([]byte(s), &v) == nil
}

// looksStructural reports whether a block's raw text claims to be a workflow
// or a node config, regardless of whether it parses. Used to decide whether a
// PARSE-FAIL is a gate violation: a block that never claimed to be config
// (a mock fragment, an output-shape illustration) is out of scope, but one
// that does and cannot be parsed would otherwise be permanently and silently
// exempt from every check below — meaning a snippet could be made "green" by
// breaking its JSON.
func looksStructural(clean string) bool {
	return strings.Contains(clean, `"nodes"`) || strings.Contains(clean, `"type":`)
}

// isNodeObject reports whether m looks like a workflow node: a "type" naming a
// plugin-qualified node type ("db.query", "transform.set").
//
// The dot is what makes this narrow enough to be safe. JSON Schema fragments
// in the docs are also maps with a "type" key ({"type": "object",
// "properties": {...}}), but every JSON Schema type name is a bare word
// ("object", "string", "array", "integer", "number", "boolean", "null") while
// every registered node type is "<plugin>.<node>". Keying on the dot excludes
// schema fragments without suppressing anything real: a typo'd node type like
// "db.qeury" still has a dot, so it is still scanned and still reported as an
// unknown node type.
func isNodeObject(m map[string]any) bool {
	t, ok := m["type"].(string)
	return ok && strings.Contains(t, ".")
}

// nodeFragmentWorkflow recognises the two node-fragment block shapes used by
// the per-node reference pages and wraps them in a synthetic edgeless
// workflow, so a fragment gets the same ConfigSchema/service-slot checking a
// real workflow block does. docs/03-nodes/ is 81 pages of exactly this shape
// and was entirely unchecked before.
//
//	{"type": "db.query", "config": {...}}                 → bare node object
//	{"get_user": {"type": "db.query", ...}, "log": {...}} → node id → node object
//
// The second shape is the body of a workflow's "nodes" map quoted on its own;
// it is accepted with any number of entries (the "with data flow" examples
// routinely show two or three) but ONLY when every value is a node object, so
// an arbitrary map that happens to contain one node-shaped value is not
// misread as a workflow.
//
// Returns nil if the block is neither shape.
func nodeFragmentWorkflow(parsed map[string]any) map[string]any {
	if isNodeObject(parsed) {
		return map[string]any{"nodes": map[string]any{"snippet_node": parsed}}
	}
	if len(parsed) == 0 {
		return nil
	}
	wrapped := make(map[string]any, len(parsed))
	for id, v := range parsed {
		nm, ok := v.(map[string]any)
		if !ok || !isNodeObject(nm) {
			return nil
		}
		wrapped[id] = nm
	}
	return map[string]any{"nodes": wrapped}
}

// scanCounts reports what the node-schema check actually examined, so a
// coverage regression is visible rather than silent.
type scanCounts struct {
	Workflow int // blocks with a top-level "nodes" object
	Fragment int // node fragments wrapped into a synthetic workflow
	Ignored  int // blocks carrying an explicit docverify:ignore directive
}

// Total returns the number of blocks handed to the validator.
func (c scanCounts) Total() int { return c.Workflow + c.Fragment }

// nodeSchemaViolations scans files for fenced json blocks shaped like a
// workflow (top-level "nodes" object) or like a single node config, and
// validates each one against the real runtime rules:
// registry.ValidateStartupDryRun (node ConfigSchemas, service slot shapes,
// edge outputs, outcome-output wiring) AND engine.Compile (graph-level rules:
// retry-only-on-error-edges, unknown edge targets, cycles, duplicate "as"
// aliases). Both are required — see the package doc comment and #445: a
// snippet that puts `retry` on a `success` edge passes ValidateStartupDryRun
// (which never calls engine.Compile) but hard-fails at real `noda start`
// boot, so a ConfigSchema-only gate would have certified it as good.
//
// It returns one "file:line: message" string per violation, plus the counts of
// blocks it examined (so callers can detect a block-extraction regression that
// would otherwise silently scan zero blocks and report a spuriously clean
// gate).
func nodeSchemaViolations(files []string) ([]string, scanCounts, error) {
	var counts scanCounts
	plugins, nodes, err := buildValidationRegistries()
	if err != nil {
		return nil, counts, err
	}
	compiler := expr.NewCompilerWithFunctions()

	var violations []string
	for _, f := range files {
		data, err := os.ReadFile(f) //nolint:gosec // G304: f comes from a fixed glob under docs/
		if err != nil {
			return nil, counts, err
		}
		for _, b := range extractJSONBlocks(f, string(data)) {
			if b.Ignore {
				counts.Ignored++
				continue
			}
			clean := stripComments(b.Content)
			var parsed map[string]any
			if err := json.Unmarshal([]byte(clean), &parsed); err != nil {
				// An unparseable block cannot be validated. Silently skipping
				// every one of them would let a contributor exempt a snippet
				// from this gate forever by introducing a syntax error, so a
				// block whose text claims to be config is a violation unless
				// it is one of the docs' deliberate `...` elision fragments.
				if looksStructural(clean) && !isDeliberateEllipsis(clean) {
					violations = append(violations, fmt.Sprintf("%s:%d unparseable config-shaped block: %s", b.File, b.Line, oneLine(err.Error())))
				}
				continue
			}

			workflow := parsed
			fragment := false
			if _, ok := parsed["nodes"].(map[string]any); ok {
				counts.Workflow++
			} else if wrapped := nodeFragmentWorkflow(parsed); wrapped != nil {
				workflow, fragment = wrapped, true
				counts.Fragment++
			} else {
				continue // neither a workflow nor a node config: e.g. a route or service example
			}

			rc := &config.ResolvedConfig{
				Root:      map[string]any{},
				Workflows: map[string]map[string]any{"snippet": workflow},
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
				// The unwired-outcome-output rule (#442) is a statement about
				// a workflow's *graph*: an outcome output that fires with no
				// outbound edge silently ends the path. A synthetic wrapper
				// around a single node fragment has no edges at all, so the
				// rule fires on every db.create/auth.get_user fragment in
				// docs/03-nodes/ — a harness artifact, not a doc defect.
				// Suppressed for fragment-derived blocks ONLY; real workflow
				// blocks, which do have edges, are still held to the rule.
				if fragment && strings.Contains(msg, `has no outbound edge`) {
					continue
				}
				violations = append(violations, fmt.Sprintf("%s:%d %s", b.File, b.Line, msg))
			}

			wf, err := engine.ParseWorkflowFromMap("snippet", workflow)
			if err != nil {
				violations = append(violations, fmt.Sprintf("%s:%d compile: %v", b.File, b.Line, err))
				continue
			}
			if _, err := engine.Compile(wf, nodes); err != nil {
				violations = append(violations, fmt.Sprintf("%s:%d compile: %v", b.File, b.Line, err))
			}
		}
	}
	return violations, counts, nil
}
