# AI-Usability Test Harness Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a reusable harness that pits AI builder agents against the *real* `noda mcp` server across a graduated brief ladder, confirms genuine gaps with an adversarial evaluator, and produces a review-then-file findings report.

**Architecture:** Static inputs (5 brief markdown files + README) live under `tools/ai-usability/`. A single Workflow script (`harness.workflow.js`) runs three stages — Build (one MCP-only builder subagent per brief), Verify (an adversarial evaluator per friction finding), Synthesize (dedup → issue-ready entries). The real MCP server is connected to the session via `.mcp.json` so workflow subagents reach `noda_*` tools through ToolSearch. The harness returns structured findings; a human approves before any GitHub issue is filed.

**Tech Stack:** Workflow tool (JS orchestration scripts — no filesystem/Date/random), Go (`go build` the `noda` binary), Claude Code MCP (`.mcp.json`), `gh` CLI for issue filing.

## Global Constraints

- Module path is `github.com/chimpanze/noda`; build target is `./bin/noda` via `go build -o ./bin/noda ./cmd/noda`.
- Builder and evaluator subagents may learn Noda **only** through `noda_*` MCP tools and `noda://` resources — no prior Noda knowledge, no reading repo source (`internal/`, `plugins/`, `pkg/`, on-disk `docs/`), no web search.
- Workflow scripts have **no filesystem, `Date.now()`, or `Math.random()` access** — brief content is read by subagents (which have file tools), not by the script; vary per-item work by index, not randomness.
- MCP path tools take **absolute** paths: `noda_scaffold_project{path}`, `noda_validate_config{config_dir}`, `noda_read_project_file{config_dir,path}`, `noda_list_project_files{config_dir}`.
- Issue gate is **review-then-file**: the harness never calls `gh`; issues are filed only after the user approves the findings report.
- The five briefs target capabilities Noda demonstrably supports (mirrored by existing `examples/`), so an agent stall is a discovery/doc gap, not an impossible ask.
- `tools/ai-usability/` is under the gitignored `docs/superpowers`-style flow? No — it is a normal repo path and is committed normally. Generated `findings/<date>.md` is committed per run (progress tracking).

---

### Task 1: Brief ladder files

Five plain-English briefs, phrased the way a real user would ask — no Noda node names, no how-to. Each states the goal and what "done" looks like, nothing more.

**Files:**
- Create: `tools/ai-usability/briefs/01-trivial.md`
- Create: `tools/ai-usability/briefs/02-basic.md`
- Create: `tools/ai-usability/briefs/03-data.md`
- Create: `tools/ai-usability/briefs/04-auth.md`
- Create: `tools/ai-usability/briefs/05-hard.md`

**Interfaces:**
- Produces: brief files at the paths above; each builder subagent is told to `Read` its brief path. Brief ids used elsewhere: `01-trivial`, `02-basic`, `03-data`, `04-auth`, `05-hard`.

- [ ] **Step 1: Write `tools/ai-usability/briefs/01-trivial.md`**

```markdown
# Brief 01 — Trivial: a greeting endpoint

You are building a small web API with Noda.

**What I want:** A single HTTP `GET /hello` endpoint that returns JSON like
`{ "message": "Hello, world!" }`. The message should be produced by a workflow,
not hard-coded in the route itself.

**Done looks like:** The project validates cleanly, and hitting `GET /hello`
would return the JSON above.
```

- [ ] **Step 2: Write `tools/ai-usability/briefs/02-basic.md`**

```markdown
# Brief 02 — Basic: validated greeting

You are building a small web API with Noda.

**What I want:** A `POST /greet` endpoint that accepts a JSON body
`{ "name": "Ada" }`. Reject the request with a clear validation error if `name`
is missing or empty. On success, return `{ "message": "Hello, Ada!" }`, where the
name is taken from the request and the greeting is assembled in a workflow.

**Done looks like:** The project validates cleanly; a request with a name returns
the greeting, and a request without a name is rejected before the workflow runs.
```

