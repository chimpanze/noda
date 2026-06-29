# E2E Endpoint Verifier Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a harness-internal `E2E` phase that boots each AI-built Noda project as a real server against ephemeral services and drives its real HTTP/upload/WebSocket/SSE endpoints, triaging failures into runtime-bug / usability / build-error.

**Architecture:** All new logic lives in the single self-contained Workflow script `tools/ai-usability/harness.workflow.js`. Because workflow scripts have no shell/filesystem access, every Docker/boot/curl/WS action is performed by **agents** (which have Bash); the script only orchestrates them and routes their structured results. A new `E2E` phase runs after `Build`: an infra-setup agent brings up one ephemeral Postgres + one Redis, per-project agents boot-and-drive each project sequentially, a teardown agent cleans up. E2E findings rejoin the existing skeptical machinery — `usability` findings flow into the existing `Verify` stage, `runtime-bug` candidates get a dedicated reproduce-or-refute mini-verify, `build-error` are reported only.

**Tech Stack:** Workflow tool (JS orchestration — no filesystem/`Date.now()`/`Math.random()`), the `./bin/noda` binary (`go build -o ./bin/noda ./cmd/noda`), Docker (ephemeral `postgres:16-alpine` + `redis:7-alpine`), `curl` (HTTP/upload/SSE), `websocat` or an inline client (WebSocket), `psql`/`redis-cli` via `docker exec`.

## Global Constraints

- Module path is `github.com/chimpanze/noda`; build target `./bin/noda` via `go build -o ./bin/noda ./cmd/noda`.
- Workflow scripts have **no filesystem, `Date.now()`, or `Math.random()`** — all shell work is done inside agent prompts; vary per-item work by index, not randomness.
- A failure to *check* must never read as a *pass*: Docker absent → `status:"skipped"` with a loud `log()`; a transport with no usable client → `not_exercised`, never silent.
- Service connection is via env interpolation in `noda.json`: `{{ $env('DATABASE_URL') }}`, `{{ $env('REDIS_URL') }}`. The e2e agent sets these env vars; it must read each project's `services.*.config` to use the env-var names that project actually references.
- Server port comes from `noda.json` `server.port` (default `3000`); there is no `--port` flag. Execution is **sequential**, so port reuse is fine — but each boot must first free the port (`lsof -ti tcp:<port> | xargs -r kill -9`).
- Ephemeral containers use **fixed names** `noda-e2e-pg` / `noda-e2e-redis` and **published random host ports** (`-p 0:5432`, `-p 0:6379`) so they never collide with a developer's local Postgres/Redis.
- Containers are torn down **unconditionally and last**, after the runtime-bug mini-verify (which needs them live).
- `tools/ai-usability/` is a normal committed repo path. Generated `findings/<date>-e2e.md` is committed per run.

---

### Task 1: E2E schemas, infra-setup + teardown agents, and `e2eOnly` entry mode

Adds the data shapes, the bring-up/tear-down agents, and a way to run the E2E phase in isolation against explicit project dirs (the test vehicle for later tasks and a useful standalone mode).

**Files:**
- Modify: `tools/ai-usability/harness.workflow.js` (append schemas + prompt builders near the existing schema block ~line 19–84; add the `e2eOnly` branch in the body ~line 127–171)

**Interfaces:**
- Produces:
  - `INFRA_SCHEMA` → object `{ skipped: boolean, reason: string, pg_base_url: string, redis_base_url: string }`
  - `infraSetupPrompt(): string`, `teardownPrompt(): string`
  - `runE2E(projects, scratchRoot): Promise<{ e2e_results, e2e_findings }>` (full body lands across Tasks 1–4; Task 1 lands the infra+teardown skeleton returning empty results)
  - `opts.e2eOnly` (boolean) + `opts.projectDirs` (array of absolute paths) entry mode
  - `projects` item shape consumed by `runE2E`: `{ id: string, dir: string, briefPath: string }`

- [ ] **Step 1: Add the E2E schemas after the existing `ISSUES_SCHEMA` block (~line 84)**

