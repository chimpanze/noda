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