- [ ] **Step 3: Write `tools/ai-usability/briefs/03-data.md`**

```markdown
# Brief 03 — Data: notes CRUD on Postgres

You are building a web API with Noda backed by PostgreSQL.

**What I want:** A "notes" resource with create, read-one, list, and delete
endpoints (`POST /notes`, `GET /notes/:id`, `GET /notes`, `DELETE /notes/:id`).
Notes have an `id`, a `title`, and a `body`, and are stored in Postgres. Include
whatever database migration is needed to create the table.

**Done looks like:** The project validates cleanly, the migration creates the
notes table, and the four endpoints read and write notes in the database.
```

- [ ] **Step 4: Write `tools/ai-usability/briefs/04-auth.md`**

```markdown
# Brief 04 — Auth: protected profile route

You are building a web API with Noda that requires authentication.

**What I want:** A `GET /me` endpoint that requires a logged-in user (JWT / OIDC
bearer token) and returns the caller's own profile. Unauthenticated requests must
be rejected with 401. Add a second endpoint `GET /admin/stats` that only users
with an `admin` role may access; everyone else gets 403.

**Done looks like:** The project validates cleanly; `/me` requires a valid token,
and `/admin/stats` additionally enforces the admin role.
```

- [ ] **Step 5: Write `tools/ai-usability/briefs/05-hard.md`**

```markdown
# Brief 05 — Hard: realtime board with auth + DB

You are building a web API with Noda combining authentication, a database, and a
realtime channel.

**What I want:** A small "board" app. Authenticated users (JWT / OIDC) can post
messages via `POST /messages`, which are stored in PostgreSQL. Connected clients
receive new messages in realtime over a WebSocket so the board updates live.
Only authenticated users may post or subscribe.

**Done looks like:** The project validates cleanly; posting a message persists it
and pushes it to subscribed WebSocket clients, and all of it is behind auth.
```

- [ ] **Step 6: Verify the briefs exist and read cleanly**

Run: `ls tools/ai-usability/briefs/ && head -3 tools/ai-usability/briefs/01-trivial.md`
Expected: all five files listed; the first lines of brief 01 print.

- [ ] **Step 7: Commit**

```bash
git add tools/ai-usability/briefs/
git commit -m "feat(ai-usability): add graduated brief ladder"
```

---

### Task 2: Harness README and directory layout

Document stage-0 setup, how to run the harness, and how findings become issues. Create the `findings/` directory so runs have a home.

**Files:**
- Create: `tools/ai-usability/README.md`
- Create: `tools/ai-usability/findings/.gitkeep`

**Interfaces:**
- Consumes: brief ids from Task 1.
- Produces: documented invocation — `Workflow({ scriptPath: 'tools/ai-usability/harness.workflow.js', args: { scratchRoot, only? } })` (script created in Task 4).

- [ ] **Step 1: Write `tools/ai-usability/README.md`**

````markdown
# Noda AI-Usability Test Harness

Pits AI builder agents against the **real** `noda mcp` server and reports where
they get stuck. See the design spec at
`docs/superpowers/specs/2026-06-25-ai-usability-harness-design.md`.

## What it does

1. **Build** — one builder subagent per brief (`briefs/*.md`), allowed to learn
   Noda *only* through `noda_*` MCP tools and `noda://` docs. It records a
   friction finding whenever the MCP surface falls short instead of guessing.
2. **Verify** — an adversarial evaluator independently tries to refute each
   finding; only genuine gaps survive.
3. **Synthesize** — confirmed gaps are deduped across briefs into issue-ready
   entries (title, body, labels, impact count).

The harness returns the findings; it never files issues itself.

## Stage 0 — one-time setup (required)

The workflow subagents reach `noda_*` tools only if the real server is connected
to this Claude Code session.