```js
const INFRA_SCHEMA = {
  type: 'object',
  additionalProperties: false,
  required: ['skipped', 'reason', 'pg_base_url', 'redis_base_url'],
  properties: {
    skipped: { type: 'boolean' },
    reason: { type: 'string' },
    pg_base_url: { type: 'string' },      // e.g. postgres://noda:noda@localhost:54321  (no trailing db)
    redis_base_url: { type: 'string' },   // e.g. redis://localhost:54322
  },
}

const E2E_ENDPOINT = {
  type: 'object',
  additionalProperties: false,
  required: ['transport', 'request', 'expected', 'actual', 'pass'],
  properties: {
    transport: { type: 'string', enum: ['http', 'upload', 'ws', 'sse'] },
    request: { type: 'string' },
    expected: { type: 'string' },
    actual: { type: 'string' },
    pass: { type: 'boolean' },
  },
}

const E2E_FINDING = {
  type: 'object',
  additionalProperties: false,
  required: ['category', 'transport', 'evidence', 'suggested_fix'],
  properties: {
    category: { type: 'string', enum: ['runtime-bug', 'usability', 'build-error'] },
    transport: { type: 'string', enum: ['http', 'upload', 'ws', 'sse', 'boot'] },
    evidence: { type: 'string' },
    suggested_fix: { type: 'string' },
  },
}

const E2E_RESULT_SCHEMA = {
  type: 'object',
  additionalProperties: false,
  required: ['brief_id', 'boot_ok', 'boot_log_tail', 'endpoints', 'findings'],
  properties: {
    brief_id: { type: 'string' },
    boot_ok: { type: 'boolean' },
    boot_log_tail: { type: 'string' },
    endpoints: { type: 'array', items: E2E_ENDPOINT },
    findings: { type: 'array', items: E2E_FINDING },
  },
}

const BUG_VERDICT_SCHEMA = {
  type: 'object',
  additionalProperties: false,
  required: ['is_real_bug', 'evidence'],
  properties: {
    is_real_bug: { type: 'boolean' },
    evidence: { type: 'string' },
  },
}
```

- [ ] **Step 2: Add the infra-setup and teardown prompt builders (after the schemas)**

```js
function infraSetupPrompt() {
  return [
    'You are setting up ephemeral test infrastructure for end-to-end verification of Noda projects. You have Bash.',
    '',
    'STEPS:',
    '1. Preflight: run `docker version` and `test -x ./bin/noda`. If Docker is unavailable/not running, return',
    '   { skipped: true, reason: "<short reason>", pg_base_url: "", redis_base_url: "" } and STOP.',
    '2. Remove any stale containers from a prior run: `docker rm -f noda-e2e-pg noda-e2e-redis 2>/dev/null || true`.',
    '3. Start Postgres on a RANDOM host port (so it never collides with a local DB):',
    '   docker run -d --name noda-e2e-pg -e POSTGRES_USER=noda -e POSTGRES_PASSWORD=noda -e POSTGRES_DB=noda -p 0:5432 postgres:16-alpine',
    '   Find the published port: `docker port noda-e2e-pg 5432` → the part after the colon is PG_PORT.',
    '4. Start Redis on a RANDOM host port:',
    '   docker run -d --name noda-e2e-redis -p 0:6379 redis:7-alpine',
    '   Find the port: `docker port noda-e2e-redis 6379` → REDIS_PORT.',
    '5. Wait until ready (poll up to 30s): `docker exec noda-e2e-pg pg_isready -U noda` and',
    '   `docker exec noda-e2e-redis redis-cli ping` (expect PONG).',
    '6. Return { skipped: false, reason: "", pg_base_url: "postgres://noda:noda@localhost:PG_PORT",',
    '   redis_base_url: "redis://localhost:REDIS_PORT" } with the real ports substituted.',
    '',
    'Return ONLY the structured object.',
  ].join('\n')
}

function teardownPrompt() {
  return [
    'You have Bash. Tear down the ephemeral e2e infrastructure, ignoring errors if already gone:',
    '  docker rm -f noda-e2e-pg noda-e2e-redis 2>/dev/null || true',
    'Then confirm they are gone with `docker ps --filter name=noda-e2e -q` (expect empty output).',
    'Return the text "torn-down".',
  ].join('\n')
}
```

