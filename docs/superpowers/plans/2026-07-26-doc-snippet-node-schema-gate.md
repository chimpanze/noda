# Doc Snippet Node-Schema Gate Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fix every dead-on-arrival JSON snippet in the user-facing docs, and add a CI gate so the class cannot come back (#445, expanded).

**Architecture:** `tools/docverify/snippets` already extracts fenced `json` blocks from the user-facing docs and checks that they parse and that `{{ }}` expressions compile — but it never validates node configs against their `ConfigSchema`, which is exactly why `fields` vs `data` shipped. Extend that package with a node-schema check that runs each workflow-shaped block through the **real** `registry.ValidateStartupDryRun`, and add a Go test that fails on violations. `make test-coverage` (what CI runs) executes `go test $(GO_PKGS)`, and `GO_PKGS` is `go list ./...`, so a test in that package gates CI with no workflow change.

**Tech Stack:** Go, testify, `internal/registry` + `plugins/all` (the same validator `noda validate` uses).

## Global Constraints

- The gate must reuse `registry.ValidateStartupDryRun`, not reimplement schema checking. Docs must be validated against the real rules or the gate rots.
- Scope of docs checked: `docs/{01-getting-started,02-config,03-nodes,04-guides,05-examples}/*.md` — the same set `tools/docverify/snippets/main.go:94` already uses. `docs/_internal/` is excluded (architecture notes, deliberately abstract).
- **`service "X" not found in config (slot: Y)` must be excluded** from the gate. A snippet is validated in isolation with no `services` block, so that error is an artifact of the harness, not a doc defect. Every other error class is a real finding.
- Never weaken a doc snippet to satisfy the gate (e.g. deleting a node). Fix the config to be correct and copy-pasteable.
- Work happens in the worktree `.claude/worktrees/fix-445-auth-guide-data-field` on branch `worktree-fix-445-auth-guide-data-field`. Confirm `git rev-parse --show-toplevel` before every commit.
- `/docs/superpowers/` is gitignored but its files are tracked; use `git add -f` for anything under it.

## Baseline

`docs/04-guides/authentication.md` is already fixed and clean (4 snippets: `db.create` and `db.upsert` `fields`→`data`, two `db.exec` `sql`→`query`). The remaining **26 violations across 5 files** are listed per-file in Task 1.

---

### Task 1: Fix the remaining dead-on-arrival snippets

**Files:**
- Modify: `docs/02-config/connections.md` (blocks at ~274, ~312, ~380)
- Modify: `docs/02-config/schedules.md` (block at ~180)
- Modify: `docs/01-getting-started/services.md` (block at ~561)
- Modify: `docs/01-getting-started/data-flow.md` (blocks at ~41, ~129)
- Modify: `docs/01-getting-started/expression-cookbook.md` (block at ~474)
- Modify: `docs/04-guides/proxy-cookbook.md` (block at ~133)

**Interfaces:** none — documentation only. Task 2 depends on this task leaving zero violations.

The violations, grouped by root cause:

**(a) `"service": "name"` written as a *config* field instead of the node's `services` block.** The correct shape is a sibling of `"type"`, not a key inside `"config"`:
```json
"save_message": {
  "type": "db.create",
  "services": { "database": "main-db" },
  "config": { "table": "messages", "data": { } }
}
```
Sites: `connections.md` `add_presence` (cache.set, slot `cache`), `save_message` (db.create, slot `database`), `remove_presence` (cache.del, slot `cache`); `schedules.md` `delete_sessions` and `delete_soft_deleted` (db.exec, slot `database`). Use a service name consistent with the surrounding prose in each file.

**(b) `db.exec` uses `sql`; the schema requires `query`** (`plugins/db/exec.go:32`). Sites: `schedules.md` `delete_sessions`, `delete_soft_deleted`. Rename the key only — leave the SQL text and `params` alone.

**(c) `http.get` uses `path`; the schema requires `url`** (`plugins/http/get.go:25`). Site: `services.md` `get_billing`. Also `proxy-cookbook.md` `fetch` passes an unknown `query` field — check `plugins/http/get.go`'s `ConfigSchema` for the supported way to express query parameters and use it, or fold the parameters into the `url`.

**(d) `control.switch` uses `value`; the schema requires `expression` and `cases`** (`plugins/core/control/switch.go:22`). Site: `connections.md` `route`. This one needs judgment, not a rename: read the surrounding prose and the node's outgoing edges, then write an `expression` plus a `cases` block that produces exactly the branching the snippet already illustrates. Do not change which branches exist.

**(e) Missing required config fields.** `util.log` requires `level` and `message` (`plugins/core/util/log.go:23`) — sites: `connections.md` `log_unknown`, `schedules.md` `log_result`. `response.error` requires `code` and `message` (`plugins/core/response/error.go:25`) — site: `expression-cookbook.md` `error_response`. Add values that match what the snippet is illustrating.