```bash
# 1. Build the binary
go build -o ./bin/noda ./cmd/noda
```

2. Register the server in the project `.mcp.json` (absolute path to the binary):

```json
{
  "mcpServers": {
    "noda": {
      "command": "/Users/marten/GolandProjects/noda/bin/noda",
      "args": ["mcp"]
    }
  }
}
```

3. **Restart Claude Code** (or reconnect MCP) so `noda_*` becomes available.
4. Sanity check: in the session, run a `ToolSearch` for `noda` and confirm
   `noda_list_nodes` is callable.

## Running the harness

From the main session (not a sub-subagent), invoke:

```
Workflow({
  scriptPath: 'tools/ai-usability/harness.workflow.js',
  args: { scratchRoot: '/tmp/noda-ai-usability' }      // dry-run subset: add  only: ['01-trivial']
})
```

The returned object has `confirmed_findings` and `issues`.

## Findings → issues (review-then-file)

1. Write the returned `issues` to `findings/<YYYY-MM-DD>.md` and commit it.
2. Review the report. For each entry you approve, file it:

```bash
gh issue create --repo chimpanze/noda \
  --title "<title>" --body "<body>" \
  --label documentation --label type:documentation
```

Nothing is filed without your approval.

## Re-running

Fix gaps, then re-run. Fewer confirmed findings = measurable progress.
````

- [ ] **Step 2: Create the findings directory placeholder**

```bash
mkdir -p tools/ai-usability/findings && touch tools/ai-usability/findings/.gitkeep
```

- [ ] **Step 3: Verify**

Run: `test -f tools/ai-usability/README.md && test -f tools/ai-usability/findings/.gitkeep && echo OK`
Expected: `OK`

- [ ] **Step 4: Commit**

```bash
git add tools/ai-usability/README.md tools/ai-usability/findings/.gitkeep
git commit -m "docs(ai-usability): add harness README and findings dir"
```

---

### Task 3: Connect the real MCP server (stage-0 prerequisite)

Build the binary and register `noda mcp` so workflow subagents can call the real tools. **This task requires a session restart and a human confirmation** — it cannot be fully automated by a subagent.

**Files:**
- Create: `bin/noda` (build artifact — ensure `bin/` is gitignored; do not commit)
- Create/Modify: `.mcp.json`

**Interfaces:**
- Produces: session-connected `noda_*` MCP tools (reachable via `ToolSearch` query `"noda"`), required by Tasks 4–6.

- [ ] **Step 1: Build the binary**

Run: `go build -o ./bin/noda ./cmd/noda && ./bin/noda version`
Expected: build succeeds; a version string prints.

- [ ] **Step 2: Ensure `bin/` is gitignored**

Run: `grep -qxF '/bin/' .gitignore || echo '/bin/' >> .gitignore`
Then: `git check-ignore -v bin/noda`
Expected: a line showing `bin/noda` is ignored.

- [ ] **Step 3: Write `.mcp.json`**

```json
{
  "mcpServers": {
    "noda": {
      "command": "/Users/marten/GolandProjects/noda/bin/noda",
      "args": ["mcp"]
    }
  }
}
```

- [ ] **Step 4: Restart the session and confirm the server is connected**

Restart Claude Code (or reconnect MCP). Then run a `ToolSearch` with query `noda`.
Expected: results include `noda_list_nodes`, `noda_get_node_schema`,
`noda_validate_config`, `noda_scaffold_project` (their schemas load on demand).

- [ ] **Step 5: Smoke-test one real tool call**

Load and call `noda_list_nodes` (no args).
Expected: a non-empty list of node types (e.g. `control.if`, `transform.set`,
`db.query`). If this fails, do not proceed — the rest of the harness depends on it.

- [ ] **Step 6: Commit the config**

```bash
git add .mcp.json .gitignore
git commit -m "chore(ai-usability): register real noda mcp server for the harness"
```

---

