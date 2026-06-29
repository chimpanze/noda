# Runtime-verification gap — 2026-06-25 (follow-up to the harness re-run)

The AI-usability harness re-run scored **0 confirmed friction findings** (down from 11) — see `2026-06-25-rerun.md`. That measures *usability* (can an AI navigate the MCP surface without getting stuck). It does **not** measure whether the projects the AI built actually work at runtime.

To check that, we ran the 5 scratch projects the builders produced (`/tmp/noda-ai-usability/*`) through `noda test`, then probed deeper. Four distinct gaps surfaced. None were visible to the harness.

---

## Finding A — AI builders write test assertions in the wrong convention (all 5 projects)

Every project's `noda test` run **failed**, despite the workflow executing with `status=success`. Root cause (confirmed on `01-trivial`): the AI wrote

```json
"expect": { "output": { "message": "Hello, world!" } }
```

but the test runner resolves `output` dot-paths against a map **keyed by node ID**, and a `response.json` node's output is a typed `*api.HTTPResponse` struct. The canonical form (from the curated `examples/init-example/tests/hello.test.json`) is

```json
"expect": { "outputs": { "respond": { "Body": { "message": "Hello, world!" } } } }
```

Three independent mistakes the AI made consistently:
1. `output` (singular) vs `outputs` (plural) — only the plural form JSON-normalizes structs; the singular form can't traverse `*api.HTTPResponse`.
2. Bare field name vs **node-ID-keyed** path (`respond.…`).
3. `body` vs **`Body`** — the assertion key is the Go struct field name, not a lowercase JSON tag.

Patching `01-trivial` to the canonical form → **test passes**. So the *workflows are runtime-correct*; the AI just doesn't know the test-output convention. Likely a doc/example gap (`noda://docs/testing` and/or `noda_get_examples`).

## Finding B — AI tests only the scaffold workflow, not the brief

Each project ships exactly one test file (`tests/hello.test.json`, one case) for the default scaffold greeter — even where the brief built real features:

| Brief | Workflows built | Tests written |
|-------|-----------------|---------------|
| 03-data | 10 (notes CRUD) | 1 (hello greeter) |
| 04-auth | 6 (auth-gated routes) | 1 (hello greeter) |
| 05-hard | 4 (websocket messaging) | 1 (hello greeter) |

The actual brief functionality has **zero test coverage**. The builders never wrote tests for what they built.

## Finding C — the harness build step is blind to runtime

`harness.workflow.js` STEP 5 asks builders to run only `noda_validate_config` (static structural validation). It never runs `noda test`. So:
- builders' tests never executed → broken assertions (Finding A) produced **no friction signal**;
- builders reported `completed=true` on the strength of static validation alone.

This is why the harness scored 0 while the projects don't actually pass their own tests.

## Finding D — `noda test` cannot exercise the transport layer at all

Even with correct tests, `noda test` calls `engine.Compile` + `engine.ExecuteGraph` **directly** — no server, no port, no real request:

- **targets a workflow by ID**, bypassing the `routes/*.json` layer (path/query params, body-schema binding, content-type all skipped);
- **input is synthetic JSON** (`engine.WithInput`) — no real HTTP request object;
- **auth is injected pre-parsed** (`engine.WithAuth` roles/claims) — JWT verification + casbin enforcement skipped;
- **every plugin/service node is mocked or unmocked** — `db`, `http`, `storage`, `email`, and crucially `upload.handle`, `ws.send`, `sse.send` never do real I/O.

| Layer | Covered by `noda test`? |
|-------|--------------------------|
| Workflow node logic, data flow, expressions | ✅ |
| HTTP routing / path & query params / body validation | ❌ |
| JWT verification + casbin authorization | ❌ |
| HTTP multipart **file upload** (`upload.handle`) | ❌ |
| **WebSocket** endpoint (handshake, subscribe, delivery) | ❌ |
| **SSE** streaming endpoint | ❌ |

There is currently **no config-project-level way, via the noda CLI or MCP surface, to verify HTTP/WS/SSE/upload endpoints actually work.** Real HTTP-layer coverage exists only as Go tests in `internal/server/*_test.go` (the framework's own tests). The `testdata/node-e2e` "e2e" suite is still `.test.json` run through the same engine path — node coverage, not transport. The only path to a true transport signal today: `noda start` against real services, then drive it with an external client (`curl -F` for upload, a WS client, an SSE client).

---

## The testing ladder (what each rung proves)

1. `noda validate` — config is structurally valid. ✅ all 5 builds.
2. `noda test` (correct assertions) — workflow node logic + data flow + expressions, synthetic input, mocked services.
3. **(missing)** real-endpoint e2e — boot the server, hit real HTTP/upload/WS/SSE endpoints through routing + middleware + real services.

The AI-usability harness only ever reaches rung 1 (and reports usability friction along the way). Rungs 2 and 3 are unverified for AI-built projects.

## Recommendations

- **Doc fix** (Finding A): document the test-output assertion convention (`outputs` / node-ID key / `Body`) prominently in `noda://docs/testing`, and make `noda_get_examples` surface a response-asserting test.
- **Harness fix** (Finding C): have the build step run `noda test` and treat a failing/missing test as friction, so runtime breakage is caught in the loop.
- **New capability** (Finding D): build a real-endpoint e2e check that boots `noda start` with services and exercises HTTP/upload/WS/SSE. Design TBD — see the brainstorming/plan that follows this write-up.