- [ ] **Step 3: Add the `runE2E` skeleton (after the prompt builders) — infra + teardown only for now**

```js
// runE2E boots ephemeral services, drives each project's real endpoints, and tears down.
// projects: [{ id, dir, briefPath }]. Returns { e2e_results, e2e_findings }.
async function runE2E(projects, scratchRoot) {
  phase('E2E')
  const infra = await agent(infraSetupPrompt(), { label: 'e2e:infra', phase: 'E2E', schema: INFRA_SCHEMA })
  if (!infra || infra.skipped) {
    log(`E2E phase skipped — ${infra ? infra.reason : 'infra agent returned no result'}. Endpoints NOT verified.`)
    return {
      e2e_results: projects.map((p) => ({ brief_id: p.id, boot_ok: false, boot_log_tail: '', endpoints: [], findings: [], status: 'skipped' })),
      e2e_findings: [],
    }
  }

  // Per-project drive lands in Task 2/3; placeholder keeps the skeleton runnable.
  const e2e_results = []

  // Runtime-bug mini-verify lands in Task 4 (must run while infra is up).

  await agent(teardownPrompt(), { label: 'e2e:teardown', phase: 'E2E' })
  const e2e_findings = e2e_results.flatMap((r) => (r.findings || []).map((f) => ({ ...f, brief_id: r.brief_id })))
  return { e2e_results, e2e_findings }
}
```

- [ ] **Step 4: Add the `e2eOnly` entry branch in the body (right after `opts` is normalized, ~line 134)**

```js
const scratchRoot = opts.scratchRoot || '/tmp/noda-ai-usability'
const only = opts.only

// Standalone E2E mode: run just the E2E phase against explicit project dirs.
// Used for the positive/negative controls and ad-hoc endpoint checks.
if (opts.e2eOnly) {
  const dirs = opts.projectDirs || []
  const projects = dirs.map((dir) => {
    const id = dir.split('/').filter(Boolean).pop()
    return { id, dir, briefPath: opts.briefPathFor ? opts.briefPathFor[id] : '' }
  })
  const { e2e_results, e2e_findings } = await runE2E(projects, scratchRoot)
  return { e2e_results, e2e_findings, confirmed_bugs: [], confirmed_findings: [], issues: [] }
}

const briefs = only ? BRIEFS.filter((b) => only.includes(b.id)) : BRIEFS
```

(Delete the now-duplicated `const scratchRoot`/`const only`/`const briefs` lines that previously sat here.)

- [ ] **Step 5: Parse-check the script**

