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
