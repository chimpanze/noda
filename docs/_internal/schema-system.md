# Noda — Schema System

**Status**: Complete

This document describes how user-authored JSON Schemas flow through Noda: how
`schemas/` files are classified, how `$ref` names are derived and resolved, and
where the resulting schemas are consumed at runtime. It is the reference for
anyone touching `internal/config/refs.go`, the server-side validators, or a
consumer of the schema registry (e.g. the OpenAPI generator).

---

## 1. Two meanings of "schema"

The word is overloaded in this codebase. They are unrelated subsystems:

| | **User schemas** | **Config schemas** |
|---|---|---|
| Location | `schemas/*.json` in a project | `internal/config/schemas/*.json` (embedded) |
| Purpose | Validate the project's API request/response data | Validate the project's *config files* (`route.json`, `workflow.json`, …) |
| Consumed by | route validators, `transform.validate`, OpenAPI gen | `internal/config` `Validate` step |

**This document is about user schemas.** Config schemas are a fixed, embedded
set that gates config-file structure; they never change per project and are out
of scope here.

## 2. File shapes

A file under `schemas/` is exactly one of two shapes. Noda decides which by
inspecting the **value shapes** of the top-level keys — never by filename and
never by mere keyword presence. The classifier is `classifySchemaFile`
(`internal/config/refs.go:207`).

### Named-definitions file

Each top-level key is a schema definition:

```json
// schemas/Task.json
{
  "Task":       { "type": "object", "properties": { ... } },
  "CreateTask": { "type": "object", "required": ["title"] }
}
```

Registers `schemas/Task` and `schemas/CreateTask`. **The filename is
irrelevant** — ref names come from the keys.

### Bare schema file

The file is itself a single JSON Schema document:

```json
// schemas/greeting.json
{ "type": "object", "properties": { "message": { "type": "string" } } }
```

Registers one ref under its **filename**: `schemas/greeting`.

### How the two are told apart

The classifier is a three-way decision, driven by whether a JSON Schema keyword
appears with the value shape that keyword actually takes in a schema document
(`bareSchemaKeywords`, `refs.go:165`):

| Top level contains… | Classification |
|---|---|
| a keyword with its **correct** shape — `type` string/array, `$schema`/`$ref` string, `enum`/`oneOf`/`anyOf`/`allOf` array | **bare schema** |
| a keyword with the **wrong** shape — e.g. `"type": { ... }` (an object) | **named-definitions** — the wrong shape *proves* the key is a definition name, not the keyword. Holds even if `properties`/`items` is also present. |
| `properties` or `items`, but **none** of those keywords at all | **ambiguous → hard error** |