### Task 4: Write the harness Workflow script

One self-contained JS script with `meta`, JSON schemas, and the three stages. Builder/evaluator prompts are embedded verbatim — they are load-bearing.

**Files:**
- Create: `tools/ai-usability/harness.workflow.js`

**Interfaces:**
- Consumes: `args.scratchRoot` (string, absolute), optional `args.only` (array of brief ids); session-connected `noda_*` tools from Task 3; brief files from Task 1.
- Produces: a return value `{ confirmed_findings: Array, issues: Array }`. `issues[i]` has `{ title, body, labels: string[], brief_ids: string[], impact_count: number }`. Consumed by Tasks 6–7.

- [ ] **Step 1: Write `tools/ai-usability/harness.workflow.js`**

```javascript
export const meta = {
  name: 'ai-usability-harness',
  description: 'Pit AI builder agents against the real noda MCP server and report friction gaps',
  phases: [
    { title: 'Build', detail: 'one MCP-only builder subagent per brief' },
    { title: 'Verify', detail: 'adversarial evaluator per friction finding' },
    { title: 'Synthesize', detail: 'dedup confirmed gaps into issue-ready entries' },
  ],
}

const BRIEFS = [
  { id: '01-trivial', path: 'tools/ai-usability/briefs/01-trivial.md' },
  { id: '02-basic', path: 'tools/ai-usability/briefs/02-basic.md' },
  { id: '03-data', path: 'tools/ai-usability/briefs/03-data.md' },
  { id: '04-auth', path: 'tools/ai-usability/briefs/04-auth.md' },
  { id: '05-hard', path: 'tools/ai-usability/briefs/05-hard.md' },
]

const SEVERITY = { type: 'string', enum: ['blocker', 'major', 'minor'] }
const CATEGORY = {
  type: 'string',
  enum: ['missing-doc', 'confusing-tool-output', 'undiscoverable-node', 'schema-gap', 'example-gap', 'other'],
}

const BUILDER_SCHEMA = {
  type: 'object',
  additionalProperties: false,
  required: ['brief_id', 'completed', 'validation_status', 'friction'],
  properties: {
    brief_id: { type: 'string' },
    completed: { type: 'boolean' },
    validation_status: { type: 'string' },
    friction: {
      type: 'array',
      items: {
        type: 'object',
        additionalProperties: false,
        required: ['goal', 'consulted', 'missing_or_confusing', 'severity', 'suggested_fix', 'category'],
        properties: {
          goal: { type: 'string' },
          consulted: { type: 'string' },
          missing_or_confusing: { type: 'string' },
          severity: SEVERITY,
          suggested_fix: { type: 'string' },
          category: CATEGORY,
        },
      },
    },
  },
}

const VERDICT_SCHEMA = {
  type: 'object',
  additionalProperties: false,
  required: ['is_real_gap', 'evidence'],
  properties: {
    is_real_gap: { type: 'boolean' },
    evidence: { type: 'string' },
    corrected_severity: SEVERITY,
  },
}

const ISSUES_SCHEMA = {
  type: 'object',
  additionalProperties: false,
  required: ['issues'],
  properties: {
    issues: {
      type: 'array',
      items: {
        type: 'object',
        additionalProperties: false,
        required: ['title', 'body', 'labels', 'brief_ids', 'impact_count'],
        properties: {
          title: { type: 'string' },
          body: { type: 'string' },
          labels: { type: 'array', items: { type: 'string' } },
          brief_ids: { type: 'array', items: { type: 'string' } },
          impact_count: { type: 'integer' },
        },
      },
    },
  },
}

function builderPrompt(b, dir) {
  return [
    'You are an AI developer building a project with Noda, using ONLY the Noda MCP server to learn how Noda works.',
    `Your task is described in the file: ${b.path} — Read it first.`,
    `Your project directory is: ${dir} — create it and pass it as the absolute path to noda_scaffold_project and noda_validate_config.`,
    '',
    'HARD RULES:',
    '- Load the Noda MCP tools via ToolSearch (query "noda"), then use only tools named noda_* and noda:// resources.',
    '- Do NOT use any prior knowledge of Noda. Do NOT read the Noda source repo (internal/, plugins/, pkg/, or docs/ on disk — only noda:// resources are allowed). Do NOT web search.',
    '- Whenever you need information to proceed that the MCP surface does NOT provide (a node you cannot discover, a schema field with no explanation, a doc that is missing or unclear, confusing tool output), STOP that line of work and record a FRICTION finding instead of guessing.',
    '',
    'STEPS: (1) Read the brief. (2) noda_scaffold_project at your dir. (3) Discover what you need via noda_list_nodes, noda_get_node_schema, noda_get_config_schema, noda_get_examples, noda_list_functions, noda_validate_expression. (4) Write the config files to satisfy the brief. (5) Validate with noda_validate_config. (6) Return your result.',
    '',
    'Return ONLY the structured object: brief_id (use ' + JSON.stringify(b.id) + '), completed (true only if validation passed AND the brief is satisfied), validation_status (the validator summary, verbatim), and friction (every point where the MCP surface fell short — empty array if none). Each friction item needs: goal, consulted (which tools/docs you checked), missing_or_confusing, severity, suggested_fix, category.',
  ].join('\n')
}

function evaluatorPrompt(b, f, dir) {
  return [
    'You are a skeptical evaluator. A builder agent restricted to the Noda MCP server claimed it hit this friction point while working on brief ' + b.id + ':',
    JSON.stringify(f, null, 2),
    '',
    'Your job is to REFUTE it if you can. Using ONLY the noda_* MCP tools and noda:// resources (load via ToolSearch query "noda"), look harder than the builder did and determine whether the information it needed is actually reachable.',
    `You may inspect the builder's project at ${dir} via noda_read_project_file / noda_list_project_files if relevant.`,
    '',
    'Decide: if the needed info IS reachable through the MCP surface, this is NOT a real gap (is_real_gap=false) — cite exactly where it is in `evidence`. If after genuine effort you also cannot find it, it IS a real gap (is_real_gap=true). If the finding only exists because the builder relied on forbidden sources (repo source / web / prior knowledge), set is_real_gap=false and say so. Default to is_real_gap=false when uncertain. Optionally set corrected_severity. Return the structured verdict.',
  ].join('\n')
}