**(f) Missing `services` block on nodes that need one.** Sites: `data-flow.md` `lookup` (db.findOne, slot `database`) and `create` (slot `database`); `workflow-patterns.md` `fetch` (slot `client`); `connections.md` `broadcast_join`/`broadcast_message`/`broadcast_typing`/`broadcast_leave` (slot `connections`). These are illustrative fragments that omitted the block for brevity; adding it makes them copy-pasteable. One line each.

- [ ] **Step 1: Reproduce the baseline with a throwaway scanner**

Task 2 builds the permanent version; for now you need something to iterate against. Create `zz_docscan_test.go` at the repo root:

```go
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/chimpanze/noda/internal/config"
	nodeexpr "github.com/chimpanze/noda/internal/expr"
	"github.com/chimpanze/noda/internal/registry"
	"github.com/chimpanze/noda/plugins/all"
)

func TestScanDocSnippets(t *testing.T) {
	var files []string
	for _, d := range []string{"01-getting-started", "02-config", "03-nodes", "04-guides", "05-examples"} {
		m, _ := filepath.Glob(filepath.Join("docs", d, "*.md"))
		files = append(files, m...)
	}
	sort.Strings(files)

	plugins := registry.NewPluginRegistry()
	nodes := registry.NewNodeRegistry()
	for _, p := range all.Core() {
		_ = plugins.Register(p)
		_ = nodes.RegisterFromPlugin(p)
	}

	total := 0
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		lines := strings.Split(string(data), "\n")
		in, start := false, 0
		var buf []string
		for i, line := range lines {
			tr := strings.TrimSpace(line)
			if !in && strings.HasPrefix(tr, "```json") {
				in, start, buf = true, i+2, nil
				continue
			}
			if in && tr == "```" {
				in = false
				var keep []string
				for _, b := range buf {
					if !strings.HasPrefix(strings.TrimSpace(b), "//") {
						keep = append(keep, b)
					}
				}
				var wf map[string]any
				if json.Unmarshal([]byte(strings.Join(keep, "\n")), &wf) == nil {
					if _, ok := wf["nodes"].(map[string]any); ok {
						rc := &config.ResolvedConfig{Workflows: map[string]map[string]any{
							fmt.Sprintf("%s:%d", f, start): wf,
						}}
						for _, e := range registry.ValidateStartupDryRun(rc, plugins, nodes, nodeexpr.NewCompilerWithFunctions(), nil) {
							if strings.Contains(e.Error(), "not found in config (slot:") {
								continue
							}
							total++
							t.Logf("HIT %s", e.Error())
						}
					}
				}
				continue
			}
			if in {
				buf = append(buf, line)
			}
		}
	}
	t.Logf("scanned %d files, %d violations", len(files), total)
}
```

Run it and record the starting count:

```bash
go test -run TestScanDocSnippets -v . 2>&1 | grep -E "HIT|scanned"
```

Expected: 26 violations across the 5 files listed above, and **zero** in `docs/04-guides/authentication.md` (already fixed).

- [ ] **Step 2: Fix the violations, file by file**

Work one file at a time, re-running the scanner after each so you always know which fixes landed. Consult the node's `ConfigSchema` in the plugin source before each edit — the schema is the authority, not the surrounding prose. For every edit, keep the snippet teaching the same thing it taught before; you are correcting how it is expressed, not what it demonstrates.

- [ ] **Step 3: Confirm zero violations**

```bash
go test -run TestScanDocSnippets -v . 2>&1 | grep -E "HIT|scanned"
```

Expected: `scanned N files, 0 violations`, no HIT lines.

- [ ] **Step 4: Confirm the existing snippet tool still passes**

Your edits must not break JSON parsing or expression compilation.

```bash
go run ./tools/docverify/snippets && tail -20 .verification/snippets/report.md
```

Expected: no PARSE-FAIL or EXPR-FAIL entries. Report the actual counts.

- [ ] **Step 5: Delete the throwaway and commit**

```bash
rm zz_docscan_test.go
git rev-parse --show-toplevel   # must end in fix-445-auth-guide-data-field
git status --porcelain          # only the doc files should appear
git add docs/
git commit -m "docs: fix node configs in snippets that fail schema validation (#445)

Every fenced json workflow snippet in the user-facing docs now passes the
same node ConfigSchema validation noda validate runs. Fixes: db.exec sql
-> query, http.get path -> url, control.switch value -> expression+cases,
'service' written as a config field instead of the services block, and
missing required fields on util.log and response.error.

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 2: Make the gate permanent

**Files:**
- Modify: `tools/docverify/snippets/main.go` (add the node-schema check to the report the tool already produces)
- Create: `tools/docverify/snippets/nodeschema_test.go` (the CI gate)

**Interfaces:**
- Consumes: `extractJSONBlocks(file, content string) []block` and `stripComments(s string) string`, both already in `tools/docverify/snippets/main.go`. Reuse them; do not write a second extractor.
- Produces: an exported-to-the-package function `nodeSchemaViolations(files []string) ([]string, error)` (or equivalent) used by both `main.go`'s report and the test.

- [ ] **Step 1: Add the check to the tool**

