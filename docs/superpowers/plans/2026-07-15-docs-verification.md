# Documentation Verification Campaign Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Verify all 114 user-facing doc files against the codebase and file one GitHub issue per doc area listing every discrepancy.

**Architecture:** Phase 0 builds a machine-readable ground-truth inventory (node schemas, config schemas, expression functions, CLI surface) plus a doc-snippet extractor/validator, both as small Go tools in `tools/docverify/`. Phase 1 runs seven static per-area passes that diff docs against the inventory and log findings to per-area files. Phase 2 executes the walkthrough docs for real (docker compose, quick-start, examples). A final task turns the findings files into GitHub issues.

**Tech Stack:** Go (tools reuse `internal/registry`, `internal/expr`, `pkg/api`), `jq`, `gh` CLI, docker compose.

**Spec:** `docs/superpowers/specs/2026-07-15-docs-verification-design.md`

## Global Constraints

- **Code on current `main` is the source of truth.** Docs describing intentionally-changed behavior are checked against current behavior, not history.
- **This campaign only reports — never fix a doc or a code bug.** The only repo changes allowed are the `tools/docverify/` tools (committed on the campaign branch).
- **Branch:** all commits go on branch `docverify-campaign` (created in Task 1). Never commit to `main`.
- **Working directory:** all campaign artifacts (ground truth dumps, findings files, temp projects) live in `/Users/marten/GolandProjects/noda/.verification/` — untracked, excluded via `.git/info/exclude`.
- **Finding format** (used in every findings file, one entry per finding):

  ```markdown
  ### [severity] <doc-file>#<section> — <one-line summary>
  - Doc says: <claim, quoted or paraphrased>
  - Code does: <actual behavior> (`path/to/file.go:NN`)
  ```

  Severity is exactly one of: `breaks-user` (a user following the doc hits an error), `wrong` (factually incorrect but survivable), `gap` (code capability the docs never mention), `cosmetic` (typo-level drift).
- **Honesty rules:** a snippet that fails mechanical validation must be re-checked by hand before logging (fragments legitimately fail out of context). Anything that couldn't be executed is listed explicitly under an `## Unverified` heading in the findings file with the reason — never silently skipped. Every finding cites a code location.
- **Known trap:** `internal/mcp/plugins.go` registers FEWER plugins than `cmd/noda/main.go` (no auth/stream/pubsub/storage-service). Ground truth must mirror `cmd/noda/main.go` (`corePlugins()` + `serviceOnlyPlugins()` + image), NOT `internal/mcp`.

---

### Task 1: Ground-truth dump tool

**Files:**
- Create: `tools/docverify/groundtruth/main.go`
- Create: `tools/docverify/groundtruth/main_test.go`
- Create: `tools/docverify/groundtruth/plugins_image.go`, `tools/docverify/groundtruth/plugins_noimage.go`

**Interfaces:**
- Produces: `.verification/ground-truth/nodes.json` — JSON array of `{type, description, config_schema, outputs, service_deps, output_data}` sorted by type; `.verification/ground-truth/functions.json` — JSON array of `{name, signature, description}`. Later tasks read these with `jq`.

- [ ] **Step 1: Create branch and working directory**

```bash
cd /Users/marten/GolandProjects/noda
git checkout -b docverify-campaign
mkdir -p .verification/ground-truth
grep -qx '.verification/' .git/info/exclude || echo '.verification/' >> .git/info/exclude
```

- [ ] **Step 2: Write the failing test**

Create `tools/docverify/groundtruth/main_test.go`:

```go
package main

import "testing"

func TestBuildGroundTruth(t *testing.T) {
	nodes, funcs, err := buildGroundTruth()
	if err != nil {
		t.Fatalf("buildGroundTruth: %v", err)
	}
	if len(nodes) < 80 {
		t.Errorf("expected >= 80 node types, got %d", len(nodes))
	}
	want := []string{"auth.create_user", "lk.token", "db.query", "control.if", "wasm.send"}
	byType := map[string]bool{}
	for _, n := range nodes {
		byType[n.Type] = true
	}
	for _, w := range want {
		if !byType[w] {
			t.Errorf("missing node type %q — plugin list does not mirror cmd/noda/main.go", w)
		}
	}
	if len(funcs) == 0 {
		t.Error("expected registered expression functions")
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./tools/docverify/groundtruth/`
Expected: FAIL — `buildGroundTruth` undefined.

- [ ] **Step 4: Write the tool**

Create `tools/docverify/groundtruth/main.go`. The plugin list mirrors `cmd/noda/main.go` `corePlugins()` (main.go:755-773) + `serviceOnlyPlugins()` (main.go:779-786); image comes via the same `noimage` build-tag pattern as `cmd/noda/plugins_image.go`:

```go
// Command groundtruth dumps the runtime's self-description (node types,
// schemas, expression functions) as JSON for the docs verification campaign.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"

	"github.com/chimpanze/noda/internal/expr"
	"github.com/chimpanze/noda/internal/registry"
	"github.com/chimpanze/noda/pkg/api"
	authplugin "github.com/chimpanze/noda/plugins/auth"
	cacheplugin "github.com/chimpanze/noda/plugins/cache"
	"github.com/chimpanze/noda/plugins/core/control"
	"github.com/chimpanze/noda/plugins/core/event"
	coreoidc "github.com/chimpanze/noda/plugins/core/oidc"
	"github.com/chimpanze/noda/plugins/core/response"
	coresse "github.com/chimpanze/noda/plugins/core/sse"
	corestorage "github.com/chimpanze/noda/plugins/core/storage"
	"github.com/chimpanze/noda/plugins/core/transform"
	"github.com/chimpanze/noda/plugins/core/upload"
	"github.com/chimpanze/noda/plugins/core/util"
	corewasm "github.com/chimpanze/noda/plugins/core/wasm"
	"github.com/chimpanze/noda/plugins/core/workflow"
	corews "github.com/chimpanze/noda/plugins/core/ws"
	dbplugin "github.com/chimpanze/noda/plugins/db"
	emailplugin "github.com/chimpanze/noda/plugins/email"
	httpplugin "github.com/chimpanze/noda/plugins/http"
	livekitplugin "github.com/chimpanze/noda/plugins/livekit"
	pubsubplugin "github.com/chimpanze/noda/plugins/pubsub"
	storageplugin "github.com/chimpanze/noda/plugins/storage"
	streamplugin "github.com/chimpanze/noda/plugins/stream"
)

// optionalPlugins is appended by the build-tagged plugins_image.go.
var optionalPlugins []api.Plugin

type nodeInfo struct {
	Type         string                    `json:"type"`
	Description  string                    `json:"description"`
	ConfigSchema map[string]any            `json:"config_schema,omitempty"`
	Outputs      []string                  `json:"outputs"`
	ServiceDeps  map[string]api.ServiceDep `json:"service_deps,omitempty"`
	OutputData   map[string]string         `json:"output_data,omitempty"`
}

func allPlugins() []api.Plugin {
	plugins := []api.Plugin{
		&control.Plugin{}, &transform.Plugin{}, &util.Plugin{},
		&workflow.Plugin{}, &response.Plugin{}, &dbplugin.Plugin{},
		&cacheplugin.Plugin{}, &event.Plugin{}, &corestorage.Plugin{},
		&upload.Plugin{}, &httpplugin.Plugin{}, &emailplugin.Plugin{},
		&corews.Plugin{}, &coresse.Plugin{}, &corewasm.Plugin{},
		&coreoidc.Plugin{}, &livekitplugin.Plugin{},
		&authplugin.Plugin{}, &streamplugin.Plugin{},
		&pubsubplugin.Plugin{}, &storageplugin.Plugin{},
	}
	return append(plugins, optionalPlugins...)
}

func buildGroundTruth() ([]nodeInfo, []expr.FunctionInfo, error) {
	reg := registry.NewNodeRegistry()
	for _, p := range allPlugins() {
		if err := reg.RegisterFromPlugin(p); err != nil {
			return nil, nil, fmt.Errorf("register %T: %w", p, err)
		}
	}
	types := reg.AllTypes()
	sort.Strings(types)
	nodes := make([]nodeInfo, 0, len(types))
	for _, t := range types {
		desc, ok := reg.GetDescriptor(t)
		if !ok {
			continue
		}
		outputs, _ := reg.OutputsForType(t)
		nodes = append(nodes, nodeInfo{
			Type:         t,
			Description:  desc.Description(),
			ConfigSchema: desc.ConfigSchema(),
			Outputs:      outputs,
			ServiceDeps:  desc.ServiceDeps(),
			OutputData:   desc.OutputDescriptions(),
		})
	}
	funcs := expr.NewFunctionRegistry().RegisteredFunctions()
	return nodes, funcs, nil
}

func writeJSON(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

func main() {
	outDir := ".verification/ground-truth"
	if len(os.Args) > 1 {
		outDir = os.Args[1]
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	nodes, funcs, err := buildGroundTruth()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := writeJSON(outDir+"/nodes.json", nodes); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := writeJSON(outDir+"/functions.json", funcs); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Printf("dumped %d node types, %d functions to %s\n", len(nodes), len(funcs), outDir)
}
```

Create `tools/docverify/groundtruth/plugins_image.go`:

```go
//go:build !noimage

package main

import imageplugin "github.com/chimpanze/noda/plugins/image"

func init() {
	optionalPlugins = append(optionalPlugins, &imageplugin.Plugin{})
}
```

Create `tools/docverify/groundtruth/plugins_noimage.go`:

```go
//go:build noimage

package main
```

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./tools/docverify/groundtruth/`
Expected: PASS. If the build fails on libvips (cgo), run with `-tags noimage` instead AND note in `.verification/findings/00-inventory.md` that `image.*` schemas must be verified by reading `plugins/image/` source.

- [ ] **Step 6: Run the dump**

```bash
go run ./tools/docverify/groundtruth
jq length .verification/ground-truth/nodes.json
jq -r '.[].type' .verification/ground-truth/nodes.json | head -20
```

Expected: `dumped N node types, M functions ...` with N ≥ 80; type list starts with `auth.*`.

- [ ] **Step 7: Commit**

```bash
git add tools/docverify/groundtruth
git commit -m "chore(docverify): ground-truth dump tool for docs verification campaign"
```

---

### Task 2: CLI surface dump, config schemas, and inventory cross-check

**Files:**
- Create: `.verification/ground-truth/cli/*.txt` (untracked)
- Create: `.verification/findings/00-inventory.md` (untracked)

**Interfaces:**
- Consumes: `.verification/ground-truth/nodes.json` from Task 1.
- Produces: `.verification/ground-truth/cli/<command>.txt` help dumps; `.verification/ground-truth/schemas/*.json` (copies of `internal/config/schemas/*.json`); `.verification/findings/00-inventory.md` using the Global Constraints finding format.

- [ ] **Step 1: Dump the CLI surface**

```bash
cd /Users/marten/GolandProjects/noda
go build -o .verification/noda ./cmd/noda
mkdir -p .verification/ground-truth/cli
.verification/noda --help > .verification/ground-truth/cli/_root.txt 2>&1
for c in auth dev generate init mcp migrate plugin schedule start test validate version; do
  .verification/noda "$c" --help > ".verification/ground-truth/cli/$c.txt" 2>&1
done
```

Then, for each command whose help lists subcommands (at minimum `auth`, `generate`, `migrate`, `plugin`, `schedule` — read the dumped files to see), dump each subcommand too, e.g. `.verification/noda auth init --help > .verification/ground-truth/cli/auth-init.txt`.

- [ ] **Step 2: Copy the embedded config schemas**

```bash
mkdir -p .verification/ground-truth/schemas
cp internal/config/schemas/*.json .verification/ground-truth/schemas/
ls .verification/ground-truth/schemas/
```

Expected files: `connections.json`, `model.json`, `root.json`, `route.json`, `schedule.json`, `test.json`, `worker.json`, `workflow.json`. Note: `model.json` exists in code but the MCP `noda_get_config_schema` tool only exposes 7 types (no `model`) — check whether `docs/02-config/schemas.md` documents model files, and record the MCP omission in the inventory findings if it holds.

- [ ] **Step 3: Cross-check doc pages vs registry (both directions)**

```bash
jq -r '.[].type' .verification/ground-truth/nodes.json | sort > .verification/registry-types.txt
ls docs/03-nodes/ | grep -v '^_index' | sed 's/\.md$//' | sort > .verification/doc-pages.txt
diff .verification/registry-types.txt .verification/doc-pages.txt
```

For every type in the registry with no doc page: log a `gap` finding. For every doc page with no registry type: log a `breaks-user` finding (documented node doesn't exist). Also compare the node list in `docs/03-nodes/_index.md` against `registry-types.txt` the same way.

- [ ] **Step 4: Verify the known MCP plugin-list drift**

Read `internal/mcp/plugins.go` `corePlugins()` and compare with `cmd/noda/main.go` `corePlugins()` + `serviceOnlyPlugins()`. If the MCP registry is missing plugins that provide node types (expected: auth), log ONE finding in `00-inventory.md` — severity `wrong`, cited as `internal/mcp/plugins.go` — noting that MCP-driven agents can't discover those nodes. (This is a code finding discovered by the campaign; it gets its own small issue in Task 13.)

- [ ] **Step 5: Write findings file**

Create `.verification/findings/00-inventory.md` with the findings from Steps 2–4 in the standard format (header: `# Inventory-level findings`). If a check was clean, say so under a `## Clean` heading.

---

### Task 3: Snippet extractor and validator

**Files:**
- Create: `tools/docverify/snippets/main.go`
- Create: `tools/docverify/snippets/main_test.go`

**Interfaces:**
- Consumes: nothing from earlier tasks (standalone).
- Produces: `.verification/snippets/report.md` — one line per fenced ```json block in `docs/{01-getting-started,02-config,03-nodes,04-guides,05-examples}`: `<file>:<line> <verdict> <detail>` where verdict ∈ `PARSE-OK`, `PARSE-FAIL`, `EXPR-OK`, `EXPR-FAIL`. Every embedded `{{ ... }}` expression is compiled with the real expression compiler. Area passes grep this report for their files.

- [ ] **Step 1: Write the failing test**

Create `tools/docverify/snippets/main_test.go`:

```go
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
	if blocks[0].Line != 2 {
		t.Errorf("expected first block at line 2, got %d", blocks[0].Line)
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
		{"valid expr", `{"key": "{{ input.user_id }}"}`, "PARSE-OK"},
		{"invalid expr", `{"key": "{{ input..user_id }}"}`, "EXPR-FAIL"},
	}
	for _, c := range cases {
		got := checkBlock(c.content)
		if got.Verdict != c.wantVerdict {
			t.Errorf("%s: got %s (%s), want %s", c.name, got.Verdict, got.Detail, c.wantVerdict)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./tools/docverify/snippets/`
Expected: FAIL — `extractJSONBlocks`, `checkBlock` undefined.

- [ ] **Step 3: Write the tool**

Create `tools/docverify/snippets/main.go`:

```go
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
	if len(exprRe.FindAllString(content, -1)) > 0 {
		return result{"EXPR-OK", ""}
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
```

API notes (verified against source): `expr.NewCompilerWithFunctions()` (`internal/expr/functions.go:284`) returns a `*Compiler` with the noda functions registered; `Compile(input string) (*CompiledExpression, error)` (`internal/expr/compiler.go:96`) takes the full `{{ ... }}`-wrapped string — this mirrors how the MCP `noda_validate_expression` handler compiles (`internal/mcp/tools.go:302-303`). The default (non-strict) compiler allows undefined variables, so EXPR-FAIL means a genuine syntax/function error, not an unknown variable.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./tools/docverify/snippets/`
Expected: PASS.

- [ ] **Step 5: Run the extractor over the docs**

```bash
go run ./tools/docverify/snippets
grep -c FAIL .verification/snippets/report.md || true
grep FAIL .verification/snippets/report.md | head -20
```

Expected: a report with several hundred snippets. FAIL lines are NOT findings yet — area passes re-check each one by hand (many blocks are deliberate fragments).

- [ ] **Step 6: Commit**

```bash
git add tools/docverify/snippets
git commit -m "chore(docverify): doc snippet extractor and expression validator"
```

---

### Task 4: Area pass 1 — `01-getting-started` (7 files)

**Files:**
- Read: `docs/01-getting-started/installation.md`, `quick-start.md`, `expressions.md`, `expression-cookbook.md`, `data-flow.md`, `services.md`, `realtime.md`
- Create: `.verification/findings/01-getting-started.md` (untracked)

**Interfaces:**
- Consumes: `.verification/ground-truth/{nodes.json,functions.json,cli/*.txt,schemas/*.json}`, `.verification/snippets/report.md`.
- Produces: findings file in the Global Constraints format. Execution-dependent claims (quick-start walkthrough steps) are NOT executed here — mark them `deferred-to-execution` in a `## Deferred` list; Task 11 picks them up.

- [ ] **Step 1: Verify each file's claims against ground truth**

For each of the 7 files, read it fully and check every verifiable claim:
- CLI commands and flags → diff against `.verification/ground-truth/cli/*.txt` (exact flag names, defaults, argument order).
- Node types mentioned → exist in `nodes.json`; config fields used in examples → exist in that node's `config_schema` with matching types; look up one node at a time with `jq '.[] | select(.type=="db.query")' .verification/ground-truth/nodes.json`.
- Expression functions mentioned (`expressions.md`, `expression-cookbook.md`) → every documented function exists in `functions.json` with matching signature, AND every function in `functions.json` appears in the docs (missing ones are `gap` findings). Check documented expr-lang builtins against `internal/mcp/tools.go:382-395` (`exprLangBuiltins`).
- Variable namespaces documented (e.g. `input`, `nodes`, `auth`, `trigger`, `env`, `request`) → cross-check against the evaluator env construction in `internal/expr/` and `internal/engine/` (grep for where the evaluation context map is built).
- Config file structure claims (`services.md`, `data-flow.md`) → diff against `.verification/ground-truth/schemas/*.json`.
- Installation claims (`installation.md`: install commands, platforms, version) → check against `.goreleaser` config / `Makefile` / release workflow files if referenced; go version claims against `go.mod`.

- [ ] **Step 2: Re-check this area's snippet failures**

```bash
grep '^docs/01-getting-started/' .verification/snippets/report.md | grep FAIL
```

For each FAIL: open the doc at that line. If the block is a deliberate fragment or pseudo-JSON, ignore. If it's presented as working config/expression and fails, log a `breaks-user` finding quoting the validator error.

- [ ] **Step 3: Write the findings file**

Create `.verification/findings/01-getting-started.md` (header `# 01-getting-started findings`), all findings in the standard format, plus `## Deferred` (execution steps for Task 11) and `## Clean` (files/sections with no findings) so coverage is auditable.

---

### Task 5: Area pass 2 — `02-config` (11 files)

**Files:**
- Read: `docs/02-config/overview.md`, `noda-json.md`, `routes.md`, `workflows.md`, `workers.md`, `schedules.md`, `connections.md`, `middleware.md`, `variables.md`, `schemas.md`, `tests.md`
- Create: `.verification/findings/02-config.md` (untracked)

**Interfaces:**
- Consumes: `.verification/ground-truth/schemas/*.json` (the authoritative JSON Schemas: `root.json`, `route.json`, `workflow.json`, `worker.json`, `schedule.json`, `connections.json`, `test.json`, `model.json`), `.verification/snippets/report.md`.
- Produces: findings file in the Global Constraints format.

- [ ] **Step 1: Field-by-field schema diff**

For each doc file, map it to its schema (`routes.md`→`route.json`, `workflows.md`→`workflow.json`, `workers.md`→`worker.json`, `schedules.md`→`schedule.json`, `connections.md`→`connections.json`, `tests.md`→`test.json`, `noda-json.md`→`root.json`, `schemas.md`→`model.json`). For every field the doc documents: confirm it exists in the schema with matching type, required-status, default, and enum values. For every schema property the doc omits: log a `gap` finding. `middleware.md` and `variables.md` have no dedicated schema file — verify them against the sections of `root.json`/`route.json` that define middleware/variables, and against `internal/config/` loading code (grep for the field names) plus `internal/server/` middleware wiring for behavioral claims (ordering, auth modes).

- [ ] **Step 2: Verify behavioral claims against loader code**

Claims the schema can't answer (merge order, env overrides, file discovery patterns, validation timing) → read `internal/config/` (loading, merging, validation) and cite exact lines. For `variables.md` `$env` claims, check the env-substitution implementation in `internal/config/` (memory aid: there were `$env` gotchas fixed in the 2026-07 backlog tranches — verify current behavior only).

- [ ] **Step 3: Re-check this area's snippet failures**

```bash
grep '^docs/02-config/' .verification/snippets/report.md | grep FAIL
```

Same manual re-check rule as Task 4 Step 2.

- [ ] **Step 4: Write the findings file**

Create `.verification/findings/02-config.md` with findings + `## Clean` section.

---

### Task 6: Area pass 3 — `03-nodes` core plugins (25 pages)

**Files:**
- Read: `docs/03-nodes/control.*.md` (3), `transform.*.md` (6), `response.*.md` (3), `util.*.md` (5), `workflow.*.md` (2), `event.emit.md`, `upload.handle.md`, `ws.send.md`, `sse.send.md`, `wasm.send.md`, `wasm.query.md`
- Create: `.verification/findings/03-nodes-core.md` (untracked)

**Interfaces:**
- Consumes: `.verification/ground-truth/nodes.json`, `.verification/snippets/report.md`; plugin source under `plugins/core/`.
- Produces: findings file in the Global Constraints format.

- [ ] **Step 1: Per-page mechanical diff against nodes.json**

For each page (get the list with `ls docs/03-nodes/{control,transform,response,util,workflow,event,upload,ws,sse,wasm}.*.md`), pull the ground truth entry:

```bash
jq '.[] | select(.type=="control.if")' .verification/ground-truth/nodes.json
```

Diff four sections mechanically:
1. **Config table** — every documented field vs `config_schema` `properties` (name, type, required list, defaults, enums). Schema properties missing from the doc table → `gap`.
2. **Outputs** — the documented output ports vs `outputs` array (order-insensitive; missing/extra ports are `wrong`).
3. **Service Dependencies table** — vs `service_deps` (slot names, prefixes, required flags).
4. **Output Shape / output descriptions** — vs `output_data` where present.

- [ ] **Step 2: Verify Behavior and Error Output sections against source**

For each page, read the node's implementation in `plugins/core/<pkg>/` (e.g. `control.if` → `plugins/core/control/`). Check: error-port semantics (what actually fires `error` and with what payload shape — compare against the doc's Error Output JSON), edge cases the doc asserts (e.g. loop limits, redirect status codes, delay caps), and any documented defaults not present in the schema. Cite `plugins/core/<pkg>/<file>.go:NN` in every finding.

- [ ] **Step 3: Re-check this area's snippet failures**

```bash
grep -E '^docs/03-nodes/(control|transform|response|util|workflow|event|upload|ws|sse|wasm)\.' .verification/snippets/report.md | grep FAIL
```

Manual re-check rule as in Task 4 Step 2.

- [ ] **Step 4: Write the findings file**

Create `.verification/findings/03-nodes-core.md` with findings + `## Clean` list of pages that fully matched.

---

### Task 7: Area pass 4 — `03-nodes` data plugins (26 pages)

**Files:**
- Read: `docs/03-nodes/db.*.md` (9), `cache.*.md` (4), `storage.*.md` (4), `image.*.md` (5), `http.*.md` (3), `email.send.md`
- Create: `.verification/findings/03-nodes-data.md` (untracked)

**Interfaces:**
- Consumes: `.verification/ground-truth/nodes.json`, `.verification/snippets/report.md`; plugin source under `plugins/{db,cache,storage,image,http,email}/`.
- Produces: findings file in the Global Constraints format.

- [ ] **Step 1: Per-page mechanical diff against nodes.json**

Same four-section diff as Task 6 Step 1 (Config table vs `config_schema`, Outputs vs `outputs`, Service Dependencies vs `service_deps`, Output Shape vs `output_data`), using `jq '.[] | select(.type=="db.query")' .verification/ground-truth/nodes.json` per page. If Task 1 had to build with `-tags noimage`, the 5 `image.*` pages have no ground-truth entry — diff their Config tables directly against the schema literals in `plugins/image/` source and say so in the findings file.

- [ ] **Step 2: Verify Behavior and Error Output sections against source**

Read implementations in `plugins/db/`, `plugins/cache/`, `plugins/storage/`, `plugins/image/`, `plugins/http/`, `plugins/email/`. Pay attention to claims about: pagination/cursor behavior (`db.find` — cursor gotchas were fixed in the 2026-07 tranches; verify against current code), upsert conflict semantics, cache TTL handling and `NotFoundError` on miss, storage path traversal rules, http timeout/retry defaults, email template resolution. Cite exact `file.go:NN` per finding.

- [ ] **Step 3: Re-check this area's snippet failures**

```bash
grep -E '^docs/03-nodes/(db|cache|storage|image|http|email)\.' .verification/snippets/report.md | grep FAIL
```

Manual re-check rule as in Task 4 Step 2.

- [ ] **Step 4: Write the findings file**

Create `.verification/findings/03-nodes-data.md` with findings + `## Clean` list.

---

### Task 8: Area pass 5 — `03-nodes` auth & oidc (11 pages)

**Files:**
- Read: `docs/03-nodes/auth.*.md` (8), `oidc.*.md` (3)
- Create: `.verification/findings/03-nodes-auth.md` (untracked)

**Interfaces:**
- Consumes: `.verification/ground-truth/nodes.json`, `.verification/snippets/report.md`; plugin source under `plugins/auth/` and `plugins/core/oidc/`.
- Produces: findings file in the Global Constraints format.

- [ ] **Step 1: Per-page mechanical diff against nodes.json**

Same four-section diff as Task 6 Step 1, per page, via `jq '.[] | select(.type=="auth.create_user")' .verification/ground-truth/nodes.json`.

- [ ] **Step 2: Verify Behavior sections against source — with extra care**

The auth plugin was hardened repeatedly in 2026-07 (anti-enumeration: verification-first register, constant-time reset/resend, atomic `set_password` token mode, invalid-path padding). Docs written before those tranches may describe the OLD behavior. For each auth page, read the current implementation in `plugins/auth/` and verify: token modes and consumption semantics (`auth.consume_token`), session opacity claims (`auth.create_session`, `auth.revoke_session`), password rules (`auth.set_password` rune validation), and error payloads (anti-enumeration means some documented error distinctions may no longer be observable). For `oidc.*`, verify flow claims against `plugins/core/oidc/`. Cite exact lines.

- [ ] **Step 3: Re-check this area's snippet failures**

```bash
grep -E '^docs/03-nodes/(auth|oidc)\.' .verification/snippets/report.md | grep FAIL
```

Manual re-check rule as in Task 4 Step 2.

- [ ] **Step 4: Write the findings file**

Create `.verification/findings/03-nodes-auth.md` with findings + `## Clean` list.

---

### Task 9: Area pass 6 — `03-nodes` LiveKit (18 pages)

**Files:**
- Read: `docs/03-nodes/lk.*.md` (18)
- Create: `.verification/findings/03-nodes-livekit.md` (untracked)

**Interfaces:**
- Consumes: `.verification/ground-truth/nodes.json`, `.verification/snippets/report.md`; plugin source under `plugins/livekit/`.
- Produces: findings file in the Global Constraints format.

- [ ] **Step 1: Per-page mechanical diff against nodes.json**

Same four-section diff as Task 6 Step 1, per page, via `jq '.[] | select(.type=="lk.token")' .verification/ground-truth/nodes.json`.

- [ ] **Step 2: Verify Behavior sections against source**

Read `plugins/livekit/`. Check claims about: token grants and TTLs (`lk.token`), room lifecycle (`lk.roomCreate`/`roomDelete`/`roomList` — empty-room semantics), egress start/stop parameters and output URLs, ingress creation, participant mutation semantics (`lk.muteTrack`, `lk.participantUpdate` permission merge — this was reworked in tranche G 2026-07-07; verify against current code). Where the doc documents LiveKit-server-side behavior noda merely proxies, verify only the request noda constructs (cite the plugin line), and don't flag upstream LiveKit semantics.

- [ ] **Step 3: Re-check this area's snippet failures**

```bash
grep '^docs/03-nodes/lk\.' .verification/snippets/report.md | grep FAIL
```

Manual re-check rule as in Task 4 Step 2.

- [ ] **Step 4: Write the findings file**

Create `.verification/findings/03-nodes-livekit.md` with findings + `## Clean` list.

---

### Task 10: Area pass 7 — `04-guides` (9 files)

**Files:**
- Read: `docs/04-guides/authentication.md`, `deployment.md`, `migrations.md`, `observability.md`, `plugin-development.md`, `proxy-cookbook.md`, `testing-and-debugging.md`, `wasm-development.md`, `workflow-patterns.md`
- Create: `.verification/findings/04-guides.md` (untracked)

**Interfaces:**
- Consumes: all `.verification/ground-truth/` artifacts, `.verification/snippets/report.md`.
- Produces: findings file. Executable flows (testing-and-debugging, deployment compose stanzas) are marked `## Deferred` for Task 11, same convention as Task 4.

- [ ] **Step 1: Verify each guide against its subsystem**

- `authentication.md` → `plugins/auth/` + `cmd/noda/auth_init.go` + scaffolded templates in `cmd/noda/auth_templates/` (the doc's flows must match the CURRENT templates — they were regenerated in PR #298; also check middleware auth modes against `internal/server/`).
- `deployment.md` → `docker-compose.yml`, `Dockerfile*`, release artifacts (binary names/platforms vs `.goreleaser`/workflow files), env vars vs `internal/config/` env handling.
- `migrations.md` → `internal/migrate/` + `cli/migrate.txt` dump (flag names, file naming conventions, up/down semantics).
- `observability.md` → OTel setup in `internal/trace/` (exporter env vars, span/attribute names the doc promises).
- `plugin-development.md` → `pkg/api/` interfaces (every interface/method the doc shows must compile against current `pkg/api` — signatures exactly).
- `proxy-cookbook.md` → `plugins/http/` request nodes + route config (recently genericized in commit `abf75a8` — verify examples still match schemas).
- `testing-and-debugging.md` → `internal/testing/` + `internal/devmode/` + `cli/test.txt`, `cli/dev.txt` (mock syntax, trace WebSocket claims).
- `wasm-development.md` → `pdk/` + `internal/wasm/` (host function names/signatures vs `docs/_internal/wasm-host-api.md` is OUT of scope, but pdk function names, tinygo build commands, and manifest fields are in scope — check against `pdk/` source and `examples/wasm-*/`).
- `workflow-patterns.md` → node usage vs `nodes.json` (same jq checks as node pages).

- [ ] **Step 2: Re-check this area's snippet failures**

```bash
grep '^docs/04-guides/' .verification/snippets/report.md | grep FAIL
```

Manual re-check rule as in Task 4 Step 2.

- [ ] **Step 3: Write the findings file**

Create `.verification/findings/04-guides.md` with findings + `## Deferred` + `## Clean`.