function synthPrompt(findings) {
  return [
    'You are turning confirmed Noda MCP-server gaps into GitHub issues for the chimpanze/noda repo.',
    'These findings were already independently verified as real gaps:',
    JSON.stringify(findings, null, 2),
    '',
    'Deduplicate: the same underlying gap reported across multiple briefs becomes ONE issue. Collect its brief_ids and set impact_count to the number of distinct briefs that hit it.',
    'For each distinct gap produce: title (concise, imperative), body (sections: Context / What is missing or confusing / How to reproduce (name the brief(s)) / Suggested fix), labels (choose any that fit from: documentation, type:documentation, type:test, enhancement, type:feature, component:engine, component:config, component:plugin, component:server), brief_ids, impact_count.',
    'Return the issues array.',
  ].join('\n')
}

const scratchRoot = (args && args.scratchRoot) || '/tmp/noda-ai-usability'
const only = args && args.only
const briefs = only ? BRIEFS.filter((b) => only.includes(b.id)) : BRIEFS

phase('Build')
const perBrief = await pipeline(
  briefs,
  (b) => agent(builderPrompt(b, `${scratchRoot}/${b.id}`), { label: `build:${b.id}`, phase: 'Build', schema: BUILDER_SCHEMA }),
  (result, b) => {
    if (!result || !Array.isArray(result.friction) || result.friction.length === 0) {
      return { brief: b.id, confirmed: [] }
    }
    return parallel(
      result.friction.map((f, i) => () =>
        agent(evaluatorPrompt(b, f, `${scratchRoot}/${b.id}`), { label: `verify:${b.id}#${i}`, phase: 'Verify', schema: VERDICT_SCHEMA })
          .then((v) => ({ ...f, brief_id: b.id, verdict: v }))
      )
    ).then((arr) => ({ brief: b.id, confirmed: arr.filter((x) => x && x.verdict && x.verdict.is_real_gap) }))
  }
)