The wrong-shape rule is load-bearing: `{"type": {...}, "properties": {...}}` is
a fully decidable named-definitions file (two definitions, `type` and
`properties`). An earlier design treated a wrong-shaped keyword as neutral and
would have called this ambiguous — silently losing both definitions, which is
exactly the class of bug this system exists to prevent (#405).

The ambiguous case is genuinely undecidable — `properties`/`items` take object
values both as keywords and as definition names, so shape cannot separate the
readings. Rather than guess, Noda rejects the file and asks the author to add
`"type"` (making it bare) or rename the definition (making it named).

### Subdirectories

The directory path from `schemas/` onward is part of the ref name
(`extractSchemasRelPath`, `refs.go:224`):

```
schemas/validation/User.json  key "CreateUser"  →  schemas/validation/CreateUser
schemas/validation/greeting.json  (bare)         →  schemas/validation/greeting
```

This is why `schemas/billing/Invoice.json` and `schemas/orders/Invoice.json`
coexist without colliding: they register `schemas/billing/Invoice` and
`schemas/orders/Invoice`.

## 3. The registry (load time)

`BuildSchemaRegistry` (`refs.go:62`) walks every `schemas/` file and produces a
single map: **ref name → schema definition**. This is the authoritative schema
namespace.

Two contracts matter for consumers:

- **Uniqueness is enforced.** If two definitions register the same ref name —
  two files in one directory sharing a top-level key, or a bare `schemas/User.json`
  alongside another file's `User` key — `BuildSchemaRegistry` returns a
  `ValidationError` naming both contributors, and the config is rejected. Before
  #405 the last writer won, and because Go randomizes map iteration, *which* one
  won varied between boots of the same config — a silent, nondeterministic
  validation bypass.
- **On error the registry is `nil`.** When it returns any errors it returns a
  nil map, never a partially- or arbitrarily-populated one. A consumer that
  ignores the errors and reads the registry anyway gets a nil-map read, not a
  plausible-looking wrong answer. This guards the exported API against
  reintroducing the very bug it was built to fix.

The registry is exposed on `ResolvedConfig` two ways (`internal/config/pipeline.go`):

- `Schemas` — keyed by **file path** (legacy; predates #405)
- `SchemaRegistry` — keyed by **ref name** (the string a config writes in `$ref`)

New consumers should use `SchemaRegistry`. `Schemas` being keyed by file path is
the direct cause of the OpenAPI generator's dangling-`$ref` bug (see §5).

## 4. `$ref` resolution and inlining (load time)

`ResolveRefs` (`refs.go:13`) builds the registry, then walks **routes,
workflows, workers, schedules, connections, tests, and models** and replaces
every `{ "$ref": "schemas/CreateTask" }` with the referenced schema, **inlined
in place**. It resolves nested refs and rejects cycles.

**The critical consequence:** after load, no `$ref` values remain anywhere.
Every runtime consumer sees a literal, fully-expanded schema map. The registry
is purely a load-time artifact; nothing resolves refs at request time.

Errors surfaced here (all fail at `noda validate`, dry-run, **and** boot — never
at runtime):

- duplicate ref name (from `BuildSchemaRegistry`)
- ambiguous schema file (from `BuildSchemaRegistry`)
- unresolved `$ref` — the error lists every registered ref name
- circular `$ref` — the error prints the cycle

All error text is deterministic: anything derived from a map is sorted before
formatting, so a config either always passes or always fails with byte-identical
output.

## 5. Where schemas are consumed (runtime)

Once inlined, a schema is a literal map and is used in exactly these places:

### Route request/response validation

`internal/server/routes.go:91-135` compiles validators **once at route
registration** (santhosh-tekuri/jsonschema v6, via `newBodyValidator`,
`validate.go:21`):

- `body.schema` — request body. On by default; `"validate": false` disables it.
- `params.schema` — path parameters.
- `query.schema` — query string.
- `response.<status>.schema` — **outgoing** response body, per status code
  (`newResponseValidator`, `validate.go:115`). Keyed by numeric status; non-numeric
  keys like `validate`/`description` are skipped.

Validation failures return structured, field-level errors
(`collectBodyErrors`, `validate.go:148`).

### The `transform.validate` node

`plugins/core/transform/validate.go` takes a `schema` in its node config,
compiles it once at factory time, and validates data **mid-workflow**. This is
the in-workflow counterpart to edge validation on routes.

### OpenAPI generation

`internal/server/openapi.go:37` and `internal/editor/codegen.go:409` read
schemas to emit `components/schemas`. **Both currently read `rc.Schemas` (keyed
by file path)**, which is why generated component `$ref`s dangle: components are
registered under file paths while refs are emitted by basename. The fix is to
read `SchemaRegistry` (keyed by ref name); that is the OpenAPI generator
unification work, which #405 unblocked by making the registry authoritative and
correctly keyed.

### The visual editor

`internal/editor/nodes.go:167` and `internal/editor/files.go` enumerate schema
files for the editor UI.

## 6. Lifecycle summary

```
discovery.go   scan schemas/**/*.json
     ▼
loader.go      parse each file → RawConfig.Schemas (keyed by file path)
     ▼
refs.go        BuildSchemaRegistry → SchemaRegistry (keyed by ref name)
                 └─ rejects: duplicate ref name, ambiguous file
     ▼
refs.go        ResolveRefs → inline every $ref into routes/workflows/…
                 └─ rejects: unresolved $ref, circular $ref
     ▼
validator.go   Validate config files against embedded config schemas
     ▼
(boot)         server.routes    → compile body/params/query/response validators
               transform.validate → compile node schema
```

**Mental model:** user schemas are named JSON Schema fragments, resolved and
inlined into the config at load time, then compiled into validators that run at
the HTTP edge (routes) or inside workflows (`transform.validate`). The #405 work
made the *naming and resolution* stage deterministic and fail-loud; it changed
nothing about the validation that runs afterward.

## 7. See also

- `docs/02-config/schemas.md` — user-facing reference for authoring `schemas/` files
- `internal/config/refs.go` — registry construction, classification, `$ref` resolution
- `internal/server/validate.go` — runtime request/response validators
- `plugins/core/transform/validate.go` — the `transform.validate` node