Run: `node --check tools/ai-usability/harness.workflow.js 2>/dev/null && echo "parse-ok" || echo "note: ESM export — real check is the dry-run below"`
Expected: `parse-ok` (or the note; `export`/workflow globals aren't valid standalone Node, so the authoritative check is Step 6).

- [ ] **Step 6: Dry-run the infra skeleton against an empty project list**

Run (from the main session, Docker running):
```
Workflow({ scriptPath: 'tools/ai-usability/harness.workflow.js', args: { e2eOnly: true, projectDirs: [] } })
```
Expected: returns `{ e2e_results: [], e2e_findings: [], ... }`; `docker ps --filter name=noda-e2e -q` is empty afterward (containers came up and were torn down). If Docker is stopped, expected: `e2e_results: []`, and a logged "E2E phase skipped" line.

- [ ] **Step 7: Commit**

```bash
git add tools/ai-usability/harness.workflow.js
git commit -m "feat(e2e): infra setup/teardown agents + e2eOnly harness entry"
```

---

### Task 2: Per-project e2e agent — boot + HTTP drive + triage (positive & negative control)

The core of the feature: boot one project against the shared services and drive its HTTP endpoints, asserting status and body, then triage failures. Proven against a known-good example (no false positives) and an injected-fault copy (real detection).

**Files:**
- Modify: `tools/ai-usability/harness.workflow.js` (add `e2ePrompt`; fill the per-project drive in `runE2E`)

**Interfaces:**
- Consumes: `INFRA_SCHEMA` result (`pg_base_url`, `redis_base_url`), `E2E_RESULT_SCHEMA`, `projects` items `{ id, dir, briefPath }`
- Produces: `e2ePrompt(project, conn): string`; `runE2E` now returns populated `e2e_results`

- [ ] **Step 1: Add the `e2ePrompt` builder (after `teardownPrompt`)**

```js
function e2ePrompt(p, conn) {
  return [
    'You are an end-to-end endpoint verifier for a Noda project. You have Bash.',
    'Derive EXPECTED behavior from the brief and the project\'s own routes/config — do NOT read Noda\'s Go source to decide what "correct" means.',
    '',
    `PROJECT DIR (absolute): ${p.dir}`,
    p.briefPath ? `BRIEF (read it first): ${p.briefPath}` : 'BRIEF: none provided — infer intent from the project\'s routes/workflows.',
    `SHARED POSTGRES BASE URL: ${conn.pg_base_url}   (no database name appended)`,
    `SHARED REDIS BASE URL: ${conn.redis_base_url}`,
    '',
    'SETUP:',
    `1. Inspect ${p.dir}/noda.json, routes/, workflows/, connections/ to learn endpoints, methods, bodies, auth, and which services it uses. Note the env-var names referenced as {{ $env('NAME') }} under services.*.config.`,
    `2. If it uses a db service: create a fresh database — psql "${conn.pg_base_url}/postgres" -c 'CREATE DATABASE e2e_${sanitize(p.id)};' — and export the project's DB env var to "${conn.pg_base_url}/e2e_${sanitize(p.id)}". If it uses redis/cache/ws connections, export the project's redis env var to "${conn.redis_base_url}".`,
    `3. If ${p.dir}/migrations exists: run \`<envs> ./bin/noda migrate up --config ${p.dir}\`. On failure: set boot_ok=false, put the error in boot_log_tail, add a finding {category, transport:"boot"}, and SKIP driving.`,
    '4. Read server.port from noda.json (default 3000 = PORT). Free it: `lsof -ti tcp:PORT | xargs -r kill -9`.',
    `5. Boot in background: \`<envs> ./bin/noda start --config ${p.dir} > /tmp/e2e-${p.id}.log 2>&1 &\` and capture the PID.`,
    '6. Health-poll up to 20s (`curl -sf http://localhost:PORT/<any defined route>`). If it never comes up: boot_ok=false, boot_log_tail=`tail -n 40 /tmp/e2e-PORT.log`, kill PID, add a finding {transport:"boot"}, STOP.',
    '',
    'DRIVE (only if boot_ok) — for each HTTP route the brief implies, send a REAL request and assert:',
    '  curl -sS -o /tmp/body -w "%{http_code}" -X <METHOD> http://localhost:PORT<path> -H "Content-Type: application/json" -d \'<json body>\'',
    '  Assert the status code AND the JSON fields the brief specifies (cat /tmp/body). Record each as an endpoint entry {transport:"http", request, expected, actual, pass}.',
    'For auth-gated routes: send once WITHOUT a token (expect 401/403) and once WITH a valid token minted per the project\'s jwt config; assert the gate behaves. Record both.',
    '',
    'CLEANUP: kill the server PID (the shared DB/containers are dropped later by teardown).',
    '',
    'TRIAGE every failing endpoint into exactly one finding category:',
    '  - "runtime-bug": config is correct for the brief, but noda returned wrong/erroring behavior.',
    '  - "usability": built wrong in a way an unclear/missing MCP doc or schema plausibly caused (name the tool/doc in evidence).',
    '  - "build-error": built wrong and the MCP surface was adequate.',
    '',
    `Return ONLY the structured object: { brief_id: ${JSON.stringify(p.id)}, boot_ok, boot_log_tail, endpoints, findings }.`,
  ].join('\n')
}

// sanitize a brief id into a safe postgres database name fragment.
function sanitize(id) {
  return id.replace(/[^a-z0-9]/gi, '_').toLowerCase()
}
```

- [ ] **Step 2: Fill the per-project drive in `runE2E` (replace the `const e2e_results = []` placeholder from Task 1 Step 3)**

```js
  // Boot-and-drive each project SEQUENTIALLY (port reuse + resource safety).
  const e2e_results = []
  for (const p of projects) {
    const r = await agent(e2ePrompt(p, infra), { label: `e2e:${p.id}`, phase: 'E2E', schema: E2E_RESULT_SCHEMA })
    if (r) {
      e2e_results.push(r)
    } else {
      log(`e2e:${p.id} returned no result — endpoints not exercised`)
      e2e_results.push({ brief_id: p.id, boot_ok: false, boot_log_tail: '', endpoints: [], findings: [] })
    }
  }