phase('Synthesize')
const confirmed_findings = perBrief.filter(Boolean).flatMap((x) => x.confirmed)
log(`${confirmed_findings.length} confirmed gap(s) across ${briefs.length} brief(s)`)
if (confirmed_findings.length === 0) {
  return { confirmed_findings: [], issues: [] }
}
const synth = await agent(synthPrompt(confirmed_findings), { label: 'synthesize', phase: 'Synthesize', schema: ISSUES_SCHEMA })
return { confirmed_findings, issues: synth ? synth.issues : [] }
```

- [ ] **Step 2: Sanity-check the script parses (structure only)**

Run: `node --check tools/ai-usability/harness.workflow.js 2>/dev/null && echo "parse-ok" || echo "note: ESM export — parse check may warn; verified by dry-run in Task 5"`
Expected: either `parse-ok`, or the note (the real verification is the Task 5 dry-run, since `export`/workflow globals aren't valid standalone Node).

- [ ] **Step 3: Commit**

```bash
git add tools/ai-usability/harness.workflow.js
git commit -m "feat(ai-usability): add harness workflow script (build/verify/synthesize)"
```

---

### Task 5: Dry-run on the trivial brief

Run the harness against `01-trivial` only, in the **main session** (so the nested workflow can reach session MCP tools). Confirm a builder produces well-formed structured output and the pipeline completes. Fix the script if the output shape is wrong.

**Files:**
- Modify (only if needed): `tools/ai-usability/harness.workflow.js`

**Interfaces:**
- Consumes: Task 3 (MCP connected), Task 4 (script).

- [ ] **Step 1: Invoke the harness for the trivial brief**

In the main session, call:
```
Workflow({
  scriptPath: 'tools/ai-usability/harness.workflow.js',
  args: { scratchRoot: '/tmp/noda-ai-usability', only: ['01-trivial'] }
})
```

- [ ] **Step 2: Verify the run shape**

Expected:
- The Build phase shows one `build:01-trivial` agent.
- If that builder reported friction, a `verify:01-trivial#…` agent ran per finding.
- The return value is an object with `confirmed_findings` (array) and `issues` (array). For the trivial brief, `issues` may legitimately be empty (no real gaps) — that is a valid pass.

- [ ] **Step 3: Verify the builder actually used the real MCP server**

Confirm (from the run/agent transcript) the builder called real `noda_*` tools
(e.g. `noda_scaffold_project`, `noda_validate_config`) rather than inventing
config. Confirm a scratch project exists:
Run: `ls /tmp/noda-ai-usability/01-trivial/ 2>/dev/null && echo "scaffolded"`
Expected: project files (`noda.json`, `routes/`, `workflows/`) and `scaffolded`.

- [ ] **Step 4: Confirm the evaluator does not rubber-stamp (known-gap check)**

Verify the verify-stage can both confirm and reject. Temporarily add a brief
`tools/ai-usability/briefs/99-known-gap.md` that asks for a capability Noda does
not document (e.g. "expose a GraphQL endpoint"), add `{ id: '99-known-gap', path: ... }`
to `BRIEFS` in the script, and run with `only: ['99-known-gap']`.
Expected: the builder records a blocker friction and the evaluator returns
`is_real_gap:true`. Then remove the temp brief and the `BRIEFS` entry — this is a
one-off check, not part of the committed ladder. (If you prefer, skip the GraphQL
brief and instead read the dry-run transcript to confirm at least one evaluator
verdict cites concrete `noda://` evidence when refuting, proving it actually looked.)

