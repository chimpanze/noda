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
    status: { type: 'string' },
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
    `6. Health-poll up to 20s (\`curl -sf http://localhost:PORT/<any defined route>\`). If it never comes up: boot_ok=false, boot_log_tail=\`tail -n 40 /tmp/e2e-${p.id}.log\`, kill PID, add a finding {transport:"boot"}, STOP.`,
    '',
    'DRIVE (only if boot_ok) — for each HTTP route the brief implies, send a REAL request and assert:',
    '  curl -sS -o /tmp/body -w "%{http_code}" -X <METHOD> http://localhost:PORT<path> -H "Content-Type: application/json" -d \'<json body>\'',
    '  Assert the status code AND the JSON fields the brief specifies (cat /tmp/body). Record each as an endpoint entry {transport:"http", request, expected, actual, pass}.',
    'For auth-gated routes: send once WITHOUT a token (expect 401/403) and once WITH a valid token minted per the project\'s jwt config; assert the gate behaves. Record both.',
    'FILE UPLOAD (if a route uses upload.handle / accepts multipart): create a small temp file (`printf hello > /tmp/e2e-up.txt`) and send it:',
    '  curl -sS -o /tmp/body -w "%{http_code}" -F "file=@/tmp/e2e-up.txt" http://localhost:PORT<path>',
    '  Assert the response references the stored file (name/size/path per the brief). Record {transport:"upload", ...}.',
    'WEBSOCKET (if connections/ + a ws.send workflow exist): use a WS client. If `command -v websocat` succeeds, use it; else write a tiny client (e.g. a 15-line Node script using the `ws` package if available, or a Python `websockets` snippet). Connect to ws://localhost:PORT<ws path>, subscribe to the channel, THEN trigger the HTTP route whose workflow ws.send-s, and assert the pushed message arrives within 5s. Record {transport:"ws", ...}. If NO WS client can be run, record {transport:"ws", request, expected, actual:"no ws client available", pass:false} AND a finding only if the brief required ws — otherwise mark the endpoint actual:"not_exercised" and pass:false; never report pass:true for an unexercised transport.',
    'SSE (if a route streams via sse.send): start a listener in the background — `curl -N http://localhost:PORT<sse path> > /tmp/e2e-sse.out 2>&1 &` — then trigger the emitting workflow, wait up to 5s, and assert an event line appears in /tmp/e2e-sse.out. Kill the curl. Record {transport:"sse", ...}.',
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

  try {
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

    // Runtime-bug mini-verify lands in Task 4 (must run while infra is up).

    const e2e_findings = e2e_results.flatMap((r) => (r.findings || []).map((f) => ({ ...f, brief_id: r.brief_id })))
    return { e2e_results, e2e_findings }
  } finally {
    await agent(teardownPrompt(), { label: 'e2e:teardown', phase: 'E2E' })
  }
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

// args may arrive as an object or as a JSON-encoded string; normalize both.
let opts = args
if (typeof opts === 'string') {
  try { opts = JSON.parse(opts) } catch (e) { opts = {} }
}
opts = opts || {}

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

phase('Build')
// NOTE: the 3-arg pipeline(items, buildFn, verifyFn) and parallel(arrayOfThunks)
// forms below are intentional and runtime-validated (a full 5-brief run spawned
// 21 agents correctly). Do not "simplify" them to the 2-arg pipeline form.
const perBrief = await pipeline(
  briefs,
  (b) => agent(builderPrompt(b, `${scratchRoot}/${b.id}`), { label: `build:${b.id}`, phase: 'Build', schema: BUILDER_SCHEMA }),
  (result, b) => {
    if (!result) {
      // A null result means the builder agent errored/was skipped — the brief was
      // never exercised. Surface it so a dropped brief is visible, not silently clean.
      log(`build:${b.id} returned no result — brief not exercised (skipped in findings)`)
      return { brief: b.id, confirmed: [] }
    }
    if (!Array.isArray(result.friction) || result.friction.length === 0) {
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