---

### Task 11: Execution pass — walkthroughs

**Files:**
- Read: `docs/01-getting-started/quick-start.md`, `realtime.md`, `installation.md`; `docs/04-guides/testing-and-debugging.md`; the `## Deferred` lists in `.verification/findings/01-getting-started.md` and `04-guides.md`
- Create: `.verification/findings/08-execution.md` (untracked)

**Interfaces:**
- Consumes: `.verification/noda` binary (Task 2), deferred lists from Tasks 4 and 10.
- Produces: findings file; every deferred item resolved to a finding, a pass, or an `## Unverified` entry with reason.

- [ ] **Step 1: Bring up the stack**

```bash
cd /Users/marten/GolandProjects/noda
docker compose up -d
docker compose ps
```

Expected: all services healthy. If the repo-root compose stack fails, that is itself a `breaks-user` finding (CLAUDE.md promises "docker compose up starts a working system").

- [ ] **Step 2: Execute quick-start.md literally**

In a scratch directory (`mkdir -p .verification/exec && cd .verification/exec`), execute every command block in `quick-start.md` in order, exactly as written (substitute nothing except unavoidable absolute paths). After each step, compare observed output/behavior with what the doc says happens. Any divergence → finding with both outputs quoted. Same for `installation.md`'s verification steps (skip actual reinstalls; verify command names and version output shape against the built binary).

