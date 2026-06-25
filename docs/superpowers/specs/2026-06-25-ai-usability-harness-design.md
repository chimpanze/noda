# Noda AI-Usability Test Harness — Design

**Date:** 2026-06-25
**Status:** Approved (brainstorm) — pending implementation plan

## Problem

Noda ships an MCP server (`internal/mcp/`) and a `noda init` scaffold wired for Claude Code,
explicitly so "AI agents can discover and build Noda projects interactively." We have never
systematically tested whether that promise holds: **can an AI agent, given only the MCP tools and
docs, actually build a working Noda project — and where does it get stuck?**

We want a reusable harness that pits AI agents against the *real* MCP server, records every point
of friction (missing docs, confusing tool output, undiscoverable nodes, dead ends), confirms which
are genuine gaps, and turns those into GitHub issues for the `chimpanze/noda` repo.

## Goals

- Exercise the **real** `noda mcp` server end-to-end, not an approximation.
- Surface friction at graduated difficulty so we learn *where* the AI experience breaks down.
- Distinguish genuine gaps from "the agent didn't look" before anything is filed.
- Produce a deduped, labeled findings report; file GitHub issues only after human review.
- Be **re-runnable** so we can measure improvement after fixing gaps.

## Non-Goals

- Not a benchmark of model quality — we test Noda's AI surface, not the agent's intelligence.
- Not an automatic doc/code fixer. It finds and reports gaps; fixes are separate work.
- No changes to the MCP server or runtime as part of this harness.

## Decisions (from brainstorm)

| Decision | Choice |
|---|---|
| Runner model | Automated subagent fleet (Workflow) |
| Fidelity | Real MCP server — build binary, register `noda mcp`, restart session |
| Brief set | Graduated ladder, ~5 briefs |
| Issue gate | Review-then-file (human approves before `gh issue create`) |

## Architecture

Four stages. Stage 0 is one-time setup; stages 1–3 are the Workflow; the review gate is manual.

```
[0. Setup — one-time, manual]
    go build -o ./bin/noda ./cmd/noda
    register `./bin/noda mcp` as a session MCP server (.mcp.json)
    restart Claude Code so the noda_* tools are session-connected
        │
[1. Build]   pipeline over N briefs
    each brief → "builder" subagent, isolated scratch dir, ONLY noda_* tools + noda:// docs
    returns: { completed, friction[], validation_status }
        │
[2. Verify]  per friction finding → independent "evaluator" subagent (adversarial)
    confirms: is this info REALLY absent from the MCP surface, or did the builder miss it?
    drops false positives
        │
[3. Synthesize]  dedup confirmed gaps across briefs → group by theme → map to repo labels
    → write findings/<date>.md
        │
[review gate — manual]  user approves → `gh issue create` per confirmed gap
```

Stages 1→2 form a `pipeline`: a brief's findings are verified as soon as that brief finishes,
while later briefs are still building. Stage 3 runs after the pipeline (it needs all confirmed
findings at once to dedup) — a single barrier is correct here.

### Stage 0 — Setup (prerequisite, not part of the Workflow)

The MCP project tools resolve paths from caller-supplied arguments, not a fixed root:
`noda_scaffold_project`, `noda_read_project_file`, `noda_list_project_files` each take an explicit
`path` / `config_dir` (relativized to cwd if not absolute — `internal/mcp/tools.go:735`). So each
builder can be handed its own absolute scratch dir; no per-agent server instance is needed.

Setup steps (documented in `tools/ai-usability/README.md`):
1. Build the binary: `go build -o ./bin/noda ./cmd/noda`.
2. Add an MCP server entry pointing at `./bin/noda mcp` (stdio) to the project `.mcp.json`.
3. Restart Claude Code so workflow subagents can reach `noda_*` via ToolSearch.
4. Sanity check: confirm `noda_list_nodes` is callable before running the harness.

### Stage 1 — Build (per-brief builder subagent)

Each builder is prompted as an AI user building a Noda project with these hard rules:

- Learn Noda **only** through `noda_*` MCP tools and `noda://` resources. No prior Noda knowledge,
  no reading the Noda repo source, no web search.
- Work **only** inside the assigned scratch dir (passed in the prompt).
- When information needed to proceed is **not** obtainable from the MCP surface, **stop and record a
  friction finding** rather than guess.
- Finish by validating: `noda_validate_config` on the project and/or running `noda test`.

