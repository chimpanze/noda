# Node ConfigSchema Enforcement — Design (#332)

**Date:** 2026-07-16
**Issue:** #332 — node ConfigSchema (incl. required fields) is never enforced at validation or runtime
**Decision:** Full tranche (audit all node schemas, then enforce as validation errors), approved by user 2026-07-16.

## Problem

Every node ships a JSON Schema via `ConfigSchema()` (`pkg/api/node.go:16`), but the only
consumers are display surfaces (editor `internal/server/editor_nodes.go`, MCP
`internal/mcp/tools.go`). `noda validate` passes configs whose per-node `config` payload is
wrong (missing fields, typo'd field names, wrong types) — failures surface only at runtime.

Enforcement can't just be switched on: schemas over-declare `required` relative to executors
(e.g. `response.json` requires `status`/`body` but the executor defaults status to 200).

## Decision

1. **Audit all ~79 node ConfigSchemas** (every `ConfigSchema()` in `plugins/`) against their
   executors' actual behavior, using `docs/03-nodes/<type>.md` (verified against source in the
   2026-07-15 docs campaign) as cross-reference. Corrected schemas also improve the editor
   forms and MCP guidance — that's a feature, not fallout.
2. **Mini-validator, not jsonschema/v6.** Node schemas use a small vocabulary (`type`,
   `enum`, `properties`, `required`, `items`, `oneOf`, `additionalProperties` + annotations).
   A ~150-line walker in `internal/registry` gives us the one semantic a generic library
   can't: **an expression string (contains `{{`) satisfies any type/enum**. A vocabulary-guard
   test rejects schemas using keywords the walker doesn't implement, so the two can't drift.
3. **Strict top-level keys:** unknown keys at the top level of a node's `config` are errors
   unless the schema sets `"additionalProperties": true`. This catches the typo'd-field-name
   class the docs campaign actually found. Nested objects are strict only with an explicit
   `"additionalProperties": false`.
4. **Enforce in `ValidateStartup` + `ValidateStartupDryRun`** (`internal/registry/validator.go`)
   — covers both server boot and `noda validate`. A node with no `config` key validates as `{}`
   (so `required` fires).
5. **Gate:** all `examples/*` and `testdata/{auth,valid-project,node-e2e,livekit-example,minimal-project}`
   projects must pass `ValidateAll` + `Bootstrap(DryRun)`; `testdata/invalid-project` must still fail.
   A permanent test in `cmd/noda` enforces this.

## Ordering (so the branch never has a broken interregnum)

Validator → vocabulary guard → per-package schema audits (schemas fixed while enforcement is
still off) → wire enforcement + fix repo-wide test fallout → examples/testdata gate → docs.

## Behavior changes (documented in CHANGELOG)

- `noda validate` and server boot now reject configs with invalid node `config` payloads
  (missing required fields, wrong types, unknown top-level fields). Previously these surfaced
  as runtime node errors or silent defaults.
- Corrected `required`/type declarations change editor form rendering (required markers) —
  toward truth.

## Not doing

- Warnings-first mode (user chose full tranche).
- Output-schema validation, route params/query schemas (separate surfaces).
