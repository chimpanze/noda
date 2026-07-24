# Unwired Outcome Outputs Fail Loudly (#442)

**Date:** 2026-07-24
**Issue:** #442 — Unwired node outputs silently end the path; workflow reports success and HTTP returns 202
**Decision maker:** Marten (severity question answered: hard error)

## Problem

When a node fires an output with no outbound edge, that execution path silently ends:
`ExecuteGraph` returns nil, the workflow is recorded as `success`, and an HTTP route
answers `202 Accepted`. The engine already treats an unwired `error` output as a bug
(`internal/engine/executor.go` fails the workflow loudly), but every other output is
silent — even outputs like `exists` / `invalid` / `not_found` that mean "the operation
did not do what was asked."

PR #441 (issue #436) made this acute: unique violations on `db.create`/`db.update`/
`db.upsert` moved from the raised-error path (loud 409 via the unwired-`error` rule) to
the new `exists` output (silent 202) — a regression at every example site, since none
wired `exists`. Pre-existing worse case: `auth-demo`'s reset-password leaves
`set_password.invalid` unwired, so an invalid/expired reset token gets `202 Accepted`.

## Output classification (from #442, adopted unchanged)

- **Outcome outputs — must be handled (8 node types):**
  `db.create`, `db.update`, `db.upsert`, `auth.create_user` → `exists`;
  `auth.get_user` → `not_found`;
  `auth.set_password`, `auth.verify_credentials`, `auth.consume_token` → `invalid`.
- **Control-flow outputs — unwired is often deliberate, exempt:**
  `control.if` → `then`/`else`; `control.switch` → `default`; `control.loop` → `done`.
  (And `success` itself: a terminal node with unwired `success` is a normal end.)

## Design

### 1. Classification contract — `pkg/api`

New optional interface, following the existing `NodeOutputSchemaProvider` pattern:

```go
// OutcomeOutputsProvider is optionally implemented by NodeDescriptor to declare
// outputs that report an operation outcome the workflow must handle. Validation
// rejects a workflow that leaves a declared outcome output unwired.
type OutcomeOutputsProvider interface {
    OutcomeOutputs() []string
}
```

- Non-breaking for plugin authors: nothing is required to implement it.
- The 8 node descriptors above implement it with static lists (none of these
  outputs is config-dependent).
- Every declared outcome output must be a subset of the node's `Outputs()`;
  a unit test enforces this for all registered nodes (audit-test pattern, as
  with `ConfigSchema`).
- Control-flow nodes do NOT implement it.

### 2. Boot/validate rule — `internal/registry/validator.go` (HARD ERROR)