Factor the scanning logic from Task 1's throwaway into `tools/docverify/snippets/main.go`, reusing the existing `extractJSONBlocks` and `stripComments` rather than duplicating them. It must:
- consider only blocks whose parsed JSON has a top-level `nodes` object;
- run each through `registry.ValidateStartupDryRun` with a registry built from `all.Core()`;
- **also run each through `engine.Compile`** — see below;
- drop errors containing `not found in config (slot:` — a snippet has no `services` block, so that class is a harness artifact and not a doc defect. Put that reasoning in a comment; a future reader will otherwise "fix" the filter and drown the gate in noise.
- report every other error with file and line.

**Why `engine.Compile` as well, and not just the dry-run.** The Task 1 review found a snippet in `docs/02-config/workflows.md` that put a `retry` block on a `success` edge. `internal/engine/compiler.go:223` rejects `retry` on any non-`error` edge — but `ValidateStartupDryRun` never calls `engine.Compile`, so that snippet passed both `noda validate` and the node-schema scan while hard-failing at real `noda start`/`noda dev` boot (`engine.NewWorkflowCache` → `buildGraphs` → `Compile`). A gate that only checks node `ConfigSchema` would certify that snippet as good. Graph-level rules — edge outputs, retry placement, unknown edge targets, cycles — live in the compiler, so the gate has to run it or it inherits the same blind spot that let this ship.

`engine.Compile` needs a node resolver; construct it the same way the test runner does (see `internal/testing/runner.go`'s `buildTestRegistry`/`engine.Compile` call for the shape). If a snippet cannot be compiled for a reason inherent to being a fragment rather than a defect, treat that the same way as the `not found in config (slot:` filter — narrowly, by message, with the reasoning in a comment. Do not broaden the filter to make the count go down.

Include the violations in the tool's existing report output so `go run ./tools/docverify/snippets` shows them.

- [ ] **Step 2: Add the CI gate test**

Create `tools/docverify/snippets/nodeschema_test.go` with a test that fails when any violation exists. The test's working directory is the package directory, so the docs root is `../../../docs` — resolve it relative to the test file, and `t.Fatal` with a clear message if the docs directory cannot be found (a silently-empty scan is a gate that passes for the wrong reason).

The failure message must name the file, line, and the exact validator error, so a contributor can act on it without rerunning anything. Also assert that the scan actually examined a non-zero number of workflow blocks — otherwise a future refactor that breaks block extraction turns the gate green.

- [ ] **Step 3: Verify the gate passes on the fixed docs**

```bash
go test ./tools/docverify/snippets/ -v 2>&1 | tail -20
```

Expected: PASS, with the block count logged.

- [ ] **Step 4: Mutation-check the gate — this is mandatory**

A gate that cannot fail is worse than no gate. Reintroduce one of the bugs Task 1 fixed, confirm the test goes red with a useful message, then revert.

```bash
# reintroduce: change one "data" back to "fields" in docs/04-guides/authentication.md
go test ./tools/docverify/snippets/ 2>&1 | tail -20
# then revert the edit and re-run — must be green again
```

Report both outputs verbatim. Confirm `git status --porcelain` is clean afterwards apart from your intended new/modified files.

- [ ] **Step 5: Confirm it runs under the command CI actually uses**

CI runs `make test-coverage`, which is `go test $(GO_PKGS)` where `GO_PKGS := $(shell go list ./... | grep -v '/node_modules/')`. Prove the new package is in that set:

```bash
go list ./... | grep -v '/node_modules/' | grep docverify
```

Expected: `tools/docverify/snippets` appears. Report the actual output.

- [ ] **Step 6: Document the gate**

Add a short paragraph to `docs/04-guides/testing-and-debugging.md` (which already covers how this repo tests things) stating that fenced `json` workflow snippets in the user-facing docs are validated against node `ConfigSchema`s in CI, and what a contributor should do when the gate fails. Keep it to a few sentences in the file's existing voice.

- [ ] **Step 7: Commit**

```bash
git rev-parse --show-toplevel   # must end in fix-445-auth-guide-data-field
git add tools/docverify/snippets/ docs/04-guides/testing-and-debugging.md
git commit -m "test(docs): gate doc snippets on node ConfigSchema validation (#445)

tools/docverify/snippets checked that fenced json blocks parse and that
expressions compile, but never validated node configs against their
ConfigSchema — which is how snippets that noda validate rejects shipped.
The check now runs the real ValidateStartupDryRun over every workflow
block and fails in CI via make test-coverage.

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

## Final Verification

- [ ] **Full build, vet, and test sweep**

```bash
go build ./... && go vet ./... && go test ./... 2>&1 | grep -vE "^ok|no test files" | head -20
```

Expected: no failures. Known flakes unrelated to this change: `TestWatcher_Debounce` (#347) and `TestEventHub_NoGoroutineLeakOnUnsubscribe` at `-count>1` (#416).

- [ ] **The docs still read correctly**

Skim each changed block in context. The gate proves the snippets are *valid*; only reading proves they still teach what the surrounding prose says they teach. Report anything where the fix changed the meaning.
