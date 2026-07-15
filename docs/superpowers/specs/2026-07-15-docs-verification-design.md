# Documentation Verification Campaign — Design

**Date:** 2026-07-15
**Status:** Approved
**Goal:** Verify all user-facing documentation against the codebase and file GitHub issues for every discrepancy.

## Scope

**In scope:** `docs/01-getting-started/` (7 files), `docs/02-config/` (11 files), `docs/03-nodes/` (80 node pages + `_index`), `docs/04-guides/` (9 files), `docs/05-examples/` (6 files).

**Not in scope:** `docs/_internal/`, `README.md`, editor UI text, `examples/` project READMEs (the projects are exercised only as targets of the `05-examples` walkthroughs), and fixing anything — this campaign only reports.

**Source of truth:** the code on current `main`. Docs describing intentionally changed behavior (e.g. wildcard-send removal, worker timeout panic shield) are checked against current behavior, not history.

**Finding types:** both inaccuracies (doc contradicts code) and coverage gaps (code supports something the docs never mention — undocumented nodes, fields, flags, functions).

## Deliverable

GitHub issues via `gh issue create`, one per doc area (9 areas, so 9 issues max; a clean area gets no issue). Each issue is a checklist of findings. Each finding records:

- Doc file + section
- What the doc says
- What the code actually does, with `file:line` reference
- Severity: **breaks-user** (a user following the doc hits an error) / **wrong** (factually incorrect but survivable) / **gap** (undocumented capability) / **cosmetic** (typo-level drift)

Findings that span areas (e.g. a systemic stale default) are logged once in the most relevant issue and cross-referenced from the others.

## Method

### Phase 0 — Ground-truth inventory (build once)

Dump the code's self-description into scratchpad files that every later pass diffs against:

- **Node inventory + schemas:** `noda_list_nodes` + `noda_get_node_schema` for every registered node type (MCP tools; the MCP server is the code describing itself).
- **Config schemas:** `noda_get_config_schema` for every config file type.
- **Expression functions:** `noda_list_functions`.
- **CLI surface:** `noda <cmd> --help` for every subcommand and its flags.
- **Snippet extractor:** a script that pulls every fenced ```json block out of a doc, classifies it (full config file vs fragment), runs full configs through `noda validate` / `noda_validate_config`, and expressions through `noda_validate_expression`.

### Phase 1 — Static per-area passes (7 areas)

Each pass: read the docs, diff every claim against the inventory (and against source directly where the inventory doesn't cover the claim, e.g. behavioral statements), validate all snippets, log findings to a per-area scratch file.

| # | Area | Files |
|---|------|-------|
| 1 | `01-getting-started` | 7 |
| 2 | `02-config` | 11 |
| 3 | `03-nodes` core plugins (control, transform, response, util, workflow, event, upload, ws, sse, wasm) | 25 |
| 4 | `03-nodes` data plugins (db, cache, storage, image, http, email) | 26 |
| 5 | `03-nodes` auth & oidc | 11 |
| 6 | `03-nodes` LiveKit `lk.*` | 18 |
| 7 | `04-guides` | 9 |

### Phase 2 — Execution passes (2 areas)

| # | Area | Method |
|---|------|--------|
| 8 | Executable walkthroughs | `docker compose up` at repo root; follow quick-start, realtime, and testing-and-debugging flows literally as written; `noda init` → validate → test. |
| 9 | `05-examples` | Run each of the 6 walkthrough docs against its `examples/` project (compose files exist for most). LiveKit-dependent flows (video-conferencing) execute only if the compose stack provides a LiveKit server; otherwise the issue marks them statically-verified-only. |

## Honesty rules

- A snippet that fails validation is re-checked by hand before being logged — fragments may legitimately fail out of context.
- Anything that couldn't be executed is listed explicitly in the issue as unverified, never silently skipped.
- Every logged finding cites the code location that contradicts (or is absent from) the doc.

## Success criteria

- All 114 in-scope doc files read and diffed against ground truth (including the `03-nodes/_index`).
- All extractable JSON snippets and expressions validated.
- All executable walkthroughs run (or explicitly listed as unverified with the reason).
- One issue per non-clean area filed with the checklist format above.