Builder returns a structured result (enforced via Workflow `schema`):

```
{
  brief_id, completed: bool, validation_status: string,
  friction: [{
    goal,                 // what I was trying to do
    consulted,            // which tool(s)/doc(s) I used to look
    missing_or_confusing, // what wasn't there or was unclear
    severity,             // blocker | major | minor
    suggested_fix,        // what doc/tool change would resolve it
    category              // missing-doc | confusing-tool-output | undiscoverable-node | schema-gap | example-gap | other
  }]
}
```

### Stage 2 — Verify (per-finding evaluator subagent)

For each friction finding, an independent evaluator (also restricted to the MCP surface) tries to
**refute** it: can the needed info actually be found via `noda_*` tools or `noda://` docs? Default to
"confirmed gap" only when the evaluator also fails to find it. Returns
`{ finding_id, is_real_gap: bool, evidence, corrected_severity }`. This kills "the agent didn't
look" false positives and catches any finding that came from forbidden sources (flag → drop).

### Stage 3 — Synthesize

- Dedup confirmed gaps across briefs (same missing doc found by 3 builders = one issue).
- Group by theme/category; map each to repo labels (`documentation`, `type:documentation`,
  `component:*`, `type:test`, etc.).
- Emit `tools/ai-usability/findings/<date>.md`: per-gap title, body (goal/what's-missing/repro
  brief/suggested fix), proposed labels, and how many briefs hit it (impact signal).

### Review gate (manual)

User reads the findings report. On approval, file one `gh issue create` per confirmed gap with the
proposed title/body/labels. Nothing reaches the public repo without sign-off.

## The Brief Ladder

Five graduated briefs, each a plain-English ask the way a real user would phrase it. Stored as files
so runs are reproducible and the ladder can grow.

1. **Trivial** — one GET route returning JSON produced by a workflow.
2. **Basic** — route with request-body validation (JSON schema) + a `transform` workflow.
3. **Data** — CRUD over Postgres via the `db` plugin, including a migration.
4. **Auth** — protected routes (OIDC/JWT) with a role/permission check.
5. **Hard** — auth + DB + a realtime piece (`ws`/`sse` or `livekit`) combined in one project.

Difficulty is graduated so a stall localizes *where* the AI experience degrades. Briefs target
capabilities Noda demonstrably supports (mirrored by existing examples), so a failure is a discovery
or documentation gap rather than an impossible request.

## File Layout

```
tools/ai-usability/
  README.md              ← setup (stage 0) + how to run + how to read findings
  briefs/
    01-trivial.md
    02-basic.md
    03-data.md
    04-auth.md
    05-hard.md
  harness.workflow.js    ← Workflow script; re-invoke via { scriptPath }
  findings/
    <date>.md            ← generated review artifact (git-ignored or committed per run)
```

The Workflow script reads the briefs, runs stages 1–3, and returns the synthesized findings. It is
re-runnable: same briefs + fixed gaps → fewer findings, which is how we measure progress.

## Honesty & Faithfulness Guards

- Builders never touch the repo; they operate only in their scratch dir.
- Builders are forbidden from reading Noda source or the web; the evaluator flags and drops any
  finding whose information actually came from such sources.
- Every finding carries enough structure (goal · consulted · missing · severity · suggested fix) to
  become a self-contained, actionable issue.
- Two-layer gap confirmation (builder self-report → independent evaluator) before anything is filed.
- Human review gate before any GitHub issue is created.

## Testing the Harness Itself

- Dry-run stage 1 with the **trivial** brief only; confirm a builder can scaffold, validate, and
  return a well-formed structured result.
- Inject a known gap (e.g., point a brief at a deliberately undocumented capability) and confirm the
  evaluator marks it a real gap and it surfaces in the report.
- Confirm dedup collapses the same gap reported by multiple briefs into one issue entry.

## Open Items for the Plan

- Exact `.mcp.json` entry shape and confirmation that subagents can reach `noda_*` post-restart.
- Whether `findings/<date>.md` is committed per run or git-ignored.
- Final wording of the builder/evaluator system prompts (the "only MCP surface" contract is
  load-bearing and worth iterating).

## Success Criteria

- One command/invocation runs the full fleet against the real MCP server and produces a deduped,
  labeled findings report.
- Findings are specific enough to file as issues with no further investigation.
- Re-running after fixes shows a measurable drop in confirmed gaps.