- [ ] **Step 3: Execute the init → validate → test loop**

```bash
cd .verification/exec
../noda init verify-init && cd verify-init
../../noda validate --verbose
../../noda test
```

Expected: init scaffolds, validate passes, tests pass (the scaffold ships a sample test). Any failure → `breaks-user` finding against whichever doc documents `noda init` (quick-start/installation).

- [ ] **Step 4: Execute realtime.md and testing-and-debugging.md flows**

Follow each doc's walkthrough against the running stack (WebSocket/SSE connections with `curl`/`websocat` or a short Node/Go script if the doc implies a client; the doc's own commands take precedence). For the dev-mode trace WebSocket claims, run `.verification/noda dev` in the scaffolded project and probe as documented. Record divergences; record anything not executable (e.g. requires a browser) under `## Unverified` with the reason.

- [ ] **Step 5: Tear down and write findings**

```bash
cd /Users/marten/GolandProjects/noda && docker compose down -v
```

Create `.verification/findings/08-execution.md` with findings + `## Unverified` + `## Clean`.

---

### Task 12: Execution pass — `05-examples` (6 walkthroughs)

**Files:**
- Read: `docs/05-examples/rest-api.md`, `saas-backend.md`, `realtime-collab.md`, `discord-bot.md`, `multiplayer-game.md`, `video-conferencing.md`
- Create: `.verification/findings/09-examples.md` (untracked)