- [ ] **Step 5: Fix and re-run if the shape is wrong**

If the structured output failed validation or the builder ignored the MCP-only
rules, adjust the prompts/schemas in `harness.workflow.js` and re-invoke Step 1
until the shape is correct. Commit any fix:

```bash
git add tools/ai-usability/harness.workflow.js
git commit -m "fix(ai-usability): correct harness output after dry-run"
```

(No commit if no change was needed.)

---

### Task 6: Full run and findings report

Run all five briefs, then write the returned issues to a dated findings file and commit it.

**Files:**
- Create: `tools/ai-usability/findings/2026-06-25.md` (use the actual run date)

**Interfaces:**
- Consumes: Task 4 return shape (`{ confirmed_findings, issues }`).
- Produces: a committed findings report for the review gate (Task 7).

- [ ] **Step 1: Run the full ladder**

In the main session, call:
```
Workflow({
  scriptPath: 'tools/ai-usability/harness.workflow.js',
  args: { scratchRoot: '/tmp/noda-ai-usability' }
})
```

- [ ] **Step 2: Write the findings report**

Create `tools/ai-usability/findings/<run-date>.md` from the returned `issues`,
one section per issue using this exact template per entry:

```markdown
## <title>

- **Labels:** <comma-separated labels>
- **Hit by:** <brief_ids>  (impact: <impact_count>)

<body>

---
```

Add a one-line summary header: `# AI-usability findings — <run-date> (<N> confirmed gaps)`.

- [ ] **Step 3: Verify the report**

Run: `test -s tools/ai-usability/findings/*.md && grep -c '^## ' tools/ai-usability/findings/*.md`
Expected: the file is non-empty and the `## ` count equals the number of returned issues (0 is a valid result — record "no confirmed gaps" in that case).

- [ ] **Step 4: Commit**

```bash
git add tools/ai-usability/findings/
git commit -m "docs(ai-usability): findings report for <run-date> run"
```

---

### Task 7: Review gate and issue filing

Human reviews the findings report; approved entries are filed as GitHub issues. **No issue is created without explicit approval.**

**Files:**
- None (uses the committed findings report).

**Interfaces:**
- Consumes: `tools/ai-usability/findings/<run-date>.md` from Task 6.

- [ ] **Step 1: Present the findings for review**

Show the user the findings report and ask which entries to file (all / a subset / none). Wait for an explicit answer.

- [ ] **Step 2: File each approved issue**

For each approved entry, run (substituting title/body/labels from the report):
```bash
gh issue create --repo chimpanze/noda \
  --title "<title>" \
  --body "<body>" \
  --label "<label1>" --label "<label2>"
```

- [ ] **Step 3: Verify issues were created**

Run: `gh issue list --repo chimpanze/noda --limit 10`
Expected: the newly filed issues appear.

- [ ] **Step 4: Record what was filed**

Append a `## Filed` section to the findings report listing the created issue
numbers/URLs, then commit:
```bash
git add tools/ai-usability/findings/
git commit -m "docs(ai-usability): record filed issues for <run-date>"
```

---

## Notes for the executor

- **Tasks 5 and 6 run in the main session**, not inside a dispatched sub-subagent — the harness Workflow spawns its own subagents and needs the session-connected `noda_*` MCP tools. If executing this plan subagent-driven, hand Tasks 3, 5, 6, 7 back to the main session (they need restart, MCP, or human approval).
- If a builder finishes with `completed:false` but `friction:[]`, treat that as a harness bug (it stalled without recording why) — tighten the builder prompt's stop-and-record rule.
- The brief scratch dirs under `scratchRoot` are disposable; delete between full runs to avoid a builder "completing" by reusing a prior run's files: `rm -rf /tmp/noda-ai-usability`.