```

- [ ] **Step 3: Positive control — run against the known-good `examples/init-example`**

Run (main session, Docker up; use an absolute path):
```
Workflow({ scriptPath: 'tools/ai-usability/harness.workflow.js',
  args: { e2eOnly: true, projectDirs: ['/Users/marten/GolandProjects/noda/examples/init-example'] } })
```
Expected: `e2e_results[0].boot_ok === true`; every `endpoints[*].pass === true`; `findings` empty. Proves no false positives.

- [ ] **Step 4: Negative control — inject a fault and confirm detection + triage**

Run:
```bash
cp -r examples/init-example /tmp/e2e-neg && \
  sed -i '' 's/Hello, World!/Goodbye/' /tmp/e2e-neg/workflows/*.json
```
Then:
```
Workflow({ scriptPath: 'tools/ai-usability/harness.workflow.js',
  args: { e2eOnly: true, projectDirs: ['/tmp/e2e-neg'] } })
```
Expected: at least one `endpoints[*].pass === false` (actual greeting ≠ expected) and a `findings[*]` entry classifying it. Cleanup: `rm -rf /tmp/e2e-neg`.

- [ ] **Step 5: Commit**

```bash
git add tools/ai-usability/harness.workflow.js
git commit -m "feat(e2e): per-project boot + HTTP drive + triage"
```

---

### Task 3: Extend the e2e agent to upload, WebSocket, and SSE transports

HTTP alone leaves the exact gap from Finding D half-open. Add multipart upload, WebSocket, and SSE driving to the same agent prompt, with explicit `not_exercised` when a client is unavailable.

**Files:**
- Modify: `tools/ai-usability/harness.workflow.js` (extend the DRIVE section of `e2ePrompt`)

**Interfaces:**
- Consumes/Produces: same `E2E_RESULT_SCHEMA`; `endpoints[*].transport` now also `upload`/`ws`/`sse`

- [ ] **Step 1: Extend the DRIVE block in `e2ePrompt` — insert these lines after the HTTP/auth driving lines, before "CLEANUP:"**

```js
    'FILE UPLOAD (if a route uses upload.handle / accepts multipart): create a small temp file (`printf hello > /tmp/e2e-up.txt`) and send it:',
    '  curl -sS -o /tmp/body -w "%{http_code}" -F "file=@/tmp/e2e-up.txt" http://localhost:PORT<path>',
    '  Assert the response references the stored file (name/size/path per the brief). Record {transport:"upload", ...}.',
    'WEBSOCKET (if connections/ + a ws.send workflow exist): use a WS client. If `command -v websocat` succeeds, use it; else write a tiny client (e.g. a 15-line Node script using the `ws` package if available, or a Python `websockets` snippet). Connect to ws://localhost:PORT<ws path>, subscribe to the channel, THEN trigger the HTTP route whose workflow ws.send-s, and assert the pushed message arrives within 5s. Record {transport:"ws", ...}. If NO WS client can be run, record {transport:"ws", request, expected, actual:"no ws client available", pass:false} AND a finding only if the brief required ws — otherwise mark the endpoint actual:"not_exercised" and pass:false; never report pass:true for an unexercised transport.',
    'SSE (if a route streams via sse.send): start a listener in the background — `curl -N http://localhost:PORT<sse path> > /tmp/e2e-sse.out 2>&1 &` — then trigger the emitting workflow, wait up to 5s, and assert an event line appears in /tmp/e2e-sse.out. Kill the curl. Record {transport:"sse", ...}.',
```

- [ ] **Step 2: Parse-check**

Run: `node --check tools/ai-usability/harness.workflow.js 2>/dev/null; echo done`
Expected: `done` (no syntax error printed).

- [ ] **Step 3: Positive control for WS — run against the realtime example**

First confirm the example path: `ls examples/realtime-collab/connections 2>/dev/null && echo has-ws`. Then run:
```
Workflow({ scriptPath: 'tools/ai-usability/harness.workflow.js',
  args: { e2eOnly: true, projectDirs: ['/Users/marten/GolandProjects/noda/examples/realtime-collab'] } })
```
Expected: `boot_ok === true`; an `endpoints[*]` with `transport:"ws"` and `pass:true`; no `not_exercised` (a WS client was found/provisioned). If the example has no upload/SSE route, those transports simply won't appear — that's correct, not a skip.

- [ ] **Step 4: Commit**

```bash
git add tools/ai-usability/harness.workflow.js
git commit -m "feat(e2e): drive upload, websocket, and sse transports"
```

---

### Task 4: Wire E2E into the full harness + route findings (usability → Verify, runtime-bug → mini-verify, build-error → report)

Connect the E2E phase to the real Build pipeline and fold its findings back into the skeptical machinery so nothing is filed unrefuted.

**Files:**
- Modify: `tools/ai-usability/harness.workflow.js` (add `bugVerifyPrompt`; run `runE2E` after Build; route findings; extend the return value)

**Interfaces:**
- Consumes: `perBrief` from the Build pipeline; `BUILDER_SCHEMA` builders already write to `scratchRoot/<id>`; `E2E_FINDING`, `BUG_VERDICT_SCHEMA`, existing `evaluatorPrompt`/`VERDICT_SCHEMA`/`synthPrompt`
- Produces: workflow return `{ confirmed_findings, issues, e2e_results, confirmed_bugs }`

- [ ] **Step 1: Add the runtime-bug mini-verify prompt (after `e2ePrompt`)**

```js
function bugVerifyPrompt(f, p, conn) {
  return [
    'You are a skeptical evaluator. An e2e verifier claimed this is a NODA RUNTIME BUG (noda misbehaving despite correct config) while testing brief ' + p.id + ':',
    JSON.stringify(f, null, 2),
    '',
    `Try to REFUTE it. You have Bash and the live shared services (pg: ${conn.pg_base_url}, redis: ${conn.redis_base_url}). Re-boot the project at ${p.dir} the same way (set its db/redis env vars, migrate, ./bin/noda start --config ${p.dir}) and re-send the failing request to reproduce.`,
    'If the behavior is actually correct, or the failure was caused by the project being built wrong (not noda), set is_real_bug=false and explain. If you reproduce genuine noda misbehavior, set is_real_bug=true with the exact request/response as evidence. Default to is_real_bug=false when uncertain. Kill any server you start.',
    'Return the structured verdict.',
  ].join('\n')
}
```

- [ ] **Step 2: Insert the runtime-bug mini-verify into `runE2E` BEFORE the teardown agent call (so infra is still up)**

```js
  // Runtime-bug candidates: reproduce-or-refute against the still-live infra.
  const bugCandidates = e2e_results.flatMap((r) =>
    (r.findings || []).filter((f) => f.category === 'runtime-bug').map((f) => ({ f, brief_id: r.brief_id }))
  )
  const projById = Object.fromEntries(projects.map((p) => [p.id, p]))
  const confirmed_bugs = []
  for (const { f, brief_id } of bugCandidates) {
    const p = projById[brief_id] || { id: brief_id, dir: `${scratchRoot}/${brief_id}` }
    const v = await agent(bugVerifyPrompt(f, p, infra), { label: `e2e:bugverify:${brief_id}`, phase: 'E2E', schema: BUG_VERDICT_SCHEMA })
    if (v && v.is_real_bug) confirmed_bugs.push({ ...f, brief_id, verdict: v })
  }
```

Then change `runE2E`'s return to include `confirmed_bugs`:

```js
  await agent(teardownPrompt(), { label: 'e2e:teardown', phase: 'E2E' })
  const e2e_findings = e2e_results.flatMap((r) => (r.findings || []).map((f) => ({ ...f, brief_id: r.brief_id })))
  return { e2e_results, e2e_findings, confirmed_bugs }
```

- [ ] **Step 3: Update the `e2eOnly` branch return (Task 1 Step 4) to surface `confirmed_bugs`**

```js
  const e2e = await runE2E(projects, scratchRoot)
  return { e2e_results: e2e.e2e_results, e2e_findings: e2e.e2e_findings, confirmed_bugs: e2e.confirmed_bugs, confirmed_findings: [], issues: [] }
```

- [ ] **Step 4: Call `runE2E` after the Build pipeline and route usability findings into Verify. Replace the Synthesize section (~line 164–171) with:**

```js
// Run real-endpoint e2e on the built projects (after Build, before Synthesize).
const projects = briefs.map((b) => ({ id: b.id, dir: `${scratchRoot}/${b.id}`, briefPath: b.path }))
const e2e = await runE2E(projects, scratchRoot)

phase('Verify')
// e2e "usability" findings get the SAME adversarial MCP-surface refutation as build-time friction.
const e2eUsability = e2e.e2e_findings.filter((f) => f.category === 'usability')
const e2eConfirmed = []
for (let i = 0; i < e2eUsability.length; i++) {
  const f = e2eUsability[i]
  const asFriction = {
    goal: `endpoint behaved wrong (${f.transport})`,
    consulted: 'e2e run',
    missing_or_confusing: f.evidence,
    severity: 'major',
    suggested_fix: f.suggested_fix,
    category: 'missing-doc',
  }
  const v = await agent(evaluatorPrompt({ id: f.brief_id }, asFriction, `${scratchRoot}/${f.brief_id}`),
    { label: `verify:e2e:${f.brief_id}#${i}`, phase: 'Verify', schema: VERDICT_SCHEMA })
  if (v && v.is_real_gap) e2eConfirmed.push({ ...asFriction, brief_id: f.brief_id, verdict: v })
}

phase('Synthesize')
const confirmed_findings = [...perBrief.filter(Boolean).flatMap((x) => x.confirmed), ...e2eConfirmed]
log(`${confirmed_findings.length} confirmed gap(s); ${e2e.confirmed_bugs.length} confirmed runtime bug(s) across ${briefs.length} brief(s)`)
if (confirmed_findings.length === 0) {
  return { confirmed_findings: [], issues: [], e2e_results: e2e.e2e_results, confirmed_bugs: e2e.confirmed_bugs }
}
const synth = await agent(synthPrompt(confirmed_findings), { label: 'synthesize', phase: 'Synthesize', schema: ISSUES_SCHEMA })
return { confirmed_findings, issues: synth ? synth.issues : [], e2e_results: e2e.e2e_results, confirmed_bugs: e2e.confirmed_bugs }
```

- [ ] **Step 5: Update `meta.phases` (top of file) to include the E2E phase**

```js
  phases: [
    { title: 'Build', detail: 'one MCP-only builder subagent per brief' },
    { title: 'E2E', detail: 'boot each project & drive real HTTP/upload/ws/sse endpoints' },
    { title: 'Verify', detail: 'adversarial evaluator per friction & e2e finding' },
    { title: 'Synthesize', detail: 'dedup confirmed gaps into issue-ready entries' },
  ],
```

- [ ] **Step 6: Scoped end-to-end dry-run on one brief that uses services**

Run:
```
Workflow({ scriptPath: 'tools/ai-usability/harness.workflow.js', args: { only: ['03-data'] } })
```
Expected: the run shows Build → E2E → Verify → Synthesize phases; the returned object has `e2e_results` (one entry, `boot_ok:true` if the builder produced a runnable project), `confirmed_bugs`, `confirmed_findings`, `issues`; `docker ps --filter name=noda-e2e -q` empty afterward.

- [ ] **Step 7: Commit**

```bash
git add tools/ai-usability/harness.workflow.js
git commit -m "feat(e2e): wire E2E phase into harness + route findings/bugs through verify"
```

---

### Task 5: Report generation + README update

Make the e2e signal human-readable and document the new phase and standalone mode.

**Files:**
- Modify: `tools/ai-usability/README.md`
- Create (per run, by the operator after a run): `tools/ai-usability/findings/<date>-e2e.md` — documented, not code

**Interfaces:**
- Consumes: the workflow return `{ e2e_results, confirmed_bugs }`

- [ ] **Step 1: Add an "E2E phase" section to `README.md` after the existing "What it does" list**

```markdown
## E2E phase (real-endpoint verification)

After Build, the harness boots each project as a real server against ephemeral
Postgres + Redis and drives its real HTTP / file-upload / WebSocket / SSE
endpoints (`noda test` cannot — it executes workflows in-process with mocked
services). Failures are triaged:

- **runtime-bug** → reproduce-or-refute mini-verify → `confirmed_bugs`
- **usability** → the same adversarial MCP-surface refutation as build-time
  friction → folded into `issues`
- **build-error** → reported only

Requires Docker. If Docker is unavailable the phase logs a loud skip and
records `status:"skipped"` per project — it never reports green by omission.

### Standalone mode

Run just the E2E phase against explicit project dirs (used for the
known-good / injected-fault controls, or ad-hoc checks):

​```
Workflow({ scriptPath: 'tools/ai-usability/harness.workflow.js',
  args: { e2eOnly: true,
          projectDirs: ['/abs/path/to/examples/init-example'] } })
​```
```

- [ ] **Step 2: Add the report-writing step to the "Findings → issues" section of `README.md`**

```markdown
3. Write the returned `e2e_results` + `confirmed_bugs` to
   `findings/<YYYY-MM-DD>-e2e.md`: a per-project boot result, a transport×endpoint
   pass/fail table, and each failure's request / expected / actual / triage
   verdict. Commit it alongside the friction report.
```

- [ ] **Step 3: Verify the README renders the new sections**

Run: `grep -c '^## E2E phase\|^### Standalone mode' tools/ai-usability/README.md`
Expected: `2`

- [ ] **Step 4: Commit**

```bash
git add tools/ai-usability/README.md
git commit -m "docs(e2e): document E2E phase, standalone mode, and the e2e report"
```

---

## Self-Review

**Spec coverage:**
- Harness-internal `E2E` phase → Tasks 1–4. ✅
- Workflow-script-has-no-shell constraint (agents do all shell work) → infra/teardown/e2e/bugverify are all agents. ✅
- Shared ephemeral Postgres+Redis, per-project DB, migrations → Task 1 infra agent + Task 2 e2e SETUP steps. ✅
- Agent-driven per-project request/assertion authoring → Task 2/3 `e2ePrompt`. ✅
- Sequential execution + free-port-before-boot → Task 2 loop + SETUP step 4. ✅
- All four transports (http/upload/ws/sse) → Tasks 2 (http) + 3 (upload/ws/sse). ✅
- Triage into runtime-bug/usability/build-error → `E2E_FINDING.category` + Task 4 routing. ✅
- usability → existing Verify; runtime-bug → mini-verify; build-error → report-only → Task 4. ✅
- Return value gains `e2e_results` + `confirmed_bugs` → Task 4 returns. ✅
- Report to `findings/<date>-e2e.md` → Task 5. ✅
- Error handling: Docker-absent skip (loud, not green), boot failure as finding, not_exercised transports, unconditional last teardown → Task 1 skip branch, Task 2 boot-failure finding, Task 3 not_exercised rule, Task 4 teardown-after-bugverify. ✅
- Four acceptance checks (positive/negative/no-Docker/self-contained) → Task 1 Step 6 (no-Docker), Task 2 Steps 3–4 (positive/negative HTTP), Task 3 Step 3 (WS positive). Self-contained = init-example (no services) covered by Task 2 Step 3. ✅

**Placeholder scan:** No "TBD"/"handle errors appropriately" — every agent prompt and wiring block is complete. The `<date>-e2e.md` filename is an intentional per-run value, not a code placeholder.

**Type consistency:** `runE2E` returns `{ e2e_results, e2e_findings, confirmed_bugs }` consistently (Task 1 skeleton omits `confirmed_bugs`; Task 4 Step 2 adds it — the e2eOnly caller is updated in Task 4 Step 3 to match). `E2E_FINDING.category` enum values (`runtime-bug`/`usability`/`build-error`) match the triage routing in Task 4. `sanitize()` defined once (Task 2) and used in `e2ePrompt`. Builder `BRIEFS` items expose `.path` (used as `briefPath`). ✅
