# Design: Real-endpoint e2e verifier for the AI-usability harness

**Date:** 2026-06-27
**Status:** Approved (brainstorming) — ready for implementation plan
**Related:** `tools/ai-usability/findings/2026-06-25-runtime-gap.md`, `2026-06-25-ai-usability-harness-design.md`

## Problem

The AI-usability harness re-run scored 0 confirmed usability findings, but actually running the AI-built scratch projects exposed that "0 friction" oversells "it works." Documented in `tools/ai-usability/findings/2026-06-25-runtime-gap.md`:

- **Finding C:** the harness build step runs only `noda_validate_config` (static), never `noda test`, so runtime breakage produces no signal.
- **Finding D:** even `noda test` cannot exercise the transport layer. It calls `engine.ExecuteGraph` directly — no server, synthetic JSON input, pre-parsed auth, and every plugin/service node (`db`, `http`, `storage`, `upload.handle`, `ws.send`, `sse.send`) mocked or unmocked. So HTTP routing, middleware (JWT + casbin), multipart **file upload**, **WebSocket**, and **SSE** endpoints are entirely unverified for AI-built projects. There is no config-project-level way, via the CLI or MCP surface, to prove these endpoints work.

This design adds that missing rung: a harness-internal verifier that boots each AI-built project as a real server and drives its real HTTP/upload/WS/SSE endpoints.

## Scope

**In scope:** a new `E2E` phase inside `tools/ai-usability/harness.workflow.js` that, for each built scratch project, boots `noda start` against real services and exercises its real endpoints, then triages and reports failures.

**Out of scope:** a general-purpose `noda` subcommand or a standalone reusable e2e CLI (considered and rejected in favor of the narrower harness-internal shape); changes to `noda test`; fixing the doc gaps from Findings A/B (those are separate doc-fix issues).

## Key constraint

Workflow scripts (`*.workflow.js`) have **no shell or filesystem access** — only `agent()`, `parallel()`, `pipeline()`, `log()`, `phase()`. Therefore all Docker, server-boot, migration, and curl/WS work is performed **by agents** (which have Bash), and the script only orchestrates and routes their structured results.

## Architecture

A new `E2E` phase runs after the existing `Build` phase (which already leaves projects on disk at `scratchRoot/<brief>`). Components, each a focused agent role with a clean structured interface:

1. **Infra-setup agent** (once per run): preflight Docker + `./bin/noda`. If Docker is unavailable, return `{ skipped: true, reason }` and the phase no-ops with a loud `log()`. Otherwise bring up **one ephemeral Postgres + one Redis**, return connection params `{ pg, redis }`.
2. **Per-project e2e agent** (one per brief, run **sequentially** for determinism and to avoid port/resource contention): given `projectDir`, the brief, and conn params, it:
   - creates a fresh database for the project on the shared Postgres;
   - applies env/config overrides pointing the project's service slots at the shared containers;
   - runs `noda migrate`;
   - starts `noda start` on a unique port in the background and polls health with a bounded timeout;
   - derives and executes **real requests** — `curl` for HTTP, multipart upload, and SSE (`curl -N`); a WS client (websocat or a provisioned inline script) for WebSocket — asserting responses;
   - stops its server.
3. **Triage** (produced in the e2e agent's output): each failure classified `runtime-bug` / `usability` / `build-error`.
4. **Teardown agent**: unconditionally stops/removes the shared containers at the end.

## Data flow

```
Build phase (existing) ──> scratch projects on disk + builder results
E2E phase (new):
  ① infra-setup agent ──> { pg, redis }  | { skipped, reason } ──> log & no-op
  ② per brief, sequential:
       e2e agent(projectDir, brief, conn) ──> {
         brief_id, boot_ok,
         endpoints: [ { transport: http|upload|ws|sse, request, expected, actual, pass } ],
         findings:  [ { category: runtime-bug|usability|build-error, transport, evidence, suggested_fix } ]
       }
  ③ teardown agent ──> stop containers
```

Findings split by category and rejoin the existing skeptical machinery — nothing is filed without adversarial refutation:

- **`usability`** (build was wrong AND an MCP-surface gap plausibly caused it) → routed into the **existing Verify stage**, asking the same question it already asks ("is the info the AI needed reachable via the MCP surface?"). Survivors flow into **Synthesize → issues**.
- **`runtime-bug`** (noda misbehaves despite correct config) → a **dedicated mini-verify**: a skeptic agent tries to refute the bug by reproducing it against the live server, defaulting to "not a bug" when uncertain. Survivors become `confirmed_bugs`.
- **`build-error`** (AI botched it, MCP surface adequate) → recorded in the report only, never filed.

The e2e agent both authors and judges its assertions, so a wrong expectation could self-confirm. The adversarial-verify stages are the guard: they independently re-derive from the MCP surface (`usability`) or re-run against the live server (`runtime-bug`) rather than trusting the e2e agent's expectation.

## Workflow return value

Extends today's `{ confirmed_findings, issues }` with:

- `e2e_results` — per-project boot status + per-endpoint pass/fail table (raw signal; includes `status: "skipped"` when Docker is absent).
- `confirmed_bugs` — adversarially-survived runtime bugs.
- `usability` e2e findings are folded into the existing `issues`.

## Report

Written to `tools/ai-usability/findings/<date>-e2e.md`: per-project boot result, a transport×endpoint pass/fail table, and for each failure the request / expected / actual / triage verdict.

## Error handling

Principle: **a failure to check must never read as a pass.**

- **Docker absent / containers won't start** → `{ skipped, reason }`, loud log, `e2e_results` per project `status: "skipped"`; harness still returns usability findings. No green-by-omission.
- **Server won't boot** (migrate fails, bad config, panic) → `boot_ok: false` with stderr/log tail; itself a finding, triaged `runtime-bug` vs `build-error`.
- **Port contention** → unique port per project; sequential execution removes the race.
- **WS/SSE client missing** → agent provisions one (websocat or inline script); if genuinely impossible, the transport is recorded `not_exercised` — explicit, never a silent skip.
- **Readiness flakiness** → bounded health-poll before any request; timeout → `boot_ok: false`, not a hang.
- **Cleanup** → teardown runs unconditionally (even if a per-project agent errored); containers never leak.

## Testing the verifier

The verifier is test infrastructure and needs its own evidence it works. Acceptance checks:

1. **Known-good positive control** — run against a curated, passing example. Use `examples/init-example` for the boot + HTTP path, and a richer example covering the other transports (e.g. `examples/realtime-collab` for WebSocket; an upload-bearing example for multipart) so the positive control isn't HTTP-only. Expect boot ok, all endpoints green, zero findings. Proves no false positives.
2. **Injected-fault negative control** — break one example endpoint (e.g. wrong response field); confirm it goes red and triages correctly. Proves detection + classification.
3. **No-Docker path** — run with Docker unavailable; confirm clean skip + loud log, harness still completes.
4. **Self-contained project** — a project needing no external services boots and passes, proving the service layer is optional when unused.

## Decisions (from brainstorming)

- **Shape:** harness-internal verifier (not a standalone CLI or `noda` subcommand).
- **Request/assertion source:** agent-driven per project (not mechanical smoke-test or builder-emitted plan).
- **Service provisioning:** shared ephemeral Postgres + Redis, per-project database, run migrations (not per-project docker-compose, not HTTP-only-no-Docker).
- **Result classification:** triage into runtime-bug / usability / build-error (not a separate report, not a pass/fail gate).
- **Execution order:** sequential per project.