**Interfaces:**
- Consumes: `.verification/noda` binary; `examples/` projects; `.verification/snippets/report.md` (for the `05-examples` doc snippets).
- Produces: findings file; every walkthrough either executed or listed `## Unverified` with reason.

- [ ] **Step 1: Map walkthroughs to example projects**

`rest-api.md`→`examples/rest-api`, `saas-backend.md`→`examples/saas-backend`, `realtime-collab.md`→`examples/realtime-collab`, `discord-bot.md`→`examples/discord-bot`, `video-conferencing.md`→`examples/video-rooms`, `multiplayer-game.md`→ check the doc for its project reference (no obvious `examples/` dir — if it references config inline only, verify statically; if it references a missing project, that's a `breaks-user` finding).

- [ ] **Step 2: Static cross-check first**

For each walkthrough: every config snippet shown in the doc must match the actual file in the example project (`diff` the doc snippet against the project file it claims to show — quote drift is a `wrong` finding). Re-check this area's snippet FAILs: `grep '^docs/05-examples/' .verification/snippets/report.md | grep FAIL`.

- [ ] **Step 3: Execute each example**

For each example with a compose file (`examples/rest-api`, `saas-backend`, `realtime-collab` have them; check the others):

```bash
cd examples/<name>
docker compose up -d
# follow the doc's walkthrough requests (curl commands etc.) exactly
docker compose down -v
cd ../..
```

Run one example at a time (port conflicts). For each documented request → compare documented response with observed response. `discord-bot.md` external-webhook steps and `video-conferencing.md` LiveKit flows: execute only what the local stack supports; a LiveKit server counts as available only if the example's compose file provides one. Everything not executed goes under `## Unverified` with the reason.

- [ ] **Step 4: Write the findings file**

Create `.verification/findings/09-examples.md` with findings + `## Unverified` + `## Clean`.

---

### Task 13: File the GitHub issues

**Files:**
- Read: all files in `.verification/findings/`
- No repo file changes.

**Interfaces:**
- Consumes: the ten findings files (`00-inventory.md`, `01-getting-started.md`, `02-config.md`, `03-nodes-core.md`, `03-nodes-data.md`, `03-nodes-auth.md`, `03-nodes-livekit.md`, `04-guides.md`, `08-execution.md`, `09-examples.md`).
- Produces: one GitHub issue per findings file that contains at least one finding; plus one issue for the MCP plugin-list drift if confirmed in Task 2.

- [ ] **Step 1: Deduplicate and cross-reference**

Read all findings files. A systemic finding appearing in several areas (e.g. the same stale field name everywhere) stays in the most relevant file; other files replace their copy with one line: `(see <area> issue: <summary>)`.

- [ ] **Step 2: Ensure the label exists**

```bash
gh label list --search docs-verification | grep -q docs-verification || \
  gh label create docs-verification --description "Docs verification campaign 2026-07-15" --color 0E8A16
```

- [ ] **Step 3: File one issue per non-clean area**

For each findings file with ≥1 finding, build an issue body: a one-paragraph intro (`Findings from the 2026-07-15 docs-verification campaign — spec: docs/superpowers/specs/2026-07-15-docs-verification-design.md. Code on main is the source of truth.`), the severity legend, then every finding as a checklist item:

```markdown
- [ ] **[wrong]** `docs/03-nodes/db.query.md` §Config — <summary>
  Doc says: ... / Code does: ... (`plugins/db/query.go:42`)
```

Include the `## Unverified` section verbatim where present. Then:

```bash
gh issue create \
  --title "docs: <area> — verification findings" \
  --label docs-verification \
  --body-file .verification/issue-bodies/<area>.md
```

(Write each body to `.verification/issue-bodies/<area>.md` first; add `--label documentation` too if `gh label list --search documentation` shows it exists.) If Task 2 confirmed the MCP plugin-list drift, file it as its own issue titled `mcp: node registry misses plugins registered by the runtime (auth)` with label `bug` if it exists, else `docs-verification`.

- [ ] **Step 4: Final summary**

Produce a summary for the user: issue numbers with per-area finding counts by severity, the `## Unverified` totals, and where the campaign branch/tools live. Ask whether to PR `tools/docverify/` to main or delete the branch; do NOT do either without an answer.

---

## Self-Review Notes

- Spec coverage: Phase 0 → Tasks 1–3; the 7 static areas → Tasks 4–10; the 2 execution areas → Tasks 11–12; issue filing incl. severity format, dedupe, unverified reporting → Task 13; honesty rules embedded in Global Constraints and every snippet-recheck step.
- The spec's "9 issues max" becomes up to 10 findings files because inventory-level findings (Task 2) get their own file; they merge into the most relevant area issue OR stand alone if cross-cutting — Task 13 Step 1 owns that call. The optional MCP-drift issue is a code bug discovered by the campaign, outside the 9-areas count.
- The `internal/expr` compiler API in Task 3 was verified against source during planning (`NewCompilerWithFunctions`, wrapped-string `Compile`); no open API questions remain.