In the same per-workflow walk that already rejects undeclared edge outputs (#379):
for every node instance whose descriptor declares outcome outputs, each declared
outcome output must have at least one outbound edge. Violation is a validation
**error** (not a warning), e.g.:

```
workflow "create-user": node "insert" (db.create): outcome output "exists" has no
outbound edge — a fired outcome output with no edge silently ends the path. Wire it
(e.g. to an error response, or to the same node as "success" if the distinction
does not matter here).
```

Because this validator runs in both `noda validate` and boot (#383 parity), and
`TestShippedProjectsValidate` runs it over every shipped example, one change enforces
the rule everywhere: CLI validate, dry-run, boot, dev-mode reload, editor, examples CI.

**This is the BREAKING change.** Any existing config that leaves an outcome output
unwired stops validating/booting. Migration is one edge per site. CHANGELOG gets a
BREAKING entry with the full affected-node/output table and the migration line.

### 3. `writeAccepted` contract guard — `internal/server/routes.go`

When a workflow completes successfully having produced **no** response, and the route
**declares response schemas** (`response` block with at least one status schema and
validation not disabled — i.e. the route's `respValidator` is non-nil), the
202 fallback contradicts the route's own declared contract. Instead of `202 Accepted`,
return `500 INTERNAL` with message "route declares responses but the workflow produced
none" (trace ID included, existing error envelope).

- Routes without a `response` block (or with `validate: disabled`) keep today's
  fire-and-forget 202 — async patterns unaffected.
- Applies at both `writeAccepted` call sites in `awaitWorkflowResponse`
  (workflow-done and at-deadline-completed paths).
- This is defense-in-depth: with §2 in place the known causes are already rejected
  at boot, but a future silent-path bug should surface as a 500, not a fake 202.

### 4. Generated and shipped config sweep

**`internal/generate/crud.go`:** generated create and update workflows wire `exists`
to a `response.error` with status 409 / code `CONFLICT` — restoring the REST semantic
that pre-#436 behavior gave for free. (The generator emits `db.create`, `db.update`,
`db.delete` only — no upsert path to touch; `db.delete` has no outcome output.)

**Examples + testdata — exact enumeration (20 sites, verified 2026-07-24):**

| File | Node | Output |
|---|---|---|
| examples/auth-demo/workflows/auth.reset-password.json | set_password | invalid |
| examples/node-cookbook/db/workflows/create.json | insert | exists |
| examples/node-cookbook/db/workflows/update.json | change | exists |
| examples/node-cookbook/db/workflows/upsert.json | save | exists |
| examples/realtime-collab/workflows/create-document.json | insert | exists |
| examples/realtime-collab/workflows/update-document.json | update | exists |
| examples/realtime-collab/workflows/ws-on-message.json | save_edit | exists |
| examples/realworld/workflows/update-user.json | set_password | invalid |
| examples/rest-api/workflows/create-task.json | insert | exists |
| examples/saas-backend/workflows/create-project.json | insert | exists |
| examples/saas-backend/workflows/create-task.json | insert | exists |
| examples/saas-backend/workflows/create-workspace.json | insert_member | exists |
| examples/saas-backend/workflows/create-workspace.json | insert_workspace | exists |
| examples/saas-backend/workflows/generate-thumbnail.json | update_record | exists |
| examples/saas-backend/workflows/handle-stripe-webhook.json | update_subscription | exists |
| examples/saas-backend/workflows/invite-member.json | insert | exists |
| examples/saas-backend/workflows/sync-github-issue.json | close_task | exists |
| examples/saas-backend/workflows/sync-github-issue.json | create_task | exists |
| examples/saas-backend/workflows/upload-attachment.json | insert | exists |
| testdata/valid-project/workflows/create-task.json | insert | exists |

**Per-trigger wiring rule** (plan decides the concrete target per site):

- HTTP-triggered workflow → `response.error` (409 `CONFLICT` for `exists`,
  400/422 for `invalid` per the flow's semantics; auth flows must preserve the
  anti-enumeration padding established in PR #289 — reset-password's `invalid`
  response must not create a token-validity oracle beyond what the flow already
  reveals).
- Non-HTTP trigger (WS message, worker, scheduler) → explicit handling appropriate
  to the trigger: `ws.send` error notice, or at minimum `util.log` at warn level —
  the point is the handling is deliberate and visible, not a dead end.

**Already compliant, verified:** `cmd/noda/auth_templates/` (all 8 flow templates
wire their outcome outputs), `testdata/auth` (pinned to templates by the drift
guard), `internal/scaffold` / `noda init` (emits no db/auth workflow nodes — #431's
green-out-of-the-box guarantee holds).

### 5. Docs, CHANGELOG, tests

- **Docs:** the 8 node pages in `docs/03-nodes/` state the must-wire rule for their
  outcome output; the workflows/edges page in `docs/02-config/` documents the
  outcome-vs-control-flow distinction and the validation error; plugin-dev guide
  (`docs/04-guides/`) documents `OutcomeOutputsProvider` for plugin authors.
- **CHANGELOG:** BREAKING entry (validation change + migration), plus the
  writeAccepted 500-on-declared-responses change, plus the example/generator fixes.
- **Tests:**
  - Validator unit tests: unwired outcome output rejected; wired accepted;
    control-flow outputs exempt; error message names node/output; audit test that
    every registered node's `OutcomeOutputs() ⊆ Outputs()`.
  - Server unit tests: no-response + declared responses → 500; no-response + no
    `responses` block → 202 unchanged.
  - Generator tests: generated CRUD workflows pass the new validation (existing
    generator round-trip tests should catch this automatically; extend if not).
  - Integration (`-tags integration`): node-cookbook db `verify.json` gains a
    duplicate-insert round-trip asserting the 409 — proving the wiring end to end.
  - Realworld harness: re-run; the update-user `invalid` wiring may flip
    known-failing entries — update `harness/known-failing.json` accordingly.

## Out of scope

- Runtime generalization of the unwired-`error` rule (#442 option 1) — redundant
  once boot rejects the shape; the engine's existing `error`-output rule stays as is.
- Editor UI affordances (highlighting must-wire ports) — possible follow-up.
- Warning-mode or config knob for the validation severity — decided against
  (hard error, no escape hatch other than wiring the output).

## Risks

- **Downstream user configs break at upgrade.** Intended and announced; the error
  message is self-explanatory and the fix is one edge.
- **Behavioral change in examples:** duplicate inserts return 409 (not 202); invalid
  reset tokens return an error (not 202). These are corrections; integration
  expectations (`verify.json`, realworld harness) updated in the same PR.
- **Anti-enumeration:** the auth-demo reset-password `invalid` wiring must match the
  scaffolded template's response shape so no new oracle is introduced (compare with
  `cmd/noda/auth_templates/workflows/auth.reset-password.json.tmpl`).
