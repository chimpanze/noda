# Follow-up Closeout — Design (#339–#347)

**Date:** 2026-07-17
**Issues:** the nine follow-ups from the #331/#332 tranches (PRs #337/#338). User approved clearing all nine in one tranche; #340 decided as "accept numeric strings".

## Decisions

- **#340 (design call, user-approved):** int-typed config resolvers become lenient — `ResolveOptionalInt` and `ResolveRawInt` fall back to `ToInt` when an expression/string resolves to a numeric string; `ToInt64` gains the same numeric-string case (it feeds `upload.handle.max_size`). Non-numeric strings still error with the existing message. This structurally kills the `{{ query.limit ?? '20' }}` → typed-int-field trap. The `toInt(...)` wraps shipped in #337 stay (double-safe).
- **#339:** empirically test uppercase `MULTIPART/FORM-DATA`; if fasthttp's `MultipartForm()` fails on it, parse manually via `mime.ParseMediaType` + `multipart.NewReader` (mirrors the urlencoded fix); add the partial-parse (`a=1&b=%zz`) regression test.
- **#347:** make the debounce test robust under CI load: 500 ms debounce vs a 25 ms write window + `require.Eventually`, then settle-check exactly 1.
- **#346:** `checkVocab` validates type-name membership; vocab test with a bad keyword inside a oneOf branch; delete the dead `staticFieldsByNodeType["transform.merge"]` entry ("type" is nested `match.type`, never matched, and strict root keys reject a top-level `type` anyway).
- **#345:** editor `POST /validate` + `/validate/all` and MCP `noda_validate_config` append `registry.ValidateStartupDryRun` errors to the pipeline. EditorAPI already holds `plugins`/`nodes`/`compiler`; MCP builds a throwaway registry from its own `corePlugins()`. Harness re-run stays post-merge (out of tranche).
- **#341:** `trigger.coerce` checkbox next to Raw Body in RouteFormPanel; RoutesView clean logic keeps the key only when explicitly `false` (default is true).
- **#344:** NodeConfigPanel uiSchema builder unions `oneOf[*].properties` when root `properties` is absent; the three auth root-oneOf schemas gain branch `title` annotations (Go side, annotation keyword — vocabulary-legal).
- **#342/#343:** rest-api.md rows bullet corrected to `db.find`; docs/03-nodes Required/type columns swept against `tools/docverify/groundtruth` dump output; the 4 illustrative `db.insert` snippets replaced with `db.create`. Snippet-validator node-type extension only if trivial; otherwise note-and-skip.

## Shape

One branch `feat/followup-closeout`, worktree `.worktrees/followup-closeout`, 7 tasks, one PR closing all nine issues.

## Not doing

- AI-usability harness re-run (post-merge, after #345 ships).
- Float-typed resolver leniency (only int-typed resolvers are in #340's scope).
- Editor E2E additions beyond existing CI jobs.
